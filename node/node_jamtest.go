package node

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"

	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
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
		log.Crit("stf", "Failed to create HTTP request", err)
	}

	req.Header.Set("Content-Type", "application/json")
	//req.Body = os.NopCloser(http.NoBody)

	resp, err := client.Do(req)
	if err != nil {
		log.Error("stf", "Failed to send block via HTTP", err)
		return
	}
	defer resp.Body.Close()
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
	jobID := generateJobID()
	currTS := currJCE * 6
	for i := 0; i < numNodes; i++ {
		node_idx := fmt.Sprintf("%d", i)
		node_path, err := computeLevelDBPath(node_idx, int(currTS), jobID)
		if err == nil {
			node_paths[i] = node_path
		}
	}
	return node_paths
}

func computeLevelDBPath(id string, unixtimestamp int, jobID string) (string, error) {
	/* standardize on
	/tmp/<user>/jam/<unixtimestamp>_<jobID>/testdb#

	/tmp/ntust/jam/1727903082_<jID>/node1/leveldb/
	/tmp/ntust/jam/1727903082_<jID>/node1/data/

	/tmp/root/jam/1727903082_<jID>/node1/

	*/
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("could not get current user: %v", err)
	}
	username := currentUser.Username
	path := fmt.Sprintf("/tmp/%s/jam/%v_%v/node%v", username, unixtimestamp, jobID, id)
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

	return epoch0Timestamp, peers, peerList, validatorSecrets, nodePaths, nil
}

func GenerateRandomBasePort() uint16 {

	// New seed every time
	seed := uint64(time.Now().UnixNano())
	r := rand.New(rand.NewSource(seed))

	// Generate base port in the 1xx00 - 6xx range to support multi-running
	region := r.Intn(4) + 1
	mid := r.Intn(100) + 1
	basePort := uint16(region*10000 + mid*100)
	return basePort
}

func SetUpNodes(numNodes int, basePort uint16) ([]*Node, error) {
	network := types.Network
	GenesisStateFile, GenesisBlockFile := getGenesisFile(network)
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelDebug, true)))

	epoch0Timestamp, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork(network, basePort)

	if err != nil {
		return nil, err
	}

	nodes := make([]*Node, numNodes)
	godIncomingCh := make(chan uint32, 10) // node sends timeslots to this channel when authoring
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], GenesisStateFile, GenesisBlockFile, epoch0Timestamp, peers, peerList, ValidatorFlag, nodePaths[i], int(basePort)+i)
		if err != nil {
			panic(err)
			return nil, fmt.Errorf("Failed to create node %d: %v", i, err)
		}
		if godMode {
			node.setGodCh(&godIncomingCh)
		}
		nodes[i] = node
	}
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				break
			case ts := <-godIncomingCh: // Correct channel receive operation
				for _, n := range nodes { // Loop through nodes
					n.receiveGodTimeslotUsed(ts)
				}
			}
		}

	}()
	return nodes, nil
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
		log.Info(module, serviceName, "codeHash", codeHash, "len", len(code))
	}
	return
}

