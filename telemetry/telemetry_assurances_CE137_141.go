package telemetry

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

/*

## Availability distribution events

These events concern availability shard and assurance distribution.

**Implementation locations for events 120-131:**
- Shard requests (120-125): `node/network.go` - in CE 137 handlers
- Assurance distribution (126-131): `node/node_assurer.go` - in assurance logic and CE 141 handlers
*/

/*
### 120: Sending shard request

Emitted when an assurer begins sending a shard request to a guarantor (CE 137).

    Peer ID (Guarantor)
    Erasure-Root
    Shard Index
*/
// Implemented in: peerCE137_fullshardrequest.go
// SendingShardRequest creates payload for sending shard request event (discriminator 120)
func (c *TelemetryClient) SendingShardRequest(peerID [32]byte, erasureRoot common.Hash, shardIndex uint16) {

	var payload []byte

	// Peer ID (32 bytes - guarantor)
	payload = append(payload, peerID[:]...)

	// Erasure-Root (32 bytes)
	payload = append(payload, erasureRoot.Bytes()...)

	// Shard Index (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(shardIndex)...)

	c.sendEvent(Telemetry_Sending_Shard_Request, payload)
}

// DecodeSendingShardRequest decodes a Sending Shard Request event payload (discriminator 120)
func DecodeSendingShardRequest(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	erasureRoot, offset := parseHash(payload, offset)
	shardIndex, _ := parseUint16(payload, offset)

	return fmt.Sprintf("peer_id:%s|erasure_root:%s|shard_index:%d", peerID, erasureRoot, shardIndex)
}

/*
### 121: Receiving shard request

Emitted by the recipient when an assurer begins sending a shard request (CE 137).

    Peer ID (Assurer)
*/
// Implemented in: peerCE137_fullshardrequest.go
// ReceivingShardRequest creates payload for receiving shard request event (discriminator 121)
func (c *TelemetryClient) ReceivingShardRequest(peerID [32]byte) {

	var payload []byte

	// Peer ID (32 bytes - assurer)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Receiving_Shard_Request, payload)
}

// DecodeReceivingShardRequest decodes a Receiving Shard Request event payload (discriminator 121)
func DecodeReceivingShardRequest(payload []byte) string {
	peerID, _ := parsePeerID(payload, 0)

	return fmt.Sprintf("peer_id:%s", peerID)
}

/*
### 122: Shard request failed

Emitted when a shard request fails (CE 137). This should be emitted by both sides, ie the assurer
and the guarantor.

    Event ID (ID of the corresponding "sending shard request" or "receiving shard request" event)
    Reason
*/
// Implemented in: peerCE137_fullshardrequest.go
// ShardRequestFailed creates payload for shard request failed event (discriminator 122)
func (c *TelemetryClient) ShardRequestFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Shard_Request_Failed, payload)
}

// DecodeShardRequestFailed decodes a Shard Request Failed event payload (discriminator 122)
func DecodeShardRequestFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 123: Shard request sent

Emitted once a shard request has been sent to a guarantor (CE 137). This should be emitted after
the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending shard request" event)
*/
// Implemented in: peerCE137_fullshardrequest.go
// ShardRequestSent creates payload for shard request sent event (discriminator 123)
func (c *TelemetryClient) ShardRequestSent(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Shard_Request_Sent, payload)
}

// DecodeShardRequestSent decodes a Shard Request Sent event payload (discriminator 123)
func DecodeShardRequestSent(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 124: Shard request received

Emitted once a shard request has been received from an assurer (CE 137).

    Event ID (ID of the corresponding "receiving shard request" event)
    Erasure-Root
    Shard Index
*/
// Implemented in: peerCE137_fullshardrequest.go
// ShardRequestReceived creates payload for shard request received event (discriminator 124)
func (c *TelemetryClient) ShardRequestReceived(eventID uint64, erasureRoot common.Hash, shardIndex uint16) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Erasure-Root (32 bytes)
	payload = append(payload, erasureRoot.Bytes()...)

	// Shard Index (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(shardIndex)...)

	c.sendEvent(Telemetry_Shard_Request_Received, payload)
}

// DecodeShardRequestReceived decodes a Shard Request Received event payload (discriminator 124)
func DecodeShardRequestReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	erasureRoot, offset := parseHash(payload, offset)
	shardIndex, _ := parseUint16(payload, offset)

	return fmt.Sprintf("event_id:%d|erasure_root:%s|shard_index:%d", eventID, erasureRoot, shardIndex)
}

/*
### 125: Shards transferred

Emitted when a shard request completes successfully (CE 137). This should be emitted by both sides,
ie the assurer and the guarantor.

    Event ID (ID of the corresponding "sending shard request" or "receiving shard request" event)
*/
// Implemented in: peerCE137_fullshardrequest.go
// ShardsTransferred creates payload for shards transferred event (discriminator 125)
func (c *TelemetryClient) ShardsTransferred(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Shards_Transferred, payload)
}

