package fuzz

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/jamerrors"

	//	"github.com/jam-duna/jamduna/refine"
	"github.com/jam-duna/jamduna/statedb"
	"github.com/jam-duna/jamduna/types"
)

// hack. find actual parent block
func FindParentBlock(stfs []*statedb.StateTransition, stfQA *StateTransitionQA) *types.Block {
	parentHeaderHash := stfQA.STF.Block.Header.ParentHeaderHash
	for _, stf := range stfs {
		if stf.Block.Header.HeaderHash() == parentHeaderHash {
			return &stf.Block
		}
	}
	return nil
}

func HasParentStfs(stfs []*statedb.StateTransition, target_stf *statedb.StateTransition) bool {
	parentHeaderHash := target_stf.Block.Header.ParentHeaderHash
	for _, stf := range stfs {
		if stf.Block.Header.HeaderHash() == parentHeaderHash {
			return true
		}
	}
	return false
}

// BuildAncestry creates ancestry chain for V1 Initialize message
// Returns up to maxDepth ancestors starting from the parent of blockToImport
func BuildAncestry(stfs []*statedb.StateTransition, blockToImport *types.Block, maxDepth int) []AncestryItem {
	var ancestry []AncestryItem

	// Start with the parent of the block we're about to import
	currentParentHash := blockToImport.Header.ParentHeaderHash

	for len(ancestry) < maxDepth {
		// Find the parent block
		var parentBlock *types.Block
		for _, stf := range stfs {
			if stf.Block.Header.HeaderHash() == currentParentHash {
				parentBlock = &stf.Block
				break
			}
		}

		// If we can't find the parent, stop building ancestry
		if parentBlock == nil {
			break
		}

		// Add this ancestor to the list
		ancestryItem := AncestryItem{
			Slot:       parentBlock.Header.Slot,
			HeaderHash: parentBlock.Header.HeaderHash(),
		}
		ancestry = append(ancestry, ancestryItem)

		// Move to the next parent
		currentParentHash = parentBlock.Header.ParentHeaderHash

		// If we reach a zero hash (genesis), stop
		if currentParentHash == (common.Hash{}) {
			break
		}
	}

	return ancestry
}

