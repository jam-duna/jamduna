package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) processTicket(ticket types.Ticket) error {
	// Store the ticket in the tip's queued tickets
	s := n.getState()
	sf := s.GetSafrole()
	id, entropy_idx, err := sf.ValidateIncomingTicket(&ticket)
	if err != nil {
		log.Error(module, "processTicket:ValidateIncomingTicket", "err", err)
		return err
	}

	used_entropy := s.GetSafrole().Entropy[entropy_idx]
	// TODO: add tracer event
	err = n.extrinsic_pool.AddTicketToPool(ticket, id, used_entropy)
	if err != nil {
		log.Error(module, "processTicket:AddTicketToPool", "err", err)
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
	// TODO: add tracer event
	if n.store.SendTrace {
		tracer := n.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] AddAssuranceToPool", n.store.NodeID))
		// n.UpdateAssuranceContext(ctx)
		defer span.End()
	}

	err := n.extrinsic_pool.AddAssuranceToPool(assurance)
	if err != nil {
		log.Error(debugA, "processAssurance:AddAssuranceToPool", "err", err)
	}

	log.Trace(debugA, "processAssurance", "n.id", n.id, "assurance.ValidatorIndex", assurance.ValidatorIndex)
	return nil // Success
}

func (n *Node) processGuarantee(g types.Guarantee) error {
	// Store the guarantee in the tip's queued guarantees
	s := n.getState()
	err := s.ValidateSingleGuarantee(g)
	if err != nil {
		log.Warn(debugG, "processGuarantee:ValidateSingleGuarantee", "err", err)
		return err
	}
	// TODO: add tracer event
	if n.store.SendTrace {
		tracer := n.store.Tp.Tracer("NodeTracer")
		_, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] AddGuaranteeToPool", n.store.NodeID))
		// n.UpdateGuaranteeContext(ctx)
		defer span.End()
	}

	err = n.extrinsic_pool.AddGuaranteeToPool(g)
	if err != nil {
		log.Error(debugG, "processGuarantee:AddGuaranteeToPool", "err", err)
	}
	return nil // Success
}

func (n *Node) processPreimage(l types.Preimages) {
	_, err := n.statedb.ValidateLookup(&l)
	if err != nil {
		log.Warn(debugP, "processPreimage:ValidateLookup", "err", err)
		return
	}
	log.Trace(debugP, "processPreimage", "n", n.id, "l", l.String())
	n.extrinsic_pool.AddPreimageToPool(l)
}
