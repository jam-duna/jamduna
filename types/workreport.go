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
	// enc, err := Encode(a)
	// if err != nil {
	// 	return nil
	// }
	// return enc

	// use json for now, codec probably has a bug
	enc, err := json.Marshal(a)
	if err != nil {
		return nil
	}
	return enc
}

func (a *WorkReport) FromBytes(data []byte) error {
	var temp WorkReport
	err := json.Unmarshal(data, &temp)
	if err != nil {
		return err
	}
	*a = temp
	emptyHash := common.Hash{}
	if a.AvailabilitySpec.WorkPackageHash == emptyHash {
		return errors.New("WorkPackageHash is empty")
	}

	a.Results[0].Result = Result{}

	return nil
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
func (a *WorkReport) Print() {
	// type WorkReport struct {
	// 	AvailabilitySpec AvailabilitySpecifier `json:"package_spec"`
	// 	RefineContext    RefineContext         `json:"context"`
	// 	CoreIndex        uint16                `json:"core_index"`
	// 	AuthorizerHash   common.Hash           `json:"authorizer_hash"`
	// 	AuthOutput       []byte                `json:"auth_output"`
	// 	Results          []WorkResult          `json:"results"`
	// }

	fmt.Println("WorkReport:")
	fmt.Println("= AvailabilitySpec:")
	fmt.Println("== WorkPackageHash:", a.AvailabilitySpec.WorkPackageHash)
	fmt.Println("== BundleLength:", a.AvailabilitySpec.BundleLength)
	fmt.Println("== ErasureRoot:", a.AvailabilitySpec.ErasureRoot)
	fmt.Println("== ExportedSegmentRoot:", a.AvailabilitySpec.ExportedSegmentRoot)
	fmt.Println("= RefineContext:")
	// Anchor           common.Hash   `json:"anchor"`
	// StateRoot        common.Hash   `json:"state_root"`
	// BeefyRoot        common.Hash   `json:"beefy_root"`
	// LookupAnchor     common.Hash   `json:"lookup_anchor"`
	// LookupAnchorSlot uint32        `json:"lookup_anchor_slot"`
	// Prerequisite     *Prerequisite `json:"prerequisite,omitempty"`
	fmt.Println("== Anchor:", a.RefineContext.Anchor)
	fmt.Println("== StateRoot:", a.RefineContext.StateRoot)
	fmt.Println("== BeefyRoot:", a.RefineContext.BeefyRoot)
	fmt.Println("== LookupAnchor:", a.RefineContext.LookupAnchor)
	fmt.Println("== LookupAnchorSlot:", a.RefineContext.LookupAnchorSlot)
	fmt.Println("== Prerequisite:", a.RefineContext.Prerequisite)
	fmt.Println("= CoreIndex:", a.CoreIndex)
	fmt.Println("= AuthorizerHash:", a.AuthorizerHash)
	fmt.Println("= AuthOutput:", a.AuthOutput)
	fmt.Println("= Results:")
	// Service     uint32      `json:"service"`
	// CodeHash    common.Hash `json:"code_hash"`
	// PayloadHash common.Hash `json:"payload_hash"`
	// GasRatio    uint64      `json:"gas_ratio"`
	// Result      Result      `json:"result"`
	for i, result := range a.Results {
		fmt.Println("== Result", i)
		fmt.Println("=== Service:", result.Service)
		fmt.Println("=== CodeHash:", result.CodeHash)
		fmt.Println("=== PayloadHash:", result.PayloadHash)
		fmt.Println("=== GasRatio:", result.GasRatio)
		fmt.Println("=== Result:")
		// Ok  []byte `json:"ok,omitempty"`
		// Err uint8  `json:"err,omitempty"`
		fmt.Println("==== Ok:", result.Result.Ok)
		fmt.Println("==== Err:", result.Result.Err)
	}
}
