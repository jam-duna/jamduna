package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	//"reflect"
)

// Input struct definition
type HInput struct {
	HeaderHash      common.Hash            `json:"header_hash"`
	ParentStateRoot common.Hash            `json:"parent_state_root"`
	AccumulateRoot  common.Hash            `json:"accumulate_root"`
	Reported        []w3fSegmentRootLookup `json:"work_packages"`
}

type w3fSegmentRootLookup struct {
	WorkPackageHash common.Hash `json:"hash"`
	SegmentRoot     common.Hash `json:"exports_root"`
}

type HState struct {
	RecentBlocks RecentBlocks `json:"beta"`
}

type HistoryData struct {
	Input     HInput   `json:"input"`
	PreState  HState   `json:"pre_state"`
	Output    *HOutput `json:"output"`
	PostState HState   `json:"post_state"`
}

type HOutput struct{}

func HistorySTF(input HInput, s HState) (poststate HState) {
	// Eq 83 n
	// Eq 83 n.p -- aggregate all the workpackagehashes of the guarantees
	preRecentBlocks := s.RecentBlocks
	if len(preRecentBlocks) > 0 {
		preRecentBlocks[len(preRecentBlocks)-1].StateRoot = input.ParentStateRoot
	}

	// Eq 83 n.b
	mmr := trie.NewMMR()
	if len(preRecentBlocks) > 0 {
		mmr.Peaks = preRecentBlocks[len(preRecentBlocks)-1].B.Peaks
	}
	mmr.Append(&(input.AccumulateRoot))

	reported := types.SegmentRootLookupHistory{}
	for _, g := range input.Reported {
		reported = append(reported, types.SegmentRootLookupItemHistory{
			WorkPackageHash: g.WorkPackageHash,
			SegmentRoot:     g.SegmentRoot,
		})
	}

	n := Beta_state{
		Reported:   reported,         // p
		HeaderHash: input.HeaderHash, // h
		B:          *mmr,             // b
		StateRoot:  common.Hash{},    // this is the POSTERIOR stateroot
	}

	// Eq 84 β' ≡ β† ++ n (last H=types.RecentHistorySize)
	postRecentBlocks := append(preRecentBlocks, n)
	if len(postRecentBlocks) > types.RecentHistorySize {
		postRecentBlocks = postRecentBlocks[1 : types.RecentHistorySize+1]
	}
	poststate.RecentBlocks = postRecentBlocks
	return
}

func TestRecentHistory(t *testing.T) {
	testCases := []string{
		"progress_blocks_history-1.json",
		"progress_blocks_history-2.json",
		"progress_blocks_history-3.json",
		"progress_blocks_history-4.json",
	}
	for testnum, jsonFile := range testCases {
		t.Run(jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/history/data", jsonFile)
			jsonData, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}
			var h HistoryData
			err = json.Unmarshal(jsonData, &h)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			expectedh := h.PostState
			posth := HistorySTF(h.Input, h.PreState)
			if len(expectedh.RecentBlocks) != len(posth.RecentBlocks) {
				fmt.Printf("Test %s FAIL\n", jsonFile)
			}
			for i := 0; i < len(posth.RecentBlocks); i++ {
				e := expectedh.RecentBlocks[i].String()
				p := posth.RecentBlocks[i].String()
				if e == p {
					fmt.Printf("Test %d %s PASS\n", i, jsonFile)
				} else {
					fmt.Printf("Test %d: element %d/%d expected %s got: %s\n", testnum, i, len(posth.RecentBlocks), e, p)
				}

			}

		})
	}
}
