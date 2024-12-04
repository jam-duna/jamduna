package statedb

import (
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
)

// chapter 11
const (
// TODO: Stanley - ensure that all 5 of these are used
// jamerrors.ErrAnchorNotRecent: Context anchor is not recent enough.
// jamerrors.ErrGBadStateRoot: Context state root doesn't match the one at anchor.
// jamerrors.ErrGDuplicatePackageRecentHistory: Package was already available in recent history.
// jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue
// jamerrors.ErrGCoreWithoutAuthorizer: Target core without any authorizer.
)

// v0.5 eq 11.42 - the rho state transition function
func (j *JamState) ProcessGuarantees(guarantees []types.Guarantee) {
	for _, guarantee := range guarantees {
		if guarantee.Report.CoreIndex >= types.TotalCores {
			fmt.Printf("ProcessGuarantees: invalid core index %v\n", guarantee.Report.CoreIndex)
			continue
		}
		if j.AvailabilityAssignments[int(guarantee.Report.CoreIndex)] == nil {
			j.SetRhoByWorkReport(guarantee.Report.CoreIndex, guarantee.Report, j.SafroleState.GetTimeSlot())
			if debug {
				fmt.Printf("ProcessGuarantees Success on Core %v\n", guarantee.Report.CoreIndex)
			}
		}
	}
}

// setRhoByWorkReport sets the Rho state for a specific core with a WorkReport and timeslot
func (state *JamState) SetRhoByWorkReport(core uint16, w types.WorkReport, t uint32) {
	state.AvailabilityAssignments[int(core)] = &Rho_state{
		WorkReport: w,
		Timeslot:   t,
	}
}

// this function is the strictest one, is for the verification after the state transition before the state gets updated by extrinsic guarantees
func (s *StateDB) Verify_Guarantees() error {
	// v0.5 eq 11.23
	err := CheckSorting_EGs(s.Block.Extrinsic.Guarantees)
	if err != nil {
		return err
	}
	for _, guarantee := range s.Block.Extrinsic.Guarantees {
		err = s.Verify_Guarantee(guarantee)
		if err != nil {
			return err
		}
	}
	// v0.5 eq 11.31
	err = s.checkLength() // not sure if this is correct behavior
	if err != nil {
		return err
	}
	// for recent history and extrinsics in the block, so it should be here
	for _, guarantee := range s.Block.Extrinsic.Guarantees {
		// v0.5 eq 11.38
		err := s.checkPrereq(guarantee)
		if err != nil {
			return jamerrors.ErrGDependencyMissing
		}
	}
	return nil
}

// this function will accept one guarantee at a time, it will be used by make block to make sure which guarantee should be included in the block
func (s *StateDB) Verify_Guarantee_MakeBlock(guarantee types.Guarantee) error {
	max_core := types.TotalCores - 1
	if guarantee.Report.CoreIndex > uint16(max_core) {
		return jamerrors.ErrGBadCoreIndex
	}
	max_validator := types.TotalValidators - 1
	for _, g := range guarantee.Signatures {
		if g.ValidatorIndex > uint16(max_validator) {
			return jamerrors.ErrGBadValidatorIndex
		}
	}
	if len(guarantee.Signatures) < 2 {
		return jamerrors.ErrGInsufficientGuarantees
	}

	// v0.5 eq 11.24 - check index
	err := CheckSorting_EG(guarantee)
	if err != nil {
		return jamerrors.ErrGDuplicateGuarantors
	}

	// v0.5 eq 11.25 - check signature, core assign check,C_v ...
	CurrV := s.JamState.SafroleState.CurrValidators
	err = guarantee.Verify(CurrV) // errBadSignature
	if err != nil {
		return jamerrors.ErrGBadSignature
	}
	// v0.5 eq 11.25 - The signing validators must be assigned to the core in G or G*
	// custom function for make block
	err = s.AreValidatorsAssignedToCore_MakeBlock(guarantee)
	if err != nil {
		fmt.Printf("Verify_Guarantee error MakeBlock: %v\n", err)
		return jamerrors.ErrGWrongAssignment
	}
	j := s.JamState
	// v0.5 eq 11.28
	if s.Block != nil {
		err = j.CheckReportTimeOut(guarantee, s.Block.TimeSlot())
		if err != nil {
			return err
		}
	}

	// v.05 eq 11.29 - check gas
	err = s.checkGas(guarantee)
	if err != nil {
		return jamerrors.ErrGWorkReportGasTooHigh
	}

	// v0.5 eq 11.33
	err = s.checkTimeSlotHeader(guarantee)
	if err != nil {
		return jamerrors.ErrGAnchorNotRecent
	}

	// v0.5 eq 11.41 - check code hash
	err = s.checkCodeHash(guarantee)
	if err != nil {
		return jamerrors.ErrGBadCodeHash
	}
	return nil
}

