package statedb

import (
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// C1
func (n *JamState) SetAuthPool(authPoolByte []byte) {
	if len(authPoolByte) == 0 {
		fmt.Println("AuthPoolByte is empty")
		return
	}
	authorizationsPool, _, err := types.Decode(authPoolByte, reflect.TypeOf([types.TotalCores][]common.Hash{}))
	if err != nil {
		fmt.Println("Error decoding AuthPoolByte: ", err)
		return
	}
	n.AuthorizationsPool = authorizationsPool.([types.TotalCores][]common.Hash)
}

// C2
func (T AuthorizationQueue) Decode(data []byte) (interface{}, uint32) {
	authorizations_queue := [types.TotalCores][80]common.Hash{}
	decoded, length, err := types.Decode(data, reflect.TypeOf(authorizations_queue))
	if err != nil {
		fmt.Println("Error decoding AuthorizationQueue: ", err)
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
		fmt.Println("AuthQueueByte is empty")
		return
	}
	authorizationQueues, _, err := types.Decode(authQueueByte, reflect.TypeOf(AuthorizationQueue{}))
	if err != nil {
		fmt.Println("Error decoding AuthQueueByte: ", err)
		return
	}
	n.AuthorizationQueue = authorizationQueues.(AuthorizationQueue)
}

// C3
func (T Peaks) Decode(data []byte) (interface{}, uint32) {
	if len(data) == 0 {
		return Peaks{}, 0
	}
	peaks_len, length, err := types.Decode(data, reflect.TypeOf(uint(0)))
	if err != nil {
		fmt.Println("Error decoding Peaks: ", err)
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
				fmt.Println("Error decoding Peaks: ", err)
				return Peaks{}, 0
			}
			peak := decoded.(common.Hash)
			peaks[i] = &peak
			length += l
		}
	}
	return peaks, length
}

// func (T Reported) Decode(data []byte) (interface{}, uint32) {
// 	reported, l := types.Decode(data, reflect.TypeOf([]common.Hash{}))
// 	if len(reported.([]common.Hash)) == 0 || len(reported.([]common.Hash)) > types.TotalCores {
// 		fmt.Println("Reported Decode: Reported is not initialized")
// 		return Reported{}, l
// 	}
// 	copy(T[:], reported.([]common.Hash))
// 	return T, l
// }

func (n *JamState) SetRecentBlocks(recentBlocksByte []byte) {
	if len(recentBlocksByte) == 0 {
		fmt.Println("RecentBlocksByte is empty")
		return
	}
	recentBlocks, _, err := types.Decode(recentBlocksByte, reflect.TypeOf(BeefyPool{}))
	if err != nil {
		fmt.Println("Error decoding RecentBlocksByte: ", err)
		return
	}
	n.BeefyPool = recentBlocks.(BeefyPool)
}

// C4
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
			fmt.Println("Error decoding CTicketsOrKeys: ", err)
			return nil, 0
		}
		ticketsMark := decoded.(types.TicketsMark)
		T.Tickets = &ticketsMark
		return T, length + 1
	case 1:
		decoded, length, err := types.Decode(data[1:], reflect.TypeOf([types.EpochLength]common.Hash{}))
		if err != nil {
			fmt.Println("Error decoding CTicketsOrKeys: ", err)
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
		fmt.Println("Error decoding TicketsOrKeys: ", err)
		return TicketsOrKeys{}, 0
	}
	ticketsOrKeys = decoded.(CTicketsOrKeys).CT2T()
	return ticketsOrKeys, length
}

// func (T SafroleBasicState) Decode(data []byte) (interface{}, uint32) {
// 	gamma_k, length := types.Decode(data, reflect.TypeOf([types.TotalValidators]types.Validator{}))
// 	for i := 0; i < types.TotalValidators; i++ {
// 		T.GammaK[i] = gamma_k.([types.TotalValidators]types.Validator)[i]
// 	}
// 	gamma_z, l := types.Decode(data[length:], reflect.TypeOf([types.TicketsVerifierKeyInBytes]byte{}))
// 	for i := 0; i < types.TicketsVerifierKeyInBytes; i++ {
// 		T.GammaZ[i] = gamma_z.([types.TicketsVerifierKeyInBytes]byte)[i]
// 	}
// 	length += l
// 	if data[l] == 0 {
// 		length++
// 		gamma_s, l := types.Decode(data[length:], reflect.TypeOf([types.EpochLength]types.TicketBody{}))
// 		for i := 0; i < types.EpochLength; i++ {
// 			*(T.GammaS.Tickets[i]) = gamma_s.([types.EpochLength]types.TicketBody)[i]
// 		}
// 		length += l
// 	} else {
// 		length++
// 		gamma_s, l := types.Decode(data[length:], reflect.TypeOf([types.EpochLength]common.Hash{}))
// 		for i := 0; i < types.EpochLength; i++ {
// 			T.GammaS.Keys[i] = gamma_s.([types.EpochLength]common.Hash)[i]
// 		}
// 		length += l
// 	}
// 	gamma_a, l := types.Decode(data[length:], reflect.TypeOf([]types.TicketBody{}))
// 	T.GammaA = gamma_a.([]types.TicketBody)
// 	length += l
// 	return T, length
// }

