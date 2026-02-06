package telemetry

import (
	"fmt"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/types"
)

// Type aliases to use types from types package
type SegmentShardRequest = types.SegmentShardRequest

/*

## Segment recovery events

These events concern recovery of segments exported by work-packages. Segments are recovered by
primary guarantors, hence these events reference "work-package submission" events.

**Implementation locations for events 160-178:**
- Segment shard requests (162-167): `node/node_guarantor.go` - in CE 139/140 handlers and segment reconstruction
- Segment requests (173-178): `node/node_guarantor.go` - in CE 148 handlers
- Segment verification (171-172): `node/node_guarantor.go` - after segment reconstruction
*/

// SegmentShardRequest represents a segment shard request (Import Segment ID ++ Shard Index)
/*
   Import Segment ID = u16 (Index in overall list of work-package imports, or for a proof page,
       2^15 plus index of a proven page)
*/
// SegmentShardRequest is now aliased from types package

/*
### 160: Work-package hash mapped

Emitted when a work-package hash is mapped to a segments-root for the purpose of segment recovery.

    Event ID (ID of the corresponding "work-package submission" event)
    Work-Package Hash
    Segments-Root
*/
// Implemented in: node/node_da.go:268 in VerifyBundle function
// WorkPackageHashMapped creates payload for work-package hash mapped event (discriminator 160)
func (c *TelemetryClient) WorkPackageHashMapped(eventID uint64, workPackageHash common.Hash, segmentsRoot common.Hash) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Work-Package Hash (32 bytes)
	payload = append(payload, workPackageHash.Bytes()...)

	// Segments-Root (32 bytes)
	payload = append(payload, segmentsRoot.Bytes()...)

	c.sendEvent(Telemetry_Work_Package_Hash_Mapped, payload)
}

// DecodeWorkPackageHashMapped decodes a Work Package Hash Mapped event payload (discriminator 160)
func DecodeWorkPackageHashMapped(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	workPackageHash, offset := parseHash(payload, offset)
	segmentsRoot, _ := parseHash(payload, offset)

	return fmt.Sprintf("event_id:%d|work_package_hash:%s|segments_root:%s", eventID, workPackageHash, segmentsRoot)
}

/*
### 161: Segments-root mapped

Emitted when a segments-root is mapped to an erasure-root for the purpose of segment recovery.

    Event ID (ID of the corresponding "work-package submission" event)
    Segments-Root
    Erasure-Root
*/
// Implemented in: node/node_da.go:277 in VerifyBundle function
// SegmentsRootMapped creates payload for segments-root mapped event (discriminator 161)
func (c *TelemetryClient) SegmentsRootMapped(eventID uint64, segmentsRoot common.Hash, erasureRoot common.Hash) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Segments-Root (32 bytes)
	payload = append(payload, segmentsRoot.Bytes()...)

	// Erasure-Root (32 bytes)
	payload = append(payload, erasureRoot.Bytes()...)

	c.sendEvent(Telemetry_Segments_Root_Mapped, payload)
}

// DecodeSegmentsRootMapped decodes a Segments Root Mapped event payload (discriminator 161)
func DecodeSegmentsRootMapped(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	segmentsRoot, offset := parseHash(payload, offset)
	erasureRoot, _ := parseHash(payload, offset)

	return fmt.Sprintf("event_id:%d|segments_root:%s|erasure_root:%s", eventID, segmentsRoot, erasureRoot)
}

/*
### 162: Sending segment shard request

Emitted when a guarantor begins sending a segment shard request to an assurer (CE 139/140).

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Assurer)
    bool (Using CE 140?)
    len++[Import Segment ID ++ Shard Index] (Segment shards being requested)
*/
// Implemented in: peerCE139140_segmentshardrequest.go
// SendingSegmentShardRequest creates payload for sending segment shard request event (discriminator 162)
func (c *TelemetryClient) SendingSegmentShardRequest(eventID uint64, peerID [32]byte, usingCE140 bool, requests []SegmentShardRequest) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - assurer)
	payload = append(payload, peerID[:]...)

	// Using CE 140? (1 byte: 0 = False, 1 = True)
	if usingCE140 {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}

	// len++[Import Segment ID ++ Shard Index]
	payload = append(payload, types.E(uint64(len(requests)))...)
	for _, req := range requests {
		// Import Segment ID (2 bytes, little-endian)
		payload = append(payload, Uint16ToBytes(req.ImportSegmentID)...)

		// Shard Index (2 bytes, little-endian)
		payload = append(payload, Uint16ToBytes(req.ShardIndex)...)
	}

	c.sendEvent(Telemetry_Sending_Segment_Shard_Request, payload)
}

