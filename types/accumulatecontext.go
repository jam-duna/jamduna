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
	Kai_a uint32            `json:"chi_a"` // The index of the assign service
	Kai_v uint32            `json:"chi_v"` // The index of the designate service
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
	//log.Info(log.StateDBMonitoring, "CLONE XContext", "X.U", X.U, "Y.U", Y.U)
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
	O []byte      `json:"O"` // o
	Y common.Hash `json:"Y"` // y
	G uint64      `json:"G"` // g 0.6.5 -- see (C.29)
	D Result      `json:"D"` // d
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
		O []byte      `json:"O"`
		Y common.Hash `json:"Y"`
		G uint64      `json:"G"` // g 0.6.5 -- see (C.29)
		D string      `json:"D"`
	}{
		H: o.H,
		E: o.E,
		A: o.A,
		O: o.O,
		Y: o.Y,
		G: o.G, // REVIEW
	}

	ResultBytes, _ := Encode(o.D)
	aux.D = fmt.Sprintf("0x%x", ResultBytes)

	enc, err := json.Marshal(aux)
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}
