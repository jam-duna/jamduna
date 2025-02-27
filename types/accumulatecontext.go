package types

import (
	"encoding/json"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

type AccumulationQueue struct {
	WorkReport      WorkReport    `json:"report"`
	WorkPackageHash []common.Hash `json:"dependencies"`
}

type AccumulationHistory struct {
	WorkPackageHash []common.Hash `json:"-"`
}

// Types for Kai
type Kai_state struct {
	Kai_m uint32            `json:"chi_m"` // The index of the bless service
	Kai_a uint32            `json:"chi_a"` // The index of the designate service
	Kai_v uint32            `json:"chi_v"` // The index of the assign service
	Kai_g map[uint32]uint64 `json:"chi_g"` // g is a small dictionary containing the indices of services which automatically accumulate in each block together with a basic amount of gas with which each accumulates
}

// fixed size for the authorization queue
type AuthorizationQueue [TotalCores][MaxAuthorizationQueueItems]common.Hash

// U: The set of partial state, used during accumulation. See equation 170.
type PartialState struct {
	D                  map[uint32]*ServiceAccount `json:"D"`
	UpcomingValidators Validators                 `json:"upcoming_validators"`
	QueueWorkReport    AuthorizationQueue         `json:"authorizations_pool"`
	PrivilegedState    Kai_state                  `json:"privileged_state"`
}

func (ah AccumulationHistory) String() string {
	jsonBytes, err := json.Marshal(ah)
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return string(jsonBytes)
}

// MarshalJSON makes an AccumulationHistory serialize as just the array of hashes.
func (ah AccumulationHistory) MarshalJSON() ([]byte, error) {
	return json.Marshal(ah.WorkPackageHash)
}

func (ah *AccumulationHistory) UnmarshalJSON(data []byte) error {
	var rawHashes []string
	if err := json.Unmarshal(data, &rawHashes); err != nil {
		return err
	}

	ah.WorkPackageHash = make([]common.Hash, len(rawHashes))
	for i, hashStr := range rawHashes {
		ah.WorkPackageHash[i] = common.HexToHash(hashStr)
	}
	return nil
}

func (U PartialState) Dump(prefix string, id uint16) {
	fmt.Printf("[N%d] Partial State Dump -- %s\n", id, prefix)
	for serviceIndex, serviceAccount := range U.D {
		fmt.Printf("[N%d] Service %d => Dirty %v %s\n", id, serviceIndex, serviceAccount.Dirty, serviceAccount.String())
	}
	fmt.Printf("[N%d]\n\n", id)
}

/*
   func (U *PartialState) Clone() (V *PartialState) {
	V = &PartialState{
		D: make(map[uint32]*ServiceAccount),
	}
	for serviceIndex, serviceAccount := range U.D {
		V.D[serviceIndex] = serviceAccount.Clone()
	}
	newV := U.UpcomingValidators
	V.UpcomingValidators = newV
	tmpPrivilegedState := U.PrivilegedState
	V.PrivilegedState = tmpPrivilegedState
	return V
}
*/

type XContext struct {
	I uint32             `json:"I"`
	S uint32             `json:"S"`
	U *PartialState      `json:"U"`
	T []DeferredTransfer `json:"T"`
	Y common.Hash        `json:"Y"` // Question: should this be a pointer or just common.Hash
}

// returns back X.U.D[s] where s is the current service index
func (X *XContext) GetX_s() (xs *ServiceAccount, s uint32) {
	// This is Mutable
	return X.U.D[X.S], X.S
}
func (X *XContext) Clone() (Y XContext) {
	Y = XContext{
		I: X.I,
		S: X.S,
		T: make([]DeferredTransfer, len(X.T)),
		Y: X.Y,
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

func (a *DeferredTransfer) String() string {
	enc, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		// Handle the error according to your needs.
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}
