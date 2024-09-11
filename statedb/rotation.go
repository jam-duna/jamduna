package statedb

import (
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// need to review again, but it can run well
func ShuffleCores(slice []uint16, entropy []uint32) []uint16 {
	n := len(slice)
	for i := n - 1; i >= 0; i-- {
		j := entropy[i] % uint32(i+1)
		slice[i], slice[j] = slice[j], slice[i]
	}
	return slice
}

// 304 Ql   the numeric sequence-from-hash function
func Compute_QL(h common.Hash, num int) []uint32 {
	entropy := make([]uint32, num)
	for i := 0; i < num; i++ {
		data := common.ComputeHash(append(h.Bytes(), types.Encode(uint32(i/8))...))
		decodedValue, _ := types.Decode(data[4*i:4*(i+1)], reflect.TypeOf(uint32(0)))
		entropy[i] = decodedValue.(uint32)
	}
	return entropy
}

// 132
func Rotation(c uint16, n uint32) uint16 {
	return uint16((uint32(c) + n) % types.TotalCores)
}

// 133
func Permute(e common.Hash, t uint32) []uint16 {
	cores := make([]uint16, types.TotalValidators)
	for i := 0; i < types.TotalValidators; i++ {
		cores[i] = uint16(i * types.TotalCores / types.TotalValidators)
	}
	cores = ShuffleCores(cores, Compute_QL(e, types.TotalValidators))
	for _, core := range cores {
		Rotation(core, t%types.EpochLength/types.ValidatorCoreRotationPeriod)
	}
	return cores
}

// 134
func (s *StateDB) AssignGuarantors() {
	assignments := make([]types.GuarantorAssignment, 0)
	entropy := s.JamState.SafroleState.Entropy[2]
	t := s.JamState.SafroleState.Timeslot
	cores := Permute(entropy, t)
	for i, kappa := range s.JamState.SafroleState.CurrValidators {
		assignments = append(assignments, types.GuarantorAssignment{
			CoreIndex: cores[i],
			Validator: kappa,
		})
	}
	s.GuarantorAssignments = assignments
}

// 134
func (s *StateDB) AssignGuarantorsTesting(entropy common.Hash) []types.GuarantorAssignment {
	assignments := make([]types.GuarantorAssignment, 0)
	t := s.JamState.SafroleState.Timeslot
	cores := Permute(entropy, t)
	for i, kappa := range s.JamState.SafroleState.CurrValidators {
		assignments = append(assignments, types.GuarantorAssignment{
			CoreIndex: cores[i],
			Validator: kappa,
		})
	}
	return assignments
}

// 135
func (s *StateDB) PreviousGuarantors() {
	assignments := make([]types.GuarantorAssignment, 0)
	if (s.JamState.SafroleState.Timeslot-types.ValidatorCoreRotationPeriod)/types.EpochLength == s.JamState.SafroleState.Timeslot/types.EpochLength {
		entropy := s.JamState.SafroleState.Entropy[1]
		t := s.JamState.SafroleState.Timeslot - types.ValidatorCoreRotationPeriod
		cores := Permute(entropy, t)
		for i, kappa := range s.JamState.SafroleState.CurrValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: cores[i],
				Validator: kappa,
			})
		}
	} else {
		entropy := s.JamState.SafroleState.Entropy[2]
		t := s.JamState.SafroleState.Timeslot - types.ValidatorCoreRotationPeriod
		cores := Permute(entropy, t)
		for i, lambda := range s.JamState.SafroleState.PrevValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: cores[i],
				Validator: lambda,
			})
		}
	}
	s.PreviousGuarantorAssignments = assignments
}
