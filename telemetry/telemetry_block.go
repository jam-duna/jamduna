package telemetry

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// Type aliases to use types from types package
type BlockOutline = types.BlockOutline
type AccumulateCost = types.AccumulateCost
type ServiceAccumulateCost = types.ServiceAccumulateCost

/*
## Block authoring/importing events

These events concern the block authoring and importing pipelines. Note that some events are common
to both authoring and importing, eg "block executed".

**Implementation locations for events 40-47:**
- Authoring events (40-42, 47): `node/node.go` - in `AuthorBlock()` and block building logic
- Importing events (43-47): `node/node.go` - in `ImportBlock()` and block verification logic
- Execution events (46-47): `statedb/statedb.go` - in accumulation and state transition code
*/

// Use BlockOutline from types package

// encodeBlockOutline encodes a BlockOutline
func encodeBlockOutline(outline types.BlockOutline) []byte {
	var payload []byte

	// Size in bytes (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(outline.SizeInBytes)...)

	// Header Hash (32 bytes)
	payload = append(payload, outline.HeaderHash.Bytes()...)

	// Number of tickets (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(outline.NumTickets)...)

	// Number of preimages (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(outline.NumPreimages)...)

	// Total size of preimages in bytes (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(outline.PreimagesSizeInBytes)...)

	// Number of guarantees (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(outline.NumGuarantees)...)

	// Number of assurances (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(outline.NumAssurances)...)

	// Number of dispute verdicts (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(outline.NumDisputeVerdicts)...)

	return payload
}

// AccumulateCost is now aliased from types package

// encodeAccumulateCost encodes an AccumulateCost
/*
   Accumulate Cost =
       u32 (Number of accumulate calls) ++
       u32 (Number of transfers processed) ++
       u32 (Number of items accumulated) ++
       Exec Cost (Total) ++
       u64 (Time taken to load and compile the code, in nanoseconds) ++
       Exec Cost (read/write calls) ++
       Exec Cost (lookup calls) ++
       Exec Cost (query/solicit/forget/provide calls) ++
       Exec Cost (info/new/upgrade/eject calls) ++
       Exec Cost (transfer calls) ++
       Gas (Total gas charged for transfer processing by destination services) ++
       Exec Cost (Other host calls)
*/
func encodeAccumulateCost(cost AccumulateCost) []byte {
	var payload []byte

	// Number of accumulate calls (4 bytes)
	payload = append(payload, Uint32ToBytes(cost.NumAccumulateCalls)...)

	// Number of transfers processed (4 bytes)
	payload = append(payload, Uint32ToBytes(cost.NumTransfersProcessed)...)

	// Number of items accumulated (4 bytes)
	payload = append(payload, Uint32ToBytes(cost.NumItemsAccumulated)...)

	// Total Exec Cost (gas + time)
	payload = append(payload, Uint64ToBytes(cost.TotalGasUsed)...)
	payload = append(payload, Uint64ToBytes(cost.TotalTimeNs)...)

	// Load and compile time (8 bytes)
	payload = append(payload, Uint64ToBytes(cost.LoadCompileTimeNs)...)

	// Read/write Exec Cost
	payload = append(payload, Uint64ToBytes(cost.ReadWriteGas)...)
	payload = append(payload, Uint64ToBytes(cost.ReadWriteTimeNs)...)

	// Lookup Exec Cost
	payload = append(payload, Uint64ToBytes(cost.LookupGas)...)
	payload = append(payload, Uint64ToBytes(cost.LookupTimeNs)...)

	// Query/solicit/forget/provide Exec Cost
	payload = append(payload, Uint64ToBytes(cost.QueryGas)...)
	payload = append(payload, Uint64ToBytes(cost.QueryTimeNs)...)

	// Info/new/upgrade/eject Exec Cost
	payload = append(payload, Uint64ToBytes(cost.InfoGas)...)
	payload = append(payload, Uint64ToBytes(cost.InfoTimeNs)...)

	// Transfer Exec Cost
	payload = append(payload, Uint64ToBytes(cost.TransferGas)...)
	payload = append(payload, Uint64ToBytes(cost.TransferTimeNs)...)

	// Total gas charged for transfer processing by destination services (8 bytes)
	payload = append(payload, Uint64ToBytes(cost.TransferProcessingGas)...)

	// Other host calls Exec Cost
	payload = append(payload, Uint64ToBytes(cost.OtherGas)...)
	payload = append(payload, Uint64ToBytes(cost.OtherTimeNs)...)

	return payload
}

// ServiceAccumulateCost is now aliased from types package

/*
### 40: Authoring

Emitted when authoring of a new block begins.

    Slot
    Header Hash (Of the parent block)
*/
// Implemented in: statedb/statedb.go:762-767 in ProcessState function
// Authoring creates payload for authoring event (discriminator 40)
func (c *TelemetryClient) Authoring(slot uint32, parentHeaderHash common.Hash) {

	var payload []byte

	// Slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(slot)...)

	// Parent Header Hash (32 bytes)
	payload = append(payload, parentHeaderHash.Bytes()...)

	c.sendEvent(Telemetry_Authoring, payload)
}

