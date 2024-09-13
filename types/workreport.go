package types

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

/*
11.1.1. Work Report. See Equation 117. A work-report, of the set W, is defined as a tuple of:
* the work-package specification $s$,
* the refinement context $x$,
* the core-index $c$ (i.e. on which the work is done)
* the authorizer hash $a$ and
* output ${\bf o}$ and
* $r$, the results of the evaluation of each of the items in the package r, which is always at least one item and may be no more than I items.
*/
// WorkReport represents a work report.

type WorkReport struct {
	AvailabilitySpec AvailabilitySpecifier `json:"package_spec"`
	RefineContext    RefineContext         `json:"context"`
	CoreIndex        uint16                `json:"core_index"`
	AuthorizerHash   common.Hash           `json:"authorizer_hash"`
	AuthOutput       []byte                `json:"auth_output"`
	Results          []WorkResult          `json:"results"`
}

type SWorkReport struct {
	AvailabilitySpec CAvailabilitySpecifier `json:"package_spec"`
	RefineContext    RefineContext          `json:"context"`
	CoreIndex        uint16                 `json:"core_index"`
	AuthorizerHash   common.Hash            `json:"authorizer_hash"`
	AuthOutput       string                 `json:"auth_output"`
	Results          []SWorkResult          `json:"results"`
}

type CWorkReport struct {
	AvailabilitySpec CAvailabilitySpecifier `json:"package_spec"`
	RefineContext    RefineContext          `json:"context"`
	CoreIndex        uint16                 `json:"core_index"`
	AuthorizerHash   common.Hash            `json:"authorizer_hash"`
	AuthOutput       []byte                 `json:"auth_output"`
	Results          []WorkResult           `json:"results"`
}

// computeWorkReportBytes abstracts the process of generating the bytes to be signed or verified.
func (a *WorkReport) computeWorkReportBytes() []byte {
	return append([]byte(X_G), common.ComputeHash(a.Bytes())...)
}

func (a *WorkReport) Sign(Ed25519Secret []byte) []byte {
	workReportBytes := a.computeWorkReportBytes()
	return ed25519.Sign(Ed25519Secret, workReportBytes)
}

func (a *WorkReport) ValidateSignature(publicKey []byte, signature []byte) error {
	workReportBytes := a.computeWorkReportBytes()

	if !ed25519.Verify(publicKey, workReportBytes, signature) {
		return errors.New("invalid signature")
	}

	return nil
}

// Bytes returns the bytes of the Assurance
func (a *WorkReport) Bytes() []byte {
	enc, err := json.Marshal(a)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (a *WorkReport) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}

func (s *SWorkReport) Deserialize() (CWorkReport, error) {
	package_spec := s.AvailabilitySpec
	context := s.RefineContext
	core_index := s.CoreIndex
	authorizer_hash := s.AuthorizerHash
	auth_output := common.FromHex(s.AuthOutput)
	results := make([]WorkResult, len(s.Results))
	for i, result := range s.Results {
		results[i] = result.Deserialize()
	}

	return CWorkReport{
		AvailabilitySpec: package_spec,
		RefineContext:    context,
		CoreIndex:        core_index,
		AuthorizerHash:   authorizer_hash,
		AuthOutput:       auth_output,
		Results:          results,
	}, nil
}
