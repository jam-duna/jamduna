package node

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	_ "net/http/pprof"
	"os"
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

func testSafrole(t *testing.T, sendtickets bool) {
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
		node.SetSendTickets(sendtickets)
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

func testJAM(t *testing.T, jam string) {
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
	serviceNames := []string{"fib"}
	if jam == "megatron" {
		serviceNames = []string{"fib", "tribonacci", "megatron"} // Others include: "padovan", "pell", "racaman"
	}
	testServices, err := getServices(serviceNames)
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
					//fmt.Printf("t.GetServiceStorage %v FOUND  %v\n", key, service_account_byte)

					err = builderNode.BroadcastPreimageAnnouncement(new_service_idx, service.CodeHash, uint32(len(service.Code)), service.Code)
					if err != nil {
						fmt.Printf("BroadcastPreimageAnnouncement ERR %v\n", err)
					}
				}

			}
		}
	}
	for _, n := range nodes {
		n.statedb.PreviousGuarantors(true)
		n.statedb.AssignGuarantors(true)
	}

	fmt.Printf("All services are ready, Send preimage announcement\n")
	for _, service := range testServices {
		builderNode.BroadcastPreimageAnnouncement(service.ServiceCode, service.CodeHash, uint32(len(service.Code)), service.Code)
	}
	fmt.Printf("Wait until all the preimage blobs are ready\n")

	for done := false; !done; {

		ready := 0
		nservices := 0
		for _, service := range testServices {
			for _, n := range nodes {
				targetStateDB := n.getState()
				if targetStateDB != nil {
					code := targetStateDB.ReadServicePreimageBlob(service.ServiceCode, service.CodeHash)
					if len(code) > 0 && bytes.Equal(code, service.Code) {
						ready++
					}
					// fmt.Printf(" check %s len(code)=%d expect %d => ready=%d\n", service.CodeHash, len(code), len(service.Code), ready)
				}
			}
			nservices++
		}
		if ready == types.TotalValidators*nservices {
			done = true
		} else {
			time.Sleep(1 * time.Second)
		}
	}

	switch jam {
	case "megatron":
		testMegatron(t, nodes, testServices)
	case "fib":
		testFib(t, nodes, testServices)
	}
}

func testFib(t *testing.T, nodes []*Node, testServices map[string]*types.TestService) {
	fmt.Printf("Start FIB\n")
	service0 := testServices["fib"]
	n1 := nodes[1]
	n4 := nodes[4]
	core := 0
	prevWorkPackageHash := common.Hash{}
	for fibN := 1; fibN <= 10; fibN++ {
		importedSegments := make([]types.ImportSegment, 0)
		if fibN > 1 {
			importedSegment := types.ImportSegment{
				WorkPackageHash: prevWorkPackageHash,
				Index:           0,
			}
			importedSegments = append(importedSegments, importedSegment)
		}
		refine_context := types.RefineContext{}
		payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(payload, uint32(fibN))
		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // TODO: set up null-authorizer
			Authorizer:    types.Authorizer{},
			RefineContext: refine_context,
			WorkItems: []types.WorkItem{
				{
					Service:          service0.ServiceCode,
					CodeHash:         service0.CodeHash,
					Payload:          payload,
					GasLimit:         10000000,
					ImportedSegments: importedSegments,
					ExportCount:      1,
				},
			},
		}
		workPackageHash := workPackage.Hash()

		fmt.Printf("\n** \033[36m FIB=%v \033[0m workPackage: %v **\n", fibN, common.Str(workPackageHash))
		err := n1.peersInfo[4].SendWorkPackageSubmission(0, workPackage, []byte{})
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v\n", err)
		}
		// wait until the work report is pending
		for {
			time.Sleep(1 * time.Second)
			if n4.statedb.JamState.AvailabilityAssignments[core] != nil {
				if false {
					var workReport types.WorkReport
					rho_state := n4.statedb.JamState.AvailabilityAssignments[core]
					workReport = rho_state.WorkReport
					fmt.Printf(" expecting to audit %v\n", workReport.Hash())
				}
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
		prevWorkPackageHash = workPackageHash
	}

}

