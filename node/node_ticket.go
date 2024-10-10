package node

import (

	//	"encoding/binary"

	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	//	"io"
)

func (n *Node) GetSelfTicketsIDs() ([]common.Hash, error) {
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	ticketsId := make([]common.Hash, 0)
	usedEntropy := n.statedb.GetSafrole().Entropy[3]
	// currJCE := statedb.ComputeCurrentJCETime()
	// if n.statedb.GetSafrole().IsNewEpoch(currJCE) && n.statedb.GetSafrole().Epoch > 0 {
	// 	usedEntropy = n.statedb.GetSafrole().Entropy[2]
	// }
	for _, ticketbucket := range n.selfTickets[usedEntropy] {
		ID, err := ticketbucket.Ticket.TicketID()
		if err != nil {
			return nil, err
		}
		ticketsId = append(ticketsId, ID)
	}
	return ticketsId, nil
}

// this function is now won't broadcast the tickets to the network
func (n *Node) generateEpochTickets(usedEntropy common.Hash) ([]types.TicketBucket, error) {
	sf := n.statedb.GetSafrole()
	auth_secret, _ := sf.ConvertBanderSnatchSecret(n.GetBandersnatchSecret())
	tickets := sf.GenerateTickets(auth_secret, usedEntropy)
	if n.selfTickets[usedEntropy] == nil {
		n.selfTickets[usedEntropy] = make([]types.TicketBucket, 0)
	}
	fmt.Printf("[N%v] Generating Tickets for = (%v)\n", n.id, usedEntropy)
	buckets := types.TicketsToBuckets(tickets, sf.Epoch)
	n.selfTickets[usedEntropy] = buckets
	for _, bucket := range n.selfTickets[usedEntropy] {
		fmt.Printf("[N%v] Put Ticket %x in Bucket:%v\n", n.id, bucket.Ticket.Attempt, usedEntropy)
	}
	return buckets, nil
}

func (n *Node) GenerateTickets() {
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	sf := n.statedb.GetSafrole()
	actualEpoch, _ := sf.EpochAndPhase(n.statedb.GetSafrole().Timeslot)
	currEpoch, _ := sf.EpochAndPhase(common.ComputeCurrentJCETime())
	usedEntropy := n.statedb.GetSafrole().Entropy[2]
	if n.statedb.GetSafrole().IsTicketSubmsissionClosed(n.statedb.GetSafrole().Timeslot) {
		fmt.Printf("Using Entropy 1 for Node %v to generate tickets\n", n.id)
		copy(usedEntropy[:], n.statedb.GetSafrole().Entropy[1][:])
	}
	fmt.Printf("GenerateTickets currEpoch=%v | actualEpoch=%v selected Entropy=%v\n", currEpoch, actualEpoch, usedEntropy)
	if len(n.selfTickets[usedEntropy]) == types.TicketEntriesPerValidator {
		fmt.Printf("Node %v has generated %v tickets in epoch %v\n", n.id, types.TicketEntriesPerValidator, currEpoch)
		return
	}
	if !n.IsTicketGenerated(usedEntropy) {
		n.generateEpochTickets(usedEntropy)
	}
}

func (n *Node) IsTicketGenerated(entropy common.Hash) bool {
	_, ok := n.selfTickets[entropy]
	return ok
}
func (n *Node) CheckSelfTicketsIsIncluded(Block types.Block, currJCE uint32) {
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	currEpoch, _ := n.statedb.GetSafrole().EpochAndPhase(currJCE)
	if currEpoch < 0 {
		return
	}
	fmt.Printf("[N%v] Checking Self Tickets for Epoch %v\n", n.id, currEpoch)
	tickets := n.selfTickets[n.statedb.GetSafrole().Entropy[2]]
	if n.statedb.GetSafrole().IsTicketSubmsissionClosed(currJCE) {
		tickets = n.selfTickets[n.statedb.GetSafrole().Entropy[1]]
	}
	if tickets == nil {
		fmt.Printf("[N%v] No Tickets for Epoch %v\n", n.id, currEpoch)
		return
	}
	for _, ticketbucket := range tickets {
		ticket := ticketbucket.Ticket
		extrinsic_tickets := Block.Extrinsic.Tickets
		for _, extrinsic_ticket := range extrinsic_tickets {
			ticket_id, _ := extrinsic_ticket.TicketID()
			ticket_id2, _ := ticket.TicketID()
			if ticket_id == ticket_id2 {
				*ticketbucket.IsIncluded = true
			}
		}
	}
	return
}

func (n *Node) BroadcastTickets() {
	sf := n.statedb.GetSafrole()
	currJCE := sf.Timeslot
	currEpoch, _ := sf.EpochAndPhase(currJCE)
	if currEpoch < 0 {
		return
	}
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	usingEntropy := n.statedb.GetSafrole().Entropy[2]

	if n.statedb.GetSafrole().IsTicketSubmsissionClosed(currJCE) {
		usingEntropy = n.statedb.GetSafrole().Entropy[1]
	}
	tickets := n.selfTickets[usingEntropy]
	for _, ticketbucket := range tickets {
		if !*ticketbucket.IsIncluded {
			ticket := ticketbucket.Ticket
			fmt.Printf("[N%v] Broadcasting Ticket %x\n", n.id, ticket.Attempt)
			if !*ticketbucket.IsBroadcasted {
				n.broadcast(ticket)
				*ticketbucket.IsBroadcasted = true
			}
			//n.writeDebug(&ticket, currJCE)
		}
	}
}

func (n *Node) CleanUpSelfTickets(currJCE uint32) {
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	currEpoch, currPhase := n.statedb.GetSafrole().EpochAndPhase(currJCE)
	// we don't need to clean up every time to run below code
	// every few epochs is enough
	if (currEpoch+1)%types.MaxEpochsToKeepSelfTickets > 0 && currPhase > 0 {
		return
	}
	if currEpoch < 0 {
		return
	}
	fmt.Printf("[N%v] Cleaning up Tickets for Epoch %v\n", n.id, currEpoch)
	for etp, tickets := range n.selfTickets {
		for _, ticketbucket := range tickets {
			// if the ticket is older than 5 epochs, remove it
			if currEpoch-int32(ticketbucket.GeneratedEpoch) > types.MaxEpochsToKeepSelfTickets {
				fmt.Printf("[N%v] Cleaning up Ticket %v\n", n.id, etp)
				delete(n.selfTickets, etp)
			} else {
				continue
			}
		}
	}
}
