package statedb

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/types"

	//"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/colorfulnotion/jam/common"
	"golang.org/x/crypto/blake2b"
	//"github.com/colorfulnotion/jam/trie"
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
type Input struct {
	Slot       uint32      `json:"slot"`
	Entropy    common.Hash `json:"entropy"`
	Extrinsics []Extrinsic `json:"extrinsics"`
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
		GammaZ: nextRingCommitment,
		GammaS: s.TicketsOrKeys,
		GammaA: s.NextEpochTicketsAccumulator,
	}
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

type Entropy []common.Hash
type Validators []types.Validator

type SafroleState struct {
	Id             uint32 `json:"Id"`
	EpochFirstSlot uint32 `json:"EpochFirstSlot"`
	Epoch          uint32 `json:"epoch"`

	TimeStamp   int    `json:"timestamp"`
	Timeslot    uint32 `json:"timeslot"`
	BlockNumber int    `json:"blockNumber"`
	// Entropy holds 4 32 byte
	// Entropy[0] CURRENT randomness accumulator (see sec 6.10). randomness_buffer[0] = BLAKE2(CONCAT(randomness_buffer[0], fresh_randomness));
	// where fresh_randomness = vrf_signed_output(claim.randomness_source)

	// Entropy[1] accumulator snapshot BEFORE the execution of the first block of epoch N   - randomness used for ticket targeting epoch N+2
	// Entropy[2] accumulator snapshot BEFORE the execution of the first block of epoch N-1 - randomness used for ticket targeting epoch N+1
	// Entropy[3] accumulator snapshot BEFORE the execution of the first block of epoch N-2 - randomness used for ticket targeting current epoch N
	Entropy Entropy `json:"entropy"`

	// 4 authorities[pre, curr, next, designed]
	PrevValidators     Validators `json:"prev_validators"`
	CurrValidators     Validators `json:"curr_validators"`
	NextValidators     Validators `json:"next_validators"`
	DesignedValidators Validators `json:"designed_validators"`

	// Accumulator of tickets, modified with Extrinsics to hold ORDERED array of Tickets
	NextEpochTicketsAccumulator []types.TicketBody `json:"next_tickets_accumulator"` //gamma_a
	TicketsAccumulator          []types.TicketBody `json:"tickets_accumulator"`
	TicketsOrKeys               TicketsOrKeys      `json:"tickets_or_keys"`

	// []bandersnatch.ValidatorKeySet
	TicketsVerifierKey []byte `json:"tickets_verifier_key"`
}

func NewSafroleState() *SafroleState {
	return &SafroleState{
		Id:                 99999,
		Timeslot:           uint32(ComputeCurrentJCETime()),
		BlockNumber:        0,
		Entropy:            make([]common.Hash, 4),
		PrevValidators:     []types.Validator{},
		CurrValidators:     []types.Validator{},
		NextValidators:     []types.Validator{},
		DesignedValidators: []types.Validator{},
		TicketsAccumulator: []types.TicketBody{},
		TicketsOrKeys: TicketsOrKeys{
			Keys: []common.Hash{},
			// Tickets: []TicketBody{}, // Uncomment if you need to initialize tickets instead
		},
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
		nextValidators[i] = v.GetBandersnatchKey()
	}
	fmt.Printf("nextValidators Len=%v\n", nextValidators)
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
	var ticketsMark []*types.TicketBody
	for idx, ticket := range outsidein_tickets {
		ticketsMark[idx] = ticket
	}
	return ticketsMark, nil
}

