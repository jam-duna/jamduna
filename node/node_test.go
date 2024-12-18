//go:build network
// +build network

package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func TestFallback(t *testing.T) {
	safrole(false)
}

func TestSafrole(t *testing.T) {
	safrole(true)
}

func TestFib(t *testing.T) {
	jamtest("fib")
}

func TestMegatron(t *testing.T) {
	jamtest("megatron")
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
		err = builderNode.peersInfo[4].SendWorkPackageSubmission(codeWorkPackage, []byte{}, 0)
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
