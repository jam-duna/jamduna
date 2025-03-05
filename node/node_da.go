package node

import (
	"fmt"

	"encoding/json"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) NewAvailabilitySpecifier(packageHash common.Hash, workPackage types.WorkPackage, segments [][]byte, extrinsics types.ExtrinsicsBlobs) (availabilityspecifier *types.AvailabilitySpecifier, erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) {
	// compile wp into b
	// FetchWorkPackageImportSegments
	log.Trace(debugDA, "NewAvailabilitySpecifier", "id", n.id, "segments", segments)
	importSegments, err := n.FetchWorkpackageImportSegments(workPackage)
	if err != nil {
		log.Error(debugDA, "NewAvailabilitySpecifier", "err", err)
	}
	package_bundle := n.CompilePackageBundle(workPackage, importSegments, extrinsics)
	b := package_bundle.Bytes()
	recovered_package_bundle, _ := types.WorkPackageBundleFromBytes(b)
	log.Trace(debugDA, "NewAvailabilitySpecifier", "packageHash", packageHash, "encodedPackage", b, "len(encodedPackage)", len(b))
	if !common.CompareBytes(package_bundle.Bytes(), recovered_package_bundle.Bytes()) {
		log.Error(debugDA, "NewAvailabilitySpecifier:Original WorkPackage and Decoded WorkPackage are different")
	}
	// Length of `b`
	bLength := uint32(len(b))

	// Build b♣ and s♣
	bClubs, bEcChunks := n.buildBClub(b)
	sClubs, sEcChunksArr := n.buildSClub(segments)
	log.Trace(debugDA, "NewAvailabilitySpecifier", "len(bEcChunks)", len(bEcChunks), "len(sEcChunksArr)", len(sEcChunksArr), "bClubs %v\n", bClubs, "sClubs", sClubs)
	// u = (bClub, sClub)
	erasure_root_u := n.generateErasureRoot(bClubs, sClubs)

	// ExportedSegmentRoot = CDT(segments)
	exported_segment_root_e := generateExportedSegmentsRoot(segments)
	log.Trace(debugDA, "n", n.id, "package_bundle", package_bundle.String(), "exported_segment_root_e", exported_segment_root_e, "importSegments", importSegments, "exported Segments", segments)

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
		verified, _ := VerifyWBTJustification(treeLen, erasureRoot, uint16(shardIdx), leafHash, path)
		if !verified {
			return shardJustifications, fmt.Errorf("VerifyWBTJustification Failure")
		}
		shardJustifications[shardIdx] = types.Justification{
			Root:     erasureRoot,
			ShardIdx: shardIdx,
			TreeLen:  types.TotalValidators,
			LeafHash: leafHash,
			Path:     path,
		}
		log.Trace(debugDA, "ErasureRootDefaultJustification:ErasureRootPath", "shardIdx", shardIdx, "treeLen", treeLen, "leafHash", leafHash, "path", path, "isFound", isFound, "verified", verified)
	}
	return shardJustifications, nil
}

// Verify T(s,i,H)
func VerifyWBTJustification(treeLen int, root common.Hash, shardIndex uint16, leafHash common.Hash, path []common.Hash) (bool, common.Hash) {
	recoveredRoot, verified, _ := trie.VerifyWBT(treeLen, int(shardIndex), root, leafHash, path)
	if root != recoveredRoot && false {
		//errStr := fmt.Sprintf("VerifyJustification Failure! Expected:%v | Recovered: %v\n", root, recoveredRoot)
		//panic("VerifyJustification")
		return verified, recoveredRoot
	}
	return verified, recoveredRoot
}

// Generating co-path for T(s,i,H)
// s: [(b♣T,s♣T)...] -  sequence of (work-package bundle shard hash, segment shard root) pairs satisfying u = MB(s)
// i: shardIdx or ChunkIdx
// H: Blake2b
func GenerateWBTJustification(root common.Hash, shardIndex uint16, leaves [][]byte) (treeLen int, leafHash common.Hash, path []common.Hash, isFound bool) {
	wbt := trie.NewWellBalancedTree(leaves, types.Blake2b)
	//treeLen, leafHash, path, isFound, nil
	treeLen, leafHash, path, isFound, _ = wbt.Trace(int(shardIndex))
	return treeLen, leafHash, path, isFound
}

