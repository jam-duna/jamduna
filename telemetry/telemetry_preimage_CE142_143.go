package telemetry

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

/*

## Preimage distribution events

These events concern distribution of preimages for inclusion in blocks.

**Implementation locations for events 190-199:**
- Preimage announcements (190-192): `node/network.go` - in CE 142 handlers
- Preimage requests (193-198): `node/network.go` - in CE 143 handlers
- Preimage pool management (192, 199): `node/preimage_pool.go` - in pool logic
*/

/*
### 190: Preimage announcement failed

Emitted when a preimage announcement fails (CE 142).

    Peer ID
    Connection Side (Announcer)
    Reason
*/
// Implemented in: peerCE142_peerimageannouncement.go
// PreimageAnnouncementFailed creates payload for preimage announcement failed event (discriminator 190)
func (c *TelemetryClient) PreimageAnnouncementFailed(peerID [32]byte, connectionSide byte, reason string) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Connection Side (1 byte: 0 = Local, 1 = Remote - announcer)
	payload = append(payload, connectionSide)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Preimage_Announcement_Failed, payload)
}

// DecodePreimageAnnouncementFailed decodes a Preimage Announcement Failed event payload (discriminator 190)
func DecodePreimageAnnouncementFailed(payload []byte) string {
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
### 191: Preimage announced

Emitted when a preimage announcement is sent to or received from a peer (CE 142).

    Peer ID
    Connection Side (Announcer)
    Service ID (Requesting service)
    Hash
    u32 (Preimage length)
*/
// Implemented in: peerCE142_peerimageannouncement.go
// PreimageAnnounced creates payload for preimage announced event (discriminator 191)
func (c *TelemetryClient) PreimageAnnounced(peerID [32]byte, connectionSide byte, serviceID uint32, hash common.Hash, preimageLength uint32) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Connection Side (1 byte: 0 = Local, 1 = Remote - announcer)
	payload = append(payload, connectionSide)

	// Service ID (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(serviceID)...)

	// Hash (32 bytes)
	payload = append(payload, hash.Bytes()...)

	// Preimage length (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(preimageLength)...)

	c.sendEvent(Telemetry_Preimage_Announced, payload)
}

// DecodePreimageAnnounced decodes a Preimage Announced event payload (discriminator 191)
func DecodePreimageAnnounced(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	connectionSide, offset := parseUint8(payload, offset)
	serviceID, offset := parseUint32(payload, offset)
	hash, offset := parseHash(payload, offset)
	preimageLength, _ := parseUint32(payload, offset)

	sideStr := "local"
	if connectionSide == 1 {
		sideStr = "remote"
	}

	return fmt.Sprintf("peer_id:%s|connection_side:%s|service_id:%d|hash:%s|preimage_length:%d", peerID, sideStr, serviceID, hash, preimageLength)
}

/*
### 192: Announced preimage forgotten

Emitted when a preimage announced by a peer is forgotten about. This event should not be emitted
for preimages the node managed to acquire (if such a preimage is discarded, a "preimage discarded"
event should be emitted instead).

	Service ID (Requesting service)
	Hash
	u32 (Preimage length)
	Announced Preimage Forget Reason

Announced Preimage Forget Reason =

	0 (Provided on-chain) OR
	1 (Not requested on-chain) OR
	2 (Failed to acquire preimage) OR
	3 (Too many announced preimages) OR
	4 (Bad length) OR
	5 (Other)
	(Single byte)
*/
const (
	AnnouncedPreimageForgetReason_ProvidedOnChain     = 0 // Provided on-chain
	AnnouncedPreimageForgetReason_NotRequestedOnChain = 1 // Not requested on-chain
	AnnouncedPreimageForgetReason_FailedToAcquire     = 2 // Failed to acquire preimage
	AnnouncedPreimageForgetReason_TooManyAnnounced    = 3 // Too many announced preimages
	AnnouncedPreimageForgetReason_BadLength           = 4 // Bad length
	AnnouncedPreimageForgetReason_Other               = 5 // Other
)

