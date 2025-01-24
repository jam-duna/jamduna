package statedb

import (
	"errors"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
)

func (s *StateDB) VerifyAssuranceWithoutSig(a types.Assurance) error {
	// 0.5.0 11.9 Verify the anchor
	if a.Anchor != s.ParentHeaderHash {
		if debugA {
			fmt.Printf("[N%d] VerifyAssurance Fail  a.Anchor %v != s.ParentHeaderHash %v (ValidatorIndex %d)\n", s.Id, a.Anchor, s.ParentHeaderHash, a.ValidatorIndex)
		}
		return jamerrors.ErrABadParentHash
	}

	// 0.5.0 11.8
	if a.ValidatorIndex >= types.TotalValidators {
		return jamerrors.ErrABadValidatorIndex
	}
	var HasRhoState []bool
	for _, rho := range s.JamState.AvailabilityAssignments {
		if rho != nil {
			HasRhoState = append(HasRhoState, true)
		} else {
			HasRhoState = append(HasRhoState, false)
		}
	}
	//0.5.0 11.8
	if !a.ValidBitfield(HasRhoState) {
		return jamerrors.ErrABadCore
	}

	// 0.5.0 11.14
	var IsRhoTimeOut []bool
	for _, rho := range s.JamState.AvailabilityAssignments {
		if rho != nil {
			ts := s.JamState.SafroleState.Timeslot
			timeoutbool := ts >= (rho.Timeslot)+uint32(types.UnavailableWorkReplacementPeriod)
			if timeoutbool {
				IsRhoTimeOut = append(IsRhoTimeOut, true)
			} else {
				IsRhoTimeOut = append(IsRhoTimeOut, false)
			}
		} else {
			IsRhoTimeOut = append(IsRhoTimeOut, false)
		}
	}
	if !a.CheckTimeout(IsRhoTimeOut) {
		return jamerrors.ErrAStaleReport
	}

	/*
		// 0.5.0 11.11 Verify the signature
		err := a.VerifySignature(s.GetSafrole().CurrValidators[a.ValidatorIndex])
		if err != nil {
			return jamerrors.ErrABadSignature //TODO.. this is too strong of a requirement for fuzzing
		}
	*/
	return nil
}

func CheckDuplicate(assurances []types.Assurance, new types.Assurance) error {
	// Check for duplicate assurances
	seen := make(map[uint16]bool)
	for _, a := range assurances {
		if seen[a.ValidatorIndex] {
			return errors.New(fmt.Sprintf("duplicate assurance from validator %v", a.ValidatorIndex))
		}
		seen[a.ValidatorIndex] = true
	}
	if seen[new.ValidatorIndex] {
		return errors.New(fmt.Sprintf("duplicate assurance from validator %v", new.ValidatorIndex))
	}
	return nil
}

// State transition function
func (j *JamState) CountAvailableWR(assurances []types.Assurance) []uint32 {
	// Count the number of available assurances for each validator
	tally := make([]uint32, types.TotalCores)
	for c := 0; c < types.TotalCores; c++ {
		for _, a := range assurances {
			if a.GetBitFieldBit(uint16(c)) {
				tally[c]++
			}
		}
	}
	return tally
}

// 0.5.4 11.17
func (j *JamState) ProcessAssuranceState(tally []uint32, timeslot uint32) (uint32, []types.WorkReport) {
	// Update the validator's assurance state
	numAssurances := uint32(0)
	BigW := make([]types.WorkReport, 0) // eq 129: BigW (available work report) is the work report that has been assured by more than 2/3 validators
	for c, available := range tally {
		if j.AvailabilityAssignments[c] == nil {
			continue
		}
		report_ts := j.AvailabilityAssignments[c].Timeslot
		timeout := timeslot >= report_ts+uint32(types.UnavailableWorkReplacementPeriod)
		if available >= 2*types.TotalValidators/3 || timeout {
			if !timeout {
				BigW = append(BigW, j.AvailabilityAssignments[c].WorkReport) // available work report
			}
			j.AvailabilityAssignments[c] = nil
		}
		numAssurances++
	}
	return numAssurances, BigW
}

