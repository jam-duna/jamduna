//go:build network_test
// +build network_test

package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

const (
	SafroleTestEpochLen   = 4  // Safrole
	FallbackEpochLen      = 4  // Fallback
	FibTestEpochLen       = 1  // Assurance
	MegaTronEpochLen      = 50 // Orderaccumalation
	TransferEpochLen      = 3  // Transfer
	BalancesEpochLen      = 6  // Balance
	ScaleBalancesEpochLen = 6
	EmptyEpochLen         = 10
	Blake2bEpochLen       = 1
)

const (
	TargetedN_Mega_L          = 911 // Long  megaTron for rebustness test
	TargetedN_Mega_S          = 20  // Short megaTron for data publishing
	TargetedN_Fib             = 10
	TargetedN_Transfer        = 10
	TargetedN_Balances        = 20 // not used !!
	TargetedN_Scaled_Transfer = 600
	Targetedn_Scaled_Balances = 100
	TargetedN_Empty           = 8
	TargetedN_Blake2b         = 1
)

var targetNum = flag.Int("targetN", -1, "targetN")

func TestFallback(t *testing.T) {
	bufferTime := 30
	basePort := GenerateRandomBasePort()
	safroleTest(t, "fallback", FallbackEpochLen, basePort, bufferTime)
}

func TestSafrole(t *testing.T) {
	bufferTime := 30
	basePort := uint16(9000)
	safroleTest(t, "safrole", SafroleTestEpochLen, basePort, bufferTime)
}

func TestFib(t *testing.T) {
	targetN := TargetedN_Fib
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "fib", targetN)
}

func TestFib2(t *testing.T) {
	targetN := TargetedN_Fib
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "fib2", targetN)
}

func TestFib3(t *testing.T) {
	targetN := TargetedN_Fib
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtestclient(t, "fib3", targetN)
}

func TestMegatron(t *testing.T) {
	// Open file to save CPU Profile
	//fmt.Printf("prereq_test: %v\n", *prereq_test)
	//fmt.Printf("authoring_log: %v\n", *authoring_log)

	cpuProfile, err := os.Create("cpu.pprof")
	if err != nil {
		t.Fatalf("Unable to create CPU Profile file: %v", err)
	}
	defer cpuProfile.Close()
	// Start CPU Profile
	if err := pprof.StartCPUProfile(cpuProfile); err != nil {
		t.Fatalf("Unable to start CPU Profile: %v", err)
	}
	defer pprof.StopCPUProfile() // Stop profiling after test completion

	// Generate memory Profile
	memProfile, err := os.Create("mem.pprof")
	if err != nil {
		t.Fatalf("Unable to create memory Profile file: %v", err)
	}
	defer memProfile.Close()

	if err := pprof.WriteHeapProfile(memProfile); err != nil {
		t.Fatalf("Unable to write memory Profile: %v", err)
	}

	targetN := TargetedN_Mega_S
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("megatron targetNum: %v\n", targetN)

	jamtest(t, "megatron", targetN)
}

func TestTransfer(t *testing.T) {
	targetN := TargetedN_Transfer
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("transfer targetNum: %v\n", targetN)

	jamtest(t, "transfer", targetN)
}

func TestScaledTransfer(t *testing.T) {
	targetN := TargetedN_Scaled_Transfer
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("scaled_transfer targetNum: %v\n", targetN)
	jamtest(t, "scaled_transfer", targetN)
}

func TestBalances(t *testing.T) {
	targetN := TargetedN_Balances
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("balances targetNum: %v\n", targetN)
	jamtest(t, "balances", targetN)
}

func TestScaledBalances(t *testing.T) {
	targetN := Targetedn_Scaled_Balances
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("scaled_balances targetNum: %v\n", targetN)
	jamtest(t, "scaled_balances", targetN)
}

func TestEmpty(t *testing.T) {
	targetN := TargetedN_Empty
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("empty targetNum: %v\n", targetN)
	jamtest(t, "empty", targetN)
}

func TestBlake2b(t *testing.T) {
	targetN := TargetedN_Blake2b
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("blake2b targetNum: %v\n", targetN)
	jamtest(t, "blake2b", targetN)
}

func TestGameOfLife(t *testing.T) {
	targetN := 10
	fmt.Printf("game_of_life targetNum: %v\n", targetN)
	jamtest(t, "game_of_life", targetN)
}

