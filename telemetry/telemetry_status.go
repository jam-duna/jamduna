package telemetry

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

/*
## Status events

These events concern the node's overall status and sync state.

**Implementation locations for events 10-13:**
- Status (10): `node/node.go` - periodic status ticker (every 2s)
- Best block changed (11): `statedb/statedb.go` - in `UpdateBestBlock()` or when new blocks are accepted
- Finalized block changed (12): `statedb/statedb.go` - in finalization logic
- Sync status changed (13): `node/sync.go` - when sync status transitions
*/

/*
### 0: Dropped events

Emitted when events are dropped due to limited buffer space.

    u64 (Timestamp of last dropped event)
    u64 (Number of events dropped)
*/
// TODO: IMPLEMENT THIS in telemetry.go
// DroppedEvents sends a "dropped events" meta event (discriminator 0) to the telemetry server
func (c *TelemetryClient) DroppedEvents(lastDroppedTimestamp uint64, numDropped uint64) {

	var payload []byte

	// Last dropped timestamp (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(lastDroppedTimestamp)...)

	// Number of dropped events (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(numDropped)...)

	c.sendEvent(Telemetry_Dropped, payload)
}

// DecodeDropped decodes a Dropped event payload (discriminator 0)
func DecodeDropped(payload []byte) string {
	lastDroppedTimestamp, offset := parseUint64(payload, 0)
	numDropped, _ := parseUint64(payload, offset)

	return fmt.Sprintf("last_dropped_timestamp:%d|num_dropped:%d",
		lastDroppedTimestamp, numDropped)
}

/*
### 10: Status

Emitted periodically (approximately every 2 seconds), to provide a summary of the node's current
state. Note that most of this information can be derived from other events.

    u32 (Total number of peers)
    u32 (Number of validator peers)
    u32 (Number of peers with a block announcement stream open)
    [u8; C] (Number of guarantees in pool, by core; C is the total number of cores)
    u32 (Number of shards in availability store)
    u64 (Total size of shards in availability store, in bytes)
    u32 (Number of preimages in pool, ready to be included in a block)
    u32 (Total size of preimages in pool, in bytes)
*/
// Implemented in: node/node.go:976 in runStatusTelemetry goroutine (started from newNode)
// Status sends a periodic status event (discriminator 10) to the telemetry server
func (c *TelemetryClient) Status(totalPeers, validatorPeers, blockAnnouncementStreamPeers uint32, guaranteesPerCore []byte, shardsInAvailabilityStore uint32, shardsSize uint64, preimagesInPool, preimagesSize uint32) {

	var payload []byte

	// Total number of peers (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(totalPeers)...)

	// Number of validator peers (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(validatorPeers)...)

	// Number of peers with block announcement stream open (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(blockAnnouncementStreamPeers)...)

	// Number of guarantees in pool, by core [u8; C]
	payload = append(payload, guaranteesPerCore...)

	// Number of shards in availability store (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(shardsInAvailabilityStore)...)

	// Total size of shards in availability store (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(shardsSize)...)

	// Number of preimages in pool (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(preimagesInPool)...)

	// Total size of preimages in pool (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(preimagesSize)...)

	c.sendEvent(Telemetry_Status, payload)
}

// DecodeStatus decodes a Status event payload (discriminator 10)
func DecodeStatus(payload []byte) string {
	totalPeers, offset := parseUint32(payload, 0)
	validatorPeers, offset := parseUint32(payload, offset)
	blockAnnouncementPeers, offset := parseUint32(payload, offset)

	// guaranteesPerCore is the remaining bytes after the fixed fields
	guaranteesPerCoreEnd := offset
	if len(payload) > offset+8+4+4 { // remaining: shardsInAvailabilityStore + shardsSize + preimagesInPool + preimagesSize
		guaranteesPerCoreEnd = len(payload) - 8 - 4 - 4 // subtract the last 3 fields
	}
	guaranteesPerCore := formatBytes(payload[offset:guaranteesPerCoreEnd])
	offset = guaranteesPerCoreEnd

	shardsInAvailabilityStore, offset := parseUint32(payload, offset)
	shardsSize, offset := parseUint64(payload, offset)
	preimagesInPool, offset := parseUint32(payload, offset)
	preimagesSize, _ := parseUint32(payload, offset)

	return fmt.Sprintf("total_peers:%d|validator_peers:%d|block_announcement_peers:%d|guarantees_per_core:%s|shards_in_availability_store:%d|shards_size:%d|preimages_in_pool:%d|preimages_size:%d",
		totalPeers, validatorPeers, blockAnnouncementPeers, guaranteesPerCore, shardsInAvailabilityStore, shardsSize, preimagesInPool, preimagesSize)
}