// Implemented in: peerCE142_peerimageannouncement.go
// AnnouncedPreimageForgotten creates payload for announced preimage forgotten event (discriminator 192)
func (c *TelemetryClient) AnnouncedPreimageForgotten(serviceID uint32, hash common.Hash, preimageLength uint32, forgetReason byte) {

	var payload []byte

	// Service ID (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(serviceID)...)

	// Hash (32 bytes)
	payload = append(payload, hash.Bytes()...)

	// Preimage length (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(preimageLength)...)

	// Announced Preimage Forget Reason (1 byte)
	payload = append(payload, forgetReason)

	c.sendEvent(Telemetry_Announced_Preimage_Forgotten, payload)
}

// DecodeAnnouncedPreimageForgotten decodes an Announced Preimage Forgotten event payload (discriminator 192)
func DecodeAnnouncedPreimageForgotten(payload []byte) string {
	serviceID, offset := parseUint32(payload, 0)
	hash, offset := parseHash(payload, offset)
	preimageLength, offset := parseUint32(payload, offset)
	forgetReason, _ := parseUint8(payload, offset)

	forgetReasonStr := "unknown"
	switch forgetReason {
	case 0:
		forgetReasonStr = "provided_on_chain"
	case 1:
		forgetReasonStr = "not_requested_on_chain"
	case 2:
		forgetReasonStr = "failed_to_acquire"
	case 3:
		forgetReasonStr = "too_many_announced"
	case 4:
		forgetReasonStr = "bad_length"
	case 5:
		forgetReasonStr = "other"
	}

	return fmt.Sprintf("service_id:%d|hash:%s|preimage_length:%d|forget_reason:%s", serviceID, hash, preimageLength, forgetReasonStr)
}

/*
### 193: Sending preimage request

Emitted when a validator begins sending a preimage request to a peer (CE 143).

    Peer ID (Recipient)
    Hash
*/
// Implemented in: peerCE142_peerimageannouncement.go
// SendingPreimageRequest creates payload for sending preimage request event (discriminator 193)
func (c *TelemetryClient) SendingPreimageRequest(peerID [32]byte, hash common.Hash) {

	var payload []byte

	// Peer ID (32 bytes - recipient)
	payload = append(payload, peerID[:]...)

	// Hash (32 bytes)
	payload = append(payload, hash.Bytes()...)

	c.sendEvent(Telemetry_Sending_Preimage_Request, payload)
}

// DecodeSendingPreimageRequest decodes a Sending Preimage Request event payload (discriminator 193)
func DecodeSendingPreimageRequest(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	hash, _ := parseHash(payload, offset)

	return fmt.Sprintf("peer_id:%s|hash:%s", peerID, hash)
}

/*
### 194: Receiving preimage request

Emitted by the recipient when a validator begins sending a preimage request (CE 143).

    Peer ID (Sender)
*/
// Implemented in: peerCE142_peerimageannouncement.go
// ReceivingPreimageRequest creates payload for receiving preimage request event (discriminator 194)
func (c *TelemetryClient) ReceivingPreimageRequest(peerID [32]byte) {

	var payload []byte

	// Peer ID (32 bytes - sender)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Receiving_Preimage_Request, payload)
}

// DecodeReceivingPreimageRequest decodes a Receiving Preimage Request event payload (discriminator 194)
func DecodeReceivingPreimageRequest(payload []byte) string {
	peerID, _ := parsePeerID(payload, 0)

	return fmt.Sprintf("peer_id:%s", peerID)
}

/*
### 195: Preimage request failed

Emitted when a preimage request (CE 143) fails.

    Event ID (ID of the corresponding "sending preimage request" or "receiving preimage request" event)
    Reason
*/
// Implemented in: peerCE142_peerimageannouncement.go
// PreimageRequestFailed creates payload for preimage request failed event (discriminator 195)
func (c *TelemetryClient) PreimageRequestFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Preimage_Request_Failed, payload)
}

// DecodePreimageRequestFailed decodes a Preimage Request Failed event payload (discriminator 195)
func DecodePreimageRequestFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 196: Preimage request sent

Emitted once a preimage request has been sent to a peer (CE 143). This should be emitted after the
initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending preimage request" event)
*/
// Implemented in: peerCE142_peerimageannouncement.go
// PreimageRequestSent creates payload for preimage request sent event (discriminator 196)
func (c *TelemetryClient) PreimageRequestSent(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Preimage_Request_Sent, payload)
}

