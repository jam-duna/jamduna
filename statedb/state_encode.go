package statedb

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type CTicketsOrKeys struct {
	Tickets *[types.EpochLength]types.TicketBody `json:"tickets,omitempty"`
	// Tickets *types.TicketsMark	  `json:"tickets,omitempty"`
	Keys *[types.EpochLength]common.Hash `json:"keys,omitempty"` //BandersnatchKey
}

// C1
func (n *JamState) GetAuthPoolBytes() []byte {
	if len(n.AuthorizationsPool) == 0 {
		fmt.Println("AuthorizationsPool is empty")
		return []byte{}
	}
	codec_bytes := types.Encode(n.AuthorizationsPool)
	return codec_bytes
}

// C2
func (T AuthorizationQueue) Encode() []byte {
	authorizations_queue := [types.TotalCores][80]common.Hash{}
	if len(T) == 0 || len(T) > types.TotalCores {
		fmt.Println("AuthorizationQueue Encode: AuthorizationQueue is not initialized")
		return []byte{}
	}
	for i := 0; i < len(T); i++ {
		copy(authorizations_queue[i][:], T[i])
	}
	encoded := types.Encode(authorizations_queue)
	return encoded
}

func (n *JamState) GetAuthQueueBytes() []byte {
	if len(n.AuthorizationQueue) == 0 {
		fmt.Println("AuthorizationQueue is empty")
		return []byte{}
	}
	codec_bytes := types.Encode(n.AuthorizationQueue)
	return codec_bytes
}

// C3
func (T BeefyPool) Encode() []byte {
	beefypool := []Beta_state{}
	for i := 0; i < len(T); i++ {
		beefypool = append(beefypool, T[i])
	}
	encoded := types.Encode(beefypool)
	return encoded
}

func (T Em_B) Encode() []byte {
	if len(T) == 0 {
		return []byte{0}
	} else {
		encoded := []byte{1}
		encoded = append(encoded, types.Encode(T)...)
		return encoded
	}
}

func (T Reported) Encode() []byte {
	reported := []common.Hash{}
	for i := 0; i < len(T); i++ {
		reported = append(reported, T[i])
	}
	encoded := types.Encode(reported)
	return encoded
}

func (n *JamState) GetRecentBlocksBytes() []byte {
	codec_bytes := types.Encode(n.BeefyPool)
	return codec_bytes
}

// C4
func (T CTicketsOrKeys) Encode() []byte {
	if T.Tickets != nil {
		return append([]byte{0}, types.Encode(T.Tickets)...)
	}
	if T.Keys != nil {
		return append([]byte{1}, types.Encode(T.Keys)...)
	}
	return []byte{}
}

func (T SafroleBasicState) Encode() []byte {
	var gamma_k [types.TotalValidators]types.Validator
	var gamma_z [types.TicketsVerifierKeyInBytes]byte
	if len(T.GammaK) == 0 || len(T.GammaK) > types.TotalValidators || len(T.GammaZ) == 0 || len(T.GammaZ) > types.TicketsVerifierKeyInBytes {
		fmt.Println("SafroleBasicState Encode: GammaK or GammaZ is not initialized")
		return []byte{}
	}
	copy(gamma_k[:], T.GammaK)
	copy(gamma_z[:], T.GammaZ)

	var gamma_s CTicketsOrKeys
	gamma_s_type := []byte{}
	if (T.GammaS.Tickets == nil && T.GammaS.Keys == nil) || (T.GammaS.Tickets != nil && T.GammaS.Keys != nil) || len(T.GammaS.Tickets) > types.EpochLength || len(T.GammaS.Keys) > types.EpochLength || len(T.GammaS.Tickets) == 0 && len(T.GammaS.Keys) == 0 {
		fmt.Println("SafroleBasicState Encode: GammaS is not initialized")
		return []byte{}
	}
	fmt.Println("T.GammaS", T.GammaS)
	fmt.Println("T.GammaS.Tickets", T.GammaS.Tickets)
	fmt.Println("T.GammaS.Keys", T.GammaS.Keys)
	if T.GammaS.Tickets != nil && T.GammaS.Keys == nil {
		for i := 0; i < len(T.GammaS.Tickets); i++ {
			gamma_s.Tickets[i] = *(T.GammaS.Tickets)[i]
		}
		gamma_s_type = []byte{0}
	}
	if T.GammaS.Keys != nil && T.GammaS.Tickets == nil {
		copy(gamma_s.Keys[:], T.GammaS.Keys)
		gamma_s_type = []byte{1}
	}
	encoded := types.Encode(gamma_k)
	encoded = append(encoded, types.Encode(gamma_z)...)
	encoded = append(encoded, gamma_s_type...)
	if gamma_s_type[0] == 0 {
		encoded = append(encoded, types.Encode(gamma_s.Tickets)...)
	} else {
		encoded = append(encoded, types.Encode(gamma_s.Keys)...)
	}
	encoded = append(encoded, types.Encode(T.GammaA)...)
	return encoded
}

