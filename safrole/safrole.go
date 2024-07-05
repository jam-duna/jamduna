package safrole

import (
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/ethereum/go-ethereum/common"
	//"sort"
)

const (
	BlsSizeInBytes            = 144
	MetadataSizeInBytes       = 128
	ExtrinsicSignatureInBytes = 784
	TicketsVerifierKeyInBytes = 144
)

const (
	// tiny (NOTE: large is 600 + 1023, should use ProtocolConfiguration)
	EpochNumSlots = 12
	NumValidators = 6

	RedundancyFactor = 1
	NumAttempts      = 100
)

// 6.1 protocol configuration
type ProtocolConfiguration struct {
	EpochNumSlots    uint32 // number of slots for each epoch (The "v")
	NumAttempts      uint8  // maximum number of tickets each authority can submit ("")
	RedundancyFactor uint8  // expected ratio between epoch slots and valid tickets
}

type Input struct {
	Slot       int         `json:"slot"`
	Entropy    common.Hash `json:"entropy"`
	Extrinsics []Extrinsic `json:"extrinsics"`
}

type Validator struct {
	Ed25519      common.Hash               `json:"ed25519"`
	Bandersnatch common.Hash               `json:"bandersnatch"`
	Bls          [BlsSizeInBytes]byte      `json:"bls"`
	Metadata     [MetadataSizeInBytes]byte `json:"metadata"`
}

// Extrinsic is submitted by authorities, which are added to Safrole State in TicketsAccumulator if valid
type Extrinsic struct {
	Attempt   int                             `json:"attempt"`
	Signature [ExtrinsicSignatureInBytes]byte `json:"signature"`
}

type SafroleAccumulator struct {
	Id      common.Hash `json:"id"`
	Attempt int         `json:"attempt"`
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
)

type State struct {
	Timeslot int `json:"timeslot"`
	// Entropy holds 4 32 byte
	// 1. CURRENT randomness accumulator (see sec 6.10). randomness_buffer[0] = BLAKE2(CONCAT(randomness_buffer[0], fresh_randomness));
	// where fresh_randomness = vrf_signed_output(claim.randomness_source)
	// 2. accumulator snapshot BEFORE the execution of the first block of epoch N   - rnadomness used for ticket targeting epoch N+2
	// 3. accumulator snapshot BEFORE the execution of the first block of epoch N-1 - rnadomness used for ticket targeting epoch N+1
	// 4. accumulator snapshot BEFORE the execution of the first block of epoch N-2 - rnadomness used for ticket targeting current epoch N
	Entropy []common.Hash `json:"entropy"`

	// 4 authorities[pre, curr, next, designed]
	PrevValidators     []Validator `json:"prev_validators"`
	CurrValidators     []Validator `json:"curr_validators"`
	NextValidators     []Validator `json:"next_validators"`
	DesignedValidators []Validator `json:"designed_validators"`

	// Accumulator of tickets, modified with Extrinsics to hold ORDERED array of Tickets
	TicketsAccumulator []SafroleAccumulator `json:"tickets_accumulator"`
	TicketsOrKeys      struct {
		Keys []common.Hash `json:"keys"`
	} `json:"tickets_or_keys"`

	// []bandersnatch.ValidatorKeySet
	TicketsVerifierKey []byte `json:"tickets_verifier_key"`
}

// EpochMark (see 6.4 Epoch change Signal) represents the descriptor for parameters to be used in the next epoch
type EpochMark struct {
	// Randomness accumulator snapshot
	Entropy []byte `json:"entropy"`
	// List of authorities scheduled for next epoch -- could be []PublicKey
	Validators [][]byte `json:"validators"`
}

// PublicKey represents a public key
type PublicKey bandersnatch.PublicKey

type Output struct {
	Ok struct {
		EpochMark   EpochMark            `json:"epoch_mark"`
		TicketsMark []SafroleAccumulator `json:"tickets_mark"`
	} `json:"ok"`
}

