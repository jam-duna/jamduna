package node

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

func generateSeedSet(ringSize int) ([][]byte, error) {

	ringSet := make([][]byte, ringSize)
	for i := 0; i < ringSize; i++ {
		seed := make([]byte, 32)
		idxBytes := big.NewInt(int64(i)).Bytes()
		copy(seed[32-len(idxBytes):], idxBytes)
		ringSet[i] = seed
	}

	/*
		entropy := common.Blake2Hash([]byte("42"))
		Generate the ring set with deterministic random seeds
		for i := 0; i < ringSize; i++ {
			seed := make([]byte, 32)
			if _, err := rand.Read(entropy.Bytes()); err != nil {
				return nil, err
			}
			// XOR the deterministic seed with the random seed to make it deterministic
			for j := range seed {
				seed[j] ^= entropy[j%len(entropy)]
			}
			ringSet[i] = common.Blake2Hash(append(seed[:], byte(i))).Bytes()
		}
	*/
	return ringSet, nil
}

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

func SetupQuicNetwork() (statedb.GenesisConfig, []string, map[uint16]*Peer, []types.ValidatorSecret, []string, error) {
	seeds, _ := generateSeedSet(numNodes)

	peers := make([]string, numNodes)
	peerList := make(map[uint16]*Peer)
	validators := make([]types.Validator, numNodes)
	nodePaths := SetLevelDBPaths(numNodes)
	for i := 0; i < numNodes; i++ {

		seed_i := seeds[i]
		bandersnatch_seed := seed_i
		ed25519_seed := seed_i
		bls_seed := seed_i
		metadata, _ := generateMetadata(i)

		validator, err := statedb.InitValidator(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err == nil {
			validators[i] = validator
		} else {
			return statedb.GenesisConfig{}, nil, nil, nil, nil, fmt.Errorf("Failed to init validator %d: %v", i, err)
		}
	}

	genesisConfig := statedb.NewGenesisConfig(validators)
	//genesisConfig.SaveToFile("../genesis.json")

	//prettyJSON, _ := json.MarshalIndent(validators, "", "  ")
	//fmt.Printf("Validators (size:%v) %s\n", numNodes, prettyJSON)

	for i := uint16(0); i < numNodes; i++ {
		addr := fmt.Sprintf(quicAddr, 9000+i)
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
		return statedb.GenesisConfig{}, nil, nil, nil, nil, fmt.Errorf("Failed to marshal peerList: %v, %v", err, prettyPeerList)
	}
	//fmt.Printf("PeerList: %s\n", prettyPeerList)

	// Compute validator secrets
	validatorSecrets := make([]types.ValidatorSecret, numNodes)
	for i := 0; i < numNodes; i++ {
		seed_i := seeds[i]
		bandersnatch_seed := seed_i
		ed25519_seed := seed_i
		bls_seed := seed_i
		metadata, _ := generateMetadata(i)
		//bandersnatch_seed, ed25519_seed, bls_seed
		validatorSecret, err := statedb.InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return statedb.GenesisConfig{}, nil, nil, nil, nil, fmt.Errorf("Failed to Generate secrets %v", err)
		}
		validatorSecrets[i] = validatorSecret
	}
	return genesisConfig, peers, peerList, validatorSecrets, nodePaths, nil
}

func TestNodeSafrole(t *testing.T) {
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error Seeting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], &genesisConfig, peers, peerList, ValidatorFlag, nodePaths[i], basePort+i)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		//node.state = statedb.ProcessGenesis(genesisAuthorities)
		nodes[i] = node
	}
	statedb.RunGraph()
	for {
	}
}

