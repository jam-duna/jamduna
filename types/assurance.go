package types

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

/*
11.2.1. The Assurances Extrinsic. ${\bf E}_A$  The assurances extrinsic is a *sequence* of assurance values, at most one per validator. Each assurance:
* is a sequence of binary values (i.e. a bitstring), one per core, together with
* a signature and
* the index of the validator who is assuring

A value of 1 (or ⊺, if interpreted as a Boolean) at any given index implies that the validator assures they are contributing to its availability.  See equations 123-128.

`Assurance` ${\bf E}_A$:
*/
//Validators, in their role as availability assurers, should index such chunks according to the index of the segmentstree whose reconstruction they facilitate.
// Since the data for segment chunks is so small at 12 bytes, fixed communications costs should be kept to a bare minimum.
// A good network protocol (out of scope at present) will allow guarantors to specify only the segments-tree root and index together with a Boolean
// to indicate whether the proof chunk need be supplied.

// Assurance represents an assurance value.
type Assurance struct {
	// H_p - see Eq 124
	Anchor common.Hash `json:"anchor"`
	// f - 1 means "available"
	Bitfield       [Avail_bitfield_bytes]byte `json:"bitfield"`
	ValidatorIndex uint16                     `json:"validator_index"`
	Signature      Ed25519Signature           `json:"signature"`
}

func (a *Assurance) Bytes() []byte {
	enc, err := Encode(a)
	if err != nil {
		return nil
	}
	return enc
}

func (a *Assurance) Hash() common.Hash {
	data := a.Bytes()
	return common.Blake2Hash(data)
}

func (A *Assurance) BitFieldToBytes() []byte {
	return A.Bitfield[:]
}

func (A *Assurance) SetBitFied_Bit(index uint16, value bool) {
	if value {
		A.Bitfield[0] |= 1 << index
	} else {
		A.Bitfield[0] &= ^(1 << index)
	}
}

func (A *Assurance) GetBitFied_Bit(index uint16) bool {
	return (A.Bitfield[0] & (1 << index)) != 0
}

func (a *Assurance) UnsignedBytes() []byte {
	signtext := common.ComputeHash(append(a.Anchor.Bytes(), a.Bitfield[0])) // this bitfield is a byte, TODO: check if this is correct
	return signtext
}

// computeAssuranceBytes abstracts the process of generating the bytes to be signed or verified.
func (a *Assurance) UnsignedBytesWithSalt() []byte {
	signtext := a.UnsignedBytes()
	return append([]byte(X_A), signtext...)
}

func (a *Assurance) Sign(Ed25519Secret []byte) {
	assuranceBytes := a.UnsignedBytesWithSalt()
	sig := ed25519.Sign(Ed25519Secret, assuranceBytes)
	copy(a.Signature[:], sig)
}

func (a *Assurance) Verify(parent common.Hash, validator Validator) error {
	if len(a.Signature) == 0 {
		return errors.New("signature is empty")
	}
	signtext := a.UnsignedBytesWithSalt()
	//verify the signature
	if !Ed25519Verify(Ed25519Key(validator.Ed25519), signtext, a.Signature) {
		return errors.New("invalid signature")
	}
	return nil
}

func (a Assurance) DeepCopy() (Assurance, error) {
	var copiedAssurance Assurance

	// Serialize the original Assurance to JSON
	data, err := Encode(a)
	if err != nil {
		return copiedAssurance, err
	}

	// Deserialize the JSON back into a new Assurance instance
	decoded, _, err := Decode(data, reflect.TypeOf(Assurance{}))
	if err != nil {
		return copiedAssurance, err
	}
	copiedAssurance = decoded.(Assurance)

	return copiedAssurance, nil
}

// Bytes returns the bytes of the Assurance
// func (a *Assurance) Bytes() []byte {
// 	enc := Encode(a)
// 	return enc
// }

// func (a *Assurance) Hash() common.Hash {
// 	data := a.Bytes()
// 	if data == nil {
// 		// Handle the error case
// 		return common.Hash{}
// 	}
// 	return common.Blake2Hash(data)
// }

// Create a object for save the availibility

type IsPackageRecieved struct {
	WorkReportBundle bool
	ExportedSegments bool
}

func (a *Assurance) UnmarshalJSON(data []byte) error {
	var s struct {
		Anchor         common.Hash `json:"anchor"`
		Bitfield       string      `json:"bitfield"`
		ValidatorIndex uint16      `json:"validator_index"`
		Signature      string      `json:"signature"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	// Convert Bitstring from hex string to []byte
	bitfieldBytes := common.FromHex(s.Bitfield)
	// Convert Signature from hex string to Ed25519Signature
	signatureBytes := common.FromHex(s.Signature)
	var bitfield [Avail_bitfield_bytes]byte
	copy(bitfield[:], bitfieldBytes)

	var signature Ed25519Signature
	copy(signature[:], signatureBytes)

	// Return the converted Assurance struct
	a.Anchor = s.Anchor
	a.Bitfield = bitfield
	a.ValidatorIndex = s.ValidatorIndex
	a.Signature = signature

	return nil
}

func (a Assurance) MarshalJSON() ([]byte, error) {
	// Convert Bitfield from []byte to hex string
	bitfield := common.HexString(a.Bitfield[:])
	// Convert Signature from Ed25519Signature to hex string
	signature := common.HexString(a.Signature[:])

	// Create a struct to hold the converted Assurance struct
	s := struct {
		Anchor         common.Hash `json:"anchor"`
		Bitfield       string      `json:"bitfield"`
		ValidatorIndex uint16      `json:"validator_index"`
		Signature      string      `json:"signature"`
	}{
		Anchor:         a.Anchor,
		Bitfield:       bitfield,
		ValidatorIndex: a.ValidatorIndex,
		Signature:      signature,
	}

	// Marshal the struct to JSON
	return json.Marshal(&s)
}
