package statedb

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// C2
type RecentBlocks []Beta_state

type Beta_state struct {
	HeaderHash common.Hash                 `json:"header_hash"`
	B          trie.MMR                    `json:"mmr"`
	StateRoot  common.Hash                 `json:"state_root"`
	Reported   map[common.Hash]common.Hash `json:"reported"`
}

// Recent History : see Section 7
func (s *StateDB) ApplyStateRecentHistory(blk *types.Block, accumulationRoot *common.Hash) {
	// Eq 83 n
	// Eq 83 n.p -- aggregate all the workpackagehashes of the guarantees
	reported := map[common.Hash]common.Hash{}
	for _, g := range blk.Guarantees() {
		reported[g.Report.AvailabilitySpec.WorkPackageHash] = g.Report.AvailabilitySpec.ExportedSegmentRoot
	}

	preRecentBlocks := s.JamState.RecentBlocks
	if len(preRecentBlocks) > 0 {
		preRecentBlocks[len(preRecentBlocks)-1].StateRoot = s.StateRoot // ****
	}

	// Eq 83 n.b
	mmr := trie.NewMMR()
	if len(preRecentBlocks) > 0 {
		mmr.Peaks = preRecentBlocks[len(preRecentBlocks)-1].B.Peaks
	}
	mmr.Append(accumulationRoot)
	n := Beta_state{
		Reported:   reported,          // p
		HeaderHash: blk.Header.Hash(), // h
		B:          *mmr,              // b
		StateRoot:  common.Hash{},     // this will become the POSTERIOR stateroot in the NEXT update, updated via **** above
	}

	// Eq 84 β' ≡ β† ++ n (last H=types.RecentHistorySize)
	postRecentBlocks := append(preRecentBlocks, n)
	if len(postRecentBlocks) > types.RecentHistorySize {
		postRecentBlocks = postRecentBlocks[1 : types.RecentHistorySize+1]
	}
	s.JamState.RecentBlocks = postRecentBlocks
	return
}
