//go:build network_test
// +build network_test

package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func safrole(n1 JNode, jceManager *ManualJCEManager) {
	if jceManager != nil {
		// replenish the JCE
		go jceManager.Replenish()
	}
}

// Monitor the Timeslot & Epoch Progression & Kill as Necessary
func (n *Node) TerminateAt(offsetTimeSlot uint32, maxTimeAllowed uint32) (bool, error) {
	initialTimeSlot := uint32(0)
	startTime := time.Now()

	// Terminate it at epoch N
	statusTicker := time.NewTicker(3 * time.Second)
	defer statusTicker.Stop()

	for done := false; !done; {
		<-statusTicker.C
		currTimeSlot := n.statedb.GetSafrole().Timeslot
		if initialTimeSlot == 0 && currTimeSlot > 0 {
			currEpoch, _ := n.statedb.GetSafrole().EpochAndPhase(currTimeSlot)
			initialTimeSlot = uint32(currEpoch) * types.EpochLength
		}
		currEpoch, currPhase := n.statedb.GetSafrole().EpochAndPhase(currTimeSlot)
		if currTimeSlot-initialTimeSlot >= offsetTimeSlot {
			done = true
			continue
		}
		if time.Since(startTime).Seconds() >= float64(maxTimeAllowed) {
			s := fmt.Sprintf("[TIMEOUT] H_t=%v e'=%v,m'=%v | missing %v Slot!", currTimeSlot, currEpoch, currPhase, currTimeSlot-initialTimeSlot)
			return false, fmt.Errorf(s)
		}
	}
	return true, nil
}

func waitForTermination(watchNode *Node, caseType string, targetedEpochLen int, bufferTime int, t *testing.T) {
	targetTimeslotLength := uint32(targetedEpochLen * types.EpochLength)
	maxTimeAllowed := (targetTimeslotLength+1)*types.SecondsPerSlot + uint32(bufferTime)

	done := make(chan bool, 1)
	errChan := make(chan error, 1)

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
		log.Info(module, "Completed")
	case err := <-errChan:
		t.Fatalf("[%v] Failed: %v", caseType, err)
	}
}

