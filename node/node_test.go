package node

import (
	//"bytes"

	//"crypto/rand"
	"fmt"
	//"math"
	"math/big"
	"path/filepath"
	"time"

	//"sync"

	"encoding/json"
	"testing"

	"io/ioutil"
	"os"
	"os/user"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"

	//"github.com/colorfulnotion/jam/trie"

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

/*
func TestSegmentECRoundTrip(t *testing.T) {
	// Define various data sizes to test
	dataSizes := []int{types.W_C * types.W_S}
	// segmentSizes := []int{1, 10}
	segmentSizes := []int{10}

	// Initialize nodes
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, DAFlag, nodePaths[i], basePort+i)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		nodes[i] = node
	}

	// Wait for nodes to be ready
	fmt.Println("Waiting for nodes to be ready...")
	time.Sleep(2 * time.Second)
	fmt.Println("Time Sleep End...")

	// Test encoding, distributing, fetching, and reconstructing data for each size

	for _, segmentLength := range segmentSizes {
		for _, size := range dataSizes {
			fmt.Printf("processing %d........\n", size)
			size := size
			t.Run(fmt.Sprintf("DataSize%d", size), func(t *testing.T) {
				// Generate random data of the specified size
				var data [][]byte
				for i := 0; i < segmentLength; i++ {
					randData := make([]byte, size)
					_, err := rand.Read(randData)
					randData = common.PadToMultipleOfN(randData, types.W_C*types.W_S)
					data = append(data, randData)
					if err != nil {
						t.Fatalf("Failed to generate random data: %v", err)
					}
				}

				pageProofs, _ := trie.GeneratePageProof(data)
				combinedSegmentAndPageProofs := append(data, pageProofs...)

				// Use sender node to encode and distribute data
				senderNode := nodes[0]
				fmt.Println("Starting EncodeAndDistributeSegmentData...")

				treeRoot := common.Hash{}
				// Encode the combined data
				var segmentsECRoots []byte
				// Flatten the combined data
				var FlattenData []byte
				for _, singleData := range combinedSegmentAndPageProofs {
					FlattenData = append(FlattenData, singleData...)
				}
				// Erasure code the combined data
				for _, singleData := range combinedSegmentAndPageProofs {
					// Encode the data into segments
					erasureCodingSegments, err := senderNode.encode(singleData, true, len(singleData)) // Set to false for variable size segments
					if err != nil {
						fmt.Printf("Error in EncodeAndDistributeSegmentData: %v\n", err)
					}

					// Build segment roots
					segmentRoots := make([][]byte, 0)
					for i := range erasureCodingSegments {
						leaves := erasureCodingSegments[i]
						tree := trie.NewCDMerkleTree(leaves)
						segmentRoots = append(segmentRoots, tree.Root())
					}

					// Generate the blob hash by hashing the original data
					blobTree := trie.NewCDMerkleTree(segmentRoots)
					segmentsECRoot := blobTree.Root()

					// Append the segment root to the list of segment roots
					segmentsECRoots = append(segmentsECRoots, segmentsECRoot...)
					// Distribute the segments
					err = senderNode.DistributeSegmentData(erasureCodingSegments, segmentRoots, len(FlattenData))
					if err != nil {
						fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
					}
				}

				fmt.Println("Finished EncodeAndDistributeSegmentData...")
				if err != nil {
					t.Fatalf("Failed to encode and distribute data: %v", err)
				}

				// Simulate a small delay before fetching and reconstructing the data
				time.Sleep(3 * time.Second)

				// Use the first node to fetch and reconstruct the data
				fmt.Println("Starting FetchAndReconstructAllSegmentsData...")
				reconstructedData, _, _, err := senderNode.FetchAndReconstructAllSegmentsData(treeRoot)
				if err != nil {
					t.Fatalf("Failed to fetch and reconstruct data: %v", err)
				}
				fmt.Println("Finished FetchAndReconstructAllSegmentsData...")

				// Compare original and reconstructed data
				for i := 0; i < len(data); i++ {
					fmt.Printf("Original data: %x\n", data[i])
					fmt.Printf("Reconstructed data: %x\n", reconstructedData[i][:len(data[i])])
					if !bytes.Equal(data[i], reconstructedData[i][:len(data[i])]) {
						fmt.Printf("Original data: %x\n", data[i])
						fmt.Printf("Reconstructed data: %x\n", reconstructedData[i][:len(data[i])])
						t.Fatalf("Data mismatch for size %d: original and reconstructed data are not the same", size)
					}
				}

				for i := 0; i < len(data); i++ {
					reconstructedSegment, reconstructedPageProof, err := senderNode.FetchAndReconstructSegmentData(treeRoot, uint32(i))
					fmt.Printf("Finished FetchAndReconstructSegmentData %d...\n", i)
					if err != nil {
						t.Fatalf("Failed to fetch and reconstruct data: %v", err)
					}
					fmt.Printf("Reconstructed data: %x\n", reconstructedSegment)
					if !common.CompareBytes(data[i], reconstructedSegment) {
						fmt.Printf("Original data: %x\n", data[i])
						fmt.Printf("Reconstructed data: %x\n", reconstructedSegment)
						t.Fatalf("Data mismatch for size %d: original and reconstructed data are not the same", size)
					}
					if !common.CompareBytes(pageProofs[int(math.Floor(float64(i)/64))], reconstructedPageProof[:len(pageProofs[int(math.Floor(float64(i)/64))])]) {
						fmt.Printf("pageProofs[int(math.Ceil(i/64))] data: %x\n", pageProofs[int(math.Floor(float64(i)/64))])
						fmt.Printf("reconstructedPageProof data: %x\n", reconstructedPageProof)
						t.Fatalf("Data mismatch for size %d: original and reconstructed data are not the same", size)
					}

					fmt.Printf("Roundtrip success for DataSize%d\n", size)
				}
			})
		}
	}
}

func TestECRoundTrip(t *testing.T) {
	// Define various data sizes to test
	//try to do it separately test for each size
	dataSizes := []int{1028, 23, 24, 25, 26, 27, 28, 29, 30, 31, 39, 1024}
	// dataSizes := []int{types.W_C * types.W_S, types.W_C * types.W_S * 2, types.W_C * types.W_S * 3}
	// dataSizes := []int{2084}
	// Initialize nodes
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, DAFlag, nodePaths[i], basePort+i)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		nodes[i] = node
	}
	// Wait for nodes to be ready
	fmt.Println("Waiting for nodes to be ready...")
	time.Sleep(1 * time.Second)

	senderNode := nodes[0]

	for _, size := range dataSizes {
		size := size
		t.Run(fmt.Sprintf("DataSize%d", size), func(t *testing.T) {
			// Generate random data of the specified size
			data := make([]byte, size)
			_, err := rand.Read(data)
			if err != nil {
				t.Fatalf("Failed to generate random data: %v", err)
			}

			paddeddata := common.PadToMultipleOfN(data, types.W_C)
			dataLength := len(data)

			chunks, err := senderNode.encode(paddeddata, false, dataLength)
			if err != nil {
				fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
			}

			blobHash := common.Blake2Hash(paddeddata)

			err = senderNode.DistributeArbitraryData(chunks, blobHash, dataLength)

			if err != nil {
				t.Fatalf("Failed to encode and distribute data: %v", err)
			}

			time.Sleep(500 * time.Millisecond)
			reconstructData, err := senderNode.FetchAndReconstructArbitraryData(blobHash, dataLength)
			if err != nil {
				t.Fatalf("Failed to fetch and reconstruct data: %v", err)
			}
			time.Sleep(1000 * time.Millisecond)
			fmt.Printf("Reconstructed data (size %d): %x\n", len(reconstructData), reconstructData)

			// Compare the original data and the reconstructed data with bytes.Equal
			if !bytes.Equal(data, reconstructData) {
				fmt.Printf("Original data: %x\n", data)
				fmt.Printf("Reconstructed data: %x\n", reconstructData)
				t.Fatalf("Original data and reconstructed data are different for size %d", size)
			} else {
				fmt.Printf("roundtrip success for DataSize%d\n", size)
			}
		})
	}
}


 Group effort - Fib
 need Willaim export & import
 need Sean's encode func for E(p,x,i,j) + e
 need Stanley availability specifier(as) & paged proof & process to reconstruct using AS
 need Shawn's Assurance/Judgement based on Stanley's reconstructed wp

*/

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
	time.Sleep(3 * time.Second)

	// fib code
	code, err := loadByteCode("../jamtestvectors/workpackages/fib-standardized.pvm")
	if err != nil {
		t.Fatalf("%v", err)
	}

	// TODO: William - need to use privileged service (stored in genesis state) to load this code with a specific service
	service := uint32(47) // WRONG: william to fix
	codeHash := common.Blake2Hash(code)
	codeLength := len(code)
	chunks, err := nodes[0].encode(code, false, codeLength)
	if err != nil {
		fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
	}

	err = nodes[0].DistributeArbitraryData(chunks, codeHash, codeLength)
	if err != nil {
		fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
	}

	for _, n := range nodes {
		target_statedb := n.getPVMStateDB()
		target_statedb.WriteServicePreimageBlob(service, code)
		tentativeRoot := target_statedb.GetTentativeStateRoot()
		target_statedb.StateRoot = tentativeRoot
		n.statedb = target_statedb.Copy()

		recovered_code := n.statedb.ReadServicePreimageBlob(service, codeHash)
		if !common.CompareBytes(code, recovered_code) {
			panic(0)
		}
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	time.Sleep(12 * time.Second)
	var exportedItems []types.ImportSegment
	for fibN := 1; fibN < 2; fibN++ {
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
			AuthCodeHost:  47,
			Authorizer:    types.Authorizer{},
			RefineContext: refine_context,
			WorkItems: []types.WorkItem{
				{
					Service:          service,
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
		time.Sleep(10 * time.Second)
		E_G, err := nodes[4].FormGuarantee(workPackage.Hash())
		if err != nil {
			t.Fatal(err)
		}
		nodes[4].broadcast(E_G)

		time.Sleep(30 * time.Second)
	}
}

// func TestCodeParse(t *testing.T) {

// 	// fib code
// 	code, err := loadByteCode("../jamtestvectors/workpackages/standardized_jam_service.pvm")
// 	if err != nil {
// 		t.Fatalf("%v", err)
// 	}
// 	fmt.Println("Code:", code)
// 	pvm.NewVMFromParseProgramTest(code)
// }

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

// ---------------------------- Helper Functions ----------------------------

// loadByteCode reads the bytes from the given file path and returns them as a byte slice.
func loadByteCode(filePath string) ([]byte, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the bytes from the file
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return bytes, nil
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
