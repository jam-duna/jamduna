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

func flatten(data [][]byte) []byte {
	var result []byte
	for _, block := range data {
		result = append(result, block[8:]...)
	}
	return result
}

func game_of_life(n1 JNode, testServices map[string]*types.TestService, targetN int, manifest bool) error {
	maxTargetN := targetN     // was 300000
	game_of_life_port := 8080 //was 80
	game_of_life_addr := fmt.Sprintf("localhost:%d", game_of_life_port)
	ws_push := StartGameOfLifeServer(game_of_life_addr, "./game_of_life.html")

	log.Info(log.Node, "Game of Life START")

	service0 := testServices["game_of_life"]
	if manifest {
		service0 = testServices["game_of_life_manifest"]
	}

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
	next_import_cnt := 0
	for game_of_life_n := 0; game_of_life_n <= maxTargetN; game_of_life_n++ {
		importedSegments := make([]types.ImportSegment, 0)
		export_count := uint16(0)
		if game_of_life_n > 1 {
			for i := 0; i < next_import_cnt; i++ {
				importedSegment := types.ImportSegment{
					RequestedHash: prevWorkPackageHash,
					Index:         uint16(i),
				}
				importedSegments = append(importedSegments, importedSegment)
			}
		}
		var payload []byte
		if game_of_life_n > 0 {
			payload = make([]byte, 0, 12)
			tmp := make([]byte, 4)
			binary.LittleEndian.PutUint32(tmp, uint32(game_of_life_n))
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
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   importedSegments,
					Extrinsics:         work_item_extrinsic,
					ExportCount:        export_count,
				},
				{
					Service:            statedb.AuthCopyServiceCode,
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
			Identifier:      fmt.Sprintf("Game_of_life(%d)", game_of_life_n),
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: extrinsics,
		}
		wr, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
		if err != nil {
			log.Error(log.Node, "RobustSubmitAndWaitForWorkPackages", "err", err)
			return err
		}
		// TODO: fix this
		workPackageHash := wr.AvailabilitySpec.WorkPackageHash
		if game_of_life_n == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			defer cancel()
			err = n1.SubmitAndWaitForPreimage(ctx, service0.ServiceCode, service0_child_code["game_of_life_child"].Code)
			if err != nil {
				log.Error(log.Node, "SubmitAndWaitForPreimage", "err", err)
			} else {
				log.Info(log.Node, "GAME OF LIFE CHILD LOADED")
			}
		} else {
			exports, Export_segments_count, err := n1.GetSegmentsByRequestedHash(workPackageHash)
			next_import_cnt = int(Export_segments_count)
			vm_export := make([][]byte, export_count)
			if err == nil {
				stepBytes := make([]byte, 4)
				binary.LittleEndian.PutUint32(stepBytes, uint32(game_of_life_n*10))

				for i := range vm_export {
					vm_export[i] = make([]byte, 4104)
				}

				for _, export := range exports {
					order := binary.LittleEndian.Uint32(export[4:8]) % 48 // first page id
					vm_export[order] = export[:]
				}

				out := append(stepBytes, flatten(vm_export)...)
				if true { // does not work on 2nd run
					ws_push(out)
				}
			}
		}
		prevWorkPackageHash = workPackageHash
	}
	return nil
}
