package statedb

import "github.com/jam-duna/jamduna/types"

// Section 8.2 - Eq 85+86
/*
   func (s *StateDB) ApplyStateTransitionAuthorizations(guarantees []types.Guarantee) error {
	for c := uint16(0); c < types.TotalCores; c++ {
		if len(guarantees) > 0 {
			// build m, holding the set of authorization hashes matching the core c in guarantees
			m := make(map[common.Hash]bool)
			hits := 0
			for _, g := range guarantees {
				if g.Report.CoreIndex == c {
					m[g.Report.AuthorizerHash] = true
					hits++
				}
			}
			// only use m to update the pool with remove_guarantees_authhash if we have hits > 0
			if hits > 0 {
				s.JamState.AuthorizationsPool[c] = s.remove_guarantees_authhash(s.JamState.AuthorizationsPool[c], m)
			}
		}
		// Eq 85 -- add
		for _, q := range s.JamState.AuthorizationQueue[c] {
			s.JamState.AuthorizationsPool[c] = append(s.JamState.AuthorizationsPool[c], q)
		}
		// cap AuthorizationsPool length at O=types.MaxAuthorizationPoolItems
		if len(s.JamState.AuthorizationsPool[c]) > types.MaxAuthorizationPoolItems {
			s.JamState.AuthorizationsPool[c] = s.JamState.AuthorizationsPool[c][:types.MaxAuthorizationPoolItems]
		}
	}
	return nil
}
*/

func (s *StateDB) ApplyStateTransitionAuthorizations() error {
	// gp 0.5.0 eq 8.3 -- we do F function first to eliminate old authorizations
	s.RemoveOldAuthorizationFromReport()
	// gp 0.5.0 eq 8.2
	s.AddNewPoolFromQueue()
	return nil
}

// gp 0.5.0 eq 8.2
func (s *StateDB) AddNewPoolFromQueue() {
	jam_state := s.GetJamState()
	time_slot := s.Block.TimeSlot()
	mod_time_slot := time_slot % types.MaxAuthorizationQueueItems
	for core_idx, pool := range jam_state.AuthorizationsPool {
		ready_queue := jam_state.AuthorizationQueue[core_idx][mod_time_slot]
		pool = append(pool, ready_queue)
		if len(pool) > types.MaxAuthorizationPoolItems {
			// if the pool is full, remove the oldest item
			head := len(pool) - types.MaxAuthorizationPoolItems
			pool = pool[head:]
		}
		jam_state.AuthorizationsPool[core_idx] = pool
	}
}

// gp 0.5.0 eq 8.3
func (s *StateDB) RemoveOldAuthorizationFromReport() {
	jam_state := s.GetJamState()
	for _, eg := range s.Block.Extrinsic.Guarantees {
		core_index := eg.Report.CoreIndex
		for i, pool := range jam_state.AuthorizationsPool[core_index] {
			// remove the authorization from the pool if it is the same as the authorizer hash from the report
			if pool == eg.Report.AuthorizerHash {
				jam_state.AuthorizationsPool[core_index] = append(jam_state.AuthorizationsPool[core_index][:i], jam_state.AuthorizationsPool[core_index][i+1:]...)
				break
			}
		}
	}
}