// this function accepts multiple guarantees at a time, it will be used by make block for dropping the invalid guarantees
// after the first picking the valid guarantees, it will be used by remaining guarantees to make sure which guarantee should be included in the block
// there are some verification need to be check with other guarantees, so it should be used after the first picking
func (s *StateDB) Verify_Guarantees_MakeBlock(EGs []types.Guarantee) ([]types.Guarantee, error, bool) {
	// v0.5 eq 11.23 - check index
	var valid bool
	err := CheckSorting_EGs(EGs)
	if err != nil {
		return nil, err, false
	}
	for i, guarantee := range EGs {
		// v0.5 eq 11.24  - check index
		err := CheckSorting_EG(guarantee)
		if err != nil {
			EGs = append(EGs[:i], EGs[i+1:]...)
			valid = false
		}
		// v0.5 eq 11.38
		err = s.checkPrereqWithoutBlock(guarantee, EGs)
		if err != nil {
			fmt.Printf("Verify_Guarantees_MakeBlock error: %v\n", err)
			EGs = append(EGs[:i], EGs[i+1:]...)
			valid = false
		}

	}
	if valid {
		return EGs, nil, true
	} else {
		return EGs, nil, false
	}
}

// this function will be used by the state transition function in the single guarantee verification
// it will be called by Verify_Guarantees
func (s *StateDB) Verify_Guarantee(guarantee types.Guarantee) error {

	// for shawn
	err := s.VerifyGuarantee_Basic(guarantee)
	// err = j.CheckReportPendingOnCore(guarantee)
	if err != nil {
		return err
	}

	// for stanley
	err = s.VerifyGuarantee_RecentHistory(guarantee)
	if err != nil {
		return err
	}

	return nil
}

func (s *StateDB) VerifyGuarantee_Basic(guarantee types.Guarantee) error {
	max_core := types.TotalCores - 1
	if guarantee.Report.CoreIndex > uint16(max_core) {
		return jamerrors.ErrGBadCoreIndex
	}
	max_validator := types.TotalValidators - 1
	for _, g := range guarantee.Signatures {
		if g.ValidatorIndex > uint16(max_validator) {
			return jamerrors.ErrGBadValidatorIndex
		}
	}
	if len(guarantee.Signatures) < 2 {
		return jamerrors.ErrGInsufficientGuarantees
	}

	// v0.5 eq 11.24
	err := CheckSorting_EG(guarantee)
	if err != nil {
		return jamerrors.ErrGDuplicateGuarantors
	}

	// v0.5 eq 11.25 - check signature, core assign check,C_v ...
	CurrV := s.JamState.SafroleState.CurrValidators
	err = guarantee.Verify(CurrV) // errBadSignature
	if err != nil {
		return jamerrors.ErrGBadSignature
	}
	// v0.5 eq 11.25 - The signing validators must be assigned to the core in G or G*
	err = s.AreValidatorsAssignedToCore(guarantee)
	if err != nil {
		return err
	}
	j := s.JamState
	// v0.5 eq 11.28
	err = j.CheckReportPendingOnCore(guarantee)
	if err != nil {
		return err
	}
	// v0.5 eq 11.28
	if s.Block != nil {
		err = j.CheckReportTimeOut(guarantee, s.Block.TimeSlot())
		if err != nil {
			return err
		}
	}

	// v.05 eq 11.29 - check gas
	err = s.checkGas(guarantee)
	if err != nil {
		return err
	}
	// v0.5 eq 11.33 g.Report.RefineContext.LookupAnchorSlot doesn't have a value
	err = s.checkTimeSlotHeader(guarantee)
	if err != nil {
		return err
	}
	// v0.5 eq 11.41 - check code hash
	err = s.checkCodeHash(guarantee)
	if err != nil {
		return err
	}

	return nil

}

