package main

import (
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"math/rand"
)

/*
TODO: Sourabh

	fuzzBlockTTicketAlreadyInState
	fuzzBlockTEpochLotteryOver
	fuzzBlockTTimeslotNotMonotonic

	type Ticket struct {
	  Attempt   uint8                     `json:"attempt"`
	  Signature BandersnatchRingSignature `json:"signature"`
	}
*/
func randomTicket(block *types.Block) (*types.Ticket, int) {
	idx := rand.Intn(len(block.Extrinsic.Tickets))
	return &(block.Extrinsic.Tickets[idx]), idx
}

// Submit an extrinsic with a bad ticket attempt number.
func fuzzBlockTBadTicketAttemptNumber(block *types.Block) error {

	t, _ := randomTicket(block)
	if t == nil {
		return nil
	}
	t.Attempt = types.TicketEntriesPerValidator + uint8(rand.Intn(100))
	return jamerrors.ErrTBadTicketAttemptNumber
}

// Submit one ticket already recorded in the state.
func fuzzBlockTTicketAlreadyInState(block *types.Block, s *statedb.StateDB) error {
	t, idx := randomTicket(block)
	if t == nil {
		return nil
	}
	sf := s.JamState.SafroleState
	if len(sf.NextEpochTicketsAccumulator) == 0 {
		return nil
	}
	// take a random ticket t2 in the accumulator with a specific index idx2
	idx2 := rand.Intn(len(sf.NextEpochTicketsAccumulator))
	if idx2 == idx {
		return nil
	}
	// TODO: Sourabh fix TicketBody <> Ticket mismatch
	// block.Extrinsic.Tickets[idx] = sf.NextEpochTicketsAccumulator[idx2]
	return jamerrors.ErrTTicketAlreadyInState
}

// Submit tickets in bad order.
func fuzzBlockTTicketsBadOrder(block *types.Block) error {
	if len(block.Extrinsic.Tickets) < 2 {
		return nil
	}
	// Reverse the order of tickets
	i := rand.Intn(len(block.Extrinsic.Tickets))
	j := (i + 1) % len(block.Extrinsic.Tickets)
	block.Extrinsic.Tickets[i], block.Extrinsic.Tickets[j] = block.Extrinsic.Tickets[j], block.Extrinsic.Tickets[i]
	return jamerrors.ErrTTicketsBadOrder
}

// Submit tickets with bad ring proof.
func fuzzBlockTBadRingProof(block *types.Block) error {
	t, _ := randomTicket(block)
	if t == nil {
		return nil
	}
	b := rand.Intn(len(t.Signature))
	t.Signature[b] = t.Signature[b] ^ 0xFF
	return jamerrors.ErrTBadRingProof
}

// Submit tickets when epoch's lottery is over.
func fuzzBlockTEpochLotteryOver(block *types.Block, s *statedb.StateDB) error {
	t, _ := randomTicket(block)
	if t == nil {
		return nil
	}
	_, currPhase := s.JamState.SafroleState.EpochAndPhase(block.Header.Slot)
	altPhase := uint32(types.TicketSubmissionEndSlot)

	// advance the slot of the block past the ticket contest period
	block.Header.Slot += (altPhase - currPhase) * types.SecondsPerEpoch
	return jamerrors.ErrTEpochLotteryOver
}

// Progress from slot X to slot X. Timeslot must be strictly monotonic.
func fuzzBlockTTimeslotNotMonotonic(block *types.Block) error {
	block.Header.Slot -= uint32(types.SecondsPerEpoch) * uint32(1+rand.Intn(10))
	return nil
}
