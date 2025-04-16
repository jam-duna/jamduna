package node

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

const (
	blk_sync = "block_sync"
	blk      = "block"
	rpc_mod  = "rpc"
)

func (n *NodeContent) GetBlockTree() *types.BlockTree {
	return n.block_tree
}

func (n *NodeContent) SetServiceDir(dir string) {
	n.loaded_services_dir = dir
}

func (n *NodeContent) LoadService(service_name string) ([]byte, error) {
	// read the .pvm from the service directory
	service_path := fmt.Sprintf("%s/%s.pvm", n.loaded_services_dir, service_name)
	return types.ReadCodeWithMetadata(service_path, service_name)
}

// input : the core index and the segments lookups (workpackages hashes only)
func (n *Node) IsCoreReady(coreIdx uint16, lookups []common.Hash, parameter ...interface{}) bool {
	printout := false
	if len(parameter) > 0 && parameter[0].(bool) {
		printout = true
	}
	var workpackagehash common.Hash
	if len(parameter) > 1 {
		workpackagehash = parameter[1].(common.Hash)
	}

	if test_prereq {
		return n.statedb.JamState.AvailabilityAssignments[coreIdx] == nil
	}
	n.statedbMutex.Lock()
	latest_statedb := n.statedb.Copy()
	n.statedbMutex.Unlock()
	// get the latest assurance
	latest_header_hash := latest_statedb.Block.Header.HeaderHash()
	// get the assurances from the extrinsic pool
	assurances := n.extrinsic_pool.GetAssurancesFromPool(latest_header_hash)
	statedb.SortAssurances(assurances)
	targetJCE, _ := latest_statedb.JamState.SafroleState.CheckTimeSlotReady(n.GetCurrJCE())
	preState := latest_statedb.JamState.Copy()
	postAssuranceState := latest_statedb.JamState.Copy()
	postAssuranceState.ProcessAssurances(assurances, targetJCE)
	// check if the core is ready
	if len(lookups) == 0 {
		empty_hash := common.Hash{}
		if workpackagehash != empty_hash {
			prestate_is_package := preState.AvailabilityAssignments[coreIdx] != nil && preState.AvailabilityAssignments[coreIdx].WorkReport.AvailabilitySpec.WorkPackageHash == workpackagehash
			poststate_will_be_assured := postAssuranceState.AvailabilityAssignments[coreIdx] == nil
			if prestate_is_package && poststate_will_be_assured {
				return true
			} else {
				return false
			}
		} else {
			return postAssuranceState.AvailabilityAssignments[coreIdx] == nil
		}
	}
	for _, lookup := range lookups {
		// first check if the work report for lookup is in the rho state
		core_rho := preState.AvailabilityAssignments[coreIdx]
		core_rho_post := postAssuranceState.AvailabilityAssignments[coreIdx]
		if core_rho != nil && core_rho.WorkReport.AvailabilitySpec.WorkPackageHash == lookup {

			// see if it pass the assurances
			if core_rho_post != nil {
				if printout {
					fmt.Printf("core is occupied by wp=%v\n", core_rho.WorkReport.AvailabilitySpec.WorkPackageHash)
				}
				return false
			}

		} else {
			// check if the work report is in the post assurance state
			if !IsWorkPackageInHistory(latest_statedb, lookup) {
				if printout {
					fmt.Printf("work package lookup not in history\n")
				}
				return false
			}
		}
	}
	return true
}
func IsWorkPackageInHistory(latestdb *statedb.StateDB, workPackageHash common.Hash) bool {
	for _, block := range latestdb.JamState.RecentBlocks {
		if len(block.Reported) != 0 {
			for _, segmentRootLookup := range block.Reported {
				if segmentRootLookup.WorkPackageHash == workPackageHash {
					// panic("invalid prerequisite work package(ErrGDuplicatePackageRecentHistory)")
					return true
				}
			}
		}
	}

	prereqSetFromAccumulationHistory := make(map[common.Hash]struct{})
	for i := 0; i < types.EpochLength; i++ {
		for _, hash := range latestdb.JamState.AccumulationHistory[i].WorkPackageHash {
			prereqSetFromAccumulationHistory[hash] = struct{}{}
		}
	}
	_, exists := prereqSetFromAccumulationHistory[workPackageHash]
	if exists {
		// TODO: REVIEW non-standard error
		return true
	}
	return false
}

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
