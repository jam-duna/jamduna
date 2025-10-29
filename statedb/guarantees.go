package statedb

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	jamerrors "github.com/colorfulnotion/jam/jamerrors"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// ProcessGuarantees applies guarantees to JamState, tracking signature counts.
func (j *JamState) ProcessGuarantees(ctx context.Context, guarantees []types.Guarantee, prev_assignment types.GuarantorAssignments) (map[types.Ed25519Key]uint16, error) {
	reports := make(map[types.Ed25519Key]uint16)
	for _, g := range guarantees {
		// tally signature counts
		if g.Slot/types.RotationPeriod == j.SafroleState.Timeslot/types.RotationPeriod {
			for _, sig := range g.Signatures {
				index := sig.ValidatorIndex
				if uint32(index) < uint32(len(j.SafroleState.CurrValidators)) {
					ed25519key := j.SafroleState.CurrValidators[int(index)].Ed25519
					reports[ed25519key]++
					// fmt.Printf("Validator %d: %s (CurrValidators)\n", index, ed25519key)
				}
			}
		} else {
			for _, sig := range g.Signatures {
				index := sig.ValidatorIndex
				// get the key from the previous validator set
				if uint32(index) < uint32(len(j.SafroleState.PrevValidators)) {
					ed25519key := prev_assignment[index].Validator.Ed25519
					// fmt.Printf("Validator %d: %s (PrevValidators)\n", index, ed25519key)
					reports[ed25519key]++
				}
			}
		}

		// skip invalid core indices
		if idx := g.Report.CoreIndex; idx >= types.TotalCores {
			log.Warn(log.G, "invalid core index", "core", idx)
			continue
		}

		// assign first report to core
		if j.AvailabilityAssignments[int(g.Report.CoreIndex)] == nil {
			j.SetAvailabilityAssignmentsByWorkReport(uint16(g.Report.CoreIndex), g.Report, j.SafroleState.GetTimeSlot())
		} else {
			continue
		}
		// cancellation check
		select {
		case <-ctx.Done():
			return reports, fmt.Errorf("ProcessGuarantees canceled")
		default:
		}
	}

	return reports, nil
}

// SetAvailabilityAssignmentsByWorkReport updates the AvailabilityAssignments state for a given core.
func (s *JamState) SetAvailabilityAssignmentsByWorkReport(core uint16, report types.WorkReport, slot uint32) {
	s.AvailabilityAssignments[int(core)] = &CoreState{WorkReport: report, Timeslot: slot}
}

// acceptableGuaranteeError categorizes errors into "acceptable" vs "not" -- if something is acceptable, it could be in the pool *temporarily*  being invalid and later be valid and error free
func AcceptableGuaranteeError(err error) bool {
	if err == jamerrors.ErrGFutureReportSlot || err == jamerrors.ErrGCoreEngaged || err == jamerrors.ErrGWrongAssignment {
		return true
	}
	return false
}

// VerifyGuaranteeBasic checks signatures, core index, assignment, timeouts, gas, and code hash.
func (s *StateDB) VerifyGuaranteeBasic(g types.Guarantee, targetJCE uint32) error {
	// common validations
	if err := s.checkServicesExist(g); err != nil {
		return err
	}
	if g.Report.CoreIndex >= types.TotalCores {
		return jamerrors.ErrGBadCoreIndex
	}
	if len(g.Signatures) < 2 {
		return jamerrors.ErrGInsufficientGuarantees
	}

	// validator index uniqueness and sorting
	if err := CheckSortedSignatures(g); err != nil {
		return err
	}

	// pending or timeout checks on JamState
	js := s.JamState
	if err := js.checkReportPendingOnCore(g); err != nil {
		return err
	}
	if targetJCE > 0 {
		// assignment to core
		if err := s.checkAssignment(g, targetJCE); err != nil {
			return err
		}
		if err := js.checkReportTimeOut(g, targetJCE); err != nil {
			return err
		}
	}

	// additional block-level checks
	for _, fn := range []func(types.Guarantee) error{s.checkTimeSlotHeader, s.checkRecentBlock, s.checkAnyPrereq, s.checkCodeHash, s.checkGas} {
		if err := fn(g); err != nil {
			return err
		}
	}

	// signature verification against current or previous validator set
	validators, prevValidators := s.JamState.chooseValidatorSets(g.Slot)
	if err := g.Verify(validators, prevValidators); err != nil {
		return err
	}

	return nil
}

