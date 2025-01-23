//go:build network
// +build network

package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func safroleTest(t *testing.T, caseType string, targetedEpochLen int, bufferTime int) {
	nodes, err := SetUpNodes(numNodes)
	if err != nil {
		panic(err)
	}

	var sendtickets bool
	if caseType == "safrole" {
		sendtickets = true
	} else {
		sendtickets = false
	}

	for _, n := range nodes {
		n.SetSendTickets(sendtickets)
	}

	targetTimeslotLength := uint32(targetedEpochLen * types.EpochLength)
	maxTimeAllowed := (targetTimeslotLength+1)*types.SecondsPerSlot + uint32(bufferTime)

	watchNode := nodes[len(nodes)-1]

	done := make(chan bool)
	errChan := make(chan error)

	go RunGrandpaGraphServer(watchNode)

	go func() {
		ok, err := watchNode.TerminateAt(targetTimeslotLength, maxTimeAllowed)
		if err != nil {
			errChan <- err
		} else if ok {
			done <- true
		}
	}()

	select {
	case <-done:
		log.Printf("[%v] Completed successfully", caseType)
	case err := <-errChan:
		t.Fatalf("[%v] Failed: %v", caseType, err)
	}
}

const SafroleTestEpochLen = 4 // Safrole
const FallbackEpochLen = 4    // Fallback
const FibTestEpochLen = 1     // Assurance
const MegaTronEpochLen = 5    // Orderaccumalation
const TransferEpochLen = 3    // Transfer
const BalancesEpochLen = 6    // Balance
const ScaleBalancesEpochLen = 6

func TestFallback(t *testing.T) {
	safroleTest(t, "fallback", FallbackEpochLen, 0)
}

func TestSafrole(t *testing.T) {
	safroleTest(t, "safrole", SafroleTestEpochLen, 0)
}

func TestFib(t *testing.T) {
	jamtest(t, "fib", FibTestEpochLen)
}

func TestMegatron(t *testing.T) {
	// Open file to save CPU Profile
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
	jamtest(t, "megatron", MegaTronEpochLen)
}

func TestTransfer(t *testing.T) {
	jamtest(t, "transfer", TransferEpochLen)
}

func TestBalances(t *testing.T) {
	jamtest(t, "balances", BalancesEpochLen)
}

func TestScaledBalances(t *testing.T) {
	jamtest(t, "scaled_balances", ScaleBalancesEpochLen)
}

func TestDisputes(t *testing.T) {
	nodes, err := SetUpNodes(numNodes)
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
		if nodes[0].statedb.GetSafrole().CheckFirstPhaseReady() {
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
	builderNode.preimages[bootstrapCodeHash] = bootstrapCode
	new_service_idx := uint32(0)

	// Load testServices
	serviceNames := []string{"fib"}
	testServices, err := getServices(serviceNames)
	if err != nil {
		panic(32)
	}

	// set builderNode's primages map
	for _, service := range testServices {
		builderNode.preimages[service.CodeHash] = service.Code
	}

	var previous_service_idx uint32
	for serviceName, service := range testServices {
		fmt.Printf("Builder storing TestService %s (%v)\n", serviceName, common.Str(service.CodeHash))
		// set up service using the Bootstrap service
		codeWorkPackage := types.WorkPackage{
			Authorization: []byte(""),
			AuthCodeHost:  bootstrapService,
			Authorizer:    types.Authorizer{},
			RefineContext: types.RefineContext{},
			WorkItems: []types.WorkItem{
				{
					Service:            bootstrapService,
					CodeHash:           bootstrapCodeHash,
					Payload:            append(service.CodeHash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(service.Code)))...),
					RefineGasLimit:     10000000,
					AccumulateGasLimit: 10000000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}
		err = builderNode.peersInfo[4].SendWorkPackageSubmission(codeWorkPackage, types.ExtrinsicsBlobs{}, 0)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}

		new_service_found := false
		fmt.Printf("Waiting for %s service to be ready...\n", serviceName)
		for !new_service_found {
			stateDB := builderNode.getState()
			if stateDB != nil && stateDB.Block != nil {
				stateRoot := stateDB.Block.GetHeader().ParentStateRoot
				t, _ := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), builderNode.store)
				k := []byte{0, 0, 0, 0}
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
					fmt.Printf("%s Service Index: %v\n", serviceName, service.ServiceCode)
					new_service_found = true
					previous_service_idx = decoded_new_service_idx
					//fmt.Printf("t.GetServiceStorage %v FOUND  %v\n", key, service_account_byte)

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
					// fmt.Printf(" check %s len(code)=%d expect %d => ready=%d\n", service.CodeHash, len(code), len(service.Code), ready)
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

func RunGrandpaGraphServer(watchNode *Node) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	block_graph_server := types.NewGraphServer()
	go block_graph_server.StartServer()
	for {
		select {
		case <-ticker.C:
			block_graph_server.Update(watchNode.block_tree)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}