func (n *JamState) SetSafroleState(safroleStateByte []byte) {
	if len(safroleStateByte) == 0 {
		fmt.Println("SafroleStateByte is empty")
		return
	}
	safroleState, _, err := types.Decode(safroleStateByte, reflect.TypeOf(SafroleBasicState{}))
	if err != nil {
		fmt.Println("Error decoding SafroleStateByte: ", err)
		return
	}
	n.SafroleStateGamma = safroleState.(SafroleBasicState)
}

// C5
func (P Psi_state) Decode(data []byte) (interface{}, uint32) {
	fmt.Println("data: ", data)
	var s struct {
		Psi_g []common.Hash      `json:"psi_g"`
		Psi_b []common.Hash      `json:"psi_b"`
		Psi_w []common.Hash      `json:"psi_w"`
		Psi_o []types.Ed25519Key `json:"psi_o"`
	}
	decoded, l, err := types.Decode(data, reflect.TypeOf(s))
	if err != nil {
		fmt.Println("Error decoding Psi_state: ", err)
		return Psi_state{}, 0
	}
	fmt.Println("Decoded: ", decoded)
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

// func (T Psi_state) Decode(data []byte) (interface{}, uint32) {
// 	psi_g, length := types.Decode(data, reflect.TypeOf([types.TotalValidators]common.Hash{}))
// 	psi_b, l := types.Decode(data[length:], reflect.TypeOf([types.TotalValidators]common.Hash{}))
// 	length += l
// 	psi_w, l := types.Decode(data[length:], reflect.TypeOf([types.TotalValidators]common.Hash{}))
// 	length += l
// 	for i := 0; i < types.TotalValidators; i++ {
// 		for j := 0; j < 32; j++ {
// 			T.Psi_g[i][j] = psi_g.([types.TotalValidators]common.Hash)[i][j]
// 			T.Psi_b[i][j] = psi_b.([types.TotalValidators]common.Hash)[i][j]
// 			T.Psi_w[i][j] = psi_w.([types.TotalValidators]common.Hash)[i][j]
// 		}
// 	}
// 	psi_o, l := types.Decode(data[length:], reflect.TypeOf([]types.Ed25519Key{}))
// 	length += l
// 	T.Psi_o = psi_o.([]types.Ed25519Key)
// 	return T, length
// }

func (j *JamState) SetPsi(psiByte []byte) {
	if len(psiByte) == 0 {
		fmt.Println("PsiByte is empty")
		return
	}
	disputesState, _, err := types.Decode(psiByte, reflect.TypeOf(Psi_state{}))
	if err != nil {
		fmt.Println("Error decoding PsiByte: ", err)
		return
	}
	j.DisputesState = disputesState.(Psi_state)
}

// C6
// func (T Entropy) Decode(data []byte) (interface{}, uint32) {
// 	entropy, length := types.Decode(data, reflect.TypeOf([types.EntropySize]common.Hash{}))
// 	for i := 0; i < types.EntropySize; i++ {
// 		T[i] = entropy.([types.EntropySize]common.Hash)[i]
// 	}
// 	return T, length
// }

func (n *JamState) SetEntropy(entropyByte []byte) {
	if len(entropyByte) == 0 {
		fmt.Println("EntropyByte is empty")
		return
	}
	fmt.Println("EntropyByte is not empty: ", entropyByte)
	entropy, _, err := types.Decode(entropyByte, reflect.TypeOf(Entropy{}))
	if err != nil {
		fmt.Println("Error decoding EntropyByte: ", err)
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
		fmt.Println("Error decoding Validators: ", err)
		return Validators{}, 0
	}
	for i := 0; i < types.TotalValidators; i++ {
		T = append(T, validators.([types.TotalValidators]types.Validator)[i])
	}
	return T, length
}

// C7
func (n *JamState) SetNextEpochValidators(nextEpochValidatorsByte []byte) {
	if len(nextEpochValidatorsByte) == 0 {
		fmt.Println("NextEpochValidatorsByte is empty")
		return
	}
	nextEpochValidators, _, err := types.Decode(nextEpochValidatorsByte, reflect.TypeOf(Validators{}))
	if err != nil {
		fmt.Println("Error decoding NextEpochValidatorsByte: ", err)
		return
	}
	n.SafroleState.NextValidators = nextEpochValidators.(Validators)
}

// C8
func (n *JamState) SetCurrEpochValidators(currEpochValidatorsByte []byte) {
	if len(currEpochValidatorsByte) == 0 {
		fmt.Println("CurrEpochValidatorsByte is empty")
		return
	}
	currEpochValidators, _, err := types.Decode(currEpochValidatorsByte, reflect.TypeOf(Validators{}))
	if err != nil {
		fmt.Println("Error decoding CurrEpochValidatorsByte: ", err)
		return
	}
	n.SafroleState.CurrValidators = currEpochValidators.(Validators)
}

// C9
func (n *JamState) SetPriorEpochValidators(priorEpochValidatorsByte []byte) {
	if len(priorEpochValidatorsByte) == 0 {
		fmt.Println("PriorEpochValidatorsByte is empty")
		return
	}
	priorEpochValidators, _, err := types.Decode(priorEpochValidatorsByte, reflect.TypeOf(Validators{}))
	if err != nil {
		fmt.Println("Error decoding PriorEpochValidatorsByte: ", err)
		return
	}
	n.SafroleState.PrevValidators = priorEpochValidators.(Validators)
}

// C10
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
				fmt.Println("Error decoding AvailabilityAssignments: ", err)
				return AvailabilityAssignments{}, 0
			}
			T[i] = rho_state.(*Rho_state)
			length += l
		}
	}
	return T, length
}

