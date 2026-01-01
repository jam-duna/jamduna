//go:build network_test && (tiny || !tiny)
// +build network_test
// +build tiny !tiny

package node

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
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

func SetupQuicNetwork(network string, basePort uint16) ([]string, map[uint16]*Peer, []types.ValidatorSecret, []string, error) {
	peers := make([]string, numNodes)
	peerList := make(map[uint16]*Peer)
	nodePaths := SetLevelDBPaths(numNodes)

	// Setup QuicNetwork using test keys
	validators, validatorSecrets, err := grandpa.GenerateValidatorSecretSet(numNodes)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("Failed to setup QuicNetwork")
	}

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
		return nil, nil, nil, nil, fmt.Errorf("Failed to marshal peerList: %v, %v", err, prettyPeerList)
	}

	return peers, peerList, validatorSecrets, nodePaths, nil
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

// Create builder node with RoleBuilder
func NewBuilder(numNodes int, basePort uint16) (*Node, error) {
	dir := "/tmp/jam_builder"
	// Clean up builder directory to ensure fresh genesis state
	os.RemoveAll(dir)
	builderIndex := uint16(numNodes)

	chainSpec, err := chainspecs.ReadSpec(types.Network)
	if err != nil {
		return nil, err
	}

	// Generate a validator secret for the builder
	_, builderSecrets, err := grandpa.GenerateValidatorSecretSet(7)
	if err != nil {
		return nil, err
	}

	builderPeers := make([]string, types.TotalValidators)
	builderPeerList := make(map[uint16]*Peer)
	validators, _, err := grandpa.GenerateValidatorSecretSet(types.TotalValidators)
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < uint16(numNodes); i++ {
		addr := fmt.Sprintf("127.0.0.1:%d", basePort+i)
		builderPeers[i] = addr
		builderPeerList[i] = &Peer{
			PeerID:    i,
			PeerAddr:  addr,
			Validator: validators[i],
		}
	}
	builder, err := newNode(builderIndex, builderSecrets[builderIndex], chainSpec, *pvmBackend,
		builderPeers, builderPeerList,
		dir, int(basePort+builderIndex), JCEDefault, types.RoleBuilder)
	return builder, err
}

func SetUpNodes(jceMode string, numNodes int, basePort uint16) ([]*Node, error) {
	chainSpec, err := chainspecs.ReadSpec(types.Network)
	if err != nil {
		panic(err)
	}

	log.InitLogger("debug")
	peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork(types.Network, basePort)

	if err != nil {
		return nil, err
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		pvmBackend := statedb.BackendInterpreter
		node, err := newNode(uint16(i), validatorSecrets[i], chainSpec, pvmBackend, peers, peerList, nodePaths[i], int(basePort)+i, jceMode, types.RoleValidator)
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
		// EVM testing has been moved to builder/evm/rpc package
		// Use: go test -v -run TestEVMBlocksTransfersRPC ./builder/evm/rpc/
		t.Skip("EVM testing moved to builder/evm/rpc - run: go test -v -run TestEVMBlocksTransfersRPC ./builder/evm/rpc/")
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
