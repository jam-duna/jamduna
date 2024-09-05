package types

import (
	"reflect"

	"github.com/colorfulnotion/jam/common"
	//"encoding/json"
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

func EpochMarkDecode(data []byte, t reflect.Type) (interface{}, uint32) {
	v:= reflect.New(t).Elem()
	length := uint32(1)
	decoded , l := Decode(data[length:], reflect.TypeOf(common.Hash{}))
	v.Field(0).Set(reflect.ValueOf(decoded))
	length += l
	decoded , l = Decode(data[length:], reflect.TypeOf([TotalValidators]common.Hash{}))
	v.Field(1).Set(reflect.ValueOf(decoded))
	length += l
	return v.Interface(), length
}