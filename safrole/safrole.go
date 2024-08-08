package safrole

import (
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/bandersnatch"

	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/colorfulnotion/jam/common"
	"golang.org/x/crypto/blake2b"
	"sort"
	//"github.com/colorfulnotion/jam/trie"
)

// state-key constructor functions C
const (
	C1  = "CoreAuthPool"
	C2  = "AuthQueue"
	C3  = "RecentBlocks"
	C4  = "safroleState"
	C5  = "PastJudgements"
	C6  = "Entropy"                 //Entropy Accumulator and epochal randomness
	C7  = "NextEpochValidatorKeys"  //Validator keys and metadata to be drawn from next
	C8  = "CurrentValidatorKeys"    //Validator keys and metadata currently active
	C9  = "PriorEpochValidatorKeys" //Validator keys and metadata which were active in the prior epoch
	C10 = "PendingReports"
	C11 = "MostRecentBlockTimeslot" //The most recent block's timeslot
	C12 = "PrivilegedServiceIndices"
	C13 = "ActiveValidator"
)

const (
	JamCommonEra = 1704100800 //1200 UTC on January 1, 2024
)

const (
	BlsSizeInBytes            = 144
	MetadataSizeInBytes       = 128
	ExtrinsicSignatureInBytes = 784
	TicketsVerifierKeyInBytes = 144
	ValidatorInByte           = 336
	EntropySize               = 4
)

const (
	// tiny (NOTE: large is 600 + 1023, should use ProtocolConfiguration)
	EpochNumSlots = 12
	NumValidators = 6
	NumAttempts   = 2
	EpochTail     = 10 //Y
)

type SafroleHeader struct {
	ParentHash         common.Hash
	PriorStateRoot     common.Hash
	ExtrinsicHash      common.Hash
	TimeSlot           uint32
	EpochMark          *EpochMark
	WinningTicketsMark []*TicketBody
	VerdictsMarkers    *VerdictMarker
	OffenderMarkers    *OffendertMarker
	BlockAuthorKey     uint16
	VRFSignature       []byte
	BlockSeal          []byte
}

type VerdictMarker struct {
}

type OffendertMarker struct {
}

/*
//γ ≡⎩γk, γz, γs, γa⎭
//γk :one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
//γz :epoch’s root, a Bandersnatch ring root composed with the one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
//γa :the ticket accumulator, a series of highest-scoring ticket identifiers to be used for the next epoch (epoch N+1)
//γs :current epoch’s slot-sealer series, which is either a full complement of E tickets or, in the case of a fallback mode, a series of E Bandersnatch keys (epoch N)
*/
type SafroleBasicState struct {

	// γk represents one Bandersnatch key of each of the next epoch’s validators (epoch N+1).
	GammaK []Validator `json:"gamma_k"`

	// γz represents the epoch’s root, a Bandersnatch ring root composed with one Bandersnatch key of each of the next epoch’s validators (epoch N+1).
	GammaZ []byte `json:"gamma_z"`

	// γs represents the current epoch’s slot-sealer series, which is either a full complement of E tickets or, in the case of fallback mode, a series of E Bandersnatch keys (epoch N).
	GammaS TicketsOrKeys `json:"gamma_s"`

	// γa represents the ticket accumulator, a series of least-scoring ticket identifiers to be used for the next epoch (epoch N+1).
	GammaA []TicketBody `json:"gamma_a"`
}

// TicketBody represents the structure of a ticket
type TicketBody struct {
	Id      common.Hash `json:"id"`
	Attempt uint8       `json:"attempt"`
}

type Ticket struct {
	Attempt   int                             `json:"attempt"`
	Signature [ExtrinsicSignatureInBytes]byte `json:"signature"`
}

func (t Ticket) MarshalJSON() ([]byte, error) {
	type Alias Ticket
	return json.Marshal(&struct {
		Signature string `json:"signature"`
		*Alias
	}{
		Signature: hex.EncodeToString(t.Signature[:]),
		Alias:     (*Alias)(&t),
	})
}

