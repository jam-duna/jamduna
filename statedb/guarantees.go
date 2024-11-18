package statedb

import (
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// TODO: ensure that 100% of these are used
const (
	errAnchorNotRecent          = "anchor_not_recent"
	errBadBeefyMMRRoot          = "bad_beefy_mmr_root"
	errBadCodeHash              = "bad_code_hash"
	errBadCoreIndex             = "bad_core_index"
	errBadServiceID             = "bad_service_id"
	errBadSignature             = "bad_signature"
	errBadStateRoot             = "bad_state_root"
	errServiceItemGasTooLow     = "service_item_gas_too_low"
	errCoreEngaged              = "core_engaged"
	errDependencyMissing        = "dependency_missing"
	errDuplicatePackage         = "duplicate_package"
	errFutureReportSlot         = "future_report_slot"
	errInsufficientGuarantees   = "insufficient_guarantees"
	errCoreUnauthorized         = "core_unauthorized"
	errDuplicateGuarantors      = "duplicate_guarantors"
	errOutOfOrderGuarantee      = "out_of_order_guarantee"
	errReportEpochBeforeLast    = "report_epoch_before_last"
	errSegmentRootLookupInvalid = "segment_root_lookup_invalid"
	errWorkReportGasTooHigh     = "work_report_gas_too_high"
	errTooManyDependencies      = "too_many_dependencies"
	errWrongAssignment          = "wrong_assignment"
)

func (j *JamState) ProcessGuarantees(guarantees []types.Guarantee) {
	for _, guarantee := range guarantees {
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

// this function will be used by what should be included in the block
func (s *StateDB) Verify_Guarantee(guarantee types.Guarantee) error {
	if len(guarantee.Signatures) < 2 {
		return fmt.Errorf("not enough signatures, this work report is made by core: %v", guarantee.Report.CoreIndex)
	}

	// v0.4.5 eq 139 - check index
	err := CheckSorting_EG(guarantee)
	if err != nil {
		return err
	}

	// v0.4.5 eq 140 - check signature, core assign check,C_v ...
	CurrV := s.JamState.SafroleState.CurrValidators
	err = guarantee.Verify(CurrV) // errBadSignature
	if err != nil {
		return err
	}
	// v0.4.5 eq 140 - The signing validators must be assigned to the core in G or G*
	err = s.AreValidatorsAssignedToCore(guarantee)
	if err != nil {
		fmt.Printf("Verify_Guarantee error: %v\n", err)
		return err
	}
	//TODO: v0.4.5 eq 140 - C_v
	// v0.4.5 eq 143
	j := s.JamState

	if s.Block != nil {
		err = j.CheckReportTimeOut(guarantee, s.Block.TimeSlot())
		if err != nil {
			return err
		}
	}

	err = j.CheckReportPendingOnCore(guarantee)
	if err != nil {
		return err
	}

	//v0.4.5 eq 144 - check gas
	err = s.checkGas(guarantee)
	if err != nil {
		return err
	}

	//v0.4.5 eq 146
	err = s.checkLength()
	if err != nil {
		return err
	}

	// v0.4.5 eq 147 recent restory
	// err = s.checkRecentBlock(guarantee)
	// if err != nil {
	// 	return err
	// }

	// v0.4.5 eq 148
	// g.Report.RefineContext.LookupAnchorSlot doesn't have a value
	// err = s.checkTimeSlotHeader(guarantee)
	// if err != nil {
	// 	return err
	// }

	// //TODO 149
	// err = s.checkAncestorSetA(guarantee)
	// if err != nil {
	// 	return err
	// }

	// v0.4.5 eq 152
	// beefy root have fucking problem
	// err = s.checkAnyPrereq(guarantee)
	// if err != nil {
	// 	return err
	// }

	// v0.4.5 eq 153
	// err = s.checkPrereq(guarantee)
	// if err != nil {
	// 	return err
	// }

	// v0.4.5 eq 155
	err = s.checkRecentWorkPackage(guarantee)
	if err != nil {
		return err
	}

	// v0.4.5 eq 156 - check code hash
	// s.JamState.PriorServiceAccountState[result.Service].CodeHash doesn't have a value
	// err = s.checkCodeHash(guarantee)
	// if err != nil {
	// 	return err
	// }

	return nil
}

// this function will be used when a validator receive a block
func (s *StateDB) ValidateGuarantees(guarantees []types.Guarantee) error {
	// v0.4.5 eq138 - check index
	err := CheckSorting_EGs(guarantees)
	if err != nil {
		return err
	}
	// v0.4.5 eq139~ check guarantee
	for _, guarantee := range guarantees {
		err := s.Verify_Guarantee(guarantee)
		if err != nil {
			fmt.Println(err)
			guarantee = types.Guarantee{}
		}
	}
	return nil
}

// v0.4.5 eq 137 - this function will be used by make block
func SortByCoreIndex(guarantees []types.Guarantee) {
	// sort guarantees by core index
	sort.Slice(guarantees, func(i, j int) bool {
		return guarantees[i].Report.CoreIndex < guarantees[j].Report.CoreIndex
	})

}

// errBadCoreIndex
func CheckCoreIndex(guarantees []types.Guarantee, new types.Guarantee) error {
	// check core index is correct
	core := make(map[uint16]bool)
	for _, guarantee := range guarantees {
		core[guarantee.Report.CoreIndex] = true
	}
	if core[new.Report.CoreIndex] {
		return fmt.Errorf("core index %v is duplicated", new.Report.CoreIndex)
	}
	return nil
}

// v0.4.5 eq 138 - this function will be used by verify the block
func CheckSorting_EGs(guarantees []types.Guarantee) error {
	//check SortByCoreIndex is correct
	for i := 0; i < len(guarantees)-1; i++ {
		if guarantees[i].Report.CoreIndex >= guarantees[i+1].Report.CoreIndex {
			return fmt.Errorf("guarantees are not sorted: %v and %v", guarantees[i].Report.CoreIndex, guarantees[i+1].Report.CoreIndex)
		}
	}
	return nil
}

// v0.4.5 eq 140 - The signing validators must be assigned to the core in G or G*
func (s *StateDB) AreValidatorsAssignedToCore(guarantee types.Guarantee) error {
	timeSlotPeriod := s.JamState.SafroleState.GetTimeSlot() / types.ValidatorCoreRotationPeriod
	reportTime := guarantee.Slot / types.ValidatorCoreRotationPeriod
	for _, g := range guarantee.Signatures {

		if timeSlotPeriod != reportTime {
			for i, assignment := range s.PreviousGuarantorAssignments {
				if uint16(i) == g.ValidatorIndex && assignment.CoreIndex == guarantee.Report.CoreIndex {
					return nil
				}
			}
			return fmt.Errorf("validator %v is not assigned to core %v (Previous)", g.ValidatorIndex, guarantee.Report.CoreIndex)
		} else {
			for i, assignment := range s.GuarantorAssignments {
				if uint16(i) == g.ValidatorIndex && assignment.CoreIndex == guarantee.Report.CoreIndex {

					return nil
				}
			}
			return fmt.Errorf("validator %v is not assigned to core %v (Now)", g.ValidatorIndex, guarantee.Report.CoreIndex)
		}
	}
	return nil
}

// v0.4.5 eq 139 - sort the guarantee by validator index
func Sort_by_validator_index(g types.Guarantee) {
	sort.Slice(g.Signatures, func(i, j int) bool {
		return g.Signatures[i].ValidatorIndex < g.Signatures[j].ValidatorIndex
	})
}

// v0.4.5 eq 139
// check the guarantee is sorted by validator index or not
func CheckSorting_EG(g types.Guarantee) error {
	// check sort_by_validator_index is correct
	for i := 0; i < len(g.Signatures)-1; i++ {
		if g.Signatures[i].ValidatorIndex >= g.Signatures[i+1].ValidatorIndex {
			return fmt.Errorf("guarantees are not sorted: V%d and V%d", g.Signatures[i].ValidatorIndex, g.Signatures[i+1].ValidatorIndex)
		}
	}
	return nil
}

// TODO:double check this again
// check all the guarantees is 1v1 Mapping

func (s *StateDB) CheckGuaranteesWorkReport(guarantees []types.Guarantee) error {
	for _, guarantee := range guarantees {
		if guarantee.Report.AvailabilitySpec.WorkPackageHash != (common.Hash{}) {
			if guarantee.Report.AvailabilitySpec.WorkPackageHash == (common.Hash{}) {
				return fmt.Errorf("invalid work report form core %v, package %v", guarantee.Report.CoreIndex, guarantee.Report.GetWorkPackageHash())
			}
		} else {
			if guarantee.Report.AvailabilitySpec.WorkPackageHash != (common.Hash{}) {
				return fmt.Errorf("invalid work report form core %v, package %v", guarantee.Report.CoreIndex, guarantee.Report.GetWorkPackageHash())
			}
		}
	}

	return nil
}

// v0.4.5 eq 142 - w
func (s *StateDB) getWorkReport() []types.WorkReport {
	w := []types.WorkReport{}
	if s.Block != nil {
		for _, guarantee := range s.Block.Extrinsic.Guarantees {
			w = append(w, guarantee.Report)
		}
	}
	return w
}

// v0.4.5 eq 143 - check pending report
func (j *JamState) CheckReportTimeOut(g types.Guarantee, ts uint32) error {
	if j.AvailabilityAssignments[int(g.Report.CoreIndex)] == nil {
		return nil
	}

	timeoutbool := ts >= (j.AvailabilityAssignments[int(g.Report.CoreIndex)].Timeslot)+uint32(types.UnavailableWorkReplacementPeriod)
	if timeoutbool {
		return nil
	}
	if j.AvailabilityAssignments[int(g.Report.CoreIndex)] != nil {
		fmt.Printf("Real Core Index:%d\n", j.AvailabilityAssignments[int(g.Report.CoreIndex)].WorkReport.CoreIndex)
		for i, rho := range j.AvailabilityAssignments {

			fmt.Printf("rho[%d],report:\n%s\n", i, rho.WorkReport.String())
		}
		return fmt.Errorf("CheckReportTimeOut:there is a pending report on core %v", g.Report.CoreIndex)

	}
	return fmt.Errorf("CheckReportTimeOut: invalid pending report on core %v, package hash:%v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
}

// v0.4.5 eq 143 - Wa ∈ α[wc]
func (j *JamState) CheckReportPendingOnCore(g types.Guarantee) error {
	authorizations_pool := j.AuthorizationsPool[int(g.Report.CoreIndex)]
	for _, authrization := range authorizations_pool {
		if authrization != g.Report.AuthorizerHash {
			return fmt.Errorf("CheckReportPendingOnCore: invalid pending report on core %v, package hash:%v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
		}
	}
	return nil
}

func (j *JamState) CheckInvalidCoreIndex() {
	for i, rho := range j.AvailabilityAssignments {
		if rho != nil && rho.WorkReport.CoreIndex != uint16(i) {
			panic(fmt.Sprintf("invalid core index %v,with report index %d", i, rho.WorkReport.CoreIndex))
		}
	}
	fmt.Printf("CheckInvalidCoreIndex: success\n")
}

// v0.4.5 eq 144
func (s *StateDB) checkGas(g types.Guarantee) error {
	sum_rg := uint64(0)
	for _, results := range g.Report.Results {
		sum_rg += results.Gas
	}
	if sum_rg <= types.AccumulationGasAllocation {
		for _, results := range g.Report.Results {
			if results.Gas >= s.JamState.PriorServiceAccountState[uint32(results.Gas)].GasLimitG {
				return nil
			}
		}
	}
	return fmt.Errorf("invalid gas allocation, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
}

// v0.4.5 eq 145 - x
func (s *StateDB) getRefineContext() []types.RefineContext {
	x := []types.RefineContext{}
	w := s.getWorkReport()
	for _, report := range w {
		x = append(x, report.RefineContext)
	}
	return x
}

// v0.4.5 eq 145 - p
func (s *StateDB) getAvailibleSpecHash() []common.Hash {
	p := []common.Hash{}
	w := s.getWorkReport()
	for _, report := range w {
		p = append(p, report.AvailabilitySpec.WorkPackageHash)
	}
	return p
}

// v0.4.5 eq 146
func (s *StateDB) checkLength() error {
	p := s.getAvailibleSpecHash()
	w := s.getWorkReport()
	if len(p) == len(w) {
		return nil
	}
	return fmt.Errorf("invalid work report length")
}

// v0.4.5 eq 147
func (s *StateDB) checkRecentBlock(g types.Guarantee) error {
	refine := g.Report.RefineContext
	anchor := true
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
		return fmt.Errorf("invalid recent block anchor, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}

	stateroot := true
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
		return fmt.Errorf("invalid recent block beefyroot, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}

	return nil
}

// v0.4.5 eq 148
func (s *StateDB) checkTimeSlotHeader(g types.Guarantee) error {
	if s.Block == nil {
		return nil
	}

	if g.Report.RefineContext.LookupAnchorSlot >= s.Block.TimeSlot()-types.LookupAnchorMaxAge {
		return nil
	} else {
		return fmt.Errorf("invalid lookup anchor slot, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}
}

// TODO: v0.4.5 eq 149
func (s *StateDB) checkAncestorSetA(g types.Guarantee) error {
	return nil
}

// v0.4.5 eq 150
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

// v0.4.5 eq 151
func (s *StateDB) getPrereqFromRho() []common.Hash {
	result := []common.Hash{}
	for _, rho := range s.JamState.AvailabilityAssignments {
		if rho != nil && len(rho.WorkReport.RefineContext.Prerequisites) != 0 {
			result = append(result, rho.WorkReport.RefineContext.Prerequisites...)
		}
	}
	return result
}

// v0.4.5 eq 152
func (s *StateDB) checkAnyPrereq(g types.Guarantee) error {
	prereqSet := make(map[common.Hash]struct{})
	for _, hash := range s.getPrereqFromAccumulationQueue() {
		prereqSet[hash] = struct{}{}
	}
	for _, hash := range s.getPrereqFromRho() {
		prereqSet[hash] = struct{}{}
	}
	for i := 0; i < types.EpochLength; i++ {
		for _, hash := range s.JamState.AccumulationHistory[i].WorkPackageHash {
			prereqSet[hash] = struct{}{}
		}
	}
	for _, block := range s.JamState.RecentBlocks {
		for key := range block.Reported {
			prereqSet[key] = struct{}{}
		}
	}
	_, exists := prereqSet[g.Report.AvailabilitySpec.WorkPackageHash]
	if !exists {
		return nil
	} else {
		return fmt.Errorf("invalid prerequisite work package, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
	}
}

// v0.4.5 eq 153
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
			return fmt.Errorf("invalid prerequisite work package, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
		}
	}

	return fmt.Errorf("invalid prerequisite work package, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
}

// v0.4.5 eq 154
func getPresentBlock(g types.Guarantee) map[common.Hash]common.Hash {
	p := map[common.Hash]common.Hash{}
	p[g.Report.AvailabilitySpec.WorkPackageHash] = g.Report.AvailabilitySpec.ExportedSegmentRoot
	return p
}

// v0.4.5 eq 155
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
				return nil
			} else {
				return fmt.Errorf("invalid recent work package, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
			}
		} else {
			return fmt.Errorf("invalid recent work package, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
		}

	}
	return fmt.Errorf("invalid recent work package, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())

}

// v0.4.5 eq 156
func (s *StateDB) checkCodeHash(g types.Guarantee) error {
	for _, result := range g.Report.Results {
		if result.CodeHash != s.JamState.PriorServiceAccountState[result.ServiceID].CodeHash {
			return fmt.Errorf("invalid code hash, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
		}
	}
	return nil
}
