package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
)

/*
Section 6.7 - Equation 73.  Ticket Extrinsic is a *sequence* of proofs of valid tickets, each of which is a tuple of an entry index (a natural number less than N) and a proof of ticket validity.
*/

type Ticket struct {
	Attempt   uint8                     `json:"attempt"`
	Signature BandersnatchRingSignature `json:"signature"`
}

type STicket struct {
	Attempt   uint8  `json:"attempt"`
	Signature string `json:"signature"`
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

func (s *STicket) Deserialize() (Ticket, error) {
	signatureBytes := common.FromHex(s.Signature)
	var signature BandersnatchRingSignature
	copy(signature[:], signatureBytes)

	return Ticket{
		Attempt:   s.Attempt,
		Signature: signature,
	}, nil
}
