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

func compareKeyVals(p0 KeyVals, p1 KeyVals) {
	if len(p0) != len(p1) {
		fmt.Printf("len pre %d != len post %d\n", len(p0), len(p1))
	}
	m0 := makemap(p0)
	m1 := makemap(p1)
	for k0, v0 := range m0 {
		v1 := m1[k0]
		if !common.CompareBytes(v0, v1) {
			fmt.Printf("K %v\n s1: %x\n st.PostState: %x\n", k0, v0, v1)
		}
	}
}

func makemap(p KeyVals) map[common.Hash][]byte {
	m := make(map[common.Hash][]byte)
	for _, kvs := range p {
		k := common.BytesToHash(kvs[0])
		v := kvs[1]
		m[k] = v
	}
	return m
}

func CheckStateTransition(storage *storage.StateDBStorage, st *StateTransition, ancestorSet []types.BlockHeader, accumulationRoot common.Hash) error {
	// Apply the state transition
	s0, err := NewStateDBFromSnapshotRaw(storage, &(st.PreState))
	if err != nil {
		return err
	}

	s0.AncestorSet = ancestorSet
	s0.AccumulationRoot = accumulationRoot
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