// DecodeShardsTransferred decodes a Shards Transferred event payload (discriminator 125)
func DecodeShardsTransferred(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 126: Distributing assurance

Emitted when an assurer begins distributing an assurance to other validators, for potential
inclusion in a block.

    Header Hash (Assurance anchor)
    [u8; ceil(C / 8)] (Availability bitfield; one bit per core, C is the total number of cores)
*/
// Implemented in: node/node.go:1694 in broadcast function
// DistributingAssurance creates payload for distributing assurance event (discriminator 126)
func (c *TelemetryClient) DistributingAssurance(headerHash common.Hash, availabilityBitfield []byte) {

	var payload []byte

	// Header Hash (32 bytes - assurance anchor)
	payload = append(payload, headerHash.Bytes()...)

	// Availability bitfield [u8; ceil(C / 8)] - one bit per core
	payload = append(payload, availabilityBitfield...)

	c.sendEvent(Telemetry_Distributing_Assurance, payload)
}

// DecodeDistributingAssurance decodes a Distributing Assurance event payload (discriminator 126)
func DecodeDistributingAssurance(payload []byte) string {
	headerHash, offset := parseHash(payload, 0)
	availabilityBitfield := formatBytes(payload[offset:])

	return fmt.Sprintf("header_hash:%s|availability_bitfield:%s", headerHash, availabilityBitfield)
}

/*
### 127: Assurance send failed

Emitted when an assurer fails to send an assurance to another validator (CE 141).

    Event ID (ID of the corresponding "distributing assurance" event)
    Peer ID (Recipient)
    Reason
*/
// Implemented in: peerCE141_assurance.go
// AssuranceSendFailed creates payload for assurance send failed event (discriminator 127)
func (c *TelemetryClient) AssuranceSendFailed(eventID uint64, peerID [32]byte, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - recipient)
	payload = append(payload, peerID[:]...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Assurance_Send_Failed, payload)
}

// DecodeAssuranceSendFailed decodes an Assurance Send Failed event payload (discriminator 127)
func DecodeAssuranceSendFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, offset := parsePeerID(payload, offset)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s|reason:%s", eventID, peerID, reason)
}

/*
### 128: Assurance sent

Emitted by assurers after sending an assurance to another validator (CE 141).

    Event ID (ID of the corresponding "distributing assurance" event)
    Peer ID (Recipient)
*/
// Implemented in: peerCE141_assurance.go
// AssuranceSent creates payload for assurance sent event (discriminator 128)
func (c *TelemetryClient) AssuranceSent(eventID uint64, peerID [32]byte) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - recipient)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Assurance_Sent, payload)
}

// DecodeAssuranceSent decodes an Assurance Sent event payload (discriminator 128)
func DecodeAssuranceSent(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, _ := parsePeerID(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s", eventID, peerID)
}

/*
### 129: Assurance distributed

Emitted once an assurer has finished distributing an assurance.

This event should be emitted even if the assurer was not successful in sending the assurance to any
other validator. The success of the distribution should be determined by the "assurance send
failed" and "assurance sent" events emitted by the assurer, as well as the "assurance receive
failed" and "assurance received" events emitted by the recipient validators.

    Event ID (ID of the corresponding "distributing assurance" event)
*/
// Implemented in: node/node.go:1706 in broadcast function (defer statement)
// AssuranceDistributed creates payload for assurance distributed event (discriminator 129)
func (c *TelemetryClient) AssuranceDistributed(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Assurance_Distributed, payload)
}

