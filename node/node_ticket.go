package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) GetSelfTicketsIDs(currPhase uint32) ([]common.Hash, error) {
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	ticketsId := make([]common.Hash, 0)
	usedEntropy := n.statedb.GetSafrole().Entropy[3]
	if currPhase == 0 {
		usedEntropy = n.statedb.GetSafrole().Entropy[2]
	}
	for _, ticketbucket := range n.selfTickets[usedEntropy] {
		//pre-calculated ticket id
		ID := ticketbucket.TicketID
		ticketsId = append(ticketsId, ID)
	}
	return ticketsId, nil
}

// this function is now won't broadcast the tickets to the network
func (n *Node) generateEpochTickets(usedEntropy common.Hash) ([]types.TicketBucket, error) {
	sf := n.statedb.GetSafrole()
	auth_secret, _ := statedb.ConvertBanderSnatchSecret(n.GetBandersnatchSecret())
	tickets, microseconds := sf.GenerateTickets(auth_secret, usedEntropy)
	n.ticketsMutex.Lock()
	if n.selfTickets[usedEntropy] == nil {
		n.selfTickets[usedEntropy] = make([]types.TicketBucket, 0)
	}
	n.ticketsMutex.Unlock()
	buckets := types.TicketsToBuckets(tickets, microseconds, sf.GetEpoch())
	n.ticketsMutex.Lock()
	n.selfTickets[usedEntropy] = buckets
	n.ticketsMutex.Unlock()

	return buckets, nil
}

func (n *Node) GenerateTickets(jce uint32) {
	sf := n.statedb.GetSafrole()
	actualEpoch, _ := sf.EpochAndPhase(jce)
	// timeslot mark
	currEpoch, _ := sf.EpochAndPhase(jce)
	usedEntropy := n.statedb.GetSafrole().Entropy[2]
	if n.statedb.GetSafrole().IsTicketSubmissionClosed(jce) {
		log.Trace(debugT, "GenerateTickets: Using Entropy 1 for Node to generate tickets", "n", n.id)
		copy(usedEntropy[:], n.statedb.GetSafrole().Entropy[1][:])
	}
	n.ticketsMutex.Lock()
	l := len(n.selfTickets[usedEntropy])
	n.ticketsMutex.Unlock()
	if l == types.TicketEntriesPerValidator {
		log.Trace(debugT, "GenerateTickets:Node has generated tickets", "n", n.id, "l", types.TicketEntriesPerValidator, "currEpoch", currEpoch, "actualEpoch", actualEpoch, "usedEntropy", usedEntropy)
		return
	}
	if !n.IsTicketGenerated(usedEntropy) {
		n.generateEpochTickets(usedEntropy)
	}
}

func (n *Node) IsTicketGenerated(entropy common.Hash) bool {
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	_, ok := n.selfTickets[entropy]
	return ok
}
func (n *Node) BroadcastTickets(currJCE uint32) {
	if n.sendTickets == false {
		return
	}
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	sf := n.statedb.GetSafrole()
	currEpoch, _ := sf.EpochAndPhase(currJCE)
	if currEpoch < 0 {
		return
	}
	usingEntropy := n.statedb.GetSafrole().Entropy[2]

	if n.statedb.GetSafrole().IsTicketSubmissionClosed(currJCE) {
		usingEntropy = n.statedb.GetSafrole().Entropy[1]
	}
	ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
	defer cancel() // ensures context is released
	tickets := n.selfTickets[usingEntropy]
	for _, ticketbucket := range tickets {
		ticket := ticketbucket.Ticket
		if !*ticketbucket.IsBroadcasted {
			log.Trace(debugT, "Broadcasting Ticket", "n", n.id, "r", ticket.Attempt)
			go func() {
				defer cancel() // ensures context is released
				_ = n.broadcast(ctx, ticket, "elapsedMicroseconds", ticketbucket.ElapsedMicroseconds)
			}()
			*ticketbucket.IsBroadcasted = true
		} else {
			if !*ticketbucket.IsIncluded && n.resendTickets {
				go func() {
					defer cancel() // ensures context is released
					_ = n.broadcast(ctx, ticket)
					fmt.Printf("[N%v] Broadcasted Ticket %v\n", n.id, ticket.Attempt)
				}()
				*ticketbucket.IsBroadcasted = true
			}
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
