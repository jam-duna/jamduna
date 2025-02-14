package node

import (
	"context"
	"fmt"
	"time"

	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) processTicket(ticket types.Ticket) error {
	// Store the ticket in the tip's queued tickets
	s := n.getState()
	sf := s.GetSafrole()
	start := time.Now()
	ticketID, entropy_idx, err := sf.ValidateIncomingTicket(&ticket)
	if err != nil {
		if debug {
			fmt.Printf("ProcessIncomingTicket Error Invalid Ticket. Err=%v\n", err)
		}
		return err
	}
	elapsed := time.Since(start).Microseconds()
	if debugtrace && elapsed > 500000 {
		fmt.Printf("[N%v] ProcessIncomingTicket -- Adding ticketID=%v [%d ms]\n", s.Id, ticketID, time.Since(start).Microseconds()/1000)
	}
	used_entropy := s.GetSafrole().Entropy[entropy_idx]
	// TODO: add tracer event
	err = n.extrinsic_pool.AddTicketToPool(ticket, used_entropy)
	if err != nil {
		fmt.Printf("processTicket: AddTicketToPool ERR %v\n", err)
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
		fmt.Printf("processAssurance: AddAssuranceToPool ERR %v\n", err)
	}

	log := fmt.Sprintf("[N%v] ProcessIncomingAssurance -- start Adding assurance form v%d\n", n.id, assurance.ValidatorIndex)
	Logger.RecordLogs(storage.Assurance_status, log, true)
	return nil // Success
}

func (n *Node) processGuarantee(g types.Guarantee) error {
	// Store the guarantee in the tip's queued guarantees
	s := n.getState()
	err := s.ValidateSingleGuarantee(g)
	if err != nil {
		Logger.RecordLogs(storage.EG_status, fmt.Sprintf("ValidateSingleGuarantee ERR %v", err), true)
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
		fmt.Printf("processGuarantee: AddGuaranteeToPool ERR %v\n", err)
	} else {
		// fmt.Printf("%s received guarantee from core_%d\n", n.String(), g.Report.CoreIndex)
	}
	return nil // Success
}

func (n *Node) processPreimage(l types.Preimages) {
	_, err := n.statedb.ValidateLookup(&l)
	if err != nil {
		return
	}
	log := fmt.Sprintf("[N%v] ProcessIncomingLookup -- start Adding lookup: %v\n", n.id, l.String())
	Logger.RecordLogs(storage.Preimage_status, log, true)
	n.extrinsic_pool.AddPreimageToPool(l)
}
