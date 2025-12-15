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
	grandpa "github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	stateStorage "github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/ethereum/go-verkle"
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

	TargetedN_EVM = 30
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
	validators, validatorSecrets, err := grandpa.GenerateValidatorSecretSet(numNodes)
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
			//Code:        code,
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
	finalizationMode := "None"
	if Grandpa {
		finalizationMode = "Grandpa"
	} else if GrandpaEasy {
		finalizationMode = "GrandpaEasy"
	}
	if !Grandpa && !GrandpaEasy {
		t.Fatalf("Either Grandpa or GrandpaEasy must be enabled for block finalization. Grandpa=%v, GrandpaEasy=%v", Grandpa, GrandpaEasy)
	}
	if Grandpa && GrandpaEasy {
		t.Fatalf("Grandpa and GrandpaEasy are mutually exclusive - only one can be enabled. Grandpa=%v, GrandpaEasy=%v", Grandpa, GrandpaEasy)
	}

	sendTickets := false //set this to false to run WP without E_T interference
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
	var nodes []*Node
	var err error

	if !*jam_node { // NodeClient -- client scenario (could be dot-0 OR localhost:____ )
		if isDisputeMode {
			t.Fatalf("Dispute mode is not supported for client test\n")
		}
		if jam == "safrole" || jam == "fallback" {
			t.Logf("Nothing to test for %s-client\n", jam)
			return
		}

		fmt.Printf("jamtest: %s-client (local=%v) finalization=%s\n", jam, *jam_local_client, finalizationMode)
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
		finalizationMode := "none"
		if Grandpa {
			finalizationMode = "grandpa"
		} else if GrandpaEasy {
			finalizationMode = "grandpaeasy"
		}
		fmt.Printf("jamtest: %s-node finalization=%s\n", jam, finalizationMode)
		basePort := GenerateRandomBasePort()
		basePort = 40000

		JCEMode := JCEDefault
		// if *jce_manual {
		// 	JCEMode = JCEManual
		// }
		JCEMode = JCEDefault
		fmt.Printf("jamtest: JCEMode=%s\n", JCEMode)

		nodes, err = SetUpNodes(JCEMode, numNodes, basePort)
		if err != nil {
			log.Crit(log.Node, "Error setting up nodes", "err", err)
			panic(err)
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

		// Handling Safrole/Fallback ticket broadcasting
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
		SafroleBufferTime := 10
		safrole(jceManager)
		if tNode == nil {
			t.Fatalf("tNode is nil")
		}
		waitForTermination(tNode, "safrole", SafroleTestEpochLen, SafroleBufferTime, t)
	case "fallback":
		FallbackEpochLen := 4
		FallbackBufferTime := 10
		safrole(jceManager)
		waitForTermination(tNode, "fallback", FallbackEpochLen, FallbackBufferTime, t)
	case "evm":
		if *jam_node {
			evm(bNode, nodes, testServices, 30)
		}
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

// primeEVMGenesis injects the issuer account and a block-0 snapshot into a node's storage
// without re-running the full chain genesis.
func primeEVMGenesis(storage *stateStorage.StateDBStorage, serviceID uint32, startBalance int64) error {
	// Ensure we have a working tree
	tree := storage.CurrentVerkleTree
	if tree == nil {
		tree = verkle.New()
		storage.CurrentVerkleTree = tree
	}

	issuerAddress, _ := common.GetEVMDevAccount(0)
	balanceWei := new(big.Int).Mul(big.NewInt(startBalance), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

	var basicData [32]byte
	// nonce = 1 per EIP-6800 layout
	binary.BigEndian.PutUint64(basicData[8:16], uint64(1))
	// balance at offset 16-31
	balanceBytes := balanceWei.Bytes()
	copy(basicData[32-len(balanceBytes):], balanceBytes)

	if err := tree.Insert(stateStorage.BasicDataKey(issuerAddress[:]), basicData[:], nil); err != nil {
		return fmt.Errorf("primeEVMGenesis: insert basic data failed: %w", err)
	}

	root := tree.Commit()
	rootBytes := root.Bytes()
	rootHash := common.BytesToHash(rootBytes[:])
	storage.StoreVerkleTree(rootHash, tree)

	genesisBlock := &evmtypes.EvmBlockPayload{
		Number:              0,
		WorkPackageHash:     common.Hash{},
		SegmentRoot:         common.Hash{},
		PayloadLength:       148,
		NumTransactions:     0,
		Timestamp:           0,
		GasUsed:             0,
		VerkleRoot:          rootHash,
		TransactionsRoot:    common.Hash{},
		ReceiptRoot:         common.Hash{},
		BlockAccessListHash: common.Hash{},
		TxHashes:            []common.Hash{},
		ReceiptHashes:       []common.Hash{},
		Transactions:        []evmtypes.TransactionReceipt{},
	}

	if err := storage.StoreServiceBlock(serviceID, genesisBlock, common.Hash{}, 0); err != nil {
		return fmt.Errorf("primeEVMGenesis: store service block failed: %w", err)
	}

	return nil
}

func evm(n1 JNode, nodes []*Node, testServices map[string]*types.TestService, targetN int) {

	evm_serviceIdx := uint32(statedb.EVMServiceCode)

	// Get the node to access its rollup instances
	node, ok := n1.(*Node)
	if !ok {
		log.Error(log.Node, "n1 is not a *Node")
		return
	}

	numNodes := len(nodes)

	// Prime EVM issuer balance/nonce for service 0 without resetting JAM state
	log.Info(log.Node, "Priming EVM genesis on all nodes", "numNodes", numNodes)
	for i, testNode := range nodes {
		storageIface, err := testNode.GetStorage()
		if err != nil {
			log.Error(log.Node, "GetStorage failed", "nodeIdx", i, "err", err)
			return
		}
		sdb, ok := storageIface.(*stateStorage.StateDBStorage)
		if !ok {
			log.Error(log.Node, "Storage type assertion failed", "nodeIdx", i, "type", fmt.Sprintf("%T", storageIface))
			return
		}

		if err := primeEVMGenesis(sdb, evm_serviceIdx, 61_000_000); err != nil {
			log.Error(log.Node, "primeEVMGenesis failed", "nodeIdx", i, "err", err)
			return
		}

		// Create Rollup instance and add to node's rollups map
		rollup, err := statedb.NewRollup(storageIface, evm_serviceIdx, testNode)
		if err != nil {
			log.Error(log.Node, "NewRollup failed", "nodeIdx", i, "err", err)
			return
		}
		testNode.rollupsMutex.Lock()
		testNode.rollups[evm_serviceIdx] = rollup
		testNode.rollupsMutex.Unlock()
	}

	// Track balances across iterations
	const numAccounts = 11 // 10 dev accounts + 1 coinbase
	prevBalances := make([]*big.Int, numAccounts)

	// Coinbase address receives all transaction fees
	coinbaseAddress := common.HexToAddress("0xEaf3223589Ed19bcd171875AC1D0F99D31A5969c")

	// Get all account addresses
	accounts := make([]common.Address, numAccounts)
	for i := 0; i < 10; i++ {
		accounts[i], _ = common.GetEVMDevAccount(i)
	}
	accounts[10] = coinbaseAddress

	// Initialize previous balances from genesis state
	rollup := node.rollups[evm_serviceIdx]
	for i := 0; i < numAccounts; i++ {
		balanceHash, err := rollup.GetBalance(accounts[i], "latest")
		if err != nil {
			log.Warn(log.Node, "Failed to get initial balance", "account", i, "err", err)
			prevBalances[i] = big.NewInt(0)
		} else {
			prevBalances[i] = new(big.Int).SetBytes(balanceHash[:])
		}
	}

	// Create and submit EVM transactions across multiple iterations
	for evmN := 0; evmN < targetN; evmN++ {
		// Use sequential nonces starting from 0
		nonce := uint64(evmN * 3) // Each iteration creates 3 transactions

		// Build transaction extrinsics
		numTxs := 3
		txBytes := make([][]byte, numTxs)
		txHashes := make([]common.Hash, numTxs)

		if evmN < 3 {

			// Create simple transfers from account 0 to other accounts
			_, senderPrivKey := common.GetEVMDevAccount(0)

			for i := 0; i < numTxs; i++ {
				recipientIndex := (evmN*numTxs + i + 1) % 10
				if recipientIndex == 0 {
					recipientIndex = 1
				}
				recipientAddr, _ := common.GetEVMDevAccount(recipientIndex)
				gasPrice := big.NewInt(1) // 1 wei
				gasLimit := uint64(12_000_000)
				amount := big.NewInt(int64(10000000*i + 200000000)) // variable amount

				_, tx, txHash, err := statedb.CreateSignedNativeTransfer(
					senderPrivKey,
					nonce+uint64(i),
					recipientAddr,
					amount,
					gasPrice,
					gasLimit,
					uint64(statedb.DefaultJAMChainID),
				)
				if err != nil {
					log.Error(log.Node, "CreateSignedNativeTransfer ERR", "err", err)
					return
				}

				txBytes[i] = tx
				txHashes[i] = txHash
			}
		} else {
			// Random transactions between dev accounts, ensuring its not the same account
			// and that the amount does not exceed 50% of available balance (based on prevBalances)

			// Track nonces per sender for this iteration
			senderNonces := make(map[int]uint64)

			for i := 0; i < numTxs; i++ {
				// Select random sender from accounts 0-9 that has sufficient balance
				var senderIndex int
				var senderBalance *big.Int
				maxAttempts := 50
				for attempt := 0; attempt < maxAttempts; attempt++ {
					senderIndex = rand.Intn(10)
					senderBalance = prevBalances[senderIndex]

					// Check if sender has enough balance (need at least for gas + some amount)
					minRequired := big.NewInt(25_000_000) // minimum for gas + transfer
					if senderBalance.Cmp(minRequired) > 0 {
						break
					}

					if attempt == maxAttempts-1 {
						log.Error(log.Node, "Could not find sender with sufficient balance", "iteration", evmN, "tx", i)
						return
					}
				}

				// Select random recipient (different from sender)
				recipientIndex := rand.Intn(10)
				for recipientIndex == senderIndex {
					recipientIndex = rand.Intn(10)
				}

				// Calculate max transferable amount (50% of balance minus gas costs)
				gasPrice := big.NewInt(1)
				gasLimit := uint64(12_000_000)
				maxGasCost := new(big.Int).Mul(gasPrice, big.NewInt(int64(gasLimit)))

				maxTransfer := new(big.Int).Div(senderBalance, big.NewInt(2))
				maxTransfer.Sub(maxTransfer, maxGasCost)

				// Ensure maxTransfer is positive
				if maxTransfer.Cmp(big.NewInt(1000)) < 0 {
					maxTransfer = big.NewInt(1000)
				}

				// Random amount between 1000 and maxTransfer
				randomAmount := big.NewInt(0)
				if maxTransfer.Cmp(big.NewInt(10000)) > 0 {
					// Use a random percentage of maxTransfer (10% to 90%)
					percentage := 10 + rand.Intn(80)
					randomAmount.Mul(maxTransfer, big.NewInt(int64(percentage)))
					randomAmount.Div(randomAmount, big.NewInt(100))
				} else {
					randomAmount.Set(maxTransfer)
				}

				// Get sender's nonce
				var currentNonce uint64
				if nonce, exists := senderNonces[senderIndex]; exists {
					currentNonce = nonce
				} else {
					// First tx from this sender in this iteration - get from rollup
					addr, _ := common.GetEVMDevAccount(senderIndex)
					nonceFromChain, err := rollup.GetTransactionCount(addr, "latest")
					if err != nil {
						log.Error(log.Node, "GetTransactionCount ERR", "account", senderIndex, "err", err)
						return
					}
					currentNonce = nonceFromChain
				}
				senderNonces[senderIndex] = currentNonce + 1

				// Create the transfer
				senderAddr, senderPrivKey := common.GetEVMDevAccount(senderIndex)
				recipientAddr, _ := common.GetEVMDevAccount(recipientIndex)

				_, tx, txHash, err := statedb.CreateSignedNativeTransfer(
					senderPrivKey,
					currentNonce,
					recipientAddr,
					randomAmount,
					gasPrice,
					gasLimit,
					uint64(statedb.DefaultJAMChainID),
				)
				if err != nil {
					log.Error(log.Node, "CreateSignedNativeTransfer ERR", "err", err)
					return
				}

				log.Info(log.Node, "📤 Random transfer",
					"iteration", evmN,
					"tx", i,
					"from", fmt.Sprintf("Account[%d](%s)", senderIndex, senderAddr.Hex()),
					"to", fmt.Sprintf("Account[%d](%s)", recipientIndex, recipientAddr.Hex()),
					"amount", randomAmount.String(),
					"nonce", currentNonce,
				)

				txBytes[i] = tx
				txHashes[i] = txHash
			}
		}

		service, ok, err := node.GetService(evm_serviceIdx)
		if err != nil || !ok {
			log.Error(log.Node, "GetService failed", "err", err)
			return
		}

		// Create extrinsics blobs and hashes
		blobs := make(types.ExtrinsicsBlobs, numTxs)
		extrinsics := make([]types.WorkItemExtrinsic, numTxs)
		for i, tx := range txBytes {
			blobs[i] = tx
			extrinsics[i] = types.WorkItemExtrinsic{
				Hash: common.Blake2Hash(tx),
				Len:  uint32(len(tx)),
			}
		}

		// Create work package
		wp := statedb.DefaultWorkPackage(evm_serviceIdx, service)
		refineContext, err := node.GetRefineContext()
		if err != nil {
			log.Error(log.Node, "GetRefineContext failed", "err", err)
			return
		}
		wp.RefineContext = refineContext
		globalDepth := uint8(0)
		wp.WorkItems[0].Extrinsics = extrinsics
		wp.WorkItems[0].Payload = statedb.BuildPayload(statedb.PayloadTypeBuilder, len(txBytes), globalDepth, 0, common.Hash{})

		// Build bundle using node's BuildBundle method
		bundle, _, err := node.BuildBundle(wp, []types.ExtrinsicsBlobs{blobs}, 0, nil)
		if err != nil {
			log.Error(log.Node, "BuildBundle failed", "err", err)
			continue
		}
		bundles := []*types.WorkPackageBundle{bundle}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		stateRoots, timeslots, err := RobustSubmitAndWaitForWorkPackageBundles(ctx, n1, bundles)
		if err != nil {
			log.Error(log.Node, "⚠️ SubmitAndWaitForWorkPackages failed, continuing to next iteration", "err", err, "evmN", evmN)
			continue
		}
		log.Info(log.Node, "✅ Submitted and accumulated EVM tx bundle", "evmN", evmN, "stateRoot", stateRoots[0], "timeslot", timeslots[0])

		// Show the balance of all accounts after each iteration, sum them and show the deltas from previous round
		currentBalances := make([]*big.Int, numAccounts)
		totalBalance := big.NewInt(0)
		totalDelta := big.NewInt(0)

		log.Info(log.Node, "Account balances after iteration", "iteration", evmN)
		for i := 0; i < numAccounts; i++ {
			balanceHash, err := rollup.GetBalance(accounts[i], "latest")
			if err != nil {
				log.Warn(log.Node, "Failed to get balance", "account", i, "err", err)
				currentBalances[i] = big.NewInt(0)
			} else {
				currentBalances[i] = new(big.Int).SetBytes(balanceHash[:])
			}

			delta := new(big.Int).Sub(currentBalances[i], prevBalances[i])
			totalBalance.Add(totalBalance, currentBalances[i])
			totalDelta.Add(totalDelta, delta)

			if i == 10 {
				log.Info(log.Node, "  Coinbase", "address", accounts[i].Hex(), "balance", currentBalances[i], "delta", delta)
			} else {
				log.Info(log.Node, "  Account", "idx", i, "address", accounts[i].Hex(), "balance", currentBalances[i], "delta", delta)
			}
		}

		log.Info(log.Node, "Total balance", "sum", totalBalance, "delta", totalDelta)

		// Verify total balance matches genesis amount (61M with 18 decimals)
		expectedTotal := new(big.Int).Mul(big.NewInt(61_000_000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		if totalBalance.Cmp(expectedTotal) != 0 {
			log.Error(log.Node, "❌ Total balance mismatch - money supply violation!",
				"iteration", evmN,
				"expected", expectedTotal.String(),
				"actual", totalBalance.String(),
				"diff", new(big.Int).Sub(totalBalance, expectedTotal).String())
			return
		}

		// Update previous balances for next iteration
		prevBalances = currentBalances

	}
	log.Info(log.Node, "✅ EVM test completed successfully", "iterations", targetN)
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
