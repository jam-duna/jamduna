package node

import (
	"fmt"

	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	types "github.com/colorfulnotion/jam/types"
)

func (n *Node) processTicket(ticket types.Ticket) error {
	// Store the ticket in the tip's queued tickets
	s := n.getState()
	sf := s.GetSafrole()
	// todo shallow check : check if the ticket is already in the pool
	id, entropy_idx, err := sf.ValidateIncomingTicket(&ticket)
	if err != nil {
		log.Error(log.Node, "processTicket:ValidateIncomingTicket", "err", err)
		return err
	}
	if n.extrinsic_pool.IsSeenTicket(ticket) {
		log.Warn(log.Node, "processTicket:IsSeenTicket", "ticket", ticket.TicketID)
		return nil // Already seen
	}
	used_entropy := s.GetSafrole().Entropy[entropy_idx]
	// TODO: add tracer event
	err = n.extrinsic_pool.AddTicketToPool(ticket, id, used_entropy)
	if err != nil {
		log.Error(log.Node, "processTicket:AddTicketToPool", "err", err)
	}
	return nil // Success
}

func (n *Node) processAssurance(assurance types.Assurance) error {
	// Store the assurance in the tip's queued assurances
	// Validate the assurance signature
	if len(assurance.Signature) == 0 {
		return fmt.Errorf("no assurance signature")
	}

	// Check the assurance validity
	if err := n.statedb.CheckIncomingAssurance(&assurance); err != nil {
		return err
	}

	// store it into extrinsic pool
	err := n.extrinsic_pool.AddAssuranceToPool(assurance)
	if err != nil {
		log.Error(log.A, "processAssurance:AddAssuranceToPool", "err", err)
	}

	log.Trace(log.A, "processAssurance", "n.id", n.id, "assurance.ValidatorIndex", assurance.ValidatorIndex)
	return nil // Success
}

func (n *Node) processGuarantee(g types.Guarantee, caller string) error {
	// Store the guarantee in the tip's queued guarantees
	s := n.getState()

	err := s.VerifyGuaranteeBasic(g, 0) // By passing in 0 we don't check against the statedb slot
	if err != nil {
		if !statedb.AcceptableGuaranteeError(err) {
			return err
		}
		log.Warn(log.G, "processGuarantee:VerifyGuaranteeBasic", "caller", caller, "err", err)
		return err
	}
	err = n.extrinsic_pool.AddGuaranteeToPool(g)
	if err != nil {
		log.Error(log.G, "processGuarantee:AddGuaranteeToPool", "caller", caller, "err", err)
	}
	return nil // Success
}
