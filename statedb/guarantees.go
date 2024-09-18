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
	if len(guarantee.Signatures) < 3 {
		return errors.New(fmt.Sprintf("not enough signatures , this work report is made by core : %v", guarantee.Report.CoreIndex))
	}
	//138 check index
	CheckSorting_EG(guarantee)
	//139 check signature, core assign check,C_v ...
	err := s.JamState.VerifySignature_EG(guarantee)
	if err != nil {
		return err
	}
	//139 The signing validators must be assigned to the core in G or G*
	err = s.areValidatorsAssignedToCore(guarantee)
	if err != nil {
		return err
	}
	//TODO: 139 C_v
	//142 check pending report
	err = s.checkReportPendingOnCore(guarantee)
	if err != nil {
		return err
	}
	//143 check gas
	err = s.checkGas(guarantee)
	if err != nil {
		return err
	}
	//145 check package and report are same hex
	//will do after check the single guarantee
	// 146 resent restory
	err = s.checkRecentBlock(guarantee)
	if err != nil {
		return err
	}
	//147
	err = s.checkTimeSlotHeader(guarantee)
	if err != nil {
		return err
	}
	//TODO 148- check ancestor set A
	err = s.checkAncestorSetA(guarantee)
	if err != nil {
		return err
	}
	//149- check report is not in recent history
	err = s.checkReportNotInRecentHistory(guarantee)
	if err != nil {
		return err
	}
	// TODO 150 check prerequisite work-package
	err = s.checkPrerequisiteWorkPackage(guarantee)
	if err != nil {
		return err
	}
	// 151 check code hash
	err = s.checkCodeHash(guarantee)
	if err != nil {
		return err
	}
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

// this function will be used by verify the block
func CheckSorting_EGs(guarantees []types.Guarantee) error {
	//check SortByCoreIndex is correct
	for i := 0; i < len(guarantees)-1; i++ {
		if guarantees[i].Report.CoreIndex >= guarantees[i+1].Report.CoreIndex {
			return errors.New(fmt.Sprintf("guarantees are not sorted: %v and %v", guarantees[i], guarantees[i+1]))
		}
	}
	return nil
}

// 139 - The signing validators must be assigned to the core in G or G*
func (s *StateDB) areValidatorsAssignedToCore(guarantee types.Guarantee) error {
	// TODO: logic to verify if validators are assigned to the core.
	return nil
}
func sort_by_validator_index(g types.Guarantee) {
	sort.Slice(g.Signatures, func(i, j int) bool {
		return g.Signatures[i].ValidatorIndex < g.Signatures[j].ValidatorIndex
	})
}

func CheckSorting_EG(g types.Guarantee) {
	// check sort_by_validator_index is correct
	for i := 0; i < len(g.Signatures)-1; i++ {
		if g.Signatures[i].ValidatorIndex >= g.Signatures[i+1].ValidatorIndex {
			fmt.Printf("guarantees are not sorted: %v and %v\n", g.Signatures[i].ValidatorIndex, g.Signatures[i+1].ValidatorIndex)
		}
	}
}

func (j *JamState) VerifySignature_EG(g types.Guarantee) error {
	hash := common.ComputeHash(g.Report.Bytes())
	signtext := append([]byte(types.X_G), hash...)

	for _, i := range g.Signatures {
		//verify the signature
		if !types.Ed25519Verify(types.Ed25519Key(j.SafroleState.CurrValidators[i.ValidatorIndex].Ed25519), signtext, i.Signature) {
			return errors.New(fmt.Sprintf("invalid signature in guarantee by validator %v", i.ValidatorIndex))
		}

	}
	return nil
}

// 142 check pending report
func (s *StateDB) checkReportPendingOnCore(g types.Guarantee) error {
	pendbool := s.JamState.AvailabilityAssignments[g.Report.CoreIndex] == nil
	timeoutbool := s.Block.Header.Slot >= s.JamState.AvailabilityAssignments[g.Report.CoreIndex].Timeslot+types.UnavailableWorkReplacementPeriod
	if pendbool || timeoutbool {
		return nil
	}
	return errors.New("invalid pending report")
}

