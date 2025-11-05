package refine

import (
	"fmt"
	"time"

	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// ExecuteWorkPackageBundle executes a work package bundle and returns a bundle snapshot
func ExecuteWorkPackageBundleV1(stateDB *statedb.StateDB, pvmBackend string, timeslot uint32, workPackageCoreIndex uint16, packageBundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, slot uint32) (types.WorkPackageBundleSnapshot, error) {

	workPackage := packageBundle.WorkPackage

	// Get authorization code
	authcode, _, authindex, err := stateDB.GetAuthorizeCode(workPackage)
	if err != nil {
		return types.WorkPackageBundleSnapshot{}, fmt.Errorf("failed to get authorization code: %v", err)
	}

	// Execute authorization
	vm_auth := statedb.NewVMFromCode(authindex, authcode, 0, 0, stateDB, pvmBackend, types.IsAuthorizedGasAllocation)
	vm_auth.SetPVMContext(log.FirstGuarantorOrAuditor)
	auth_result := vm_auth.ExecuteAuthorization(workPackage, workPackageCoreIndex)
	auth_output := auth_result.Ok
	authGasUsed := int64(types.IsAuthorizedGasAllocation) - vm_auth.GetGas()

	// Compute authorization hash
	p_u := workPackage.AuthorizationCodeHash
	p_p := workPackage.ConfigurationBlob
	authorizer_hash := common.Blake2Hash(append(p_u.Bytes(), p_p...))

	// Refine - Process work items (also need auth_output)
	var segments [][]byte
	importsegments := make([][][]byte, len(packageBundle.WorkPackage.WorkItems))
	copy(importsegments, packageBundle.ImportSegmentData)

	refine_results := []types.WorkDigest{}
	for index, workItem := range workPackage.WorkItems {
		serviceIndex := workItem.Service
		code, ok, err0 := stateDB.ReadServicePreimageBlob(serviceIndex, workItem.CodeHash)
		if err0 != nil || !ok || len(code) == 0 {
			return types.WorkPackageBundleSnapshot{}, fmt.Errorf("failed to read service preimage blob: s_id %v, codehash %v, err %v, ok=%v", serviceIndex, workItem.CodeHash, err0, ok)
		}

		if common.Blake2Hash(code) != workItem.CodeHash {
			return types.WorkPackageBundleSnapshot{}, fmt.Errorf("code and CodeHash mismatch")
		}

		// Execute refine
		vm := statedb.NewVMFromCode(serviceIndex, code, 0, 0, stateDB, pvmBackend, workItem.RefineGasLimit)
		vm.Timeslot = timeslot
		vm.SetPVMContext(log.FirstGuarantorOrAuditor)

		refineStart := time.Now()
		output, _, exported_segments := vm.ExecuteRefine(
			workPackageCoreIndex,
			uint32(index), workPackage, auth_result, importsegments,
			workItem.ExportCount, packageBundle.ExtrinsicData[index],
			p_u, common.BytesToHash(trie.H0),
		)
		refineElapsed := time.Since(refineStart)
		if false {
			fmt.Printf("%sΨ.R_%d: %v%s\n", common.ColorGray, index, refineElapsed, common.ColorReset)
		}

		// Process exported segments
		expectedSegmentCnt := int(workItem.ExportCount)
		if expectedSegmentCnt != len(exported_segments) {
			expectedSegmentCnt = len(exported_segments)
		}
		if expectedSegmentCnt != 0 {
			for i := 0; i < expectedSegmentCnt; i++ {
				segment := common.PadToMultipleOfN(exported_segments[i], types.SegmentSize)
				segments = append(segments, segment)
			}
		}

		// Create work digest
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
			NumExtrinsics:       uint(len(packageBundle.ExtrinsicData)),
			NumBytesExtrinsics:  uint(z),
		}

		if len(output.Ok)+z > types.MaxEncodedWorkReportSize {
			result.Result.Err = types.WORKDIGEST_OVERSIZE
			result.Result.Ok = nil
		} else if expectedSegmentCnt != len(exported_segments) {
			result.Result.Err = types.WORKDIGEST_BAD_EXPORT
			result.Result.Ok = nil
		} else {
			result.Result = output
		}
		refine_results = append(refine_results, result)
	}

	// Create availability specifier using standardized logic
	specStart := time.Now()
	spec := MakeAvailabilitySpecifier(packageBundle, segments)
	specElapsed := time.Since(specStart)
	fmt.Printf("%sGen Spec: %v%s\n", common.ColorGray, specElapsed, common.ColorReset)

	// Create work report
	workReport := types.WorkReport{
		AvailabilitySpec:  *spec,
		RefineContext:     workPackage.RefineContext,
		CoreIndex:         uint(workPackageCoreIndex),
		AuthorizerHash:    authorizer_hash,
		Trace:             auth_output,
		SegmentRootLookup: segmentRootLookup,
		Results:           refine_results,
		AuthGasUsed:       uint(authGasUsed),
	}

	// Create bundle snapshot
	snapshot := types.WorkPackageBundleSnapshot{
		PackageHash:       packageBundle.PackageHash(),
		CoreIndex:         workPackageCoreIndex,
		Bundle:            packageBundle,
		SegmentRootLookup: segmentRootLookup,
		Slot:              slot,
		Report:            workReport,
	}

	return snapshot, nil
}

