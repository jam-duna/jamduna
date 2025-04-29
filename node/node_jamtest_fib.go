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

	_ "net/http/pprof"
)

func fib(n1 JNode, testServices map[string]*types.TestService, targetN int) {
	log.Info(module, "FIB START", "targetN", targetN)
	service0 := testServices["fib"]
	service_authcopy := testServices["auth_copy"]

	prevWorkPackageHash := common.Hash{}
	for fibN := 1; fibN <= targetN; fibN++ {
		importedSegments := make([]types.ImportSegment, 0)
		if fibN > 1 {
			importedSegment := types.ImportSegment{
				RequestedHash: prevWorkPackageHash,
				Index:         0,
			}
			importedSegments = append(importedSegments, importedSegment)
		}
		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			WorkItems: []types.WorkItem{
				{
					Service:            service0.ServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   importedSegments,
					ExportCount:        1,
				},
				{
					Service:            service_authcopy.ServiceCode,
					CodeHash:           service_authcopy.CodeHash,
					Payload:            []byte{},
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("FIB(%d)", fibN),
			CoreIndex:       1,
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		workPackageHashes, err := n1.SubmitAndWaitForWorkPackages(ctx, []*WorkPackageRequest{wpr})
		if err != nil {
			fmt.Printf("SubmitAndWaitForWorkPackage ERR %v\n", err)
		}
		prevWorkPackageHash = workPackageHashes[0]
		k := common.ServiceStorageKey(service0.ServiceCode, []byte{0})
		service_account_byte, _, _ := n1.GetServiceStorage(service0.ServiceCode, k)
		log.Info(module, wpr.Identifier, "result", fmt.Sprintf("%x", service_account_byte))
	}
}

func fib2(n1 JNode, testServices map[string]*types.TestService, targetN int) {
	log.Info(module, "FIB2 START")

	jam_key := []byte("jam")
	// jam_key_hash := common.Blake2Hash(jam_key)
	// jam_key_length := uint32(len(jam_key))

	service0 := testServices["corevm"]
	service_authcopy := testServices["auth_copy"]
	fib2_child_code, _ := getServices([]string{"corevm_child"}, false)
	fib2_child_codehash := fib2_child_code["corevm_child"].CodeHash

	fib2_child_code_length := uint32(len(fib2_child_code["corevm_child"].Code))
	fib2_child_code_length_bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(fib2_child_code_length_bytes, fib2_child_code_length)

	prevWorkPackageHash := common.Hash{}

	// Generate the extrinsic
	extrinsics := types.ExtrinsicsBlobs{}

	extrinsic := make([]byte, 0)
	extrinsic = append(extrinsic, fib2_child_codehash.Bytes()...)
	extrinsic = append(extrinsic, fib2_child_code_length_bytes...)

	extrinsic_hash := common.Blake2Hash(extrinsic)
	extrinsic_len := uint32(len(extrinsic))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsic_hash,
		Len:  extrinsic_len,
	})

	extrinsics = append(extrinsics, extrinsic)

	for fibN := -1; fibN <= targetN; fibN++ {
		importedSegments := make([]types.ImportSegment, 0)
		if fibN > 0 {
			for i := range fibN {
				importedSegment := types.ImportSegment{
					RequestedHash: prevWorkPackageHash,
					Index:         uint16(i),
				}
				//fmt.Printf("fibN=%d ImportedSegment %d (%v, %d)\n", fibN, i, prevWorkPackageHash, i)
				importedSegments = append(importedSegments, importedSegment)
			}
		}
		var payload []byte
		if fibN >= 0 {
			for range fibN + 2 {
				tmp := make([]byte, 4)
				binary.LittleEndian.PutUint32(tmp, uint32(fibN))
				payload = append(payload, tmp...)

				tmp = make([]byte, 4)
				binary.LittleEndian.PutUint32(tmp, uint32(1)) // function id
				payload = append(payload, tmp...)
			}
		}
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			WorkItems: []types.WorkItem{
				{
					Service:            service0.ServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     56789,
					AccumulateGasLimit: 98765,
					ImportedSegments:   importedSegments,
					Extrinsics:         work_item_extrinsic,
					ExportCount:        uint16(fibN + 1),
				},
				{
					Service:            service_authcopy.ServiceCode,
					CodeHash:           service_authcopy.CodeHash,
					Payload:            []byte{},
					RefineGasLimit:     56789,
					AccumulateGasLimit: 98765,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}

		var fibN_string string
		if fibN == -1 {
			fibN_string = "init"
		} else {
			fibN_string = fmt.Sprintf("%d", fibN)
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("FIB2(%s)", fibN_string),
			CoreIndex:       1,
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: extrinsics,
		}

		workPackageHashes, err := n1.SubmitAndWaitForWorkPackages(ctx, []*WorkPackageRequest{wpr})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}

		prevWorkPackageHash = workPackageHashes[0]
		keys := []byte{0, 1, 2, 5, 6, 7, 8, 9}
		for _, key := range keys {
			k := common.ServiceStorageKey(service0.ServiceCode, []byte{key})
			service_account_byte, _, _ := n1.GetServiceStorage(service0.ServiceCode, k)
			log.Info(module, fmt.Sprintf("Fib2-(%s) result with key %d", fibN_string, key), "result", fmt.Sprintf("%x", service_account_byte))
		}

		if fibN == 3 || fibN == 6 {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			defer cancel()
			err = n1.SubmitAndWaitForPreimage(ctx, service0.ServiceCode, jam_key)
			if err != nil {
				log.Error(module, "SubmitAndWaitForPreimage", "err", err)
			}
		}

		if fibN == -1 {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			defer cancel()
			err = n1.SubmitAndWaitForPreimage(ctx, service0.ServiceCode, fib2_child_code["corevm_child"].Code)
			if err != nil {
				log.Error(module, "SubmitAndWaitForPreimage", "err", err)
			} else {
				log.Info(module, "COREVM CHILD LOADED")
			}
		}
	}
}
