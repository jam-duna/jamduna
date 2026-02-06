package types

import (
	"reflect"

	"github.com/jam-duna/jamduna/common"
)

// TicketBody represents the structure of a ticket
// 6.6 C.30
type TicketBody struct {
	Id      common.Hash `json:"id"`      // y
	Attempt uint8       `json:"attempt"` // e
}

type TicketsMark [EpochLength]TicketBody

func (T *TicketsMark) Encode() []byte {
	if T == nil {
		return []byte{0}
	}
	v := reflect.ValueOf(*T)
	encoded := []byte{1}
	for i := 0; i < v.Len(); i++ {
		encodedV, err := Encode(v.Index(i).Interface())
		if err != nil {
			return []byte{}
		}
		encoded = append(encoded, encodedV...)
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
		elem, l, err := Decode(data[length:], reflect.TypeOf(TicketBody{}))
		if err != nil {
			return nil, length
		}
		decoded[i] = elem.(TicketBody)
		length += l
	}
	return &decoded, length
}
