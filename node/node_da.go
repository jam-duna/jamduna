package node

import (
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

const (
	debugSpec = false
)

func (n *Node) NewAvailabilitySpecifier(package_bundle types.WorkPackageBundle, export_segments [][]byte) (availabilityspecifier *types.AvailabilitySpecifier, bClubs []common.Hash, sClubs []common.Hash, bECChunks []types.DistributeECChunk, sECChunksArray []types.DistributeECChunk) {
	// compile wp into b
	b := package_bundle.Bytes() // check
	// Build b♣ and s♣
	bClubs, bEcChunks := n.buildBClub(b)
	sClubs, sEcChunksArr := n.buildSClub(export_segments)

	// ExportedSegmentRoot = CDT(segments)
	cdt := trie.NewCDMerkleTree(export_segments)

	availabilitySpecifier := types.AvailabilitySpecifier{
		WorkPackageHash:       package_bundle.WorkPackage.Hash(),
		BundleLength:          uint32(len(b)),
		ErasureRoot:           generateErasureRoot(bClubs, sClubs), // u = (bClub, sClub)
		ExportedSegmentRoot:   common.Hash(cdt.Root()),
		ExportedSegmentLength: uint16(len(export_segments)),
	}

	return &availabilitySpecifier, bClubs, sClubs, bEcChunks, sEcChunksArr
}

// this is the default justification from (b,s) to erasureRoot
func ErasureRootDefaultJustification(b []common.Hash, s []common.Hash) (shardJustifications []types.Justification, err error) {
	shardJustifications = make([]types.Justification, types.TotalValidators)
	erasureTree, _ := GenerateErasureTree(b, s)
	erasureRoot := erasureTree.RootHash()
	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		treeLen, leaf, path, isFound, _ := erasureTree.Trace(shardIdx)
		verified, _ := VerifyWBTJustification(treeLen, erasureRoot, uint16(shardIdx), leaf, path)
		if !verified {
			// TEMPORARY
			// return shardJustifications, fmt.Errorf("VerifyWBTJustification Failure")
		}
		shardJustifications[shardIdx] = types.Justification{
			Root:     erasureRoot,
			ShardIdx: shardIdx,
			TreeLen:  types.TotalValidators,
			LeafHash: leaf,
			Path:     path,
		}
		log.Trace(debugDA, "ErasureRootDefaultJustification:ErasureRootPath", "shardIdx", shardIdx, "treeLen", treeLen, "leaf", leaf, "path", path, "isFound", isFound, "verified", verified)
	}
	return shardJustifications, nil
}

// Verify T(s,i,H)
func VerifyWBTJustification(treeLen int, root common.Hash, shardIndex uint16, leafHash []byte, path [][]byte) (bool, common.Hash) {
	recoveredRoot, verified, _ := trie.VerifyWBT(treeLen, int(shardIndex), root, leafHash, path)
	if root != recoveredRoot {
		log.Debug(debugDA, "VerifyJustification Failure : Input", "shardIdx", shardIndex, "treeLen", treeLen, "leafHash", fmt.Sprintf("%x", leafHash), "path", fmt.Sprintf("%x", path))
		errStr := fmt.Sprintf("VerifyJustification Failure! Expected:%v | Recovered: %v\n", root, recoveredRoot)
		fmt.Printf(errStr)
		//panic("VerifyJustification")
		return verified, recoveredRoot
	}
	return true, recoveredRoot // TEMPORARY
}

// Generating co-path for T(s,i,H)
// s: [(b♣T,s♣T)...] -  sequence of (work-package bundle shard hash, segment shard root) pairs satisfying u = MB(s)
// i: shardIdx or ChunkIdx
// H: Blake2b
func GenerateWBTJustification(root common.Hash, shardIndex uint16, leaves [][]byte) (treeLen int, leafHash []byte, path [][]byte, isFound bool) {
	wbt := trie.NewWellBalancedTree(leaves, types.Blake2b)
	// fmt.Printf("GenerateWBTJustification:root %v, shardIndex %v, leaves %x\n", root, shardIndex, leaves)
	// wbt.PrintTree()
	treeLen, leafHash, path, isFound, _ = wbt.Trace(int(shardIndex))
	return treeLen, leafHash, path, isFound
}

