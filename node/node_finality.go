package node

import (
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

// Check if a StateDB has exactly one child (i.e., no forks)
func (n *Node) hasSingleChild(blockHash common.Hash) bool {
	count := 0
	for _, statedb := range n.statedbMap {
		if statedb.ParentHash == blockHash {
			count++
		}
		if count > 1 {
			return false // More than one child means a fork
		}
	}
	return count == 1
}

// Count descendants with no forks
func (n *Node) countDescendantsWithNoForks(blockHash common.Hash, depth int) int {
	if depth == 5 {
		return 1 // Reached depth of 5 descendants
	}

	for _, statedb := range n.statedbMap {
		if statedb.ParentHash == blockHash {
			if n.hasSingleChild(blockHash) {
				// Recursively check descendants
				return n.countDescendantsWithNoForks(statedb.BlockHash, depth+1)
			}
		}
	}
	return 0
}

// Mark StateDB as finalized if it has 5 descendants with no forks
func (n *Node) finalizeBlocks() {
	for _, statedb := range n.statedbMap {
		if !statedb.Finalized && n.countDescendantsWithNoForks(statedb.BlockHash, 0) == 1 {
			if debugF {
				fmt.Printf("%s Finality %v\n", n.String(), statedb.BlockHash)
			}
			statedb.Finalized = true
		}
	}
}
