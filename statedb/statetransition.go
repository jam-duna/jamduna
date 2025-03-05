package statedb

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/nsf/jsondiff"
)

type StateTransitionChallenge struct {
	PreState StateSnapshotRaw `json:"pre_state"`
	Block    types.Block      `json:"block"`
}

func (s *StateTransitionChallenge) String() string {
	jsonEncode, _ := json.Marshal(s)
	return string(jsonEncode)
}

type StateTransition struct {
	PreState  StateSnapshotRaw `json:"pre_state"`
	Block     types.Block      `json:"block"`
	PostState StateSnapshotRaw `json:"post_state"`
}

type StateTransitionCheck struct {
	ValidMatch         bool          `json:"valid_match"`
	PostStateRootMatch bool          `json:"post_state_root_match"`
	PostStateMismatch  []common.Hash `json:"post_state_mismatch"`
}

func compareKeyVals(p0 []KeyVal, p1 []KeyVal) {
	if len(p0) != len(p1) {
		fmt.Printf("len pre %d != len post %d\n", len(p0), len(p1))
	}
	kv0, m0 := makemap(p0)
	kv1, m1 := makemap(p1)
	for k0, v0 := range kv0 {
		v1 := kv1[k0]
		if !common.CompareBytes(v0, v1) {
			metaKey := fmt.Sprintf("meta_%v", k0)
			metaData0 := m0[metaKey]
			metaData1 := m1[metaKey]
			fmt.Printf("K %v\nC: %s %x\nP: %s %x\n", k0, metaData0, v0, metaData1, v1)
		}
	}
}

type DiffState struct {
	Prestate          []byte
	PoststateCompared []byte
	Poststate         []byte
}

func compareKeyValsWithOutput(org []KeyVal, p0 []KeyVal, p1 []KeyVal) (diffs map[string]DiffState) {
	if len(p0) != len(p1) {
		fmt.Printf("len pre %d != len post %d\n", len(p0), len(p1))
	}
	diffs = make(map[string]DiffState)
	kvog, _ := makemap(org)
	kv0, m0 := makemap(p0)
	kv1, _ := makemap(p1)

	for k0, v0 := range kv0 {
		v_og := kvog[k0]
		v1 := kv1[k0]

		if !common.CompareBytes(v0, v1) {
			metaKey := fmt.Sprintf("meta_%v", k0)
			metaData0 := m0[metaKey]
			diffs[metaData0] = DiffState{
				Prestate:          v_og,
				PoststateCompared: v0,
				Poststate:         v1,
			}

		}
	}
	return diffs
}

func makemap(p []KeyVal) (map[common.Hash][]byte, map[string]string) {
	kvMap := make(map[common.Hash][]byte)
	metaMap := make(map[string]string)
	for _, kvs := range p {
		k := common.BytesToHash(kvs.Key)
		v := kvs.Value
		kvMap[k] = v
		metaKey := fmt.Sprintf("meta_%v", k)
		metaMap[metaKey] = fmt.Sprintf("%s|%s", kvs.StructType, kvs.Metadata)
	}
	return kvMap, metaMap
}

func ComputeStateTransition(storage *storage.StateDBStorage, stc *StateTransitionChallenge) (ok bool, postStateSnapshot *StateSnapshotRaw, jamErr error, miscErr error) {
	scPreState := stc.PreState
	scBlock := stc.Block
	preState, miscErr := NewStateDBFromSnapshotRaw(storage, &scPreState)
	if miscErr != nil {
		// Invalid pre-state
		return false, nil, nil, fmt.Errorf("PreState Error")
	}
	postState, jamErr := ApplyStateTransitionFromBlock(preState, context.Background(), &scBlock, "ComputeStateTransition")
	if jamErr != nil {
		// When validating Fuzzed STC, we shall expect jamError
		return true, nil, jamErr, nil
	}

	postState.StateRoot = postState.UpdateTrieState()
	postStateSnapshot = &StateSnapshotRaw{
		StateRoot: postState.StateRoot,
		KeyVals:   postState.GetAllKeyValues(),
	}
	return true, postStateSnapshot, nil, nil

}

