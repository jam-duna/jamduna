package node

import (
	"fmt"
	"time"

	"encoding/json"

	"github.com/colorfulnotion/jam/pvm"

	"github.com/colorfulnotion/jam/common"
	//"github.com/colorfulnotion/jam/erasurecoding"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) NewAvailabilitySpecifier(packageHash common.Hash, workPackage types.WorkPackage, segments [][]byte) (availabilityspecifier *types.AvailabilitySpecifier, erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) {
	// compile wp into b
	// FetchWorkPackageImportSegments
	if debugSegments {
		fmt.Printf("NewAvailabilitySpecifier segments: %x\n", segments)
	}
	importSegments, err := n.FetchWorkpackageImportSegments(workPackage)
	if err != nil {
		// fmt.Printf("FetchWorkPackageImportSegments Error: %v\n", err)
	}
	package_bundle := n.CompilePackageBundle(workPackage, importSegments)
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
	if debugSegments {
		fmt.Printf("[N%d] package_bundle %v\n", n.id, package_bundle)
		fmt.Printf("[N%d] exported_segment_root_e %v importSegments %x\n", n.id, exported_segment_root_e, importSegments)
		fmt.Printf("[N%d] exported Segments %x\n", n.id, segments)
	}
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

func (n *Node) FetchImportSegments(erasureRoot common.Hash, segmentIdx uint16) (segments []byte, err error) {
	// now call C139 to get bundle_shard from assurer...
	// and then do ec rescontruction for b
	return segments, nil
}

// The E(p,x,s,j) function is a function that takes a package and its segments and returns a result, in EQ(186)
func (n *Node) CompilePackageBundle(p types.WorkPackage, importSegments [][][]byte) types.WorkPackageBundle {
	// imports := make([][]byte, 0)
	// for _, segment := range importSegments {
	// 	imports = append(imports, segment...)
	// }
	workItems := p.WorkItems
	workItemCnt := 0
	for _, workItem := range workItems {
		if len(workItem.ImportedSegments) > 0 {
			workItemCnt++
		}
	}

	// p - workPackage
	// x - [extrinsic data] for some workitem argument w
	extrinsicData := make([]types.ExtrinsicsBlobs, len(workItems))
	for workIdx, workItem := range workItems {
		extrinsicData[workIdx] = workItem.ExtrinsicsBlobs
	}

	// s - [ImportSegmentData] should be size of G = W_E * W_S
	importedSegmentData := importSegments
	imports := make([][]byte, 0)
	for _, segment := range importSegments {
		imports = append(imports, segment...)
	}

	// j - justifications
	verifyIndex := 0
	justifications := make([][][]common.Hash, 0)
	for itemIndex := range len(importSegments) {
		CDTTree := trie.NewCDMerkleTree(imports)
		tmpJustifications := make([][]common.Hash, 0)
		for i := 0; i < len(importSegments[itemIndex]); i++ {
			justification, err := CDTTree.Justify(verifyIndex)
			if err != nil {
				fmt.Printf("Justification Error: %v\n", err)
			}
			justificationHashes := make([]common.Hash, 0)
			for _, j := range justification {
				justificationHashes = append(justificationHashes, common.Hash(j))
			}
			tmpJustifications = append(tmpJustifications, justificationHashes)
			verifyIndex++
		}
		justifications = append(justifications, tmpJustifications)
	}
	workPackageBundle := types.WorkPackageBundle{
		WorkPackage:       p,
		ExtrinsicData:     extrinsicData,
		ImportSegmentData: importedSegmentData,
		Justification:     justifications,
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

func (n *Node) GetImportedSegmentRoots(wp types.WorkPackage) (importedSegmentRoots []common.Hash, err error) {
	importedSegmentRoots = make([]common.Hash, 0)
	for _, workItem := range wp.WorkItems {
		for _, importedSegment := range workItem.ImportedSegments {
			importedSegmentRoot, err := n.getExportedSegmenstRootFromHash(importedSegment.WorkPackageHash)
			if err != nil {
				// fmt.Printf("Error getting exported segment root: %v\n", err)
				return nil, err
			} else {
				if debugSegments {
					fmt.Printf("importedSegmentRoot: %v\n", importedSegmentRoot)
				}
			}
			importedSegmentRoots = append(importedSegmentRoots, importedSegmentRoot)
		}
	}
	return importedSegmentRoots, nil
}

func (n *Node) executeWorkPackageBundle(package_bundle types.WorkPackageBundle, importedSegmentRoots []common.Hash) (work_report types.WorkReport, err error) {
	if len(package_bundle.Justification) > 0 && len(package_bundle.Justification[0]) > 0 {
		if debugSegments {
			fmt.Printf("package_bundle.ImportSegmentData, package_bundle.Justification: %x, %v\n", package_bundle.ImportSegmentData, package_bundle.Justification)
		}
		ok, verifyErr := VerifyBundleJustification(package_bundle.ImportSegmentData, package_bundle.Justification, importedSegmentRoots)
		if verifyErr != nil || !ok {
			if verifyErr != nil {
				fmt.Printf("Justification Verification Error %v\n", verifyErr)
			}
			if !ok {
				fmt.Printf("Justification Verification Failed\n")
			}
			// packageHash := package_bundle.WorkPackage.Hash()
			// fetchPackageBundle, fetchErr := n.FetchWorkPackageBundle(packageHash)
			// if fetchErr != nil {
			// 	fmt.Printf("Error in fetching package bundle: %v\n", err)
			// 	return
			// }
			// package_bundle = *fetchPackageBundle
		} else {
			fmt.Printf("Justification Verification Passed\n")
		}
	}

	start := time.Now()
	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()
	workPackage := package_bundle.WorkPackage
	service_index := uint32(workPackage.AuthCodeHost)
	workPackageHash := workPackage.Hash()
	importsegments := make([][][]byte, 0)
	// Import Segments
	for _, segment := range package_bundle.ImportSegmentData {
		importsegments = append(importsegments, segment)
	}
	var segments [][]byte
	for index, workItem := range workPackage.WorkItems {
		imports := importsegments[index]

		service_index = workItem.Service
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
		if debugSegments {
			fmt.Printf("before SetImports Importsegments %x\n", imports)
		}
		vm.SetImports(imports)
		vm.SetExtrinsicsPayload(workItem.ExtrinsicsBlobs, workItem.Payload)
		output, _ := vm.ExecuteRefine(service_index, workItem.Payload, workPackageHash, workItem.CodeHash, workPackage.Authorizer.CodeHash, workPackage.Authorization, workItem.ExtrinsicsBlobs)
		exports := common.PadToMultipleOfN(output.Ok, types.W_E*types.W_S)
		if debugSegments {
			fmt.Printf("[N%d] [%d] len(exports) %d, exports %x\n", n.id, index, len(exports), exports)
		}
		for i := 0; i < len(exports); i += types.W_E * types.W_S {
			segments = append(segments, exports[i:i+types.W_E*types.W_S])
		}
		result := types.WorkResult{
			ServiceID:   workItem.Service,
			CodeHash:    workItem.CodeHash,
			PayloadHash: common.Blake2Hash(workItem.Payload),
			Gas:         10,
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

func (n *Node) FetchWorkpackageImportSegments(workPackage types.WorkPackage) ([][][]byte, error) {
	importsegments := make([][][]byte, len(workPackage.WorkItems))
	for _, workItem := range workPackage.WorkItems {
		if len(workItem.ImportedSegments) == 0 {
			return importsegments, nil
		}
	}

	// workPackageHashes
	workPackageHashes := make([]common.Hash, 0)
	for _, workItem := range workPackage.WorkItems {
		for _, ImportedSegment := range workItem.ImportedSegments {
			if !common.HashContains(workPackageHashes, ImportedSegment.WorkPackageHash) {
				workPackageHashes = append(workPackageHashes, ImportedSegment.WorkPackageHash)
			}
		}
	}

	// the mapping of workItem -> workPackageHashes -> indices (for remap the result of reconstruct)
	workItemPackageHashesMapping := make([]map[common.Hash][]uint16, len(workPackage.WorkItems))

	//the mapping of workPackageHashes -> indices (for make request)
	workPackageHashesMapping := make(map[common.Hash][]uint16, len(workPackage.WorkItems))

	for i, workItem := range workPackage.WorkItems {
		packageIdicesMap := make(map[common.Hash][]uint16, len(workItem.ImportedSegments))
		for _, ImportedSegment := range workItem.ImportedSegments {
			currentIndex := uint16(ImportedSegment.Index)
			packageIdicesMap[ImportedSegment.WorkPackageHash] = append(packageIdicesMap[ImportedSegment.WorkPackageHash], currentIndex)
			if !common.Uint16Contains(workPackageHashesMapping[ImportedSegment.WorkPackageHash], currentIndex) {
				workPackageHashesMapping[ImportedSegment.WorkPackageHash] = append(workPackageHashesMapping[ImportedSegment.WorkPackageHash], currentIndex)
			}
		}
		workItemPackageHashesMapping[i] = packageIdicesMap
	}
	if debugSegments {
		fmt.Printf("WorkItemPackageHashesMapping: %v\n", workItemPackageHashesMapping)
	}
	receiveSegmentMapping, err := n.reconstructSegments(workPackageHashes, workPackageHashesMapping)
	if err != nil {
		fmt.Printf("Error in reconstructSegments: %v\n", err)
		return importsegments, err
	}
	for workItemIndex, packageHashMapping := range workItemPackageHashesMapping {
		for packageHash, indices := range packageHashMapping {
			receivedSegments, exists := receiveSegmentMapping[packageHash]
			if !exists {
				fmt.Printf("Missing segments for packageHash: %v\n", packageHash)
				continue
			}
			for _, index := range indices {
				if int(index) >= len(receivedSegments) {
					fmt.Printf("Index out of range: %d for packageHash: %v\n", index, packageHash)
					continue
				}
				importsegments[workItemIndex] = append(importsegments[workItemIndex], receivedSegments[index])
			}
		}
	}
	if debugSegments {
		fmt.Printf("Final importsegments: %v\n", importsegments)
	}
	return importsegments, nil
}

// work types.GuaranteeReport, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error
func (n *Node) executeWorkPackage(workPackage types.WorkPackage, importSegments [][][]byte) (guarantee types.Guarantee, spec *types.AvailabilitySpecifier, treeRoot common.Hash, err error) {
	start := time.Now()
	// Create a new PVM instance with mock code and execute it
	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()
	service_index := uint32(workPackage.AuthCodeHost)
	workPackageHash := workPackage.Hash()

	segments := make([][]byte, 0)
	for index, workItem := range workPackage.WorkItems {
		imports := importSegments[index]
		// recover code from the bpt. NOT from DA
		service_index = workItem.Service
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
		vm.SetImports(imports)

		vm.SetExtrinsicsPayload(workItem.ExtrinsicsBlobs, workItem.Payload)
		output, _ := vm.ExecuteRefine(service_index, workItem.Payload, workPackageHash, workItem.CodeHash, workPackage.Authorizer.CodeHash, workPackage.Authorization, workItem.ExtrinsicsBlobs)
		exports := common.PadToMultipleOfN(output.Ok, types.W_E*types.W_S)
		for i := 0; i < len(exports); i += types.W_E * types.W_S {
			segments = append(segments, exports[i:i+types.W_E*types.W_S])
		}

		// Decode the Exports Segments to FIB format
		if len(segments) > 0 && service_index != 0 {
			// fib_exported_result := segments[0][:12]
			// num := binary.LittleEndian.Uint32(fib_exported_result[0:4])
			// Fib_n := binary.LittleEndian.Uint32(fib_exported_result[4:8])
			// Fib_n_1 := binary.LittleEndian.Uint32(fib_exported_result[8:12])
			// fmt.Printf("%s Exported FIB: n= %v, Fib[n]= %v, Fib[n-1]= %v\n", n.String(), num, Fib_n, Fib_n_1)
			if debugSegments {
				fmt.Printf("Exported segment: %x\n", segments[0])
			}
		}

		result := types.WorkResult{
			ServiceID:   workItem.Service,
			CodeHash:    workItem.CodeHash,
			PayloadHash: common.Blake2Hash(workItem.Payload),
			Gas:         10,
			Result:      output,
		}
		results = append(results, result)
	}

	// Step 2:  Now create a WorkReport with AvailabilitySpecification and RefinementContext
	fmt.Printf("[N%d] executeWorkPackage segments %x\n", n.id, segments)
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

func VerifyBundleJustification(importSegments [][][]byte, justifications [][][]common.Hash, exportedRoots []common.Hash) (ok bool, err error) {
	// Verify the justifications
	if debugSegments {
		fmt.Printf("exportedRoots %v\n", exportedRoots)
	}
	verifyIndex := 0
	for itemIndex := range importSegments {
		for segmentIdx := range importSegments[itemIndex] {
			if debugSegments {
				fmt.Printf("itemIndex %v, segmentIdx %v\n", itemIndex, segmentIdx)
				fmt.Printf("importSegments[itemIndex] %x\n", importSegments[itemIndex])
			}
			segmentData := importSegments[itemIndex][segmentIdx]
			segmentHash := common.ComputeLeafHash_WBT_Blake2B(segmentData)
			root := exportedRoots[itemIndex]
			transferJustifications := make([][]byte, 0)
			for _, justification := range justifications[itemIndex][segmentIdx] {
				transferJustifications = append(transferJustifications, justification[:])
			}
			computedRoot := trie.VerifyJustification(segmentHash[:], verifyIndex, transferJustifications)
			if !common.CompareBytes(root[:], computedRoot) && !common.CompareBytes(root[:], segmentHash[:]) {
				fmt.Printf("segmentData %x, segmentHash %v, transferJustifications %x\n", segmentData, segmentHash, transferJustifications)
				fmt.Printf("except root %x, computed root %x\n", root[:], computedRoot)
				return true, fmt.Errorf("justification failure")
			}
			verifyIndex++
		}
	}
	return true, nil
	// fmt.Printf("exportedRoots %v\n", exportedRoots)
	// for itemIndex := range importSegments {
	// 	for segmentIdx := range importSegments[itemIndex] {
	// 		fmt.Printf("itemIndex %v, segmentIdx %v\n", itemIndex, segmentIdx)
	// 		fmt.Printf("importSegments[itemIndex] %x\n", importSegments[itemIndex])
	// 		segmentData := importSegments[itemIndex][segmentIdx]
	// 		segmentHash := common.ComputeLeafHash_WBT_Blake2B(segmentData)
	// 		root := exportedRoots[itemIndex]
	// 		transferJustifications := make([][]byte, 0)
	// 		for _, justification := range justifications[itemIndex][segmentIdx] {
	// 			transferJustifications = append(transferJustifications, justification[:])
	// 		}
	// 		computedRoot := trie.VerifyJustification(segmentHash[:], segmentIdx, transferJustifications)
	// 		if !common.CompareBytes(root[:], computedRoot) {
	// 			fmt.Printf("segmentData %x, segmentHash %v, transferJustifications %x\n", segmentData, segmentHash, transferJustifications)
	// 			fmt.Printf("except root %x, computed root %x\n", root[:], computedRoot)
	// 			return true, fmt.Errorf("justification failure")
	// 		}
	// 	}
	// }
	// return true, nil
}