// TODO remove this
func jamtestclient(t *testing.T, jam string, targetN int) {

	var logLevel string
	flag.StringVar(&logLevel, "log", "trace", "Logging level (e.g., debug, info, warn, error, crit)")
	flag.Parse()
	peerListMapFile := "../cmd/archive_node/peerlist/local.json"
	peerListMap, err := ParsePeerList(peerListMapFile)
	if err != nil {
		t.Fatalf("Error loading peerlist %v: %v", peerListMapFile, err)
	}

	nodes, err := LoadRPCClients(peerListMap)
	if err != nil {
		panic(fmt.Sprintf("Error setting up nodes: %v\n", err))
	}
	fmt.Printf("Node Clients: %v\n", nodes)

	log.InitLogger(logLevel)
	//log.EnableModule(debugDA)
	// log.EnableModule(debugSeg)
	log.Info(module, "JAMTEST", "jam", jam, "targetN", targetN)

	nodeClient := nodes[1]

	if *prereq_test {
		test_prereq = true
	}

	fmt.Printf("Test PreReq: %v\n", test_prereq)

	// give some time for nodes to come up
	initTicker := time.NewTicker(1 * time.Second)
	defer initTicker.Stop()
	for range initTicker.C {
		currJCE, err := nodeClient.GetCurrJCE()
		if err != nil {
			continue
		}
		if currJCE >= types.EpochLength {
			break
		}
	}

	currJCE, _ := nodeClient.GetCurrJCE()
	fmt.Printf("Ready @JCE: %v\n", currJCE)
	time.Sleep(types.SecondsPerSlot * time.Second) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot
	// code length: 206
	bootstrapCode, err := types.ReadCodeWithMetadata(statedb.BootstrapServiceFile, "bootstrap")
	if err != nil {
		panic(0)
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)
	log.Info(module, "BootstrapCodeHash", "bootstrapCodeHash", bootstrapCodeHash, "codeLen", len(bootstrapCode), "fileName", statedb.BootstrapServiceFile)

	serviceNames := []string{"auth_copy", "fib"}
	new_service_idx := uint32(0)

	// Load testServices
	if jam == "fib2" || jam == "fib3" {
		serviceNames = []string{"corevm", "auth_copy"}
		log.EnableModule(log.StateDBMonitoring) //enable here to avoid concurrent map
	}

	fmt.Printf("Services to Load: %v\n", serviceNames)

	testServices, err := getServices(serviceNames, true)
	if err != nil {
		panic(32)
	}

	log.Trace(module, "Waiting for the first block to be ready...")
	time.Sleep(2 * types.SecondsPerSlot * time.Second) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot
	var previous_service_idx uint32
	for serviceName, service := range testServices {
		log.Info(module, "Builder storing TestService", "serviceName", serviceName, "codeHash", service.CodeHash)
		// set up service using the Bootstrap service
		codeWorkPackage := types.WorkPackage{
			Authorization:         []byte(""),
			AuthCodeHost:          bootstrapService,
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			WorkItems: []types.WorkItem{
				{
					Service:            bootstrapService,
					CodeHash:           bootstrapCodeHash,
					Payload:            append(service.CodeHash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(service.Code)))...),
					RefineGasLimit:     5678,
					AccumulateGasLimit: 9876,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		_, submissionErr := nodeClient.SubmitAndWaitForWorkPackage(ctx, &WorkPackageRequest{
			CoreIndex:       0,
			WorkPackage:     codeWorkPackage,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		})
		if submissionErr != nil {
			log.Crit("SubmitAndWaitForWorkpackage ERR", "err", submissionErr)
		}

		fmt.Printf("service:%v SendWorkPackage submission DONE %v\n", serviceName, submissionErr)

		new_service_found := false
		log.Info(module, "Waiting for service to be ready", "service", serviceName)
		for !new_service_found {
			k := common.ServiceStorageKey(bootstrapService, []byte{0, 0, 0, 0})
			service_account_byte, ok, err := nodeClient.GetServiceStorage(bootstrapService, k)
			if err != nil || !ok {
				time.Sleep(1 * time.Second)
				continue
			}
			time.Sleep(1 * time.Second)
			decoded_new_service_idx := uint32(types.DecodeE_l(service_account_byte))
			if decoded_new_service_idx != 0 && (decoded_new_service_idx != previous_service_idx) {
				log.Info(module, "!!! Service Found", "service", serviceName, "service_idx", decoded_new_service_idx)
				service.ServiceCode = decoded_new_service_idx
				new_service_idx = decoded_new_service_idx
				new_service_found = true
				previous_service_idx = decoded_new_service_idx
				err = nodeClient.SubmitAndWaitForPreimage(ctx, new_service_idx, service.Code)
				if err != nil {
					log.Error(debugP, "SubmitAndWaitForPreimage", "err", err)
				} else {
					log.Info(module, "SubmitAndWaitFor Preimage DONE", "service", serviceName, "service_idx", new_service_idx)
				}
			}
		}
	}
	fmt.Printf("All services are ready, Sending preimage announcement\n")

	for done := false; !done; {
		ready := 0
		nservices := 0
		for serviceName, service := range testServices {
			for _, nodeClient := range nodes {
				fmt.Printf("Calling nodeClient GetServicePreimage: Name:%v service.ServiceCode:%v CodeHash:%v\n", serviceName, service.ServiceCode, service.CodeHash)
				code, err := nodeClient.GetServicePreimage(service.ServiceCode, service.CodeHash)
				if err != nil {
					log.Trace(debugDA, "ReadServicePreimageBlob Pending")
				} else if len(code) > 0 {
					log.Info(module, "GetServicePreimage", "serviceName", serviceName, "ServiceIndex", service.ServiceCode, "codeHash", service.CodeHash, "len", len(code))
					if bytes.Equal(code, service.Code) {
						fmt.Printf("GetServicePreimage: %v Ready and Match\n", serviceName)
						ready++
					} else {
						fmt.Printf("GetServicePreimage: %v NOT Match!!!\n", serviceName)
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

	fmt.Printf("Service Loaded!\n")

	switch jam {
	case "fib3":
		targetN := 9
		fib3(nodes[1], testServices, targetN)
	}
}