// NOTE CheckStateTransition vs CheckStateTransitionWithOutput a good example of "copy-paste" coding increases in complexity
func CheckStateTransition(storage *storage.StateDBStorage, st *StateTransition, ancestorSet map[common.Hash]uint32) error {
	// Apply the state transition
	s0, err := NewStateDBFromSnapshotRaw(storage, &(st.PreState))
	if err != nil {
		return err
	}
	s0.Id = storage.NodeID
	s0.AncestorSet = ancestorSet
	s1, err := ApplyStateTransitionFromBlock(s0, context.Background(), &(st.Block), "CheckStateTransition")
	if err != nil {
		return err
	}
	if st.PostState.StateRoot == s1.StateRoot {
		return nil
	}

	compareKeyVals(s1.GetAllKeyValues(), st.PostState.KeyVals)
	return fmt.Errorf("mismatch")

}

func CheckStateTransitionWithOutput(storage *storage.StateDBStorage, st *StateTransition, ancestorSet map[common.Hash]uint32) (diffs map[string]DiffState, err error) {
	// Apply the state transition
	s0, err := NewStateDBFromSnapshotRaw(storage, &(st.PreState))
	if err != nil {
		return nil, err
	}
	s0.Id = storage.NodeID
	s0.AncestorSet = ancestorSet
	s1, err := ApplyStateTransitionFromBlock(s0, context.Background(), &(st.Block), "CheckStateTransitionWithOutput")
	if err != nil {
		return nil, err
	}
	if st.PostState.StateRoot == s1.StateRoot {
		return nil, nil
	}

	return compareKeyValsWithOutput(st.PreState.KeyVals, s1.GetAllKeyValues(), st.PostState.KeyVals), fmt.Errorf("mismatch")
}

