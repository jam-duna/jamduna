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

// v0.5.0 (11.20) + (11.21)
// RotateGuarantors computes GuarantorAssignments and PreviousGuarantorAssignments based
// on SafroleState (a) Entropy[2] or Entropy[3] and (b) the current Timeslot.
func (s *StateDB) RotateGuarantors() {
	// uses (a) entropy[2] and timesslot to update s.GuarantorAssignments
	assignments := make([]types.GuarantorAssignment, 0)
	entropy := s.JamState.SafroleState.Entropy[2]
	t := s.JamState.SafroleState.Timeslot
	//fmt.Printf("[N%d] t=%d RotateGuarantors before rototation s.PreviousGuarantorAssignments=%x | s.GuarantorAssignments=%x\n", s.Id, t, s.PreviousGuarantorAssignments, s.GuarantorAssignments)
	cores := Permute(entropy, t)
	for i, kappa := range s.JamState.SafroleState.CurrValidators {
		assignments = append(assignments, types.GuarantorAssignment{
			CoreIndex: uint16(cores[i]),
			Validator: kappa,
		})
	}
	s.GuarantorAssignments = make([]types.GuarantorAssignment, len(assignments))
	copy(s.GuarantorAssignments, assignments)

	assignments = make([]types.GuarantorAssignment, 0)
	t = s.JamState.SafroleState.Timeslot - types.ValidatorCoreRotationPeriod
	if (s.JamState.SafroleState.Timeslot-types.ValidatorCoreRotationPeriod)/types.EpochLength == s.JamState.SafroleState.Timeslot/types.EpochLength {
		cores := Permute(s.JamState.SafroleState.Entropy[2], t)
		for i, kappa := range s.JamState.SafroleState.CurrValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: kappa,
			})
		}
	} else {
		cores := Permute(s.JamState.SafroleState.Entropy[3], t)
		if len(s.JamState.SafroleState.PrevValidators) == 0 {
			panic("PrevValidators is empty")
		}
		for i, lambda := range s.JamState.SafroleState.PrevValidators {
			assignments = append(assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: lambda,
			})
		}
	}
	s.PreviousGuarantorAssignments = make([]types.GuarantorAssignment, len(assignments))
	copy(s.PreviousGuarantorAssignments, assignments)
	//fmt.Printf("[N%d] t=%d RotateGuarantors after rototation s.PreviousGuarantorAssignments=%x | s.GuarantorAssignments=%x\n", s.Id, t, s.PreviousGuarantorAssignments, s.GuarantorAssignments)

}

// this function are using the timeslot from block header and the entropy from safrole state(shift if needed). To Calculate the guarantor assignments
func (s *StateDB) CaculateAssignments(shift_bool bool) (PreviousGuarantorAssignments []types.GuarantorAssignment, GuarantorAssignments []types.GuarantorAssignment) {

	// uses (a) entropy[2] and timesslot to update s.GuarantorAssignments
	assignments := make([]types.GuarantorAssignment, 0)
	entropy := s.JamState.SafroleState.Entropy[2]
	if shift_bool {
		entropy = s.JamState.SafroleState.Entropy[1]
	}
	t := s.Block.Header.Slot
	//fmt.Printf("[N%d] t=%d RotateGuarantors before rototation s.PreviousGuarantorAssignments=%x | s.GuarantorAssignments=%x\n", s.Id, t, s.PreviousGuarantorAssignments, s.GuarantorAssignments)
	cores := Permute(entropy, t)
	for i, kappa := range s.JamState.SafroleState.CurrValidators {
		assignments = append(assignments, types.GuarantorAssignment{
			CoreIndex: uint16(cores[i]),
			Validator: kappa,
		})
	}

	prev_assignments := make([]types.GuarantorAssignment, 0)
	t = s.Block.Header.Slot - types.ValidatorCoreRotationPeriod
	prev_using_entropy := 2
	if shift_bool {
		prev_using_entropy = 1
	}
	if (s.JamState.SafroleState.Timeslot-types.ValidatorCoreRotationPeriod)/types.EpochLength == s.JamState.SafroleState.Timeslot/types.EpochLength {
		cores := Permute(s.JamState.SafroleState.Entropy[prev_using_entropy], t)
		for i, kappa := range s.JamState.SafroleState.CurrValidators {
			prev_assignments = append(prev_assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: kappa,
			})
		}
	} else {
		cores := Permute(s.JamState.SafroleState.Entropy[prev_using_entropy+1], t)
		for i, lambda := range s.JamState.SafroleState.PrevValidators {
			prev_assignments = append(prev_assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: lambda,
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
