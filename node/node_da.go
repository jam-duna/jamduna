package node

import (
	"fmt"
	"time"

	"encoding/binary"
	"encoding/json"

	"github.com/colorfulnotion/jam/pvm"

	"github.com/colorfulnotion/jam/common"
	//"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) NewAvailabilitySpecifier(packageHash common.Hash, workPackage types.WorkPackage, segments [][]byte) (availabilityspecifier *types.AvailabilitySpecifier, erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) {
	// compile wp into b
	package_bundle := n.CompilePackageBundle(workPackage)
	b := package_bundle.Bytes()
	recovered_package_bundle, _ := types.WorkPackageBundleFromBytes(b)

	if debugDA {
		fmt.Printf("packageHash=%v, encodedPackage(Len=%v):%x\n", packageHash, len(b), b)
		fmt.Printf("raw=%v\n", package_bundle.String())
		//recovered_b := n.decodeWorkPackage(package_bundle)
		//TODO: make sure b equals recovered_b
		if common.CompareBytes(package_bundle.Bytes(), recovered_package_bundle.Bytes()) {
			//fmt.Printf("----------Original WorkPackage and Decoded WorkPackage are the same-------\n")
		} else {
			fmt.Printf("----------Original WorkPackage and Decoded WorkPackage are different-------\n")
		}
	}
	// Length of `b`
	bLength := uint32(len(b))

	// Build b♣ and s♣
	bClubs, bEcChunks := n.buildBClub(b)
	sClubs, sEcChunksArr := n.buildSClub(segments)
	if debugDA {
		fmt.Printf("len(bEcChunks)=%v\n", len(bEcChunks))
		fmt.Printf("len(sEcChunksArr)=%v\n", len(sEcChunksArr))
		fmt.Printf("bClubs %v\n", bClubs)
		fmt.Printf("sClubs %v\n", sClubs)
	}
	// u = (bClub, sClub)
	erasure_root_u := n.generateErasureRoot(bClubs, sClubs)

	// ExportedSegmentRoot = CDT(segments)
	exported_segment_root_e := generateExportedSegmentsRoot(segments)

	// Return the Availability Specifier
	availabilitySpecifier := types.AvailabilitySpecifier{
		WorkPackageHash:     packageHash,
		BundleLength:        bLength,
		ErasureRoot:         erasure_root_u,
		ExportedSegmentRoot: exported_segment_root_e,
	}

	erasureMeta = ECCErasureMap{
		ErasureRoot:         erasure_root_u,
		ExportedSegmentRoot: exported_segment_root_e,
		WorkPackageHash:     packageHash,
		BundleLength:        bLength,
		BClubs:              bClubs,
		SClubs:              sClubs,
	}

	return &availabilitySpecifier, erasureMeta, bEcChunks, sEcChunksArr
}

// this is the default justification from (b,s) to erasureRoot
func ErasureRootDefaultJustification(b []common.Hash, s []common.Hash) (shardJustifications []types.Justification, err error) {
	shardJustifications = make([]types.Justification, types.TotalValidators)
	erasureTree, _ := GenerateErasureTree(b, s)
	erasureRoot := erasureTree.RootHash()
	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		treeLen, leafHash, path, isFound, _ := erasureTree.Trace(shardIdx)
		verified, _ := VerifyJustification(treeLen, erasureRoot, uint16(shardIdx), leafHash, path)
		if !verified {
			return shardJustifications, fmt.Errorf("Justification Failure")
		}
		shardJustifications[shardIdx] = types.Justification{
			Root:     erasureRoot,
			ShardIdx: shardIdx,
			TreeLen:  types.TotalValidators,
			LeafHash: leafHash,
			Path:     path,
		}
		if debugDA {
			fmt.Printf("ErasureRootPath shardIdx=%v, treeLen=%v leafHash=%v, path=%v, isFound=%v | verified=%v\n", shardIdx, treeLen, leafHash, path, isFound, verified)
		}
	}
	return shardJustifications, nil
}

// Verify T(s,i,H)
func VerifyJustification(treeLen int, root common.Hash, shardIndex uint16, leafHash common.Hash, path []common.Hash) (bool, common.Hash) {
	recoveredRoot, verified, _ := trie.VerifyWBT(treeLen, int(shardIndex), root, leafHash, path)
	if root != recoveredRoot {
		fmt.Sprintf("VerifyJustification Failure! Expected:%v | Recovered: %v\n", root, recoveredRoot)
		//panic("VerifyJustification")
		return verified, recoveredRoot
	}
	return verified, recoveredRoot
}