// TODO 142 Wa ∈ α[wc]
// 143
func (s *StateDB) checkGas(g types.Guarantee) error {
	count := 0
	for _, results := range g.Report.Results {
		count += int(s.JamState.PriorServiceAccountState[uint32(results.Service)].GasLimitG)
	}
	//TODO: G_A has not been defined
	if count > types.AccumulationGasAllocation {
		return errors.New("invalid gas limit")
	}
	return nil
}

// TODO:double check this again
// 145 check all the guarantees is 1v1 Mapping
func (s *StateDB) CheckGuaranteesWorkReport(guarantees []types.Guarantee) error {
	for _, guarantee := range guarantees {
		if &guarantee != nil {
			if &guarantee.Report.AvailabilitySpec.WorkPackageHash == nil {
				return errors.New("invalid work report")
			}
		} else {
			if &guarantee.Report.AvailabilitySpec.WorkPackageHash != nil {
				return errors.New("invalid work report")
			}
		}
	}

	return nil
}

// 146
func (s *StateDB) checkRecentBlock(g types.Guarantee) error {
	anchor := (s.JamState.BeefyPool[types.RecentHistorySize-1].HeaderHash == g.Report.RefineContext.Anchor)
	stateroot := (s.JamState.BeefyPool[types.RecentHistorySize-1].StateRoot == g.Report.RefineContext.StateRoot)
	beefy := common.Keccak256(s.JamState.BeefyPool[types.RecentHistorySize-1].MMR_Bytes()) == g.Report.RefineContext.BeefyRoot
	if anchor && stateroot && beefy {
		return nil
	}
	return errors.New("invalid recent block")
}

// 147
func (s *StateDB) checkTimeSlotHeader(g types.Guarantee) error {
	if g.Report.RefineContext.LookupAnchorSlot >= s.Block.TimeSlot()-types.LookupAnchorMaxAge {
		return nil
	}
	return errors.New("invalid lookup anchor slot")
}

// TODO 148
func (s *StateDB) checkAncestorSetA(g types.Guarantee) error {
	return nil
}

func (s *StateDB) checkReportNotInRecentHistory(g types.Guarantee) error {
	for _, beta := range s.JamState.BeefyPool {
		for _, report := range beta.Reported {
			if report == g.Report.GetWorkPackageHash() {

				return errors.New("invalid report in recent history")
			}
		}
	}
	return nil
}

// TODO 150 check prerequisite work-package: most recent history haven't been implemented
func (s *StateDB) checkPrerequisiteWorkPackage(g types.Guarantee) error {
	if g.Report.RefineContext.Prerequisite != nil {
		exBool := g.Report.RefineContext.Prerequisite.Hash() == g.Report.GetWorkPackageHash()
		betaBool := false
		if exBool || betaBool {
			return nil
		}
		return errors.New("invalid prerequisite work package")
	}
	return nil
}

func (s *StateDB) checkCodeHash(g types.Guarantee) error {
	for _, results := range g.Report.Results {
		if results.CodeHash != s.JamState.PriorServiceAccountState[uint32(results.Service)].CodeHash {
			return errors.New("invalid code hash")
		}
	}
	return nil
}

// update the rho state here
func (j *JamState) ProcessGuarantees(guarantees []types.Guarantee) {
	for _, guarantee := range guarantees {
		if j.AvailabilityAssignments[guarantee.Report.CoreIndex] == nil {
			j.setRhoByWorkReport(guarantee.Report.CoreIndex, guarantee.Report, j.SafroleState.GetTimeSlot())
		}
	}
}

// setRhoByWorkReport sets the Rho state for a specific core with a WorkReport and timeslot
func (state *JamState) setRhoByWorkReport(core uint16, w types.WorkReport, t uint32) {
	state.AvailabilityAssignments[core] = &Rho_state{
		WorkReport: w,
		Timeslot:   t,
	}
}
