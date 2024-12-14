package statedb

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
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

// v0.5.0 (11.20)
func (s *StateDB) AssignGuarantors(lock bool) {
	if lock {
		assignments := make([]types.GuarantorAssignment, 0)
		fixed_assignment := map[uint16]uint16{
			0: 1,
			1: 0,
			2: 1,
			3: 1,
			4: 0,
			5: 0,
		}
		for i, kappa := range s.JamState.SafroleState.CurrValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: fixed_assignment[uint16(i)],
				Validator: kappa,
			})
		}
		s.GuarantorAssignments = make([]types.GuarantorAssignment, len(assignments))
		copy(s.GuarantorAssignments, assignments)

	} else {
		assignments := make([]types.GuarantorAssignment, 0)
		entropy := s.JamState.SafroleState.Entropy[2]
		t := s.JamState.SafroleState.Timeslot
		cores := Permute(entropy, t)
		for i, kappa := range s.JamState.SafroleState.CurrValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: kappa,
			})
		}
		s.GuarantorAssignments = make([]types.GuarantorAssignment, len(assignments))
		copy(s.GuarantorAssignments, assignments)
	}
}

// v0.5.0 (11.20)
func (s *StateDB) AssignGuarantorsTesting(entropy common.Hash) []types.GuarantorAssignment {
	assignments := make([]types.GuarantorAssignment, 0)
	t := s.JamState.SafroleState.Timeslot
	cores := Permute(entropy, t)
	for i, kappa := range s.JamState.SafroleState.CurrValidators {
		assignments = append(assignments, types.GuarantorAssignment{
			CoreIndex: uint16(cores[i]),
			Validator: kappa,
		})
	}
	return assignments
}

// v0.5.0 (11.21)
func (s *StateDB) PreviousGuarantors(lock bool) {
	if lock {
		assignments := make([]types.GuarantorAssignment, 0)
		fixed_assignment := map[uint16]uint16{
			0: 1,
			1: 0,
			2: 1,
			3: 1,
			4: 0,
			5: 0,
		}
		for i, lambda := range s.JamState.SafroleState.PrevValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: fixed_assignment[uint16(i)],
				Validator: lambda,
			})
		}
		s.PreviousGuarantorAssignments = assignments

	} else {
		assignments := make([]types.GuarantorAssignment, 0)
		if (s.JamState.SafroleState.Timeslot-types.ValidatorCoreRotationPeriod)/types.EpochLength == s.JamState.SafroleState.Timeslot/types.EpochLength {
			entropy := s.JamState.SafroleState.Entropy[2]
			t := s.JamState.SafroleState.Timeslot - types.ValidatorCoreRotationPeriod
			cores := Permute(entropy, t)
			for i, kappa := range s.JamState.SafroleState.CurrValidators {
				assignments = append(assignments, types.GuarantorAssignment{
					CoreIndex: uint16(cores[i]),
					Validator: kappa,
				})
			}
		} else {
			entropy := s.JamState.SafroleState.Entropy[3]
			t := s.JamState.SafroleState.Timeslot - types.ValidatorCoreRotationPeriod
			cores := Permute(entropy, t)

			for i, lambda := range s.JamState.SafroleState.PrevValidators {
				assignments = append(assignments, types.GuarantorAssignment{
					CoreIndex: uint16(cores[i]),
					Validator: lambda,
				})
			}
		}
		s.PreviousGuarantorAssignments = assignments
	}

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