// DecodeSendingSegmentShardRequest decodes a Sending Segment Shard Request event payload (discriminator 162)
func DecodeSendingSegmentShardRequest(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, offset := parsePeerID(payload, offset)
	usingCE140, offset := parseUint8(payload, offset)

	// Parse request count
	requestCount, _ := parseVariableLength(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s|using_ce140:%t|request_count:%d", eventID, peerID, usingCE140 != 0, requestCount)
}

/*
### 163: Receiving segment shard request

Emitted by the recipient when a node begins sending a segment shard request (CE 139/140).

    Peer ID (Sender)
    bool (Using CE 140?)
*/
// Implemented in: peerCE139140_segmentshardrequest.go
// ReceivingSegmentShardRequest creates payload for receiving segment shard request event (discriminator 163)
func (c *TelemetryClient) ReceivingSegmentShardRequest(peerID [32]byte, usingCE140 bool) {

	var payload []byte

	// Peer ID (32 bytes - sender)
	payload = append(payload, peerID[:]...)

	// Using CE 140? (1 byte: 0 = False, 1 = True)
	if usingCE140 {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}

	c.sendEvent(Telemetry_Receiving_Segment_Shard_Request, payload)
}

// DecodeReceivingSegmentShardRequest decodes a Receiving Segment Shard Request event payload (discriminator 163)
func DecodeReceivingSegmentShardRequest(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	usingCE140, _ := parseUint8(payload, offset)

	return fmt.Sprintf("peer_id:%s|using_ce140:%t", peerID, usingCE140 != 0)
}

/*
### 164: Segment shard request failed

Emitted when a segment shard request fails (CE 139/140). This should be emitted by both sides.

    Event ID (ID of the corresponding "sending segment shard request" or "receiving segment shard request" event)
    Reason
*/
// Implemented in: peerCE139140_segmentshardrequest.go
// SegmentShardRequestFailed creates payload for segment shard request failed event (discriminator 164)
func (c *TelemetryClient) SegmentShardRequestFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Segment_Shard_Request_Failed, payload)
}

// DecodeSegmentShardRequestFailed decodes a Segment Shard Request Failed event payload (discriminator 164)
func DecodeSegmentShardRequestFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 165: Segment shard request sent

Emitted once a segment shard request has been sent to an assurer (CE 139/140). This should be
emitted after the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending segment shard request" event)
*/
// Implemented in: peerCE139140_segmentshardrequest.go
// SegmentShardRequestSent creates payload for segment shard request sent event (discriminator 165)
func (c *TelemetryClient) SegmentShardRequestSent(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Segment_Shard_Request_Sent, payload)
}

// DecodeSegmentShardRequestSent decodes a Segment Shard Request Sent event payload (discriminator 165)
func DecodeSegmentShardRequestSent(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 166: Segment shard request received

Emitted once a segment shard request has been received (CE 139/140).

    Event ID (ID of the corresponding "receiving segment shard request" event)
    u16 (Number of segment shards requested)
*/
// Implemented in: peerCE139140_segmentshardrequest.go
// SegmentShardRequestReceived creates payload for segment shard request received event (discriminator 166)
func (c *TelemetryClient) SegmentShardRequestReceived(eventID uint64, numSegmentShards uint16) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Number of segment shards requested (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(numSegmentShards)...)

	c.sendEvent(Telemetry_Segment_Shard_Request_Received, payload)
}

// DecodeSegmentShardRequestReceived decodes a Segment Shard Request Received event payload (discriminator 166)
func DecodeSegmentShardRequestReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	numSegmentShards, _ := parseUint16(payload, offset)

	return fmt.Sprintf("event_id:%d|num_segment_shards:%d", eventID, numSegmentShards)
}

/*
### 167: Segment shards transferred

Emitted when a segment shard request completes successfully (CE 139/140). This should be emitted by
both sides.

    Event ID (ID of the corresponding "sending segment shard request" or "receiving segment shard request" event)
*/
// Implemented in: peerCE139140_segmentshardrequest.go
// SegmentShardsTransferred creates payload for segment shards transferred event (discriminator 167)
func (c *TelemetryClient) SegmentShardsTransferred(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Segment_Shards_Transferred, payload)
}

// DecodeSegmentShardsTransferred decodes a Segment Shards Transferred event payload (discriminator 167)
func DecodeSegmentShardsTransferred(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 168: Reconstructing segments

Emitted when reconstruction of a set of segments from shards received from assurers begins.

    Event ID (ID of the corresponding "work-package submission" event)
    len++[Import Segment ID] (Segments being reconstructed)
    bool (Is this a trivial reconstruction, using only original-data shards?)
*/
// Implemented in: node/node_guarantee.go:1172 in buildBundle function
// ReconstructingSegments creates payload for reconstructing segments event (discriminator 168)
func (c *TelemetryClient) ReconstructingSegments(eventID uint64, segmentIDs []uint16, isTrivial bool) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// len++[Import Segment ID]
	payload = append(payload, types.E(uint64(len(segmentIDs)))...)
	for _, segID := range segmentIDs {
		payload = append(payload, Uint16ToBytes(segID)...)
	}

	// Is this a trivial reconstruction? (1 byte: 0 = False, 1 = True)
	if isTrivial {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}

	c.sendEvent(Telemetry_Reconstructing_Segments, payload)
}

