package types

import (
	"encoding/json"
	"fmt"
	"reflect"

	"bytes"
	"encoding/binary"
	"io"

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

type GuaranteeHashed struct {
	Report     common.Hash           `json:"report"`
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

// ToBytes serializes the GuaranteeCredential struct into a byte array
func (cred *GuaranteeCredential) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize ValidatorIndex (2 bytes)
	if err := binary.Write(buf, binary.LittleEndian, cred.ValidatorIndex); err != nil {
		return nil, err
	}

	// Serialize Signature (64 bytes for Ed25519Signature)
	if _, err := buf.Write(cred.Signature[:]); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a GuaranteeCredential struct
func (cred *GuaranteeCredential) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize ValidatorIndex (2 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &cred.ValidatorIndex); err != nil {
		return err
	}

	// Deserialize Signature (64 bytes)
	if _, err := io.ReadFull(buf, cred.Signature[:]); err != nil {
		return err
	}

	return nil
}

func (g Guarantee) DeepCopy() (Guarantee, error) {
	var copiedGuarantee Guarantee

	// Serialize the original Guarantee to JSON
	data, err := Encode(g)
	if err != nil {
		return copiedGuarantee, err
	}

	// Deserialize the JSON back into a new Guarantee instance
	decoded, _, err := Decode(data, reflect.TypeOf(Guarantee{}))
	if err != nil {
		return copiedGuarantee, err
	}
	copiedGuarantee = decoded.(Guarantee)

	return copiedGuarantee, nil
}

// Bytes returns the bytes of the Guarantee.
func (g *Guarantee) Bytes() []byte {
	enc, err := Encode(g)
	if err != nil {
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
	return common.Blake2Hash(data)
}

func (g *GuaranteeHashed) Bytes() []byte {
	enc, err := Encode(g)
	if err != nil {
		return nil
	}
	return enc
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

func (g *Guarantee) Verify(CurrV []Validator) error {
	signtext := g.Report.computeWorkReportBytes()
	//fmt.Printf("[guarantee:Verify] Verifying Guarantee %s\nSigntext:[%s]\n", g.String(), common.Bytes2Hex(signtext))
	//verify the signature
	numErrors := 0
	for _, i := range g.Signatures {
		//verify the signature
		// [i.ValidatorIndex].Ed25519
		validatorKey := CurrV[i.ValidatorIndex].GetEd25519Key()
		if !Ed25519Verify(validatorKey, signtext, i.Signature) {
			numErrors++
			fmt.Printf("[guarantee:Verify] ERR %d invalid signature in guarantee by validator %v [PubKey: %s]\n", numErrors, i.ValidatorIndex, common.Bytes2Hex(validatorKey[:]))
			fmt.Printf("work report hash : %s\n", g.Report.Hash().String())
			fmt.Printf("sign salt : %s\n", common.Bytes2Hex(signtext))
			fmt.Printf("signature : %s\n", common.Bytes2Hex(i.Signature[:]))
		}
	}
	if numErrors > 0 {
		return fmt.Errorf("%s", "invalid signature")
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
func (g *Guarantee) String() string {
	enc, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		// Handle the error according to your needs.
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}

// helper function to convert Gurantee to GuaranteeHashed
func (g *Guarantee) ToGuaranteeHashed() GuaranteeHashed {
	return GuaranteeHashed{
		Report:     g.Report.Hash(),
		Slot:       g.Slot,
		Signatures: g.Signatures,
	}
}
