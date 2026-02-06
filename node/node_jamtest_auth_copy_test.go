//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"

	"github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/statedb"
	"github.com/jam-duna/jamduna/types"
)

func reassign(n1 JNode, testServices map[string]*types.TestService, targetN int) error {
	log.Info(log.Node, "REASSIGN SERVICE")
	serviceAuth := testServices["auth_copy"]
	auth_serviceIdx := serviceAuth.ServiceCode // statedb.AuthCopyServiceCode
	auth_serviceIdx_orig := uint32(statedb.AuthCopyServiceCode)

	isDry := false
	auth_payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(auth_payload, uint32(auth_serviceIdx))

	for algoN := 0; algoN < targetN; algoN++ {
		auth_token := []byte{byte(algoN)} // null-authorizer
		wp := types.WorkPackage{
			AuthCodeHost:          0,
			AuthorizationToken:    auth_token,
			AuthorizationCodeHash: getBootstrapAuthCodeHash(),
			ConfigurationBlob:     nil,
			WorkItems: []types.WorkItem{
				{
					Service:            auth_serviceIdx_orig,
					CodeHash:           serviceAuth.CodeHash,
					Payload:            auth_payload,
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   nil,
					ExportCount:        0,
				},
			},
		}

		wpr := &types.WorkPackageBundle{
			WorkPackage:   wp,
			ExtrinsicData: []types.ExtrinsicsBlobs{},
		}

		if !isDry {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
			hashes, _, err := RobustSubmitAndWaitForWorkPackageBundles(ctx, n1, []*types.WorkPackageBundle{wpr})
			cancel()
			if err != nil {
				log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
				return err
			}
			if len(hashes) > 0 {
				log.Info(log.Node, "auth_copy", "workPackageHash", hashes[0])
			}
		} else {
			log.Info(log.Node, "auth_copy", "workPackageHash", wp.Hash(), "wp", wp.String())
		}
	}

	return nil
}
