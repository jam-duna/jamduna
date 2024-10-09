package types

import (
	"encoding/json"
	"fmt"
	"reflect"

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

func (t Ticket) MarshalJSON() ([]byte, error) {
	type Alias Ticket
	return json.Marshal(&struct {
		*Alias
		Signature string `json:"signature"`
	}{
		Alias:     (*Alias)(&t),
		Signature: common.HexString(t.Signature[:]),
	})
}

func (t *Ticket) UnmarshalJSON(data []byte) error {
	type Alias Ticket
	aux := &struct {
		*Alias
		Signature string `json:"signature"`
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	sigBytes := common.FromHex(aux.Signature)
	copy(t.Signature[:], sigBytes)
	return nil
}

func (t Ticket) DeepCopy() (Ticket, error) {
	var copiedTicket Ticket

	// Serialize the original Ticket to JSON
	data, err := Encode(t)
	if err != nil {
		return copiedTicket, err
	}

	// Deserialize the JSON back into a new Ticket instance
	decoded, _, err := Decode(data, reflect.TypeOf(Ticket{}))
	if err != nil {
		return copiedTicket, err
	}
	copiedTicket = decoded.(Ticket)

	return copiedTicket, nil
}

func (t Ticket) String() string {
	return fmt.Sprintf("Ticket{Attempt: %d, Signature: %s}", t.Attempt, common.HexString(t.Signature[:]))
}

func (t *Ticket) TicketIDWithCheck() (common.Hash, error) {
	ticket_id, err := bandersnatch.VRFSignedOutput(t.Signature[:])
	if err != nil {
		return common.Hash{}, fmt.Errorf("invalid ticket_id err=%v", err)
	}
	return common.BytesToHash(ticket_id), err
}

func (t *Ticket) TicketID() (common.Hash, error) {
	ticket_id, err := bandersnatch.VRFSignedOutput(t.Signature[:])
	if err != nil {
		return common.Hash{}, fmt.Errorf("invalid ticket_id err=%v", err)
	}
	return common.BytesToHash(ticket_id), nil
}

func (t *Ticket) Bytes() ([]byte, error) {
	return t.BytesWithSig(), nil
}

func (t *Ticket) UnsignedHash() common.Hash {
	unsignedBytes := t.BytesWithoutSig()
	return common.Blake2Hash(unsignedBytes)
}

func (t *Ticket) Hash() common.Hash {
	data := t.BytesWithSig()
	return common.Blake2Hash(data)
}

func (t *Ticket) BytesWithSig() []byte {
	enc, err := Encode(t)
	if err != nil {
		return nil
	}
	return enc
}

func (t *Ticket) BytesWithoutSig() []byte {
	s := struct {
		Attempt uint8 `json:"attempt"`
	}{
		Attempt: t.Attempt,
	}
	enc, err := Encode(s)
	if err != nil {
		return nil
	}
	fmt.Printf("BytesWithoutSig %x\n", enc)
	return enc
}

// shawn added ticket bucket to store tickets for node itself

type TicketBucket struct {
	Ticket         Ticket
	IsIncluded     *bool
	IsBroadcasted  *bool
	GeneratedEpoch uint32
}

func (t *Ticket) TicketToBucket(epoch uint32) TicketBucket {
	return TicketBucket{
		Ticket:         *t,
		IsIncluded:     new(bool),
		IsBroadcasted:  new(bool),
		GeneratedEpoch: epoch,
	}
}

func TicketsToBuckets(tickets []Ticket, epoch uint32) []TicketBucket {
	bucket := make([]TicketBucket, len(tickets))
	for i := range tickets {
		bucket[i] = tickets[i].TicketToBucket(epoch)
	}
	return bucket
}
