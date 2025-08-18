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
	game_of_life_addr := fmt.Sprintf("0.0.0.0:%d", game_of_life_port)
	ws_push := StartGameOfLifeServer(game_of_life_addr, "./game_of_life.html")

	log.Info(log.Node, "Game of Life START")

	service0 := testServices["game_of_life"]
	if manifest {
		service0 = testServices["game_of_life_manifest"]
	}

	// service_authcopy := testServices["auth_copy"]

	service0_child_code, _ := getServices([]string{"game_of_life_child"}, false)
	service0_child_codehash := service0_child_code["game_of_life_child"].CodeHash

	service0_child_code_length := uint32(len(service0_child_code["game_of_life_child"].Code))
	service0_child_code_length_bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(service0_child_code_length_bytes, service0_child_code_length)

	// Put the extrinsic hash and length into the work item extrinsic
	segRoot := common.Hash{}
	next_import_cnt := 0
	executedStep := uint32(0)
	for game_of_life_n := 0; game_of_life_n <= maxTargetN; game_of_life_n++ {
		importedSegments := make([]types.ImportSegment, 0)
		export_count := uint16(0)
		if game_of_life_n > 0 {
			for i := 0; i < next_import_cnt; i++ {
				importedSegment := types.ImportSegment{
					RequestedHash: segRoot,
					Index:         uint16(i),
				}
				importedSegments = append(importedSegments, importedSegment)
			}
			fmt.Printf("Game_of_life N=%d, %v\n", game_of_life_n, importedSegments)

		}
		var payload []byte

		payload = service0_child_codehash.Bytes()
		tmp := make([]byte, 4)
		binary.LittleEndian.PutUint32(tmp, uint32(game_of_life_n))
		payload = append(payload, tmp...)
		gld := uint32(10)
		binary.LittleEndian.PutUint32(tmp, gld) // num_of_gliders
		payload = append(payload, tmp...)
		step := uint32(10 + 750*(game_of_life_n))
		binary.LittleEndian.PutUint32(tmp, step) // total_execution_steps
		payload = append(payload, tmp...)

		export_count = 9

		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0xs"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			WorkItems: []types.WorkItem{
				// {
				// 	Service:            statedb.AuthCopyServiceCode,
				// 	CodeHash:           service_authcopy.CodeHash,
				// 	Payload:            []byte{0, 0, 0, 0},
				// 	RefineGasLimit:     DefaultRefineGasLimit,
				// 	AccumulateGasLimit: DefaultAccumulateGasLimit,
				// 	ImportedSegments:   []types.ImportSegment{},
				// 	ExportCount:        0,
				// },
				{
					Service:            statedb.GameOfLifeCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   importedSegments,
					ExportCount:        export_count,
				},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		wpr := &WorkPackageRequest{
			Identifier:  fmt.Sprintf("Game_of_life(%d)", game_of_life_n),
			WorkPackage: workPackage,
		}
		wr, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
		if err != nil {
			log.Error(log.Node, "RobustSubmitAndWaitForWorkPackages", "err", err)
			return err
		}
		for _, result := range wr.Results {
			if result.Result.Err != types.WORKRESULT_OK {
				game_of_life_n--
				continue
			}
		}
		// TODO: fix this
		segRootHash := wr.AvailabilitySpec.ExportedSegmentRoot
		if game_of_life_n > 0 {
			result := wr.Results[0]
			var gas uint64
			if result.Result.Err == types.WORKRESULT_OK {
				gas = binary.LittleEndian.Uint64(result.Result.Ok)
				log.Info(log.Node, "Game of Life", "N", game_of_life_n, "child gas", gas, "step", step, "ExportedSegmentRoot", segRootHash.String())
			}
			Export_segments_count := wr.AvailabilitySpec.ExportedSegmentLength
			next_import_cnt = int(Export_segments_count)
			exports, err := n1.GetSegmentsByRequestedHash(segRootHash, next_import_cnt)
			if err != nil {
				log.Error(log.Node, "GetSegmentsByRequestedHash", "err", err)
				return err
			}
			vm_export := make([][]byte, export_count)
			if err == nil {
				stepBytes := make([]byte, 4)
				binary.LittleEndian.PutUint32(stepBytes, uint32(executedStep))

				for i := range vm_export {
					vm_export[i] = make([]byte, 4104)
				}

				for i, export := range exports {
					// order := binary.LittleEndian.Uint32(export[4:8]) % 48 // first page id
					vm_export[i] = export[:]
				}

				out := append(stepBytes, flatten(vm_export)...)
				if true { // does not work on 2nd run
					ws_push(out)
				}
			}
			// time.Sleep(30 * time.Second) // wait for the server to process the data
		}
		segRoot = segRootHash
		executedStep += step
	}
	return nil
}
