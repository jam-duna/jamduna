package statedb

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

type StateTransition struct {
	PreState  StateSnapshotRaw `json:"pre_state"`
	Block     types.Block      `json:"block"`
	PostState StateSnapshotRaw `json:"post_state"`
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
	kv0, m0 := makemap(p0)
	kv1, m1 := makemap(p1)
	for k0, v0 := range kv0 {
		v1 := kv1[k0]
		if !common.CompareBytes(v0, v1) {
			metaKey := fmt.Sprintf("meta_%v", k0)
			metaData0 := m0[metaKey]
			metaData1 := m1[metaKey]
			fmt.Printf("K %v\ns1(Current) Meta Data: %s\nPostState Meta Data:   %s\n", k0, metaData0, metaData1)
			fmt.Printf("s1(Current) Value: %x\nPostState   Value: %x\n", v0, v1)
		}
	}
}

func makemap(p []KeyVal) (map[common.Hash][]byte, map[string]string) {
	kvMap := make(map[common.Hash][]byte)
	metaMap := make(map[string]string)
	for _, kvs := range p {
		k := common.BytesToHash(kvs.Key)
		v := kvs.Value
		kvMap[k] = v
		metaKey := fmt.Sprintf("meta_%v", k)
		metaMap[metaKey] = fmt.Sprintf("%s|%s", kvs.StructType, kvs.Metadata)
	}
	return kvMap, metaMap
}

func CheckStateTransition(storage *storage.StateDBStorage, st *StateTransition, ancestorSet map[common.Hash]uint32) error {
	// Apply the state transition
	s0, err := NewStateDBFromSnapshotRaw(storage, &(st.PreState))
	if err != nil {
		return err
	}

	s0.AncestorSet = ancestorSet
	s1, err := ApplyStateTransitionFromBlock(s0, context.Background(), &(st.Block))
	if err != nil {
		panic(err)
	}
	if st.PostState.StateRoot == s1.StateRoot {
		return nil
	}

	fmt.Printf("STATEROOT does not match: s1: %v st.PostState: %v FAIL\n", s1.StateRoot, st.PostState.StateRoot)
	compareKeyVals(s1.GetAllKeyValues(), st.PostState.KeyVals)
	return fmt.Errorf("mismatch")

}
