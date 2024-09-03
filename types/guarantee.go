package types

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

/*
Section 11.4 - Work Report Guarantees. See Equations 136 - 143. The guarantees extrinsic, ${\bf E}_G$, a *series* of guarantees, at most one for each core, each of which is a tuple of:
* core index
* work-report
* $a$, credential
* $t$, its corresponding timeslot.

The core index of each guarantee must be unique and guarantees must be in ascending order of this.
*/

// Guaranteed Work Report`

type Guarantee struct {
	Report     WorkReport            `json:"report"`
	Slot       uint32                `json:"slot"`
	Signatures []GuaranteeCredential `json:"signatures"`
}

/*
Section 11.4 - Work Report Guarantees. See Equations 136 - 143. The guarantees extrinsic, ${\bf E}_G$, a *series* of guarantees, at most one for each core, each of which is a tuple of:
* core index
* work-report
* $a$, credential
* $t$, its corresponding timeslot.

The core index of each guarantee must be unique and guarantees must be in ascending order of this.
*/
// Credential represents a series of tuples of a signature and a validator index.
type GuaranteeCredential struct {
	ValidatorIndex uint16           `json:"validator_index"`
	Signature      Ed25519Signature `json:"signature"`
}

func (g Guarantee) DeepCopy() (Guarantee, error) {
	var copiedGuarantee Guarantee

	// Serialize the original Guarantee to JSON
	data, err := json.Marshal(g)
	if err != nil {
		return copiedGuarantee, err
	}

	// Deserialize the JSON back into a new Guarantee instance
	err = json.Unmarshal(data, &copiedGuarantee)
	if err != nil {
		return copiedGuarantee, err
	}

	return copiedGuarantee, nil
}

// Bytes returns the bytes of the Guarantee.
func (g *Guarantee) Bytes() []byte {
	enc, err := json.Marshal(g)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (g *Guarantee) Hash() common.Hash {
	data := g.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}

func (g *Guarantee) ValidateSignatures() error {
	return nil
}

type SGuarantee struct {
	Report     SWorkReport            `json:"report"`
	Slot       uint32                 `json:"slot"`
	Signatures []SGuaranteeCredential `json:"signatures"`
}

type SGuaranteeCredential struct {
	ValidatorIndex uint16 `json:"validator_index"`
	Signature      string `json:"signature"`
}

func (s *SGuarantee) Deserialize() (Guarantee, error) {
	report, err := s.Report.Deserialize()
	if err != nil {
		return Guarantee{}, err
	}

	signatures := make([]GuaranteeCredential, len(s.Signatures))
	for i, signature := range s.Signatures {
		signatures[i] = signature.Deserialize()
	}

	return Guarantee{
		Report:     report,
		Slot:       s.Slot,
		Signatures: signatures,
	}, nil
}

func (s *SGuaranteeCredential) Deserialize() GuaranteeCredential {
	signatureBytes := common.FromHex(s.Signature)
	var signature Ed25519Signature
	copy(signature[:], signatureBytes)

	return GuaranteeCredential{
		ValidatorIndex: s.ValidatorIndex,
		Signature:      signature,
	}
}