// RunUnixSocketChallenge executes a state transition test over an existing connection.
func RunUnixSocketChallenge(fuzzer *Fuzzer, stfQA *StateTransitionQA, verbose bool, stfs []*statedb.StateTransition) (matched bool, solverFuzzed bool, err error) {
	initialStatePayload := &HeaderWithState{
		State: statedb.StateKeyVals{KeyVals: stfQA.STF.PreState.KeyVals},
	}
	parentBlock := FindParentBlock(stfs, stfQA)
	if parentBlock != nil {
		//log.Printf("Challenge B#%.3d Using parent HeaderHash: %s", stfQA.STF.Block.Header.Slot, parentBlock.Header.HeaderHash().Hex())
		initialStatePayload.Header = parentBlock.Header
	} else {
		//log.Printf("FATAL: Challenge B#%.3d Parent block not found for STF: %s", stfQA.STF.Block.Header.Slot, stfQA.STF.Block.Header.HeaderHash().Hex())
		return false, false, fmt.Errorf("parent block not found for STF: %s", stfQA.STF.Block.Header.HeaderHash().Hex())
	}
	expectedPreStateRoot := stfQA.STF.PreState.StateRoot

	if stfQA.Mutated {
		log.Printf("%sFUZZED%s %s%s%s B#%.3d", common.ColorMagenta, common.ColorReset, common.ColorMagenta, jamerrors.GetErrorName(stfQA.Error), common.ColorReset, stfQA.STF.Block.Header.Slot)
	} else {
		log.Printf("%sORIGINAL%s B#%.3d", common.ColorGray, common.ColorReset, stfQA.STF.Block.Header.Slot)
	}

	// For V1 protocol with ancestry support, build ancestry chain
	// According to V1 spec, tiny chains use max depth of 24
	ancestry := BuildAncestry(stfs, &stfQA.STF.Block, MaxAncestorsLengthA)

	targetPreStateRoot, err := fuzzer.InitializeOrSetState(initialStatePayload, ancestry)
	if err != nil {
		return false, false, fmt.Errorf("InitializeOrSetState failed: %w", err)
	}

	// Verification for pre-state.
	if !bytes.Equal(targetPreStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
		log.Printf("FATAL: Pre-state root MISMATCH!\n  Got:  %s\n  Want: %s", targetPreStateRoot.Hex(), expectedPreStateRoot.Hex())
		return false, false, fmt.Errorf("InitializeOrSetState failed: %w", err)
	}

	blockToProcess := &stfQA.STF.Block
	headerHash := blockToProcess.Header.HeaderHash()
	expectedPostStateRoot := stfQA.STF.PostState.StateRoot
	expectedPreStateRoot = stfQA.STF.PreState.StateRoot

	targetPostStateRoot, err := fuzzer.ImportBlock(blockToProcess)
	if err != nil {
		// Check if this is a V1 Error response (correct detection of fuzzed block)
		if strings.Contains(err.Error(), "target returned Error") {
			// V1 Error response - treat as correct detection
			if stfQA.Mutated {
				// Fuzzed block correctly detected by target
				return true, true, nil // matched=true, solverFuzzed=true
			} else {
				// Original block incorrectly rejected - this is an error
				return false, false, fmt.Errorf("ImportBlock failed: %w", err)
			}
		}
		// Other types of ImportBlock errors
		return false, false, fmt.Errorf("ImportBlock failed: %w", err)
	}

	//fmt.Printf("B#%.3d HeaderHash: %s | PostStateRoot: %s | PreStateRoot: %s\n", blockToProcess.Header.Slot, headerHash.Hex(), targetPostStateRoot.Hex(), expectedPreStateRoot.Hex())
	if bytes.Equal(targetPostStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
		solverFuzzed = true
		//log.Printf("B#%.3d solverFuzzed : %v\n", blockToProcess.Header.Slot, jamerrors.GetErrorName(stfQA.Error))
	}

	if stfQA.Mutated {
		// fuzzed blocks should return as pre-state root
		if bytes.Equal(targetPostStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
			//log.Printf("B#%.3d Fuzzed block returned expected pre-state root.", stfQA.STF.Block.Header.Slot)
			matched = true
		} else {
			log.Printf("FATAL: Fuzzed block returned expected post-state root: %s | Pre-State Root:%s", targetPostStateRoot.Hex(), expectedPreStateRoot.Hex())
			matched = false // Fuzzed Undetected
		}
	} else {
		// Unfuzzed. Do verification for post-state.
		if bytes.Equal(targetPostStateRoot.Bytes(), expectedPostStateRoot.Bytes()) {
			matched = true
			//log.Printf("B#%.3d Original block returned expected post-state root: %s", blockToProcess.Header.Slot, targetPostStateRoot.Hex())
		} else {
			log.Printf("FATAL: Post-state root MISMATCH!\n  Got:  %s\n  Want: %s", targetPostStateRoot.Hex(), expectedPostStateRoot.Hex())
			matched = false // Post-state root MISMATCH
		}
	}

	if !matched {
		// Log the mismatch for further analysis.
		if !matched {
			log.Printf("B#%.3d MISMATCH: HeaderHash: %s | PostStateRoot: %s", blockToProcess.Header.Slot, headerHash.Hex(), targetPostStateRoot.Hex())
		} else {
			log.Printf("B#%.3d MATCH: HeaderHash: %s | PostStateRoot: %s", blockToProcess.Header.Slot, headerHash.Hex(), targetPostStateRoot.Hex())
		}
		targetStateKeyVals, err := fuzzer.GetState(&headerHash) // Fetch the state for debugging.
		if err != nil {
			log.Printf("Error fetching state for headerHash %s: %v", headerHash.Hex(), err)
		} else if targetStateKeyVals == nil {
			log.Printf("No state found for headerHash %s", headerHash.Hex())
		} else {
			internalExecutionReport := fuzzer.GenerateExecutionReport(stfQA, *targetStateKeyVals, *targetPostStateRoot, expectedPostStateRoot)
			externalResport := fuzzer.GenerateJsonDiffReport(internalExecutionReport)
			// Store report in the specified directory
			if fuzzer.reportDir != "" {
				currTs := common.ComputeCurrentTS()
				if err := externalResport.SaveToFile(fuzzer.reportDir, currTs, blockToProcess.Header.Slot, stfQA); err != nil {
					log.Printf("Failed to save execution report: %v", err)
				} else {
					log.Printf("Execution report saved for B#%.3d in %s", blockToProcess.Header.Slot, fuzzer.reportDir)
				}
			} else {
				log.Printf("Report directory not set. Skipping saving report for B#%.3d", blockToProcess.Header.Slot)
			}
			log.Printf("Execution Report for B#%.3d:\n%s", blockToProcess.Header.Slot, externalResport.String())
			if verbose {
				diffs := statedb.CompareKeyValsWithOutput(internalExecutionReport.PreState.KeyVals, internalExecutionReport.TargetPostState.KeyVals, internalExecutionReport.PostState.KeyVals)
				fmt.Printf("B#%.3d Diffs:\v%s", blockToProcess.Header.Slot, diffs)
				statedb.HandleDiffs(diffs)
			}

		}
		if !matched {
			os.Exit(1)
		}
	}
	return matched, solverFuzzed, nil
}

