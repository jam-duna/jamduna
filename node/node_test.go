package node

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"time"

	//"sync"
	//"context"
	"encoding/json"
	"testing"

	"io/ioutil"
	"os"

	"github.com/colorfulnotion/jam/common"

	//"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"

	"github.com/colorfulnotion/jam/types"
)

func SetupQuicNetwork() (statedb.GenesisConfig, []string, map[string]NodeInfo, []types.ValidatorSecret, error) {
	seeds, _ := generateSeedSet(numNodes)
	fmt.Printf("seeds %x\n", seeds)

	peers := make([]string, numNodes)
	peerList := make(map[string]NodeInfo)

	validators := make([]types.Validator, numNodes)
	for i := 0; i < numNodes; i++ {
		validator, err := statedb.InitValidator(seeds[i], seeds[i])
		if err == nil {
			validators[i] = validator
		} else {
			return statedb.GenesisConfig{}, nil, nil, nil, fmt.Errorf("Failed to init validator %d: %v", i, err)
		}
	}

	genesisConfig := statedb.NewGenesisConfig(validators)

	prettyJSON, _ := json.MarshalIndent(validators, "", "  ")
	fmt.Printf("Validators (size:%v) %s\n", numNodes, prettyJSON)

	for i := uint32(0); i < numNodes; i++ {
		addr := fmt.Sprintf(quicAddr, 9000+i)
		peers[i] = addr
		ed25519Key := validators[i].Ed25519.String()
		peerList[ed25519Key] = NodeInfo{
			PeerID:    i,
			PeerAddr:  addr,
			Validator: validators[i],
		}
	}

	// Print out peerList
	prettyPeerList, err := json.MarshalIndent(peerList, "", "  ")
	if err != nil {
		return statedb.GenesisConfig{}, nil, nil, nil, fmt.Errorf("Failed to marshal peerList: %v", err)
	}
	fmt.Printf("PeerList: %s\n", prettyPeerList)

	// Compute validator secrets
	validatorSecrets := make([]types.ValidatorSecret, numNodes)
	for i := 0; i < numNodes; i++ {
		validatorSecret, err := statedb.InitValidatorSecret(seeds[i], seeds[i])
		if err != nil {
			return statedb.GenesisConfig{}, nil, nil, nil, fmt.Errorf("Failed to Generate secrets %v", err)
		}
		validatorSecrets[i] = validatorSecret
	}
	return genesisConfig, peers, peerList, validatorSecrets, nil
}

func TestNodeSafrole(t *testing.T) {
	genesisConfig, peers, peerList, validatorSecrets, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error Seeting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, ValidatorFlag)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		//node.state = statedb.ProcessGenesis(genesisAuthorities)
		nodes[i] = node
	}
	for {
	}
}

func TestECRoundTrip(t *testing.T) {
	// Define various data sizes to test
	//try to do it separately test for each size
	dataSizes := []int{1028, 23, 24, 25, 26, 27, 28, 29, 30, 31, 39, 1024}
	// Initialize nodes
	genesisConfig, peers, peerList, validatorSecrets, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, DAFlag)
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
		t.Run(fmt.Sprintf("DataSize%d", size), func(t *testing.T) {
			// Generate random data of the specified size
			data := make([]byte, size)
			_, err := rand.Read(data)
			if err != nil {
				t.Fatalf("Failed to generate random data: %v", err)
			}

			blob_hash, err := senderNode.EncodeAndDistributeData(data)
			if err != nil {
				t.Fatalf("Failed to encode and distribute data: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
			reconstructData, err := senderNode.FetchAndReconstructData(blob_hash)
			if err != nil {
				t.Fatalf("Failed to fetch and reconstruct data: %v", err)
			}
			time.Sleep(1000 * time.Millisecond)
			fmt.Printf("Reconstructed data (size %d): %x\n", len(reconstructData), reconstructData)

			// Compare the original data and the reconstructed data with bytes.Equal
			if !bytes.Equal(data, reconstructData) {
				t.Fatalf("Original data and reconstructed data are different for size %d", size)
			} else {
				fmt.Printf("roundtrip success for DataSize%d\n", size)
			}
		})
	}
}

// ---------------------------- Helper Functions ----------------------------

func print3DByteArray(arr [][][]byte) {
	for i := range arr {
		fmt.Printf("Segment %d:\n", i)
		fmt.Println("----------------")
		for j := range arr[i] {
			for k := range arr[i][j] {
				fmt.Printf("%02x ", arr[i][j][k])
			}
			fmt.Println()
		}
		fmt.Println("----------------")
	}
}

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

func TestWorkGuarantee(t *testing.T) {

	genesisConfig, peers, peerList, validatorSecrets, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}
	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, DAFlag)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		nodes[i] = node
	}

	// TODO: make squaring refine code that reads an extrinsic and exports the square of the 4 byte value in the extrinsic
	code, err := loadByteCode("../pvm/hostfunctions/squareextrinsic.pvm")
	if err != nil {
		t.Fatalf("%v", err)
	}

	codeHash, err := nodes[0].EncodeAndDistributeData(code)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Prepare a mock work package and work item
	importedSegments := make([]types.ImportSegment, 0)
	// TODO: write out some imported segments similar to the Extrinsics
	workPackage := types.WorkPackage{
		AuthorizationToken: []byte("0x"),
		ServiceIndex:       1,
		AuthorizationCode:  common.BytesToHash([]byte{}),
		ParamBlob:          []byte("0x"),
		Context:            []byte("0x"),
		WorkItems: []types.WorkItem{
			{
				ServiceIdentifier:   47,
				CodeHash:            codeHash,
				PayloadBlob:         []byte("0x00000010"),
				GasLimit:            10000000,
				ImportedSegments:    importedSegments,
				NewData:             nil, // Add any mock new data segments as needed
				NumSegmentsExported: 1,   // ??
				Extrinsics: [][]byte{
					{0x00, 0x00, 0x00, 0x01},
					{0x00, 0x00, 0x00, 0x03},
					{0x00, 0x00, 0x00, 0x05},
					{0x00, 0x00, 0x00, 0x07},
				},
			},
		},
	}
	for _, n := range nodes {
		if n.coreIndex == 0 {
			n.processWorkPackage(workPackage)
		}
	}
}
