package statedb

import (
	"errors"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// this function will be used by what should be included in the block
func (s *StateDB) Verify_Guarantee(guarantee types.Guarantee) error {
	if len(guarantee.Signatures) < 2 {
		return errors.New(fmt.Sprintf("not enough signatures , this work report is made by core : %v", guarantee.Report.CoreIndex))
	}
	//138 check index
	err := CheckSorting_EG(guarantee)
	if err != nil {
		return err
	}
	//139 check signature, core assign check,C_v ...
	CurrV := s.JamState.SafroleState.CurrValidators
	err = guarantee.Verify(CurrV)
	if err != nil {
		return err
	}
	//139 The signing validators must be assigned to the core in G or G*
	err = s.AreValidatorsAssignedToCore(guarantee)
	if err != nil {
		fmt.Printf("Verify_Guarantee error: %v\n", err)
		return err
	}
	//TODO: 139 C_v

	//143 check gas
	err = s.CheckGas(guarantee)
	if err != nil {
		return err
	}
	//145 check package and report are same hex
	//will do after check the single guarantee
	// 146 resent restory
	// err = s.checkRecentBlock(guarantee)
	// if err != nil {
	// 	return err
	// }
	//147
	err = s.CheckTimeSlotHeader(guarantee)
	if err != nil {
		return err
	}
	// //TODO 148- check ancestor set A
	// err = s.checkAncestorSetA(guarantee)
	// if err != nil {
	// 	return err
	// }
	// //149- check report is not in recent history
	// err = s.checkReportNotInRecentHistory(guarantee)
	// if err != nil {
	// 	return err
	// }
	// // TODO 150 check prerequisite work-package
	// err = s.checkPrerequisiteWorkPackage(guarantee)
	// if err != nil {
	// 	return err
	// }
	// 151 check code hash
	// err = s.checkCodeHash(guarantee)
	// if err != nil {
	// 	return err
	// }
	return nil
}

// this function will be used when a validator receive a block
func (s *StateDB) ValidateGuarantees(guarantees []types.Guarantee) error {
	// 137 check index
	err := CheckSorting_EGs(guarantees)
	if err != nil {
		return err
	}
	// 138~ check guarantee
	for _, guarantee := range guarantees {
		err := s.Verify_Guarantee(guarantee)
		if err != nil {
			fmt.Println(err)
			guarantee = types.Guarantee{}
		}
	}
	return nil
}

// func 137 this function will be used by make block
func SortByCoreIndex(guarantees []types.Guarantee) {
	// sort guarantees by core index
	sort.Slice(guarantees, func(i, j int) bool {
		return guarantees[i].Report.CoreIndex < guarantees[j].Report.CoreIndex
	})

}

func CheckCoreIndex(guarantees []types.Guarantee, new types.Guarantee) error {
	// check core index is correct
	core := make(map[uint16]bool)
	for _, guarantee := range guarantees {
		core[guarantee.Report.CoreIndex] = true
	}
	if core[new.Report.CoreIndex] {
		return errors.New(fmt.Sprintf("core index %v is duplicated", new.Report.CoreIndex))
	}
	return nil
}

// this function will be used by verify the block
func CheckSorting_EGs(guarantees []types.Guarantee) error {
	//check SortByCoreIndex is correct
	for i := 0; i < len(guarantees)-1; i++ {
		if guarantees[i].Report.CoreIndex >= guarantees[i+1].Report.CoreIndex {
			return errors.New(fmt.Sprintf("guarantees are not sorted: %v and %v", guarantees[i].Report.CoreIndex, guarantees[i+1].Report.CoreIndex))
		}
	}
	return nil
}

// 139 - The signing validators must be assigned to the core in G or G*
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
			return errors.New(fmt.Sprintf("validator %v is not assigned to core %v (Previous)", g.ValidatorIndex, guarantee.Report.CoreIndex))
		} else {
			for i, assignment := range s.GuarantorAssignments {
				if uint16(i) == g.ValidatorIndex && assignment.CoreIndex == guarantee.Report.CoreIndex {

					return nil
				}
			}
			return errors.New(fmt.Sprintf("validator %v is not assigned to core %v (Now)", g.ValidatorIndex, guarantee.Report.CoreIndex))
		}
	}
	return nil
}

// sort the guarantee by validator index
func Sort_by_validator_index(g types.Guarantee) {
	sort.Slice(g.Signatures, func(i, j int) bool {
		return g.Signatures[i].ValidatorIndex < g.Signatures[j].ValidatorIndex
	})
}

// check the guarantee is sorted by validator index or not
func CheckSorting_EG(g types.Guarantee) error {
	// check sort_by_validator_index is correct
	for i := 0; i < len(g.Signatures)-1; i++ {
		if g.Signatures[i].ValidatorIndex >= g.Signatures[i+1].ValidatorIndex {
			return errors.New(fmt.Sprintf("guarantees are not sorted: V%d and V%d", g.Signatures[i].ValidatorIndex, g.Signatures[i+1].ValidatorIndex))
		}
	}
	return nil
}

// 142 check pending report
func (j *JamState) CheckReportTimeOut(g types.Guarantee, ts uint32) error {
	if j.AvailabilityAssignments[g.Report.CoreIndex] == nil {
		return nil
	}
	timeoutbool := ts >= j.AvailabilityAssignments[g.Report.CoreIndex].Timeslot+types.UnavailableWorkReplacementPeriod
	if timeoutbool {
		return nil
	}
	return errors.New(fmt.Sprintf("invalid pending report on core %v, package hash:%v", g.Report.CoreIndex, g.Report.GetWorkPackageHash()))
}