// RefineBundleQA represents a RefineBundle test case with expected results
type RefineBundleQA struct {
	RefineBundle       types.RefineBundle       `json:"refine_bundle"`
	ExpectedWorkReport types.WorkReport         `json:"expected_work_report"`
	StateContext       *statedb.StateTransition `json:"state_context,omitempty"` // STF for state setup
	Mutated            bool                     `json:"mutated"`
	Error              error                    `json:"error,omitempty"`
}

/*
// RunRefineBundleChallenge executes a bundle refinement test over an existing connection.
// Follows the same pattern as RunUnixSocketChallenge: SetState -> RefineBundle -> Verify
func RunRefineBundleChallenge(fuzzer *Fuzzer, bundleQA *RefineBundleQA, verbose bool) (matched bool, solverFuzzed bool, err error) {
	if bundleQA.Mutated {
		log.Printf("%sFUZZED BUNDLE%s %s", common.ColorMagenta, common.ColorReset, common.ColorMagenta)
	} else {
		//log.Printf("%sORIGINAL BUNDLE%s", common.ColorGray, common.ColorReset)
	}

	// Step 1: SetState - Tell target what state to use for bundle execution
	if bundleQA.StateContext != nil {
		initialStatePayload := &HeaderWithState{
			State: statedb.StateKeyVals{KeyVals: bundleQA.StateContext.PreState.KeyVals},
		}

		// For bundles, we need the parent block context for proper state setup
		// Use the state context's block header as the parent
		if bundleQA.StateContext.Block.Header.Slot > 0 {
			initialStatePayload.Header = bundleQA.StateContext.Block.Header
		}

		expectedPreStateRoot := bundleQA.StateContext.PreState.StateRoot

		// For bundle challenges, use empty ancestry since we don't have full chain context
		emptyAncestry := []AncestryItem{}
		targetPreStateRoot, err := fuzzer.InitializeOrSetState(initialStatePayload, emptyAncestry)
		if err != nil {
			return false, false, fmt.Errorf("InitializeOrSetState failed: %w", err)
		}

		// Verification for pre-state
		if !bytes.Equal(targetPreStateRoot.Bytes(), expectedPreStateRoot.Bytes()) {
			log.Printf("FATAL: Bundle pre-state root MISMATCH!\n  Got:  %s\n  Want: %s",
				targetPreStateRoot.Hex(), expectedPreStateRoot.Hex())
			return false, false, fmt.Errorf("InitializeOrSetState pre-state mismatch")
		}

	}

	// Step 2: RefineBundle - ALWAYS compute expected WorkReport by executing the bundle being sent
	// This ensures fuzzer and target work with identical bundle execution logic
	if bundleQA.StateContext == nil {
		return false, false, fmt.Errorf("StateContext required for expected WorkReport computation")
	}

	// Create StateDB using fuzzer's existing storage and state context
	stateKeyVals := &statedb.StateKeyVals{KeyVals: bundleQA.StateContext.PreState.KeyVals}
	stateDB, err := statedb.NewStateDBFromStateKeyVals(fuzzer.store, stateKeyVals)
	if err != nil {
		return false, false, fmt.Errorf("failed to create StateDB: %v", err)
	}

	// Execute the EXACT same bundle being sent to target using stateless function
	snapshot, err := refine.ExecuteWorkPackageBundleV2(
		stateDB,
		fuzzer.pvmBackend,
		stateDB.JamState.SafroleState.Timeslot, // Use actual timeslot from StateDB
		bundleQA.RefineBundle.Core,
		bundleQA.RefineBundle.Bundle,
		bundleQA.RefineBundle.SegmentRootMappings, // Use SegmentRootMappings from RefineBundle
		38, // Slot from original data
	)
	if err != nil {
		return false, false, fmt.Errorf("failed to compute expected WorkReport by executing bundle: %v", err)
	}
	expectedWorkReport := snapshot.Report

	targetWorkReport, err := fuzzer.RefineBundle(&bundleQA.RefineBundle, &expectedWorkReport)

	exportSegmentsByPackageHashErr := err // Save the RefineBundle error for later

	// Only test GetExports if there are actually exported segments
	// Check if target has exports by looking at ExportedSegmentLength > 0
	if targetWorkReport != nil && targetWorkReport.AvailabilitySpec.ExportedSegmentLength > 0 {
		workPackageHash := bundleQA.RefineBundle.Bundle.PackageHash()
		log.Printf("%sGetExports (via WP Hash: %s)%s",
			common.ColorCyan, workPackageHash.Hex(), common.ColorReset)

		exportSegmentsByPackageHash, getExportsErr := fuzzer.GetExports(&workPackageHash)
		if getExportsErr != nil {
			log.Printf("%sGetExports via WP Hash failed: %v%s", common.ColorRed, getExportsErr, common.ColorReset)
		} else {
			log.Printf("%sGetExports via WP Hash returned %d segments%s", common.ColorGreen, len(*exportSegmentsByPackageHash), common.ColorReset)
		}

		// Also test with exports root if available (non-zero hash)
		if targetWorkReport.AvailabilitySpec.ExportedSegmentRoot != (common.Hash{}) {
			exportsRoot := targetWorkReport.AvailabilitySpec.ExportedSegmentRoot
			log.Printf("%sGetExports (via ExportsRoot: %s)%s", common.ColorCyan, exportsRoot.Hex(), common.ColorReset)

			exportSegmentsByRoot, err := fuzzer.GetExports(&exportsRoot)
			if err != nil {
				log.Printf("%sGetExports via ExportsRoot failed: %v%s", common.ColorRed, err, common.ColorReset)
			} else {
				log.Printf("%sGetExports via ExportsRoot returned %d segments%s", common.ColorGreen, len(*exportSegmentsByRoot), common.ColorReset)

				// Verify both calls return the same data
				if exportSegmentsByPackageHash != nil && len(*exportSegmentsByPackageHash) == len(*exportSegmentsByRoot) {
					log.Printf("%sBoth GetExports calls returned same number of segments%s", common.ColorGreen, common.ColorReset)
				} else {
					log.Printf("%sGetExports calls returned different results%s", common.ColorYellow, common.ColorReset)
				}
			}
		}
	} else {
		if targetWorkReport == nil {
			log.Printf("%sNo WorkReport received, skipping GetExports test%s", common.ColorGray, common.ColorReset)
		} else {
			log.Printf("%sNo exported segments (count: %d), skipping GetExports test%s",
				common.ColorGray, targetWorkReport.AvailabilitySpec.ExportedSegmentLength, common.ColorReset)
		}
	}

	// Now handle the original RefineBundle error if there was one
	if exportSegmentsByPackageHashErr != nil {
		return false, false, fmt.Errorf("RefineBundle failed: %w", exportSegmentsByPackageHashErr)
	}

	// Step 3: Verify - The RefineBundle call already handles WorkReport hash comparison
	// If we reach here, the WorkReport matched (or the call would have failed)
	matched = true

	if bundleQA.Mutated {
		// Fuzzed bundles should be rejected or return error indicators
		if matched {
			log.Printf("FATAL: Fuzzed bundle was accepted by target")
			solverFuzzed = false // Target failed to detect invalid bundle
		} else {
			log.Printf("Fuzzed bundle correctly rejected by target")
			solverFuzzed = true // Target correctly detected invalid bundle
		}
	} else {
		// Original bundles should be processed successfully
		if matched {
			log.Printf("%s ✓ RefineBundle Success%s\n", common.ColorGray, common.ColorReset)
			solverFuzzed = false // Expected behavior
		} else {
			log.Printf("FATAL: Original bundle rejected by target")
			solverFuzzed = true // Unexpected rejection
		}
	}

	if !matched && !bundleQA.Mutated {
		// Log detailed mismatch for original bundles that failed
		log.Printf("BUNDLE MISMATCH: Expected successful processing of original bundle")
		if verbose {
			log.Printf("Expected WorkReport: %s", expectedWorkReport.String())
			log.Printf("Target WorkReport: %s", targetWorkReport.String())
		}
	}

	return matched, solverFuzzed, nil
}
*/
