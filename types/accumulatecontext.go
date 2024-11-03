package types

import (
	"github.com/colorfulnotion/jam/common"
)

type AccumulationQueue struct {
	WorkReports     []WorkReport  `json:"work_reports"`
	WorkPackageHash []common.Hash `json:"work_package_hash"`
}

type AccumulationHistory struct {
	WorkPackageHash []common.Hash `json:"work_package_hash"`
}

// Types for Kai
type Kai_state struct {
	Kai_m uint32            `json:"chi_m"` // The index of the empower service
	Kai_a uint32            `json:"chi_a"` // The index of the designate service
	Kai_v uint32            `json:"chi_v"` // The index of the assign service
	Kai_g map[uint32]uint32 `json:"chi_g"` // g is a small dictionary containing the indices of services which automatically accumulate in each block together with a basic amount of gas with which each accumulates
}

type AuthorizationQueue [TotalCores][]common.Hash

// U: The set of partial state, used during accumulation. See equation 170.
type PartialState struct {
	D                  map[uint32]ServiceAccount `json:"service_account"`
	UpcomingValidators Validators                `json:"upcoming_validators"`
	QueueWorkReport    AuthorizationQueue        `json:"authorizations_pool"`
	PrivilegedState    Kai_state                 `json:"privileged_state"`
}

type XContext struct {
	D map[uint32]ServiceAccount
	I uint32
	S uint32
	U *PartialState
	T []DeferredTransfer
}

func (X *XContext) Clone() (Y XContext) {
	Y = XContext{
		D: make(map[uint32]ServiceAccount),
		I: X.I,
		S: X.S,
		T: make([]DeferredTransfer, len(X.T)),
	}
	for serviceIndex, service := range X.D {
		Y.D[serviceIndex] = service.Clone()
	}
	for i, t := range X.T {
		Y.T[i] = t.Clone()
	}
	return
}

// T: The set of deferred transfers.
type DeferredTransfer struct {
	SenderIndex   uint32    `json:"sender_index"`
	ReceiverIndex uint32    `json:"receiver_index"`
	Amount        uint64    `json:"amount"`
	Memo          [128]byte `json:"memo"`
	GasLimit      uint64    `json:"gas_limit"`
}

func (d DeferredTransfer) Clone() DeferredTransfer {
	return DeferredTransfer{
		SenderIndex:   d.SenderIndex,
		ReceiverIndex: d.ReceiverIndex,
		Amount:        d.Amount,
		Memo:          d.Memo, // Arrays are copied by value in Go, so this will be a deep copy
		GasLimit:      d.GasLimit,
	}
}

// wrangled operand tuples,
// 175
// O: The accumulation operand element, corresponding to a single work result.
type AccumulateOperandElements struct {
	Results         Result      `json:"results"` //o
	Payload         common.Hash `json:"lookup"`  //l
	WorkPackageHash common.Hash `json:"k"`       //k
	AuthOutput      []byte      `json:"a"`       //a
}

func (a *DeferredTransfer) Bytes() []byte {
	enc, err := Encode(a)
	if err != nil {
		return nil
	}
	return enc
}
