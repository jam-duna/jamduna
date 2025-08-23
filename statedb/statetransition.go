package statedb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/nsf/jsondiff"
	"github.com/yudai/gojsondiff"
	"github.com/yudai/gojsondiff/formatter"
)

type StateTransitionChallenge struct {
	PreState StateSnapshotRaw `json:"pre_state"`
	Block    types.Block      `json:"block"`
}

func (s *StateTransitionChallenge) String() string {
	jsonEncode, _ := json.Marshal(s)
	return string(jsonEncode)
}

type StateTransition struct {
	PreState  StateSnapshotRaw `json:"pre_state"`
	Block     types.Block      `json:"block"`
	PostState StateSnapshotRaw `json:"post_state"`
}

func (s *StateTransition) ToJSON() string {
	return types.ToJSON(s)
}

func (s *StateTransition) DeepCopy() *StateTransition {

	stfCopyByte, err := types.Encode(s)
	if err != nil {
		return nil
	}
	stfInterface, _, _ := types.Decode(stfCopyByte, reflect.TypeOf(StateTransition{}))
	stf, ok := stfInterface.(StateTransition)
	if !ok {
		return nil
	}
	return &stf
}

type StateTransitionCheck struct {
	ValidMatch         bool          `json:"valid_match"`
	PostStateRootMatch bool          `json:"post_state_root_match"`
	PostStateMismatch  []common.Hash `json:"post_state_mismatch"`
}

type DiffState struct {
	Prestate          []byte
	ActualPostState   []byte
	ExpectedPostState []byte
	ActualMeta        string
	ExpectedMeta      string
}

func (d DiffState) String() string {
	return types.ToJSONHex(d)
}

func (d *StateTransition) String() string {
	return types.ToJSON(d)
}

func CompareKeyValsWithOutput(prestate, actual, expected []KeyVal) map[string]DiffState {
	return compareKeyValsWithOutput(prestate, actual, expected)
}

func compareKeyValsWithOutput(prestate, actual, expected []KeyVal) map[string]DiffState {
	// build maps: key → bytes and key → metadata
	kv_pre := makemap(prestate)
	kv_actual := makemap(actual)
	kv_expected := makemap(expected)

	diffs := make(map[string]DiffState)

	// collect the union of all keys
	allKeys := make(map[common.Hash]struct{})
	for k := range kv_actual {
		allKeys[k] = struct{}{}
	}
	for k := range kv_expected {
		allKeys[k] = struct{}{}
	}

	for k := range allKeys {
		v_actual, actual_found := kv_actual[k]
		v_expected, expected_found := kv_expected[k]

		// if both present and equal, skip
		if actual_found && expected_found && bytes.Equal(v_actual, v_expected) {
			continue
		}

		// pick a human‐readable diff key from metadata, else hex
		diffState := DiffState{
			Prestate:          kv_pre[k],  // from your original map
			ActualPostState:   v_actual,   // p0 value, or nil if !ok0
			ExpectedPostState: v_expected, // p1 value, or nil if !ok1
		}
		diffs[k.Hex()] = diffState
	}

	return diffs
}

func makemap(p []KeyVal) map[common.Hash][]byte {
	kvMap := make(map[common.Hash][]byte)

	for _, kvs := range p {
		keyBytes := make([]byte, 32)
		copy(keyBytes[:], kvs.Key[:]) // pad one extra byte !!!
		k := common.BytesToHash(keyBytes)
		v := kvs.Value
		kvMap[k] = v
	}
	return kvMap
}