func (s *StateDB) VerifyGuarantee_RecentHistory(guarantee types.Guarantee) error {
	/*
		// v0.4.5 eq 147 recent restory
		err := s.checkRecentBlock(guarantee)
		if err != nil {
			return err
		}

		//TODO 149
		err = s.checkAncestorSetA(guarantee)
		if err != nil {
			return err
		}

		// v0.4.5 eq 152
		// beefy root have fucking problem
		err = s.checkAnyPrereq(guarantee)
		if err != nil {
			return err
		}
	*/

	/*
		// v0.4.5 eq 155
		err = s.checkRecentWorkPackage(guarantee)
		if err != nil {
			return err
		}
	*/
	return nil
}

// this function will be used when a validator receive a block
func (s *StateDB) ValidateGuarantees(guarantees []types.Guarantee) error {
	for _, guarantee := range guarantees {
		err := s.Verify_Guarantee_MakeBlock(guarantee)
		if err != nil {
			fmt.Printf("ValidateGuarantees error: %v\n", err)
		}
	}
	_, err, valid := s.Verify_Guarantees_MakeBlock(guarantees)
	if err != nil || !valid {
		fmt.Printf("ValidateGuarantees error: %v\n", err)
		return err
	}
	return nil
}

// v0.5 eq 11.23 - this function will be used by make block
func SortByCoreIndex(guarantees []types.Guarantee) {
	// sort guarantees by core index
	sort.Slice(guarantees, func(i, j int) bool {
		return guarantees[i].Report.CoreIndex < guarantees[j].Report.CoreIndex
	})

}

// v0.5 eq 11.23  - this function will be used by verify the block
func CheckCoreIndex(guarantees []types.Guarantee, new types.Guarantee) error {
	// check core index is correct
	core := make(map[uint16]bool)
	for _, guarantee := range guarantees {
		if guarantee.Report.CoreIndex > types.TotalCores {
			return jamerrors.ErrGBadCoreIndex
		}
		core[guarantee.Report.CoreIndex] = true
	}
	if core[new.Report.CoreIndex] {
		return jamerrors.ErrGBadCoreIndex // CHECK: not quite "coreindextoo big"
	}
	return nil
}

// v0.5 eq 11.23 - this function will be used by verify the block
func CheckSorting_EGs(guarantees []types.Guarantee) error {
	//check SortByCoreIndex is correct
	for i := 0; i < len(guarantees)-1; i++ {
		if guarantees[i].Report.CoreIndex >= guarantees[i+1].Report.CoreIndex {
			return jamerrors.ErrGOutOfOrderGuarantee
		}
	}
	return nil
}

