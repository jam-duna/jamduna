package fuzz

import (
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"

	"github.com/colorfulnotion/jam/statedb"

	"github.com/colorfulnotion/jam/types"
)

/*
func ConvertSnapshotToMultipleRefineBundleQA(snapshot *types.WorkPackageBundleSnapshot, stfs []*statedb.StateTransition, numVariants int, pvmBackend string, stateDB *statedb.StateDB) ([]*RefineBundleQA, error) {
	// Find the STF that matches this bundle's slot
	var matchingSTF *statedb.StateTransition
	for _, stf := range stfs {
		if stf.Block.Header.Slot == snapshot.Slot {
			matchingSTF = stf
			break
		}
	}

	if matchingSTF == nil {
		return nil, fmt.Errorf("no matching STF found for bundle snapshot slot %d", snapshot.Slot)
	}

	var bundleQAs []*RefineBundleQA

	// Check if we can use the refine package
	useRefinePackage := (pvmBackend != "" && stateDB != nil)

	// For original bundle, compute ExpectedWorkReport using refine package with proper state
	var originalExpectedReport types.WorkReport
	var originalAuthGasUsed uint
	var originalAuthTrace []byte

	if useRefinePackage && matchingSTF != nil {
		fmt.Printf("Computing ExpectedWorkReport for original bundle using refine package...\n")

		// Note: State context setup is handled during StateDB creation from StateKeyVals
		originalSnapshot, err := statedb.ExecuteWorkPackageBundleV2(
			stateDB,
			pvmBackend,
			stateDB.JamState.SafroleState.Timeslot,
			snapshot.CoreIndex,
			snapshot.Bundle,
			snapshot.SegmentRootLookup,
			snapshot.Slot,
		)

		if err != nil {
			fmt.Printf("Failed to compute WorkReport for original bundle: %v\n", err)
			originalExpectedReport = snapshot.Report // Fallback
			originalAuthGasUsed = snapshot.Report.AuthGasUsed
			originalAuthTrace = snapshot.Report.Trace
		} else {
			fmt.Printf("Successfully computed WorkReport for original bundle\n")
			originalExpectedReport = originalSnapshot.Report
			originalAuthGasUsed = originalSnapshot.Report.AuthGasUsed
			originalAuthTrace = originalSnapshot.Report.Trace
		}
	} else {
		originalExpectedReport = snapshot.Report // Use snapshot report if no refine package
		originalAuthGasUsed = snapshot.Report.AuthGasUsed
		originalAuthTrace = snapshot.Report.Trace
	}

	// Create original bundle (variant 0) with auth data
	originalRefineBundle := types.RefineBundle{
		Core:                snapshot.CoreIndex,
		Bundle:              snapshot.Bundle,
		SegmentRootMappings: snapshot.SegmentRootLookup,
		AuthGasUsed:         originalAuthGasUsed,
		AuthTrace:           originalAuthTrace,
	}

	originalBundleQA := &RefineBundleQA{
		RefineBundle:       originalRefineBundle,
		ExpectedWorkReport: originalExpectedReport, // Use computed report
		StateContext:       matchingSTF,            // Include state context for bundle execution
		Mutated:            false,
		Error:              nil,
	}
	bundleQAs = append(bundleQAs, originalBundleQA)

	// Self-verification with proper state context setup
	if useRefinePackage {
		fmt.Printf("🔍 Starting fuzzer self-verification for original bundle...\n")

		// Note: State context setup is handled during StateDB creation from StateKeyVals
		if matchingSTF != nil {
			fmt.Printf("Original bundle state context - Slot: %d, PreStateRoot: %s\n",
				matchingSTF.Block.Header.Slot, matchingSTF.PreState.StateRoot.Hex())
		}

		isMatch, err := refine.VerifyBundleSnapshotReproduction(
			stateDB,
			pvmBackend,
			stateDB.JamState.SafroleState.Timeslot,
			snapshot,
		)
		if err != nil {
			fmt.Printf("⚠️ Self-verification failed: %v\n", err)
		} else if isMatch {
			fmt.Printf("✅ Fuzzer self-verification PASSED - can reproduce original bundle exactly\n")
		} else {
			fmt.Printf("❌ Fuzzer self-verification FAILED - cannot reproduce original bundle\n")
		}
	}

	// Generate mutated variants (variants 1 to numVariants-1)
	for i := 1; i < numVariants; i++ {
		mutatedBundle := MutateWorkPackageBundle(snapshot.Bundle, uint64(i+int(snapshot.Slot)*1000))

		//fmt.Printf("%sVariant %d\nOriginal bundle hash: %s \nMutated  bundle hash: %s%s\n", common.ColorCyan, i, snapshot.Bundle.WorkPackage.Hash().Hex(), mutatedBundle.WorkPackage.Hash().Hex(), common.ColorReset)

		var expectedWorkReport types.WorkReport
		var computeError error
		var mutatedAuthGasUsed uint = originalAuthGasUsed
		var mutatedAuthTrace []byte = originalAuthTrace

		// Try to compute actual ExpectedWorkReport using refine package
		if useRefinePackage {
			fmt.Printf("Computing WorkReport for mutated bundle variant %d using refine package...\n", i)

			// Note: State context setup is handled during StateDB creation from StateKeyVals
			if matchingSTF != nil {
				fmt.Printf("Mutated bundle state context - Slot: %d, PreStateRoot: %s\n",
					matchingSTF.Block.Header.Slot, matchingSTF.PreState.StateRoot.Hex())
			}

			bundleSnapshot, err := refine.ExecuteWorkPackageBundleV2(
				stateDB,
				pvmBackend,
				stateDB.JamState.SafroleState.Timeslot,
				snapshot.CoreIndex,
				mutatedBundle,
				snapshot.SegmentRootLookup,
				snapshot.Slot,
			)

			if err != nil {
				fmt.Printf("Failed to compute WorkReport for variant %d: %v\n", i, err)
				expectedWorkReport = snapshot.Report // Fallback to original report
				computeError = err
				// Keep original auth data for fallback
			} else {
				fmt.Printf("Successfully computed WorkReport for variant %d\n", i)
				expectedWorkReport = bundleSnapshot.Report // Extract WorkReport from snapshot
				mutatedAuthGasUsed = bundleSnapshot.Report.AuthGasUsed
				mutatedAuthTrace = bundleSnapshot.Report.Trace
			}
		} else {
			// Use original report as placeholder if no refine package available
			expectedWorkReport = snapshot.Report
		}

		// Create mutated RefineBundle with auth data
		mutatedRefineBundle := types.RefineBundle{
			Core:                snapshot.CoreIndex,
			Bundle:              mutatedBundle,
			SegmentRootMappings: snapshot.SegmentRootLookup,
			AuthGasUsed:         mutatedAuthGasUsed,
			AuthTrace:           mutatedAuthTrace,
		}

		mutatedBundleQA := &RefineBundleQA{
			RefineBundle:       mutatedRefineBundle,
			ExpectedWorkReport: expectedWorkReport,
			StateContext:       matchingSTF, // Include state context for bundle execution
			Mutated:            false,       // Non-malicious mutations
			Error:              computeError,
		}
		bundleQAs = append(bundleQAs, mutatedBundleQA)
	}

	return bundleQAs, nil
}
*/

