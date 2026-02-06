package node

import (
	"context"
	"fmt"

	"github.com/jam-duna/jamduna/common"
	log "github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/statedb"
	types "github.com/jam-duna/jamduna/types"
)

func (n *Node) GetSelfTicketsIDs(currPhase uint32, isEpochChanged bool) ([]common.Hash, error) {
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	ticketsId := make([]common.Hash, 0)
	usedEntropy := n.statedb.GetSafrole().Entropy[3]
	if isEpochChanged {
		usedEntropy = n.statedb.GetSafrole().Entropy[2]
	}
	for _, ticketbucket := range n.selfTickets[usedEntropy] {
		//pre-calculated ticket id
		ID := ticketbucket.TicketID
		ticketsId = append(ticketsId, ID)
	}
	if len(ticketsId) == 0 {
		return nil, fmt.Errorf("no tickets found")
	}
	return ticketsId, nil
}

// this function is now won't broadcast the tickets to the network
func (n *Node) generateEpochTickets(usedEntropy common.Hash) ([]types.TicketBucket, error) {
	sf := n.statedb.GetSafrole()

	authSecret, err := statedb.ConvertBanderSnatchSecret(n.GetBandersnatchSecret())
	if err != nil {
		return nil, fmt.Errorf("convert bandersnatch secret: %w", err)
	}

	tickets, microseconds := sf.GenerateTickets(authSecret, usedEntropy)
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

func (n *Node) GenerateTickets(jce uint32) (eventID uint64) {
	eventID = 0
	sf := n.statedb.GetSafrole()
	actualEpoch, _ := sf.EpochAndPhase(jce)
	// timeslot mark
	currEpoch, _ := sf.EpochAndPhase(jce)
	usedEntropy := n.statedb.GetSafrole().Entropy[2]
	if n.statedb.GetSafrole().IsTicketSubmissionClosed(jce) {
		log.Trace(log.Node, "GenerateTickets: Using Entropy 1 for Node to generate tickets", "n", n.id)
		copy(usedEntropy[:], n.statedb.GetSafrole().Entropy[1][:])
	}
	n.ticketsMutex.Lock()
	l := len(n.selfTickets[usedEntropy])
	n.ticketsMutex.Unlock()
	if l == types.TicketEntriesPerValidator {
		log.Trace(log.Node, "GenerateTickets:Node has generated tickets", "n", n.id, "l", types.TicketEntriesPerValidator, "currEpoch", currEpoch, "actualEpoch", actualEpoch, "usedEntropy", usedEntropy)
		return
	}
	if !n.IsTicketGenerated(usedEntropy) {
		epoch := sf.GetEpoch()
		eventID = n.telemetryClient.GetEventID(epoch)
		n.telemetryClient.GeneratingTickets(epoch)

		buckets, err := n.generateEpochTickets(usedEntropy)
		if err != nil {
			n.telemetryClient.TicketGenerationFailed(eventID, err.Error())
			log.Warn(log.Node, "GenerateTickets: generateEpochTickets failed", "err", err)
			return
		}

		vrfOutputs := make([][32]byte, 0, len(buckets))
		for _, bucket := range buckets {
			var output [32]byte
			copy(output[:], bucket.TicketID.Bytes())
			vrfOutputs = append(vrfOutputs, output)
		}

		n.telemetryClient.TicketsGenerated(eventID, vrfOutputs)
	}
	return eventID
}

func (n *Node) IsTicketGenerated(entropy common.Hash) bool {
	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()
	_, ok := n.selfTickets[entropy]
	return ok
}
func (n *Node) BroadcastTickets(currJCE uint32, eventID uint64) {
	if !n.sendTickets {
		return
	}

	n.ticketsMutex.Lock()
	defer n.ticketsMutex.Unlock()

	sf := n.statedb.GetSafrole()
	currEpoch, _ := sf.EpochAndPhase(currJCE)
	if currEpoch < 0 {
		return
	}

	entropy := sf.Entropy[2]
	if sf.IsTicketSubmissionClosed(currJCE) {
		entropy = sf.Entropy[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
	defer cancel()

	for _, bucket := range n.selfTickets[entropy] {
		ticket := bucket.Ticket
		proxy, err := ticket.ProxyValidator()
		if err != nil {
			continue
		}
		shouldSend := !*bucket.IsBroadcasted || (!*bucket.IsIncluded && n.resendTickets)
		selfCurrentIdx := uint16(sf.GetCurrValidatorIndex(n.GetEd25519Key()))
		if shouldSend {
			if proxy == selfCurrentIdx {
				n.broadcast(ctx, ticket, eventID) // we are the proxy, so just use CE132
			} else {
				// first step: send to proxy with CE131
				// Get proxy's Ed25519 key from current safrole (not stale PeerID)
				proxyPubKey := sf.CurrValidators[proxy].Ed25519
				peerKey := proxyPubKey.SAN()
				peer, ok := n.peersByPubKey[peerKey]
				if !ok {
					log.Warn(log.Quic, "SendTicketDistribution", "n", n.String(), "proxy", proxy, "proxyPubKey", proxyPubKey.SAN(), "err", "peer not found")
					continue
				}
				epoch := n.getEpoch()
				if err := peer.SendTicketDistribution(ctx, epoch, ticket, true, eventID); err != nil {
					log.Warn(log.Quic, "SendTicketDistribution", "n", n.String(), "->peerKey", peer.SanKey(), "err", err)
				}
			}
			*bucket.IsBroadcasted = true
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