// DecodeReconstructingSegments decodes a Reconstructing Segments event payload (discriminator 168)
func DecodeReconstructingSegments(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse segment IDs length
	segmentIDsLength, offset := parseVariableLength(payload, offset)
	// Skip the actual segment IDs for simplicity
	offset += int(segmentIDsLength) * 2

	isTrivial, _ := parseUint8(payload, offset)

	return fmt.Sprintf("event_id:%d|segment_ids_count:%d|is_trivial:%t", eventID, segmentIDsLength, isTrivial != 0)
}

/*
### 169: Segment reconstruction failed

Emitted if reconstruction of a set of segments fails.

    Event ID (ID of the corresponding "reconstructing segments" event)
    Reason
*/
// Implemented in: node/node_guarantee.go:1176 in buildBundle function
// SegmentReconstructionFailed creates payload for segment reconstruction failed event (discriminator 169)
func (c *TelemetryClient) SegmentReconstructionFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Segment_Reconstruction_Failed, payload)
}

// DecodeSegmentReconstructionFailed decodes a Segment Reconstruction Failed event payload (discriminator 169)
func DecodeSegmentReconstructionFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 170: Segments reconstructed

Emitted once a set of segments has been successfully reconstructed from shards.

    Event ID (ID of the corresponding "reconstructing segments" event)
*/
// Implemented in: node/node_guarantee.go:1190 in buildBundle function
// SegmentsReconstructed creates payload for segments reconstructed event (discriminator 170)
func (c *TelemetryClient) SegmentsReconstructed(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Segments_Reconstructed, payload)
}

// DecodeSegmentsReconstructed decodes a Segments Reconstructed event payload (discriminator 170)
func DecodeSegmentsReconstructed(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 171: Segment verification failed

Emitted if, following reconstruction of a segment and its proof page, extraction or verification of
the segment proof fails. This should only be possible in two cases:

- CE 139 was used to fetch some of the segment shards. CE 139 responses are not justified;
  requesters cannot verify that returned shards are consistent with their erasure-roots.
- The erasure-root or segments-root is incorrect. This implies an invalid work-report for the
  exporting work-package.

For efficiency, multiple segments may be reported in a single event.

    Event ID (ID of the corresponding "work-package submission" event)
    len++[u16] (Indices of the failed segments in the import list)
    Reason
*/
// Implemented in: node/node.go:2457 in reconstructSegments function
// SegmentVerificationFailed creates payload for segment verification failed event (discriminator 171)
func (c *TelemetryClient) SegmentVerificationFailed(eventID uint64, failedIndices []uint16, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// len++[u16] (Indices of the failed segments in the import list)
	payload = append(payload, types.E(uint64(len(failedIndices)))...)
	for _, idx := range failedIndices {
		payload = append(payload, Uint16ToBytes(idx)...)
	}

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Segment_Verification_Failed, payload)
}

// DecodeSegmentVerificationFailed decodes a Segment Verification Failed event payload (discriminator 171)
func DecodeSegmentVerificationFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse failed indices length
	failedIndicesLength, offset := parseVariableLength(payload, offset)
	// Skip the actual failed indices for simplicity
	offset += int(failedIndicesLength) * 2

	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|failed_indices_count:%d|reason:%s", eventID, failedIndicesLength, reason)
}

/*
### 172: Segments verified

Emitted once a reconstructed segment has been successfully verified against the corresponding
segments-root. For efficiency, multiple segments may be reported in a single event.

    Event ID (ID of the corresponding "work-package submission" event)
    len++[u16] (Indices of the verified segments in the import list)
*/
// Implemented in: node/node.go:2465 in reconstructSegments function
// SegmentsVerified creates payload for segments verified event (discriminator 172)
func (c *TelemetryClient) SegmentsVerified(eventID uint64, verifiedIndices []uint16) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// len++[u16] (Indices of the verified segments in the import list)
	payload = append(payload, types.E(uint64(len(verifiedIndices)))...)
	for _, idx := range verifiedIndices {
		payload = append(payload, Uint16ToBytes(idx)...)
	}

	c.sendEvent(Telemetry_Segments_Verified, payload)
}

// DecodeSegmentsVerified decodes a Segments Verified event payload (discriminator 172)
func DecodeSegmentsVerified(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse verified indices length
	verifiedIndicesLength, _ := parseVariableLength(payload, offset)

	return fmt.Sprintf("event_id:%d|verified_indices_count:%d", eventID, verifiedIndicesLength)
}