// Generating co-path for T(s,i,H)
// s: [(b♣T,s♣T)...] -  sequence of (work-package bundle shard hash, segment shard root) pairs satisfying u = MB(s)
// i: shardIdx or ChunkIdx
// H: Blake2b
func GenerateJustification(root common.Hash, shardIndex uint16, leaves [][]byte) (treeLen int, leafHash common.Hash, path []common.Hash, isFound bool) {
	wbt := trie.NewWellBalancedTree(leaves, types.Blake2b)
	//treeLen, leafHash, path, isFound, nil
	treeLen, leafHash, path, isFound, _ = wbt.Trace(int(shardIndex))
	//fmt.Printf("[shardIndex=%v] erasureRoot=%v, leafHash=%v, path=%v, found=%v\n", shardIndex, erasureRoot, leafHash, path, isFound)
	return treeLen, leafHash, path, isFound
}

func GetOrderedChunks(erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) (shardJustifications []types.Justification, orderedBundleShards []types.ConformantECChunk, orderedSegmentShards [][]types.ConformantECChunk) {
	shardJustifications, _ = ErasureRootDefaultJustification(erasureMeta.BClubs, erasureMeta.SClubs)
	//fmt.Printf("shardJustifications: %v\n", shardJustifications[0].String())
	orderedBundleShards = ComputeOrderedNPBundleChunks(bECChunks)
	//fmt.Printf("orderedBundleShards %x\v\n", orderedBundleShards)
	orderedSegmentShards = ComputeOrderedExportedNPChunks(sECChunksArray)
	//fmt.Printf("orderedSegmentShards %x\v\n", orderedSegmentShards)
	return shardJustifications, orderedBundleShards, orderedSegmentShards
}

func GetShardSpecificOrderedChunks(shardIdx uint16, erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) (shardJustification types.Justification, bundleShard types.ConformantECChunk, segmentShards []types.ConformantECChunk) {
	idx := int(shardIdx)
	shardJustifications, orderedBundleShards, orderedSegmentShards := GetOrderedChunks(erasureMeta, bECChunks, sECChunksArray)
	return shardJustifications[idx], orderedBundleShards[idx], orderedSegmentShards[idx]
}

type ECCErasureMap struct {
	ErasureRoot         common.Hash
	ExportedSegmentRoot common.Hash
	WorkPackageHash     common.Hash
	BundleLength        uint32
	BClubs              []common.Hash
	SClubs              []common.Hash
}

// Marshal marshals ECCErasureMap into JSON
func (e *ECCErasureMap) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

func (e *ECCErasureMap) Unmarshal(data []byte) error {
	return json.Unmarshal(data, e)
}

// TODO: use codec ..
func (e *ECCErasureMap) Bytes() []byte {
	jsonData, _ := e.Marshal()
	return jsonData
}

// TODO: use codec ..
func (e *ECCErasureMap) String() string {
	return string(e.Bytes())
}

func (n *Node) PrepareArbitaryData(b []byte) ([][][]byte, common.Hash, int) {
	// Padding b to the length of W_E
	paddedB := common.PadToMultipleOfN(b, types.W_E)
	bLength := len(b)

	chunks, err := n.encode(paddedB, false, bLength)
	if err != nil {
		fmt.Println("Error in prepareArbitaryData:", err)
	}
	blobHash := common.Blake2Hash(paddedB)
	return chunks, blobHash, bLength
}

// Compute b♣ using the EncodeWorkPackage function
func (n *Node) buildBClub(b []byte) ([]common.Hash, []types.DistributeECChunk) {
	chunks, _, bLength := n.PrepareArbitaryData(b)
	// Hash each element of the encoded data
	bClubs := make([]common.Hash, types.TotalValidators)
	bundleShards := chunks[0] // this should be of size 1
	for shardIdx, shard := range bundleShards {
		bClubs[shardIdx] = common.Blake2Hash(shard)
	}

	ecChunks, err := n.BuildArbitraryDataChunks(chunks, bLength)
	if err != nil {
		fmt.Println("Error in DistributeSegmentData:", err)
	}

	return bClubs, ecChunks
}

