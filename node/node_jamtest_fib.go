//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func fib(n1 JNode, testServices map[string]*types.TestService, targetN int) {
	log.Info(log.Node, "FIB START", "targetN", targetN)

	service0 := testServices["fib"]
	serviceAuth := testServices["auth_copy"]

	isDry := false
	var prevExportSegmentRoot common.Hash
	for fibN := 1; fibN < 100; fibN++ {
		imported := []types.ImportSegment{}
		if fibN > 1 {
			imported = append(imported, types.ImportSegment{
				RequestedHash: prevExportSegmentRoot,
				Index:         0, // TODO: add variety
			})
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
			Identifier: fmt.Sprintf("FIB(%d)", fibN),

			WorkPackage:     wp,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		if !isDry {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
			wr, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
			cancel()
			if err != nil {
				log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
				return
			}

			prevExportSegmentRoot = wr.AvailabilitySpec.ExportedSegmentRoot
			k := common.ServiceStorageKey(statedb.FibServiceCode, []byte{0})
			data, _, _ := n1.GetServiceStorage(statedb.FibServiceCode, k)
			log.Info(log.Node, wpr.Identifier, "workPackageHash", wr.AvailabilitySpec.WorkPackageHash, "exportedSegmentRoot", wr.AvailabilitySpec.ExportedSegmentRoot, "result", fmt.Sprintf("%x", data))
		} else {
			log.Info(log.Node, wpr.Identifier, "workPackageHash", wp.Hash(), "wp", wp.String())
		}
	}
}

func fib2(n1 JNode, testServices map[string]*types.TestService, targetN int) error {
	log.Info(log.Node, "FIB2 START")

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
			WorkPackage:     wp,
			ExtrinsicsBlobs: extrinsics,
		}

		wr, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
		cancel()
		if err != nil {
			log.Error(log.Node, "RobustSubmitAndWaitForWorkPackages", "err", err)
			return err
		}
		prevWP = wr.AvailabilitySpec.ExportedSegmentRoot

		// inspect storage keys [0,1,2,5,6,7,8,9]
		for _, key := range []byte{0, 1, 2, 5, 6, 7, 8, 9} {
			k := common.ServiceStorageKey(service0.ServiceCode, []byte{key})
			data, _, _ := n1.GetServiceStorage(service0.ServiceCode, k)
			log.Info(log.Node,
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
				log.Error(log.Node, "SubmitAndWaitForPreimage", "err", err)
			} else {
				log.Info(log.Node, "COREVM CHILD LOADED")
			}
			cancel2()
		}
	}
	return nil
}