// v0.5 eq 11.25 - The signing validators must be assigned to the core in G or G*
func (s *StateDB) checkAssignment(g types.Guarantee, ts uint32) error {
	// old reports aren’t allowed
	diff := int64(ts) - int64(g.Slot)
	if diff > int64(ts%types.ValidatorCoreRotationPeriod+types.ValidatorCoreRotationPeriod) {
		return jamerrors.ErrGReportEpochBeforeLast
	}

	// choose prev vs curr assignment based on period
	prev_assignments, assignments := s.CalculateAssignments(ts)
	s.GuarantorAssignments = assignments
	s.PreviousGuarantorAssignments = prev_assignments
	lookup := make(map[uint16]uint16, len(assignments))
	if ts/types.ValidatorCoreRotationPeriod == (g.Slot / types.ValidatorCoreRotationPeriod) {
		for idx, a := range assignments {
			lookup[uint16(idx)] = a.CoreIndex
		}
	} else {
		for idx, a := range prev_assignments {
			lookup[uint16(idx)] = a.CoreIndex
		}
	}
	// verify each signature lands on the expected core
	for _, sig := range g.Signatures {
		core, ok := lookup[sig.ValidatorIndex]
		if !ok || core != uint16(g.Report.CoreIndex) {
			log.Warn(log.G, "checkAssignment: G14-ErrGWrongAssignment",
				"validator", sig.ValidatorIndex,
				"ok", ok,
				"slot", g.Slot,
				"expectedCore", g.Report.CoreIndex,
				"actualCore", core)
			return jamerrors.ErrGWrongAssignment
		}
		// fmt.Printf("Validator %d: %s (Assigned to Core %d), g slot %d\n", sig.ValidatorIndex, assignments[sig.ValidatorIndex].Validator.Ed25519, core, g.Slot)
	}
	return nil
}

// Helper: chooseValidatorSet returns Prev or Curr based on slot.
func (j *JamState) chooseValidatorSet(slot uint32) types.Validators {
	if j.IsPreviousValidators(slot) {
		return j.SafroleState.PrevValidators
	}
	return j.SafroleState.CurrValidators
}
func (j *JamState) chooseValidatorSets(slot uint32) (types.Validators, types.Validators) {
	// if j.IsPreviousValidators(slot) {
	// 	return j.SafroleState.PrevValidators, j.SafroleState.CurrValidators
	// }
	return j.SafroleState.CurrValidators, j.SafroleState.PrevValidators
}

// CheckSortedGuarantees ensures guarantees are sorted by core index.
func CheckSortedGuarantees(gs []types.Guarantee) error {
	for i := 1; i < len(gs); i++ {
		if gs[i-1].Report.CoreIndex >= gs[i].Report.CoreIndex {
			return jamerrors.ErrGOutOfOrderGuarantee
		}
	}
	return nil
}

// CheckSortedSignatures ensures signature slice is sorted by validator index.
func CheckSortedSignatures(g types.Guarantee) error {
	s := g.Signatures
	for i := 1; i < len(s); i++ {
		if s[i].ValidatorIndex >= types.TotalValidators {
			return jamerrors.ErrGBadValidatorIndex
		}
		if s[i-1].ValidatorIndex >= s[i].ValidatorIndex {
			return jamerrors.ErrGDuplicateGuarantors
		}
	}
	return nil
}

