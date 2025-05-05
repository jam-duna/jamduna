//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/rand"

	_ "net/http/pprof"
	"os"
	"os/user"
	"time"
)

const (
	DefaultRefineGasLimit     = uint64(800000)
	DefaultAccumulateGasLimit = uint64(80000)
)

var jce_manual = flag.Bool("jce_manual", false, "jce_manual")
var jam_node = flag.Bool("jam_node", false, "jam_node")
var jam_local_client = flag.Bool("jam_local_client", false, "jam_local_client")
var manifest = flag.Bool("manifest", false, "manifest")

const (
	webServicePort = 8079
)

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
	GenesisStateFile := GetGenesisFile(network)
	log.InitLogger("debug")

	epoch0Timestamp, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork(network, basePort)

	if err != nil {
		return nil, err
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], GenesisStateFile, epoch0Timestamp, peers, peerList, nodePaths[i], int(basePort)+i, jceMode)
		if err != nil {
			return nil, err
		}
		nodes[i] = node
	}
	return nodes, nil
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

// run any test with dispute using testName_dispute (i.e fib_dispute)
func testWithDispute(raw_jam string) (jam string, isDisputeMode bool) {
	const suffix = "_dispute"
	if strings.HasSuffix(raw_jam, suffix) {
		jam = strings.TrimSuffix(raw_jam, suffix)
		return jam, true
	}
	return raw_jam, false
}

