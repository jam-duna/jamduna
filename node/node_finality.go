package node

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

// Check if a StateDB has exactly one child (i.e., no forks)
func (n *Node) hasSingleChild(headerHash common.Hash) bool {

	count := 0
	for _, statedb := range n.statedbMap {
		if statedb.ParentHeaderHash == headerHash {
			count++
		}
		if count > 1 {
			return false // More than one child means a fork
		}
	}
	return count == 1
}

// Count descendants with no forks
func (n *Node) countDescendantsWithNoForks(headerHash common.Hash, depth int) int {
	if depth == 5 {
		return 1 // Reached depth of 5 descendants
	}

	for _, statedb := range n.statedbMap {
		if statedb.ParentHeaderHash == headerHash {
			if n.hasSingleChild(headerHash) {
				// Recursively check descendants
				return n.countDescendantsWithNoForks(statedb.ParentHeaderHash, depth+1)
			}
		}
	}
	return 0
}

// Mark StateDB as finalized if it has 5 descendants with no forks
func (n *Node) finalizeBlocks() {
	n.statedbMapMutex.Lock()
	defer n.statedbMapMutex.Unlock()

	for _, statedb := range n.statedbMap {
		if !statedb.Finalized && n.countDescendantsWithNoForks(statedb.HeaderHash, 0) == 1 {
			if debugF {
				fmt.Printf("%s Finality %v\n", n.String(), statedb.HeaderHash)
			}
			// TODO: connect to Grandpa
			blsSignature, finalizedEpoch, err := statedb.Finalize(n.credential)
			if err != nil {
				fmt.Printf("%s [finalizeBlocks:Finalize] ERR %v", n.String(), err)
			} else if finalizedEpoch {
				if debug {
					fmt.Printf("%s [finalizeBlocks:Finalize] BLS Signature %x\n", n.String(), blsSignature)
				}
			}
		}
	}
}
