package statedb

import (
	"encoding/json"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// C3
type RecentBlocks []Beta_state

type Beta_state struct {
	HeaderHash common.Hash             `json:"header_hash"`
	B          trie.MMR                `json:"mmr"`
	StateRoot  common.Hash             `json:"state_root"`
	Reported   types.SegmentRootLookup `json:"report"` // Use the custom type
}

func (b *Beta_state) String() string {
	enc, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}

func (b *Beta_state) UnmarshalJSON(data []byte) error {
	var s struct {
		HeaderHash common.Hash             `json:"header_hash"`
		B          trie.MMR                `json:"mmr"`
		StateRoot  common.Hash             `json:"state_root"`
		Report     types.SegmentRootLookup `json:"reported"`
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
		HeaderHash common.Hash             `json:"header_hash"`
		B          trie.MMR                `json:"mmr"`
		StateRoot  common.Hash             `json:"state_root"`
		Report     types.SegmentRootLookup `json:"reported"`
	}{
		HeaderHash: b.HeaderHash,
		B:          b.B,
		StateRoot:  b.StateRoot,
		Report:     b.Reported,
	})
}

func dump_recent_blocks(prefix string, arr RecentBlocks) {
	for i, b := range arr {
		fmt.Printf(" %s[%d]=%s\n", prefix, i, b.String())
	}
}

// Recent History : see Section 7
func (s *StateDB) ApplyStateRecentHistory(blk *types.Block, accumulationRoot *common.Hash) {
	// Eq 83 n
	// Eq 83 n.p -- aggregate all the workpackagehashes of the guarantees
	reported := []types.SegmentRootLookupItem{}
	for _, g := range blk.Guarantees() {
		temReported := types.SegmentRootLookupItem{
			WorkPackageHash: g.Report.AvailabilitySpec.WorkPackageHash,
			SegmentRoot:     g.Report.AvailabilitySpec.ExportedSegmentRoot,
		}
		reported = append(reported, temReported)
	}
	preRecentBlocks := s.JamState.RecentBlocks
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

}

func (s *StateDB) ApplyStateRecentHistoryDagga(previousStateRoot common.Hash) {
	preRecentBlocks := s.JamState.RecentBlocks
	if len(preRecentBlocks) > 0 {
		preRecentBlocks[len(preRecentBlocks)-1].StateRoot = previousStateRoot
	}
	s.JamState.RecentBlocks = preRecentBlocks
}
