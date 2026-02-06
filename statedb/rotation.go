package statedb

import (
	"fmt"

	"github.com/jam-duna/jamduna/common"
	log "github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/types"
)

// (11.19) Rotation r(c,n)
func Rotation(c []uint32, n uint32) []uint32 {
	result := make([]uint32, len(c))
	for i, x := range c {
		result[i] = (x + n) % types.TotalCores
	}
	return result
}

// (11.20) Permute p(e,t)
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

// (11.20) + (11.21)
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
	if s.JamState.SafroleState.Timeslot >= types.ValidatorCoreRotationPeriod {
		t = s.JamState.SafroleState.Timeslot - types.ValidatorCoreRotationPeriod
	} else {
		t = 0
	}
	prev_slot := uint32(0)
	if s.JamState.SafroleState.Timeslot >= types.ValidatorCoreRotationPeriod {
		prev_slot = s.JamState.SafroleState.Timeslot - types.ValidatorCoreRotationPeriod
	}
	if prev_slot/types.EpochLength == s.JamState.SafroleState.Timeslot/types.EpochLength {
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
			log.Warn(log.SDB, "PrevValidators is empty, falling back to CurrValidators", "timeslot", s.JamState.SafroleState.Timeslot)
			for i, validator := range s.JamState.SafroleState.CurrValidators {
				assignments = append(assignments, types.GuarantorAssignment{
					CoreIndex: uint16(cores[i]),
					Validator: validator,
				})
			}
		} else {
			for i, validator := range s.JamState.SafroleState.PrevValidators {
				assignments = append(assignments, types.GuarantorAssignment{
					CoreIndex: uint16(cores[i]),
					Validator: validator,
				})
			}
		}
	}
	s.PreviousGuarantorAssignments = make([]types.GuarantorAssignment, len(assignments))
	copy(s.PreviousGuarantorAssignments, assignments)
}

func (s *StateDB) CalculateAssignments(slot uint32) (PreviousGuarantorAssignments []types.GuarantorAssignment, GuarantorAssignments []types.GuarantorAssignment) {
	safrole := s.JamState.SafroleState

	// (11.21) Current assignments M: (η2, k)
	assignments := make([]types.GuarantorAssignment, 0)
	cores := Permute(safrole.Entropy[2], slot)
	for i, validator := range safrole.CurrValidators {
		assignments = append(assignments, types.GuarantorAssignment{
			CoreIndex: uint16(cores[i]),
			Validator: validator,
		})
	}

	// (11.22) Previous assignments M* = (η2, k) if (τ - R) ÷ E = τ ÷ E, otherwise (η3, λ)
	prev_assignments := make([]types.GuarantorAssignment, 0)
	prev_slot := uint32(0)
	if slot >= types.ValidatorCoreRotationPeriod {
		prev_slot = slot - types.ValidatorCoreRotationPeriod
	}

	if prev_slot/types.EpochLength == slot/types.EpochLength {
		cores := Permute(safrole.Entropy[2], prev_slot)
		for i, validator := range safrole.CurrValidators {
			prev_assignments = append(prev_assignments, types.GuarantorAssignment{
				CoreIndex: uint16(cores[i]),
				Validator: validator,
			})
		}
	} else {
		cores := Permute(safrole.Entropy[3], prev_slot)
		if len(safrole.PrevValidators) == 0 {
			log.Warn(log.SDB, "PrevValidators is empty, falling back to CurrValidators", "slot", slot)
			for i, validator := range safrole.CurrValidators {
				prev_assignments = append(prev_assignments, types.GuarantorAssignment{
					CoreIndex: uint16(cores[i]),
					Validator: validator,
				})
			}
		} else {
			for i, validator := range safrole.PrevValidators {
				prev_assignments = append(prev_assignments, types.GuarantorAssignment{
					CoreIndex: uint16(cores[i]),
					Validator: validator,
				})
			}
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
