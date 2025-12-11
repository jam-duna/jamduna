//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

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

	// Load expected roots for verification
	expectedRoots, err := loadExpectedFibRoots()
	if err != nil {
		log.Error(log.Node, "Failed to load expected roots", "err", err)
		return
	}
	// Test specific edge cases around powers of 2 boundaries
	targetNList := []int{0, 1, 2, 3, 5, 10, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1023, 1024, 1025, 2047, 2048, 2049, 3071, 3072}
	log.Info(log.Node, "FIB START", "targetNList", targetNList, "expectedRootsLoaded", len(expectedRoots))
	log.Info(log.Node, "FIB START", "fib", fib_serviceIdx, "codeHash", service0.CodeHash)

	var prevExportSegmentRoot common.Hash
	prevFibN := -1 // Track the previous fibN to determine if we need to prefill

	for idx, fibN := range targetNList {
		// Pre-fill prevExportSegmentRoot from expectedRoots if not consecutive
		if prevFibN != fibN-1 && fibN > 0 && fibN-1 < len(expectedRoots) {
			prevExportSegmentRoot = expectedRoots[fibN-1]
			log.Info(log.Node, "FIB prefilled prevExportSegmentRoot", "fibN", fibN, "fromIndex", fibN-1, "root", prevExportSegmentRoot.Hex())
		}

		imported := []types.ImportSegment{}
		if fibN > 0 && idx > 0 && false {
			imported = append(imported, types.ImportSegment{
				RequestedHash: prevExportSegmentRoot,
				Index:         0, // TODO: add variety
			})
		}

		fib_payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(fib_payload, uint32(fibN))

		// Get RefineContext
		refineCtx, err := n1.GetRefineContext()
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

		wpr := &types.WorkPackageBundle{
			WorkPackage:   wp,
			ExtrinsicData: []types.ExtrinsicsBlobs{{}},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		wr, err := RobustSubmitAndWaitForWorkPackageBundles(ctx, n1, []*types.WorkPackageBundle{wpr})
		cancel()
		if err != nil {
			log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
			return
		}

		prevExportSegmentRoot = wr.AvailabilitySpec.ExportedSegmentRoot
		k := common.ServiceStorageKey(fib_serviceIdx, []byte{0})
		data, _, _ := n1.GetServiceStorage(fib_serviceIdx, k)

		// Verify against expected root (index fibN in the array corresponds to FIB(fibN))
		var verified string
		if fibN < len(expectedRoots) {
			expectedRoot := expectedRoots[fibN]
			if prevExportSegmentRoot == expectedRoot {
				verified = "MATCH"
			} else {
				verified = fmt.Sprintf("MISMATCH expected=%s", expectedRoot.Hex())
			}
		} else {
			verified = "NO_EXPECTED (fibN >= len(expectedRoots))"
		}

		log.Info(log.Node, fmt.Sprintf("FIB(%d)", fibN), "workPackageHash", wr.AvailabilitySpec.WorkPackageHash, "exportedSegmentRoot", wr.AvailabilitySpec.ExportedSegmentRoot, "verified", verified, "result", fmt.Sprintf("%x", data))
		prevFibN = fibN
	}
}