// v0.5 eq 11.25 - The signing validators must be assigned to the core in G or G*
func (s *StateDB) AreValidatorsAssignedToCore(guarantee types.Guarantee) error {
	timeSlotPeriod := s.GetTimeslot() / types.ValidatorCoreRotationPeriod
	reportTime := guarantee.Slot / types.ValidatorCoreRotationPeriod
	for _, g := range guarantee.Signatures {
		find_and_correct := false
		if timeSlotPeriod != reportTime {
			for i, assignment := range s.PreviousGuarantorAssignments {
				if uint16(i) == g.ValidatorIndex && assignment.CoreIndex == guarantee.Report.CoreIndex {
					find_and_correct = true
					break
				}
			}
			if !find_and_correct {
				return jamerrors.ErrGWrongAssignment
			}
		} else {
			for i, assignment := range s.GuarantorAssignments {
				if uint16(i) == g.ValidatorIndex && assignment.CoreIndex == guarantee.Report.CoreIndex {
					find_and_correct = true
					break
				}
			}
			if !find_and_correct {
				fmt.Printf("%s\n", guarantee.String())
				fmt.Printf("core %d has\n", guarantee.Report.CoreIndex)
				for _, assignment := range s.GuarantorAssignments {
					if assignment.CoreIndex == guarantee.Report.CoreIndex {
						fmt.Printf("validator %d\n", s.GetSafrole().GetCurrValidatorIndex(assignment.Validator.Ed25519))
					}
				}
				return jamerrors.ErrGWrongAssignment
			}
		}

	}
	return nil
}

// v0.5 eq 11.25 - The signing validators must be assigned to the core in G or G*
func (s *StateDB) AreValidatorsAssignedToCore_MakeBlock(guarantee types.Guarantee) error {
	timeSlotPeriod := s.Block.TimeSlot() / types.ValidatorCoreRotationPeriod
	reportTime := guarantee.Slot / types.ValidatorCoreRotationPeriod
	for _, g := range guarantee.Signatures {
		find_and_correct := false
		if timeSlotPeriod != reportTime {
			for i, assignment := range s.PreviousGuarantorAssignments {
				if uint16(i) == g.ValidatorIndex && assignment.CoreIndex == guarantee.Report.CoreIndex {
					find_and_correct = true
					break
				}
			}
			if !find_and_correct {
				fmt.Printf("prev core assignment\n")
				for _, assignment := range s.PreviousGuarantorAssignments {
					fmt.Printf("[core%d]validator %d\n", assignment.CoreIndex, s.GetSafrole().GetCurrValidatorIndex(assignment.Validator.Ed25519))
				}

				fmt.Printf("curr core assignment\n")
				for _, assignment := range s.GuarantorAssignments {
					fmt.Printf("[core%d]validator %d\n", assignment.CoreIndex, s.GetSafrole().GetCurrValidatorIndex(assignment.Validator.Ed25519))
				}
				return jamerrors.ErrGWrongAssignment

			}
		} else {
			for i, assignment := range s.GuarantorAssignments {
				if uint16(i) == g.ValidatorIndex && assignment.CoreIndex == guarantee.Report.CoreIndex {
					find_and_correct = true
					break
				}
			}
			if !find_and_correct {
				fmt.Printf("%s\n", guarantee.String())
				fmt.Printf("core %d has\n", guarantee.Report.CoreIndex)
				for _, assignment := range s.GuarantorAssignments {
					if assignment.CoreIndex == guarantee.Report.CoreIndex {
						fmt.Printf("validator %d\n", s.GetSafrole().GetCurrValidatorIndex(assignment.Validator.Ed25519))
					}
				}
				return jamerrors.ErrGWrongAssignment
			}
		}

	}
	return nil
}

// v0.5 eq 11.24 - sort the guarantee by validator index
func Sort_by_validator_index(g types.Guarantee) {
	sort.Slice(g.Signatures, func(i, j int) bool {
		return g.Signatures[i].ValidatorIndex < g.Signatures[j].ValidatorIndex
	})
}

// v0.5 eq 11.24 check the guarantee is sorted by validator index or not
func CheckSorting_EG(g types.Guarantee) error {
	// check sort_by_validator_index is correct
	for i := 0; i < len(g.Signatures)-1; i++ {
		if g.Signatures[i].ValidatorIndex >= g.Signatures[i+1].ValidatorIndex {
			return jamerrors.ErrGDuplicateGuarantors
		}
	}
	return nil
}

