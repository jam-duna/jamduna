package trie

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

var TestingSegmentsNum = 31

func generateSegment(idx int) []byte {
	//data := []byte(fmt.Sprintf("segments%d", idx))
	prefix, _ := hex.DecodeString("deadbeef")
	data := make([]byte, 2) // Adjust size if needed.
	binary.LittleEndian.PutUint16(data, uint16(idx))
	data = append(prefix, data...)
	data = common.PadToMultipleOfN(data, types.SegmentSize)
	return data
}

func generateSegmentTree(t *testing.T, numSegments int) (tree *CDMerkleTree, segments [][]byte) {
	// Initialize segments
	for i := 0; i < numSegments; i++ {
		data := generateSegment(i)
		segments = append(segments, data)
	}
	leaves := segments
	// Build Merkle Tree
	tree = NewCDMerkleTree(leaves)
	if debugCDT {
		tree.PrintTree()
	}

	segmentLen := len(segments)
	treeLen := tree.Length()
	if debugCDT {
		t.Logf("Input segments Len: %v (Padded CDT length: %v). Root:%v\n", segmentLen, treeLen, tree.RootHash())
	}

	return tree, segments
}

func TestGetCDT(t *testing.T) {
	cdtTree, segments := generateSegmentTree(t, TestingSegmentsNum)
	testCDTGet(t, cdtTree, segments)
}

func TestJustify0(t *testing.T) {
	cdtTree, segments := generateSegmentTree(t, TestingSegmentsNum)
	testJustify0(t, cdtTree, segments)
}

func TestVerifyCDTJustificationX(t *testing.T) {
	cdtTree, segments := generateSegmentTree(t, TestingSegmentsNum)
	testVerifyCDTJustificationX(t, cdtTree, segments)
}

func TestPageProof(t *testing.T) {
	cdtTree, segments := generateSegmentTree(t, TestingSegmentsNum)
	testPageProof(t, cdtTree, segments)
}

func TestCDT(t *testing.T) {

	var TestingSegmentsNums = []int{0, 1, 2, 3, 5, 10, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1023, 1024, 1025, 2047, 2048, 2049, 3071, 3072}
	//var TestingSegmentsNums = []int{129}
	//var TestingSegmentsNums = []int{3072}

	//var TestingSegmentsNums []int
	for i := 2724; i <= types.MaxImports; i++ {
		//TestingSegmentsNums = append(TestingSegmentsNums, i)
	}
	for _, numSegments := range TestingSegmentsNums {
		t.Run(fmt.Sprintf("SegmentsNum=%d", numSegments), func(t *testing.T) {
			//t.Parallel() // Optionally run in parallel
			cdtTree, segments := generateSegmentTree(t, numSegments)
			t.Run("TestCDTGet", func(t *testing.T) {
				testCDTGet(t, cdtTree, segments)
			})

			t.Run("TestJustify0", func(t *testing.T) {
				testJustify0(t, cdtTree, segments)
			})

			t.Run("TestVerifyCDTJustificationX", func(t *testing.T) {
				testVerifyCDTJustificationX(t, cdtTree, segments)
			})

			t.Run("TestPageProof", func(t *testing.T) {
				testPageProof(t, cdtTree, segments)
			})

		})
	}
}

func testJustify0(t *testing.T, tree *CDMerkleTree, segments [][]byte) {
	leaves := segments
	root := tree.Root()
	// Test justification for each leaf
	for i, leaf := range leaves {
		justification, err := tree.GenerateCDTJustificationX(i, 0)
		if err != nil {
			t.Fatalf("Error justifying leaf %d: %v", i, err)
		}
		leafHash := computeLeaf(leaf)
		computedRoot := VerifyCDTJustificationX(leafHash, i, justification, 0)

		if !compareBytes(computedRoot, root) {
			t.Errorf("Justification failed for leaf %d: expected root %s, got %s", i, common.Bytes2Hex(root), common.Bytes2Hex(computedRoot))
		} else if debugCDT {
			t.Logf("CDT Justification verified for leaf %d: %x, %v", i, root, justification)
		}
	}
}