/*
### 11: Best block changed

Emitted when the node's best block changes.

    Slot (New best slot)
    Header Hash (New best header hash)
*/
// TODO: IMPLEMENT THIS in statedb/statedb.go
// BestBlockChanged sends a best block changed event (discriminator 11) to the telemetry server
func (c *TelemetryClient) BestBlockChanged(slot uint32, headerHash common.Hash) {

	var payload []byte

	// Slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(slot)...)

	// Header Hash (32 bytes)
	payload = append(payload, headerHash.Bytes()...)

	c.sendEvent(Telemetry_Best_Block_Changed, payload)
}

// DecodeBestBlockChanged decodes a Best Block Changed event payload (discriminator 11)
func DecodeBestBlockChanged(payload []byte) string {
	slot, offset := parseUint32(payload, 0)
	headerHash, _ := parseHash(payload, offset)

	return fmt.Sprintf("slot:%d|header_hash:%s", slot, headerHash)
}

/*
### 12: Finalized block changed

Emitted when the latest finalized block (from the node's perspective) changes.

    Slot (New finalized slot)
    Header Hash (New finalized header hash)
*/
// TODO: IMPLEMENT THIS in statedb/statedb.go
// FinalizedBlockChanged sends a finalized block changed event (discriminator 12) to the telemetry server
func (c *TelemetryClient) FinalizedBlockChanged(slot uint32, headerHash common.Hash) {

	var payload []byte

	// Slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(slot)...)

	// Header Hash (32 bytes)
	payload = append(payload, headerHash.Bytes()...)

	c.sendEvent(Telemetry_Finalized_Block_Changed, payload)
}

// DecodeFinalizedBlockChanged decodes a Finalized Block Changed event payload (discriminator 12)
func DecodeFinalizedBlockChanged(payload []byte) string {
	slot, offset := parseUint32(payload, 0)
	headerHash, _ := parseHash(payload, offset)

	return fmt.Sprintf("slot:%d|header_hash:%s", slot, headerHash)
}

/*
### 13: Sync status changed

Emitted when the node's sync status changes. This status is subjective, indicating whether or not
the node believes it is sufficiently synced with the network to be able to perform all of the
duties of a validator node (authoring, guaranteeing, assuring, auditing, and so on).

    bool (Does the node believe it is sufficiently in sync with the network?)
*/
// Implemented in: node/node.go:338 in SetIsSync function
// SyncStatusChanged sends a sync status changed event (discriminator 13) to the telemetry server
func (c *TelemetryClient) SyncStatusChanged(isSynced bool) {

	var payload []byte

	// Is synced? (1 byte: 0 = False, 1 = True)
	if isSynced {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}

	c.sendEvent(Telemetry_Sync_Status_Changed, payload)
}

// DecodeSyncStatusChanged decodes a Sync Status Changed event payload (discriminator 13)
func DecodeSyncStatusChanged(payload []byte) string {
	isSynced, _ := parseUint8(payload, 0)

	return fmt.Sprintf("is_synced:%t", isSynced != 0)
}

/*
## Networking events

These events concern the peer-to-peer networking layer.
*/

/*
### 20: Connection refused

Emitted when a connection attempt from a peer is refused.

	Peer Address
*/
// Implemented in: node/node.go:1567 in handleConnection function
// ConnectionRefused creates payload for connection refused event (discriminator 20)
func (c *TelemetryClient) ConnectionRefused(peerAddress [16]byte, peerPort uint16) {

	var payload []byte

	// Peer Address (16 bytes IPv6)
	payload = append(payload, peerAddress[:]...)

	// Port (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(peerPort)...)

	c.sendEvent(Telemetry_Connection_Refused, payload)
}

// DecodeConnectionRefused decodes a Connection Refused event payload (discriminator 20)
func DecodeConnectionRefused(payload []byte) string {
	address, _ := parseAddress(payload, 0)

	return fmt.Sprintf("address:%s", address)
}

/*
### 21: Connecting in

Emitted when a connection attempt from a peer is accepted. This event should be emitted as soon as
possible. In particular it should be emitted _before_ the connection handshake completes.

	Peer Address
*/
// Implemented in: node/node.go:1611 in handleConnection function
// ConnectingIn creates payload for connecting in event (discriminator 21)
func (c *TelemetryClient) ConnectingIn(peerAddress [16]byte, peerPort uint16) {

	var payload []byte

	// Peer Address (16 bytes IPv6)
	payload = append(payload, peerAddress[:]...)

	// Port (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(peerPort)...)

	c.sendEvent(Telemetry_Connecting_In, payload)
}

// DecodeConnectingIn decodes a Connecting In event payload (discriminator 21)
func DecodeConnectingIn(payload []byte) string {
	address, _ := parseAddress(payload, 0)

	return fmt.Sprintf("address:%s", address)
}

/*
### 22: Connect in failed

Emitted when an incoming connection attempt fails.

	Event ID (ID of the corresponding "connecting in" event)
	Reason
*/
// Implemented in: node/node.go:1656 in handleConnection function (incoming connection failures)
// ConnectInFailed creates payload for connect in failed event (discriminator 22)
func (c *TelemetryClient) ConnectInFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Connect_In_Failed, payload)
}

