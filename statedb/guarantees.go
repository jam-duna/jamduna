package statedb

import (
	"context"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// ProcessGuarantees applies guarantees to JamState, tracking signature counts.
func (j *JamState) ProcessGuarantees(ctx context.Context, guarantees []types.Guarantee) (map[uint16]uint16, error) {
	reports := make(map[uint16]uint16)

	for _, g := range guarantees {
		// tally signature counts
		for _, sig := range g.Signatures {
			reports[sig.ValidatorIndex]++
		}

		// skip invalid core indices
		if idx := g.Report.CoreIndex; idx >= types.TotalCores {
			log.Warn(debugG, "invalid core index", "core", idx)
			continue
		}

		// assign first report to core
		if j.AvailabilityAssignments[int(g.Report.CoreIndex)] == nil {
			j.SetRhoByWorkReport(uint16(g.Report.CoreIndex), g.Report, j.SafroleState.GetTimeSlot())
			log.Trace(debugG, "assigned core", "core", g.Report.CoreIndex)
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

// SetRhoByWorkReport updates the Rho state for a given core.
func (s *JamState) SetRhoByWorkReport(core uint16, report types.WorkReport, slot uint32) {
	s.AvailabilityAssignments[int(core)] = &Rho_state{WorkReport: report, Timeslot: slot}
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
		return jamerrors.ErrGDuplicateGuarantors
	}

	// signature verification against current or previous validator set
	validators := s.chooseValidatorSet(g.Slot)
	if err := g.Verify(validators); err != nil {
		return jamerrors.ErrGBadSignature
	}

	// pending or timeout checks on JamState
	js := s.JamState
	if err := js.checkReportPendingOnCore(g); err != nil {
		return err
	}
	if s.Block != nil {
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

	return nil
}

// v0.5 eq 11.25 - The signing validators must be assigned to the core in G or G*
func (s *StateDB) checkAssignment(g types.Guarantee, ts uint32) error {
	// old reports aren’t allowed
	diff := int64(ts) - int64(g.Slot)

	if diff > int64(ts%types.ValidatorCoreRotationPeriod+types.ValidatorCoreRotationPeriod) {
		fmt.Printf("checkAssignment: diff: %d\n", diff)
		return jamerrors.ErrGReportEpochBeforeLast
	}

	// choose prev vs curr assignment based on period
	period := ts / types.ValidatorCoreRotationPeriod
	reportPeriod := g.Slot / types.ValidatorCoreRotationPeriod
	prevAssign, currAssign := s.CalculateAssignments(ts)
	assignments := currAssign
	if period != reportPeriod {
		assignments = prevAssign
	}

	// build validator→core map
	lookup := make(map[uint16]uint16, len(assignments))
	for idx, a := range assignments {
		lookup[uint16(idx)] = a.CoreIndex
	}

	// verify each signature lands on the expected core
	for _, sig := range g.Signatures {
		core, ok := lookup[sig.ValidatorIndex]
		if !ok || core != uint16(g.Report.CoreIndex) {
			log.Warn(debugG, "checkAssignment: core assignment", "validator", sig.ValidatorIndex, "expectedCore", g.Report.CoreIndex, "actualCore", core)
			return jamerrors.ErrGWrongAssignment
		}
	}

	return nil
}

// Helper: chooseValidatorSet returns Prev or Curr based on slot.
func (s *StateDB) chooseValidatorSet(slot uint32) types.Validators {
	if s.IsPreviousValidators(slot) {
		return s.JamState.SafroleState.PrevValidators
	}
	return s.JamState.SafroleState.CurrValidators
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
		if s[i-1].ValidatorIndex >= s[i].ValidatorIndex {
			return jamerrors.ErrGDuplicateGuarantors
		}
	}
	return nil
}

// v0.5 eq 11.28 - check pending report
func (j *JamState) checkReportTimeOut(g types.Guarantee, ts uint32) error {
	if g.Slot > ts {
		return jamerrors.ErrGFutureReportSlot
	}
	if j.AvailabilityAssignments[int(g.Report.CoreIndex)] == nil {
		return nil
	}
	timeoutbool := ts >= (j.AvailabilityAssignments[int(g.Report.CoreIndex)].Timeslot)+uint32(types.UnavailableWorkReplacementPeriod)
	if timeoutbool {
		return nil
	} else {
		return jamerrors.ErrGCoreEngaged
	}
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
		fmt.Printf("checkReportPendingOnCore: %v\n", g.String())
		return jamerrors.ErrGCoreUnexpectedAuthorizer
	}
	return nil
}

func (j *JamState) CheckInvalidCoreIndex() {
	problem := false
	for i, rho := range j.AvailabilityAssignments {
		if rho != nil && rho.WorkReport.CoreIndex != uint(i) {
			problem = true
		}
	}
	// Core 0 : receiving megatron report; Core 1 : receiving fib+trib report
	if problem {
		for i, rho := range j.AvailabilityAssignments {
			log.Trace(debugG, "CheckInvalidCoreIndex", "i", i, "rho.WorkReportHash", rho.WorkReport.Hash(),
				"CoreIndex", rho.WorkReport.CoreIndex, "WorkReport", rho.WorkReport.String())
		}
		log.Crit(debugG, "CheckInvalidCoreIndex: FAILURE")
	}
	log.Trace(debugG, "CheckInvalidCoreIndex: success")

}

func (s *StateDB) checkServicesExist(g types.Guarantee) error {
	for _, result := range g.Report.Results {
		// check if serviceID exists
		_, ok, err := s.GetService(result.ServiceID)
		if err != nil {
			return err
		}
		if !ok {
			log.Warn(debugG, "checkServicesExist: serviceID not found", "serviceID", result.ServiceID, "slot", s.GetTimeslot())
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

	// current gas allocation is unlimited
	if sum_rg <= types.AccumulationGasAllocation {
		for _, results := range g.Report.Results {
			serviceID := results.ServiceID
			if service, ok, err := s.GetService(serviceID); ok && err == nil {
				gas_limitG := service.GasLimitG
				if results.Gas >= gas_limitG {
					return nil
				} else {
					log.Warn(module, "ErrGServiceItemTooLow", "service", service, "results.Gas", results.Gas, "service.GasLimitG", service.GasLimitG)
					return jamerrors.ErrGServiceItemTooLow
				}
			}
		}
	}

	return jamerrors.ErrGWorkReportGasTooHigh
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
	for i := len(s.JamState.RecentBlocks) - 1; i >= 0; i-- {
		block := s.JamState.RecentBlocks[i]

		if !anchor && block.HeaderHash == refine.Anchor {
			anchor = true
		}
		if !stateroot && block.StateRoot == refine.StateRoot {
			stateroot = true
		}
		if !beefyroot {
			superPeak := block.B.SuperPeak()
			if *superPeak == refine.BeefyRoot {
				beefyroot = true
			}
		}

		if anchor && stateroot && beefyroot {
			return nil
		}
	}
	// ****** TEMPORARY SOLUTION ******
	return nil
	if !anchor {
		log.Warn(debugG, "checkRecentBlock:anchor not in recent blocks", "refine.Anchor", refine.Anchor)
		return jamerrors.ErrGAnchorNotRecent
	}
	if !stateroot {
		log.Warn(debugG, "checkRecentBlock:state root not in recent blocks", "refine.StateRoot", refine.StateRoot)
		return jamerrors.ErrGBadStateRoot
	}
	if !beefyroot {
		log.Warn(debugG, "checkRecentBlock:beefy root not in recent blocks", "refine.BeefyRoot", refine.BeefyRoot)
		return jamerrors.ErrGBadBeefyMMRRoot
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
func (s *StateDB) getPrereqFromRho() []common.Hash {
	result := []common.Hash{}
	for _, rho := range s.JamState.AvailabilityAssignments {
		if rho != nil && len(rho.WorkReport.RefineContext.Prerequisites) != 0 {
			result = append(result, rho.WorkReport.RefineContext.Prerequisites...)
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
	for _, block := range s.JamState.RecentBlocks {
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
	prereqSetFromRho := make(map[common.Hash]struct{})
	for _, hash := range s.getPrereqFromRho() {
		prereqSetFromRho[hash] = struct{}{}
	}
	_, exists = prereqSetFromRho[workPackageHash]
	if exists {
		// TODO: REVIEW non-standard error
		return fmt.Errorf("invalid prerequisite work package(from rho), core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
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
	for _, block := range s.JamState.RecentBlocks {
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
			log.Error(debugG, "checkPrereq", "missing hash", hash.String())
			return jamerrors.ErrGDependencyMissing
		}
	}

	return nil
}

// v0.5 eq 11.39
func getPresentBlock(s *StateDB) types.SegmentRootLookup {
	p := types.SegmentRootLookup{}
	for _, block := range s.JamState.RecentBlocks {
		for _, lookupItem := range block.Reported {
			tmeItem := types.SegmentRootLookupItem{
				WorkPackageHash: lookupItem.WorkPackageHash,
				SegmentRoot:     lookupItem.SegmentRoot,
			}
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

	if len(s.JamState.RecentBlocks) == 0 {
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
			return jamerrors.ErrGBadCodeHash
		}
		if codeHash != service.CodeHash {
			return jamerrors.ErrGBadCodeHash
		}
	}
	return nil
}

func (s *StateDB) IsPreviousValidators(eg_timeslot uint32) bool {
	curr_timeslot := s.GetTimeslot()
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
