package statedb

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// C1
func (n *JamState) GetAuthPoolBytes() []byte {
	if len(n.AuthorizationsPool) == 0 {
		return []byte{}
	}
	codec_bytes, err := types.Encode(n.AuthorizationsPool)
	if err != nil {
		return []byte{}
	}
	return codec_bytes
}

func (n *JamState) GetAuthQueueBytes() []byte {
	if len(n.AuthorizationQueue) == 0 {
		return []byte{}
	}
	codec_bytes, err := types.Encode(n.AuthorizationQueue)
	if err != nil {
		return []byte{}
	}
	return codec_bytes
}

// C3
func (T Peaks) Encode() []byte {
	if len(T) == 0 {
		return []byte{0}
	}

	encoded, err := types.Encode(uint(len(T)))
	if err != nil {
		return nil
	}

	for i := 0; i < len(T); i++ {
		if T[i] == nil {
			encoded = append(encoded, 0)
		} else {
			encoded = append(encoded, 1)
			encodedTi, err := types.Encode(T[i])
			if err != nil {
				return nil
			}
			encoded = append(encoded, encodedTi...)
		}
	}
	return encoded
}

func (n *JamState) GetRecentBlocksBytes() []byte {
	codec_bytes, err := types.Encode(n.RecentBlocks)
	if err != nil {
		return []byte{}
	}
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
		encodedTickets, err := types.Encode(T.Tickets)
		if err != nil {
			return []byte{}
		}
		encoded = append(encoded, encodedTickets...)
	}
	if T.Keys != nil && T.Tickets == nil {
		encoded = append(encoded, byte(1))
		encodedKeys, err := types.Encode(T.Keys)
		if err != nil {
			return []byte{}
		}
		encoded = append(encoded, encodedKeys...)
	}
	return encoded
}

func (T TicketsOrKeys) Encode() []byte {
	CT := T.T2CT()
	encoded, err := types.Encode(CT)
	if err != nil {
		return []byte{}
	}
	return encoded
}

func (G GammaZ) Encode() []byte {
	var gammaZ [144]byte
	copy(gammaZ[:], G[:])
	encoded, err := types.Encode(gammaZ)
	if err != nil {
		return []byte{}
	}
	return encoded
}

func (s SafroleBasicState) GetSafroleStateBytes() []byte {
	codec_bytes, err := types.Encode(s)
	if err != nil {
		return []byte{}
	}
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
	encoded, err := types.Encode(s)
	if err != nil {
		return []byte{}
	}
	return encoded
}

func (j *JamState) GetPsiBytes() []byte {
	codec_bytes, err := types.Encode(j.DisputesState)
	if err != nil {
		return []byte{}
	}
	return codec_bytes
}

// C6
func (s *SafroleState) GetEntropyBytes() []byte {
	if s == nil {
		return []byte{}
	}
	codec_bytes, err := types.Encode(s.Entropy)
	if err != nil {
		return []byte{}
	}
	return codec_bytes
}

// C7
func (s *SafroleState) GetNextEpochValidatorsBytes() []byte {
	if s == nil {
		return []byte{}
	}
	codec_bytes, err := types.Encode(s.NextValidators)
	if err != nil {
		return []byte{}
	}
	return codec_bytes
}

// C8
func (s *SafroleState) GetCurrEpochValidatorsBytes() []byte {
	if s == nil {
		return []byte{}
	}
	codec_bytes, err := types.Encode(s.CurrValidators)
	if err != nil {
		return []byte{}
	}
	return codec_bytes
}

// C9
func (s *SafroleState) GetPriorEpochValidatorsBytes() []byte {
	if s == nil {
		return []byte{}
	}
	codec_bytes, err := types.Encode(s.PrevValidators)
	if err != nil {
		return []byte{}
	}
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
			encodedTi, err := types.Encode(T[i])
			if err != nil {
				return []byte{}
			}
			encoded = append(encoded, encodedTi...)
		}
	}
	return encoded
}

func (n *JamState) GetRhoBytes() []byte {
	codec_bytes, err := types.Encode(n.AvailabilityAssignments)
	if err != nil {
		return []byte{}
	}
	return codec_bytes
}

// C11
func (s *SafroleState) GetMostRecentBlockTimeSlotBytes() []byte {
	if s == nil {
		return []byte{}
	}
	codec_bytes, err := types.Encode(s.Timeslot)
	if err != nil {
		return []byte{}
	}
	return codec_bytes
}

// C12
func (n *JamState) GetPrivilegedServicesIndicesBytes() []byte {
	codec_bytes, err := types.Encode(n.PrivilegedServiceIndices)
	if err != nil {
		return []byte{}
	}
	return codec_bytes
}

// C13
func (n *JamState) GetPiBytes() []byte {
	encoded, err := types.Encode(n.ValidatorStatistics)
	if err != nil {
		return []byte{}
	}
	return encoded
}

// C14
func (n *JamState) GetAccumulationQueueBytes() []byte {
	encoded, err := types.Encode(n.AccumulationQueue)
	if err != nil {
		return []byte{}
	}
	return encoded
}

// C15
func (n *JamState) GetAccumulationHistoryBytes() []byte {
	encoded, err := types.Encode(n.AccumulationHistory)
	if err != nil {
		return []byte{}
	}
	return encoded
}
