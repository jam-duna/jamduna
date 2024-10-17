package trie

import (
	"bytes"
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

func TestWBTTrace(t *testing.T) {
	// Initialize the tree with some values
	values := [][]byte{}
	numShards := 200
	for i := 0; i < numShards; i++ {
		values = append(values, []byte(fmt.Sprintf("value%d", i)))
	}

	wbt := NewWellBalancedTree(values)

	// Test the Trace method to get the proof path for a given index
	for shardIndex := 0; shardIndex < numShards; shardIndex++ {
		treeLen, leafHash, path, isFound, err := wbt.Trace(int(shardIndex))
		if err != nil || !isFound {
			t.Errorf("Trace error: %v", err)
		}
		// wbt.PrintTree()
		// fmt.Printf("Trace path:%s\n", path)
		// fmt.Printf("Trace path for index %d:\n", shardIndex)
		// for i, hash := range path {
		// 	fmt.Printf("Step %d: %x\n", i, hash.Bytes())
		// }

		derivedRoot, verified, err := VerifyWBT(treeLen, shardIndex, wbt.RootHash(), leafHash, path)

		if err != nil || verified == false {
			t.Errorf("VerifyWBT error: %v", err)
		}
		expectedHash := wbt.Root()
		if !bytes.Equal(derivedRoot[:], expectedHash) {
			t.Errorf("shardIndex %d, expected hash %x, got %s", shardIndex, expectedHash, derivedRoot)
		}
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
