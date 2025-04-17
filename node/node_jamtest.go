//go:build network_test
// +build network_test

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
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/rand"

	"net/http"
	_ "net/http/pprof"
	"os"
	"os/user"
	"time"
)

var jamManualSimple = flag.Bool("jce_manual_simple", false, "jce_manual_simple")
var prereq_test = flag.Bool("prereq_test", false, "prereq_test")
var pvm_authoring_log = flag.Bool("pvm_authoring_log", false, "pvm_authoring_log")

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
	currTS := common.ComputeCurrentTS()
	jobID := generateJobID()
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

func SetupQuicNetwork(network string, basePort uint16) (uint64, []string, map[uint16]*Peer, []types.ValidatorSecret, []string, error) {
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

func SetUpNodes(jceMode string, numNodes int, basePort uint16) ([]*Node, error) {
	network := types.Network
	GenesisStateFile, GenesisBlockFile := GetGenesisFile(network)
	log.InitLogger("debug")

	epoch0Timestamp, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork(network, basePort)

	if err != nil {
		return nil, err
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], GenesisStateFile, GenesisBlockFile, epoch0Timestamp, peers, peerList, ValidatorFlag, nodePaths[i], int(basePort)+i, jceMode)
		if err != nil {
			panic(err)
		}
		nodes[i] = node
	}
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
	nodes, err := SetUpNodes(JCEDefault, numNodes, 10000)
	if err != nil {
		panic(err)
	}
	for _, n := range nodes {
		n.SetSendTickets(sendtickets)
	}
}

func GetService(serviceNames []string, getmetadata bool) (services map[string]*types.TestService, err error) {
	return getServices(serviceNames, getmetadata)
}

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
		log.Info(module, serviceName, "codeHash", codeHash, "len", len(code))
	}
	return
}

func safroleTest(t *testing.T, caseType string, targetedEpochLen int, basePort uint16, bufferTime int) {
	nodes, err := SetUpNodes(JCEManual, numNodes, basePort) // TODO change this to JCEManual
	if err != nil {
		panic(err)
	}

	var sendtickets bool
	if caseType == "safrole" {
		sendtickets = true
	} else {
		sendtickets = false
	}

	// Run the JCE updater (it will run indefinitely).
	initialJCE := uint32(11)
	jceMode, jceManager, managerCancel, err := SetupJceManager(nodes, initialJCE, nodes[0].jceMode)
	if err != nil && jceManager == nil {
		t.Fatalf("JCE Manager setup failed: %v", err)
	}

	if managerCancel != nil {
		// TODO Schedule the cancel function call for when jamtest exits
		defer managerCancel()
	}
	if jceManager != nil {
		fmt.Printf("jamtest: Manual JCE Manager (%p) is set up and running.\n", jceManager)
		go jceManager.Replenish()
	} else {
		fmt.Printf("jamtest: JCE Mode: %s\n", jceMode)
	}
	targetTimeslotLength := uint32(targetedEpochLen * types.EpochLength)
	maxTimeAllowed := (targetTimeslotLength+1)*types.SecondsPerSlot + uint32(bufferTime)

	//log.EnableModule(log.GeneralAuthoring)
	log.Info(module, "JAMTEST", "jam", caseType, "targetN", targetTimeslotLength)

	for _, n := range nodes {
		n.SetSendTickets(sendtickets)
	}

	watchNode := nodes[len(nodes)-1]

	done := make(chan bool)
	errChan := make(chan error)

	go RunGrandpaGraphServer(watchNode, basePort)

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

func jamtestclient(t *testing.T, jam string, targetedEpochLen int, basePort uint16, targetN int) {

	var logLevel string
	flag.StringVar(&logLevel, "log", "debug", "Logging level (e.g., debug, info, warn, error, crit)")
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
	/*
		//log.EnableModule(log.PvmAuthoring)
		//log.EnableModule(log.FirstGuarantor)
	*/

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
		fib3(nodes, testServices, targetN)
	}
}

