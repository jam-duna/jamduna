package telemetry

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

/*

## Block distribution events

These events concern announcement and transfer of blocks between peers.

**Implementation locations for events 60-68:**
- Block announcement streams (60-62): `node/network.go` - in UP 0 stream handlers
- Block requests (63-68): `node/network.go` - in CE 128 request/response handlers
*/

/*
### 60: Block announcement stream opened

Emitted when a block announcement stream (UP 0) is opened.

    Peer ID
    Connection Side (The side that opened the stream)
*/
// Implemented in: node/peerUP0_block.go:197 and node/peerUP0_block.go:343 (local and remote stream opening)
// BlockAnnouncementStreamOpened creates payload for block announcement stream opened event (discriminator 60)
func (c *TelemetryClient) BlockAnnouncementStreamOpened(peerID [32]byte, connectionSide byte) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Connection Side (1 byte: 0 = Local, 1 = Remote)
	payload = append(payload, connectionSide)

	c.sendEvent(Telemetry_Block_Announcement_Stream_Opened, payload)
}

// DecodeBlockAnnouncementStreamOpened decodes a Block Announcement Stream Opened event payload (discriminator 60)
func DecodeBlockAnnouncementStreamOpened(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	connectionSide, _ := parseUint8(payload, offset)

	sideStr := "local"
	if connectionSide == 1 {
		sideStr = "remote"
	}

	return fmt.Sprintf("peer_id:%s|connection_side:%s", peerID, sideStr)
}

/*
### 61: Block announcement stream closed

Emitted when a block announcement stream (UP 0) is closed. This need not be emitted if the stream
is closed due to disconnection.

    Peer ID
    Connection Side (The side that closed the stream)
    Reason
*/
// Implemented in: node/peerUP0_block.go:351 in cleanup function
// BlockAnnouncementStreamClosed creates payload for block announcement stream closed event (discriminator 61)
func (c *TelemetryClient) BlockAnnouncementStreamClosed(peerID [32]byte, connectionSide byte, reason string) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Connection Side (1 byte: 0 = Local, 1 = Remote)
	payload = append(payload, connectionSide)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Block_Announcement_Stream_Closed, payload)
}

// DecodeBlockAnnouncementStreamClosed decodes a Block Announcement Stream Closed event payload (discriminator 61)
func DecodeBlockAnnouncementStreamClosed(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	connectionSide, offset := parseUint8(payload, offset)
	reason, _ := parseString(payload, offset)

	sideStr := "local"
	if connectionSide == 1 {
		sideStr = "remote"
	}

	return fmt.Sprintf("peer_id:%s|connection_side:%s|reason:%s", peerID, sideStr, reason)
}

/*
### 62: Block announced

Emitted when a block announcement is sent to or received from a peer (UP 0).

    Peer ID
    Connection Side (Announcer)
    Slot
    Header Hash
*/
// Implemented in: node/peerUP0_block.go:398 in onBlockAnnouncement function
// BlockAnnounced creates payload for block announced event (discriminator 62)
func (c *TelemetryClient) BlockAnnounced(peerID [32]byte, connectionSide byte, slot uint32, headerHash common.Hash) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Connection Side (1 byte: 0 = Local, 1 = Remote - announcer)
	payload = append(payload, connectionSide)

	// Slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(slot)...)

	// Header Hash (32 bytes)
	payload = append(payload, headerHash.Bytes()...)

	c.sendEvent(Telemetry_Block_Announced, payload)
}

// DecodeBlockAnnounced decodes a Block Announced event payload (discriminator 62)
func DecodeBlockAnnounced(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	connectionSide, offset := parseUint8(payload, offset)
	slot, offset := parseUint32(payload, offset)
	headerHash, _ := parseHash(payload, offset)

	sideStr := "local"
	if connectionSide == 1 {
		sideStr = "remote"
	}

	return fmt.Sprintf("peer_id:%s|connection_side:%s|slot:%d|header_hash:%s", peerID, sideStr, slot, headerHash)
}

