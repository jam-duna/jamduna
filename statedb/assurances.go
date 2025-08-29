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
	log.Trace(log.A, "CountAvailableWR", "num_validated_assurances", len(validatedAssurances))
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
		log.Trace(log.A, "CountAvailableWR", "core", c, "available", tally[c])
	}
	return tally, num_assurances
}

func (j *JamState) GetReportTimeouts(timeslot uint32) []bool {
	timeouts := make([]bool, types.TotalCores)
	for i, assignment := range j.AvailabilityAssignments {
		if assignment == nil {
			timeouts[i] = false // no report, no timeout
			continue
		}
		timeouts[i] = timeslot >= assignment.Timeslot+uint32(types.UnavailableWorkReplacementPeriod) // (11.17)
	}
	return timeouts
}

func (j *JamState) ComputeAvailabilityAssignments(validatedAssurances []types.Assurance, timeslot uint32) (wr []types.WorkReport, num_assurances map[uint16]uint16) {
	tally, num_assurances := j.CountAvailableWR(validatedAssurances)
	wr = make([]types.WorkReport, 0)

	reportTimeouts := j.GetReportTimeouts(timeslot)

	for c, availableCnt := range tally {
		if j.AvailabilityAssignments[c] == nil {
			continue
		}
		assuranceThreshold := uint32(2 * types.TotalValidators / 3)
		isTimedOut := reportTimeouts[c]
		isThresholdReached := availableCnt > assuranceThreshold
		if isThresholdReached {
			log.Trace(log.A, "ComputeAvailabilityAssignments - Reached availability threshold", "Core", c, "availableCnt", availableCnt, "threshold", assuranceThreshold, "timeslot", timeslot)
			wr = append(wr, j.AvailabilityAssignments[c].WorkReport)
		}
		if isTimedOut {
			log.Trace(log.A, "ComputeAvailabilityAssignments - Timeout detected", "Core", c, "availableCnt", availableCnt, "threshold", assuranceThreshold, "timeslot", timeslot)
		}
		if isThresholdReached || isTimedOut {
			j.AvailabilityAssignments[c] = nil
			log.Trace(log.A, "ComputeAvailabilityAssignments - Clearing core assignment", "Core", c, "availableCnt", availableCnt, "threshold", assuranceThreshold, "timeslot", timeslot)
		}
	}

	return wr, num_assurances
}

func (s *StateDB) getWRStatus() (hasRecentWR, hasStaleWR []bool) {
	ts := s.JamState.SafroleState.Timeslot
	hasRecentWR = make([]bool, types.TotalCores)
	hasStaleWR = make([]bool, types.TotalCores)
	for core, availability_assignment := range s.JamState.AvailabilityAssignments {
		if availability_assignment != nil {
			hasRecentWR[core] = true
			// (11.17) A work-report is considered stale if its timeslot `availability_assignment_t` is greater than or equal timeslot `t` by at least `UWRP` period
			if ts >= availability_assignment.Timeslot+uint32(types.UnavailableWorkReplacementPeriod) {
				hasStaleWR[core] = true
			}
		}
	}
	return
}

func (s *StateDB) checkAssurance(a types.Assurance, anchor common.Hash, validators types.Validators, hasRecentWR, hasStaleWR []bool) error {

	// (11.11) assurance's anchor `a_a` to be the parent block hash `H_p`.
	if a.Anchor != anchor {
		return jamerrors.ErrABadParentHash
	}

	// (11.13)
	if int(a.ValidatorIndex) >= len(validators) {
		return jamerrors.ErrABadValidatorIndex
	}

	// (11.13)
	if err := a.VerifySignature(validators[a.ValidatorIndex]); err != nil {
		log.Error(log.SDB, "Assurance signature verification failed", "err", err, "validator_index", a.ValidatorIndex)
		return jamerrors.ErrABadSignature
	}

	// (11.15) if a bit `a_f[c]` is set for a core, there must be a pending work-report on that core
	if !a.ValidBitfield(hasRecentWR) {
		return jamerrors.ErrABadCore
	}
	return nil
}

func (s *StateDB) GetValidAssurances(assurances []types.Assurance, anchor common.Hash, sortRequired bool) (checkedAssurances []types.Assurance, err error) {
	validators := s.GetSafrole().CurrValidators
	hasRecentWR, hasStaleWR := s.getWRStatus()

	for _, a := range assurances {
		if err := s.checkAssurance(a, anchor, validators, hasRecentWR, hasStaleWR); err == nil {
			// no error, so add to the list of valid assurances
			checkedAssurances = append(checkedAssurances, a)
		} else {
			log.Error(log.SDB, "GetValidAssurances checkAssurance", "err", err)
			return nil, err
		}
	}
	// Sort the assurances by validator index
	if sortRequired {
		sort.Slice(checkedAssurances, func(i, j int) bool {
			return checkedAssurances[i].ValidatorIndex < checkedAssurances[j].ValidatorIndex
		})
	}

	return
}

// ValidateAssurances checks the validity of assurances against the current state.
// Note that checkTimeout should only be true for the Assurances STF as timeouts are treated within JAM logic
func (s *StateDB) ValidateAssurances(ctx context.Context, assurances []types.Assurance, anchor common.Hash, checkTimeout bool) error {
	validators := s.GetSafrole().CurrValidators
	hasRecentWR, hasStaleWR := s.getWRStatus()
	// check sorting by validatorIndex
	//var prevIndex int
	prevIndex := -1
	for _, a := range assurances {
		if err := s.checkAssurance(a, anchor, validators, hasRecentWR, hasStaleWR); err != nil {
			//log.Error(log.SDB, "ValidateAssurances checkAssurance", "err", err, "assurance", a)
			return err
		}
		if checkTimeout {
			if !a.CheckTimeout(hasStaleWR) {
				return jamerrors.ErrAStaleReport
			}
		}
		if int(a.ValidatorIndex) == prevIndex {
			return jamerrors.ErrADuplicateAssurer
		}
		if int(a.ValidatorIndex) < prevIndex {
			return jamerrors.ErrANotSortedAssurers
		}
		prevIndex = int(a.ValidatorIndex)
		select {
		case <-ctx.Done():
			return fmt.Errorf("ValidateAssurances canceled")
		default:
		}
	}
	return nil
}

// For generateAssurance, get the INCOMING work package hashes ... that have **not timed out**
func (j *JamState) GetRecentWorkPackagesFromAvailabilityAssignments(timeslot uint32) (wph map[uint16]common.Hash) {
	wph = make(map[uint16]common.Hash)
	for core, availability_assignment := range j.AvailabilityAssignments {
		if availability_assignment != nil && (timeslot-availability_assignment.Timeslot < uint32(types.UnavailableWorkReplacementPeriod)) {
			wph[uint16(core)] = availability_assignment.WorkReport.AvailabilitySpec.WorkPackageHash
		}
	}
	return wph
}