func (n *Node) buildSClub(segments [][]byte) (sClub []common.Hash, ecChunksArr [][]types.DistributeECChunk) {
	ecChunksArr = make([][]types.DistributeECChunk, 0)
	// key data structure: sequentialTranspose
	sequentialTranspose := make([][][]byte, types.TotalValidators)

	// gathering root per segment or pageProof
	for segmentIdx, segmentData := range segments {
		// Encode segmentData into leaves
		erasureCodingSegments, err := n.encode(segmentData, true, len(segmentData)) // Set to false for variable size segments
		if err != nil {
			fmt.Printf("Error in buildSClub segment#%v: %v\n", segmentIdx, err)
		}
		if debugDA {
			fmt.Printf("segment#%v len=%v\n", segmentIdx, len(erasureCodingSegments))
		}
		if len(erasureCodingSegments) != 1 {
			panic("Invalid segment implementation! NOT OK")
		}
		// Build segment roots from erasureCodingSegments[0] which are the leaves of the segmentData
		segmentTree := trie.NewCDMerkleTree(erasureCodingSegments[0])
		segmentRoots := [][]byte{segmentTree.RootHash().Bytes()}
		// Build the segment chunks
		ecChunks, err := n.BuildExportedSegmentChunks(erasureCodingSegments, segmentRoots)
		if err != nil {
			fmt.Printf("Error in buildSClub segment#%v: %v\n", segmentIdx, err)
		}
		for chunkIdx, ecChunks := range ecChunks {
			shardIdx := uint32(chunkIdx % types.TotalValidators)
			sequentialTranspose[shardIdx] = append(sequentialTranspose[shardIdx], ecChunks.Data)
		}
		if debugDA {
			fmt.Printf("buildSClub segment#%v: len(ecChunks)=%v\n", segmentIdx, len(ecChunks))
		}
		ecChunksArr = append(ecChunksArr, ecChunks)
	}

	// gathering root per pagrProof, each pageProof can be more than G per our implementation
	pageProofs, _ := trie.GeneratePageProof(segments)
	for pageIdx, pageData := range pageProofs {
		// Encode the data into segments
		erasureCodingPageSegments, err := n.encode(pageData, true, len(pageData)) // Set to false for variable size segments
		if err != nil {
			fmt.Printf("Error in buildSClub pageProof#%v: %v\n", pageIdx, err)
		}

		// Build segment roots -- page can be longer than G long
		if debugDA {
			fmt.Printf("!!! pageProof#%v len=%v\n", pageIdx, len(erasureCodingPageSegments))
		}
		pageSegmentRoots := make([][]byte, 0)
		ith_pageProof := make([]common.Hash, 0)
		for i := range erasureCodingPageSegments {
			pageProofleaves := erasureCodingPageSegments[i]
			pageProofSubtree := trie.NewCDMerkleTree(pageProofleaves)
			pageProofSubtreeRoot := pageProofSubtree.RootHash()
			ith_pageProof = append(ith_pageProof, pageProofSubtreeRoot)
			pageSegmentRoots = append(pageSegmentRoots, pageProofSubtreeRoot.Bytes())
		}

		// Build ecChunks for the exported segments
		ecChunks, err := n.BuildExportedSegmentChunks(erasureCodingPageSegments, pageSegmentRoots)
		if err != nil {
			fmt.Printf("Error in buildSClub pageProof#%v: %v\n", pageIdx, err)
		}
		for chunkIdx, ecChunks := range ecChunks {
			shardIdx := uint32(chunkIdx % types.TotalValidators)
			sequentialTranspose[shardIdx] = append(sequentialTranspose[shardIdx], ecChunks.Data)
		}
		if debugDA {
			fmt.Printf("len(ecChunks)=%v\n", len(ecChunks)) // this is multiple of totalValidators
			fmt.Printf("buildSClub pageProof#%v: len(ecChunks)=%v\n", pageIdx, len(ecChunks))
		}
		ecChunksArr = append(ecChunksArr, ecChunks)
	}
	if debugDA {
		fmt.Printf("len(ecChunksArr)=%v\n", len(ecChunksArr))
	}

	sClub = make([]common.Hash, types.TotalValidators)
	for shardIdx, shardData := range sequentialTranspose {
		shard_wbt := trie.NewWellBalancedTree(shardData, types.Blake2b)
		sClub[shardIdx] = shard_wbt.RootHash()
	}
	return sClub, ecChunksArr
}