// DecodeAssuranceDistributed decodes an Assurance Distributed event payload (discriminator 129)
func DecodeAssuranceDistributed(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 130: Assurance receive failed

Emitted when a validator fails to receive an assurance from a peer (CE 141).

    Peer ID (Sender)
    Reason
*/
// Implemented in: peerCE141_assurance.go
// AssuranceReceiveFailed creates payload for assurance receive failed event (discriminator 130)
func (c *TelemetryClient) AssuranceReceiveFailed(peerID [32]byte, reason string) {

	var payload []byte

	// Peer ID (32 bytes - sender)
	payload = append(payload, peerID[:]...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Assurance_Receive_Failed, payload)
}

// DecodeAssuranceReceiveFailed decodes an Assurance Receive Failed event payload (discriminator 130)
func DecodeAssuranceReceiveFailed(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("peer_id:%s|reason:%s", peerID, reason)
}

/*
### 131: Assurance received

Emitted when an assurance is received from a peer (CE 141). This should be emitted as soon as the
assurance is received, before checking if it is valid. If the assurance is found to be invalid, a
"peer misbehaved" event should be emitted.

    Peer ID (Sender)
    Header Hash (Assurance anchor)
*/
// Implemented in: peerCE141_assurance.go
// AssuranceReceived creates payload for assurance received event (discriminator 131)
func (c *TelemetryClient) AssuranceReceived(peerID [32]byte, headerHash common.Hash) {

	var payload []byte

	// Peer ID (32 bytes - sender)
	payload = append(payload, peerID[:]...)

	// Header Hash (32 bytes - assurance anchor)
	payload = append(payload, headerHash.Bytes()...)

	c.sendEvent(Telemetry_Assurance_Received, payload)
}

// DecodeAssuranceReceived decodes an Assurance Received event payload (discriminator 131)
func DecodeAssuranceReceived(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	headerHash, _ := parseHash(payload, offset)

	return fmt.Sprintf("peer_id:%s|header_hash:%s", peerID, headerHash)
}

/*
### 132: Context available

Emitted when the context required to provide an assurance becomes available locally. This includes
having stored the work-report and its availability data, making it possible to construct an
assurance for the corresponding work-package.

    Work-Report Hash
    Core Index
    Slot
    Availability Specifier (Work-Package Hash ++ Bundle Length ++ Erasure-Root ++ Exported-Segment-Root ++ Exported-Segment-Length)
*/
// Implemented in: node/node_assurance.go:262 in assureData function
// ContextAvailable creates payload for context available event (discriminator 132)
func (c *TelemetryClient) ContextAvailable(workReportHash common.Hash, coreIndex uint16, slot uint32, spec types.AvailabilitySpecifier) {

	var payload []byte

	// Work-Report Hash (32 bytes)
	payload = append(payload, workReportHash.Bytes()...)

	// Core Index (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(coreIndex)...)

	// Slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(slot)...)

	// Availability Specifier fields

	// Work-Package Hash (32 bytes)
	payload = append(payload, spec.WorkPackageHash.Bytes()...)

	// Bundle Length (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(spec.BundleLength)...)

	// Erasure-Root (32 bytes)
	payload = append(payload, spec.ErasureRoot.Bytes()...)

	// Exported-Segment-Root (32 bytes)
	payload = append(payload, spec.ExportedSegmentRoot.Bytes()...)

	// Exported-Segment-Length (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(spec.ExportedSegmentLength)...)

	c.sendEvent(Telemetry_Context_Available, payload)
}

// DecodeContextAvailable decodes a Context Available event payload (discriminator 132)
func DecodeContextAvailable(payload []byte) string {
	workReportHash, offset := parseHash(payload, 0)
	coreIndex, offset := parseUint16(payload, offset)
	slot, offset := parseUint32(payload, offset)

	// Parse Availability Specifier
	workPackageHash, offset := parseHash(payload, offset)
	bundleLength, offset := parseUint32(payload, offset)
	erasureRoot, offset := parseHash(payload, offset)
	exportedSegmentRoot, offset := parseHash(payload, offset)
	exportedSegmentLength, _ := parseUint16(payload, offset)

	return fmt.Sprintf("work_report_hash:%s|core_index:%d|slot:%d|work_package_hash:%s|bundle_length:%d|erasure_root:%s|exported_segment_root:%s|exported_segment_length:%d",
		workReportHash, coreIndex, slot, workPackageHash, bundleLength, erasureRoot, exportedSegmentRoot, exportedSegmentLength)
}

/*
### 133: Assurance provided

Emitted when an assurance is produced locally and is ready for distribution. The event contains the
fields of the assurance itself so the telemetry backend can correlate it with distribution events
and extrinsics.

    Header Hash (Assurance anchor)
    [u8; ceil(C / 8)] (Availability bitfield)
    Validator Index
    Ed25519 Signature
*/
// Implemented in: node/node_assurance.go:46 in generateAssurance function
// AssuranceProvided creates payload for assurance provided event (discriminator 133)
func (c *TelemetryClient) AssuranceProvided(assurance types.Assurance) {

	var payload []byte

	// Header Hash (32 bytes - assurance anchor)
	payload = append(payload, assurance.Anchor.Bytes()...)

	// Availability bitfield [u8; ceil(C / 8)]
	payload = append(payload, assurance.Bitfield[:]...)

	// Validator Index (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(assurance.ValidatorIndex)...)

	// Ed25519 Signature (64 bytes)
	payload = append(payload, assurance.Signature[:]...)

	c.sendEvent(Telemetry_Assurance_Provided, payload)
}

// DecodeAssuranceProvided decodes an Assurance Provided event payload (discriminator 133)
func DecodeAssuranceProvided(payload []byte) string {
	headerHash, offset := parseHash(payload, 0)

	// Parse availability bitfield (variable length, but we'll assume it's reasonable size)
	bitfieldEnd := offset + 64           // Assume up to 64 bytes for bitfield
	if bitfieldEnd > len(payload)-2-64 { // Leave room for validator index and signature
		bitfieldEnd = len(payload) - 2 - 64
	}
	availabilityBitfield := formatBytes(payload[offset:bitfieldEnd])
	offset = bitfieldEnd

	validatorIndex, offset := parseUint16(payload, offset)
	signature := formatBytes(payload[offset : offset+64])

	return fmt.Sprintf("header_hash:%s|availability_bitfield:%s|validator_index:%d|signature:%s", headerHash, availabilityBitfield, validatorIndex, signature)
}
