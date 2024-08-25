package types

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
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
	ParentHash common.Hash `json:"parent_hash"`
	// f - 1 means "available"
	Bitstring      []byte           `json:"bitstring"`
	ValidatorIndex uint32           `json:"validator_index"`
	Signature      Ed25519Signature `json:"signature"`
}

// computeAssuranceBytes abstracts the process of generating the bytes to be signed or verified.
func (a *Assurance) computeAssuranceBytes() []byte {
	h := common.ComputeHash(append(a.ParentHash.Bytes(), a.Bitstring...))
	return append([]byte(X_A), h...)
}

func (a *Assurance) Sign(Ed25519Secret []byte, parentHash common.Hash) {
	assuranceBytes := a.computeAssuranceBytes()
	a.Signature = ed25519.Sign(Ed25519Secret, assuranceBytes)
}

func (a *Assurance) ValidateSignature(publicKey []byte) error {
	assuranceBytes := a.computeAssuranceBytes()

	if !ed25519.Verify(publicKey, assuranceBytes, a.Signature) {
		return errors.New("invalid signature")
	}
	return nil
}

func (a Assurance) DeepCopy() (Assurance, error) {
	var copiedAssurance Assurance

	// Serialize the original Assurance to JSON
	data, err := json.Marshal(a)
	if err != nil {
		return copiedAssurance, err
	}

	// Deserialize the JSON back into a new Assurance instance
	err = json.Unmarshal(data, &copiedAssurance)
	if err != nil {
		return copiedAssurance, err
	}

	return copiedAssurance, nil
}

// Bytes returns the bytes of the Assurance
func (a *Assurance) Bytes() []byte {
	enc, err := json.Marshal(a)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (a *Assurance) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}
