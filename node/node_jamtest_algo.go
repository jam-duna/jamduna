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

	//algo_serviceIdx := service0.ServiceCode
	algo_serviceIdx := uint32(statedb.AlgoServiceCode)
	auth_serviceIdx := uint32(statedb.AuthCopyServiceCode)

	log.Info(log.Node, "ALGO START", "algo", algo_serviceIdx, "auth", auth_serviceIdx, "targetN", targetN, "isSimple", isSimple)

	for algoN := 1; algoN < targetN; algoN++ {
		imported := []types.ImportSegment{}
		algo_payload := statedb.GenerateAlgoPayload(algoN, isSimple)

		auth_payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(auth_payload, uint32(auth_serviceIdx))

		// Get RefineContext with buffer=3 for more tolerance
		refineCtx, err := n1.GetRefineContextWithBuffer(3)
		if err != nil {
			log.Error(log.Node, "GetRefineContext failed", "err", err)
			return
		}

		wp := types.WorkPackage{
			AuthCodeHost:          0,
			AuthorizationToken:    nil, // null-authorizer
			AuthorizationCodeHash: getBootstrapAuthCodeHash(),
			ConfigurationBlob:     nil,
			RefineContext:         refineCtx,
			WorkItems: []types.WorkItem{
				{
					Service:            algo_serviceIdx,
					CodeHash:           service0.CodeHash,
					Payload:            algo_payload,
					RefineGasLimit:     types.RefineGasAllocation / 2,
					AccumulateGasLimit: types.AccumulationGasAllocation,
					ImportedSegments:   imported,
					ExportCount:        uint16(algoN),
				},
			},
		}

		wpr := &types.WorkPackageBundle{
			WorkPackage:   wp,
			ExtrinsicData: []types.ExtrinsicsBlobs{},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		_, _, err = RobustSubmitAndWaitForWorkPackageBundles(ctx, n1, []*types.WorkPackageBundle{wpr})
		cancel()
		if err != nil {
			log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
			return
		}
		k := common.ServiceStorageKey(algo_serviceIdx, []byte{0})
		data, _, _ := n1.GetServiceStorage(algo_serviceIdx, k)
		log.Info(log.Node, "algo", "algoN", algoN, "result", fmt.Sprintf("%x", data))
	}
}
