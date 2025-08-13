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

// Types for Kai
type Kai_state struct {
	Kai_m uint32             `json:"bless"`      // χₘ ∈ ℕₛ: Manager service index – authorized to alter χ and assign deposits.
	Kai_a [TotalCores]uint32 `json:"designate"`  // χₐ ∈ ⟦ℕₛ⟧𝒞: List of service indices (one per core) that can modify authorizer queue φ. One per core
	Kai_v uint32             `json:"assign"`     // χᵥ ∈ ℕₛ: Service index allowed to set ι. (upcoming validator)
	Kai_g AlwaysAccMap       `json:"always_acc"` // χ𝗀 ∈ 𝒟(ℕₛ → ℕG): Services that auto-accumulate gas per block. (is this renamed as "z")
}

func (k Kai_state) Copy() Kai_state {
	copy := Kai_state{
		Kai_m: k.Kai_m,
		Kai_v: k.Kai_v,
		Kai_g: make(AlwaysAccMap),
	}
	copy.Kai_a = k.Kai_a
	for k, v := range k.Kai_g {
		copy.Kai_g[k] = v
	}
	return copy
}

func (k *Kai_state) GetAllServices() []uint32 {
	services := make(map[uint32]bool)
	services[k.Kai_m] = true // Manager service index
	services[k.Kai_v] = true // Upcoming validator service index
	for _, serviceIndex := range k.Kai_a {
		services[serviceIndex] = true // Designated service indices
	}
	// Add all services from the AlwaysAccMap
	for serviceID := range k.Kai_g {
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
	D                  map[uint32]*ServiceAccount `json:"D"`                   // d: Service accounts δ
	UpcomingValidators Validators                 `json:"upcoming_validators"` // i: Upcoming validators keys ι
	QueueWorkReport    AuthorizationQueue         `json:"authorizations_pool"` // q: queue of authorizers φ
	PrivilegedState    Kai_state                  `json:"privileged_state"`    // x: Privileged state χ
}

func (u *PartialState) Checkpoint() {
	for _, sa := range u.D {
		if sa.NewAccount || sa.Dirty {
			sa.Dirty = true
			sa.Checkpointed = true
		}
	}
	return
}

func (u *PartialState) GetService(s uint32) (*ServiceAccount, bool) {
	if sa, ok := u.D[s]; ok {
		return sa, true
	}
	return nil, false
}

// IMPORTANT: have to get X vs Y correctly clone with checkpoint!
func (u *PartialState) Clone() *PartialState {
	//fmt.Printf("Cloning PartialState called from %s\n", caller)
	v := &PartialState{
		D: make(map[uint32]*ServiceAccount),
		// must have copy of the slice
		UpcomingValidators: make([]Validator, len(u.UpcomingValidators)),
		// Copying by value works here
		QueueWorkReport: u.QueueWorkReport,
		// Shallow copy; Kai_g handled below
		PrivilegedState: u.PrivilegedState,
	}

	// Copy UpcomingValidators
	copy(v.UpcomingValidators, u.UpcomingValidators)

	v.QueueWorkReport = u.QueueWorkReport

	// Copy Kai_g properly
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

type XContext struct {
	I uint32             `json:"I"`
	S uint32             `json:"S"`
	U *PartialState      `json:"U"`
	T []DeferredTransfer `json:"T"`
	Y common.Hash        `json:"Y"` // Question: should this be a pointer or just common.Hash
	P []P                `json:"P"`
}
type P struct {
	ServiceIndex uint32 `json:"ServiceIndex"`
	P_data       []byte `json:"P_data"`
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

// 0.6.4
// see 12.3 Wrangling - Eq 159
// WrangledWorkResult
// wrangled operand tuples
// O: The accumulation operand element, corresponding to a single work result.
type AccumulateOperandElements struct {
	H common.Hash `json:"H"` // h
	E common.Hash `json:"E"` // e
	A common.Hash `json:"A"` // a
	Y common.Hash `json:"Y"` // y
	G uint        `json:"G"` // g 0.6.5 -- see (C.29) -- check
	D Result      `json:"D"` // d
	O []byte      `json:"O"` // o
}

func (a AccumulateOperandElements) Encode() []byte {
	hBytes, err := Encode(a.H)
	if err != nil {
		return nil
	}
	eBytes, err := Encode(a.E)
	if err != nil {
		return nil
	}
	aBytes, err := Encode(a.A)
	if err != nil {
		return nil
	}

	yBytes, err := Encode(a.Y)
	if err != nil {
		return nil
	}

	gBytes := E(uint64(a.G))

	dBytes, err := Encode(a.D)
	if err != nil {
		return nil
	}
	oBytes, err := Encode(a.O)
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
	return ToJSON(a)
}

func DecodedWrangledResults(o *AccumulateOperandElements) string {
	aux := struct {
		H common.Hash `json:"H"`
		E common.Hash `json:"E"`
		A common.Hash `json:"A"`
		Y common.Hash `json:"Y"`
		G uint        `json:"G"`
		D string      `json:"D"`
		O string      `json:"O"`
	}{
		H: o.H,
		E: o.E,
		A: o.A,
		Y: o.Y,
		G: o.G,
		O: fmt.Sprintf("0x%x", o.O),
	}

	ResultBytes, _ := Encode(o.D)
	aux.D = fmt.Sprintf("0x%x", ResultBytes)
	enc, err := json.Marshal(aux)
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}
