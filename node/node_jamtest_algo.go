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

	isSimple := targetN == 39
	service0 := testServices["algo"]

	algo_serviceIdx := service0.ServiceCode
	auth_serviceIdx := uint32(statedb.AuthCopyServiceCode)

	log.Info(log.Node, "ALGO START", "algo", algo_serviceIdx, "auth", auth_serviceIdx)

	for algoN := 1; algoN < targetN; algoN++ {
		imported := []types.ImportSegment{}
		algo_payload := statedb.GenerateAlgoPayload(algoN, isSimple)

		auth_payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(auth_payload, uint32(auth_serviceIdx))
		wp := types.WorkPackage{
			AuthCodeHost:          0,
			AuthorizationToken:    nil, // null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ConfigurationBlob:     nil,
			WorkItems: []types.WorkItem{
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

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		wr, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
		cancel()
		if err != nil {
			log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
			return
		}
		k := common.ServiceStorageKey(algo_serviceIdx, []byte{0})
		data, _, _ := n1.GetServiceStorage(algo_serviceIdx, k)
		log.Info(log.Node, wpr.Identifier, "workPackageHash", wr.AvailabilitySpec.WorkPackageHash, "exportedSegmentRoot", wr.AvailabilitySpec.ExportedSegmentRoot, "result", fmt.Sprintf("%x", data))
		//log.Info(log.Node, wpr.Identifier, "workPackageHash", wp.Hash(), "wr", wr.String())
	}
}
