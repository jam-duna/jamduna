package safrole

import (
	"fmt"
	ring_vrf "github.com/colorfulnotion/jam/ring-vrf"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
)

type Safrole struct {
	RandomnessBuffer RandomnessBuffer
	Tickets          [EpochNumSlots]TicketEnvelope
	OrderedTickets   []TicketEnvelope
}

// RandomnessBuffer is initialized after the genesis block construction.
type RandomnessBuffer struct {
	Current common.Hash
	EpochN2 common.Hash
	EpochN1 common.Hash
	EpochN0 common.Hash
}

func (s *Safrole) initializeRandomnessBuffer(genesisBlock *BlockHeader) {
	rd := &s.RandomnessBuffer
	d, _ := genesisBlock.Encode()
	rd.Current = common.BytesToHash(computeHash(d))
	rd.EpochN2 = common.BytesToHash(computeHash(rd.Current.Bytes()))
	rd.EpochN1 = common.BytesToHash(computeHash(rd.EpochN2.Bytes()))
	rd.EpochN0 = common.BytesToHash(computeHash(rd.EpochN1.Bytes()))
}

var (
	sassafrasTicketSeal   = []byte("sassafras_ticket_seal")
	sassafrasFallbackSeal = []byte("sassafras_fallback_seal")
	sassafrasRandomness   = []byte("sassafras_randomness")
)

// ticketSealVRFInput constructs the input for VRF based on target epoch randomness and attempt.
func (s *Safrole) ticketSealVRFInput(targetEpochRandomness common.Hash, attempt byte) []byte {
	return append(append([]byte(sassafrasTicketSeal), targetEpochRandomness.Bytes()...), attempt)
}

func (s *Safrole) computeTicketIDSecretKey(secret [32]byte, targetEpochRandomness common.Hash, attempt byte) ([]byte, []byte) {
	ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, attempt)
	message := []byte{}
	signature, ticketID := ring_vrf.RingVRFSignSimple(secret, sassafrasTicketSeal, message, ticketVRFInput)
	return signature, ticketID
}

func (s *Safrole) validTicket(ticketID common.Hash) bool {
	T := new(big.Int).SetUint64(RedundancyFactor * EpochNumSlots / (NumAttempts * NumValidators))
	return ticketID.Big().Cmp(T) < 0
}

func (s *Safrole) validateTicket(ticketID common.Hash, targetEpochRandomness common.Hash, envelope *TicketEnvelope) error {
	ticketVRFInput := s.ticketSealVRFInput(targetEpochRandomness, envelope.Attempt)
	err := ring_vrf.RingVRFVerifySimple(ticketVRFInput, envelope.Extra, envelope.RingSignature)
	if err != nil {
		return err
	}
	computedTicketID := ring_vrf.RingVRFSignedOutput(envelope.RingSignature)
	if s.validTicket(common.BytesToHash(computedTicketID)) {
		return nil
	}
	return fmt.Errorf("invalid ticket")
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

func (s *Safrole) queueTicketEnvelope(envelope *TicketEnvelope) error {
	err := s.validateTicket(envelope.TicketID, s.RandomnessBuffer.Current, envelope)
	if err != nil {
		return err
	}
	// Add envelope to s.Tickets and sort
	// Implement logic to add envelope to s.Tickets
	// Sort tickets using compareTickets
	return nil
}

// computeTicketSlotBinding has logic for assigning tickets to slots
func (s *Safrole) computeTicketSlotBinding() {
	i := 0
	tickets := make([]TicketEnvelope, 0, EpochNumSlots)
	for i < len(s.Tickets)/2 {
		tickets = append(tickets, s.Tickets[i])
		tickets = append(tickets, s.Tickets[EpochNumSlots-1-i])
		i++
	}
	s.OrderedTickets = tickets
	if len(s.OrderedTickets) < EpochNumSlots {
		// Handle case where not all slots are filled with Fallback
	}
}
