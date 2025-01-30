package node

import "github.com/colorfulnotion/jam/statedb"

func (n *Node) IsCoreReady(coreIdx uint16) bool {
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
	tmpState := latest_statedb.JamState.Copy()
	tmpState.ProcessAssurances(assurances, targetJCE)
	return tmpState.AvailabilityAssignments[coreIdx] == nil
}
