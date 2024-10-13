package node

import (
	"fmt"
	"reflect"

	"encoding/binary"

	"github.com/colorfulnotion/jam/pvm"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) NewAvailabilitySpecifier(packageHash common.Hash, workPackage types.WorkPackage, segments [][]byte) *types.AvailabilitySpecifier {
	// Compute b using EncodeWorkPackage
	b := n.encodeWorkPackage(workPackage)
	fmt.Printf("packageHash=%v, encodedPackage(Len=%v):%x\n", packageHash, len(b), b)
	fmt.Printf("raw=%v\n", workPackage.String())
	p := n.decodeWorkPackage(b)
	if common.CompareBytes(workPackage.Bytes(), p.Bytes()) {
		fmt.Printf("----------Original WorkPackage and Decoded WorkPackage are the same-------\n")
	} else {
		fmt.Printf("----------Original WorkPackage and Decoded WorkPackage are different-------\n")
	}

	// Length of `b`
	bLength := uint32(len(b))

	// Compute b♣ and s♣
	blobRoot, bClub := n.computeAndDistributeBClub(b)
	treeRoot, sClub := n.computeAndDistributeSClub(segments)

	// u = (bClub, sClub)
	erasure_root_u := n.generateErasureRoot(bClub, sClub, blobRoot, treeRoot)

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
func (n *Node) computeAndDistributeBClub(b []byte) (common.Hash, [][]common.Hash) {
	// Padding b to the length of W_C
	paddedB := common.PadToMultipleOfN(b, types.W_C)
	bLength := len(b)

	chunks, err := n.encode(paddedB, false, bLength)
	if err != nil {
		fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
	}

	blobHash := common.Blake2Hash(paddedB)

	err = n.DistributeArbitraryData(chunks, blobHash, bLength)

	if err != nil {
		fmt.Println("Error in DistributeSegmentData:", err)
	}

	// Hash each element of the encoded data
	var tmpbClub []common.Hash
	var bClub [][]common.Hash

	for _, block := range chunks {
		for _, b := range block {
			tmpbClub = append(tmpbClub, common.Hash(common.PadToMultipleOfN(b, 32)))
		}
		bClub = append(bClub, tmpbClub)
	}

	return blobHash, bClub
}

func (n *Node) computeAndDistributeSClub(segments [][]byte) (common.Hash, [][]common.Hash) {
	var combinedData [][][]byte

	pageProofs, _ := trie.GeneratePageProof(segments)
	combinedSegmentAndPageProofs := append(segments, pageProofs...)

	// Encode the combined data
	basicTree := trie.NewCDMerkleTree(combinedSegmentAndPageProofs)
	treeRoot := common.Hash(basicTree.Root())
	var segmentsECRoots []byte
	// Flatten the combined data
	var FlattenData []byte
	for _, singleData := range combinedSegmentAndPageProofs {
		FlattenData = append(FlattenData, singleData...)
	}
	// Erasure code the combined data
	encodedSegment, _ := erasurecoding.Encode(FlattenData, 6)
	for _, singleData := range combinedSegmentAndPageProofs {
		// Encode the data into segments
		erasureCodingSegments, err := n.encode(singleData, true, len(singleData)) // Set to false for variable size segments
		if err != nil {
			fmt.Printf("Error in EncodeAndDistributeSegmentData: %v\n", err)
		}

		// Build segment roots
		segmentRoots := make([][]byte, 0)
		for i := range erasureCodingSegments {
			leaves := erasureCodingSegments[i]
			tree := trie.NewCDMerkleTree(leaves)
			segmentRoots = append(segmentRoots, tree.Root())
		}

		// Generate the blob hash by hashing the original data
		blobTree := trie.NewCDMerkleTree(segmentRoots)
		segmentsECRoot := blobTree.Root()

		// Append the segment root to the list of segment roots
		segmentsECRoots = append(segmentsECRoots, segmentsECRoot...)
		// Distribute the segments
		err = n.DistributeSegmentData(erasureCodingSegments, segmentRoots, len(FlattenData))
		if err != nil {
			fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
		}
	}

	//STANLEY TODO: this has to be part of metadata
	n.FakeWriteKV(treeRoot, segmentsECRoots)

	fmt.Printf("treeRoot: %x\n", treeRoot[:])
	// Append the encoded segment to the combined data
	combinedData = append(combinedData, encodedSegment...)
	// fmt.Printf("Before Transpose Size: %d, %d, %d\n", len(combinedData), len(combinedData[0]), len(combinedData[0][0]))

	// Transpose the combined data
	transposedData := transpose3D(combinedData)
	// fmt.Printf("After Transpose Size: %d, %d, %d\n", len(transposedData), len(transposedData[0]), len(transposedData[0][0]))

	var sClub [][]common.Hash
	var tmpsClub []common.Hash
	for _, data := range transposedData {
		root := trie.NewWellBalancedTree(data).Root()
		tmpsClub = append(tmpsClub, common.Hash(root))
	}
	sClub = append(sClub, tmpsClub)

	return treeRoot, sClub
}

// MB([x∣x∈T[b♣,s♣]]) - Encode b♣ and s♣ into a matrix
func (n *Node) generateErasureRoot(b [][]common.Hash, s [][]common.Hash, blobHash common.Hash, treeRoot common.Hash) common.Hash {
	// Combine b♣ and s♣ into a matrix and transpose it

	transposedB := transposeHash(b)
	transposedS := transposeHash(s)

	combined := make([][][]byte, len(transposedB))
	// Transpose the b♣ array and s♣ array
	for i := range transposedB {
		for j := range transposedB[i] {
			combined[i] = append(combined[i], transposedB[i][j][:])
		}
		for j := range transposedS[i] {
			combined[i] = append(combined[i], transposedS[i][j][:])
		}
	}

	// Compute x̂ for each x in the transposed matrix
	var hashedElements [][]byte
	for _, x := range combined {
		hashedElements = append(hashedElements, x...)
	}

	var flattenHashedElements [][]byte
	for _, element := range hashedElements {
		flattenHashedElements = append(flattenHashedElements, element[:])
	}

	// Generate WBT from the hashed elements and return the root (u)
	wbt := trie.NewWellBalancedTree(flattenHashedElements)
	erasureRoot := common.Hash(wbt.Root())
	fmt.Printf("Len(blobHash), blobHash: %d, %x\n", len(blobHash), blobHash[:])
	fmt.Printf("Len(treeRoot), treeRoot: %d, %x\n", len(treeRoot), treeRoot[:])

	//STANLEY TODO: this has to be part of metadata
	n.FakeWriteKV(erasureRoot, append(blobHash[:], treeRoot[:]...))

	fmt.Printf("Len(ErasureRoot), ErasureRoot: %d, %v\n", len(erasureRoot), erasureRoot)
	return erasureRoot
}

// Transpose the list of common.Hash into a list of byte slices
func transposeHash(matrix [][]common.Hash) [][]common.Hash {
	if len(matrix) == 0 || len(matrix[0]) == 0 {
		return nil
	}

	// Transposed matrix initialization
	transposed := make([][]common.Hash, len(matrix[0]))
	for i := range transposed {
		transposed[i] = make([]common.Hash, len(matrix))
	}

	// Transposing the data
	for i := 0; i < len(matrix[0]); i++ {
		for j := 0; j < len(matrix); j++ {
			transposed[i][j] = matrix[j][i]
		}
	}

	return transposed
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

	sClubData, _, _, err := n.FetchAndReconstructAllSegmentsData(sClubBlobHash)
	if err != nil {
		return false, err
	}

	// Recalculate the AvailabilitySpecifier
	reconstructbClub := n.recomputeBClub(bClubData)
	reconstructsClub := n.recomputeSClub(sClubData)

	//bClub, sClub, blobRoot, treeRoot
	erasureRoot_u := n.generateErasureRoot(reconstructbClub, reconstructsClub, bClubBlobHash, sClubBlobHash)

	// compare the recalculated AvailabilitySpecifier with the original
	if originalAS.ErasureRoot != erasureRoot_u {
		fmt.Printf("ErasureRoot mismatch (%x, %x)\n", originalAS.ErasureRoot, erasureRoot_u)
		return false, nil
	}
	return true, nil
}

// Recompute b♣ using the EncodeWorkPackage function
func (n *Node) recomputeBClub(paddedB []byte) [][]common.Hash {
	// Process the padded data using erasure coding
	c_base := types.ComputeC_Base(len(paddedB))

	encodedB, err := erasurecoding.Encode(paddedB, c_base)
	if err != nil {
		fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
	}

	// Hash each element of the encoded data
	var tmpbClub []common.Hash
	var bClub [][]common.Hash

	for _, block := range encodedB {
		for _, b := range block {
			tmpbClub = append(tmpbClub, common.Hash(common.PadToMultipleOfN(b, 32)))
		}
		bClub = append(bClub, tmpbClub)
	}

	return bClub
}

func (n *Node) recomputeSClub(combinedSegmentAndPageProofs [][]byte) [][]common.Hash {
	var combinedData [][][]byte

	// Flatten the combined data
	var FlattenData []byte
	for _, singleData := range combinedSegmentAndPageProofs {
		FlattenData = append(FlattenData, singleData...)
	}
	// Erasure code the combined data
	encodedSegment, err := erasurecoding.Encode(FlattenData, 6)
	if err != nil {
		fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
	}

	// Append the encoded segment to the combined data
	combinedData = append(combinedData, encodedSegment...)
	// fmt.Printf("Before Transpose Size: %d, %d, %d\n", len(combinedData), len(combinedData[0]), len(combinedData[0][0]))

	// Transpose the combined data
	transposedData := transpose3D(combinedData)
	// fmt.Printf("After Transpose Size: %d, %d, %d\n", len(transposedData), len(transposedData[0]), len(transposedData[0][0]))

	var sClub [][]common.Hash
	var tmpsClub []common.Hash
	for _, data := range transposedData {
		root := trie.NewWellBalancedTree(data).Root()
		tmpsClub = append(tmpsClub, common.Hash(root))
	}
	sClub = append(sClub, tmpsClub)

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

	// Building a new 3D array to store the transposed data
	transposed := make([][][]byte, len(data[0]))
	for i := range transposed {
		transposed[i] = make([][]byte, len(data))
		for j := range transposed[i] {
			transposed[i][j] = make([]byte, len(data[0][0]))
		}
	}

	// Transposing the data
	for i := 0; i < len(data[0]); i++ {
		for j := 0; j < len(data); j++ {
			for k := 0; k < len(data[0][0]); k++ {
				transposed[i][j][k] = data[j][i][k]
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
	encodedPackage, err := types.Encode(wp)
	if err != nil {
		fmt.Println("Error in encodeWorkPackage:", err)
	}
	output = append(output, encodedPackage...)

	// 2. Encode the extrinsic (x)
	x := wp.WorkItems
	extrinsics := make([][]byte, 0)
	for _, WorkItem := range x {
		extrinsics = append(extrinsics, WorkItem.ExtrinsicsBlobs...)
	}
	fmt.Println("extrinsics:", extrinsics)
	encodedExtrinsic, err := types.Encode(extrinsics)
	if err != nil {
		fmt.Println("Error in encodeWorkPackage:", err)
	}
	output = append(output, encodedExtrinsic...)

	// 3. Encode the segments (i)
	var encodedSegments []byte
	var segments [][]byte
	for _, WorkItem := range x {
		segments, _ = n.getImportSegments(WorkItem.ImportedSegments)
	}
	fmt.Println("segments:", segments)
	encodedSegments, err = types.Encode(segments)
	if err != nil {
		fmt.Println("Error in encodeWorkPackage:", err)
	}
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
	encodedJustification, err := types.Encode(justification)
	if err != nil {
		fmt.Println("Error in encodeWorkPackage:", err)
	}
	encodedJustifications = append(encodedJustifications, encodedJustification...)

	output = append(output, encodedJustifications...)

	// Combine all encoded parts: e(p,x,i,j)
	return output
}

func (n *Node) decodeWorkPackage(encodedWorkPackage []byte) types.WorkPackage {
	fmt.Println("decodeWorkPackage")
	// length := uint32(0)

	// // Decode the package (p)
	wp, _, err := types.Decode(encodedWorkPackage, reflect.TypeOf(types.WorkPackage{}))
	if err != nil {
		fmt.Println("Error in decodeWorkPackage:", err)
	}
	decodedPackage := wp.(types.WorkPackage)
	fmt.Println("decodedPackage:", decodedPackage.String())
	// length += l
	/*
		// Decode the extrinsic (x)
		extrinsics, l := types.Decode(encodedWorkPackage[length:], reflect.TypeOf([][]byte{}))
		fmt.Println("extrinsics:", extrinsics)
		decodedPackage.WorkItems = make([]types.WorkItem, 0)
		for _, extrinsic := range extrinsics.([][]byte) {
			decodedPackage.WorkItems = append(decodedPackage.WorkItems, types.WorkItem{
				ExtrinsicsBlobs: [][]byte{extrinsic},
			})
		}
		length += l

		// Decode the segments (i)
		segments, l := types.Decode(encodedWorkPackage[length:], reflect.TypeOf([][]byte{}))
		fmt.Println("segments:", segments)
		// setImportSegments
		length += l

		// Decode the justifications (j)
		justifications, l := types.Decode(encodedWorkPackage[length:], reflect.TypeOf([][]byte{}))
		fmt.Println("justifications:", justifications)
		// setJustifications
		length += l
	*/

	return decodedPackage
	// return decodedPackage, decodedPackage.WorkItems, segments,justifications
}

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

// Verify WorkPackage by comparing the original and the decoded WorkPackage
func (n *Node) VerifyWorkPackage(workPackage types.WorkPackage) bool {
	b := n.encodeWorkPackage(workPackage)
	p := n.decodeWorkPackage(b)
	if compareWorkPackages(workPackage, p) {
		return true
	} else {
		return false
	}
}

// After FetchExportedSegments and quick verify exported segments
func (n *Node) FetchWorkPackage(erasureRoot common.Hash, lengthB int) (types.WorkPackage, common.Hash, error) {
	// Fetch the WorkPackage and the exported segments
	// erasureRoot = B^Club + S^Club
	allHash, err := n.store.ReadKV(erasureRoot)
	if err != nil {
		fmt.Println("Error in FetchWorkPackageAndExportedSegments:", err)
	}
	fmt.Printf("erasureRoot Val %x\n", allHash)
	bClubRoot := allHash[:32]
	bClubHash := common.Hash(bClubRoot)

	encodedB, err := n.FetchAndReconstructArbitraryData(bClubHash, lengthB)
	if err != nil {
		fmt.Printf("\nError in FetchWorkPackage: %v\n", err)
		return types.WorkPackage{}, common.Hash{}, err
	}
	fmt.Printf("FetchWorkPackage erasureRoot=%v, bClubHash=%v, encodedB=%v\n", erasureRoot, bClubHash, encodedB)
	encodedB = encodedB[:lengthB]
	workpackage := n.decodeWorkPackage(encodedB)
	// TODO:Do something like audit

	return workpackage, bClubHash, err
}

func (n *Node) FetchExportedSegments(erasureRoot common.Hash) ([][]byte, [][]byte, []common.Hash, common.Hash, error) {
	// Fetch the WorkPackage and the exported segments
	// TODO: use broadcast to fetch the data
	// erasureRoot = B^Club + S^Club
	allHash, err := n.store.ReadKV(erasureRoot)
	if err != nil {
		fmt.Println("Error in FetchWorkPackageAndExportedSegments:", err)
	}
	fmt.Printf("allHash: %x\n", allHash)
	sClubRoot := allHash[32:]
	sClubHash := common.Hash(sClubRoot)
	fmt.Printf("sClubHash: %v\n", sClubHash)
	exportedSegments, pageProofs, treeRoots, err := n.FetchAndReconstructAllSegmentsData(sClubHash)
	if err != nil {
		return [][]byte{}, [][]byte{}, []common.Hash{}, common.Hash{}, err
	}
	return exportedSegments, pageProofs, treeRoots, sClubHash, nil
}

// func (n *Node) FetchWorkPackageAndExportedSegments(erasureRoot common.Hash) (workPackage types.WorkPackage, exportedSegments [][]byte, err error) {
// 	exportedSegments, treeRoots, sClubHash, segment_err := n.FetchExportedSegments(erasureRoot)
// 	if (segment_err != nil){
// 		return workPackage, exportedSegments, segment_err
// 	}
// 	workPackage, bClubHash, wp_err := n.FetchWorkPackage(erasureRoot)
// 	if (wp_err != nil){
// 		return types.WorkPackage{}, exportedSegments, wp_err
// 	}
// 	//b♣,s♣
// 	fmt.Printf("b♣Hash=%v, s♣Hash=%v, treeRoots=%v\n", bClubHash, sClubHash, treeRoots)
// 	return workPackage, exportedSegments, nil
// }

// work types.GuaranteeReport, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error
func (n *Node) executeWorkPackage(workPackage types.WorkPackage) (work types.WorkReport, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error) {

	// Create a new PVM instance with mock code and execute it
	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()
	service_index := uint32(workPackage.AuthCodeHost)
	packageHash := workPackage.Hash()
	fmt.Printf("[V%d]Processing Work Package: %v, Key: %v\n", n.GetCurrValidatorIndex(), packageHash, n.GetEd25519Key())

	//TODO: do we still need audit friendly work WorkPackage?

	segments := make([][]byte, 0)
	for _, workItem := range workPackage.WorkItems {
		// recover code from the bpt. NOT from DA
		code := targetStateDB.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if len(code) == 0 {
			err = fmt.Errorf("code not found in bpt. C(%v, %v)", service_index, workItem.CodeHash)
			fmt.Println(err)
			return types.WorkReport{}, spec, common.Hash{}, err
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			fmt.Printf("Code and CodeHash Mismatch\n")
			panic(0)
		}
		vm := pvm.NewVMFromCode(service_index, code, 0, targetStateDB)
		// set malicious mode here
		vm.IsMalicious = false
		imports, err := n.getImportSegments(workItem.ImportedSegments)
		if err != nil {
			// return spec, common.Hash{}, err
			imports = make([][]byte, 0)
		}
		// Decode Import Segments to FIB fromat
		fmt.Printf("Import Segments: %v\n", imports)
		if len(imports) > 0 {
			fib_imported_result := imports[0][:12]
			n := binary.LittleEndian.Uint32(fib_imported_result[0:4])
			Fib_n := binary.LittleEndian.Uint32(fib_imported_result[4:8])
			Fib_n_1 := binary.LittleEndian.Uint32(fib_imported_result[8:12])
			fmt.Printf("Imported FIB: n= %v, Fib[n]= %v, Fib[n-1]= %v\n\n", n, Fib_n, Fib_n_1)
		}
		vm.SetImports(imports)
		vm.SetExtrinsicsPayload(workItem.ExtrinsicsBlobs, workItem.Payload)
		err = vm.Execute(types.EntryPointRefine)
		if err != nil {
			return types.WorkReport{}, spec, common.Hash{}, err
		}
		output, _ := vm.GetArgumentOutputs()

		// The workitem is an ordered collection of segments
		asWorkItem := types.ASWorkItem{
			Segments:   make([]types.Segment, 0),
			Extrinsics: make([]types.WorkItemExtrinsic, 0),
		}
		for _, i := range vm.Imports {
			asWorkItem.Segments = append(asWorkItem.Segments, types.Segment{Data: i})
		}
		for _, extrinsicblob := range workItem.ExtrinsicsBlobs {
			asWorkItem.Extrinsics = append(asWorkItem.Extrinsics, types.WorkItemExtrinsic{Hash: common.BytesToHash(extrinsicblob), Len: uint32(len(extrinsicblob))})
		}

		// 1. NOTE: We do NOT need to erasure code import data
		// 2. TODO: We DO need to erasure encode extrinsics into "Audit DA"
		// ******TODO******

		// 3. We DO need to erasure code exports from refine execution into "Import DA"
		fmt.Printf("VM Exports: %v\n", vm.Exports)
		for _, e := range vm.Exports {
			s := e
			segments = append(segments, s) // this is used in NewAvailabilitySpecifier
		}

		// Decode the Exports Segments to FIB format
		fmt.Printf("Exports Segments: %v\n", segments)
		if len(segments) > 0 {
			fib_exported_result := segments[0][:12]
			n := binary.LittleEndian.Uint32(fib_exported_result[0:4])
			Fib_n := binary.LittleEndian.Uint32(fib_exported_result[4:8])
			Fib_n_1 := binary.LittleEndian.Uint32(fib_exported_result[8:12])
			fmt.Printf("Exported FIB: n= %v, Fib[n]= %v, Fib[n-1]= %v\n\n", n, Fib_n, Fib_n_1)
		}

		//pageProofs, _ := trie.GeneratePageProof(segments)
		//combinedSegmentAndPageProofs := append(segments, pageProofs...)

		// setup work results
		// 11.1.4. Work Result. Equation 121. We finally come to define a work result, L, which is the data conduit by which services’ states may be altered through the computation done within a work-package.
		result := types.WorkResult{
			Service:     workItem.Service,
			CodeHash:    workItem.CodeHash,
			PayloadHash: common.Blake2Hash(workItem.Payload),
			GasRatio:    0,
			Result:      output,
		}
		results = append(results, result)
	}

	//treeRoot, err = n.EncodeAndDistributeSegmentData(combinedSegmentAndPageProofs, &wg)
	// TODO: need to figure out where distribution is happening

	// Step 2:  Now create a WorkReport with AvailabilitySpecification and RefinementContext
	spec = n.NewAvailabilitySpecifier(packageHash, workPackage, segments)
	prerequisite_hash := common.HexToHash("0x")
	refinementContext := types.RefineContext{
		Anchor:           n.statedb.ParentHash,                      // TODO  common.HexToHash("0x123abc")
		StateRoot:        n.statedb.Block.Header.ParentStateRoot,    // TODO, common.HexToHash("0x")
		BeefyRoot:        common.HexToHash("0x"),                    // SKIP
		LookupAnchor:     n.statedb.ParentHash,                      // TODO
		LookupAnchorSlot: n.statedb.Block.Header.Slot,               //TODO: uint32(0)
		Prerequisite:     (*types.Prerequisite)(&prerequisite_hash), //common.HexToHash("0x"), // SKIP
	}

	core, err := n.GetSelfCoreIndex()
	if err != nil {
		return types.WorkReport{}, spec, common.Hash{}, err
	}

	workReport := types.WorkReport{
		AvailabilitySpec: *spec,
		AuthorizerHash:   common.HexToHash("0x"), // SKIP
		CoreIndex:        core,
		//	Output:               result.Output,
		RefineContext: refinementContext,
		Results:       results,
	}

	//workReport.Print()
	//work = n.MakeGuaranteeReport(workReport)
	//work.Sign(n.GetEd25519Secret())

	return workReport, spec, treeRoot, nil
}
