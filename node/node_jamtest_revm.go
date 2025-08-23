//go:build network_test
// +build network_test

package node

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func revm(n1 JNode, testServices map[string]*types.TestService) {
	log.Info(log.Node, "Revm START")
	service0 := testServices["revm_test"]
	service_authcopy := testServices["auth_copy"]

	for N := 1; N <= 2; N++ {
		payload := make([]byte, 0)
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			AuthorizationToken:    []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ConfigurationBlob:     []byte{},
			WorkItems: []types.WorkItem{
				{
					Service:            service0.ServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
				{
					Service:            service_authcopy.ServiceCode,
					CodeHash:           service_authcopy.CodeHash,
					Payload:            []byte{},
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("Revm(%d)", N),
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		var err error

		_, err1 := n1.SubmitAndWaitForWorkPackage(ctx, wpr)
		if err1 != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
			continue
		}
	}
}
