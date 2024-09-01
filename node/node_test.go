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

	//prettyJSON, _ := json.MarshalIndent(validators, "", "  ")
	//fmt.Printf("Validators (size:%v) %s\n", numNodes, prettyJSON)

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

	// fib code
	code, err := loadByteCode("../jamtestvectors/workpackages/fib.pvm")
	if err != nil {
		t.Fatalf("%v", err)
	}

	codeHash, err := nodes[0].EncodeAndDistributeData(code)
	if err != nil {
		t.Fatalf("%v", err)
	}
	authToken := []byte("0x") // TODO: sign
	var exportedItem types.ImportSegment
	for n := 1; n < 20; n++ {
		importedSegments := make([]types.ImportSegment, 0)
		if n > 1 {
			importedSegments = append(importedSegments, exportedItem)
		}
		context := types.RefinementContext{}
		workPackage := types.WorkPackage{
			AuthorizationToken: authToken,
			ServiceIndex:       47,
			AuthorizationCode:  common.BytesToHash([]byte{}),
			ParamBlob:          []byte("0x"),
			Context:            context,
			WorkItems: []types.WorkItem{
				{
					ServiceIdentifier: 47,
					CodeHash:          codeHash,
					Payload:           []byte("0x00000010"),
					GasLimit:          10000000,
					ImportedSegments:  importedSegments,
					ExportCount:       1,
				},
			},
		}
		for _, n := range nodes {
			if n.coreIndex == 0 {
				n.processWorkPackage(workPackage)
			}
		}

		// TODO: get exportedItem from refine execution ... or accumulate if it can do export

	}
}

func TestBuildAvailabilitySpecifier(t *testing.T) {
	// Step 1: Prepare test data
	packageHash := common.Hash{0x1, 0x2, 0x3, 0x4}
	workPackage := ASWorkPackage{
		ImportSegments: []ASWorkItem{
			{segments: []common.Segment{
				{Data: []byte("segment1")},
				{Data: []byte("segment2")},
			}},
		},
		Extrinsic: []byte("extrinsicData"),
	}
	segments := []common.Segment{
		{Data: []byte("segment1")},
		{Data: []byte("segment2")},
	}

	// Step 2: Call the function
	specifier := BuildAvailabilitySpecifier(packageHash, workPackage, segments)

	// Step 3: Check the results
	// 1. Check PackageHash
	if specifier.PackageHash != packageHash {
		t.Errorf("expected PackageHash %v, got %v", packageHash, specifier.PackageHash)
	}else{
		fmt.Printf("Exported Segments root:%s\n", specifier.ExportedSegments)
	}

	expectedLength := uint32(len(EncodeWorkPackage(workPackage)))
	if specifier.AuditFriendlyWorkPackageLength != expectedLength {
		t.Errorf("expected AuditFriendlyWorkPackageLength %v, got %v", expectedLength, specifier.AuditFriendlyWorkPackageLength)
	}else{
		fmt.Printf("AuditFriendlyWorkPackageLength:%d\n", specifier.AuditFriendlyWorkPackageLength)
	}

	if len(specifier.AvailabilityVector) == 0 {
		t.Error("AvailabilityVector should not be empty")
	}else{
		fmt.Printf("AvailabilityVector root:%s\n", specifier.AvailabilityVector)
	}

	// 4. Check ExportedSegments (simplified check)
	if len(specifier.ExportedSegments) == 0 {
		t.Error("ExportedSegments should not be empty")
	}else{
		fmt.Printf("Exported Segments root:%s\n", specifier.ExportedSegments)
	}

	t.Logf("Generated Availability Specifier: %+v", specifier)
}