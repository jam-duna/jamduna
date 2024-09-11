package statedb

import (
	//"github.com/ethereum/go-ethereum/crypto"

	"errors"
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// check validity of assurance

func (s *StateDB) Verify_H_P(a types.Assurance) error {
	//check the anchor
	if a.Anchor != s.ParentHash {
		return errors.New(fmt.Sprintf("invalid anchor in assurance %x, expected %x, Validator[%v]", a.Anchor, s.ParentHash, a.ValidatorIndex))
	}
	return nil
}

func (s *StateDB) VerifySignature_EA(a types.Assurance) error {
	// Verify the signature
	hash := common.ComputeHash(append(s.Block.Header.Parent[:], a.BitFieldToBytes()...))
	signtext := append([]byte(types.X_A), hash...)

	//verify the signature
	if !types.Ed25519Verify(types.Ed25519Key(s.GetSafrole().CurrValidators[a.ValidatorIndex].Ed25519), signtext, a.Signature) {
		return errors.New("invalid signature")
	}
	return nil
}

func (s *StateDB) VerifyAssurance(a types.Assurance) error {
	// Verify the anchor
	if err := s.Verify_H_P(a); err != nil {
		return err
	}

	// Verify the signature
	if err := s.VerifySignature_EA(a); err != nil {
		return err
	}

	return nil
}

// state trasition function
// 129
func (J *JamState) CountAvailableWR(assurances []types.Assurance) []uint32 {
	// Count the number of available assurances for each validator
	tally := make([]uint32, types.TotalCores)
	for _, a := range assurances {
		for c := 0; c < types.TotalCores; c++ {
			if a.GetBitFied_Bit(uint16(c)) == true {
				tally[c]++
			}
		}
	}
	return tally
}

// 130
func (J *JamState) ProcessAssuranceState(tally []uint32) (uint32, []types.WorkReport) {
	// Update the validator's assurance state
	num_assurances := uint32(0)
	Big_W := make([]types.WorkReport, types.TotalCores) //eq 129: Big_W(avalible work report) is the work report that has been assured by more than 2/3 validators
	for c, available := range tally {
		if available > 2*types.TotalValidators/3 {
			Big_W[c] = J.AvailabilityAssignments[c].WorkReport
			J.AvailabilityAssignments[c] = nil
		}
		num_assurances++
	}
	return num_assurances, Big_W
}

func (J *JamState) ProcessAssurances(assurances []types.Assurance) (uint32, []types.WorkReport) {
	// Count the number of available assurances for each validator
	tally := J.CountAvailableWR(assurances)

	// Update the validator's assurance state
	num_assurances, Big_W := J.ProcessAssuranceState(tally)
	return num_assurances, Big_W
}

func SortAssurances(Assurances []types.Assurance) {
	// Sort the assurances by validator index
	sort.Slice(Assurances, func(i, j int) bool {
		return Assurances[i].ValidatorIndex < Assurances[j].ValidatorIndex
	})
}

// this function is for the validator check the assurances extrinsic when the block is received
// Strong Verification
func (S *StateDB) ValidateAssurances(assurances []types.Assurance) error {
	// Sort the assurances by validator index
	CheckSorting_EAs(assurances)
	// Verify each assurance
	for _, a := range assurances {
		if err := S.VerifyAssurance(a); err != nil {
			return err
		}
	}
	return nil
}

func CheckSorting_EAs(assurances []types.Assurance) error {
	// check the SortAssurances is correct
	for i := 0; i < len(assurances)-1; i++ {
		if assurances[i].ValidatorIndex >= assurances[i+1].ValidatorIndex {
			return errors.New("assurances are not sorted")
		}
	}
	return nil

}
