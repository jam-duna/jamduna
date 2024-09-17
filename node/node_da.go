package node

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) NewAvailabilitySpecifier(packageHash common.Hash, workPackage types.WorkPackage, segments [][]byte) *types.AvailabilitySpecifier {
	// Compute b using EncodeWorkPackage
	b := encodeWorkPackage(workPackage)

	// Length of `b`
	bLength := uint32(len(b))

	// Compute b♣ and s♣
	bClub := n.computeBClub(b)
	sClub := n.computeSClub(segments)

	// u = (bClub, sClub)
	erasure_root_u := generateErasureRoot(bClub, sClub)

	// ExportedSegmentRoot = CDT(segments)
	exported_segment_root_e := generateSegmentsRoot(segments)

	// Return the Availability Specifier
	availabilitySpecifier := types.AvailabilitySpecifier{
		WorkPackageHash:     packageHash,
		BundleLength:        bLength,
		ErasureRoot:         erasure_root_u,
		ExportedSegmentRoot: exported_segment_root_e,
	}
	return &availabilitySpecifier
}

// Compute b♣ using the EncodeWorkPackage function
func (n *Node) computeBClub(b []byte) []common.Hash {
	// Padding b to the length of W_C
	paddedB := common.PadToMultipleOfN(b, types.W_C)

	// Process the padded data using erasure coding
	c_base := types.ComputeC_Base(len(b))
	encodedB, _ := erasurecoding.Encode(paddedB, c_base)

	// Hash each element of the encoded data
	var bClub []common.Hash
	for _, block := range encodedB {
		for _, b := range block {
			bClub = append(bClub, common.Hash(common.PadToMultipleOfN(b, 32)))
		}
	}

	return bClub
}

func (n *Node) computeSClub(segments [][]byte) []common.Hash {
	var combinedData [][][]byte

	pageProofs, _ := trie.GeneratePageProof(segments)
	combinedSegmentAndPageProofs := append(segments, pageProofs...)

	// Flatten the combined data
	var FlattenData []byte
	for _, data := range combinedSegmentAndPageProofs {
		FlattenData = append(FlattenData, data...)
	}

	// Erasure code the combined data
	encodedSegment, _ := erasurecoding.Encode(FlattenData, 6)

	// Append the encoded segment to the combined data
	combinedData = append(combinedData, encodedSegment...)

	// Transpose the combined data
	transposedData := transpose3D(combinedData)
	var sClub []common.Hash
	for _, data := range transposedData {
		root := trie.NewWellBalancedTree(data).Root()
		sClub = append(sClub, common.Hash(root))
	}

	return sClub
}

// MB([x∣x∈T[b♣,s♣]]) - Encode b♣ and s♣ into a matrix
func generateErasureRoot(b []common.Hash, s []common.Hash) common.Hash {
	// Combine b♣ and s♣ into a matrix and transpose it
	var combined [][]byte

	// Transpose the b♣ array
	for _, hash := range b {
		combined = append(combined, hash[:])
	}

	// Append the s♣ array
	for _, segment := range s {
		combined = append(combined, segment[:])
	}

	transposedMatrix := transpose(combined)

	// Compute x̂ for each x in the transposed matrix
	var hashedElements [][]byte
	for _, x := range transposedMatrix {
		hashedX := common.ComputeHash(x)
		hashedElements = append(hashedElements, hashedX[:])
	}

	// Generate WBT from the hashed elements and return the root (u)
	wbt := trie.NewWellBalancedTree(hashedElements)
	return common.Hash(wbt.Root())
}

// M(s) - TODO: Stanley please check, should be CDT here. Not WBT
func generateSegmentsRoot(segments [][]byte) common.Hash {
	var segmentData [][]byte
	for _, segment := range segments {
		segmentData = append(segmentData, segment)
	}

	cdt := trie.NewCDMerkleTree(segmentData)
	return common.Hash(cdt.Root())
}

// Validate the availability specifier
func (n *Node) IsValidAvailabilitySpecifier(bClubBlobHash common.Hash, bLength int, sClubBlobHash common.Hash, originalAS *types.AvailabilitySpecifier) (bool, error) {
	// Fetch and reconstruct the data for b♣ and s♣
	bClubData, err := n.FetchAndReconstructArbitraryData(bClubBlobHash, bLength)
	if err != nil {
		return false, err
	}

	sClubData, err := n.FetchAndReconstructAllSegmentsData(sClubBlobHash)
	if err != nil {
		return false, err
	}

	// Recalculate the AvailabilitySpecifier
	reconstructbClub := n.recomputeBClub(bClubData)
	reconstructsClub := n.recomputeSClub(sClubData)
	erasureRoot_u := generateErasureRoot(reconstructbClub, reconstructsClub)

	recalculatedAS := &types.AvailabilitySpecifier{
		WorkPackageHash:     originalAS.WorkPackageHash,
		BundleLength:        originalAS.BundleLength,
		ErasureRoot:         erasureRoot_u,
		ExportedSegmentRoot: originalAS.ExportedSegmentRoot,
	}

	// compare the recalculated AvailabilitySpecifier with the original
	if originalAS.ErasureRoot != recalculatedAS.ErasureRoot {
		fmt.Printf("ErasureRoot mismatch (%x, %x)\n", originalAS.ErasureRoot, recalculatedAS.ErasureRoot)
		return false, nil
	}
	return true, nil
}

