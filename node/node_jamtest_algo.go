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

func algo(n1 JNode, testServices map[string]*types.TestService, targetN int) {

	service0 := testServices["algo"]
	serviceAuth := testServices["auth_copy"]
	algo_serviceIdx := service0.ServiceCode
	auth_serviceIdx := uint32(statedb.AuthCopyServiceCode)

	isReassign := false
	if isReassign {
		auth_serviceIdx = serviceAuth.ServiceCode // statedb.AuthCopyServiceCode
		if err := reassign(n1, testServices); err != nil {
			log.Error(log.Node, "Reassign failed", "err", err)
			return
		}
	}
	log.Info(log.Node, "ALGO START", "targetN", targetN)
	log.Info(log.Node, "ALGO START", "algo", algo_serviceIdx, "auth", auth_serviceIdx)

	isDry := false
	var prevExportSegmentRoot common.Hash
	for algoN := 1; algoN < 100; algoN++ {
		imported := []types.ImportSegment{}
		if algoN > 1 {
			imported = append(imported, types.ImportSegment{
				RequestedHash: prevExportSegmentRoot,
				Index:         0, // TODO: add variety
			})
		}

		algo_payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(algo_payload, uint32(algoN))
		auth_payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(auth_payload, uint32(auth_serviceIdx))

		wp := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         nil, // null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  nil,
			WorkItems: []types.WorkItem{
				{
					Service:            auth_serviceIdx,
					CodeHash:           serviceAuth.CodeHash,
					Payload:            auth_payload,
					RefineGasLimit:     types.RefineGasAllocation / 2,
					AccumulateGasLimit: types.AccumulationGasAllocation / 2,
					ImportedSegments:   nil,
					ExportCount:        0,
				},
				{
					Service:            algo_serviceIdx,
					CodeHash:           service0.CodeHash,
					Payload:            algo_payload,
					RefineGasLimit:     types.RefineGasAllocation / 2,
					AccumulateGasLimit: types.AccumulationGasAllocation / 2,
					ImportedSegments:   imported,
					ExportCount:        uint16(algoN),
				},
			},
		}

		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("ALGO(%d)", algoN),
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
			k := common.ServiceStorageKey(algo_serviceIdx, []byte{0})
			data, _, _ := n1.GetServiceStorage(algo_serviceIdx, k)
			log.Info(log.Node, wpr.Identifier, "workPackageHash", wr.AvailabilitySpec.WorkPackageHash, "exportedSegmentRoot", wr.AvailabilitySpec.ExportedSegmentRoot, "result", fmt.Sprintf("%x", data))
		} else {
			log.Info(log.Node, wpr.Identifier, "workPackageHash", wp.Hash(), "wp", wp.String())
		}
	}
}
