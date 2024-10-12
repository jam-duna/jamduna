package statedb

import (
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/types"

	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
)

type SafroleHeader struct {
	ParentHash         common.Hash
	PriorStateRoot     common.Hash
	ExtrinsicHash      common.Hash
	TimeSlot           uint32
	EpochMark          *types.EpochMark
	WinningTicketsMark []*types.TicketBody
	VerdictsMarkers    *types.VerdictMarker
	OffenderMarkers    *types.OffenderMarker
	BlockAuthorKey     uint16
	VRFSignature       []byte
	BlockSeal          []byte
}

type SafroleAccumulator struct {
	Id      common.Hash `json:"id"`
	Attempt int         `json:"attempt"`
}

// 6.1 protocol configuration
type SInput struct {
	Slot          uint32             `json:"slot"`
	Entropy       common.Hash        `json:"entropy"`
	Extrinsics    []Extrinsic        `json:"extrinsic"`
	PostOffenders []types.Ed25519Key `json:"post_offenders"`
}

// Bytes serializes the SafroleBasicState to a byte slice
func (s *SafroleBasicState) Bytes() ([]byte, error) {
	return json.Marshal(s)
}

func SafroleBasicStateFromBytes(data []byte) (SafroleBasicState, error) {
	var sb SafroleBasicState
	err := json.Unmarshal(data, &sb)
	return sb, err
}

func (s *SafroleState) GetNextRingCommitment() ([]byte, error) {
	ringsetBytes := s.GetRingSet("Next")
	nextRingCommitment, err := bandersnatch.GetRingCommitment(ringsetBytes)
	if err != nil {
		return nil, err
	}
	return nextRingCommitment, err
}

func (s *SafroleState) GetSafroleBasicState() SafroleBasicState {
	nextRingCommitment, _ := s.GetNextRingCommitment()
	return SafroleBasicState{
		GammaK: s.NextValidators,
		GammaA: s.NextEpochTicketsAccumulator,
		GammaS: s.TicketsOrKeys,
		GammaZ: nextRingCommitment,
	}
}

func (s *SafroleState) GetNextN2() common.Hash {
	_, currphase := s.EpochAndPhase(uint32(s.Timeslot))
	if currphase == types.EpochLength-1 {
		return s.Entropy[1]
	}
	return s.Entropy[2]
}

// Extrinsic is submitted by authorities, which are added to Safrole State in TicketsAccumulator if valid
type Extrinsic struct {
	Attempt   uint8                                 `json:"attempt"`
	Signature [types.ExtrinsicSignatureInBytes]byte `json:"signature"`
}

// 6.5.3. Ticket Envelope
type TicketEnvelope struct {
	Id            common.Hash                           //not part of the official TicketEnvelope struct
	Attempt       uint8                                 //Index associated to the ticket.
	Extra         []byte                                //additional data for user-defined applications.
	RingSignature [types.ExtrinsicSignatureInBytes]byte //ring signature of the envelope data (attempt & extra)
}

type Entropy [types.EntropySize]common.Hash
type Validators []types.Validator

type SafroleState struct {
	Id             uint32 `json:"Id"`
	EpochFirstSlot uint32 `json:"EpochFirstSlot"`
	Epoch          uint32 `json:"epoch"`

	TimeStamp int    `json:"timestamp"`
	Timeslot  uint32 `json:"timeslot"`

	Entropy Entropy `json:"entropy"`

	// 4 authorities[pre, curr, next, designed]
	PrevValidators     Validators `json:"prev_validators"`
	CurrValidators     Validators `json:"curr_validators"`
	NextValidators     Validators `json:"next_validators"`
	DesignedValidators Validators `json:"designed_validators"`

	// Accumulator of tickets, modified with Extrinsics to hold ORDERED array of Tickets
	NextEpochTicketsAccumulator []types.TicketBody `json:"next_tickets_accumulator"` //gamma_a
	TicketsOrKeys               TicketsOrKeys      `json:"tickets_or_keys"`

	// []bandersnatch.ValidatorKeySet
	TicketsVerifierKey []byte `json:"tickets_verifier_key"`
}

func NewSafroleState() *SafroleState {
	return &SafroleState{
		Id: 99999,
		// Timeslot:           uint32(common.ComputeCurrentJCETime()),
		Timeslot: common.ComputeTimeUnit(types.TimeUnitMode),

		Entropy:            Entropy{},
		PrevValidators:     []types.Validator{},
		CurrValidators:     []types.Validator{},
		NextValidators:     []types.Validator{},
		DesignedValidators: []types.Validator{},
		TicketsOrKeys:      TicketsOrKeys{},
		TicketsVerifierKey: []byte{},
	}
}

func VerifyEpochMarker(epochMark *types.EpochMark) (bool, error) {
	//STUB
	return true, nil
}

func (s *SafroleState) GenerateEpochMarker() *types.EpochMark {
	var nextValidators [6]common.Hash
	for i, v := range s.NextValidators {
		nextValidators[i] = v.GetBandersnatchKey().Hash()
	}
	//fmt.Printf("nextValidators Len=%v\n", nextValidators)
	return &types.EpochMark{
		Entropy:    s.Entropy[1], // Assuming s.Entropy has at least two elements
		Validators: nextValidators,
	}
}

func VerifyWinningMarker(winning_marker [types.EpochLength]*types.TicketBody, expected_marker []*types.TicketBody) (bool, error) {
	// Check if both slices have the same length
	if len(winning_marker) != len(expected_marker) {
		return false, fmt.Errorf("length mismatch: winning_marker has %d elements, expected_marker has %d elements", len(winning_marker), len(expected_marker))
	}

	for i := range winning_marker {
		if winning_marker[i].Id != expected_marker[i].Id || winning_marker[i].Attempt != expected_marker[i].Attempt {
			return false, nil
		}
	}

	return true, nil
}

func (s *SafroleState) InTicketAccumulator(ticketID common.Hash) bool {
	for _, t := range s.NextEpochTicketsAccumulator {
		if t.Id == ticketID {
			return true
		}
	}
	return false
}

// func (s *SafroleState) GenerateWinningMarker() (*types.TicketsMark, error) {
func (s *SafroleState) GenerateWinningMarker() ([]*types.TicketBody, error) {
	tickets := s.NextEpochTicketsAccumulator
	if len(tickets) != types.EpochLength {
		//accumulator should keep exactly EpochLength ticket
		return nil, fmt.Errorf("Invalid Ticket size")
	}
	outsidein_tickets := s.computeTicketSlotBinding(tickets)
	return outsidein_tickets, nil
}

