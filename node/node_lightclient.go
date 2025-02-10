package node

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// input : the core index and the segments lookups (workpackages hashes only)
func (n *Node) IsCoreReady(coreIdx uint16, lookups []common.Hash) bool {
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
	targetJCE, _ := latest_statedb.JamState.SafroleState.CheckTimeSlotReady()
	preState := latest_statedb.JamState.Copy()
	postAssuranceState := latest_statedb.JamState.Copy()
	postAssuranceState.ProcessAssurances(assurances, targetJCE)
	// check if the core is ready
	if lookups == nil {
		return postAssuranceState.AvailabilityAssignments[coreIdx] == nil
	}
	for _, lookup := range lookups {
		// first check if the work report for lookup is in the rho state
		core_rho := preState.AvailabilityAssignments[coreIdx]
		core_rho_post := postAssuranceState.AvailabilityAssignments[coreIdx]
		if core_rho != nil && core_rho.WorkReport.AvailabilitySpec.WorkPackageHash == lookup {
			// see if it pass the assurances
			if core_rho_post != nil {
				return false
			}

		} else {
			// check if the work report is in the post assurance state
			if !IsWorkPackageInHistory(latest_statedb, lookup) {
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
