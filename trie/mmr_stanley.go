package trie

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// Node represents a node in the Merkle Tree
type MMRNode struct {
	Hash  []byte
	Value []byte
	Left  *MMRNode
	Right *MMRNode
}

// MerkleMountainRange represents the MMR structure
type MerkleMountainRange struct {
	peaks    []*MMRNode
	hashType string
}

// NewMMR creates a new Merkle Mountain Range
func NewMMR_stanley(hashTypes ...string) *MerkleMountainRange {
	hashType := types.Blake2b
	if len(hashTypes) != 0 && hashTypes[0] == types.Keccak {
		hashType = types.Keccak
	}
	return &MerkleMountainRange{
		peaks:    make([]*MMRNode, 0),
		hashType: hashType,
	}
}

// Append adds a new leaf to the MMR
func (mmr *MerkleMountainRange) Append(value []byte) {
	leaf := &MMRNode{
		// Hash:  computeHash(value),
		Hash:  value,
		Value: value,
	}
	mmr.peaks = append(mmr.peaks, leaf)
	mmr.rebalance()
}

// rebalance maintains the MMR properties
func (mmr *MerkleMountainRange) rebalance() {
	for len(mmr.peaks) > 1 {
		lastPeakSize := mmr.nodeSize(mmr.peaks[len(mmr.peaks)-1])
		secondLastPeakSize := mmr.nodeSize(mmr.peaks[len(mmr.peaks)-2])
		if lastPeakSize == secondLastPeakSize && isPowerOfTwo(lastPeakSize) {
			left := mmr.peaks[len(mmr.peaks)-2]
			right := mmr.peaks[len(mmr.peaks)-1]
			combined := &MMRNode{
				Hash:  computeHash(append(left.Hash, right.Hash...), mmr.hashType),
				Left:  left,
				Right: right,
			}
			mmr.peaks = mmr.peaks[:len(mmr.peaks)-2]
			mmr.peaks = append(mmr.peaks, combined)
		} else {
			break
		}
	}
}

// Root returns the root hash of the MMR
func (mmr *MerkleMountainRange) Root() common.Hash {
	if len(mmr.peaks) == 0 {
		return common.BytesToHash([]byte{})
	}
	combined := mmr.peaks[0].Hash
	for i := 1; i < len(mmr.peaks); i++ {
		combined = append(combined, mmr.peaks[i].Hash...)
	}
	fmt.Printf("Compute Root Hash(%x): %x\n", combined, computeHash(combined, mmr.hashType))
	return common.BytesToHash(computeHash(combined, mmr.hashType))
}

// Get returns the value of the leaf at the given index
func (mmr *MerkleMountainRange) Get(index int) ([]byte, error) {
	if index < 0 {
		return nil, errors.New("index out of range")
	}

	currentIndex := 0
	for _, peak := range mmr.peaks {
		value, err := getLeafAtIndex(peak, index, &currentIndex)
		if err == nil {
			return value, nil
		}
	}

	return nil, errors.New("index out of range")
}

func getLeafAtIndex(node *MMRNode, targetIndex int, currentIndex *int) ([]byte, error) {
	if node.Left == nil && node.Right == nil {
		if *currentIndex == targetIndex {
			return node.Value, nil
		}
		*currentIndex++
		return nil, errors.New("index not found")
	}

	if node.Left != nil {
		value, err := getLeafAtIndex(node.Left, targetIndex, currentIndex)
		if err == nil {
			return value, nil
		}
	}

	if node.Right != nil {
		value, err := getLeafAtIndex(node.Right, targetIndex, currentIndex)
		if err == nil {
			return value, nil
		}
	}

	return nil, errors.New("index not found")
}

// PrintTree prints the structure of the MMR
func (mmr *MerkleMountainRange) PrintTree() {
	fmt.Println("Merkle Mountain Range Structure:")
	for i, peak := range mmr.peaks {
		fmt.Printf("Peak %d (Hash: %s, Size: %d):\n", i, common.Bytes2Hex(peak.Hash), mmr.nodeSize(peak))
		mmr.printNode(peak, "  ")
	}
}

