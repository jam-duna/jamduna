package statedb

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

var orderedStateList = []string{
	"CoreAuthPool",             // C1
	"AuthQueue",                // C2
	"RecentBlocks",             // C3
	"safroleState",             // C4
	"PastJudgements",           // C5
	"Entropy",                  // C6
	"NextEpochValidatorKeys",   // C7
	"CurrentValidatorKeys",     // C8
	"PriorEpochValidatorKeys",  // C9
	"PendingReports",           // C10
	"MostRecentBlockTimeslot",  // C11
	"PrivilegedServiceIndices", // C12
	"ActiveValidator",          // C13
	"AccumulationQueue",        // C14
	"AccumulationHistory",      // C15
}

// C1 - C15
type StateSnapshot struct {
	AuthorizationsPool       [types.TotalCores][]common.Hash              `json:"alpha"`             // c1
	AuthorizationQueue       types.AuthorizationQueue                     `json:"varphi"`            // c2
	RecentBlocks             RecentBlocks                                 `json:"beta"`              // c3
	Gamma                    SafroleBasicState                            `json:"gamma"`             // c4
	Disputes                 Psi_state                                    `json:"psi"`               // c5
	Entropy                  Entropy                                      `json:"eta"`               // c6
	NextValidators           types.Validators                             `json:"iota"`              // c7
	CurrValidators           types.Validators                             `json:"kappa"`             // c8
	PrevValidators           types.Validators                             `json:"lambda"`            // c9
	AvailabilityAssignments  AvailabilityAssignments                      `json:"rho"`               // c10
	Timeslot                 uint32                                       `json:"tau"`               // c11
	PrivilegedServiceIndices types.Kai_state                              `json:"chi"`               // c12
	ValidatorStatistics      types.ValidatorStatistics                    `json:"pi"`                // c13
	AccumulationQueue        [types.EpochLength][]types.AccumulationQueue `json:"theta"`             // c14 Accumulation Queue
	AccumulationHistory      [types.EpochLength]types.AccumulationHistory `json:"xi"`                // c15 Accumulation History
	StateUpdates             *types.StateUpdate                           `json:"updates,omitempty"` // service updates
}

type KeyVal struct {
	Key   [31]byte `json:"key"`
	Value []byte   `json:"value"`
}

type KeyValMap map[common.Hash][]byte

type StateSnapshotRaw struct {
	StateRoot common.Hash `json:"state_root"`
	KeyVals   []KeyVal    `json:"keyvals"`
}

// kvAlias type for davxy traces
type kvAlias struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	StructType string `json:"type,omitempty"`
	Metadata   string `json:"metadata,omitempty"`
}

func (sn *StateSnapshot) Raw() *StateSnapshotRaw {
	//convert this from our struct format into this keyval format..

	keyValList := make([]KeyVal, 0)

	for _, _stateIdentifier := range orderedStateList {
		stateKey := make([]byte, 32)
		stateVal := []byte{}
		switch _stateIdentifier {
		case C1:
			stateKey[0] = 0x01
			stateVal, _ = types.Encode(sn.AuthorizationsPool)
		case C2:
			stateKey[0] = 0x02
			stateVal, _ = types.Encode(sn.AuthorizationQueue)
		case C3:
			stateKey[0] = 0x03
			stateVal, _ = types.Encode(sn.RecentBlocks)
		case C4:
			stateKey[0] = 0x04
			stateVal, _ = types.Encode(sn.Gamma)
		case C5:
			stateKey[0] = 0x05
			stateVal, _ = types.Encode(sn.Disputes)
		case C6:
			stateKey[0] = 0x06
			stateVal, _ = types.Encode(sn.Entropy)
		case C7:
			stateKey[0] = 0x07
			stateVal, _ = types.Encode(sn.NextValidators)
		case C8:
			stateKey[0] = 0x08
			stateVal, _ = types.Encode(sn.CurrValidators)
		case C9:
			stateKey[0] = 0x09
			stateVal, _ = types.Encode(sn.PrevValidators)
		case C10:
			stateKey[0] = 0x0A
			stateVal, _ = types.Encode(sn.AvailabilityAssignments)
		case C11:
			stateKey[0] = 0x0B
			stateVal, _ = types.Encode(sn.Timeslot)
		case C12:
			stateKey[0] = 0x0C
			stateVal, _ = types.Encode(sn.PrivilegedServiceIndices)
		case C13:
			stateKey[0] = 0x0D
			stateVal, _ = types.Encode(sn.ValidatorStatistics)
		case C14:
			stateKey[0] = 0x0E
			stateVal, _ = types.Encode(sn.AccumulationQueue)
		case C15:
			stateKey[0] = 0x0F
			stateVal, _ = types.Encode(sn.AccumulationQueue)
		default:

		}
		kv := KeyVal{}
		copy(kv.Key[:], stateKey[:])
		kv.Value = stateVal
		keyValList = append(keyValList, kv)
	}

	snapshotRaw := StateSnapshotRaw{
		KeyVals: keyValList,
	}
	return &snapshotRaw
}

