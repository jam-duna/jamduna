package rpcclient

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

var modeToServices = map[string][]string{
	"fib":          {"fib", "auth_copy"},
	"fib2":         {"corevm", "auth_copy"},
	"game_of_life": {"game_of_life", "auth_copy"},
}
var mode = flag.String("mode", "fib", "Mode to run the test client")
var isLocal = flag.Bool("isLocal", false, "Run local version")

func TestClient(t *testing.T) {

	flag.Parse()
	testMode := *mode
	local := *isLocal

	services, ok := modeToServices[testMode]
	if !ok {
		t.Fatalf("Invalid mode: %s", testMode)
	}

	addresses := make([]string, 6)
	port := common.GetJAMNetworkPort() + 1300
	if local {
		for i := 0; i < 6; i++ {
			addresses[i] = fmt.Sprintf("localhost:%d", port+i)
		}
	} else {
		for i := 0; i < 6; i++ {
			addresses[i] = fmt.Sprintf("%s-%d.jamduna.org:%d", common.GetJAMNetwork(), i, port)
		}
	}
	coreIndex := uint16(0)
	client, err := NewNodeClient(coreIndex, addresses)
	if err != nil {
		t.Fatalf("Error: %s", err)
	}
	defer client.Close()

	wsUrl := "ws://dot-0.jamduna.org:10800/ws"
	err = client.ConnectWebSocket(wsUrl)
	if err != nil {
		fmt.Println("WebSocket connection failed:", err)
		return
	}

	client.Subscribe("subscribeBestBlock", map[string]interface{}{"finalized": false})

	var game_of_life_ws_push func([]byte)
	if testMode == "game_of_life" {
		//this is local
		game_of_life_ws_push = node.StartGameOfLifeServer("localhost:8080", "../client/game_of_life.html")
	}

	done := false
	var services_map map[string]types.ServiceInfo
	for !done {
		fmt.Printf("LoadServices\n")
		services_map, err = client.LoadServices(services)
		if err != nil {
			fmt.Printf("LoadServices err %v\n", err)
			time.Sleep(15 * time.Second)
		} else {
			done = true
		}
	}

	switch testMode {
	case "fib":
		fib(t, client, services_map, 314159)
	case "fib2":
		fib2(t, client, services_map, 50)
	case "game_of_life":
		game_of_life(t, client, services_map, game_of_life_ws_push)
	default:
		t.Fatalf("Invalid mode: %s", testMode)
	}
}

func fib(t *testing.T, client *NodeClient, testServices map[string]types.ServiceInfo, targetN int) (err error) {
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
		fibIndex := testServices["fib"].ServiceIndex
		fibCodeHash := testServices["fib"].ServiceCodeHash
		authIndex := testServices["auth_copy"].ServiceIndex
		authCodeHash := testServices["auth_copy"].ServiceCodeHash
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte(""),
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			WorkItems: []types.WorkItem{
				{
					Service:            fibIndex,
					CodeHash:           fibCodeHash,
					Payload:            payload,
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   importedSegments,
					ExportCount:        1,
				},
				{
					Service:            authIndex,
					CodeHash:           authCodeHash,
					Payload:            []byte{},
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}
		coreIndex := uint16(0)
		workPackageHash := workPackage.Hash()
		workpackage_req := node.WorkPackageRequest{
			CoreIndex:       coreIndex,
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		fmt.Printf("Submitted Fib(%d) [wp=%s] to core %d\n", fibN, workPackage.Hash(), coreIndex)
		workPackageHash, err := client.RobustSubmitWorkPackage(workpackage_req, 5)
		if err != nil {
			t.Fatalf("Error: %s", err)
		}
		prevWorkPackageHash = workPackageHash
		fib_index := testServices["fib"].ServiceIndex
		k := common.ServiceStorageKey(fib_index, []byte{0})
		service_account_byte, ok, err := client.GetServiceValue(fib_index, k)
		if err != nil {
			fmt.Printf("client.GetServiceValue(fib_index=%d, k=%x) ERR=%s\n", fib_index, k, err)
		} else if !ok {
			fmt.Printf("client.GetServiceValue(fib_index=%d, k=%x) NOT OK\n", fib_index, k)
		} else {
			fmt.Printf("Fib(%v) result: %x\n", fibN, service_account_byte)
		}
	}
	return nil
}
func (client *NodeClient) RobustSubmitWorkPackage(workpackage_req node.WorkPackageRequest, maxTries int) (workPackageHash common.Hash, err error) {
	tries := 0
	for tries < maxTries {
		refine_context, err := client.GetRefineContext()
		if err != nil {
			return workPackageHash, err
		}
		workpackage_req.WorkPackage.RefineContext = refine_context
		workPackageHash = workpackage_req.WorkPackage.Hash()
		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		err = client.SubmitAndWaitForWorkPackage(ctx, workpackage_req)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
			tries = tries + 1
		} else {
			return workPackageHash, nil
		}
	}
	return workPackageHash, fmt.Errorf("Timeout after maxTries %d", maxTries)
}

