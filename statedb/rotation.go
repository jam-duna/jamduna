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
		encoded, err := types.Encode(uint32(i / 8))
		if err != nil {
			panic(err)
		}
		data := common.ComputeHash(append(h.Bytes(), encoded...))
		decodedValue, _, err := types.Decode(data[4*i:4*(i+1)], reflect.TypeOf(uint32(0)))
		if err != nil {
			panic(err)
		}
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
func (s *StateDB) AssignGuarantors(lock ...bool) {
	lockbool := false
	if len(lock) > 0 {
		lockbool = lock[0]
	}
	if lockbool {
		assignments := make([]types.GuarantorAssignment, 0)
		entropy := common.Blake2Hash([]byte("colorfulnotion"))
		t := uint32(222)
		cores := Permute(entropy, t)
		for i, kappa := range s.JamState.SafroleState.CurrValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: cores[i],
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
				CoreIndex: cores[i],
				Validator: kappa,
			})
		}
		s.GuarantorAssignments = make([]types.GuarantorAssignment, len(assignments))
		copy(s.GuarantorAssignments, assignments)
	}

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
func (s *StateDB) PreviousGuarantors(lock ...bool) {
	lockbool := false
	if len(lock) > 0 {
		lockbool = lock[0]
	}
	if lockbool {
		assignments := make([]types.GuarantorAssignment, 0)
		entropy := common.Blake2Hash([]byte("colorfulnotion"))
		t := uint32(222)
		cores := Permute(entropy, t)
		for i, lambda := range s.JamState.SafroleState.PrevValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: cores[i],
				Validator: lambda,
			})
		}
		s.PreviousGuarantorAssignments = assignments

	} else {
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

}
