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
	for coreIdx, rhoState := range jamstate.AvailabilityAssignments {
		if rhoState == nil {
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

	for coreIdx, rhoState := range jamState.AvailabilityAssignments {
		if rhoState != nil {
			anyAvailability = true
			if (rhoState.Timeslot + types.UnavailableWorkReplacementPeriod) <= blkTimeSlot {
				anyStaleAvailability = true
				stalePendingCores[uint16(coreIdx)] = rhoState.Timeslot
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