func ExecuteWorkPackageBundleV2(stateDB *statedb.StateDB, pvmBackend string, timeslot uint32, workPackageCoreIndex uint16, packageBundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, slot uint32) (types.WorkPackageBundleSnapshot, error) {

	// Step 1 - Execute authorization
	authStart := time.Now()
	auth_result, auth_output, authGasUsed, authorizer_hash, authErr := ExecuteWPAuthorization(stateDB, pvmBackend, packageBundle.WorkPackage, workPackageCoreIndex)
	if authErr != nil {
		return types.WorkPackageBundleSnapshot{}, authErr
	}
	authElapsed := time.Since(authStart)
	fmt.Printf("%sΨ.Auth: %v%s\n", common.ColorGray, authElapsed, common.ColorReset)

	// Step 2 - Execute refinement
	refineStart := time.Now()
	refine_results, raw_exported_segments, refineErr := ExecuteWBRefinement(stateDB, pvmBackend, timeslot, workPackageCoreIndex, packageBundle, auth_result)
	if refineErr != nil {
		return types.WorkPackageBundleSnapshot{}, refineErr
	}
	refineElapsed := time.Since(refineStart)
	fmt.Printf("%sΨ.Refine: %v%s\n", common.ColorGray, refineElapsed, common.ColorReset)

	// Step 3 - Create availability specifier
	specStart := time.Now()
	spec := MakeAvailabilitySpecifier(packageBundle, raw_exported_segments)
	specElapsed := time.Since(specStart)
	fmt.Printf("%sGen Avail.Spec: %v%s\n", common.ColorGray, specElapsed, common.ColorReset)

	// Step 4 - Make work report
	reportStart := time.Now()
	workReport := MakeWorkReport(packageBundle, workPackageCoreIndex, auth_output, authGasUsed, refine_results, spec, segmentRootLookup, authorizer_hash)
	reportElapsed := time.Since(reportStart)
	fmt.Printf("%sGen WorkReport: %v%s\n", common.ColorGray, reportElapsed, common.ColorReset)
	// Step 5 - Make bundle snapshot
	snapshot := MakeBundleSnapshot(packageBundle, workPackageCoreIndex, segmentRootLookup, slot, workReport)

	return snapshot, nil
}

func ExecuteWorkPackageBundleSkipAuth(stateDB *statedb.StateDB, pvmBackend string, timeslot uint32, workPackageCoreIndex uint16, packageBundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, slot uint32, authGasUsed uint, authTrace []byte) (types.WorkPackageBundleSnapshot, error) {

	// Step 1 - Synthesize authorization results from RefineBundle
	authStart := time.Now()
	auth_result, auth_output, authGasUsedInt64, authorizer_hash := makeSyntheticAuth(packageBundle, authGasUsed, authTrace)
	authElapsed := time.Since(authStart)
	fmt.Printf("%sΨ.Auth: %v (Synthesized)%s\n", common.ColorGray, authElapsed, common.ColorReset)

	// Step 2 - Execute refinement
	refineStart := time.Now()
	refine_results, raw_exported_segments, refineErr := ExecuteWBRefinement(stateDB, pvmBackend, timeslot, workPackageCoreIndex, packageBundle, auth_result)
	if refineErr != nil {
		return types.WorkPackageBundleSnapshot{}, refineErr
	}
	refineElapsed := time.Since(refineStart)
	fmt.Printf("%sΨ.Refine: %v%s\n", common.ColorGray, refineElapsed, common.ColorReset)

	// Step 3 - Create availability specifier
	specStart := time.Now()
	spec := MakeAvailabilitySpecifier(packageBundle, raw_exported_segments)
	specElapsed := time.Since(specStart)
	fmt.Printf("%sGen Avail.Spec: %v%s\n", common.ColorGray, specElapsed, common.ColorReset)

	// Step 4 - Make work report
	reportStart := time.Now()
	workReport := MakeWorkReport(packageBundle, workPackageCoreIndex, auth_output, authGasUsedInt64, refine_results, spec, segmentRootLookup, authorizer_hash)
	reportElapsed := time.Since(reportStart)
	fmt.Printf("%sGen WorkReport: %v%s\n", common.ColorGray, reportElapsed, common.ColorReset)

	// Step 5 - Make bundle snapshot
	snapshot := MakeBundleSnapshot(packageBundle, workPackageCoreIndex, segmentRootLookup, slot, workReport)

	return snapshot, nil
}

