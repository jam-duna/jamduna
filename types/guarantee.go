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

// ToBytes serializes the GuaranteeCredential struct into a byte array
func (cred *GuaranteeCredential) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize ValidatorIndex (2 bytes)
	if err := binary.Write(buf, binary.BigEndian, cred.ValidatorIndex); err != nil {
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
	if err := binary.Read(buf, binary.BigEndian, &cred.ValidatorIndex); err != nil {
		return err
	}

	// Deserialize Signature (64 bytes)
	if _, err := io.ReadFull(buf, cred.Signature[:]); err != nil {
		return err
	}

	return nil
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
	// print json
	jsonBytes, err := json.Marshal(g.Report)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(jsonBytes))
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
	enc, err := Encode(g)
	if err != nil {
		return nil
	}
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
	// print json
	jsonBytes, err := json.Marshal(g.Report)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(string(jsonBytes))
	return signtext
}

func (g *Guarantee) UnsignedBytesWithSalt() []byte {
	signtext := g.UnsignedBytes()
	signtext = append([]byte(X_G), signtext...)
	return signtext
}

func (g *Guarantee) Verify(CurrV []Validator) error {
	signtext := g.UnsignedBytesWithSalt()
	emptyHash := common.Hash{}
	if g.Report.AvailabilitySpec.WorkPackageHash == emptyHash {
		return fmt.Errorf("WorkPackageHash is empty")
	}
	//verify the signature
	for _, i := range g.Signatures {
		//verify the signature
		// [i.ValidatorIndex].Ed25519
		validatorKey := CurrV[i.ValidatorIndex].GetEd25519Key()
		if !Ed25519Verify(validatorKey, signtext, i.Signature) {
			return fmt.Errorf("invalid signature in guarantee by validator %v", i.ValidatorIndex)
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
