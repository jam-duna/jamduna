//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func parseExportRoots(exportRootsStr string) []common.Hash {
	exportedRoots := []common.Hash{}
	for _, rootStr := range strings.Split(exportRootsStr, "|") {
		if len(rootStr) == 0 {
			continue
		}
		exportedRoot := common.HexToHash(rootStr)

		exportedRoots = append(exportedRoots, exportedRoot)
	}
	return exportedRoots
}

func fib(n1 JNode, testServices map[string]*types.TestService, targetN int) {
	log.Info(module, "FIB START", "targetN", targetN)
	fn := "../trie/test/exportedRoots.json"
	exportedRoots, err := trie.ParseExportedRoots(fn)
	if err != nil {
		fmt.Printf("Error parsing exported roots: %v\n", err)
		return
	}
	for i, root := range exportedRoots {
		fmt.Printf("ExportedRoot[%d]: %s\n", i, root.Hex())
	}

	service0 := testServices["fib"]
	serviceAuth := testServices["auth_copy"]
	coreIdx := uint16(0)
	isDry := false
	isWithImport := true
	//startCase := 3

	// 10 => 25 cases from TestCDT to have fib generate that many exported segments and capture the data
	// TODO: 2, 3, 5, 10, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1023, 1024, 1025, 2047, 2048, 2049, 3071, 3072}
	//var TestingSegmentsNums = []int{0, 1, 2, 64, 128, 256, 512, 1024, 2048}
	var TestingSegmentsNums = []int{2, 3, 5, 10, 31, 32, 33, 63, 64, 65, 127, 128, 129, 255, 256, 257, 511, 512, 513, 1023, 1024, 1025, 2047, 2048, 2049, 3071, 3072}

	//var prevWP common.Hash
	for case_i, fibN := range TestingSegmentsNums {
		imported := []types.ImportSegment{}
		requestedHash := common.Hash{} // requestedHash (via workpackageHash or ExportRoots)
		if isWithImport {              /* disabled for now since polkajam doesn't support importing segments*/
			if fibN > 1 && case_i > 0 {
				finN_prev := TestingSegmentsNums[case_i-1]
				fibN_previous_exported := exportedRoots[finN_prev]
				requestedHash = fibN_previous_exported
				imported = append(imported, types.ImportSegment{
					RequestedHash: requestedHash,
					Index:         uint16(finN_prev - 1),
				})
			}
		}

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))

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
					ImportedSegments:   imported,
					ExportCount:        uint16(fibN),
				},
				{
					Service:            statedb.AuthCopyServiceCode,
					CodeHash:           serviceAuth.CodeHash,
					Payload:            nil,
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   nil,
					ExportCount:        0,
				},
			},
		}

		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("FIB(%d)", fibN),
			CoreIndex:       coreIdx,
			WorkPackage:     wp,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}
		fmt.Printf("CASE#%d FIB(%d) WORK PACKAGE: %s. WP=%v\n", case_i, fibN, wpr.Identifier, wp.String())

		if !isDry {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
			hashes, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
			cancel()
			if err != nil {
				log.Error(module, "SubmitAndWaitForWorkPackages ERR", "err", err)
				return
			}
			requestedHash = hashes[0]
			fmt.Printf("FIB(%d) WORK PACKAGE HASH | RequestHash: %s\n", fibN, requestedHash)
			k := common.ServiceStorageKey(statedb.FibServiceCode, []byte{0})
			data, _, _ := n1.GetServiceStorage(statedb.FibServiceCode, k)
			log.Info(module, wpr.Identifier, "result", fmt.Sprintf("%x", data))
		}
	}
}

func fib2(n1 JNode, testServices map[string]*types.TestService, targetN int) error {
	log.Info(module, "FIB2 START")

	jamKey := []byte("jam")
	service0 := testServices["corevm"]
	serviceAuth := testServices["auth_copy"]
	coreIdx := uint16(1)

	childSvc, _ := getServices([]string{"corevm_child"}, false)
	childCodeHash := childSvc["corevm_child"].CodeHash
	childCodeLen := uint32(len(childSvc["corevm_child"].Code))
	childLenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(childLenBytes, childCodeLen)

	prevWP := common.Hash{}

	// prepare extrinsics for child loader
	extrinsics := types.ExtrinsicsBlobs{}
	ext := append(childCodeHash.Bytes(), childLenBytes...)
	extrinsics = append(extrinsics, ext)
	wiExt := []types.WorkItemExtrinsic{{
		Hash: common.Blake2Hash(ext),
		Len:  uint32(len(ext)),
	}}

	for fibN := -1; fibN <= targetN; fibN++ {
		// build imported segments
		imported := []types.ImportSegment{}
		if fibN > 0 {
			for i := 0; i < fibN; i++ {
				imported = append(imported, types.ImportSegment{
					RequestedHash: prevWP,
					Index:         uint16(i),
				})
			}
		}

		// build payload
		var payload []byte
		if fibN >= 0 {
			for i := 0; i < fibN+2; i++ {
				tmp := make([]byte, 4)
				binary.LittleEndian.PutUint32(tmp, uint32(fibN))
				payload = append(payload, tmp...)
				binary.LittleEndian.PutUint32(tmp, uint32(1)) // function id
				payload = append(payload, tmp...)
			}
		}

		wp := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         nil, // null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  nil,
			WorkItems: []types.WorkItem{
				{
					Service:            service0.ServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   imported,
					Extrinsics:         wiExt,
					ExportCount:        uint16(fibN + 1),
				},
				{
					Service:            serviceAuth.ServiceCode,
					CodeHash:           serviceAuth.CodeHash,
					Payload:            nil,
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   nil,
					ExportCount:        0,
				},
			},
		}

		label := "init"
		if fibN >= 0 {
			label = fmt.Sprintf("%d", fibN)
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("FIB2(%s)", label),
			CoreIndex:       coreIdx,
			WorkPackage:     wp,
			ExtrinsicsBlobs: extrinsics,
		}

		hashes, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
		cancel()
		if err != nil {
			log.Error(module, "RobustSubmitAndWaitForWorkPackages", "err", err)
			return err
		}
		prevWP = hashes[0]

		// inspect storage keys [0,1,2,5,6,7,8,9]
		for _, key := range []byte{0, 1, 2, 5, 6, 7, 8, 9} {
			k := common.ServiceStorageKey(service0.ServiceCode, []byte{key})
			data, _, _ := n1.GetServiceStorage(service0.ServiceCode, k)
			log.Info(module,
				fmt.Sprintf("Fib2-(%s) result key %d", label, key),
				"result", fmt.Sprintf("%x", data),
			)
		}

		// occasionally load preimages
		switch fibN {
		case 3, 6:
			ctx2, cancel2 := context.WithTimeout(context.Background(), RefineTimeout)
			_ = n1.SubmitAndWaitForPreimage(ctx2, service0.ServiceCode, jamKey)
			cancel2()
		case -1:
			ctx2, cancel2 := context.WithTimeout(context.Background(), RefineTimeout)
			if err := n1.SubmitAndWaitForPreimage(ctx2, service0.ServiceCode, childSvc["corevm_child"].Code); err != nil {
				log.Error(module, "SubmitAndWaitForPreimage", "err", err)
			} else {
				log.Info(module, "COREVM CHILD LOADED")
			}
			cancel2()
		}
	}
	return nil
}