// DecodeAuthoring decodes an Authoring event payload (discriminator 40)
func DecodeAuthoring(payload []byte) string {
	slot, offset := parseUint32(payload, 0)
	parentHeaderHash, _ := parseHash(payload, offset)

	return fmt.Sprintf("slot:%d|parent_header_hash:%s", slot, parentHeaderHash)
}

/*
### 41: Authoring failed

Emitted if block authoring fails for some reason.

    Event ID (ID of the corresponding "authoring" event)
    Reason
*/
// Implemented in: statedb/statedb.go:782 in ProcessState function
// AuthoringFailed creates payload for authoring failed event (discriminator 41)
func (c *TelemetryClient) AuthoringFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Authoring_Failed, payload)
}

// DecodeAuthoringFailed decodes an Authoring Failed event payload (discriminator 41)
func DecodeAuthoringFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 42: Authored

Emitted when a new block has been authored. This should be emitted as soon the contents of the
block have been determined, ideally before accumulation is performed and the new state root is
computed (which is included only in the following block).

    Event ID (ID of the corresponding "authoring" event)
    Block Outline
*/
// Implemented in: statedb/statedb.go:784-802 in ProcessState function
// Authored creates payload for authored event (discriminator 42)
func (c *TelemetryClient) Authored(eventID uint64, blockOutline BlockOutline) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Block Outline
	payload = append(payload, encodeBlockOutline(blockOutline)...)

	c.sendEvent(Telemetry_Authored, payload)
}

// DecodeAuthored decodes an Authored event payload (discriminator 42)
func DecodeAuthored(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Decode Block Outline
	sizeInBytes, offset := parseUint32(payload, offset)
	headerHash, offset := parseHash(payload, offset)
	numTickets, offset := parseUint32(payload, offset)
	numPreimages, offset := parseUint32(payload, offset)
	preimagesSizeInBytes, offset := parseUint32(payload, offset)
	numGuarantees, offset := parseUint32(payload, offset)
	numAssurances, offset := parseUint32(payload, offset)
	numDisputeVerdicts, _ := parseUint32(payload, offset)

	return fmt.Sprintf("event_id:%d|size_in_bytes:%d|header_hash:%s|num_tickets:%d|num_preimages:%d|preimages_size_in_bytes:%d|num_guarantees:%d|num_assurances:%d|num_dispute_verdicts:%d",
		eventID, sizeInBytes, headerHash, numTickets, numPreimages, preimagesSizeInBytes, numGuarantees, numAssurances, numDisputeVerdicts)
}

/*
### 43: Importing

Emitted when importing of a block begins. This should not be emitted by the block author; the
author should emit the "authoring" event instead.

    Slot
    Block Outline
*/
// Implemented in: node/node.go:1979-2009 in ApplyBlock function
// Importing creates payload for importing event (discriminator 43)
func (c *TelemetryClient) Importing(slot uint32, blockOutline BlockOutline) {

	var payload []byte

	// Slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(slot)...)

	// Block Outline
	payload = append(payload, encodeBlockOutline(blockOutline)...)

	c.sendEvent(Telemetry_Importing, payload)
}

// DecodeImporting decodes an Importing event payload (discriminator 43)
func DecodeImporting(payload []byte) string {
	slot, offset := parseUint32(payload, 0)

	// Decode Block Outline
	sizeInBytes, offset := parseUint32(payload, offset)
	headerHash, offset := parseHash(payload, offset)
	numTickets, offset := parseUint32(payload, offset)
	numPreimages, offset := parseUint32(payload, offset)
	preimagesSizeInBytes, offset := parseUint32(payload, offset)
	numGuarantees, offset := parseUint32(payload, offset)
	numAssurances, offset := parseUint32(payload, offset)
	numDisputeVerdicts, _ := parseUint32(payload, offset)

	return fmt.Sprintf("slot:%d|size_in_bytes:%d|header_hash:%s|num_tickets:%d|num_preimages:%d|preimages_size_in_bytes:%d|num_guarantees:%d|num_assurances:%d|num_dispute_verdicts:%d",
		slot, sizeInBytes, headerHash, numTickets, numPreimages, preimagesSizeInBytes, numGuarantees, numAssurances, numDisputeVerdicts)
}

/*
### 44: Block verification failed

Emitted if verification of a block being imported fails for some reason. This includes if the block
is determined to be invalid, ie it does not satisfy all the validity conditions listed in the GP.
In this case, a "peer misbehaved" event should also be emitted for the peer which sent the block.

This event should never be emitted by the block author (authors should emit the "authoring failed"
event instead).

    Event ID (ID of the corresponding "importing" event)
    Reason
*/
// Implemented in: statedb/applystatetransition.go:198-206 in ApplyStateTransitionFromBlock function
// BlockVerificationFailed creates payload for block verification failed event (discriminator 44)
func (c *TelemetryClient) BlockVerificationFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Block_Verification_Failed, payload)
}