func (kv KeyVal) MarshalJSON_OLD() ([]byte, error) {
	aux := kvAlias{
		Key:   common.HexString(kv.Key[0:31]), // 31 byte keys
		Value: common.HexString(kv.Value),
	}
	return json.Marshal(aux)
}

func BytesToHex(b []byte) string {
	// hex.EncodeToString is highly optimized.
	// The "0x" is prepended in a single, efficient string allocation.
	return "0x" + hex.EncodeToString(b)
}

func (kv KeyVal) MarshalJSON() ([]byte, error) {
	aux := kvAlias{
		// Call the new, fast, and type-safe function.
		Key:   BytesToHex(kv.Key[0:31]),
		Value: BytesToHex(kv.Value),
	}
	return json.Marshal(aux)
}

func (kv *KeyVal) UnmarshalJSON(data []byte) error {
	var aux kvAlias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// strip "0x"
	keyHex := strings.TrimPrefix(aux.Key, "0x")
	valHex := strings.TrimPrefix(aux.Value, "0x")

	// decode
	rawKey, err := hex.DecodeString(keyHex)
	if err != nil {
		return fmt.Errorf("invalid hex in key %q: %w", aux.Key, err)
	}

	switch len(rawKey) {
	case 31:
		copy(kv.Key[:], rawKey[:])
	default:
		return fmt.Errorf("invalid key length: expected 31 or 32 bytes, got %d", len(rawKey))
	}

	rawVal, err := hex.DecodeString(valHex)
	if err != nil {
		return fmt.Errorf("invalid hex in value %q: %w", aux.Value, err)
	}
	kv.Value = rawVal

	return nil
}

func (snr *StateSnapshotRaw) FromStateSnapshotRaw() *StateSnapshot {
	sn := StateSnapshot{}
	for idx, kv := range snr.KeyVals {
		_stateIdentifier := orderedStateList[idx]
		// TODO: we should use kv[0] to determine what _stateIdentifier we are talking about
		//k := kv[0]
		switch _stateIdentifier {
		case C1:
			authorizationsPool, _, _ := types.Decode(kv.Value, reflect.TypeOf([types.TotalCores][]common.Hash{}))
			sn.AuthorizationsPool = authorizationsPool.([types.TotalCores][]common.Hash)
		case C2:
			authorizationQueue, _, _ := types.Decode(kv.Value, reflect.TypeOf(types.AuthorizationQueue{}))
			sn.AuthorizationQueue = authorizationQueue.(types.AuthorizationQueue)
		case C3:
			recentBlocks, _, _ := types.Decode(kv.Value, reflect.TypeOf(RecentBlocks{}))
			sn.RecentBlocks = recentBlocks.(RecentBlocks)
		case C4:
			gamma, _, _ := types.Decode(kv.Value, reflect.TypeOf(SafroleBasicState{}))
			sn.Gamma = gamma.(SafroleBasicState)
		case C5:
			disputes, _, _ := types.Decode(kv.Value, reflect.TypeOf(Psi_state{}))
			sn.Disputes = disputes.(Psi_state)
		case C6:
			entropy, _, _ := types.Decode(kv.Value, reflect.TypeOf(Entropy{}))
			sn.Entropy = entropy.(Entropy)
		case C7:
			nextValidators, _, _ := types.Decode(kv.Value, reflect.TypeOf(types.Validators{}))
			sn.NextValidators = nextValidators.(types.Validators)
		case C8:
			currValidators, _, _ := types.Decode(kv.Value, reflect.TypeOf(types.Validators{}))
			sn.CurrValidators = currValidators.(types.Validators)
		case C9:
			prevValidators, _, _ := types.Decode(kv.Value, reflect.TypeOf(types.Validators{}))
			sn.PrevValidators = prevValidators.(types.Validators)
		case C10:
			availabilityAssignments, _, _ := types.Decode(kv.Value, reflect.TypeOf(AvailabilityAssignments{}))
			sn.AvailabilityAssignments = availabilityAssignments.(AvailabilityAssignments)
		case C11:
			timeslot, _, _ := types.Decode(kv.Value, reflect.TypeOf(uint32(0)))
			sn.Timeslot = timeslot.(uint32)
		case C12:
			privilegedServiceIndices, _, _ := types.Decode(kv.Value, reflect.TypeOf(types.Kai_state{}))
			sn.PrivilegedServiceIndices = privilegedServiceIndices.(types.Kai_state)
		case C13:
			validatorStatistics, _, _ := types.Decode(kv.Value, reflect.TypeOf(types.ValidatorStatistics{}))
			sn.ValidatorStatistics = validatorStatistics.(types.ValidatorStatistics)
		case C14:
			validatorStatistics, _, _ := types.Decode(kv.Value, reflect.TypeOf([types.EpochLength][]types.AccumulationQueue{}))
			sn.AccumulationQueue = validatorStatistics.([types.EpochLength][]types.AccumulationQueue)
		case C15:
			validatorStatistics, _, _ := types.Decode(kv.Value, reflect.TypeOf([types.EpochLength]types.AccumulationHistory{}))
			sn.AccumulationHistory = validatorStatistics.([types.EpochLength]types.AccumulationHistory)

		default:

		}
	}
	return &sn
}