// STF Errors
const (
	errTimeslotNotMonotonic                = "Fail: Timeslot must be strictly monotonic."
	errExtrinsicWithMoreTicketsThanAllowed = "Fail: Submit an extrinsic with more tickets than allowed."
	errTicketResubmission                  = "Fail: Re-submit tickets from authority 0."
	errTicketBadOrder                      = "Fail: Submit tickets in bad order."
	errTicketBadRingProof                  = "Fail: Submit tickets with bad ring proof."
	errTicketSubmissionInTail              = "Fail: Submit a ticket while in epoch's tail."
	errNone                                = "ok"
)

// 6.4.1 Startup parameters
func NewState(genesisBlock *BlockHeader) *State {
	//d, _ := genesisBlock.Encode()
	//s.Entropy[0] = common.BytesToHash(computeHash(d))                  //BLAKE2B hash of the genesis block#0
	//s.Entropy[1] = common.BytesToHash(computeHash(rd.Current.Bytes())) //BLAKE2B of Current
	//s.Entropy[2] = common.BytesToHash(computeHash(rd.EpochN2.Bytes())) //BLAKE2B of EpochN1
	//s.Entropy[3] = common.BytesToHash(computeHash(rd.EpochN1.Bytes())) //BLAKE2B of EpochN2
	return &State{}
}

// 6.5.1. Ticket Identifier (Primary Method)
// ticketSealVRFInput constructs the input for VRF based on target epoch randomness and attempt.
func (s *State) ticketSealVRFInput(targetEpochRandomness common.Hash, attempt int) []byte {
	// Concatenate sassafrasTicketSeal, targetEpochRandomness, and attemptBytes
	ticket_vrf_input := append(append(jamTicketSeal, targetEpochRandomness.Bytes()...), []byte{byte(attempt & 0xF)}...)
	return ticket_vrf_input
}

func (s *State) computeTicketID(authority_secret_key bandersnatch.SecretKey, ticket_vrf_input []byte) common.Hash {
	ticket_id := bandersnatch.VRFOutput(authority_secret_key, ticket_vrf_input)
	return common.BytesToHash(ticket_id)
}

func (s *State) computeTicketIDSecretKey(secret bandersnatch.SecretKey, targetEpochRandomness common.Hash, attempt int) ([]byte, []byte) {
	ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, attempt)
	message := []byte{}
	signature, ticketID := bandersnatch.RingVRFSign(secret, jamTicketSeal, message, ticketVRFInput) // ??
	return signature, ticketID
}

// 6.6. On-chain Tickets Validation
func (s *State) validateTicket(ticketID common.Hash, targetEpochRandomness common.Hash, envelope *TicketEnvelope) error {

	//step 0: derive ticketVRFInput
	ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, envelope.Attempt)

	//step 1: verify envelope's VRFSignature using ring verifier
	var pubkeys [][]byte
	_, result := bandersnatch.RingVRFVerify(pubkeys, ticketVRFInput, envelope.Extra, envelope.RingSignature[:])
	if result != 1 {
		return fmt.Errorf("Bad signature")
	}

	return nil
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
func (s *State) queueTicketEnvelope(envelope *TicketEnvelope) error {
	return nil
	/*
		err := s.validateTicket(envelope.Id, s.Entropy[0], envelope)
		if err != nil {
			return err
		}
		// Add envelope to s.TicketsAccumulator as "t"
		t := SafroleAccumulator{
			Id:      envelope.Id,
			Attempt: envelope.Attempt,
		}
		s.TicketsAccumulator = append(s.TicketsAccumulator, t)
		// Sort tickets using compareTickets
		sort.SliceStable(s.TicketsAccumulator, func(i, j int) bool {
			return compareTickets(s.TicketsAccumulator[i].Id, s.TicketsAccumulator[j].Id) < 0
		})

		return nil */
}