// Compute b♣ using the EncodeWorkPackage function
func (n *Node) buildBClub(b []byte) ([]common.Hash, []types.DistributeECChunk) {
	// Padding b to the length of W_G
	paddedB := common.PadToMultipleOfN(b, types.ECPieceSize) // this makes sense

	if debugSpec {
		fmt.Printf("Padded %d bytes to %d bytes (multiple of %d bytes) => %x\n", len(b), len(paddedB), types.ECPieceSize, paddedB)
	}

	// instead of a tower of abstraction, collapse it to the minimal number of lines
	chunks, err := bls.Encode(paddedB, types.TotalValidators)
	if err != nil {
		log.Error(module, "buildBclub", "err", err)
	}

	// Hash each element of the encoded data
	bClubs := make([]common.Hash, types.TotalValidators)
	bundleShards := chunks // this should be of size 1
	ecChunks := make([]types.DistributeECChunk, types.TotalValidators)
	for shardIdx, shard := range bundleShards {
		bClubs[shardIdx] = common.Blake2Hash(shard)
		if debugSpec {
			fmt.Printf("buildBClub hash %d: %s Shard: %x (%d bytes)\n", shardIdx, bClubs[shardIdx], shard, len(shard))
		}
		ecChunks[shardIdx] = types.DistributeECChunk{
			//SegmentRoot: bClubs[shardIdx].Bytes(), // SegmentRoot used to store the hash of the shard
			Data: shard,
		}
	}
	return bClubs, ecChunks
}

func (n *Node) buildSClub(segments [][]byte) (sClub []common.Hash, ecChunksArr []types.DistributeECChunk) {
	ecChunksArr = make([]types.DistributeECChunk, types.TotalValidators)

	// EC encode segments in ecChunksArr
	for segmentIdx, segmentData := range segments {
		if segmentIdx == 0 {
			for i := range types.TotalValidators {
				ecChunksArr[i] = types.DistributeECChunk{
					Data: []byte{},
				}
			}
		}

		// Encode segmentData into leaves
		erasureCodingSegments, err := bls.Encode(segmentData, types.TotalValidators)
		if err != nil {
			log.Error(debugDA, "buildSClub", "segmentIdx", segmentIdx, "err", err)
		}
		for shardIndex, shard := range erasureCodingSegments {
			ecChunksArr[shardIndex].Data = append(ecChunksArr[shardIndex].Data, shard...)
		}
	}

	// now take up to 64 segments at a time and build a page proof
	// IMPORTANT: these pageProofs are provided in OTHER bundles for imported segments
	//   The guarantor who builds the bundle must pull out a specific pageproof and verify it against the correct exported segment root
	pageProofs, pageProofGenerationErr := trie.GeneratePageProof(segments)
	if pageProofGenerationErr != nil {
		log.Error(debugDA, "GeneratePageProof", "Error", pageProofGenerationErr)
	}
	for pageIdx, pagedProofByte := range pageProofs {
		if paranoidVerification {
			tree := trie.NewCDMerkleTree(segments)
			global_segmentsRoot := tree.Root()
			decodedData, _, decodingErr := types.Decode(pagedProofByte, reflect.TypeOf(types.PageProof{}))
			if decodingErr != nil {
				log.Error(debugDA, "buildSClub Proof decoding err", "Error", decodingErr)
			}
			recoveredPageProof := decodedData.(types.PageProof)
			for subTreeIdx := 0; subTreeIdx < len(recoveredPageProof.LeafHashes); subTreeIdx++ {
				leafHash := recoveredPageProof.LeafHashes[subTreeIdx]
				pageSize := 1 << trie.PageFixedDepth
				index := pageIdx*pageSize + subTreeIdx
				fullJustification, err := trie.PageProofToFullJustification(pagedProofByte, pageIdx, subTreeIdx)
				if err != nil {
					log.Error(debugDA, "buildSClub PageProofToFullJustification ERR", "Error", err)
				}
				derived_global_segmentsRoot := trie.VerifyCDTJustificationX(leafHash.Bytes(), index, fullJustification, 0)
				if !common.CompareBytes(derived_global_segmentsRoot, global_segmentsRoot) {
					log.Error(debugDA, "buildSClub fullJustification Root hash mismatch", "expected", fmt.Sprintf("%x", global_segmentsRoot), "got", fmt.Sprintf("%x", derived_global_segmentsRoot))
				}
			}
		}
		paddedProof := common.PadToMultipleOfN(pagedProofByte, types.SegmentSize)
		erasureCodingPageSegments, err := bls.Encode(paddedProof, types.TotalValidators)
		if err != nil {
			return
		}
		for shardIndex, shard := range erasureCodingPageSegments {
			ecChunksArr[shardIndex].Data = append(ecChunksArr[shardIndex].Data, shard...)
		}
	}
	sClub = make([]common.Hash, types.TotalValidators)

	chunkSize := (types.SegmentSize / (types.TotalValidators / 3))
	for shardIndex, ec := range ecChunksArr {
		chunks := make([][]byte, len(segments)+len(pageProofs))
		for n := 0; n < len(chunks); n++ {
			chunks[n] = ec.Data[n*chunkSize : (n+1)*chunkSize] // Michael claims this needs a hash
		}
		t := trie.NewWellBalancedTree(chunks, types.Blake2b)
		sClub[shardIndex] = common.BytesToHash(t.Root())
	}

	return sClub, ecChunksArr
}

