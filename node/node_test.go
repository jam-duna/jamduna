//go:build network_test && (tiny || !tiny)
// +build network_test
// +build tiny !tiny

package node

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	_ "net/http/pprof"
	"os"
	"os/user"
	"runtime"
	"strings"
	"testing"
	"time"

	chainspecs "github.com/colorfulnotion/jam/chainspecs"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/rand"
)

const (
	DefaultRefineGasLimit     = uint64(400_000_000)
	DefaultAccumulateGasLimit = uint64(4_000_000)

	webServicePort    = 8079
	runSamePvmbackend = true

	SafroleTestEpochLen = 4 // Safrole
	FallbackEpochLen    = 4 // Fallback
	EmptyEpochLen       = 10

	TargetedN_EVM = 10
)

var jce_manual = flag.Bool("jce_manual", false, "jce_manual")
var jam_node = flag.Bool("jam_node", false, "jam_node")
var jam_local_client = flag.Bool("jam_local_client", false, "jam_local_client")
var manifest = flag.Bool("manifest", false, "manifest")
var pvmBackend = flag.String("pvm_backend", statedb.BackendInterpreter, "PVM mode to use (interpreter, compiler, sandbox)")
var targetNum = flag.Int("targetN", -1, "targetN")

func generateJobID() string {
	seed := uint64(time.Now().UnixNano()) // nano seconds. but still not unique
	source := rand.NewSource(seed)
	r := rand.New(source)
	var out [8]byte
	binary.LittleEndian.PutUint64(out[:], r.Uint64())
	jobID := fmt.Sprintf("%x", out)
	return jobID
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
	/* standardize on /tmp/<user>/jam/<unixtimestamp>_<jobID>/testdb#
	Example: /tmp/root/jam/1727903082_<jID>/node1/
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
		pvmBackend := statedb.BackendInterpreter
		node, err := newNode(uint16(i), validatorSecrets[i], chainSpec, pvmBackend, epoch0Timestamp, peers, peerList, nodePaths[i], int(basePort)+i, jceMode)
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
		fileName := fmt.Sprintf("/services/%s/%s.pvm", serviceName, serviceName)

		var code []byte
		if getmetadata {
			code, _ = types.ReadCodeWithMetadata(fileName, serviceName)
		} else {
			filePath, err := common.GetFilePath(fileName)
			if err != nil {
				log.Error(log.Node, "Failed to get file path", "fileName", fileName, "err", err)
				continue
			}
			code, _ = os.ReadFile(filePath)
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

// run any test with dispute using testName_dispute (i.e majik_dispute)
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
	case "algo":
		serviceNames = []string{"algo", "auth_copy"}
	case "fib":
		serviceNames = []string{"fib", "auth_copy"}
	default:
		serviceNames = []string{"evm", "auth_copy"}
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
		// only "node" can set pvm backend
		fmt.Printf("jamtest: %s-node\n", jam)
		basePort := GenerateRandomBasePort()
		basePort = 40000

		JCEMode := JCEDefault
		// if *jce_manual {
		// 	JCEMode = JCEManual
		// }
		JCEMode = JCEDefault
		fmt.Printf("jamtest: JCEMode=%s\n", JCEMode)

		nodes, err := SetUpNodes(JCEMode, numNodes, basePort)
		if err != nil {
			log.Crit(log.Node, "Error setting up nodes", "err", err)
			return
		}

		for i := 0; i < numNodes; i++ {
			if runSamePvmbackend {
				nodes[i].SetPVMBackend(*pvmBackend)
			} else {
				if i%2 == 1 {
					if runtime.GOOS == "linux" {
						compilerBackend := statedb.BackendCompiler
						pvmBackend = &compilerBackend
					}
				}
			}
		}

		// Handling Safrole
		sendTickets = true
		for _, n := range nodes {
			if sendTickets {
				n.SetSendTickets(sendTickets)
			}
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
	// log.EnableModule(log.PvmAuthoring)
	log.EnableModule(log.GeneralAuthoring)

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
	log.Info(log.Node, "JAMTEST", "jam", jam, "targetN", targetN)

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
	case "evm":
		evm(bNode, testServices, 5)
	case "algo":
		algo(bNode, testServices, targetN)
	case "fib":
		fib(bNode, testServices, targetN)
	default:
		t.Fatalf("Unknown jam test: %s\n", jam)
	}
}

func safrole(jceManager *ManualJCEManager) {
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
			return false, fmt.Errorf("[TIMEOUT] H_t=%v e'=%v,m'=%v | missing %v Slot!",
				currTimeSlot, currEpoch, currPhase, currTimeSlot-initialTimeSlot)
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
		log.Info(log.Node, "Completed")
	case err := <-errChan:
		t.Fatalf("[%v] Failed: %v", caseType, err)
	}
}

func evm(n1 JNode, testServices map[string]*types.TestService, targetN int) {

	evm_serviceIdx := uint32(statedb.EVMServiceCode)

	// Set USDM initial balances and nonces
	blobs := types.ExtrinsicsBlobs{}
	workItems := []types.WorkItemExtrinsic{}
	totalSupplyValue := new(big.Int).Mul(big.NewInt(61_000_000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	usdmInitialState := []statedb.MappingEntry{
		{Slot: 0, Key: evmtypes.IssuerAddress, Value: totalSupplyValue}, // balanceOf[issuer]
		{Slot: 1, Key: evmtypes.IssuerAddress, Value: big.NewInt(1)},    // nonces[issuer]
	}
	statedb.InitializeMappings(&blobs, &workItems, evmtypes.UsdmAddress, usdmInitialState)

	numExtrinsics := len(workItems)

	log.Info(log.Node, "EVM START (genesis)", "evm", evm_serviceIdx, "numExtrinsics", numExtrinsics, "targetN", targetN)

	service, ok, err := n1.GetService(evm_serviceIdx)
	if err != nil || !ok {
		panic(fmt.Sprintf("EVM service not found: %d %v", evm_serviceIdx, err))
	}

	// Create work package with updated witness count and refine context
	wp := statedb.DefaultWorkPackage(evm_serviceIdx, service)

	// Get RefineContext and log what we get
	refineCtx, err := n1.GetRefineContext()
	if err != nil {
		log.Error(log.Node, "GetRefineContext failed", "err", err)
	}

	globalDepth := uint8(0)

	wp.RefineContext = refineCtx
	wp.WorkItems[0].Payload = statedb.BuildPayload(statedb.PayloadTypeGenesis, numExtrinsics, globalDepth, 0, common.Hash{})
	wp.WorkItems[0].Extrinsics = workItems

	// Genesis adds 3 exports: 1 StorageShard + 1 SSRMetadata + 1 meta for USDM contract
	wp.WorkItems[0].ExportCount = uint16(3)
	wpr := &types.WorkPackageBundle{
		WorkPackage:   wp,
		ExtrinsicData: []types.ExtrinsicsBlobs{blobs},
	}
	ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
	_, err = RobustSubmitAndWaitForWorkPackageBundles(ctx, n1, []*types.WorkPackageBundle{wpr})
	cancel()
	if err != nil {
		log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
		return
	}

	// Create signed transactions

	for evmN := 0; evmN < 30; evmN++ {
		nonce, _ := n1.GetTransactionCount(evmtypes.IssuerAddress, "latest")
		log.Info(log.Node, "GetTransactionCount", "nonce", nonce)

		globalDepth, err := n1.ReadGlobalDepth(evm_serviceIdx)
		if err != nil {
			globalDepth = 0
		}

		// Build transaction extrinsics
		numTxs := 3
		txBytes := make([][]byte, numTxs)
		txHashes := make([]common.Hash, numTxs)

		// Create simple transfers from account 0 to account 1
		senderAddress, senderPrivKey := common.GetEVMDevAccount(0)

		for i := 0; i < numTxs; i++ {
			recipientIndex := (evmN*numTxs + i + 1) % 10
			if recipientIndex == 0 {
				recipientIndex = 1
			}
			recipientAddr, _ := common.GetEVMDevAccount(recipientIndex)
			gasPrice := big.NewInt(1) // 1 wei
			gasLimit := uint64(2_000_000)
			amount := big.NewInt(int64(1000*i + 20000)) // 1 wei transfer

			_, tx, txHash, err := statedb.CreateSignedUSDMTransfer(
				senderPrivKey,
				nonce+uint64(i),
				recipientAddr,
				amount,
				gasPrice,
				gasLimit,
				uint64(statedb.DefaultJAMChainID),
			)
			if err != nil {
				log.Error(log.Node, "CreateSignedUSDMTransfer ERR", "err", err)
				return
			}

			txBytes[i] = tx
			txHashes[i] = txHash
		}

		// Build extrinsics blobs and hashes
		blobs := make(types.ExtrinsicsBlobs, numTxs)
		extrinsics := make([]types.WorkItemExtrinsic, numTxs)
		for i, tx := range txBytes {
			blobs[i] = tx
			extrinsics[i] = types.WorkItemExtrinsic{
				Hash: common.Blake2Hash(tx),
				Len:  uint32(len(tx)),
			}
		}

		wp := statedb.DefaultWorkPackage(evm_serviceIdx, service)
		wp.WorkItems[0].Payload = statedb.BuildPayload(statedb.PayloadTypeBuilder, numTxs, globalDepth, 0, common.Hash{})
		wp.WorkItems[0].Extrinsics = extrinsics

		//  BuildBundle should return a Bundle (with ImportedSegments)
		bundle, _, err := n1.BuildBundle(wp, []types.ExtrinsicsBlobs{blobs}, uint16(0), []common.Hash{})
		if err != nil {
			log.Error(log.Node, "BuildBundle ERR", "err", err)

		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			n1.SubmitAndWaitForWorkPackageBundle(ctx, bundle)
			block, err := n1.GetBlockByNumber("latest", true)
			if err != nil {
				log.Error(log.Node, "GetBlockByNumber ERR", "err", err)
			} else {
				log.Info(log.Node, "EVM Block", "number", types.ToJSON(block))
			}
			balance, err := n1.GetBalance(senderAddress, "latest")
			if err != nil {
				log.Error(log.Node, "GetBalance ERR", "err", err)
			} else {
				log.Info(log.Node, "Sender Balance", "address", senderAddress.Hex(), "balance", balance)
			}
			// if txHashes, ok := block.Transactions.([]string); ok {
			// 	for _, txHashStr := range txHashes {
			// 		txHash := common.HexToHash(txHashStr)
			// 		receipt, err := n1.GetTransactionReceipt(txHash)
			// 		if err != nil {
			// 			log.Error(log.Node, "GetTransactionReceipt ERR", "err", err)
			// 		} else {
			// 			log.Info(log.Node, "EVM Tx Receipt", "txHash", txHash.Hex(), "receipt", types.ToJSON(receipt))
			// 		}
			// 	}
			// }
		}
	}
}

func TestFallback(t *testing.T) {
	jamtest(t, "fallback", 0)
}

func TestSafrole(t *testing.T) {
	jamtest(t, "safrole", 0)
}

func TestEVM(t *testing.T) {
	targetN := TargetedN_EVM
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "evm", targetN)
}

func TestAuthCopy(t *testing.T) {
	targetN := TargetedN_EVM
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "auth_copy", targetN)
}

func TestAlgo(t *testing.T) {
	targetN := TargetedN_EVM
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "algo", targetN)
}

func TestFib(t *testing.T) {
	targetN := TargetedN_EVM
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "fib", targetN)
}