// v0.5 eq 11.28 - check pending report
func (j *JamState) checkReportTimeOut(g types.Guarantee, ts uint32) error {
	if ts == 0 {
		// nothing to check
		return nil
	}
	if g.Slot > ts {
		return jamerrors.ErrGFutureReportSlot
	}
	if j.AvailabilityAssignments[int(g.Report.CoreIndex)] == nil {
		return nil
	}
	timeoutbool := ts >= (j.AvailabilityAssignments[int(g.Report.CoreIndex)].Timeslot)+uint32(types.UnavailableWorkReplacementPeriod)
	if timeoutbool {
		return nil
	}

	log.Warn(log.G, "checkReportTimeOut: G14-ErrGCoreEngaged",
		"core", g.Report.CoreIndex,
		"ts", ts,
		"timeslot", j.AvailabilityAssignments[int(g.Report.CoreIndex)].Timeslot,
		"currentTimeslot", ts,
		"unavailablePeriod", types.UnavailableWorkReplacementPeriod)
	// if the core is engaged, we return an error
	return jamerrors.ErrGCoreEngaged
}

// v0.5 eq 11.28
func (j *JamState) checkReportPendingOnCore(g types.Guarantee) error {

	authorizations_pool := j.AuthorizationsPool[int(g.Report.CoreIndex)]
	find := false
	if len(authorizations_pool) == 0 {
		return jamerrors.ErrGCoreWithoutAuthorizer
	}
	for _, authrization := range authorizations_pool {
		if authrization == g.Report.AuthorizerHash {
			find = true
			break
		}
	}
	if !find {
		return jamerrors.ErrGCoreUnexpectedAuthorizer
	}
	return nil
}

func (j *JamState) CheckInvalidCoreIndex() {
	problem := false
	for i, availability_assignment := range j.AvailabilityAssignments {
		if availability_assignment != nil && availability_assignment.WorkReport.CoreIndex != uint(i) {
			problem = true
		}
	}
	// Core 0 : receiving megatron report; Core 1 : receiving fib+trib report
	if problem {
		for i, availability_assignment := range j.AvailabilityAssignments {
			log.Trace(log.G, "CheckInvalidCoreIndex", "i", i, "availability_assignment.WorkReportHash", availability_assignment.WorkReport.Hash(),
				"CoreIndex", availability_assignment.WorkReport.CoreIndex, "WorkReport", availability_assignment.WorkReport.String())
		}
		log.Crit(log.G, "CheckInvalidCoreIndex: FAILURE")
	}
	log.Trace(log.G, "CheckInvalidCoreIndex: success")

}

func (s *StateDB) checkServicesExist(g types.Guarantee) error {
	for _, result := range g.Report.Results {
		// check if serviceID exists
		_, ok, err := s.GetService(result.ServiceID)
		if err != nil {
			return err
		}
		if !ok {
			log.Warn(log.G, "checkServicesExist: serviceID not found", "serviceID", result.ServiceID, "slot", s.GetTimeslot())
			return jamerrors.ErrGBadServiceID
		}
	}
	return nil
}

// v0.5 eq 11.29
func (s *StateDB) checkGas(g types.Guarantee) error {
	sum_rg := uint64(0)
	for _, results := range g.Report.Results {
		sum_rg += results.Gas
	}
	// check if gas is within limits
	if sum_rg > types.AccumulationGasAllocation {
		return jamerrors.ErrGWorkReportGasTooHigh
	}
	// current gas allocation is unlimited
	for _, results := range g.Report.Results {
		serviceID := results.ServiceID
		service, ok, err := s.GetService(serviceID)
		if err != nil {
			log.Warn(log.SDB, "checkGas: GetService Error", "serviceID", serviceID, "err", err)
			continue
		}
		if !ok {
			log.Warn(log.SDB, "checkGas: serviceID not found", "serviceID", serviceID, "slot", s.GetTimeslot())
			continue
		}
		if results.Gas < service.GasLimitG {
			log.Warn(log.SDB, "ErrGServiceItemTooLow", "service", service, "results.Gas", results.Gas, "service.GasLimitG", service.GasLimitG)
			return jamerrors.ErrGServiceItemTooLow
		}
	}
	return nil
}

