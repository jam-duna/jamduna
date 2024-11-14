package node

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/exp/rand"
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

func getServices(serviceNames []string) (services map[string]*types.TestService, err error) {
	services = make(map[string]*types.TestService)
	for i, serviceName := range serviceNames {
		fileName := fmt.Sprintf("../services/%s.pvm", serviceName)
		code, err1 := os.ReadFile(fileName)
		if err1 != nil {
			return services, err1
		}
		tmpServiceCode := uint32(i + 1)
		codeHash := common.Blake2Hash(code)
		services[serviceName] = &types.TestService{
			ServiceCode: tmpServiceCode, // TEMPORARY
			FileName:    fileName,
			CodeHash:    codeHash,
			Code:        code,
		}
		fmt.Printf("%d: %s Code Hash: %x (Code Length: %v)\n", tmpServiceCode, serviceName, codeHash.Bytes(), len(code))
	}
	return
}

func TestWorkGuaranteeFIB(t *testing.T) {
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

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	// code length: 206
	bootstrapCode, err := os.ReadFile(statedb.BootstrapServiceFile)
	if err != nil {
		panic(0)
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)

	builderIdx := 1
	builderNode := nodes[builderIdx]
	builderNode.preimages[bootstrapCodeHash] = bootstrapCode
	new_service_idx := uint32(0)

	// Load testServices
	testServices, err := getServices([]string{"fib", "tribonacci", "megatron"}) // "padovan", "pell", "racaman",
	//testServices, err := getServices([]string{"fib"})
	if err != nil {
		panic(32)
	}

	// set builderNode's primages map
	for _, service := range testServices {
		builderNode.preimages[service.CodeHash] = service.Code
	}

	var previous_service_idx uint32
	for serviceName, service := range testServices {
		fmt.Printf("Builder storing TestService %s (%x)\n", serviceName, service.CodeHash)
		// set up service using the Bootstrap service
		codeWorkPackage := types.WorkPackage{
			Authorization: []byte(""),
			AuthCodeHost:  bootstrapService,
			Authorizer:    types.Authorizer{},
			RefineContext: types.RefineContext{},
			WorkItems: []types.WorkItem{
				{
					Service:          bootstrapService,
					CodeHash:         bootstrapCodeHash,
					Payload:          append(service.CodeHash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(service.Code)))...),
					GasLimit:         10000000,
					ImportedSegments: make([]types.ImportSegment, 0),
					ExportCount:      0,
				},
			},
		}
		err = builderNode.peersInfo[4].SendWorkPackageSubmission(0, codeWorkPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}

		// service_ticker := time.NewTicker(types.SecondsPerSlot)
		// defer service_ticker.Stop()

		new_service_found := false

		fmt.Printf("Waiting for %s service to be ready...\n", serviceName)
		for !new_service_found {
			// select {
			// case <-service_ticker.C:
			//TODO: find service_index ... can be serviced via CE129 using stateKey=00b5000000000000ffffffffffffffffffffffffffffffffffffffffffffffff
			stateDB := builderNode.getState()
			if stateDB != nil && stateDB.Block != nil {
				// fmt.Printf("finding newservice from bootstrapService.s[0]...\n")
				stateRoot := stateDB.Block.GetHeader().ParentStateRoot
				t, _ := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), builderNode.store)

				k := []byte{0, 0, 0, 0}
				key := common.Compute_storageKey_internal(bootstrapService, k)
				service_account_byte, err := t.GetServiceStorage(bootstrapService, key)
				if err != nil {
					//fmt.Printf("t.GetServiceStorage %v %v\n", key, err)
					//not ready yet ...
					time.Sleep(1 * time.Second)
					continue
				} else {
					//fmt.Printf("t.GetServiceStorage %v FOUND  %v\n", key, service_account_byte)
				}
				time.Sleep(1 * time.Second)
				decoded_new_service_idx := uint32(types.DecodeE_l(service_account_byte))
				if decoded_new_service_idx != 0 && (decoded_new_service_idx != previous_service_idx) {
					service.ServiceCode = decoded_new_service_idx
					new_service_idx = decoded_new_service_idx
					//TODO: now use new_service_idx and see if (c,l) is correct
					new_service_found = true
					previous_service_idx = decoded_new_service_idx
					fmt.Printf("%s Service Index: %v [key=%v]\n", serviceName, service.ServiceCode, key)

					for validatorIdx, _ := range nodes {
						if validatorIdx != builderIdx {
							if new_service_idx > 0 {
								// fmt.Printf("Sending new service_idx %v service.CodeHash %v, len(service.Code)=%v\n", new_service_idx, service.CodeHash, len(service.Code))
								err = builderNode.peersInfo[uint16(validatorIdx)].SendPreimageAnnouncement(new_service_idx, service.CodeHash, uint32(len(service.Code)))
								if err != nil {
									fmt.Printf("SendPreimageAnnouncement ERR %v\n", err)
								}
							}
						}
					}
				}

			}
			// }
		}
	}

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	//----------------------------------------------
	time.Sleep(30 * time.Second)
	fmt.Printf("Start FIB\n")

	// n1 := nodes[1]
	n4 := nodes[4]
	core := 0
	service0 := testServices["fib"]
	service1 := testServices["tribonacci"]
	serviceM := testServices["megatron"]
	fmt.Printf("service0: %v, codehash: %v\n", service0.ServiceCode, service0.CodeHash)
	fmt.Printf("service1: %v, codehash: %v\n", service1.ServiceCode, service1.CodeHash)
	fmt.Printf("serviceM: %v, codehash: %v\n", serviceM.ServiceCode, serviceM.CodeHash)
	FibWorkPackages := make([]types.WorkPackage, 0)
	FibPackageHashes := make([]common.Hash, 0)
	prevWorkPackageHash := common.Hash{}
	for megaN := 1; megaN < 21; megaN++ {
		importedSegments := make([]types.ImportSegment, 0)
		// importedSegmentsM := make([]types.ImportSegment, 0)
		refineContext := types.RefineContext{
			// These values don't matter until we have a historical lookup -- which we do not!
			Anchor:           common.Hash{},
			StateRoot:        common.Hash{},
			BeefyRoot:        common.Hash{},
			LookupAnchor:     common.Hash{},
			LookupAnchorSlot: 0,
		}

		if megaN > 1 {
			// TODO: Sean
			//			prerequisite := types.Prerequisite{prevWorkPackageHash.Bytes()}
			//			refineContext.Prerequisite = &common.Hash{}
			importedSegments = append(importedSegments, types.ImportSegment{
				WorkPackageHash: prevWorkPackageHash,
				Index:           0,
			})
			// importedSegments = append(importedSegments, types.ImportSegment{
			// 	WorkPackageHash: prevWorkPackageHash,
			// 	Index:           1, // TODO: check
			// })
		}

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(megaN))
		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // TODO: set up null-authorizer
			// AuthCodeHost:  serviceM.ServiceCode,
			AuthCodeHost:  service0.ServiceCode,
			Authorizer:    types.Authorizer{},
			RefineContext: refineContext,
			WorkItems: []types.WorkItem{
				{
					Service:          service0.ServiceCode,
					CodeHash:         service0.CodeHash,
					Payload:          payload,
					GasLimit:         10000000,
					ImportedSegments: importedSegments,
					ExportCount:      1,
				},
				// {
				// 	Service:          service1.ServiceCode,
				// 	CodeHash:         service1.CodeHash,
				// 	Payload:          payload,
				// 	GasLimit:         10000000,
				// 	ImportedSegments: importedSegments,
				// 	ExportCount:      1,
				// },
				// {
				// 	Service:          serviceM.ServiceCode,
				// 	CodeHash:         serviceM.CodeHash,
				// 	Payload:          payloadM,
				// 	GasLimit:         10000000,
				// 	ImportedSegments: importedSegmentsM,
				// 	ExportCount:      0,
				// },
			},
		}

		workPackageHash := workPackage.Hash()
		//fmt.Println("WorkPackageHash:", workPackageHash)

		FibWorkPackages = append(FibWorkPackages, workPackage)
		FibPackageHashes = append(FibPackageHashes, workPackageHash)

		prevWorkPackageHash = workPackageHash
	}

	// for i := 0; i < len(FibWorkPackages)-1; i++ {
	// 	prerequisite := types.Prerequisite(FibPackageHashes[i+1])
	// 	FibWorkPackages[i].RefineContext.Prerequisite = &prerequisite
	// }

	for i, workPackage := range FibWorkPackages {
		megaN := i + 1
		workPackageHash := FibPackageHashes[i]

		// Update guarantors before each submission if necessary
		for _, n := range nodes {
			n.statedb.PreviousGuarantors(true)
			n.statedb.AssignGuarantors(true)
		}
		fmt.Printf("\n** \033[36m MEGATRON %d \033[0m workPackage: %v **\n", megaN, common.Str(workPackageHash))
		// CE133_WorkPackageSubmission: n1 => n4
		// v1, v2, v4 => core
		// random select 1 sender and 1 receiver
		senderIdx := rand.Intn(3)
		receiverIdx := rand.Intn(3)
		for senderIdx == receiverIdx {
			receiverIdx = rand.Intn(3)
		}
		if senderIdx == 0 {
			senderIdx = 1
		} else if senderIdx == 1 {
			senderIdx = 2
		} else {
			senderIdx = 4
		}
		if receiverIdx == 0 {
			receiverIdx = 1
		} else if receiverIdx == 1 {
			receiverIdx = 2
		} else {
			receiverIdx = 4
		}
		err := nodes[senderIdx].peersInfo[uint16(receiverIdx)].SendWorkPackageSubmission(0, workPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v, sender:%d, receiver %d\n", err, senderIdx, receiverIdx)
		}

		// Wait until the work report is pending
		var workReport types.WorkReport
		// audit := false
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				// audit = true
				break
			}
		}

		// Wait until the work report is cleared
		for {
			if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		// if audit {
		// 	for _, n := range nodes {
		// 		n.Audit()
		// 	}
		// }
		time.Sleep(15 * time.Second)
		prevWorkPackageHash = workPackageHash
	}
	http.ListenAndServe("localhost:6060", nil)
}

