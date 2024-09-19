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
	authorizationsPool, _ := types.Decode(authPoolByte, reflect.TypeOf([types.TotalCores][]common.Hash{}))
	n.AuthorizationsPool = authorizationsPool.([types.TotalCores][]common.Hash)
}

// C2
func (T AuthorizationQueue) Decode(data []byte) (interface{}, uint32) {
	authorizations_queue := [types.TotalCores][80]common.Hash{}
	decoded, length := types.Decode(data, reflect.TypeOf(authorizations_queue))
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
	authorizationQueues, _ := types.Decode(authQueueByte, reflect.TypeOf(AuthorizationQueue{}))
	n.AuthorizationQueue = authorizationQueues.(AuthorizationQueue)
}

// C3
func (T BeefyPool) Decode(data []byte) (interface{}, uint32) {
	beefy_pool, length := types.Decode(data, reflect.TypeOf([]Beta_state{}))
	if len(beefy_pool.([]Beta_state)) == 0 || len(beefy_pool.([]Beta_state)) > types.RecentHistorySize {
		fmt.Println("BeefyPool Decode: BeefyPool is not initialized")
		return BeefyPool{}, length
	}
	copy(T[:], beefy_pool.([]Beta_state))
	return T, length
}
func (T Em_B) Decode(data []byte) (interface{}, uint32) {
	switch data[0] {
	case 0:
		return Em_B{}, 1
	case 1:
		length := uint32(1)
		decoded, l := types.Decode(data[length:], reflect.TypeOf([]common.Hash{}))
		length += l
		return decoded, length
	}
	return Em_B{}, 0
}

func (T Reported) Decode(data []byte) (interface{}, uint32) {
	reported, l := types.Decode(data, reflect.TypeOf([]common.Hash{}))
	if len(reported.([]common.Hash)) == 0 || len(reported.([]common.Hash)) > types.TotalCores {
		fmt.Println("Reported Decode: Reported is not initialized")
		return Reported{}, l
	}
	copy(T[:], reported.([]common.Hash))
	return T, l
}

func (n *JamState) SetRecentBlocks(recentBlocksByte []byte) {
	if len(recentBlocksByte) == 0 {
		fmt.Println("RecentBlocksByte is empty")
		return
	}
	recentBlocks, _ := types.Decode(recentBlocksByte, reflect.TypeOf(BeefyPool{}))
	n.BeefyPool = recentBlocks.(BeefyPool)
}

// C4
func (T SafroleBasicState) Decode(data []byte) (interface{}, uint32) {
	gamma_k, length := types.Decode(data, reflect.TypeOf([types.TotalValidators]types.Validator{}))
	for i := 0; i < types.TotalValidators; i++ {
		T.GammaK[i] = gamma_k.([types.TotalValidators]types.Validator)[i]
	}
	gamma_z, l := types.Decode(data[length:], reflect.TypeOf([types.TicketsVerifierKeyInBytes]byte{}))
	for i := 0; i < types.TicketsVerifierKeyInBytes; i++ {
		T.GammaZ[i] = gamma_z.([types.TicketsVerifierKeyInBytes]byte)[i]
	}
	length += l
	if data[l] == 0 {
		length++
		gamma_s, l := types.Decode(data[length:], reflect.TypeOf([types.EpochLength]types.TicketBody{}))
		for i := 0; i < types.EpochLength; i++ {
			*(T.GammaS.Tickets[i]) = gamma_s.([types.EpochLength]types.TicketBody)[i]
		}
		length += l
	} else {
		length++
		gamma_s, l := types.Decode(data[length:], reflect.TypeOf([types.EpochLength]common.Hash{}))
		for i := 0; i < types.EpochLength; i++ {
			T.GammaS.Keys[i] = gamma_s.([types.EpochLength]common.Hash)[i]
		}
		length += l
	}
	gamma_a, l := types.Decode(data[length:], reflect.TypeOf([]types.TicketBody{}))
	T.GammaA = gamma_a.([]types.TicketBody)
	length += l
	return T, length
}

func (n *JamState) SetSafroleState(safroleStateByte []byte) {
	if len(safroleStateByte) == 0 {
		fmt.Println("SafroleStateByte is empty")
		return
	}
	safroleState, _ := types.Decode(safroleStateByte, reflect.TypeOf(SafroleBasicState{}))
	n.SafroleStateGamma = safroleState.(SafroleBasicState)
}

// C5
func (T Psi_state) Decode(data []byte) (interface{}, uint32) {
	psi_g, length := types.Decode(data, reflect.TypeOf([types.TotalValidators]common.Hash{}))
	psi_b, l := types.Decode(data[length:], reflect.TypeOf([types.TotalValidators]common.Hash{}))
	length += l
	psi_w, l := types.Decode(data[length:], reflect.TypeOf([types.TotalValidators]common.Hash{}))
	length += l
	for i := 0; i < types.TotalValidators; i++ {
		for j := 0; j < 32; j++ {
			T.Psi_g[i][j] = psi_g.([types.TotalValidators]common.Hash)[i][j]
			T.Psi_b[i][j] = psi_b.([types.TotalValidators]common.Hash)[i][j]
			T.Psi_w[i][j] = psi_w.([types.TotalValidators]common.Hash)[i][j]
		}
	}
	psi_o, l := types.Decode(data[length:], reflect.TypeOf([]types.Ed25519Key{}))
	length += l
	T.Psi_o = psi_o.([]types.Ed25519Key)
	return T, length
}

