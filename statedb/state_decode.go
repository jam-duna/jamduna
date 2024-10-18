package statedb

import (
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

/*
const (
	C1  = "CoreAuthPool"
	C2  = "AuthQueue"
	C3  = "RecentBlocks"
	C4  = "safroleState"
	C5  = "PastJudgements"
	C6  = "Entropy"
	C7  = "NextEpochValidatorKeys"
	C8  = "CurrentValidatorKeys"
	C9  = "PriorEpochValidatorKeys"
	C10 = "PendingReports"
	C11 = "MostRecentBlockTimeslot"
	C12 = "PrivilegedServiceIndices"
	C13 = "ActiveValidator"
)
*/

// C1 CoreAuthPool
func (n *JamState) SetAuthPool(authPoolByte []byte) {
	if len(authPoolByte) == 0 {
		return
	}
	authorizationsPool, _, err := types.Decode(authPoolByte, reflect.TypeOf([types.TotalCores][]common.Hash{}))
	if err != nil {
		return
	}
	n.AuthorizationsPool = authorizationsPool.([types.TotalCores][]common.Hash)
}

// C2 AuthQueue
func (T AuthorizationQueue) Decode(data []byte) (interface{}, uint32) {
	authorizations_queue := [types.TotalCores][80]common.Hash{}
	decoded, length, err := types.Decode(data, reflect.TypeOf(authorizations_queue))
	if err != nil {
		return AuthorizationQueue{}, 0
	}
	authorizations_queue = decoded.([types.TotalCores][80]common.Hash)
	for i := 0; i < len(authorizations_queue); i++ {
		copy(T[i][:], authorizations_queue[i][:])
	}
	return T, length
}

func (n *JamState) SetAuthQueue(authQueueByte []byte) {
	if len(authQueueByte) == 0 {
		return
	}
	authorizationQueues, _, err := types.Decode(authQueueByte, reflect.TypeOf(AuthorizationQueue{}))
	if err != nil {
		return
	}
	n.AuthorizationQueue = authorizationQueues.(AuthorizationQueue)
}

// C3 RecentBlocks
func (T Peaks) Decode(data []byte) (interface{}, uint32) {
	if len(data) == 0 {
		return Peaks{}, 0
	}
	peaks_len, length, err := types.Decode(data, reflect.TypeOf(uint(0)))
	if err != nil {
		return Peaks{}, 0
	}
	if peaks_len.(uint) == 0 {
		return Peaks{}, length
	}
	peaks := make([]*common.Hash, peaks_len.(uint))
	for i := 0; i < int(peaks_len.(uint)); i++ {
		if data[length] == 0 {
			peaks[i] = nil
			length++
		} else if data[length] == 1 {
			length++
			decoded, l, err := types.Decode(data[length:], reflect.TypeOf(common.Hash{}))
			if err != nil {
				return Peaks{}, 0
			}
			peak := decoded.(common.Hash)
			peaks[i] = &peak
			length += l
		}
	}
	return peaks, length
}

func (n *JamState) SetRecentBlocks(recentBlocksByte []byte) {
	if len(recentBlocksByte) == 0 {
		return
	}
	recentBlocks, _, err := types.Decode(recentBlocksByte, reflect.TypeOf(BeefyPool{}))
	if err != nil {
		return
	}
	n.BeefyPool = recentBlocks.(BeefyPool)
}

// C4 safroleState
func (C CTicketsOrKeys) CT2T() TicketsOrKeys {
	var T TicketsOrKeys

	if C.Tickets != nil {
		T.Tickets = make([]*types.TicketBody, 0)
		for i := 0; i < types.EpochLength; i++ {
			if (*C.Tickets)[i] != (types.TicketBody{}) {
				ticket := (*C.Tickets)[i]
				T.Tickets = append(T.Tickets, &ticket)
			} else {
				break
			}
		}
	}

	if C.Keys != nil {
		T.Keys = make([]common.Hash, 0)
		for i := 0; i < types.EpochLength; i++ {
			if (*C.Keys)[i] != (common.Hash{}) {
				T.Keys = append(T.Keys, (*C.Keys)[i])
			} else {
				break
			}
		}
	}

	return T
}

func (T CTicketsOrKeys) Decode(data []byte) (interface{}, uint32) {
	if len(data) == 0 {
		return nil, 0
	}
	switch data[0] {
	case 0:
		decoded, length, err := types.Decode(data[1:], reflect.TypeOf(types.TicketsMark{}))
		if err != nil {
			return nil, 0
		}
		ticketsMark := decoded.(types.TicketsMark)
		T.Tickets = &ticketsMark
		return T, length + 1
	case 1:
		decoded, length, err := types.Decode(data[1:], reflect.TypeOf([types.EpochLength]common.Hash{}))
		if err != nil {
			return nil, 0
		}
		keys := decoded.([types.EpochLength]common.Hash)
		T.Keys = &keys
		return T, length + 1
	}
	return nil, 0
}

func (T TicketsOrKeys) Decode(data []byte) (interface{}, uint32) {
	if len(data) == 0 {
		return TicketsOrKeys{}, 0
	}
	var ticketsOrKeys TicketsOrKeys
	decoded, length, err := types.Decode(data, reflect.TypeOf(CTicketsOrKeys{}))
	if err != nil {
		return TicketsOrKeys{}, 0
	}
	ticketsOrKeys = decoded.(CTicketsOrKeys).CT2T()
	return ticketsOrKeys, length
}

func (G GammaZ) Decode(data []byte) (interface{}, uint32) {
	decoded, length, err := types.Decode(data, reflect.TypeOf([144]byte{}))
	if err != nil {
		return GammaZ{}, 0
	}
	var gammaZ GammaZ
	decodedArray := decoded.([144]byte)
	copy(gammaZ[:], decodedArray[:])
	return gammaZ, length
}

func (n *JamState) SetSafroleState(safroleStateByte []byte) {
	if len(safroleStateByte) == 0 {
		return
	}
	safroleState, _, err := types.Decode(safroleStateByte, reflect.TypeOf(SafroleBasicState{}))
	if err != nil {
		return
	}
	n.SafroleStateGamma = safroleState.(SafroleBasicState)
}

// C5 PastJudgements
func (P Psi_state) Decode(data []byte) (interface{}, uint32) {
	var s struct {
		Psi_g []common.Hash      `json:"psi_g"`
		Psi_b []common.Hash      `json:"psi_b"`
		Psi_w []common.Hash      `json:"psi_w"`
		Psi_o []types.Ed25519Key `json:"psi_o"`
	}
	decoded, l, err := types.Decode(data, reflect.TypeOf(s))
	if err != nil {
		return Psi_state{}, 0
	}
	sDecoded := decoded.(struct {
		Psi_g []common.Hash      `json:"psi_g"`
		Psi_b []common.Hash      `json:"psi_b"`
		Psi_w []common.Hash      `json:"psi_w"`
		Psi_o []types.Ed25519Key `json:"psi_o"`
	})
	P.Psi_g = make([][]byte, len(sDecoded.Psi_g))
	for i, hash := range sDecoded.Psi_g {
		P.Psi_g[i] = hash[:]
	}
	P.Psi_b = make([][]byte, len(sDecoded.Psi_b))
	for i, hash := range sDecoded.Psi_b {
		P.Psi_b[i] = hash[:]
	}
	P.Psi_w = make([][]byte, len(sDecoded.Psi_w))
	for i, hash := range sDecoded.Psi_w {
		P.Psi_w[i] = hash[:]
	}
	P.Psi_o = sDecoded.Psi_o
	return P, l
}

func (j *JamState) SetPsi(psiByte []byte) {
	if len(psiByte) == 0 {
		return
	}
	disputesState, _, err := types.Decode(psiByte, reflect.TypeOf(Psi_state{}))
	if err != nil {
		return
	}
	j.DisputesState = disputesState.(Psi_state)
}

// C6 Entropy
func (n *JamState) SetEntropy(entropyByte []byte) {
	if len(entropyByte) == 0 {
		return
	}
	entropy, _, err := types.Decode(entropyByte, reflect.TypeOf(Entropy{}))
	if err != nil {
		return
	}
	n.SafroleState.Entropy = entropy.(Entropy)
}

// validators
func (T Validators) Decode(data []byte) (interface{}, uint32) {
	if len(data) == 0 {
		return Validators{}, 0
	}
	validators, length, err := types.Decode(data, reflect.TypeOf([types.TotalValidators]types.Validator{}))
	if err != nil {
		return Validators{}, 0
	}
	for i := 0; i < types.TotalValidators; i++ {
		T = append(T, validators.([types.TotalValidators]types.Validator)[i])
	}
	return T, length
}

// C7 NextEpochValidatorKeys
func (n *JamState) SetNextEpochValidators(nextEpochValidatorsByte []byte) {
	if len(nextEpochValidatorsByte) == 0 {
		return
	}
	nextEpochValidators, _, err := types.Decode(nextEpochValidatorsByte, reflect.TypeOf(Validators{}))
	if err != nil {
		return
	}
	n.SafroleState.NextValidators = nextEpochValidators.(Validators)
}

// C8 CurrentValidatorKeys
func (n *JamState) SetCurrEpochValidators(currEpochValidatorsByte []byte) {
	if len(currEpochValidatorsByte) == 0 {
		return
	}
	currEpochValidators, _, err := types.Decode(currEpochValidatorsByte, reflect.TypeOf(Validators{}))
	if err != nil {
		return
	}
	n.SafroleState.CurrValidators = currEpochValidators.(Validators)
}

// C9 PriorEpochValidatorKeys
func (n *JamState) SetPriorEpochValidators(priorEpochValidatorsByte []byte) {
	if len(priorEpochValidatorsByte) == 0 {
		return
	}
	priorEpochValidators, _, err := types.Decode(priorEpochValidatorsByte, reflect.TypeOf(Validators{}))
	if err != nil {
		return
	}
	n.SafroleState.PrevValidators = priorEpochValidators.(Validators)
}

// C10 PendingReports
func (T AvailabilityAssignments) Decode(data []byte) (interface{}, uint32) {
	length := uint32(0)
	for i := 0; i < len(T); i++ {
		if data[length] == 0 {
			T[i] = nil
			length++
		} else if data[length] == 1 {
			length++
			rho_state, l, err := types.Decode(data[length:], reflect.TypeOf(Rho_state{}))
			if err != nil {
				return AvailabilityAssignments{}, 0
			}
			for i := 0; i < types.TotalCores; i++ {
				rho := rho_state.(Rho_state)
				T[i] = &rho
			}
			length += l
		}
	}
	return T, length
}

func (n *JamState) SetRho(rhoByte []byte) {
	if len(rhoByte) == 0 {
		return
	}
	availabilityAssignments, _, err := types.Decode(rhoByte, reflect.TypeOf(AvailabilityAssignments{}))
	if err != nil {
		return
	}
	n.AvailabilityAssignments = availabilityAssignments.(AvailabilityAssignments)
}

// C11 MostRecentBlockTimeslot
func (n *JamState) SetMostRecentBlockTimeSlot(mostRecentBlockTimeSlotByte []byte) {
	if len(mostRecentBlockTimeSlotByte) == 0 {
		return
	}
	mostRecentBlockTimeSlot, _, err := types.Decode(mostRecentBlockTimeSlotByte, reflect.TypeOf(uint32(0)))
	//fmt.Printf("Recoved C11 MostRecentBlockTimeSlot=%v (byte=%v)\n", mostRecentBlockTimeSlot, mostRecentBlockTimeSlotByte)
	if err != nil {
		return
	}
	n.SafroleState.Timeslot = mostRecentBlockTimeSlot.(uint32)
}

// C12 PrivilegedServiceIndices
func (n *JamState) SetPrivilegedServicesIndices(privilegedServicesIndicesByte []byte) {
	if len(privilegedServicesIndicesByte) == 0 {
		return
	}
	privilegedServicesIndices, _, err := types.Decode(privilegedServicesIndicesByte, reflect.TypeOf(Kai_state{}))
	if err != nil {
		return
	}
	n.PrivilegedServiceIndices = privilegedServicesIndices.(Kai_state)
}

// C13 ActiveValidator
func (n *JamState) SetPi(piByte []byte) {
	if len(piByte) == 0 {
		return
	}
	validatorStatistics, _, err := types.Decode(piByte, reflect.TypeOf([2][types.TotalValidators]Pi_state{}))
	if err != nil {
		return
	}
	n.ValidatorStatistics = validatorStatistics.([2][types.TotalValidators]Pi_state)
}
