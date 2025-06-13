package statedb

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"

	"encoding/json"
	"errors"
	"sort"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
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
	Slot       uint32         `json:"slot"`
	Entropy    common.Hash    `json:"entropy"`
	Extrinsics []types.Ticket `json:"extrinsic"`
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
	ringsetBytes, ringSize := s.GetRingSet("Next")
	if len(ringsetBytes) == 0 {
		return nil, fmt.Errorf("Not ready yet")
	}
	nextRingCommitment, err := bandersnatch.GetRingCommitment(ringSize, ringsetBytes)
	if err != nil {
		return nil, err
	}
	return nextRingCommitment, err
}

func (s *SafroleState) GetSafroleBasicState() SafroleBasicState {
	nextRingCommitment, _ := s.GetNextRingCommitment()
	return SafroleBasicState{
		GammaK: []types.Validator(s.NextValidators),
		GammaA: s.NextEpochTicketsAccumulator,
		GammaS: s.TicketsOrKeys,
		GammaZ: nextRingCommitment,
	}
}

func (s *SafroleState) GetNextN2(targetJCE uint32) common.Hash {
	epochChanged := s.EpochChanged(targetJCE)
	if epochChanged {
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

func (e Entropy) String() string {
	return fmt.Sprintf("Entropy: n0=%v | n1=%v n2=%v | n3=%v", e[0], e[1], e[2], e[3])
}

type SafroleState struct {
	Id             uint16 `json:"Id"`
	EpochFirstSlot uint32 `json:"EpochFirstSlot"`

	OffenderState []types.Ed25519Key `json:"offender_state"`

	Timeslot uint32 `json:"timeslot"`

	Entropy Entropy `json:"entropy"`

	// 4 authorities[pre, curr, next, designed]
	PrevValidators     types.Validators `json:"prev_validators"`
	CurrValidators     types.Validators `json:"curr_validators"`
	NextValidators     types.Validators `json:"next_validators"`
	DesignedValidators types.Validators `json:"designed_validators"`

	// Accumulator of tickets, modified with Extrinsics to hold ORDERED array of Tickets
	NextEpochTicketsAccumulator []types.TicketBody `json:"next_tickets_accumulator"` //gamma_a
	TicketsOrKeys               TicketsOrKeys      `json:"tickets_or_keys"`

	// []bandersnatch.ValidatorKeySet
	TicketsVerifierKey []byte `json:"tickets_verifier_key"`
}

func NewSafroleState() *SafroleState {
	return &SafroleState{
		Id:                 9999,
		Timeslot:           0, // MK check! was common.ComputeTimeUnit(types.TimeUnitMode)
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
	var nextValidators [types.TotalValidators]types.ValidatorKeyTuple
	for i, v := range s.NextValidators {
		nextValidators[i].BandersnatchKey = v.GetBandersnatchKey().Hash()
		nextValidators[i].Ed25519Key = common.Hash(v.GetEd25519Key())
	}
	return &types.EpochMark{ // see https://graypaper.fluffylabs.dev/#/911af30/0e72030e7203
		Entropy:        s.Entropy[0], // this is eta1' = eta0
		TicketsEntropy: s.Entropy[1], // this is eta2' = eta1
		Validators:     nextValidators,
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
	errEpochFirstSlotNotMet                = "Fail: EpochFirst not met"
	errExtrinsicWithMoreTicketsThanAllowed = "Fail: Submit an extrinsic with more tickets than allowed."
	errInvalidWinningMarker                = "Fail: Invalid winning marker"
)

type ClaimData struct {
	Slot             uint32 `json:"slot"`
	AuthorityIndex   uint32 `json:"authority_index"`
	RandomnessSource []byte `json:"randomness_source"`
}

func ComputeEpochAndPhase(ts uint32, Epoch0Timestamp uint64) (currentEpoch uint32, currentPhase uint32) {
	if types.TimeUnitMode == "JAM" {
		if ts < uint32(Epoch0Timestamp/types.SecondsPerSlot) {
			currentEpoch = 0
			currentPhase = 0
			return currentEpoch, currentPhase
		}
		currentEpoch = uint32(ts / types.EpochLength)
		currentPhase = uint32(ts % types.EpochLength)
		return currentEpoch, currentPhase

	}
	return 0, 0
}

func (s *SafroleState) EpochAndPhase(ts uint32) (currentEpoch int32, currentPhase uint32) {
	if types.TimeUnitMode == "JAM" {
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

func (s *SafroleState) IsNewEpoch(currJCE uint32) bool {
	prevEpoch, _ := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, _ := s.EpochAndPhase(currJCE)
	return currEpoch > prevEpoch
}

// used for detecting the end of submission period
func (s *SafroleState) IseWinningMarkerNeeded(currJCE uint32) bool {
	prevEpoch, prevPhase := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, currPhase := s.EpochAndPhase(currJCE)
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
func ticketSealVRFInput(targetEpochRandomness common.Hash, attempt uint8) []byte {
	return append(append([]byte(types.X_T), targetEpochRandomness.Bytes()...), []byte{byte(attempt & 0xF)}...)
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

func (s *SafroleState) GetPrevValidatorIndex(key types.Ed25519Key) int {
	for i, v := range s.PrevValidators {
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

func (s *SafroleState) GetRingSet(phase string) (ringsetBytes []byte, ringSize int) {
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
	ringSize = len(validatorSet)
	pubkeys := []bandersnatch.BanderSnatchKey{}
	for _, v := range validatorSet {
		pubkey := bandersnatch.BanderSnatchKey(common.ConvertToSlice(v.Bandersnatch))
		pubkeys = append(pubkeys, pubkey)
	}
	ringsetBytes = bandersnatch.InitRingSet(pubkeys)
	return ringsetBytes, ringSize
}

func (s *SafroleState) GenerateTickets(secret bandersnatch.BanderSnatchSecret, usedEntropy common.Hash) ([]types.Ticket, []uint32) {
	tickets := make([]types.Ticket, types.TicketEntriesPerValidator) // Pre-allocate space for tickets
	microseconds := make([]uint32, types.TicketEntriesPerValidator)
	var wg sync.WaitGroup
	var mu sync.Mutex // To synchronize access to the tickets slice

	for attempt := uint8(0); attempt < types.TicketEntriesPerValidator; attempt++ {
		wg.Add(1)

		// Launch a goroutine for each attempt
		go func(attempt uint8) {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Recovered from panic in ticket generation (attempt %d): %v\n", attempt, r)
				}
				wg.Done()
			}()

			entropy := usedEntropy
			ticket, ms, err := s.generateTicket(secret, entropy, attempt)
			if err == nil {
				// Lock to safely append to tickets
				mu.Lock()
				tickets[attempt] = ticket // Store the ticket at the index of the attempt
				microseconds[attempt] = ms
				mu.Unlock()
			} else {
				fmt.Printf("Error generating ticket for attempt %d: %v\n", attempt, err)
			}
		}(attempt) // Pass the attempt variable to avoid closure capture issues
	}

	wg.Wait() // Wait for all goroutines to finish

	return tickets, microseconds
}
func (s *SafroleState) SimulateTicket(secret bandersnatch.BanderSnatchSecret, targetEpochRandomness common.Hash, attempt uint8) (types.Ticket, uint32, error) {
	return s.generateTicket(secret, targetEpochRandomness, attempt)
}

func (s *SafroleState) generateTicket(secret bandersnatch.BanderSnatchSecret, targetEpochRandomness common.Hash, attempt uint8) (types.Ticket, uint32, error) {
	ticket_start := time.Now()
	ticket_vrf_input := ticketSealVRFInput(targetEpochRandomness, attempt)
	auxData := []byte{}

	if len(s.NextValidators) == 0 {
		return types.Ticket{}, 0, fmt.Errorf("No validators in NextValidators")
	}
	ringsetBytes, ringSize := s.GetRingSet("Next")

	signature, _, err := bandersnatch.RingVrfSign(ringSize, secret, ringsetBytes, ticket_vrf_input, auxData)
	if err != nil {
		return types.Ticket{}, 0, fmt.Errorf("signTicket failed")
	}

	var signatureArray [types.ExtrinsicSignatureInBytes]byte
	copy(signatureArray[:], signature)

	ticket := types.Ticket{
		Attempt:   uint8(attempt),
		Signature: signatureArray,
	}

	ticket_elapsed := common.Elapsed(ticket_start)
	return ticket, ticket_elapsed, nil
}

// ringVRF
func (s *SafroleState) ValidateProposedTicket(t *types.Ticket, shifted bool) (common.Hash, error) {
	if t.Attempt >= types.TicketEntriesPerValidator {
		return common.Hash{}, jamerrors.ErrTBadTicketAttemptNumber
	}

	//step 0: derive ticketVRFInput
	entroptIdx := 2
	targetEpochRandomness := s.Entropy[entroptIdx]
	isTicketSubmissionClosed := s.IsTicketSubmissionClosed(uint32(s.Timeslot))

	if isTicketSubmissionClosed || shifted {
		entroptIdx = 1
		targetEpochRandomness = s.Entropy[entroptIdx]
		ticketVRFInput := ticketSealVRFInput(targetEpochRandomness, t.Attempt)

		//step 1: verify envelope's VRFSignature using ring verifier
		//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
		ringsetBytes, ringSize := s.GetRingSet("Next")
		ticket_id, err := bandersnatch.RingVrfVerify(ringSize, ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
		if err == nil {
			return common.BytesToHash(ticket_id), nil
		}
	} else {
		ticketVRFInput := ticketSealVRFInput(targetEpochRandomness, t.Attempt)
		//step 1: verify envelope's VRFSignature using ring verifier
		//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
		ringsetBytes, ringSize := s.GetRingSet("Next")
		ticket_id, err := bandersnatch.RingVrfVerify(ringSize, ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
		if err == nil {
			return common.BytesToHash(ticket_id), nil
		} else {
			ticketID, _ := t.TicketID()
			log.Warn(module, "Failed to verify ticket", "ticket_id", ticketID, "attempt", t.Attempt, "target_epoch_randomness", targetEpochRandomness, "ERR", err)
			log.Warn(module, "Failed to verify ticket", "ticket_id", ticketID, "ticket_signature", t.String())
		}
	}

	//ticketID, _ := t.TicketID()
	//fmt.Printf("Failed to validate ticket %s | %s\n", ticketID.String(), t.String())
	return common.Hash{}, jamerrors.ErrTBadRingProof
}

// ringVRF
func (s *SafroleState) ValidateIncomingTicket(t *types.Ticket) (common.Hash, int, error) {
	if t.Attempt >= types.TicketEntriesPerValidator {
		return common.Hash{}, -1, fmt.Errorf(errExtrinsicWithMoreTicketsThanAllowed)
	}
	for i := 1; i < 3; i++ {
		targetEpochRandomness := s.Entropy[i]
		ticketVRFInput := ticketSealVRFInput(targetEpochRandomness, t.Attempt)
		//step 1: verify envelope's VRFSignature using ring verifier
		//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
		ringsetBytes, ringSize := s.GetRingSet("Next")
		ticket_id, err := bandersnatch.RingVrfVerify(ringSize, ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
		if err == nil {
			//fmt.Printf("ticket_id: %x validated using entropy[%v]\n", ticket_id, i, targetEpochRandomness)
			return common.BytesToHash(ticket_id), i, nil
		}
	}
	return common.Hash{}, -1, jamerrors.ErrTBadRingProof
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

func ConvertBanderSnatchSecret(block_author_ietf_priv []byte) (bandersnatch.BanderSnatchSecret, error) {
	//TODO: figure out a plan to standardize between bandersnatch package and types
	return bandersnatch.BytesToBanderSnatchSecret(block_author_ietf_priv)
}

func ConvertBanderSnatchPub(block_author_ietf_pub []byte) (bandersnatch.BanderSnatchKey, error) {
	//TODO: figure out a plan to standardize between bandersnatch package and types
	return bandersnatch.BytesToBanderSnatchKey(block_author_ietf_pub)
}

func (s *SafroleState) GetAuthorIndex(authorkey common.Hash, phase string) (uint16, error) {
	var validatorSet []types.Validator
	// Select the appropriate set of validators based on the phase
	switch phase {
	case "Pre": // N-1
		if len(s.PrevValidators) == 0 {
			return 0, errors.New("PrevValidators is empty")
		}
		validatorSet = s.PrevValidators
	case "Curr": // N
		if len(s.CurrValidators) == 0 {
			return 0, errors.New("CurrValidators is empty")
		}
		validatorSet = s.CurrValidators
	case "Next": // N+1
		if len(s.NextValidators) == 0 {
			return 0, errors.New("NextValidators is empty")
		}
		validatorSet = s.NextValidators
	case "Designed": // N+2
		validatorSet = s.DesignedValidators
		if len(validatorSet) == 0 {
			return 0, errors.New("DesignedValidators is empty")
		}
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

	// transform the index to uint32
	indexUint32, ok := index.(uint32)
	if !ok {
		return 0, fmt.Errorf("failed to convert index to uint32")
	}
	return indexUint32 % uint32(validatorsLen), nil
}

// Fallback is using the perspective of right now
func (s *SafroleState) GetFallbackValidator(slot_index uint32) common.Hash {
	// fallback validator has been updated
	return s.TicketsOrKeys.Keys[slot_index]
}

func (state *SafroleState) String() string {
	return types.ToJSON(state)
}

func (s *SafroleState) GetPrimaryWinningTicket(slot_index uint32) types.TicketBody {
	t_or_k := s.TicketsOrKeys
	_, currPhase := s.EpochAndPhase(slot_index)

	winning_tickets := t_or_k.Tickets
	selected_ticket := winning_tickets[currPhase]
	return *selected_ticket
}

func (s *SafroleState) GetEpochType() string {
	if len(s.TicketsOrKeys.Tickets) > 0 {
		return "safrole"
	}
	return "fallback"
}

func (s *SafroleState) GetEpochT() int {
	if len(s.TicketsOrKeys.Tickets) > 0 {
		return 1
	}
	return 0
}

// eq 59
func (s *SafroleState) IsAuthorizedBuilder(slot_index uint32, bandersnatchPub common.Hash, ticketIDs []common.Hash) (bool, common.Hash, uint8, string) {

	_, currPhase := s.EpochAndPhase(slot_index)
	//TicketsOrKeys
	t_or_k := s.TicketsOrKeys
	var mode string
	if len(t_or_k.Tickets) == types.EpochLength {
		mode = "tickets"
		winning_ticket_id := s.GetPrimaryWinningTicket(slot_index)
		for _, ticketID := range ticketIDs {
			if ticketID == winning_ticket_id.Id {
				return true, ticketID, winning_ticket_id.Attempt, "safrole author"
			}
		}
		// should not reach here if the ticket is valid
	} else if len(t_or_k.Keys) > 0 {
		//fallback mode
		mode = "keys"
		fallback_validator := s.GetFallbackValidator(currPhase)
		if fallback_validator == bandersnatchPub {
			return true, common.Hash{}, 0, "fallback author"
		}
	} else {
		log.Warn(module, "no way hit here", "slot_index", slot_index, "bandersnatchPub", bandersnatchPub.String(), "currPhase", currPhase, "t_or_k.Tickets", len(t_or_k.Tickets), "t_or_k.Keys", len(t_or_k.Keys))
		if mode == "tickets" {
			tickets_json, _ := json.Marshal(t_or_k.Tickets)
			log.Warn(module, "no way hit here", "tickets_json", string(tickets_json))
		} else if mode == "keys" {
			keys_json, _ := json.Marshal(t_or_k.Keys)
			log.Warn(module, "no way hit here", "keys_json", string(keys_json))
		}
		return false, common.Hash{}, 0, "no way hit here"
	}
	return false, common.Hash{}, 0, "not authorized"
}

func (s *SafroleState) GetGonnaAuthorSlot(first_slot_index uint32, bandersnatchPub common.Hash, ticketIDs []common.Hash) map[uint32]common.Hash {

	//TicketsOrKeys
	slotMap := make(map[uint32]common.Hash)
	t_or_k := s.TicketsOrKeys
	first_slot_index = first_slot_index - 1
	if len(t_or_k.Tickets) == types.EpochLength {
		for i := 0; i < types.EpochLength; i++ {
			winning_ticket_id := s.GetPrimaryWinningTicket(first_slot_index + uint32(i))
			for _, ticketID := range ticketIDs {
				if ticketID == winning_ticket_id.Id {
					slotMap[first_slot_index+uint32(i)] = ticketID
				}
			}
			// should not reach here if the ticket is valid
		}
	} else {
		//fallback mode
		for i := 0; i < types.EpochLength; i++ {
			fallback_validator := s.GetFallbackValidator(uint32(i))
			if fallback_validator == bandersnatchPub {
				slotMap[first_slot_index+uint32(i)] = common.Hash{}
			}
		}
	}
	return slotMap
}

func (s *SafroleState) CheckTimeSlotReady(currJCE uint32) (uint32, bool) {
	// timeslot mark
	prevEpoch, prevPhase := s.EpochAndPhase(s.GetTimeSlot())
	currEpoch, currPhase := s.EpochAndPhase(currJCE)
	if currEpoch > prevEpoch {
		return currJCE, true
	} else if currEpoch == prevEpoch && currPhase > prevPhase {
		// normal case
		return currJCE, true
	}
	return currJCE, false
}

func (s *SafroleState) CheckFirstPhaseReady(currJCE uint32) (isReady bool) {
	// timeslot mark
	phaseEnd := uint32(types.EpochLength)
	if currJCE < phaseEnd {
		//fmt.Printf("CheckFirstPhaseReady:FALSE currJCE: %d, phaseEnd: %d\n", currJCE, phaseEnd)
		return false
	}
	return true
}

func cloneSafroleState(original SafroleState) SafroleState {
	copied := SafroleState{
		Id:                          original.Id,
		OffenderState:               make([]types.Ed25519Key, len(original.OffenderState)),
		EpochFirstSlot:              original.EpochFirstSlot,
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
	copy(copied.OffenderState, original.OffenderState)
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

func (s *SafroleState) GetEpoch() uint32 {
	epoch, _ := s.EpochAndPhase(uint32(s.Timeslot))
	return uint32(epoch)
}

// statefrole_stf is the function to be tested
func (s *SafroleState) ApplyStateTransitionTickets(ctx context.Context, tickets []types.Ticket, targetJCE uint32, header types.BlockHeader, validated_tickets map[common.Hash]common.Hash) (SafroleState, error) {

	ticketBodies, err := s.ValidateSaforle(tickets, targetJCE, header, validated_tickets)
	if err != nil {
		fmt.Printf("ApplyStateTransitionTickets ValidateSafrole len(E_T)=%d | len(validated_tickets)=%d. Err=%v", len(tickets), len(validated_tickets), err)
		return *s, err
	}

	// tally existing ticketIDs
	ticketIDs := make(map[common.Hash]uint8)
	for _, a := range s.NextEpochTicketsAccumulator {
		ticketIDs[a.Id] = a.Attempt
	}

	// Process Extrinsic Tickets
	fresh_randomness, err := s.GetFreshRandomness(header.EntropySource[:])
	if err != nil {
		fmt.Printf("GetFreshRandomness ERR %v (len=%d)", err, len(header.EntropySource[:]))
		return *s, fmt.Errorf("GetFreshRandomness %v", err)
	}
	s2, _, err := s.ValidateTicketTransition(targetJCE, fresh_randomness)
	if err != nil {
		return *s, fmt.Errorf("error applying state transition safrole in MakeBlock: %v", err)
	}

	// If the ticket is valid, add it to the accumulator
	for _, ticket := range ticketBodies {
		s2.PutTicketInAccumulator(ticket)
		select {
		case <-ctx.Done():
			return *s, fmt.Errorf("ApplyStateTransitionTickets canceled")
		default:
		}
	}

	// Sort and trim tickets
	s2.SortAndTrimTickets()

	s2.Timeslot = targetJCE

	return s2, nil
}

func (s *StateDB) GetPosteriorSafroleEntropy(targetJCE uint32) (*SafroleState, error) {
	//epochChanged := s.GetSafrole().EpochChanged(targetJCE)
	// if s.posteriorSafroleEntropy != nil && !epochChanged {
	// 	return s.posteriorSafroleEntropy
	// }
	sf := s.GetSafrole()
	simulated_posteriorSafroleEntropy, _, err := sf.SimulatePostiorEntropy(targetJCE)
	if err != nil {
		return nil, fmt.Errorf("GetPosteriorSafroleEntropy SimulatePostiorEntropy %v", err)
	}
	s.posteriorSafroleEntropy = simulated_posteriorSafroleEntropy
	//log.Debug(module, "GetPosteriorSafroleEntropy", "epochChanged", epochChanged, "s.JCE", s.JamState.SafroleState.Timeslot, "targetJCE", targetJCE, "entropy", simulated_posteriorSafroleEntropy.Entropy.String())
	return simulated_posteriorSafroleEntropy, nil
}

func (s *StateDB) UnsetPosteriorEntropy() {
	s.posteriorSafroleEntropy = nil
}

func (s *SafroleState) EpochChanged(targetJCE uint32) bool {
	prevEpoch, _ := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, _ := s.EpochAndPhase(uint32(targetJCE))
	return prevEpoch != currEpoch
}

// (6.23) Simulate a Detached PostiorEntropy for Block Signing and Verification
func (s *SafroleState) SimulatePostiorEntropy(targetJCE uint32) (s2 *SafroleState, epochAdvanced bool, err error) {

	epochAdvanced = s.EpochChanged(targetJCE)
	s2 = s.Copy()
	// here we set the sealer authority
	if epochAdvanced {
		// New Epoch change! Shift entropy
		// [fresh, η0, η1, η2] - > [η0', η1', η2', η3']. η0' doesn't matter as it would be updated after STF
		s2.AdvanceEntropyAndValidator(s, common.Hash{})
	} else {
		// Epoch in progress ... no change. entropy0 is unimportant
	}
	if epochAdvanced {
		// eq 68 primary mode
		if len(s2.NextEpochTicketsAccumulator) == types.EpochLength {
			winning_tickets, err := s2.GenerateWinningMarker()
			if err != nil {
				return s2, epochAdvanced, fmt.Errorf("GenerateWinningMarker Failed: %v", err)
			}
			// do something to set this marker
			ticketsOrKeys := TicketsOrKeys{
				Tickets: winning_tickets,
			}
			s2.TicketsOrKeys = ticketsOrKeys
		} else { // eq 68 fallback mode
			chosenkeys, err := s2.ChooseFallBackValidator()
			if err != nil {
				return s2, epochAdvanced, fmt.Errorf("ChooseFallBackValidator %v", err)
			}
			ticketsOrKeys := TicketsOrKeys{
				Keys: chosenkeys,
			}
			s2.TicketsOrKeys = ticketsOrKeys
		}
		s2.NextEpochTicketsAccumulator = make([]types.TicketBody, 0)
	}
	return s2, epochAdvanced, nil
}

func (s *SafroleState) ValidateTicketTransition(targetJCE uint32, fresh_randomness_H_v common.Hash) (s2 SafroleState, epochAdvanced bool, err error) {
	prevEpoch, prevPhase := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, _ := s.EpochAndPhase(targetJCE)
	epochAdvanced = false
	if currEpoch > prevEpoch {
		// New Epoch
		epochAdvanced = true
	}
	s2 = cloneSafroleState(*s)
	// entropy phasing eq 67 & 57
	new_entropy_0 := s.ComputeCurrRandomness(fresh_randomness_H_v)
	if epochAdvanced {
		// New Epoch
		s2.AdvanceEntropyAndValidator(s, new_entropy_0)
	} else {
		// Epoch in progress
		s2.StableEntropy(s, new_entropy_0)
	}
	// in the tail slot with a full set of tickets
	if epochAdvanced {
		//this is Winning ticket eligible. Check if header has marker, if yes, verify it

		// eq 68 primary mode
		if len(s2.NextEpochTicketsAccumulator) == types.EpochLength && prevPhase >= types.TicketSubmissionEndSlot && prevEpoch+1 == currEpoch {
			// not using any entropy for primary mode
			winning_tickets, err := s2.GenerateWinningMarker()
			if err != nil {
				return s2, epochAdvanced, fmt.Errorf("GenerateWinningMarker Failed: %v", err)
			}
			expected_tickets := s.computeTicketSlotBinding(s2.NextEpochTicketsAccumulator)
			verified, err := VerifyWinningMarker([types.EpochLength]*types.TicketBody(winning_tickets), expected_tickets)
			if !verified || err != nil {
				return s2, epochAdvanced, fmt.Errorf("VerifyWinningMarker Failed:%s", err)
			}
			// do something to set this marker
			ticketsOrKeys := TicketsOrKeys{
				Tickets: winning_tickets,
			}
			s2.TicketsOrKeys = ticketsOrKeys

		} else { // eq 68 fallback mode
			chosenkeys, err := s2.ChooseFallBackValidator()
			if err != nil {
				return s2, epochAdvanced, fmt.Errorf("ChooseFallBackValidator %v", err)
			}
			ticketsOrKeys := TicketsOrKeys{
				Keys: chosenkeys,
			}
			s2.TicketsOrKeys = ticketsOrKeys
		}
		s2.NextEpochTicketsAccumulator = make([]types.TicketBody, 0)

	}

	return s2, epochAdvanced, nil
}

// this function is for validate the block input is correct or not
// should use the s3
func (s *SafroleState) ValidateSaforle(tickets []types.Ticket, targetJCE uint32, header types.BlockHeader, valid_tickets map[common.Hash]common.Hash) ([]types.TicketBody, error) {
	if valid_tickets == nil {
		valid_tickets = make(map[common.Hash]common.Hash)
	}
	prevEpoch, _ := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, currPhase := s.EpochAndPhase(targetJCE)
	s2 := cloneSafroleState(*s) // CHECK: why cloning here?
	if currPhase >= types.TicketSubmissionEndSlot && len(tickets) > 0 {
		return nil, jamerrors.ErrTEpochLotteryOver
	}
	if s.Timeslot >= targetJCE {
		return nil, jamerrors.ErrTTimeslotNotMonotonic
	}
	new_entropy_0 := common.Hash{} // use empty hash as entropy 0, since it's not useful in validation
	isShifted := false
	if currEpoch > prevEpoch {
		// New Epoch
		isShifted = true
	} else {
		// Epoch in progress
		s2.StableEntropy(s, new_entropy_0)
	}
	ticketBodies := make([]types.TicketBody, 0) // n
	var wg sync.WaitGroup
	errCh := make(chan error, len(tickets))
	var ticketMutex sync.Mutex
	for _, t := range tickets {
		wg.Add(1)

		go func(t types.Ticket) {
			defer wg.Done()
			if t.Attempt >= types.TicketEntriesPerValidator {
				errCh <- jamerrors.ErrTBadTicketAttemptNumber
				return
			}
			// see if ticket exists in valid_tickets
			ticket_hash := t.Hash()
			ticket_id, ticketWasValid := valid_tickets[ticket_hash]
			var err error
			if !ticketWasValid {
				ticket_id, err = s2.ValidateProposedTicket(&t, isShifted)
				if err != nil {
					errCh <- jamerrors.ErrTBadRingProof
					return
				}
				ticketMutex.Lock()
				valid_tickets[ticket_hash] = ticket_id
				ticketMutex.Unlock()
			}

			for _, a := range s2.NextEpochTicketsAccumulator {
				if ticket_id == a.Id {
					errCh <- jamerrors.ErrTTicketAlreadyInState
					return
				}

			}
		}(t)
	}
	wg.Wait()

	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	default:
		// No error
	}
	for _, t := range tickets {
		ticketId, ok := valid_tickets[t.Hash()]
		if !ok {
			return nil, fmt.Errorf("ticket not found in valid_tickets")
		}
		ticketBody := types.TicketBody{
			Id:      ticketId,
			Attempt: t.Attempt,
		}
		ticketBodies = append(ticketBodies, ticketBody)
	}
	// check ticketBodies sorted by Id
	// use bytes to compare
	for i, a := range ticketBodies {
		if i > 0 && compareTickets(a.Id, ticketBodies[i-1].Id) < 0 {
			return nil, jamerrors.ErrTTicketsBadOrder
		}
	}
	return ticketBodies, nil
}

func (s2 *SafroleState) AdvanceEntropyAndValidator(s *SafroleState, new_entropy_0 common.Hash) {
	s2.PrevValidators = s2.CurrValidators
	s2.CurrValidators = s2.NextValidators
	if types.GetValidatorsLength(s2.NextValidators) == 0 {
		log.Crit(module, "no NextValidators")
		return
	}
	s2.NextValidators = s2.CleanValidators(s2.DesignedValidators)
	if types.GetValidatorsLength(s2.DesignedValidators) == 0 {
		log.Crit(module, "no NextValidators")
		return
	}
	//prev_n0 := s2.Entropy[0]
	s2.Entropy[1] = s.Entropy[0]
	s2.Entropy[2] = s.Entropy[1]
	s2.Entropy[3] = s.Entropy[2]
	s2.Entropy[0] = new_entropy_0

}

func (s2 *SafroleState) CleanValidators(validators types.Validators) types.Validators {
	offender_set := s2.OffenderState
	if len(offender_set) == 0 {
		return validators
	}
	validators_copy := make(types.Validators, len(validators))
	copy(validators_copy, validators)
	for i, v := range validators_copy {
		for _, o := range offender_set {
			if v.Ed25519 == o {
				validators_copy[i] = types.Validator{}
			}
		}
	}
	return validators_copy
}

func (s2 *SafroleState) StableEntropy(s *SafroleState, new_entropy_0 common.Hash) {
	s2.Entropy[0] = new_entropy_0
	s2.Entropy[1] = s.Entropy[1]
	s2.Entropy[2] = s.Entropy[2]
	s2.Entropy[3] = s.Entropy[3]

}

func (s2 *SafroleState) PutTicketInAccumulator(newa types.TicketBody) {
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

func SortTicketBodies(tickets []types.TicketBody) {
	// Sort tickets using compareTickets
	sort.SliceStable(tickets, func(i, j int) bool {
		return compareTickets(tickets[i].Id, tickets[j].Id) < 0
	})
}

func TicketInTmpAccumulator(ticketID common.Hash, tmpAccumulator []types.TicketBody) bool {
	for _, a := range tmpAccumulator {
		if ticketID == a.Id {
			return true
		}
	}
	return false
}

func TrimTicketBodies(tickets []types.TicketBody) []types.TicketBody {
	// Drop useless tickets
	if len(tickets) > types.EpochLength {
		tickets = tickets[0:types.EpochLength]
	}
	return tickets
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
		using_entropy := s.Entropy[2]
		authority_idx, err := s.computeFallbackAuthorityIndex(using_entropy, uint32(i), len(s.CurrValidators))
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
	Tau           uint32             `json:"tau"`
	Eta           Entropy            `json:"eta"`
	Lambda        types.Validators   `json:"lambda"`
	Kappa         types.Validators   `json:"kappa"`
	GammaK        types.Validators   `json:"gamma_k"`
	Iota          types.Validators   `json:"iota"`
	GammaA        []types.TicketBody `json:"gamma_a"`
	GammaS        TicketsOrKeys      `json:"gamma_s"`
	GammaZ        [144]byte          `json:"gamma_z"`
	PostOffenders []types.Ed25519Key `json:"post_offenders"`
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
		Lambda types.Validators   `json:"lambda"`
		Kappa  types.Validators   `json:"kappa"`
		GammaK types.Validators   `json:"gamma_k"`
		Iota   types.Validators   `json:"iota"`
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
		Lambda types.Validators   `json:"lambda"`
		Kappa  types.Validators   `json:"kappa"`
		GammaK types.Validators   `json:"gamma_k"`
		Iota   types.Validators   `json:"iota"`
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
func SortTicketsById(tickets []types.Ticket) {
	sort.SliceStable(tickets, func(i, j int) bool {
		a_id, _ := tickets[i].TicketID()
		b_id, _ := tickets[j].TicketID()
		return compareTickets(a_id, b_id) < 0
	})
}

// shawn: this function is for safrole ultimate verfication
// ver 0.6.2
func VerifySafroleSTF(old_sf_origin *SafroleState, new_sf_origin *SafroleState, block_origin *types.Block) error {
	// use the copy of the state to be safe
	old_sf := old_sf_origin.Copy()
	new_sf := new_sf_origin.Copy()
	block := block_origin.Copy()
	// 6.1
	slot_header := block.Header.Slot
	new_slot := new_sf.Timeslot
	if slot_header != new_slot {
		return fmt.Errorf("slot_header != new_slot")
	}
	header := block.Header
	// 6.2
	old_epoch, old_phase := old_sf.EpochAndPhase(uint32(old_sf.Timeslot))
	new_epoch, new_phase := new_sf.EpochAndPhase(uint32(new_sf.Timeslot))
	is_epoch_changed := old_epoch < new_epoch
	// 6.22
	fresh_randomness, err := old_sf.GetFreshRandomness(header.EntropySource[:])
	if err != nil {
		return fmt.Errorf("GetFreshRandomness %v", err)
	}
	new_entropy_0 := old_sf.ComputeCurrRandomness(fresh_randomness)
	new_entropy := new_sf.Entropy
	if !reflect.DeepEqual(new_entropy[0], new_entropy_0) {
		return fmt.Errorf("new_entropy[0] mismatch")
	}
	if !is_epoch_changed {
		// 6.27
		//6.13
		// TODO offender
		switch {
		case !reflect.DeepEqual(new_sf.CurrValidators, old_sf.CurrValidators):
			return fmt.Errorf("CurrValidators mismatch")
		case !reflect.DeepEqual(new_sf.PrevValidators, old_sf.PrevValidators):
			return fmt.Errorf("PrevValidators mismatch")
		case !reflect.DeepEqual(new_sf.DesignedValidators, old_sf.DesignedValidators):
			return fmt.Errorf("DesignedValidators mismatch")
		case !reflect.DeepEqual(new_sf.NextValidators, old_sf.NextValidators):
			return fmt.Errorf("NextValidators mismatch")
		}
		// entropy
		// 6.23
		old_entropy := old_sf.Entropy
		new_entropy := new_sf.Entropy
		switch {
		case old_entropy[1] != new_entropy[1]:
			return fmt.Errorf("Entropy[1] mismatch")
		case old_entropy[2] != new_entropy[2]:
			return fmt.Errorf("Entropy[2] mismatch")
		case old_entropy[3] != new_entropy[3]:
			return fmt.Errorf("Entropy[3] mismatch")
		}

	} else { // !!! here is epoch changed case
		if block.Header.EpochMark == nil {
			return fmt.Errorf("EpochMark is nil")
		}
		//6.13
		switch {
		case !reflect.DeepEqual(new_sf.PrevValidators, old_sf.CurrValidators):
			return fmt.Errorf("PrevValidators mismatch")
		case !reflect.DeepEqual(new_sf.CurrValidators, old_sf.NextValidators):
			return fmt.Errorf("CurrValidators mismatch")
		case !reflect.DeepEqual(new_sf.NextValidators, old_sf.DesignedValidators):
			return fmt.Errorf("NextValidators mismatch")
		}
		// entropy
		// 6.23
		old_entropy := old_sf.Entropy
		new_entropy := new_sf.Entropy
		switch {
		case old_entropy[0] != new_entropy[1]:
			return fmt.Errorf("Entropy[1] mismatch")
		case old_entropy[1] != new_entropy[2]:
			return fmt.Errorf("Entropy[2] mismatch")
		case old_entropy[2] != new_entropy[3]:
			return fmt.Errorf("Entropy[3] mismatch")
		}
	}
	// gamma s verification
	old_gamma_a := old_sf.NextEpochTicketsAccumulator
	if new_epoch == old_epoch+1 && old_phase > types.TicketSubmissionEndSlot && len(old_gamma_a) == types.EpochLength {
		new_gamma_s := new_sf.TicketsOrKeys
		winning_tickets, err := old_sf.GenerateWinningMarker()
		if err != nil {
			return fmt.Errorf("GenerateWinningMarker Failed: %v", err)
		}
		ticketsOrKeys := TicketsOrKeys{
			Tickets: winning_tickets,
		}
		if !reflect.DeepEqual(new_gamma_s, ticketsOrKeys) {
			return fmt.Errorf("TicketsOrKeys mismatch")
		}

	} else if !is_epoch_changed {
		old_gamma_s := old_sf.TicketsOrKeys
		new_gamma_s := new_sf.TicketsOrKeys
		if !reflect.DeepEqual(old_gamma_s, new_gamma_s) {
			return fmt.Errorf("TicketsOrKeys mismatch: old: %v new: %v", old_gamma_s, new_gamma_s)
		}
	} else {
		// fallback mode
		new_gamma_s := new_sf.TicketsOrKeys
		// we use n3' so it has to be after we check the entropy state transition
		choosenkeys, err := new_sf.ChooseFallBackValidator()
		if err != nil {
			return fmt.Errorf("ChooseFallBackValidator %v", err)
		}
		ticketsOrKeys := TicketsOrKeys{
			Keys: choosenkeys,
		}
		if !reflect.DeepEqual(new_gamma_s, ticketsOrKeys) {
			return fmt.Errorf("TicketsOrKeys mismatch")
		}

	}
	//6.28
	if !is_epoch_changed && old_phase < types.TicketSubmissionEndSlot && new_phase >= types.TicketSubmissionEndSlot && len(old_gamma_a) == types.EpochLength {
		// winning ticket mark required
		winning_ticket_mark := block.Header.TicketsMark
		expect_winning_ticket, err := old_sf.GenerateWinningMarker()
		if err != nil {
			return fmt.Errorf("GenerateWinningMarker Failed: %v", err)
		}
		for index, t := range expect_winning_ticket {
			if t.Id != winning_ticket_mark[index].Id {
				return fmt.Errorf("winning ticket mark mismatch")
			}
		}
	}
	// 6.15~6.20
	blockSealEntropy := new_sf.Entropy[3]
	var c []byte
	if new_sf.GetEpochT() == 1 {
		winning_ticket := (new_sf.TicketsOrKeys.Tickets)[new_phase]
		c = ticketSealVRFInput(blockSealEntropy, uint8(winning_ticket.Attempt))
	} else {
		c = append([]byte(types.X_F), blockSealEntropy.Bytes()...)
	}

	// H_s Verification (6.15/6.16)
	H_s := header.Seal[:]
	m := header.BytesWithoutSig()
	// author_idx is the K' so we use the sf_tmp
	validatorIdx := block.Header.AuthorIndex
	signing_validator := new_sf.GetCurrValidator(int(validatorIdx))
	block_author_ietf_pub := bandersnatch.BanderSnatchKey(signing_validator.GetBandersnatchKey())
	vrfOutput, err := bandersnatch.IetfVrfVerify(block_author_ietf_pub, H_s, c, m)
	if err != nil {
		log.Error(module, "IetfVrfVerify Failed", "validatorIdx", validatorIdx, "block_author_ietf_pub", block_author_ietf_pub.String(), "H_s(seal)", H_s, "c", c, "m(headerWithoutSig)", m, "err", err)
		log.Error(module, "IetfVrfVerify Failed (Extra)", "slot", slot_header, "type", new_sf.GetEpochT(), "validatorIdx", validatorIdx, "blockSealEntropy n[3]", blockSealEntropy, "Entropy", new_sf.Entropy)
		return fmt.Errorf("VerifyBlockHeader Failed: H_s Verification")
	} else {
		log.Error(module, "IetfVrfVerify OK (H_s)", "slot", slot_header, "type", new_sf.GetEpochT(), "validatorIdx", validatorIdx, "blockSealEntropy n[3]", blockSealEntropy, "Entropy", new_sf.Entropy)
	}
	// H_v Verification (6.17)
	H_v := header.EntropySource[:]
	c = append([]byte(types.X_E), vrfOutput...)
	_, err = bandersnatch.IetfVrfVerify(block_author_ietf_pub, H_v, c, []byte{})
	if err != nil {
		return fmt.Errorf("VerifyBlockHeader Failed: H_v Verification")
	}
	// 6.33
	tickets := block.Extrinsic.Tickets
	for _, block_ticket := range tickets {
		for _, a := range old_gamma_a {
			block_ticket_id, _ := block_ticket.TicketID()
			if block_ticket_id == a.Id {
				return fmt.Errorf("ticket already in state")
			}
		}
	}
	// useless key shouldn't be included in a block
	for _, block_ticket := range tickets {
		found := false
		for _, a := range new_sf.NextEpochTicketsAccumulator {
			block_ticket_id, _ := block_ticket.TicketID()
			if block_ticket_id == a.Id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("ticket not in state")
		}
	}
	gamma_a_len := len(new_sf.NextEpochTicketsAccumulator)
	if gamma_a_len > types.EpochLength {
		return fmt.Errorf("too many tickets in accumulator")
	}
	for i := 1; i < gamma_a_len; i++ {
		if compareTickets(new_sf.NextEpochTicketsAccumulator[i].Id, new_sf.NextEpochTicketsAccumulator[i-1].Id) < 0 {
			return fmt.Errorf("tickets not sorted")
		}
	}
	// tickets verification
	// using n2'
	for _, t := range tickets {
		targetEpochRandomness := new_sf.Entropy[2]
		ticketVRFInput := ticketSealVRFInput(targetEpochRandomness, t.Attempt)
		ringsetBytes, ringSize := old_sf.GetRingSet("Next")
		_, err := bandersnatch.RingVrfVerify(ringSize, ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
		if err != nil {
			return fmt.Errorf("VRFSignature verification failed")
		}
	}
	return nil
}