func jamtest(t *testing.T, jam_raw string, targetN int) {
	flag.Parse()

	sendTickets := true //set this to false to run WP without E_T interference
	jam, isDisputeMode := testWithDispute(jam_raw)
	//fmt.Printf("jamtest: jam=%s | isDisputeMode=%v | raw=%v\n", jam, isDisputeMode, jam_raw)

	// Specify testServices
	defaultDelay := 2 * types.SecondsPerSlot * time.Second
	var serviceNames []string
	switch jam {
	case "safrole", "fallback":
		defaultDelay = 0 * types.SecondsPerSlot * time.Second
		sendTickets = (jam == "safrole")
	case "megatron":
		serviceNames = []string{"fib", "tribonacci", "megatron", "auth_copy"} // Others include: "padovan", "pell", "racaman"
	case "transfer", "scaled_transfer":
		serviceNames = []string{"transfer_0", "transfer_1", "auth_copy"} // 2 transfer services share the same code
	case "balances", "scaled_balances":
		serviceNames = []string{"balances", "auth_copy"}
	case "empty":
		serviceNames = []string{"delay", "auth_copy"}
	case "blake2b":
		serviceNames = []string{"blake2b"}
	case "fib":
		serviceNames = []string{"fib", "auth_copy"}
	case "fib2":
		serviceNames = []string{"corevm", "auth_copy"}
		//log.EnableModule(log.StateDBMonitoring) //enable here to avoid concurrent map
	case "game_of_life":
		if *manifest {
			serviceNames = []string{"game_of_life_manifest", "auth_copy"}
		} else {
			serviceNames = []string{"game_of_life", "auth_copy"}
		}
	case "auth_copy":
		serviceNames = []string{"auth_copy"}
	case "revm":
		serviceNames = []string{"revm_test", "auth_copy"}
	default:
		serviceNames = []string{"auth_copy", "fib"}
	}

	var bNode JNode
	var tNode *Node
	var err error

	if !*jam_node { // NodeClient -- client scenario (could be dot-0 OR localhost:____ )
		if isDisputeMode {
			t.Fatalf("Dispute mode is not supported for client test\n")
		}
		if jam == "safrole" || jam == "fallback" {
			t.Logf("Nothing to test for %s-client\n", jam)
			return
		}

		fmt.Printf("jamtest: %s-client (local=%v)\n", jam, *jam_local_client)
		tcpServers, wsUrl := GetAddresses(*jam_local_client)
		bNode, err = NewNodeClient(tcpServers, wsUrl[0])
		if err != nil {
			fmt.Printf("NewNodeClient ERR %v\n", err)
			err = fmt.Errorf("‼️ jamtest: %s-client (local=%v) Failed. Connection Problem?\n", jam, *jam_local_client)
			t.Fatalf("%v", err)
		}
		fmt.Printf("%s tcp:%v\n", wsUrl, tcpServers)
		client := bNode.(*NodeClient)
		err = client.ConnectWebSocket(wsUrl[0])
	} else { // Node

		fmt.Printf("jamtest: %s-node\n", jam)
		basePort := GenerateRandomBasePort()

		JCEMode := JCEDefault
		if *jce_manual {
			JCEMode = JCEManual
		}
		if jam == "safrole" || jam == "fallback" {
			JCEMode = JCEFast
		}

		nodes, err := SetUpNodes(JCEMode, numNodes, basePort)
		if err != nil {
			log.Crit(module, "Error setting up nodes", "err", err)
			return
		}

		// Handling Safrole
		for _, n := range nodes {
			n.SetSendTickets(sendTickets)
		}

		// Handling Dispute Mode
		if isDisputeMode {
			nodeTypeList := []string{"lying_judger_F", "lying_judger_T", "lying_judger_T", "lying_judger_T", "lying_judger_T", "lying_judger_T"}
			fmt.Printf("****** DisputeMode! ******\n")
			for i, node := range nodes {
				fmt.Printf("N%d ---- %s\n", i, nodeTypeList[i])
				node.AuditNodeType = nodeTypeList[i]
			}
			fmt.Printf("**************************\n")
		}
		log.Info(module, "JAMTEST", "jam", jam, "targetN", targetN)

		fmt.Printf("Test PreReq: %v\n", test_prereq)

		// Run the JCE updater (it will run indefinitely).
		initialJCE := uint32(11)
		jceMode, jceManager, managerCancel, err := SetupJceManager(nodes, initialJCE, JCEMode)
		fmt.Println("jamtest: setupJceManager returned.")

		if err != nil && jceManager == nil {
			t.Fatalf("JCE Manager setup failed: %v", err)
		}

		tNode = nodes[1]
		if managerCancel != nil {
			tNode.SetJCEManager(jceManager)
			defer managerCancel()
		}
		if jceManager != nil {
			fmt.Printf("jamtest: Manual JCE Manager (%p) is set up and running.\n", jceManager)
		} else {
			fmt.Printf("jamtest: JCE Mode: %s\n", jceMode)
		}
		//go RunGrandpaGraphServer(n1, basePort)
		for {
			time.Sleep(1 * time.Second)
			currJCE := tNode.GetCurrJCE()
			if tNode.statedb.GetSafrole().CheckFirstPhaseReady(currJCE) {
				break
			}
		}
		bNode = nodes[1]
	}

	log.EnableModule(log.FirstGuarantorOrAuditor)
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule(log.GeneralAuthoring)

	bootstrapCode, err := types.ReadCodeWithMetadata(statedb.BootstrapServiceFile, "bootstrap")
	if err != nil {
		log.Error(module, "ReadCodeWithMetadata", "err", err)
		return
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)

	new_service_idx := uint32(0)

	testServices, err := getServices(serviceNames, true)
	if err != nil {
		t.Fatalf("GetServices %v", err)
	}

	log.Info(module, "Waiting for the first block to be ready...")
	time.Sleep(defaultDelay) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot

	var jceManager *ManualJCEManager
	jceManager = nil
	var previous_service_idx uint32
	for serviceName, service := range testServices {
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
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}
		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("NewService(%s, %s)", serviceName, common.Str(service.CodeHash)),
			WorkPackage:     codeWorkPackage,
			CoreIndex:       0,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		_, err := RobustSubmitAndWaitForWorkPackages(ctx, bNode, []*WorkPackageRequest{wpr})
		if err != nil {
			t.Fatalf("SendWorkPackageSubmission ERR %v\n", err)
		}
		k := common.ServiceStorageKey(bootstrapService, []byte{0, 0, 0, 0})
		service_account_byte, ok, err := bNode.GetServiceStorage(0, k)
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

		err = bNode.SubmitAndWaitForPreimage(ctx, new_service_idx, service.Code)
		if err != nil {
			log.Error(module, "SubmitAndWaitForPreimage", "err", err)
		}
		log.Info(module, "----- NEW SERVICE", "service", serviceName, "service_idx", new_service_idx)

	}
	if len(testServices) > 0 {
		log.Info(module, "testServices Loaded", "jam", jam, "testServices", testServices, "targetN", targetN)
	}

	if isDisputeMode {
		targetTimeslotLength := (12 * types.EpochLength)
		maxTimeAllowed := (targetTimeslotLength + 1) * types.SecondsPerSlot
		waitForTermination(tNode, "jam", targetTimeslotLength, maxTimeAllowed, t)
	}

	switch jam {
	case "safrole":
		SafroleTestEpochLen := 4
		SafroleBufferTime := 30
		safrole(jceManager)
		if tNode == nil {
			t.Fatalf("tNode is nil")
		}
		waitForTermination(tNode, "safrole", SafroleTestEpochLen, SafroleBufferTime, t)
	case "fallback":
		FallbackEpochLen := 3
		FallbackBufferTime := 10
		safrole(jceManager)
		waitForTermination(tNode, "fallback", FallbackEpochLen, FallbackBufferTime, t)
	case "fib":
		fib(bNode, testServices, targetN)
	case "fib2":
		//targetN := 100
		fib2(bNode, testServices, targetN)
	case "game_of_life":
		game_of_life(bNode, testServices, *manifest)
	case "megatron":
		megatron(bNode, testServices, targetN)
	case "auth_copy":
		auth_copy(bNode, testServices, targetN)

	// TODO: modernize those tests?
	case "transfer":
		transferNum := targetN
		transfer(bNode, testServices, transferNum)
	case "scaled_transfer":
		transferNum := 10
		splitTransferNum := targetN
		scaled_transfer(bNode, testServices, transferNum, splitTransferNum)
	case "balances":
		// not using anything
		balances(bNode, testServices)
	case "scaled_balances":
		targetN_mint := targetN
		targetN_transfer := targetN
		scaled_balances(bNode, testServices, targetN_mint, targetN_transfer)
	case "blake2b":
		blake2b(bNode, testServices)
	case "revm":
		revm(bNode, testServices)
	}
}

func auth_copy(n1 JNode, testServices map[string]*types.TestService, targetN int) {
	log.Info(module, "FIB START", "targetN", targetN)
	service_authcopy := testServices["auth_copy"]

	for fibN := 1; fibN <= targetN; fibN++ {
		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))
		workPackage := types.WorkPackage{
			AuthCodeHost:          0,
			Authorization:         []byte("0x"), // TODO: set up null-authorizer
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			WorkItems: []types.WorkItem{
				{
					Service:            service_authcopy.ServiceCode,
					CodeHash:           service_authcopy.CodeHash,
					Payload:            []byte{},
					RefineGasLimit:     DefaultRefineGasLimit,
					AccumulateGasLimit: DefaultAccumulateGasLimit,
					ImportedSegments:   make([]types.ImportSegment, 0),
					ExportCount:        0,
				},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()
		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("Auth(%d)", fibN),
			CoreIndex:       1,
			WorkPackage:     workPackage,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		_, err := n1.SubmitAndWaitForWorkPackage(ctx, wpr)
		if err != nil {
			fmt.Printf("SubmitAndWaitForWorkPackage ERR %v\n", err)
		} else {
			log.Info(module, "Authcopy Success", "N", fibN)
		}
	}
}
