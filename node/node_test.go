package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	//"sync"

	"encoding/json"
	"testing"

	"io/ioutil"
	"os"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"

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
		ed25519Key := fmt.Sprintf("%x", validators[i].Ed25519)
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

func TestSegmentECRoundTrip(t *testing.T) {
	// Define various data sizes to test
	dataSizes := []int{types.W_C * types.W_S}
	segmentSizes := []int{1, 10}
	// dataSizes := []int{10}

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
				var wg sync.WaitGroup
				wg.Add(1)
				// Coroutine to encode and distribute data
				go func() {
					treeRoot, err = senderNode.EncodeAndDistributeSegmentData(combinedSegmentAndPageProofs, &wg)
					if err != nil {
						fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
					}
				}()

				// Wait for the encoding and distribution to finish
				wg.Wait()
				fmt.Println("Finished EncodeAndDistributeSegmentData...")
				if err != nil {
					t.Fatalf("Failed to encode and distribute data: %v", err)
				}

				// Simulate a small delay before fetching and reconstructing the data
				time.Sleep(2 * time.Second)

				// Use the first node to fetch and reconstruct the data
				fmt.Println("Starting FetchAndReconstructSegmentData...")
				reconstructedData, err := senderNode.FetchAndReconstructAllSegmentsData(treeRoot)
				if err != nil {
					t.Fatalf("Failed to fetch and reconstruct data: %v", err)
				}
				fmt.Println("Finished FetchAndReconstructSegmentData...")

				// Compare original and reconstructed data
				for i := 0; i < len(data); i++ {
					if !bytes.Equal(data[i], reconstructedData[i][:len(data[i])]) {
						fmt.Printf("Original data: %x\n", data[i])
						fmt.Printf("Reconstructed data: %x\n", reconstructedData[i][:len(data[i])])
						t.Fatalf("Data mismatch for size %d: original and reconstructed data are not the same", size)
					}
				}
				fmt.Printf("Roundtrip success for DataSize%d\n", size)
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
		size := size
		t.Run(fmt.Sprintf("DataSize%d", size), func(t *testing.T) {
			// Generate random data of the specified size
			data := make([]byte, size)
			_, err := rand.Read(data)
			if err != nil {
				t.Fatalf("Failed to generate random data: %v", err)
			}

			blob_len := len(data)
			blob_hash := common.Hash{}

			var wg sync.WaitGroup
			wg.Add(1) // Wait for one task to complete

			// Coroutine to encode and distribute arbitrary data
			go func() {
				blob_hash, err = senderNode.EncodeAndDistributeArbitraryData(data, blob_len, &wg)
				if err != nil {
					fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
				}
			}()

			// Wait for the encoding and distribution to finish
			wg.Wait()

			if err != nil {
				t.Fatalf("Failed to encode and distribute data: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
			reconstructData, err := senderNode.FetchAndReconstructArbitraryData(blob_hash, blob_len)
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

/*

 Group effort - Fib
 need Willaim export & import
 need Sean's encode func for E(p,x,i,j) + e
 need Stanley availability specifier(as) & paged proof & process to reconstruct using AS
 need Shawn's Assurance/Judgement based on Stanley's reconstructed wp

*/

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
	code, err := loadByteCode("../jamtestvectors/workpackages/fib-refine-fixed.pvm")
	if err != nil {
		t.Fatalf("%v", err)
	}

	// TODO: need to use TestNodePOAAccumulatePVM logic to put the code into system
	codeHash := common.Hash{}
	var wg sync.WaitGroup
	wg.Add(1) // Wait for one task to complete

	// Coroutine to encode and distribute arbitrary data
	go func() {
		codeHash, err = nodes[0].EncodeAndDistributeArbitraryData(code, len(code), &wg)
		if err != nil {
			fmt.Println("Error in EncodeAndDistributeSegmentData:", err)
		}
	}()

	// Wait for the encoding and distribution to finish
	wg.Wait()

	if err != nil {
		t.Fatalf("%v", err)
	}

	for _, n := range nodes {
		target_statedb := n.getPVMStateDB()
		target_statedb.WriteServicePreimageBlob(47, code)
		tentativeRoot := target_statedb.GetTentativeStateRoot()
		target_statedb.StateRoot = tentativeRoot
		n.statedb = target_statedb.Copy()

		recovered_code := n.statedb.ReadServicePreimageBlob(47, codeHash)
		if !common.CompareBytes(code, recovered_code) {
			panic(0)
		}
	}

	authToken := []byte("0x")               // TODO: sign
	var exportedItems []types.ImportSegment // how do you get import segments from previous round?
	for n := 1; n < 20; n++ {
		time.Sleep(types.PeriodSecond * time.Second)
		fmt.Printf("\n\n\n********************** FIB N=%v Starts **********************\n", n)
		importedSegments := make([]types.ImportSegment, 0)
		if n > 1 {
			importedSegments = append(importedSegments, exportedItems...)
		}
		refine_context := types.RefineContext{}

		// WorkPackage represents a work package.
		/*type WorkPackage struct {
			// $j$ - a simple blob acting as an authorization token
			Authorization []byte `json:"authorization"`
			// $h$ - the index of the service which hosts the authorization code
			AuthCodeHost uint32 `json:"auth_code_host"`
			// $c$ - an authorization code hash
			Authorizer Authorizer `json:"authorizer"`
			// $x$ - context
			RefineContext RefinementContext `json:"context"`
			// $i$ - a sequence of work items
			WorkItems []WorkItem `json:"items"`
		}*/

		workPackage := types.WorkPackage{
			Authorization: authToken,
			AuthCodeHost:  47,
			Authorizer:    types.Authorizer{},
			RefineContext: refine_context,
			WorkItems: []types.WorkItem{
				{
					Service:          47,
					CodeHash:         codeHash,
					Payload:          []byte("0x00000010"),
					GasLimit:         10000000,
					ImportedSegments: importedSegments,
					ExportCount:      1,
				},
			},
		}
		packageHash := workPackage.Hash()
		for _, n := range nodes {
			ctx := context.Background()
			s0 := n.statedb
			if s0 == nil {
				fmt.Println("s0 is nil")
			}
			targetJCE := statedb.ComputeCurrentJCETime() + 120
			b1, b1_err := s0.MakeBlock(n.credential, targetJCE)
			if b1_err != nil {
				t.Fatalf("MakeBlock err %v\n", b1_err)
			}
			s1, s1_err := statedb.ApplyStateTransitionFromBlock(s0, ctx, b1)
			if s1_err != nil {
				t.Fatalf("S0->S1 Transition Err: %v\n", s1_err)
			}
			n.addStateDB(s1)
			target_statedb := n.getPVMStateDB()
			target_statedb.WriteServicePreimageBlob(workPackage.AuthCodeHost, code)
			tentativeRoot := target_statedb.GetTentativeStateRoot()
			target_statedb.StateRoot = tentativeRoot
			n.statedb = target_statedb.Copy()
			recovered_code := n.statedb.ReadServicePreimageBlob(workPackage.AuthCodeHost, codeHash)
			if !common.CompareBytes(code, recovered_code) {
				panic(0)
			}

			fmt.Println("Node ID:", n.id)
			if n.coreIndex == 0 {
				//spec *types.AvailabilitySpecifier, err error, bBlobHash common.Hash, sBlobHash common.Hash, exportedSegments [][]byte
				specifier, treeRoot, err := n.processWorkPackage(workPackage)
				if err != nil {
					panic(0)
				}
				// 1. Check PackageHash
				if specifier.WorkPackageHash != packageHash {
					t.Errorf("expected PackageHash %v, got %v", packageHash, specifier.WorkPackageHash)
				} else {
					fmt.Printf("Exported Segments root:%s\n", specifier.ExportedSegmentRoot)
				}

				if len(specifier.ErasureRoot) == 0 {
					t.Error("ErasureRoot should not be empty")
				} else {
					fmt.Printf("ErasureRoot:%s\n", specifier.ErasureRoot)
				}

				if len(specifier.ErasureRoot) == 0 {
					t.Error("SegmentRoot should not be empty")
				} else {
					fmt.Printf("SegmentRoot:%s\n", specifier.ErasureRoot)
				}

				// 4. Check ExportedSegmentRoot (simplified check)
				if len(specifier.ExportedSegmentRoot) == 0 {
					t.Error("ExportedSegmentRoot should not be empty")
				} else {
					fmt.Printf("Exported Segments root:%s\n", specifier.ExportedSegmentRoot)
				}
				t.Logf("Generated Availability Specifier: %+v", specifier)
				// TODO: use specifier to setup the next importsegments
				exportedSegmentsRoot, _ := n.GetSegmentTreeRoots(treeRoot)
				fmt.Printf("exportedSegments Len=%d, exportedSegments: %x\n", len(exportedSegmentsRoot), exportedSegmentsRoot)
				exportedItems = nil
				for i, segmentRoot := range exportedSegmentsRoot {
					fmt.Printf("[seg_idx=%v] segmentRoot=%v\n", i, treeRoot)
					exportedItem := types.ImportSegment{
						TreeRoot: segmentRoot,
						Index:    uint16(i),
					}
					fmt.Printf("exportedItem: %v\n", exportedItem)
					exportedItems = append(exportedItems, exportedItem)
				}
				fmt.Printf("++ exportedSegments: %v\n", exportedSegmentsRoot)
			}
		}
	}
}

func TestCodeParse(t *testing.T) {

	// fib code
	code, err := loadByteCode("../jamtestvectors/workpackages/fib-full.pvm")
	if err != nil {
		t.Fatalf("%v", err)
	}
	fmt.Println("Code:", code)
	pvm.NewVMFromParseProgramTest(code)
}
func TestNodeRotation(t *testing.T) {
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
	assign := nodes[0].statedb.AssignGuarantorsTesting(common.BytesToHash(common.ComputeHash([]byte("test"))))
	for _, a := range assign {
		fmt.Printf("CoreIndex:%d, Validator:%x\n", a.CoreIndex, a.Validator.Ed25519)
	}
}

// func TestIsValidAvailabilitySpecifier(t *testing.T) {
// 	// Set up the network
// 	genesisConfig, peers, peerList, validatorSecrets, err := SetupQuicNetwork()
// 	if err != nil {
// 		t.Fatalf("Error setting up nodes: %v\n", err)
// 	}
// 	nodes := make([]*Node, numNodes)
// 	for i := 0; i < numNodes; i++ {
// 		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, DAFlag)
// 		if err != nil {
// 			t.Fatalf("Failed to create node %d: %v\n", i, err)
// 		}
// 		nodes[i] = node
// 	}

// 	senderNode := nodes[0]

// 	// Simulate a work package and segments
// 	workPackage := types.WorkPackage{ /*...Set fields...*/ }
// 	segments := [][]byte{[]byte("segment1"), []byte("segment2")}

// 	// Generate the AvailabilitySpecifier
// 	packageHash := common.ComputeHash([]byte("test_package"))
// 	originalAS, blobHash, treeRoot := senderNode.NewAvailabilitySpecifier(common.Hash(packageHash), workPackage, segments)

// 	// Validate the AvailabilitySpecifier
// 	isValid, err := senderNode.IsValidAvailabilitySpecifier(blobHash, int(originalAS.BundleLength), treeRoot, originalAS)
// 	if err != nil {
// 		t.Fatalf("Error validating AvailabilitySpecifier: %v", err)
// 	}
// 	if isValid == false {
// 		t.Fatalf("AvailabilitySpecifier is not valid: %v", err)
// 	}
// }

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