func (n *JamState) Snapshot(state *StateSnapshotRaw, stateUpdates *types.StateUpdate) *StateSnapshot {
	original := n.SafroleState
	copied := &StateSnapshot{
		AuthorizationsPool:       n.AuthorizationsPool,                                  // C1
		AuthorizationQueue:       n.AuthorizationQueue,                                  // C2
		RecentBlocks:             n.RecentBlocks,                                        // C3
		Gamma:                    n.SafroleState.GetSafroleBasicState().Copy(),          // C4
		Disputes:                 n.DisputesState,                                       // C5
		Entropy:                  n.SafroleState.Entropy,                                // C6
		NextValidators:           make([]types.Validator, len(original.NextValidators)), // C7
		CurrValidators:           make([]types.Validator, len(original.CurrValidators)), // C8
		PrevValidators:           make([]types.Validator, len(original.PrevValidators)), // C9
		AvailabilityAssignments:  n.AvailabilityAssignments,                             // C10
		Timeslot:                 n.SafroleState.Timeslot,                               // C11
		PrivilegedServiceIndices: n.PrivilegedServiceIndices,                            // C12
		ValidatorStatistics:      n.ValidatorStatistics,                                 // C13
		AccumulationQueue:        n.AccumulationQueue,                                   // C14
		AccumulationHistory:      n.AccumulationHistory,                                 // C15
		StateUpdates:             stateUpdates,                                          // service updates
	}
	copy(copied.Entropy[:], original.Entropy[:])
	copy(copied.PrevValidators, original.PrevValidators)
	copy(copied.CurrValidators, original.CurrValidators)
	copy(copied.NextValidators, original.NextValidators)
	for j := 0; j < types.TotalValidators; j++ {
		copied.ValidatorStatistics.Current[j] = n.ValidatorStatistics.Current[j]
		copied.ValidatorStatistics.Last[j] = n.ValidatorStatistics.Last[j]
		// CHECK
	}
	return copied
}

func (original TicketsOrKeys) Copy() TicketsOrKeys {
	// Only one of either Tickets or Keys can have a value, so they cannot be initialized with `make` beforehand.
	if original.Tickets != nil && original.Keys == nil {
		copid := TicketsOrKeys{
			Tickets: make([]*types.TicketBody, len(original.Tickets)),
		}
		copy(copid.Tickets[:], original.Tickets[:])
		return copid
	} else if original.Tickets == nil && original.Keys != nil {
		copid := TicketsOrKeys{
			Keys: make([]common.Hash, len(original.Keys)),
		}
		copy(copid.Keys[:], original.Keys[:])
		return copid
	}
	return TicketsOrKeys{}
}

func (original SafroleBasicState) Copy() SafroleBasicState {
	copied := SafroleBasicState{
		GammaK: make([]types.Validator, len(original.GammaK)),
		GammaA: make([]types.TicketBody, len(original.GammaA)),
		GammaS: original.GammaS.Copy(),
		GammaZ: make([]byte, len(original.GammaZ)),
	}
	copy(copied.GammaK[:], original.GammaK[:])
	copy(copied.GammaA[:], original.GammaA[:])
	copy(copied.GammaZ[:], original.GammaZ[:])
	return copied
}

func (s *StateSnapshot) String() string {
	return types.ToJSON(s)
}

func (snr *StateSnapshotRaw) String() string {
	return types.ToJSON(snr)
}
