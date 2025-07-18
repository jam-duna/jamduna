package statedb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/nsf/jsondiff"
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

type StateTransitionCheck struct {
	ValidMatch         bool          `json:"valid_match"`
	PostStateRootMatch bool          `json:"post_state_root_match"`
	PostStateMismatch  []common.Hash `json:"post_state_mismatch"`
}

func compareKeyVals(p0 []KeyVal, p1 []KeyVal) {
	if len(p0) != len(p1) {
		fmt.Printf("len pre %d != len post %d\n", len(p0), len(p1))
	}
	kv0 := makemap(p0)
	kv1 := makemap(p1)
	for k0, v0 := range kv0 {
		v1 := kv1[k0]
		if !common.CompareBytes(v0, v1) {
			fmt.Printf("K %v\nC: %x\nP: %x\n", k0, v0, v1)
		}
	}
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

	compareKeyVals(s1.GetAllKeyValues(), st.PostState.KeyVals)
	return fmt.Errorf("mismatch")
}

func CheckStateTransitionWithOutput(storage *storage.StateDBStorage, st *StateTransition, ancestorSet map[common.Hash]uint32, pvmBackend string, writeFile ...string) (diffs map[string]DiffState, err error) {
	// Apply the state transition
	s0, err := NewStateDBFromStateTransition(storage, st)
	if err != nil {
		return nil, err
	}
	s0.Id = storage.NodeID
	s0.AncestorSet = ancestorSet
	s1, err := ApplyStateTransitionFromBlock(s0, context.Background(), &(st.Block), nil, pvmBackend)
	if err != nil {
		fmt.Printf("!!! ApplyStateTransitionFromBlock error: %v\n", err)
		return nil, err
	}
	if st.PostState.StateRoot == s1.StateRoot {
		return nil, nil
	}
	// s1 is the ACTUAL stf output
	// st.PostState.KeyVals is the EXPECTED stf output

	post_actual := s1.GetAllKeyValues()
	post_expected := st.PostState.KeyVals
	if len(post_actual) != len(post_expected) {
		fmt.Printf("len post_actual %d != len post_expected %d\n", len(post_actual), len(post_expected))
		//fmt.Printf("post_actual\n%v\n", KeyVals(post_actual).String())
		//fmt.Printf("post_expected\n%v\n", KeyVals(post_expected).String())
	}
	if len(writeFile) > 0 && writeFile[0] != "" {
		// write the output to a file
		var stateTransition StateTransition
		stateTransition.PreState = st.PreState
		stateTransition.Block = st.Block
		stateTransition.PostState = StateSnapshotRaw{
			StateRoot: s1.StateRoot,
			KeyVals:   post_actual,
		}
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
