package statedb

// X_B

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// BeefyRoot creates the Beefy root for the present block using service accumulations.
func BeefyRoot(serviceAccumulations []types.ServiceAccumulation, mmr trie.MerkleMountainRange) common.Hash {
	for _, accumulation := range serviceAccumulations {
		mmr.Append(accumulation.Result.Bytes())
	}
	return mmr.Root()
}
