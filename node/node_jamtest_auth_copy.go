//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
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
			AuthorizationCodeHash: bootstrap_auth_codehash,
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

		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("Auth_Copy(%d)", algoN),
			WorkPackage:     wp,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		if !isDry {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
			wr, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
			cancel()
			if err != nil {
				log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
				return err
			}
			log.Info(log.Node, wpr.Identifier, "workPackageHash", wr.AvailabilitySpec.WorkPackageHash, "exportedSegmentRoot", wr.AvailabilitySpec.ExportedSegmentRoot)
		} else {
			log.Info(log.Node, wpr.Identifier, "workPackageHash", wp.Hash(), "wp", wp.String())
		}
	}

	return nil
}
