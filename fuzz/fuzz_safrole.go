package fuzz

import (
	"math/rand"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func randomTicket(block *types.Block) (*types.Ticket, int) {
	idx := rand.Intn(len(block.Extrinsic.Tickets))
	return &(block.Extrinsic.Tickets[idx]), idx
}

func outsideBoundary(s *statedb.StateDB, slot uint32) bool {
	_, currPhase := s.JamState.SafroleState.EpochAndPhase(slot)
	if currPhase == 0 {
		return true
	}
	return currPhase >= types.TicketSubmissionEndSlot
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
func fuzzBlockTTicketAlreadyInState(block *types.Block, s *statedb.StateDB, validatorSecrets []types.ValidatorSecret) error {
	if outsideBoundary(s, block.Header.Slot) {
		return nil
	}
	t, t_idx := randomTicket(block)
	simpleFuzz := false
	if t == nil {
		// Can be fuzzed with anything...
		simpleFuzz = true
	}
	sf := s.JamState.SafroleState
	if len(sf.NextEpochTicketsAccumulator) == 0 {
		return nil
	}
	// take a random ticket t2 in the accumulator with a specific index idx2
	existingTicketBodyID := rand.Intn(len(sf.NextEpochTicketsAccumulator))
	if existingTicketBodyID == t_idx && !simpleFuzz {
		return nil
	}
	existingTicketBody := sf.NextEpochTicketsAccumulator[existingTicketBodyID]
	existingTicketID := existingTicketBody.Id

	var existingTicket *types.Ticket
	for _, secret := range validatorSecrets {
		auth_secret, _ := statedb.ConvertBanderSnatchSecret(secret.BandersnatchSecret)
		ticketCandidate, _, err := sf.SimulateTicket(auth_secret, sf.Entropy[2], existingTicketBody.Attempt)
		if err == nil {
			ticketCandidateID, tIDErr := ticketCandidate.TicketID()
			if tIDErr == nil {
				if ticketCandidateID == existingTicketID || simpleFuzz {
					existingTicket = &ticketCandidate
					continue
				}
			}
		}
	}
	if existingTicket == nil {
		return nil
	}
	if simpleFuzz {
		block.Extrinsic.Tickets = append(block.Extrinsic.Tickets, *existingTicket)
	} else {
		block.Extrinsic.Tickets[t_idx] = *existingTicket
	}
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
	altPhase := uint32(rand.Intn(int(types.EpochLength-types.TicketSubmissionEndSlot))) + uint32(types.TicketSubmissionEndSlot)
	currSlot := block.Header.Slot
	altSlot := currSlot - currPhase + altPhase

	// advance the slot of the block past the ticket contest period ... need to loop through entire authorset to find a properkey
	block.Header.Slot = altSlot
	return jamerrors.ErrTEpochLotteryOver
}

// Progress from slot X to slot X. Timeslot must be strictly monotonic.
func fuzzBlockTTimeslotNotMonotonic(block *types.Block, s *statedb.StateDB) error {
	_, currPhase := s.JamState.SafroleState.EpochAndPhase(block.Header.Slot)
	if currPhase == 0 {
		return nil
	}
	altPhase := uint32(rand.Intn(int(currPhase)))
	currSlot := block.Header.Slot
	altSlot := currSlot - currPhase + altPhase
	block.Header.Slot = altSlot
	return jamerrors.ErrTTimeslotNotMonotonic
}
