package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/rand"

	//"math/big"

	"net/http"
	_ "net/http/pprof"
	"os"
	"os/user"
	"time"
)

const (
	webServicePort = 8079
)

func sendStateTransition(endpoint string, st *statedb.StateTransition) (err error, check *statedb.StateTransitionCheck) {
	client := &http.Client{}
	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		log.Fatalf("Failed to create HTTP request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	//req.Body = os.NopCloser(http.NoBody)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to send block via HTTP: %v", err)
		return
	}
	defer resp.Body.Close()

	/*log.Printf("StateTransition sent to HTTP endpoint. Response status: %s", resp.Status)
	data, err := json.Marshal(st)
	if err != nil {
			log.Fatalf("Failed to serialize StateTransition: %v", err)
	}*/
	return
}

// Non-conformant metadata generator
func generateMetadata(idx int) (string, error) {
	//should be max of 128 bytes
	var nodeName string
	// assign metadata names for the first 6
	switch idx {
	case 0:
		nodeName = "Alice"
	case 1:
		nodeName = "Bob"
	case 2:
		nodeName = "Charlie"
	case 3:
		nodeName = "Dave"
	case 4:
		nodeName = "Eve"
	case 5:
		nodeName = "Fergie"
	default:
		nodeName = fmt.Sprintf("Node%d", idx)
	}
	remoteAddr := fmt.Sprintf("127.0.0.1:%d", 9900+idx)
	metadata := fmt.Sprintf("%s:%s", remoteAddr, nodeName)
	metadata_byte := []byte(metadata)

	if len(metadata_byte) > types.MetadataSizeInBytes {
		return metadata, fmt.Errorf("invalid input length for metadata %s", metadata)
	}
	return metadata, nil
}

func SetLevelDBPaths(numNodes int) []string {
	node_paths := make([]string, numNodes)
	// timeslot mark
	currJCE := common.ComputeCurrentJCETime()
	//currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
	currTS := currJCE * 6
	for i := 0; i < numNodes; i++ {
		node_idx := fmt.Sprintf("%d", i)
		node_path, err := computeLevelDBPath(node_idx, int(currTS))
		if err == nil {
			node_paths[i] = node_path
		}
	}
	return node_paths
}

func computeLevelDBPath(id string, unixtimestamp int) (string, error) {
	/* standardize on
	/tmp/<user>/jam/<unixtimestamp>/testdb#

	/tmp/ntust/jam/1727903082/node1/leveldb/
	/tmp/ntust/jam/1727903082/node1/data/

	/tmp/root/jam/1727903082/node1/

	*/
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("could not get current user: %v", err)
	}
	username := currentUser.Username
	path := fmt.Sprintf("/tmp/%s/jam/%v/node%v", username, unixtimestamp, id)
	return path, nil
}

func SetupQuicNetwork(network string, basePort uint16) (uint32, []string, map[uint16]*Peer, []types.ValidatorSecret, []string, error) {
	peers := make([]string, numNodes)
	peerList := make(map[uint16]*Peer)
	nodePaths := SetLevelDBPaths(numNodes)

	// Setup QuicNetwork using test keys
	validators, validatorSecrets, err := statedb.GenerateValidatorSecretSet(numNodes)
	if err != nil {
		return 0, nil, nil, nil, nil, fmt.Errorf("Failed to setup QuicNetwork")
	}

	epoch0Timestamp := statedb.NewEpoch0Timestamp()
	for i := uint16(0); i < numNodes; i++ {
		addr := fmt.Sprintf(quicAddr, basePort+i)
		peers[i] = addr
		peerList[i] = &Peer{
			PeerID:    i,
			PeerAddr:  addr,
			Validator: validators[i],
		}
	}

	// Print out peerList
	prettyPeerList, err := json.MarshalIndent(peerList, "", "  ")
	if err != nil {
		return epoch0Timestamp, nil, nil, nil, nil, fmt.Errorf("Failed to marshal peerList: %v, %v", err, prettyPeerList)
	}
	//fmt.Printf("PeerList: %s\n", prettyPeerList)

	return epoch0Timestamp, peers, peerList, validatorSecrets, nodePaths, nil
}

var Logger *storage.DebugLogger

func GenerateRandomBasePort() uint16 {

	// New seed every time
	seed := uint64(time.Now().UnixNano())
	r := rand.New(rand.NewSource(seed))

	// Generate base port in the 1xx00 - 6xx range to support multi-running
	region := r.Intn(6) + 1
	mid := r.Intn(100)
	basePort := uint16(region*10000 + mid*100)
	return basePort
}

func SetUpNodes(numNodes int, basePort uint16) ([]*Node, error) {
	network := types.Network
	GenesisFile := getGenesisFile(network)
	fmt.Printf("Using BasePort: %v\n", basePort)

	epoch0Timestamp, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork(network, basePort)

	if err != nil {
		return nil, err
	}
	if debugL {

		Logger = storage.InitDefaultLoggers()
		storage.Logger = Logger
	}

	nodes := make([]*Node, numNodes)
	godIncomingCh := make(chan uint32, 10) // node sends timeslots to this channel when authoring
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], GenesisFile, epoch0Timestamp, peers, peerList, ValidatorFlag, nodePaths[i], int(basePort)+i)
		if err != nil {
			panic(err)
			return nil, fmt.Errorf("Failed to create node %d: %v", i, err)
		}
		if godMode {
			node.setGodCh(&godIncomingCh)
		}
		nodes[i] = node
		if i == 0 {
			go node.runWebService(webServicePort + basePort)
		}
	}
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				break
			case ts := <-godIncomingCh: // Correct channel receive operation
				//fmt.Printf("god received %d\n", ts)
				for _, n := range nodes { // Loop through nodes
					//fmt.Printf("god sent %d to [N%d]\n", ts, i)
					n.receiveGodTimeslotUsed(ts)
				}
			}
		}

	}()
	return nodes, nil
}

