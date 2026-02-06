package telemetry

import (
	"fmt"

	"github.com/jam-duna/jamduna/common"
)

/*

## Bundle recovery events

These events concern recovery of work-package bundles for auditing.

**Implementation locations for events 140-153:**
- Bundle shard requests (140-145): `node/node_auditor.go` - in CE 138 handlers and bundle reconstruction
- Bundle requests (148-153): `node/node_auditor.go` - in CE 147 handlers
*/

/*
### 140: Sending bundle shard request

Emitted when an auditor begins sending a bundle shard request to an assurer (CE 138).

    Event ID (TODO, should reference auditing event)
    Peer ID (Assurer)
    Shard Index
*/
// Implemented in: peerCE138_bundleshardrequest.go
// SendingBundleShardRequest creates payload for sending bundle shard request event (discriminator 140)
func (c *TelemetryClient) SendingBundleShardRequest(eventID uint64, peerID [32]byte, shardIndex uint16) {

	var payload []byte

	// Event ID (8 bytes, little-endian) - TODO: should reference auditing event
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - assurer)
	payload = append(payload, peerID[:]...)

	// Shard Index (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(shardIndex)...)

	c.sendEvent(Telemetry_Sending_Bundle_Shard_Request, payload)
}

// DecodeSendingBundleShardRequest decodes a Sending Bundle Shard Request event payload (discriminator 140)
func DecodeSendingBundleShardRequest(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, offset := parsePeerID(payload, offset)
	shardIndex, _ := parseUint16(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s|shard_index:%d", eventID, peerID, shardIndex)
}

/*
### 141: Receiving bundle shard request

Emitted by the recipient when an auditor begins sending a bundle shard request (CE 138).

    Peer ID (Auditor)
*/
// Implemented in: peerCE138_bundleshardrequest.go
// ReceivingBundleShardRequest creates payload for receiving bundle shard request event (discriminator 141)
func (c *TelemetryClient) ReceivingBundleShardRequest(peerID [32]byte) {

	var payload []byte

	// Peer ID (32 bytes - auditor)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Receiving_Bundle_Shard_Request, payload)
}

// DecodeReceivingBundleShardRequest decodes a Receiving Bundle Shard Request event payload (discriminator 141)
func DecodeReceivingBundleShardRequest(payload []byte) string {
	peerID, _ := parsePeerID(payload, 0)

	return fmt.Sprintf("peer_id:%s", peerID)
}

/*
### 142: Bundle shard request failed

Emitted when a bundle shard request fails (CE 138). This should be emitted by both sides, ie the
auditor and the assurer.

    Event ID (ID of the corresponding "sending bundle shard request" or "receiving bundle shard request" event)
    Reason
*/
// Implemented in: peerCE138_bundleshardrequest.go
// BundleShardRequestFailed creates payload for bundle shard request failed event (discriminator 142)
func (c *TelemetryClient) BundleShardRequestFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Bundle_Shard_Request_Failed, payload)
}

// DecodeBundleShardRequestFailed decodes a Bundle Shard Request Failed event payload (discriminator 142)
func DecodeBundleShardRequestFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 143: Bundle shard request sent

Emitted once a bundle shard request has been sent to an assurer (CE 138). This should be emitted
after the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending bundle shard request" event)
*/
// Implemented in: peerCE138_bundleshardrequest.go
// BundleShardRequestSent creates payload for bundle shard request sent event (discriminator 143)
func (c *TelemetryClient) BundleShardRequestSent(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Bundle_Shard_Request_Sent, payload)
}

// DecodeBundleShardRequestSent decodes a Bundle Shard Request Sent event payload (discriminator 143)
func DecodeBundleShardRequestSent(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 144: Bundle shard request received

Emitted once a bundle shard request has been received from an auditor (CE 138).

    Event ID (ID of the corresponding "receiving bundle shard request" event)
    Erasure-Root
    Shard Index
*/
// Implemented in: peerCE138_bundleshardrequest.go
// BundleShardRequestReceived creates payload for bundle shard request received event (discriminator 144)
func (c *TelemetryClient) BundleShardRequestReceived(eventID uint64, erasureRoot common.Hash, shardIndex uint16) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Erasure-Root (32 bytes)
	payload = append(payload, erasureRoot.Bytes()...)

	// Shard Index (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(shardIndex)...)

	c.sendEvent(Telemetry_Bundle_Shard_Request_Received, payload)
}

// DecodeBundleShardRequestReceived decodes a Bundle Shard Request Received event payload (discriminator 144)
func DecodeBundleShardRequestReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	erasureRoot, offset := parseHash(payload, offset)
	shardIndex, _ := parseUint16(payload, offset)

	return fmt.Sprintf("event_id:%d|erasure_root:%s|shard_index:%d", eventID, erasureRoot, shardIndex)
}

/*
### 145: Bundle shard transferred

Emitted when a bundle shard request completes successfully (CE 138). This should be emitted by both
sides, ie the auditor and the assurer.

    Event ID (ID of the corresponding "sending bundle shard request" or "receiving bundle shard request" event)
*/
// Implemented in: peerCE138_bundleshardrequest.go
// BundleShardTransferred creates payload for bundle shard transferred event (discriminator 145)
func (c *TelemetryClient) BundleShardTransferred(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Bundle_Shard_Transferred, payload)
}