// DecodePreimageRequestSent decodes a Preimage Request Sent event payload (discriminator 196)
func DecodePreimageRequestSent(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 197: Preimage request received

Emitted once a preimage request has been received from a peer (CE 143).

    Event ID (ID of the corresponding "receiving preimage request" event)
    Hash
*/
// Implemented in: peerCE142_peerimageannouncement.go
// PreimageRequestReceived creates payload for preimage request received event (discriminator 197)
func (c *TelemetryClient) PreimageRequestReceived(eventID uint64, hash common.Hash) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Hash (32 bytes)
	payload = append(payload, hash.Bytes()...)

	c.sendEvent(Telemetry_Preimage_Request_Received, payload)
}

// DecodePreimageRequestReceived decodes a Preimage Request Received event payload (discriminator 197)
func DecodePreimageRequestReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	hash, _ := parseHash(payload, offset)

	return fmt.Sprintf("event_id:%d|hash:%s", eventID, hash)
}

/*
### 198: Preimage transferred

Emitted when a preimage has been fully sent to or received from a peer (CE 143).

    Event ID (ID of the corresponding "sending preimage request" or "receiving preimage request" event)
    u32 (Preimage length)
*/
// Implemented in: peerCE142_peerimageannouncement.go
// PreimageTransferred creates payload for preimage transferred event (discriminator 198)
func (c *TelemetryClient) PreimageTransferred(eventID uint64, preimageLength uint32) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Preimage length (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(preimageLength)...)

	c.sendEvent(Telemetry_Preimage_Transferred, payload)
}

// DecodePreimageTransferred decodes a Preimage Transferred event payload (discriminator 198)
func DecodePreimageTransferred(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	preimageLength, _ := parseUint32(payload, offset)

	return fmt.Sprintf("event_id:%d|preimage_length:%d", eventID, preimageLength)
}

/*
### 199: Preimage discarded

Emitted when a preimage is discarded from the local preimage pool.

Note that in the case where the preimage was requested by multiple services, there may not be a
unique discard reason. For example, the preimage may have been provided to one service, while
another service may have stopped requesting it. In this case, either reason may be reported.

	Hash
	u32 (Preimage length)
	Preimage Discard Reason

Preimage Discard Reason =

	0 (Provided on-chain) OR
	1 (Not requested on-chain) OR
	2 (Too many preimages) OR
	3 (Other)
	(Single byte)
*/
const (
	PreimageDiscardReason_ProvidedOnChain     = 0 // Provided on-chain
	PreimageDiscardReason_NotRequestedOnChain = 1 // Not requested on-chain
	PreimageDiscardReason_TooManyPreimages    = 2 // Too many preimages
	PreimageDiscardReason_Other               = 3 // Other
)

// Implemented in: peerCE142_peerimageannouncement.go
// PreimageDiscarded creates payload for preimage discarded event (discriminator 199)
func (c *TelemetryClient) PreimageDiscarded(hash common.Hash, preimageLength uint32, discardReason byte) {

	var payload []byte

	// Hash (32 bytes)
	payload = append(payload, hash.Bytes()...)

	// Preimage length (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(preimageLength)...)

	// Preimage Discard Reason (1 byte)
	payload = append(payload, discardReason)

	c.sendEvent(Telemetry_Preimage_Discarded, payload)
}

// DecodePreimageDiscarded decodes a Preimage Discarded event payload (discriminator 199)
func DecodePreimageDiscarded(payload []byte) string {
	hash, offset := parseHash(payload, 0)
	preimageLength, offset := parseUint32(payload, offset)
	discardReason, _ := parseUint8(payload, offset)

	discardReasonStr := "unknown"
	switch discardReason {
	case 0:
		discardReasonStr = "provided_on_chain"
	case 1:
		discardReasonStr = "not_requested_on_chain"
	case 2:
		discardReasonStr = "too_many_preimages"
	case 3:
		discardReasonStr = "other"
	}

	return fmt.Sprintf("hash:%s|preimage_length:%d|discard_reason:%s", hash, preimageLength, discardReasonStr)
}