func zipPairs(b []common.Hash, s []common.Hash) (pairs [][]byte) {
	pairs = make([][]byte, len(b))
	if len(b) != len(s) {
		return
	}
	for i := 0; i < len(b); i++ {
		pairs[i] = append(b[i].Bytes(), s[i].Bytes()...)
	}
	return pairs
}

func GenerateErasureTree(b []common.Hash, s []common.Hash) (*trie.WellBalancedTree, [][]byte) {
	// Combine b and s into (work-package bundle shard hash, segment shard root) pairs
	bundleSegmentPairs := zipPairs(b, s)

	// Generate and return erasureroot
	t := trie.NewWellBalancedTree(bundleSegmentPairs, types.Blake2b)
	if debugSpec {
		fmt.Printf("\nWBT of bclub-sclub pairs:\n")
		t.PrintTree()
	}
	return t, bundleSegmentPairs
}

// MB([x∣x∈T[b♣,s♣]]) - Encode b♣ and s♣ into a matrix
func generateErasureRoot(b []common.Hash, s []common.Hash) common.Hash {
	erasureTree, _ := GenerateErasureTree(b, s)
	return erasureTree.RootHash()
}

// M(s) - CDT of exportedSegment
func generateExportedSegmentsRoot(segments [][]byte) common.Hash {
	cdt := trie.NewCDMerkleTree(segments)
	return common.Hash(cdt.Root())
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

func (n *Node) GetSegmentRootLookup(wp types.WorkPackage) (segmentRootLookup types.SegmentRootLookup, err error) {
	// TODO: check if this should actually come from n.statedb.JamState.RecentBlocks.Reported.SegmentRootLookup
	segmentRootLookupMap := make(map[common.Hash]common.Hash)
	segmentRootLookup = make([]types.SegmentRootLookupItem, 0)
	for _, workItem := range wp.WorkItems {
		for _, importedSegment := range workItem.ImportedSegments {
			si := n.SpecSearch(importedSegment.RequestedHash)
			if si == nil {
				ferr := fmt.Errorf("GetSegmentRootLookup:SpecSearch NOT FOUND %s", importedSegment.RequestedHash)
				log.Error(debugDA, "GetSegmentRootLookup:SpecSearch", "err", ferr)
				return nil, ferr
			} else {
				log.Debug(debugDA, "GetSegmentRootLookup:RequestedHash", "segmentRoot", si.Spec.ExportedSegmentRoot, "importedPackageHash", si.Spec.WorkPackageHash)
			}
			_, exists := segmentRootLookupMap[si.Spec.ExportedSegmentRoot]
			if !exists {
				segmentRootLookupItem := types.SegmentRootLookupItem{
					WorkPackageHash: si.Spec.WorkPackageHash,
					SegmentRoot:     si.Spec.ExportedSegmentRoot,
				}
				segmentRootLookup = append(segmentRootLookup, segmentRootLookupItem)
				segmentRootLookupMap[si.Spec.ExportedSegmentRoot] = si.Spec.WorkPackageHash
			}
		}
	}
	return segmentRootLookup, nil
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

// Verify the justifications (picked out of PageProofs) for the imported segments, which can come from different work packages
func (n *Node) VerifyBundle(b *types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup) (verified bool, err error) {
	return true, nil
	if len(b.ImportSegmentData) != len(b.Justification) {
		return false, fmt.Errorf("importSegments and justifications length mismatch")
	}

	// verify the segments with CDT_6 justification included by first guarantor
	for itemIndex, workitem_segments := range b.ImportSegmentData {
		for segmentIdx, segmentData := range workitem_segments {
			requestedHash := b.WorkPackage.WorkItems[itemIndex].ImportedSegments[segmentIdx].RequestedHash
			// loop through segmentRootLookup so we get the workpackage hash
			for _, x := range segmentRootLookup {
				if x.SegmentRoot == requestedHash {
					requestedHash = x.WorkPackageHash
				}
			}
			specIndex := n.SpecSearch(requestedHash)
			if specIndex == nil {
				log.Warn(module, "VerifyBundle: SpecSearch NOT FOUND", "reqHash", requestedHash)
				return false, fmt.Errorf("VerifyBundle: could not find %x", requestedHash)
			} else {
				index := b.WorkPackage.WorkItems[itemIndex].ImportedSegments[segmentIdx].Index
				exportedSegmentRoot := specIndex.Spec.ExportedSegmentRoot
				j := b.Justification[itemIndex][segmentIdx]
				leafHash := trie.ComputeLeaf(segmentData)
				global_segmentsRoot := trie.VerifyCDTJustificationX(leafHash, int(index), j, 0)
				if !common.CompareBytes(exportedSegmentRoot[:], global_segmentsRoot) {
					log.Warn(module, "trie.VerifyCDTJustificationX NOT VERIFIED", "index", index)
					return false, fmt.Errorf("justification failure computedRoot %x != exportedSegmentRoot %s (h=%s)", exportedSegmentRoot, exportedSegmentRoot, leafHash)
				} else {
					log.Info(debugDA, "VerifyBundle: Justification Verified", "index", index, "exportedSegmentRoot", exportedSegmentRoot)
				}
			}
		}
	}

	return true, nil
}

// executeWorkPackageBundle can be called by a guarantor OR an auditor -- the caller MUST do  VerifyBundle call prior to execution (verifying the imported segments)
func (n *Node) executeWorkPackageBundle(workPackageCoreIndex uint16, package_bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, firstGuarantor bool) (work_report types.WorkReport, err error) {
	importsegments := make([][][]byte, len(package_bundle.WorkPackage.WorkItems))
	results := []types.WorkResult{}
	targetStateDB := n.getPVMStateDB()
	workPackage := package_bundle.WorkPackage
	service_index := uint32(workPackage.AuthCodeHost)

	// Import Segments
	for workItemIdx, workItem_segments := range package_bundle.ImportSegmentData {
		importsegments[workItemIdx] = workItem_segments
	}
	authcode, _, authindex, err := n.statedb.GetAuthorizeCode(workPackage)
	if err != nil {
		return
	}
	vm_auth := pvm.NewVMFromCode(authindex, authcode, 0, targetStateDB)
	r := vm_auth.ExecuteAuthorization(workPackage, workPackageCoreIndex)
	p_u := workPackage.AuthorizationCodeHash
	p_p := workPackage.ParameterizationBlob
	p_a := common.Blake2Hash(append(p_u.Bytes(), p_p...))

	var segments [][]byte
	for index, workItem := range workPackage.WorkItems {
		service_index = workItem.Service
		code, ok, err0 := targetStateDB.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if err0 != nil || !ok || len(code) == 0 {
			return work_report, fmt.Errorf("executeWorkPackageBundle: Code not found")
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			log.Crit(module, "executeWorkPackageBundle: Code and CodeHash Mismatch")
		}
		vm := pvm.NewVMFromCode(service_index, code, 0, targetStateDB)
		vm.Timeslot = n.statedb.JamState.SafroleState.Timeslot
		vm.SetCore(workPackageCoreIndex, firstGuarantor)
		output, _, exported_segments := vm.ExecuteRefine(uint32(index), workPackage, r, importsegments, workItem.ExportCount, package_bundle.ExtrinsicData, p_a)

		expectedSegmentCnt := int(workItem.ExportCount)
		if expectedSegmentCnt != len(exported_segments) {
			log.Warn(module, "executeWorkPackageBundle: ExportCount and ExportedSegments Mismatch", "ExportCount", expectedSegmentCnt, "ExportedSegments", len(exported_segments), "ExportedSegments", common.FormatPaddedBytesArray(exported_segments, 20))
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
			Gas:         workItem.AccumulateGasLimit, // put a
			Result:      output,
		}
		results = append(results, result)

		o := types.AccumulateOperandElements{
			H: common.Hash{},
			E: common.Hash{},
			A: p_a,
			O: r.Ok,
			Y: result.PayloadHash,
			D: result.Result,
		}
		log.Debug(debugDA, "DA: WrangledResults", "n", types.DecodedWrangledResults(&o))
	}

	spec, bClubs, sClubs, bECChunks, sECChunksArray := n.NewAvailabilitySpecifier(package_bundle, segments)

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
	n.StoreMeta_Guarantor(spec, bClubs, sClubs, bECChunks, sECChunksArray)

	return workReport, err
}

// importSegments is a 3D array of [workItemIndex][importedSegmentIndex][segmentBytes]
func (n *Node) FetchWorkpackageImportSegments(workPackage types.WorkPackage, segmentRootLookup types.SegmentRootLookup) (importSegments [][][]byte, justifications [][][]common.Hash, err error) {
	importSegments = make([][][]byte, len(workPackage.WorkItems))
	justifications = make([][][]common.Hash, len(workPackage.WorkItems))

	// because CE139 requires erasureroot
	erasureRootIndex := make(map[common.Hash]*SpecIndex)
	workItemErasureRootsMapping := make([][]*SpecIndex, len(workPackage.WorkItems))
	for workItemIdx, workItem := range workPackage.WorkItems {
		workItemErasureRootsMapping[workItemIdx] = make([]*SpecIndex, len(workItem.ImportedSegments))
		for idx, ImportedSegment := range workItem.ImportedSegments {
			// these are actually work package hashes or segment roots
			wpi := n.SpecSearch(ImportedSegment.RequestedHash)
			if wpi != nil {
				oldwpi, exists := erasureRootIndex[wpi.Spec.ErasureRoot]
				if exists {
					oldwpi.AddIndex(uint16(ImportedSegment.Index))
					workItemErasureRootsMapping[workItemIdx][idx] = oldwpi
				} else {
					erasureRootIndex[wpi.Spec.ErasureRoot] = wpi
					workItemErasureRootsMapping[workItemIdx][idx] = wpi
					wpi.AddIndex(uint16(ImportedSegment.Index))
				}
			} else {
				log.Warn(module, "SpecSearch returned nil")
				return importSegments, justifications, fmt.Errorf("SpecSearch returned nil")
			}
		}
	}

	// Use makerequest CE139 to fetch the segments by erasureRoots and indices
	receiveSegmentMapping := make(map[common.Hash][][]byte)
	justificationsMapping := make(map[common.Hash][][]common.Hash)
	for erasureRoot, specIndex := range erasureRootIndex {
		receiveSegments, specJustifications, err := n.reconstructSegments(specIndex)
		if err != nil {
			log.Error(debugDA, "reconstructSegments", "err", err)
			return importSegments, justifications, err
		}
		if len(receiveSegments) != len(specIndex.Indices) {
			panic("receiveSegments and specIndex.Indices length mismatch")
		}
		receiveSegmentMapping[erasureRoot] = receiveSegments
		justificationsMapping[erasureRoot] = specJustifications
	}

	// Remap the segments to [workItenIndex][importedSegmentIndex][bytes]
	for workItemIndex, workItem := range workPackage.WorkItems {
		for idx, _ := range workItem.ImportedSegments {
			wpi := workItemErasureRootsMapping[workItemIndex][idx]

			if wpi == nil {
				panic("wpi is nil2")
			} else {
				receivedSegments, exists := receiveSegmentMapping[wpi.Spec.ErasureRoot]
				if !exists {
					log.Error(debugDA, "Missing segments for erasureRoot %v\n", wpi.Spec.ErasureRoot)
					continue
				} else {
					importSegments[workItemIndex] = receivedSegments
					j, existJ := justificationsMapping[wpi.Spec.ErasureRoot]
					if existJ {
						justifications[workItemIndex] = j
					}
				}
			}
		}
	}
	log.Trace(debugDA, "[N%d] WP=%v Final importSegments: %x\n", n.id, workPackage.Hash(), importSegments)

	return importSegments, justifications, nil
}
