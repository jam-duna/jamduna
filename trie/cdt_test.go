package trie

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

var TestingSegmentsNum = 31

func TestCDTGet(t *testing.T) {
	testCDTGet(t, TestingSegmentsNum)
}

func TestJustify0(t *testing.T) {
	testJustify0(t, TestingSegmentsNum)
}

func TestVerifyCDTJustificationX(t *testing.T) {
	testVerifyCDTJustificationX(t, TestingSegmentsNum)
}

func TestPageProof(t *testing.T) {
	testPageProof(t, TestingSegmentsNum)
}

func TestCDT(t *testing.T) {
	//var TestingSegmentsNums = []int{0, 1, 2, 3, 5, 10, 31, 32, 33, 64, 65, 128}
	var TestingSegmentsNums []int
	for i := 0; i <= 513; i++ {
		TestingSegmentsNums = append(TestingSegmentsNums, i)
	}
	// for i := 0; i <= 2; i++ {
	// 	TestingSegmentsNums = append(TestingSegmentsNums, i)
	// }
	for _, numSegments := range TestingSegmentsNums {
		numSegments := numSegments // Capture loop variable
		t.Run(fmt.Sprintf("SegmentsNum=%d", numSegments), func(t *testing.T) {
			// Optionally run in parallel
			// t.Parallel()

			t.Run("TestCDTGet", func(t *testing.T) {
				testCDTGet(t, numSegments)
			})

			t.Run("TestJustify", func(t *testing.T) {
				testJustify0(t, numSegments)
			})

			t.Run("TestVerifyCDTJustificationX", func(t *testing.T) {
				testVerifyCDTJustificationX(t, numSegments)
			})

			t.Run("testPageProof", func(t *testing.T) {
				testPageProof(t, numSegments)
			})

		})
	}
}

func testJustify0(t *testing.T, numSegments int) {
	// Initialize segments
	var segments [][]byte
	for i := 0; i < numSegments; i++ {
		data := []byte(fmt.Sprintf("segment%d", i))
		data = common.PadToMultipleOfN(data, types.W_E*types.W_S)
		segments = append(segments, data)
	}
	leaves := segments
	// Build Merkle Tree
	tree := NewCDMerkleTree(leaves)
	if bptDebug {
		tree.PrintTree()
	}
	segmentLen := len(segments)
	treeLen := tree.Length()
	t.Logf("Input segments Len: %v (Padded CDT length: %v)\n", segmentLen, treeLen)

	// Test justification for each leaf
	for i, leaf := range leaves {
		justification, err := tree.GenerateCDTJustificationX(i, 0)
		if err != nil {
			t.Fatalf("Error justifying leaf %d: %v", i, err)
		}
		leafHash := computeLeaf(leaf)
		computedRoot := VerifyCDTJustificationX(leafHash, i, justification, 0)

		if !compareBytes(computedRoot, tree.Root()) {
			t.Errorf("Justification failed for leaf %d: expected root %s, got %s", i, common.Bytes2Hex(tree.Root()), common.Bytes2Hex(computedRoot))
		} else {
			//t.Logf("CDT Justification verified for leaf %d", i)
		}
	}
}

func testVerifyCDTJustificationX(t *testing.T, numSegments int) {
	if numSegments == 0 {
		return
	}
	var segments [][]byte
	for i := 0; i < numSegments; i++ {
		data := []byte(fmt.Sprintf("segment%d", i))
		data = common.PadToMultipleOfN(data, types.W_E*types.W_S)
		segments = append(segments, data)
	}
	leaves := segments
	// Build Merkle Tree
	tree := NewCDMerkleTree(leaves)

	segmentLen := len(segments)
	treeLen := tree.Length()
	t.Logf("Input segments Len: %v (Padded CDT length: %v)\n", segmentLen, treeLen)

	// Index of the leaf to justify
	index := 0
	leafHash := computeLeaf(segments[index])

	// Get justification for the leaf
	x := 2 // The depth of the tree or less
	justification, err := tree.GenerateCDTJustificationX(index, x)
	if err != nil {
		t.Fatalf("GenerateCDTJustificationX failed: %v", err)
	}

	// Verify the justification
	computedRoot := verifyCDTJustificationX(leafHash, index, justification, x)
	expectedRoot, _ := tree.LocalRootX(index, x)

	if !compareBytes(computedRoot, expectedRoot) {
		t.Errorf("Root hash mismatch: expected %x, got %x", expectedRoot, computedRoot)
	}
	t.Logf("Root hash verified: %x\n", computedRoot)
}

