// Package proof provides proof generation and verification for the trie.
//
// This implements path proofs and multi-proofs for the Nearly Optimal Merkle Trie.
// Proofs allow proving that a key-value pair exists (or doesn't exist) in the trie
// without revealing the entire trie structure.
package proof

import (
	"github.com/jam-duna/jamduna/bmt/core"
)

// PathProof represents a proof of inclusion/exclusion for a single key.
//
// A path proof contains all nodes along the path from root to the target key,
// plus sibling nodes needed to recompute the root hash.
type PathProof struct {
	// Key being proved
	Key core.KeyPath

	// Value at the key (nil if key doesn't exist - proof of exclusion)
	Value []byte

	// Nodes along the path from root to target
	// Each node includes its sibling for hash reconstruction
	Path []ProofNode

	// Root hash of the trie
	Root core.Node
}

// ProofNode represents a node in the proof path.
type ProofNode struct {
	// The node hash
	Node core.Node

	// Sibling node hash (needed to compute parent)
	Sibling core.Node

	// Whether this node is the left (false) or right (true) child
	IsRightChild bool
}

// Verify verifies that this proof is valid for the given root hash.
//
// Returns true if the proof correctly proves the key-value pair exists
// (or proves the key doesn't exist for exclusion proofs).
func (p *PathProof) Verify(hasher core.NodeHasher) bool {
	// For a valid proof, we should be able to reconstruct the root hash
	// by hashing up from the leaf through all path nodes

	// Start from the bottom (leaf or terminator)
	var current core.Node
	if p.Value != nil {
		// Inclusion proof - start with leaf node
		gpHasher := &core.GpNodeHasher{}
		valueHash := gpHasher.HashValue(p.Value)
		leafData := core.NewLeafDataGP(p.Key, p.Value, valueHash)
		current = hasher.HashLeaf(leafData)
	} else {
		// Exclusion proof - path ends at terminator
		current = core.Terminator
	}

	// If no path nodes, this is a single-leaf tree - compare leaf hash to root directly
	if len(p.Path) == 0 {
		return p.Root == current
	}

	// Hash up through the path
	for i := len(p.Path) - 1; i >= 0; i-- {
		node := p.Path[i]

		// Combine current with sibling to compute parent
		var left, right core.Node
		if node.IsRightChild {
			left = node.Sibling
			right = current
		} else {
			left = current
			right = node.Sibling
		}

		// Hash the internal node
		internal := &core.InternalData{Left: left, Right: right}
		current = hasher.HashInternal(internal)
	}

	// Current should now equal the root
	return current == p.Root
}

// MultiProof represents a proof for multiple keys simultaneously.
//
// Multi-proofs are more efficient than individual path proofs because
// they can share common path nodes.
type MultiProof struct {
	// Keys being proved
	Keys []core.KeyPath

	// Values for each key (nil entries are exclusion proofs)
	Values [][]byte

	// Shared proof nodes (more compact than individual proofs)
	Nodes []core.Node

	// Root hash
	Root core.Node
}

// IsInclusionProof returns true if this is a proof of inclusion (key exists).
func (p *PathProof) IsInclusionProof() bool {
	return p.Value != nil
}

// IsExclusionProof returns true if this is a proof of exclusion (key doesn't exist).
func (p *PathProof) IsExclusionProof() bool {
	return p.Value == nil
}
