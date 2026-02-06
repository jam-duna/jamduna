package statedb

import (
	"reflect"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/types"
)

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
func (n *JamState) SetAuthQueue(authQueueByte []byte) {
	if len(authQueueByte) == 0 {
		return
	}
	authorizationQueues, _, err := types.Decode(authQueueByte, reflect.TypeOf(types.AuthorizationQueue{}))
	if err != nil {
		return
	}
	n.AuthorizationQueue = authorizationQueues.(types.AuthorizationQueue)
}

// C3 RecentBlocks
func (n *JamState) SetRecentBlocks(recentBlocksByte []byte) {
	if len(recentBlocksByte) == 0 {
		return
	}
	recentBlocks, _, err := types.Decode(recentBlocksByte, reflect.TypeOf(RecentBlocks{}))
	if err != nil {
		return
	}
	n.RecentBlocks = recentBlocks.(RecentBlocks)
}

// C4 safroleState

// SlotSealerSeries
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
	typeIdentifier := data[0]
	encodedData := data[1:]
	switch typeIdentifier {
	case 0: // Primary
		decoded, length, err := types.Decode(encodedData, reflect.TypeOf(types.TicketsMark{}))
		if err != nil {
			return nil, 0
		}
		ticketsMark := decoded.(types.TicketsMark)
		T.Tickets = &ticketsMark
		return T, length + 1
	case 1: // Fallback
		decoded, length, err := types.Decode(encodedData, reflect.TypeOf([types.EpochLength]common.Hash{}))
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
	if decoded == nil {
		return TicketsOrKeys{}, 0
	}
	ticketsOrKeys = decoded.(CTicketsOrKeys).CT2T()
	return ticketsOrKeys, length
}

func (Z RingCommitment) Decode(data []byte) (interface{}, uint32) {
	decoded, length, err := types.Decode(data, reflect.TypeOf([144]byte{}))
	if err != nil {
		return RingCommitment{}, 0
	}
	gammaZ := make(RingCommitment, 144)
	decodedArray := decoded.([144]byte)
	for i := 0; i < 144; i++ {
		gammaZ[i] = decodedArray[i]
	}
	return gammaZ, length
}

func (K NextValidators) Decode(data []byte) (interface{}, uint32) {
	decoded, length, err := types.Decode(data, reflect.TypeOf([types.TotalValidators]types.Validator{}))
	if err != nil {
		return NextValidators{}, 0
	}
	gammaK := make(NextValidators, types.TotalValidators)
	decodedArray := decoded.([types.TotalValidators]types.Validator)
	for i := 0; i < types.TotalValidators; i++ {
		gammaK[i] = decodedArray[i]
	}
	return gammaK, length
}

func (n *JamState) SetSafroleState(safroleStateByte []byte) {
	if len(safroleStateByte) == 0 {
		return
	}
	safroleState, _, err := types.Decode(safroleStateByte, reflect.TypeOf(SafroleBasicState{}))
	if err != nil {
		return
	}
	n.SafroleBasicState = safroleState.(SafroleBasicState)
}

// C5 PastJudgements
func (P DisputeState) Decode(data []byte) (interface{}, uint32) {
	var s struct {
		GoodSet   []common.Hash      `json:"good"`
		BadSet    []common.Hash      `json:"bad"`
		WonkySet  []common.Hash      `json:"wonky"`
		Offenders []types.Ed25519Key `json:"offenders"`
	}
	decoded, l, err := types.Decode(data, reflect.TypeOf(s))
	if err != nil {
		return DisputeState{}, 0
	}
	sDecoded := decoded.(struct {
		GoodSet   []common.Hash      `json:"good"`
		BadSet    []common.Hash      `json:"bad"`
		WonkySet  []common.Hash      `json:"wonky"`
		Offenders []types.Ed25519Key `json:"offenders"`
	})
	P.GoodSet = make([][]byte, len(sDecoded.GoodSet))
	for i, hash := range sDecoded.GoodSet {
		P.GoodSet[i] = hash[:]
	}
	P.BadSet = make([][]byte, len(sDecoded.BadSet))
	for i, hash := range sDecoded.BadSet {
		P.BadSet[i] = hash[:]
	}
	P.WonkySet = make([][]byte, len(sDecoded.WonkySet))
	for i, hash := range sDecoded.WonkySet {
		P.WonkySet[i] = hash[:]
	}
	P.Offenders = sDecoded.Offenders
	return P, l
}

