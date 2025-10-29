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

type AccumulationOutput struct {
	Service uint32      `json:"s"`
	Output  common.Hash `json:"h"`
}

type AlwaysAccumulateEntry struct {
	ServiceID uint32 `json:"id"`
	Gas       uint64 `json:"gas"`
}

type AlwaysAccMap map[uint32]uint64

func (aam AlwaysAccMap) MarshalJSON() ([]byte, error) {
	entries := make([]AlwaysAccumulateEntry, 0, len(aam))
	for id, gas := range aam {
		entries = append(entries, AlwaysAccumulateEntry{
			ServiceID: id,
			Gas:       gas,
		})
	}
	return json.Marshal(entries)
}

func (aam *AlwaysAccMap) UnmarshalJSON(data []byte) error {
	var entries []AlwaysAccumulateEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	*aam = make(AlwaysAccMap, len(entries))
	for _, entry := range entries {
		(*aam)[entry.ServiceID] = entry.Gas
	}
	return nil
}

type PrivilegedServiceState struct {
	ManagerServiceID            uint32             `json:"bless"`      // χₘ ∈ ℕₛ: Manager service index – authorized to alter χ and assign deposits.
	AuthQueueServiceID          [TotalCores]uint32 `json:"designate"`  // χₐ ∈ ⟦ℕₛ⟧𝒞: List of service indices (one per core) that can modify authorizer queue φ. One per core
	UpcomingValidatorsServiceID uint32             `json:"assign"`     // χᵥ ∈ ℕₛ: Service index allowed to set ι. (upcoming validator)
	RegistrarServiceID          uint32             `json:"registrar"`  // χr ∈ ℕₛ: Service index allowed to register new services.
	AlwaysAccServiceID          AlwaysAccMap       `json:"always_acc"` // χz ∈ 𝒟(ℕₛ → ℕG): Services that auto-accumulate gas per block. (is this renamed as "z" from "g")
}

func (k PrivilegedServiceState) Copy() PrivilegedServiceState {
	copy := PrivilegedServiceState{
		ManagerServiceID:            k.ManagerServiceID,
		UpcomingValidatorsServiceID: k.UpcomingValidatorsServiceID,
		RegistrarServiceID:          k.RegistrarServiceID,
		AlwaysAccServiceID:          make(AlwaysAccMap),
	}
	copy.AuthQueueServiceID = k.AuthQueueServiceID
	for k, v := range k.AlwaysAccServiceID {
		copy.AlwaysAccServiceID[k] = v
	}
	return copy
}

func (k *PrivilegedServiceState) GetAllServices() []uint32 {
	services := make(map[uint32]bool)
	services[k.ManagerServiceID] = true            // Manager service index
	services[k.UpcomingValidatorsServiceID] = true // Upcoming validator service index
	services[k.RegistrarServiceID] = true          // Registrar service index
	for _, serviceIndex := range k.AuthQueueServiceID {
		services[serviceIndex] = true // Designated service indices
	}
	// Add all services from the AlwaysAccMap
	for serviceID := range k.AlwaysAccServiceID {
		services[serviceID] = true
	}
	list := []uint32{}
	for s, _ := range services {
		list = append(list, s) // Convert map keys to slice
	}
	// Remove duplicates
	return list
}

// fixed size for the authorization queue
type AuthorizationQueue [TotalCores][MaxAuthorizationQueueItems]common.Hash

// U: The set of partial state, used during accumulation. See (12.13).
type PartialState struct {
	ServiceAccounts map[uint32]*ServiceAccount `json:"D"` // d: Service accounts δ

	UpcomingValidators Validators `json:"upcoming_validators"` // i: Upcoming validators keys ι (DESIGNATE)
	UpcomingDirty      bool       `json:"-"`                   // Dirty flag to indicate if the upcoming validators have been modified

	QueueWorkReport AuthorizationQueue `json:"authorizations_pool"` // q: queue of authorizers φ (ASSIGN)
	QueueDirty      bool               `json:"-"`                   // Dirty flag to indicate if the queue has been modified

	PrivilegedState PrivilegedServiceState `json:"privileged_state"` // x: Privileged state χ (BLESS)
	PrivilegedDirty bool                   `json:"-"`                // Dirty flag to indicate if the state has been modified
}

