//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"fmt"
	_ "net/http/pprof"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func makeWorkPackageRequest(coreIndex uint16, identifier string, prerequisites []string, extrinsics types.ExtrinsicsBlobs, workItems []types.WorkItem) *WorkPackageRequest {
	return &WorkPackageRequest{
		CoreIndex:       coreIndex,
		Identifier:      identifier,
		Prerequisites:   prerequisites,
		ExtrinsicsBlobs: extrinsics,
		WorkPackage: types.WorkPackage{
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthCodeHost:          0,
			AuthorizationCodeHash: bootstrap_auth_codehash,
			WorkItems:             workItems,
			// NOTE: RefineContext is filled in later usint Identifier and Prereqs
		},
	}
}

func megatron(n1 JNode, testServices map[string]*types.TestService, targetN int) {
	serviceFib := testServices["fib"]
	serviceTrib := testServices["tribonacci"]
	serviceMeg := testServices["megatron"]
	serviceAuthCopy := testServices["auth_copy"]

	var workPackageHashes []common.Hash

	for n := 0; n <= targetN; n++ {
		var fibImports, tribImports []types.ImportSegment
		if n > 0 {
			fibImports = []types.ImportSegment{{RequestedHash: workPackageHashes[0], Index: 0}}
			tribImports = []types.ImportSegment{{RequestedHash: workPackageHashes[0], Index: 1}}
		}

		// Payload for fib and trib services
		input := make([]byte, 4)
		binary.LittleEndian.PutUint32(input, uint32(n+1))

		// Work Package: fib and tribonacci (core 0)
		wprFibTrib := makeWorkPackageRequest(0, "fibtrib", nil, types.ExtrinsicsBlobs{}, []types.WorkItem{
			{
				Service:            serviceFib.ServiceCode,
				CodeHash:           serviceFib.CodeHash,
				Payload:            input,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   fibImports,
				ExportCount:        1,
			},
			{
				Service:            serviceTrib.ServiceCode,
				CodeHash:           serviceTrib.CodeHash,
				Payload:            input,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   tribImports,
				ExportCount:        1,
			},
		})

		// Payload for megatron service: encoded service codes of fib and trib
		mergePayload := make([]byte, 8)
		binary.LittleEndian.PutUint32(mergePayload[0:], serviceFib.ServiceCode)
		binary.LittleEndian.PutUint32(mergePayload[4:], serviceTrib.ServiceCode)

		// Work Package: megatron and auth_copy (core 1)
		wprMeg := makeWorkPackageRequest(1, "meg", []string{"fibtrib"}, types.ExtrinsicsBlobs{}, []types.WorkItem{
			{
				Service:            serviceMeg.ServiceCode,
				CodeHash:           serviceMeg.CodeHash,
				Payload:            mergePayload,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 100000,
				ImportedSegments:   nil,
				ExportCount:        0,
			},
			{
				Service:            serviceAuthCopy.ServiceCode,
				CodeHash:           serviceAuthCopy.CodeHash,
				Payload:            nil,
				RefineGasLimit:     5678,
				AccumulateGasLimit: 9876,
				ImportedSegments:   nil,
				ExportCount:        0,
			},
		})

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()

		wprs := []*WorkPackageRequest{wprFibTrib, wprMeg}
		hashes, err := n1.SubmitAndWaitForWorkPackages(ctx, wprs)
		if err != nil {
			fmt.Printf("SubmitAndWaitForWorkPackages error: %v\n", err)
			continue
		}
		workPackageHashes = hashes

		printServiceOutput := func(service *types.TestService, label string) {
			val, ok, err := n1.GetServiceStorage(service.ServiceCode, common.ServiceStorageKey(service.ServiceCode, []byte{0}))
			if err != nil {
				fmt.Printf("%s error: %v\n", label, err)
			} else if ok {
				fmt.Printf("%s: %x\n", label, val)
			}
		}

		printServiceOutput(serviceFib, "fib")
		printServiceOutput(serviceTrib, "trib")
		printServiceOutput(serviceMeg, "meg")
	}
}
