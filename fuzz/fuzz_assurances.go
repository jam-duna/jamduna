package fuzz

import (
	"math/rand"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func randomAssurance(r *rand.Rand, block *types.Block) *types.Assurance {
	if len(block.Extrinsic.Assurances) == 0 {
		return nil
	}
	return &(block.Extrinsic.Assurances[r.Intn(len(block.Extrinsic.Assurances))])
}

// One assurance has a bad signature.
func fuzzBlockABadSignature(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	a := randomAssurance(r, block)
	if a == nil {
		return nil
	}
	a.Signature = types.GenerateRandomEd25519Signature()
	return jamerrors.ErrABadSignature
}

// One assurance has a bad validator index.
func fuzzBlockABadValidatorIndex(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	a := randomAssurance(r, block)
	if a == nil {
		return nil
	}
	a.ValidatorIndex = types.TotalValidators + 100
	return jamerrors.ErrABadValidatorIndex
}

// One assurance targets a core without any assigned work report.
func fuzzBlockABadCore(seed []byte, block *types.Block, s *statedb.StateDB, secrets []types.ValidatorSecret) error {
	r := NewSeededRand(seed)
	jamstate := s.GetJamState()
	for coreIdx, availability_assignment := range jamstate.AvailabilityAssignments {
		if availability_assignment == nil {
			a := randomAssurance(r, block)
			if a == nil {
				return nil
			}
			a.SetBitFieldBit(uint16(coreIdx), true)
			a.Sign(secrets[a.ValidatorIndex].Ed25519Secret[:])
			return jamerrors.ErrABadCore
		}
	}
	return nil
}

// One assurance has a bad attestation parent hash.
func fuzzBlockABadParentHash(seed []byte, block *types.Block, secrets []types.ValidatorSecret) error {
	r := NewSeededRand(seed)
	a := randomAssurance(r, block)
	if a == nil {
		return nil
	}
	a.Anchor = a.Hash()
	a.Sign(secrets[a.ValidatorIndex].Ed25519Secret[:])
	return jamerrors.ErrABadParentHash
}

func fuzzBlockAStaleReport(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	jamState := s.GetJamState()
	blkTimeSlot := block.Header.Slot
	stalePendingCores := map[uint16]uint32{}

	anyAvailability := false
	anyStaleAvailability := false

	for coreIdx, availability_assignment := range jamState.AvailabilityAssignments {
		if availability_assignment != nil {
			anyAvailability = true
			if (availability_assignment.Timeslot + types.UnavailableWorkReplacementPeriod) <= blkTimeSlot {
				anyStaleAvailability = true
				stalePendingCores[uint16(coreIdx)] = availability_assignment.Timeslot
			}
		}
	}

	if !anyAvailability || !anyStaleAvailability {
		return nil
	}

	a := randomAssurance(r, block)
	if a == nil {
		return nil
	}

	for staleCoreIdx := range stalePendingCores {
		a.SetBitFieldBit(uint16(staleCoreIdx), true)
	}
	// TODO: probably need to re-sign EA with new signature
	return jamerrors.ErrAStaleReport
}

func fuzzBlockADuplicateAssurer(seed []byte, block *types.Block, s *statedb.StateDB, secrets []types.ValidatorSecret) error {
	r := NewSeededRand(seed)
	a := randomAssurance(r, block)
	if a == nil {
		return nil
	}
	if len(block.Extrinsic.Assurances) < 2 {
		return nil
	}

	a0 := block.Extrinsic.Assurances[0]
	a1 := block.Extrinsic.Assurances[1]

	// Duplicate the assurer
	a0.ValidatorIndex = a1.ValidatorIndex
	a0.Sign(secrets[a0.ValidatorIndex].Ed25519Secret[:])
	block.Extrinsic.Assurances[0] = a0
	return jamerrors.ErrADuplicateAssurer
}

func fuzzBlockANotSortedAssurers(seed []byte, block *types.Block, s *statedb.StateDB, secrets []types.ValidatorSecret) error {
	r := NewSeededRand(seed)
	a := randomAssurance(r, block)
	if a == nil {
		return nil
	}

	if len(block.Extrinsic.Assurances) < 2 {
		return nil // Not enough assurances to sort
	}
	// destroy the sorted order
	for i := 0; i < len(block.Extrinsic.Assurances)-1; i++ {
		if r.Intn(2) == 0 {
			block.Extrinsic.Assurances[i], block.Extrinsic.Assurances[i+1] = block.Extrinsic.Assurances[i+1], block.Extrinsic.Assurances[i]
		}
	}
	return jamerrors.ErrANotSortedAssurers
}
