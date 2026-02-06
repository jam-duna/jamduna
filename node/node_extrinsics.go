package node

import (
	"fmt"

	log "github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/statedb"
	types "github.com/jam-duna/jamduna/types"
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

func (n *Node) processAssurance(assuranceObj AssuranceObject) error {
	if _, ok := n.statedbMap[assuranceObj.Anchor]; !ok {
		return fmt.Errorf("processAssurance: unknown anchor %s", assuranceObj.Anchor.Hex())
	}
	var assurance types.Assurance
	assurance.Anchor = assuranceObj.Anchor
	assurance.Bitfield = assuranceObj.Bitfield
	assurance.Signature = assuranceObj.Signature
	var err error
	assurance.ValidatorIndex, err = n.findValidatorIndexByAnchor(assurance.Anchor, assuranceObj.Ed25519Key)
	if err != nil {
		return fmt.Errorf("processAssurance: findValidatorIndexByAnchor: %w", err)
	}
	if len(assurance.Signature) == 0 {
		return fmt.Errorf("no assurance signature")
	}

	n.statedbMapMutex.Lock()
	anchorStateDB, hasAnchorState := n.statedbMap[assurance.Anchor]
	n.statedbMapMutex.Unlock()

	var verifyStateDB *statedb.StateDB
	if hasAnchorState {
		verifyStateDB = anchorStateDB
	} else {
		// not sure if we should ever get here...
		verifyStateDB = n.statedb
	}

	// Check the assurance validity
	if err := verifyStateDB.CheckIncomingAssurance(&assurance); err != nil {
		return err
	}

	// store it into extrinsic pool
	err = n.extrinsic_pool.AddAssuranceToPool(assurance)
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
			log.Error(log.G, "processGuarantee:VerifyGuaranteeBasic", "caller", caller, "err", err)
			return err
		} else {
			log.Trace(log.G, "[IGNORE]processGuarantee:VerifyGuaranteeBasic:acceptable error", "caller", caller, "err", err)
		}
		return err
	}
	err = n.extrinsic_pool.AddGuaranteeToPool(g)
	if err != nil {
		log.Error(log.G, "processGuarantee:AddGuaranteeToPool", "caller", caller, "err", err)
	}
	return nil // Success
}
