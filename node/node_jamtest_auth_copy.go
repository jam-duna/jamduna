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

func reassign(n1 JNode, testServices map[string]*types.TestService) error {
	log.Info(log.Node, "REASSIGN SERVICE")
	serviceAuth := testServices["auth_copy"]
	auth_serviceIdx := serviceAuth.ServiceCode // statedb.AuthCopyServiceCode
	auth_serviceIdx_orig := uint32(statedb.AuthCopyServiceCode)
	isDry := false
	auth_payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(auth_payload, uint32(auth_serviceIdx))

	wp := types.WorkPackage{
		AuthCodeHost:          0,
		AuthorizationToken:    nil, // null-authorizer
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
		Identifier: fmt.Sprintf("Assign(%d)", auth_serviceIdx),

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

	return nil
}
