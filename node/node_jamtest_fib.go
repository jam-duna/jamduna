//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func fib(n1 JNode, testServices map[string]*types.TestService, targetN int) {
	log.Info(module, "FIB START", "targetN", targetN)
	service0 := testServices["fib"]
	serviceAuth := testServices["auth_copy"]

	var prevWP common.Hash
	for fibN := 1; fibN <= targetN; fibN++ {
		imported := []types.ImportSegment{}
		if fibN > 1 {
			imported = append(imported, types.ImportSegment{
				RequestedHash: prevWP,
				Index:         0,
			})
		}

		// payload = uint32(fibN)
		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))

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
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   imported,
					ExportCount:        1,
				},
				{
					Service:            serviceAuth.ServiceCode,
					CodeHash:           serviceAuth.CodeHash,
					Payload:            nil,
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   nil,
					ExportCount:        0,
				},
			},
		}

		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("FIB(%d)", fibN),
			CoreIndex:       0,
			WorkPackage:     wp,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		hashes, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
		cancel()
		if err != nil {
			log.Error(module, "SubmitAndWaitForWorkPackages ERR", "err", err)
			return
		}
		prevWP = hashes[0]

		k := common.ServiceStorageKey(service0.ServiceCode, []byte{0})
		data, _, _ := n1.GetServiceStorage(service0.ServiceCode, k)
		log.Info(module, wpr.Identifier, "result", fmt.Sprintf("%x", data))
	}
}

func fib2(n1 JNode, testServices map[string]*types.TestService, targetN int) error {
	log.Info(module, "FIB2 START")

	jamKey := []byte("jam")
	service0 := testServices["corevm"]
	serviceAuth := testServices["auth_copy"]

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
					RefineGasLimit:     56789,
					AccumulateGasLimit: 98765,
					ImportedSegments:   imported,
					Extrinsics:         wiExt,
					ExportCount:        uint16(fibN + 1),
				},
				{
					Service:            serviceAuth.ServiceCode,
					CodeHash:           serviceAuth.CodeHash,
					Payload:            nil,
					RefineGasLimit:     56789,
					AccumulateGasLimit: 98765,
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
			CoreIndex:       1,
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
