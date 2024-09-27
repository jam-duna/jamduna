package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

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

type GuaranteeReport struct {
	Report              WorkReport          `json:"report"`
	GuaranteeCredential GuaranteeCredential `json:"guarantee_credential"`
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

func (g *GuaranteeReport) Sign(secret []byte) error {
	signtext := g.UnsignedBytesWithSalt()
	if len(secret) != 64 {
		return fmt.Errorf("secret length is not 64")
	}
	sig := Ed25519SignByBytes(secret, signtext)
	g.GuaranteeCredential.Signature = sig
	return nil
}

func (g *GuaranteeReport) UnsignedBytes() []byte {
	signtext := g.Report.Hash().Bytes()
	return signtext
}

func (g *GuaranteeReport) UnsignedBytesWithSalt() []byte {
	signtext := g.UnsignedBytes()
	signtext = append([]byte(X_G), signtext...)
	return signtext
}

func (g *GuaranteeReport) Verify(key Ed25519Key) bool {
	signtext := g.UnsignedBytesWithSalt()
	boo := Ed25519Verify(key, signtext, g.GuaranteeCredential.Signature)
	return boo
}

func (g *GuaranteeReport) DeepCopy() (GuaranteeReport, error) {
	var copiedGuarantee GuaranteeReport

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

func (g *GuaranteeReport) Bytes() []byte {
	enc := Encode(g)
	return enc
}

func BytesToGuaranteeReport(data []byte) (GuaranteeReport, error) {
	var copiedGuarantee GuaranteeReport
	err := json.Unmarshal(data, &copiedGuarantee)
	if err != nil {
		return copiedGuarantee, err
	}
	return copiedGuarantee, nil
}

func (g Guarantee) DeepCopy() (Guarantee, error) {
	var copiedGuarantee Guarantee

	// Serialize the original Guarantee to JSON
	data := Encode(g)

	// Deserialize the JSON back into a new Guarantee instance
	decoded, _ := Decode(data, reflect.TypeOf(Guarantee{}))
	copiedGuarantee = decoded.(Guarantee)

	return copiedGuarantee, nil
}

// Bytes returns the bytes of the Guarantee.
func (g *Guarantee) Bytes() []byte {
	enc := Encode(g)
	return enc
}

func (g *Guarantee) Hash() common.Hash {
	data := g.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.Blake2Hash(data)
}

func (g *GuaranteeCredential) UnmarshalJSON(data []byte) error {
	type Alias GuaranteeCredential
	aux := &struct {
		*Alias
		Signature string `json:"signature"`
	}{
		Alias: (*Alias)(g),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	sigBytes := common.FromHex(aux.Signature)
	copy(g.Signature[:], sigBytes)
	return nil
}

func (g *Guarantee) UnsignedBytes() []byte {
	signtext := g.Report.Hash().Bytes()
	return signtext
}

func (g *Guarantee) UnsignedBytesWithSalt() []byte {
	signtext := g.UnsignedBytes()
	signtext = append([]byte(X_G), signtext...)
	return signtext
}

func (g *Guarantee) Verify(CurrV []Validator) error {
	signtext := g.UnsignedBytesWithSalt()

	for _, i := range g.Signatures {
		//verify the signature
		// [i.ValidatorIndex].Ed25519
		fmt.Println("Process Verify EG")
		fmt.Printf("Validator Index: %v, Validator Key: %v\n", i.ValidatorIndex, CurrV[i.ValidatorIndex].Ed25519)
		if !Ed25519Verify(Ed25519Key(CurrV[i.ValidatorIndex].Ed25519), signtext, i.Signature) {
			return errors.New(fmt.Sprintf("invalid signature in guarantee by validator %v", i.ValidatorIndex))
		}

	}
	return nil
}

func (g GuaranteeCredential) MarshalJSON() ([]byte, error) {
	type Alias GuaranteeCredential
	sig := common.HexString(g.Signature[:])
	return json.Marshal(&struct {
		*Alias
		Signature string `json:"signature"`
	}{
		Alias:     (*Alias)(&g),
		Signature: sig,
	})
}

// helper function to print the Guarantee
func (g *Guarantee) Print() {
	fmt.Println("Guarantee:")
	fmt.Println("= Report:")
	g.Report.Print()
	fmt.Println("= Slot:", g.Slot)
	fmt.Println("= Signatures:")
	for _, signature := range g.Signatures {
		fmt.Printf("== ValidatorIndex: %d\n", signature.ValidatorIndex)
		fmt.Printf("== Signature: %x\n", signature.Signature)
	}
}
