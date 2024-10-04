package statedb

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

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
func (T Peaks) Encode() []byte {
	if len(T) == 0 {
		return []byte{0}
	}
	encoded := types.Encode(uint(len(T)))
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

// func (T Reported) Encode() []byte {
// 	reported := []common.Hash{}
// 	for i := 0; i < len(T); i++ {
// 		reported = append(reported, T[i])
// 	}
// 	encoded := types.Encode(reported)
// 	return encoded
// }

func (n *JamState) GetRecentBlocksBytes() []byte {
	codec_bytes := types.Encode(n.BeefyPool)
	return codec_bytes
}

// C4
func (T TicketsOrKeys) T2CT() CTicketsOrKeys {
	var Tickets types.TicketsMark
	var Keys [types.EpochLength]common.Hash
	var C CTicketsOrKeys

	if T.Tickets != nil {
		n := len(T.Tickets)
		if n > types.EpochLength {
			// Handle the error or truncate the slice
			n = types.EpochLength
		}
		for i := 0; i < n; i++ {
			Tickets[i] = *(T.Tickets)[i]
		}
		C.Tickets = &Tickets
	}

	if T.Keys != nil {
		n := len(T.Keys)
		if n > types.EpochLength {
			// Handle the error or truncate the slice
			n = types.EpochLength
		}
		for i := 0; i < n; i++ {
			Keys[i] = T.Keys[i]
		}
		C.Keys = &Keys
	}

	return C
}

func (T CTicketsOrKeys) Encode() []byte {
	if T.Tickets == nil && T.Keys == nil || len(T.Tickets) == 0 && len(T.Keys) == 0 {
		return []byte{}
	}
	encoded := []byte{}
	if T.Tickets != nil && T.Keys == nil {
		encoded = append(encoded, byte(0))
		encoded = append(encoded, types.Encode(T.Tickets)...)
	}
	if T.Keys != nil && T.Tickets == nil {
		encoded = append(encoded, byte(1))
		encoded = append(encoded, types.Encode(T.Keys)...)
	}
	return encoded
}

func (T TicketsOrKeys) Encode() []byte {
	CT := T.T2CT()
	encoded := types.Encode(CT)
	return encoded
}

// func (T SafroleBasicState) Encode() []byte {
// 	var gamma_k [types.TotalValidators]types.Validator
// 	var gamma_z [types.TicketsVerifierKeyInBytes]byte
// 	if len(T.GammaK) == 0 || len(T.GammaK) > types.TotalValidators || len(T.GammaZ) == 0 || len(T.GammaZ) > types.TicketsVerifierKeyInBytes {
// 		fmt.Println("SafroleBasicState Encode: GammaK or GammaZ is not initialized")
// 		return []byte{}
// 	}
// 	copy(gamma_k[:], T.GammaK)
// 	copy(gamma_z[:], T.GammaZ)

// 	var gamma_s CTicketsOrKeys
// 	gamma_s_type := []byte{}
// 	if (T.GammaS.Tickets == nil && T.GammaS.Keys == nil) || (T.GammaS.Tickets != nil && T.GammaS.Keys != nil) || len(T.GammaS.Tickets) > types.EpochLength || len(T.GammaS.Keys) > types.EpochLength || len(T.GammaS.Tickets) == 0 && len(T.GammaS.Keys) == 0 {
// 		fmt.Println("SafroleBasicState Encode: GammaS is not initialized")
// 		return []byte{}
// 	}
// 	fmt.Println("T.GammaS", T.GammaS)
// 	fmt.Println("T.GammaS.Tickets", T.GammaS.Tickets)
// 	fmt.Println("T.GammaS.Keys", T.GammaS.Keys)
// 	if T.GammaS.Tickets != nil && T.GammaS.Keys == nil {
// 		for i := 0; i < len(T.GammaS.Tickets); i++ {
// 			gamma_s.Tickets[i] = *(T.GammaS.Tickets)[i]
// 		}
// 		gamma_s_type = []byte{0}
// 	}
// 	if T.GammaS.Keys != nil && T.GammaS.Tickets == nil {
// 		copy(gamma_s.Keys[:], T.GammaS.Keys)
// 		gamma_s_type = []byte{1}
// 	}
// 	encoded := types.Encode(gamma_k)
// 	encoded = append(encoded, types.Encode(gamma_z)...)
// 	encoded = append(encoded, gamma_s_type...)
// 	if gamma_s_type[0] == 0 {
// 		encoded = append(encoded, types.Encode(gamma_s.Tickets)...)
// 	} else {
// 		encoded = append(encoded, types.Encode(gamma_s.Keys)...)
// 	}
// 	encoded = append(encoded, types.Encode(T.GammaA)...)
// 	return encoded
// }

func (s SafroleBasicState) GetSafroleStateBytes() []byte {
	codec_bytes := types.Encode(s)
	return codec_bytes
}

// C5
func (P Psi_state) Encode() []byte {
	var psi_g []common.Hash
	for _, v := range P.Psi_g {
		psi_g = append(psi_g, common.BytesToHash(v))
	}
	var psi_b []common.Hash
	for _, v := range P.Psi_b {
		psi_b = append(psi_b, common.BytesToHash(v))
	}
	var psi_w []common.Hash
	for _, v := range P.Psi_w {
		psi_w = append(psi_w, common.BytesToHash(v))
	}

	s := struct {
		Psi_g []common.Hash      `json:"psi_g"`
		Psi_b []common.Hash      `json:"psi_b"`
		Psi_w []common.Hash      `json:"psi_w"`
		Psi_o []types.Ed25519Key `json:"psi_o"`
	}{
		Psi_g: psi_g,
		Psi_b: psi_b,
		Psi_w: psi_w,
		Psi_o: P.Psi_o,
	}
	encoded := types.Encode(s)
	return encoded
}

func (j *JamState) GetPsiBytes() []byte {
	codec_bytes := types.Encode(j.DisputesState)
	return codec_bytes
}

// C6
// func (T Entropy) Encode() []byte {
// 	var entropy [types.EntropySize]common.Hash
// 	if len(T) == 0 || len(T) > types.EntropySize {
// 		fmt.Println("Entropy Encode: Entropy is not initialized")
// 		return []byte{}
// 	}
// 	copy(entropy[:], T)
// 	encoded := types.Encode(entropy)
// 	return encoded
// }

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