func (mmr *MerkleMountainRange) printNode(node *MMRNode, indent string) {
	if node.Left != nil && node.Right != nil {
		fmt.Printf("%sBranch Node: %s\n", indent, common.Bytes2Hex(node.Hash))
		fmt.Printf("%sLeft:\n", indent)
		mmr.printNode(node.Left, indent+"  ")
		fmt.Printf("%sRight:\n", indent)
		mmr.printNode(node.Right, indent+"  ")
	} else {
		fmt.Printf("%sLeaf Node: %s\n", indent, common.Bytes2Hex(node.Hash))
	}
}

// nodeSize calculates the size of a given node
func (mmr *MerkleMountainRange) nodeSize(node *MMRNode) int {
	if node == nil {
		return 0
	}
	if node.Left == nil && node.Right == nil {
		return 1
	}
	return mmr.nodeSize(node.Left) + mmr.nodeSize(node.Right)
}

// MerkleProof represents the proof needed to verify an entry in the MMR
type MerkleProof struct {
	LeafHash        []byte
	RootHash        []byte
	PeakHashes      [][]byte
	SiblingHashes   [][]byte
	SiblingPosition []string
}

// Trace generates a Merkle proof for the given value
func (mmr *MerkleMountainRange) Trace(value []byte) *MerkleProof {
	// leafHash := computeHash(value)
	leafHash := value
	var siblingHashes [][]byte
	var siblingPositions []string

	for _, peak := range mmr.peaks {
		if tracePath, pos := traceNode(peak, leafHash); tracePath != nil {
			siblingHashes = append(siblingHashes, tracePath...)
			siblingPositions = append(siblingPositions, pos...)
			break
		}
	}

	var peakHashes [][]byte
	for _, peak := range mmr.peaks {
		peakHashes = append(peakHashes, peak.Hash)
	}

	return &MerkleProof{
		LeafHash:        leafHash,
		RootHash:        mmr.Root().Bytes(),
		PeakHashes:      peakHashes,
		SiblingHashes:   siblingHashes,
		SiblingPosition: siblingPositions,
	}
}

func traceNode(node *MMRNode, hash []byte) ([][]byte, []string) {
	if node == nil {
		return nil, nil
	}
	if bytes.Equal(node.Hash, hash) {
		return [][]byte{}, []string{}
	}
	if node.Left != nil {
		leftPath, leftPos := traceNode(node.Left, hash)
		if leftPath != nil {
			return append(leftPath, node.Right.Hash), append(leftPos, "right")
		}
	}
	if node.Right != nil {
		rightPath, rightPos := traceNode(node.Right, hash)
		if rightPath != nil {
			return append(rightPath, node.Left.Hash), append(rightPos, "left")
		}
	}
	return nil, nil
}

// Verify verifies the given value against the provided Merkle proof
func (mmr *MerkleMountainRange) Verify(value []byte, proof *MerkleProof) bool {
	// hash := computeHash(value)
	hash := value
	for i, siblingHash := range proof.SiblingHashes {
		if proof.SiblingPosition[i] == "left" {
			fmt.Printf("Compute Hash(%x, %x): %x\n", siblingHash, hash, computeHash(append(siblingHash, hash...), mmr.hashType))
			hash = computeHash(append(siblingHash, hash...), mmr.hashType)
		} else {
			fmt.Printf("Compute Hash(%x, %x): %x\n", hash, siblingHash, computeHash(append(hash, siblingHash...), mmr.hashType))
			hash = computeHash(append(hash, siblingHash...), mmr.hashType)
		}
	}

	var combined []byte
	for _, peakHash := range proof.PeakHashes {
		if bytes.Equal(peakHash, proof.LeafHash) {
			combined = append(combined, hash...)
		} else {
			combined = append(combined, peakHash...)
		}
	}
	fmt.Printf("Compute finalHash(%x): %x\n", combined, computeHash(combined, mmr.hashType))
	finalHash := computeHash(combined, mmr.hashType)
	return bytes.Equal(finalHash, proof.RootHash)
}

// isPowerOfTwo checks if a number is a power of two
func isPowerOfTwo(x int) bool {
	return (x & (x - 1)) == 0
}
