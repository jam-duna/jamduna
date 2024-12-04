package main

import (
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
	"math/rand"
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

// assurances
// Sourabh
func fuzzBlockABadSignature(block *types.Block) error {
	a := randomAssurance(block)
	if a != nil {
		a.Signature = types.GenerateRandomEd25519Signature()
		return jamerrors.ErrABadSignature
	}
	return nil
}

// Sourabh
func fuzzBlockABadValidatorIndex(block *types.Block) error {
	a := randomAssurance(block)
	if a != nil {
		a.ValidatorIndex = types.TotalValidators + 100
		return jamerrors.ErrABadValidatorIndex
	}
	return nil
}

// Sourabh
func fuzzBlockABadCore(block *types.Block) error {
	a := randomAssurance(block)
	if a != nil {
		for x := 0; x < len(a.Bitfield); x++ {
			a.Bitfield[x] = 0xff
		}
		return jamerrors.ErrABadCore
	}
	return nil
}

// Sourabh
func fuzzBlockABadParentHash(block *types.Block) error {
	a := randomAssurance(block)
	a.Anchor = a.Hash()
	return jamerrors.ErrABadParentHash
}

// Sean
func fuzzBlockAStaleReport(block *types.Block) error {
	// TODO: Implement fuzzing logic for AStaleReport
	return nil
}