// DecodeBundleShardTransferred decodes a Bundle Shard Transferred event payload (discriminator 145)
func DecodeBundleShardTransferred(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 146: Reconstructing bundle

Emitted when reconstruction of a bundle from shards received from assurers begins.

    Event ID (TODO, should reference auditing event)
    bool (Is this a trivial reconstruction, using only original-data shards?)
*/
// Implemented in: node/node.go:2451 in reconstructPackageBundleSegments function
// ReconstructingBundle creates payload for reconstructing bundle event (discriminator 146)
func (c *TelemetryClient) ReconstructingBundle(eventID uint64, isTrivial bool) {

	var payload []byte

	// Event ID (8 bytes, little-endian) - TODO: should reference auditing event
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Is this a trivial reconstruction? (1 byte: 0 = False, 1 = True)
	if isTrivial {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}

	c.sendEvent(Telemetry_Reconstructing_Bundle, payload)
}

// DecodeReconstructingBundle decodes a Reconstructing Bundle event payload (discriminator 146)
func DecodeReconstructingBundle(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	isTrivial, _ := parseUint8(payload, offset)

	return fmt.Sprintf("event_id:%d|is_trivial:%t", eventID, isTrivial != 0)
}

/*
### 147: Bundle reconstructed

Emitted once a bundle has been successfully reconstructed from shards.

    Event ID (TODO, should reference auditing event)
*/
// Implemented in: node/node.go:2563 in reconstructPackageBundleSegments function
// BundleReconstructed creates payload for bundle reconstructed event (discriminator 147)
func (c *TelemetryClient) BundleReconstructed(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian) - TODO: should reference auditing event
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Bundle_Reconstructed, payload)
}

// DecodeBundleReconstructed decodes a Bundle Reconstructed event payload (discriminator 147)
func DecodeBundleReconstructed(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 148: Sending bundle request

Emitted when an auditor begins sending a bundle request to a guarantor (CE 147).

    Event ID (TODO, should reference auditing event)
    Peer ID (Guarantor)
*/
// Implemented in: peerCE147_bundlerequest.go
// SendingBundleRequest creates payload for sending bundle request event (discriminator 148)
func (c *TelemetryClient) SendingBundleRequest(eventID uint64, peerID [32]byte) {

	var payload []byte

	// Event ID (8 bytes, little-endian) - TODO: should reference auditing event
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - guarantor)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Sending_Bundle_Request, payload)
}

// DecodeSendingBundleRequest decodes a Sending Bundle Request event payload (discriminator 148)
func DecodeSendingBundleRequest(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, _ := parsePeerID(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s", eventID, peerID)
}

/*
### 149: Receiving bundle request

Emitted by the recipient when an auditor begins sending a bundle request (CE 147).

    Peer ID (Auditor)
*/
// Implemented in: peerCE147_bundlerequest.go
// ReceivingBundleRequest creates payload for receiving bundle request event (discriminator 149)
func (c *TelemetryClient) ReceivingBundleRequest(peerID [32]byte) {

	var payload []byte

	// Peer ID (32 bytes - auditor)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Receiving_Bundle_Request, payload)
}

// DecodeReceivingBundleRequest decodes a Receiving Bundle Request event payload (discriminator 149)
func DecodeReceivingBundleRequest(payload []byte) string {
	peerID, _ := parsePeerID(payload, 0)

	return fmt.Sprintf("peer_id:%s", peerID)
}

/*
### 150: Bundle request failed

Emitted when a bundle request fails (CE 147). This should be emitted by both sides, ie the auditor
and the guarantor.

    Event ID (ID of the corresponding "sending bundle request" or "receiving bundle request" event)
    Reason
*/
// Implemented in: peerCE147_bundlerequest.go
// BundleRequestFailed creates payload for bundle request failed event (discriminator 150)
func (c *TelemetryClient) BundleRequestFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Bundle_Request_Failed, payload)
}

// DecodeBundleRequestFailed decodes a Bundle Request Failed event payload (discriminator 150)
func DecodeBundleRequestFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 151: Bundle request sent

Emitted once a bundle request has been sent to a guarantor (CE 147). This should be emitted after
the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending bundle request" event)
*/
// Implemented in: peerCE147_bundlerequest.go
// BundleRequestSent creates payload for bundle request sent event (discriminator 151)
func (c *TelemetryClient) BundleRequestSent(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Bundle_Request_Sent, payload)
}

// DecodeBundleRequestSent decodes a Bundle Request Sent event payload (discriminator 151)
func DecodeBundleRequestSent(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 152: Bundle request received

Emitted once a bundle request has been received from an auditor (CE 147).

    Event ID (ID of the corresponding "receiving bundle request" event)
    Erasure-Root
*/
// Implemented in: peerCE147_bundlerequest.go
// BundleRequestReceived creates payload for bundle request received event (discriminator 152)
func (c *TelemetryClient) BundleRequestReceived(eventID uint64, erasureRoot common.Hash) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Erasure-Root (32 bytes)
	payload = append(payload, erasureRoot.Bytes()...)

	c.sendEvent(Telemetry_Bundle_Request_Received, payload)
}

// DecodeBundleRequestReceived decodes a Bundle Request Received event payload (discriminator 152)
func DecodeBundleRequestReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	erasureRoot, _ := parseHash(payload, offset)

	return fmt.Sprintf("event_id:%d|erasure_root:%s", eventID, erasureRoot)
}

/*
### 153: Bundle transferred

Emitted when a bundle request completes successfully (CE 147). This should be emitted by both
sides, ie the auditor and the guarantor.

    Event ID (ID of the corresponding "sending bundle request" or "receiving bundle request" event)
*/
// Implemented in: peerCE147_bundlerequest.go
// BundleTransferred creates payload for bundle transferred event (discriminator 153)
func (c *TelemetryClient) BundleTransferred(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Bundle_Transferred, payload)
}

// DecodeBundleTransferred decodes a Bundle Transferred event payload (discriminator 153)
func DecodeBundleTransferred(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}
