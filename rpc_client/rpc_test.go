package rpcclient

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

var modeToServices = map[string][]string{
	"fib":  {"fib", "auth_copy"},
	"fib2": {"corevm", "auth_copy"},
}
var mode = flag.String("mode", "fib", "Mode to run the test client")

func TestClient(t *testing.T) {
	//9900+1200

	flag.Parse()

	services, ok := modeToServices[*mode]
	if !ok {
		t.Fatalf("Invalid mode: %s", *mode)
	}
	port := 9900 + 1200
	// address := fmt.Sprintf("localhost:%d", port)
	address := fmt.Sprintf("jam-0.jamduna.org:%d", port)
	client, err := NewNodeClient(address)
	if err != nil {
		t.Fatalf("Error: %s", err)
	}
	defer client.Close()
	services_map, err := client.LoadServices(services)
	if err != nil {
		t.Fatalf("Error: %s", err)
	}
	go client.RunState()
	switch *mode {
	case "fib":
		fib(t, client, services_map, 10)
	case "fib2":
		fib2(t, client, services_map, 10)
	default:
		t.Fatalf("Invalid mode: %s", *mode)
	}
}

func fib(t *testing.T, client *NodeClient, testServices map[string]types.ServiceInfo, targetN int) {
	time.Sleep(12 * time.Second)
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
		refine_context, err := client.GetRefineContext()
		if err != nil {
			t.Fatalf("Error: %s", err)
		}

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))
		fibIndex := testServices["fib"].ServiceIndex
		fibCodeHash := testServices["fib"].ServiceCodeHash
		authIndex := testServices["auth_copy"].ServiceIndex
		authCodeHash := testServices["auth_copy"].ServiceCodeHash
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			RefineContext:         refine_context,
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
		workPackageHash := workPackage.Hash()
		workpackage_req := types.WorkPackageRequest{
			CoreIndex:       0,
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}
		err = client.SendWorkPackage(workpackage_req)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// wait until the work report is pending
		for {
			time.Sleep(1 * time.Second)
			if client.GetState() == nil {
				continue
			}
			if client.GetState().AvailabilityAssignments[0] != nil {
				if false {
					var workReport types.WorkReport
					rho_state := client.GetState().AvailabilityAssignments[0]
					workReport = rho_state.WorkReport
					fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				}
				break
			}
		}

		// wait until the work report is cleared
		for {
			if client.GetState().AvailabilityAssignments[0] == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		prevWorkPackageHash = workPackageHash
		fib_index := testServices["fib"].ServiceIndex
		k := common.ServiceStorageKey(fib_index, []byte{0})
		service_account_byte, ok, err := client.GetServiceStorage(fib_index, k)
		if err != nil || ok == false {
			t.Fatalf("Error: %s", err)
		}
		fmt.Printf("Fib(%v) result: %x\n", fibN, service_account_byte)
	}
}

func fib2(t *testing.T, client *NodeClient, testServices map[string]types.ServiceInfo, targetN int) {
	time.Sleep(12 * time.Second)
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

	extrinsics = append(extrinsics, extrinsic)
	for fibN := -1; fibN <= 10; fibN++ {
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
		refine_context, err := client.GetRefineContext()
		if err != nil {
			t.Fatalf("Error: %s", err)
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

		fibIndex := testServices["corevm"].ServiceIndex
		fibCodeHash := testServices["corevm"].ServiceCodeHash
		authIndex := testServices["auth_copy"].ServiceIndex
		authCodeHash := testServices["auth_copy"].ServiceCodeHash
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			RefineContext:         refine_context,
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
		workPackageHash := workPackage.Hash()

		var fibN_string string
		if fibN == -1 {
			fibN_string = "init"
		} else {
			fibN_string = fmt.Sprintf("%d", fibN)
		}
		workpackage_req := types.WorkPackageRequest{
			CoreIndex:       0,
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: extrinsics,
		}
		err = client.SendWorkPackage(workpackage_req)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// wait until the work report is pending
		for {
			time.Sleep(1 * time.Second)
			if client.GetState() == nil {
				continue
			}
			find := false
			for _, packagehash := range client.GetState().AccumulationHistory[types.EpochLength-1].WorkPackageHash {
				if packagehash == workPackageHash {
					find = true
					break
				}
			}
			if find {
				break
			}
		}

		prevWorkPackageHash = workPackageHash
		time.Sleep(1 * time.Second)
		fib_index := fibIndex
		keys := []byte{0, 1, 2, 5, 6, 7, 8, 9}
		for _, key := range keys {
			k := common.ServiceStorageKey(fib_index, []byte{key})
			if true {
				service_account_byte, _, _ := client.GetServiceStorage(fib_index, k)
				fmt.Printf("Fib2(%v) result %d: %x\n", fibN_string, key, service_account_byte)
			}
		}
		if fibN == 3 || fibN == 6 {
			time.Sleep(3 * time.Second) // wait for the empty anchor to be set
			// err = n1.BroadcastPreimageAnnouncement(service0.ServiceCode, jam_key_hash, jam_key_length, jam_key)
			// if err != nil {
			// 	log.Error(debugP, "BroadcastPreimageAnnouncement", "err", err)
			// }
			_, err := client.AddPreimage(jam_key)
			if err != nil {
				t.Fatalf("AddPreimage: %s", err)
			}
			err = client.SendPreimageAnnouncement(fibIndex, jam_key)
			if err != nil {
				t.Fatalf("SendPreimageAnnouncement: %s", err)
			}
			time.Sleep(6 * time.Second) // make sure EP is sent and insert a time slot
		}

		if fibN == -1 {
			// err = n1.BroadcastPreimageAnnouncement(service0.ServiceCode, fib2_child_codehash, fib2_child_code_length, fib2_child_code["corevm_child"].Code)
			// if err != nil {
			// 	log.Error(debugP, "BroadcastPreimageAnnouncement", "err", err)
			// }
			_, err := client.AddPreimage(fib2_child_code["corevm_child"].Code)
			if err != nil {
				t.Fatalf("AddPreimage: %s", err)
			}
			err = client.SendPreimageAnnouncement(fibIndex, fib2_child_code["corevm_child"].Code)
			if err != nil {
				t.Fatalf("SendPreimageAnnouncement: %s", err)
			}
			time.Sleep(18 * time.Second) // make sure EP is sent and insert a time slot
		}
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
