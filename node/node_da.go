package node

import (
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func compareWorkPackages(wp1, wp2 types.WorkPackage) bool {
	// Compare Authorization
	if !common.CompareBytes(wp1.Authorization, wp2.Authorization) {
		fmt.Printf("Authorization mismatch (%x, %x)\n", wp1.Authorization, wp2.Authorization)
		fmt.Println("Authorization mismatch")
		return false
	}

	// Compare AuthCodeHost
	if wp1.AuthCodeHost != wp2.AuthCodeHost {
		return false
	}

	// Compare Authorizer struct
	if !common.CompareBytes(wp1.Authorizer.CodeHash[:], wp2.Authorizer.CodeHash[:]) {
		return false
	}
	if !common.CompareBytes(wp1.Authorizer.Params, wp2.Authorizer.Params) {
		return false
	}

	// Compare RefineContext struct
	if !common.CompareBytes(wp1.RefineContext.Anchor[:], wp2.RefineContext.Anchor[:]) {
		return false
	}
	if !common.CompareBytes(wp1.RefineContext.StateRoot[:], wp2.RefineContext.StateRoot[:]) {
		return false
	}
	if !common.CompareBytes(wp1.RefineContext.BeefyRoot[:], wp2.RefineContext.BeefyRoot[:]) {
		return false
	}
	if !common.CompareBytes(wp1.RefineContext.LookupAnchor[:], wp2.RefineContext.LookupAnchor[:]) {
		return false
	}
	if wp1.RefineContext.LookupAnchorSlot != wp2.RefineContext.LookupAnchorSlot {
		return false
	}

	// Compare WorkItems
	if len(wp1.WorkItems) != len(wp2.WorkItems) {
		return false
	}
	for i := range wp1.WorkItems {
		if wp1.WorkItems[i].CodeHash != wp2.WorkItems[i].CodeHash {
			return false
		}
	}
	return true
}

func (n *Node) NewAvailabilitySpecifier(packageHash common.Hash, workPackage types.WorkPackage, segments [][]byte) *types.AvailabilitySpecifier {
	// Compute b using EncodeWorkPackage
	b := n.encodeWorkPackage(workPackage)
	p := n.decodeWorkPackage(b)
	if compareWorkPackages(workPackage, p) {
		fmt.Println("----------Original WorkPackage and Decoded WorkPackage are the same-------")
	} else {
		fmt.Println("----------Original WorkPackage and Decoded WorkPackage are different-------")
	}

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
func (n *Node) encodeWorkPackage(wp types.WorkPackage) []byte {
	fmt.Println("encodeWorkPackage")
	output := make([]byte, 0)
	// 1. Encode the package (p)
	fmt.Println("wp:", wp)
	encodedPackage := types.Encode(wp)
	output = append(output, encodedPackage...)

	// 2. Encode the extrinsic (x)
	x := wp.WorkItems
	extrinsics := make([][]byte, 0)
	for _, WorkItem := range x {
		extrinsics = append(extrinsics, WorkItem.ExtrinsicsBlobs...)
	}
	fmt.Println("extrinsics:", extrinsics)
	encodedExtrinsic := types.Encode(extrinsics)
	output = append(output, encodedExtrinsic...)

	// 3. Encode the segments (i)
	var encodedSegments []byte
	var segments [][]byte
	for _, WorkItem := range x {
		segments, _ = n.getImportSegments(WorkItem.ImportedSegments)
	}
	fmt.Println("segments:", segments)
	encodedSegments = types.Encode(segments)
	output = append(output, encodedSegments...)

	// 4. Encode the justifications (j)
	var encodedJustifications []byte
	var justification [][]byte
	for _, WorkItem := range x {
		tree := trie.NewCDMerkleTree(segments)
		for i, _ := range WorkItem.ImportedSegments {
			justifies, _ := tree.Justify(i)
			var tmpJustification [][]byte
			for _, justify := range justifies {
				tmpJustification = append(tmpJustification, justify)
			}
			justification = append(justification, tmpJustification...)
		}
	}
	encodedJustifications = append(encodedJustifications, types.Encode(justification)...)

	output = append(output, encodedJustifications...)

	// Combine all encoded parts: e(p,x,i,j)
	return output
}

func (n *Node) decodeWorkPackage(encodedWorkPackage []byte) types.WorkPackage {
	fmt.Println("decodeWorkPackage")
	decodedPackage := types.WorkPackage{}
	// length := uint32(0)

	// // Decode the package (p)
	// wp, l := types.Decode(encodedWorkPackage, reflect.TypeOf(types.WorkPackage{}))
	wp, _ := types.Decode(encodedWorkPackage, reflect.TypeOf(types.WorkPackage{}))
	fmt.Println("wp:", wp)
	decodedPackage = wp.(types.WorkPackage)
	// length += l

	// // Decode the extrinsic (x)
	// extrinsics, l := types.Decode(encodedWorkPackage[length:], reflect.TypeOf([][]byte{}))
	// fmt.Println("extrinsics:", extrinsics)
	// decodedPackage.WorkItems = make([]types.WorkItem, 0)
	// for _, extrinsic := range extrinsics.([][]byte) {
	// 	decodedPackage.WorkItems = append(decodedPackage.WorkItems, types.WorkItem{
	// 		ExtrinsicsBlobs: [][]byte{extrinsic},
	// 	})
	// }
	// length += l

	// // Decode the segments (i)
	// segments, l := types.Decode(encodedWorkPackage[length:], reflect.TypeOf([][]byte{}))
	// fmt.Println("segments:", segments)
	// // setImportSegments
	// length += l

	// // Decode the justifications (j)
	// justifications, l := types.Decode(encodedWorkPackage[length:], reflect.TypeOf([][]byte{}))
	// fmt.Println("justifications:", justifications)
	// // setJustifications
	// length += l

	return decodedPackage
	// return decodedPackage, decodedPackage.WorkItems, segments,justifications
	/* return four item */
}

func (n *Node) VerifyWorkPackage(wp types.WorkPackage, decodedB [][]byte) []byte {
	output := make([]byte, 0)
	// 1. Encode the package (p)
	encodedPackage := types.Encode(wp)
	output = append(output, encodedPackage...)

	// 2. Encode the extrinsic (x)
	x := wp.WorkItems
	encodedExtrinsic := make([]byte, 0)
	for _, WorkItem := range x {
		encodedExtrinsic = types.Encode(WorkItem.Extrinsics)
	}
	output = append(output, encodedExtrinsic...)

	// 3. Encode the segments (i)
	var encodedSegments []byte
	var segments [][]byte
	for _, WorkItem := range x {
		segments, _ = n.getImportSegments(WorkItem.ImportedSegments)
	}
	encodedSegments = types.Encode(segments)
	output = append(output, encodedSegments...)

	// 4. Encode the justifications (j)
	var encodedJustifications []byte
	for _, WorkItem := range x {
		var justification []common.Hash
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
	output = append(output, encodedJustifications...)

	// Combine all encoded parts: e(p,x,i,j)
	return output
}
