package main

import (
	"math/rand"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

/*
// Assurance represents an assurance value.
type Assurance struct {
	// H_p - see Eq 124
	Anchor common.Hash `json:"anchor"`
	// f - 1 means "available"
	Bitfield       [Avail_bitfield_bytes]byte `json:"bitfield"`
	ValidatorIndex uint16                     `json:"validator_index"`
	Signature      Ed25519Signature           `json:"signature"`
}
*/

func randomAssurance(block *types.Block) *types.Assurance {
	return &(block.Extrinsic.Assurances[rand.Intn(len(block.Extrinsic.Assurances))])
}

// One assurance has a bad signature.
func fuzzBlockABadSignature(block *types.Block) error {
	a := randomAssurance(block)
	if a == nil {
		return nil
	}
	a.Signature = types.GenerateRandomEd25519Signature()
	return jamerrors.ErrABadSignature
}

// One assurance has a bad validator index.
func fuzzBlockABadValidatorIndex(block *types.Block) error {
	a := randomAssurance(block)
	if a == nil {
		return nil
	}
	a.ValidatorIndex = types.TotalValidators + 100
	return jamerrors.ErrABadValidatorIndex
}

// One assurance targets a core without any assigned work report.
func fuzzBlockABadCore(block *types.Block) error {
	a := randomAssurance(block)
	if a == nil {
		return nil
	}
	for x := 0; x < len(a.Bitfield); x++ {
		a.Bitfield[x] = 0xff
	}
	return jamerrors.ErrABadCore
}

// One assurance has a bad attestation parent hash.
func fuzzBlockABadParentHash(block *types.Block) error {
	a := randomAssurance(block)
	if a == nil {
		return nil
	}
	a.Anchor = a.Hash()
	return jamerrors.ErrABadParentHash
}

func fuzzBlockAStaleReport(block *types.Block, s *statedb.StateDB) error {
	// Look into AvailabilityAssignments. Filter on Stale. And manually set the bitfield to true.
	jamState := s.GetJamState()
	blkTimeSlot := block.Header.Slot
	stalePendingCores := map[uint16]uint32{}

	anyAvailability := false
	anyStaleAvailability := false

	for coreIdx, rhoState := range jamState.AvailabilityAssignments {
		if rhoState != nil {
			anyAvailability = true
			if (rhoState.Timeslot + types.UnavailableWorkReplacementPeriod) <= blkTimeSlot {
				// can be fuzzed to stale
				anyStaleAvailability = true
				stalePendingCores[uint16(coreIdx)] = rhoState.Timeslot
			}
		}
	}

	if !anyAvailability {
		// No available work reports to assure about
		return nil
	}

	if !anyStaleAvailability {
		// No available work reports that's already stale
		return nil
	}

	a := randomAssurance(block)
	if a == nil {
		return nil
	}

	for staleCoreIdx, _ := range stalePendingCores {
		// mark the assurance to target the stale core to fuzz the error
		a.SetBitFieldBit(uint16(staleCoreIdx), true)
	}
	// TODO: probably need to re-sign EA with new signature
	return jamerrors.ErrAStaleReport
}
