//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/statedb"
	"github.com/jam-duna/jamduna/types"
)

// completedFibResult tracks a completed FIB execution for later import
type completedFibResult struct {
	fibN                int
	exportedSegmentRoot common.Hash
	numExported         int // number of segments exported (equals fibN)
}

// loadExpectedFibRoots loads the expected exported segment roots from exportedRoots.json
func loadExpectedFibRoots() ([]common.Hash, error) {
	// Get the directory of this source file
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("failed to get current file path")
	}
	// Navigate from node/ to trie/test/exportedRoots.json
	rootsPath := filepath.Join(filepath.Dir(filename), "..", "trie", "test", "exportedRoots.json")

	data, err := os.ReadFile(rootsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read exportedRoots.json: %w", err)
	}

	var hexStrings []string
	if err := json.Unmarshal(data, &hexStrings); err != nil {
		return nil, fmt.Errorf("failed to parse exportedRoots.json: %w", err)
	}

	roots := make([]common.Hash, len(hexStrings))
	for i, hexStr := range hexStrings {
		roots[i] = common.HexToHash(hexStr)
	}
	return roots, nil
}

func fib(n1 JNode, testServices map[string]*types.TestService, targetN int) {

	service0 := testServices["fib"]
	_ = testServices["auth_copy"] // Keep for potential future use
	fib_serviceIdx := uint32(statedb.FibServiceCode)
	importEnabled := true
	maxImportsPerWorkItem := 5 // Maximum number of segments to import per work item

	// Use a fixed seed for deterministic, reproducible import selection across runs
	const fibTestSeed = 42
	rng := rand.New(rand.NewSource(fibTestSeed))

	// Load expected roots for verification
	expectedRoots, err := loadExpectedFibRoots()
	if err != nil {
		log.Error(log.Node, "Failed to load expected roots", "err", err)
		return
	}
	// Test specific edge cases around powers of 2 boundaries
	//targetNList := []int{0, 1, 2, 3, 5, 10, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1023, 1024, 1025, 2047, 2048, 2049, 3071, 3072}
	//targetNList := []int{5, 10, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1023, 1024, 1025, 2047, 2048, 2049, 3071, 3072}
	targetNList := []int{0, 1, 2, 3, 5, 10, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1006, 1007, 1008, 1009, 1010, 1011, 1012, 1013, 1014, 1015, 1016, 1017, 1018, 1019, 1020, 1021, 1022, 1023, 1024, 1025, 2047, 2048, 2049, 3071, 3072}

	log.Info(log.Node, "FIB START", "targetNList", targetNList, "expectedRootsLoaded", len(expectedRoots))
	log.Info(log.Node, "FIB START", "fib", fib_serviceIdx, "codeHash", service0.CodeHash)

	// Track all completed FIB results for importing
	completedFibs := []completedFibResult{}

	for _, fibN := range targetNList {
		// Build imports from completed FIBs
		imported := []types.ImportSegment{}

		if importEnabled && fibN > 1 && len(completedFibs) > 0 {
			// Filter to FIBs that have exportable segments (numExported > 0)
			availableFibs := []completedFibResult{}
			for _, cf := range completedFibs {
				if cf.numExported > 0 {
					availableFibs = append(availableFibs, cf)
				}
			}

			if len(availableFibs) > 0 {
				// Determine how many imports to make (1 to maxImportsPerWorkItem, but not more than available)
				numImports := 1 + rng.Intn(min(maxImportsPerWorkItem, len(availableFibs)))

				// Shuffle available FIBs and pick the first numImports
				rng.Shuffle(len(availableFibs), func(i, j int) {
					availableFibs[i], availableFibs[j] = availableFibs[j], availableFibs[i]
				})

				for i := 0; i < numImports && i < len(availableFibs); i++ {
					selectedFib := availableFibs[i]
					// Pick a random valid index from this FIB's exported segments
					randomIndex := rng.Intn(selectedFib.numExported)

					imported = append(imported, types.ImportSegment{
						RequestedHash: selectedFib.exportedSegmentRoot,
						Index:         uint16(randomIndex),
					})

					log.Info(log.Node, "FIB: selected import",
						"forFibN", fibN,
						"fromFibN", selectedFib.fibN,
						"segmentRoot", selectedFib.exportedSegmentRoot,
						"index", randomIndex,
						"maxIndex", selectedFib.numExported-1)
				}
			}
		}

		fib_payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(fib_payload, uint32(fibN))

		// Get RefineContext with buffer=5 for more tolerance
		refineCtx, err := n1.GetRefineContextWithBuffer(5)
		if err != nil {
			log.Error(log.Node, "GetRefineContext failed", "err", err)
			return
		}

		// Use single work item (like algo) - node code only preserves ExtrinsicData[0]
		wp := types.WorkPackage{
			AuthCodeHost:          0,
			AuthorizationToken:    nil, // null-authorizer
			AuthorizationCodeHash: getBootstrapAuthCodeHash(),
			ConfigurationBlob:     nil,
			RefineContext:         refineCtx,
			WorkItems: []types.WorkItem{
				{
					Service:            fib_serviceIdx,
					CodeHash:           service0.CodeHash,
					Payload:            fib_payload,
					RefineGasLimit:     types.RefineGasAllocation,
					AccumulateGasLimit: types.AccumulationGasAllocation,
					ImportedSegments:   imported,
					ExportCount:        uint16(fibN),
				},
			},
		}

		// Build the bundle with import segment data and justifications if needed
		var importSegmentData [][][]byte
		var justifications [][][]common.Hash

		if len(imported) > 0 {
			// Initialize arrays for the single work item
			importSegmentData = make([][][]byte, 1)
			justifications = make([][][]common.Hash, 1)
			importSegmentData[0] = make([][]byte, len(imported))
			justifications[0] = make([][]common.Hash, len(imported))

			// Fetch each imported segment with its proof
			for i, imp := range imported {
				segment, proof, found := n1.GetSegmentWithProof(imp.RequestedHash, uint16(imp.Index))
				if !found {
					panic(fmt.Sprintf("FIB: failed to get segment with proof segmentsRoot=%s index=%d", imp.RequestedHash, imp.Index))
				}
				importSegmentData[0][i] = segment
				justifications[0][i] = proof
				log.Info(log.Node, "FIB: fetched imported segment",
					"fibN", fibN,
					"segmentsRoot", imp.RequestedHash,
					"index", imp.Index,
					"segmentLen", len(segment),
					"proofLen", len(proof))
			}
		}

		wpr := &types.WorkPackageBundle{
			WorkPackage:       wp,
			ExtrinsicData:     []types.ExtrinsicsBlobs{{}},
			ImportSegmentData: importSegmentData,
			Justification:     justifications,
		}

		fmt.Printf("Submitting FIB(%d) WorkPackageBundle %v\n", fibN, wpr.StringL())

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		_, _, err = RobustSubmitAndWaitForWorkPackageBundles(ctx, n1, []*types.WorkPackageBundle{wpr})
		cancel()
		if err != nil {
			log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
			panic(fmt.Sprintf("FIB(%d) FAILED", fibN))
			return
		}

		// Get exportedSegmentRoot from work report using work package hash
		workPackageHash := wp.Hash()
		wr, err := n1.GetWorkReport(workPackageHash)
		var exportedSegmentRoot common.Hash
		if err != nil {
			log.Warn(log.Node, "FIB: GetWorkReport failed", "workPackageHash", workPackageHash, "err", err)
			exportedSegmentRoot = common.Hash{}
		} else {
			exportedSegmentRoot = wr.AvailabilitySpec.ExportedSegmentRoot
		}

		// Track this completed FIB for future imports (only if we got a valid exportedSegmentRoot)
		if exportedSegmentRoot != (common.Hash{}) {
			completedFibs = append(completedFibs, completedFibResult{
				fibN:                fibN,
				exportedSegmentRoot: exportedSegmentRoot,
				numExported:         fibN, // FIB(N) exports N segments
			})
		} else {
			log.Warn(log.Node, "FIB: skipping from completedFibs due to zero exportedSegmentRoot", "fibN", fibN)
		}

		k := common.ServiceStorageKey(fib_serviceIdx, []byte{0})
		data, _, _ := n1.GetServiceStorage(fib_serviceIdx, k)

		// Verify against expected root (index fibN in the array corresponds to FIB(fibN))
		var verified string
		if fibN < len(expectedRoots) {
			expectedRoot := expectedRoots[fibN]
			if exportedSegmentRoot == expectedRoot {
				verified = "MATCH"
			} else {
				verified = fmt.Sprintf("MISMATCH expected=%s", expectedRoot.Hex())
			}
		} else {
			verified = "NO_EXPECTED (fibN >= len(expectedRoots))"
		}

		log.Info(log.Node, fmt.Sprintf("FIB(%d)", fibN), "fibN", fibN, "exportedSegmentRoot", exportedSegmentRoot, "verified", verified, "resultLen", len(data), "numImports", len(imported))
	}
}
