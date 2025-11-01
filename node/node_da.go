package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"time"

	bls "github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	trie "github.com/colorfulnotion/jam/trie"
	types "github.com/colorfulnotion/jam/types"
)

const (
	debugSpec = false
)

type AvailabilitySpecifierDerivation struct {
	BClubs        []common.Hash             `json:"bClubs"`
	SClubs        []common.Hash             `json:"sClubs"`
	BundleChunks  []types.DistributeECChunk `json:"bundle_chunks"`
	SegmentChunks []types.DistributeECChunk `json:"segment_chunks"`
}

func (n *NodeContent) NewAvailabilitySpecifier(package_bundle types.WorkPackageBundle, export_segments [][]byte) (availabilityspecifier *types.AvailabilitySpecifier, d AvailabilitySpecifierDerivation) {
	// compile wp into b
	b := package_bundle.Bytes() // check
	// Build b♣ and s♣
	bClubs, bEcChunks := n.buildBClub(b)
	sClubs, sEcChunksArr := n.buildSClub(export_segments)

	// for segIdx, seg := range export_segments {
	// 	segmentHash := trie.ComputeLeaf(seg) //Blake2Hash(“leaf” || segment)
	// 	fmt.Printf("Exported Segment %d (H=%x): %x\n", segIdx, segmentHash, seg)
	// }

	// ExportedSegmentRoot = CDT(segments)
	exportedSegmentTree := trie.NewCDMerkleTree(export_segments)
	log.Debug(log.G, "executeWorkPackageBundle", "n", n.String(), "exportedSegmentLength", len(export_segments), "exportedSegmentRoot", exportedSegmentTree.RootHash())
	//exportedSegmentTree.PrintTree()
	log.Trace(log.G, "executeWorkPackageBundle", "n", n.String(), "bClubs", fmt.Sprintf("%v", bClubs), "sClubs", fmt.Sprintf("%v", sClubs))

	d = AvailabilitySpecifierDerivation{
		BClubs:        bClubs,
		SClubs:        sClubs,
		BundleChunks:  bEcChunks,
		SegmentChunks: sEcChunksArr,
	}

	log.Trace(log.G, "executeWorkPackageBundle", "derivation", d)
	availabilitySpecifier := types.AvailabilitySpecifier{
		WorkPackageHash:       package_bundle.WorkPackage.Hash(),
		BundleLength:          uint32(len(b)),
		ErasureRoot:           generateErasureRoot(bClubs, sClubs), // u = (bClub, sClub)
		ExportedSegmentRoot:   exportedSegmentTree.RootHash(),
		ExportedSegmentLength: uint16(len(export_segments)),
	}

	return &availabilitySpecifier, d
}

// this is the default justification from (b,s) to erasureRoot
func ErasureRootDefaultJustification(b []common.Hash, s []common.Hash) (shardJustifications []types.Justification, err error) {
	shardJustifications = make([]types.Justification, types.TotalValidators)
	erasureTree, _ := GenerateErasureTree(b, s)
	erasureRoot := erasureTree.RootHash()
	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		treeLen, leaf, path, _, _ := erasureTree.Trace(shardIdx)
		verified, _ := VerifyWBTJustification(treeLen, erasureRoot, uint16(shardIdx), leaf, path, "ErasureRootDefaultJustification")
		if !verified {
			return shardJustifications, fmt.Errorf("verifyWBTJustification Failure")
		}
		shardJustifications[shardIdx] = types.Justification{
			Root:     erasureRoot,
			ShardIdx: shardIdx,
			TreeLen:  types.TotalValidators,
			LeafHash: leaf,
			Path:     path,
		}
	}
	return shardJustifications, nil
}