func TestWorkGuaranteeTRIB(t *testing.T) {
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

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	// code length: 206
	bootstrapCode, err := os.ReadFile(statedb.BootstrapServiceFile)
	if err != nil {
		panic(0)
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)

	builderIdx := 1
	builderNode := nodes[builderIdx]
	builderNode.preimages[bootstrapCodeHash] = bootstrapCode
	new_service_idx := uint32(0)

	// Load testServices
	testServices, err := getServices([]string{"fib", "tribonacci", "megatron"}) // "padovan", "pell", "racaman",
	if err != nil {
		panic(32)
	}

	// set builderNode's primages map
	for _, service := range testServices {
		builderNode.preimages[service.CodeHash] = service.Code
	}

	var previous_service_idx uint32
	for serviceName, service := range testServices {
		fmt.Printf("Builder storing TestService %s (%x)\n", serviceName, service.CodeHash)
		// set up service using the Bootstrap service
		codeWorkPackage := types.WorkPackage{
			Authorization: []byte(""),
			AuthCodeHost:  bootstrapService,
			Authorizer:    types.Authorizer{},
			RefineContext: types.RefineContext{},
			WorkItems: []types.WorkItem{
				{
					Service:          bootstrapService,
					CodeHash:         bootstrapCodeHash,
					Payload:          append(service.CodeHash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(service.Code)))...),
					GasLimit:         10000000,
					ImportedSegments: make([]types.ImportSegment, 0),
					ExportCount:      0,
				},
			},
		}
		err = builderNode.peersInfo[4].SendWorkPackageSubmission(0, codeWorkPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}

		// service_ticker := time.NewTicker(types.SecondsPerSlot)
		// defer service_ticker.Stop()

		new_service_found := false

		fmt.Printf("Waiting for %s service to be ready...\n", serviceName)
		for !new_service_found {
			// select {
			// case <-service_ticker.C:
			//TODO: find service_index ... can be serviced via CE129 using stateKey=00b5000000000000ffffffffffffffffffffffffffffffffffffffffffffffff
			stateDB := builderNode.getState()
			if stateDB != nil && stateDB.Block != nil {
				// fmt.Printf("finding newservice from bootstrapService.s[0]...\n")
				stateRoot := stateDB.Block.GetHeader().ParentStateRoot
				t, _ := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), builderNode.store)

				k := []byte{0, 0, 0, 0}
				key := common.Compute_storageKey_internal(bootstrapService, k)
				service_account_byte, err := t.GetServiceStorage(bootstrapService, key)
				if err != nil {
					//fmt.Printf("t.GetServiceStorage %v %v\n", key, err)
					//not ready yet ...
					time.Sleep(1 * time.Second)
					continue
				} else {
					fmt.Printf("t.GetServiceStorage %v FOUND  %v\n", key, service_account_byte)
				}
				time.Sleep(1 * time.Second)
				decoded_new_service_idx := uint32(types.DecodeE_l(service_account_byte))
				if decoded_new_service_idx != 0 && (decoded_new_service_idx != previous_service_idx) {
					service.ServiceCode = decoded_new_service_idx
					new_service_idx = decoded_new_service_idx
					//TODO: now use new_service_idx and see if (c,l) is correct
					fmt.Printf("%s Service Index: %v\n", serviceName, service.ServiceCode)
					new_service_found = true
					previous_service_idx = decoded_new_service_idx
					fmt.Printf("t.GetServiceStorage %v FOUND  %v\n", key, service_account_byte)

					for validatorIdx, _ := range nodes {
						if validatorIdx != builderIdx {
							if new_service_idx > 0 {
								fmt.Printf("Sending new service_idx %v service.CodeHash %v, len(service.Code)=%v\n", new_service_idx, service.CodeHash, len(service.Code))
								err = builderNode.peersInfo[uint16(validatorIdx)].SendPreimageAnnouncement(new_service_idx, service.CodeHash, uint32(len(service.Code)))
								if err != nil {
									fmt.Printf("SendPreimageAnnouncement ERR %v\n", err)
								}
							}
						}
					}
				}

			}
			// }
		}
	}

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	//----------------------------------------------
	time.Sleep(30 * time.Second)
	fmt.Printf("Start FIB\n")

	// n1 := nodes[1]
	n4 := nodes[4]
	core := 0
	service0 := testServices["fib"]
	service1 := testServices["tribonacci"]
	serviceM := testServices["megatron"]
	fmt.Printf("service0: %v, codehash: %v\n", service0.ServiceCode, service0.CodeHash)
	fmt.Printf("service1: %v, codehash: %v\n", service1.ServiceCode, service1.CodeHash)
	fmt.Printf("serviceM: %v, codehash: %v\n", serviceM.ServiceCode, serviceM.CodeHash)
	FibWorkPackages := make([]types.WorkPackage, 0)
	FibPackageHashes := make([]common.Hash, 0)
	prevWorkPackageHash := common.Hash{}
	for megaN := 1; megaN < 21; megaN++ {
		importedSegments := make([]types.ImportSegment, 0)
		// importedSegmentsM := make([]types.ImportSegment, 0)
		refineContext := types.RefineContext{
			// These values don't matter until we have a historical lookup -- which we do not!
			Anchor:           common.Hash{},
			StateRoot:        common.Hash{},
			BeefyRoot:        common.Hash{},
			LookupAnchor:     common.Hash{},
			LookupAnchorSlot: 0,
		}

		if megaN > 1 {
			// TODO: Sean
			//			prerequisite := types.Prerequisite{prevWorkPackageHash.Bytes()}
			//			refineContext.Prerequisite = &common.Hash{}
			importedSegments = append(importedSegments, types.ImportSegment{
				WorkPackageHash: prevWorkPackageHash,
				Index:           0,
			})
			// importedSegments = append(importedSegments, types.ImportSegment{
			// 	WorkPackageHash: prevWorkPackageHash,
			// 	Index:           1, // TODO: check
			// })
		}

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(megaN))
		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // TODO: set up null-authorizer
			// AuthCodeHost:  serviceM.ServiceCode,
			AuthCodeHost:  service0.ServiceCode,
			Authorizer:    types.Authorizer{},
			RefineContext: refineContext,
			WorkItems: []types.WorkItem{
				// {
				// 	Service:          service0.ServiceCode,
				// 	CodeHash:         service0.CodeHash,
				// 	Payload:          payload,
				// 	GasLimit:         10000000,
				// 	ImportedSegments: importedSegments,
				// 	ExportCount:      1,
				// },
				{
					Service:          service1.ServiceCode,
					CodeHash:         service1.CodeHash,
					Payload:          payload,
					GasLimit:         10000000,
					ImportedSegments: importedSegments,
					ExportCount:      1,
				},
				// {
				// 	Service:          serviceM.ServiceCode,
				// 	CodeHash:         serviceM.CodeHash,
				// 	Payload:          payloadM,
				// 	GasLimit:         10000000,
				// 	ImportedSegments: importedSegmentsM,
				// 	ExportCount:      0,
				// },
			},
		}

		workPackageHash := workPackage.Hash()
		//fmt.Println("WorkPackageHash:", workPackageHash)

		FibWorkPackages = append(FibWorkPackages, workPackage)
		FibPackageHashes = append(FibPackageHashes, workPackageHash)

		prevWorkPackageHash = workPackageHash
	}

	// for i := 0; i < len(FibWorkPackages)-1; i++ {
	// 	prerequisite := types.Prerequisite(FibPackageHashes[i+1])
	// 	FibWorkPackages[i].RefineContext.Prerequisite = &prerequisite
	// }

	for i, workPackage := range FibWorkPackages {
		megaN := i + 1
		workPackageHash := FibPackageHashes[i]

		// Update guarantors before each submission if necessary
		for _, n := range nodes {
			n.statedb.PreviousGuarantors(true)
			n.statedb.AssignGuarantors(true)
		}
		fmt.Printf("\n** \033[36m MEGATRON %d \033[0m workPackage: %v **\n", megaN, common.Str(workPackageHash))
		// CE133_WorkPackageSubmission: n1 => n4
		// v1, v2, v4 => core
		// random select 1 sender and 1 receiver
		senderIdx := rand.Intn(3)
		receiverIdx := rand.Intn(3)
		for senderIdx == receiverIdx {
			receiverIdx = rand.Intn(3)
		}
		if senderIdx == 0 {
			senderIdx = 1
		} else if senderIdx == 1 {
			senderIdx = 2
		} else {
			senderIdx = 4
		}
		if receiverIdx == 0 {
			receiverIdx = 1
		} else if receiverIdx == 1 {
			receiverIdx = 2
		} else {
			receiverIdx = 4
		}
		err := nodes[senderIdx].peersInfo[uint16(receiverIdx)].SendWorkPackageSubmission(0, workPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v, sender:%d, receiver %d\n", err, senderIdx, receiverIdx)
		}

		// Wait until the work report is pending
		var workReport types.WorkReport
		// audit := false
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				// audit = true
				break
			}
		}

		// Wait until the work report is cleared
		for {
			if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		// if audit {
		// 	for _, n := range nodes {
		// 		n.Audit()
		// 	}
		// }
		time.Sleep(15 * time.Second)
		prevWorkPackageHash = workPackageHash
	}
	http.ListenAndServe("localhost:6060", nil)
}