func testMegatron(t *testing.T, nodes []*Node, testServices map[string]*types.TestService) {
	fmt.Printf("Start FIB\n")
	service0 := testServices["fib"]
	service1 := testServices["tribonacci"]
	serviceM := testServices["megatron"]
	fmt.Printf("service0: %v, codehash: %v\n", service0.ServiceCode, service0.CodeHash)
	fmt.Printf("service1: %v, codehash: %v\n", service1.ServiceCode, service1.CodeHash)
	fmt.Printf("serviceM: %v, codehash: %v\n", serviceM.ServiceCode, serviceM.CodeHash)
	Fib_Trib_WorkPackages := make([]types.WorkPackage, 0)
	Meg_WorkPackages := make([]types.WorkPackage, 0)
	prevWorkPackageHash := common.Hash{}
	// ================================================
	// make 20 workpackages for Fib and Trib
	for n := 0; n < 6; n++ {
		importedSegments := make([]types.ImportSegment, 0)

		refineContext := types.RefineContext{
			// These values don't matter until we have a historical lookup -- which we do not!
			Anchor:           common.Hash{},
			StateRoot:        common.Hash{},
			BeefyRoot:        common.Hash{},
			LookupAnchor:     common.Hash{},
			LookupAnchorSlot: 0,
			Prerequisites:    []common.Hash{},
		}

		if n > 0 {
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
		binary.LittleEndian.PutUint32(payload, uint32(n+1))
		workPackage := types.WorkPackage{
			Authorization: []byte("0x"), // TODO: set up null-authorizer
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
			},
		}
		Fib_Trib_WorkPackages = append(Fib_Trib_WorkPackages, workPackage)
		workPackageHash := workPackage.Hash()
		prevWorkPackageHash = workPackageHash
	}
	// =================================================
	// make 20 workpackages for Megatron
	for megaN := 0; megaN < 6; megaN++ {
		importedSegmentsM := make([]types.ImportSegment, 0)
		prereq := make([]common.Hash, 0)
		prereq = append(prereq, Fib_Trib_WorkPackages[megaN].Hash())
		refineContext := types.RefineContext{
			// These values don't matter until we have a historical lookup -- which we do not!
			Anchor:           common.Hash{},
			StateRoot:        common.Hash{},
			BeefyRoot:        common.Hash{},
			LookupAnchor:     common.Hash{},
			LookupAnchorSlot: 1,
			Prerequisites:    prereq,
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
					Service:          serviceM.ServiceCode,
					CodeHash:         serviceM.CodeHash,
					Payload:          payloadM,
					GasLimit:         10000000,
					ImportedSegments: importedSegmentsM,
					ExportCount:      0,
				},
			},
		}

		workPackageHash := workPackage.Hash()
		//fmt.Println("WorkPackageHash:", workPackageHash)

		Meg_WorkPackages = append(Meg_WorkPackages, workPackage)
		prevWorkPackageHash = workPackageHash
	}
	fmt.Printf("Fib_Trib_WorkPackages: %v\n", len(Fib_Trib_WorkPackages))
	for _, wp := range Fib_Trib_WorkPackages {
		fmt.Printf("Fib_Trib_WorkPackage: %v\n", wp.Hash())
	}
	fmt.Printf("Meg_WorkPackages: %v\n", len(Meg_WorkPackages))
	for _, wp := range Meg_WorkPackages {
		fmt.Printf("Meg_WorkPackage: %v\n", wp.Hash())
		fmt.Printf("Prerequisite:")
		for _, prereq := range wp.RefineContext.Prerequisites {
			fmt.Printf("%v, ", prereq)
		}
		fmt.Printf("\n")
	}

	// =================================================
	// set up ticker for loop
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	Fib_Tri_Chan := make(chan types.WorkPackage, 1)
	Fib_Tri_counter := 0
	Fib_Tri_Ready := true
	Fib_Tri_Keeper := false
	Meg_Chan := make(chan types.WorkPackage, 1)
	Meg_counter := 0
	Meg_Ready := true
	Meg_Keeper := false
	fmt.Printf("Guarantor Assignment\n")
	for _, assign := range nodes[0].statedb.GuarantorAssignments {
		vid := nodes[0].statedb.GetSafrole().GetCurrValidatorIndex(assign.Validator.GetEd25519Key())
		fmt.Printf("v%d->c%v\n", vid, assign.CoreIndex)
	}
	/*
	   v0->c1
	   v2->c1
	   v3->c1

	   v1->c0
	   v4->c0
	   v5->c0
	*/

	for {
		select {
		case <-ticker.C:
			// here we can put some logic to send workpackages
			// fmt.Printf("Checking for workpackages to send...\n")
			// fmt.Printf("Fib_Tri_counter: %v, Meg_counter: %v\n", Fib_Tri_counter, Meg_counter)
			// fmt.Printf("Fib_Tri_Ready: %v, Meg_Ready: %v\n", Fib_Tri_Ready, Meg_Ready)
			// fmt.Printf("Fib_Tri_Keeper: %v, Meg_Keeper: %v\n", Fib_Tri_Keeper, Meg_Keeper)

			// if nodes[3].statedb.JamState.AvailabilityAssignments[1] == nil {
			// 	fmt.Printf("FIB_TRIB: Ready\n")
			// } else {
			// 	fmt.Printf("FIB_TRIB: Not Ready\n")
			// }

			// if nodes[3].statedb.JamState.AvailabilityAssignments[0] == nil {
			// 	fmt.Printf("MEGATRON: Ready\n")
			// } else {
			// 	fmt.Printf("MEGATRON: Not Ready\n")
			// }

			if nodes[0].statedb.JamState.AvailabilityAssignments[0] == nil && Meg_Ready {
				// if false{
				// send workpackages to the network
				Meg_Chan <- Meg_WorkPackages[Meg_counter]
				Meg_counter++
				Meg_Ready = false
			} else if nodes[0].statedb.JamState.AvailabilityAssignments[0] != nil {
				Meg_Ready = false
				Meg_Keeper = true

			} else if Meg_Keeper && nodes[0].statedb.JamState.AvailabilityAssignments[0] == nil {
				Meg_Ready = true
				Meg_Keeper = false
			}
			if nodes[0].statedb.JamState.AvailabilityAssignments[1] == nil && Fib_Tri_Ready {
				if Meg_Keeper {
					Fib_Tri_Chan <- Fib_Trib_WorkPackages[Fib_Tri_counter]
					Fib_Tri_counter++
					Fib_Tri_Ready = false
				}
			} else if nodes[0].statedb.JamState.AvailabilityAssignments[1] != nil && !Fib_Tri_Ready {
				Fib_Tri_Ready = false
				Fib_Tri_Keeper = true
			} else if nodes[0].statedb.JamState.AvailabilityAssignments[1] == nil && Fib_Tri_Keeper == true {
				Fib_Tri_Ready = true
				Fib_Tri_Keeper = false
			}

		case workPackage := <-Fib_Tri_Chan:
			// submit to core 1
			// v0, v3, v5 => core
			fmt.Printf("\n** \033[32m Fib_Tri %d \033[0m workPackage: %v **\n", Fib_Tri_counter, common.Str(workPackage.Hash()))
			// Randomly select sender and receiver
			senderIdx := rand.Intn(3)
			receiverIdx := rand.Intn(3)
			for senderIdx == receiverIdx {
				receiverIdx = rand.Intn(3)
			}
			nodeIndices := []int{0, 2, 3}
			senderIdx = nodeIndices[senderIdx]
			receiverIdx = nodeIndices[receiverIdx]
			err := nodes[senderIdx].peersInfo[uint16(receiverIdx)].SendWorkPackageSubmission(1, workPackage, []byte{})
			if err != nil {
				fmt.Printf("SendWorkPackageSubmission ERR %v, sender:%d, receiver %d\n", err, senderIdx, receiverIdx)
			}
		case workPackage := <-Meg_Chan:
			// submit to core 0
			// CE133_WorkPackageSubmission: n1 => n4
			// v1, v2, v4 => core
			// random select 1 sender and 1 receiver
			// Randomly select sender and receiver
			fmt.Printf("\n** \033[36m MEGATRON %d \033[0m workPackage: %v **\n", Meg_counter, common.Str(workPackage.Hash()))
			senderIdx := rand.Intn(3)
			receiverIdx := rand.Intn(3)
			for senderIdx == receiverIdx {
				receiverIdx = rand.Intn(3)
			}
			nodeIndices := []int{1, 4, 5}
			senderIdx = nodeIndices[senderIdx]
			receiverIdx = nodeIndices[receiverIdx]
			fmt.Printf("Sending WorkPackage...\n")
			err := nodes[senderIdx].peersInfo[uint16(receiverIdx)].SendWorkPackageSubmission(0, workPackage, []byte{})
			if err != nil {
				fmt.Printf("SendWorkPackageSubmission ERR %v, sender:%d, receiver %d\n", err, senderIdx, receiverIdx)
			}
		}

	}
}

func TestFallback(t *testing.T) {
	testSafrole(t, false)
}

func TestSafrole(t *testing.T) {
	testSafrole(t, true)
}

func TestFib(t *testing.T) {
	testJAM(t, "fib")
}

func TestMegatron(t *testing.T) {
	testJAM(t, "megatron")
}
