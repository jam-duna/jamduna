package main

import (
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"math/rand"
)

/*
type Ticket struct {
  Attempt   uint8                     `json:"attempt"`
  Signature BandersnatchRingSignature `json:"signature"`
}
*/
// safrole fuzzers
func randomTicket(block *types.Block) *types.Ticket {
	return &(block.Extrinsic.Tickets[rand.Intn(len(block.Extrinsic.Tickets))])
}

// Sourabh
func fuzzBlockTBadTicketAttemptNumber(block *types.Block) error {
	// TODO: Implement fuzzing logic for TBadTicketAttemptNumber
	t := randomTicket(block)
	if t != nil {
		t.Attempt = types.TicketEntriesPerValidator + uint8(rand.Intn(100))
		return jamerrors.ErrTBadTicketAttemptNumber
	}
	return nil
}

// Sean
func fuzzBlockTTicketAlreadyInState(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for TTicketAlreadyInState - find a ticket in the state and add that to the block
	return jamerrors.ErrTTicketAlreadyInState
}

// Sourabh
func fuzzBlockTTicketsBadOrder(block *types.Block) error {
	if len(block.Extrinsic.Tickets) < 2 {
		return nil
	}
	// Reverse the order of tickets
	for i, j := 0, len(block.Extrinsic.Tickets)-1; i < j; i, j = i+1, j-1 {
		block.Extrinsic.Tickets[i], block.Extrinsic.Tickets[j] = block.Extrinsic.Tickets[j], block.Extrinsic.Tickets[i]
	}
	return nil
}

// Sourabh
func fuzzBlockTBadRingProof(block *types.Block) error {
	t := randomTicket(block)
	if t != nil {
		// TOD
		b := rand.Intn(len(t.Signature))
		t.Signature[b] = t.Signature[b] ^ 0xFF
		return jamerrors.ErrTBadRingProof
	}
	return nil
}

// Sean
func fuzzBlockTEpochLotteryOver(block *types.Block) error {
	// TODO: Implement fuzzing logic for TEpochLotteryOver -- advance the slot of the block past the ticket contest period
	return nil
}

// Sean
func fuzzBlockTTimeslotNotMonotonic(block *types.Block) error {
	// TODO: Implement fuzzing logic for TTimeslotNotMonotonic -- take the block timeslot and make it the one before
	return nil
}
