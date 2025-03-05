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

func (u *PartialState) GetService(s uint32) (*ServiceAccount, bool) {
	if sa, ok := u.D[s]; ok {
		return sa, true
	}
	return nil, false
}

// IMPORTANT: have to get X vs Y correctly clone with checkpoint!
func (u *PartialState) Clone() *PartialState {
	v := &PartialState{
		D:                  make(map[uint32]*ServiceAccount),
		// must have copy of the slice
		UpcomingValidators: make([]Validator, len(u.UpcomingValidators)),
		// Copying by value works here 
		QueueWorkReport:    u.QueueWorkReport,
		// Shallow copy; Kai_g handled below
		PrivilegedState:    u.PrivilegedState,
	}

	// Copy UpcomingValidators
	copy(v.UpcomingValidators, u.UpcomingValidators)

	v.QueueWorkReport = u.QueueWorkReport

	// Copy PrivilegedState fields properly
	v.PrivilegedState.Kai_g = make(map[uint32]uint64)
	for k, val := range u.PrivilegedState.Kai_g {
		v.PrivilegedState.Kai_g[k] = val
	}

	// Deep copy D (ServiceAccount map) -- we do need cloning here because of X vs Y
	for s, sa := range u.D {
		v.D[s] = sa.Clone() // Assuming ServiceAccount has a Clone() method
	}

	return v
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
		U: X.U.Clone(),
		T: make([]DeferredTransfer, len(X.T)),
		Y: X.Y,
	}
	for i, t := range X.T {
		Y.T[i] = t.Clone()
	}
	//log.Info("statedb", "CLONE XContext", "X.U", X.U, "Y.U", Y.U)
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
	Results         Result      `json:"results"`           //o
	Payload         common.Hash `json:"payload"`           //l
	WorkPackageHash common.Hash `json:"work_package_hash"` //k
	AuthOutput      []byte      `json:"auth_output"`       //a
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

func DecodedWrangledResults(o *AccumulateOperandElements) string {
	type Alias AccumulateOperandElements
	aux := struct {
		Results         string      `json:"results"`
		Payload         common.Hash `json:"payload"`
		WorkPackageHash common.Hash `json:"work_package_hash"`
		AuthOutput      []byte      `json:"auth_output"`
	}{
		Payload:         o.Payload,
		WorkPackageHash: o.WorkPackageHash,
		AuthOutput:      o.AuthOutput,
	}

	ResultBytes, _ := Encode(o.Results)
	aux.Results = fmt.Sprintf("0x%x", ResultBytes)

	enc, err := json.Marshal(aux)
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}
