//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

var testExportedRoots []common.Hash
var once sync.Once

const maxN = 1024
const importTargetN = 13

func loadExportedRoots() {
	once.Do(func() {
		rand.Seed(time.Now().UnixNano())
		log.Info(log.G, "Parsing master list of exported roots for testing...")
		roots, err := trie.ParseExportedRoots("../trie/test/exportedRoots.json")
		if err != nil {
			panic(fmt.Sprintf("setup failed: could not parse test/exportedRoots.json: %v", err))
		}
		testExportedRoots = roots
	})
}

type availableRoot struct {
	Hash     common.Hash
	TrieSize int
}

// generateImportsFromPool creates a slice of random import segments by drawing from a pool
// of previously validated and available roots.
func generateImportsFromPool(pool []availableRoot, numImports int) []types.ImportSegment {
	if len(pool) == 0 || numImports <= 0 {
		return []types.ImportSegment{}
	}

	importedSegs := make([]types.ImportSegment, numImports)
	for i := 0; i < numImports; i++ {

		randPoolIndex := rand.Intn(len(pool))
		//randPoolIndex := len(pool) - 1
		rootToQuery := pool[randPoolIndex]

		index := rand.Intn(rootToQuery.TrieSize)

		importedSegs[i] = types.ImportSegment{
			RequestedHash: rootToQuery.Hash,
			Index:         uint16(index),
		}
	}
	return importedSegs
}

func rubic(n1 JNode, testServices map[string]*types.TestService, targetN int) {
	log.Info(log.Node, "RUBIC START", "targetN", targetN)

	loadExportedRoots()

	service0 := testServices["fib"]
	serviceAuth := testServices["auth_copy"]

	availableImportPool := make([]availableRoot, 0)

	isDry := false
	withImport := true
	targetN = 50
	startN := 50
	jmpSize := 3

	for packageN := startN; packageN < startN+targetN*jmpSize && packageN < maxN; packageN += jmpSize {
		var imported0, imported1 []types.ImportSegment

		if withImport && len(availableImportPool) > 0 {
			// Generate a variable number of imports to increase test diversity.
			numImports0 := rand.Intn(importTargetN) + 1 // Request 1 to 5 random segments
			numImports1 := rand.Intn(importTargetN) + 0
			imported0 = generateImportsFromPool(availableImportPool, numImports0)
			imported1 = generateImportsFromPool(availableImportPool, numImports1)
		}

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(packageN))

		wp := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         nil, // null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  nil,
			WorkItems: []types.WorkItem{
				{
					Service:            statedb.FibServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     DefaultRefineGasLimit * 5,
					AccumulateGasLimit: DefaultAccumulateGasLimit * 5,
					ImportedSegments:   imported0,
					ExportCount:        uint16(packageN),
				},
				{
					Service:            statedb.AuthCopyServiceCode,
					CodeHash:           serviceAuth.CodeHash,
					Payload:            nil,
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   imported1,
					ExportCount:        0,
				},
			},
		}

		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("Rubic(%d). ImpSegs[%d|%d]", packageN, len(imported0), len(imported1)),
			WorkPackage:     wp,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}
		fmt.Printf("Rubic(%d). ImpSegs[%d|%d]\n%v\n", packageN, len(imported0), len(imported1), types.ToJSONHexIndent(wp.WorkItems))

		if !isDry {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
			wr, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
			cancel()
			if err != nil {
				log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
				return
			}
			if packageN < len(testExportedRoots) {
				newlyAvailableRoot := availableRoot{
					Hash:     testExportedRoots[packageN],
					TrieSize: packageN,
				}
				availableImportPool = append(availableImportPool, newlyAvailableRoot)
				log.Info(log.G, "Root unlocked and added to import pool", "trieSize", packageN, "root", newlyAvailableRoot.Hash)
			} else {
				log.Warn(log.G, "Cannot add root to pool: packageN is out of bounds for testExportedRoots", "packageN", packageN)
			}

			k := common.ServiceStorageKey(statedb.FibServiceCode, []byte{0})
			data, _, _ := n1.GetServiceStorage(statedb.FibServiceCode, k)
			log.Info(log.Node, wpr.Identifier, "workPackageHash", wr.AvailabilitySpec.WorkPackageHash, "exportedSegmentRoot", wr.AvailabilitySpec.ExportedSegmentRoot, "result", fmt.Sprintf("%x", data))
		} else {
			log.Info(log.Node, wpr.Identifier, "workPackageHash", wp.Hash(), "wp", wp.String())
		}
	}
}