type SOutput struct {
	Ok *struct {
		EpochMark   *types.EpochMark   `json:"epoch_mark"`
		TicketsMark *types.TicketsMark `json:"tickets_mark"`
	} `json:"ok,omitempty"`
	Err *string `json:"err,omitempty" ` // ErrorCode
}

// StateDB STF Errors
const (
	errEpochFirstSlotNotMet = "Fail: EpochFirst not met"
)

// STF Errors
const (
	errTimeslotNotMonotonic                = "Fail: Timeslot must be strictly monotonic."
	errExtrinsicWithMoreTicketsThanAllowed = "Fail: Submit an extrinsic with more tickets than allowed."
	errTicketResubmission                  = "Fail: Re-submit tickets from authority 0."
	errTicketBadOrder                      = "Fail: Submit tickets in bad order."
	errTicketBadRingProof                  = "Fail: Submit tickets with bad ring proof."
	errTicketSubmissionInTail              = "Fail: Submit a ticket while in epoch's tail."
	errNone                                = "ok"
	errInvalidWinningMarker                = "Fail: Invalid winning marker"
)

type ClaimData struct {
	Slot             uint32 `json:"slot"`
	AuthorityIndex   uint32 `json:"authority_index"`
	RandomnessSource []byte `json:"randomness_source"`
}

func ComputeEpochAndPhase(ts, Epoch0Timestamp uint32) (currentEpoch uint32, currentPhase uint32) {

	if types.TimeUnitMode == "TimeStamp" {
		if ts < Epoch0Timestamp || ts == 0xFFFFFFFF {
			currentEpoch = 0
			currentPhase = 0
			return currentEpoch, currentPhase
		}
	} else if types.TimeUnitMode == "TimeSlot" {
		if ts < Epoch0Timestamp/types.SecondsPerSlot {
			currentEpoch = 0
			currentPhase = 0
			return currentEpoch, currentPhase
		} else {
			currentEpoch = (ts - Epoch0Timestamp/types.SecondsPerSlot) / types.EpochLength
			currentPhase = (ts - Epoch0Timestamp/types.SecondsPerSlot) % types.EpochLength
			return currentEpoch, currentPhase
		}
	} else if types.TimeUnitMode == "JAM" {
		if ts < Epoch0Timestamp/types.SecondsPerSlot {
			currentEpoch = 0
			currentPhase = 0
			return currentEpoch, currentPhase
		}
		currentEpoch = ts / types.EpochLength
		currentPhase = ts % types.EpochLength
		return currentEpoch, currentPhase

	} else {
		return 0, 0
	}

	return 0, 0
}

func (s *SafroleState) EpochAndPhase(ts uint32) (currentEpoch int32, currentPhase uint32) {
	if types.TimeUnitMode == "TimeStamp" {
		if ts < s.EpochFirstSlot {
			currentEpoch = -1
			currentPhase = 0
			return
		}
		currentEpoch = int32((ts - s.EpochFirstSlot) / (types.SecondsPerSlot * types.EpochLength)) // eg. / 60
		currentPhase = ((ts - s.EpochFirstSlot) % (types.SecondsPerSlot * types.EpochLength)) / types.SecondsPerSlot
		return
	} else if types.TimeUnitMode == "TimeSlot" {
		realEpochFirstSlot := s.EpochFirstSlot / uint32(types.SecondsPerSlot)
		if ts < realEpochFirstSlot {
			currentEpoch = -1
			currentPhase = 0
			return
		}
		currentEpoch = int32((ts - realEpochFirstSlot) / types.EpochLength) // eg. / 60
		currentPhase = ((ts - realEpochFirstSlot) % types.EpochLength)
		return
	} else if types.TimeUnitMode == "JAM" {
		realEpochFirstSlot := s.EpochFirstSlot / uint32(types.SecondsPerSlot)
		if ts < realEpochFirstSlot {
			currentEpoch = -1
			currentPhase = 0
			return
		}

		currentEpoch = int32(ts / types.EpochLength) // eg. / 60
		currentPhase = ts % types.EpochLength
		return
	} else {
		return -1, 0
	}

}

func (s *SafroleState) IsNewEpoch(currCJE uint32) bool {
	prevEpoch, _ := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, _ := s.EpochAndPhase(currCJE)
	return currEpoch > prevEpoch
}

// used for detecting the end of submission period
func (s *SafroleState) IseWinningMarkerNeeded(currCJE uint32) bool {
	prevEpoch, prevPhase := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, currPhase := s.EpochAndPhase(currCJE)
	if currEpoch == prevEpoch && prevPhase < types.TicketSubmissionEndSlot && types.TicketSubmissionEndSlot <= currPhase && len(s.NextEpochTicketsAccumulator) == types.EpochLength {
		return true
	}
	return false
}

func (s *SafroleState) IsTicketSubmissionClosed(currJCE uint32) bool {
	_, currPhase := s.EpochAndPhase(currJCE)
	if currPhase >= types.TicketSubmissionEndSlot {
		return true
	}
	return false
}

func CalculateEpochAndPhase(currentTimeslot, priorTimeslot uint32) (currentEpoch, priorEpoch, currentPhase, priorPhase uint32) {
	currentEpoch = currentTimeslot / types.EpochLength
	currentPhase = currentTimeslot % types.EpochLength
	priorEpoch = priorTimeslot / types.EpochLength
	priorPhase = priorTimeslot % types.EpochLength
	return
}

func (s *SafroleState) GetFreshRandomness(vrfSig []byte) (common.Hash, error) {
	fresh_randomness, err := bandersnatch.VRFSignedOutput(vrfSig)
	if err != nil {
		return common.Hash{}, err
	}
	return common.BytesToHash(fresh_randomness), nil
}

func (s *SafroleState) ComputeCurrRandomness(fresh_randomness common.Hash) common.Hash {
	// η0 Entropy[0] CURRENT randomness accumulator (see sec 6.10).
	// randomness_buffer[0] = BLAKE2(CONCAT(randomness_buffer[0], fresh_randomness));
	randomness_buffer0 := s.Entropy[0].Bytes()

	// Compute BLAKE2 hash of the combined data
	new_randomness := common.Blake2Hash(append(randomness_buffer0, fresh_randomness.Bytes()...))
	return new_randomness
}