func TestDisputes(t *testing.T) {
	basePort := GenerateRandomBasePort()
	nodes, err := SetUpNodes(JCEDefault, numNodes, basePort)
	if err != nil {
		t.Fatalf("Failed to set up nodes: %v", err)
	}
	nodes[0].AuditNodeType = "lying_judger_F"
	nodes[1].AuditNodeType = "lying_judger_T"
	nodes[2].AuditNodeType = "lying_judger_T"
	nodes[3].AuditNodeType = "lying_judger_T"
	nodes[4].AuditNodeType = "lying_judger_T"
	nodes[5].AuditNodeType = "lying_judger_T"

	// give some time for nodes to come up
	for {
		time.Sleep(1 * time.Second)
		currJCE := nodes[0].GetCurrJCE()
		if nodes[0].statedb.GetSafrole().CheckFirstPhaseReady(currJCE) {
			break
		}
	}

	// code length: 206
	bootstrapCode, err := os.ReadFile(statedb.BootstrapServiceFile)
	if err != nil {
		panic(0)
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)

	builderIdx := 1
	builderNode := nodes[builderIdx]
	new_service_idx := uint32(0)

	// Load testServices
	serviceNames := []string{"fib"}
	testServices, err := getServices(serviceNames, true)
	if err != nil {
		panic(32)
	}

	var previous_service_idx uint32
	for serviceName, service := range testServices {
		log.Info(module, "Builder storing TestService", "serviceName", serviceName, "codeHash", service.CodeHash)
		// set up service using the Bootstrap service
		refineContext := builderNode.statedb.GetRefineContext()
		codeWorkPackage := types.WorkPackage{
			Authorization:         []byte(""),
			AuthCodeHost:          0,
			AuthorizationCodeHash: bootstrap_auth_codehash,
			RefineContext:         refineContext,
			WorkItems: []types.WorkItem{
				{
					Service:            bootstrapService,
					CodeHash:           bootstrapCodeHash,
					Payload:            append(service.CodeHash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(service.Code)))...),
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		_, err = builderNode.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
			WorkPackage:     codeWorkPackage,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
			CoreIndex:       0,
		})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}

		new_service_found := false
		log.Info(module, "Waiting for service to be ready...", "service", serviceName)
		for !new_service_found {
			stateDB := builderNode.getState()
			if stateDB != nil && stateDB.Block != nil {
				stateRoot := stateDB.Block.GetHeader().ParentStateRoot
				t, _ := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), builderNode.store)
				k := common.ServiceStorageKey(bootstrapService, []byte{0, 0, 0, 0})
				service_account_byte, ok, err := t.GetServiceStorage(bootstrapService, k)
				if err != nil || !ok {
					time.Sleep(1 * time.Second)
					continue
				}
				time.Sleep(1 * time.Second)
				decoded_new_service_idx := uint32(types.DecodeE_l(service_account_byte))
				if decoded_new_service_idx != 0 && (decoded_new_service_idx != previous_service_idx) {
					service.ServiceCode = decoded_new_service_idx
					new_service_idx = decoded_new_service_idx
					new_service_found = true
					previous_service_idx = decoded_new_service_idx

					err = builderNode.BroadcastPreimageAnnouncement(new_service_idx, service.CodeHash, uint32(len(service.Code)), service.Code)
					if err != nil {
						fmt.Printf("BroadcastPreimageAnnouncement ERR %v\n", err)
					}
				}

			}
		}
	}

	fmt.Printf("All services are ready, Send preimage announcement\n")
	for _, service := range testServices {
		builderNode.BroadcastPreimageAnnouncement(service.ServiceCode, service.CodeHash, uint32(len(service.Code)), service.Code)
	}
	fmt.Printf("Wait until all the preimage blobs are ready\n")

	for done := false; !done; {

		ready := 0
		nservices := 0
		for _, service := range testServices {
			for _, n := range nodes {
				targetStateDB := n.getState()
				if targetStateDB != nil {
					code, ok, err := targetStateDB.ReadServicePreimageBlob(service.ServiceCode, service.CodeHash)
					if err != nil || !ok {
					} else if len(code) > 0 && bytes.Equal(code, service.Code) {
						ready++
					}
				}
			}
			nservices++
		}
		if ready == types.TotalValidators*nservices {
			done = true
		} else {
			time.Sleep(1 * time.Second)
		}
	}

	time.Sleep(50 * time.Second)
}