func jamtest(t *testing.T, jam string, targetedEpochLen int, basePort uint16, targetN int) {
	flag.Parse()

	/// TODO: use JCEManual;
	nodes, err := SetUpNodes(JCEDefault, numNodes, basePort) // TODO change this to JCEManual
	if err != nil {
		panic("Error setting up nodes: %v\n")
	}
	log.Info(module, "JAMTEST", "jam", jam, "targetN", targetN)

	if *prereq_test {
		test_prereq = true
	}

	fmt.Printf("Test PreReq: %v\n", test_prereq)
	// log.EnableModule(log.PvmAuthoring)
	// log.EnableModule(log.FirstGuarantorOrAuditor)
	//log.EnableModule(debugDA)
	//log.EnableModule(log.GeneralAuthoring)
	var game_of_life_ws_push func([]byte)
	if jam == "game_of_life" {
		game_of_life_ws_push = StartGameOfLifeServer("localhost:8080", "../client/game_of_life.html")
	}

	// Run the JCE updater (it will run indefinitely).
	initialJCE := uint32(11)
	jceMode, jceManager, managerCancel, err := SetupJceManager(nodes, initialJCE, nodes[0].jceMode)
	fmt.Println("jamtest: setupJceManager returned.")

	if err != nil && jceManager == nil {
		t.Fatalf("JCE Manager setup failed: %v", err)
	}

	if managerCancel != nil {
		// TODO Schedule the cancel function call for when jamtest exits
		defer managerCancel()
	}

	if jceManager != nil {
		fmt.Printf("jamtest: Manual JCE Manager (%p) is set up and running.\n", jceManager)
	} else {
		fmt.Printf("jamtest: JCE Mode: %s\n", jceMode)
	}

	//go RunGrandpaGraphServer(nodes[0], basePort)
	for {
		time.Sleep(1 * time.Second)
		currJCE := nodes[0].GetCurrJCE()
		if nodes[0].statedb.GetSafrole().CheckFirstPhaseReady(currJCE) {
			break
		}
	}

	// code length: 206
	bootstrapCode, err := types.ReadCodeWithMetadata(statedb.BootstrapServiceFile, "bootstrap")
	if err != nil {
		panic(0)
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)

	builderNode := nodes[1]
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
		serviceNames = []string{"corevm", "auth_copy"}
		//log.EnableModule(log.StateDBMonitoring) //enable here to avoid concurrent map
	}
	if jam == "game_of_life" {
		serviceNames = []string{"game_of_life", "auth_copy"}
	}
	testServices, err := getServices(serviceNames, true)
	if err != nil {
		t.Fatalf("GetServices %v", err)
	}

	log.Info(module, "Waiting for the first block to be ready...")
	time.Sleep(2 * types.SecondsPerSlot * time.Second) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot

	var previous_service_idx uint32
	for serviceName, service := range testServices {
		// set up service using the Bootstrap service
		log.Info(module, "Builder storing TestService", "serviceName", serviceName, "codeHash", service.CodeHash)
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
		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("Builder storing TestService %s", serviceName),
			WorkPackage:     codeWorkPackage,
			CoreIndex:       0,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}
		if jceManager != nil {
			wpr.JCEManager = jceManager
		}

		_, err := builderNode.SubmitAndWaitForWorkPackage(ctx, wpr)
		if err != nil {
			t.Fatalf("SendWorkPackageSubmission ERR %v\n", err)
		}
		k := common.ServiceStorageKey(bootstrapService, []byte{0, 0, 0, 0})
		service_account_byte, ok, err := builderNode.GetServiceStorage(0, k)
		if err != nil {
			t.Fatalf("SendWorkPackageSubmission ERR %v", err)
		}
		if !ok {

		}
		decoded_new_service_idx := uint32(types.DecodeE_l(service_account_byte))
		if decoded_new_service_idx != 0 && (decoded_new_service_idx != previous_service_idx) {
			service.ServiceCode = decoded_new_service_idx
			new_service_idx = decoded_new_service_idx
		}

		err = builderNode.SubmitAndWaitForPreimage(ctx, new_service_idx, service.Code)
		if err != nil {
			log.Error(module, "SubmitAndWaitForPreimage", "err", err)
		}
		log.Info(module, "----- NEW SERVICE", "service", serviceName, "service_idx", new_service_idx)

	}

	log.Info(module, "testServices Loaded", "jam", jam, "testServices", testServices, "targetN", targetN)

	switch jam {
	case "megatron":
		megatron(nodes, testServices, targetN, jceManager)
	case "fib":
		fib(nodes, testServices, targetN, jceManager)
	case "fib2":
		targetN := 100
		fib2(nodes, testServices, targetN, jceManager)
	case "transfer":
		transferNum := targetN
		transfer(nodes, testServices, transferNum, jceManager)
	case "scaled_transfer":
		transferNum := 10
		splitTransferNum := targetN
		scaled_transfer(nodes, testServices, transferNum, splitTransferNum, jceManager)
	case "balances":
		// not using anything
		balances(nodes, testServices, targetN, jceManager)
	case "scaled_balances":
		targetN_mint := targetN
		targetN_transfer := targetN
		scaled_balances(nodes, testServices, targetN_mint, targetN_transfer, jceManager)
	case "blake2b":
		blake2b(nodes, testServices, targetN, jceManager)
	case "game_of_life":
		time.Sleep(10 * time.Second)
		game_of_life(nodes, testServices, game_of_life_ws_push, jceManager)
	}
}
