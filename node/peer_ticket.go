package node

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/types"
)

/*
CE 131/132: Safrole ticket distribution
Sharing of a Safrole ticket for inclusion in a block.

Safrole tickets are distributed (and included on chain) in the epoch prior to the one in which they are used.
They are distributed in two steps. Each ticket is first sent from the generating validator to a deterministically-selected "proxy" validator.
This proxy validator then sends the ticket to all current validators.

Protocol 131 is used for the first step (generating validator to proxy validator), protocol 132 is used for the second step (proxy validator to all current validators).
Both protocols look the same on the wire; the difference is only in which step they are used for.

The first step should be performed shortly after the connectivity changes for a new epoch are applied. The index of the proxy validator
for a ticket is determined by interpreting the last 4 bytes of the ticket's VRF output as a big-endian unsigned integer, modulo the number of validators.
The proxy validator is selected from the next epoch's validator list. If the generating validator is chosen as the proxy validator,
then the first step should effectively be skipped and the generating validator should distribute the ticket to the current validators itself, as per the following section.

Proxy validators should verify the proof of any ticket they receive, and verify that they are the correct proxy for the ticket.
If these checks succeed, they should forward the ticket to all current validators. Forwarding should be delayed until 2 minutes into the epoch,
to avoid exposing the timing of the message from the generating validator. Forwarding should be evenly spaced out from this point until half-way through the Safrole lottery period.
Forwarding may be stopped if the ticket is included in a finalized block.

If finality is running far enough behind that the state required to verify a received ticket is not known with certainty, the stream should be reset/stopped.
This applies to both protocol 131 and 132.

Epoch Index = u32 (Should identify the epoch that the ticket will be used in)
Attempt = { 0, 1 } (Single byte)
Bandersnatch RingVRF Proof = [u8; 784]
Ticket = Epoch Index ++ Attempt ++ Bandersnatch RingVRF Proof

Validator -> Validator

--> Ticket
--> FIN
<-- FIN
*/
type JAMSNPTicketDistribution struct {
	Epoch     uint32                          `json:"epoch"`
	Attempt   uint8                           `json:"attempt"`
	Signature types.BandersnatchRingSignature `json:"signature"`
}

// ToBytes serializes the JAMSNPTicket struct into a byte array
func (ticket *JAMSNPTicketDistribution) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize Epoch (4 bytes)
	if err := binary.Write(buf, binary.BigEndian, ticket.Epoch); err != nil {
		return nil, err
	}

	// Serialize Attempt (1 byte)
	if err := binary.Write(buf, binary.BigEndian, ticket.Attempt); err != nil {
		return nil, err
	}

	// Serialize Signature (Assumes BandersnatchRingSignature has its own ToBytes method)
	if _, err := buf.Write(ticket.Signature[:]); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPTicket struct
func (ticket *JAMSNPTicketDistribution) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize Epoch (4 bytes)
	if err := binary.Read(buf, binary.BigEndian, &ticket.Epoch); err != nil {
		return err
	}

	// Deserialize Attempt (1 byte)
	if err := binary.Read(buf, binary.BigEndian, &ticket.Attempt); err != nil {
		return err
	}

	// Deserialize Signature (Assumes BandersnatchRingSignature has its own FromBytes method)
	copy(ticket.Signature[:], data[5:])
	return nil
}

func (p *Peer) SendTicketDistribution(epoch uint32, t types.Ticket, isProxy bool) (err error) {
	req := &JAMSNPTicketDistribution{
		Epoch:     epoch,
		Attempt:   t.Attempt,
		Signature: t.Signature,
	}

	reqBytes, err := req.ToBytes()
	if err != nil {
		return err
	}
	if isProxy {
		p.sendCode(CE131_TicketDistribution)
	} else {
		p.sendCode(CE132_TicketDistribution)
	}
	// TODO: proper treatment of Proxy
	err = p.sendQuicBytes(reqBytes)
	if err != nil {
		return err
	}
	// --> FIN
	p.sendFIN()
	// <-- FIN
	p.receiveFIN()

	return nil
}

func (p *Peer) processTicketDistribution(msg []byte) (err error) {
	var newReq JAMSNPTicketDistribution
	// Deserialize byte array back into the struct
	err = newReq.FromBytes(msg)
	if err != nil {
		fmt.Println("Error deserializing:", err)
		return
	}
	// --> FIN
	p.receiveFIN()

	p.node.OnTicketDistribution(newReq.Epoch, newReq.Attempt, newReq.Signature)
	// <-- FIN
	p.sendFIN()
	return
}
