package trie

import (
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/common"
)

// TestMMR tests the Merkle Mountain Range implementation
func TestMMR(t *testing.T) {
	// t.Skip()
	// t.Skip()
	mmr := NewMMR()

	leaves := [][2][]byte{
		{[]byte("value1")},
		{[]byte("value2")},
		{[]byte("value3")},
		{[]byte("value4")},
		{[]byte("value5")},
		{[]byte("value1")},
		{[]byte("value2")},
		{[]byte("value3")},
		{[]byte("value4")},
		{[]byte("value5")},
		{[]byte("value2")},
		{[]byte("value3")},
		{[]byte("value4")},
		{[]byte("value5")},
		{[]byte("value5")},
	}

	for _, leaf := range leaves {
		mmr.Append(leaf[0])
	}

	mmr.PrintTree()
	rootHash := mmr.Root()
	t.Logf("rootHash:%x\n", rootHash)
	if len(rootHash.Bytes()) == 0 {
		t.Logf("Root hash should not be empty")
	}
}

func TestTraceAndVerify(t *testing.T) {
	mmr := NewMMR()

	// Append some values
	values := [][]byte{
		[]byte("A"),
		[]byte("B"),
		[]byte("C"),
		[]byte("D"),
		[]byte("E"),
		[]byte("F"),
		[]byte("G"),
	}

	for _, value := range values {
		mmr.Append(value)
	}
	fmt.Printf("Root Hash %x:\n", mmr.Root())
	mmr.PrintTree()

	// Generate and verify proofs for each value
	for _, value := range values {
		proof := mmr.Trace(value)
		fmt.Printf("Trace path for value %s:\n", string(value))
		fmt.Printf("Value %x:\n", computeHash(value))
		fmt.Printf("Leaf hash: %s\n", common.Bytes2Hex(proof.LeafHash))
		fmt.Printf("Root hash: %s\n", common.Bytes2Hex(proof.RootHash))
		for i, hash := range proof.SiblingHashes {
			fmt.Printf("Sibling hash %d: %s (Position: %s)\n", i, common.Bytes2Hex(hash), proof.SiblingPosition[i])
		}
		if !mmr.Verify(value, proof) {
			t.Errorf("Failed to verify proof for value %s", string(value))
		}
	}

	// Attempt to verify an incorrect proof
	wrongValue := []byte("Z")
	wrongProof := mmr.Trace(values[0]) // Use proof of a valid value
	if !mmr.Verify(wrongValue, wrongProof) {
		t.Errorf("Input incorrectly verified proof for wrong value should be Error")
	}
}

func TestMMRGet(t *testing.T) {
	leaves := [][2][]byte{
		{[]byte("value1")},
		{[]byte("value2")},
		{[]byte("value3")},
		{[]byte("value4")},
		{[]byte("value5")},
		{[]byte("value1")},
		{[]byte("value2")},
		{[]byte("value3")},
		{[]byte("value4")},
		{[]byte("value5")},
		{[]byte("value2")},
		{[]byte("value3")},
		{[]byte("value4")},
		{[]byte("value5")},
		{[]byte("value5")},
	}

	// Create the Merkle Mountain Range
	mmr := NewMMR()
	for _, leaf := range leaves {
		mmr.Append(leaf[0])
	}
	mmr.PrintTree()

	// Test Get
	for i := 0; i <= len(leaves); i++ {
		{
			if i < len(leaves) {
				leaf, err := mmr.Get(i)
				if err == nil {
					fmt.Printf("Get [%d]: %s\n", i, string(leaf))
				} else {
					t.Errorf("Get [%d] should be Error: %v\n", i, err)
				}
			} else {
				_, err := mmr.Get(i)
				if err != nil {
					fmt.Printf("Get [%d]: %v\n", i, err)
				} else {
					t.Errorf("Get [%d] should be Error: %v\n", i, err)
				}
			}
		}
	}
}