// 6.5.1. Ticket Identifier (Primary Method)
// ticketSealVRFInput constructs the input for VRF based on target epoch randomness and attempt.
func (s *SafroleState) ticketSealVRFInput(targetEpochRandomness common.Hash, attempt uint8) []byte {
	// Concatenate sassafrasTicketSeal, targetEpochRandomness, and attemptBytes
	ticket_vrf_input := append(append([]byte(types.X_T), targetEpochRandomness.Bytes()...), []byte{byte(attempt & 0xF)}...)
	return ticket_vrf_input
}

func (s *SafroleState) computeTicketID(authority_secret_key bandersnatch.BanderSnatchSecret, ticket_vrf_input []byte) (common.Hash, error) {
	//ticket_id := bandersnatch.VRFOutput(authority_secret_key, ticket_vrf_input)
	auxData := []byte{}
	ticket_id, err := bandersnatch.VRFOutput(authority_secret_key, ticket_vrf_input, auxData)
	if err != nil {
		return common.Hash{}, fmt.Errorf("signTicket failed")
	}
	return common.BytesToHash(ticket_id), nil
}

func (s *SafroleState) EntropyToBytes() []byte {
	var entropyBytes []byte
	for _, hash := range s.Entropy {
		entropyBytes = append(entropyBytes, hash.Bytes()...)
	}
	return entropyBytes
}

func EntropyFromBytes(data []byte) ([]common.Hash, error) {
	if len(data)%32 != 0 || len(data)/32 != 4 {
		return nil, fmt.Errorf("invalid data length for entropy")
	}
	entropy := make([]common.Hash, 4)
	for i := 0; i < 4; i++ {
		entropy[i] = common.BytesToHash(data[i*32 : (i+1)*32])
	}
	return entropy, nil
}

func (s *SafroleState) SetEntropyFromBytes(data []byte) error {
	if len(data)%32 != 0 || len(data)/32 != 4 {
		return fmt.Errorf("invalid data length for entropy")
	}
	entropy, err := EntropyFromBytes(data)
	if err != nil {
		return err
	}
	for i := 0; i < 4; i++ {
		s.Entropy[i] = entropy[i]
	}
	return nil
}

func (s *SafroleState) GetCurrEpochFirst() uint32 {
	return s.EpochFirstSlot
}

func (s *SafroleState) GetNextEpochFirst() uint32 {
	nextEpochFirstSlot := s.EpochFirstSlot + types.EpochLength*types.SecondsPerSlot
	return nextEpochFirstSlot
}

func (s *SafroleState) GetTimeSlot() uint32 {
	return uint32(s.Timeslot)
}

func (s *SafroleState) GetEpochValidatorAndRandomness(phase string) ([]types.Validator, common.Hash) {
	var validatorSet []types.Validator
	var target_randomness common.Hash
	switch phase {
	case "Pre": //N-1
		validatorSet = s.PrevValidators
	case "Curr": //N
		validatorSet = s.CurrValidators
	case "Next": //N+1
		validatorSet = s.NextValidators
	case "Designed": //N+2
		validatorSet = s.DesignedValidators
	}
	return validatorSet, target_randomness
}

func (s *SafroleState) GetValidatorData(phase string) (validatorsData []byte) {
	// 4 authorities[pre, curr, next, designed]
	var validatorSet []types.Validator
	switch phase {
	case "Pre": //N-1
		validatorSet = s.PrevValidators
	case "Curr": //N
		validatorSet = s.CurrValidators
	case "Next": //N+1
		validatorSet = s.NextValidators
	case "Designed": //N+2
		validatorSet = s.DesignedValidators
	}
	for _, v := range validatorSet {
		validatorsData = append(validatorsData, v.Bytes()...)
	}
	return validatorsData
}

func (s *SafroleState) GetCurrValidatorIndex(key types.Ed25519Key) int {
	for i, v := range s.CurrValidators {
		if v.Ed25519 == key {
			return i
		}
	}
	// If not found, return -1
	return -1
}

func (s *SafroleState) GetCurrValidator(index int) types.Validator {
	return s.CurrValidators[index]
}

func (s *SafroleState) SetValidatorData(validatorsData []byte, phase string) error {
	// Calculate the expected length for each validator data
	const validatorLength = 32 + 32 + 144 + 128

	// Check if validatorsData length is a multiple of validatorLength
	if len(validatorsData)%validatorLength != 0 {
		return fmt.Errorf("invalid validatorsData length: expected a multiple of %d, got %d", validatorLength, len(validatorsData))
	}

	var validatorSet []types.Validator
	for offset := 0; offset < len(validatorsData); offset += validatorLength {
		validator, err := types.ValidatorFromBytes(validatorsData[offset : offset+validatorLength])
		if err != nil {
			return err
		}
		validatorSet = append(validatorSet, validator)
	}

	switch phase {
	case "Pre": // N-1
		s.PrevValidators = validatorSet
	case "Curr": // N
		s.CurrValidators = validatorSet
	case "Next": // N+1
		s.NextValidators = validatorSet
	case "Designed": // N+2
		s.DesignedValidators = validatorSet
	default:
		return fmt.Errorf("unknown phase: %s", phase)
	}
	return nil
}

func (s *SafroleState) GetRingSet(phase string) (ringsetBytes []byte) {
	var validatorSet []types.Validator
	// 4 authorities[pre, curr, next, designed]
	switch phase {
	case "Pre": //N-1
		validatorSet = s.PrevValidators
	case "Curr": //N
		validatorSet = s.CurrValidators
	case "Next": //N+1
		validatorSet = s.NextValidators
	case "Designed": //N+2
		validatorSet = s.DesignedValidators
	}
	pubkeys := []bandersnatch.BanderSnatchKey{}
	for _, v := range validatorSet {
		pubkey := bandersnatch.BanderSnatchKey(common.ConvertToSlice(v.Bandersnatch))
		pubkeys = append(pubkeys, pubkey)
	}
	ringsetBytes = bandersnatch.InitRingSet(pubkeys)
	return ringsetBytes
}