func TestWorkGuarantee(t *testing.T) {
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error Seeting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], &genesisConfig, peers, peerList, ValidatorFlag, nodePaths[i], basePort+i)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		//node.state = statedb.ProcessGenesis(genesisAuthorities)
		nodes[i] = node
	}

	// give some time for nodes to come up
	for {
		time.Sleep(1 * time.Second)
		if nodes[0].statedb.GetSafrole().CheckFirstPhaseReady() {
			break
		}
	}

	// this is fib
	code, err := os.ReadFile(statedb.TestServiceFile)
	if err != nil {
		panic(0)
	}

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	codeHash := common.Blake2Hash(code)
	time.Sleep(12 * time.Second)
	fmt.Printf("Code Length: %v\n", len(code))
	fmt.Printf("Code Hash: %x\n", codeHash.Bytes())
	// code length: 206
	// Code Hash: d570234cc23da642334328827383e3625e1199a26747fc50c5ae45a731e6ff5c

	builerIdx := 1
	builderNode := nodes[builerIdx]
	builderNode.preimages[codeHash] = code
	new_service_idx := uint32(0)
	fmt.Printf("Builder storing  %v -> %x\n", codeHash, code)

	//----------------------------------------------
	loadTestService := true
	if loadTestService {
		// set up testservice using the Bootstrap service
		bootstrapCode, err := os.ReadFile(statedb.BootstrapServiceFile)
		if err != nil {
			panic(0)
		}
		bootstrapService := uint32(statedb.BootstrapServiceCode)
		bootstrapCodeHash := common.Blake2Hash(bootstrapCode)
		fibCodeWorkPackage := types.WorkPackage{
			Authorization: []byte(""),
			AuthCodeHost:  bootstrapService,
			Authorizer:    types.Authorizer{},
			RefineContext: types.RefineContext{},
			WorkItems: []types.WorkItem{
				{
					Service:          bootstrapService,
					CodeHash:         bootstrapCodeHash,
					Payload:          codeHash.Bytes(),
					GasLimit:         10000000,
					ImportedSegments: make([]types.ImportSegment, 0),
					ExportCount:      0,
				},
			},
		}
		err = builderNode.peersInfo[4].SendWorkPackageSubmission(0, fibCodeWorkPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// TODO: William - figure out how to get new service back from the above accumulate XContext (see hostNew SetX_i call)
		time.Sleep(12 * time.Second)

		// node1 can only broadcast announcement if it observes "service.storage("0") is non-empty"
		time.Sleep(12 * time.Second)

		service_ticker := time.NewTicker(types.SecondsPerSlot)
		defer service_ticker.Stop()

		new_service_found := false

		for !new_service_found {
			select {
			case <-service_ticker.C:
				//TODO: find service_index ... can be serviced via CE129 using stateKey=00b5000000000000ffffffffffffffffffffffffffffffffffffffffffffffff
				stateDB := builderNode.getState()
				if stateDB != nil && stateDB.Block != nil {
					// fmt.Printf("finding newservice from bootstrapService.s[0]...\n")
					stateRoot := stateDB.Block.GetHeader().ParentStateRoot
					t, _ := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), builderNode.store)

					newserviceKey := []byte{0, 0, 0, 0} // "0"
					service_account_byte, _ := t.GetServiceStorage(bootstrapService, newserviceKey)
					if err != nil {
						//not ready yet ...
						continue
					}
					decoded_new_service_idx := types.DecodeE_l(service_account_byte)
					if decoded_new_service_idx != 0 {
						new_service_idx = uint32(decoded_new_service_idx)
						//TODO: now use new_service_idx and see if (c,l) is correct
						fmt.Printf("Found service_idx: %v!!!\n", new_service_idx)
						new_service_found = true
					}
				}
			}
		}

		for validatorIdx, _ := range nodes {
			if validatorIdx != builerIdx {
				if new_service_idx > 0 {
					err = builderNode.peersInfo[uint16(validatorIdx)].SendPreimageAnnouncement(new_service_idx, codeHash, uint32(len(code)))
					if err != nil {
						fmt.Printf("SendPreimageAnnouncement ERR %v\n", err)
					}
				}
			}
		}
	}

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	//----------------------------------------------
	time.Sleep(60 * time.Second)
	fmt.Printf("Start FIB\n")

	n1 := nodes[1]
	n4 := nodes[4]
	core := 0
	prevWorkPackageHash := common.Hash{}
	for fibN := 1; fibN < 21; fibN++ {
		importedSegments := make([]types.ImportSegment, 0)
		if fibN > 1 {
			importedSegment := types.ImportSegment{
				WorkPackageHash: prevWorkPackageHash,
				Index:           0,
			}
			importedSegments = append(importedSegments, importedSegment)
		}
		refine_context := types.RefineContext{
			//TODO: Sean prereq of fib(n) is fib(n-1) and fib(n-2) package
			// Prerequisite     *Prerequisite `json:"prerequisite"`
		}
		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))
		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // TODO: set up null-authorizer
			//AuthCodeHost:  statedb.TestServiceCode,
			AuthCodeHost:  new_service_idx,
			Authorizer:    types.Authorizer{},
			RefineContext: refine_context,
			WorkItems: []types.WorkItem{
				{
					//Service:          statedb.TestServiceCode,
					Service:          new_service_idx,
					CodeHash:         codeHash,
					Payload:          payload,
					GasLimit:         10000000,
					ImportedSegments: importedSegments,
					ExportCount:      1,
				},
			},
		}
		workPackageHash := workPackage.Hash()

		// Question: can we remove this?  Why do we need this here?
		for _, n := range nodes {
			n.statedb.PreviousGuarantors(true)
			n.statedb.AssignGuarantors(true)
		}
		fmt.Printf("\n** \033[36m FIB=%v \033[0m workPackage: %v **\n", fibN, common.Str(workPackageHash))
		// CE133_WorkPackageSubmission: n1 => n4
		err := n1.peersInfo[4].SendWorkPackageSubmission(0, workPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// wait until the work report is pending
		var workReport types.WorkReport
		audit := false
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				audit = true
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
		if audit {
			err := n1.auditWorkReport(workReport)
			if err != nil {
				t.Fatalf("[auditWorkReport] ERR: %v", err)
			}
		}
		prevWorkPackageHash = workPackageHash
	}
}