func (j *JamState) SetPsi(psiByte []byte) {
	if len(psiByte) == 0 {
		fmt.Println("PsiByte is empty")
		return
	}
	disputesState, _ := types.Decode(psiByte, reflect.TypeOf(Psi_state{}))
	j.DisputesState = disputesState.(Psi_state)
}

// C6
func (T Entropy) Decode(data []byte) (interface{}, uint32) {
	entropy, length := types.Decode(data, reflect.TypeOf([types.EntropySize]common.Hash{}))
	for i := 0; i < types.EntropySize; i++ {
		T[i] = entropy.([types.EntropySize]common.Hash)[i]
	}
	return T, length
}

func (n *JamState) SetEntropy(entropyByte []byte) {
	if len(entropyByte) == 0 {
		fmt.Println("EntropyByte is empty")
		return
	}
	fmt.Println("EntropyByte is not empty: ", entropyByte)
	entropy, _ := types.Decode(entropyByte, reflect.TypeOf(Entropy{}))
	n.SafroleState.Entropy = entropy.(Entropy)
}

// validators
func (T Validators) Decode(data []byte) (interface{}, uint32) {
	validators, length := types.Decode(data, reflect.TypeOf([types.TotalValidators]types.Validator{}))
	for i := 0; i < types.TotalValidators; i++ {
		T[i] = validators.([types.TotalValidators]types.Validator)[i]
	}
	return T, length
}

// C7
func (n *JamState) SetNextEpochValidators(nextEpochValidatorsByte []byte) {
	if len(nextEpochValidatorsByte) == 0 {
		fmt.Println("NextEpochValidatorsByte is empty")
		return
	}
	nextEpochValidators, _ := types.Decode(nextEpochValidatorsByte, reflect.TypeOf(Validators{}))
	n.SafroleState.NextValidators = nextEpochValidators.(Validators)
}

// C8
func (n *JamState) SetCurrEpochValidators(currEpochValidatorsByte []byte) {
	if len(currEpochValidatorsByte) == 0 {
		fmt.Println("CurrEpochValidatorsByte is empty")
		return
	}
	currEpochValidators, _ := types.Decode(currEpochValidatorsByte, reflect.TypeOf(Validators{}))
	n.SafroleState.CurrValidators = currEpochValidators.(Validators)
}

// C9
func (n *JamState) SetPriorEpochValidators(priorEpochValidatorsByte []byte) {
	if len(priorEpochValidatorsByte) == 0 {
		fmt.Println("PriorEpochValidatorsByte is empty")
		return
	}
	priorEpochValidators, _ := types.Decode(priorEpochValidatorsByte, reflect.TypeOf(Validators{}))
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
			rho_state, l := types.Decode(data[length:], reflect.TypeOf(Rho_state{}))
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
	availabilityAssignments, _ := types.Decode(rhoByte, reflect.TypeOf(AvailabilityAssignments{}))
	n.AvailabilityAssignments = availabilityAssignments.(AvailabilityAssignments)
}

// C11
func (n *JamState) SetMostRecentBlockTimeSlot(mostRecentBlockTimeSlotByte []byte) {
	if len(mostRecentBlockTimeSlotByte) == 0 {
		fmt.Println("MostRecentBlockTimeSlotByte is empty")
		return
	}
	mostRecentBlockTimeSlot, _ := types.Decode(mostRecentBlockTimeSlotByte, reflect.TypeOf(uint32(0)))
	n.SafroleState.Timeslot = mostRecentBlockTimeSlot.(uint32)
}

// C12
func (n *JamState) SetPrivilegedServicesIndices(privilegedServicesIndicesByte []byte) {
	if len(privilegedServicesIndicesByte) == 0 {
		fmt.Println("PrivilegedServicesIndicesByte is empty")
		return
	}
	privilegedServicesIndices, _ := types.Decode(privilegedServicesIndicesByte, reflect.TypeOf(Kai_state{}))
	n.PrivilegedServiceIndices = privilegedServicesIndices.(Kai_state)
}

// C13
func (n *JamState) SetPi(piByte []byte) {
	if len(piByte) == 0 {
		fmt.Println("PiByte is empty")
		return
	}
	validatorStatistics, _ := types.Decode(piByte, reflect.TypeOf([2][types.TotalValidators]Pi_state{}))
	n.ValidatorStatistics = validatorStatistics.([2][types.TotalValidators]Pi_state)
}