func GenerateErasureTree(b []common.Hash, s []common.Hash) (*trie.WellBalancedTree, [][]byte) {
	// Combine b and s into (work-package bundle shard hash, segment shard root) pairs
	bundleSegmentPairs := make([][]byte, types.TotalValidators)
	for i := 0; i < types.TotalValidators; i++ {
		bundleSegmentPairs[i] = append(b[i].Bytes(), s[i].Bytes()...)
	}

	// Generate and return erasureroot
	return trie.NewWellBalancedTree(bundleSegmentPairs, types.Blake2b), bundleSegmentPairs
}

// MB([x∣x∈T[b♣,s♣]]) - Encode b♣ and s♣ into a matrix
func (n *Node) generateErasureRoot(b []common.Hash, s []common.Hash) common.Hash {
	erasureTree, bundle_segment_pairs := GenerateErasureTree(b, s)
	erasureRoot := erasureTree.RootHash()
	if debugDA {
		//fmt.Printf("Len(bundle_segment_pairs), bundle_segment_pairs: %d, %x\n", len(bundle_segment_pairs), bundle_segment_pairs)
	}

	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		treeLen, leafHash, path, isFound := GenerateJustification(erasureRoot, uint16(shardIdx), bundle_segment_pairs)
		verified, _ := VerifyJustification(treeLen, erasureRoot, uint16(shardIdx), leafHash, path)
		if debugDA {
			if !verified {
				fmt.Printf("ErasureRootPath shardIdx=%v, treeLen=%v leafHash=%v, path=%v, isFound=%v | verified=%v\n", shardIdx, treeLen, leafHash, path, isFound, verified)
				panic(1999)
			}
		}
	}
	return erasureRoot
}

// M(s) - CDT of exportedSegment
func generateExportedSegmentsRoot(segments [][]byte) common.Hash {
	var segmentData [][]byte
	for _, segment := range segments {
		segmentData = append(segmentData, segment)
	}

	cdt := trie.NewCDMerkleTree(segmentData)
	return common.Hash(cdt.Root())
}

func (n *Node) FetchWorkPackageBundle(erasureRoot common.Hash) (pb *types.WorkPackageBundle, err error) {
	// now call C138 to get bundle_shard from assurer...
	// and then do ec rescontruction for b
	return pb, nil
}

func (n *Node) FetchImportSegements(erasureRoot common.Hash, segmentIdx uint16) (segments []byte, err error) {
	// now call C139 to get bundle_shard from assurer...
	// and then do ec rescontruction for b
	return segments, nil
}