func (s *SafroleState) GenerateTickets(secret bandersnatch.BanderSnatchSecret, usedEntropy common.Hash) []types.Ticket {
	tickets := make([]types.Ticket, types.TicketEntriesPerValidator) // Pre-allocate space for tickets
	var wg sync.WaitGroup
	var mu sync.Mutex // To synchronize access to the tickets slice

	start := time.Now()
	for attempt := uint8(0); attempt < types.TicketEntriesPerValidator; attempt++ {
		wg.Add(1)

		// Launch a goroutine for each attempt
		go func(attempt uint8) {
			defer wg.Done()

			entropy := usedEntropy
			ticket, err := s.generateTicket(secret, entropy, attempt)

			if err == nil {
				if debug {
					fmt.Printf("[N%d] Generated ticket %d: %v\n", s.Id, attempt, entropy)
				}
				// Lock to safely append to tickets
				mu.Lock()
				tickets[attempt] = ticket // Store the ticket at the index of the attempt
				mu.Unlock()
			} else {
				fmt.Printf("Error generating ticket for attempt %d: %v\n", attempt, err)
			}
		}(attempt) // Pass the attempt variable to avoid closure capture issues
	}

	wg.Wait() // Wait for all goroutines to finish

	elapsed := time.Since(start).Microseconds()
	if trace && elapsed > 1000000 { // OPTIMIZED generateTicket
		fmt.Printf(" --- GenerateTickets took %d ms\n", elapsed/1000)
	}

	return tickets
}

func (s *SafroleState) generateTicket(secret bandersnatch.BanderSnatchSecret, targetEpochRandomness common.Hash, attempt uint8) (types.Ticket, error) {
	ticket_vrf_input := s.ticketSealVRFInput(targetEpochRandomness, attempt)
	//RingVrfSign(privateKey, ringsetBytes, vrfInputData, auxData []byte)
	//During epoch N, each authority scheduled for epoch N+2 constructs a set of tickets which may be eligible (6.5.2) for on-chain submission.
	auxData := []byte{}
	ringsetBytes := s.GetRingSet("Next")
	//fmt.Printf("generateTicket secrete=%x, ringsetBytes=%x, ticket_vrf_input=%x, auxData=%x\n", secret, ringsetBytes, ticket_vrf_input, auxData)
	//RingVrfSign(privateKeys[proverIdx], ringSet, vrfInputData, auxData)
	signature, _, err := bandersnatch.RingVrfSign(secret, ringsetBytes, ticket_vrf_input, auxData) // ??
	if err != nil {
		return types.Ticket{}, fmt.Errorf("signTicket failed")
	}
	var signatureArray [types.ExtrinsicSignatureInBytes]byte
	copy(signatureArray[:], signature)
	ticket := types.Ticket{
		Attempt:   uint8(attempt),
		Signature: signatureArray,
	}
	return ticket, nil
}

func (s *SafroleState) ValidateProposedTicket(t *types.Ticket, shifted bool) (common.Hash, error) {
	if t.Attempt >= types.TicketEntriesPerValidator {
		return common.Hash{}, fmt.Errorf(errExtrinsicWithMoreTicketsThanAllowed)
	}

	//step 0: derive ticketVRFInput
	entroptIdx := 2
	targetEpochRandomness := s.Entropy[entroptIdx]
	isTicketSubmissionClosed := s.IsTicketSubmissionClosed(uint32(s.Timeslot))

	if isTicketSubmissionClosed || shifted {
		entroptIdx = 1
		targetEpochRandomness = s.Entropy[entroptIdx]
		ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, t.Attempt)

		//step 1: verify envelope's VRFSignature using ring verifier
		//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
		ringsetBytes := s.GetRingSet("Next")
		ticket_id, err := bandersnatch.RingVrfVerify(ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
		if err == nil {
			if debug {
				fmt.Printf("[N%d] ValidateProposed Ticket Succ (SubmissionClosed - using η%v:%v) TicketID=%x\n", s.Id, entroptIdx, targetEpochRandomness, ticket_id)
			}
			return common.BytesToHash(ticket_id), nil
		}
	} else {
		ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, t.Attempt)
		//step 1: verify envelope's VRFSignature using ring verifier
		//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
		ringsetBytes := s.GetRingSet("Next")
		ticket_id, err := bandersnatch.RingVrfVerify(ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
		if err == nil {
			if debug {
				fmt.Printf("[N%d] ValidateProposed Ticket Succ (Regular - using η%v:%v) TicketID=%x\n", s.Id, entroptIdx, targetEpochRandomness, ticket_id)
			}
			return common.BytesToHash(ticket_id), nil
		}
	}

	ticketID, _ := t.TicketID()
	if debug {
		fmt.Printf("[N%d] ValidateProposed Ticket Fail (using n%v:%v) TicketID=%v\nη0:%v\nη1:%v\nη2:%v\nη3:%v\n(SubmissionClosed=%v)\n", s.Id, entroptIdx, targetEpochRandomness, ticketID, s.Entropy[0], s.Entropy[1], s.Entropy[2], s.Entropy[3], isTicketSubmissionClosed)
	}
	return common.Hash{}, fmt.Errorf(errTicketBadRingProof)
}

func (s *SafroleState) ValidateIncomingTicket(t *types.Ticket) (common.Hash, int, error) {
	if t.Attempt >= types.TicketEntriesPerValidator {
		return common.Hash{}, -1, fmt.Errorf(errExtrinsicWithMoreTicketsThanAllowed)
	}
	for i := 1; i < 3; i++ {
		targetEpochRandomness := s.Entropy[i]
		ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, t.Attempt)
		//step 1: verify envelope's VRFSignature using ring verifier
		//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
		ringsetBytes := s.GetRingSet("Next")
		ticket_id, err := bandersnatch.RingVrfVerify(ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
		if err == nil {
			if debug {
				fmt.Printf("[N%d] ValidateIncoming Ticket Succ (using η%v:%v) TicketID=%x\n", s.Id, i, targetEpochRandomness, ticket_id)
			}
			return common.BytesToHash(ticket_id), i, nil
		}
	}
	ticketID, _ := t.TicketID()
	fmt.Printf("[N%d] ValidateIncoming Ticket Fail TicketID=%v\nη0:%v\nη1:%v\nη2:%v\nη3:%v\n", s.Id, ticketID, s.Entropy[0], s.Entropy[1], s.Entropy[2], s.Entropy[3])

	return common.Hash{}, -1, fmt.Errorf(errTicketBadRingProof)
}

func (s *StateDB) RemoveUnusedTickets() {
	//Remove the tickets when the ticket submission is closed
	//remove the tickets entropy[2] in queue
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	sf := s.GetSafrole()
	uselessEntropy := sf.Entropy[2]
	if sf.IsTicketSubmissionClosed(uint32(s.GetSafrole().Timeslot)) {
		delete(s.queuedTickets, uselessEntropy)
	}
}

