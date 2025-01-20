package node

import (
	"fmt"
	"time"

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
	if trace && elapsed > 500000 {
		fmt.Printf("[N%v] ProcessIncomingTicket -- Adding ticketID=%v [%d ms]\n", s.Id, ticketID, time.Since(start).Microseconds()/1000)
	}
	used_entropy := s.GetSafrole().Entropy[entropy_idx]
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
	err := n.extrinsic_pool.AddAssuranceToPool(assurance)
	if err != nil {
		fmt.Printf("processAssurance: AddAssuranceToPool ERR %v\n", err)
	}
	return nil // Success
}

func (n *Node) processGuarantee(g types.Guarantee) error {
	// Store the guarantee in the tip's queued guarantees
	s := n.getState()
	CurrV := s.GetSafrole().CurrValidators
	err := g.Verify(CurrV)
	if err != nil {
		fmt.Printf("processGuarantee: Invalid guarantee %s. Err=%v\n", g.String(), err)
		return err
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

	// fmt.Printf("[N%v] ProcessIncomingLookup -- start Adding lookup: %v\n", s.Id, l.String())
	n.extrinsic_pool.AddPreimageToPool(l)
}