// Recompute b♣ using the EncodeWorkPackage function
func (n *Node) recomputeBClub(paddedB []byte) []common.Hash {
	c_base := types.ComputeC_Base(len(paddedB))
	encodedB, _ := erasurecoding.Encode(paddedB, c_base)

	// Hash each element of the encoded data
	var bClub []common.Hash
	for _, block := range encodedB {
		for _, b := range block {
			bClub = append(bClub, common.Hash(common.PadToMultipleOfN(b, 32)))
		}
	}

	return bClub
}

func (n *Node) recomputeSClub(combinedSegment [][]byte) []common.Hash {
	var combinedData [][][]byte

	// Flatten the combined data
	var FlattenData []byte
	for _, data := range combinedSegment {
		FlattenData = append(FlattenData, data...)
	}
	encodedSegment, _ := erasurecoding.Encode(FlattenData, 6)

	// Append the encoded segment to the combined data
	combinedData = append(combinedData, encodedSegment...)

	// Transpose the combined data
	transposedData := transpose3D(combinedData)
	var sClub []common.Hash
	for _, data := range transposedData {
		root := trie.NewWellBalancedTree(data).Root()
		sClub = append(sClub, common.Hash(root))
	}

	return sClub
}

// Helper function to transpose a matrix
func transpose(matrix [][]byte) [][]byte {
	if len(matrix) == 0 {
		return nil
	}

	transposed := make([][]byte, len(matrix[0]))
	for i := range transposed {
		transposed[i] = make([]byte, len(matrix))
		for j := range matrix {
			transposed[i][j] = matrix[j][i]
		}
	}
	return transposed
}

func transpose3D(data [][][]byte) [][][]byte {
	if len(data) == 0 || len(data[0]) == 0 {
		return [][][]byte{}
	}

	rowCount := len(data[0])
	colCount := len(data[0][0])

	// Building a new 3D array to store the transposed data
	transposed := make([][][]byte, len(data))
	for i := range transposed {
		transposed[i] = make([][]byte, colCount)
		for j := range transposed[i] {
			transposed[i][j] = make([]byte, rowCount)
		}
	}

	// Transposing the data
	for i := 0; i < len(data); i++ {
		for j := 0; j < rowCount; j++ {
			for k := 0; k < colCount; k++ {
				transposed[i][k][j] = data[i][j][k]
			}
		}
	}

	return transposed
}

// TODO: Sean to encode & decode properly
// The E(p,x,i,j) function is a function that takes a package and its segments and returns a result, in EQ(186)
func encodeWorkPackage(wp types.WorkPackage) []byte {
	// 1. Encode the package (p)
	encodedPackage := types.Encode(wp)

	// 2. Encode the extrinsic (x)
	x := wp.WorkItems
	encodedExtrinsic := make([]byte, 0)
	for _, WorkItem := range x {
		encodedExtrinsic = types.Encode(WorkItem.Extrinsics)
	}
	// 3. Encode the segments (i)
	var encodedSegments []byte
	for _, WorkItem := range x {
		for _, segment := range WorkItem.ImportedSegments {
			encodedSegments = append(encodedSegments, types.Encode(segment)...)
		}
	}

	// 4. Encode the justifications (j)
	var encodedJustifications []byte
	for _, WorkItem := range x {
		var justification []common.Hash
		var segments [][]byte
		for _, segment := range WorkItem.ImportedSegments {
			segments = append(segments, segment.TreeRoot[:])
		}
		tree := trie.NewCDMerkleTree(segments)
		for i, _ := range WorkItem.ImportedSegments {
			justifies, _ := tree.Justify(i)
			var tmpJustification []common.Hash
			for _, justify := range justifies {
				tmpJustification = append(tmpJustification, common.Hash(justify))
			}
			justification = append(justification, tmpJustification...)
		}
		encodedJustifications = append(encodedJustifications, types.Encode(justification)...)
	}
	// Combine all encoded parts: e(p,x,i,j)
	return append(append(append(encodedPackage, encodedExtrinsic...), encodedSegments...), encodedJustifications...)
}
