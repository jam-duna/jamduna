package types

import (
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"reflect"
)

// EpochMark (see 6.4 Epoch change Signal) represents the descriptor for parameters to be used in the next epoch
type EpochMark struct {
	// Randomness accumulator snapshot
	Entropy        common.Hash `json:"entropy"`
	TicketsEntropy common.Hash `json:"tickets_entropy"`
	// List of authorities scheduled for next epoch
	Validators [TotalValidators]common.Hash `json:"validators"` //bandersnatch keys
}

func (e EpochMark) String() string {
	entropyShort := common.Str(e.Entropy)
	ticketsEntropyShort := common.Str(e.TicketsEntropy)
	/*validatorsShort := ""
	q := ""
	for _, validator := range e.Validators {
		validatorsShort += q + common.Str(validator)
		q = ", "
	} */
	return fmt.Sprintf(" EpochMarker(η1'=%s, η2'=%s)", entropyShort, ticketsEntropyShort) // validatorsShort
}

func (E *EpochMark) Encode() []byte {
	if E == nil {
		return []byte{0}
	}
	encoded := []byte{1}
	encodedEpochMark, err := Encode(*E)
	if err != nil {
		return []byte{}
	}
	encoded = append(encoded, encodedEpochMark...)
	return encoded
}

func (target *EpochMark) Decode(data []byte) (interface{}, uint32) {
	if data[0] == 0 {
		return nil, 1
	}
	length := uint32(1)
	entropy, l, err := Decode(data[length:], reflect.TypeOf(common.Hash{}))
	if err != nil {
		return nil, length
	}
	length += l
	ticketsentropy, l, err := Decode(data[length:], reflect.TypeOf(common.Hash{}))
	if err != nil {
		return nil, length
	}
	length += l
	validators, l, err := Decode(data[length:], reflect.TypeOf([TotalValidators]common.Hash{}))
	if err != nil {
		return nil, length
	}
	decoded := EpochMark{
		Entropy:        entropy.(common.Hash),
		TicketsEntropy: ticketsentropy.(common.Hash),
		Validators:     validators.([TotalValidators]common.Hash),
	}
	length += l
	return &decoded, length
}
