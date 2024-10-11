package statedb

import (
	"encoding/json"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// C1 - C13
type StateSnapshot struct {
	AuthorizationsPool       [types.TotalCores][]common.Hash    `json:"alpha"`  // c1
	AuthorizationQueue       AuthorizationQueue                 `json:"varphi"` // c2
	BeefyPool                BeefyPool                          `json:"beta"`   // c3
	Gamma                    SafroleBasicState                  `json:"gamma"`  // c4
	Disputes                 Psi_state                          `json:"psi"`    // c5
	Entropy                  Entropy                            `json:"eta"`    // c6
	NextValidators           Validators                         `json:"iota"`   // c7
	CurrValidators           Validators                         `json:"kappa"`  // c8
	PrevValidators           Validators                         `json:"lambda"` // c9
	AvailabilityAssignments  AvailabilityAssignments            `json:"rho"`    // c10
	Timeslot                 uint32                             `json:"tau"`    // c11
	PrivilegedServiceIndices Kai_state                          `json:"chi"`    // c12
	ValidatorStatistics      [2][types.TotalValidators]Pi_state `json:"pi"`     // c13
}

func (n *JamState) Snapshot() *StateSnapshot {
	original := n.SafroleState
	copied := &StateSnapshot{
		AuthorizationsPool:       n.AuthorizationsPool,                                  // C1 -- todo
		AuthorizationQueue:       n.AuthorizationQueue,                                  // C2 -- todo
		BeefyPool:                n.BeefyPool,                                           // C3 -- todo
		Gamma:                    n.SafroleState.GetSafroleBasicState().Copy(),          // C4
		Disputes:                 n.DisputesState,                                       // C5 -- todo
		Entropy:                  n.SafroleState.Entropy,                                // C6
		NextValidators:           make([]types.Validator, len(original.NextValidators)), // C7
		CurrValidators:           make([]types.Validator, len(original.CurrValidators)), // C8
		PrevValidators:           make([]types.Validator, len(original.PrevValidators)), // C9
		AvailabilityAssignments:  n.AvailabilityAssignments,                             // C10 -- todo
		Timeslot:                 n.SafroleState.Timeslot,                               // C11
		PrivilegedServiceIndices: n.PrivilegedServiceIndices,                            // C12 -- todo
		ValidatorStatistics:      n.ValidatorStatistics,                                 // C13
	}
	copy(copied.Entropy[:], original.Entropy[:])
	copy(copied.PrevValidators, original.PrevValidators)
	copy(copied.CurrValidators, original.CurrValidators)
	copy(copied.NextValidators, original.NextValidators)
	for i := 0; i < len(n.ValidatorStatistics); i++ {
		for j := 0; j < types.TotalValidators; j++ {
			copied.ValidatorStatistics[i][j] = n.ValidatorStatistics[i][j]
		}
	}
	return copied
}

func (original TicketsOrKeys) Copy() TicketsOrKeys {
	copied := TicketsOrKeys{
		Tickets: make([]*types.TicketBody, len(original.Tickets)),
		Keys:    make([]common.Hash, len(original.Keys)),
	}
	copy(copied.Tickets[:], original.Tickets[:])
	copy(copied.Keys[:], original.Keys[:])
	return copied
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
	//jsonEncode, _ := json.MarshalIndent(s, "", "    ")
	jsonEncode, _ := json.Marshal(s)
	return string(jsonEncode)
}