// DecodeBlockVerificationFailed decodes a Block Verification Failed event payload (discriminator 44)
func DecodeBlockVerificationFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 45: Block verified

Emitted once a block being imported has been verified. That is, the block satisfies all the
validity conditions listed in the GP. This should be emitted as soon as this has been determined,
ideally before accumulation is performed and the new state root is computed. This should not be
emitted by the block author (the author should emit the "authored" event instead).

    Event ID (ID of the corresponding "importing" event)
*/
// Implemented in: statedb/applystatetransition.go:208-211 in ApplyStateTransitionFromBlock function
// BlockVerified creates payload for block verified event (discriminator 45)
func (c *TelemetryClient) BlockVerified(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Block_Verified, payload)
}

// DecodeBlockVerified decodes a Block Verified event payload (discriminator 45)
func DecodeBlockVerified(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 46: Block execution failed

Emitted if execution of a block fails after authoring/verification. This can happen if, for
example, there is a collision amongst created service IDs during accumulation.

    Event ID (ID of the corresponding "authoring" or "importing" event)
    Reason
*/
// Implemented in: statedb/statedb.go:854 and node/node.go:2062 (authoring and importing paths)
// BlockExecutionFailed creates payload for block execution failed event (discriminator 46)
func (c *TelemetryClient) BlockExecutionFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Block_Execution_Failed, payload)
}

// DecodeBlockExecutionFailed decodes a Block Execution Failed event payload (discriminator 46)
func DecodeBlockExecutionFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 47: Block executed

Emitted following successful execution of a block. This should be emitted by both the block author
and importers.

    Event ID (ID of the corresponding "authoring" or "importing" event)
    len++[Service ID ++ Accumulate Cost] (Accumulated services and the cost of their accumulate calls)

Each service should be listed at most once in the accumulated services list. The length of the
accumulated services list should not exceed 500. If more than 500 services are accumulated in a
block, the costs of the services with lowest total gas usage should be combined and reported with
service ID 0xffffffff (note that this is otherwise not a valid service ID). Ties should be broken
by combining services with greater IDs.
*/
// Implemented in: statedb/applystatetransition.go:384-420 in ApplyStateTransitionFromBlock function
// BlockExecuted creates payload for block executed event (discriminator 47)
func (c *TelemetryClient) BlockExecuted(eventID uint64, services []ServiceAccumulateCost) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// len++[Service ID ++ Accumulate Cost]
	payload = append(payload, types.E(uint64(len(services)))...)

	for _, svc := range services {
		// Service ID (4 bytes, little-endian)
		payload = append(payload, Uint32ToBytes(svc.ServiceID)...)

		// Accumulate Cost
		payload = append(payload, encodeAccumulateCost(svc.Cost)...)
	}

	c.sendEvent(Telemetry_Block_Executed, payload)
}

// DecodeBlockExecuted decodes a Block Executed event payload (discriminator 47)
func DecodeBlockExecuted(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse service list length
	serviceListLength, offset := parseVariableLength(payload, offset)

	serviceDetails := ""
	for i := uint64(0); i < serviceListLength && offset < len(payload); i++ {
		if i > 0 {
			serviceDetails += ","
		}

		serviceID, newOffset := parseUint32(payload, offset)
		offset = newOffset

		// Skip the accumulate cost structure (quite complex, just show service ID for now)
		if offset+22*8 <= len(payload) { // Skip 22 uint64 fields
			offset += 22 * 8
		}

		serviceDetails += fmt.Sprintf("service_%d", serviceID)
	}

	return fmt.Sprintf("event_id:%d|services:[%s]", eventID, serviceDetails)
}

/*
### 43: Accumulate result available

Emitted when accumulate result is available during block processing.

    Slot
    Header Hash
*/
// Implemented in: statedb/applystatetransition.go:260-265 in ApplyStateTransitionFromBlock function
// AccumulateResultAvailable creates payload for accumulate result available event (discriminator 48)
func (c *TelemetryClient) AccumulateResultAvailable(slot uint32, headerHash common.Hash) {

	var payload []byte

	// Slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(slot)...)

	// Header Hash (32 bytes)
	payload = append(payload, headerHash.Bytes()...)

	c.sendEvent(Telemetry_Accumulate_Result_Available, payload)
}

// DecodeAccumulateResultAvailable decodes an Accumulate Result Available event payload (discriminator 48)
func DecodeAccumulateResultAvailable(payload []byte) string {
	slot, offset := parseUint32(payload, 0)
	headerHash, _ := parseHash(payload, offset)

	return fmt.Sprintf("slot:%d|header_hash:%s", slot, headerHash)
}