// ValidateSTF validates the state transition function output: error number and diffs[error discription][diff]
// note!!! only state transition part , no validation of extrinsic, data ...
func ValidateSTF(pre_state *StateDB, block_origin types.Block, post_state *StateDB) (error_num int, diffs map[string]string) {
	//α, β, γ, δ, η, ι, κ, λ, ρ, τ, φ, χ, ψ, π, ϑ, ξ
	error_num = 0
	diffs = make(map[string]string)
	block := block_origin.Copy()
	header := block.Header
	tickets := block.Extrinsic.Tickets
	guarantees := block.Extrinsic.Guarantees
	//section 6
	old_sf := pre_state.JamState.SafroleState.Copy()
	new_sf := post_state.JamState.SafroleState.Copy()
	old_epoch, old_phase := old_sf.EpochAndPhase(uint32(old_sf.Timeslot))
	new_epoch, new_phase := new_sf.EpochAndPhase(uint32(new_sf.Timeslot))
	is_epoch_changed := old_epoch < new_epoch
	slot_header := block.Header.Slot
	new_slot := new_sf.Timeslot
	//τ = τ' slot header
	// 6.1
	if slot_header != new_slot {
		error_num++
		diffs["slot τ"] = fmt.Sprintf("slot header %d != new slot %d", slot_header, new_slot)
	}
	// 6.22
	fresh_randomness, err := old_sf.GetFreshRandomness(header.EntropySource[:])
	if err != nil {
		error_num++
		diffs["FreshRandomness"] = fmt.Sprintf("GetFreshRandomness error: %v", err)
	} else {
		new_entropy_0 := old_sf.ComputeCurrRandomness(fresh_randomness)
		new_entropy := new_sf.Entropy
		if !reflect.DeepEqual(new_entropy[0], new_entropy_0) {
			error_num++
			diffs["entropy[0]"] = CompareJSON(new_entropy[0], new_entropy_0)
		}
	}
	if !is_epoch_changed {
		// 6.27
		//6.13
		// TODO offender
		switch {
		case !reflect.DeepEqual(new_sf.CurrValidators, old_sf.CurrValidators):
			error_num++
			diffs["CurrValidators"] = CompareJSON(new_sf.CurrValidators, old_sf.CurrValidators)
		case !reflect.DeepEqual(new_sf.PrevValidators, old_sf.PrevValidators):
			error_num++
			diffs["PrevValidators"] = CompareJSON(new_sf.PrevValidators, old_sf.PrevValidators)
		case !reflect.DeepEqual(new_sf.DesignedValidators, old_sf.DesignedValidators):
			error_num++
			diffs["DesignedValidators"] = CompareJSON(new_sf.DesignedValidators, old_sf.DesignedValidators)
		case !reflect.DeepEqual(new_sf.NextValidators, old_sf.NextValidators):
			error_num++
			diffs["NextValidators"] = CompareJSON(new_sf.NextValidators, old_sf.NextValidators)
		}
		// entropy
		// 6.23
		old_entropy := old_sf.Entropy
		new_entropy := new_sf.Entropy
		switch {
		case old_entropy[1] != new_entropy[1]:
			error_num++
			diffs["Entropy[1]"] = CompareJSON(old_entropy[1], new_entropy[1])
		case old_entropy[2] != new_entropy[2]:
			error_num++
			diffs["Entropy[2]"] = CompareJSON(old_entropy[2], new_entropy[2])
		case old_entropy[3] != new_entropy[3]:
			error_num++
			diffs["Entropy[3]"] = CompareJSON(old_entropy[3], new_entropy[3])
		}

	} else { // !!! here is epoch changed case
		if block.Header.EpochMark == nil {
			error_num++
			diffs["EpochMark"] = "EpochMark is nil"
		}
		//6.13
		switch {
		case !reflect.DeepEqual(new_sf.PrevValidators, old_sf.CurrValidators):
			error_num++
			diffs["PrevValidators"] = CompareJSON(new_sf.PrevValidators, old_sf.CurrValidators)
		case !reflect.DeepEqual(new_sf.CurrValidators, old_sf.NextValidators):
			error_num++
			diffs["CurrValidators"] = CompareJSON(new_sf.CurrValidators, old_sf.NextValidators)
		case !reflect.DeepEqual(new_sf.NextValidators, old_sf.DesignedValidators):
			error_num++
			diffs["NextValidators"] = CompareJSON(new_sf.NextValidators, old_sf.DesignedValidators)
		}
		// entropy
		// 6.23
		old_entropy := old_sf.Entropy
		new_entropy := new_sf.Entropy
		switch {
		case old_entropy[0] != new_entropy[1]:
			error_num++
			diffs["NEW Entropy[1]"] = CompareJSON(old_entropy[0], new_entropy[1])
		case old_entropy[1] != new_entropy[2]:
			error_num++
			diffs["NEW Entropy[2]"] = CompareJSON(old_entropy[1], new_entropy[2])
		case old_entropy[2] != new_entropy[3]:
			error_num++
			diffs["NEW Entropy[3]"] = CompareJSON(old_entropy[2], new_entropy[3])
		}
	}
	// gamma s verification
	old_gamma_a := old_sf.NextEpochTicketsAccumulator
	if new_epoch == old_epoch+1 && old_phase > types.TicketSubmissionEndSlot && len(old_gamma_a) == types.EpochLength {
		new_gamma_s := new_sf.TicketsOrKeys
		winning_tickets, err := old_sf.GenerateWinningMarker()
		if err != nil {
			error_num++
			diffs["TicketsOrKeys"] = fmt.Sprintf("GenerateWinningMarker error: %v", err)
		} else {
			ticketsOrKeys := TicketsOrKeys{
				Tickets: winning_tickets,
			}
			if !reflect.DeepEqual(new_gamma_s, ticketsOrKeys) {
				error_num++
				diffs["TicketsOrKeys"] = CompareJSON(new_gamma_s, ticketsOrKeys)
			}
		}

	} else if !is_epoch_changed {
		old_gamma_s := old_sf.TicketsOrKeys
		new_gamma_s := new_sf.TicketsOrKeys
		if !reflect.DeepEqual(old_gamma_s, new_gamma_s) {
			error_num++
			diffs["TicketsOrKeys"] = CompareJSON(old_gamma_s, new_gamma_s)
		}
	} else {
		// fallback mode
		new_gamma_s := new_sf.TicketsOrKeys
		// we use n3' so it has to be after we check the entropy state transition
		choosenkeys, err := new_sf.ChooseFallBackValidator()
		if err != nil {
			error_num++
			diffs["ChooseFallBackValidator"] = fmt.Sprintf("ChooseFallBackValidator error: %v", err)
		} else {
			ticketsOrKeys := TicketsOrKeys{
				Keys: choosenkeys,
			}
			if !reflect.DeepEqual(new_gamma_s, ticketsOrKeys) {
				error_num++
				diffs["TicketsOrKeys"] = CompareJSON(new_gamma_s, ticketsOrKeys)
			}
		}
	}
	//6.28
	if !is_epoch_changed && old_phase < types.TicketSubmissionEndSlot && new_phase >= types.TicketSubmissionEndSlot && len(old_gamma_a) == types.EpochLength {
		// winning ticket mark required
		winning_ticket_mark := block.Header.TicketsMark
		expect_winning_ticket, err := old_sf.GenerateWinningMarker()
		if err != nil {
			error_num++
			diffs["WinningTicketMark"] = fmt.Sprintf("GenerateWinningMarker error: %v", err)
		} else {
			for index, t := range expect_winning_ticket {
				if t.Id != winning_ticket_mark[index].Id {
					error_num++
					diffs["WinningTicketMark"] = CompareJSON(t, winning_ticket_mark[index])
				}
			}
		}
	}
	// 6.15~6.20
	blockSealEntropy := new_sf.Entropy[3]
	var c []byte
	if new_sf.GetEpochT() == 1 {
		winning_ticket := (new_sf.TicketsOrKeys.Tickets)[new_phase]
		c = ticketSealVRFInput(blockSealEntropy, uint8(winning_ticket.Attempt))
	} else {
		c = append([]byte(types.X_F), blockSealEntropy.Bytes()...)
	}

	// H_s Verification (6.15/6.16)
	H_s := header.Seal[:]
	m := header.BytesWithoutSig()
	// author_idx is the K' so we use the sf_tmp
	validatorIdx := block.Header.AuthorIndex
	signing_validator := new_sf.GetCurrValidator(int(validatorIdx))
	block_author_ietf_pub := bandersnatch.BanderSnatchKey(signing_validator.GetBandersnatchKey())
	vrfOutput, err := bandersnatch.IetfVrfVerify(block_author_ietf_pub, H_s, c, m)
	if err != nil {
		error_num++
		diffs["H_s"] = fmt.Sprintf("H_s Verification: %v", err)
	}
	// H_v Verification (6.17)
	H_v := header.EntropySource[:]
	c = append([]byte(types.X_E), vrfOutput...)
	_, err = bandersnatch.IetfVrfVerify(block_author_ietf_pub, H_v, c, []byte{})
	if err != nil {
		error_num++
		diffs["H_v"] = fmt.Sprintf("H_v Verification: %v", err)
	}
	// 6.33
	for _, block_ticket := range tickets {
		for _, a := range old_gamma_a {
			block_ticket_id, _ := block_ticket.TicketID()
			if block_ticket_id == a.Id {
				error_num++
				diffs["Ticket"] = fmt.Sprintf("ticket already in state \nticket:%s", block_ticket.String())
			}
		}
	}
	// useless key shouldn't be included in a block
	for _, block_ticket := range tickets {
		found := false
		for _, a := range new_sf.NextEpochTicketsAccumulator {
			block_ticket_id, _ := block_ticket.TicketID()
			if block_ticket_id == a.Id {
				found = true
				break
			}
		}
		if !found {
			error_num++
			diffs["Ticket"] = fmt.Sprintf("ticket not in state \nticket:%s", block_ticket.String())
		}
	}
	gamma_a_len := len(new_sf.NextEpochTicketsAccumulator)
	if gamma_a_len > types.EpochLength {
		error_num++
		diffs["NextEpochTicketsAccumulator Length"] = fmt.Sprintf("NextEpochTicketsAccumulator length %d > EpochLength %d", gamma_a_len, types.EpochLength)
	}
	for i := 1; i < gamma_a_len; i++ {
		if compareTickets(new_sf.NextEpochTicketsAccumulator[i].Id, new_sf.NextEpochTicketsAccumulator[i-1].Id) < 0 {
			error_num++
			diffs["NextEpochTicketsAccumulator"] = fmt.Sprintf("NextEpochTicketsAccumulator not sorted")
			break
		}
	}
	// tickets verification
	// using n2'
	for _, t := range tickets {
		targetEpochRandomness := new_sf.Entropy[2]
		ticketVRFInput := ticketSealVRFInput(targetEpochRandomness, t.Attempt)
		ringsetBytes := old_sf.GetRingSet("Next")
		_, err := bandersnatch.RingVrfVerify(ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
		if err != nil {
			error_num++
			diffs["Ticket"] = fmt.Sprintf("ticket verification failed \nticket:%s", t.String())
		}
	}
	// section 7
	//7.2 check
	new_history := post_state.JamState.RecentBlocks
	new_history_len := len(new_history)
	lastest_history := new_history[new_history_len-1]
	// check segment lookup p
	for _, eg := range guarantees {
		wp_hash := eg.Report.AvailabilitySpec.WorkPackageHash
		seg_root := eg.Report.AvailabilitySpec.ExportedSegmentRoot
		find := false
		for _, lookup := range lastest_history.Reported {
			if lookup.WorkPackageHash == wp_hash && lookup.SegmentRoot == seg_root {
				find = true
				break
			}
		}
		if !find {
			error_num++
			SegmentRootLookup_info := fmt.Sprintf("RecentBlocks SegmentRootLookup[%s]", wp_hash.String_short())
			diffs[SegmentRootLookup_info] = fmt.Sprintf("SegmentRootLookup not found in RecentBlocks")
		}
	}
	zero_hash := common.Hash{}
	if new_history[new_history_len-1].StateRoot != zero_hash {
		error_num++
		diffs["RecentBlocks Last StateRoot"] = fmt.Sprintf("RecentBlocks last element StateRoot is not zero")
	}
	// check parent state root
	if new_history_len > 1 {
		if new_history[new_history_len-2].StateRoot != header.ParentStateRoot {
			error_num++
			diffs["RecentBlocks Parent StateRoot"] = fmt.Sprintf("RecentBlocks last element StateRoot is not ParentStateRoot")
		}
	}

	// section 8
	//α to α' authorization pool
	old_authorization_pool := pre_state.JamState.AuthorizationsPool
	new_authorization_pool := post_state.JamState.AuthorizationsPool
	// 8.3
	for _, eg := range guarantees {
		core_index := eg.Report.CoreIndex
		for i, pool := range old_authorization_pool[core_index] {
			// remove the authorization from the pool if it is the same as the authorizer hash from the report
			if pool == eg.Report.AuthorizerHash {
				old_authorization_pool[core_index] = append(old_authorization_pool[core_index][:i], old_authorization_pool[core_index][i+1:]...)
				break
			}
		}
	}
	// 8.2
	new_authorizatioin_queue := post_state.JamState.AuthorizationQueue
	mod_time_slot := new_slot % types.MaxAuthorizationQueueItems
	for core_idx, pool := range old_authorization_pool {
		ready_queue := new_authorizatioin_queue[core_idx][mod_time_slot]
		pool = append(pool, ready_queue)
		if len(pool) > types.MaxAuthorizationPoolItems {
			// if the pool is full, remove the oldest item
			head := len(pool) - types.MaxAuthorizationPoolItems
			pool = pool[head:]
		}
		old_authorization_pool[core_idx] = pool
	}
	// compare the authorization pool
	if !reflect.DeepEqual(old_authorization_pool, new_authorization_pool) {
		error_num++
		diffs["AuthorizationPool is different"] = CompareJSON(old_authorization_pool, new_authorization_pool)
	}
	if len(new_authorization_pool) > types.MaxAuthorizationPoolItems {
		error_num++
		diffs["AuthorizationPool length"] = fmt.Sprintf("AuthorizationPool length %d > %d", len(new_authorization_pool), types.MaxAuthorizationPoolItems)
	}
	// β to β' recent history
	return error_num, diffs
}

func CompareJSON(obj1, obj2 interface{}) string {
	json1, err1 := json.Marshal(obj1)
	json2, err2 := json.Marshal(obj2)
	if err1 != nil || err2 != nil {
		return "Error marshalling JSON"
	}
	opts := jsondiff.DefaultJSONOptions()
	diff, diffStr := jsondiff.Compare(json1, json2, &opts)

	if diff == jsondiff.FullMatch {
		return "JSONs are identical"
	}
	return fmt.Sprintf("Diff detected:\n%s", diffStr)
}
