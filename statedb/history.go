package statedb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// C3
type RecentBlocks struct {
	B_H []Beta_state `json:"history"` // History of Beta states, each with header hash, B, state root, and reported segment roots
	B_B trie.MMR     `json:"mmr"`     // MMR of the B values from Beta states
}

// TODO: [Sourabh] Recent Blocks: AccOuts in state, store historical roots only
// Store only MMB root hash in Recent History, as only this is what is needed for verification
// Separately, store the MMB peak set and the accumulation outputs of the most recent block.
//
//	https://github.com/gavofyork/graypaper/commit/c48ad4498ba9df4abbad470aa9aac553ac33b864
type Beta_state struct {
	HeaderHash common.Hash                    `json:"header_hash"`
	B          common.Hash                    `json:"beefy_root"`
	StateRoot  common.Hash                    `json:"state_root"`
	Reported   types.SegmentRootLookupHistory `json:"reported"` // Use the custom type
}

func (b *Beta_state) String() string {
	enc, err := json.Marshal(b)
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}

func (b *Beta_state) UnmarshalJSON(data []byte) error {
	var s struct {
		HeaderHash common.Hash                    `json:"header_hash"`
		B          common.Hash                    `json:"beefy_root"`
		StateRoot  common.Hash                    `json:"state_root"`
		Report     types.SegmentRootLookupHistory `json:"reported"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	b.HeaderHash = s.HeaderHash
	b.B = s.B
	b.StateRoot = s.StateRoot
	b.Reported = s.Report
	return nil
}

func (b Beta_state) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		HeaderHash common.Hash                    `json:"header_hash"`
		B          common.Hash                    `json:"beefy_root"`
		StateRoot  common.Hash                    `json:"state_root"`
		Report     types.SegmentRootLookupHistory `json:"reported"`
	}{
		HeaderHash: b.HeaderHash,
		B:          b.B,
		StateRoot:  b.StateRoot,
		Report:     b.Reported,
	})
}

// Recent History : see Section 7
func (s *StateDB) ApplyStateRecentHistory(blk *types.Block, accumulationRoot *common.Hash, accumulationOutputs []types.AccumulationOutput) {
	// Eq 83 n
	// Eq 83 n.p -- aggregate all the workpackagehashes of the guarantees
	reported := []types.SegmentRootLookupItemHistory{}
	for _, g := range blk.Guarantees() {
		temReported := types.SegmentRootLookupItemHistory{
			WorkPackageHash: g.Report.AvailabilitySpec.WorkPackageHash,
			SegmentRoot:     g.Report.AvailabilitySpec.ExportedSegmentRoot,
		}
		reported = append(reported, temReported)
	}
	// sort the reported by workpackagehash

	sort.Slice(reported, func(i, j int) bool {
		return bytes.Compare(reported[i].WorkPackageHash.Bytes(), reported[j].WorkPackageHash.Bytes()) < 0
	})

	preRecentBlocks := s.JamState.RecentBlocks.B_H
	// Eq 83 n.b
	mmr := trie.NewMMR()
	if len(preRecentBlocks) > 0 {
		mmr.Peaks = s.JamState.RecentBlocks.B_B.Peaks
	}
	mmr.Append(accumulationRoot)
	s.JamState.AccumulationOutputs = accumulationOutputs
	n := Beta_state{
		Reported:   reported,          // p
		HeaderHash: blk.Header.Hash(), // h
		B:          *mmr.SuperPeak(),  // b
		StateRoot:  common.Hash{},     // this will become the POSTERIOR stateroot in the NEXT update, updated via **** above
	}

	// Eq 84 β' ≡ β† ++ n (last H=types.RecentHistorySize)
	postRecentBlocks := append(preRecentBlocks, n)
	if len(postRecentBlocks) > types.RecentHistorySize {
		postRecentBlocks = postRecentBlocks[1 : types.RecentHistorySize+1]
	}
	s.JamState.RecentBlocks.B_H = postRecentBlocks
	s.JamState.RecentBlocks.B_B = *mmr

}

func (s *StateDB) ApplyStateRecentHistoryDagga(previousStateRoot common.Hash) {
	preRecentBlocks := s.JamState.RecentBlocks.B_H
	if len(preRecentBlocks) > 0 {
		preRecentBlocks[len(preRecentBlocks)-1].StateRoot = previousStateRoot
	}
	s.JamState.RecentBlocks.B_H = preRecentBlocks
}
