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

	"github.com/colorfulnotion/jam/chainspecs"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/rand"

	_ "net/http/pprof"
	"os"
	"os/user"
	"time"
)

const (
	DefaultRefineGasLimit     = uint64(8000000)
	DefaultAccumulateGasLimit = uint64(1000000)
)

var jce_manual = flag.Bool("jce_manual", false, "jce_manual")
var jam_node = flag.Bool("jam_node", false, "jam_node")
var jam_local_client = flag.Bool("jam_local_client", false, "jam_local_client")
var manifest = flag.Bool("manifest", false, "manifest")
var pvmBackend = flag.String("pvm_backend", "interpreter", "PVM mode to use (interpreter, recompiler, sandbox)")

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
	chainSpec, err := chainspecs.ReadSpec(types.Network)
	if err != nil {
		panic(err)
	}

	log.InitLogger("debug")

	epoch0Timestamp, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork(types.Network, basePort)

	if err != nil {
		return nil, err
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], chainSpec, epoch0Timestamp, peers, peerList, nodePaths[i], int(basePort)+i, jceMode)
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
		log.Info(log.Node, serviceName, "codeHash", codeHash, "len", len(code))
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

	// SETUP DEBUG LOGGING with EnableModules
	// EnableModules is a comma-separated list of modules to enable for debug logging
	// For example, to enable DEBUG logging for rotation, guarantees, node, state, quic, preimage:
	// "rotation,guarantees,node,state,quic,preimage"
	log.InitLogger("debug")
	debug := "" // guarantees
	log.EnableModules(debug)

	// Specify testServices
	targetedFinalityDelay := 0 // TODO: write key to levelDB for starting time slot
	defaultDelay := time.Duration(targetedFinalityDelay*types.SecondsPerSlot) * time.Second
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
	case "rubic":
		serviceNames = []string{"fib", "auth_copy"}
	case "fib":
		serviceNames = []string{"fib", "auth_copy"}
	case "fib2":
		serviceNames = []string{"corevm", "auth_copy"}
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
	case "all":
		serviceNames = []string{"fib", "tribonacci", "megatron", "auth_copy", "game_of_life"}
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
		bNode, err = NewNodeClient(tcpServers, wsUrl)
		if err != nil {
			fmt.Printf("NewNodeClient ERR %v\n", err)
			err = fmt.Errorf("‼️ jamtest: %s-client (local=%v) Failed. Connection Problem?\n", jam, *jam_local_client)
			t.Fatalf("%v", err)
		}
		fmt.Printf("%s tcp:%v\n", wsUrl, tcpServers)
		client := bNode.(*NodeClient)
		err = client.ConnectWebSocket(wsUrl)
	} else { // Node
		// onlt "node" can set pvm backend
		pvm.Set_PVM_Backend(*pvmBackend)
		fmt.Printf("[%v] Mode\n", pvm.VM_MODE)
		fmt.Printf("jamtest: %s-node\n", jam)
		basePort := GenerateRandomBasePort()
		basePort = 40000

		JCEMode := JCEDefault
		if *jce_manual {
			JCEMode = JCEManual
		}
		if jam == "safrole" || jam == "fallback" {
			JCEMode = JCEFast
		}
		JCEMode = JCEFast //MK:TODO. bring back faster mode. This seems extremely slow

		nodes, err := SetUpNodes(JCEMode, numNodes, basePort)
		if err != nil {
			log.Crit(log.Node, "Error setting up nodes", "err", err)
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
		log.Info(log.Node, "JAMTEST", "jam", jam, "targetN", targetN)

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

	//log.Enablelog.Node(log.FirstGuarantorOrAuditor)
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule(log.GeneralAuthoring)

	bootstrapCode, err := types.ReadCodeWithMetadata(statedb.BootstrapServiceFile, "bootstrap")
	if err != nil {
		log.Error(log.Node, "ReadCodeWithMetadata", "err", err)
		return
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)

	testServices, err := getServices(serviceNames, true)
	if err != nil {
		t.Fatalf("GetServices %v", err)
	}

	log.Info(log.Node, "Waiting for the first block to be ready...")
	isReady := false
	for !isReady {
		finalizedBlock, _ := bNode.GetFinalizedBlock()
		if finalizedBlock != nil && finalizedBlock.Header.Slot > 0 {
			isReady = true
			log.Info(log.Node, "JAMTEST READY", "Last Finalized", finalizedBlock.Header.Slot, "HeaderHash", finalizedBlock.Header.Hash())
			break
		}
		time.Sleep(3 * time.Second)
	}

	time.Sleep(defaultDelay) // this delay is necessary to ensure the first block is ready, nor it will send the wrong anchor slot

	var jceManager *ManualJCEManager
	jceManager = nil
	var previous_service_idx uint32
	requireNew := jam == "fib"
	var bootstrap_workItems []types.WorkItem
	log.Info(log.Node, "JAMTEST", "jam", jam, "targetN", targetN, "requireNew", requireNew)

	serviceList := []string{}
	serviceCodeList := make([][]byte, 0)
	serviceCodeHashList := []string{}
	for serviceName, service := range testServices {
		if serviceName == "auth_copy" {
			service.ServiceCode = statedb.AuthCopyServiceCode
			continue
		}
		if serviceName == "fib" {
			service.ServiceCode = statedb.FibServiceCode
			//continue
		}
		workItem := types.WorkItem{
			Service:            bootstrapService,
			CodeHash:           bootstrapCodeHash,
			Payload:            append(service.CodeHash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(service.Code)))...),
			RefineGasLimit:     DefaultRefineGasLimit,
			AccumulateGasLimit: DefaultAccumulateGasLimit,
			ImportedSegments:   make([]types.ImportSegment, 0),
			ExportCount:        0,
		}
		bootstrap_workItems = append(bootstrap_workItems, workItem)
		serviceList = append(serviceList, serviceName)
		serviceCodeList = append(serviceCodeList, service.Code)
		serviceCodeHashList = append(serviceCodeHashList, fmt.Sprintf("%s:%s", serviceName, common.Str(service.CodeHash)))
	}
	fmt.Printf("Services to be loaded: %s\n", serviceCodeHashList)

	if requireNew {
		// set up service using the Bootstrap service
		codeWorkPackage := types.WorkPackage{
			Authorization:         []byte(""),
			AuthCodeHost:          bootstrapService,
			AuthorizationCodeHash: bootstrap_auth_codehash,
			ParameterizationBlob:  []byte{},
			WorkItems:             bootstrap_workItems,
		}
		ctx, cancel := context.WithTimeout(context.Background(), RefineAndAccumalateTimeout)
		defer cancel()
		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("NewServices(%s)", serviceList),
			WorkPackage:     codeWorkPackage,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		_, err = RobustSubmitAndWaitForWorkPackages(ctx, bNode, []*WorkPackageRequest{wpr})
		if err != nil {
			t.Fatalf("SendWorkPackageSubmission ERR %v\n", err)
		}

		// batch service submission done, now check if the service was set up correctly
		serviceIDList := make([]uint32, len(serviceList))
		for workItemIdx, serverName := range serviceList {
			workItemKey, _ := types.Encode(uint32(workItemIdx))
			k := common.ServiceStorageKey(bootstrapService, workItemKey)
			k_service_account_byte, ok, err := bNode.GetServiceStorage(0, k)
			if err != nil {
				t.Fatalf("SendWorkPackageSubmission ERR %v", err)
			}
			if !ok {
				t.Fatalf("SendWorkPackageSubmission NOT OK %v", err)
			}
			decoded_new_service_idx := uint32(types.DecodeE_l(k_service_account_byte))
			if decoded_new_service_idx != 0 && (decoded_new_service_idx != previous_service_idx) {
				testServices[serverName].ServiceCode = decoded_new_service_idx
				serviceIDList[workItemIdx] = decoded_new_service_idx
				fmt.Printf("Service %s created=%d\n", serverName, decoded_new_service_idx)
			}
		}

		for workItemIdx, serviceID := range serviceIDList {
			serviceName := serviceList[workItemIdx]
			preimageBlob := serviceCodeList[workItemIdx]
			codeHash := common.Blake2Hash(preimageBlob)
			err = bNode.SubmitAndWaitForPreimage(ctx, serviceID, preimageBlob)
			if err != nil {
				log.Error(log.Node, "SubmitAndWaitForPreimage", "err", err)
			}
			log.Info(log.Node, "----- NEW SERVICE", "service", serviceName, "service_idx", serviceID, "codeHash", codeHash, "codeLen", len(preimageBlob))
		}
	}

	if len(testServices) > 0 {
		log.Info(log.Node, "testServices Loaded", "jam", jam, "testServices", testServices, "targetN", targetN)
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
	case "rubic":
		rubic(bNode, testServices, targetN)
	case "fib2":
		//targetN := 100
		fib2(bNode, testServices, 10)
	case "game_of_life":
		//targetN := 20
		game_of_life(bNode, testServices, targetN, *manifest)
	case "megatron":
		megatron(bNode, testServices, 5)
	case "auth_copy":
		reassign(bNode, testServices)

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