// The E(p,x,s,j) function is a function that takes a package and its segments and returns a result, in EQ(186)
func (n *Node) CompilePackageBundle(p types.WorkPackage) types.WorkPackageBundle {
	workItems := p.WorkItems
	workItemCnt := len(workItems)
	// p - workPackage
	// x - [extrinsic data] for some workitem argument w
	extrinsicData := make([]types.ExtrinsicsBlobs, workItemCnt)
	for workIdx, workItem := range workItems {
		extrinsicData[workIdx] = workItem.ExtrinsicsBlobs
	}

	// s - [ImportSegmentData] should be size of G = W_E * W_S
	importedSegmentData := make([][][]byte, workItemCnt)
	importedSegmentIdx := make([][]uint16, workItemCnt)

	for workIdx, workItem := range workItems {
		// not sure what does idx mean inside of ImportedSegments
		segmentIdxMap := make([]uint16, len(workItem.ImportedSegments))
		workItemIdx_importedSegmentData, err := n.GetImportSegments(workItem.ImportedSegments)
		if err != nil {
			fmt.Printf("getImportSegments: %v\n", err)
			panic(40)
		}
		for i, workItem_importedSegment := range workItem.ImportedSegments {
			segmentIdxMap[i] = workItem_importedSegment.Index
		}
		importedSegmentIdx[workIdx] = segmentIdxMap
		importedSegmentData[workIdx] = workItemIdx_importedSegmentData
	}

	// j - justifications
	justification := make([][][]common.Hash, workItemCnt)
	for workIdx, item_segmentData := range importedSegmentData {
		workIdx_segment_tree := trie.NewCDMerkleTree(item_segmentData)
		var item_Justification [][]common.Hash
		for segment_idx, _ := range item_segmentData {
			// not sure what does index mean here...
			importedSegments_Index := importedSegmentIdx[workIdx][segment_idx]
			// not sure if we should use segment_idx or Index from ImportedSegments
			segment_Justification, _ := workIdx_segment_tree.Justify(int(importedSegments_Index))
			segment_Justification_hash := make([]common.Hash, len(segment_Justification))
			for hash_idx, byte := range segment_Justification {
				segment_Justification_hash[hash_idx] = common.Hash(byte)
			}
			item_Justification = append(item_Justification, segment_Justification_hash)
			justification[workIdx] = item_Justification
		}
	}

	workPackageBundle := types.WorkPackageBundle{
		WorkPackage:       p,
		ExtrinsicData:     extrinsicData,
		ImportSegmentData: importedSegmentData,
		Justification:     justification,
	}
	//fmt.Printf("ImportSegmentData: %d %v\n", len(importedSegmentData), importedSegmentData)
	return workPackageBundle
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
func (n *Node) VerifyWorkPackageBundle(package_bundle types.WorkPackageBundle) bool {
	package_bundle_byte := package_bundle.Bytes()
	recovered_package_bundle, _ := types.WorkPackageBundleFromBytes(package_bundle_byte)
	if common.CompareBytes(package_bundle.Bytes(), recovered_package_bundle.Bytes()) {
		return true
	} else {
		return false
	}
}

func (n *Node) executeWorkPackageBundle(package_bundle types.WorkPackageBundle) (work_report types.WorkReport, err error) {
	start := time.Now()
	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()
	workPackage := package_bundle.WorkPackage
	service_index := uint32(workPackage.AuthCodeHost)
	workPackageHash := workPackage.Hash()
	Importsegments := make([][]byte, 0)
	// Import Segments
	for _, segment := range package_bundle.ImportSegmentData {
		Importsegments = append(Importsegments, segment...)
	}
	var segments [][]byte
	for _, workItem := range workPackage.WorkItems {
		code := targetStateDB.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if len(code) == 0 {
			err = fmt.Errorf("code not found in bpt. C(%v, %v)", service_index, workItem.CodeHash)
			fmt.Println(err)
			return
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			fmt.Printf("Code and CodeHash Mismatch\n")
			panic(0)
		}
		vm := pvm.NewVMFromCode(service_index, code, 0, targetStateDB)
		vm.IsMalicious = false
		vm.SetImports(Importsegments)
		vm.SetExtrinsicsPayload(workItem.ExtrinsicsBlobs, workItem.Payload)
		output, _ := vm.ExecuteRefine(service_index, workItem.Payload, workPackageHash, workItem.CodeHash, workPackage.Authorizer.CodeHash, workPackage.Authorization, workItem.ExtrinsicsBlobs)
		exports := common.PadToMultipleOfN(output.Ok, types.W_E*types.W_S)
		for i := 0; i < len(exports); i += types.W_E * types.W_S {
			segments = append(segments, exports[i:i+types.W_E*types.W_S])
		}
		result := types.WorkResult{
			Service:     workItem.Service,
			CodeHash:    workItem.CodeHash,
			PayloadHash: common.Blake2Hash(workItem.Payload),
			GasRatio:    0,
			Result:      output,
		}
		results = append(results, result)
	}
	spec, erasureMeta, bECChunks, sECChunksArray := n.NewAvailabilitySpecifier(workPackageHash, workPackage, segments)
	core, err := n.GetSelfCoreIndex()
	if err != nil {
		return
	}
	workReport := types.WorkReport{
		AvailabilitySpec: *spec,
		AuthorizerHash:   common.HexToHash("0x"), // SKIP
		CoreIndex:        core,
		RefineContext:    workPackage.RefineContext,
		Results:          results,
	}
	if debugG {
		fmt.Printf("%s executeWorkPackage  workreporthash %v => erasureRoot: %v\n", n.String(), common.Str(workReport.Hash()), spec.ErasureRoot)
	}
	n.StoreMeta_Guarantor(spec, erasureMeta, bECChunks, sECChunksArray)
	if debugE {
		fmt.Printf("%s executeWorkPackageBundle took %v\n", n.String(), time.Since(start))
	}
	return workReport, err
}

// work types.GuaranteeReport, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error
func (n *Node) executeWorkPackage(workPackage types.WorkPackage) (guarantee types.Guarantee, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error) {
	start := time.Now()
	// Create a new PVM instance with mock code and execute it
	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()
	service_index := uint32(workPackage.AuthCodeHost)
	workPackageHash := workPackage.Hash()

	segments := make([][]byte, 0)
	for _, workItem := range workPackage.WorkItems {
		// recover code from the bpt. NOT from DA
		code := targetStateDB.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if len(code) == 0 {
			err = fmt.Errorf("code not found in bpt. C(%v, %v)", service_index, workItem.CodeHash)
			fmt.Println(err)
			return
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			fmt.Printf("Code and CodeHash Mismatch\n")
			panic(0)
		}
		vm := pvm.NewVMFromCode(service_index, code, 0, targetStateDB)
		// set malicious mode here
		vm.IsMalicious = false
		imports, err0 := n.GetImportSegments(workItem.ImportedSegments)
		if err0 != nil {
			// return spec, common.Hash{}, err
			imports = make([][]byte, 0)
		}
		// Decode Import Segments to FIB fromat
		if len(imports) > 0 {
			if debug {
				fib_imported_result := imports[0][:12]
				n := binary.LittleEndian.Uint32(fib_imported_result[0:4])
				Fib_n := binary.LittleEndian.Uint32(fib_imported_result[4:8])
				Fib_n_1 := binary.LittleEndian.Uint32(fib_imported_result[8:12])
				fmt.Printf("Imported FIB: n= %v, Fib[n]= %v, Fib[n-1]= %v\n\n", n, Fib_n, Fib_n_1)
			}
		}
		vm.SetImports(imports)

		vm.SetExtrinsicsPayload(workItem.ExtrinsicsBlobs, workItem.Payload)
		output, _ := vm.ExecuteRefine(service_index, workItem.Payload, workPackageHash, workItem.CodeHash, workPackage.Authorizer.CodeHash, workPackage.Authorization, workItem.ExtrinsicsBlobs)

		exports := common.PadToMultipleOfN(output.Ok, types.W_E*types.W_S)
		for i := 0; i < len(exports); i += types.W_E * types.W_S {
			segments = append(segments, exports[i:i+types.W_E*types.W_S])
		}

		// Decode the Exports Segments to FIB format
		if len(segments) > 0 && service_index != 0 {
			fib_exported_result := segments[0][:12]
			num := binary.LittleEndian.Uint32(fib_exported_result[0:4])
			Fib_n := binary.LittleEndian.Uint32(fib_exported_result[4:8])
			Fib_n_1 := binary.LittleEndian.Uint32(fib_exported_result[8:12])
			fmt.Printf("%s Exported FIB: n= %v, Fib[n]= %v, Fib[n-1]= %v\n", n.String(), num, Fib_n, Fib_n_1)
		}

		result := types.WorkResult{
			Service:     workItem.Service,
			CodeHash:    workItem.CodeHash,
			PayloadHash: common.Blake2Hash(workItem.Payload),
			GasRatio:    0,
			Result:      output,
		}
		results = append(results, result)
	}

	// Step 2:  Now create a WorkReport with AvailabilitySpecification and RefinementContext
	spec, erasureMeta, bECChunks, sECChunksArray := n.NewAvailabilitySpecifier(workPackageHash, workPackage, segments)
	core, err := n.GetSelfCoreIndex()
	if err != nil {
		return
	}

	workReport := types.WorkReport{
		AvailabilitySpec: *spec,
		AuthorizerHash:   common.HexToHash("0x"), // SKIP
		CoreIndex:        core,
		RefineContext:    workPackage.RefineContext,
		Results:          results,
	}
	if debugG {
		fmt.Printf("%s executeWorkPackage  workreporthash %v => erasureRoot: %v\n", n.String(), common.Str(workReport.Hash()), spec.ErasureRoot)
	}

	// a guarantor uses StoreImportDAErasureRootToSegments store segments but proper solution is with getImportSegment using CE139
	// err = n.StoreImportDAErasureRootToSegments(spec, common.ConcatenateByteSlices(segments))
	// if err != nil {
	// 	panic(1349)
	// }
	n.StoreMeta_Guarantor(spec, erasureMeta, bECChunks, sECChunksArray)
	//n.FakeDistributeChunks(erasureMeta, bECChunks, sECChunksArray)

	gc := workReport.Sign(n.GetEd25519Secret(), uint16(n.GetCurrValidatorIndex()))
	guarantee = types.Guarantee{
		Report:     workReport,
		Signatures: []types.GuaranteeCredential{gc},
	}
	if debugE {
		fmt.Printf("%s executeWorkPackage took %v\n", n.String(), time.Since(start))
	}
	return
}
