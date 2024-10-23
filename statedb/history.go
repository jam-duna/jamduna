package statedb

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// Recent History : see Section 7
func (s *StateDB) ApplyStateRecentHistory(serviceAccumulations []types.ServiceAccumulation, reported []common.Hash) (mmr *trie.MerkleMountainRange) {
	mmrPeaks := MMR{}
	preRecentBlocks := s.JamState.BeefyPool
	newBeta_state := Beta_state{
		HeaderHash: s.Block.Header.Hash(),
		MMR:        mmrPeaks,
		StateRoot:  s.StateRoot,
		Reported:   reported,
	}
	postRecentBlocks := append(preRecentBlocks, newBeta_state)
	// TODO: review Section 7 eq 83 closely
	mmr = trie.NewMMR()
	for _, accumulation := range serviceAccumulations {
		mmr.Append(accumulation.Result.Bytes())
	}
	if len(postRecentBlocks) > types.RecentHistorySize {
		postRecentBlocks = postRecentBlocks[1:types.RecentHistorySize]
	}
	s.JamState.BeefyPool = postRecentBlocks
	return
}
