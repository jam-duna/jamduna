package types

import (
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

// EpochMark (see 6.4 Epoch change Signal) represents the descriptor for parameters to be used in the next epoch
type EpochMark struct {
	// Randomness accumulator snapshot
	Entropy common.Hash `json:"entropy"`
	// List of authorities scheduled for next epoch
	Validators [TotalValidators]common.Hash `json:"validators"` //bandersnatch keys
}

func (E EpochMark) Encode() []byte {
	encoded := []byte{1}
	encoded = append(encoded, Encode(E.Entropy)...)
	encoded = append(encoded, Encode(E.Validators)...)
	return encoded
}

func (target EpochMark) Decode(data []byte) (interface{}, uint32) {
	length := uint32(1)
	entropy, l := Decode(data[length:], reflect.TypeOf(common.Hash{}))
	length += l
	validators, l := Decode(data[length:], reflect.TypeOf([TotalValidators]common.Hash{}))
	decoded := EpochMark{
		Entropy:    entropy.(common.Hash),
		Validators: validators.([TotalValidators]common.Hash),
	}
	length += l
	return decoded, length
}
