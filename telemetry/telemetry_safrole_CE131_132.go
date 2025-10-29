package telemetry

import (
	"fmt"

	"github.com/colorfulnotion/jam/types"
)

/*

## Safrole ticket events

These events concern generation and distribution of tickets for the Safrole lottery.

**Implementation locations for events 80-84:**
- Ticket generation (80-82): `statedb/safrole.go` - in VRF ticket generation logic
- Ticket transfer (83-84): `node/network.go` - in CE 131/132 handlers
*/

/*
### 80: Generating tickets

Emitted when generation of a new set of Safrole tickets begins.

    Epoch Index (The epoch the tickets are to be used in)
*/
// Implemented in: node/node_ticket.go:74 in GenerateTickets function
// GeneratingTickets creates payload for generating tickets event (discriminator 80)
func (c *TelemetryClient) GeneratingTickets(epochIndex uint32) {

	var payload []byte

	// Epoch Index (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(epochIndex)...)

	c.sendEvent(Telemetry_Generating_Tickets, payload)
}

// DecodeGeneratingTickets decodes a Generating Tickets event payload (discriminator 80)
func DecodeGeneratingTickets(payload []byte) string {
	epochIndex, _ := parseUint32(payload, 0)

	return fmt.Sprintf("epoch_index:%d", epochIndex)
}

/*
### 81: Ticket generation failed

Emitted if Safrole ticket generation fails.

    Event ID (ID of the corresponding "generating tickets" event)
    Reason
*/
// Implemented in: node/node_ticket.go:80 in GenerateTickets function
// TicketGenerationFailed creates payload for ticket generation failed event (discriminator 81)
func (c *TelemetryClient) TicketGenerationFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Ticket_Generation_Failed, payload)
}

// DecodeTicketGenerationFailed decodes a Ticket Generation Failed event payload (discriminator 81)
func DecodeTicketGenerationFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 82: Tickets generated

Emitted once a set of Safrole tickets has been generated.

    Event ID (ID of the corresponding "generating tickets" event)
    len++[[u8; 32]] (Ticket VRF outputs, index is attempt number)
*/
// Implemented in: node/node_ticket.go:94 in GenerateTickets function
// TicketsGenerated creates payload for tickets generated event (discriminator 82)
func (c *TelemetryClient) TicketsGenerated(eventID uint64, vrfOutputs [][32]byte) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// len++[[u8; 32]] - VRF outputs indexed by attempt number
	payload = append(payload, types.E(uint64(len(vrfOutputs)))...)
	for _, output := range vrfOutputs {
		payload = append(payload, output[:]...)
	}

	c.sendEvent(Telemetry_Tickets_Generated, payload)
}

// DecodeTicketsGenerated decodes a Tickets Generated event payload (discriminator 82)
func DecodeTicketsGenerated(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse VRF outputs length
	vrfOutputsLength, offset := parseVariableLength(payload, offset)

	vrfOutputs := ""
	for i := uint64(0); i < vrfOutputsLength && offset+32 <= len(payload); i++ {
		if i > 0 {
			vrfOutputs += ","
		}
		vrfOutput, newOffset := parseHash(payload, offset)
		offset = newOffset
		vrfOutputs += vrfOutput
	}

	return fmt.Sprintf("event_id:%d|vrf_outputs:[%s]", eventID, vrfOutputs)
}

/*
### 83: Ticket transfer failed

Emitted when a Safrole ticket send or receive fails (CE 131/132).

    Peer ID
    Connection Side (Sender)
    bool (Was CE 132 used?)
    Reason
*/
// Implemented in: peerCE131_ticketdistribution.go
// TicketTransferFailed creates payload for ticket transfer failed event (discriminator 83)
func (c *TelemetryClient) TicketTransferFailed(peerID [32]byte, connectionSide byte, wasCE132 bool, reason string) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Connection Side (1 byte: 0 = Local, 1 = Remote - sender)
	payload = append(payload, connectionSide)

	// Was CE 132 used? (1 byte: 0 = False, 1 = True)
	if wasCE132 {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Ticket_Transfer_Failed, payload)
}

// DecodeTicketTransferFailed decodes a Ticket Transfer Failed event payload (discriminator 83)
func DecodeTicketTransferFailed(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	connectionSide, offset := parseUint8(payload, offset)
	wasCE132, offset := parseUint8(payload, offset)
	reason, _ := parseString(payload, offset)

	sideStr := "local"
	if connectionSide == 1 {
		sideStr = "remote"
	}

	return fmt.Sprintf("peer_id:%s|connection_side:%s|was_ce132:%t|reason:%s", peerID, sideStr, wasCE132 != 0, reason)
}

/*
### 84: Ticket transferred

Emitted when a Safrole ticket is sent to or received from a peer (CE 131/132).

In the case of a received ticket, this should be emitted before the ticket is checked. If the
ticket is found to be invalid, a "peer misbehaved" event should be emitted.

    Peer ID
    Connection Side (Sender)
    bool (Was CE 132 used?)
    Epoch Index (The epoch the ticket is to be used in)
    0 OR 1 (Single byte, ticket attempt number)
    [u8; 32] (VRF output)
*/
// Implemented in: peerCE131_ticketdistribution.go
// TicketTransferred creates payload for ticket transferred event (discriminator 84)
func (c *TelemetryClient) TicketTransferred(peerID [32]byte, connectionSide byte, wasCE132 bool, epochIndex uint32, attemptNumber byte, vrfOutput [32]byte) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Connection Side (1 byte: 0 = Local, 1 = Remote - sender)
	payload = append(payload, connectionSide)

	// Was CE 132 used? (1 byte: 0 = False, 1 = True)
	if wasCE132 {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}

	// Epoch Index (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(epochIndex)...)

	// Ticket attempt number (1 byte: 0 or 1)
	payload = append(payload, attemptNumber)

	// VRF output (32 bytes)
	payload = append(payload, vrfOutput[:]...)

	c.sendEvent(Telemetry_Ticket_Transferred, payload)
}

// DecodeTicketTransferred decodes a Ticket Transferred event payload (discriminator 84)
func DecodeTicketTransferred(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	connectionSide, offset := parseUint8(payload, offset)
	wasCE132, offset := parseUint8(payload, offset)
	epochIndex, offset := parseUint32(payload, offset)
	attemptNumber, offset := parseUint8(payload, offset)
	vrfOutput, _ := parseHash(payload, offset)

	sideStr := "local"
	if connectionSide == 1 {
		sideStr = "remote"
	}

	return fmt.Sprintf("peer_id:%s|connection_side:%s|was_ce132:%t|epoch_index:%d|attempt_number:%d|vrf_output:%s", peerID, sideStr, wasCE132 != 0, epochIndex, attemptNumber, vrfOutput)
}
