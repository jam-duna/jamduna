package trie

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func TestZeroLeaves(t *testing.T) {
	var values [][]byte
	tree := NewWellBalancedTree(values, types.Keccak)
	fmt.Printf("%x\n", tree.Root())
}

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
	tree := NewWellBalancedTree(values, types.Blake2b)
	// Print the tree structure
	if bptDebug {
		tree.PrintTree()
	}
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

	wbt := NewWellBalancedTree(values, types.Blake2b)

	// Test the Trace method to get the proof path for a given index
	for shardIndex := 0; shardIndex < numShards; shardIndex++ {
		treeLen, leafHash, path, isFound, err := wbt.Trace(int(shardIndex))
		if err != nil || !isFound {
			t.Errorf("Trace error: %v", err)
		}

		derivedRoot, verified, err := VerifyWBT(treeLen, shardIndex, wbt.RootHash(), leafHash, path, wbt.hashType)

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
	tree := NewWellBalancedTree(values, types.Keccak)

	if bptDebug {
		fmt.Printf("Root: %s\n", common.Bytes2Hex(tree.Root()))
		fmt.Printf("Total leaves: %d\n", len(tree.leaves))
		tree.PrintTree()
	}

	// Test Get
	for i := 0; i <= len(values); i++ {
		{
			if i < len(values) {
				leaf, err := tree.Get(i)
				if err == nil {
					if bptDebug {
						fmt.Printf("Get [%d]: %s\n", i, string(leaf))
					}
				} else {
					t.Errorf("Get [%d] should be Error: %v\n", i, err)
				}
			} else {
				_, err := tree.Get(i)
				if err != nil {
					if bptDebug {
						fmt.Printf("Get [%d]: %v\n", i, err)
					}
				} else {
					t.Errorf("Get [%d] should be Error: %v\n", i, err)
				}
			}
		}
	}
}