/*
### 173: Sending segment request

Emitted when a guarantor begins sending a segment request to a previous guarantor (CE 148). Note
that proof pages need not (and in fact cannot) be requested using this protocol, hence the use of
`u16` rather than `Import Segment ID` to identify each requested segment.

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Previous guarantor)
    len++[u16] (Indices of requested segments in overall list of work-package imports)
*/
// Implemented in: peerCE148_segmentrequest.go
// SendingSegmentRequest creates payload for sending segment request event (discriminator 173)
func (c *TelemetryClient) SendingSegmentRequest(eventID uint64, peerID [32]byte, segmentIndices []uint16) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - previous guarantor)
	payload = append(payload, peerID[:]...)

	// len++[u16] (Indices of requested segments in overall list of work-package imports)
	payload = append(payload, types.E(uint64(len(segmentIndices)))...)
	for _, idx := range segmentIndices {
		payload = append(payload, Uint16ToBytes(idx)...)
	}

	c.sendEvent(Telemetry_Sending_Segment_Request, payload)
}

// DecodeSendingSegmentRequest decodes a Sending Segment Request event payload (discriminator 173)
func DecodeSendingSegmentRequest(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, offset := parsePeerID(payload, offset)

	// Parse segment indices length
	segmentIndicesLength, _ := parseVariableLength(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s|segment_indices_count:%d", eventID, peerID, segmentIndicesLength)
}

/*
### 174: Receiving segment request

Emitted by the recipient when a guarantor begins sending a segment request (CE 148).

    Peer ID (Guarantor)
*/
// Implemented in: peerCE148_segmentrequest.go
// ReceivingSegmentRequest creates payload for receiving segment request event (discriminator 174)
func (c *TelemetryClient) ReceivingSegmentRequest(peerID [32]byte) {

	var payload []byte

	// Peer ID (32 bytes - guarantor)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Receiving_Segment_Request, payload)
}

// DecodeReceivingSegmentRequest decodes a Receiving Segment Request event payload (discriminator 174)
func DecodeReceivingSegmentRequest(payload []byte) string {
	peerID, _ := parsePeerID(payload, 0)

	return fmt.Sprintf("peer_id:%s", peerID)
}

/*
### 175: Segment request failed

Emitted when a segment request fails (CE 148). This should be emitted by both sides.

    Event ID (ID of the corresponding "sending segment request" or "receiving segment request" event)
    Reason
*/
// Implemented in: peerCE148_segmentrequest.go
// SegmentRequestFailed creates payload for segment request failed event (discriminator 175)
func (c *TelemetryClient) SegmentRequestFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Segment_Request_Failed, payload)
}

// DecodeSegmentRequestFailed decodes a Segment Request Failed event payload (discriminator 175)
func DecodeSegmentRequestFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 176: Segment request sent

Emitted once a segment request has been sent to a previous guarantor (CE 148). This should be
emitted after the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending segment request" event)
*/
// Implemented in: peerCE148_segmentrequest.go
// SegmentRequestSent creates payload for segment request sent event (discriminator 176)
func (c *TelemetryClient) SegmentRequestSent(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Segment_Request_Sent, payload)
}

// DecodeSegmentRequestSent decodes a Segment Request Sent event payload (discriminator 176)
func DecodeSegmentRequestSent(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 177: Segment request received

Emitted once a segment request has been received from a guarantor (CE 148).

    Event ID (ID of the corresponding "receiving segment request" event)
    u16 (Number of segments requested)
*/
// Implemented in: peerCE148_segmentrequest.go
// SegmentRequestReceived creates payload for segment request received event (discriminator 177)
func (c *TelemetryClient) SegmentRequestReceived(eventID uint64, numSegments uint16) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Number of segments requested (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(numSegments)...)

	c.sendEvent(Telemetry_Segment_Request_Received, payload)
}

// DecodeSegmentRequestReceived decodes a Segment Request Received event payload (discriminator 177)
func DecodeSegmentRequestReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	numSegments, _ := parseUint16(payload, offset)

	return fmt.Sprintf("event_id:%d|num_segments:%d", eventID, numSegments)
}

/*
### 178: Segments transferred

Emitted when a segment request completes successfully (CE 148). This should be emitted by both
sides.

    Event ID (ID of the corresponding "sending segment request" or "receiving segment request" event)
*/
// Implemented in: peerCE148_segmentrequest.go
// SegmentsTransferred creates payload for segments transferred event (discriminator 178)
func (c *TelemetryClient) SegmentsTransferred(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Segments_Transferred, payload)
}

// DecodeSegmentsTransferred decodes a Segments Transferred event payload (discriminator 178)
func DecodeSegmentsTransferred(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}
