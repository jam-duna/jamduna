//go:build network_test
// +build network_test

package node

import (
	_ "net/http/pprof"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) MakeWorkPackage(prereq []common.Hash, service_code uint32, WorkItems []types.WorkItem) (types.WorkPackage, error) {
	refineContext := n.statedb.GetRefineContext(prereq...)
	workPackage := types.WorkPackage{
		Authorization:         []byte("0x"), // TODO: set up null-authorizer
		AuthCodeHost:          0,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		RefineContext:         refineContext,
		WorkItems:             WorkItems,
	}
	return workPackage, nil
}

/*

func buildMegItem(importedSegmentsM []types.ImportSegment, megaN int, service_code_mega uint32, service_code0 uint32, service_code1 uint32, codehash common.Hash) []types.WorkItem {
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, uint32(megaN))
	payloadM := make([]byte, 8)
	binary.LittleEndian.PutUint32(payloadM[0:4], service_code0)
	binary.LittleEndian.PutUint32(payloadM[4:8], service_code1)
	WorkItems := []types.WorkItem{
		{
			Service:            service_code_mega,
			CodeHash:           codehash,
			Payload:            payloadM,
			RefineGasLimit:     1000,
			AccumulateGasLimit: 100000,
			ImportedSegments:   importedSegmentsM,
			ExportCount:        0,
		},
	}
	return WorkItems
}

func buildFibTribItem(fibImportSegments []types.ImportSegment, tribImportSegments []types.ImportSegment, n int, service_code_fib uint32, codehash_fib common.Hash, service_code_trib uint32, codehash_trib common.Hash) []types.WorkItem {
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, uint32(n+1))
	WorkItems := []types.WorkItem{
		{
			Service:            service_code_fib,
			CodeHash:           codehash_fib,
			Payload:            payload,
			RefineGasLimit:     1000,
			AccumulateGasLimit: 1000,
			ImportedSegments:   fibImportSegments,
			ExportCount:        1,
		},
		{
			Service:            service_code_trib,
			CodeHash:           codehash_trib,
			Payload:            payload,
			RefineGasLimit:     1000,
			AccumulateGasLimit: 1000,
			ImportedSegments:   tribImportSegments,
			ExportCount:        1,
		},
	}
	return WorkItems
}

func megatron(n1 JNode, testServices map[string]*types.TestService, targetMegatronN int, jceManager *ManualJCEManager) {
	fmt.Printf("Start Fib_Trib\n")
	service0 := testServices["fib"]
	service1 := testServices["tribonacci"]
	serviceM := testServices["megatron"]
	service_authcopy := testServices["auth_copy"]
	fmt.Printf("service0: %v, codehash: %v (len=%v) | %v\n", service0.ServiceCode, service0.CodeHash, len(service0.Code), service0.ServiceName)
	fmt.Printf("service1: %v, codehash: %v (len=%v) | %v\n", service1.ServiceCode, service1.CodeHash, len(service1.Code), service1.ServiceName)
	fmt.Printf("serviceM: %v, codehash: %v (len=%v) | %v\n", serviceM.ServiceCode, serviceM.CodeHash, len(serviceM.Code), serviceM.ServiceName)
	targetNMax := targetMegatronN
	time.Sleep(1 * time.Second)
	// =================================================
	// set up ticker for loop
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	ticker_runtime := time.NewTicker(2 * time.Second)
	defer ticker_runtime.Stop()
	Fib_Tri_Chan := make(chan *types.WorkPackage, 1)
	Fib_Tri_counter := 0
	Fib_Tri_successful := make(chan string)
	Fib_Tri_Ready := true
	Fib_Tri_Ok := false
	Meg_Chan := make(chan *types.WorkPackage, 1)
	Meg_counter := 0
	Meg_successful := make(chan string)
	Meg_Ready := true
	Meg_Ok := false
	// =================================================
	// set up the workpackages
	fib_importedSegments := make([]types.ImportSegment, 0)
	trib_importedSegments := make([]types.ImportSegment, 0)
	fib_trib_items := buildFibTribItem(fib_importedSegments, trib_importedSegments, Fib_Tri_counter, service0.ServiceCode, service0.CodeHash, service1.ServiceCode, service1.CodeHash)
	next_fib_tri_WorkPackage, err := n1.MakeWorkPackage([]common.Hash{}, service0.ServiceCode, fib_trib_items)
	if err != nil {
		fmt.Printf("MakeWorkPackage ERR %v\n", err)
	}
	for _, n := range nodes {
		n.delaysend[next_fib_tri_WorkPackage.Hash()] = 1
	}
	var curr_fib_tri_prereqs []common.Hash
	meg_no_import_segment := make([]types.ImportSegment, 0)
	meg_items := buildMegItem(meg_no_import_segment, Meg_counter, serviceM.ServiceCode, service0.ServiceCode, service1.ServiceCode, serviceM.CodeHash)
	meg_preq := []common.Hash{next_fib_tri_WorkPackage.Hash()}
	next_Meg_WorkPackage, err := n1.MakeWorkPackage(meg_preq, serviceM.ServiceCode, meg_items)
	var last_Meg []common.Hash

	fmt.Printf("Guarantor Assignment\n")
	for _, assign := range n1.statedb.GuarantorAssignments {
		vid := n1.GetCurrValidatorIndex(assign.Validator.GetEd25519Key())
		fmt.Printf("v%d->c%v\n", vid, assign.CoreIndex)
	}
	// get the core index every time before we send the workpackage

	ok := false
	sentLastWorkPackage := false
	FinalRho := false
	FinalAssurance := false
	FinalMeg := false
	var curr_fib_tri_workpackage types.WorkPackage
	var curr_meg_workpackage types.WorkPackage

	meg_good_togo := make(chan bool, 1)
	fib_tri_good_togo := make(chan bool, 1)
	// =================================================
	for {
		if ok {
			break
		}
		select {

		case workPackage := <-Fib_Tri_Chan:
			// submit to core 1
			// v0, v3, v5 => core
			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			go func() {
				defer cancel()
				sendWorkPackageTrack(ctx, n1, workPackage, uint16(1), Fib_Tri_successful, types.ExtrinsicsBlobs{})
			}()
		case successful := <-Fib_Tri_successful:
			if successful == "ok" {
				k0 := common.ServiceStorageKey(service0.ServiceCode, []byte{0})
				k1 := common.ServiceStorageKey(service1.ServiceCode, []byte{0})
				service_account_byte, _, _ := n1.GetServiceStorage(service0.ServiceCode, k0)
				// get the first four byte of the result
				if len(service_account_byte) < 4 {
					fmt.Printf("Fib %d = %v\n", 0, service_account_byte)
				} else {
					index_bytes := service_account_byte[:4]
					index := binary.LittleEndian.Uint32(index_bytes)

					fmt.Printf("Fib %d = %v\n", index, service_account_byte)
					service_account_byte, _, _ = n1.GetServiceStorage(service1.ServiceCode, k1)
					fmt.Printf("Tri %d = %v\n", index, service_account_byte)
				}
				fib_tri_good_togo <- true
			}
		case successful := <-Meg_successful:
			if successful == "ok" {
				km := common.ServiceStorageKey(serviceM.ServiceCode, []byte{0})
				service_account_byte, _, _ := n1.GetServiceStorage(serviceM.ServiceCode, km)

				// get the first four byte of the result
				if len(service_account_byte) < 4 {
					fmt.Printf("Meg %d = %v\n", 0, service_account_byte)
				} else {
					index_bytes := service_account_byte[:4]
					index := binary.LittleEndian.Uint32(index_bytes)
					fmt.Printf("Meg %d = %v\n", index, service_account_byte)
				}
				meg_good_togo <- true
			}

		case workPackage := <-Meg_Chan:
			// submit to core 0
			// CE133_WorkPackageSubmission: n1 => n4
			// v1, v2, v4 => core
			// random select 1 sender and 1 receiver
			// Randomly select sender and receiver
			// senderIdx := rand.Intn(6)
			// receiverIdx := rand.Intn(3)
			// core0_peers := nodes[senderIdx].GetCoreCoWorkersPeers(0)
			// for senderIdx == int(core0_peers[receiverIdx].PeerID) {
			// 	receiverIdx = rand.Intn(3)
			// }
			megCoreIdx := uint16(0)

			ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
			go func() {
				defer cancel()
				sendWorkPackageTrack(ctx, n1, workPackage, megCoreIdx, Meg_successful, types.ExtrinsicsBlobs{})
			}()

		case <-ticker.C:
			if sentLastWorkPackage {
				if FinalRho && FinalAssurance && FinalMeg {
					ok = true
					log.Info(module, "megatron success")
					break
				} else if test_prereq && n1.statedb.JamState.AvailabilityAssignments[0] != nil && n1.statedb.JamState.AvailabilityAssignments[1] != nil {
					FinalRho = true
				} else if test_prereq && FinalRho {
					if FinalAssurance {
						if n1.statedb.JamState.AvailabilityAssignments[1] == nil {
							FinalMeg = true
						}

					} else if nodes[0].statedb.JamState.AvailabilityAssignments[0] == nil {
						FinalAssurance = true
					}

				} else if !test_prereq {
					if n1.statedb.JamState.AvailabilityAssignments[1] != nil {
						FinalRho = true
					}
					if n1.statedb.JamState.AvailabilityAssignments[0] != nil {
						FinalMeg = true
					}
					if FinalRho && FinalMeg {
						if n1.statedb.JamState.AvailabilityAssignments[0] == nil && nodes[0].statedb.JamState.AvailabilityAssignments[1] == nil {
							FinalAssurance = true
						}
					}
					if FinalAssurance {
						ok = true
						log.Info(module, "megatron success")
						break
					}
				}
			}
			if Fib_Tri_counter == targetNMax && Meg_counter == targetNMax {
				if !sentLastWorkPackage {
					fmt.Printf("All workpackages are sent\n")
				}
				sentLastWorkPackage = true
			} else if (n1.IsCoreReady(0, last_Meg) && Meg_Ready) && (n1.IsCoreReady(1, curr_fib_tri_prereqs) && Fib_Tri_Ready) {
				// send workpackages to the network
				fmt.Printf("**  %v  Preparing Fib_Tri#%v %v Meg#%v %v **\n", time.Now().Format("04:05.000"), Fib_Tri_counter, next_fib_tri_WorkPackage.Hash().String_short(), Meg_counter, next_Meg_WorkPackage.Hash().String_short())
				fmt.Printf("\n** \033[32m Fib_Tri %d \033[0m workPackage: %v **\n", Fib_Tri_counter, common.Str(next_fib_tri_WorkPackage.Hash()))
				// in case it gets stuck
				// refine context need to be updated
				next_Meg_WorkPackage.RefineContext.Prerequisites = []common.Hash{next_fib_tri_WorkPackage.Hash()}
				// send workpackages to the network
				curr_fib_tri_workpackage = next_fib_tri_WorkPackage
				curr_meg_workpackage = next_Meg_WorkPackage
				Fib_Tri_Chan <- &curr_fib_tri_workpackage
				Fib_Tri_counter++
				if Fib_Tri_counter <= targetNMax-1 {
					fib_importedSegments := []types.ImportSegment{
						{
							RequestedHash: curr_fib_tri_workpackage.Hash(),
							Index:         0,
						},
					}
					trib_importedSegments := []types.ImportSegment{
						{
							RequestedHash: curr_fib_tri_workpackage.Hash(),
							Index:         1,
						},
					}
					fib_trib_items = buildFibTribItem(fib_importedSegments, trib_importedSegments, Fib_Tri_counter, service0.ServiceCode, service0.CodeHash, service1.ServiceCode, service1.CodeHash)
					next_fib_tri_WorkPackage, err = nodes[2].MakeWorkPackage([]common.Hash{}, service0.ServiceCode, fib_trib_items)
					if err != nil {
						panic(err)
					}
					curr_fib_tri_prereqs = []common.Hash{}
					// for _, item := range next_fib_tri_WorkPackage.WorkItems {
					// 	for _, seg := range item.ImportedSegments {
					// 		curr_fib_tri_prereqs = append(curr_fib_tri_prereqs, seg.RequestedHash)
					// 	}
					// }
					Fib_Tri_Ready = false
					Fib_Tri_Ok = false
				}
				// send workpackages to the network
				fmt.Printf("\n** \033[36m MEGATRON %d \033[0m workPackage: %v **\n", Meg_counter, common.Str(next_Meg_WorkPackage.Hash()))
				Meg_Chan <- &curr_meg_workpackage
				Meg_counter++
				if Meg_counter <= targetNMax-1 {
					// previous_workpackage_hash := curr_meg_workpackage.Hash()
					meg_items = buildMegItem(meg_no_import_segment, Meg_counter, serviceM.ServiceCode, service0.ServiceCode, service1.ServiceCode, serviceM.CodeHash)
					meg_items = append(meg_items, types.WorkItem{

						Service:            service_authcopy.ServiceCode,
						CodeHash:           service_authcopy.CodeHash,
						Payload:            []byte{},
						RefineGasLimit:     5678,
						AccumulateGasLimit: 9876,
						ImportedSegments:   make([]types.ImportSegment, 0),
						ExportCount:        0,
					})
					next_Meg_WorkPackage, err = n1.MakeWorkPackage([]common.Hash{next_fib_tri_WorkPackage.Hash()}, serviceM.ServiceCode, meg_items)
					last_Meg = []common.Hash{}
					// last_Meg = append(last_Meg, previous_workpackage_hash)
					Meg_Ready = false
					Meg_Ok = false
				}
			} else {
				select {
				case <-fib_tri_good_togo:
					Fib_Tri_Ok = true
				case <-meg_good_togo:
					Meg_Ok = true
				default:
					if Fib_Tri_Ok && Meg_Ok {
						is_0_ready := n1.IsCoreReady(0, last_Meg)
						is_1_ready := n1.IsCoreReady(1, curr_fib_tri_prereqs, true, curr_fib_tri_workpackage.Hash())
						if is_0_ready && is_1_ready {
							Meg_Ready = true
							Fib_Tri_Ready = true
						}
					}
				}

			}
		default:
			time.Sleep(10 * time.Millisecond)
		}

	}
}

func sendWorkPackageTrack(ctx context.Context, senderNode JNode, workPackage *types.WorkPackage, receiverCore uint16, msg chan string, extrinsics types.ExtrinsicsBlobs) {
	trialCount := 0
	MaxTrialCount := 100
	// send it right away for one time
	wpr := &WorkPackageRequest{
		WorkPackage:     *workPackage,
		CoreIndex:       receiverCore,
		ExtrinsicsBlobs: extrinsics,
	}
	for trialCount < MaxTrialCount {
		workPackageHash, err := senderNode.SubmitAndWaitForWorkPackage(ctx, wpr)
		if err != nil {
			fmt.Printf("SendWorkPackageSubmission ERR %v", err)
		} else {
			return nil
		}
	}
}
*/
