package statedb

//
// import (
// 	"encoding/json"
// 	"fmt"
// 	"os"
// 	"path/filepath"
// 	"testing"
//
// 	"github.com/colorfulnotion/jam/common"
// 	"github.com/colorfulnotion/jam/trie"
// 	"github.com/colorfulnotion/jam/types"
//
// 	"reflect"
// )
//
// // Input struct definition
// type HInput struct {
// 	HeaderHash      common.Hash   `json:"header_hash"`
// 	ParentStateRoot common.Hash   `json:"parent_state_root"`
// 	AccumulateRoot  common.Hash   `json:"accumulate_root"`
// 	Reported        []common.Hash `json:"work_packages"`
// }
//
// type HState struct {
// 	RecentBlocks RecentBlocks `json:"beta"`
// }
//
// type HistoryData struct {
// 	Input     HInput   `json:"input"`
// 	PreState  HState   `json:"pre_state"`
// 	Output    *HOutput `json:"output"`
// 	PostState HState   `json:"post_state"`
// }
//
// type HOutput struct{}
//
// func (b Beta_state) String() string {
// 	enc, err := json.MarshalIndent(b, "", "  ")
// 	if err != nil {
// 		// Handle the error according to your needs.
// 		return fmt.Sprintf("Error marshaling JSON: %v", err)
// 	}
// 	return string(enc)
// }
//
// func HistorySTF(input HInput, s HState) (poststate HState) {
// 	// Eq 83 n
// 	// Eq 83 n.p -- aggregate all the workpackagehashes of the guarantees
// 	preRecentBlocks := s.RecentBlocks
// 	if len(preRecentBlocks) > 0 {
// 		preRecentBlocks[len(preRecentBlocks)-1].StateRoot = input.ParentStateRoot
// 	}
//
// 	// Eq 83 n.b
// 	mmr := trie.NewMMR_stanley()
// 	if len(preRecentBlocks) > 0 {
// 		mmr.Peaks = preRecentBlocks[len(preRecentBlocks)-1].B.Peaks
// 	}
// 	mmr.Append(&(input.AccumulateRoot))
// 	n := Beta_state{
// 		Reported:   input.Reported,   // p
// 		HeaderHash: input.HeaderHash, // h
// 		B:          *mmr,             // b
// 		StateRoot:  common.Hash{},    // this is the POSTERIOR stateroot
// 	}
//
// 	// Eq 84 β' ≡ β† ++ n (last H=types.RecentHistorySize)
// 	postRecentBlocks := append(preRecentBlocks, n)
// 	if len(postRecentBlocks) > types.RecentHistorySize {
// 		postRecentBlocks = postRecentBlocks[1 : types.RecentHistorySize+1]
// 	}
// 	poststate.RecentBlocks = postRecentBlocks
// 	return
// }
//
// func TestRecentHistory(t *testing.T) {
// 	testCases := []string{
// 		//		"progress_blocks_history-1.json",
// 		//"progress_blocks_history-2.json",
// 		//"progress_blocks_history-3.json",
// 		"progress_blocks_history-4.json",
// 	}
// 	for testnum, jsonFile := range testCases {
// 		t.Run(jsonFile, func(t *testing.T) {
// 			jsonPath := filepath.Join("../jamtestvectors/history/data", jsonFile)
// 			jsonData, err := os.ReadFile(jsonPath)
// 			if err != nil {
// 				t.Fatalf("failed to read JSON file: %v", err)
// 			}
// 			var h HistoryData
// 			err = json.Unmarshal(jsonData, &h)
// 			expectedh := h.PostState
// 			posth := HistorySTF(h.Input, h.PreState)
// 			if len(expectedh.RecentBlocks) != len(posth.RecentBlocks) {
// 				panic(4)
// 			}
// 			for i := 0; i < len(posth.RecentBlocks); i++ {
// 				e := expectedh.RecentBlocks[i].String()
// 				p := posth.RecentBlocks[i].String()
// 				if e == p {
// 				} else {
// 					fmt.Printf("Test %d: element %d/%d expected %s got: %s\n", testnum, i, len(posth.RecentBlocks), e, p)
// 				}
//
// 			}
//
// 		})
// 	}
// }
// func TestRecentHistoryWithMMR(t *testing.T) {
// 	testCases := []struct {
// 		jsonFile     string
// 		binFile      string
// 		expectedType interface{}
// 	}{
// 		{"progress_blocks_history-1.json", "progress_blocks_history-1.bin", &HistoryData{}},
// 		{"progress_blocks_history-2.json", "progress_blocks_history-2.bin", &HistoryData{}},
// 		// {"progress_blocks_history-3.json", "progress_blocks_history-3.bin", &HistoryData{}},
// 		// {"progress_blocks_history-4.json", "progress_blocks_history-4.bin", &HistoryData{}},
// 	}
// 	mmr := trie.NewMMR()
// 	for _, tc := range testCases {
// 		// for i, tc := range testCases {
// 		// if i != 1 {
// 		// 	continue
// 		// }
// 		t.Run(tc.jsonFile, func(t *testing.T) {
//
// 			jsonPath := filepath.Join("../jamtestvectors/history/data", tc.jsonFile)
//
// 			targetedStructType := reflect.TypeOf(tc.expectedType)
//
// 			fmt.Printf("\n\n\nTesting %v\n", targetedStructType)
// 			// Read and unmarshal JSON file
// 			jsonData, err := os.ReadFile(jsonPath)
// 			if err != nil {
// 				t.Fatalf("failed to read JSON file: %v", err)
// 			}
//
// 			var data HistoryData
// 			err = json.Unmarshal(jsonData, &data)
//
// 			mmr.PrintTree()
// 			fmt.Printf("data.Input.AccumulateRoot[:] %x\n", data.Input.AccumulateRoot[:])
// 			mmr.Append(data.Input.AccumulateRoot[:])
//
// 			mmr.PrintTree()
// 		})
// 	}
// }