func compareTickets(a, b common.Hash) int {
	aBytes := a.Bytes()
	bBytes := b.Bytes()
	for i := 0; i < len(aBytes); i++ {
		if aBytes[i] < bBytes[i] {
			return -1
		} else if aBytes[i] > bBytes[i] {
			return 1
		}
	}
	return 0
}

// computeTicketSlotBinding has logic for assigning tickets to slots
func (s *SafroleState) computeTicketSlotBinding(inp []types.TicketBody) []*types.TicketBody {
	i := 0
	tickets := make([]*types.TicketBody, 0, types.EpochLength)
	for i < len(inp)/2 { //MK: check this line, was s.TicketsAccumulator/2
		tickets = append(tickets, &inp[i])
		tickets = append(tickets, &inp[types.EpochLength-1-i])
		i++
	}
	return tickets
}

func (s *SafroleState) SignPrimary(authority_secret_key bandersnatch.BanderSnatchSecret, unsignHeaderHash common.Hash, attempt uint8) ([]byte, []byte, error) {
	sealVRFInput := s.ticketSealVRFInput(s.Entropy[3], attempt)

	blockSeal, inner_vrfOutput, err := s.SignBlockSeal(authority_secret_key, sealVRFInput, unsignHeaderHash)
	if err != nil {
		return nil, nil, fmt.Errorf("Fallback BlockSeal ERR=%v", err)
	}
	// use inner_vrfOutput to to generate fresh randomness
	entropyVRFInput := computeEntropyVRFInput(common.BytesToHash(inner_vrfOutput))
	fresh_VRFSignature, freshRandomness, err := bandersnatch.IetfVrfSign(authority_secret_key, entropyVRFInput, []byte{})
	if err != nil {
		return nil, nil, fmt.Errorf("H_v ERR=%v", err)
	}
	if debug {
		fmt.Printf("SignPrimary unsignHeaderHash=%v, blockSeal=%x, fresh_VRFSignature=%x, freshRandomness=%x\n", unsignHeaderHash, blockSeal, fresh_VRFSignature, freshRandomness)
	}
	return blockSeal, fresh_VRFSignature, nil
}

func (s *SafroleState) ConvertBanderSnatchSecret(authority_secret_key []byte) (bandersnatch.BanderSnatchSecret, error) {
	//TODO: figure out a plan to standardize between bandersnatch package and types
	return bandersnatch.BytesToBanderSnatchSecret(authority_secret_key)
}

// 6.8.1 Primary Method for legit ticket - is it bare VRF here???
func (s *SafroleState) SignFallBack(authority_secret_key bandersnatch.BanderSnatchSecret, unsignHeaderHash common.Hash) ([]byte, []byte, error) {
	sealVRFInput := fallbackSealVRFInput(s.Entropy[3]) // the context
	blockSeal, inner_vrfOutput, err := s.SignBlockSeal(authority_secret_key, sealVRFInput, unsignHeaderHash)
	if err != nil {
		return nil, nil, fmt.Errorf("Fallback BlockSeal ERR=%v", err)
	}
	// use inner_vrfOutput to to generate fresh randomness
	entropyVRFInput := computeEntropyVRFInput(common.BytesToHash(inner_vrfOutput))
	fresh_VRFSignature, freshRandomness, err := bandersnatch.IetfVrfSign(authority_secret_key, entropyVRFInput, []byte{})
	if err != nil {
		return nil, nil, fmt.Errorf("H_v ERR=%v", err)
	}
	if debug {
		fmt.Printf("SignFallBack unsignHeaderHash=%v, blockSeal=%x, fresh_VRFSignature=%x, freshRandomness=%x\n", unsignHeaderHash, blockSeal, fresh_VRFSignature, freshRandomness)
	}
	return blockSeal, fresh_VRFSignature, nil
}

func (s *SafroleState) SignBlockSeal(authority_secret_key bandersnatch.BanderSnatchSecret, sealVRFInput []byte, unsignHeaderHash common.Hash) ([]byte, []byte, error) {
	//IetfVrfSign(privateKey PrivateKey, vrfInputData, auxData []byte)
	ietfSig, inner_vrfOutput, err := bandersnatch.IetfVrfSign(authority_secret_key, sealVRFInput, unsignHeaderHash.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("IetfVrfSign ERR=%v", err)
	}
	return ietfSig, inner_vrfOutput, err
}

// 6.8.2. Secondary Method
func computeEntropyVRFInput(inner_vrfOutput common.Hash) []byte {
	entropyPrefix := []byte(types.X_E)
	entropyVRFInput := append(entropyPrefix, inner_vrfOutput.Bytes()...)
	return entropyVRFInput
}

func fallbackSealVRFInput(targetRandomness common.Hash) []byte {
	sealPrefix := []byte(types.X_F)
	sealVRFInput := append(sealPrefix, targetRandomness.Bytes()...)
	return sealVRFInput
}

func (s *SafroleState) GetRelativeSlotIndex(slot_index uint32) (uint32, error) {
	//relative slot index = slot - epoch_first_slot
	relativeSlotIndex := slot_index - s.EpochFirstSlot
	return relativeSlotIndex, nil
}

func (s *SafroleState) GetAuthorIndex(authorkey common.Hash, phase string) (uint16, error) {
	var validatorSet []types.Validator

	// Select the appropriate set of validators based on the phase
	switch phase {
	case "Pre": // N-1
		validatorSet = s.PrevValidators
	case "Curr": // N
		validatorSet = s.CurrValidators
	case "Next": // N+1
		validatorSet = s.NextValidators
	case "Designed": // N+2
		validatorSet = s.DesignedValidators
	default:
		return 0, errors.New("invalid phase")
	}

	for authorIdx, v := range validatorSet {
		if common.Hash(v.GetBandersnatchKey()) == authorkey {
			return uint16(authorIdx), nil
		}
	}
	return 0, errors.New("author key not found")
}

