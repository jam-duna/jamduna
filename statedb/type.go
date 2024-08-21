package statedb

import (
	"github.com/colorfulnotion/jam/common"
)

type Ed25519Signature []byte
type PublicKey []byte
type BMTProof []common.Hash

const (
	JamCommonEra = 1704100800 //1200 UTC on January 1, 2024
)

const (
	validatorCount = 1023
	coreCount      = 341
	epochLength    = 600
	rotationPeriod = 10
)

type SafroleAccumulator struct {
	Id      common.Hash `json:"id"`
	Attempt int         `json:"attempt"`
}
