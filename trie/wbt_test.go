package trie

import (
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/common"
)

// TestWellBalancedTree tests the MerkleB method of the WellBalancedTree
func TestWBMerkleTree(t *testing.T) {
	values := [][]byte{
		[]byte("a"),
		[]byte("b"),
		[]byte("c"),
		[]byte("d"),
		[]byte("e"),
		[]byte("f"),
	}
	tree := NewWellBalancedTree(values)
	// Print the tree structure
	tree.PrintTree()
	if tree.Root() == nil {
		t.Errorf("Root is nil")
	}
}

func TestWBTProof(t *testing.T) {
	values := [][]byte{
		[]byte("a"),
		[]byte("b"),
		[]byte("c"),
		[]byte("d"),
		[]byte("e"),
		[]byte("f"),
		[]byte("g"),
		[]byte("h"),
		[]byte("i"),
		[]byte("j"),
		[]byte("k"),
	}
	tree := NewWellBalancedTree(values)

	fmt.Printf("Root: %s\n", common.Bytes2Hex(tree.Root()))
	fmt.Printf("Total leaves: %d\n", len(tree.leaves))

	// Print the tree structure
	tree.PrintTree()

	value := []byte("e")
	path, found := tree.Trace(value)
	if found {
		fmt.Printf("Proof path for value '%s':\n", value)
		for _, p := range path {
			fmt.Println(common.Bytes2Hex(p[0]))
		}
		if Verify(tree.Root(), value, path) {
			fmt.Printf("Verification: %v\n", true)
		} else {
			t.Errorf("Proof for key [%s] is invalid.\n", value)
		}
	} else {
		fmt.Printf("Value '%s' not found in the tree.\n", value)
	}
}

func TestWBTGet(t *testing.T) {
	values := [][]byte{
		[]byte("a"),
		[]byte("b"),
		[]byte("c"),
		[]byte("d"),
		[]byte("e"),
		[]byte("f"),
		[]byte("g"),
		[]byte("h"),
		[]byte("i"),
		[]byte("j"),
		[]byte("k"),
	}
	tree := NewWellBalancedTree(values)

	fmt.Printf("Root: %s\n", common.Bytes2Hex(tree.Root()))
	fmt.Printf("Total leaves: %d\n", len(tree.leaves))

	// Print the tree structure
	tree.PrintTree()

	// Test Get
	for i := 0; i <= len(values); i++ {
		{
			if i < len(values) {
				leaf, err := tree.Get(i)
				if err == nil {
					fmt.Printf("Get [%d]: %s\n", i, string(leaf))
				} else {
					t.Errorf("Get [%d] should be Error: %v\n", i, err)
				}
			} else {
				_, err := tree.Get(i)
				if err != nil {
					fmt.Printf("Get [%d]: %v\n", i, err)
				} else {
					t.Errorf("Get [%d] should be Error: %v\n", i, err)
				}
			}
		}
	}
}

func TestTraceByIndex(t *testing.T) {
	values := [][]byte{
		[]byte("leaf1"),
		[]byte("leaf2"),
		[]byte("leaf3"),
		[]byte("leaf4"),
	}

	// Build a well-balanced tree
	wbt := NewWellBalancedTree(values)
	wbt.PrintTree()

	// Test tracing by index
	for i := 0; i < len(values); i++ {
		leafHash := computeNode(values[i])
		path, found, err := wbt.TraceByIndex(i)

		if err != nil {
			t.Fatalf("Error tracing index %d: %v", i, err)
		}
		if !found {
			t.Fatalf("Node not found for index %d", i)
		}

		if VerifyByIndex(wbt.Root(), leafHash, path) {
			fmt.Printf("Verification succeeded for index %d\n", i)
		} else {
			t.Fatalf("Verification failed for index %d", i)
		}

		fmt.Printf("Proof path for index %d:\n", i)
		for _, p := range path {
			fmt.Printf("  Sibling Hash: %s, Direction: %s\n", common.Bytes2Hex(p[0]), p[1])
		}
	}
}