func (u *PartialState) Checkpoint() {
	for _, sa := range u.ServiceAccounts {
		if sa.NewAccount || sa.Dirty {
			sa.Dirty = true
			sa.Checkpointed = true
		}
	}
}

func (u *PartialState) GetService(s uint32) (*ServiceAccount, bool) {
	if sa, ok := u.ServiceAccounts[s]; ok {
		return sa, true
	}
	return nil, false
}

// IMPORTANT: have to get X vs Y correctly clone with checkpoint!
func (u *PartialState) Clone() *PartialState {
	//fmt.Printf("Cloning PartialState called from %s\n", caller)
	v := &PartialState{
		ServiceAccounts: make(map[uint32]*ServiceAccount),
		// must have copy of the slice
		UpcomingValidators: make([]Validator, len(u.UpcomingValidators)),
		// Copying by value works here
		QueueWorkReport: u.QueueWorkReport,
		// Shallow copy; AlwaysAccServiceID handled below
		PrivilegedState: u.PrivilegedState,
	}

	// Copy UpcomingValidators
	copy(v.UpcomingValidators, u.UpcomingValidators)

	v.QueueWorkReport = u.QueueWorkReport

	// Copy AlwaysAccServiceID properly
	for k, val := range u.PrivilegedState.AlwaysAccServiceID {
		v.PrivilegedState.AlwaysAccServiceID[k] = val
	}

	// Deep copy ServiceAccounts (ServiceAccount map) -- we do need cloning here because of X vs Y
	for s, sa := range u.ServiceAccounts {
		v.ServiceAccounts[s] = sa.Clone() // Assuming ServiceAccount has a Clone() method
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
	for serviceIndex, serviceAccount := range U.ServiceAccounts {
		fmt.Printf("[N%d] Service %d => Dirty %v %s\n", id, serviceIndex, serviceAccount.Dirty, serviceAccount.String())
	}
	fmt.Printf("[N%d]\n\n", id)
}

type XContext struct {
	NewServiceIndex uint32             `json:"new_service_index"`
	ServiceIndex    uint32             `json:"service_index"`
	U               *PartialState      `json:"U"`
	Transfers       []DeferredTransfer `json:"transfers"`
	Yield           common.Hash        `json:"Y"` // Question: should this be a pointer or just common.Hash
	Provided        []Provided         `json:"provided"`
}
type Provided struct {
	ServiceIndex uint32 `json:"ServiceIndex"`
	P_data       []byte `json:"P_data"`
}

// returns back X.U.D[s] where s is the current service index
func (X *XContext) GetX_s() (xs *ServiceAccount, s uint32) {
	// This is Mutable
	return X.U.ServiceAccounts[X.ServiceIndex], X.ServiceIndex
}

func (X *XContext) Clone() (Y XContext) {
	Y = XContext{
		NewServiceIndex: X.NewServiceIndex,
		ServiceIndex:    X.ServiceIndex,
		U:               X.U.Clone(),
		Transfers:       make([]DeferredTransfer, len(X.Transfers)),
		Yield:           X.Yield,
	}
	for i, t := range X.Transfers {
		Y.Transfers[i] = t.Clone()
	}
	return
}

// T: The set of deferred transfers. 12.14
type DeferredTransfer struct {
	SenderIndex   uint32    `json:"sender_index"`   // s ∈ ⟦ℕₛ⟧: Sender service index
	ReceiverIndex uint32    `json:"receiver_index"` // d ∈ ⟦ℕₛ⟧: Receiver service index
	Amount        uint64    `json:"amount"`         // a ∈ ℕ: Amount to transfer
	Memo          [128]byte `json:"memo"`           // m ∈ {0,1}^128: Memo Component
	GasLimit      uint64    `json:"gas_limit"`      // g ∈ ℕ: Gas limit for transfer
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

// https://graypaper.fluffylabs.dev/#/1c979cb/3ac3043ac304?v=0.7.1
const (
	ACCUMULATE_INPUT_OPERANDS  = 0
	ACCUMULATE_INPUT_TRANSFERS = 1
)

type AccumulateInput struct {
	InputType uint8
	T         *DeferredTransfer          `json:"transfer,omitempty"`   // when InputType == 1
	A         *AccumulateOperandElements `json:"accumulate,omitempty"` // when InputType == 0
}

func (a AccumulateInput) Encode() []byte {
	if a.InputType == ACCUMULATE_INPUT_TRANSFERS {
		transferBytes, err := Encode(a.T)
		if err != nil {
			return nil
		}
		return append([]byte{ACCUMULATE_INPUT_TRANSFERS}, transferBytes...)
	}
	return append([]byte{ACCUMULATE_INPUT_OPERANDS}, a.A.Encode()...)
}

// The accumulation operand element, corresponding to a single work item information
type AccumulateOperandElements struct {
	WorkPackageHash     common.Hash `json:"workPackageHash"`     // p = (w_s)_p WorkPackageHash
	ExportedSegmentRoot common.Hash `json:"exportedSegmentRoot"` // e = (w_s)_e ExportedSegmentRoot
	AuthorizerHash      common.Hash `json:"authorizerHash"`      // a = w_a AuthorizerHash
	PayloadHash         common.Hash `json:"payloadHash"`         // y = r_y PayloadHash
	Gas                 uint        `json:"gas"`                 // g = r_g Gas
	Result              Result      `json:"result"`              // l = r_l Result
	Trace               []byte      `json:"trace"`               // t = w_t Trace
}

func (a *AccumulateOperandElements) String() string {
	return ToJSONHex(a)
}

func (a AccumulateOperandElements) Encode() []byte {
	hBytes, err := Encode(a.WorkPackageHash)
	if err != nil {
		return nil
	}
	eBytes, err := Encode(a.ExportedSegmentRoot)
	if err != nil {
		return nil
	}
	aBytes, err := Encode(a.AuthorizerHash)
	if err != nil {
		return nil
	}

	yBytes, err := Encode(a.PayloadHash)
	if err != nil {
		return nil
	}

	gBytes := E(uint64(a.Gas))

	dBytes, err := Encode(a.Result)
	if err != nil {
		return nil
	}
	oBytes, err := Encode(a.Trace)
	if err != nil {
		return nil
	}

	var out []byte
	out = append(out, hBytes...)
	out = append(out, eBytes...)
	out = append(out, aBytes...)
	out = append(out, yBytes...)
	out = append(out, gBytes...)
	out = append(out, dBytes...)
	out = append(out, oBytes...)

	return out
}

func (a *DeferredTransfer) Bytes() []byte {
	enc, err := Encode(a)
	if err != nil {
		return nil
	}
	return enc
}

func (a *DeferredTransfer) String() string {
	return ToJSONHex(a)
}

func DecodedWrangledResults(o *AccumulateOperandElements) string {
	aux := struct {
		WorkPackageHash     common.Hash `json:"workPackageHash"`
		ExportedSegmentRoot common.Hash `json:"exportedSegmentRoot"`
		AuthorizerHash      common.Hash `json:"authorizerHash"`
		PayloadHash         common.Hash `json:"payloadHash"`
		Gas                 uint        `json:"gas"`
		Result              string      `json:"result"`
		Trace               string      `json:"trace"`
	}{
		WorkPackageHash:     o.WorkPackageHash,
		ExportedSegmentRoot: o.ExportedSegmentRoot,
		AuthorizerHash:      o.AuthorizerHash,
		PayloadHash:         o.PayloadHash,
		Gas:                 o.Gas,
		Trace:               fmt.Sprintf("0x%x", o.Trace),
	}

	ResultBytes, _ := Encode(o.Result)
	aux.Result = fmt.Sprintf("0x%x", ResultBytes)
	enc, err := json.Marshal(aux)
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}