func TestNodeRotation(t *testing.T) {
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error Seeting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], &genesisConfig, peers, peerList, ValidatorFlag, nodePaths[i], basePort+i)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		//node.state = statedb.ProcessGenesis(genesisAuthorities)
		nodes[i] = node
	}
	assign := nodes[0].statedb.AssignGuarantorsTesting(common.BytesToHash(common.ComputeHash([]byte("test"))))
	for _, a := range assign {
		fmt.Printf("CoreIndex:%d, Validator:%v\n", a.CoreIndex, a.Validator.Ed25519.String())
	}
}

func deleteUserJamDirectory(force bool) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("could not get current user: %v", err)
	}
	username := currentUser.Username

	path := filepath.Join("/tmp", username, "jam")

	// Safety checks
	if path == "/" || path == "" {
		return fmt.Errorf("invalid path: %s", path)
	}

	if !filepath.HasPrefix(path, "/tmp/") {
		return fmt.Errorf("refusing to delete directory outside /tmp/: %s", path)
	}

	// Check if directory exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Directory %s does not exist, nothing to delete.\n", path)
		return nil
	}

	// Skip prompt if 'force' is true
	if !force {
		fmt.Printf("Are you sure you want to delete all contents under %s? (y/N): ", path)
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Operation canceled.")
			return nil
		}
	}

	// Remove the directory and its contents
	err = os.RemoveAll(path)
	if err != nil {
		return fmt.Errorf("failed to delete directory %s: %v", path, err)
	}

	fmt.Printf("Successfully deleted directory %s and all its contents.\n", path)
	return nil
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

func SetLevelDBPaths(numNodes int) []string {
	node_paths := make([]string, numNodes)
	// timeslot mark
	// currJCE := common.ComputeCurrentJCETime()
	currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
	for i := 0; i < numNodes; i++ {
		node_idx := fmt.Sprintf("%d", i)
		node_path, err := computeLevelDBPath(node_idx, int(currJCE))
		if err == nil {
			node_paths[i] = node_path
		}
	}
	return node_paths
}

func TestLevelDBDelete(t *testing.T) {
	err := deleteUserJamDirectory(true)
	if err != nil {
		t.Fatalf("Deletetion Error: %v\n", err)
	}
}