func (n *JamState) SetRho(rhoByte []byte) {
	if len(rhoByte) == 0 {
		fmt.Println("RhoByte is empty")
		return
	}
	availabilityAssignments, _, err := types.Decode(rhoByte, reflect.TypeOf(AvailabilityAssignments{}))
	if err != nil {
		fmt.Println("Error decoding RhoByte: ", err)
		return
	}
	n.AvailabilityAssignments = availabilityAssignments.(AvailabilityAssignments)
}

// C11
func (n *JamState) SetMostRecentBlockTimeSlot(mostRecentBlockTimeSlotByte []byte) {
	if len(mostRecentBlockTimeSlotByte) == 0 {
		fmt.Println("MostRecentBlockTimeSlotByte is empty")
		return
	}
	mostRecentBlockTimeSlot, _, err := types.Decode(mostRecentBlockTimeSlotByte, reflect.TypeOf(uint32(0)))
	if err != nil {
		fmt.Println("Error decoding MostRecentBlockTimeSlotByte: ", err)
		return
	}
	n.SafroleState.Timeslot = mostRecentBlockTimeSlot.(uint32)
}

// C12
func (n *JamState) SetPrivilegedServicesIndices(privilegedServicesIndicesByte []byte) {
	if len(privilegedServicesIndicesByte) == 0 {
		fmt.Println("PrivilegedServicesIndicesByte is empty")
		return
	}
	privilegedServicesIndices, _, err := types.Decode(privilegedServicesIndicesByte, reflect.TypeOf(Kai_state{}))
	if err != nil {
		fmt.Println("Error decoding PrivilegedServicesIndicesByte: ", err)
		return
	}
	n.PrivilegedServiceIndices = privilegedServicesIndices.(Kai_state)
}

// C13
func (n *JamState) SetPi(piByte []byte) {
	if len(piByte) == 0 {
		fmt.Println("PiByte is empty")
		return
	}
	validatorStatistics, _, err := types.Decode(piByte, reflect.TypeOf([2][types.TotalValidators]Pi_state{}))
	if err != nil {
		fmt.Println("Error decoding PiByte: ", err)
		return
	}
	n.ValidatorStatistics = validatorStatistics.([2][types.TotalValidators]Pi_state)
}