func testVerifyCDTJustificationX(t *testing.T, tree *CDMerkleTree, segments [][]byte) {
	if (tree == nil) || (len(segments) == 0) {
		return
	}
	segmentLen := len(segments)
	globalRoot := tree.Root()
	if debugCDT {
		t.Logf("segmentLen=%d globalRoot=%x\n", segmentLen, globalRoot)
	}

	for index := 0; index < segmentLen; index++ {
		// Index of the leaf to justify
		pgIdx, isFixedPadding := ComputePageIdx(segmentLen, index)
		leafHash := computeLeaf(segments[index])

		// Get justification for the leaf
		globalJustification, err := tree.GenerateCDTJustificationX(index, PageFixedDepth)
		if debugCDT {
			t.Logf("segmentLen=%d index=%d leafHash: %x\n", segmentLen, index, leafHash)
			t.Logf("globalJustification(len=%d): %v\n", len(globalJustification), globalJustification)
		} else if err != nil {
			t.Logf("segmentLen=%d index=%d leafHash: %x\n", segmentLen, index, leafHash)
			t.Logf("globalJustification(len=%d): %v\n", len(globalJustification), globalJustification)
			t.Fatalf("GenerateCDTJustificationX failed: %v", err)
		}

		fullJustification, err := tree.GenerateCDTJustificationX(index, 0)

		if err != nil {
			t.Fatalf("GenerateLocalJustificationX fullJustification failed: %v", err)
		}
		derived_globalRoot_j0 := VerifyCDTJustificationX(leafHash, index, fullJustification, 0)
		if !compareBytes(derived_globalRoot_j0, globalRoot) {
			t.Logf("fullJustification(len=%d): %v\n", len(fullJustification), fullJustification)
			t.Fatalf("fullJustification Root hash mismatch: expected %x | got %x", globalRoot, derived_globalRoot_j0)
		} else if debugCDT {
			t.Logf("fullJustification(len=%d): %v\n", len(fullJustification), fullJustification)
			t.Logf("OK fullJustification segmentLen=%d index=%d leafHash: %x, Verified\n", segmentLen, index, leafHash)
		}

		localLeaves, err := tree.GenerateLocalLeavesX(pgIdx, PageFixedDepth)
		if err != nil {
			t.Fatalf("GenerateLocalLeavesX failed: %v", err)
		}
		subTree := NewCDTSubtree(localLeaves, isFixedPadding)
		localPageRoot := subTree.Root()

		// Verify the justification
		derived_globalRoot_pg := VerifyCDTJustificationX(localPageRoot, pgIdx, globalJustification, 0)
		if !compareBytes(derived_globalRoot_pg, globalRoot) {
			t.Logf("localLeaves(len=%d): %v\n", len(localLeaves), localLeaves)
			t.Logf("subTree localPageRoot: %x\n", localPageRoot)
			t.Fatalf("Local page Root hash mismatch: expected %x, got %x", globalRoot, derived_globalRoot_pg)
		} else if debugCDT {
			t.Logf("Local page Root segmentLen=%d index=%d leafHash: %x, Verified\n", segmentLen, index, leafHash)
		}
	}

}

func testCDTGet(t *testing.T, tree *CDMerkleTree, segments [][]byte) {
	for i := 0; i < len(segments); i++ {
		segment_data := segments[i]
		recovered_segment_data, err := tree.Get(i)
		if err != nil {
			t.Errorf("Getting %d but got Error: %v\n", i, err)
		} else if !compareBytes(segment_data, recovered_segment_data) {
			t.Errorf("cdtTree Get mismatch %d: %x\n", i, recovered_segment_data)
		} else if len(recovered_segment_data) != types.SegmentSize {
			t.Errorf("segments not padded to %d bytes: %d\n", types.SegmentSize, len(recovered_segment_data))
		}
	}
	treeLen := tree.Length()
	leaf, err := tree.Get(treeLen)
	if err != nil {
		// t.Logf("Error: %v\n", err)
	} else {
		t.Errorf("Get %d should be Error: %v but got %v\n", treeLen, err, leaf)
	}
}

func testPageProof(t *testing.T, tree *CDMerkleTree, segments [][]byte) {
	exportedSegmentLen := len(segments)
	if exportedSegmentLen == 0 {
		return
	}
	//debugBPT := true
	if debugBPT {
		fmt.Printf("-------------------%d-------------------\n", exportedSegmentLen)
	}

	globalRoot := tree.Root()

	// Generate the paged proofs
	pagedProofs, err := GeneratePageProof(segments)
	pageSize := 1 << PageFixedDepth
	numPages := (len(segments) + pageSize - 1) / pageSize

	if debugBPT {
		fmt.Printf("SegmentN=%d, GeneratePageProof: numPages=%d, pageSize=%d\n", len(segments), numPages, pageSize)
		for pgIdx, proofByte := range pagedProofs {
			fmt.Printf("!!pagedProofs[%d] bytesize:%d, %x\n", pgIdx, len(proofByte), proofByte)
		}
	}

	if err != nil {
		t.Fatalf("Failed to generate page proofs: %v", err)
	}

	if len(pagedProofs) == 0 {
		t.Fatalf("Expected non-empty page proofs")
	}

	leaves := make([][]byte, len(tree.leaves))
	for i, leaf := range tree.leaves {
		leaves[i] = leaf.Value
	}
	// Print the Merkle Root of each page
	for pageIdx, pagedProofByte := range pagedProofs {
		// Decode the proof back to segments and verify
		decodedData, _, err := types.Decode(pagedProofByte, reflect.TypeOf(types.PageProof{}))
		if err != nil {
			t.Fatalf("Failed to decode page proof: %v", err)
		}
		recoveredPageProof := decodedData.(types.PageProof)
		for subTreeIdx := 0; subTreeIdx < len(recoveredPageProof.LeafHashes); subTreeIdx++ {
			leafHash := recoveredPageProof.LeafHashes[subTreeIdx]
			pageSize := 1 << PageFixedDepth
			index := pageIdx*pageSize + subTreeIdx
			fullJustification, err := PageProofToFullJustification(pagedProofByte, pageIdx, subTreeIdx)
			if err != nil {
				t.Fatalf("fullJustification len: %d, PageProofToFullJustification ERR: %v.", len(fullJustification), err)
			} else if debugCDT {
				t.Logf("fullJustification len: %d, fullJustification %v", len(fullJustification), fullJustification)
			}
			derived_globalRoot_j0 := VerifyCDTJustificationX(leafHash.Bytes(), index, fullJustification, 0)
			if !compareBytes(derived_globalRoot_j0, globalRoot) {
				t.Fatalf("fullJustification Root hash mismatch: expected %x, got %x", globalRoot, derived_globalRoot_j0)
			}
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