// Monitor the Timeslot & Epoch Progression & Kill as Necessary
func (n *Node) TerminateAt(offsetTimeSlot uint32, maxTimeAllowed uint32) (bool, error) {
	fmt.Printf("*** TimeSlot Watcher on N%v ***\n", n.id)
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
			//fmt.Printf("[WATCH START] H_t=%v e'=%v,m'=%v | Terminated after %v slots (MaxAllowed:%v Sec)\n", initialTimeSlot, currEpoch, 0, offsetTimeSlot, maxTimeAllowed)

		}
		currEpoch, currPhase := n.statedb.GetSafrole().EpochAndPhase(currTimeSlot)
		if currTimeSlot-initialTimeSlot >= offsetTimeSlot {
			//fmt.Printf("[WATCH DONE] H_t=%v e'=%v,m'=%v | Elapsed: %v Sec\n", currTimeSlot, currEpoch, currPhase, int(time.Since(startTime).Seconds()))
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

func safrole(sendtickets bool) {
	nodes, err := SetUpNodes(numNodes, 10000)
	if err != nil {
		panic(err)
	}
	for _, n := range nodes {
		n.SetSendTickets(sendtickets)
	}
}

func getServices(serviceNames []string) (services map[string]*types.TestService, err error) {
	services = make(map[string]*types.TestService)
	for i, serviceName := range serviceNames {
		fileName := common.GetFilePath(fmt.Sprintf("/services/%s.pvm", serviceName))
		code, err1 := os.ReadFile(fileName)
		if err1 != nil {
			return services, err1
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
		fmt.Printf("%d: %s Code Hash: %x (Code Length: %v)\n", tmpServiceCode, serviceName, codeHash.Bytes(), len(code))
	}
	return
}

func ImportBlocks(config *types.ConfigJamBlocks) {
	// MK: DO NOT COMBINE importblock with jamtest or safrole generation!!!
	// assurances - jamtest("fib")
	// orderedaccumulation - jamtest("megatron")
}

func jamtest(t *testing.T, jam string, targetedEpochLen int, basePort uint16) {

	nodes, err := SetUpNodes(numNodes, basePort)
	if err != nil {
		panic("Error setting up nodes: %v\n")
	}
	Logger.RecordLogs(storage.Testing_record, fmt.Sprintf("[JAMTEST : %s] Start!!!\n", jam), true)
	_ = nodes
	block_graph_server := types.NewGraphServer(basePort)
	go block_graph_server.StartServer()
	ticker_blockserver := time.NewTicker(1 * time.Second)
	go func() {
		for {
			select {
			case <-ticker_blockserver.C:
				block_graph_server.Update(nodes[0].block_tree)
			default:
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
	// give some time for nodes to come up
	for {
		time.Sleep(1 * time.Second)
		if nodes[0].statedb.GetSafrole().CheckFirstPhaseReady() {
			break
		}
	}
	time.Sleep(7 * time.Second) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot
	// code length: 206
	fn := common.GetFilePath(statedb.BootstrapServiceFile)
	bootstrapCode, err := os.ReadFile(fn)
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
	if jam == "megatron" {
		serviceNames = []string{"fib", "tribonacci", "megatron"} // Others include: "padovan", "pell", "racaman"
	}
	if jam == "transfer" {
		serviceNames = []string{"transfer_0", "transfer_1"} // 2 transfer services share the same code
	}
	if jam == "balances" || jam == "scaled_balances" {
		serviceNames = []string{"balances"}
	}
	testServices, err := getServices(serviceNames)
	if err != nil {
		panic(32)
	}

	// set builderNode's primages map
	for _, service := range testServices {
		builderNode.preimages[service.CodeHash] = service.Code
	}
	fmt.Printf("Waiting for the first block to be ready...\n")
	time.Sleep(6 * time.Second) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot
	var previous_service_idx uint32
	for serviceName, service := range testServices {
		fmt.Printf("Builder storing TestService %s (%v)\n", serviceName, common.Str(service.CodeHash))
		// set up service using the Bootstrap service
		slot := builderNode.statedb.GetSafrole().GetTimeSlot()
		codeWorkPackage := types.WorkPackage{
			Authorization: []byte(""),
			AuthCodeHost:  bootstrapService,
			Authorizer:    types.Authorizer{},
			RefineContext: types.RefineContext{
				LookupAnchorSlot: slot,
			},
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
		// use get coworkers peers
		core0_peers := builderNode.GetCoreCoWorkersPeers(0)
		// ramdom pick the index from 0, 1, 2
		randomIdx := rand.Intn(3)
		err = core0_peers[randomIdx].SendWorkPackageSubmission(codeWorkPackage, types.ExtrinsicsBlobs{}, 0)
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
						// TODO
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

	switch jam {
	case "megatron":
		megatron(nodes, testServices)
	case "fib":
		fib(nodes, testServices)
	case "transfer":
		transfer(nodes, testServices)
	case "balances":
		balances(nodes, testServices)
	case "scaled_balances":
		scaled_balances(nodes, testServices)
	}
}

func fib(nodes []*Node, testServices map[string]*types.TestService) {
	fmt.Printf("Start FIB\n")

	service0 := testServices["fib"]
	n1 := nodes[1]
	n4 := nodes[4]
	core := 0
	prevWorkPackageHash := common.Hash{}
	for fibN := 1; fibN <= 10; fibN++ {
		importedSegments := make([]types.ImportSegment, 0)
		if fibN > 1 {
			importedSegment := types.ImportSegment{
				RequestedHash: prevWorkPackageHash,
				Index:         0,
			}
			importedSegments = append(importedSegments, importedSegment)
		}
		timeslot := nodes[1].statedb.Block.GetHeader().Slot
		refine_context := types.RefineContext{
			Anchor:           common.Hash{},
			StateRoot:        common.Hash{},
			BeefyRoot:        common.Hash{},
			LookupAnchor:     common.Hash{},
			LookupAnchorSlot: timeslot,
			Prerequisites:    []common.Hash{},
		}
		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))
		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // TODO: set up null-authorizer
			Authorizer:    types.Authorizer{},
			RefineContext: refine_context,
			WorkItems: []types.WorkItem{
				{
					Service:            service0.ServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     10000000,
					AccumulateGasLimit: 10000000,
					ImportedSegments:   importedSegments,
					ExportCount:        1,
				},
			},
		}
		workPackageHash := workPackage.Hash()

		fmt.Printf("\n** \033[36m FIB=%v \033[0m workPackage: %v **\n", fibN, common.Str(workPackageHash))
		core0_peers := n1.GetCoreCoWorkersPeers(uint16(core))
		ramdamIdx := rand.Intn(3)
		err := core0_peers[ramdamIdx].SendWorkPackageSubmission(workPackage, types.ExtrinsicsBlobs{}, 0)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// wait until the work report is pending
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
				if false {
					var workReport types.WorkReport
					rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
					workReport = rho_state.WorkReport
					fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				}
				break
			}
		}

		// wait until the work report is cleared
		for {
			if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		prevWorkPackageHash = workPackageHash
	}

}

func megatron(nodes []*Node, testServices map[string]*types.TestService) {

	fmt.Printf("Start Fib_Trib\n")
	service0 := testServices["fib"]
	service1 := testServices["tribonacci"]
	serviceM := testServices["megatron"]
	fmt.Printf("service0: %v, codehash: %v (len=%v) | %v\n", service0.ServiceCode, service0.CodeHash, len(service0.Code), service0.ServiceName)
	fmt.Printf("service1: %v, codehash: %v (len=%v) | %v\n", service1.ServiceCode, service1.CodeHash, len(service0.Code), service1.ServiceName)
	fmt.Printf("serviceM: %v, codehash: %v (len=%v) | %v\n", serviceM.ServiceCode, serviceM.CodeHash, len(service0.Code), serviceM.ServiceName)
	Fib_Trib_WorkPackages := make([]types.WorkPackage, 0)
	Meg_WorkPackages := make([]types.WorkPackage, 0)
	prevWorkPackageHash := common.Hash{}
	// ================================================
	// make n workpackages for Fib and Trib
	targetNMax := 10
	for n := 0; n < targetNMax; n++ {
		fibImportedSegments := make([]types.ImportSegment, 0)
		tribImportedSegments := make([]types.ImportSegment, 0)
		timeslot := nodes[1].statedb.GetSafrole().GetTimeSlot()
		// lastHeaderHash := nodes[1].statedb.HeaderHash
		refineContext := types.RefineContext{
			// These values don't matter until we have a historical lookup -- which we do not!
			Anchor:       common.Hash{},
			StateRoot:    common.Hash{},
			BeefyRoot:    common.Hash{},
			LookupAnchor: common.Hash{},
			// LookupAnchorSlot: timeslot + 100,// TODO: check this
			LookupAnchorSlot: timeslot,
			Prerequisites:    []common.Hash{},
		}
		workPackage := types.WorkPackage{}
		if n > 0 {
			fibImportedSegments = append(fibImportedSegments, types.ImportSegment{
				RequestedHash: prevWorkPackageHash,
				Index:         0,
			})
			tribImportedSegments = append(tribImportedSegments, types.ImportSegment{
				RequestedHash: prevWorkPackageHash,
				Index:         1, // TODO: check
			})
			payload := make([]byte, 4)
			binary.LittleEndian.PutUint32(payload, uint32(n+1))
			workPackage = types.WorkPackage{
				Authorization: []byte("0x"), // TODO: set up null-authorizer
				AuthCodeHost:  service0.ServiceCode,
				Authorizer:    types.Authorizer{},
				RefineContext: refineContext,
				WorkItems: []types.WorkItem{
					{
						Service:            service0.ServiceCode,
						CodeHash:           service0.CodeHash,
						Payload:            payload,
						RefineGasLimit:     10000000,
						AccumulateGasLimit: 10000000,
						ImportedSegments:   fibImportedSegments,
						ExportCount:        1,
					},
					{
						Service:            service1.ServiceCode,
						CodeHash:           service1.CodeHash,
						Payload:            payload,
						RefineGasLimit:     10000000,
						AccumulateGasLimit: 10000000,
						ImportedSegments:   tribImportedSegments,
						ExportCount:        1,
					},
				},
			}
		} else {
			payload := make([]byte, 4)
			binary.LittleEndian.PutUint32(payload, uint32(n+1))
			workPackage = types.WorkPackage{
				Authorization: []byte("0x"), // TODO: set up null-authorizer
				AuthCodeHost:  service0.ServiceCode,
				Authorizer:    types.Authorizer{},
				RefineContext: refineContext,
				WorkItems: []types.WorkItem{
					{
						Service:            service0.ServiceCode,
						CodeHash:           service0.CodeHash,
						Payload:            payload,
						RefineGasLimit:     10000000,
						AccumulateGasLimit: 10000000,
						ImportedSegments:   fibImportedSegments,
						ExportCount:        1,
					},
					{
						Service:            service1.ServiceCode,
						CodeHash:           service1.CodeHash,
						Payload:            payload,
						RefineGasLimit:     10000000,
						AccumulateGasLimit: 10000000,
						ImportedSegments:   tribImportedSegments,
						ExportCount:        1,
					},
				},
			}
		}

		Fib_Trib_WorkPackages = append(Fib_Trib_WorkPackages, workPackage)
		workPackageHash := workPackage.Hash()
		prevWorkPackageHash = workPackageHash

	}
	// =================================================
	// make n workpackages for Megatron
	for megaN := 0; megaN < targetNMax; megaN++ {
		importedSegmentsM := make([]types.ImportSegment, 0)
		prereq := make([]common.Hash, 0)
		prereq = append(prereq, Fib_Trib_WorkPackages[megaN].Hash())
		// prereq = append(prereq, common.BytesToHash([]byte("hack")))
		ts := nodes[1].statedb.GetSafrole().GetTimeSlot()
		refineContext := types.RefineContext{
			// These values don't matter until we have a historical lookup -- which we do not!
			Anchor:       common.Hash{},
			StateRoot:    common.Hash{},
			BeefyRoot:    common.Hash{},
			LookupAnchor: common.Hash{},
			// LookupAnchorSlot: ts + 100, // TODO: check this
			LookupAnchorSlot: ts,
			Prerequisites:    prereq,
		}

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(megaN))
		payloadM := make([]byte, 8)
		binary.LittleEndian.PutUint32(payloadM[0:4], uint32(service0.ServiceCode))
		binary.LittleEndian.PutUint32(payloadM[4:8], uint32(service1.ServiceCode))

		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // TODO: set up null-authorizer
			AuthCodeHost:  serviceM.ServiceCode,
			Authorizer:    types.Authorizer{},
			RefineContext: refineContext,
			WorkItems: []types.WorkItem{
				{
					Service:            serviceM.ServiceCode,
					CodeHash:           serviceM.CodeHash,
					Payload:            payloadM,
					RefineGasLimit:     10000000,
					AccumulateGasLimit: 10000000,
					ImportedSegments:   importedSegmentsM,
					ExportCount:        0,
				},
			},
		}
		workPackageHash := workPackage.Hash()
		Meg_WorkPackages = append(Meg_WorkPackages, workPackage)
		prevWorkPackageHash = workPackageHash
	}
	fmt.Printf("%v Fib_Trib_WorkPackages:\n", len(Fib_Trib_WorkPackages))
	for wp_idx, wp := range Fib_Trib_WorkPackages {
		fmt.Printf("  Fib_Trib_WorkPackage #%v: %v\n", wp_idx, wp.Hash())
	}
	fmt.Printf("%v Megatron_WorkPackages:\n", len(Meg_WorkPackages))
	for wp_idx, wp := range Meg_WorkPackages {
		fmt.Printf("  Meg_WorkPackage #%v %v. PreReqs=%v\n", wp_idx, wp.Hash(), wp.RefineContext.Prerequisites)
	}
	// setting up the delay send for the fib_tri
	for _, n := range nodes {
		for _, wp := range Fib_Trib_WorkPackages {
			if test_prereq {
				n.delaysend[wp.Hash()] = 1
			}
		}
	}
	// =================================================
	// set up ticker for loop
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	ticker_runtime := time.NewTicker(2 * time.Second)
	defer ticker_runtime.Stop()
	Fib_Tri_Chan := make(chan types.WorkPackage, 1)
	Fib_Tri_counter := 0
	Fib_Tri_successful := make(chan bool)
	Fib_Tri_Ready := true
	Fib_Tri_Keeper := false
	Meg_Chan := make(chan types.WorkPackage, 1)
	Meg_counter := 0
	Meg_successful := make(chan bool)
	Meg_Ready := true
	Meg_Keeper := false
	curr_Meg_WorkPackage := types.WorkPackage{}
	curr_fib_tri_WorkPackage := types.WorkPackage{}
	fmt.Printf("Guarantor Assignment\n")
	for _, assign := range nodes[0].statedb.GuarantorAssignments {
		vid := nodes[0].statedb.GetSafrole().GetCurrValidatorIndex(assign.Validator.GetEd25519Key())
		fmt.Printf("v%d->c%v\n", vid, assign.CoreIndex)
	}
	/*
	 get the core index every time before we send the workpackage
	*/
	ok := false
	sentLastWorkPackage := false
	FinalRho := false
	FinalAssurance := false
	FinalMeg := false
	for {
		if ok {
			break
		}
		select {
		case <-ticker.C:
			if sentLastWorkPackage {
				if FinalRho && FinalAssurance && FinalMeg {
					fmt.Printf("Meg Finish\n")
					ok = true
					Logger.RecordLogs(storage.Testing_record, "[JAMTEST : megatron] Success!!!\n", true)
					break
				} else if nodes[0].statedb.JamState.AvailabilityAssignments[0] != nil && nodes[0].statedb.JamState.AvailabilityAssignments[1] != nil {
					FinalRho = true
				} else if FinalRho {
					if FinalAssurance {
						if nodes[0].statedb.JamState.AvailabilityAssignments[1] == nil {
							FinalMeg = true
						}

					} else if nodes[0].statedb.JamState.AvailabilityAssignments[0] == nil {
						FinalAssurance = true
					}

				}
			} else {
				if Fib_Tri_counter == targetNMax && Meg_counter == targetNMax {
					fmt.Printf("All workpackages are sent\n")
					sentLastWorkPackage = true
				} else if (nodes[0].IsCoreReady(0) && Meg_Ready) && (nodes[0].IsCoreReady(1) && Fib_Tri_Ready) {
					// if false{

					// send workpackages to the network

					curr_fib_tri_WorkPackage = Fib_Trib_WorkPackages[Fib_Tri_counter]
					Fib_Tri_Chan <- curr_fib_tri_WorkPackage
					Fib_Tri_counter++
					Fib_Tri_Ready = false

					// send workpackages to the network
					curr_Meg_WorkPackage = Meg_WorkPackages[Meg_counter]
					Meg_Chan <- curr_Meg_WorkPackage
					Meg_counter++
					Meg_Ready = false
					fmt.Printf("**  Preparing Fib_Tri#%v %v Meg#%v %v **\n", Fib_Tri_counter, curr_fib_tri_WorkPackage.Hash().String_short(), Meg_counter, curr_Meg_WorkPackage.Hash().String_short())
				} else if (nodes[0].statedb.JamState.AvailabilityAssignments[0] != nil) && (nodes[0].statedb.JamState.AvailabilityAssignments[1] != nil && !Fib_Tri_Ready) {

					Meg_Ready = false
					Meg_Keeper = true
					Fib_Tri_Ready = false
					Fib_Tri_Keeper = true

				} else if (Meg_Keeper && nodes[0].IsCoreReady(0)) && (Fib_Tri_Keeper && nodes[0].IsCoreReady(1)) {
					Meg_Ready = true
					Meg_Keeper = false
					Fib_Tri_Ready = true
					Fib_Tri_Keeper = false
				}
			}
		// case <-ticker_runtime.C:
		// 	var stats runtime.MemStats
		// 	runtime.ReadMemStats(&stats)

		// 	numGoroutine := runtime.NumGoroutine()
		// 	numCPU := runtime.NumCPU()
		// 	cpuPercent := float64(runtime.NumCgoCall()) / float64(numCPU)

		// 	fmt.Printf("\033[31mGoroutines: %d, CPUs: %d, CPU Usage (approx): %.2f%%\033[0m\n", numGoroutine, numCPU, cpuPercent*100)

		case workPackage := <-Fib_Tri_Chan:
			// submit to core 1
			// v0, v3, v5 => core
			senderIdx := 5
			fmt.Printf("\n** \033[32m Fib_Tri %d \033[0m workPackage: %v **\n", Fib_Tri_counter, common.Str(workPackage.Hash()))
			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
			go func() {
				defer cancel()
				sendWorkPackageTrack(ctx, nodes[senderIdx], workPackage, uint16(1), Fib_Tri_successful, types.ExtrinsicsBlobs{})
			}()
		case successful := <-Fib_Tri_successful:
			if successful {
				fmt.Printf("Fib_Tri %d is successful\n", Fib_Tri_counter)
			} else {
				panic(fmt.Sprintf("Fib_Tri %d is failed\n", Fib_Tri_counter))
			}
		case successful := <-Meg_successful:

			if successful {
				fmt.Printf("Meg %d is successful\n", Meg_counter)
			} else {
				panic(fmt.Sprintf("Meg %d is failed\n", Meg_counter))
			}

		case workPackage := <-Meg_Chan:
			// submit to core 0
			// CE133_WorkPackageSubmission: n1 => n4
			// v1, v2, v4 => core
			// random select 1 sender and 1 receiver
			// Randomly select sender and receiver

			fmt.Printf("\n** \033[36m MEGATRON %d \033[0m workPackage: %v **\n", Meg_counter, common.Str(workPackage.Hash()))
			// senderIdx := rand.Intn(6)
			// receiverIdx := rand.Intn(3)
			// core0_peers := nodes[senderIdx].GetCoreCoWorkersPeers(0)
			// for senderIdx == int(core0_peers[receiverIdx].PeerID) {
			// 	receiverIdx = rand.Intn(3)
			// }
			megCoreIdx := uint16(0)

			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
			go func() {
				defer cancel()
				sendWorkPackageTrack(ctx, nodes[5], workPackage, megCoreIdx, Meg_successful, types.ExtrinsicsBlobs{})
			}()
		}

	}
}

func sendWorkPackageTrack(ctx context.Context, senderNode *Node, workPackage types.WorkPackage, receiverCore uint16, successful chan bool, extrinsics types.ExtrinsicsBlobs) {
	ticker := time.NewTicker(types.SecondsPerSlot * time.Second)
	ticker2 := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	defer ticker2.Stop()
	workPackageHash := workPackage.Hash()
	trialCount := 0
	MaxTrialCount := 100
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Context cancelled for work package %v\n", workPackageHash)
			return
		case <-ticker.C:

			// Sending the work package again
			corePeers := senderNode.GetCoreCoWorkersPeers(receiverCore)
			randIdx := rand.Intn(len(corePeers))
			err := corePeers[randIdx].SendWorkPackageSubmission(workPackage, extrinsics, receiverCore)
			log := fmt.Sprintf("[N%v -> p[%v] SendWorkPackageSubmission to core_%d for %v trial=%v for timeslot %d\n", senderNode.id, corePeers[randIdx].PeerID, receiverCore, workPackageHash, trialCount, senderNode.statedb.GetSafrole().GetTimeSlot())
			Logger.RecordLogs(storage.EG_status, log, true)
			if err != nil {
				fmt.Printf("SendWorkPackageSubmission ERR %v, sender: %d, receiver %d\n", err, senderNode.id, corePeers[randIdx].PeerID)
			}
			trialCount++
			if trialCount > MaxTrialCount {
				successful <- false
				return
			}
		case <-ticker2.C:
			if senderNode.statedb.JamState.AvailabilityAssignments[receiverCore] != nil {
				rho := senderNode.statedb.JamState.AvailabilityAssignments[receiverCore]
				pendingWPHash := rho.WorkReport.AvailabilitySpec.WorkPackageHash
				if workPackageHash == pendingWPHash {
					fmt.Printf("Found pending work package %v at core %v!\n", workPackageHash, receiverCore)
					successful <- true
					return
				} else {
					fmt.Printf("Found different pending work package %s at core %v!\n", rho.WorkReport.AvailabilitySpec.WorkPackageHash.String_short(), receiverCore)
				}
			}

		}

	}
}