func fib2(t *testing.T, client *NodeClient, testServices map[string]types.ServiceInfo, targetN int) {
	jam_key := []byte("jam")

	fib2_child_code, _ := getServices([]string{"corevm_child"}, false)
	fib2_child_codehash := fib2_child_code["corevm_child"].CodeHash
	fib2_child_code_length := uint32(len(fib2_child_code["corevm_child"].Code))
	fib2_child_code_length_bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(fib2_child_code_length_bytes, fib2_child_code_length)
	fmt.Printf("CHILD_CODE: %d %s\n", fib2_child_code_length, fib2_child_codehash)
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

	fibIndex := testServices["corevm"].ServiceIndex
	client.Subscribe(SubServiceValue, map[string]interface{}{"serviceID": fmt.Sprintf("%d", fibIndex)})

	extrinsics = append(extrinsics, extrinsic)
	for fibN := -1; fibN <= targetN; fibN++ {
		importedSegments := make([]types.ImportSegment, 0)
		if fibN > 0 {
			for i := 0; i < fibN; i++ {
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
			for i := 0; i < fibN+2; i++ {
				tmp := make([]byte, 4)
				binary.LittleEndian.PutUint32(tmp, uint32(fibN))
				payload = append(payload, tmp...)

				tmp = make([]byte, 4)
				binary.LittleEndian.PutUint32(tmp, uint32(1)) // function id
				payload = append(payload, tmp...)
			}
		}

		fibCodeHash := testServices["corevm"].ServiceCodeHash
		authIndex := testServices["auth_copy"].ServiceIndex
		authCodeHash := testServices["auth_copy"].ServiceCodeHash
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			//			RefineContext:         refine_context,
			WorkItems: []types.WorkItem{
				{
					Service:            fibIndex,
					CodeHash:           fibCodeHash,
					Payload:            payload,
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   importedSegments,
					Extrinsics:         work_item_extrinsic,
					ExportCount:        uint16(fibN + 1),
				},
				{
					Service:            authIndex,
					CodeHash:           authCodeHash,
					Payload:            []byte{},
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
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
		workpackage_req := node.WorkPackageRequest{
			CoreIndex:       0,
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: extrinsics,
		}
		workPackageHash, err := client.RobustSubmitWorkPackage(workpackage_req, 5)
		if err != nil {
			t.Fatalf("Error: %s", err)
		}

		prevWorkPackageHash = workPackageHash
		fib_index := fibIndex
		keys := []byte{0, 1, 2, 5, 6, 7, 8, 9}
		for _, key := range keys {
			k := common.ServiceStorageKey(fib_index, []byte{key})
			if true {
				service_account_byte, _, _ := client.GetServiceValue(fib_index, k)
				fmt.Printf("Fib2(%v) result %d: %x\n", fibN_string, key, service_account_byte)
			}
		}

		if fibN == -1 {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			defer cancel()
			err := client.SubmitAndWaitForPreimage(ctx, fibIndex, fib2_child_code["corevm_child"].Code)
			if err != nil {
				t.Fatalf("SubmitAndWaitForPreimage: %s", err)
			}
		}
		if fibN == 3 || fibN == 6 {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			defer cancel()
			err = client.SubmitAndWaitForPreimage(ctx, fibIndex, jam_key)
			if err != nil {
				t.Fatalf("SubmitAndWaitForPreimage: %s", err)
			}
		}

		for i := 0; i <= int(fibN); i++ {
			segment, err := client.Segment(workPackageHash, uint16(i))
			if err != nil {
				fmt.Printf("Segment Err i=%d @ FibN %d ERR %v\n", i, fibN, err)
			} else {
				if len(segment) > 24 {
					fmt.Printf("segment %d %v\n", i, segment[:24])
				} else {

				}
			}
		}
	}
}

func game_of_life(t *testing.T, client *NodeClient, testServices map[string]types.ServiceInfo, ws_push func([]byte)) {

	flatten := func(data [][]byte) []byte {
		var result []byte
		for _, block := range data {
			result = append(result, block[8:]...)
		}
		return result
	}

	fmt.Printf("Game of Life START\n")

	service0_child_code, _ := getServices([]string{"game_of_life_child"}, false)
	service0_child_codehash := service0_child_code["game_of_life_child"].CodeHash

	service0_child_code_length := uint32(len(service0_child_code["game_of_life_child"].Code))
	service0_child_code_length_bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(service0_child_code_length_bytes, service0_child_code_length)
	fmt.Printf("game_of_life_CHILD_CODE: %d %s\n", service0_child_code_length, service0_child_codehash)

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

	export_count := uint16(0)
	for step_n := 0; step_n <= 30; step_n++ {
		importedSegments := make([]types.ImportSegment, 0)
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

		game_of_life_index := testServices["game_of_life"].ServiceIndex
		game_of_life_code_hash := testServices["game_of_life"].ServiceCodeHash
		service_authcopy_index := testServices["auth_copy"].ServiceIndex
		service_authcopy_code_hash := testServices["auth_copy"].ServiceCodeHash

		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			WorkItems: []types.WorkItem{
				{
					Service:            game_of_life_index,
					CodeHash:           game_of_life_code_hash,
					Payload:            payload,
					RefineGasLimit:     56789,
					AccumulateGasLimit: 98765,
					ImportedSegments:   importedSegments,
					Extrinsics:         work_item_extrinsic,
					ExportCount:        export_count,
				},
				{
					Service:            service_authcopy_index,
					CodeHash:           service_authcopy_code_hash,
					Payload:            []byte{},
					RefineGasLimit:     56789,
					AccumulateGasLimit: 98765,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}

		workpackage_req := node.WorkPackageRequest{
			CoreIndex:       0,
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: extrinsics,
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		err := client.SubmitAndWaitForWorkPackage(ctx, workpackage_req)
		if err != nil {
			fmt.Printf("SubmitAndWaitForWorkPackage ERR %v\n", err)
		}
		workPackageHash := workPackage.Hash()

		if step_n > 0 {
			var vm_export [][]byte
			for i := 0; i < int(export_count); i++ {
				segment, err := client.Segment(workPackageHash, uint16(i))
				if err == nil {
					vm_export = append(vm_export, segment)
				}
			}
			stepBytes := make([]byte, 4)
			binary.LittleEndian.PutUint32(stepBytes, uint32(step_n*10))
			out := append(stepBytes, flatten(vm_export)...)

			ws_push(out)

		} else {
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			defer cancel()
			code := service0_child_code["game_of_life_child"].Code
			err = client.SubmitAndWaitForPreimage(ctx, game_of_life_index, code)
			if err != nil {
				t.Fatalf("SubmitAndWaitPreimage: %s", err)
			}
		}
		prevWorkPackageHash = workPackageHash
	}
}

var auth_code_bytes, _ = os.ReadFile(common.GetFilePath(statedb.BootStrapNullAuthFile))
var auth_code = statedb.AuthorizeCode{
	PackageMetaData:   []byte("bootstrap"),
	AuthorizationCode: auth_code_bytes,
}
var auth_code_encoded_bytes, _ = auth_code.Encode()
var auth_code_hash = common.Blake2Hash(auth_code_encoded_bytes) //pu
var auth_code_hash_hash = common.Blake2Hash(auth_code_hash[:])  //pa
var bootstrap_auth_codehash = auth_code_hash

func getServices(serviceNames []string, getmetadata bool) (services map[string]*types.TestService, err error) {
	services = make(map[string]*types.TestService)
	for i, serviceName := range serviceNames {
		fileName := fmt.Sprintf("/services/%s.pvm", serviceName)
		var code []byte
		if getmetadata {
			code, _ = types.ReadCodeWithMetadata(fileName, serviceName)
		} else {
			fileName := common.GetFilePath(fmt.Sprintf("/services/%s.pvm", serviceName))
			code, _ = os.ReadFile(fileName)
		}

		tmpServiceCode := uint32(i + 1)
		codeHash := common.Blake2Hash(code)
		services[serviceName] = &types.TestService{
			ServiceName: serviceName,
			ServiceCode: tmpServiceCode, // TEMPORARY
			FileName:    fileName,
			CodeHash:    codeHash,
			Code:        code,
		}
	}
	return
}