func jamtest(t *testing.T, jam string, targetedEpochLen int, basePort uint16, targetN int) {
	flag.Parse()
	nodes, err := SetUpNodes(numNodes, basePort)
	if err != nil {
		panic("Error setting up nodes: %v\n")
	}
	log.Info(module, "JAMTEST", "jam", jam, "targetN", targetN)
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
	time.Sleep(types.SecondsPerSlot * time.Second) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot
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
	serviceNames := []string{"auth_copy", "fib"}
	if jam == "megatron" {
		serviceNames = []string{"fib", "tribonacci", "megatron", "auth_copy"} // Others include: "padovan", "pell", "racaman"
	}
	if jam == "transfer" || jam == "scaled_transfer" {
		serviceNames = []string{"transfer_0", "transfer_1", "auth_copy"} // 2 transfer services share the same code
	}
	if jam == "balances" || jam == "scaled_balances" {
		serviceNames = []string{"balances", "auth_copy"}
	}
	if jam == "empty" {
		serviceNames = []string{"delay", "auth_copy"}
	}
	if jam == "blake2b" {
		serviceNames = []string{"blake2b"}
	}
	if jam == "fib2" {
		serviceNames = []string{"fib2", "auth_copy"}
	}
	testServices, err := getServices(serviceNames)
	if err != nil {
		panic(32)
	}

	// set builderNode's primages map
	for _, service := range testServices {
		builderNode.preimages[service.CodeHash] = service.Code
	}
	log.Trace(module, "Waiting for the first block to be ready...")
	time.Sleep(2 * types.SecondsPerSlot * time.Second) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot
	var previous_service_idx uint32
	for serviceName, service := range testServices {
		log.Info(module, "Builder storing TestService", "serviceName", serviceName, "codeHash", service.CodeHash)
		// set up service using the Bootstrap service
		refine_context := builderNode.statedb.GetRefineContext()
		codeWorkPackage := types.WorkPackage{
			Authorization:         []byte(""),
			AuthCodeHost:          bootstrapService,
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			RefineContext:         refine_context,
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
		// use get coworkers peers
		core0_peers := builderNode.GetCoreCoWorkersPeers(0)
		// ramdom pick the index from 0, 1, 2
		randomIdx := rand.Intn(3)
		err = core0_peers[randomIdx].SendWorkPackageSubmission(codeWorkPackage, types.ExtrinsicsBlobs{}, 0)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}

		new_service_found := false
		log.Trace(module, "Waiting for service to be ready", "service", serviceName)
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
						log.Error(debugP, "BroadcastPreimageAnnouncement", "err", err)
					}
				}

			}
		}
	}

	log.Debug(module, "All services are ready, Sending preimage announcement\n")

	for done := false; !done; {

		ready := 0
		nservices := 0
		for _, service := range testServices {
			for _, n := range nodes {
				targetStateDB := n.getState()
				if targetStateDB != nil {
					code, ok, err := targetStateDB.ReadServicePreimageBlob(service.ServiceCode, service.CodeHash)
					if err != nil || !ok {
						log.Debug(debugDA, "ReadServicePreimageBlob", "err", err, "ok", ok)
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

	switch jam {
	case "megatron":
		megatron(nodes, testServices, targetN)
	case "fib":
		fib(nodes, testServices, targetN)
	case "fib2":
		targetN := 9
		fib2(nodes, testServices, targetN)
	case "transfer":
		transferNum := targetN
		transfer(nodes, testServices, transferNum)
	case "scaled_transfer":
		transferNum := 10
		splitTransferNum := targetN
		scaled_transfer(nodes, testServices, transferNum, splitTransferNum)
	case "balances":
		// not using anything
		balances(nodes, testServices, targetN)
	case "scaled_balances":
		targetN_mint := targetN
		targetN_transfer := targetN
		scaled_balances(nodes, testServices, targetN_mint, targetN_transfer)
	case "empty":
		empty(nodes, testServices)
	case "blake2b":
		blake2b(nodes, testServices, targetN)
	}
}

func fib(nodes []*Node, testServices map[string]*types.TestService, targetN int) {
	log.Info(module, "FIB START", "targetN", targetN)

	service0 := testServices["fib"]
	service_authcopy := testServices["auth_copy"]
	n1 := nodes[1]
	n4 := nodes[4]
	core := 0
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
		refine_context := n1.statedb.GetRefineContext()

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			RefineContext:         refine_context,
			WorkItems: []types.WorkItem{
				{
					Service:            service0.ServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   importedSegments,
					ExportCount:        1,
				},
				{
					Service:            service_authcopy.ServiceCode,
					CodeHash:           service_authcopy.CodeHash,
					Payload:            []byte{},
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}
		workPackageHash := workPackage.Hash()

		log.Info(module, fmt.Sprintf("FIB(%v) work package submitted", fibN), "workPackage", workPackageHash)
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
		k := common.ServiceStorageKey(service0.ServiceCode, []byte{0})
		service_account_byte, _, _ := n4.getState().GetTrie().GetServiceStorage(service0.ServiceCode, k)
		log.Info(module, fmt.Sprintf("Fib(%v) result", fibN), "result", fmt.Sprintf("%x", service_account_byte))
	}

}

func megatron(nodes []*Node, testServices map[string]*types.TestService, targetMegatronN int) {

	fmt.Printf("Start Fib_Trib\n")
	service0 := testServices["fib"]
	service1 := testServices["tribonacci"]
	serviceM := testServices["megatron"]
	service_authcopy := testServices["auth_copy"]
	fmt.Printf("service0: %v, codehash: %v (len=%v) | %v\n", service0.ServiceCode, service0.CodeHash, len(service0.Code), service0.ServiceName)
	fmt.Printf("service1: %v, codehash: %v (len=%v) | %v\n", service1.ServiceCode, service1.CodeHash, len(service1.Code), service1.ServiceName)
	fmt.Printf("serviceM: %v, codehash: %v (len=%v) | %v\n", serviceM.ServiceCode, serviceM.CodeHash, len(serviceM.Code), serviceM.ServiceName)
	targetNMax := targetMegatronN
	time.Sleep(1 * time.Second)
	// =================================================
	// set up ticker for loop
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	ticker_runtime := time.NewTicker(2 * time.Second)
	defer ticker_runtime.Stop()
	Fib_Tri_Chan := make(chan *types.WorkPackage, 1)
	Fib_Tri_counter := 0
	Fib_Tri_successful := make(chan string)
	Fib_Tri_Ready := true
	Fib_Tri_Keeper := false
	Meg_Chan := make(chan *types.WorkPackage, 1)
	Meg_counter := 0
	Meg_successful := make(chan string)
	Meg_Ready := true
	Meg_Keeper := false
	// =================================================
	// set up the workpackages
	fib_importedSegments := make([]types.ImportSegment, 0)
	trib_importedSegments := make([]types.ImportSegment, 0)
	fib_trib_items := buildFibTribItem(fib_importedSegments, trib_importedSegments, Fib_Tri_counter, service0.ServiceCode, service0.CodeHash, service1.ServiceCode, service1.CodeHash)
	next_fib_tri_WorkPackage, err := nodes[0].MakeWorkPackage([]common.Hash{}, service0.ServiceCode, fib_trib_items)
	if err != nil {
		fmt.Printf("MakeWorkPackage ERR %v\n", err)
	}
	for _, n := range nodes {
		n.delaysend[next_fib_tri_WorkPackage.Hash()] = 1
	}
	var curr_fib_tri_prereqs []common.Hash
	meg_no_import_segment := make([]types.ImportSegment, 0)
	meg_items := buildMegItem(meg_no_import_segment, Meg_counter, serviceM.ServiceCode, service0.ServiceCode, service1.ServiceCode, serviceM.CodeHash)
	meg_preq := []common.Hash{next_fib_tri_WorkPackage.Hash()}
	next_Meg_WorkPackage, err := nodes[0].MakeWorkPackage(meg_preq, serviceM.ServiceCode, meg_items)
	var last_Meg []common.Hash

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
	var curr_fib_tri_workpackage types.WorkPackage
	var curr_meg_workpackage types.WorkPackage
	// =================================================
	for {
		if ok {
			break
		}
		select {
		case workPackage := <-Fib_Tri_Chan:
			// submit to core 1
			// v0, v3, v5 => core
			senderIdx := 5
			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
			go func() {
				defer cancel()
				sendWorkPackageTrack(ctx, nodes[senderIdx], workPackage, uint16(1), Fib_Tri_successful, types.ExtrinsicsBlobs{})
			}()
		case successful := <-Fib_Tri_successful:
			if successful == "ok" {
				k0 := common.ServiceStorageKey(service0.ServiceCode, []byte{0})
				k1 := common.ServiceStorageKey(service1.ServiceCode, []byte{0})
				service_account_byte, _, _ := nodes[1].getState().GetTrie().GetServiceStorage(service0.ServiceCode, k0)
				// get the first four byte of the result
				if len(service_account_byte) < 4 {
					fmt.Printf("Fib %d = %v\n", 0, service_account_byte)
				} else {
					index_bytes := service_account_byte[:4]
					index := binary.LittleEndian.Uint32(index_bytes)

					fmt.Printf("Fib %d = %v\n", index, service_account_byte)
					service_account_byte, _, _ = nodes[1].getState().GetTrie().GetServiceStorage(service1.ServiceCode, k1)
					fmt.Printf("Tri %d = %v\n", index, service_account_byte)
				}
			} else if successful == "trial" {
				fmt.Printf("Fib_Tri %d sending again. wp hash=%v\n", Fib_Tri_counter-1, curr_fib_tri_workpackage.Hash())
				currfib_trib_hash := curr_fib_tri_workpackage.Hash()
				curr_meg_workpackage.RefineContext.Prerequisites = []common.Hash{currfib_trib_hash}
				if Fib_Tri_counter <= targetNMax-1 {
					fib_importedSegments := []types.ImportSegment{
						{
							RequestedHash: curr_fib_tri_workpackage.Hash(),
							Index:         0,
						},
					}
					trib_importedSegments := []types.ImportSegment{
						{
							RequestedHash: curr_fib_tri_workpackage.Hash(),
							Index:         1,
						},
					}
					fib_trib_items = buildFibTribItem(fib_importedSegments, trib_importedSegments, Fib_Tri_counter, service0.ServiceCode, service0.CodeHash, service1.ServiceCode, service1.CodeHash)
					next_fib_tri_WorkPackage, err = nodes[2].MakeWorkPackage([]common.Hash{}, service0.ServiceCode, fib_trib_items)
					if err != nil {
						panic(err)
					}
				}
			}
		case successful := <-Meg_successful:
			if successful == "ok" {
				km := common.ServiceStorageKey(serviceM.ServiceCode, []byte{0})
				service_account_byte, _, _ := nodes[1].getState().GetTrie().GetServiceStorage(serviceM.ServiceCode, km)

				// get the first four byte of the result
				if len(service_account_byte) < 4 {
					fmt.Printf("Meg %d = %v\n", 0, service_account_byte)
				} else {
					index_bytes := service_account_byte[:4]
					index := binary.LittleEndian.Uint32(index_bytes)
					fmt.Printf("Meg %d = %v\n", index, service_account_byte)
				}
			} else if successful == "trial" {
				fmt.Printf("Meg %d sending again. wp hash=%v\n", Meg_counter-1, curr_meg_workpackage.Hash())
				currfib_trib_hash := curr_fib_tri_workpackage.Hash()
				curr_meg_workpackage.RefineContext.Prerequisites = []common.Hash{currfib_trib_hash}
			}

		case workPackage := <-Meg_Chan:
			// submit to core 0
			// CE133_WorkPackageSubmission: n1 => n4
			// v1, v2, v4 => core
			// random select 1 sender and 1 receiver
			// Randomly select sender and receiver
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

		case <-ticker.C:
			if sentLastWorkPackage {
				if FinalRho && FinalAssurance && FinalMeg {
					ok = true
					log.Info(module, "megatron success")
					break
				} else if test_prereq && nodes[0].statedb.JamState.AvailabilityAssignments[0] != nil && nodes[0].statedb.JamState.AvailabilityAssignments[1] != nil {
					FinalRho = true
				} else if test_prereq && FinalRho {
					if FinalAssurance {
						if nodes[0].statedb.JamState.AvailabilityAssignments[1] == nil {
							FinalMeg = true
						}

					} else if nodes[0].statedb.JamState.AvailabilityAssignments[0] == nil {
						FinalAssurance = true
					}

				} else if !test_prereq {
					if nodes[0].statedb.JamState.AvailabilityAssignments[1] != nil {
						FinalRho = true
					}
					if nodes[0].statedb.JamState.AvailabilityAssignments[0] != nil {
						FinalMeg = true
					}
					if FinalRho && FinalMeg {
						if nodes[0].statedb.JamState.AvailabilityAssignments[0] == nil && nodes[0].statedb.JamState.AvailabilityAssignments[1] == nil {
							FinalAssurance = true
						}
					}
					if FinalAssurance {
						ok = true
						log.Info(module, "megatron success")
						break
					}
				}
			} else if test_prereq {
				if Fib_Tri_counter == targetNMax && Meg_counter == targetNMax {
					fmt.Printf("All workpackages are sent\n")
					sentLastWorkPackage = true
				} else if (nodes[0].IsCoreReady(0, last_Meg) && Meg_Ready) && (nodes[0].IsCoreReady(1, curr_fib_tri_prereqs) && Fib_Tri_Ready) {
					// if false{

					// send workpackages to the network
					curr_fib_tri_workpackage = next_fib_tri_WorkPackage
					curr_meg_workpackage = next_Meg_WorkPackage
					Fib_Tri_Chan <- &curr_fib_tri_workpackage
					Fib_Tri_counter++
					// send workpackages to the network
					Meg_Chan <- &curr_meg_workpackage
					Meg_counter++
					if Meg_counter <= targetNMax-1 {
						// build the next workpackage
						// fib
						fib_importedSegments := []types.ImportSegment{
							{
								RequestedHash: curr_fib_tri_workpackage.Hash(),
								Index:         0,
							},
						}
						trib_importedSegments := []types.ImportSegment{
							{
								RequestedHash: curr_fib_tri_workpackage.Hash(),
								Index:         1,
							},
						}
						fib_trib_items = buildFibTribItem(fib_importedSegments, trib_importedSegments, Fib_Tri_counter, service0.ServiceCode, service0.CodeHash, service1.ServiceCode, service1.CodeHash)
						next_fib_tri_WorkPackage, err = nodes[2].MakeWorkPackage([]common.Hash{}, service0.ServiceCode, fib_trib_items)
						if err != nil {
							panic(err)
						}
						curr_fib_tri_prereqs = []common.Hash{}
						for _, n := range nodes {
							n.delaysend[curr_fib_tri_workpackage.Hash()] = 1
						}

						Fib_Tri_Ready = false
						// meg
						meg_items = buildMegItem(meg_no_import_segment, Meg_counter, serviceM.ServiceCode, service0.ServiceCode, service1.ServiceCode, serviceM.CodeHash)
						meg_items = append(meg_items, types.WorkItem{
							Service:            service_authcopy.ServiceCode,
							CodeHash:           service_authcopy.CodeHash,
							Payload:            []byte{},
							RefineGasLimit:     1000,
							AccumulateGasLimit: 1000,
							ImportedSegments:   make([]types.ImportSegment, 0),
							ExportCount:        2,
						})
						next_Meg_WorkPackage, err = nodes[2].MakeWorkPackage([]common.Hash{next_fib_tri_WorkPackage.Hash()}, serviceM.ServiceCode, meg_items)
						last_Meg = []common.Hash{}
						Meg_Ready = false
					}
					fmt.Printf("**  %v  Preparing Fib_Tri#%v %v Meg#%v %v **\n", time.Now().Format("04:05.000"), Fib_Tri_counter, next_fib_tri_WorkPackage.Hash().String_short(), Meg_counter, next_Meg_WorkPackage.Hash().String_short())
				} else if (nodes[0].statedb.JamState.AvailabilityAssignments[0] != nil) || (nodes[0].statedb.JamState.AvailabilityAssignments[1] != nil && !Fib_Tri_Ready) {

					Meg_Ready = false
					Meg_Keeper = true
					Fib_Tri_Ready = false
					Fib_Tri_Keeper = true

				} else if (Meg_Keeper && nodes[0].IsCoreReady(0, last_Meg)) && (Fib_Tri_Keeper && nodes[0].IsCoreReady(1, curr_fib_tri_prereqs, false, curr_fib_tri_workpackage.Hash())) {
					Meg_Ready = true
					Meg_Keeper = false
					Fib_Tri_Ready = true
					Fib_Tri_Keeper = false
				}
			} else if !test_prereq {
				if Fib_Tri_counter == targetNMax && Meg_counter == targetNMax {
					fmt.Printf("All workpackages are sent\n")
					sentLastWorkPackage = true
				} else if (nodes[0].IsCoreReady(0, last_Meg) && Meg_Ready) && (nodes[0].IsCoreReady(1, curr_fib_tri_prereqs) && Fib_Tri_Ready) {
					// send workpackages to the network
					fmt.Printf("**  %v  Preparing Fib_Tri#%v %v Meg#%v %v **\n", time.Now().Format("04:05.000"), Fib_Tri_counter, next_fib_tri_WorkPackage.Hash().String_short(), Meg_counter, next_Meg_WorkPackage.Hash().String_short())
					fmt.Printf("\n** \033[32m Fib_Tri %d \033[0m workPackage: %v **\n", Fib_Tri_counter, common.Str(next_fib_tri_WorkPackage.Hash()))
					// in case it gets stuck
					// refine context need to be updated
					next_fib_tri_WorkPackage.RefineContext = nodes[2].statedb.GetRefineContext()
					next_Meg_WorkPackage.RefineContext = nodes[2].statedb.GetRefineContext()
					next_Meg_WorkPackage.RefineContext.Prerequisites = []common.Hash{next_fib_tri_WorkPackage.Hash()}
					// send workpackages to the network
					curr_fib_tri_workpackage = next_fib_tri_WorkPackage
					curr_meg_workpackage = next_Meg_WorkPackage
					Fib_Tri_Chan <- &curr_fib_tri_workpackage
					Fib_Tri_counter++
					if Fib_Tri_counter <= targetNMax-1 {
						fib_importedSegments := []types.ImportSegment{
							{
								RequestedHash: curr_fib_tri_workpackage.Hash(),
								Index:         0,
							},
						}
						trib_importedSegments := []types.ImportSegment{
							{
								RequestedHash: curr_fib_tri_workpackage.Hash(),
								Index:         1,
							},
						}
						fib_trib_items = buildFibTribItem(fib_importedSegments, trib_importedSegments, Fib_Tri_counter, service0.ServiceCode, service0.CodeHash, service1.ServiceCode, service1.CodeHash)
						next_fib_tri_WorkPackage, err = nodes[2].MakeWorkPackage([]common.Hash{}, service0.ServiceCode, fib_trib_items)
						if err != nil {
							panic(err)
						}
						curr_fib_tri_prereqs = []common.Hash{}
						// for _, item := range next_fib_tri_WorkPackage.WorkItems {
						// 	for _, seg := range item.ImportedSegments {
						// 		curr_fib_tri_prereqs = append(curr_fib_tri_prereqs, seg.RequestedHash)
						// 	}
						// }
						Fib_Tri_Ready = false
					}
					// send workpackages to the network
					fmt.Printf("\n** \033[36m MEGATRON %d \033[0m workPackage: %v **\n", Meg_counter, common.Str(next_Meg_WorkPackage.Hash()))
					Meg_Chan <- &curr_meg_workpackage
					Meg_counter++
					if Meg_counter <= targetNMax-1 {
						// previous_workpackage_hash := curr_meg_workpackage.Hash()
						meg_items = buildMegItem(meg_no_import_segment, Meg_counter, serviceM.ServiceCode, service0.ServiceCode, service1.ServiceCode, serviceM.CodeHash)
						meg_items = append(meg_items, types.WorkItem{

							Service:            service_authcopy.ServiceCode,
							CodeHash:           service_authcopy.CodeHash,
							Payload:            []byte{},
							RefineGasLimit:     1000,
							AccumulateGasLimit: 1000,
							ImportedSegments:   make([]types.ImportSegment, 0),
							ExportCount:        2,
						})
						next_Meg_WorkPackage, err = nodes[2].MakeWorkPackage([]common.Hash{next_fib_tri_WorkPackage.Hash()}, serviceM.ServiceCode, meg_items)
						last_Meg = []common.Hash{}
						// last_Meg = append(last_Meg, previous_workpackage_hash)
						Meg_Ready = false
					}
				} else {
					is_0_ready := nodes[0].IsCoreReady(0, last_Meg)
					is_1_ready := nodes[0].IsCoreReady(1, curr_fib_tri_prereqs, true, curr_fib_tri_workpackage.Hash())
					if is_0_ready && is_1_ready {
						Meg_Ready = true
						Fib_Tri_Ready = true
					}
				}
			}
		}
		// case <-ticker_runtime.C:
		// 	var stats runtime.MemStats
		// 	runtime.ReadMemStats(&stats)

		// 	numGoroutine := runtime.NumGoroutine()
		// 	numCPU := runtime.NumCPU()
		// 	cpuPercent := float64(runtime.NumCgoCall()) / float64(numCPU)

		// 	fmt.Printf("\033[31mGoroutines: %d, CPUs: %d, CPU Usage (approx): %.2f%%\033[0m\n", numGoroutine, numCPU, cpuPercent*100)

	}
}

func sendWorkPackageTrack(ctx context.Context, senderNode *Node, workPackage *types.WorkPackage, receiverCore uint16, msg chan string, extrinsics types.ExtrinsicsBlobs) {

	workPackageHash := workPackage.Hash()
	trialCount := 0
	MaxTrialCount := 100
	// send it right away for one time
	corePeers := senderNode.GetCoreCoWorkersPeers(receiverCore)
	randIdx := rand.Intn(len(corePeers))
	err := corePeers[randIdx].SendWorkPackageSubmission(*workPackage, extrinsics, receiverCore)

	log.Trace(debugG, "SendWorkPackageSubmission to core for trial/timeslot", "n", senderNode.id, "p", corePeers[randIdx].PeerID, "core", receiverCore, "wph", workPackageHash,
		"trialCount", trialCount, "timeslot", senderNode.statedb.GetSafrole().GetTimeSlot())
	if err != nil {
		fmt.Printf("SendWorkPackageSubmission ERR %v, sender: %d, receiver %d\n", err, senderNode.id, corePeers[randIdx].PeerID)
	}
	ticker := time.NewTicker(2 * types.SecondsPerSlot * time.Second)
	ticker2 := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	defer ticker2.Stop()
	time.Sleep(6 * time.Second)
	trialCount++
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Context cancelled for work package %v\n", workPackageHash)
			return
		case <-ticker2.C:
			if senderNode.statedb.JamState.AvailabilityAssignments[receiverCore] != nil {
				rho := senderNode.statedb.JamState.AvailabilityAssignments[receiverCore]
				pendingWPHash := rho.WorkReport.AvailabilitySpec.WorkPackageHash
				if workPackageHash == pendingWPHash {
					fmt.Printf("Found pending work package %v at core %v!\n", workPackageHash, receiverCore)
					msg <- "ok"
					return
				}
				//or it's in the history or the accumulate history
				is_core_ready := senderNode.IsCoreReady(receiverCore, []common.Hash{workPackageHash})
				if is_core_ready {
					fmt.Printf("Found work package %v in the history of core %v!\n", workPackageHash, receiverCore)
					msg <- "ok"
					return
				}
			}

		case <-ticker.C:
			// Sending the work package again

			prereqs := workPackage.RefineContext.Prerequisites
			newRefineContext := senderNode.statedb.GetRefineContext(prereqs...)
			workPackage.RefineContext = newRefineContext
			workPackageHash = workPackage.Hash()
			corePeers := senderNode.GetCoreCoWorkersPeers(receiverCore)
			randIdx := rand.Intn(len(corePeers))
			msg <- fmt.Sprint("trial")
			err := corePeers[randIdx].SendWorkPackageSubmission(*workPackage, extrinsics, receiverCore)
			log.Debug(debugG, "SendWorkPackageSubmission to core for trial/timeslot",
				"n", senderNode.id, "p", corePeers[randIdx].PeerID, "c", receiverCore, "wph", workPackageHash,
				"trialCount", trialCount, "ts", senderNode.statedb.GetSafrole().GetTimeSlot())
			if err != nil {
				fmt.Printf("SendWorkPackageSubmission ERR %v, sender: %d, receiver %d\n", err, senderNode.id, corePeers[randIdx].PeerID)
			}
			trialCount++
			if trialCount > MaxTrialCount {
				msg <- "failed"
				return
			}

		}
	}
}

func transfer(nodes []*Node, testServices map[string]*types.TestService, transferNum int) {
	fmt.Printf("\n=========================Start Transfer=========================\n\n")
	service0 := testServices["transfer_0"]
	service1 := testServices["transfer_1"]
	service_authcopy := testServices["auth_copy"]
	auth_copy_item := types.WorkItem{
		Service:            service_authcopy.ServiceCode,
		CodeHash:           service_authcopy.CodeHash,
		Payload:            []byte{},
		RefineGasLimit:     1000,
		AccumulateGasLimit: 1000,
		ImportedSegments:   make([]types.ImportSegment, 0),
		ExportCount:        0,
	}
	n1 := nodes[1]
	core := 0

	sa0, _, _ := n1.getState().GetService(service0.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_0\033[0m initial balance: \033[32m%v\033[0m\n", sa0.Balance)
	sa1, _, _ := n1.getState().GetService(service1.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_1\033[0m initial balance: \033[32m%v\033[0m\n", sa1.Balance)
	fmt.Printf("\n")
	prevBalance0 := sa0.Balance
	prevBalance1 := sa1.Balance

	TransferNum := transferNum
	Transfer_WorkPackages := make([]types.WorkPackage, 0, TransferNum)
	for n := 1; n <= TransferNum; n++ {
		refineContext := n1.statedb.GetRefineContext()
		var workPackage types.WorkPackage
		if n%2 == 0 {
			payload := make([]byte, 8)
			reciver := make([]byte, 4)
			binary.LittleEndian.PutUint32(reciver, uint32(service1.ServiceCode))
			amount := make([]byte, 4)
			binary.LittleEndian.PutUint32(amount, uint32(n*10))
			payload = append(reciver, amount...)

			workPackage = types.WorkPackage{
				Authorization:         []byte("0x"),
				AuthCodeHost:          0,
				AuthorizationCodeHash: bootstrap_auth_codehash,
				ParameterizationBlob:  []byte{},
				RefineContext:         refineContext,
				WorkItems: []types.WorkItem{
					{
						Service:            service0.ServiceCode,
						CodeHash:           service0.CodeHash,
						Payload:            payload,
						RefineGasLimit:     1000,
						AccumulateGasLimit: 1000,
						ImportedSegments:   make([]types.ImportSegment, 0),
						ExportCount:        1,
					},
					auth_copy_item,
				},
			}
		} else {
			payload := make([]byte, 8)
			reciver := make([]byte, 4)
			binary.LittleEndian.PutUint32(reciver, uint32(service0.ServiceCode))
			amount := make([]byte, 4)
			binary.LittleEndian.PutUint32(amount, uint32(n*10))
			payload = append(reciver, amount...)

			workPackage = types.WorkPackage{
				Authorization:         []byte("0x"),
				AuthCodeHost:          0,
				AuthorizationCodeHash: bootstrap_auth_codehash,
				ParameterizationBlob:  []byte{},
				RefineContext:         refineContext,
				WorkItems: []types.WorkItem{
					{
						Service:            service1.ServiceCode,
						CodeHash:           service1.CodeHash,
						Payload:            payload,
						RefineGasLimit:     1000,
						AccumulateGasLimit: 1000,
						ImportedSegments:   make([]types.ImportSegment, 0),
						ExportCount:        1,
					},
					auth_copy_item,
				},
			}
		}
		Transfer_WorkPackages = append(Transfer_WorkPackages, workPackage)
		fmt.Printf("** \033[36mTRANSFER WorkPackage #%d\033[0m: %v **\n", n, workPackage.Hash())
	}

	transferChan := make(chan types.WorkPackage, 1)
	transferSuccessful := make(chan string)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	transferCounter := 0
	transferReady := true

	for {
		if transferCounter >= TransferNum && transferReady {
			fmt.Printf("All transfer work packages processed\n")
			break
		}

		select {
		case <-ticker.C:
			if transferCounter < TransferNum && transferReady {
				wp := Transfer_WorkPackages[transferCounter]
				transferChan <- wp
				transferCounter++
				transferReady = false

				fmt.Printf("\n** \033[36m TRANSFER=%v \033[0m workPackage: %v **\n", transferCounter, wp.Hash().String_short())
			}
		case wp := <-transferChan:
			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
			go func(wp types.WorkPackage) {
				defer cancel()
				sendWorkPackageTrack(ctx, n1, &wp, uint16(core), transferSuccessful, types.ExtrinsicsBlobs{})
			}(wp)
		case success := <-transferSuccessful:
			if success != "ok" {
				fmt.Println("Transfer work package failed")
			}

			sa0, _, _ := n1.getState().GetService(service0.ServiceCode)
			sa1, _, _ := n1.getState().GetService(service1.ServiceCode)
			fmt.Printf("\033[38;5;208mtransfer_0_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", prevBalance0, sa0.Balance)
			fmt.Printf("\033[38;5;208mtransfer_1_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", prevBalance1, sa1.Balance)
			prevBalance0 = sa0.Balance
			prevBalance1 = sa1.Balance

			if transferCounter%2 == 0 {
				fmt.Printf("\n\033[38;5;208mtransfer_0\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_1\033[0m\n", transferCounter*10)
			} else {
				fmt.Printf("\n\033[38;5;208mtransfer_1\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_0\033[0m\n", transferCounter*10)
			}

			transferReady = true
		}
	}
}

func scaled_transfer(nodes []*Node, testServices map[string]*types.TestService, transferNum int, splitTransferNum int) {
	fmt.Printf("\n=========================Start Scaled Transfer=========================\n\n")
	service0 := testServices["transfer_0"]
	service1 := testServices["transfer_1"]
	service_authcopy := testServices["auth_copy"]
	auth_copy_item := types.WorkItem{
		Service:            service_authcopy.ServiceCode,
		CodeHash:           service_authcopy.CodeHash,
		Payload:            []byte{},
		RefineGasLimit:     1000,
		AccumulateGasLimit: 1000,
		ImportedSegments:   make([]types.ImportSegment, 0),
		ExportCount:        0,
	}

	n1 := nodes[1]
	core := 0

	sa0, _, _ := n1.getState().GetService(service0.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_0\033[0m initial balance: \033[32m%v\033[0m\n", sa0.Balance)
	sa1, _, _ := n1.getState().GetService(service1.ServiceCode)
	fmt.Printf("\033[38;5;208mtransfer_1\033[0m initial balance: \033[32m%v\033[0m\n", sa1.Balance)
	fmt.Printf("\n")
	prevBalance0 := sa0.Balance
	prevBalance1 := sa1.Balance

	TransferNum := transferNum           // 10
	SplitTransferNum := splitTransferNum // 600. 10000 for 0.62 MB
	Transfer_WorkPackages := make([]types.WorkPackage, 0, TransferNum)

	for n := 1; n <= TransferNum; n++ {
		refineContext := n1.statedb.GetRefineContext()
		var workPackage types.WorkPackage
		var Transfer_WorkItems []types.WorkItem
		if n%2 == 0 {
			for i := 0; i < SplitTransferNum; i++ {
				payload := make([]byte, 8)
				reciver := make([]byte, 4)
				binary.LittleEndian.PutUint32(reciver, uint32(service1.ServiceCode))
				amount := make([]byte, 4)
				binary.LittleEndian.PutUint32(amount, uint32(n))
				payload = append(reciver, amount...)

				Transfer_WorkItems = append(Transfer_WorkItems, types.WorkItem{
					Service:            service0.ServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        1,
				})
				Transfer_WorkItems = append(Transfer_WorkItems, auth_copy_item)
			}
			workPackage = types.WorkPackage{
				Authorization:         []byte("0x"),
				AuthCodeHost:          0,
				AuthorizationCodeHash: bootstrap_auth_codehash,
				ParameterizationBlob:  []byte{},
				RefineContext:         refineContext,
				WorkItems:             Transfer_WorkItems,
			}
		} else {
			for i := 0; i < SplitTransferNum; i++ {
				payload := make([]byte, 8)
				reciver := make([]byte, 4)
				binary.LittleEndian.PutUint32(reciver, uint32(service0.ServiceCode))
				amount := make([]byte, 4)
				binary.LittleEndian.PutUint32(amount, uint32(n))
				payload = append(reciver, amount...)

				Transfer_WorkItems = append(Transfer_WorkItems, types.WorkItem{
					Service:            service1.ServiceCode,
					CodeHash:           service1.CodeHash,
					Payload:            payload,
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        1,
				})
				Transfer_WorkItems = append(Transfer_WorkItems, auth_copy_item)
			}
			workPackage = types.WorkPackage{
				Authorization:         []byte("0x"),
				AuthCodeHost:          0,
				AuthorizationCodeHash: bootstrap_auth_codehash,
				ParameterizationBlob:  []byte{},
				RefineContext:         refineContext,
				WorkItems:             Transfer_WorkItems,
			}
		}
		Transfer_WorkPackages = append(Transfer_WorkPackages, workPackage)
		fmt.Printf("** \033[36mTRANSFER WorkPackage #%d\033[0m: %v **\n", n, workPackage.Hash())
		totalWPSizeInMB := calaulateTotalWPSize(workPackage, types.ExtrinsicsBlobs{})
		fmt.Printf("Total Work Package Size: %v MB\n", totalWPSizeInMB)
	}

	transferChan := make(chan types.WorkPackage, 1)
	transferSuccessful := make(chan string)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	transferCounter := 0
	transferReady := true

	for {
		if transferCounter >= TransferNum && transferReady {
			fmt.Printf("All transfer work packages processed\n")
			break
		}

		select {
		case <-ticker.C:
			if transferCounter < TransferNum && transferReady {
				wp := Transfer_WorkPackages[transferCounter]
				transferChan <- wp
				transferCounter++
				transferReady = false

				fmt.Printf("\n** \033[36m TRANSFER=%v \033[0m workPackage: %v **\n", transferCounter, wp.Hash().String_short())
			}
		case wp := <-transferChan:
			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
			go func(wp types.WorkPackage) {
				defer cancel()
				sendWorkPackageTrack(ctx, n1, &wp, uint16(core), transferSuccessful, types.ExtrinsicsBlobs{})
			}(wp)
		case success := <-transferSuccessful:
			if success != "ok" {
				fmt.Println("Transfer work package failed")
			}

			sa0, _, _ := n1.getState().GetService(service0.ServiceCode)
			sa1, _, _ := n1.getState().GetService(service1.ServiceCode)
			fmt.Printf("\033[38;5;208mtransfer_0_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", prevBalance0, sa0.Balance)
			fmt.Printf("\033[38;5;208mtransfer_1_balance\033[0m: \033[32m%v\033[0m -> \033[32m%v\033[0m\n", prevBalance1, sa1.Balance)
			prevBalance0 = sa0.Balance
			prevBalance1 = sa1.Balance

			if transferCounter%2 == 0 {
				fmt.Printf("\n\033[38;5;208mtransfer_0\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_1\033[0m\n", transferCounter*SplitTransferNum)
			} else {
				fmt.Printf("\n\033[38;5;208mtransfer_1\033[0m sent \033[32m%v\033[0m tokens to \033[38;5;208mtransfer_0\033[0m\n", transferCounter*SplitTransferNum)
			}

			transferReady = true
		}
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

func balances(nodes []*Node, testServices map[string]*types.TestService, targetN int) {

	// !!!! targetN is NOT USED - need to wire this up
	fmt.Printf("\n=========================Balances Test=========================\n")

	// General setup
	BalanceService := testServices["balances"]
	service_authcopy := testServices["auth_copy"]
	auth_copy_item := types.WorkItem{
		Service:            service_authcopy.ServiceCode,
		CodeHash:           service_authcopy.CodeHash,
		Payload:            []byte{},
		RefineGasLimit:     1000,
		AccumulateGasLimit: 1000,
		ImportedSegments:   make([]types.ImportSegment, 0),
		ExportCount:        0,
	}
	balancesServiceIndex := BalanceService.ServiceCode
	balancesServiceCodeHash := BalanceService.CodeHash

	n1 := nodes[1]
	n4 := nodes[4]
	core := 0

	refineContext := nodes[1].statedb.GetRefineContext()
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
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
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
			Authorization:         []byte("0x"),
			AuthCodeHost:          0,
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			RefineContext:         refineContext,
			WorkItems: []types.WorkItem{
				{
					Service:            balancesServiceIndex,
					CodeHash:           balancesServiceCodeHash,
					Payload:            payload_bytes,
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					Extrinsics:         work_item_extrinsic,
					ExportCount:        1,
				},
				auth_copy_item,
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
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
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
		Authorization:         []byte("0x"),
		AuthCodeHost:          balancesServiceIndex,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
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
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
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
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
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

func scaled_balances(nodes []*Node, testServices map[string]*types.TestService, targetN_mint int, targetN_transfer int) {
	fmt.Printf("\n=========================Balances Scale Test=========================\n")
	// General setup
	BalanceService := testServices["balances"]
	balancesServiceIndex := BalanceService.ServiceCode
	balancesServiceCodeHash := BalanceService.CodeHash
	service_authcopy := testServices["auth_copy"]
	auth_copy_item := types.WorkItem{
		Service:            service_authcopy.ServiceCode,
		CodeHash:           service_authcopy.CodeHash,
		Payload:            []byte{},
		RefineGasLimit:     1000,
		AccumulateGasLimit: 1000,
		ImportedSegments:   make([]types.ImportSegment, 0),
		ExportCount:        0,
	}
	n1 := nodes[1]
	n4 := nodes[4]
	core := 0
	refineContext := nodes[1].statedb.GetRefineContext()

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
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refineContext,
		WorkItems: []types.WorkItem{
			{
				Service:            balancesServiceIndex,
				CodeHash:           balancesServiceCodeHash,
				Payload:            payload_bytes,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   make([]types.ImportSegment, 0),
				Extrinsics:         work_item_extrinsic,
				ExportCount:        1,
			},
			auth_copy_item,
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
	num_of_mint := targetN_mint // 110
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
			RefineGasLimit:     1000,
			AccumulateGasLimit: 1000,
			ImportedSegments:   make([]types.ImportSegment, 0),
			Extrinsics:         work_item_extrinsic,
			ExportCount:        1,
		}

		mint_workItems = append(mint_workItems, workItem)
		mint_workItems = append(mint_workItems, auth_copy_item)
	}

	mint_workPackage = types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refineContext,
		WorkItems:             mint_workItems,
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
	num_of_transfer := targetN_transfer // 100
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
			RefineGasLimit:     1000,
			AccumulateGasLimit: 1000,
			ImportedSegments:   make([]types.ImportSegment, 0),
			Extrinsics:         work_item_extrinsic,
			ExportCount:        1,
		}

		transfer_workItems = append(transfer_workItems, workItem)
		transfer_workItems = append(transfer_workItems, auth_copy_item)

	}

	transfer_workPackage = types.WorkPackage{
		Authorization:         []byte("0x"),
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ParameterizationBlob:  []byte{},
		RefineContext:         refineContext,
		WorkItems:             transfer_workItems,
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

func empty(nodes []*Node, testServices map[string]*types.TestService) {
	delayservice := testServices["delay"]

	n1 := nodes[1]
	core := 0

	mbs := []int{1, 3, 6, 12}
	seconds := []int{1, 2, 3, 6}
	service_authcopy := testServices["auth_copy"]
	auth_copy_item := types.WorkItem{
		Service:            service_authcopy.ServiceCode,
		CodeHash:           service_authcopy.CodeHash,
		Payload:            []byte{},
		RefineGasLimit:     1000,
		AccumulateGasLimit: 1000,
		ImportedSegments:   make([]types.ImportSegment, 0),
		ExportCount:        0,
	}
	fmt.Printf("\n=========================Start Size Test=========================\n")
	SizeWorkPackages := make([]types.WorkPackage, 0, len(mbs))
	SizeExtrinsicsBlobs := make([]types.ExtrinsicsBlobs, 0, len(mbs))
	for _, mb := range mbs {
		refineContext := n1.statedb.GetRefineContext()
		var workPackage types.WorkPackage

		payload_bytes := make([]byte, 8)
		second_bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(second_bytes, uint32(0))
		mb_bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(mb_bytes, uint32(mb))
		payload_bytes = append(second_bytes, mb_bytes...)

		workPackage = types.WorkPackage{
			Authorization:         []byte("0x"),
			AuthCodeHost:          0,
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			RefineContext:         refineContext,
			WorkItems: []types.WorkItem{
				{
					Service:            delayservice.ServiceCode,
					CodeHash:           delayservice.CodeHash,
					Payload:            payload_bytes,
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        1,
				},
				auth_copy_item,
			},
		}
		ExtrinsicsBlobs := make(types.ExtrinsicsBlobs, 1)
		ExtrinsicsBlobs[0] = GenerateNMBFilledBytes(mb)

		SizeWorkPackages = append(SizeWorkPackages, workPackage)
		SizeExtrinsicsBlobs = append(SizeExtrinsicsBlobs, ExtrinsicsBlobs)
		fmt.Printf("** \033[36m %d MB \033[0m workPackage: %v \033[0m\n", mb, workPackage.Hash().String_short())
	}

	SizeChan := make(chan types.WorkPackage, 1)
	SizeSuccessful := make(chan string)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	SizeCounter := 0
	SizeReady := true

	for {
		if SizeCounter >= len(mbs) && SizeReady {
			fmt.Printf("All Size work packages processed\n\n")
			break
		}

		select {
		case <-ticker.C:
			if SizeCounter < len(mbs) && SizeReady {
				wp := SizeWorkPackages[SizeCounter]
				SizeChan <- wp
				SizeCounter++
				SizeReady = false
				fmt.Printf("\n** \033[36m %d MB \033[0m workPackage: %v \033[0m \033[38;5;208m actual size: \033[0m %.2f MB **\n", mbs[SizeCounter-1], wp.Hash().String_short(), calaulateTotalWPSize(wp, SizeExtrinsicsBlobs[SizeCounter-1]))

			}
		case wp := <-SizeChan:
			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
			go func(wp types.WorkPackage) {
				defer cancel()
				sendWorkPackageTrack(ctx, n1, &wp, uint16(core), SizeSuccessful, SizeExtrinsicsBlobs[SizeCounter-1])
			}(wp)
		case success := <-SizeSuccessful:
			if success != "ok" {
				fmt.Printf("%d MB work package failed\n", mbs[SizeCounter-1])
			}
			SizeReady = true
		}
	}

	time.Sleep(12 * time.Second)

	fmt.Printf("\n=========================Start Time Test=========================\n")
	SecondsWorkPackages := make([]types.WorkPackage, 0, len(seconds))
	SecondsExtrinsicsBlobs := make([]types.ExtrinsicsBlobs, 0, len(seconds))
	for _, second := range seconds {
		refineContext := n1.statedb.GetRefineContext()
		var workPackage types.WorkPackage

		payload_bytes := make([]byte, 8)
		second_bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(second_bytes, uint32(second))
		mb_bytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(mb_bytes, uint32(0))
		payload_bytes = append(second_bytes, mb_bytes...)

		workPackage = types.WorkPackage{
			Authorization:         []byte("0x"),
			AuthCodeHost:          0,
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			RefineContext:         refineContext,
			WorkItems: []types.WorkItem{
				{
					Service:            delayservice.ServiceCode,
					CodeHash:           delayservice.CodeHash,
					Payload:            payload_bytes,
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        1,
				},
				auth_copy_item,
			},
		}
		ExtrinsicsBlobs := make(types.ExtrinsicsBlobs, 1)
		ExtrinsicsBlobs[0] = GenerateNMBFilledBytes(0)

		SecondsWorkPackages = append(SecondsWorkPackages, workPackage)
		SecondsExtrinsicsBlobs = append(SecondsExtrinsicsBlobs, ExtrinsicsBlobs)
		fmt.Printf("** \033[36m %d seconds \033[0m workPackage: %v \033[0m\n", second, workPackage.Hash().String_short())
	}

	SecondsChan := make(chan types.WorkPackage, 1)
	SecondsSuccessful := make(chan string)

	SecondsCounter := 0
	SecondsReady := true

	for {
		if SecondsCounter >= len(seconds) && SecondsReady {
			fmt.Printf("All Seconds work packages processed\n")
			break
		}

		select {
		case <-ticker.C:
			if SecondsCounter < len(seconds) && SecondsReady {
				wp := SecondsWorkPackages[SecondsCounter]
				SecondsChan <- wp
				SecondsCounter++
				SecondsReady = false
				fmt.Printf("\n** \033[36m %d seconds \033[0m workPackage: %v \033[0m\n", seconds[SecondsCounter-1], wp.Hash().String_short())
			}
		case wp := <-SecondsChan:
			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
			go func(wp types.WorkPackage) {
				defer cancel()
				sendWorkPackageTrack(ctx, n1, &wp, uint16(core), SecondsSuccessful, SecondsExtrinsicsBlobs[SecondsCounter-1])
			}(wp)
		case success := <-SecondsSuccessful:
			if success == "ok" {
				fmt.Printf("%d seconds work package failed\n", seconds[SecondsCounter-1])
			}
			SecondsReady = true
		}
	}

	time.Sleep(12 * time.Second)
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
	ka := common.ServiceStorageKey(balance_service_index, key_byte)
	service_account_byte, _, _ := n.getState().GetTrie().GetServiceStorage(balance_service_index, ka)
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
	ka := common.ServiceStorageKey(balance_service_index, key_byte)
	service_account_byte, _, _ := n.getState().GetTrie().GetServiceStorage(balance_service_index, ka)

	fetched_account, _ := account.FromBytes(service_account_byte)
	fmt.Printf("\n\033[38;5;13mAccount Key\033[0m: \033[32m%x\033[0m\n", account_key)
	fmt.Printf("\033[38;5;13mAsset ID\033[0m: \033[32m%v\033[0m\n", asset_id)
	fmt.Printf("\033[38;5;13mAccount Nonce\033[0m: \033[32m%v\033[0m\n", fetched_account.Nonce)
	fmt.Printf("\033[38;5;13mAccount Free\033[0m: \033[32m%v\033[0m\n", fetched_account.Free)
	fmt.Printf("\033[38;5;13mAccount Reserved\033[0m: \033[32m%v\033[0m\n", fetched_account.Reserved)
}

func generateJobID() string {
	seed := uint64(time.Now().UnixNano()) // nano seconds. but still not unique
	source := rand.NewSource(seed)
	r := rand.New(source)
	var out [8]byte
	binary.LittleEndian.PutUint64(out[:], r.Uint64())
	jobID := fmt.Sprintf("%x", out)
	return jobID
}

func GenerateNMBFilledBytes(n int) []byte {
	size := n * 1024 * 1024
	return bytes.Repeat([]byte{1}, size)
}

func blake2b(nodes []*Node, testServices map[string]*types.TestService, targetN int) {
	fmt.Printf("Start Blake2b Test\n")

	service0 := testServices["blake2b"]
	n1 := nodes[1]
	n4 := nodes[4]
	core := 0

	refine_context := n1.statedb.GetRefineContext()

	payload := make([]byte, 0)
	input := []byte("Hello, Blake2b!")
	input_length := uint32(len(input))
	input_length_bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(input_length_bytes, input_length)
	payload = append(payload, input_length_bytes...)
	payload = append(payload, input...)

	workPackage := types.WorkPackage{
		Authorization:         []byte("0x"), // TODO: set up null-authorizer
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		RefineContext:         refine_context,
		WorkItems: []types.WorkItem{
			{
				Service:            service0.ServiceCode,
				CodeHash:           service0.CodeHash,
				Payload:            payload,
				RefineGasLimit:     1000,
				AccumulateGasLimit: 1000,
				ImportedSegments:   []types.ImportSegment{},
				ExportCount:        1,
			},
		},
	}
	workPackageHash := workPackage.Hash()

	fmt.Printf("\n** \033[36m Blake2b=%v \033[0m workPackage: %v **\n", 1, common.Str(workPackageHash))
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
	fmt.Printf("\nBlake2b Expected Result: %x\n", common.Blake2Hash(input).Bytes())
	k := common.ServiceStorageKey(service0.ServiceCode, []byte{0})
	hash_result, _, _ := n1.getState().GetTrie().GetServiceStorage(service0.ServiceCode, k)
	fmt.Printf("Blake2b Actual Result:   %x\n", hash_result)
}

func fib2(nodes []*Node, testServices map[string]*types.TestService, targetN int) {
	log.Info(module, "FIB2 START", "targetN", targetN)

	jam_key := []byte("jam")
	jam_key_hash := common.Blake2Hash(jam_key)
	jam_key_length := uint32(len(jam_key))

	service0 := testServices["fib2"]
	service_authcopy := testServices["auth_copy"]
	n1 := nodes[1]
	n4 := nodes[4]
	core := 0
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
		refine_context := n1.statedb.GetRefineContext()

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			RefineContext:         refine_context,
			WorkItems: []types.WorkItem{
				{
					Service:            service0.ServiceCode,
					CodeHash:           service0.CodeHash,
					Payload:            payload,
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   importedSegments,
					ExportCount:        1,
				},
				{
					Service:            service_authcopy.ServiceCode,
					CodeHash:           service_authcopy.CodeHash,
					Payload:            []byte{},
					RefineGasLimit:     1000,
					AccumulateGasLimit: 1000,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}
		workPackageHash := workPackage.Hash()

		log.Info(module, fmt.Sprintf("FIB2-(%v) work package submitted", fibN), "workPackage", workPackageHash)
		core0_peers := n1.GetCoreCoWorkersPeers(uint16(core))
		ramdamIdx := rand.Intn(3)
		err := core0_peers[ramdamIdx].SendWorkPackageSubmission(workPackage, types.ExtrinsicsBlobs{}, 0)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// wait until the work report is pending
		i := 0
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil || i > 12 {
				if false {
					var workReport types.WorkReport
					rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
					workReport = rho_state.WorkReport
					fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				}
				break
			}
			i += 1
		}

		// wait until the work report is cleared
		for {
			if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}

		prevWorkPackageHash = workPackageHash
		time.Sleep(1 * time.Second)
		keys := []byte{0, 1, 2, 5, 6, 8, 9}
		for _, key := range keys {
			k := common.ServiceStorageKey(service0.ServiceCode, []byte{key})
			service_account_byte, _, _ := n4.getState().GetTrie().GetServiceStorage(service0.ServiceCode, k)
			log.Info(module, fmt.Sprintf("Fib2-(%v) result with key %d", fibN, key), "result", fmt.Sprintf("%x", service_account_byte))
		}

		if fibN == 3 || fibN == 6 {
			time.Sleep(3 * time.Second)
			err = n1.BroadcastPreimageAnnouncement(service0.ServiceCode, jam_key_hash, jam_key_length, jam_key)
			if err != nil {
				log.Error(debugP, "BroadcastPreimageAnnouncement", "err", err)
			}
			time.Sleep(3 * time.Second)
		}
	}
}