func (j JamState) CheckReportPendingOnCore(g types.Guarantee) error {
	if j.AvailabilityAssignments[g.Report.CoreIndex] == nil {
		return nil
	} else {
		return errors.New(fmt.Sprintf("invalid pending report on core %v, package hash:%v", g.Report.CoreIndex, g.Report.GetWorkPackageHash()))
	}
}

// TODO 142 Wa ∈ α[wc]
// 143
func (s *StateDB) CheckGas(g types.Guarantee) error {
	count := 0
	for _, results := range g.Report.Results {
		count += int(s.JamState.PriorServiceAccountState[uint32(results.Service)].GasLimitG)
	}
	//TODO: G_A has not been defined
	if count > types.AccumulationGasAllocation {
		return errors.New(fmt.Sprintf("invalid gas limit: %v, from core %v, package %v", count, g.Report.CoreIndex, g.Report.GetWorkPackageHash()))
	}
	return nil
}

// TODO:double check this again
// 145 check all the guarantees is 1v1 Mapping
func (s *StateDB) CheckGuaranteesWorkReport(guarantees []types.Guarantee) error {
	for _, guarantee := range guarantees {
		if &guarantee != nil {
			if &guarantee.Report.AvailabilitySpec.WorkPackageHash == nil {
				return errors.New(fmt.Sprintf("invalid work report form core %v, package %v", guarantee.Report.CoreIndex, guarantee.Report.GetWorkPackageHash()))
			}
		} else {
			if &guarantee.Report.AvailabilitySpec.WorkPackageHash != nil {
				return errors.New(fmt.Sprintf("invalid work report form core %v, package %v", guarantee.Report.CoreIndex, guarantee.Report.GetWorkPackageHash()))
			}
		}
	}

	return nil
}

// 146
func (s *StateDB) CheckRecentBlock(g types.Guarantee) error {
	anchor := (s.JamState.BeefyPool[types.RecentHistorySize-1].HeaderHash == g.Report.RefineContext.Anchor)
	stateroot := (s.JamState.BeefyPool[types.RecentHistorySize-1].StateRoot == g.Report.RefineContext.StateRoot)
	beefy := common.Keccak256(s.JamState.BeefyPool[types.RecentHistorySize-1].MMR_Bytes()) == g.Report.RefineContext.BeefyRoot
	if anchor && stateroot && beefy {
		return nil
	}
	return errors.New(fmt.Sprintf("Invalid recent block, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash()))
}

// TODO: 147
func (s *StateDB) CheckTimeSlotHeader(g types.Guarantee) error {
	return nil
	if g.Report.RefineContext.LookupAnchorSlot >= s.Block.TimeSlot()-types.LookupAnchorMaxAge {
		return nil
	}
	return errors.New(fmt.Sprintf("invalid lookup anchor slot, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash()))
}

// TODO 148
func (s *StateDB) CheckAncestorSetA(g types.Guarantee) error {
	return nil
}

func (s *StateDB) CheckReportNotInRecentHistory(g types.Guarantee) error {
	for _, beta := range s.JamState.BeefyPool {
		for _, report := range beta.Reported {
			if report == g.Report.GetWorkPackageHash() {

				return errors.New(fmt.Sprintf("invalid recent history, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash()))
			}
		}
	}
	return nil
}

// TODO 150 check prerequisite work-package: most recent history haven't been implemented
func (s *StateDB) CheckPrerequisiteWorkPackage(g types.Guarantee) error {
	if g.Report.RefineContext.Prerequisite != nil {
		exBool := g.Report.RefineContext.Prerequisite.Hash() == g.Report.GetWorkPackageHash()
		betaBool := false
		if exBool || betaBool {
			return nil
		}
		return errors.New(fmt.Sprintf("invalid prerequisite work package, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash()))
	}
	return nil
}

func (s *StateDB) CheckCodeHash(g types.Guarantee) error {
	for _, results := range g.Report.Results {
		if results.CodeHash != s.JamState.PriorServiceAccountState[uint32(results.Service)].CodeHash {
			return errors.New(fmt.Sprintf("invalid code hash, core %v, package %v", g.Report.CoreIndex, g.Report.GetWorkPackageHash()))
		}
	}
	return nil
}

// update the rho state here
func (j *JamState) ProcessGuarantees(guarantees []types.Guarantee) {
	for _, guarantee := range guarantees {
		if j.AvailabilityAssignments[guarantee.Report.CoreIndex] == nil {
			j.SetRhoByWorkReport(guarantee.Report.CoreIndex, guarantee.Report, j.SafroleState.GetTimeSlot())
			if debug {
				fmt.Printf("ProcessGuarantees Success on Core %v\n", guarantee.Report.CoreIndex)
			}
		}
	}

}

// setRhoByWorkReport sets the Rho state for a specific core with a WorkReport and timeslot
func (state *JamState) SetRhoByWorkReport(core uint16, w types.WorkReport, t uint32) {
	state.AvailabilityAssignments[core] = &Rho_state{
		WorkReport: w,
		Timeslot:   t,
	}
}