// Verify T(s,i,H)
func VerifyWBTJustification(treeLen int, root common.Hash, shardIndex uint16, leafHash []byte, path [][]byte, caller string) (bool, common.Hash) {
	recoveredRoot, verified, _ := trie.VerifyWBT(treeLen, int(shardIndex), root, leafHash, path)
	encodedPath, _ := common.EncodeJustification(path, types.NumECPiecesPerSegment)
	reversedEncodedPath, _ := common.EncodeJustification((common.ReversedByteArray(path)), types.NumECPiecesPerSegment)
	if root != recoveredRoot {
		log.Warn(log.Node, "VerifyWBTJustification Failure Part.A", "caller", caller, "shardIdx", shardIndex, "Expected", root, "recovered", recoveredRoot, "verified", verified, "treeLen", treeLen, "leafHash", fmt.Sprintf("%x", leafHash), "path", fmt.Sprintf("%x", path))
		log.Warn(log.Node, "VerifyWBTJustification Failure Part.B", "caller", caller, "shardIdx", shardIndex, "Expected", root, "encodedPath", common.Bytes2String(encodedPath), "reversedEncodedPath", common.Bytes2String(reversedEncodedPath))
		return false, recoveredRoot
	}
	log.Trace(log.Node, "VerifyWBTJustification Success", "caller", caller, "shardIdx", shardIndex, "Expected", root, "recovered", recoveredRoot, "verified", verified, "treeLen", treeLen, "leafHash", fmt.Sprintf("%x", leafHash), "path", fmt.Sprintf("%x", path))
	return true, recoveredRoot
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
func (n *NodeContent) buildBClub(b []byte) ([]common.Hash, []types.DistributeECChunk) {
	// Padding b to the length of W_G
	paddedB := common.PadToMultipleOfN(b, types.ECPieceSize) // this makes sense

	if debugSpec {
		fmt.Printf("Padded %d bytes to %d bytes (multiple of %d bytes) => %x\n", len(b), len(paddedB), types.ECPieceSize, paddedB)
	}

	// instead of a tower of abstraction, collapse it to the minimal number of lines
	chunks, err := bls.Encode(paddedB, types.TotalValidators)
	if err != nil {
		log.Error(log.Node, "buildBclub", "err", err)
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

func (n *NodeContent) buildSClub(segments [][]byte) (sClub []common.Hash, ecChunksArr []types.DistributeECChunk) {
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
			log.Error(log.DA, "buildSClub", "segmentIdx", segmentIdx, "err", err)
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
		log.Error(log.DA, "GeneratePageProof", "Error", pageProofGenerationErr)
	}
	for pageIdx, pagedProofByte := range pageProofs {
		if paranoidVerification {
			tree := trie.NewCDMerkleTree(segments)
			global_segmentsRoot := tree.Root()
			decodedData, _, decodingErr := types.Decode(pagedProofByte, reflect.TypeOf(types.PageProof{}))
			if decodingErr != nil {
				log.Error(log.DA, "buildSClub Proof decoding err", "Error", decodingErr)
			}
			recoveredPageProof := decodedData.(types.PageProof)
			for subTreeIdx := 0; subTreeIdx < len(recoveredPageProof.LeafHashes); subTreeIdx++ {
				leafHash := recoveredPageProof.LeafHashes[subTreeIdx]
				pageSize := 1 << trie.PageFixedDepth
				index := pageIdx*pageSize + subTreeIdx
				fullJustification, err := trie.PageProofToFullJustification(pagedProofByte, pageIdx, subTreeIdx)
				if err != nil {
					log.Error(log.DA, "buildSClub PageProofToFullJustification ERR", "Error", err)
				}
				derived_global_segmentsRoot := trie.VerifyCDTJustificationX(leafHash.Bytes(), index, fullJustification, 0)
				if !common.CompareBytes(derived_global_segmentsRoot, global_segmentsRoot) {
					log.Error(log.DA, "buildSClub fullJustification Root hash mismatch", "expected", fmt.Sprintf("%x", global_segmentsRoot), "got", fmt.Sprintf("%x", derived_global_segmentsRoot))
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
			chunks[n] = ec.Data[n*chunkSize : (n+1)*chunkSize]
		}
		t := trie.NewWellBalancedTree(chunks, types.Blake2b)
		sClub[shardIndex] = common.BytesToHash(t.Root())
	}

	return sClub, ecChunksArr
}

func GenerateErasureTree(bClubs []common.Hash, sClubs []common.Hash) (*trie.WellBalancedTree, [][]byte) {
	// Combine b♣, s♣ into 64bytes pairs
	bundle_segment_pairs := common.BuildBundleSegmentPairs(bClubs, sClubs)

	// Generate and return erasureroot
	t := trie.NewWellBalancedTree(bundle_segment_pairs, types.Blake2b)
	if debugSpec {
		fmt.Printf("\nWBT of bclub-sclub pairs:\n")
		t.PrintTree()
	}
	return t, bundle_segment_pairs
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

// Verify the justifications (picked out of PageProofs) for the imported segments, which can come from different work packages
func (n *NodeContent) VerifyBundle(b *types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, eventID uint64) (verified bool, err error) {
	// verify the segments with CDT_6 justification included by first guarantor
	for itemIndex, workItem := range b.WorkPackage.WorkItems {
		importedSegments := b.ImportSegmentData[itemIndex]
		if len(importedSegments) != len(workItem.ImportedSegments) {
			return false, fmt.Errorf(" VerifyBundle %d != %d", len(importedSegments), len(workItem.ImportedSegments))
		}
		for segmentIdx, i := range workItem.ImportedSegments {
			exportedSegmentRoot := i.RequestedHash
			for _, x := range segmentRootLookup {
				if x.WorkPackageHash == i.RequestedHash {
					exportedSegmentRoot = x.SegmentRoot

					// Telemetry: Work package hash mapped to segments-root for segment recovery
					// Emit WorkPackageHashMapped (event 160)
					n.telemetryClient.WorkPackageHashMapped(eventID, x.WorkPackageHash, x.SegmentRoot)
				}
				// Also emit SegmentsRootMapped (event 161) - mapping segments-root to erasure-root
				// Note: In this context, we don't have direct access to the erasure root,
				// but this would typically be the availability spec's erasure root
				// For now, we'll emit a second mapping event to indicate the segments-root usage
				// TODO: n.telemetryClient.SegmentsRootMapped(eventID, x.SegmentRoot, i.RequestedHash)

			}
			// requestedHash MUST map to exportedSegmentRoot
			segmentData := importedSegments[segmentIdx]
			global_segmentsRoot := trie.VerifyCDTJustificationX(trie.ComputeLeaf(segmentData), int(i.Index), b.Justification[itemIndex][segmentIdx], 0)
			if !common.CompareBytes(exportedSegmentRoot[:], global_segmentsRoot) {
				log.Warn(log.Node, "trie.VerifyCDTJustificationX NOT VERIFIED", "index", i.Index)
				return false, fmt.Errorf("justification failure computedRoot %s != exportedSegmentRoot %s", exportedSegmentRoot, exportedSegmentRoot)
			} else {
				log.Trace(log.DA, "VerifyBundle: Justification Verified", "index", i.Index, "exportedSegmentRoot", exportedSegmentRoot)
			}
		}
	}

	return true, nil
}

// authorizeWP executes the authorization step for a work package
func (n *NodeContent) authorizeWP(workPackage types.WorkPackage, workPackageCoreIndex uint16, targetStateDB *statedb.StateDB, pvmBackend string) (r types.Result, p_a common.Hash, authGasUsed int64, err error) {
	authcode, _, authindex, err := n.statedb.GetAuthorizeCode(workPackage)
	if err != nil {
		return
	}

	vm_auth := statedb.NewVMFromCode(authindex, authcode, 0, 0, targetStateDB, pvmBackend, types.IsAuthorizedGasAllocation)
	if vm_auth == nil {
		err = fmt.Errorf("authorizeWP: failed to create VM for authorization (corrupted bytecode?)")
		return
	}

	r = vm_auth.ExecuteAuthorization(workPackage, workPackageCoreIndex)

	p_u := workPackage.AuthorizationCodeHash
	p_p := workPackage.ConfigurationBlob
	p_a = common.Blake2Hash(append(p_u.Bytes(), p_p...))
	authGasUsed = int64(types.IsAuthorizedGasAllocation) - vm_auth.GetGas()

	return
}

// BuildBundle maps a work package into a work package bundle which is LIKE executeWorkPackageBundle, but does NOT require ExportCount + importedSegments to be pre-set,
// Instead, it updates the workpackage work items: (1)  ExportCount ImportedSegments with a special HostFetchWitness call

func (n *NodeContent) BuildBundle(workPackage types.WorkPackage, extrinsicsBlobs []types.ExtrinsicsBlobs, coreIndex uint16) (b *types.WorkPackageBundle, wr *types.WorkReport, err error) {
	wp := workPackage.Clone()

	targetStateDB := n.getPVMStateDB()
	currentStateRoot := targetStateDB.GetStateRoot()

	authorization, p_a, authGasUsed, err := n.authorizeWP(wp, coreIndex, targetStateDB, n.pvmBackend)
	if err != nil {
		return nil, nil, err
	}

	results := []types.WorkDigest{}
	var segments [][]byte

	for index, workItem := range wp.WorkItems {
		code, ok, err0 := targetStateDB.ReadServicePreimageBlob(workItem.Service, workItem.CodeHash)
		if err != nil || !ok || len(code) == 0 {
			return nil, nil, fmt.Errorf("buildBundle:ReadServicePreimageBlob:s_id %v, codehash %v, err %v, ok=%v", workItem.Service, workItem.CodeHash, err0, ok)
		}
		vm := statedb.NewVMFromCode(workItem.Service, code, 0, 0, targetStateDB, n.pvmBackend, workItem.RefineGasLimit)
		if vm == nil {
			return nil, nil, fmt.Errorf("buildBundle:NewVMFromCode:s_id %v, codehash %v, err %v, ok=%v", workItem.Service, workItem.CodeHash, err0, ok)
		}
		vm.Timeslot = n.statedb.JamState.SafroleState.Timeslot
		vm.SetPVMContext(log.FirstGuarantorOrAuditor)
		importsegments := make([][][]byte, len(wp.WorkItems))
		result, _, exported_segments := vm.ExecuteRefine(coreIndex, uint32(index), wp, authorization, importsegments, 0, extrinsicsBlobs[index], p_a, common.BytesToHash(trie.H0))

		importedSegments, witnesses, err := vm.GetBuilderWitnesses()
		if err != nil {
			log.Warn(log.DA, "BuildBundle: GetBuilderWitnesses failed", "err", err)
			return nil, nil, err
		}
		// tx_count is the number of transaction extrinsics (before adding witnesses)
		txCount := uint32(len(wp.WorkItems[index].Extrinsics))
		wp.WorkItems[index].ExportCount = uint16(len(exported_segments))
		wp.WorkItems[index].ImportedSegments = importedSegments
		// Append witnesses to extrinsicsBlobs and update witness_count in PayloadTransaction
		witnessCount := uint16(len(witnesses))
		for _, witness := range witnesses {
			witnessBytes := witness.SerializeWitness()
			extrinsicsBlobs[index] = append(extrinsicsBlobs[index], witnessBytes)
			witnessExtrinsic := types.WorkItemExtrinsic{
				Hash: common.Blake2Hash(witnessBytes),
				Len:  uint32(len(witnessBytes)),
			}
			wp.WorkItems[index].Extrinsics = append(wp.WorkItems[index].Extrinsics, witnessExtrinsic)
		}
		if len(wp.WorkItems[index].Payload) >= 1 && bytes.Equal(wp.WorkItems[index].Payload[:1], types.PayloadTransactions) {
			newPayload := make([]byte, 7)
			newPayload[0] = 'T'
			binary.LittleEndian.PutUint32(newPayload[1:5], txCount)
			binary.LittleEndian.PutUint16(newPayload[5:7], witnessCount)
			wp.WorkItems[index].Payload = newPayload
		}

		// Append exported segments (append slice directly)
		segments = append(segments, exported_segments...)

		// Calculate total extrinsic bytes
		totalExtrinsicBytes := 0
		for _, e := range extrinsicsBlobs[index] {
			totalExtrinsicBytes += len(e)
		}

		// Store result for work report
		results = append(results, types.WorkDigest{
			ServiceID:           workItem.Service,
			CodeHash:            workItem.CodeHash,
			PayloadHash:         common.Blake2Hash(workItem.Payload),
			Gas:                 workItem.AccumulateGasLimit,
			GasUsed:             uint(workItem.RefineGasLimit - uint64(vm.GetGas())),
			NumImportedSegments: uint(len(importedSegments)),
			NumExportedSegments: uint(len(exported_segments)),
			NumExtrinsics:       uint(len(extrinsicsBlobs[index])),
			NumBytesExtrinsics:  uint(totalExtrinsicBytes),
			Result:              result,
		})
	}

	// Use buildBundle to fetch imported segments and justifications
	wpQueueItem := &WPQueueItem{
		workPackage: wp,
		coreIndex:   coreIndex,
		extrinsics:  extrinsicsBlobs[0], // buildBundle expects single ExtrinsicsBlobs
	}
	bundle, _, err := n.buildBundle(wpQueueItem)
	if err != nil {
		log.Warn(log.DA, "BuildBundle: buildBundle failed", "err", err)
		return nil, nil, err
	}

	// Update ExtrinsicData with all work items (buildBundle only handles first work item)
	bundle.ExtrinsicData = extrinsicsBlobs

	// BuildBundle creates its own RefineContext with the targetStateDB state root (after execution)
	bundle.WorkPackage.RefineContext = types.RefineContext{
		Anchor:           targetStateDB.GetHeaderHash(),
		StateRoot:        currentStateRoot,
		BeefyRoot:        common.Hash{}, // TODO: set correctly
		LookupAnchor:     targetStateDB.GetHeaderHash(),
		LookupAnchorSlot: targetStateDB.JamState.SafroleState.Timeslot,
		Prerequisites:    []common.Hash{},
	}

	// Create AvailabilitySpecifier and WorkReport
	spec, _ := n.NewAvailabilitySpecifier(bundle, segments)
	workReport := &types.WorkReport{
		AvailabilitySpec:  *spec,
		RefineContext:     bundle.WorkPackage.RefineContext,
		CoreIndex:         uint(coreIndex),
		AuthorizerHash:    p_a,
		Trace:             authorization.Ok,
		SegmentRootLookup: types.SegmentRootLookup{}, // BuildBundle doesn't need segment root lookup
		Results:           results,
		AuthGasUsed:       uint(authGasUsed),
	}

	return &bundle, workReport, nil
}

// executeWorkPackageBundle can be called by a guarantor OR an auditor -- the caller MUST do  VerifyBundle call prior to execution (verifying the imported segments)
// If eventID is non-zero, telemetry events for Authorized and Refined will be emitted
func (n *NodeContent) executeWorkPackageBundle(workPackageCoreIndex uint16, package_bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, slot uint32, firstGuarantorOrAuditor bool, eventID uint64) (work_report types.WorkReport, d AvailabilitySpecifierDerivation, elapsed uint32, bundleSnapshot *types.WorkPackageBundleSnapshot, err error) {
	importsegments := make([][][]byte, len(package_bundle.WorkPackage.WorkItems))
	results := []types.WorkDigest{}
	targetStateDB := n.getPVMStateDB()
	workPackage := package_bundle.WorkPackage

	// Import Segments
	copy(importsegments, package_bundle.ImportSegmentData)

	pvmStart := time.Now()
	authStart := time.Now()
	r, p_a, authGasUsed, err := n.authorizeWP(workPackage, workPackageCoreIndex, targetStateDB, n.pvmBackend)
	authElapsed := time.Since(authStart)
	if authGasUsed < 0 {
		authGasUsed = 0
	}

	// Telemetry: Authorized (event 93)
	authCost := telemetry.IsAuthorizedCost{
		TotalGasUsed: uint64(authGasUsed),
		TotalTimeNs:  uint64(authElapsed.Nanoseconds()),
	}
	n.telemetryClient.Authorized(eventID, authCost)

	if err != nil {
		pvmFailedElapsed := common.Elapsed(pvmStart)
		return work_report, d, pvmFailedElapsed, bundleSnapshot, err
	}

	var segments [][]byte
	refineCosts := make([]telemetry.RefineCost, 0, len(workPackage.WorkItems))
	for index, workItem := range workPackage.WorkItems {
		// map workItem.ImportedSegments into segment
		service_index := workItem.Service
		compileStart := time.Now()
		code, ok, err0 := targetStateDB.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if err0 != nil || !ok || len(code) == 0 {
			pvmFailedElapsed := common.Elapsed(pvmStart)
			return work_report, d, pvmFailedElapsed, bundleSnapshot, fmt.Errorf("executeWorkPackageBundle(ReadServicePreimageBlob):s_id %v, codehash %v, err %v, ok=%v", service_index, workItem.CodeHash, err0, ok)
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			log.Crit(log.Node, "executeWorkPackageBundle: Code and CodeHash Mismatch")
		}
		vm := statedb.NewVMFromCode(service_index, code, 0, 0, targetStateDB, n.pvmBackend, workItem.RefineGasLimit)
		if vm == nil {
			pvmFailedElapsed := common.Elapsed(pvmStart)
			return work_report, d, pvmFailedElapsed, bundleSnapshot, fmt.Errorf("executeWorkPackageBundle: failed to create VM for service %d (corrupted bytecode?)", service_index)
		}
		vm.Timeslot = n.statedb.JamState.SafroleState.Timeslot

		if firstGuarantorOrAuditor {
			vm.SetPVMContext(log.FirstGuarantorOrAuditor)
		} else {
			vm.SetPVMContext(log.OtherGuarantor)
		}
		// 0.7.1 : core index is part of refine args
		execStart := time.Now()
		output, _, exported_segments := vm.ExecuteRefine(workPackageCoreIndex, uint32(index), workPackage, r, importsegments, workItem.ExportCount, package_bundle.ExtrinsicData[index], workPackage.AuthorizationCodeHash, common.BytesToHash(trie.H0))
		execElapsed := time.Since(execStart)
		compileElapsed := time.Since(compileStart)

		expectedSegmentCnt := int(workItem.ExportCount)
		actualSegmentCnt := len(exported_segments)
		segmentCountMismatch := (expectedSegmentCnt != actualSegmentCnt)

		if segmentCountMismatch {
			log.Trace(log.Node, "executeWorkPackageBundle: ExportCount and ExportedSegments Mismatch", "ExportCount", expectedSegmentCnt, "ExportedSegments", actualSegmentCnt, "ExportedSegments", common.FormatPaddedBytesArray(exported_segments, 20))
			// Non-first guarantors/auditors trust the first guarantor's ExportCount
			// and use actual segment count for appending to segments slice
			if !firstGuarantorOrAuditor {
				expectedSegmentCnt = actualSegmentCnt
			}
		}

		// Append exported segments (use actualSegmentCnt for iteration)
		if actualSegmentCnt != 0 {
			for i := 0; i < actualSegmentCnt; i++ {
				segment := common.PadToMultipleOfN(exported_segments[i], types.SegmentSize)
				segments = append(segments, segment)
			}
		}
		z := 0
		for _, extrinsic := range workItem.Extrinsics {
			z += int(extrinsic.Len)
		}
		result := types.WorkDigest{
			ServiceID:           workItem.Service,
			CodeHash:            workItem.CodeHash,
			PayloadHash:         common.Blake2Hash(workItem.Payload),
			Gas:                 workItem.AccumulateGasLimit,
			GasUsed:             uint(workItem.RefineGasLimit - uint64(vm.GetGas())),
			NumImportedSegments: uint(len(workItem.ImportedSegments)),
			NumExportedSegments: uint(expectedSegmentCnt),
			NumExtrinsics: uint(func() int {
				total := 0
				for _, extrinsicBlobs := range package_bundle.ExtrinsicData {
					total += len(extrinsicBlobs)
				}
				return total
			}()),
			NumBytesExtrinsics: uint(z),
		}
		if len(output.Ok)+z > types.MaxEncodedWorkReportSize {
			result.Result.Err = types.WORKDIGEST_OVERSIZE
			result.Result.Ok = nil

			// TODO: renable with BuildBundle witness support
			// } else if segmentCountMismatch {
			// 	// Only first guarantor/auditor flags BAD_EXPORT for mismatched segment counts
			// 	result.Result.Err = types.WORKDIGEST_BAD_EXPORT
			// 	result.Result.Ok = nil
		} else {
			result.Result = output
		}
		results = append(results, result)

		if eventID != 0 {
			gasUsed := workItem.RefineGasLimit - uint64(vm.GetGas())
			refineCosts = append(refineCosts, telemetry.RefineCost{
				TotalGasUsed:      gasUsed,
				TotalTimeNs:       uint64(execElapsed.Nanoseconds()),
				LoadCompileTimeNs: uint64(compileElapsed.Nanoseconds()),
			})
		}

	}

	// Telemetry: Refined (event 94)
	n.telemetryClient.Refined(eventID, refineCosts)

	spec, d := n.NewAvailabilitySpecifier(package_bundle, segments)
	workReport := types.WorkReport{
		AvailabilitySpec:  *spec,
		RefineContext:     workPackage.RefineContext,
		CoreIndex:         uint(workPackageCoreIndex),
		AuthorizerHash:    p_a,
		Trace:             r.Ok,
		SegmentRootLookup: segmentRootLookup,
		Results:           results,
		AuthGasUsed:       uint(authGasUsed),
	}
	log.Trace(log.Node, "executeWorkPackageBundle", "NODE", n.id, "workReport", workReport.String())

	n.StoreBundleSpecSegments(spec, d, package_bundle, segments)
	pvmElapsed := common.Elapsed(pvmStart)
	bundleSnapshot = &types.WorkPackageBundleSnapshot{
		PackageHash:       workReport.GetWorkPackageHash(),
		CoreIndex:         workPackageCoreIndex,
		Bundle:            package_bundle,
		SegmentRootLookup: segmentRootLookup,
		Slot:              slot,
		Report:            workReport,
	}

	// Telemetry: Work-report built (event 102)
	bundleBytes := package_bundle.Bytes()
	workReportOutline := telemetry.WorkReportOutline{
		WorkReportHash: workReport.Hash(),
		BundleSize:     uint32(len(bundleBytes)),
		ErasureRoot:    workReport.AvailabilitySpec.ErasureRoot,
		SegmentsRoot:   workReport.AvailabilitySpec.ExportedSegmentRoot,
	}
	n.telemetryClient.WorkReportBuilt(eventID, workReportOutline)

	return workReport, d, pvmElapsed, bundleSnapshot, err
}
