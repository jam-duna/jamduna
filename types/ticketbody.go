package types

import (
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

// TicketBody represents the structure of a ticket
type TicketBody struct {
	Id      common.Hash `json:"id"`
	Attempt uint8       `json:"attempt"`
}

type TicketsMark [EpochLength]TicketBody

func (T *TicketsMark) Encode() []byte {
	if T == nil {
		return []byte{0}
	}
	v := reflect.ValueOf(*T)
	encoded := []byte{1}
	for i := 0; i < v.Len(); i++ {
		encoded = append(encoded, Encode(v.Index(i).Interface())...)
	}
	return encoded
}

func (target *TicketsMark) Decode(data []byte) (interface{}, uint32) {
	if data[0] == 0 {
		return nil, 1
	}
	var decoded TicketsMark
	length := uint32(1)
	for i := 0; i < EpochLength; i++ {
		elem, l := Decode(data[length:], reflect.TypeOf(TicketBody{}))
		decoded[i] = elem.(TicketBody)
		length += l
	}
	return &decoded, length
}