// ConvertSnapshotToRefineBundleQA converts a WorkPackageBundleSnapshot to single RefineBundleQA
// by linking it with the corresponding state transition data (legacy function)

func ConvertSnapshotToRefineBundleQA(snapshot *types.WorkPackageBundleSnapshot, stfs []*statedb.StateTransition) (*RefineBundleQA, error) {
	/*
		variants, err := ConvertSnapshotToMultipleRefineBundleQA(snapshot, stfs, 1, "", nil)
		if err != nil {
			return nil, err
		}
		return variants[0], nil
	*/
	return nil, nil
}

func DeepCopyWorkPackageBundle(original types.WorkPackageBundle) types.WorkPackageBundle {
	encoded, err := types.Encode(original)
	if err != nil {
		fmt.Printf("Warning: Failed to encode WorkPackageBundle for deep copy: %v\n", err)
		return original
	}

	decoded, _, err := types.Decode(encoded, reflect.TypeOf(types.WorkPackageBundle{}))
	if err != nil {
		fmt.Printf("Warning: Failed to decode WorkPackageBundle for deep copy: %v\n", err)
		return original
	}

	if bundle, ok := decoded.(types.WorkPackageBundle); ok {
		return bundle
	}

	fmt.Printf("Warning: Failed to type assert decoded WorkPackageBundle\n")
	return original
}

