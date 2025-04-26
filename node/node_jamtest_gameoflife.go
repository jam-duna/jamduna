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
)

func flatten(data [][]byte) []byte {
	var result []byte
	for _, block := range data {
		result = append(result, block[8:]...)
	}
	return result
}

func game_of_life(n1 JNode, testServices map[string]*types.TestService) {

	ws_push := StartGameOfLifeServer("localhost:80", "./game_of_life.html")

	log.Info(module, "Game of Life START")

	service0 := testServices["game_of_life"]
	service_authcopy := testServices["auth_copy"]

	service0_child_code, _ := getServices([]string{"game_of_life_child"}, false)
	service0_child_codehash := service0_child_code["game_of_life_child"].CodeHash

	service0_child_code_length := uint32(len(service0_child_code["game_of_life_child"].Code))
	service0_child_code_length_bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(service0_child_code_length_bytes, service0_child_code_length)

	// Generate the extrinsic
	extrinsics := types.ExtrinsicsBlobs{}

	extrinsic := make([]byte, 0)
	extrinsic = append(extrinsic, service0_child_codehash.Bytes()...)
	extrinsic = append(extrinsic, service0_child_code_length_bytes...)

	extrinsic_hash := common.Blake2Hash(extrinsic)
	extrinsic_len := uint32(len(extrinsic))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsic_hash,
		Len:  extrinsic_len,
	})

	extrinsics = append(extrinsics, extrinsic)
	prevWorkPackageHash := common.Hash{}

	for step_n := 0; step_n <= 300000; step_n++ {
		importedSegments := make([]types.ImportSegment, 0)
		export_count := uint16(0)
		if step_n > 1 {
			for i := 0; i < 9; i++ {
				importedSegment := types.ImportSegment{
					RequestedHash: prevWorkPackageHash,
					Index:         uint16(i),
				}
				importedSegments = append(importedSegments, importedSegment)
			}
		}
		var payload []byte
		if step_n > 0 {
			payload = make([]byte, 0, 12)
			tmp := make([]byte, 4)
			binary.LittleEndian.PutUint32(tmp, uint32(step_n))
			payload = append(payload, tmp...)

			binary.LittleEndian.PutUint32(tmp, uint32(10)) // num_of_gliders
			payload = append(payload, tmp...)

			binary.LittleEndian.PutUint32(tmp, uint32(10)) // total_execution_steps
			payload = append(payload, tmp...)

			export_count = 9
		} else {
			payload = []byte{}
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
					ExportCount:        export_count,
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

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("Game_of_life(%d)", step_n),
			CoreIndex:       0,
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: extrinsics,
		}
		var err error
		var workPackageHash common.Hash
		workPackageHash, err = n1.SubmitAndWaitForWorkPackage(ctx, wpr)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
			if step_n > 0 {
				step_n--
			}
			continue
		}

		if step_n == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			defer cancel()
			err = n1.SubmitAndWaitForPreimage(ctx, service0.ServiceCode, service0_child_code["game_of_life_child"].Code)
			if err != nil {
				log.Error(module, "SubmitAndWaitForPreimage", "err", err)
			} else {
				log.Info(module, "GAME OF LIFE CHILD LOADED")
			}
		} else {
			vm_export, err := n1.GetSegments(importedSegments)
			if err == nil {
				stepBytes := make([]byte, 4)
				binary.LittleEndian.PutUint32(stepBytes, uint32(step_n*10))
				out := append(stepBytes, flatten(vm_export)...)
				if true { // does not work on 2nd run
					ws_push(out)
				}
			}

		}
		prevWorkPackageHash = workPackageHash
	}
}