// shawn modified, align gp
// eq 70
// computeAuthorityIndex computes the authority index for claiming an orphan slot
func (s *SafroleState) computeFallbackAuthorityIndex(targetRandomness common.Hash, relativeSlotIndex uint32, validatorsLen int) (uint32, error) {
	// Concatenate target_randomness and relative_slot_index
	encodedRelativeSlotIndex, err := types.Encode(relativeSlotIndex)
	if err != nil {
		return 0, err
	}
	hashInput := append(targetRandomness.Bytes(), encodedRelativeSlotIndex...)

	// Compute BLAKE2 hash
	hash := common.ComputeHash(hashInput)

	// Extract the first 4 bytes of the hash to compute the index
	indexBytes := hash[:4]
	index, _, err := types.Decode(indexBytes, reflect.TypeOf(uint32(0))) //Check, shawn use the codec here
	if err != nil {
		return 0, err
	}
	//fmt.Printf("targetRandomness=%x, relativeSlotIndex=%v, indexBytes=%x, priv_index=%v. (authLen=%v)", targetRandomness, relativeSlotIndex, indexBytes, index, len(s.Authorities))
	// transform the index to uint32
	indexUint32, ok := index.(uint32)
	if !ok {
		return 0, fmt.Errorf("failed to convert index to uint32")
	}
	return indexUint32 % uint32(validatorsLen), nil
}

// Fallback is using the perspective of right now
func (s *SafroleState) GetFallbackValidator(slot_index uint32) common.Hash {
	// fallback validator has been updated at eq 68
	return s.TicketsOrKeys.Keys[slot_index]
}

func (s *SafroleState) GetPrimaryWinningTicket(slot_index uint32) common.Hash {
	t_or_k := s.TicketsOrKeys
	winning_tickets := t_or_k.Tickets
	_, currPhase := s.EpochAndPhase(slot_index)
	selected_ticket := winning_tickets[currPhase]
	return selected_ticket.Id
}

func (s *SafroleState) CheckEpochType() string {
	t_or_k := s.TicketsOrKeys
	if len(t_or_k.Tickets) > 0 {
		return "primary"
	} else {
		return "fallback"
	}
}

func (s *SafroleState) GetBindedAttempt(targetJCE uint32) (uint8, error) {
	_, currPhase := s.EpochAndPhase(targetJCE)
	t_or_k := s.TicketsOrKeys
	if len(t_or_k.Tickets) == types.EpochLength {
		winning_ticket := (t_or_k.Tickets)[currPhase]
		return uint8(winning_ticket.Attempt), nil
	}
	return 0, fmt.Errorf("Shouldn't be fallback")
}

// eq 59
func (s *SafroleState) IsAuthorizedBuilder(slot_index uint32, bandersnatchPub common.Hash, ticketIDs []common.Hash) bool {

	currEpoch, currPhase := s.EpochAndPhase(slot_index)
	//TicketsOrKeys
	t_or_k := s.TicketsOrKeys

	if len(t_or_k.Tickets) == types.EpochLength {
		winning_ticket_id := s.GetPrimaryWinningTicket(slot_index)
		for _, ticketID := range ticketIDs {
			if ticketID == winning_ticket_id {
				if debug {
					fmt.Printf("[N%v] [AUTHORIZED] (%v, %v) slot_index=%v primary validator=%v\n", s.Id, currEpoch, currPhase, slot_index, bandersnatchPub)
				}
				return true
			}
		}
		if debug {
			fmt.Printf("[N%v] [UNAUTHORIZED] (%v, %v) slot_index=%v primary validator=%v\n", s.Id, currEpoch, currPhase, slot_index, bandersnatchPub)
		}
	} else {

		//fallback mode
		fallback_validator := s.GetFallbackValidator(currPhase)
		if fallback_validator == bandersnatchPub {
			if debug {
				fmt.Printf("[N%v] [AUTHORIZED] (%v, %v) slot_index=%v fallback validator=%v\n", s.Id, currEpoch, currPhase, slot_index, bandersnatchPub)
			}
			return true
		}
	}

	return false
}

func (s *SafroleState) CheckTimeSlotReady() (uint32, bool) {
	// timeslot mark
	// currJCE := common.ComputeCurrentJCETime()
	currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
	prevEpoch, prevPhase := s.EpochAndPhase(s.GetTimeSlot())
	currEpoch, currPhase := s.EpochAndPhase(currJCE)
	if debug {
		if prevEpoch != currEpoch || currPhase != prevPhase {
			fmt.Printf("[N%d] CheckTimeSlotReady PREV [%d] %d %d Curr [%d] %d %d\n", s.Id, s.GetTimeSlot(), prevEpoch, prevPhase, currJCE, currEpoch, currPhase)
		}
	}
	if currEpoch > prevEpoch {
		return currJCE, true
	} else if currEpoch == prevEpoch && currPhase > prevPhase {
		// normal case
		return currJCE, true
	}
	return currJCE, false
}

func (s *SafroleState) CheckFirstPhaseReady() (isReady bool) {
	// timeslot mark
	currJCE := common.ComputeRealCurrentJCETime(types.TimeUnitMode)
	if currJCE < s.EpochFirstSlot {
		//fmt.Printf("Not ready currJCE: %v < s.EpochFirstSlot %v\n", currJCE, s.EpochFirstSlot)
		return false
	}
	return true
}

func cloneSafroleState(original SafroleState) SafroleState {
	copied := SafroleState{
		Id:                          original.Id,
		EpochFirstSlot:              original.EpochFirstSlot,
		Epoch:                       original.Epoch,
		TimeStamp:                   original.TimeStamp,
		Timeslot:                    original.Timeslot,
		Entropy:                     original.Entropy,
		PrevValidators:              make([]types.Validator, len(original.PrevValidators)),
		CurrValidators:              make([]types.Validator, len(original.CurrValidators)),
		NextValidators:              make([]types.Validator, len(original.NextValidators)),
		DesignedValidators:          make([]types.Validator, len(original.DesignedValidators)),
		NextEpochTicketsAccumulator: make([]types.TicketBody, len(original.NextEpochTicketsAccumulator)),
		TicketsOrKeys:               original.TicketsOrKeys,
		TicketsVerifierKey:          make([]byte, len(original.TicketsVerifierKey)),
	}

	// Copy each field individually
	copy(copied.PrevValidators, original.PrevValidators)
	copy(copied.CurrValidators, original.CurrValidators)
	copy(copied.NextValidators, original.NextValidators)
	copy(copied.DesignedValidators, original.DesignedValidators)
	copy(copied.NextEpochTicketsAccumulator, original.NextEpochTicketsAccumulator)
	copy(copied.TicketsVerifierKey, original.TicketsVerifierKey)

	return copied
}

