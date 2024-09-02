package types

import (
	//"fmt"

	"github.com/colorfulnotion/jam/common"
)

const (
	x_s = "S"
	x_c = "C"
	x_v = "V"
	x_i = "I"
	x_t = "T"
	x_n = "N"
	x_p = "P"
)

type XContext struct {
	S *ServiceAccount
	C [TotalCores][MaxAuthorizationQueueItems]common.Hash
	V []Validator
	I uint32
	T []*AddTransfer
	N map[uint32]*ServiceAccount
	P *Empower
}

func NewXContext() *XContext {
	//TODO
	x := &XContext{
		N: make(map[uint32]*ServiceAccount, 0),
	}
	return x
}

func (x *XContext) GetX_s_ServiceIndex() uint32 {
	sa := x.GetX_s()
	return sa.ServiceIndex()
}

func (x *XContext) GetX_s() *ServiceAccount {
	return x.S
}

func (x *XContext) SetX_s(s *ServiceAccount) {
	x.S = s
}

func (x *XContext) SetX_i(i uint32) {
	x.I = i
}

func (x *XContext) GetX_i() uint32 {
	return x.I
}

func (x *XContext) SetX_v(v []Validator) {
	x.V = v
}

func (x *XContext) SetX_t(t *AddTransfer) {
	x.T = append(x.T, t)
}

func (x *XContext) SetX_n(service_index uint32, sa *ServiceAccount) {
	x.N[service_index] = sa
}

func (x *XContext) GetX_n(service_index uint32) (bool, *ServiceAccount) {
	sa, exist := x.N[service_index]
	if !exist {
		return false, nil
	}
	return true, sa
}

func (x *XContext) Set_p(p *Empower) {
	x.P = p
}

func (x *XContext) Get_p() *Empower {
	return x.P
}