func ExecuteWPAuthorization(stateDB *statedb.StateDB, pvmBackend string, workPackage types.WorkPackage, workPackageCoreIndex uint16) (types.Result, []byte, int64, common.Hash, error) {

	authcode, _, authindex, err := stateDB.GetAuthorizeCode(workPackage)
	if err != nil {
		return types.Result{}, nil, 0, common.Hash{}, fmt.Errorf("failed to get authorization code: %v", err)
	}

	vm_auth := statedb.NewVMFromCode(authindex, authcode, 0, 0, stateDB, pvmBackend, types.IsAuthorizedGasAllocation)
	vm_auth.SetPVMContext(log.FirstGuarantorOrAuditor)
	auth_result := vm_auth.ExecuteAuthorization(workPackage, workPackageCoreIndex)
	auth_output := auth_result.Ok
	authGasUsed := int64(types.IsAuthorizedGasAllocation) - vm_auth.GetGas()
	authorizer_hash := ComputeAuthHash(workPackage)

	return auth_result, auth_output, authGasUsed, authorizer_hash, nil
}

func ExecuteWBRefinement(stateDB *statedb.StateDB, pvmBackend string, timeslot uint32, workPackageCoreIndex uint16, packageBundle types.WorkPackageBundle, auth_result types.Result) ([]types.WorkDigest, [][]byte, error) {
	workPackage := packageBundle.WorkPackage
	var segments [][]byte
	importsegments := make([][][]byte, len(packageBundle.WorkPackage.WorkItems))
	copy(importsegments, packageBundle.ImportSegmentData)

	refine_results := []types.WorkDigest{}
	p_u := workPackage.AuthorizationCodeHash

	for index, workItem := range workPackage.WorkItems {
		serviceIndex := workItem.Service
		code, ok, err0 := stateDB.ReadServicePreimageBlob(serviceIndex, workItem.CodeHash)
		if err0 != nil || !ok || len(code) == 0 {
			return nil, nil, fmt.Errorf("failed to read service preimage blob: s_id %v, codehash %v, err %v, ok=%v", serviceIndex, workItem.CodeHash, err0, ok)
		}

		if common.Blake2Hash(code) != workItem.CodeHash {
			return nil, nil, fmt.Errorf("code and CodeHash mismatch")
		}

		// Execute refine
		vm := statedb.NewVMFromCode(serviceIndex, code, 0, 0, stateDB, pvmBackend, workItem.RefineGasLimit)
		vm.Timeslot = timeslot
		vm.SetPVMContext(log.FirstGuarantorOrAuditor)

		refineStart := time.Now()
		output, _, exported_segments := vm.ExecuteRefine(
			workPackageCoreIndex,
			uint32(index), workPackage, auth_result, importsegments,
			workItem.ExportCount, packageBundle.ExtrinsicData[index],
			p_u, common.BytesToHash(trie.H0),
		)
		refineElapsed := time.Since(refineStart)
		fmt.Printf("%sΨ.R_%d: %v%s\n", common.ColorGray, index, refineElapsed, common.ColorReset)

		// Process exported segments
		expectedSegmentCnt := int(workItem.ExportCount)
		if expectedSegmentCnt != len(exported_segments) {
			expectedSegmentCnt = len(exported_segments)
		}
		if expectedSegmentCnt != 0 {
			for i := 0; i < expectedSegmentCnt; i++ {
				segment := common.PadToMultipleOfN(exported_segments[i], types.SegmentSize)
				segments = append(segments, segment)
			}
		}

		// Create work digest
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
			NumExtrinsics:       uint(len(packageBundle.ExtrinsicData)),
			NumBytesExtrinsics:  uint(z),
		}

		if len(output.Ok)+z > types.MaxEncodedWorkReportSize {
			result.Result.Err = types.WORKDIGEST_OVERSIZE
			result.Result.Ok = nil
		} else if expectedSegmentCnt != len(exported_segments) {
			result.Result.Err = types.WORKDIGEST_BAD_EXPORT
			result.Result.Ok = nil
		} else {
			result.Result = output
		}
		refine_results = append(refine_results, result)
	}

	return refine_results, segments, nil
}