func testCDTGet(t *testing.T, numSegments int) {

	// Initialize segments
	var segments [][]byte
	for i := 0; i < numSegments; i++ {
		data := []byte(fmt.Sprintf("segment%d", i))
		data = common.PadToMultipleOfN(data, types.W_E*types.W_S)
		segments = append(segments, data)
	}
	leaves := segments
	// Build Merkle Tree
	tree := NewCDMerkleTree(leaves)
	if bptDebug {
		tree.PrintTree()
	}

	segmentLen := len(segments)
	treeLen := tree.Length()
	t.Logf("Input segments Len: %v (Padded CDT length: %v)\n", segmentLen, treeLen)

	for i := 0; i < segmentLen; i++ {
		_, err := tree.Get(i)
		if err != nil {
			t.Errorf("Getting %d but got Error: %v\n", i, err)
		}
	}

	leaf, err := tree.Get(treeLen)
	if err != nil {
		// t.Logf("Error: %v\n", err)
	} else {
		t.Errorf("Get %d should be Error: %v but got %v\n", treeLen, err, leaf)
	}
}

func TestP(t *testing.T) {
	testPageProof(t, 127)
}

func testPageProof(t *testing.T, numSegments int) {
	var segments [][]byte
	// numSegments must be power of 2
	var TestingSegmentsNums = []int{numSegments}
	// var TestingSegmentsNums = []int{1, 2}

	// Initialize segments
	for _, segmentsN := range TestingSegmentsNums {
		if segmentsN == 0 {
			continue
		}
		if bptDebug {
			fmt.Printf("-------------------%d-------------------\n", segmentsN)
		}
		segments = nil
		for i := 0; i < segmentsN; i++ {
			data := []byte(fmt.Sprintf("segment%d", i))
			data = common.PadToMultipleOfN(data, types.W_E*types.W_S)
			segments = append(segments, data)
		}
		// Generate the paged proofs
		pagedProofs, err := generatePageProof(segments)
		if err != nil {
			t.Fatalf("Failed to generate page proofs: %v", err)
		}

		if len(pagedProofs) == 0 {
			t.Fatalf("Expected non-empty page proofs")
		}

		tree := NewCDMerkleTree(segments)
		leaves := make([][]byte, len(tree.leaves))
		for i, leaf := range tree.leaves {
			leaves[i] = leaf.Value
		}
		// Print the Merkle Root of each page
		for i, proof := range pagedProofs {
			// Decode the proof back to segments and verify
			decodedData, _, err := types.Decode(proof, reflect.TypeOf([][]uint8{}))
			if err != nil {
				t.Fatalf("Failed to decode page proof: %v", err)
			}
			decodedSegmentHashes := decodedData.([][]byte)

			// Compare decoded segments with original
			start := i * 64
			end := start + 64
			if end > len(leaves) {
				end = len(leaves)
			}
			pageSegments := leaves[start:end]
			var position int

			expectedSegments := pageSegments
			traces, decodedSegmentHashes := splitPageProof(decodedSegmentHashes)
			// decodedSegmentHashes = decodedSegmentHashes[position:]
			expectedSegmentHashes := make([][]byte, len(expectedSegments))

			for j, segment := range expectedSegments {
				expectedSegmentHashes[j] = computeLeaf(segment)
			}
			for j := range expectedSegmentHashes {
				if !compareBytes(expectedSegmentHashes[j], decodedSegmentHashes[j]) {
					t.Errorf("(%d) Segment mismatch: expected %x, got %x", j, expectedSegmentHashes[j], decodedSegmentHashes[j])
				}
			}
			computePageProofHashes(decodedSegmentHashes)
		}
	}
}

func NextPowerOf2(n int) int {

	if n <= 1 {
		return 1
	}
	return 1 << bitsLen(n)
}

func bitsLen(n int) int {
	return int(math.Log2(float64(n))) + 1
}