func DetermineWorkBundleClass(bundle types.WorkPackageBundle) string {
	// Code hash to service name mapping
	codeHashToService := map[string]string{
		"0xc6751d94d8bcb7e6132450ad896bcf659618681f585217b8c2d3628f26f9d4e6": "fib",
		"0x98c439352c0028dd0b317f2584d54b495e6469ade14d1fe0225c3d07282224ee": "algo",
		"0x6bb83234ff67d400ac01f1ad6d58c705848e949bd6a37955d48796acd2508098": "auth_copy",
	}

	services := make(map[string]bool)

	// Analyze work items by their code hashes
	for _, workItem := range bundle.WorkPackage.WorkItems {
		codeHashHex := workItem.CodeHash.Hex()
		if serviceName, exists := codeHashToService[codeHashHex]; exists {
			services[serviceName] = true
		} else {
			services["unknown"] = true
		}
	}

	// Return primary service name or combination
	if services["fib"] {
		return "fib"
	} else if services["algo"] {
		return "algo"
	} else if services["auth_copy"] && len(services) == 1 {
		return "auth_copy"
	}
	return "unknown"
}

// MutateWorkPackageBundle generates a mutated variant of a WorkPackageBundle
func MutateWorkPackageBundle(original types.WorkPackageBundle, seed uint64) types.WorkPackageBundle {

	serviceType := DetermineWorkBundleClass(original)
	if serviceType == "unknown" {
		fmt.Printf("Skipping mutation for unknown service type\n")
		return original
	}
	//fmt.Printf("Mutating Service %v with seed %d\n", serviceType, seed)
	mutated := original
	switch serviceType {
	case "fib":
		mutated = MutateFibBundle(original, seed)
	case "algo":
		mutated = MutateAlgoBundle(original, seed)

	default:
		fmt.Printf("No specific mutation strategy for service %v\n", serviceType)
	}
	// TODO: do we store in statedb?
	return mutated
}

func MutateFibBundle(original types.WorkPackageBundle, seed uint64) types.WorkPackageBundle {
	// Implement specific mutation logic for Fib bundles
	fmt.Printf("%sTODO:Mutating Fib bundle%s\n", common.ColorBrightWhite, common.ColorReset)
	originalHash := original.WorkPackage.Hash()
	algoWorkItemIdx := 1
	debug := true

	gasDiff := seed % 365

	// Create a deep copy to ensure mutations don't affect original
	mutated := DeepCopyWorkPackageBundle(original)

	// Now mutate the gas limit
	mutated.WorkPackage.WorkItems[algoWorkItemIdx].RefineGasLimit -= uint64(gasDiff)
	mutatedHash := mutated.WorkPackage.Hash()

	if debug {
		fmt.Printf("%sMutated Fib bundle\noriginal hash: %s\n mutated hash: %s%s%s%s\n", common.ColorGray, originalHash, common.ColorBlue, mutatedHash, common.ColorReset, common.ColorReset)
	}

	return mutated
}

func MutateAlgoBundle(original types.WorkPackageBundle, seed uint64) types.WorkPackageBundle {
	// Implement specific mutation logic for Algo bundles
	originalHash := original.WorkPackage.Hash()
	algoWorkItemIdx := 1
	debug := true

	sz := seed % 365
	isSimple := true
	//isSimple := (seed%2 == 0)

	// Create a deep copy to ensure mutations don't affect original
	mutated := DeepCopyWorkPackageBundle(original)

	// Now mutate the payload
	randomPayload := statedb.GenerateAlgoPayload(int(sz), isSimple)
	mutated.WorkPackage.WorkItems[algoWorkItemIdx].Payload = randomPayload
	mutatedHash := mutated.WorkPackage.Hash()

	if debug {
		fmt.Printf("%sMutated Algo bundle payload size to %d (simple=%v)%s\n", common.ColorGray, len(randomPayload), isSimple, common.ColorReset)
		fmt.Printf("%soriginal hash: %s%s\n mutated hash: %s%s%s\n", common.ColorGray, originalHash, common.ColorBlue, mutatedHash, common.ColorReset, common.ColorReset)
	}

	return mutated
}
