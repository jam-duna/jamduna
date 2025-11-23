package statedb

import (
	bls "github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func (s *StateDB) Finalize(v types.ValidatorSecret) (blsSignature bls.Signature, finalizedEpoch uint32, beefy_hash common.Hash, err error) {
	finalizedEpoch = 0
	s.Finalized = true
	if s.Block.Header.EpochMark != nil {
		lastB := s.JamState.RecentBlocks.B_B
		bEncoding, err0 := types.Encode(lastB)
		if err0 != nil {
			return
		}
		beefy_hash = common.Keccak256(bEncoding)
		beefyBytes := append([]byte(types.X_B), beefy_hash.Bytes()...)
		var blsSecretKey bls.SecretKey
		copy(blsSecretKey[:], v.BlsSecret[:])
		blsSignature, err = blsSecretKey.Sign(beefyBytes)
		if err != nil {
			return
		}
		epoch := s.GetSafrole().GetEpoch()
		return blsSignature, epoch, beefy_hash, nil
	}
	return
}
