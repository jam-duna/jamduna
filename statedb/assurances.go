package statedb

import (
	"context"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// State transition function
func (j *JamState) CountAvailableWR(validatedAssurances []types.Assurance) ([]uint32, map[uint16]uint16) {
	// Count the number of available assurances for each validator
	num_assurances := make(map[uint16]uint16)
	for _, a := range validatedAssurances {
		num_assurances[a.ValidatorIndex] = 1
	}
	tally := make([]uint32, types.TotalCores)
	for c := range types.TotalCores {
		for _, a := range validatedAssurances {
			if a.GetBitFieldBit(uint16(c)) {
				tally[c]++
			}
		}
	}
	return tally, num_assurances
}

func (j *JamState) ComputeAvailabilityAssignments(validatedAssurances []types.Assurance, timeslot uint32) (wr []types.WorkReport, num_assurances map[uint16]uint16) {
	// Count the number of available assurances for each validator on each core
	tally, num_assurances := j.CountAvailableWR(validatedAssurances)
	wr = make([]types.WorkReport, 0)
	for c, available := range tally {
		if j.AvailabilityAssignments[c] == nil {
			continue
		}
		diff := timeslot - (j.AvailabilityAssignments[c].Timeslot + uint32(types.UnavailableWorkReplacementPeriod))
		timeout := diff > 0
		if available > 2*types.TotalValidators/3 || timeout {
			if timeout {
				//fmt.Printf("Core %d: Timeout detected, removing work report -- %d - %d = %d\n", c, timeslot, j.AvailabilityAssignments[c].Timeslot+uint32(types.UnavailableWorkReplacementPeriod), diff)
			} else {
				wr = append(wr, j.AvailabilityAssignments[c].WorkReport)
			}
			j.AvailabilityAssignments[c] = nil
		}
	}
	return wr, num_assurances
}

func (s *StateDB) getWRStatus() (hasRecentWR, hasStaleWR []bool) {
	ts := s.JamState.SafroleState.Timeslot
	hasRecentWR = make([]bool, types.TotalCores)
	hasStaleWR = make([]bool, types.TotalCores)
	for core, rho := range s.JamState.AvailabilityAssignments {
		if rho != nil {
			hasRecentWR[core] = true
			if ts > rho.Timeslot+uint32(types.UnavailableWorkReplacementPeriod) {
				hasStaleWR[core] = true
			}
		}
	}
	return
}

func (s *StateDB) checkAssurance(a types.Assurance, anchor common.Hash, validators types.Validators, hasRecentWR, hasStaleWR []bool) error {
	if a.Anchor != anchor {
		return jamerrors.ErrABadParentHash
	}
	if int(a.ValidatorIndex) >= len(validators) {
		return jamerrors.ErrABadValidatorIndex
	}
	if err := a.VerifySignature(validators[a.ValidatorIndex]); err != nil {
		return jamerrors.ErrABadSignature
	}
	if !a.ValidBitfield(hasRecentWR) {
		return jamerrors.ErrABadCore
	}
	if !a.CheckTimeout(hasStaleWR) {
		return jamerrors.ErrAStaleReport
	}
	return nil
}

func (s *StateDB) GetValidAssurances(assurances []types.Assurance, anchor common.Hash) (checkedAssurances []types.Assurance) {
	validators := s.GetSafrole().CurrValidators
	hasRecentWR, hasStaleWR := s.getWRStatus()

	for _, a := range assurances {
		if err := s.checkAssurance(a, anchor, validators, hasRecentWR, hasStaleWR); err == nil {
			// no error, so add to the list of valid assurances
			checkedAssurances = append(checkedAssurances, a)
		} else {
			log.Warn(log.SDB, "GetValidAssurances checkAssurance", "err", err)
		}
	}
	// Sort the assurances by validator index
	sort.Slice(checkedAssurances, func(i, j int) bool {
		return checkedAssurances[i].ValidatorIndex < checkedAssurances[j].ValidatorIndex
	})

	return
}

func (s *StateDB) ValidateAssurances(ctx context.Context, assurances []types.Assurance, anchor common.Hash) error {
	validators := s.GetSafrole().CurrValidators
	hasRecentWR, hasStaleWR := s.getWRStatus()
	// check sorting by validatorIndex
	var prevIndex uint16
	for i, a := range assurances {
		if err := s.checkAssurance(a, anchor, validators, hasRecentWR, hasStaleWR); err != nil {
			return err
		}
		if i > 0 {
			if a.ValidatorIndex == prevIndex {
				return jamerrors.ErrADuplicateAssurer
			}
			if a.ValidatorIndex < prevIndex {
				return jamerrors.ErrANotSortedAssurers
			}
		}
		prevIndex = a.ValidatorIndex
		select {
		case <-ctx.Done():
			return fmt.Errorf("ValidateAssurances canceled")
		default:
		}
	}
	return nil
}

// For generateAssurance, get the INCOMING work package hashes ... that have **not timed out**
func (j *JamState) GetRecentWorkPackagesFromRho(timeslot uint32) (wph map[uint16]common.Hash) {
	wph = make(map[uint16]common.Hash)
	for core, rho := range j.AvailabilityAssignments {
		if rho != nil && (timeslot-rho.Timeslot < uint32(types.UnavailableWorkReplacementPeriod)) {
			wph[uint16(core)] = rho.WorkReport.AvailabilitySpec.WorkPackageHash
		}
	}
	return wph
}