// v0.5 eq 11.27
func (s *StateDB) getWorkReport() []types.WorkReport {
	w := []types.WorkReport{}

	if s.Block != nil {
		for _, guarantee := range s.Block.Extrinsic.Guarantees {
			w = append(w, guarantee.Report)
		}
	}
	return w
}

// v0.5 eq 11.28 - check pending report
func (j *JamState) CheckReportTimeOut(g types.Guarantee, ts uint32) error {
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
	if j.AvailabilityAssignments[int(g.Report.CoreIndex)] != nil {
		return jamerrors.ErrGCoreEngaged
	}
	return fmt.Errorf("CheckReportTimeOut: invalid pending report on core %v, package hash:%v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
}

// v0.5 eq 11.28
func (j *JamState) CheckReportPendingOnCore(g types.Guarantee) error {
	return nil

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
	for i, rho := range j.AvailabilityAssignments {
		if rho != nil && rho.WorkReport.CoreIndex != uint16(i) {
			problem = true
		}
	}
	// Core 0 : receiving megatron report; Core 1 : receiving fib+trib report
	if problem {
		for i, rho := range j.AvailabilityAssignments {
			fmt.Printf("[Node] CheckInvalidCoreIndex i=%d rho: (WorkReportHash:%v) CoreIndex: %d WorkReport: %v\n",
				i, rho.WorkReport.Hash(), rho.WorkReport.CoreIndex, rho.WorkReport.String())
		}
		fmt.Printf("CheckInvalidCoreIndex: FAILURE\n")
		panic(1111)
	} else if debug {
		fmt.Printf("CheckInvalidCoreIndex: success\n")
	}

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
			if results.Gas >= s.JamState.PriorServiceAccountState[uint32(results.Gas)].GasLimitG {
				return nil
			}
		}
	}
	return jamerrors.ErrGWorkReportGasTooHigh
}

// v0.5 eq 11.30
func (s *StateDB) getRefineContext() []types.RefineContext {
	x := []types.RefineContext{}
	w := s.getWorkReport()
	for _, report := range w {
		x = append(x, report.RefineContext)
	}
	return x
}

