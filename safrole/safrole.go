package safrole

import (
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/blake2b"
	"sort"
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
	NumAttempts   = 2
	Y             = 10
)

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
	Entropy common.Hash `json:"entropy"`
	// List of authorities scheduled for next epoch -- could be []PublicKey
	Validators []common.Hash `json:"validators"`
}

// PublicKey represents a public key
type PublicKey bandersnatch.PublicKey

type Output struct {
	Ok *struct {
		EpochMark   *EpochMark            `json:"epoch_mark"`
		TicketsMark []*SafroleAccumulator `json:"tickets_mark"`
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

func (s *State) computeTicketID(authority_secret_key bandersnatch.PrivateKey, ticket_vrf_input []byte) (common.Hash, error) {
	//ticket_id := bandersnatch.VRFOutput(authority_secret_key, ticket_vrf_input)
	auxData := []byte{}
	ticket_id, err := bandersnatch.VRFOutput(authority_secret_key, ticket_vrf_input, auxData)
	if err != nil {
		return common.Hash{}, fmt.Errorf("signTicket failed")
	}
	return common.BytesToHash(ticket_id), nil
}

func (s *State) getRingSet(phase string) (ringsetBytes []byte){
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

func (s *State) generateTicket(secret bandersnatch.PrivateKey, targetEpochRandomness common.Hash, attempt int) ([]byte, []byte, error) {
	ticket_vrf_input := s.ticketSealVRFInput(targetEpochRandomness, attempt)
	auxData := []byte{}
	//RingVrfSign(privateKey, ringsetBytes, vrfInputData, auxData []byte)
	//During epoch N, each authority scheduled for epoch N+2 constructs a set of tickets which may be eligible (6.5.2) for on-chain submission.
	ringsetBytes := s.getRingSet("Designed")
	signature, ticketID, err := bandersnatch.RingVrfSign(secret, ringsetBytes, ticket_vrf_input, auxData) // ??
	if err != nil {
		return signature, ticketID, fmt.Errorf("signTicket failed")
	}
	return signature, ticketID, nil
}

// 6.6. On-chain Tickets Validation
func (s *State) validateTicket(ticketID common.Hash, targetEpochRandomness common.Hash, envelope *TicketEnvelope) (common.Hash, error) {

	//step 0: derive ticketVRFInput
	ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, envelope.Attempt)

	//step 1: verify envelope's VRFSignature using ring verifier
	//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
	ringsetBytes := s.getRingSet("Designed")
	ticket_id, err := bandersnatch.RingVrfVerify(ringsetBytes, envelope.RingSignature[:], ticketVRFInput, envelope.Extra)
	if err != nil {
		return common.Hash{}, fmt.Errorf("Bad signature")
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

		return nil */
}

// computeTicketSlotBinding has logic for assigning tickets to slots
func (s *State) computeTicketSlotBinding(inp []SafroleAccumulator) []*SafroleAccumulator {
	i := 0
	tickets := make([]*SafroleAccumulator, 0, EpochNumSlots)
	for i < len(s.TicketsAccumulator)/2 {
		tickets = append(tickets, &inp[i])
		tickets = append(tickets, &inp[EpochNumSlots-1-i])
		i++
	}
	return tickets
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
	o := &Output{
		Ok: nil,
	}
	ok := &struct {
		EpochMark   *EpochMark            `json:"epoch_mark"`
		TicketsMark []*SafroleAccumulator `json:"tickets_mark"`
	}{
		EpochMark:   nil,
		TicketsMark: nil,
	}
	if input.Slot <= s.Timeslot {
		return *o, s2, fmt.Errorf(errTimeslotNotMonotonic)
	}

	// tally existing ticketIDs
	ticketIDs := make(map[common.Hash]int)
	for i, a := range s.TicketsAccumulator {
		fmt.Printf("ticketID? %d => %s\n", i, a.Id.String())
		ticketIDs[a.Id] = a.Attempt
	}

	/*
	pubkeys := []bandersnatch.PublicKey{}
	for _, v := range s.NextValidators {
		pubkeys = append(pubkeys, v.Bandersnatch.Bytes())
	}
	*/
	ringsetBytes := s.getRingSet("Designed")
	// Process Extrinsic Tickets
	fmt.Printf("Current Slot: %d => Input Slot: %d \n", s.Timeslot, input.Slot)
	newTickets := []SafroleAccumulator{}
	for _, e := range input.Extrinsics {
		if input.Slot >= Y {
			return *o, s2, fmt.Errorf(errTicketSubmissionInTail)
		}
		if len(s.Entropy) == 4 {
			vrfInputData := append(append([]byte("jam_ticket_seal"), s.Entropy[2].Bytes()...), byte(e.Attempt))
			vrfOutput, err := bandersnatch.RingVrfVerify(ringsetBytes, e.Signature[:], vrfInputData, []byte{})
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
			newa := SafroleAccumulator{
				Id:      common.BytesToHash(vrfOutput),
				Attempt: e.Attempt,
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
