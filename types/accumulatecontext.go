package types

import (
	"fmt"

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
	D                  map[uint32]*ServiceAccount `json:"D"`
	UpcomingValidators Validators                 `json:"upcoming_validators"`
	QueueWorkReport    AuthorizationQueue         `json:"authorizations_pool"`
	PrivilegedState    Kai_state                  `json:"privileged_state"`
}

func (U PartialState) Dump(prefix string, id uint16) {
	fmt.Printf("[N%d] Partial State Dump -- %s\n", id, prefix)
	for serviceIndex, serviceAccount := range U.D {
		fmt.Printf("[N%d] Service %d => Dirty %v %s\n", id, serviceIndex, serviceAccount.Dirty, serviceAccount.String())
	}
	fmt.Printf("[N%d]\n\n", id)
}

type XContext struct {
	D map[uint32]*ServiceAccount `json:"D"`
	I uint32                     `json:"I"`
	S uint32                     `json:"S"`
	U *PartialState              `json:"U"`
	T []DeferredTransfer         `json:"T"`
}

// returns back X.U.D[s] but if there is a miss, uses
func (X *XContext) GetMutableServiceAccount(serviceIndex uint32, hostenv HostEnv) (xs *ServiceAccount, ok bool) {
	if account, exists := X.U.D[serviceIndex]; exists {
		return account, true
	}

	// If not found, get it from the host environment
	service, err := hostenv.GetService(serviceIndex)
	if err != nil {
		return nil, false
	}
	X.D[serviceIndex] = service.Clone() // doesn't change
	if X.U.D == nil {
		X.U.D = make(map[uint32]*ServiceAccount)
	}
	X.U.D[serviceIndex] = service.Clone() // changes
	return X.U.D[serviceIndex], true

}
func (X *XContext) Clone() (Y XContext) {
	Y = XContext{
		D: make(map[uint32]*ServiceAccount),
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
