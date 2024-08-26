package statedb

import (
	"encoding/json"
	//"errors"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/stretchr/testify/assert"
)

// Input struct definition
type HInput struct {
	HeaderHash      common.Hash `json:"header_hash"`
	ParentStateRoot common.Hash `json:"parent_state_root"`
	// This is "r" in Eq (82), which is derived from (162)
	AccumulateRoot common.Hash `json:"accumulate_root"`

	WorkPackages []common.Hash `json:"work_packages"`
}

// MMR struct definition
type MMR struct {
	Peaks []common.Hash `json:"peaks"`
}

// Beta struct definition

type HBeta struct {
	HeaderHash common.Hash   `json:"header_hash"`
	MMR        MMR           `json:"mmr"`
	StateRoot  common.Hash   `json:"state_root"`
	Reported   []common.Hash `json:"reported"`
}

// PreState and PostState struct definitions
type State struct {
	Beta []HBeta `json:"beta"`
}

// HistoryData struct definition
type HistoryData struct {
	Input     HInput `json:"input"`
	PreState  State  `json:"pre_state"`
	PostState State  `json:"post_state"`
}

func TestRecentHistory(t *testing.T) {
	files := []string{
		"../jamtestvectors/history/data/progress_blocks_history-1.json",
		"../jamtestvectors/history/data/progress_blocks_history-2.json",
		"../jamtestvectors/history/data/progress_blocks_history-3.json",
		"../jamtestvectors/history/data/progress_blocks_history-4.json",
	}

	sdb, err := storage.NewStateDBStorage("/tmp/rh")
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := ioutil.ReadFile(file)
			if err != nil {
				t.Fatalf("Failed to read file %s: %v", file, err)
			}

			var history HistoryData
			if err := json.Unmarshal(data, &history); err != nil {
				t.Fatalf("Failed to unmarshal JSON data: %v", err)
			}

			// Assume RecentHistorySTF is a function that takes input and pre_state, and returns post_state
			resultPostState, err := RecentHistorySTF(sdb, history.Input, history.PreState)
			if err != nil {
				t.Fatalf("RecentHistorySTF returned an error: %v", err)
			}

			// Compare the expected and actual post_state
			assert.Equal(t, history.PostState, resultPostState, "Post state mismatch for file: %s", file)
		})
	}
}

func RecentHistorySTF(sdb *storage.StateDBStorage, input HInput, preState State) (State, error) {
	// Ensure there is at least one block in the state history
	var lastBeta HBeta
	if len(preState.Beta) == 0 {
		lastBeta = HBeta{}
	} else {
		lastBeta = preState.Beta[len(preState.Beta)-1]
	}

	// Step 1: Update β† ≡ β except β†[|β|−1]ₛ = Hᵣ
	// Extract the last β from preState

	r := []common.Hash{}
	trie_mmr := trie.NewMMR() // nil, sdb
	// Step 2: Compute the accumulation-
	for _, wp := range input.WorkPackages {
		trie_mmr.Append(wp.Bytes())
		r = append(r, wp)
	}
	trie_mmr.Append(input.AccumulateRoot.Bytes())
	r = append(r, input.AccumulateRoot)

	// Step 3: Append the new block's header hash and Merkle root to β†
	newBeta := HBeta{
		HeaderHash: input.HeaderHash, // present Header hash
		MMR:        MMRAdjusted(lastBeta.MMR, r),
		StateRoot:  common.Hash{},
		Reported:   input.WorkPackages, // from E_G
	}

	// Step 4: Update the state trie root
	newBetaEncode, _ := json.Marshal(newBeta)

	trie := trie.NewMerkleTree(nil, sdb)
	trie.SetState(C10, newBetaEncode)
	newBeta.StateRoot = trie.GetRoot()

	// Step 5: Append n to β† and shift out the oldest block if necessary
	postState := State{
		Beta: append(preState.Beta, newBeta),
	}

	// Keep only the last H blocks (remove the oldest if necessary)
	if len(postState.Beta) > types.RecentHistorySize {
		postState.Beta = postState.Beta[len(postState.Beta)-types.RecentHistorySize:]
	}

	return postState, nil
}

func MMRAdjusted(pre MMR, newPeaks []common.Hash) (post MMR) {
	// TODO
	return post
}