/*
### 71: Block announcement malformed

Emitted when a malformed block announcement is received via UP 0.

    Peer ID
    Reason
*/
// Implemented in: node/peerUP0_block.go:373 in onBlockAnnouncement function
// BlockAnnouncementMalformed creates payload for block announcement malformed event (discriminator 71)
func (c *TelemetryClient) BlockAnnouncementMalformed(peerID [32]byte, reason string) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Block_Announcement_Malformed, payload)
}

// DecodeBlockAnnouncementMalformed decodes a Block Announcement Malformed event payload (discriminator 71)
func DecodeBlockAnnouncementMalformed(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("peer_id:%s|reason:%s", peerID, reason)
}

/*
### 63: Sending block request

Emitted when a node begins sending a block request to a peer (CE 128).

    Peer ID (Recipient)
    Header Hash
    0 (Ascending exclusive) OR 1 (Descending inclusive) (Direction, single byte)
    u32 (Maximum number of blocks)
*/
// Implemented in: peerCE128_blockrequest.go
// SendingBlockRequest creates payload for sending block request event (discriminator 63)
func (c *TelemetryClient) SendingBlockRequest(peerID [32]byte, headerHash common.Hash, direction byte, maxBlocks uint32) {

	var payload []byte

	// Peer ID (32 bytes - recipient)
	payload = append(payload, peerID[:]...)

	// Header Hash (32 bytes)
	payload = append(payload, headerHash.Bytes()...)

	// Direction (1 byte: 0 = Ascending exclusive, 1 = Descending inclusive)
	payload = append(payload, direction)

	// Maximum number of blocks (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(maxBlocks)...)

	c.sendEvent(Telemetry_Sending_Block_Request, payload)
}

// DecodeSendingBlockRequest decodes a Sending Block Request event payload (discriminator 63)
func DecodeSendingBlockRequest(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	headerHash, offset := parseHash(payload, offset)
	direction, offset := parseUint8(payload, offset)
	maxBlocks, _ := parseUint32(payload, offset)

	directionStr := "ascending_exclusive"
	if direction == 1 {
		directionStr = "descending_inclusive"
	}

	return fmt.Sprintf("peer_id:%s|header_hash:%s|direction:%s|max_blocks:%d", peerID, headerHash, directionStr, maxBlocks)
}

/*
### 64: Receiving block request

Emitted by the recipient when a node begins sending a block request (CE 128).

    Peer ID (Sender)
*/
// Implemented in: peerCE128_blockrequest.go
// ReceivingBlockRequest creates payload for receiving block request event (discriminator 64)
func (c *TelemetryClient) ReceivingBlockRequest(peerID [32]byte) {

	var payload []byte

	// Peer ID (32 bytes - sender)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Receiving_Block_Request, payload)
}

// DecodeReceivingBlockRequest decodes a Receiving Block Request event payload (discriminator 64)
func DecodeReceivingBlockRequest(payload []byte) string {
	peerID, _ := parsePeerID(payload, 0)

	return fmt.Sprintf("peer_id:%s", peerID)
}

/*
### 65: Block request failed

Emitted when a block request (CE 128) fails.

    Event ID (ID of the corresponding "sending block request" or "receiving block request" event)
    Reason
*/
// Implemented in: peerCE128_blockrequest.go
// BlockRequestFailed creates payload for block request failed event (discriminator 65)
func (c *TelemetryClient) BlockRequestFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Block_Request_Failed, payload)
}

// DecodeBlockRequestFailed decodes a Block Request Failed event payload (discriminator 65)
func DecodeBlockRequestFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 66: Block request sent

Emitted once a block request has been sent to a peer (CE 128). This should be emitted after the
intial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending block request" event)
*/
// Implemented in: peerCE128_blockrequest.go
// BlockRequestSent creates payload for block request sent event (discriminator 66)
func (c *TelemetryClient) BlockRequestSent(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Block_Request_Sent, payload)
}