func (j *JamState) SetDisputesState(disputesStateBytes []byte) {
	if len(disputesStateBytes) == 0 {
		return
	}
	disputesState, _, err := types.Decode(disputesStateBytes, reflect.TypeOf(DisputeState{}))
	if err != nil {
		return
	}
	j.DisputesState = disputesState.(DisputeState)
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

// C7 NextEpochValidatorKeys
func (n *JamState) SetDesignatedValidators(DesignedEpochValidatorsByte []byte) {
	if len(DesignedEpochValidatorsByte) == 0 {
		return
	}
	DesignedEpochValidators, _, err := types.Decode(DesignedEpochValidatorsByte, reflect.TypeOf(types.Validators{}))
	if err != nil {
		return
	}
	n.SafroleState.DesignatedValidators = DesignedEpochValidators.(types.Validators)
}

// C8 CurrentValidatorKeys
func (n *JamState) SetCurrEpochValidators(currEpochValidatorsByte []byte) {
	if len(currEpochValidatorsByte) == 0 {
		return
	}
	currEpochValidators, _, err := types.Decode(currEpochValidatorsByte, reflect.TypeOf(types.Validators{}))
	if err != nil {
		return
	}
	n.SafroleState.CurrValidators = currEpochValidators.(types.Validators)
}

// C9 PriorEpochValidatorKeys
func (n *JamState) SetPriorEpochValidators(priorEpochValidatorsByte []byte) {
	if len(priorEpochValidatorsByte) == 0 {
		return
	}
	priorEpochValidators, _, err := types.Decode(priorEpochValidatorsByte, reflect.TypeOf(types.Validators{}))
	if err != nil {
		return
	}
	n.SafroleState.PrevValidators = priorEpochValidators.(types.Validators)
}

// C10 - AvailabilityAssignments
func (T AvailabilityAssignments) Decode(data []byte) (interface{}, uint32) {
	length := uint32(0)
	for i := 0; i < len(T); i++ {
		if data[length] == 0 {
			T[i] = nil
			length++
		} else if data[length] == 1 {
			length++
			coreState, l, err := types.Decode(data[length:], reflect.TypeOf(CoreState{}))
			if err != nil {
				return AvailabilityAssignments{}, 0
			}
			availability_assignment := coreState.(CoreState)
			T[i] = &availability_assignment
			length += l
		}
	}
	return T, length
}

func (n *JamState) SetAvailabilityAssignments(availability_assignmentByte []byte) {
	if len(availability_assignmentByte) == 0 {
		return
	}
	availabilityAssignments, _, err := types.Decode(availability_assignmentByte, reflect.TypeOf(AvailabilityAssignments{}))
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
	privilegedServicesIndices, _, err := types.Decode(privilegedServicesIndicesByte, reflect.TypeOf(types.PrivilegedServiceState{}))
	if err != nil {
		return
	}
	n.PrivilegedServiceIndices = privilegedServicesIndices.(types.PrivilegedServiceState)
}

// C13 ValidatorStatistics
func (n *JamState) SetPi(piByte []byte) {
	if len(piByte) == 0 {
		return
	}
	statistics, _, err := types.Decode(piByte, reflect.TypeOf(types.ValidatorStatistics{}))

	if err != nil {
		return
	}
	stats := statistics.(types.ValidatorStatistics)
	n.ValidatorStatistics = *stats.Copy()
}

// C14 AccumulateQueue
func (n *JamState) SetAccumulateQueue(accumulateQueueByte []byte) {
	if len(accumulateQueueByte) == 0 {
		return
	}
	validatorStatistics, _, err := types.Decode(accumulateQueueByte, reflect.TypeOf([types.EpochLength][]types.AccumulationQueue{}))
	if err != nil {
		return
	}
	n.AccumulationQueue = validatorStatistics.([types.EpochLength][]types.AccumulationQueue)
}

// C15 AccumulateHistory
func (n *JamState) SetAccumulateHistory(accumulateHistoryByte []byte) {
	if len(accumulateHistoryByte) == 0 {
		return
	}
	validatorStatistics, _, err := types.Decode(accumulateHistoryByte, reflect.TypeOf([types.EpochLength]types.AccumulationHistory{}))
	if err != nil {
		return
	}
	n.AccumulationHistory = validatorStatistics.([types.EpochLength]types.AccumulationHistory)
}

// C16 AccumulateOutputs
func (n *JamState) SetAccumulateOutputs(accumulateOutputsByte []byte) {
	if len(accumulateOutputsByte) == 0 {
		return
	}
	sh, _, err := types.Decode(accumulateOutputsByte, reflect.TypeOf([]types.AccumulationOutput{}))
	if err != nil {
		return
	}
	n.AccumulationOutputs = sh.([]types.AccumulationOutput)
}
