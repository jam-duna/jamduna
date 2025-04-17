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

type JNode interface {
	SubmitAndWaitForWorkPackage(ctx context.Context, wpr *WorkPackageRequest) (common.Hash, error)
	SubmitAndWaitForPreimage(ctx context.Context, serviceID uint32, preimage []byte) error
	GetService(service uint32) (sa *types.ServiceAccount, ok bool, err error)
	GetServiceStorage(serviceID uint32, stroageKey common.Hash) ([]byte, bool, error)
}

var jce_manual_mode = flag.Bool("jce_manual_mode", false, "jce_manual_mode")
var prereq_test = flag.Bool("prereq_test", false, "prereq_test")
var pvm_authoring_log = flag.Bool("pvm_authoring_log", false, "pvm_authoring_log")

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
	GenesisStateFile, GenesisBlockFile := GetGenesisFile(network)
	log.InitLogger("trace")

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
func testWithDispute(s string) (jam string, isDisputeMode bool) {
	const suffix = "_dispute"
	if strings.HasSuffix(s, suffix) {
		jam = strings.TrimSuffix(s, suffix)
		isDisputeMode = true
	} else {
		jam = s
		isDisputeMode = false
	}
	return
}

func jamtest(t *testing.T, jam_raw string, targetN int) {
	flag.Parse()

	JCEMode := JCEDefault
	if *jce_manual_mode {
		JCEMode = JCEManual
	}

	sendTickets := true //set this to false to run WP without E_T interference
	jam, isDisputeMode := testWithDispute(jam_raw)

	// Specify testServices
	defaultDelay := 2 * types.SecondsPerSlot * time.Second
	var serviceNames []string
	switch jam {
	case "safrole", "fallback":
		JCEMode = JCEFast
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
		serviceNames = []string{"game_of_life", "auth_copy"}
	default:
		serviceNames = []string{"auth_copy", "fib"}
	}

	basePort := GenerateRandomBasePort()
	nodes, err := SetUpNodes(JCEMode, numNodes, basePort) // TODO change this to JCEManual
	if err != nil {
		panic("Error setting up nodes: %v\n")
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

	if *prereq_test {
		test_prereq = true
	}

	fmt.Printf("Test PreReq: %v\n", test_prereq)

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
	//log.EnableModule(log.PvmAuthoring)
	//log.EnableModule(log.FirstGuarantorOrAuditor)
	//log.EnableModule(log.PvmAuthoring)

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

	testServices, err := getServices(serviceNames, true)
	if err != nil {
		t.Fatalf("GetServices %v", err)
	}

	log.Info(module, "Waiting for the first block to be ready...")
	time.Sleep(defaultDelay) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot

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
			Identifier:      fmt.Sprintf("NewService(%s, %s)", serviceName, common.Str(service.CodeHash)),
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
	if len(testServices) > 0 {
		log.Info(module, "testServices Loaded", "jam", jam, "testServices", testServices, "targetN", targetN)
	}

	switch jam {
	case "safrole":
		SafroleTestEpochLen := 4
		SafroleBufferTime := 30
		safrole(nodes[1], jceManager)
		waitForTermination(nodes[1], "safrole", SafroleTestEpochLen, SafroleBufferTime, t)
	case "fallback":
		FallbackEpochLen := 3
		FallbackBufferTime := 10
		safrole(nodes[1], jceManager)
		waitForTermination(nodes[1], "fallback", FallbackEpochLen, FallbackBufferTime, t)
	case "fib":
		fib(nodes[1], testServices, targetN, jceManager)
	case "fib2":
		targetN := 100
		fib2(nodes[1], testServices, targetN, jceManager)
	case "transfer":
		transferNum := targetN
		transfer(nodes[1], testServices, transferNum)
	case "scaled_transfer":
		transferNum := 10
		splitTransferNum := targetN
		scaled_transfer(nodes[1], testServices, transferNum, splitTransferNum)
	case "balances":
		// not using anything
		balances(nodes[1], testServices)
	case "scaled_balances":
		targetN_mint := targetN
		targetN_transfer := targetN
		scaled_balances(nodes[1], testServices, targetN_mint, targetN_transfer)
	case "blake2b":
		blake2b(nodes[1], testServices)
	case "game_of_life":
		game_of_life(nodes[1], testServices, jceManager)
	case "megatron":
		//megatron(nodes, testServices, targetN)
	}
}