func transfer(nodes []*Node, testServices map[string]*types.TestService) {
	fmt.Printf("\n=========================Start Transfer=========================\n\n")
	service0 := testServices["transfer_0"]
	service1 := testServices["transfer_1"]

	var previous_sa_0_balance, previous_sa_1_balance uint64

	n1 := nodes[1]
	n4 := nodes[4]
	core := 0

	// Show initial balances
	sa_0, _, _ := n1.getState().GetService(service0.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_0\033[0m initial balance: \033[32m%v\033[0m\n", sa_0.Balance)
	previous_sa_0_balance = sa_0.Balance

	sa_1, _, _ := n1.getState().GetService(service1.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_1\033[0m initial balance: \033[32m%v\033[0m\n", sa_1.Balance)
	previous_sa_1_balance = sa_1.Balance

	Transfer_WorkPackages := make([]types.WorkPackage, 0)
	prevWorkPackageHash := common.Hash{}
	_ = prevWorkPackageHash
	// ================================================
	// make n workpackages for Transfer
	Transfer_num := 10
	for n := 1; n <= Transfer_num; n++ {
		// timeslot := n1.statedb.GetSafrole().GetTimeSlot()
		timeslot := nodes[1].statedb.Block.GetHeader().Slot
		refineContext := types.RefineContext{
			Anchor:           common.Hash{},
			StateRoot:        common.Hash{},
			BeefyRoot:        common.Hash{},
			LookupAnchor:     common.Hash{},
			LookupAnchorSlot: timeslot,
			Prerequisites:    []common.Hash{},
		}

		workPackage := types.WorkPackage{}

		// Bank service send 1000 tokens to transfer_0 and transfer_1
		if n%2 == 0 {
			payload := make([]byte, 8)
			reciver := make([]byte, 4)
			binary.LittleEndian.PutUint32(reciver, service1.ServiceCode)
			amount := make([]byte, 4)
			binary.LittleEndian.PutUint32(amount, uint32(n)*10)
			payload = append(reciver, amount...)

			workPackage = types.WorkPackage{
				Authorization: []byte("0x"),
				AuthCodeHost:  service0.ServiceCode,
				Authorizer:    types.Authorizer{},
				RefineContext: refineContext,
				WorkItems: []types.WorkItem{
					{
						Service:            service0.ServiceCode,
						CodeHash:           service0.CodeHash,
						Payload:            payload,
						RefineGasLimit:     10000000,
						AccumulateGasLimit: 10000000,
						ImportedSegments:   make([]types.ImportSegment, 0),
						ExportCount:        1,
					},
				},
			}
		} else {
			payload := make([]byte, 8)
			reciver := make([]byte, 4)
			binary.LittleEndian.PutUint32(reciver, service0.ServiceCode)
			amount := make([]byte, 4)
			binary.LittleEndian.PutUint32(amount, uint32(n)*10)
			payload = append(reciver, amount...)

			workPackage = types.WorkPackage{
				Authorization: []byte("0x"),
				AuthCodeHost:  service1.ServiceCode,
				Authorizer:    types.Authorizer{},
				RefineContext: refineContext,
				WorkItems: []types.WorkItem{
					{
						Service:            service1.ServiceCode,
						CodeHash:           service1.CodeHash,
						Payload:            payload,
						RefineGasLimit:     10000000,
						AccumulateGasLimit: 10000000,
						ImportedSegments:   make([]types.ImportSegment, 0),
						ExportCount:        1,
					},
				},
			}
		}

		Transfer_WorkPackages = append(Transfer_WorkPackages, workPackage)
		workPackageHash := workPackage.Hash()
		prevWorkPackageHash = workPackageHash

		fmt.Printf("\n** \033[36m TRANSFER=%v \033[0m workPackage: %v **\n", n, common.Str(workPackageHash))

		core0_peers := n1.GetCoreCoWorkersPeers(uint16(core))
		ramdamIdx := rand.Intn(3)
		err := core0_peers[ramdamIdx].SendWorkPackageSubmission(workPackage, types.ExtrinsicsBlobs{}, 0)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// wait until the work report is pending
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
				if false {
					var workReport types.WorkReport
					rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
					workReport = rho_state.WorkReport
					fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				}
				break
			}
		}

		// wait until the work report is cleared
		for {
			if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		prevWorkPackageHash = workPackageHash

		if n%2 == 0 {
			fmt.Printf("\n\033[38;5;208mtransfer_0\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_1\033[0m\n", n*10)
		} else {
			fmt.Printf("\n\033[38;5;208mtransfer_1\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_0\033[0m\n", n*10)
		}

		sa_0, _, _ := n1.getState().GetService(service0.ServiceCode)
		fmt.Printf("\033[38;5;208mtransfer_0_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", previous_sa_0_balance, sa_0.Balance)
		previous_sa_0_balance = sa_0.Balance

		sa_1, _, _ := n1.getState().GetService(service1.ServiceCode)
		fmt.Printf("\033[38;5;208mtransfer_1_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", previous_sa_1_balance, sa_1.Balance)
		previous_sa_1_balance = sa_1.Balance

		time.Sleep(6 * time.Second)
	}
}

// some useful struct, function for balances test case
const AssetSize = 89 // 8 + 32 + 8 + 32 + 8 + 1

type Asset struct {
	AssetID     uint64
	Issuer      [32]byte // 32 bytes
	MinBalance  uint64
	Symbol      [32]byte // 32 bytes
	TotalSupply uint64
	Decimals    uint8
}

func (a *Asset) Bytes() []byte {
	result := make([]byte, AssetSize)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.AssetID)
	buffer.Write(a.Issuer[:])
	binary.Write(buffer, binary.LittleEndian, a.MinBalance)
	buffer.Write(a.Symbol[:])
	binary.Write(buffer, binary.LittleEndian, a.TotalSupply)
	buffer.WriteByte(a.Decimals)

	return buffer.Bytes()
}

func (a *Asset) FromBytes(data []byte) (Asset, error) {
	if len(data) != AssetSize {
		return Asset{}, fmt.Errorf("invalid data size: expected %d, got %d", AssetSize, len(data))
	}
	buffer := bytes.NewReader(data)
	asset := Asset{}
	binary.Read(buffer, binary.LittleEndian, &asset.AssetID)
	buffer.Read(asset.Issuer[:])
	binary.Read(buffer, binary.LittleEndian, &asset.MinBalance)
	buffer.Read(asset.Symbol[:])
	binary.Read(buffer, binary.LittleEndian, &asset.TotalSupply)
	dec, _ := buffer.ReadByte()
	asset.Decimals = dec
	return asset, nil
}

const AccountSize = 24 // 8 (nonce) + 8 (free) + 8 (reserved)
type Account struct {
	Nonce    uint64
	Free     uint64
	Reserved uint64
}

func (a *Account) Bytes() []byte {
	result := make([]byte, AccountSize)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.Nonce)
	binary.Write(buffer, binary.LittleEndian, a.Free)
	binary.Write(buffer, binary.LittleEndian, a.Reserved)
	return buffer.Bytes()
}

func (a *Account) FromBytes(data []byte) (Account, error) {
	if len(data) != AccountSize {
		return Account{}, fmt.Errorf("invalid data size: expected %d, got %d", AccountSize, len(data))
	}
	buffer := bytes.NewReader(data)
	account := Account{}
	binary.Read(buffer, binary.LittleEndian, &account.Nonce)
	binary.Read(buffer, binary.LittleEndian, &account.Free)
	binary.Read(buffer, binary.LittleEndian, &account.Reserved)
	return account, nil
}

type CreateAssetExtrinsic struct {
	method_id uint32
	asset     Asset
}

func (a *CreateAssetExtrinsic) Bytes() []byte {
	result := make([]byte, 4+AssetSize)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	buffer.Write(a.asset.Bytes())
	return buffer.Bytes()
}

type MintExtrinsic struct {
	method_id   uint32
	asset_id    uint64
	account_key [32]byte
	amount      uint64
}

func (a *MintExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.account_key[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

type BurnExtrinsic struct {
	method_id   uint32
	asset_id    uint64
	account_key [32]byte
	amount      uint64
}

func (a *BurnExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.account_key[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

type BondExtrinsic struct {
	method_id   uint32
	asset_id    uint64
	account_key [32]byte
	amount      uint64
}

func (a *BondExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.account_key[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

type UnbondExtrinsic struct {
	method_id   uint32
	asset_id    uint64
	account_key [32]byte
	amount      uint64
}

func (a *UnbondExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.account_key[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

type TransferExtrinsic struct {
	method_id uint32
	asset_id  uint64
	from      [32]byte
	to        [32]byte
	amount    uint64
}

func (a *TransferExtrinsic) Bytes() []byte {
	result := make([]byte, 4+8+32+32+8)
	buffer := bytes.NewBuffer(result[:0])
	binary.Write(buffer, binary.LittleEndian, a.method_id)
	binary.Write(buffer, binary.LittleEndian, a.asset_id)
	buffer.Write(a.from[:])
	buffer.Write(a.to[:])
	binary.Write(buffer, binary.LittleEndian, a.amount)
	return buffer.Bytes()
}

func balances(nodes []*Node, testServices map[string]*types.TestService) {
	fmt.Printf("\n=========================Balances Test=========================\n")

	// General setup
	BalanceService := testServices["balances"]
	balancesServiceIndex := BalanceService.ServiceCode
	balancesServiceCodeHash := BalanceService.CodeHash

	n1 := nodes[1]
	n4 := nodes[4]
	core := 0

	timeslot := nodes[1].statedb.Block.GetHeader().Slot
	refineContext := types.RefineContext{
		Anchor:           common.Hash{},
		StateRoot:        common.Hash{},
		BeefyRoot:        common.Hash{},
		LookupAnchor:     common.Hash{},
		LookupAnchorSlot: timeslot,
		Prerequisites:    []common.Hash{},
	}

	// Method ID bytes
	create_asset_id := uint32(0)
	mint_id := uint32(1)
	burn_id := uint32(2)
	bond_id := uint32(3)
	unbond_id := uint32(4)
	transfer_id := uint32(5)

	_ = create_asset_id
	_ = mint_id
	_ = burn_id
	_ = bond_id
	_ = unbond_id
	_ = transfer_id

	v0_bytes := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	v1_bytes := []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	v2_bytes := []byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
	v3_bytes := []byte{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3}
	v4_bytes := []byte{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4}
	v5_bytes := []byte{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5}
	_ = v0_bytes
	_ = v1_bytes
	_ = v2_bytes
	_ = v3_bytes
	_ = v4_bytes
	_ = v5_bytes

	asset := Asset{
		AssetID:     1984,
		Issuer:      [32]byte{},
		MinBalance:  100,
		Symbol:      [32]byte{},
		TotalSupply: 100,
		Decimals:    8,
	}
	copy(asset.Issuer[:], v1_bytes)
	copy(asset.Symbol[:], []byte("USDT"))

	// Create Asset test
	fmt.Printf("\n\033[38;5;208mCreating Asset (\033[38;5;46m1984 USDT\033[38;5;208m)...\033[0m\n")

	create_asset_workPackage := types.WorkPackage{}

	// Generate the extrinsic
	extrinsicsBytes := types.ExtrinsicsBlobs{}
	extrinsic := CreateAssetExtrinsic{
		method_id: create_asset_id,
		asset:     asset,
	}
	extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len := uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	create_asset_workPackage = types.WorkPackage{
		Authorization: []byte("0x"),
		AuthCodeHost:  balancesServiceIndex,
		Authorizer:    types.Authorizer{},
		RefineContext: refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     10000000,
				AccumulateGasLimit: 10000000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
		},
	}

	core0_peers := n1.GetCoreCoWorkersPeers(uint16(core))
	ramdamIdx := rand.Intn(3)
	err := core0_peers[ramdamIdx].SendWorkPackageSubmission(create_asset_workPackage, extrinsicsBytes, 0)
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	// wait until the work report is pending
	for {
		time.Sleep(1 * time.Second)
		if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
			if false {
				var workReport types.WorkReport
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
			}
			break
		}
	}
	// wait until the work report is cleared
	for {
		if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	time.Sleep(6 * time.Second)
	ShowAssetDetail(n1, balancesServiceIndex, 1984)

	// Mint test
	// mint 1234 for v1, v2, v3

	validators := [][]byte{v1_bytes, v2_bytes, v3_bytes}
	fmt.Printf("\n\033[38;5;208mMinting \033[38;5;46m1234 USDT\033[38;5;208m for \033[38;5;46mV1, V2, V3\033[38;5;208m...\033[0m\n")

	for _, v_key_bytes := range validators {
		var account_key [32]byte
		copy(account_key[:], v_key_bytes)

		// Generate the extrinsic
		extrinsicsBytes := types.ExtrinsicsBlobs{}
		extrinsic := MintExtrinsic{
			method_id:   mint_id,
			asset_id:    1984,
			account_key: account_key,
			amount:      1234,
		}
		extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
		extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

		extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
		extrinsic_len := uint32(len(extrinsicBytes_signed))

		// Put the extrinsic hash and length into the work item extrinsic
		work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
		work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
			Hash: extrinsics_hash,
			Len:  extrinsic_len,
		})

		payload_bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

		mint_workPackage := types.WorkPackage{
			Authorization: []byte("0x"),
			AuthCodeHost:  balancesServiceIndex,
			Authorizer:    types.Authorizer{},
			RefineContext: refineContext,
			WorkItems: []types.WorkItem{
				{
					Service:            balancesServiceIndex,
					CodeHash:           balancesServiceCodeHash,
					Payload:            payload_bytes,
					RefineGasLimit:     10000000,
					AccumulateGasLimit: 10000000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					Extrinsics:         work_item_extrinsic,
					ExportCount:        1,
				},
			},
		}

		core0_peers := n1.GetCoreCoWorkersPeers(uint16(core))
		ramdamIdx := rand.Intn(3)
		err = core0_peers[ramdamIdx].SendWorkPackageSubmission(mint_workPackage, extrinsicsBytes, 0)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// wait until the work report is pending
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
				if false {
					var workReport types.WorkReport
					rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
					workReport = rho_state.WorkReport
					fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				}
				break
			}
		}
		// wait until the work report is cleared
		for {
			if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}

		time.Sleep(6 * time.Second)
	}

	ShowAssetDetail(n1, balancesServiceIndex, 1984)
	for _, v_key_bytes := range validators {
		ShowAccountDetail(n1, balancesServiceIndex, 1984, [32]byte(v_key_bytes))
	}

	// Burn test
	// burn 100 for v3
	fmt.Printf("\n\033[38;5;208mBurning \033[38;5;46m234 USDT\033[38;5;208m from \033[38;5;46mV3\033[38;5;208m...\033[0m\n")

	var account_key [32]byte
	copy(account_key[:], v3_bytes)

	extrinsicsBytes = types.ExtrinsicsBlobs{}
	burnextrinsic := BurnExtrinsic{
		method_id:   burn_id,
		asset_id:    1984,
		account_key: account_key,
		amount:      234,
	}
	extrinsicBytes_signed = AddEd25519Sign(burnextrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash = common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len = uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic = make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes = make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	mint_workPackage := types.WorkPackage{
		Authorization: []byte("0x"),
		AuthCodeHost:  balancesServiceIndex,
		Authorizer:    types.Authorizer{},
		RefineContext: refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     10000000,
				AccumulateGasLimit: 10000000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
		},
	}

	core0_peers = n1.GetCoreCoWorkersPeers(uint16(core))
	ramdamIdx = rand.Intn(3)
	err = core0_peers[ramdamIdx].SendWorkPackageSubmission(mint_workPackage, extrinsicsBytes, 0)
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	// wait until the work report is pending
	for {
		time.Sleep(1 * time.Second)
		if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
			if false {
				var workReport types.WorkReport
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
			}
			break
		}
	}
	// wait until the work report is cleared
	for {
		if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	time.Sleep(6 * time.Second)
	ShowAssetDetail(n1, balancesServiceIndex, 1984)
	ShowAccountDetail(n1, balancesServiceIndex, 1984, account_key)

	// Bond test
	// bond 666 for v3
	fmt.Printf("\n\033[38;5;208mBonding \033[38;5;46m666 USDT\033[38;5;208m from \033[38;5;46mV3\033[38;5;208m...\033[0m\n")

	account_key = [32]byte{}
	copy(account_key[:], v3_bytes)

	extrinsicsBytes = types.ExtrinsicsBlobs{}
	bondextrinsic := BondExtrinsic{
		method_id:   bond_id,
		asset_id:    1984,
		account_key: account_key,
		amount:      666,
	}
	extrinsicBytes_signed = AddEd25519Sign(bondextrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash = common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len = uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic = make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes = make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	bond_workPackage := types.WorkPackage{
		Authorization: []byte("0x"),
		AuthCodeHost:  balancesServiceIndex,
		Authorizer:    types.Authorizer{},
		RefineContext: refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     10000000,
				AccumulateGasLimit: 10000000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
		},
	}

	core0_peers = n1.GetCoreCoWorkersPeers(uint16(core))
	ramdamIdx = rand.Intn(3)
	err = core0_peers[ramdamIdx].SendWorkPackageSubmission(bond_workPackage, extrinsicsBytes, 0)
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	// wait until the work report is pending
	for {
		time.Sleep(1 * time.Second)
		if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
			if false {
				var workReport types.WorkReport
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
			}
			break
		}
	}
	// wait until the work report is cleared
	for {
		if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	time.Sleep(6 * time.Second)
	ShowAccountDetail(n1, balancesServiceIndex, 1984, account_key)

	// Unbond test
	// unbond 333 for v3
	fmt.Printf("\n\033[38;5;208mUnbonding \033[38;5;46m333 USDT\033[38;5;208m from \033[38;5;46mV3\033[38;5;208m...\033[0m\n")

	account_key = [32]byte{}
	copy(account_key[:], v3_bytes)

	extrinsicsBytes = types.ExtrinsicsBlobs{}
	unbondextrinsic := UnbondExtrinsic{
		method_id:   unbond_id,
		asset_id:    1984,
		account_key: account_key,
		amount:      333,
	}
	extrinsicBytes_signed = AddEd25519Sign(unbondextrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash = common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len = uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic = make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes = make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	unbond_workPackage := types.WorkPackage{
		Authorization: []byte("0x"),
		AuthCodeHost:  balancesServiceIndex,
		Authorizer:    types.Authorizer{},
		RefineContext: refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     10000000,
				AccumulateGasLimit: 10000000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
		},
	}

	core0_peers = n1.GetCoreCoWorkersPeers(uint16(core))
	ramdamIdx = rand.Intn(3)
	err = core0_peers[ramdamIdx].SendWorkPackageSubmission(unbond_workPackage, extrinsicsBytes, 0)
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	// wait until the work report is pending
	for {
		time.Sleep(1 * time.Second)
		if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
			if false {
				var workReport types.WorkReport
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
			}
			break
		}
	}
	// wait until the work report is cleared
	for {
		if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	time.Sleep(6 * time.Second)
	ShowAccountDetail(n1, balancesServiceIndex, 1984, account_key)

	// Transfer test
	// transfer 100 from v3 to v2
	fmt.Printf("\n\033[38;5;208mTransferring \033[38;5;46m167 USDT\033[38;5;208m from \033[38;5;46mV3\033[38;5;208m to \033[38;5;46mV2\033[38;5;208m...\033[0m\n")

	from_account := [32]byte{}
	copy(from_account[:], v3_bytes)

	to_account := [32]byte{}
	copy(to_account[:], v2_bytes)

	extrinsicsBytes = types.ExtrinsicsBlobs{}
	transferextrinsic := TransferExtrinsic{
		method_id: transfer_id,
		asset_id:  1984,
		from:      from_account,
		to:        to_account,
		amount:    167,
	}

	extrinsicBytes_signed = AddEd25519Sign(transferextrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash = common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len = uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic = make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes = make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	transfer_workPackage := types.WorkPackage{
		Authorization: []byte("0x"),
		AuthCodeHost:  balancesServiceIndex,
		Authorizer:    types.Authorizer{},
		RefineContext: refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     10000000,
				AccumulateGasLimit: 10000000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
		},
	}

	core0_peers = n1.GetCoreCoWorkersPeers(uint16(core))
	ramdamIdx = rand.Intn(3)
	err = core0_peers[ramdamIdx].SendWorkPackageSubmission(transfer_workPackage, extrinsicsBytes, 0)
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	// wait until the work report is pending
	for {
		time.Sleep(1 * time.Second)
		if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
			if false {
				var workReport types.WorkReport
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
			}
			break
		}
	}
	// wait until the work report is cleared
	for {
		if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	time.Sleep(6 * time.Second)
	ShowAccountDetail(n1, balancesServiceIndex, 1984, from_account)
	ShowAccountDetail(n1, balancesServiceIndex, 1984, to_account)
}

func scaled_balances(nodes []*Node, testServices map[string]*types.TestService) {
	fmt.Printf("\n=========================Balances Scale Test=========================\n")
	// General setup
	BalanceService := testServices["balances"]
	balancesServiceIndex := BalanceService.ServiceCode
	balancesServiceCodeHash := BalanceService.CodeHash

	n1 := nodes[1]
	n4 := nodes[4]
	core := 0

	timeslot := nodes[1].statedb.Block.GetHeader().Slot
	refineContext := types.RefineContext{
		Anchor:           common.Hash{},
		StateRoot:        common.Hash{},
		BeefyRoot:        common.Hash{},
		LookupAnchor:     common.Hash{},
		LookupAnchorSlot: timeslot,
		Prerequisites:    []common.Hash{},
	}

	// Total Work Package Size in MB
	var totalWPSizeInMB float64

	// Method ID bytes
	create_asset_id := uint32(0)
	mint_id := uint32(1)
	transfer_id := uint32(5)

	_ = create_asset_id
	_ = mint_id
	_ = transfer_id

	issder_bytes := [32]byte{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5}

	asset := Asset{
		AssetID:     1984,
		Issuer:      issder_bytes,
		MinBalance:  100,
		Symbol:      [32]byte{},
		TotalSupply: 0,
		Decimals:    8,
	}
	copy(asset.Symbol[:], []byte("USDT"))

	// Create Asset test
	fmt.Printf("\n\033[38;5;208mCreating Asset (\033[38;5;46m1984 USDT\033[38;5;208m)...\033[0m\n")

	create_asset_workPackage := types.WorkPackage{}

	// Generate the extrinsic
	var extrinsicsBytes types.ExtrinsicsBlobs
	extrinsicsBytes = types.ExtrinsicsBlobs{}
	extrinsic := CreateAssetExtrinsic{
		method_id: create_asset_id,
		asset:     asset,
	}
	extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
	extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

	extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
	extrinsic_len := uint32(len(extrinsicBytes_signed))

	// Put the extrinsic hash and length into the work item extrinsic
	work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
	work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
		Hash: extrinsics_hash,
		Len:  extrinsic_len,
	})

	payload_bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(payload_bytes, 0) // extrinsic index

	create_asset_workPackage = types.WorkPackage{
		Authorization: []byte("0x"),
		AuthCodeHost:  balancesServiceIndex,
		Authorizer:    types.Authorizer{},
		RefineContext: refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     10000000,
				AccumulateGasLimit: 10000000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
		},
	}

	core0_peers := n1.GetCoreCoWorkersPeers(uint16(core))
	ramdamIdx := rand.Intn(3)

	totalWPSizeInMB = calaulateTotalWPSize(create_asset_workPackage, extrinsicsBytes)
	fmt.Printf("\nTotal Work Package Size: %v MB\n\n", totalWPSizeInMB)

	err := core0_peers[ramdamIdx].SendWorkPackageSubmission(create_asset_workPackage, extrinsicsBytes, 0)
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	// wait until the work report is pending
	for {
		time.Sleep(1 * time.Second)
		if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
			if false {
				var workReport types.WorkReport
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
			}
			break
		}
	}
	// wait until the work report is cleared
	for {
		if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	time.Sleep(6 * time.Second)
	ShowAssetDetail(n1, balancesServiceIndex, 1984)

	// Scaled mint test
	num_of_mint := 110
	fmt.Printf("\n\033[38;5;208mMinting \033[38;5;46m90000 USDT\033[38;5;208m for \033[38;5;46m%v\033[38;5;208m accounts...\033[0m\n", num_of_mint)

	var mint_workPackage types.WorkPackage
	var mint_workItems []types.WorkItem
	extrinsicsBytes = types.ExtrinsicsBlobs{}
	for i := 0; i < num_of_mint; i++ {
		account_key := generateVBytes(uint64(i))

		// Generate the extrinsic
		extrinsic := MintExtrinsic{
			method_id:   mint_id,
			asset_id:    1984,
			account_key: account_key,
			amount:      90000,
		}
		extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
		extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

		extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
		extrinsic_len := uint32(len(extrinsicBytes_signed))

		// Put the extrinsic hash and length into the work item extrinsic
		work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
		work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
			Hash: extrinsics_hash,
			Len:  extrinsic_len,
		})

		payload_bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(payload_bytes, uint64(i)) // extrinsic index

		workItem := types.WorkItem{
			Service:            balancesServiceIndex,
			CodeHash:           balancesServiceCodeHash,
			Payload:            payload_bytes,
			RefineGasLimit:     10000000,
			AccumulateGasLimit: 10000000,
			ImportedSegments:   make([]types.ImportSegment, 0),
			Extrinsics:         work_item_extrinsic,
			ExportCount:        1,
		}

		mint_workItems = append(mint_workItems, workItem)
	}

	mint_workPackage = types.WorkPackage{
		Authorization: []byte("0x"),
		AuthCodeHost:  balancesServiceIndex,
		Authorizer:    types.Authorizer{},
		RefineContext: refineContext,
		WorkItems:     mint_workItems,
	}

	totalWPSizeInMB = calaulateTotalWPSize(mint_workPackage, extrinsicsBytes)
	fmt.Printf("\nTotal Work Package Size: %v MB\n\n", totalWPSizeInMB)

	core0_peers = n1.GetCoreCoWorkersPeers(uint16(core))
	ramdamIdx = rand.Intn(3)
	err = core0_peers[ramdamIdx].SendWorkPackageSubmission(mint_workPackage, extrinsicsBytes, 0)
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	// wait until the work report is pending
	for {
		time.Sleep(1 * time.Second)
		if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
			if false {
				var workReport types.WorkReport
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
			}
			break
		}
	}
	// wait until the work report is cleared
	for {
		if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	time.Sleep(6 * time.Second)
	ShowAssetDetail(n1, balancesServiceIndex, 1984)

	// Scaled Transfer test
	num_of_transfer := 110
	fmt.Printf("\n\033[38;5;208mTransferring \033[38;5;46m1 USDT\033[38;5;208m from \033[38;5;46mV6\033[38;5;208m to \033[38;5;46m%v\033[38;5;208m accounts...\033[0m\n", num_of_transfer)

	var transfer_workPackage types.WorkPackage
	var transfer_workItems []types.WorkItem
	extrinsicsBytes = types.ExtrinsicsBlobs{}

	sender_id := uint64(6)
	sender := generateVBytes(sender_id)
	var receiver [32]byte
	for i := 0; i < num_of_transfer; i++ {
		receiver = generateVBytes(uint64(i))
		if uint64(i) == sender_id {
			receiver = generateVBytes(0)
		}

		// Generate the extrinsic
		extrinsic := TransferExtrinsic{
			method_id: transfer_id,
			asset_id:  1984,
			from:      sender,
			to:        receiver,
			amount:    1,
		}
		extrinsicBytes_signed := AddEd25519Sign(extrinsic.Bytes())
		extrinsicsBytes = append(extrinsicsBytes, extrinsicBytes_signed)

		extrinsics_hash := common.Blake2Hash(extrinsicBytes_signed)
		extrinsic_len := uint32(len(extrinsicBytes_signed))

		// Put the extrinsic hash and length into the work item extrinsic
		work_item_extrinsic := make([]types.WorkItemExtrinsic, 0)
		work_item_extrinsic = append(work_item_extrinsic, types.WorkItemExtrinsic{
			Hash: extrinsics_hash,
			Len:  extrinsic_len,
		})

		payload_bytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(payload_bytes, uint64(i)) // extrinsic index

		workItem := types.WorkItem{
			Service:            balancesServiceIndex,
			CodeHash:           balancesServiceCodeHash,
			Payload:            payload_bytes,
			RefineGasLimit:     10000000,
			AccumulateGasLimit: 10000000,
			ImportedSegments:   make([]types.ImportSegment, 0),
			Extrinsics:         work_item_extrinsic,
			ExportCount:        1,
		}

		transfer_workItems = append(transfer_workItems, workItem)
	}

	transfer_workPackage = types.WorkPackage{
		Authorization: []byte("0x"),
		AuthCodeHost:  balancesServiceIndex,
		Authorizer:    types.Authorizer{},
		RefineContext: refineContext,
		WorkItems:     transfer_workItems,
	}

	totalWPSizeInMB = calaulateTotalWPSize(transfer_workPackage, extrinsicsBytes)
	fmt.Printf("\nTotal Work Package Size: %v MB\n\n", totalWPSizeInMB)

	core0_peers = n1.GetCoreCoWorkersPeers(uint16(core))
	ramdamIdx = rand.Intn(3)
	err = core0_peers[ramdamIdx].SendWorkPackageSubmission(transfer_workPackage, extrinsicsBytes, 0)
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
	}
	// wait until the work report is pending
	for {
		time.Sleep(1 * time.Second)
		if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
			if false {
				var workReport types.WorkReport
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
			}
			break
		}
	}
	// wait until the work report is cleared
	for {
		if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	time.Sleep(6 * time.Second)
	ShowAccountDetail(n1, balancesServiceIndex, 1984, sender)

}

func AddEd25519Sign(data []byte) []byte {
	// Generate a new key pair
	seed := make([]byte, 32)
	rand.Read(seed)
	pubKey, privKey, err := types.InitEd25519Key(seed)
	if err != nil {
		return nil
	}

	signature := types.Ed25519Sign(types.Ed25519Priv(privKey), data)
	combinedBytes := append(pubKey.Bytes(), data...)
	data = append(combinedBytes, signature.Bytes()...)
	return data
}

func generateVBytes(n uint64) [32]byte {
	var arr [32]byte
	binary.LittleEndian.PutUint64(arr[:8], n)
	return arr
}

func calaulateTotalWPSize(create_asset_workPackage types.WorkPackage, extrinsicsBytes types.ExtrinsicsBlobs) float64 {
	WPsizeInBytes := len(create_asset_workPackage.Bytes())
	WPsizeInMB := float64(WPsizeInBytes) / 1024 / 1024

	ETsizeInBytes := len(extrinsicsBytes.Bytes())
	ETsizeInMB := float64(ETsizeInBytes) / 1024 / 1024

	totalWPsizeInMB := WPsizeInMB + ETsizeInMB
	return totalWPsizeInMB
}

func ShowAssetDetail(n *Node, balance_service_index uint32, asset_id uint64) {
	var asset Asset
	key_byte := make([]byte, 8)
	binary.LittleEndian.PutUint64(key_byte, asset_id)
	service_account_byte, _, _ := n.getState().GetTrie().GetServiceStorage(balance_service_index, key_byte)

	fetched_asset, _ := asset.FromBytes(service_account_byte)
	fmt.Printf("\n\033[38;5;13mAsset ID\033[0m: \033[32m%v\033[0m\n", fetched_asset.AssetID)
	fmt.Printf("\033[38;5;13mAsset Issuer\033[0m: \033[32m%x\033[0m\n", fetched_asset.Issuer)
	fmt.Printf("\033[38;5;13mAsset MinBalance\033[0m: \033[32m%v\033[0m\n", fetched_asset.MinBalance)
	fmt.Printf("\033[38;5;13mAsset Symbol\033[0m: \033[32m%s\033[0m\n", fetched_asset.Symbol)
	fmt.Printf("\033[38;5;13mAsset TotalSupply\033[0m: \033[32m%v\033[0m\n", fetched_asset.TotalSupply)
	fmt.Printf("\033[38;5;13mAsset Decimals\033[0m: \033[32m%v\033[0m\n", fetched_asset.Decimals)
}

func ShowAccountDetail(n *Node, balance_service_index uint32, asset_id uint64, account_key [32]byte) {
	var account Account
	key_byte := make([]byte, 8+32)
	binary.LittleEndian.PutUint64(key_byte, asset_id)
	copy(key_byte[8:], account_key[:])
	service_account_byte, _, _ := n.getState().GetTrie().GetServiceStorage(balance_service_index, key_byte)

	fetched_account, _ := account.FromBytes(service_account_byte)
	fmt.Printf("\n\033[38;5;13mAccount Key\033[0m: \033[32m%x\033[0m\n", account_key)
	fmt.Printf("\033[38;5;13mAsset ID\033[0m: \033[32m%v\033[0m\n", asset_id)
	fmt.Printf("\033[38;5;13mAccount Nonce\033[0m: \033[32m%v\033[0m\n", fetched_account.Nonce)
	fmt.Printf("\033[38;5;13mAccount Free\033[0m: \033[32m%v\033[0m\n", fetched_account.Free)
	fmt.Printf("\033[38;5;13mAccount Reserved\033[0m: \033[32m%v\033[0m\n", fetched_account.Reserved)
}