func MakeWorkReport(packageBundle types.WorkPackageBundle, workPackageCoreIndex uint16, auth_output []byte, authGasUsed int64, refine_results []types.WorkDigest, spec *types.AvailabilitySpecifier, segmentRootLookup types.SegmentRootLookup, authorizer_hash common.Hash) types.WorkReport {
	workPackage := packageBundle.WorkPackage

	workReport := types.WorkReport{
		AvailabilitySpec:  *spec,
		RefineContext:     workPackage.RefineContext,
		CoreIndex:         uint(workPackageCoreIndex),
		AuthorizerHash:    authorizer_hash,
		Trace:             auth_output,
		SegmentRootLookup: segmentRootLookup,
		Results:           refine_results,
		AuthGasUsed:       uint(authGasUsed),
	}

	return workReport
}

func MakeBundleSnapshot(packageBundle types.WorkPackageBundle, workPackageCoreIndex uint16, segmentRootLookup types.SegmentRootLookup, slot uint32, workReport types.WorkReport) types.WorkPackageBundleSnapshot {
	snapshot := types.WorkPackageBundleSnapshot{
		PackageHash:       packageBundle.PackageHash(),
		CoreIndex:         workPackageCoreIndex,
		Bundle:            packageBundle,
		SegmentRootLookup: segmentRootLookup,
		Slot:              slot,
		Report:            workReport,
	}

	return snapshot
}

func ComputeAuthHash(workPackage types.WorkPackage) common.Hash {
	p_u := workPackage.AuthorizationCodeHash
	p_p := workPackage.ConfigurationBlob
	return common.Blake2Hash(append(p_u.Bytes(), p_p...))
}

func makeSyntheticAuth(packageBundle types.WorkPackageBundle, authGasUsed uint, authTrace []byte) (types.Result, []byte, int64, common.Hash) {
	auth_result := types.Result{
		Ok:  authTrace,
		Err: types.WORKDIGEST_OK,
	}
	auth_output := authTrace
	authorizer_hash := ComputeAuthHash(packageBundle.WorkPackage)
	return auth_result, auth_output, int64(authGasUsed), authorizer_hash
}

func MakeAvailabilitySpecifier(packageBundle types.WorkPackageBundle, exportSegments [][]byte) *types.AvailabilitySpecifier {
	b := packageBundle.Bytes()

	// Build b♣ and s♣
	bClubStart := time.Now()
	bClubs := buildBClub(b)
	bClubElapsed := time.Since(bClubStart)

	sClubStart := time.Now()
	sClubs := buildSClub(exportSegments)
	sClubElapsed := time.Since(sClubStart)
	fmt.Printf("%sGen b♣: %v \nGen s♣: %v%s\n", common.ColorGray, bClubElapsed, sClubElapsed, common.ColorReset)

	// Create exported segment tree
	exportedSegmentTree := trie.NewCDMerkleTree(exportSegments)
	exportedSegmentRoot := exportedSegmentTree.RootHash()

	// Generate ErasureRoot
	erasureRoot := generateErasureRoot(bClubs, sClubs)

	spec := &types.AvailabilitySpecifier{
		WorkPackageHash:       packageBundle.WorkPackage.Hash(),
		BundleLength:          uint32(len(b)),
		ErasureRoot:           erasureRoot,
		ExportedSegmentRoot:   exportedSegmentRoot,
		ExportedSegmentLength: uint16(len(exportSegments)),
	}

	return spec
}

func buildBClub(b []byte) []common.Hash {
	// Padding b to the length of W_G
	paddedB := common.PadToMultipleOfN(b, types.ECPieceSize)

	// BLS encode the padded bundle
	chunks, err := bls.Encode(paddedB, types.TotalValidators)
	if err != nil {
		// Handle error gracefully - return empty hashes if encoding fails
		return make([]common.Hash, types.TotalValidators)
	}

	// Hash each element of the encoded data
	bClubs := make([]common.Hash, types.TotalValidators)
	for shardIdx, shard := range chunks {
		if shardIdx < types.TotalValidators {
			bClubs[shardIdx] = common.Blake2Hash(shard)
		}
	}

	return bClubs
}