// DecodeConnectInFailed decodes a Connect In Failed event payload (discriminator 22)
func DecodeConnectInFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 23: Connected in

Emitted when an incoming connection attempt succeeds.

	Event ID (ID of the corresponding "connecting in" event)
	Peer ID
*/
// Implemented in: node/node.go:1615 in handleConnection function
// ConnectedIn creates payload for connected in event (discriminator 23)
func (c *TelemetryClient) ConnectedIn(eventID uint64, peerID [32]byte) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Connected_In, payload)
}

// DecodeConnectedIn decodes a Connected In event payload (discriminator 23)
func DecodeConnectedIn(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, _ := parsePeerID(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s", eventID, peerID)
}

/*
### 24: Connecting out

Emitted when an outgoing connection attempt is initiated.

	Peer ID
	Peer Address
*/
// Implemented in: node/peer.go:135 in openStream function (outgoing connection attempts)
// ConnectingOut creates payload for connecting out event (discriminator 24)
func (c *TelemetryClient) ConnectingOut(peerID [32]byte, peerAddress [16]byte, peerPort uint16) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Peer Address (16 bytes IPv6)
	payload = append(payload, peerAddress[:]...)

	// Port (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(peerPort)...)

	c.sendEvent(Telemetry_Connecting_Out, payload)
}

// DecodeConnectingOut decodes a Connecting Out event payload (discriminator 24)
func DecodeConnectingOut(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	address, _ := parseAddress(payload, offset)

	return fmt.Sprintf("peer_id:%s|address:%s", peerID, address)
}

/*
### 25: Connect out failed

Emitted when an outgoing connection attempt fails.

	Event ID (ID of the corresponding "connecting out" event)
	Reason
*/
// Implemented in: node/peer.go:156 in openStream function (outgoing connection failures)
// ConnectOutFailed creates payload for connect out failed event (discriminator 25)
func (c *TelemetryClient) ConnectOutFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Connect_Out_Failed, payload)
}

// DecodeConnectOutFailed decodes a Connect Out Failed event payload (discriminator 25)
func DecodeConnectOutFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 26: Connected out

Emitted when an outgoing connection attempt succeeds.

	Event ID (ID of the corresponding "connecting out" event)
*/
// Implemented in: node/peer.go:165 in openStream function (outgoing connection success)
// ConnectedOut creates payload for connected out event (discriminator 26)
func (c *TelemetryClient) ConnectedOut(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Connected_Out, payload)
}

// DecodeConnectedOut decodes a Connected Out event payload (discriminator 26)
func DecodeConnectedOut(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 27: Disconnected

Emitted when a connection to a peer is broken.

	Peer ID
	Option<Connection Side> (Terminator of the connection, may be omitted in case of eg a timeout)
	Reason
*/
// Implemented in: node/node.go:1620 in handleConnection defer function (connection termination)
// Disconnected creates payload for disconnected event (discriminator 27)
func (c *TelemetryClient) Disconnected(peerID [32]byte, connectionSide *byte, reason string) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Option<Connection Side>
	if connectionSide == nil {
		payload = append(payload, 0) // None
	} else {
		payload = append(payload, 1)               // Some
		payload = append(payload, *connectionSide) // 0 = Local, 1 = Remote
	}

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Disconnected, payload)
}

// DecodeDisconnected decodes a Disconnected event payload (discriminator 27)
func DecodeDisconnected(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)

	connectionSide := "none"
	if offset < len(payload) {
		hasConnectionSide, newOffset := parseUint8(payload, offset)
		if hasConnectionSide != 0 && newOffset < len(payload) {
			side, newOffset := parseUint8(payload, newOffset)
			if side == 0 {
				connectionSide = "local"
			} else {
				connectionSide = "remote"
			}
			offset = newOffset
		}
	}

	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("peer_id:%s|connection_side:%s|reason:%s", peerID, connectionSide, reason)
}

/*
### 28: Peer misbehaved

Emitted when a peer misbehaves. Misbehaviour is any behaviour which is objectively not compliant
with the network protocol or the GP. This includes for example sending a malformed message or an
invalid signature. This does _not_ include, for example, timing out (timeouts are subjective) or
prematurely closing a stream (this is permitted by the network protocol).

	Peer ID
	Reason
*/
// Implemented in: node/peerUP0_block.go:397 in onBlockAnnouncement function (protocol violations)
// PeerMisbehaved creates payload for peer misbehaved event (discriminator 28)
func (c *TelemetryClient) PeerMisbehaved(peerID [32]byte, reason string) {

	var payload []byte

	// Peer ID (32 bytes)
	payload = append(payload, peerID[:]...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Peer_Misbehaved, payload)
}

// DecodePeerMisbehaved decodes a Peer Misbehaved event payload (discriminator 28)
func DecodePeerMisbehaved(payload []byte) string {
	peerID, offset := parsePeerID(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("peer_id:%s|reason:%s", peerID, reason)
}