// v0.5 eq 11.30
func (s *StateDB) getAvailibleSpecHash() []common.Hash {
	p := []common.Hash{}
	w := s.getWorkReport()
	for _, report := range w {
		p = append(p, report.AvailabilitySpec.WorkPackageHash)
	}
	return p
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
	anchor := true // CHECK -- I think this should be false
	if refine.Anchor != (common.Hash{}) {
		for _, block := range s.JamState.RecentBlocks {
			if block.HeaderHash != refine.Anchor {
				anchor = false
			} else {
				anchor = true
				break
			}
		}
	}
	if !anchor {
		return jamerrors.ErrGAnchorNotRecent
	}

	stateroot := true // CHECK
	if refine.StateRoot != (common.Hash{}) {
		for _, block := range s.JamState.RecentBlocks {
			if block.StateRoot != refine.StateRoot {
				stateroot = false
			} else {
				stateroot = true
				break
			}
		}
	}
	if !stateroot {
		// CHECK
		return fmt.Errorf("invalid recent block stateroot, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}
	beefyroot := true
	if refine.BeefyRoot != (common.Hash{}) {
		for _, block := range s.JamState.RecentBlocks {
			encoded_b, err := types.Encode(block.B)
			if err != nil {
				return err
			}
			hash_encoded_b := common.Keccak256(encoded_b)
			if hash_encoded_b != refine.BeefyRoot {
				beefyroot = false
			} else {
				beefyroot = true
				break
			}
		}
	}
	if !beefyroot {
		return jamerrors.ErrGBadBeefyMMRRoot
	}

	return nil
}

// v0.5 eq 11.33
func (s *StateDB) checkTimeSlotHeader(g types.Guarantee) error {
	if s.Block == nil {
		return fmt.Errorf("invalid lookup anchor slot: block is nil")
	}
	var valid_anchor uint32
	valid_anchor = s.Block.TimeSlot() - types.LookupAnchorMaxAge
	if types.LookupAnchorMaxAge > s.Block.TimeSlot() {
		valid_anchor = 0
	}
	if g.Report.RefineContext.LookupAnchorSlot >= valid_anchor {
		return nil
	} else {
		return jamerrors.ErrGReportEpochBeforeLast
	}
}

// TODO: v0.5 eq 11.34
func (s *StateDB) checkAncestorSetA(g types.Guarantee) error {
	return nil
}

// TODO: v0.5 eq 11.35
func (s *StateDB) getPrereqFromAccumulationQueue() []common.Hash {
	result := []common.Hash{}
	for i := 0; i < types.EpochLength; i++ {
		for _, queues := range s.JamState.AccumulationQueue[i] {
			for _, workreport := range queues.WorkReports {
				if len(workreport.RefineContext.Prerequisites) != 0 {
					result = append(result, workreport.RefineContext.Prerequisites...)
				}
			}
		}
	}
	return result
}

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
func (s *StateDB) checkAnyPrereq(g types.Guarantee) error {
	prereqSetFromQueue := make(map[common.Hash]struct{})
	for _, hash := range s.getPrereqFromAccumulationQueue() {
		prereqSetFromQueue[hash] = struct{}{}
	}
	_, exists := prereqSetFromQueue[g.Report.AvailabilitySpec.WorkPackageHash]
	if exists {
		return fmt.Errorf("invalid prerequisite work package(from queue), core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}
	prereqSetFromRho := make(map[common.Hash]struct{})
	for _, hash := range s.getPrereqFromRho() {
		prereqSetFromRho[hash] = struct{}{}
	}
	_, exists = prereqSetFromRho[g.Report.AvailabilitySpec.WorkPackageHash]
	if exists {
		return fmt.Errorf("invalid prerequisite work package(from rho), core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}
	prereqSetFromAccumulationHistory := make(map[common.Hash]struct{})
	for i := 0; i < types.EpochLength; i++ {
		for _, hash := range s.JamState.AccumulationHistory[i].WorkPackageHash {
			prereqSetFromAccumulationHistory[hash] = struct{}{}
		}
	}
	_, exists = prereqSetFromAccumulationHistory[g.Report.AvailabilitySpec.WorkPackageHash]
	if exists {
		return fmt.Errorf("invalid prerequisite work package(from accumulation history), core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}

	prereqSet := make(map[common.Hash]struct{})
	for _, block := range s.JamState.RecentBlocks {
		for key := range block.Reported {
			prereqSet[key] = struct{}{}
		}
	}
	_, exists = prereqSet[g.Report.AvailabilitySpec.WorkPackageHash]
	if exists {
		return fmt.Errorf("invalid prerequisite work package(from recent block), core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}
	return nil
}

// v0.5 eq 11.38
func (s *StateDB) checkPrereq(g types.Guarantee) error {
	prereqSet := make(map[common.Hash]struct{})
	p := []common.Hash{}
	if len(g.Report.RefineContext.Prerequisites) != 0 {
		p = append(p, g.Report.RefineContext.Prerequisites...)
	}
	for key := range g.Report.SegmentRootLookup {
		p = append(p, key)
	}

	for _, block := range s.JamState.RecentBlocks {
		for key := range block.Reported {
			prereqSet[key] = struct{}{}
		}
	}

	for _, hash := range s.getAvailibleSpecHash() {
		prereqSet[hash] = struct{}{}
	}

	if len(p) == 0 {
		return nil
	}

	for _, hash := range p {
		exists := false
		for key := range prereqSet {
			if hash == key {
				exists = true
				break
			}
		}
		if !exists {
			return jamerrors.ErrGDependencyMissing
		}
	}

	return nil
}

// this function is for the making block
func (s *StateDB) checkPrereqWithoutBlock(g types.Guarantee, EGs []types.Guarantee) error {
	prereqSet := make(map[common.Hash]struct{})
	p := []common.Hash{}
	if len(g.Report.RefineContext.Prerequisites) != 0 {
		p = append(p, g.Report.RefineContext.Prerequisites...)
	}
	for key := range g.Report.SegmentRootLookup {
		p = append(p, key)
	}

	for _, block := range s.JamState.RecentBlocks {
		for key := range block.Reported {
			prereqSet[key] = struct{}{}
		}
	}

	for _, guarantee := range EGs {
		prereqSet[guarantee.Report.AvailabilitySpec.WorkPackageHash] = struct{}{}
	}

	if len(p) == 0 {
		return nil
	}

	for _, hash := range p {
		exists := false
		for key := range prereqSet {
			if hash == key {
				exists = true
				break
			}
		}
		if !exists {
			return jamerrors.ErrGDependencyMissing
		}
	}

	return nil
}

// v0.5 eq 11.39
func getPresentBlock(g types.Guarantee) types.Hash2Hash {
	p := types.Hash2Hash{}
	p[g.Report.AvailabilitySpec.WorkPackageHash] = g.Report.AvailabilitySpec.ExportedSegmentRoot
	return p
}

// v0.5 eq 11.40
func (s *StateDB) checkRecentWorkPackage(g types.Guarantee) error {
	present_block := getPresentBlock(g)
	for _, block := range s.JamState.RecentBlocks {
		for key, value := range block.Reported {
			if _, exists := present_block[key]; !exists {
				present_block[key] = value
			}
		}
	}

	// compare present block and report segment root lookup
	if g.Report.SegmentRootLookup == nil {
		return nil
	}
	for key, value := range g.Report.SegmentRootLookup {
		// if present_block[key] exists, compare the value
		if _, exists := present_block[key]; exists {
			if present_block[key] == value {
				return nil // CHECK THIS and ErrGSegmentRootLookupInvalidUnexpectedValue and errGDuplicatePackageRecentHistory
			} else {
				return jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks
			}
		} else {
			return jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks
		}
	}
	return jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks
}

// v0.5 eq 11.41
func (s *StateDB) checkCodeHash(g types.Guarantee) error {
	var err error
	for _, result := range g.Report.Results {
		if result.CodeHash != s.JamState.PriorServiceAccountState[result.ServiceID].CodeHash {
			// fmt.Printf("checkCodeHash: %v\n", result.CodeHash)
			// fmt.Printf("checkCodeHash from service: %v\n", s.JamState.PriorServiceAccountState[result.ServiceID].CodeHash)
			// IMPORTANT: we didn't pass the service into JamState, so we can't get the codehash from the service
			// we have to check the codehash from the trie
			// but we can save this function just for pass the test vectors
			err = jamerrors.ErrGBadCodeHash
		}
	}
	// get service from trie
	if err != nil {
		t := s.CopyTrieState(s.StateRoot)
		if t == nil {
			return jamerrors.ErrGBadCodeHash
		}
		for _, result := range g.Report.Results {
			v, ok, err := t.GetService(255, result.ServiceID)
			if err != nil || !ok { // CHECK
				fmt.Printf("checkCodeHash: %v\n", err)
				return jamerrors.ErrGBadServiceID
			}
			a, _ := types.ServiceAccountFromBytes(result.ServiceID, v)
			if result.CodeHash != a.CodeHash {
				fmt.Printf("result.CodeHash: %v, service.codehash: %v\n", result.CodeHash, a.CodeHash)
				return jamerrors.ErrGBadCodeHash
			}
		}

	}
	return nil
}