type Output struct {
	Ok *struct {
		EpochMark   *types.EpochMark    `json:"epoch_mark"`
		TicketsMark []*types.TicketBody `json:"tickets_mark"`
	} `json:"ok"`
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

func (s *SafroleState) EpochAndPhase(currCJE uint32) (currentEpoch int32, currentPhase uint32) {
	if currCJE < s.EpochFirstSlot {
		currentEpoch = -1
		currentPhase = 0
		return
	}
	currentEpoch = int32((currCJE - s.EpochFirstSlot) / (types.SecondsPerSlot * types.EpochLength)) // eg. / 60
	currentPhase = ((currCJE - s.EpochFirstSlot) % (types.SecondsPerSlot * types.EpochLength)) / types.SecondsPerSlot
	return
}

func (s *SafroleState) IsNewEpoch(currCJE uint32) bool {
	prevEpoch, _ := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, _ := s.EpochAndPhase(currCJE)
	return currEpoch > prevEpoch
}

// used for detecting the end of submission period
func (s *SafroleState) IsTicketSubmissionCloses(currCJE uint32) bool {
	//prevEpoch, _ := s.EpochAndPhase(uint32(s.Timeslot))
	_, currPhase := s.EpochAndPhase(currCJE)
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

func (s *SafroleState) computeTicketID(authority_secret_key bandersnatch.PrivateKey, ticket_vrf_input []byte) (common.Hash, error) {
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
	s.Entropy = entropy
	return nil
}

func (s *SafroleState) GetCurrEpochFirst() uint32 {
	return s.EpochFirstSlot
}

func (s *SafroleState) GetNextEpochFirst() uint32 {
	nextEpochFirstSlot := s.EpochFirstSlot + types.EpochLength
	return nextEpochFirstSlot
}

func (s *SafroleState) GetTimeSlot() uint32 {
	return uint32(s.Timeslot)
}

func (s *SafroleState) GetEpochValidatorAndRandomness(phase string) ([]types.Validator, common.Hash) {
	// 4 authorities[pre, curr, next, designed]
	// η0 Entropy[0] CURRENT randomness accumulator (see sec 6.10). randomness_buffer[0] = BLAKE2(CONCAT(randomness_buffer[0], fresh_randomness));
	// η1 Entropy[1] accumulator snapshot BEFORE the execution of the first block of epoch N   - randomness used for ticket targeting epoch N+2
	// η2 Entropy[2] accumulator snapshot BEFORE the execution of the first block of epoch N-1 - randomness used for ticket targeting epoch N+1
	// η3 Entropy[3] accumulator snapshot BEFORE the execution of the first block of epoch N-2 - randomness used for ticket targeting current epoch N
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
	pubkeys := []bandersnatch.PublicKey{}
	for _, v := range validatorSet {
		//pubkeys = append(pubkeys, v.Bandersnatch.Bytes())
		pubkeys = append(pubkeys, common.ConvertToSlice(v.Bandersnatch))
	}
	ringsetBytes = bandersnatch.InitRingSet(pubkeys)
	return ringsetBytes
}

func (s *SafroleState) GenerateTickets(secret bandersnatch.PrivateKey, isNextEpoch bool) []types.Ticket {
	tickets := make([]types.Ticket, 0)
	for attempt := uint8(0); attempt < types.TicketEntriesPerValidator; attempt++ {
		// We can GenerateTickets for the NEXT epoch based on s.Entropy[1], but the CURRENT epoch based on s.Entropy[2]
		entropy := s.Entropy[2]
		if isNextEpoch {
			entropy = s.Entropy[1]
		}
		ticket, err := s.generateTicket(secret, entropy, attempt)
		if err == nil {
			fmt.Printf("[N%d] Generated ticket %d: %v\n", s.Id, attempt, entropy)
			tickets = append(tickets, ticket)
		} else {
			fmt.Printf("Error generating ticket for attempt %d: %v\n", attempt, err)
		}
	}
	return tickets
}

func (s *SafroleState) generateTicket(secret bandersnatch.PrivateKey, targetEpochRandomness common.Hash, attempt uint8) (types.Ticket, error) {
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

// 6.6. On-chain Tickets Validation
func (s *SafroleState) validateTicket_test_vector(targetEpochRandomness common.Hash, envelope *TicketEnvelope) (common.Hash, error) {

	//step 0: derive ticketVRFInput
	ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, envelope.Attempt)

	//step 1: verify envelope's VRFSignature using ring verifier
	//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
	ringsetBytes := s.GetRingSet("Designed")
	ticket_id, err := bandersnatch.RingVrfVerify(ringsetBytes, envelope.RingSignature[:], ticketVRFInput, envelope.Extra)
	if err != nil {
		return common.Hash{}, fmt.Errorf("Bad signature")
	}

	return common.BytesToHash(ticket_id), nil
}

func (s *SafroleState) ValidateProposedTicket(t *types.Ticket) (common.Hash, error) {
	if t.Attempt >= types.TicketEntriesPerValidator {
		return common.Hash{}, fmt.Errorf(errExtrinsicWithMoreTicketsThanAllowed)
	}

	//step 0: derive ticketVRFInput
	for i := 2; i <= 3; i++ {
		targetEpochRandomness := s.Entropy[i]
		ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, t.Attempt)

		//step 1: verify envelope's VRFSignature using ring verifier
		//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
		ringsetBytes := s.GetRingSet("Next")
		ticket_id, err := bandersnatch.RingVrfVerify(ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
		if err == nil {
			//fmt.Printf("[N%d] ValidateProposed Ticket Succ %v %d:%v\n", s.Id, t.TicketID(), i, targetEpochRandomness)
			return common.BytesToHash(ticket_id), nil
		}
	}
	fmt.Printf("[N%d] ValidateProposed Ticket Fail %v 0:%v 1:%v 2:%v 3:%v\n", s.Id, t.TicketID(), s.Entropy[0], s.Entropy[1], s.Entropy[2], s.Entropy[3])
	return common.Hash{}, fmt.Errorf(errTicketBadRingProof)
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

// queueTicketEnvelope adds a ticket envelope to the list and sorts it
func (s *SafroleState) QueueTicketEnvelope(envelope *TicketEnvelope) error {
	//return nil
	ticket_id, err := s.validateTicket_test_vector(s.Entropy[0], envelope)
	if err != nil {
		return err
	}
	// Add envelope to s.TicketsAccumulator as "t"
	t := types.TicketBody{
		Id:      ticket_id,
		Attempt: uint8(envelope.Attempt),
	}
	s.TicketsAccumulator = append(s.TicketsAccumulator, t)

	return nil

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

func (s *SafroleState) SignPrimary(authority_secret_key bandersnatch.PrivateKey, unsignHeaderHash common.Hash, attempt uint8) ([]byte, []byte, error) {
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
	fmt.Printf("SignPrimary unsignHeaderHash=%v, blockSeal=%x, fresh_VRFSignature=%x, freshRandomness=%x\n", unsignHeaderHash, blockSeal, fresh_VRFSignature, freshRandomness)
	return blockSeal, fresh_VRFSignature, nil
}

// 6.8.1 Primary Method for legit ticket - is it bare VRF here???
func (s *SafroleState) SignFallBack(authority_secret_key bandersnatch.PrivateKey, unsignHeaderHash common.Hash) ([]byte, []byte, error) {
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
	fmt.Printf("SignFallBack unsignHeaderHash=%v, blockSeal=%x, fresh_VRFSignature=%x, freshRandomness=%x\n", unsignHeaderHash, blockSeal, fresh_VRFSignature, freshRandomness)
	return blockSeal, fresh_VRFSignature, nil
}

func (s *SafroleState) SignBlockSeal(authority_secret_key bandersnatch.PrivateKey, sealVRFInput []byte, unsignHeaderHash common.Hash) ([]byte, []byte, error) {
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
		if v.GetBandersnatchKey() == authorkey {
			return uint16(authorIdx), nil
		}
	}
	return 0, errors.New("author key not found")
}

// computeAuthorityIndex computes the authority index for claiming an orphan slot
func (s *SafroleState) computeFallbackAuthorityIndex(targetRandomness common.Hash, relativeSlotIndex uint32, validatorsLen int) (uint32, error) {
	// Concatenate target_randomness and relative_slot_index
	hashInput := append(targetRandomness.Bytes(), common.Uint32ToBytes(relativeSlotIndex)...)

	// Compute BLAKE2 hash
	hash := common.ComputeHash(hashInput)

	// Extract the first 4 bytes of the hash to compute the index
	indexBytes := hash[:4]
	index := binary.BigEndian.Uint32(indexBytes) % uint32(validatorsLen) //Check
	//fmt.Printf("targetRandomness=%x, relativeSlotIndex=%v, indexBytes=%x, priv_index=%v. (authLen=%v)", targetRandomness, relativeSlotIndex, indexBytes, index, len(s.Authorities))
	return index, nil
}

// Fallback is using the perspective of right now
func (s *SafroleState) GetFallbackValidator(slot_index uint32) common.Hash {
	// Entropy[0] CURRENT randomness accumulator (see sec 6.10). randomness_buffer[0] = BLAKE2(CONCAT(randomness_buffer[0], fresh_randomness));
	// Entropy[1] accumulator snapshot BEFORE the execution of the first block of epoch N   - randomness used for ticket targeting epoch N+2
	// Entropy[2] accumulator snapshot BEFORE the execution of the first block of epoch N-1 - randomness used for ticket targeting epoch N+1
	// Entropy[3] accumulator snapshot BEFORE the execution of the first block of epoch N-2 - randomness used for ticket targeting current epoch N
	targetRandomness := s.GetProposedTargetEpochRandomness() //TODO check
	relativeSlotIndex, _ := s.GetRelativeSlotIndex(slot_index)
	authority_idx, _ := s.computeFallbackAuthorityIndex(targetRandomness, relativeSlotIndex, len(s.CurrValidators))
	valdator := s.CurrValidators[authority_idx]
	//fmt.Printf("Fallback slot_index=%v relativeSlotIdx=%v, Validator=%v (authority_idx=%v)\n", slot_index, relativeSlotIndex, valdator.GetBandersnatchKey(), authority_idx)
	return valdator.GetBandersnatchKey()
}

func (s *SafroleState) GetPrimaryWinningTicket(slot_index uint32) common.Hash {
	t_or_k := s.TicketsOrKeys
	winning_tickets := t_or_k.Tickets
	_, currPhase := s.EpochAndPhase(slot_index)
	selected_ticket := winning_tickets[currPhase]
	return selected_ticket.Id
}

func (s *SafroleState) GetProposedTargetEpochRandomness() common.Hash {
	targetRandomness := s.Entropy[3]
	return targetRandomness
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
		winning_ticket := t_or_k.Tickets[currPhase]
		return uint8(winning_ticket.Attempt), nil
	}
	return 0, fmt.Errorf("Shouldn't be fallback")
}

func (s *SafroleState) IsAuthorizedBuilder(slot_index uint32, bandersnatchPub common.Hash, ticketIDs []common.Hash) bool {
	/*
	   currEpoch, currPhase := s.EpochAndPhase(slot_index)
	   //TicketsOrKeys
	   t_or_k := s.TicketsOrKeys

	   	if len(t_or_k.Tickets) == EpochNumSlots {
	   		winning_ticket_id := s.GetPrimaryWinningTicket(slot_index)
	   		for _, ticketID := range ticketIDs {
	   			if ticketID == winning_ticket_id {
	   				fmt.Printf("[AUTHORIZED] (%v, %v) slot_index=%v primary validator=%v\n", currEpoch, currPhase, slot_index, bandersnatchPub)
	   				return true
	   			}
	   		}
	   		//full complement of E tickets
	   		//TODO: where should we keep the info where s has signed the ticekts???
	   	} else {

	   		//fallback mode
	   		fallback_validator := s.GetFallbackValidator(slot_index)
	   		if fallback_validator == bandersnatchPub {
	   			fmt.Printf("[AUTHORIZED] (%v, %v) slot_index=%v fallback validator=%v\n", currEpoch, currPhase, slot_index, bandersnatchPub)
	   			return true
	   		}
	   	}
	*/
	return false
}

func (s *SafroleState) CheckTimeSlotReady() (uint32, bool) {
	currJCE := ComputeCurrentJCETime()
	prevEpoch, prevPhase := s.EpochAndPhase(s.GetTimeSlot())
	currEpoch, currPhase := s.EpochAndPhase(currJCE)
	fmt.Printf("[N%d] CheckTimeSlotReady PREV [%d] %d %d Curr [%d] %d %d\n",
		s.Id, s.GetTimeSlot(), prevEpoch, prevPhase,
		currJCE, currEpoch, currPhase)
	if currEpoch > prevEpoch {
		return currJCE, true
	} else if currEpoch == prevEpoch && currPhase > prevPhase {
		return currJCE, true
	}
	return currJCE, false
}

func (s *SafroleState) CheckGenesisReady() (isReady bool) {
	if s.BlockNumber == 0 {
		currJCE := ComputeCurrentJCETime()
		if currJCE < s.EpochFirstSlot {
			fmt.Printf("Not ready currJCE: %v < epoch0TimeSlot %v\n", currJCE, s.EpochFirstSlot)
			return false
		}
	}
	return true
}

func (s *SafroleState) validateExtrinsic(e Extrinsic) (*TicketEnvelope, error) {
	ticket_id, err := bandersnatch.VRFSignedOutput(e.Signature[:])
	if err != nil {
		return nil, fmt.Errorf("invalid ticket_id err=%v", err)
	}
	return &TicketEnvelope{
		Id:            common.BytesToHash(ticket_id),
		Attempt:       e.Attempt,
		Extra:         []byte{},
		RingSignature: e.Signature,
	}, nil
}

func cloneSafroleState(original SafroleState) SafroleState {
	copied := SafroleState{
		Id:                          original.Id,
		EpochFirstSlot:              original.EpochFirstSlot,
		Epoch:                       original.Epoch,
		TimeStamp:                   original.TimeStamp,
		Timeslot:                    original.Timeslot,
		BlockNumber:                 original.BlockNumber,
		Entropy:                     make([]common.Hash, len(original.Entropy)),
		PrevValidators:              make([]types.Validator, len(original.PrevValidators)),
		CurrValidators:              make([]types.Validator, len(original.CurrValidators)),
		NextValidators:              make([]types.Validator, len(original.NextValidators)),
		DesignedValidators:          make([]types.Validator, len(original.DesignedValidators)),
		NextEpochTicketsAccumulator: make([]types.TicketBody, len(original.NextEpochTicketsAccumulator)),
		TicketsAccumulator:          make([]types.TicketBody, len(original.TicketsAccumulator)),
		TicketsOrKeys:               original.TicketsOrKeys,
		TicketsVerifierKey:          make([]byte, len(original.TicketsVerifierKey)),
	}

	// Copy each field individually
	copy(copied.Entropy, original.Entropy)
	copy(copied.PrevValidators, original.PrevValidators)
	copy(copied.CurrValidators, original.CurrValidators)
	copy(copied.NextValidators, original.NextValidators)
	copy(copied.DesignedValidators, original.DesignedValidators)
	copy(copied.NextEpochTicketsAccumulator, original.NextEpochTicketsAccumulator)
	copy(copied.TicketsAccumulator, original.TicketsAccumulator)
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
		BlockNumber:                 original.BlockNumber,
		Entropy:                     make([]common.Hash, len(original.Entropy)),
		PrevValidators:              make([]types.Validator, len(original.PrevValidators)),
		CurrValidators:              make([]types.Validator, len(original.CurrValidators)),
		NextValidators:              make([]types.Validator, len(original.NextValidators)),
		DesignedValidators:          make([]types.Validator, len(original.DesignedValidators)),
		NextEpochTicketsAccumulator: make([]types.TicketBody, len(original.NextEpochTicketsAccumulator)),
		TicketsAccumulator:          make([]types.TicketBody, len(original.TicketsAccumulator)),
		TicketsOrKeys:               original.TicketsOrKeys, // Assuming this has value semantics
		TicketsVerifierKey:          make([]byte, len(original.TicketsVerifierKey)),
	}

	// Copy the Entropy slice
	copy(copyState.Entropy, original.Entropy)

	// Copy the PrevValidators slice
	for i, v := range original.PrevValidators {
		copyState.PrevValidators[i] = v // Assuming Validator is a value type, or use v.Copy() if it's a pointer
	}

	// Copy the CurrValidators slice
	for i, v := range original.CurrValidators {
		copyState.CurrValidators[i] = v // Same assumption as above
	}

	// Copy the NextValidators slice
	for i, v := range original.NextValidators {
		copyState.NextValidators[i] = v // Same assumption as above
	}

	// Copy the DesignedValidators slice
	for i, v := range original.DesignedValidators {
		copyState.DesignedValidators[i] = v // Same assumption as above
	}

	// Copy the NextEpochTicketsAccumulator slice
	for i, v := range original.NextEpochTicketsAccumulator {
		copyState.NextEpochTicketsAccumulator[i] = v // Assuming TicketBody is a value type, or use v.Copy() if it's a pointer
	}

	// Copy the TicketsAccumulator slice
	for i, v := range original.TicketsAccumulator {
		copyState.TicketsAccumulator[i] = v // Same assumption as above
	}

	// Copy the TicketsVerifierKey slice
	copy(copyState.TicketsVerifierKey, original.TicketsVerifierKey)

	return copyState
}

func (s *SafroleState) AdvanceSafrole(targetJCE uint32) error {
	prevJCE := s.GetTimeSlot()
	if prevJCE < targetJCE {
		return fmt.Errorf(errTimeslotNotMonotonic)
	}
	currEpochFirst := s.GetCurrEpochFirst()
	nextEpochFirst := s.GetNextEpochFirst()
	if currEpochFirst > targetJCE {
		//not ready to advance
		return fmt.Errorf(errEpochFirstSlotNotMet)
	}

	if targetJCE >= nextEpochFirst {
		// New Epoch
		s.Entropy[0] = s.ComputeCurrRandomness(s.Entropy[0])
		s.Entropy[1] = s.Entropy[0]
		s.Entropy[2] = s.Entropy[1]
		s.Entropy[3] = s.Entropy[2]
	} else {
		// Epoch in progress
		s.Entropy[0] = s.ComputeCurrRandomness(s.Entropy[0])
	}
	if targetJCE > prevJCE {
		s.Timeslot = targetJCE
	}
	return nil
}

// statefrole_stf is the function to be tested
func (s *SafroleState) ApplyStateTransitionTickets(tickets []types.Ticket, targetJCE uint32, header types.BlockHeader, id uint32) (SafroleState, error) {
	prevEpoch, prevPhase := s.EpochAndPhase(uint32(s.Timeslot))
	currEpoch, currPhase := s.EpochAndPhase(targetJCE)

	s2 := cloneSafroleState(*s)
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

	for _, e := range tickets {
		if currPhase >= types.TicketSubmissionEndSlot {
			return s2, fmt.Errorf(errTicketSubmissionInTail)
		}
		if len(s.Entropy) == 4 {
			//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
			if e.Attempt >= types.TicketEntriesPerValidator {
				return s2, fmt.Errorf(errExtrinsicWithMoreTicketsThanAllowed)
			}
			ticket_id, err := s.ValidateProposedTicket(&e)
			if err != nil {
				return s2, fmt.Errorf(errTicketBadRingProof)
			}

			_, exists := ticketIDs[ticket_id]
			if exists {
				fmt.Printf("DETECTED Resubmit %v\n", ticket_id)
				continue // return s2, fmt.Errorf(errTicketResubmission)
			}

			newa := types.TicketBody{
				Id:      ticket_id,
				Attempt: uint8(e.Attempt),
			}
			//			if len(newTickets) > 0 && compareTickets(newa.Id, newTickets[len(newTickets)-1].Id) < 0 {
			//				return s2, fmt.Errorf(errTicketBadOrder)
			//			}
			s2.NextEpochTicketsAccumulator = append(s2.NextEpochTicketsAccumulator, newa)
			if s.Id == 0 {
				fmt.Printf("[N%d] added Ticket ID %v\n", id, ticket_id)
			}
		}
	}

	// Sort tickets using compareTickets
	sort.SliceStable(s2.NextEpochTicketsAccumulator, func(i, j int) bool {
		return compareTickets(s2.NextEpochTicketsAccumulator[i].Id, s2.NextEpochTicketsAccumulator[j].Id) < 0
	})

	// Drop useless tickets. Should this be error or not?
	if len(s2.NextEpochTicketsAccumulator) > types.EpochLength {
		s2.NextEpochTicketsAccumulator = s2.NextEpochTicketsAccumulator[0:types.EpochLength]
	}

	// in the tail slot with a full set of tickets

	if len(s2.NextEpochTicketsAccumulator) == types.EpochLength && currPhase >= types.TicketSubmissionEndSlot { //MK check EpochNumSlots-1 ?
		//TODO this is Winning ticket elgible. Check if header has marker, if yes, verify it
		winning_tickets := header.TicketsMark

		if len(winning_tickets) == types.EpochLength {
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
		}

	}

	fresh_randomness, err := s.GetFreshRandomness(header.EntropySource[:])
	if err != nil {

		fmt.Printf("GetFreshRandomness ERR %v (len=%d)", err, len(header.EntropySource[:]))
		panic(0)
		return s2, fmt.Errorf("GetFreshRandomness %v", err)
	}

	new_entropy_0 := s.ComputeCurrRandomness(fresh_randomness)
	if currEpoch > prevEpoch {
		s2.PrevValidators = s2.CurrValidators
		s2.CurrValidators = s2.NextValidators
		s2.NextValidators = s2.DesignedValidators
		s2.Entropy[0] = new_entropy_0
		s2.Entropy[1] = s.Entropy[0]
		s2.Entropy[2] = s.Entropy[1]
		s2.Entropy[3] = s.Entropy[2]
		s2.NextEpochTicketsAccumulator = s2.NextEpochTicketsAccumulator[0:0]
		fmt.Printf("[N%d] ApplyStateTransitionTickets: ENTROPY shifted new epoch 0:%v 1:%v 2:%v 3:%v\n", s2.Id, s2.Entropy[0], s2.Entropy[1], s2.Entropy[2], s2.Entropy[3])
	} else {
		// Epoch in progress
		s2.Entropy[0] = new_entropy_0
		s2.Entropy[1] = s.Entropy[1]
		s2.Entropy[2] = s.Entropy[2]
		s2.Entropy[3] = s.Entropy[3]
		fmt.Printf("[N%d] ApplyStateTransitionTickets: ENTROPY norm 1:%v 2:%v 3:%v\n", s2.Id, s2.Entropy[1], s2.Entropy[2], s2.Entropy[3])
	}

	s2.BlockNumber = s.BlockNumber + 1
	s2.Timeslot = targetJCE
	return s2, nil
}

// statefrole_stf is the function to be tested
func (s *SafroleState) STF(input Input) (Output, *SafroleState, error) {

	s2 := s.Copy()
	o := &Output{
		Ok: nil,
	}
	ok := &struct {
		EpochMark   *types.EpochMark    `json:"epoch_mark"`
		TicketsMark []*types.TicketBody `json:"tickets_mark"`
	}{
		EpochMark:   nil,
		TicketsMark: nil,
	}
	if input.Slot <= s.Timeslot {
		return *o, s2, fmt.Errorf(errTimeslotNotMonotonic)
	}

	// tally existing ticketIDs
	ticketIDs := make(map[common.Hash]uint8)
	for _, a := range s.TicketsAccumulator {
		//fmt.Printf("ticketID? %d => %s\n", i, a.Id.String())
		ticketIDs[a.Id] = a.Attempt
	}

	pubkeys := []bandersnatch.PublicKey{}
	for _, v := range s.NextValidators {
		//pubkeys = append(pubkeys, v.Bandersnatch.Bytes())
		pubkeys = append(pubkeys, common.ConvertToSlice(v.Bandersnatch))
	}
	ringsetBytes := bandersnatch.InitRingSet(pubkeys)

	// Process Extrinsic Tickets
	//fmt.Printf("Current Slot: %d => Input Slot: %d \n", s.Timeslot, input.Slot)
	newTickets := []types.TicketBody{}
	for _, e := range input.Extrinsics {
		if input.Slot >= types.TicketSubmissionEndSlot {
			return *o, s2, fmt.Errorf(errTicketSubmissionInTail)
		}
		if len(s.Entropy) == 4 {
			//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
			vrfInputData := append(append([]byte(types.X_T), s.Entropy[2].Bytes()...), byte(e.Attempt))
			auxData := []byte{}
			vrfOutput, err := bandersnatch.RingVrfVerify(ringsetBytes, e.Signature[:], vrfInputData, auxData)
			if err != nil {
				panic(33)
				return *o, s2, fmt.Errorf(errTicketBadRingProof)
			}
			_, exists := ticketIDs[common.BytesToHash(vrfOutput)]
			if exists {
				fmt.Printf("DETECTED Resubmit %x\n", vrfOutput)
				return *o, s2, fmt.Errorf(errTicketResubmission)
			}
			if e.Attempt >= types.TicketEntriesPerValidator {
				return *o, s2, fmt.Errorf(errExtrinsicWithMoreTicketsThanAllowed)
			}
			newa := types.TicketBody{
				Id:      common.BytesToHash(vrfOutput),
				Attempt: uint8(e.Attempt),
			}
			if len(newTickets) > 0 && compareTickets(newa.Id, newTickets[len(newTickets)-1].Id) < 0 {
				return *o, s2, fmt.Errorf(errTicketBadOrder)
			}
			newTickets = append(newTickets, newa)
			fmt.Printf("added Ticket ID %x\n", vrfOutput)
		}
	}
	for _, newa := range newTickets {
		s2.TicketsAccumulator = append(s2.TicketsAccumulator, newa)
	}
	// Sort tickets using compareTickets
	sort.SliceStable(s2.TicketsAccumulator, func(i, j int) bool {
		return compareTickets(s2.TicketsAccumulator[i].Id, s2.TicketsAccumulator[j].Id) < 0
	})
	fmt.Printf("Sorted Tickets:\n")
	for i, t := range s2.TicketsAccumulator {
		fmt.Printf(" %d: %s\n", i, t.Id)
	}
	// Drop useless tickets
	if len(s2.TicketsAccumulator) > types.EpochLength {
		s2.TicketsAccumulator = s2.TicketsAccumulator[0:types.EpochLength]
	}

	// the last slot in the Epoch, with a full set of tickets
	if len(s2.TicketsAccumulator) == types.EpochLength && input.Slot == types.EpochLength-1 {
		ok.TicketsMark = s.computeTicketSlotBinding(s2.TicketsAccumulator)
	}
	fmt.Printf(" -- %d %d %d \n", input.Slot, types.EpochLength, input.Slot%types.EpochLength)
	// adjust entropy
	hasher256, err := blake2b.New256(nil)
	if err != nil {
		return *o, s2, err
	}
	hasher256.Write(append(s.Entropy[0].Bytes(), input.Entropy.Bytes()...))
	res := hasher256.Sum(nil)

	if input.Slot%types.EpochLength == 0 {
		// New Epoch
		var v [6]common.Hash
		for i, x := range s.DesignedValidators {
			v[i] = x.Bandersnatch.Hash()
			fmt.Printf(" !! %d %x\n", i, v[i])
		}
		ok.EpochMark = &types.EpochMark{
			Entropy:    s.Entropy[0],
			Validators: v,
		}
		s2.Entropy[1] = s.Entropy[0]
		s2.Entropy[2] = s.Entropy[1]
		s2.Entropy[3] = s.Entropy[2]
		s2.Entropy[0] = common.BytesToHash(res)
		s2.TicketsAccumulator = s2.TicketsAccumulator[0:0]
	} else {
		// Epoch in progress
		s2.Entropy[0] = common.BytesToHash(res)
		s2.Entropy[1] = s.Entropy[1]
		s2.Entropy[2] = s.Entropy[2]
		s2.Entropy[3] = s.Entropy[3]
	}

	s2.Timeslot = input.Slot
	o.Ok = ok
	return *o, s2, fmt.Errorf(errNone)
}