func (t *Ticket) UnmarshalJSON(data []byte) error {
	type Alias Ticket
	aux := &struct {
		Signature string `json:"signature"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	sigBytes, err := hex.DecodeString(aux.Signature)
	if err != nil {
		return err
	}
	copy(t.Signature[:], sigBytes)
	return nil
}

func (t Ticket) DeepCopy() (Ticket, error) {
	var copiedTicket Ticket

	// Serialize the original Ticket to JSON
	data, err := json.Marshal(t)
	if err != nil {
		return copiedTicket, err
	}

	// Deserialize the JSON back into a new Ticket instance
	err = json.Unmarshal(data, &copiedTicket)
	if err != nil {
		return copiedTicket, err
	}

	return copiedTicket, nil
}

func (t Ticket) String() string {
	return fmt.Sprintf("Ticket{Attempt: %d, Signature: %s}", t.Attempt, hex.EncodeToString(t.Signature[:]))
}

// TicketsOrKeys represents the choice between tickets and keys
type TicketsOrKeys struct {
	Tickets []*TicketBody `json:"tickets,omitempty"`
	Keys    []common.Hash `json:"keys,omitempty"` //BandersnatchKey
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

// 6.1 protocol configuration
type ProtocolConfiguration struct {
	EpochNumSlots uint32 // number of slots for each epoch (The "v")
	NumAttempts   uint8  // maximum number of tickets each authority can submit ("")
}

type Input struct {
	Slot       int         `json:"slot"`
	Entropy    common.Hash `json:"entropy"`
	Extrinsics []Extrinsic `json:"extrinsics"`
}

// used for calling FFI
type ValidatorSecret struct {
	Ed25519Pub         common.Hash `json:"ed25519_pub"`
	BandersnatchPub    common.Hash `json:"bandersnatch_pub"`
	Ed25519Secret      []byte      `json:"ed25519_priv"`
	BandersnatchSecret []byte      `json:"bandersnatch_priv"`
}

func (v ValidatorSecret) MarshalJSON() ([]byte, error) {
	type Alias ValidatorSecret
	return json.Marshal(&struct {
		Alias
		Ed25519Secret      string `json:"ed25519_priv"`
		BandersnatchSecret string `json:"bandersnatch_priv"`
	}{
		Alias:              (Alias)(v),
		Ed25519Secret:      hex.EncodeToString(v.Ed25519Secret),
		BandersnatchSecret: hex.EncodeToString(v.BandersnatchSecret),
	})
}

type Validator struct {
	Ed25519      common.Hash               `json:"ed25519"`
	Bandersnatch common.Hash               `json:"bandersnatch"`
	Bls          [BlsSizeInBytes]byte      `json:"bls"`
	Metadata     [MetadataSizeInBytes]byte `json:"metadata"`
}

func (v Validator) GetBandersnatchKey() common.Hash {
	return v.Bandersnatch
}

func (v Validator) Bytes() []byte {
	// Initialize a byte slice with the required size
	bytes := make([]byte, 0, ValidatorInByte)
	bytes = append(bytes, v.Ed25519.Bytes()...)
	bytes = append(bytes, v.Bandersnatch.Bytes()...)
	bytes = append(bytes, v.Bls[:]...)
	bytes = append(bytes, v.Metadata[:]...)
	return bytes
}

func ValidatorFromBytes(data []byte) (Validator, error) {
	if len(data) != ValidatorInByte {
		return Validator{}, fmt.Errorf("invalid data length: expected %d, got %d", 32+32+144+128, len(data))
	}
	var v Validator
	offset := 0
	copy(v.Ed25519[:], data[offset:offset+32])
	offset += 32
	copy(v.Bandersnatch[:], data[offset:offset+32])
	offset += 32
	copy(v.Bls[:], data[offset:offset+144])
	offset += 144
	copy(v.Metadata[:], data[offset:offset+128])
	return v, nil
}

// MarshalJSON custom marshaler to convert byte arrays to hex strings
func (v Validator) MarshalJSON() ([]byte, error) {
	type Alias Validator
	return json.Marshal(&struct {
		Alias
		Bls      string `json:"bls"`
		Metadata string `json:"metadata"`
	}{
		Alias:    (Alias)(v),
		Bls:      hex.EncodeToString(v.Bls[:]),
		Metadata: hex.EncodeToString(v.Metadata[:]),
	})
}

// UnmarshalJSON custom unmarshal method to handle hex strings for byte arrays
func (v *Validator) UnmarshalJSON(data []byte) error {
	type Alias Validator
	aux := &struct {
		Bls      string `json:"bls"`
		Metadata string `json:"metadata"`
		*Alias
	}{
		Alias: (*Alias)(v),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Decode the hex strings into byte arrays
	blsBytes, err := hex.DecodeString(aux.Bls)
	if err != nil {
		return err
	}
	if len(blsBytes) != BlsSizeInBytes {
		return fmt.Errorf("invalid length for bls field")
	}
	copy(v.Bls[:], blsBytes)

	metadataBytes, err := hex.DecodeString(aux.Metadata)
	if err != nil {
		return err
	}
	if len(metadataBytes) != MetadataSizeInBytes {
		return fmt.Errorf("invalid length for metadata field")
	}
	copy(v.Metadata[:], metadataBytes)

	return nil
}

// Extrinsic is submitted by authorities, which are added to Safrole State in TicketsAccumulator if valid
type Extrinsic struct {
	Attempt   int                             `json:"attempt"`
	Signature [ExtrinsicSignatureInBytes]byte `json:"signature"`
}

// 6.5.3. Ticket Envelope
type TicketEnvelope struct {
	Id            common.Hash                     //not part of the official TicketEnvelope struct
	Attempt       int                             //Index associated to the ticket.
	Extra         []byte                          //additional data for user-defined applications.
	RingSignature [ExtrinsicSignatureInBytes]byte //ring signature of the envelope data (attempt & extra)
}

var (
	jamTicketSeal   = []byte("jam_ticket_seal")
	jamFallbackSeal = []byte("jam_fallback_seal")
	jamRandomness   = []byte("jam_randomness")
	jamEntropy      = []byte("jam_entropy")
)

type SafroleState struct {
	EpochFirstSlot uint32
	Epoch          uint32

	TimeStamp   int `json:"timestamp"`
	Timeslot    int `json:"timeslot"`
	BlockNumber int `json:"blockNumber"`
	// Entropy holds 4 32 byte
	// Entropy[0] CURRENT randomness accumulator (see sec 6.10). randomness_buffer[0] = BLAKE2(CONCAT(randomness_buffer[0], fresh_randomness));
	// where fresh_randomness = vrf_signed_output(claim.randomness_source)

	// Entropy[1] accumulator snapshot BEFORE the execution of the first block of epoch N   - randomness used for ticket targeting epoch N+2
	// Entropy[2] accumulator snapshot BEFORE the execution of the first block of epoch N-1 - randomness used for ticket targeting epoch N+1
	// Entropy[3] accumulator snapshot BEFORE the execution of the first block of epoch N-2 - randomness used for ticket targeting current epoch N
	Entropy []common.Hash `json:"entropy"`

	// 4 authorities[pre, curr, next, designed]
	PrevValidators     []Validator `json:"prev_validators"`
	CurrValidators     []Validator `json:"curr_validators"`
	NextValidators     []Validator `json:"next_validators"`
	DesignedValidators []Validator `json:"designed_validators"`

	// Accumulator of tickets, modified with Extrinsics to hold ORDERED array of Tickets
	NextEpochTicketsAccumulator []TicketBody  `json:"next_tickets_accumulator"` //gamma_z
	TicketsAccumulator          []TicketBody  `json:"tickets_accumulator"`
	TicketsOrKeys               TicketsOrKeys `json:"tickets_or_keys"`

	// []bandersnatch.ValidatorKeySet
	TicketsVerifierKey []byte `json:"tickets_verifier_key"`
}

func NewSafroleState() *SafroleState {
	return &SafroleState{
		Timeslot:           0,
		BlockNumber:        0,
		Entropy:            make([]common.Hash, 4),
		PrevValidators:     []Validator{},
		CurrValidators:     []Validator{},
		NextValidators:     []Validator{},
		DesignedValidators: []Validator{},
		TicketsAccumulator: []TicketBody{},
		TicketsOrKeys: TicketsOrKeys{
			Keys: []common.Hash{},
			// Tickets: []TicketBody{}, // Uncomment if you need to initialize tickets instead
		},
		TicketsVerifierKey: []byte{},
	}
}

// EpochMark (see 6.4 Epoch change Signal) represents the descriptor for parameters to be used in the next epoch
type EpochMark struct {
	// Randomness accumulator snapshot
	Entropy common.Hash `json:"entropy"`
	// List of authorities scheduled for next epoch
	Validators []common.Hash `json:"validators"` //bandersnatch keys
}

func VerifyEpochMarker(epochMark *EpochMark) (bool, error) {
	//STUB
	return true, nil
}

func (s *SafroleState) GenerateEpochMarker() *EpochMark {
	nextValidators := make([]common.Hash, 0, len(s.NextValidators)) // Preallocate the slice
	for _, v := range s.NextValidators {
		nextValidators = append(nextValidators, v.GetBandersnatchKey())
	}
	fmt.Printf("nextValidators Len=%v\n", nextValidators)
	return &EpochMark{
		Entropy:    s.Entropy[1], // Assuming s.Entropy has at least two elements
		Validators: nextValidators,
	}
}

func VerifyWinningMarker(winning_marker []*TicketBody, expected_marker []*TicketBody) (bool, error) {
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

func (s *SafroleState) GenerateWinningMarker() ([]*TicketBody, error) {
	tickets := s.NextEpochTicketsAccumulator
	if len(tickets) != EpochNumSlots {
		//accumulator should keep exactly EpochNumSlots ticket
		return nil, fmt.Errorf("Invalid Ticket size")
	}
	outsidein_tickets := s.computeTicketSlotBinding(tickets)
	return outsidein_tickets, nil
}

type Output struct {
	Ok *struct {
		EpochMark   *EpochMark    `json:"epoch_mark"`
		TicketsMark []*TicketBody `json:"tickets_mark"`
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

func InitValidator(bandersnatch_seed, ed25519_seed []byte) (Validator, error) {
	validator := Validator{}
	banderSnatch_pub, _, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init BanderSnatch Key")
	}
	ed25519_pub, _, err := bandersnatch.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init Ed25519 Key")
	}
	validator.Ed25519 = common.BytesToHash(ed25519_pub)
	validator.Bandersnatch = common.BytesToHash(banderSnatch_pub)
	//fmt.Printf("validator %v\n", validator)
	return validator, nil
}

func InitValidatorSecret(bandersnatch_seed, ed25519_seed []byte) (ValidatorSecret, error) {
	validatorSecret := ValidatorSecret{}
	banderSnatch_pub, banderSnatch_priv, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init BanderSnatch Key")
	}
	ed25519_pub, ed25519_priv, err := bandersnatch.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init Ed25519 Key")
	}
	validatorSecret.Ed25519Secret = ed25519_priv
	validatorSecret.Ed25519Pub = common.BytesToHash(ed25519_pub)

	validatorSecret.BandersnatchSecret = banderSnatch_priv
	validatorSecret.BandersnatchPub = common.BytesToHash(banderSnatch_pub)
	//fmt.Printf("validatorSecret %v\n", validatorSecret)
	return validatorSecret, nil
}

// 6.4.1 Startup parameters
func InitGenesisState(genesisConfig *GenesisConfig) (s *SafroleState) {
	s = new(SafroleState)
	s.EpochFirstSlot = genesisConfig.StartTimeslot
	s.Timeslot = 0
	validators := genesisConfig.Authorities
	vB := []byte{}
	for _, v := range validators {
		vB = append(vB, v.Bytes()...)
	}
	s.PrevValidators = validators
	s.CurrValidators = validators
	s.NextValidators = validators
	s.DesignedValidators = validators

	/*
		The on-chain randomness is initialized after the genesis block construction.
		The first buffer entry is set as the Blake2b hash of the genesis block,
		each of the other entries is set as the Blake2b hash of the previous entry.
	*/

	s.Entropy = make([]common.Hash, EntropySize)
	s.Entropy[0] = common.BytesToHash(common.ComputeHash(vB))                   //BLAKE2B hash of the genesis block#0
	s.Entropy[1] = common.BytesToHash(common.ComputeHash(s.Entropy[0].Bytes())) //BLAKE2B of Current
	s.Entropy[2] = common.BytesToHash(common.ComputeHash(s.Entropy[1].Bytes())) //BLAKE2B of EpochN1
	s.Entropy[3] = common.BytesToHash(common.ComputeHash(s.Entropy[2].Bytes())) //BLAKE2B of EpochN2

	return s
}

type ClaimData struct {
	Slot             uint32 `json:"slot"`
	AuthorityIndex   uint32 `json:"authority_index"`
	RandomnessSource []byte `json:"randomness_source"`
}

func EpochAndPhase(currCJE uint32) (currentEpoch, currentPhase uint32) {
	currentEpoch = currCJE / EpochNumSlots
	currentPhase = currCJE % EpochNumSlots
	return
}

// used for detecting epoch marker
func (s *SafroleState) IsFirstEpochPhase(currCJE uint32) bool {
	prevEpoch, _ := EpochAndPhase(uint32(s.Timeslot))
	currEpoch, _ := EpochAndPhase(currCJE)
	if currEpoch > prevEpoch {
		return true
	}
	return false
}

// used for detecting the end of submission period
func (s *SafroleState) IsTicketSubmissionCloses(currCJE uint32) bool {
	//prevEpoch, _ := EpochAndPhase(uint32(s.Timeslot))
	_, currPhase := EpochAndPhase(currCJE)
	if currPhase >= EpochTail {
		return true
	}
	return false
}

func CalculateEpochAndPhase(currentTimeslot, priorTimeslot uint32) (currentEpoch, priorEpoch, currentPhase, priorPhase uint32) {
	currentEpoch = currentTimeslot / EpochNumSlots
	currentPhase = currentTimeslot % EpochNumSlots
	priorEpoch = priorTimeslot / EpochNumSlots
	priorPhase = priorTimeslot % EpochNumSlots
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
	new_randomness := common.Blake2AsHex(append(randomness_buffer0, fresh_randomness.Bytes()...))
	return new_randomness
}

// 6.5.1. Ticket Identifier (Primary Method)
// ticketSealVRFInput constructs the input for VRF based on target epoch randomness and attempt.
func (s *SafroleState) ticketSealVRFInput(targetEpochRandomness common.Hash, attempt int) []byte {
	// Concatenate sassafrasTicketSeal, targetEpochRandomness, and attemptBytes
	ticket_vrf_input := append(append(jamTicketSeal, targetEpochRandomness.Bytes()...), []byte{byte(attempt & 0xF)}...)
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
	nextEpochFirstSlot := s.EpochFirstSlot + EpochNumSlots
	return nextEpochFirstSlot
}

func (s *SafroleState) GetTimeSlot() uint32 {
	return uint32(s.Timeslot)
}

func (s *SafroleState) GetEpochValidatorAndRandomness(phase string) ([]Validator, common.Hash) {
	// 4 authorities[pre, curr, next, designed]
	// η0 Entropy[0] CURRENT randomness accumulator (see sec 6.10). randomness_buffer[0] = BLAKE2(CONCAT(randomness_buffer[0], fresh_randomness));
	// η1 Entropy[1] accumulator snapshot BEFORE the execution of the first block of epoch N   - randomness used for ticket targeting epoch N+2
	// η2 Entropy[2] accumulator snapshot BEFORE the execution of the first block of epoch N-1 - randomness used for ticket targeting epoch N+1
	// η3 Entropy[3] accumulator snapshot BEFORE the execution of the first block of epoch N-2 - randomness used for ticket targeting current epoch N
	var validatorSet []Validator
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
	var validatorSet []Validator
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

func (s *SafroleState) SetValidatorData(validatorsData []byte, phase string) error {
	// Calculate the expected length for each validator data
	const validatorLength = 32 + 32 + 144 + 128

	// Check if validatorsData length is a multiple of validatorLength
	if len(validatorsData)%validatorLength != 0 {
		return fmt.Errorf("invalid validatorsData length: expected a multiple of %d, got %d", validatorLength, len(validatorsData))
	}

	var validatorSet []Validator
	for offset := 0; offset < len(validatorsData); offset += validatorLength {
		validator, err := ValidatorFromBytes(validatorsData[offset : offset+validatorLength])
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
	var validatorSet []Validator
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
		pubkeys = append(pubkeys, v.Bandersnatch.Bytes())
	}
	ringsetBytes = bandersnatch.InitRingSet(pubkeys)
	return ringsetBytes
}

func (t *Ticket) TicketIDWithCheck() (common.Hash, error) {
	ticket_id, err := bandersnatch.VRFSignedOutput(t.Signature[:])
	if err != nil {
		return common.Hash{}, fmt.Errorf("invalid ticket_id err=%v", err)
	}
	return common.BytesToHash(ticket_id), err
}

func (t *Ticket) TicketID() common.Hash {
	ticket_id, err := bandersnatch.VRFSignedOutput(t.Signature[:])
	if err != nil {
		return common.Hash{}
	}
	return common.BytesToHash(ticket_id)
}

func (s *SafroleState) GenerateTickets(secret bandersnatch.PrivateKey) []Ticket {
	tickets := make([]Ticket, 0)
	targetEpochRandomness := s.Entropy[2] // check eq 74
	for attempt := 0; attempt < NumAttempts; attempt++ {
		ticket, err := s.generateTicket(secret, targetEpochRandomness, attempt)
		if err == nil {
			tickets = append(tickets, ticket)
		} else {
			fmt.Printf("Error generating ticket for attempt %d: %v\n", attempt, err)
		}
	}
	return tickets
}

func (s *SafroleState) generateTicket(secret bandersnatch.PrivateKey, targetEpochRandomness common.Hash, attempt int) (Ticket, error) {
	ticket_vrf_input := s.ticketSealVRFInput(targetEpochRandomness, attempt)
	//RingVrfSign(privateKey, ringsetBytes, vrfInputData, auxData []byte)
	//During epoch N, each authority scheduled for epoch N+2 constructs a set of tickets which may be eligible (6.5.2) for on-chain submission.
	auxData := []byte{}
	ringsetBytes := s.GetRingSet("Next")
	//fmt.Printf("generateTicket secrete=%x, ringsetBytes=%x, ticket_vrf_input=%x, auxData=%x\n", secret, ringsetBytes, ticket_vrf_input, auxData)
	//RingVrfSign(privateKeys[proverIdx], ringSet, vrfInputData, auxData)
	signature, _, err := bandersnatch.RingVrfSign(secret, ringsetBytes, ticket_vrf_input, auxData) // ??
	if err != nil {
		return Ticket{}, fmt.Errorf("signTicket failed")
	}
	var signatureArray [ExtrinsicSignatureInBytes]byte
	copy(signatureArray[:], signature)
	ticket := Ticket{
		Attempt:   attempt,
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

func (s *SafroleState) ValidateProposedTicket(t *Ticket) (common.Hash, error) {

	//step 0: derive ticketVRFInput
	targetEpochRandomness := s.Entropy[2]
	ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, t.Attempt)
	if t.Attempt >= NumAttempts {
		return common.Hash{}, fmt.Errorf(errExtrinsicWithMoreTicketsThanAllowed)
	}

	//step 1: verify envelope's VRFSignature using ring verifier
	//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
	ringsetBytes := s.GetRingSet("Next")
	ticket_id, err := bandersnatch.RingVrfVerify(ringsetBytes, t.Signature[:], ticketVRFInput, []byte{})
	if err != nil {
		fmt.Printf("!!!ValidateProposed Ticket ticket_id=%v targetEpochRandomness=%v attempt=%v, ticket_vrf_input=%x, sig=%x\n", t.TicketID(), targetEpochRandomness, t.Attempt, ticketVRFInput, t.Signature[:])
		return common.Hash{}, fmt.Errorf(errTicketBadRingProof)
	}
	return common.BytesToHash(ticket_id), nil
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
	t := TicketBody{
		Id:      ticket_id,
		Attempt: uint8(envelope.Attempt),
	}
	s.TicketsAccumulator = append(s.TicketsAccumulator, t)

	return nil

}

// computeTicketSlotBinding has logic for assigning tickets to slots
func (s *SafroleState) computeTicketSlotBinding(inp []TicketBody) []*TicketBody {
	i := 0
	tickets := make([]*TicketBody, 0, EpochNumSlots)
	for i < len(inp)/2 { //MK: check this line, was s.TicketsAccumulator/2
		tickets = append(tickets, &inp[i])
		tickets = append(tickets, &inp[EpochNumSlots-1-i])
		i++
	}
	return tickets
}

func (s *SafroleState) SignPrimary(authority_secret_key bandersnatch.PrivateKey, unsignHeaderHash common.Hash, attempt int) ([]byte, []byte, error) {
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
	entropyPrefix := []byte(jamEntropy)
	entropyVRFInput := append(entropyPrefix, inner_vrfOutput.Bytes()...)
	return entropyVRFInput
}

func fallbackSealVRFInput(targetRandomness common.Hash) []byte {
	sealPrefix := []byte(jamFallbackSeal)
	sealVRFInput := append(sealPrefix, targetRandomness.Bytes()...)
	return sealVRFInput
}

func (s *SafroleState) GetRelativeSlotIndex(slot_index uint32) (uint32, error) {
	//relative slot index = slot - epoch_first_slot
	relativeSlotIndex := slot_index - s.EpochFirstSlot
	return relativeSlotIndex, nil
}

func (s *SafroleState) GetAuthorIndex(authorkey common.Hash, phase string) (uint16, error) {
	var validatorSet []Validator

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
	_, currPhase := EpochAndPhase(slot_index)
	selected_ticket := winning_tickets[currPhase]
	return selected_ticket.Id
}

func (s *SafroleState) GetProposedTargetEpochRandomness() common.Hash {
	targetRandomness := s.Entropy[3]
	return targetRandomness
}

func (s *SafroleState) GetRandomness() []common.Hash {
	return s.Entropy
}

func (s *SafroleState) IsAuthorizedTicketBuilder(bandersnatchPub common.Hash) bool {
	//TODO
	return true
}

func (s *SafroleState) CheckEpochType() string {
	t_or_k := s.TicketsOrKeys
	if len(t_or_k.Tickets) > 0 {
		return "primary"
	} else {
		return "fallback"
	}
}

func (s *SafroleState) GetBindedAttempt(targetJCE uint32) (int, error) {
	_, currPhase := EpochAndPhase(targetJCE)
	t_or_k := s.TicketsOrKeys
	if len(t_or_k.Tickets) == EpochNumSlots {
		winning_ticket := t_or_k.Tickets[currPhase]
		return int(winning_ticket.Attempt), nil
	}
	return 0, fmt.Errorf("Shouldn't be fallback")
}

func (s *SafroleState) IsAuthorizedBuilder(slot_index uint32, bandersnatchPub common.Hash, ticketIDs []common.Hash) bool {
	currEpoch, currPhase := EpochAndPhase(slot_index)
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
	return false
}

func (s *SafroleState) CheckTimeSlotReady() (uint32, bool) {
	prev := s.GetTimeSlot()
	currJCE := ComputeCurrentJCETime()
	if currJCE > uint32(prev) {
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
		EpochFirstSlot:              original.EpochFirstSlot,
		Epoch:                       original.Epoch,
		TimeStamp:                   original.TimeStamp,
		Timeslot:                    original.Timeslot,
		BlockNumber:                 original.BlockNumber,
		Entropy:                     make([]common.Hash, len(original.Entropy)),
		PrevValidators:              make([]Validator, len(original.PrevValidators)),
		CurrValidators:              make([]Validator, len(original.CurrValidators)),
		NextValidators:              make([]Validator, len(original.NextValidators)),
		DesignedValidators:          make([]Validator, len(original.DesignedValidators)),
		NextEpochTicketsAccumulator: make([]TicketBody, len(original.NextEpochTicketsAccumulator)),
		TicketsAccumulator:          make([]TicketBody, len(original.TicketsAccumulator)),
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
func copyState(original SafroleState) SafroleState {
	// Convert to JSON
	originalJSON, err := json.Marshal(original)
	if err != nil {
		panic(err) // Handle error as needed
	}

	// Create a new State struct
	var copied SafroleState

	// Convert from JSON to struct
	err = json.Unmarshal(originalJSON, &copied)
	if err != nil {
		panic(err) // Handle error as needed
	}

	return copied
}

func (s *SafroleState) AdvanceSafrole(targetJCE uint32) error {
	prevJCE := s.GetTimeSlot()
	if prevJCE < targetJCE {
		return fmt.Errorf(errTimeslotNotMonotonic)
	}
	//currJCE := ComputeCurrentJCETime()
	currEpochFirst := s.GetCurrEpochFirst()
	nextEpochFirst := s.GetNextEpochFirst()
	if currEpochFirst > targetJCE {
		//not ready to advance
		return fmt.Errorf(errEpochFirstSlotNotMet)
	}

	randomnessBuf := s.GetRandomness()
	//TODO: how to get the claimData??
	fresh_randomness := randomnessBuf[0]
	//fresh_randomness := GetFreshRandomness(claim)
	updated_randomness := s.ComputeCurrRandomness(fresh_randomness)

	if targetJCE >= nextEpochFirst {
		// New Epoch
		s.Entropy[1] = randomnessBuf[0]
		s.Entropy[2] = randomnessBuf[1]
		s.Entropy[3] = randomnessBuf[2]
		s.Entropy[0] = updated_randomness
		//s.TicketsAccumulator = s.TicketsAccumulator[0:0]
	} else {
		// Epoch in progress
		s.Entropy[0] = updated_randomness
		//s.Entropy[1] = randomnessBuf[1]
		//s.Entropy[2] = randomnessBuf[2]
		//s.Entropy[3] = randomnessBuf[3]
	}
	if targetJCE > prevJCE {
		s.Timeslot = int(targetJCE)
	}
	return nil
}

// statefrole_stf is the function to be tested
func (s *SafroleState) ApplyStateTransitionFromBlock(tickets []Ticket, targetJCE uint32, header SafroleHeader) (SafroleState, error) {
	prevEpoch, prevPhase := EpochAndPhase(uint32(s.Timeslot))
	currEpoch, currPhase := EpochAndPhase(targetJCE)

	s2 := cloneSafroleState(*s)
	if currEpoch <= prevEpoch && currPhase <= prevPhase {
		return s2, fmt.Errorf(errTimeslotNotMonotonic)
	}

	if currEpoch != prevEpoch && currEpoch != prevEpoch+1 {
		// epoch jumped ... not okay
		return s2, fmt.Errorf(errTimeslotNotMonotonic)
	}

	// tally existing ticketIDs
	ticketIDs := make(map[common.Hash]uint8)
	for i, a := range s.NextEpochTicketsAccumulator {
		fmt.Printf("ticketID? %d => %s\n", i, a.Id.String())
		ticketIDs[a.Id] = a.Attempt
	}

	// Process Extrinsic Tickets
	fmt.Printf("Current Slot: %d => Input Slot: %d \n", s.Timeslot, targetJCE)

	newTickets := []TicketBody{}
	for _, e := range tickets {
		if currPhase >= EpochTail {
			return s2, fmt.Errorf(errTicketSubmissionInTail)
		}
		if len(s.Entropy) == 4 {
			//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
			if e.Attempt >= NumAttempts {
				return s2, fmt.Errorf(errExtrinsicWithMoreTicketsThanAllowed)
			}
			ticket_id, err := s.ValidateProposedTicket(&e)
			if err != nil {
				return s2, fmt.Errorf(errTicketBadRingProof)
			}
			_, exists := ticketIDs[ticket_id]
			if exists {
				fmt.Printf("DETECTED Resubmit %v\n", ticket_id)
				return s2, fmt.Errorf(errTicketResubmission)
			}
			newa := TicketBody{
				Id:      ticket_id,
				Attempt: uint8(e.Attempt),
			}
			if len(newTickets) > 0 && compareTickets(newa.Id, newTickets[len(newTickets)-1].Id) < 0 {
				return s2, fmt.Errorf(errTicketBadOrder)
			}
			newTickets = append(newTickets, newa)
			fmt.Printf("added Ticket ID %x\n", ticket_id)
		}
	}

	for _, newa := range newTickets {
		s2.NextEpochTicketsAccumulator = append(s2.NextEpochTicketsAccumulator, newa)
	}

	// Sort tickets using compareTickets
	sort.SliceStable(s2.NextEpochTicketsAccumulator, func(i, j int) bool {
		return compareTickets(s2.NextEpochTicketsAccumulator[i].Id, s2.NextEpochTicketsAccumulator[j].Id) < 0
	})

	// Drop useless tickets. Should this be error or not?
	if len(s2.NextEpochTicketsAccumulator) > EpochNumSlots {
		s2.NextEpochTicketsAccumulator = s2.NextEpochTicketsAccumulator[0:EpochNumSlots]
	}

	// in the tail slot with a full set of tickets

	if len(s2.NextEpochTicketsAccumulator) == EpochNumSlots && currPhase >= EpochTail { //MK check EpochNumSlots-1 ?
		//TODO this is Winning ticket elgible. Check if header has marker, if yes, verify it
		winning_tickets := header.WinningTicketsMark
		if len(winning_tickets) == EpochNumSlots {
			expected_tickets := s.computeTicketSlotBinding(s2.NextEpochTicketsAccumulator)
			verified, err := VerifyWinningMarker(winning_tickets, expected_tickets)
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
	if len(s2.NextEpochTicketsAccumulator) != EpochNumSlots && currPhase >= EpochNumSlots-1 { //MK what if EpochNumSlots-1 not proposed

	}

	fresh_randomness, err := s.GetFreshRandomness(header.VRFSignature)
	if err != nil {
		return s2, fmt.Errorf("Invalid VrfSig")
	}

	new_entropy_0 := s.ComputeCurrRandomness(fresh_randomness)

	if currEpoch == prevEpoch+1 {
		//TODO: verify epoch marker and use it
		epochMark := header.EpochMark
		verified, err := VerifyEpochMarker(epochMark) //STUB
		if !verified || err != nil {
			return s2, fmt.Errorf("Invalid Epoch Mark")
		}

		s2.Entropy[1] = s.Entropy[0]
		s2.Entropy[2] = s.Entropy[1]
		s2.Entropy[3] = s.Entropy[2]
		s2.Entropy[0] = new_entropy_0
		s2.NextEpochTicketsAccumulator = s2.NextEpochTicketsAccumulator[0:0]
	} else {
		// Epoch in progress
		s2.Entropy[0] = new_entropy_0
		s2.Entropy[1] = s.Entropy[1]
		s2.Entropy[2] = s.Entropy[2]
		s2.Entropy[3] = s.Entropy[3]
	}

	s2.Timeslot = int(targetJCE)
	return s2, nil
}

// statefrole_stf is the function to be tested
func (s *SafroleState) STF(input Input) (Output, SafroleState, error) {

	s2 := copyState(*s)
	o := &Output{
		Ok: nil,
	}
	ok := &struct {
		EpochMark   *EpochMark    `json:"epoch_mark"`
		TicketsMark []*TicketBody `json:"tickets_mark"`
	}{
		EpochMark:   nil,
		TicketsMark: nil,
	}
	if input.Slot <= s.Timeslot {
		return *o, s2, fmt.Errorf(errTimeslotNotMonotonic)
	}

	// tally existing ticketIDs
	ticketIDs := make(map[common.Hash]uint8)
	for i, a := range s.TicketsAccumulator {
		fmt.Printf("ticketID? %d => %s\n", i, a.Id.String())
		ticketIDs[a.Id] = a.Attempt
	}

	pubkeys := []bandersnatch.PublicKey{}
	for _, v := range s.NextValidators {
		pubkeys = append(pubkeys, v.Bandersnatch.Bytes())
	}
	ringsetBytes := bandersnatch.InitRingSet(pubkeys)

	//ringsetBytes := s.GetRingSet("Next")
	// Process Extrinsic Tickets
	fmt.Printf("Current Slot: %d => Input Slot: %d \n", s.Timeslot, input.Slot)
	newTickets := []TicketBody{}
	for _, e := range input.Extrinsics {
		if input.Slot >= EpochTail {
			return *o, s2, fmt.Errorf(errTicketSubmissionInTail)
		}
		if len(s.Entropy) == 4 {
			//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
			vrfInputData := append(append(jamTicketSeal, s.Entropy[2].Bytes()...), byte(e.Attempt))
			auxData := []byte{}
			vrfOutput, err := bandersnatch.RingVrfVerify(ringsetBytes, e.Signature[:], vrfInputData, auxData)
			if err != nil {
				return *o, s2, fmt.Errorf(errTicketBadRingProof)
			}
			_, exists := ticketIDs[common.BytesToHash(vrfOutput)]
			if exists {
				fmt.Printf("DETECTED Resubmit %x\n", vrfOutput)
				return *o, s2, fmt.Errorf(errTicketResubmission)
			}
			if e.Attempt >= NumAttempts {
				return *o, s2, fmt.Errorf(errExtrinsicWithMoreTicketsThanAllowed)
			}
			newa := TicketBody{
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
	if len(s2.TicketsAccumulator) > EpochNumSlots {
		s2.TicketsAccumulator = s2.TicketsAccumulator[0:EpochNumSlots]
	}

	// the last slot in the Epoch, with a full set of tickets
	if len(s2.TicketsAccumulator) == EpochNumSlots && input.Slot == EpochNumSlots-1 {
		ok.TicketsMark = s.computeTicketSlotBinding(s2.TicketsAccumulator)
	}
	fmt.Printf(" -- %d %d %d \n", input.Slot, EpochNumSlots, input.Slot%EpochNumSlots)
	// adjust entropy
	hasher256, err := blake2b.New256(nil)
	if err != nil {
		return *o, s2, err
	}
	hasher256.Write(append(s.Entropy[0].Bytes(), input.Entropy.Bytes()...))
	res := hasher256.Sum(nil)

	if input.Slot%EpochNumSlots == 0 {
		// New Epoch
		v := make([]common.Hash, NumValidators)
		for i, x := range s.DesignedValidators {
			v[i] = x.Bandersnatch
			fmt.Printf(" !! %d %x\n", i, v[i])
		}
		ok.EpochMark = &EpochMark{
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