func buildSClub(segments [][]byte) []common.Hash {
	if len(segments) == 0 {
		return make([]common.Hash, types.TotalValidators)
	}

	// Initialize combined data for all shards
	combinedShardData := make([][]byte, types.TotalValidators)
	for i := range combinedShardData {
		combinedShardData[i] = []byte{}
	}

	// Process each segment
	for _, segmentData := range segments {
		// Encode segmentData into shards
		erasureCodingSegments, err := bls.Encode(segmentData, types.TotalValidators)
		if err != nil {
			continue // Skip failed encodings
		}

		// Append each shard to combined data
		for shardIndex, shard := range erasureCodingSegments {
			if shardIndex < types.TotalValidators {
				combinedShardData[shardIndex] = append(combinedShardData[shardIndex], shard...)
			}
		}
	}

	// Hash the combined data for each shard
	sClubs := make([]common.Hash, types.TotalValidators)
	for shardIdx, combinedData := range combinedShardData {
		sClubs[shardIdx] = common.Blake2Hash(combinedData)
	}

	return sClubs
}

func generateErasureRoot(bClubs []common.Hash, sClubs []common.Hash) common.Hash {
	erasureTree := generateErasureTree(bClubs, sClubs)
	return erasureTree.RootHash()
}

func generateErasureTree(bClubs []common.Hash, sClubs []common.Hash) *trie.WellBalancedTree {
	// Combine b♣, s♣ into 64bytes pairs
	bundleSegmentPairs := common.BuildBundleSegmentPairs(bClubs, sClubs)

	// Generate and return erasure tree
	return trie.NewWellBalancedTree(bundleSegmentPairs, types.Blake2b)
}

func VerifyBundleSnapshotReproduction(stateDB *statedb.StateDB, pvmBackend string, timeslot uint32, originalSnapshot *types.WorkPackageBundleSnapshot) (bool, error) {
	fmt.Printf("Verifying fuzzer can reproduce original bundle snapshot for slot %d...\\n", originalSnapshot.Slot)

	reproducedSnapshot, err := ExecuteWorkPackageBundleV1(
		stateDB,
		pvmBackend,
		timeslot,
		originalSnapshot.CoreIndex,
		originalSnapshot.Bundle,
		originalSnapshot.SegmentRootLookup,
		originalSnapshot.Slot,
	)

	if err != nil {
		return false, fmt.Errorf("failed to reproduce bundle snapshot: %v", err)
	}

	match := true
	var mismatches []string

	if originalSnapshot.PackageHash != reproducedSnapshot.PackageHash {
		match = false
		mismatches = append(mismatches, fmt.Sprintf("PackageHash: original=%s, reproduced=%s",
			originalSnapshot.PackageHash.Hex(), reproducedSnapshot.PackageHash.Hex()))
	}

	if originalSnapshot.CoreIndex != reproducedSnapshot.CoreIndex {
		match = false
		mismatches = append(mismatches, fmt.Sprintf("CoreIndex: original=%d, reproduced=%d",
			originalSnapshot.CoreIndex, reproducedSnapshot.CoreIndex))
	}

	if originalSnapshot.Slot != reproducedSnapshot.Slot {
		match = false
		mismatches = append(mismatches, fmt.Sprintf("Slot: original=%d, reproduced=%d",
			originalSnapshot.Slot, reproducedSnapshot.Slot))
	}

	origReport := originalSnapshot.Report
	reprodReport := reproducedSnapshot.Report

	if origReport.AuthorizerHash != reprodReport.AuthorizerHash {
		match = false
		mismatches = append(mismatches, fmt.Sprintf("AuthorizerHash: original=%s, reproduced=%s",
			origReport.AuthorizerHash.Hex(), reprodReport.AuthorizerHash.Hex()))
	}

	if len(origReport.Results) != len(reprodReport.Results) {
		match = false
		mismatches = append(mismatches, fmt.Sprintf("Results count: original=%d, reproduced=%d",
			len(origReport.Results), len(reprodReport.Results)))
	}

	if origReport.AuthGasUsed != reprodReport.AuthGasUsed {
		match = false
		mismatches = append(mismatches, fmt.Sprintf("AuthGasUsed: original=%d, reproduced=%d",
			origReport.AuthGasUsed, reprodReport.AuthGasUsed))
	}

	if match {
		fmt.Printf("✅ Bundle snapshot reproduction SUCCESSFUL - exact match!\\n")
		return true, nil
	} else {
		fmt.Printf("❌ Bundle snapshot reproduction FAILED - mismatches found:\\n")
		for _, mismatch := range mismatches {
			fmt.Printf("  - %s\\n", mismatch)
		}
		return false, fmt.Errorf("bundle snapshot reproduction failed with %d mismatches", len(mismatches))
	}
}