func GetOrderedChunks(erasureMeta ECCErasureMap, bECChunks []types.DistributeECChunk, sECChunksArray [][]types.DistributeECChunk) (shardJustifications []types.Justification, orderedBundleShards []types.ConformantECChunk, orderedSegmentShards [][]types.ConformantECChunk) {
	shardJustifications, _ = ErasureRootDefaultJustification(erasureMeta.BClubs, erasureMeta.SClubs)
	orderedBundleShards = ComputeOrderedNPBundleChunks(bECChunks)
	orderedSegmentShards = ComputeOrderedExportedNPChunks(sECChunksArray)
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
	// Padding b to the length of W_G
	paddedB := common.PadToMultipleOfN(b, types.ECPieceSize) // this makes sense
	bLength := len(b)

	chunks, err := n.encode(paddedB, false, bLength)
	if err != nil {
		log.Error(module, "PrepareArbitaryData:encode", "err", err)
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
		log.Error(module, "buildBClub:BuildArbitraryDataChunks", "err", err)
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
			log.Error(debugDA, "buildSClub", "segmentIdx", segmentIdx, "err", err)
		}
		log.Debug(debugDA, "buildSClub", "segmentIdx", segmentIdx, "len", len(erasureCodingSegments))

		if len(erasureCodingSegments) != 1 {
			panic("Invalid segment implementation! NOT OK")
		}
		// Build segment roots from erasureCodingSegments[0] which are the leaves of the segmentData
		segmentTree := trie.NewCDMerkleTree(erasureCodingSegments[0])
		segmentRoots := [][]byte{segmentTree.RootHash().Bytes()}
		// Build the segment chunks
		ecChunks, err := n.BuildExportedSegmentChunks(erasureCodingSegments, segmentRoots)
		if err != nil {
			log.Error(debugDA, "buildSClub:BuildExportedSegmentChunks", "err", err)
		}
		for chunkIdx, ecChunks := range ecChunks {
			shardIdx := uint32(chunkIdx % types.TotalValidators)
			sequentialTranspose[shardIdx] = append(sequentialTranspose[shardIdx], ecChunks.Data)
		}
		log.Debug(debugDA, "buildSClub", "segmentIdx", segmentIdx, "len(ecChunks)", len(ecChunks))

		ecChunksArr = append(ecChunksArr, ecChunks)
	}

	// gathering root per pagrProof, each pageProof can be more than G per our implementation
	pageProofs, _ := trie.GeneratePageProof(segments)
	for pageIdx, pageData := range pageProofs {
		// Encode the data into segments
		erasureCodingPageSegments, err := n.encode(pageData, true, len(pageData)) // Set to false for variable size segments
		if err != nil {
			log.Error(module, "GeneratePageProof", "err", err)
		}

		// Build segment roots -- page can be longer than G long
		log.Debug(debugDA, "GeneratePageProof", "pageIdx", pageIdx, "len", len(erasureCodingPageSegments))
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
			log.Error(debugDA, "BuildExportedSegmentChunks", "err", err)
		}
		for chunkIdx, ecChunks := range ecChunks {
			shardIdx := uint32(chunkIdx % types.TotalValidators)
			sequentialTranspose[shardIdx] = append(sequentialTranspose[shardIdx], ecChunks.Data)
		}
		// this is multiple of totalValidators
		log.Trace(debugDA, "GeneratePageProof", "pageIdx", pageIdx, "len(ecChunks)", len(ecChunks))
		ecChunksArr = append(ecChunksArr, ecChunks)
	}
	log.Trace(debugDA, "GeneratePageProof", "len(ecChunksArr)", len(ecChunksArr))

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

	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		treeLen, leafHash, path, isFound := GenerateWBTJustification(erasureRoot, uint16(shardIdx), bundle_segment_pairs)
		verified, _ := VerifyWBTJustification(treeLen, erasureRoot, uint16(shardIdx), leafHash, path)
		if !verified {
			log.Crit(debugDA, "VerifyWBTJustification ErasureRootPath NOT VERIFIED", "shardIdx", shardIdx, "treeLen", treeLen, "leafHash", leafHash, "path", path, "isFound", isFound)
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

func (n *Node) FetchWorkPackageBundle(expectedWorkPackageHash common.Hash, erasureRoot common.Hash, blength uint32) (pb types.WorkPackageBundle, err error) {
	// now call C138 to get bundle_shard from assurer...
	// and then do ec rescontruction for b
	packageBundle, err := n.reconstructPackageBundleSegments(erasureRoot, blength)
	if err != nil {
		log.Error(debugDA, "FetchWorkPackageBundle:reconstructPackageBundleSegments", "err", err)
		return pb, err
	}
	if packageBundle.PackageHash() != expectedWorkPackageHash {
		return pb, fmt.Errorf("WorkPackageHash mismatch retrieved=%v | expected=%v", packageBundle.PackageHash(), expectedWorkPackageHash)
	}

	return packageBundle, nil
}

func (n *Node) FetchImportSegments(erasureRoot common.Hash, segmentIndices []uint16) (segments [][]byte, err error) {
	// now call C139 to get segment_shard from assurer...
	// and then do ec rescontruction for s
	segments, err = n.reconstructSegments(erasureRoot, segmentIndices)
	if err != nil {
		log.Error(debugDA, "FetchImportSegments:reconstructSegments", "err", err)
		return segments, err
	}
	return segments, nil
}

// The E(p,x,s,j) function is a function that takes a package and its segments and returns a result, in EQ(186)
func (n *Node) CompilePackageBundle(p types.WorkPackage, importSegments [][][]byte, extrinsics types.ExtrinsicsBlobs) types.WorkPackageBundle {

	workItems := p.WorkItems
	workItemCnt := 0
	for _, workItem := range workItems {
		if len(workItem.ImportedSegments) > 0 {
			workItemCnt++
		}
	}

	// p - workPackage
	// x - [extrinsic data] for some workitem argument w
	// extrinsicData := make([]types.ExtrinsicsBlobs, len(workItems))
	// for workIdx, workItem := range workItems {
	// 	extrinsicData[workIdx] = workItem.ExtrinsicsBlobs
	// }

	// s - [ImportSegmentData] should be size of G = W_E * W_S
	importedSegmentData := importSegments
	imports := make([][]byte, 0)
	for _, segment := range importSegments {
		imports = append(imports, segment...)
	}

	// j - justifications
	// (14.14) J(W in I)
	verifyIndex := 0
	justifications := make([][][]common.Hash, 0)
	for itemIndex := range len(importSegments) {
		cdtTree := trie.NewCDMerkleTree(imports)
		//cdtTree.PrintTree()
		tmpJustifications := make([][]common.Hash, 0)
		for i := 0; i < len(importSegments[itemIndex]); i++ {
			//justification, err := cdtTree.Justify(verifyIndex) //J0
			justification, err := cdtTree.GenerateCDTJustificationX(verifyIndex, 0) // supposedly now calling GenerateCDTJustificationX, not Justify
			if err != nil {
				log.Error(debugDA, "CompilePackageBundle:GenerateCDTJustificationX", "err", err)
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
		ExtrinsicData:     extrinsics,
		ImportSegmentData: importedSegmentData,
		Justification:     justifications,
	}
	return workPackageBundle
}

func compareWorkPackages(wp1, wp2 types.WorkPackage) bool {
	// Compare Authorization
	if !common.CompareBytes(wp1.Authorization, wp2.Authorization) {
		log.Error(debugDA, "compareWorkPackages:Authorization mismatch", "wp1", wp1.Authorization, "wp2", wp2.Authorization)
		return false
	}

	// Compare AuthCodeHost
	if wp1.AuthCodeHost != wp2.AuthCodeHost {
		return false
	}

	// Compare Authorizer struct
	if !common.CompareBytes(wp1.AuthorizationCodeHash[:], wp2.AuthorizationCodeHash[:]) {
		return false
	}
	if !common.CompareBytes(wp1.ParameterizationBlob, wp2.ParameterizationBlob) {
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

func (n *Node) GetSegmentRootLookup(wp types.WorkPackage) (segmentRootLookup types.SegmentRootLookup, err error) {
	segmentRootLookupMap := make(map[common.Hash]common.Hash)
	segmentRootLookup = make([]types.SegmentRootLookupItem, 0)
	for _, workItem := range wp.WorkItems {
		for _, importedSegment := range workItem.ImportedSegments {
			importedSegmentRoot, importedPackageHash, err := n.getExportedSegmenstRootFromHash(importedSegment.RequestedHash)
			if err != nil {
				return nil, err
			} else {
				log.Debug(debugDA, "GetSegmentRootLookup:RequestedHash", "importedSegmentRoot", importedSegmentRoot, "importedPackageHash", importedPackageHash)
			}
			_, exists := segmentRootLookupMap[importedSegmentRoot]
			if !exists {
				segmentRootLookupItem := types.SegmentRootLookupItem{
					WorkPackageHash: importedPackageHash,
					SegmentRoot:     importedSegmentRoot,
				}
				segmentRootLookup = append(segmentRootLookup, segmentRootLookupItem)
				segmentRootLookupMap[importedSegmentRoot] = importedPackageHash
			}
		}
	}
	return segmentRootLookup, nil
}

func (n *Node) GetImportedSegmentRoots(wp types.WorkPackage) (importedSegmentRoots []common.Hash, importedPackageHashes []common.Hash, err error) {
	importedSegmentRoots = make([]common.Hash, 0)
	importedPackageHashes = make([]common.Hash, 0)
	for _, workItem := range wp.WorkItems {
		for _, importedSegment := range workItem.ImportedSegments {
			importedSegmentRoot, importedPackageHash, err := n.getExportedSegmenstRootFromHash(importedSegment.RequestedHash)
			if err != nil {
				return nil, nil, err
			}

			log.Trace(debugDA, "GetImportedSegmentRoots", "importedSegmentRoot", importedSegmentRoot)
			importedSegmentRoots = append(importedSegmentRoots, importedSegmentRoot)
			importedPackageHashes = append(importedPackageHashes, importedPackageHash)
		}
	}
	return importedSegmentRoots, importedPackageHashes, nil
}

func fuzzJustification(package_bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup) (fuzz_importsegments [][][]byte, fuzz_segmentRootLookup types.SegmentRootLookup) {

	fuzz_importsegments = package_bundle.ImportSegmentData
	fuzz_segmentRootLookup = segmentRootLookup

	fuzz_hashing := false
	if fuzz_hashing {
		for workItemIdx, segmentData_i := range package_bundle.ImportSegmentData {
			fakeSegmentData := make([][]byte, len(segmentData_i))
			for j, segmentData_j := range segmentData_i {
				fakeSegmentData[j] = common.Blake2Hash(segmentData_j[:]).Bytes()[:len(segmentData_j)]
			}
			fuzz_importsegments[workItemIdx] = fakeSegmentData
		}

		for idx, lookupItem := range fuzz_segmentRootLookup {

			fuzz_lookupItem := lookupItem
			lookupItem.SegmentRoot = common.Blake2Hash(lookupItem.SegmentRoot[:])
			lookupItem.WorkPackageHash = common.Blake2Hash(lookupItem.WorkPackageHash[:])

			fuzz_segmentRootLookup[idx] = fuzz_lookupItem
		}
	}
	fuzz_ordering := true
	if fuzz_ordering {
		//reverse the order of the imported segments
		for i, j := 0, len(fuzz_importsegments)-1; i < j; i, j = i+1, j-1 {
			fuzz_importsegments[i], fuzz_importsegments[j] = fuzz_importsegments[j], fuzz_importsegments[i]
		}
		// reserse the order of the segment roots
		for i, j := 0, len(fuzz_segmentRootLookup)-1; i < j; i, j = i+1, j-1 {
			fuzz_segmentRootLookup[i], fuzz_segmentRootLookup[j] = fuzz_segmentRootLookup[j], fuzz_segmentRootLookup[i]
		}
	}
	fuzz_null_both := false
	if fuzz_null_both {
		fuzz_importsegments = make([][][]byte, len(package_bundle.WorkPackage.WorkItems))
		fuzz_segmentRootLookup = make([]types.SegmentRootLookupItem, len(segmentRootLookup))
	}
	return fuzz_importsegments, fuzz_segmentRootLookup
}

// now we only have executeWorkPackageBundle now
func (n *Node) executeWorkPackageBundle(workPackageCoreIndex uint16, package_bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup) (work_report types.WorkReport, err error) {
	importsegments := make([][][]byte, len(package_bundle.WorkPackage.WorkItems))
	if len(package_bundle.Justification) > 0 && len(package_bundle.Justification[0]) > 0 {
		ok, verifyErr := VerifyBundleJustification(package_bundle.ImportSegmentData, package_bundle.Justification, package_bundle.WorkPackage, segmentRootLookup)
		if verifyErr != nil || !ok {
			if verifyErr != nil {
				log.Error(module, "executeWorkPackageBundle:Justification Verification Error", "err", verifyErr)
			} else if !ok {
				log.Error(module, "executeWorkPackageBundle:Justification Verification Not OK")
			}

			wp := package_bundle.WorkPackage
			fallBackImportedSegments, err := n.FetchWorkpackageImportSegments(wp)
			if err != nil {
				return work_report, err
			}
			package_bundle.ImportSegmentData = fallBackImportedSegments
		}
	}

	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()
	workPackage := package_bundle.WorkPackage
	service_index := uint32(workPackage.AuthCodeHost)
	workPackageHash := workPackage.Hash()

	// Import Segments
	for workItemIdx, workItem_segments := range package_bundle.ImportSegmentData {
		log.Debug(debugDA, "[N%d] [workItem#%d] workItem_segments %x\n", n.id, workItemIdx, workItem_segments)
		importsegments[workItemIdx] = workItem_segments
	}
	authcode, authindex, err := n.statedb.GetAuthorizeCode(workPackage)
	if err != nil {
		return
	}
	vm_auth := pvm.NewVMFromCode(authindex, authcode, 0, targetStateDB)
	r := vm_auth.ExecuteAuthorization(workPackage, workPackageCoreIndex)
	p_p := workPackage.ParameterizationBlob
	p_a := common.Blake2Hash(append(authcode, p_p...))

	var segments [][]byte
	for index, workItem := range workPackage.WorkItems {
		imports := make([][]byte, 0)
		if len(workItem.ImportedSegments) > 0 {
			imports = importsegments[index]
		}
		service_index = workItem.Service
		code, ok, err0 := targetStateDB.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if err0 != nil || !ok || len(code) == 0 {
			return work_report, fmt.Errorf("executeWorkPackageBundle: Code not found")
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			log.Crit(module, "executeWorkPackageBundle: Code and CodeHash Mismatch")
		}
		vm := pvm.NewVMFromCode(service_index, code, 0, targetStateDB)

		output, _, exported_segments := vm.ExecuteRefine(uint32(index), workPackage, r, imports, workItem.ExportCount, package_bundle.ExtrinsicData, p_a)
		//fmt.Printf("refine Output.Ok=%x\n", output.Ok)
		//fmt.Printf("refine exportsLen=%v vm=%x\n", len(exported_segments), exported_segments)

		expectedSegmentCnt := int(workItem.ExportCount)
		if expectedSegmentCnt != len(exported_segments) {
			log.Crit(module, "executeWorkPackageBundle: ExportCount and ExportedSegments Mismatch", "ExportCount", expectedSegmentCnt, "ExportedSegments", len(exported_segments))
		}
		if expectedSegmentCnt != 0 {
			for i := 0; i < expectedSegmentCnt; i++ {
				segment := common.PadToMultipleOfN(exported_segments[i], types.SegmentSize)
				segments = append(segments, segment)
			}
		}
		result := types.WorkResult{
			ServiceID:   workItem.Service,
			CodeHash:    workItem.CodeHash,
			PayloadHash: common.Blake2Hash(workItem.Payload),
			Gas:         9111,
			Result:      output,
		}
		results = append(results, result)

		o := types.AccumulateOperandElements{
			Results:         result.Result,
			Payload:         result.PayloadHash,
			WorkPackageHash: workPackageHash,
			AuthOutput:      r.Ok,
		}
		log.Debug(debugDA, "DA: WrangledResults", "n", types.DecodedWrangledResults(&o))
	}
	//fmt.Printf("Len exportSegments=%d, data=%x\n", len(segments), segments)
	spec, erasureMeta, bECChunks, sECChunksArray := n.NewAvailabilitySpecifier(workPackageHash, workPackage, segments, package_bundle.ExtrinsicData)

	workReport := types.WorkReport{
		AvailabilitySpec:  *spec,
		RefineContext:     workPackage.RefineContext,
		CoreIndex:         workPackageCoreIndex,
		AuthorizerHash:    p_a,
		AuthOutput:        r.Ok,
		SegmentRootLookup: segmentRootLookup,
		Results:           results,
	}
	log.Debug(debugDA, "executeWorkPackageBundle", "n", n.id, "workReport", workReport.String())
	log.Debug(debugG, "executeWorkPackageBundle", "workreporthash", common.Str(workReport.Hash()), "erasureRoot", spec.ErasureRoot)
	n.StoreMeta_Guarantor(spec, erasureMeta, bECChunks, sECChunksArray)

	return workReport, err
}

// importSegments is a 3D array of [workItemIndex][importedSegmentIndex][segmentBytes]
func (n *Node) FetchWorkpackageImportSegments(workPackage types.WorkPackage) (importSegments [][][]byte, err error) {

	log.Trace(debugDA, "[N%d] FetchWorkpackageImportSegments wp=%v (with %v items)\n", n.id, workPackage.Hash(), len(workPackage.WorkItems))
	for workItemIdx, workItem := range workPackage.WorkItems {
		if len(workItem.ImportedSegments) > 0 {
			for ImportedSegmentIdx, ImportedSegment := range workItem.ImportedSegments {
				log.Trace(debugDA, "[N%d] wp-idx=%v-%d (H,I)=(%v,%v)\n", n.id, workPackage.Hash(), workItemIdx, ImportedSegment.RequestedHash, ImportedSegmentIdx)
			}
		} else {
			log.Trace(debugDA, "[N%d] wp-idx=%v-%d no ImportedSegments\n", n.id, workPackage.Hash(), workItemIdx)
		}
	}

	// Check if there are any imported segments need to be fetched
	needFetch := false
	importSegments = make([][][]byte, len(workPackage.WorkItems))
	for _, workItem := range workPackage.WorkItems {
		if len(workItem.ImportedSegments) != 0 {
			needFetch = true
			// return importsegments, nil //MK: this seems wrong!
		}
	}
	if !needFetch {
		// @mk - pls verify the rest of the code 2025/2/20
		return importSegments, nil
	}

	// Colloect all unique erasureRoots
	erasureRoots := make([]common.Hash, 0)
	for _, workItem := range workPackage.WorkItems {
		for _, ImportedSegment := range workItem.ImportedSegments {
			erasureRoot, _ := n.ErasureRootLookUP(ImportedSegment.RequestedHash)
			if !common.HashContains(erasureRoots, erasureRoot) {
				erasureRoots = append(erasureRoots, erasureRoot)
			}
		}
	}

	// the mapping of workItem -> erasureRoots -> indices (for remap the result of reconstruct)
	workItemErasureRootsMapping := make([]map[common.Hash][]uint16, len(workPackage.WorkItems))

	//the mapping of workPackageHashes -> indices (for make request)
	erasureRootsMapping := make(map[common.Hash][]uint16, len(workPackage.WorkItems))

	// the mapping of erasureRoots -> segments positions
	for i, workItem := range workPackage.WorkItems {
		packageIdicesMap := make(map[common.Hash][]uint16, len(workItem.ImportedSegments))
		for _, ImportedSegment := range workItem.ImportedSegments {
			currentIndex := uint16(ImportedSegment.Index)
			// TODO: everything should standardized to erasureRoot ImportedSegment.WorkPackageHash can be [exportedSegmentRoot, erasureRoot and WorkPackageHash]
			// We ask question using erasureRoot only and mapping is portentially needed for work report
			erasureRoot, _ := n.ErasureRootLookUP(ImportedSegment.RequestedHash)
			packageIdicesMap[erasureRoot] = append(packageIdicesMap[erasureRoot], currentIndex)
			if !common.Uint16Contains(erasureRootsMapping[ImportedSegment.RequestedHash], currentIndex) {
				erasureRootsMapping[erasureRoot] = append(erasureRootsMapping[erasureRoot], currentIndex)
			}
		}
		workItemErasureRootsMapping[i] = packageIdicesMap
	}
	log.Trace(debugDA, "[N%d] workItemErasureRootsMapping: %v\n", n.id, workItemErasureRootsMapping)

	// Yse makerequerst to fetch the segments by erasureRoots and indices
	receiveSegmentMapping := make(map[common.Hash][][]byte, len(erasureRoots))
	for erasureRoot, indices := range erasureRootsMapping {
		receiveSegments, err := n.reconstructSegments(erasureRoot, indices)
		if err != nil {
			log.Error(debugDA, "reconstructSegments", "err", err)
			return importSegments, err
		}
		receiveSegmentMapping[erasureRoot] = receiveSegments
	}

	// Remap the segments to [workItenIndex][importedSegmentIndex][bytes]
	for workItemIndex, erasureRootMapping := range workItemErasureRootsMapping {
		for erasureRoot, indices := range erasureRootMapping {
			receivedSegments, exists := receiveSegmentMapping[erasureRoot]
			if !exists {
				log.Error(debugDA, "Missing segments for erasureRoot %v\n", erasureRoot)
				continue
			}
			for _, index := range indices {
				if int(index) >= len(receivedSegments) {
					log.Error(debugDA, "Index out of range: %d for erasureRoot: %v\n", index, erasureRoot)
					continue
				}
				importSegments[workItemIndex] = append(importSegments[workItemIndex], receivedSegments[index])
			}
		}
	}
	log.Trace(debugDA, "[N%d] WP=%v Final importSegments: %x\n", n.id, workPackage.Hash(), importSegments)

	return importSegments, nil
}

// item 1 => 1 segment
// item 2 => 0 segment
// importSegments [][][]byte
// importSegments [0][1][x]byte=>item 1 => 1 segment
// importSegments [1][0][0]byte=>item 2 => 0 segment

// item 1 => 1 segment
// item 2 => 1 segment
// importSegments [][][]byte
// importSegments [0][1][x]byte=>item 1 => 1 segment
// importSegments [1][1][x]byte=>item 2 => 1 segment

func VerifyBundleJustification(importSegments [][][]byte, justifications [][][]common.Hash, workPackage types.WorkPackage, segmentRootLookup types.SegmentRootLookup) (ok bool, err error) {
	// Verify the justifications
	// if !CheckSegmentJustificationSize(importSegments, justifications) {
	// 	return false, fmt.Errorf("importSegments and justification length mismatch")
	// }
	verifyIndex := 0
	for itemIndex, workItem := range workPackage.WorkItems {
		for segmentIdx := range justifications[itemIndex] {
			log.Trace(debugDA, "VerifyBundleJustification itemIndex %v, segmentIdx %v, importSegments[%x]\n", itemIndex, segmentIdx, importSegments[itemIndex])
			segmentData := importSegments[itemIndex][segmentIdx]
			segmentHash := common.ComputeLeafHash_WBT_Blake2B(segmentData)
			importWorkPackageHash := workItem.ImportedSegments[segmentIdx].RequestedHash
			root, err := GetExportSegmentRootByWorkPackageHash(segmentRootLookup, importWorkPackageHash)
			if err != nil {
				return false, err
			}
			transferJustifications := make([][]byte, 0)
			for _, justification := range justifications[itemIndex][segmentIdx] {
				transferJustifications = append(transferJustifications, justification[:])
			}
			computedRoot := trie.VerifyCDTJustificationX(segmentHash[:], verifyIndex, transferJustifications, 0) // replaced from VerifyJustification0
			if !common.CompareBytes(root[:], computedRoot) {
				log.Trace(debugDA, "segmentData %x, segmentHash %v, transferJustifications %x\n", segmentData, segmentHash, transferJustifications)
				log.Trace(debugDA, "expected root %x, computed root %x\n", root[:], computedRoot)
				return false, fmt.Errorf("justification failure")
			}
			verifyIndex++
		}
	}
	return true, nil
}

// Check importSegments and justifications
func CheckSegmentJustificationSize(importSegments [][][]byte, justifications [][][]common.Hash) bool {
	if len(importSegments) != len(justifications) {
		return false
	}
	for i := range importSegments {
		if len(importSegments[i]) != len(justifications[i]) {
			return false
		}
		// MK review this part is this needed?
		if len(importSegments[i]) == 0 || len(importSegments[i][0]) == 0 {
			return false
		}
		for j := range importSegments[i] {
			if len(importSegments[i][j]) == 0 {
				return false
			}
		}
	}
	return true
}

func GetExportSegmentRootByWorkPackageHash(segmentRootLookup types.SegmentRootLookup, workPackageHash common.Hash) (exportedSegmentRoot common.Hash, err error) {
	for _, segmentRootLookupItem := range segmentRootLookup {
		if segmentRootLookupItem.WorkPackageHash == workPackageHash {
			return segmentRootLookupItem.SegmentRoot, nil
		}
	}
	return common.Hash{}, fmt.Errorf("exported segment root not found")
}
