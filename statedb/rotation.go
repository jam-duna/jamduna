package statedb

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// v0.5.0 (11.18)
func Rotation(c []uint32, n uint32) []uint32 {
	result := make([]uint32, len(c))
	for i, x := range c {
		result[i] = (x + n) % types.TotalCores
	}
	return result
}

// v0.5.0 (11.19)
func Permute(e common.Hash, t uint32) []uint32 {
	cores := make([]uint32, types.TotalValidators)
	for i := 0; i < types.TotalValidators; i++ {
		cores[i] = uint32(types.TotalCores * i / types.TotalValidators)
	}
	cores = ShuffleFromHash(cores, e)
	uint16Cores := make([]uint32, len(cores))
	for i, x := range cores {
		uint16Cores[i] = uint32(x)
	}
	return Rotation(uint16Cores, (t%types.EpochLength)/types.ValidatorCoreRotationPeriod)
}

// v0.5.0 (11.20) + (11.21)
// RotateGuarantors computes GuarantorAssignments and PreviousGuarantorAssignments based
// on SafroleState (a) Entropy[2] or Entropy[3] and (b) the current Timeslot.
func (s *StateDB) RotateGuarantors() {
	// uses (a) entropy[2] and timesslot to update s.GuarantorAssignments
	assignments := make([]types.GuarantorAssignment, 0)
	entropy := s.JamState.SafroleState.Entropy[2]
	t := s.JamState.SafroleState.Timeslot
	cores := Permute(entropy, t)
	for i, v := range s.JamState.SafroleState.CurrValidators {
		assignments = append(assignments, types.GuarantorAssignment{
			CoreIndex: uint16(cores[i]),
			Validator: v,
		})
	}
	s.GuarantorAssignments = make([]types.GuarantorAssignment, len(assignments))
	copy(s.GuarantorAssignments, assignments)

	assignments = make([]types.GuarantorAssignment, 0)
	t = s.JamState.SafroleState.Timeslot - types.ValidatorCoreRotationPeriod
	if (s.JamState.SafroleState.Timeslot-types.ValidatorCoreRotationPeriod)/types.EpochLength == s.JamState.SafroleState.Timeslot/types.EpochLength {
		cores := Permute(s.JamState.SafroleState.Entropy[2], t)
		for i, validator := range s.JamState.SafroleState.CurrValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: validator,
			})
		}
	} else {
		cores := Permute(s.JamState.SafroleState.Entropy[3], t)
		if len(s.JamState.SafroleState.PrevValidators) == 0 {
			log.Crit(log.SDB, "PrevValidators is empty")
			return
		}
		for i, validator := range s.JamState.SafroleState.PrevValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: validator,
			})
		}
	}
	s.PreviousGuarantorAssignments = make([]types.GuarantorAssignment, len(assignments))
	copy(s.PreviousGuarantorAssignments, assignments)
}

// this function are using the timeslot from block header and the entropy from safrole state(shift if needed). To Calculate the guarantor assignments
func (s *StateDB) CalculateAssignments(slot uint32) (PreviousGuarantorAssignments []types.GuarantorAssignment, GuarantorAssignments []types.GuarantorAssignment) {
	// uses (a) entropy[2] and timeslot to update s.GuarantorAssignments
	sf_tmp := s.JamState.SafroleState
	assignments := make([]types.GuarantorAssignment, 0)
	entropy := sf_tmp.Entropy[2]
	t := slot
	cores := Permute(entropy, t)
	for i, validator := range sf_tmp.CurrValidators {
		assignments = append(assignments, types.GuarantorAssignment{
			CoreIndex: uint16(cores[i]),
			Validator: validator,
		})
	}

	prev_assignments := make([]types.GuarantorAssignment, 0)
	t = slot - types.ValidatorCoreRotationPeriod
	prev_using_entropy := 2
	if (slot-types.ValidatorCoreRotationPeriod)/types.EpochLength == slot/types.EpochLength {
		cores := Permute(sf_tmp.Entropy[prev_using_entropy], t)
		for i, validator := range sf_tmp.CurrValidators {
			prev_assignments = append(prev_assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: validator,
			})
		}
	} else {
		cores := Permute(sf_tmp.Entropy[prev_using_entropy+1], t)
		for i, validator := range sf_tmp.PrevValidators {
			prev_assignments = append(prev_assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: validator,
			})
		}
	}
	return prev_assignments, assignments

}

func (s *StateDB) GuarantorsAssignmentsPrint() {
	sf := s.GetSafrole()
	fmt.Printf("CurrGuarantorAssignments:\n")
	for _, v := range s.GuarantorAssignments {
		validator_index := sf.GetCurrValidatorIndex(v.Validator.Ed25519)
		if validator_index == -1 {
			fmt.Printf("Validator not found for %v\n", v.Validator.Ed25519)
			continue
		}
		core_index := v.CoreIndex
		fmt.Printf("CoreIndex: %v => Validator: %v, key: %v\n", core_index, validator_index, v.Validator.Ed25519)
	}
	fmt.Printf("PrevGuarantorAssignments:\n")
	if (s.JamState.SafroleState.Timeslot-types.ValidatorCoreRotationPeriod)/types.EpochLength == s.JamState.SafroleState.Timeslot/types.EpochLength {
		fmt.Printf("using entropy[2]\n")
	} else {
		fmt.Printf("using entropy[3]\n")
	}
	if len(s.PreviousGuarantorAssignments) == 0 {
		fmt.Printf("PreviousGuarantorAssignments is empty\n")
	}
	for _, v := range s.PreviousGuarantorAssignments {
		validator_index := sf.GetCurrValidatorIndex(v.Validator.Ed25519)
		if (s.JamState.SafroleState.Timeslot-types.ValidatorCoreRotationPeriod)/types.EpochLength < s.JamState.SafroleState.Timeslot/types.EpochLength {
			validator_index = sf.GetPrevValidatorIndex(v.Validator.Ed25519)
		}
		if validator_index == -1 {
			fmt.Printf("Validator not found for %v\n", v.Validator.Ed25519)
			continue
		}
		core_index := v.CoreIndex
		fmt.Printf("CoreIndex: %v => Validator: %v, key: %v\n", core_index, validator_index, v.Validator.Ed25519)
	}
}

func (s *StateDB) GetCoreCoWorkers(core_index uint16) []types.Validator {
	co_workers := make([]types.Validator, 0)
	for _, v := range s.GuarantorAssignments {
		if v.CoreIndex == core_index {
			co_workers = append(co_workers, v.Validator)
		}
	}
	return co_workers
}

func (s *StateDB) GetPrevCoreCoWorkers(core_index uint16) []types.Validator {
	co_workers := make([]types.Validator, 0)
	for _, v := range s.PreviousGuarantorAssignments {
		if v.CoreIndex == core_index {
			co_workers = append(co_workers, v.Validator)
		}
	}
	return co_workers
}

func (s *StateDB) GetSelfCoreIndex() uint16 {
	curr_validators := s.JamState.SafroleState.CurrValidators
	for _, v := range s.GuarantorAssignments {
		if v.Validator.Ed25519 == curr_validators[s.Id].Ed25519 {
			return v.CoreIndex
		}
	}
	return 0
}
