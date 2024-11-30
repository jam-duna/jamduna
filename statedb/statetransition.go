package statedb

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type StateTransition struct {
	PreState      StateSnapshotRaw `json:"pre_state"`
	Block         types.Block      `json:"block"`
	PostState     StateSnapshotRaw `json:"post_state"`
	Valid         bool             `json:"valid"`
	PostStateRoot common.Hash      `json:"post_state_root"`
}

type StateTransitionCheck struct {
	ValidMatch         bool          `json:"valid_match"`
	PostStateRootMatch bool          `json:"post_state_root_match"`
	PostStateMismatch  []common.Hash `json:"post_state_mismatch"`
}