// computeTicketSlotBinding has logic for assigning tickets to slots
func (s *State) computeTicketSlotBinding() {
	i := 0
	tickets := make([]SafroleAccumulator, 0, EpochNumSlots)
	for i < len(s.TicketsAccumulator)/2 {
		tickets = append(tickets, s.TicketsAccumulator[i])
		tickets = append(tickets, s.TicketsAccumulator[EpochNumSlots-1-i])
		i++
	}
	//s.OrderedTickets = tickets
	//if len(s.OrderedTickets) < EpochNumSlots {
	// orphan block - where not all slots are filled with Fallback
	// 6.8.2 Slot Claim - Secondary Method
	//}
	fmt.Printf("%d Tickets\n", len(tickets))
}

//validate claim
//6.8.1 Primary Method for legit ticket - is it bare VRF here???

// 6.8.2. Secondary Method
func fallbackSealVRFInput(targetRandomness common.Hash) []byte {
	sealPrefix := []byte(jamFallbackSeal)
	sealVRFInput := append(sealPrefix, targetRandomness.Bytes()...)
	return sealVRFInput
}

// computeAuthorityIndex computes the authority index for claiming an orphan slot
func (s *State) computeFallbackAuthorityIndex(targetRandomness common.Hash, relativeSlotIndex uint32) (uint32, error) {
	// Concatenate target_randomness and relative_slot_index
	hashInput := append(targetRandomness.Bytes(), uint32ToBytes(relativeSlotIndex)...)

	// Compute BLAKE2 hash
	hash := computeHash(hashInput)

	// Extract the first 4 bytes of the hash to compute the index
	indexBytes := hash[:4]
	index := binary.BigEndian.Uint32(indexBytes) % uint32(len(s.DesignedValidators))
	//fmt.Printf("targetRandomness=%x, relativeSlotIndex=%v, indexBytes=%x, priv_index=%v. (authLen=%v)", targetRandomness, relativeSlotIndex, indexBytes, index, len(s.Authorities))
	return index, nil
}

func (s *State) validateExtrinsic(e Extrinsic) (*TicketEnvelope, error) {

	return &TicketEnvelope{
		Id:            blake2AsHex(e.Signature[:]),
		Attempt:       e.Attempt,
		Extra:         []byte{},
		RingSignature: e.Signature,
	}, nil
}

// statefrole_stf is the function to be tested
func (s *State) STF(input Input) (Output, State, error) {

	s2 := copyState(*s)
	o := Output{}
	if input.Slot <= s2.Timeslot {
		return o, s2, fmt.Errorf(errTimeslotNotMonotonic)
	}
	s2.Timeslot = input.Slot

	/* TODO: detect invalid transition
	epoch change signal - observed in first block of a new epoch?
	epoch ticket signal - observed is first block in epoch TAIL?
	slot claim info - observed in second to last entry of every block?
	seal - block signature. observed in last entry of every block?
	*/

	for _, e := range input.Extrinsics {
		env, err := s.validateExtrinsic(e)
		if err != nil {
			fmt.Printf("Invalid extrinsic %v", e)
			continue
		}

		err = s.queueTicketEnvelope(env)

	}

	pubkeys := [][]byte{}
	for _, v := range s.NextValidators {
	    pubkeys = append(pubkeys, v.Bandersnatch.Bytes())
	}
	for _, e := range input.Extrinsics {
	    if ( len(s.Entropy) == 4 ) {
	    	vrfInputData := append(append([]byte("jam_ticket_seal"), s.Entropy[2].Bytes()...), byte(e.Attempt))
	        vrfOutput, result := bandersnatch.RingVRFVerify(pubkeys, vrfInputData, []byte{}, e.Signature[:])
	        fmt.Printf("%x %d\n", vrfOutput, result);
	     } else {
	        fmt.Printf("fff %d\n", len(s.Entropy))
	     }
	}

	return o, s2, fmt.Errorf("ok")
}