// Function to copy a State struct
func (original *SafroleState) Copy() *SafroleState {
	// Create a new instance of SafroleState
	copyState := &SafroleState{
		Id:                          original.Id,
		EpochFirstSlot:              original.EpochFirstSlot,
		Epoch:                       original.Epoch,
		TimeStamp:                   original.TimeStamp,
		Timeslot:                    original.Timeslot,
		Entropy:                     original.Entropy,
		PrevValidators:              make([]types.Validator, len(original.PrevValidators)),
		CurrValidators:              make([]types.Validator, len(original.CurrValidators)),
		NextValidators:              make([]types.Validator, len(original.NextValidators)),
		DesignedValidators:          make([]types.Validator, len(original.DesignedValidators)),
		NextEpochTicketsAccumulator: make([]types.TicketBody, len(original.NextEpochTicketsAccumulator)),
		TicketsOrKeys:               original.TicketsOrKeys, // Assuming this has value semantics
		TicketsVerifierKey:          make([]byte, len(original.TicketsVerifierKey)),
	}

	// Copy the PrevValidators slice
	copy(copyState.PrevValidators, original.PrevValidators)
	copy(copyState.CurrValidators, original.CurrValidators)
	copy(copyState.NextValidators, original.NextValidators)
	copy(copyState.DesignedValidators, original.DesignedValidators)
	copy(copyState.NextEpochTicketsAccumulator, original.NextEpochTicketsAccumulator)
	// Copy the TicketsVerifierKey slice
	copy(copyState.TicketsVerifierKey, original.TicketsVerifierKey)

	return copyState
}