func ComputeStateTransition(storage *storage.StateDBStorage, stc *StateTransition, pvmBackend string) (ok bool, postStateSnapshot *StateSnapshotRaw, jamErr error, miscErr error) {
	preState, miscErr := NewStateDBFromStateTransition(storage, stc)
	if miscErr != nil {
		// Invalid pre-state
		return false, nil, nil, fmt.Errorf("PreState Error")
	}
	scBlock := stc.Block
	postState, jamErr := ApplyStateTransitionFromBlock(preState, context.Background(), &scBlock, nil, pvmBackend)
	if jamErr != nil {
		// When validating Fuzzed STC, we shall expect jamError
		return true, nil, jamErr, nil
	}

	postState.StateRoot = postState.UpdateTrieState()
	postStateSnapshot = &StateSnapshotRaw{
		StateRoot: postState.StateRoot,
		KeyVals:   postState.GetAllKeyValues(),
	}
	return true, postStateSnapshot, nil, nil

}

// NOTE CheckStateTransition vs CheckStateTransitionWithOutput a good example of "copy-paste" coding increases in complexity
func CheckStateTransition(storage *storage.StateDBStorage, st *StateTransition, ancestorSet map[common.Hash]uint32, pvmBackend string) error {
	// Apply the state transition
	s0, err := NewStateDBFromStateTransition(storage, st)
	if err != nil {
		return err
	}
	s0.Id = storage.NodeID
	s0.AncestorSet = ancestorSet
	s1, err := ApplyStateTransitionFromBlock(s0, context.Background(), &(st.Block), nil, pvmBackend)
	if err != nil {
		return err
	}

	if st.PostState.StateRoot == s1.StateRoot {
		return nil
	}

	//compareKeyVals(s1.GetAllKeyValues(), st.PostState.KeyVals)
	return fmt.Errorf("mismatch")
}

func CheckStateTransitionWithOutput(storage *storage.StateDBStorage, st *StateTransition, ancestorSet map[common.Hash]uint32, pvmBackend string, runPrevalidation bool, writeFile ...string) (diffs map[string]DiffState, err error) {
	// Apply the state transition
	preState, err := NewStateDBFromStateTransition(storage, st)
	if err != nil {
		return nil, err
	}

	if runPrevalidation {
		if bytes.Equal(preState.StateRoot.Bytes(), st.PostState.StateRoot.Bytes()) {
			//			return nil, fmt.Errorf("OMIT")
		}

		if !bytes.Equal(preState.StateRoot.Bytes(), st.PreState.StateRoot.Bytes()) {
			return diffs, fmt.Errorf("PreState.StateRoot mismatch: expected %s, got %s", st.PreState.StateRoot.Hex(), preState.StateRoot.Hex())
		}

		post_state, err := NewStateDBFromStateTransitionPost(storage, st)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(post_state.StateRoot.Bytes(), st.PostState.StateRoot.Bytes()) {
			return diffs, fmt.Errorf("PostState.StateRoot mismatch: expected %s, got %s", st.PostState.StateRoot.Hex(), post_state.StateRoot.Hex())
		}
		fmt.Printf("STF StateRoot - Pre: %s | Post: %s\n", preState.StateRoot.Hex(), post_state.StateRoot.Hex())
	}

	s0 := preState
	s0.Id = storage.NodeID
	s0.AncestorSet = ancestorSet
	s1, err := ApplyStateTransitionFromBlock(s0, context.Background(), &(st.Block), nil, pvmBackend)
	if err != nil {
		fmt.Printf("!!! ApplyStateTransitionFromBlock error: %v\n", err)
		return nil, err
	}

	if bytes.Compare(st.PostState.StateRoot.Bytes(), s1.StateRoot.Bytes()) == 0 {
		return nil, nil
	}

	// s1 is the ACTUAL stf output
	// st.PostState.KeyVals is the EXPECTED stf output
	post_actual_root := s1.UpdateTrieState()
	post_actual := s1.GetAllKeyValues()
	post_expected := st.PostState.KeyVals
	if len(post_actual) != len(post_expected) {
		fmt.Printf("len post_actual %d != len post_expected %d\n", len(post_actual), len(post_expected))
		//fmt.Printf("post_actual\n%v\n", KeyVals(post_actual).String())
		//fmt.Printf("post_expected\n%v\n", KeyVals(post_expected).String())
	}

	if post_actual_root != st.PostState.StateRoot {
		fmt.Printf("!!! post_actual_root %s != post_expected %s\n", post_actual_root.Hex(), st.PostState.StateRoot.Hex())
	}

	// write the output to a file
	var stateTransition StateTransition
	stateTransition.PreState = st.PreState
	stateTransition.Block = st.Block
	stateTransition.PostState = StateSnapshotRaw{
		StateRoot: s1.StateRoot,
		KeyVals:   post_actual,
	}

	showPostState := false

	if len(writeFile) > 0 && writeFile[0] != "" {
		fmt.Printf("writing post-state to %s\n", writeFile[0])
		// create the file
		fileName := writeFile[0]
		file, err := os.Create(fileName)
		if err != nil {
			fmt.Printf("Error creating file: %v\n", err)
			return nil, err
		}
		defer file.Close()
		// write the output to the file
		jsonOutput, err := json.MarshalIndent(stateTransition, "", "  ")
		if err != nil {
			fmt.Printf("Error marshalling JSON: %v\n", err)
			return nil, err
		}
		_, err = file.Write(jsonOutput)
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
			return nil, err
		}
		fmt.Printf("Output written to %s\n", fileName)

	} else if showPostState {
		jsonStr, _ := json.Marshal(stateTransition)
		fmt.Printf("!!StateTransition: %s\n", string(jsonStr))

	}
	diffs = compareKeyValsWithOutput(st.PreState.KeyVals, post_actual, post_expected)
	return diffs, fmt.Errorf("mismatch")
}