// v0.5 eq 11.31
func (s *StateDB) checkLength() error {
	p_w := make(map[common.Hash]common.Hash)
	for _, eg := range s.Block.Extrinsic.Guarantees {
		if _, exists := p_w[eg.Report.GetWorkPackageHash()]; exists {
			return jamerrors.ErrGDuplicatePackageTwoReports
		}
		p_w[eg.Report.GetWorkPackageHash()] = eg.Report.AvailabilitySpec.WorkPackageHash
	}

	return nil
}

// v0.5 eq 11.32
func (s *StateDB) checkRecentBlock(g types.Guarantee) error {
	refine := g.Report.RefineContext
	// check if the refine context anchor/stateroot/beefy root is in the recent blocks
	anchor := false
	stateroot := false
	beefyroot := false
	// goes backwards, short-circuits each of above 3 conditions
	for i := len(s.JamState.RecentBlocks.B_H) - 1; i >= 0; i-- {
		block := s.JamState.RecentBlocks.B_H[i]

		if !anchor && block.HeaderHash == refine.Anchor {
			anchor = true
		}
		if !stateroot && block.StateRoot == refine.StateRoot {
			stateroot = true
		}
		if !beefyroot {
			superPeak := block.B
			if superPeak == refine.BeefyRoot {
				beefyroot = true
			}
		}

		if anchor && stateroot && beefyroot {
			return nil
		}
	}

	if !anchor {
		//log.Warn(log.G, "checkRecentBlock:anchor not in recent blocks", "refine.Anchor", refine.Anchor)
		//return jamerrors.ErrGAnchorNotRecent
	}
	if !stateroot {
		//TMPlog.Warn(log.G, "checkRecentBlock:state root not in recent blocks", "refine.StateRoot", refine.StateRoot)

		// MK WARNING: this is failing on 0.7.0 corevm
		//return jamerrors.ErrGBadStateRoot
	}
	if !beefyroot {
		//		log.Warn(log.G, "checkRecentBlock:beefy root not in recent blocks", "refine.BeefyRoot", refine.BeefyRoot)
	}
	return nil
}

// v0.5 eq 11.33
func (s *StateDB) checkTimeSlotHeader(g types.Guarantee) error {
	var valid_anchor uint32
	valid_anchor = s.Block.TimeSlot() - types.LookupAnchorMaxAge
	if types.LookupAnchorMaxAge > s.Block.TimeSlot() {
		valid_anchor = 0
	}
	if g.Report.RefineContext.LookupAnchorSlot >= valid_anchor {
		return nil
	} else {
		fmt.Printf("lookup anchor slot %d before last %d\n", g.Report.RefineContext.LookupAnchorSlot, valid_anchor)
		return jamerrors.ErrGReportEpochBeforeLast
	}
}

// TODO: v0.5 eq 11.35
/*
func (s *StateDB) getPrereqFromAccumulationQueue() []common.Hash {
	result := []common.Hash{}
	fmt.Printf("s.JamState.AccumulationQueue %v\n", s.JamState.AccumulationQueue)
	for i := 0; i < types.EpochLength; i++ {
		for _, queues := range s.JamState.AccumulationQueue[i] {
			workreport := queues.WorkReport
			if len(workreport.RefineContext.Prerequisites) != 0 {
				result = append(result, workreport.RefineContext.Prerequisites...)
			}
		}
	}
	return result
}
*/
// TODO: v0.5 eq 11.36
func (s *StateDB) getPrereqFromAvailabilityAssignments() []common.Hash {
	result := []common.Hash{}
	for _, availability_assignment := range s.JamState.AvailabilityAssignments {
		if availability_assignment != nil && len(availability_assignment.WorkReport.RefineContext.Prerequisites) != 0 {
			result = append(result, availability_assignment.WorkReport.RefineContext.Prerequisites...)
		}
	}
	return result
}

