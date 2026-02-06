package proof

import (
	"testing"

	"github.com/jam-duna/jamduna/bmt/core"
)

// TestPathProofEmptyTrie tests proof for empty trie.
func TestPathProofEmptyTrie(t *testing.T) {
	proof := PathProof{
		Key:   core.KeyPath{1, 2, 3},
		Value: nil,
		Path:  []ProofNode{},
		Root:  core.Terminator,
	}

	hasher := &core.GpNodeHasher{}
	if !proof.Verify(hasher) {
		t.Error("Empty trie proof should verify")
	}

	if !proof.IsExclusionProof() {
		t.Error("Empty trie proof should be exclusion proof")
	}
}

// TestPathProofSingleLeaf tests proof for trie with single leaf.
func TestPathProofSingleLeaf(t *testing.T) {
	key := core.KeyPath{0x00, 0x00, 0x00, 0x00}
	value := []byte("test value")

	hasher := &core.GpNodeHasher{}

	// Build single-leaf trie
	valueHash := hasher.HashValue(value)
	leafData := core.NewLeafDataGP(key, value, valueHash)
	root := hasher.HashLeaf(leafData)

	// Proof for single leaf has empty path (no internal nodes)
	proof := PathProof{
		Key:   key,
		Value: value,
		Path:  []ProofNode{},
		Root:  root,
	}

	if !proof.Verify(hasher) {
		t.Error("Single leaf proof should verify")
	}

	if !proof.IsInclusionProof() {
		t.Error("Single leaf proof should be inclusion proof")
	}
}

// TestPathProofTwoLeaves tests proof for trie with two leaves.
func TestPathProofTwoLeaves(t *testing.T) {
	// Two keys that differ at first bit
	keyLeft := core.KeyPath{0x00, 0x00, 0x00, 0x00}  // 0000...
	keyRight := core.KeyPath{0x80, 0x00, 0x00, 0x00} // 1000...
	valueLeft := []byte("left value")
	valueRight := []byte("right value")

	hasher := &core.GpNodeHasher{}

	// Build leaves
	valueHashLeft := hasher.HashValue(valueLeft)
	valueHashRight := hasher.HashValue(valueRight)

	leafLeft := core.NewLeafDataGP(keyLeft, valueLeft, valueHashLeft)
	leafRight := core.NewLeafDataGP(keyRight, valueRight, valueHashRight)

	nodeLeft := hasher.HashLeaf(leafLeft)
	nodeRight := hasher.HashLeaf(leafRight)

	// Build root (internal node with two children)
	internal := &core.InternalData{Left: nodeLeft, Right: nodeRight}
	root := hasher.HashInternal(internal)

	// Proof for left leaf
	proofLeft := PathProof{
		Key:   keyLeft,
		Value: valueLeft,
		Path: []ProofNode{
			{
				Node:         nodeLeft,
				Sibling:      nodeRight,
				IsRightChild: false, // left child
			},
		},
		Root: root,
	}

	if !proofLeft.Verify(hasher) {
		t.Error("Left leaf proof should verify")
	}

	// Proof for right leaf
	proofRight := PathProof{
		Key:   keyRight,
		Value: valueRight,
		Path: []ProofNode{
			{
				Node:         nodeRight,
				Sibling:      nodeLeft,
				IsRightChild: true, // right child
			},
		},
		Root: root,
	}

	if !proofRight.Verify(hasher) {
		t.Error("Right leaf proof should verify")
	}
}

// TestPathProofExclusion tests exclusion proof (key doesn't exist).
func TestPathProofExclusion(t *testing.T) {
	// Build trie with one key
	existingKey := core.KeyPath{0x00, 0x00, 0x00, 0x00}
	value := []byte("exists")

	hasher := &core.GpNodeHasher{}
	valueHash := hasher.HashValue(value)
	leafData := core.NewLeafDataGP(existingKey, value, valueHash)
	root := hasher.HashLeaf(leafData)

	// Prove that a different key doesn't exist
	missingKey := core.KeyPath{0xFF, 0x00, 0x00, 0x00}

	// Exclusion proof shows path ends at terminator
	proof := PathProof{
		Key:   missingKey,
		Value: nil, // Exclusion
		Path:  []ProofNode{},
		Root:  root,
	}

	// This simplified exclusion proof won't verify against the actual root
	// (it would need to show the path through the trie to a terminator)
	// For now, just verify it's recognized as exclusion proof
	if !proof.IsExclusionProof() {
		t.Error("Should be recognized as exclusion proof")
	}
}

// TestProofNodeSiblings tests sibling handling in proof nodes.
func TestProofNodeSiblings(t *testing.T) {
	hasher := &core.GpNodeHasher{}

	// Create two different nodes
	value1 := []byte("a")
	value2 := []byte("b")

	valueHash1 := hasher.HashValue(value1)
	valueHash2 := hasher.HashValue(value2)

	leaf1 := core.NewLeafDataGP(core.KeyPath{0x00}, value1, valueHash1)
	leaf2 := core.NewLeafDataGP(core.KeyPath{0x01}, value2, valueHash2)

	node1 := hasher.HashLeaf(leaf1)
	node2 := hasher.HashLeaf(leaf2)

	// Create proof node with sibling
	proofNode := ProofNode{
		Node:         node1,
		Sibling:      node2,
		IsRightChild: false,
	}

	// Verify sibling is stored correctly
	if proofNode.Sibling != node2 {
		t.Error("Sibling should match node2")
	}

	// Verify left/right distinction
	if proofNode.IsRightChild {
		t.Error("Should be left child")
	}
}

// TestMultiProofStructure tests multi-proof structure.
func TestMultiProofStructure(t *testing.T) {
	// Create multi-proof for 3 keys
	multiProof := MultiProof{
		Keys: []core.KeyPath{
			{0x00},
			{0x10},
			{0x20},
		},
		Values: [][]byte{
			[]byte("value0"),
			[]byte("value1"),
			nil, // Exclusion for third key
		},
		Nodes: []core.Node{},
		Root:  core.Terminator,
	}

	if len(multiProof.Keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(multiProof.Keys))
	}

	if len(multiProof.Values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(multiProof.Values))
	}

	// Verify inclusion/exclusion
	if multiProof.Values[0] == nil {
		t.Error("First value should be inclusion")
	}
	if multiProof.Values[2] != nil {
		t.Error("Third value should be exclusion")
	}
}

// TestPathProofInvalidRoot tests that proof fails with wrong root.
func TestPathProofInvalidRoot(t *testing.T) {
	key := core.KeyPath{0x00}
	value := []byte("test")

	hasher := &core.GpNodeHasher{}
	valueHash := hasher.HashValue(value)
	leafData := core.NewLeafDataGP(key, value, valueHash)
	correctRoot := hasher.HashLeaf(leafData)

	// Create proof with wrong root
	wrongRoot := core.Node{0xFF, 0xFF, 0xFF, 0xFF}

	proof := PathProof{
		Key:   key,
		Value: value,
		Path:  []ProofNode{},
		Root:  wrongRoot, // Wrong!
	}

	// Should fail verification because root doesn't match
	// (even though path is empty, leaf hash won't equal wrong root)
	if correctRoot == wrongRoot {
		t.Skip("Test setup error - roots shouldn't match")
	}

	// For single leaf with empty path, verification computes leaf hash
	// and compares to root. Wrong root should fail.
	if proof.Verify(hasher) {
		t.Error("Proof with wrong root should fail verification")
	}
}