func (j *JamState) ProcessAssurances(assurances []types.Assurance, timeslot uint32) (uint32, []types.WorkReport) {
	// Count the number of available assurances for each validator
	tally := j.CountAvailableWR(assurances)
	// Update the validator's assurance state
	numAssurances, BigW := j.ProcessAssuranceState(tally, timeslot)
	return numAssurances, BigW
}

func SortAssurances(assurances []types.Assurance) {
	// Sort the assurances by validator index
	sort.Slice(assurances, func(i, j int) bool {
		return assurances[i].ValidatorIndex < assurances[j].ValidatorIndex
	})
}

// This function is for the validator to check the assurances extrinsic when the block is received
// Strong Verification
func (s *StateDB) ValidateAssurancesWithSig(assurances []types.Assurance) error {

	// MK...move coreIdx checking after transitioning
	//CheckSortingEAs(assurances)

	// Verify each assurance without checking signature
	transitionErr := s.ValidateAssurancesTransition(assurances)
	if transitionErr != nil {
		return transitionErr
	}

	// Sort the assurances by validator index
	sortingErr := CheckSortingEAs(assurances)
	if sortingErr != nil {
		return sortingErr
	}

	// Verify each assurance's signature
	sigErr := s.ValidateAssurancesSig(assurances)
	if sigErr != nil {
		return sigErr
	}

	return nil
}

// Verify each assurance without checking signature
func (s *StateDB) ValidateAssurancesTransition(assurances []types.Assurance) error {
	// Verify each assurance without checking signature
	for _, a := range assurances {
		if err := s.VerifyAssuranceWithoutSig(a); err != nil {
			return err
		}
	}
	err := s.ValidateAssurancesSig(assurances)
	if err != nil {
		return err
	}
	return nil
}

func (s *StateDB) ValidateAssurancesSig(assurances []types.Assurance) error {
	// Verify each assurance's signature

	validators := s.GetSafrole().CurrValidators
	for _, a := range assurances {

		// Shawn: I have to do one more check here to make sure the validator index won't be out of range
		//@mkchungs
		if a.ValidatorIndex >= uint16(len(validators)) {
			return jamerrors.ErrABadValidatorIndex
		}
		if err := a.VerifySignature(validators[a.ValidatorIndex]); err != nil {
			return err
		}
	}
	return nil
}

func (s *StateDB) ValidateAssurancesWithoutSig(assurances []types.Assurance) error {
	// Verify each assurance without checking signature
	for _, a := range assurances {
		if err := s.VerifyAssuranceWithoutSig(a); err != nil {
			return err
		}
	}
	return nil
}

func CheckSortingEAs(assurances []types.Assurance) error {
	// Check the SortAssurances is correct
	for i := 0; i < len(assurances)-1; i++ {
		if assurances[i].ValidatorIndex >= types.TotalValidators {
			return jamerrors.ErrABadValidatorIndex
		}
		if assurances[i].ValidatorIndex > assurances[i+1].ValidatorIndex {
			return jamerrors.ErrANotSortedAssurers
		}
		for j := i + 1; j < len(assurances); j++ {
			if assurances[i].ValidatorIndex == assurances[j].ValidatorIndex {
				return jamerrors.ErrADuplicateAssurer
			}
		}
	}
	return nil
}

// For generating assurance extrinsic
func (j *JamState) GetWorkReportFromRho() ([types.TotalCores]types.WorkReport, error) {
	reports := [types.TotalCores]types.WorkReport{}
	for i, rho := range j.AvailabilityAssignments {
		if rho == nil {
			reports[i] = types.WorkReport{}
		} else {
			reports[i] = rho.WorkReport
		}
	}
	return reports, nil
}