// v0.5 eq 11.37
// v0.5.2 eq 11.39
// TODO:stanley ∀ p ∈ 𝒑, p ∉ ⋃{keys(x𝒑) | x ∈ β} ∪ ⋃{x | x ∈ accumulated} ∪ 𝒒 ∪ 𝒂
func (s *StateDB) checkAnyPrereq(g types.Guarantee) error {
	// prereqSetFromQueue := make(map[common.Hash]struct{})
	// for _, hash := range s.getPrereqFromAccumulationQueue() {
	// 	prereqSetFromQueue[hash] = struct{}{}
	// }
	workPackageHash := g.Report.AvailabilitySpec.WorkPackageHash
	if workPackageHash == (common.Hash{}) {
		// TODO: REVIEW non-standard error
		return fmt.Errorf("invalid work package hash")
	}
	// _, exists := prereqSetFromQueue[workPackageHash]
	// if exists {
	// 	fmt.Printf("invalid prerequisite work package(from queue), core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	// 	return jamerrors.ErrGDuplicatePackageRecentHistory
	// }
	for _, block := range s.JamState.RecentBlocks.B_H {
		if len(block.Reported) != 0 {
			for _, segmentRootLookup := range block.Reported {
				if segmentRootLookup.WorkPackageHash == workPackageHash {
					return jamerrors.ErrGDuplicatePackageRecentHistory
				}
			}
		}
	}

	prereqSetFromAccumulationHistory := make(map[common.Hash]struct{})
	for i := 0; i < types.EpochLength; i++ {
		for _, hash := range s.JamState.AccumulationHistory[i].WorkPackageHash {
			prereqSetFromAccumulationHistory[hash] = struct{}{}
		}
	}
	_, exists := prereqSetFromAccumulationHistory[workPackageHash]
	if exists {
		// TODO: REVIEW non-standard error
		return fmt.Errorf("invalid prerequisite work package(from accumulation history), core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}

	// 𝒒:Means the workPackageHash should not be in the ready work package set
	accumulateWorkPackage := s.AvailableWorkReport
	readyWorkPackagesHashes := s.GetReadyQueue(accumulateWorkPackage)
	readyWorkPackagesHashMap := make(map[common.Hash]struct{})
	for _, hash := range readyWorkPackagesHashes {
		readyWorkPackagesHashMap[hash] = struct{}{}
	}
	_, exists = readyWorkPackagesHashMap[workPackageHash]
	if exists {
		// TODO: REVIEW non-standard error
		return fmt.Errorf("invalid prerequisite work package(from ready work package), core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}

	// 𝒂:This is a collection of "assigned" work packages.
	prereqSetFromAvailabilityAssignments := make(map[common.Hash]struct{})
	for _, hash := range s.getPrereqFromAvailabilityAssignments() {
		prereqSetFromAvailabilityAssignments[hash] = struct{}{}
	}
	_, exists = prereqSetFromAvailabilityAssignments[workPackageHash]
	if exists {
		// TODO: REVIEW non-standard error
		return fmt.Errorf("invalid prerequisite work package(from availability_assignment), core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}
	return nil
}

// v0.5 eq 11.38

// this function is for the making block
func (s *StateDB) checkPrereq(g types.Guarantee, EGs []types.Guarantee) error {
	prereqSet := make(map[common.Hash]bool)
	p := []common.Hash{}
	if len(g.Report.RefineContext.Prerequisites) != 0 {
		p = append(p, g.Report.RefineContext.Prerequisites...)
	}
	for _, lookupItem := range g.Report.SegmentRootLookup {
		p = append(p, lookupItem.WorkPackageHash)
	}

	// check if we only get her after accumulate..
	for _, block := range s.JamState.RecentBlocks.B_H {
		for _, segmentRootLookup := range block.Reported {
			prereqSet[segmentRootLookup.WorkPackageHash] = true
		}
	}

	for _, guarantee := range EGs {
		workPackageHash := guarantee.Report.AvailabilitySpec.WorkPackageHash
		prereqSet[workPackageHash] = true
	}

	if len(p) == 0 {
		return nil
	}

	for _, hash := range p {
		exists := false
		for knownWPHash := range prereqSet {
			if hash == knownWPHash {
				exists = true
				break
			}
		}
		if !exists {
			log.Error(log.G, "checkPrereq", "missing hash", hash.String())
			return jamerrors.ErrGDependencyMissing
		}
	}

	return nil
}

// v0.5 eq 11.39
func getPresentBlock(s *StateDB) types.SegmentRootLookup {
	p := types.SegmentRootLookup{}
	for _, block := range s.JamState.RecentBlocks.B_H {
		for _, lookupItem := range block.Reported {
			tmeItem := types.SegmentRootLookupItem(lookupItem)
			p = append(p, tmeItem)
		}
	}
	return p
}

// v0.5 eq 11.40
// v0.5.2 eq 11.42
func (s *StateDB) checkRecentWorkPackage(g types.Guarantee, egs []types.Guarantee) error {
	currentSegmentRootLookUp := g.Report.SegmentRootLookup
	if len(currentSegmentRootLookUp) == 0 {
		return nil
	}

	if len(s.JamState.RecentBlocks.B_H) == 0 {
		return nil
	}
	// Combine the present block and the recent blocks
	presentBlockSegmentRootLookup := getPresentBlock(s)
	// Check presentBlockHash2Hash include currentHash2Hash or not
	segmentLookUpIncluded := make([]bool, len(currentSegmentRootLookUp))
	for i, currentSegmentRootLookup := range currentSegmentRootLookUp {
		for _, segmentRootLookup := range presentBlockSegmentRootLookup {
			if segmentRootLookup.WorkPackageHash == currentSegmentRootLookup.WorkPackageHash {
				if segmentRootLookup.SegmentRoot != currentSegmentRootLookup.SegmentRoot {
					return jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue
				} else {
					segmentLookUpIncluded[i] = true
					break
				}
			}
			segmentLookUpIncluded[i] = false
		}
	}
	for _, guarantee := range egs {
		wp_hash := guarantee.Report.AvailabilitySpec.WorkPackageHash
		segment_root := guarantee.Report.AvailabilitySpec.ExportedSegmentRoot
		for i, lookup := range currentSegmentRootLookUp {
			if wp_hash == lookup.WorkPackageHash {
				if segment_root != lookup.SegmentRoot {
					return jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue
				} else {
					segmentLookUpIncluded[i] = true
				}
			}
		}
	}

	// Check if all the segmentRootLookup are included
	for _, included := range segmentLookUpIncluded {
		if !included {
			return jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks
		}
	}

	return nil
}

// v0.5 eq 11.41
func (s *StateDB) checkCodeHash(g types.Guarantee) error {
	//prior_trie := s.CopyTrieState(s.StateRoot)
	for _, result := range g.Report.Results {
		serviceID := result.ServiceID
		codeHash := result.CodeHash
		service, ok, err := s.GetService(serviceID)
		if err != nil {
			return err
		}
		if !ok {
			log.Warn(log.G, "checkCodeHash: serviceID not found", "serviceID", serviceID, "slot", s.GetTimeslot())
			return jamerrors.ErrGBadCodeHash
		}
		if codeHash != service.CodeHash {
			log.Warn(log.G, "checkCodeHash: codeHash != service.CodeHash", "serviceID", serviceID, "codeHash", codeHash, "service.CodeHash", service.CodeHash)
			return jamerrors.ErrGBadCodeHash
		}
	}
	return nil
}

func (j *JamState) IsPreviousValidators(eg_timeslot uint32) bool {
	curr_timeslot := j.SafroleState.GetTimeSlot()
	assignment_idx := curr_timeslot / types.ValidatorCoreRotationPeriod
	previous_assignment_idx := assignment_idx - 1
	eg_assignment_idx := eg_timeslot / types.ValidatorCoreRotationPeriod
	if eg_assignment_idx == curr_timeslot {
		return false
	} else if eg_assignment_idx == previous_assignment_idx {
		if eg_timeslot/types.EpochLength+1 == curr_timeslot/types.EpochLength {
			return true
		} else {
			return false
		}
	}
	return false
}
