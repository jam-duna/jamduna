package node

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"time"

	//"sync"
	"context"
	"encoding/json"
	"testing"

	"io/ioutil"
	"os"

	"github.com/colorfulnotion/jam/common"

	"github.com/colorfulnotion/jam/pvm"
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

func TestNodePOAAccumulatePVM(t *testing.T) {

	genesisConfig, peers, peerList, validatorSecrets, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error Seeting up nodes: %v\n", err)
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
	extrinsics := []string{"abcdef", "abcd", "cat", "dog"}
	for _, s := range extrinsics {
		data := []byte(s)
		fmt.Println(data)
		blob_hash, err := senderNode.EncodeAndDistributeData(data)
		if err != nil {
			t.Fatalf("Failed to encode and distribute data: %v", err)
		}
		fmt.Printf("successfully distributed blob_hash : %v\n", blob_hash)
	}

	// Accumulate function performs the accumulation of a single service.
	serviceIndex := 49
	//solict_program_code := pvm.LoadPVMCode("../pvm/hostfunctions/solicit.pvm")

	// solict_program_code := []byte{0, 0, 0, 1, 2, 3, 4, 5}
	solict_program_code := []byte{
		0,
		0,
		74, // size of c
		4, 0, 0, 64,
		4, 1, 6,
		38, 2, 0, 64, 2, 140, 91, 117,
		38, 2, 4, 64, 26, 184, 246, 11,
		38, 2, 8, 64, 158, 6, 29, 62,
		38, 2, 12, 64, 227, 25, 126, 23,
		38, 2, 16, 64, 101, 37, 44, 137,
		38, 2, 20, 64, 77, 169, 193, 149,
		38, 2, 24, 64, 43, 144, 90, 219,
		38, 2, 28, 64, 18, 98, 60, 158,
		78, 13,
		0,
		145, 128, 128, 128, 128, 128, 128, 128, 128, 254, // bitmask
		0}

	// solicit code executes causes SDB writes for a_l USING: ReadServiceBytes, ReadServicePreimageLookup, WriteServicePreimageLookup
	for i := 0; i < 1; i++ {
		//you can potentially call NewForceCreateVM() and initialize your vm, make sure you handle the reg properly
		poa_node := nodes[i]
		target_statedb := poa_node.getPVMStateDB()
		vm := pvm.NewVMFromCode(solict_program_code, 0, target_statedb)
		// NEW IDEA: hostSolicit will fill this array
		// lookups = vm.Solicits
		err := vm.Execute(types.EntryPointAccumulate)
		lookups := vm.Solicits
		if err != nil {
			fmt.Printf("VM Execute Err:%v/n", err)
		}

		ctx := context.Background()
		//stateDB.NewStateDB(nodes[0].storage)
		//s := nodes[i].statedb.NewStateDB(nodes[0].storage)

		s := poa_node.statedb
		targetJCE := statedb.ComputeCurrentJCETime() + 120
		b0, s2, err0 := s.MakeBlock(poa_node.credential, targetJCE)
		if err0 != nil {
			t.Fatalf("MakeBlock err %v\n", err0)
		} else {
			fmt.Printf("S2 StateRoot:%v Block:%v\n", b0.String(), s2.StateRoot)
		}
		// use lookups to do Fetch
		poa_node.statedb.ApplyStateTransitionFromBlock(ctx, b0)
		for _, l := range lookups {
			reconstructData, err := senderNode.FetchAndReconstructData(l.BlobHash)
			//reconstructData, err := senderNode.FetchAndReconstructData(l.BlobHash, l.Length)
			if err != nil {
				t.Fatalf("Failed to fetch and reconstruct data: %v", err)
			}
			// now you have Preimage AND blob
			lookup := types.PreimageLookup{
				ServiceIndex: uint32(serviceIndex),
				Data:         reconstructData[0:l.Length],
			}

			// ADD TO Queue  which is used in the NEXT MakeBlock to fill the E_P
			//stateDB need to add lookup
			nodes[i].processLookup(lookup)
		}

		b1, s3, err0 := s.MakeBlock(poa_node.credential, targetJCE+1)
		if err0 != nil {
			t.Fatalf("MakeBlock err %v\n", err0)
		} else {
			fmt.Printf("S3 StateRoot:%v Block:%v\n", b1.String(), s3.StateRoot)
		}
		// THIS update a_p
		nodes[i].statedb.ApplyStateTransitionFromBlock(ctx, b1)

		// check whetere intergrate successfully
		t := s.GetTrie()
		t = s.CopyTrieState(s.StateRoot)
		data, _ := t.GetPreImageBlob(b1.Extrinsic.PreimageLookups[0].ServiceIndex, bhash(b1.Extrinsic.PreimageLookups[0].Data).Bytes())
		ts, _ := t.GetPreImageLookup(b1.Extrinsic.PreimageLookups[0].ServiceIndex, bhash(b1.Extrinsic.PreimageLookups[0].Data), uint32(len(b1.Extrinsic.PreimageLookups[0].Data)))
		fmt.Println("data:", data)
		fmt.Println("ts:", ts)
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
