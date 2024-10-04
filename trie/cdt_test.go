package trie

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

var TestingSegmentsNum = 31

func TestCDTGet(t *testing.T) {
	testCDTGet(t, TestingSegmentsNum)
}

func TestJustify(t *testing.T) {
	testJustify(t, TestingSegmentsNum)
}

func TestVerifyJustifyX(t *testing.T) {
	testVerifyJustifyX(t, TestingSegmentsNum)
}

func TestPageProof(t *testing.T) {
	testPageProof(t, TestingSegmentsNum)
}

func TestCDT(t *testing.T) {
	var TestingSegmentsNums = []int{1, 2, 3, 5, 10, 31, 32, 33, 64, 65, 128}
	// var TestingSegmentsNums []int
	// for i := 1; i <= 513; i++ {
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
				testJustify(t, numSegments)
			})

			t.Run("TestVerifyJustifyX", func(t *testing.T) {
				testVerifyJustifyX(t, numSegments)
			})

			t.Run("testPageProof", func(t *testing.T) {
				testPageProof(t, numSegments)
			})

		})
	}
}

func testJustify(t *testing.T, numSegments int) {
	// Initialize segments
	var segments [][]byte
	for i := 0; i < TestingSegmentsNum; i++ {
		data := []byte(fmt.Sprintf("segment%d", i))
		data = common.PadToMultipleOfN(data, types.W_C*types.W_S)
		segments = append(segments, data)
	}
	leaves := segments
	// Build Merkle Tree
	tree := NewCDMerkleTree(leaves)
	tree.PrintTree()

	segmentLen := len(segments)
	treeLen := tree.Length()
	t.Logf("Input segments Len: %v (Padded CDT length: %v)\n", segmentLen, treeLen)

	// Test justification for each leaf
	for i, leaf := range leaves {
		justification, err := tree.Justify(i)
		if err != nil {
			t.Fatalf("Error justifying leaf %d: %v", i, err)
		}

		leafHash := computeLeaf(leaf)
		computedRoot := verifyJustification(leafHash, i, justification)

		if !compareBytes(computedRoot, tree.Root()) {
			t.Errorf("Justification failed for leaf %d: expected root %s, got %s", i, common.Bytes2Hex(tree.Root()), common.Bytes2Hex(computedRoot))
		} else {
			t.Logf("Justification verified for leaf %d", i)
		}
	}
}

func testVerifyJustifyX(t *testing.T, numSegments int) {

	// Initialize segments
	var segments [][]byte
	for i := 0; i < TestingSegmentsNum; i++ {
		data := []byte(fmt.Sprintf("segment%d", i))
		data = common.PadToMultipleOfN(data, types.W_C*types.W_S)
		segments = append(segments, data)
	}
	leaves := segments
	// Build Merkle Tree
	tree := NewCDMerkleTree(leaves)
	tree.PrintTree()

	segmentLen := len(segments)
	treeLen := tree.Length()
	t.Logf("Input segments Len: %v (Padded CDT length: %v)\n", segmentLen, treeLen)

	// Index of the leaf to justify
	index := 0
	leafHash := computeLeaf(segments[index])

	// Get justification for the leaf
	x := 2 // The depth of the tree or less
	justification, err := tree.JustifyX(index, x)
	if err != nil {
		t.Fatalf("JustifyX failed: %v", err)
	}

	// Verify the justification
	computedRoot := verifyJustifyX(leafHash, index, justification, x)
	expectedRoot := justification[len(justification)-1]

	if !compareBytes(computedRoot, expectedRoot) {
		t.Errorf("Root hash mismatch: expected %x, got %x", expectedRoot, computedRoot)
	} else {
		t.Logf("Root hash verified: %x\n", computedRoot)
	}
}

func testCDTGet(t *testing.T, numSegments int) {

	// Initialize segments
	var segments [][]byte
	for i := 0; i < TestingSegmentsNum; i++ {
		data := []byte(fmt.Sprintf("segment%d", i))
		data = common.PadToMultipleOfN(data, types.W_C*types.W_S)
		segments = append(segments, data)
	}
	leaves := segments
	// Build Merkle Tree
	tree := NewCDMerkleTree(leaves)
	tree.PrintTree()

	segmentLen := len(segments)
	treeLen := tree.Length()
	t.Logf("Input segments Len: %v (Padded CDT length: %v)\n", segmentLen, treeLen)

	for i := 0; i < segmentLen; i++ {
		leaf, err := tree.Get(i)
		if err == nil {
			t.Logf("Get %d: %s\n", i, string(leaf))
		} else {
			t.Errorf("Getting %d but got Error: %v\n", i, err)
		}
	}

	leaf, err := tree.Get(treeLen)
	if err != nil {
		t.Logf("Error: %v\n", err)
	} else {
		t.Errorf("Get %d should be Error: %v but got %v\n", treeLen, err, leaf)
	}
}

func testPageProof(t *testing.T, numSegments int) {
	var segments [][]byte
	var TestingSegmentsNums = []int{1, 2, 3, 5, 10, 31, 32, 33, 64, 65, 128, 2046}
	// var TestingSegmentsNums = []int{1, 2}

	// Initialize segments
	for _, TestingSegmentsNum := range TestingSegmentsNums {
		fmt.Printf("-------------------%d-------------------\n", TestingSegmentsNum)
		segments = nil
		for i := 0; i < TestingSegmentsNum+1; i++ {
			data := []byte(fmt.Sprintf("segment%d", i))
			data = common.PadToMultipleOfN(data, types.W_C*types.W_S)
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
		// Print the Merkle Root of each page
		for i, proof := range pagedProofs {
			// Decode the proof back to segments and verify
			decodedData, _ := types.Decode(proof, reflect.TypeOf([][]uint8{}))
			decodedSegments := decodedData.([][]byte)
			t.Logf("Page %d PageProof: %x\n", i, decodedSegments)
			// Compare decoded segments with original
			start := i * 64
			end := start + 64
			if end > len(segments) {
				end = len(segments)
			}
			var position int
			for _, segment := range segments {
				position = findPositions(decodedSegments, segment)
				if position != -1 {
					break
				}
			}
			if position == -1 {
				t.Fatalf("Segment not found in the decoded segments")
			}

			expectedSegments := segments[start:end]
			expecteddecodedSegments := decodedSegments[position:]
			for j := range expecteddecodedSegments {
				if !compareBytes(expecteddecodedSegments[j], expectedSegments[j]) {
					t.Errorf("Segment mismatch: expected %x, got %x", expectedSegments[j], decodedSegments[j])
				}
			}
			result, err := VerifyPageProof(decodedSegments, i)
			if err != nil {
				t.Errorf("Page %d PageProof Verification failed: %v\n", i, err)
			} else if result {
				t.Logf("Page %d PageProof Verified\n", i)
			} else {
				t.Errorf("Page %d PageProof Verification failed: %v\n", i, err)
			}
		}
	}
}