func (s SafroleBasicState) GetSafroleStateBytes() []byte {
	codec_bytes := types.Encode(s)
	return codec_bytes
}

// C5
func (T Psi_state) Encode() []byte {
	var psi_g [types.TotalValidators]common.Hash
	var psi_b [types.TotalValidators]common.Hash
	var psi_w [types.TotalValidators]common.Hash
	if len(T.Psi_g) == 0 || len(T.Psi_g) > types.TotalValidators || len(T.Psi_b) == 0 || len(T.Psi_b) > types.TotalValidators || len(T.Psi_w) == 0 || len(T.Psi_w) > types.TotalValidators {
		fmt.Println("Psi_state Encode: Psi_g, Psi_b or Psi_w is not initialized")
		return []byte{}
	}
	for i := 0; i < types.TotalValidators; i++ {
		for j := 0; j < 32; j++ {
			psi_g[i][j] = T.Psi_g[i][j]
			psi_b[i][j] = T.Psi_b[i][j]
			psi_w[i][j] = T.Psi_w[i][j]
		}
	}
	encoded := types.Encode(psi_g)
	encoded = append(encoded, types.Encode(psi_b)...)
	encoded = append(encoded, types.Encode(psi_w)...)
	encoded = append(encoded, types.Encode(T.Psi_o)...)
	return encoded
}

func (j *JamState) GetPsiBytes() []byte {
	codec_bytes := types.Encode(j.DisputesState)
	return codec_bytes
}

// C6
func (T Entropy) Encode() []byte {
	var entropy [types.EntropySize]common.Hash
	if len(T) == 0 || len(T) > types.EntropySize {
		fmt.Println("Entropy Encode: Entropy is not initialized")
		return []byte{}
	}
	copy(entropy[:], T)
	encoded := types.Encode(entropy)
	return encoded
}

func (s *SafroleState) GetEntropyBytes() []byte {
	if s == nil {
		fmt.Println("Entropy is empty")
		return []byte{}
	}
	codec_bytes := types.Encode(s.Entropy)
	return codec_bytes
}

// validators
func (T Validators) Encode() []byte {
	var validators [types.TotalValidators]types.Validator
	if len(T) == 0 || len(T) > types.TotalValidators {
		fmt.Println("Validators Encode: Validators is not initialized")
		return []byte{}
	}
	copy(validators[:], T)
	encoded := types.Encode(validators)
	return encoded
}

// C7
func (s *SafroleState) GetNextEpochValidatorsBytes() []byte {
	if s == nil {
		fmt.Println("Entropy is empty")
		return []byte{}
	}
	codec_bytes := types.Encode(s.NextValidators)
	return codec_bytes
}

// C8
func (s *SafroleState) GetCurrEpochValidatorsBytes() []byte {
	if s == nil {
		fmt.Println("Entropy is empty")
		return []byte{}
	}
	codec_bytes := types.Encode(s.CurrValidators)
	return codec_bytes
}

// C9
func (s *SafroleState) GetPriorEpochValidatorsBytes() []byte {
	if s == nil {
		fmt.Println("Entropy is empty")
		return []byte{}
	}
	codec_bytes := types.Encode(s.PrevValidators)
	return codec_bytes
}

// C10
func (T AvailabilityAssignments) Encode() []byte {
	encoded := []byte{}
	for i := 0; i < len(T); i++ {
		if T[i] == nil {
			encoded = append(encoded, []byte{0}...)
		} else {
			encoded = append(encoded, []byte{1}...)
			encoded = append(encoded, types.Encode(T[i])...)
		}
	}
	return encoded
}

func (n *JamState) GetRhoBytes() []byte {
	codec_bytes := types.Encode(n.AvailabilityAssignments)
	return codec_bytes
}

// C11
func (s *SafroleState) GetMostRecentBlockTimeSlotBytes() []byte {
	if s == nil {
		fmt.Println("Entropy is empty")
		return []byte{}
	}
	codec_bytes := types.Encode(s.Timeslot)
	return codec_bytes
}

// C12
func (n *JamState) GetPrivilegedServicesIndicesBytes() []byte {
	codec_bytes := types.Encode(n.PrivilegedServiceIndices)
	return codec_bytes
}

// C13
func (n *JamState) GetPiBytes() []byte {
	encoded := types.Encode(n.ValidatorStatistics)
	return encoded
}