func TestWorkGuaranteeFIBTRIB(t *testing.T) {
	genesisConfig, peers, peerList, validatorSecrets, nodePaths, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error Setting up nodes: %v\n", err)
	}

	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint16(i), validatorSecrets[i], &genesisConfig, peers, peerList, ValidatorFlag, nodePaths[i], basePort+i)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		nodes[i] = node
	}

	// Wait for nodes to initialize
	for {
		time.Sleep(1 * time.Second)
		if nodes[0].statedb.GetSafrole().CheckFirstPhaseReady() {
			break
		}
	}

	// Assign guarantors
	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}

	// Read bootstrap code
	bootstrapCode, err := os.ReadFile(statedb.BootstrapServiceFile)
	if err != nil {
		panic(0)
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)

	builderIdx := 1
	builderNode := nodes[builderIdx]
	builderNode.preimages[bootstrapCodeHash] = bootstrapCode
	new_service_idx := uint32(0)

	// Load testServices
	// testServices, err := getServices([]string{"fib", "tribonacci", "megatron"})
	testServices, err := getServices([]string{"fib"})
	if err != nil {
		panic(32)
	}

	// Set builderNode's preimages map
	for _, service := range testServices {
		builderNode.preimages[service.CodeHash] = service.Code
	}

	var previous_service_idx uint32
	for serviceName, service := range testServices {
		fmt.Printf("Builder storing TestService %s (%x)\n", serviceName, service.CodeHash)
		// Set up service using the Bootstrap service
		codeWorkPackage := types.WorkPackage{
			Authorization: []byte(""),
			AuthCodeHost:  bootstrapService,
			Authorizer:    types.Authorizer{},
			RefineContext: types.RefineContext{},
			WorkItems: []types.WorkItem{
				{
					Service:          bootstrapService,
					CodeHash:         bootstrapCodeHash,
					Payload:          append(service.CodeHash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(service.Code)))...),
					GasLimit:         10000000,
					ImportedSegments: make([]types.ImportSegment, 0),
					ExportCount:      0,
				},
			},
		}
		err = builderNode.peersInfo[4].SendWorkPackageSubmission(0, codeWorkPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}

		new_service_found := false
		fmt.Printf("Waiting for %s service to be ready...\n", serviceName)
		for !new_service_found {
			stateDB := builderNode.getState()
			if stateDB != nil && stateDB.Block != nil {
				stateRoot := stateDB.Block.GetHeader().ParentStateRoot
				t, _ := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), builderNode.store)

				k := []byte{0, 0, 0, 0}
				key := common.Compute_storageKey_internal(bootstrapService, k)
				service_account_byte, err := t.GetServiceStorage(bootstrapService, key)
				if err != nil {
					time.Sleep(1 * time.Second)
					continue
				}
				time.Sleep(1 * time.Second)
				decoded_new_service_idx := uint32(types.DecodeE_l(service_account_byte))
				if decoded_new_service_idx != 0 && (decoded_new_service_idx != previous_service_idx) {
					service.ServiceCode = decoded_new_service_idx
					new_service_idx = decoded_new_service_idx
					fmt.Printf("%s Service Index: %v\n", serviceName, service.ServiceCode)
					new_service_found = true
					previous_service_idx = decoded_new_service_idx
					fmt.Printf("t.GetServiceStorage %v FOUND  %v\n", key, service_account_byte)

					for validatorIdx := range nodes {
						if validatorIdx != builderIdx {
							if new_service_idx > 0 {
								fmt.Printf("Sending new service_idx %v service.CodeHash %v, len(service.Code)=%v\n", new_service_idx, service.CodeHash, len(service.Code))
								err = builderNode.peersInfo[uint16(validatorIdx)].SendPreimageAnnouncement(new_service_idx, service.CodeHash, uint32(len(service.Code)))
								if err != nil {
									fmt.Printf("SendPreimageAnnouncement ERR %v\n", err)
								}
							}
						}
					}
				}
			}
		}
	}

	// Assign guarantors again
	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}

	// Wait before starting execution
	time.Sleep(30 * time.Second)
	fmt.Printf("Start alternating FIB and TRIB execution\n")

	n4 := nodes[4]
	core := 0
	serviceFIB := testServices["fib"]
	serviceTRIB := testServices["tribonacci"]
	fmt.Printf("serviceFIB: %v, codehash: %v\n", serviceFIB.ServiceCode, serviceFIB.CodeHash)
	fmt.Printf("serviceTRIB: %v, codehash: %v\n", serviceTRIB.ServiceCode, serviceTRIB.CodeHash)

	// Prepare work packages for FIB and TRIB
	totalIterations := 20
	prevWorkPackageHashFIB := common.Hash{}
	prevWorkPackageHashTRIB := common.Hash{}

	for i := 1; i <= totalIterations; i++ {
		// Decide which service to execute based on iteration
		var currentService *types.TestService
		var prevWorkPackageHash common.Hash
		var serviceName string
		var iterationNumber int

		if i%2 == 1 {
			currentService = serviceFIB
			prevWorkPackageHash = prevWorkPackageHashFIB
			serviceName = "FIB"
			iterationNumber = (i + 1) / 2
		} else {
			currentService = serviceTRIB
			prevWorkPackageHash = prevWorkPackageHashTRIB
			serviceName = "TRIB"
			iterationNumber = i / 2
		}

		importedSegments := make([]types.ImportSegment, 0)
		refineContext := types.RefineContext{
			Anchor:           common.Hash{},
			StateRoot:        common.Hash{},
			BeefyRoot:        common.Hash{},
			LookupAnchor:     common.Hash{},
			LookupAnchorSlot: 0,
		}

		// Only add imported segments if previous hash is non-zero
		if iterationNumber > 1 {
			importedSegments = append(importedSegments, types.ImportSegment{
				WorkPackageHash: prevWorkPackageHash,
				Index:           0,
			})
		}

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(iterationNumber))
		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // Null-authorizer
			AuthCodeHost:  currentService.ServiceCode,
			Authorizer:    types.Authorizer{},
			RefineContext: refineContext,
			WorkItems: []types.WorkItem{
				{
					Service:          currentService.ServiceCode,
					CodeHash:         currentService.CodeHash,
					Payload:          payload,
					GasLimit:         10000000,
					ImportedSegments: importedSegments,
					ExportCount:      1,
				},
			},
		}

		workPackageHash := workPackage.Hash()
		fmt.Printf("\n** \033[36m %s Iteration %d \033[0m workPackage: %v **\n", serviceName, iterationNumber, common.Str(workPackageHash))

		// Update guarantors before each submission
		for _, n := range nodes {
			n.statedb.PreviousGuarantors(true)
			n.statedb.AssignGuarantors(true)
		}

		// Randomly select sender and receiver
		senderIdx := rand.Intn(3)
		receiverIdx := rand.Intn(3)
		for senderIdx == receiverIdx {
			receiverIdx = rand.Intn(3)
		}
		nodeIndices := []int{1, 2, 4}
		senderIdx = nodeIndices[senderIdx]
		receiverIdx = nodeIndices[receiverIdx]

		err := nodes[senderIdx].peersInfo[uint16(receiverIdx)].SendWorkPackageSubmission(0, workPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v, sender:%d, receiver %d\n", err, senderIdx, receiverIdx)
		}

		// Wait until the work report is pending
		var workReport types.WorkReport
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				break
			}
		}

		// Wait until the work report is cleared
		for {
			if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}

		// Update previous work package hash
		if i%2 == 1 {
			prevWorkPackageHashFIB = workPackageHash
		} else {
			prevWorkPackageHashTRIB = workPackageHash
		}

		// Wait before next iteration
		time.Sleep(15 * time.Second)
	}

	http.ListenAndServe("localhost:6060", nil)
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

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	// code length: 206
	bootstrapCode, err := os.ReadFile(statedb.BootstrapServiceFile)
	if err != nil {
		panic(0)
	}
	bootstrapService := uint32(statedb.BootstrapServiceCode)
	bootstrapCodeHash := common.Blake2Hash(bootstrapCode)

	builderIdx := 1
	builderNode := nodes[builderIdx]
	builderNode.preimages[bootstrapCodeHash] = bootstrapCode
	new_service_idx := uint32(0)

	// Load testServices
	testServices, err := getServices([]string{"fib", "tribonacci", "megatron"}) // "padovan", "pell", "racaman",
	if err != nil {
		panic(32)
	}

	// set builderNode's primages map
	for _, service := range testServices {
		builderNode.preimages[service.CodeHash] = service.Code
	}

	var previous_service_idx uint32
	for serviceName, service := range testServices {
		fmt.Printf("Builder storing TestService %s (%x)\n", serviceName, service.CodeHash)
		// set up service using the Bootstrap service
		codeWorkPackage := types.WorkPackage{
			Authorization: []byte(""),
			AuthCodeHost:  bootstrapService,
			Authorizer:    types.Authorizer{},
			RefineContext: types.RefineContext{},
			WorkItems: []types.WorkItem{
				{
					Service:          bootstrapService,
					CodeHash:         bootstrapCodeHash,
					Payload:          append(service.CodeHash.Bytes(), binary.LittleEndian.AppendUint32(nil, uint32(len(service.Code)))...),
					GasLimit:         10000000,
					ImportedSegments: make([]types.ImportSegment, 0),
					ExportCount:      0,
				},
			},
		}
		err = builderNode.peersInfo[4].SendWorkPackageSubmission(0, codeWorkPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}

		// service_ticker := time.NewTicker(types.SecondsPerSlot)
		// defer service_ticker.Stop()

		new_service_found := false

		fmt.Printf("Waiting for %s service to be ready...\n", serviceName)
		for !new_service_found {
			// select {
			// case <-service_ticker.C:
			//TODO: find service_index ... can be serviced via CE129 using stateKey=00b5000000000000ffffffffffffffffffffffffffffffffffffffffffffffff
			stateDB := builderNode.getState()
			if stateDB != nil && stateDB.Block != nil {
				// fmt.Printf("finding newservice from bootstrapService.s[0]...\n")
				stateRoot := stateDB.Block.GetHeader().ParentStateRoot
				t, _ := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), builderNode.store)

				k := []byte{0, 0, 0, 0}
				key := common.Compute_storageKey_internal(bootstrapService, k)
				service_account_byte, err := t.GetServiceStorage(bootstrapService, key)
				if err != nil {
					//fmt.Printf("t.GetServiceStorage %v %v\n", key, err)
					//not ready yet ...
					time.Sleep(1 * time.Second)
					continue
				}
				time.Sleep(1 * time.Second)
				decoded_new_service_idx := uint32(types.DecodeE_l(service_account_byte))
				if decoded_new_service_idx != 0 && (decoded_new_service_idx != previous_service_idx) {
					service.ServiceCode = decoded_new_service_idx
					new_service_idx = decoded_new_service_idx
					//TODO: now use new_service_idx and see if (c,l) is correct
					fmt.Printf("%s Service Index: %v\n", serviceName, service.ServiceCode)
					new_service_found = true
					previous_service_idx = decoded_new_service_idx
					fmt.Printf("t.GetServiceStorage %v FOUND  %v\n", key, service_account_byte)

					for validatorIdx, _ := range nodes {
						if validatorIdx != builderIdx {
							if new_service_idx > 0 {
								fmt.Printf("Sending new service_idx %v service.CodeHash %v, len(service.Code)=%v\n", new_service_idx, service.CodeHash, len(service.Code))
								err = builderNode.peersInfo[uint16(validatorIdx)].SendPreimageAnnouncement(new_service_idx, service.CodeHash, uint32(len(service.Code)))
								if err != nil {
									fmt.Printf("SendPreimageAnnouncement ERR %v\n", err)
								}
							}
						}
					}
				}

			}
			// }
		}
	}

	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}
	//----------------------------------------------
	time.Sleep(30 * time.Second)
	fmt.Printf("Start FIB\n")

	// n1 := nodes[1]
	n4 := nodes[4]
	core := 0
	service0 := testServices["fib"]
	service1 := testServices["tribonacci"]
	serviceM := testServices["megatron"]
	fmt.Printf("service0: %v, codehash: %v\n", service0.ServiceCode, service0.CodeHash)
	fmt.Printf("service1: %v, codehash: %v\n", service1.ServiceCode, service1.CodeHash)
	fmt.Printf("serviceM: %v, codehash: %v\n", serviceM.ServiceCode, serviceM.CodeHash)
	FibWorkPackages := make([]types.WorkPackage, 0)
	FibPackageHashes := make([]common.Hash, 0)
	prevWorkPackageHash := common.Hash{}
	for megaN := 1; megaN < 21; megaN++ {
		importedSegments := make([]types.ImportSegment, 0)
		// importedSegmentsM := make([]types.ImportSegment, 0)
		refineContext := types.RefineContext{
			// These values don't matter until we have a historical lookup -- which we do not!
			Anchor:           common.Hash{},
			StateRoot:        common.Hash{},
			BeefyRoot:        common.Hash{},
			LookupAnchor:     common.Hash{},
			LookupAnchorSlot: 0,
		}

		if megaN > 1 {
			// TODO: Sean
			//			prerequisite := types.Prerequisite{prevWorkPackageHash.Bytes()}
			//			refineContext.Prerequisite = &common.Hash{}
			importedSegments = append(importedSegments, types.ImportSegment{
				WorkPackageHash: prevWorkPackageHash,
				Index:           0,
			})
			importedSegments = append(importedSegments, types.ImportSegment{
				WorkPackageHash: prevWorkPackageHash,
				Index:           1, // TODO: check
			})
		}

		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(megaN))
		payloadM := make([]byte, 8)
		binary.LittleEndian.PutUint32(payloadM[0:4], uint32(service0.ServiceCode))
		binary.LittleEndian.PutUint32(payloadM[4:8], uint32(service1.ServiceCode))
		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // TODO: set up null-authorizer
			AuthCodeHost:  serviceM.ServiceCode,
			Authorizer:    types.Authorizer{},
			RefineContext: refineContext,
			WorkItems: []types.WorkItem{
				{
					Service:          service0.ServiceCode,
					CodeHash:         service0.CodeHash,
					Payload:          payload,
					GasLimit:         10000000,
					ImportedSegments: importedSegments,
					ExportCount:      1,
				},
				{
					Service:          service1.ServiceCode,
					CodeHash:         service1.CodeHash,
					Payload:          payload,
					GasLimit:         10000000,
					ImportedSegments: importedSegments,
					ExportCount:      1,
				},
				// {
				// 	Service:          serviceM.ServiceCode,
				// 	CodeHash:         serviceM.CodeHash,
				// 	Payload:          payloadM,
				// 	GasLimit:         10000000,
				// 	ImportedSegments: importedSegmentsM,
				// 	ExportCount:      0,
				// },
			},
		}

		workPackageHash := workPackage.Hash()
		//fmt.Println("WorkPackageHash:", workPackageHash)

		FibWorkPackages = append(FibWorkPackages, workPackage)
		FibPackageHashes = append(FibPackageHashes, workPackageHash)

		prevWorkPackageHash = workPackageHash
	}

	// for i := 0; i < len(FibWorkPackages)-1; i++ {
	// 	prerequisite := types.Prerequisite(FibPackageHashes[i+1])
	// 	FibWorkPackages[i].RefineContext.Prerequisite = &prerequisite
	// }

	for i, workPackage := range FibWorkPackages {
		megaN := i + 1
		workPackageHash := FibPackageHashes[i]

		// Update guarantors before each submission if necessary
		for _, n := range nodes {
			n.statedb.PreviousGuarantors(true)
			n.statedb.AssignGuarantors(true)
		}
		fmt.Printf("\n** \033[36m MEGATRON %d \033[0m workPackage: %v **\n", megaN, common.Str(workPackageHash))
		// CE133_WorkPackageSubmission: n1 => n4
		// v1, v2, v4 => core
		// random select 1 sender and 1 receiver
		senderIdx := rand.Intn(3)
		receiverIdx := rand.Intn(3)
		for senderIdx == receiverIdx {
			receiverIdx = rand.Intn(3)
		}
		if senderIdx == 0 {
			senderIdx = 1
		} else if senderIdx == 1 {
			senderIdx = 2
		} else {
			senderIdx = 4
		}
		if receiverIdx == 0 {
			receiverIdx = 1
		} else if receiverIdx == 1 {
			receiverIdx = 2
		} else {
			receiverIdx = 4
		}
		err := nodes[senderIdx].peersInfo[uint16(receiverIdx)].SendWorkPackageSubmission(0, workPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v, sender:%d, receiver %d\n", err, senderIdx, receiverIdx)
		}

		// Wait until the work report is pending
		var workReport types.WorkReport
		// audit := false
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
				rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
				workReport = rho_state.WorkReport
				fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				// audit = true
				break
			}
		}

		// Wait until the work report is cleared
		for {
			if n4.statedb.JamState.AvailabilityAssignments[core] == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		// if audit {
		// 	for _, n := range nodes {
		// 		n.Audit()
		// 	}
		// }
		time.Sleep(15 * time.Second)
		prevWorkPackageHash = workPackageHash
	}
	http.ListenAndServe("localhost:6060", nil)
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
