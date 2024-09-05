package types

import (
	"github.com/colorfulnotion/jam/common"
	//"encoding/json"
	"reflect"
)

// TicketBody represents the structure of a ticket
type TicketBody struct {
	Id      common.Hash `json:"id"`
	Attempt uint8       `json:"attempt"`
}

type TicketsMark [EpochLength]TicketBody

func (T TicketsMark) Encode() []byte {
	v := reflect.ValueOf(T)
	encoded := []byte{1}
	for i := 0; i < v.Len(); i++ {
		encoded = append(encoded, Encode(v.Index(i).Interface())...)
	}
	return encoded
}

func TicketsMarkDecode(data []byte, t reflect.Type) (interface{}, uint32) {
	v:= reflect.New(t).Elem()
	length := uint32(1)
	for i := 0; i < EpochLength; i++ {
		elem, l := Decode(data[length:], v.Index(i).Type())
		v.Index(i).Set(reflect.ValueOf(elem))
		length += l
	}
	return v.Interface(), length
}

func (T TicketsMark) AsTicketBody() [12]*TicketBody {
	tickets := [12]*TicketBody{}
	for idx, t := range T {
		tickets[idx] = &t
	}
	return tickets
}