func CompareJSON(obj1, obj2 interface{}) string {
	json1, err1 := json.Marshal(obj1)
	json2, err2 := json.Marshal(obj2)
	if err1 != nil || err2 != nil {
		return "Error marshalling JSON"
	}
	opts := jsondiff.DefaultJSONOptions()
	diff, diffStr := jsondiff.Compare(json1, json2, &opts)

	if diff == jsondiff.FullMatch {
		return "JSONs are identical"
	}
	return fmt.Sprintf("Diff detected:\n%s", diffStr)
}

func HandleDiffs(diffs map[string]DiffState) {
	keys := make([]string, 0, len(diffs))
	for k := range diffs {
		keys = append(keys, k)
	}
	SortDiffKeys(keys)

	fmt.Printf("Diff on %d keys: %v\n", len(keys), keys)
	for _, key := range keys {
		val := diffs[key]

		stateType := "unknown"
		if m := strings.TrimSuffix(val.ActualMeta, "|"); m != "" {
			stateType = m
		} else {
			keyFirstByte := common.FromHex(key)[0]
			if tmp, ok := StateKeyMap[keyFirstByte]; ok {
				stateType = tmp
			}
		}

		fmt.Println(strings.Repeat("=", 40))
		fmt.Printf("\033[34mState Key: %s (%s)\033[0m\n", stateType, key[:64])
		fmt.Printf("%-10s | PreState : 0x%x\n", stateType, val.Prestate)
		printHexDiff(stateType, val.ExpectedPostState, val.ActualPostState)

		if stateType != "unknown" {
			expJSON, _ := StateDecodeToJson(val.ExpectedPostState, stateType)
			actJSON, _ := StateDecodeToJson(val.ActualPostState, stateType)

			differ := gojsondiff.New()
			delta, err := differ.Compare([]byte(expJSON), []byte(actJSON))
			if err == nil && delta.Modified() {
				var leftObj, rightObj interface{}
				_ = json.Unmarshal([]byte(expJSON), &leftObj)
				_ = json.Unmarshal([]byte(actJSON), &rightObj)

				cfg := formatter.AsciiFormatterConfig{ShowArrayIndex: true, Coloring: true}
				asciiFmt := formatter.NewAsciiFormatter(leftObj, cfg)
				asciiDiff, _ := asciiFmt.Format(delta)
				fmt.Println(asciiDiff)
			}
			fmt.Printf("------ %s JSON DONE ------\n", stateType)
		}
		fmt.Println(strings.Repeat("=", 40))
	}
}
