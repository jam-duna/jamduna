package statedb

import (
	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func (s *StateDB) Finalize(v types.ValidatorSecret) (blsSignature bls.Signature, finalizedEpoch bool, err error) {
	finalizedEpoch = false
	s.Finalized = true
	if s.Block.Header.EpochMark != nil {
		lastB := s.JamState.RecentBlocks.B_B
		bEncoding, err0 := types.Encode(lastB)
		if err0 != nil {
			return
		}
		beefyBytes := append([]byte(types.X_B), common.Keccak256(bEncoding).Bytes()...)
		var blsSecretKey bls.SecretKey
		copy(blsSecretKey[:], v.BlsSecret[:])
		blsSignature, err = blsSecretKey.Sign(beefyBytes)
		if err != nil {
			return
		}
		return blsSignature, true, nil
	}
	return
}
