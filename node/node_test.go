package node

import (
	"encoding/json"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"math/big"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"
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

	code, err := os.ReadFile(statedb.TestServiceFile)
	if err != nil {
		panic(0)
	}

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	time.Sleep(12 * time.Second)
	//----------------------------------------------
	loadTestService := false
	if loadTestService {
		// set up testservice using the Bootstrap service
		bootstrapCode, err := os.ReadFile(statedb.BootstrapServiceFile)
		if err != nil {
			panic(0)
		}
		bootstrapService := uint32(statedb.BootstrapServiceCode)
		bootstrapCodeHash := common.Blake2Hash(bootstrapCode)
		
		codeLenBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(codeLenBytes, uint32(len(code)))
		fibCodeWorkPackage := types.WorkPackage{
			Authorization: []byte(""),
			AuthCodeHost:  bootstrapService,
			Authorizer:    types.Authorizer{},
			RefineContext: types.RefineContext{},
			WorkItems: []types.WorkItem{
				{
					Service:          bootstrapService,
					CodeHash:         bootstrapCodeHash,
					Payload:          append(codeLenBytes, code...), // "y" = |c| ++ c
					GasLimit:         10000000,
					ImportedSegments: make([]types.ImportSegment, 0),
					ExportCount:      0,
				},
			},
		}
		err = nodes[1].peersInfo[4].SendWorkPackageSubmission(0, fibCodeWorkPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// TODO: William - figure out how to get new service back from the above accumulate XContext (see hostNew SetX_i call)
		time.Sleep(12 * time.Second)
	}
	codeHash := common.Blake2Hash(code)

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	//----------------------------------------------
	time.Sleep(12 * time.Second)
	var exportedItems []types.ImportSegment
	for fibN := 1; fibN < 21; fibN++ {
		fmt.Printf("\n\n\n********************** FIB N=%v Starts **********************\n", fibN)
		importedSegments := make([]types.ImportSegment, 0)

		if fibN > 1 {
			importedSegments = append(importedSegments, exportedItems...)
		}
		refine_context := types.RefineContext{
			//TODO: Sean
			// Prerequisite     *Prerequisite `json:"prerequisite"`
		}
		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // TODO: sign
			AuthCodeHost:  statedb.TestServiceCode,
			Authorizer:    types.Authorizer{},
			RefineContext: refine_context,
			WorkItems: []types.WorkItem{
				{
					Service:          statedb.TestServiceCode,
					CodeHash:         codeHash,
					Payload:          []byte("0x00000010"),
					GasLimit:         10000000,
					ImportedSegments: importedSegments,
					ExportCount:      1,
				},
			},
		}

		// CE133_WorkPackageSubmission: Node 1 (outside Core 0) sends package to Node 0 (in Core 0) via
		for _, n := range nodes {
			n.statedb.PreviousGuarantors(true)
			n.statedb.AssignGuarantors(true)
		}
		err := nodes[1].peersInfo[4].SendWorkPackageSubmission(0, workPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// we trace core 0 only
		for {
			time.Sleep(1 * time.Second)
			if nodes[4].statedb.JamState.AvailabilityAssignments[0] != nil {
				break
			}
		}

		// add assurance
		for {
			if nodes[4].statedb.JamState.AvailabilityAssignments[0] == nil {
				break
			}
			time.Sleep(types.SecondsPerSlot * time.Second)
		}
		exportedItems = nodes[4].segments[workPackage.Hash()]
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