// DecodeBlockRequestSent decodes a Block Request Sent event payload (discriminator 66)
func DecodeBlockRequestSent(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 67: Block request received

Emitted once a block request has been received from a peer (CE 128).

    Event ID (ID of the corresponding "receiving block request" event)
    Header Hash
    0 (Ascending exclusive) OR 1 (Descending inclusive) (Direction, single byte)
    u32 (Maximum number of blocks)
*/
// Implemented in: peerCE128_blockrequest.go
// BlockRequestReceived creates payload for block request received event (discriminator 67)
func (c *TelemetryClient) BlockRequestReceived(eventID uint64, headerHash common.Hash, direction byte, maxBlocks uint32) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Header Hash (32 bytes)
	payload = append(payload, headerHash.Bytes()...)

	// Direction (1 byte: 0 = Ascending exclusive, 1 = Descending inclusive)
	payload = append(payload, direction)

	// Maximum number of blocks (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(maxBlocks)...)

	c.sendEvent(Telemetry_Block_Request_Received, payload)
}

// DecodeBlockRequestReceived decodes a Block Request Received event payload (discriminator 67)
func DecodeBlockRequestReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	headerHash, offset := parseHash(payload, offset)
	direction, offset := parseUint8(payload, offset)
	maxBlocks, _ := parseUint32(payload, offset)

	directionStr := "ascending_exclusive"
	if direction == 1 {
		directionStr = "descending_inclusive"
	}

	return fmt.Sprintf("event_id:%d|header_hash:%s|direction:%s|max_blocks:%d", eventID, headerHash, directionStr, maxBlocks)
}

/*
### 68: Block transferred

Emitted when a block has been fully sent to or received from a peer (CE 128).

In the case of a received block, this event may be emitted before any checks are performed. If the
block is found to be invalid or to not match the request, a "peer misbehaved" event should be
emitted; emitting a "block request failed" event is optional.

    Event ID (ID of the corresponding "sending block request" or "receiving block request" event)
    Slot
    Block Outline
    bool (Last block for the request?)
*/
// Implemented in: peerCE128_blockrequest.go
// BlockTransferred creates payload for block transferred event (discriminator 68)
func (c *TelemetryClient) BlockTransferred(eventID uint64, slot uint32, blockOutline BlockOutline, isLast bool) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(slot)...)

	// Block Outline
	payload = append(payload, encodeBlockOutline(blockOutline)...)

	// Last block for the request (1 byte: 0 = False, 1 = True)
	if isLast {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}

	c.sendEvent(Telemetry_Block_Transferred, payload)
}

// DecodeBlockTransferred decodes a Block Transferred event payload (discriminator 68)
func DecodeBlockTransferred(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	slot, offset := parseUint32(payload, offset)

	// Decode Block Outline
	sizeInBytes, offset := parseUint32(payload, offset)
	headerHash, offset := parseHash(payload, offset)
	numTickets, offset := parseUint32(payload, offset)
	numPreimages, offset := parseUint32(payload, offset)
	preimagesSizeInBytes, offset := parseUint32(payload, offset)
	numGuarantees, offset := parseUint32(payload, offset)
	numAssurances, offset := parseUint32(payload, offset)
	numDisputeVerdicts, offset := parseUint32(payload, offset)

	isLast, _ := parseUint8(payload, offset)

	return fmt.Sprintf("event_id:%d|slot:%d|size_in_bytes:%d|header_hash:%s|num_tickets:%d|num_preimages:%d|preimages_size_in_bytes:%d|num_guarantees:%d|num_assurances:%d|num_dispute_verdicts:%d|is_last:%t",
		eventID, slot, sizeInBytes, headerHash, numTickets, numPreimages, preimagesSizeInBytes, numGuarantees, numAssurances, numDisputeVerdicts, isLast != 0)
}
