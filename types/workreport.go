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
	AvailabilitySpec  AvailabilitySpecifier `json:"package_spec"`
	RefineContext     RefineContext         `json:"context"`
	CoreIndex         uint16                `json:"core_index"`
	AuthorizerHash    common.Hash           `json:"authorizer_hash"`
	AuthOutput        []byte                `json:"auth_output"`
	SegmentRootLookup SegmentRootLookup     `json:"segment_root_lookup"`
	Results           []WorkResult          `json:"results"`
}

// Temporarily ignore encoding.
type SegmentRootLookup map[common.Hash]common.Hash

func (S SegmentRootLookup) Encode() []byte {
	encoded := []byte{}
	return encoded
}

func (S SegmentRootLookup) Decode(data []byte) (interface{}, uint32) {
	return nil, 0
}

// MarshalJSON serializes SegmentRootLookup as a map[string]string
func (s SegmentRootLookup) MarshalJSON() ([]byte, error) {
	stringMap := make(map[string]string)
	for k, v := range s {
		stringMap[k.Hex()] = v.Hex() // Assume `Hex()` returns a string representation of `common.Hash`
	}
	return json.Marshal(stringMap)
}

// UnmarshalJSON deserializes SegmentRootLookup from a map[string]string
func (s *SegmentRootLookup) UnmarshalJSON(data []byte) error {
	stringMap := make(map[string]string)
	if err := json.Unmarshal(data, &stringMap); err != nil {
		return err
	}

	*s = make(SegmentRootLookup)
	for k, v := range stringMap {
		keyHash := common.HexToHash(k) // Assume `HexToHash()` converts a string to `common.Hash`
		valueHash := common.HexToHash(v)
		(*s)[keyHash] = valueHash
	}
	return nil
}

// eq 190
type WorkReportNeedAudit struct {
	Q [TotalCores]WorkReport `json:"available_work_report"`
}

type WorkReportSelection struct {
	WorkReport WorkReport `json:"work_report"`
	Core       uint16     `json:"core_index"`
}

func (a *WorkReport) GetWorkPackageHash() common.Hash {
	return a.AvailabilitySpec.WorkPackageHash
}

// computeWorkReportBytes abstracts the process of generating the bytes to be signed or verified.
func (a *WorkReport) computeWorkReportBytes() []byte {
	return append([]byte(X_G), common.ComputeHash(a.Bytes())...)
}

func ComputeWorkReportSignBytesWithHash(a common.Hash) []byte {
	return append([]byte(X_G), a.Bytes()...)
}

func (a *WorkReport) Sign(Ed25519Secret []byte, validatorIndex uint16) (gc GuaranteeCredential) {
	gc.ValidatorIndex = validatorIndex
	sig := ed25519.Sign(Ed25519Secret, a.computeWorkReportBytes())
	copy(gc.Signature[:], sig[:])
	return gc
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
	enc, err := Encode(a)
	if err != nil {
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
	return common.Blake2Hash(data)
}

func (a *WorkReport) UnmarshalJSON(data []byte) error {
	var s struct {
		AvailabilitySpec AvailabilitySpecifier `json:"package_spec"`
		RefineContext    RefineContext         `json:"context"`
		CoreIndex        uint16                `json:"core_index"`
		AuthorizerHash   common.Hash           `json:"authorizer_hash"`
		AuthOutput       string                `json:"auth_output"`
		Results          []WorkResult          `json:"results"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	a.AvailabilitySpec = s.AvailabilitySpec
	a.RefineContext = s.RefineContext
	a.CoreIndex = s.CoreIndex
	a.AuthorizerHash = s.AuthorizerHash
	a.AuthOutput = common.FromHex(s.AuthOutput)
	a.Results = s.Results
	return nil
}

func (a WorkReport) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		AvailabilitySpec AvailabilitySpecifier `json:"package_spec"`
		RefineContext    RefineContext         `json:"context"`
		CoreIndex        uint16                `json:"core_index"`
		AuthorizerHash   common.Hash           `json:"authorizer_hash"`
		AuthOutput       string                `json:"auth_output"`
		Results          []WorkResult          `json:"results"`
	}{
		AvailabilitySpec: a.AvailabilitySpec,
		RefineContext:    a.RefineContext,
		CoreIndex:        a.CoreIndex,
		AuthorizerHash:   a.AuthorizerHash,
		AuthOutput:       common.HexString(a.AuthOutput),
		Results:          a.Results,
	})
}

// helper function to print the WorkReport
func (a *WorkReport) String() string {
	enc, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		// Handle the error according to your needs.
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}
