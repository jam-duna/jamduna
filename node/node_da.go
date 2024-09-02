package node

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"math"
)

func NewAvailabilitySpecifier(packageHash common.Hash, workPackage types.ASWorkPackage, segments []types.Segment) *types.AvailabilitySpecifier {
	// Compute b using EncodeWorkPackage
	b := encodeWorkPackage(workPackage)

	// Length of `b`
	bLength := uint32(len(b))

	// Compute b♣ and s♣
	bClub := computeBClub(b)
	sClub := computeSClub(segments)

	// u = (bClub, sClub)
	u := generateU(bClub, sClub)

	// Return the Availability Specifier
	return &types.AvailabilitySpecifier{
		WorkPackageHash:  packageHash,
		BundleLength:     bLength,
		ErasureRoot:      u,
		ExportedSegments: sClub,
		// TODO: SegmentRoot:
	}
}

// Compute b♣ using the EncodeWorkPackage function
func computeBClub(b []byte) []common.Hash {
	// Padding b to the length of W_C
	paddedB := padToLength(b, types.W_C)

	// Process the padded data using erasure coding
	encodedB, _ := erasurecoding.Encode(paddedB, int(math.Ceil(float64(len(b))/float64(types.W_C))))

	// Hash each element of the encoded data
	var bClub []common.Hash
	for _, block := range encodedB {
		for _, b := range block {
			bClub = append(bClub, common.Hash(padToLength(b, 32)))
		}
	}

	return bClub
}

func computeSClub(segments []types.Segment) []common.Hash {
	var combinedData [][][]byte

	pageProofs := trie.GeneratePageProof(segments)
	for _, segment := range segments {
		// Generate PageProofs for the segment, and combine them with the segment data

		combinedSegment := segment.Data
		for _, pageProof := range pageProofs {
			combinedSegment = append(combinedSegment, pageProof.ToBytes()...)
		}

		// Erasure Coding
		encodedSegment, _ := erasurecoding.Encode(combinedSegment, 6)

		// Append the encoded segment to the combined data
		combinedData = append(combinedData, encodedSegment...)
	}

	// Transpose the combined data
	transposedData := transpose3D(combinedData)
	var sClub []common.Hash
	for _, data := range transposedData {
		root := trie.NewWellBalancedTree(data).Root()
		sClub = append(sClub, common.Hash(root))
	}

	return sClub
}

// Pad the data to the specified length
func padToLength(data []byte, length int) []byte {
	padded := make([]byte, length)
	copy(padded, data)
	return padded
}

// Encode b♣ and s♣ into a matrix
func generateU(b []common.Hash, s []common.Hash) common.Hash {
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

// Gererate the root of the WBT from the segments
func generateSegmentsRoot(segments []types.Segment) common.Hash {
	var segmentData [][]byte
	for _, segment := range segments {
		segmentData = append(segmentData, segment.Data)
	}

	wbt := trie.NewWellBalancedTree(segmentData)
	return common.Hash(wbt.Root())
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

// The E(p,x,i,j) function is a function that takes a package and its segments and returns a result, in EQ(186)
func encodeWorkPackage(wp types.ASWorkPackage) []byte {
	// 1. Encode the package (p)
	encodedPackage := types.Encode(wp)

	// 2. Encode the extrinsic (x)
	encodedExtrinsic := types.Encode(wp.Extrinsic)

	// 3. Encode the segments (i)
	var encodedSegments []byte
	for _, segment := range wp.ImportSegments {
		encodedSegments = append(encodedSegments, types.Encode(segment)...)
	}

	// 4. Encode the justifications (j)
	var encodedJustifications []byte
	for i, segment := range wp.ImportSegments {
		byteSlices := segment.ToByteSlices()
		tree := trie.NewCDMerkleTree(byteSlices)
		justification, _ := tree.Justify(i)
		encodedJustifications = append(encodedJustifications, types.Encode(justification)...)
	}
	// Combine all encoded parts: e(p,x,i,j)
	return append(append(append(encodedPackage, encodedExtrinsic...), encodedSegments...), encodedJustifications...)
}