// statefrole_stf is the function to be tested
func (s *SafroleState) ApplyStateTransitionTickets(tickets []types.Ticket, targetJCE uint32, header types.BlockHeader, id uint32) (SafroleState, error) {
	prevEpoch, prevPhase := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, currPhase := s.EpochAndPhase(targetJCE)
	s2 := cloneSafroleState(*s)
	if currPhase >= types.TicketSubmissionEndSlot && len(tickets) > 0 {
		return s2, fmt.Errorf(errTicketSubmissionInTail)
	}

	if currEpoch < prevEpoch || (currEpoch == prevEpoch && currPhase < prevPhase) {
		return s2, fmt.Errorf("%v - currEpoch %d prevEpoch %d  currPhase %d  prevPhase %d", errTimeslotNotMonotonic, currEpoch, prevEpoch, currPhase, prevPhase)
	}

	// tally existing ticketIDs
	ticketIDs := make(map[common.Hash]uint8)
	for _, a := range s.NextEpochTicketsAccumulator {
		//fmt.Printf("[N%d] ticketID? %d => %s\n", s.Id, i, a.Id.String())
		ticketIDs[a.Id] = a.Attempt
	}

	// Process Extrinsic Tickets
	//fmt.Printf("Current Slot: %d => Input Slot: %d \n", s.Timeslot, targetJCE)
	fresh_randomness, err := s.GetFreshRandomness(header.EntropySource[:])
	if err != nil {

		fmt.Printf("GetFreshRandomness ERR %v (len=%d)", err, len(header.EntropySource[:]))
		return s2, fmt.Errorf("GetFreshRandomness %v", err)
	}
	// in the tail slot with a full set of tickets

	if prevEpoch < currEpoch { //MK check EpochNumSlots-1 ?
		//TODO this is Winning ticket elgible. Check if header has marker, if yes, verify it
		if debug {
			fmt.Printf("[N%d] ApplyStateTransitionTickets: Winning Tickets %d\n", s2.Id, len(s2.NextEpochTicketsAccumulator))
		}
		// eq 68 primary mode
		if len(s2.NextEpochTicketsAccumulator) == types.EpochLength {
			winning_tickets, err := s2.GenerateWinningMarker()
			if err != nil {
				return s2, fmt.Errorf("Get Winning Tickets Failed: %v", err)
			}
			expected_tickets := s.computeTicketSlotBinding(s2.NextEpochTicketsAccumulator)
			verified, err := VerifyWinningMarker([types.EpochLength]*types.TicketBody(winning_tickets), expected_tickets)
			if !verified || err != nil {
				return s2, fmt.Errorf(errTicketResubmission)
			}
			// do something to set this marker
			ticketsOrKeys := TicketsOrKeys{
				Tickets: winning_tickets,
			}
			s2.TicketsOrKeys = ticketsOrKeys
			s2.NextEpochTicketsAccumulator = make([]types.TicketBody, 0)
		} else { // eq 68 fallback mode
			chosenkeys, err := s.ChooseFallBackValidator()
			if err != nil {
				return s2, fmt.Errorf("ChooseFallBackValidator %v", err)
			}
			ticketsOrKeys := TicketsOrKeys{
				Keys: chosenkeys,
			}
			s2.TicketsOrKeys = ticketsOrKeys
		}

	}
	// entropy phasing eq 67 & 57
	new_entropy_0 := s.ComputeCurrRandomness(fresh_randomness)
	isShifted := false
	if currEpoch > prevEpoch {
		// New Epoch
		s2.PhasingEntropyAndValidator(s, new_entropy_0)
		isShifted = true
	} else {
		// Epoch in progress
		s2.StableEntropy(s, new_entropy_0)
	}

	var wg sync.WaitGroup
	ticketMutex := &sync.Mutex{}
	errCh := make(chan error, len(tickets))

	// Iterate over tickets in parallel
	for _, e := range tickets {
		wg.Add(1)

		go func(e types.Ticket) {
			defer wg.Done()

			// Validate the ticket in parallel
			ticket_id, err := s.ValidateProposedTicket(&e, isShifted)
			if err != nil {
				errCh <- fmt.Errorf(errTicketBadRingProof)
				return
			}

			// Protect access to shared resources (map) with a mutex
			ticketMutex.Lock()
			defer ticketMutex.Unlock()

			_, exists := ticketIDs[ticket_id]
			if exists {
				if debug {
					fmt.Printf("DETECTED Resubmit %v\n", ticket_id)
				}
				return
			}

			// If the ticket is valid, add it to the accumulator
			s2.PutTicketInAccumulator(ticket_id, e.Attempt)
		}(e)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Sort and trim tickets
	s2.SortAndTrimTickets()

	s2.Timeslot = targetJCE
	return s2, nil
}

func (s2 *SafroleState) PhasingEntropyAndValidator(s *SafroleState, new_entropy_0 common.Hash) {
	s2.PrevValidators = s2.CurrValidators
	s2.CurrValidators = s2.NextValidators
	s2.NextValidators = s2.DesignedValidators
	prev_n0 := s2.Entropy[0]
	s2.Entropy[1] = s.Entropy[0]
	s2.Entropy[2] = s.Entropy[1]
	s2.Entropy[3] = s.Entropy[2]
	s2.Entropy[0] = new_entropy_0
	s2.NextEpochTicketsAccumulator = s2.NextEpochTicketsAccumulator[0:0]
	if debug {
		fmt.Printf("[N%d] ApplyStateTransitionTickets: ENTROPY shifted new epoch\nη0:%v\nη1:%v\nη2:%v\nη3:%v\nη0:%v (original)\n", s2.Id, s2.Entropy[0], s2.Entropy[1], s2.Entropy[2], s2.Entropy[3], prev_n0)
	}
}

func (s2 *SafroleState) StableEntropy(s *SafroleState, new_entropy_0 common.Hash) {
	s2.Entropy[0] = new_entropy_0
	s2.Entropy[1] = s.Entropy[1]
	s2.Entropy[2] = s.Entropy[2]
	s2.Entropy[3] = s.Entropy[3]
	if debug {
		fmt.Printf("[N%d] ApplyStateTransitionTickets: ENTROPY norm\nη1:%v\nη2:%v\nη3:%v\n", s2.Id, s2.Entropy[1], s2.Entropy[2], s2.Entropy[3])
	}
}

func (s2 *SafroleState) PutTicketInAccumulator(tickeID common.Hash, attempt uint8) {
	newa := types.TicketBody{
		Id:      tickeID,
		Attempt: attempt,
	}
	s2.NextEpochTicketsAccumulator = append(s2.NextEpochTicketsAccumulator, newa)
}

func (s *SafroleState) SortAndTrimTickets() {
	// Sort tickets using compareTickets
	sort.SliceStable(s.NextEpochTicketsAccumulator, func(i, j int) bool {
		return compareTickets(s.NextEpochTicketsAccumulator[i].Id, s.NextEpochTicketsAccumulator[j].Id) < 0
	})

	// Drop useless tickets
	if len(s.NextEpochTicketsAccumulator) > types.EpochLength {
		s.NextEpochTicketsAccumulator = s.NextEpochTicketsAccumulator[0:types.EpochLength]
	}
}

func (s *SafroleState) ChooseFallBackValidator() ([]common.Hash, error) {
	// get the bandersnatch keys of the validators
	banderkeys := make([]common.Hash, 0)
	for _, v := range s.CurrValidators {
		banderkeys = append(banderkeys, common.Hash(v.GetBandersnatchKey()))
	}
	chosenkeys := make([]common.Hash, 0)
	for i := 0; i < types.EpochLength; i++ {
		// get the authority index
		authority_idx, err := s.computeFallbackAuthorityIndex(s.Entropy[2], uint32(i), len(s.CurrValidators))
		if err != nil {
			return nil, err
		}
		chosenkeys = append(chosenkeys, banderkeys[authority_idx])
	}
	return chosenkeys, nil

}

func (E *Extrinsic) UnmarshalJSON(data []byte) error {
	var s struct {
		Attempt   uint8  `json:"attempt"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	E.Attempt = s.Attempt
	sig := common.FromHex(s.Signature)
	if len(sig) != types.ExtrinsicSignatureInBytes {
		return fmt.Errorf("invalid signature length")
	}
	copy(E.Signature[:], sig)
	return nil
}

func (E Extrinsic) MarshalJSON() ([]byte, error) {
	sig := common.HexString(E.Signature[:])
	return json.Marshal(struct {
		Attempt   uint8  `json:"attempt"`
		Signature string `json:"signature"`
	}{
		Attempt:   E.Attempt,
		Signature: sig,
	})
}

type SafroleStateCodec struct {
	Tau    uint32             `json:"tau"`
	Eta    Entropy            `json:"eta"`
	Lambda Validators         `json:"lambda"`
	Kappa  Validators         `json:"kappa"`
	GammaK Validators         `json:"gamma_k"`
	Iota   Validators         `json:"iota"`
	GammaA []types.TicketBody `json:"gamma_a"`
	GammaS TicketsOrKeys      `json:"gamma_s"`
	GammaZ [144]byte          `json:"gamma_z"`
}

func (s *SafroleState) SafroleStateCodec() SafroleStateCodec {

	return SafroleStateCodec{
		Tau:    s.Timeslot,
		Eta:    s.Entropy,
		Lambda: s.PrevValidators,
		Kappa:  s.CurrValidators,
		GammaK: s.NextValidators,
		Iota:   s.DesignedValidators,
		GammaA: s.NextEpochTicketsAccumulator,
		GammaS: s.TicketsOrKeys,
		GammaZ: [144]byte(s.TicketsVerifierKey),
	}
}

func (a *SafroleStateCodec) UnmarshalJSON(data []byte) error {
	var s struct {
		Tau    uint32             `json:"tau"`
		Eta    Entropy            `json:"eta"`
		Lambda Validators         `json:"lambda"`
		Kappa  Validators         `json:"kappa"`
		GammaK Validators         `json:"gamma_k"`
		Iota   Validators         `json:"iota"`
		GammaA []types.TicketBody `json:"gamma_a"`
		GammaS TicketsOrKeys      `json:"gamma_s"`
		GammaZ string             `json:"gamma_z"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	a.Tau = s.Tau
	a.Eta = s.Eta
	a.Lambda = s.Lambda
	a.Kappa = s.Kappa
	a.GammaK = s.GammaK
	a.Iota = s.Iota
	a.GammaA = s.GammaA
	a.GammaS = s.GammaS
	copy(a.GammaZ[:], common.FromHex(s.GammaZ))

	return nil
}

func (a *SafroleStateCodec) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Tau    uint32             `json:"tau"`
		Eta    Entropy            `json:"eta"`
		Lambda Validators         `json:"lambda"`
		Kappa  Validators         `json:"kappa"`
		GammaK Validators         `json:"gamma_k"`
		Iota   Validators         `json:"iota"`
		GammaA []types.TicketBody `json:"gamma_a"`
		GammaS TicketsOrKeys      `json:"gamma_s"`
		GammaZ string             `json:"gamma_z"`
	}{
		Tau:    a.Tau,
		Eta:    a.Eta,
		Lambda: a.Lambda,
		Kappa:  a.Kappa,
		GammaK: a.GammaK,
		Iota:   a.Iota,
		GammaA: a.GammaA,
		GammaS: a.GammaS,
		GammaZ: common.HexString(a.GammaZ[:]),
	})
}
