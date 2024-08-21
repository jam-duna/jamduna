package types

import (
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
//Validators, in their role as availability assurers, should index such chunks according to the index of the segmentstree whose reconstruction they facilitate. Since the data for segment chunks is so small at 12 bytes, fixed communications costs should be kept to a bare minimum. A good network protocol (out of scope at present) will allow guarantors to specify only the segments-tree root and index together with a Boolean to indicate whether the proof chunk need be supplied.

// Assurance represents an assurance value.
type Assurance struct {
	// H_p - see Eq 124
	ParentHash common.Hash `json:"parent_hash"`
	// f - 1 means "available"
	Bitstring      []byte           `json:"bitstring"`
	ValidatorIndex int              `json:"validator_index"`
	Signature      Ed25519Signature `json:"signature"`
}
