package telemetry

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// Type aliases to use types from types package
type WorkItemOutline = types.WorkItemOutline
type WorkPackageOutline = types.WorkPackageOutline
type WorkReportOutline = types.WorkReportOutline
type GuaranteeOutline = types.GuaranteeOutline
type IsAuthorizedCost = types.IsAuthorizedCost
type RefineCost = types.RefineCost

/*
## Guaranteeing events

These events concern the guaranteeing pipeline and guarantee pool.

**Implementation locations for events 90-113:**
- Work-package submission (90-109): `node/node_guarantor.go` - in guarantor pipeline
- Work-package sharing (91-103): `node/node_guarantor.go` - in primary/secondary guarantor logic
- Guarantee distribution (106-113): `node/network.go` - in CE 135 handlers and guarantee pool management
- Authorization/Refine/Build (95, 101-102): `node/node_guarantor.go` - in PVM execution callbacks
*/

// WorkItemOutline represents the outline of a work item
// WorkItemOutline is now aliased from types package

/*
   Work-Package Outline =
       u32 (Work-package size in bytes, excluding extrinsic data) ++
       Work-Package Hash ++
       Header Hash (Anchor) ++
       Slot (Lookup anchor slot) ++
       len++[Work-Package Hash] (Prerequisites) ++
       len++[Work-Item Outline]
*/
// WorkPackageOutline represents the outline of a work package
// WorkPackageOutline is now aliased from types package
// WorkReportOutline represents the outline of a work report
/*
   Work-Report Outline =
       Work-Report Hash ++
       u32 (Bundle size in bytes) ++
       Erasure-Root ++
       Segments-Root
*/
// WorkReportOutline is now aliased from types package
// GuaranteeOutline represents the outline of a guarantee
/*
   Guarantee Outline =
       Work-Report Hash ++
       Slot ++
       len++[Validator Index] (Guarantors)
*/
// GuaranteeOutline is now aliased from types package
// encodeGuaranteeOutline encodes a GuaranteeOutline
func encodeGuaranteeOutline(outline GuaranteeOutline) []byte {
	var payload []byte

	// Work-Report Hash (32 bytes)
	payload = append(payload, outline.WorkReportHash.Bytes()...)

	// Slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(outline.Slot)...)

	// len++[Validator Index]
	payload = append(payload, types.E(uint64(len(outline.Guarantors)))...)
	for _, validatorIndex := range outline.Guarantors {
		payload = append(payload, Uint16ToBytes(validatorIndex)...)
	}

	return payload
}

// encodeWorkPackageOutline encodes a WorkPackageOutline
func encodeWorkPackageOutline(outline WorkPackageOutline) []byte {
	var payload []byte

	// Size in bytes (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(outline.SizeInBytes)...)

	// Work-Package Hash (32 bytes)
	payload = append(payload, outline.WorkPackageHash.Bytes()...)

	// Anchor Hash (32 bytes) - if needed
	if outline.AnchorHash != (common.Hash{}) {
		payload = append(payload, outline.AnchorHash.Bytes()...)
	}

	// Lookup anchor slot (4 bytes) - if needed
	if outline.LookupAnchorSlot != 0 {
		payload = append(payload, Uint32ToBytes(outline.LookupAnchorSlot)...)
	}

	// Prerequisites len++[Work-Package Hash] - if needed
	if len(outline.Prerequisites) > 0 {
		payload = append(payload, types.E(uint64(len(outline.Prerequisites)))...)
		for _, prereq := range outline.Prerequisites {
			payload = append(payload, prereq.Bytes()...)
		}
	}

	// Work items len++[Work-Item Outline] - if needed
	if len(outline.WorkItems) > 0 {
		payload = append(payload, types.E(uint64(len(outline.WorkItems)))...)
		for _, item := range outline.WorkItems {
			payload = append(payload, Uint32ToBytes(item.ServiceID)...)
			payload = append(payload, Uint32ToBytes(item.PayloadSize)...)
			payload = append(payload, Uint64ToBytes(item.RefineGasLimit)...)
			payload = append(payload, Uint64ToBytes(item.AccumulateGasLimit)...)
			payload = append(payload, Uint32ToBytes(item.ExtrinsicsLength)...)
			payload = append(payload, types.E(uint64(len(item.ImportSpecs)))...)
			for _, spec := range item.ImportSpecs {
				payload = append(payload, types.E(uint64(len(spec)))...)
				payload = append(payload, spec...)
			}
			payload = append(payload, Uint16ToBytes(item.NumExportedSegs)...)
		}
	}

	return payload
}

// IsAuthorizedCost represents the cost of is-authorized check
// IsAuthorizedCost is now aliased from types package
/*
   Refine Cost =
       Exec Cost (Total) ++
       u64 (Time taken to load and compile the code, in nanoseconds) ++
       Exec Cost (historical_lookup calls) ++
       Exec Cost (machine/expunge calls) ++
       Exec Cost (peek/poke/pages calls) ++
       Exec Cost (invoke calls) ++
       Exec Cost (Other host calls)
*/
// RefineCost represents the cost of refine call
// RefineCost is now aliased from types package
/*
### 90: Work-package submission

Emitted when a builder opens a stream to submit a work-package (CE 133/146). This should be emitted
as soon as the stream is opened, before the work-package is read from the stream.

    Peer ID (Builder)
    bool (Using CE 146?)
*/
// Implemented in: peerCE133_workpackagesubmission.go
// WorkPackageSubmission creates payload for work-package submission event (discriminator 90)
func (c *TelemetryClient) WorkPackageSubmission(eventID uint64, peerID [32]byte, workPackageOutline WorkPackageOutline) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - builder)
	payload = append(payload, peerID[:]...)

	// Using CE 146? (1 byte: 0 = False, always false for now)
	payload = append(payload, 0)

	// Work-Package Outline
	payload = append(payload, encodeWorkPackageOutline(workPackageOutline)...)

	c.sendEvent(Telemetry_Work_Package_Submission, payload)
}

// DecodeWorkPackageSubmission decodes a Work Package Submission event payload (discriminator 90)
func DecodeWorkPackageSubmission(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, offset := parsePeerID(payload, offset)
	usingCE146, offset := parseUint8(payload, offset)

	// Skip complex work package outline parsing for now
	return fmt.Sprintf("event_id:%d|peer_id:%s|using_ce146:%t", eventID, peerID, usingCE146 != 0)
}

/*
### 91: Work-package being shared

Emitted by the secondary guarantor when a work-package sharing stream is opened (CE 134). This
should be emitted as soon as the stream is opened, before any messages are read from the stream.

    Peer ID (Primary guarantor)
*/
// Implemented in: peerCE134_workpackageshare.go
// WorkPackageBeingShared creates payload for work-package being shared event (discriminator 91)
func (c *TelemetryClient) WorkPackageBeingShared(peerID [32]byte) {

	var payload []byte

	// Peer ID (32 bytes - primary guarantor)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Work_Package_Being_Shared, payload)
}

// DecodeWorkPackageBeingShared decodes a Work Package Being Shared event payload (discriminator 91)
func DecodeWorkPackageBeingShared(payload []byte) string {
	peerID, _ := parsePeerID(payload, 0)

	return fmt.Sprintf("peer_id:%s", peerID)
}

/*
### 92: Work-package failed

Emitted if receiving a work-package from a builder or another guarantor fails, or processing of a
received work-package fails. This may be emitted at any point in the guarantor pipeline.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    Reason
*/
// Implemented in: peerCE133_workpackagesubmission.go, peerCE134_workpackageshare.go
// WorkPackageFailed creates payload for work-package failed event (discriminator 92)
func (c *TelemetryClient) WorkPackageFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Work_Package_Failed, payload)
}

// DecodeWorkPackageFailed decodes a Work Package Failed event payload (discriminator 92)
func DecodeWorkPackageFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 93: Duplicate work-package

Emitted when a "duplicate" work-package is received from a builder or shared by another guarantor
(CE 133/134). A duplicate work-package is one that exactly matches (same hash) a previously
received work-package. This event may be emitted at any point in the guarantor pipeline. In
particular, it may be emitted instead of a "work-package received" event. An efficient
implementation should check for duplicates early on to avoid wasted effort!

In the case of a duplicate work-package received from a builder, no further events should be
emitted referencing the submission.

In the case of a duplicate work-package shared by another guarantor, only one more event should be
emitted referencing the "work-package being shared" event: either a "work-package failed" event
indicating failure or a "work-report signature sent" event indicating success.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    Core Index
    Work-Package Hash
*/
// Implemented in: node/peerCE133_workpackagesubmission.go:230 in onWorkPackageSubmission function
// DuplicateWorkPackage creates payload for duplicate work-package event (discriminator 93)
func (c *TelemetryClient) DuplicateWorkPackage(eventID uint64, coreIndex uint16, workPackageHash common.Hash) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Core Index (2 bytes, little-endian)
	payload = append(payload, Uint16ToBytes(coreIndex)...)

	// Work-Package Hash (32 bytes)
	payload = append(payload, workPackageHash.Bytes()...)

	c.sendEvent(Telemetry_Duplicate_Work_Package, payload)
}

// DecodeDuplicateWorkPackage decodes a Duplicate Work Package event payload (discriminator 93)
func DecodeDuplicateWorkPackage(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	coreIndex, offset := parseUint16(payload, offset)
	workPackageHash, _ := parseHash(payload, offset)

	return fmt.Sprintf("event_id:%d|core_index:%d|work_package_hash:%s", eventID, coreIndex, workPackageHash)
}

/*
### 94: Work-package received

Emitted once a work-package has been received from a builder or a primary guarantor (CE 133/134).
This should be emitted _before_ authorization is checked, and ideally before the extrinsic data and
imports are received/fetched.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    Core Index
    Work-Package Outline
*/
// Implemented in: peerCE133_workpackagesubmission.go
// WorkPackageReceived creates payload for work-package received event (discriminator 94)
func (c *TelemetryClient) WorkPackageReceived(eventID uint64, workPackageOutline WorkPackageOutline) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Work-Package Outline
	// Size in bytes (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(workPackageOutline.SizeInBytes)...)

	// Work-Package Hash (32 bytes)
	payload = append(payload, workPackageOutline.WorkPackageHash.Bytes()...)

	// Anchor Header Hash (32 bytes)
	payload = append(payload, workPackageOutline.AnchorHash.Bytes()...)

	// Lookup anchor slot (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(workPackageOutline.LookupAnchorSlot)...)

	// Prerequisites len++[Work-Package Hash]
	payload = append(payload, types.E(uint64(len(workPackageOutline.Prerequisites)))...)
	for _, prereq := range workPackageOutline.Prerequisites {
		payload = append(payload, prereq.Bytes()...)
	}

	// Work items len++[Work-Item Outline]
	payload = append(payload, types.E(uint64(len(workPackageOutline.WorkItems)))...)
	for _, item := range workPackageOutline.WorkItems {
		// Service ID (4 bytes)
		payload = append(payload, Uint32ToBytes(item.ServiceID)...)

		// Payload size (4 bytes)
		payload = append(payload, Uint32ToBytes(item.PayloadSize)...)

		// Refine gas limit (8 bytes)
		payload = append(payload, Uint64ToBytes(item.RefineGasLimit)...)

		// Accumulate gas limit (8 bytes)
		payload = append(payload, Uint64ToBytes(item.AccumulateGasLimit)...)

		// Sum of extrinsic lengths (4 bytes)
		payload = append(payload, Uint32ToBytes(item.ExtrinsicsLength)...)

		// Import specs len++[Import Spec]
		payload = append(payload, types.E(uint64(len(item.ImportSpecs)))...)
		for _, spec := range item.ImportSpecs {
			payload = append(payload, spec...)
		}

		// Number of exported segments (2 bytes)
		payload = append(payload, Uint16ToBytes(item.NumExportedSegs)...)
	}

	c.sendEvent(Telemetry_Work_Package_Received, payload)
}

// DecodeWorkPackageReceived decodes a Work Package Received event payload (discriminator 94)
func DecodeWorkPackageReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Simplified work package outline parsing
	sizeInBytes, offset := parseUint32(payload, offset)
	workPackageHash, offset := parseHash(payload, offset)
	anchorHash, offset := parseHash(payload, offset)
	lookupAnchorSlot, offset := parseUint32(payload, offset)

	return fmt.Sprintf("event_id:%d|size_in_bytes:%d|work_package_hash:%s|anchor_hash:%s|lookup_anchor_slot:%d", eventID, sizeInBytes, workPackageHash, anchorHash, lookupAnchorSlot)
}

/*
### 95: Authorized

Emitted once basic validity checks have been performed on a received work-package, including the
authorization check. This should be emitted by both primary (received the work-package from a
builder) and secondary (received the work-package from a primary) guarantors.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    Is-Authorized Cost
*/
// Implemented in: node/node_guarantee.go:255 and node/peerCE134_workpackageshare.go:503
// Authorized creates payload for authorized event (discriminator 95)
func (c *TelemetryClient) Authorized(eventID uint64, cost IsAuthorizedCost) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Is-Authorized Cost
	// Total Exec Cost (gas + time)
	payload = append(payload, Uint64ToBytes(cost.TotalGasUsed)...)
	payload = append(payload, Uint64ToBytes(cost.TotalTimeNs)...)

	// Load and compile time
	payload = append(payload, Uint64ToBytes(cost.LoadCompileTimeNs)...)

	// Host calls Exec Cost
	payload = append(payload, Uint64ToBytes(cost.HostCallsGasUsed)...)
	payload = append(payload, Uint64ToBytes(cost.HostCallsTimeNs)...)

	c.sendEvent(Telemetry_Authorized, payload)
}

// DecodeAuthorized decodes an Authorized event payload (discriminator 95)
func DecodeAuthorized(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse IsAuthorized Cost (simplified)
	totalGasUsed, offset := parseUint64(payload, offset)
	totalTimeNs, offset := parseUint64(payload, offset)
	loadCompileTimeNs, offset := parseUint64(payload, offset)
	hostCallsGasUsed, offset := parseUint64(payload, offset)
	hostCallsTimeNs, _ := parseUint64(payload, offset)

	return fmt.Sprintf("event_id:%d|total_gas_used:%d|total_time_ns:%d|load_compile_time_ns:%d|host_calls_gas_used:%d|host_calls_time_ns:%d", eventID, totalGasUsed, totalTimeNs, loadCompileTimeNs, hostCallsGasUsed, hostCallsTimeNs)
}

/*
### 96: Extrinsic data received

Emitted once the extrinsic data for a work-package has been received from a builder or a primary
guarantor (CE 133/134) and verified as consistent with the extrinsic hashes in the work-package.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
*/
// Implemented in: node/peerCE133_workpackagesubmission.go:222 in onWorkPackageSubmission function
// ExtrinsicDataReceived creates payload for extrinsic data received event (discriminator 96)
func (c *TelemetryClient) ExtrinsicDataReceived(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Extrinsic_Data_Received, payload)
}

// DecodeExtrinsicDataReceived decodes an Extrinsic Data Received event payload (discriminator 96)
func DecodeExtrinsicDataReceived(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 97: Imports received

Emitted once all the imports for a work-package have been fetched/received and verified.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
*/
// Implemented in: node/node_guarantee.go:215 in processWPQueueItem function
// ImportsReceived creates payload for imports received event (discriminator 97)
func (c *TelemetryClient) ImportsReceived(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Imports_Received, payload)
}

// DecodeImportsReceived decodes an Imports Received event payload (discriminator 97)
func DecodeImportsReceived(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 98: Sharing work-package

Emitted by the primary guarantor when a work-package sharing stream is opened (CE 134).

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Secondary guarantor)
*/
// Implemented in: peerCE134_workpackageshare.go
// SharingWorkPackage creates payload for sharing work-package event (discriminator 98)
func (c *TelemetryClient) SharingWorkPackage(eventID uint64, peerID [32]byte, workPackageOutline WorkPackageOutline) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - secondary guarantor)
	payload = append(payload, peerID[:]...)

	// Work-Package Outline
	payload = append(payload, encodeWorkPackageOutline(workPackageOutline)...)

	c.sendEvent(Telemetry_Sharing_Work_Package, payload)
}

// DecodeSharingWorkPackage decodes a Sharing Work Package event payload (discriminator 98)
func DecodeSharingWorkPackage(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, _ := parsePeerID(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s", eventID, peerID)
}

// DecodeWorkPackageBeingSharedReceived decodes a Work Package Being Shared Received event payload (discriminator 100)
func DecodeWorkPackageBeingSharedReceived(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 99: Work-package sharing failed

Emitted if sharing a work-package with another guarantor fails (CE 134). Possible failures include
failure to send the bundle, failure to receive a work-report signature, or receipt of an invalid
work-report signature (in this case, a "peer misbehaved" event should also be emitted for the
secondary guarantor). This event should only be emitted by the primary guarantor; the secondary
guarantor should emit the "work-package failed" event on failure.

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Secondary guarantor)
    Reason
*/
// Implemented in: node/node_guarantee.go:286 in processWPQueueItem function
// WorkPackageSharingFailed creates payload for work-package sharing failed event (discriminator 99)
func (c *TelemetryClient) WorkPackageSharingFailed(eventID uint64, peerID [32]byte, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - secondary guarantor)
	payload = append(payload, peerID[:]...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Work_Package_Sharing_Failed, payload)
}

// DecodeWorkPackageSharingFailed decodes a Work Package Sharing Failed event payload (discriminator 99)
func DecodeWorkPackageSharingFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, offset := parsePeerID(payload, offset)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s|reason:%s", eventID, peerID, reason)
}

/*
### 100: Bundle sent

Emitted by the primary guarantor once a work-package bundle has been sent to a secondary guarantor
(CE 134).

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Secondary guarantor)
*/
// Implemented in: node/node_guarantee.go:291 in processWPQueueItem function
// BundleSent creates payload for bundle sent event (discriminator 100)
func (c *TelemetryClient) BundleSent(eventID uint64, peerID [32]byte) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - secondary guarantor)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Bundle_Sent, payload)
}

// DecodeBundleSent decodes a Bundle Sent event payload (discriminator 100)
func DecodeBundleSent(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, _ := parsePeerID(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s", eventID, peerID)
}

/*
### 101: Refined

Emitted once a work-package has been refined locally. This should be emitted by both primary and
secondary guarantors.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    len++[Refine Cost] (Cost of refine call for each work item)
*/
// Implemented in: node/node_guarantee.go:258 and node/peerCE134_workpackageshare.go:506
// Refined creates payload for refined event (discriminator 101)
func (c *TelemetryClient) Refined(eventID uint64, refineCosts []RefineCost) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// len++[Refine Cost]
	payload = append(payload, types.E(uint64(len(refineCosts)))...)

	for _, cost := range refineCosts {
		// Total Exec Cost (gas + time)
		payload = append(payload, Uint64ToBytes(cost.TotalGasUsed)...)
		payload = append(payload, Uint64ToBytes(cost.TotalTimeNs)...)

		// Load and compile time
		payload = append(payload, Uint64ToBytes(cost.LoadCompileTimeNs)...)

		// historical_lookup Exec Cost
		payload = append(payload, Uint64ToBytes(cost.HistoricalLookupGas)...)
		payload = append(payload, Uint64ToBytes(cost.HistoricalLookupTimeNs)...)

		// machine/expunge Exec Cost
		payload = append(payload, Uint64ToBytes(cost.MachineExpungeGas)...)
		payload = append(payload, Uint64ToBytes(cost.MachineExpungeTimeNs)...)

		// peek/poke/pages Exec Cost
		payload = append(payload, Uint64ToBytes(cost.PeekPokeGas)...)
		payload = append(payload, Uint64ToBytes(cost.PeekPokeTimeNs)...)

		// invoke Exec Cost
		payload = append(payload, Uint64ToBytes(cost.InvokeGas)...)
		payload = append(payload, Uint64ToBytes(cost.InvokeTimeNs)...)

		// Other host calls Exec Cost
		payload = append(payload, Uint64ToBytes(cost.OtherGas)...)
		payload = append(payload, Uint64ToBytes(cost.OtherTimeNs)...)
	}

	c.sendEvent(Telemetry_Refined, payload)
}

// DecodeRefined decodes a Refined event payload (discriminator 101)
func DecodeRefined(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse refine costs length
	refineCostsLength, offset := parseVariableLength(payload, offset)

	return fmt.Sprintf("event_id:%d|refine_costs_count:%d", eventID, refineCostsLength)
}

/*
### 102: Work-report built

Emitted once a work-report has been built for a work-package. This should be emitted by both
primary and secondary guarantors.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    Work-Report Outline
*/
// Implemented in: node/node_guarantee.go:267 and node/peerCE134_workpackageshare.go:515
// WorkReportBuilt creates payload for work-report built event (discriminator 102)
func (c *TelemetryClient) WorkReportBuilt(eventID uint64, workReportOutline WorkReportOutline) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Work-Report Outline
	// Work-Report Hash (32 bytes)
	payload = append(payload, workReportOutline.WorkReportHash.Bytes()...)

	// Bundle size (4 bytes, little-endian)
	payload = append(payload, Uint32ToBytes(workReportOutline.BundleSize)...)

	// Erasure-Root (32 bytes)
	payload = append(payload, workReportOutline.ErasureRoot.Bytes()...)

	// Segments-Root (32 bytes)
	payload = append(payload, workReportOutline.SegmentsRoot.Bytes()...)

	c.sendEvent(Telemetry_Work_Report_Built, payload)
}

// DecodeWorkReportBuilt decodes a Work Report Built event payload (discriminator 102)
func DecodeWorkReportBuilt(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse Work Report Outline
	workReportHash, offset := parseHash(payload, offset)
	bundleSize, offset := parseUint32(payload, offset)
	erasureRoot, offset := parseHash(payload, offset)
	segmentsRoot, _ := parseHash(payload, offset)

	return fmt.Sprintf("event_id:%d|work_report_hash:%s|bundle_size:%d|erasure_root:%s|segments_root:%s", eventID, workReportHash, bundleSize, erasureRoot, segmentsRoot)
}

/*
### 103: Work-report signature sent

Emitted once a work-report signature for a shared work-package has been sent to the primary
guarantor (CE 134). This is the final event in the guaranteeing pipeline for secondary guarantors.

    Event ID (ID of the corresponding "work-package being shared" event)
*/
// Implemented in: peerCE134_workpackageshare.go

// WorkReportSignatureSent creates payload for work-report signature sent event (discriminator 103)
func (c *TelemetryClient) WorkReportSignatureSent(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Work_Report_Signature_Sent, payload)
}

// DecodeWorkReportSignatureSent decodes a Work Report Signature Sent event payload (discriminator 103)
func DecodeWorkReportSignatureSent(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 104: Work-report signature received

Emitted by the primary guarantor once a valid work-report signature has been received from a
secondary guarantor (CE 134). If an invalid work-report signature is received, a "work-package
sharing failed" event should be emitted instead, as well as a "peer misbehaved" event.

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Secondary guarantor)
*/
// Implemented in: peerCE134_workpackageshare.go
// WorkReportSignatureReceived creates payload for work-report signature received event (discriminator 104)
func (c *TelemetryClient) WorkReportSignatureReceived(eventID uint64, peerID [32]byte, workReportHash common.Hash) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - secondary guarantor)
	payload = append(payload, peerID[:]...)

	// Work-Report Hash (32 bytes)
	payload = append(payload, workReportHash.Bytes()...)

	c.sendEvent(Telemetry_Work_Report_Signature_Received, payload)
}

// DecodeWorkReportSignatureReceived decodes a Work Report Signature Received event payload (discriminator 104)
func DecodeWorkReportSignatureReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, offset := parsePeerID(payload, offset)
	workReportHash, _ := parseHash(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s|work_report_hash:%s", eventID, peerID, workReportHash)
}

/*
### 105: Guarantee built

Emitted by the primary guarantor once a work-report guarantee has been built. If a secondary
guarantor is slow to send their signature, this event may be emitted twice: once for the guarantee
with just two signatures, and again for the guarantee with all three signatures.

    Event ID (ID of the corresponding "work-package submission" event)
    Guarantee Outline
*/
// Implemented in: node/node_guarantee.go:352 in processWPQueueItem function
// GuaranteeBuilt creates payload for guarantee built event (discriminator 105)
func (c *TelemetryClient) GuaranteeBuilt(eventID uint64, guaranteeOutline GuaranteeOutline) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Guarantee Outline
	payload = append(payload, encodeGuaranteeOutline(guaranteeOutline)...)

	c.sendEvent(Telemetry_Guarantee_Built, payload)
}

// DecodeGuaranteeBuilt decodes a Guarantee Built event payload (discriminator 105)
func DecodeGuaranteeBuilt(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse Guarantee Outline
	workReportHash, offset := parseHash(payload, offset)
	slot, offset := parseUint32(payload, offset)

	// Parse guarantors length
	guarantorsLength, offset := parseVariableLength(payload, offset)

	return fmt.Sprintf("event_id:%d|work_report_hash:%s|slot:%d|guarantors_count:%d", eventID, workReportHash, slot, guarantorsLength)
}

/*
### 106: Sending guarantee

Emitted when a guarantor begins sending a work-report guarantee to another validator, for potential
inclusion in a block (CE 135). This should reference the "guarantee built" event corresponding to
the guarantee that is being sent.

    Event ID (ID of the corresponding "guarantee built" event)
    Peer ID (Recipient)
*/
// Implemented in: peerCE135_workreportdistribution.go
// SendingGuarantee creates payload for sending guarantee event (discriminator 106)
func (c *TelemetryClient) SendingGuarantee(eventID uint64, peerID [32]byte) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Peer ID (32 bytes - recipient)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Sending_Guarantee, payload)
}

// DecodeSendingGuarantee decodes a Sending Guarantee event payload (discriminator 106)
func DecodeSendingGuarantee(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	peerID, _ := parsePeerID(payload, offset)

	return fmt.Sprintf("event_id:%d|peer_id:%s", eventID, peerID)
}

/*
### 107: Guarantee send failed

Emitted if sending a work-report guarantee fails (CE 135).

    Event ID (ID of the corresponding "sending guarantee" event)
    Reason
*/
// Implemented in: peerCE135_workreportdistribution.go
// GuaranteeSendFailed creates payload for guarantee send failed event (discriminator 107)
func (c *TelemetryClient) GuaranteeSendFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Guarantee_Send_Failed, payload)
}

// DecodeGuaranteeSendFailed decodes a Guarantee Send Failed event payload (discriminator 107)
func DecodeGuaranteeSendFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 108: Guarantee sent

Emitted if sending a work-report guarantee succeeds (CE 135).

    Event ID (ID of the corresponding "sending guarantee" event)
*/
// Implemented in: peerCE135_workreportdistribution.go
// GuaranteeSent creates payload for guarantee sent event (discriminator 108)
func (c *TelemetryClient) GuaranteeSent(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Guarantee_Sent, payload)
}

// DecodeGuaranteeSent decodes a Guarantee Sent event payload (discriminator 108)
func DecodeGuaranteeSent(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 109: Guarantees distributed

Emitted by the primary guarantor once they have finished distributing the work-report guarantee(s).
This is the final event in the guaranteeing pipeline for primary guarantors.

This event may be emitted even if the guarantor was not successful in sending the guarantee(s) to
any other validator, although the guarantor may prefer to emit a "work-package failed" event in
that case.

    Event ID (ID of the corresponding "work-package submission" event)
*/
// Implemented in: node/node_guarantee.go:356 in processWPQueueItem function
// GuaranteesDistributed creates payload for guarantees distributed event (discriminator 109)
func (c *TelemetryClient) GuaranteesDistributed(eventID uint64) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	c.sendEvent(Telemetry_Guarantees_Distributed, payload)
}

// DecodeGuaranteesDistributed decodes a Guarantees Distributed event payload (discriminator 109)
func DecodeGuaranteesDistributed(payload []byte) string {
	eventID, _ := parseUint64(payload, 0)

	return fmt.Sprintf("event_id:%d", eventID)
}

/*
### 110: Receiving guarantee

Emitted by the recipient when a guarantor begins sending a work-report guarantee (CE 135).

    Peer ID (Sender)
*/
// Implemented in: peerCE135_workreportdistribution.go
// ReceivingGuarantee creates payload for receiving guarantee event (discriminator 110)
func (c *TelemetryClient) ReceivingGuarantee(peerID [32]byte) {

	var payload []byte

	// Peer ID (32 bytes - sender)
	payload = append(payload, peerID[:]...)

	c.sendEvent(Telemetry_Receiving_Guarantee, payload)
}

// DecodeReceivingGuarantee decodes a Receiving Guarantee event payload (discriminator 110)
func DecodeReceivingGuarantee(payload []byte) string {
	peerID, _ := parsePeerID(payload, 0)

	return fmt.Sprintf("peer_id:%s", peerID)
}

/*
### 111: Guarantee receive failed

Emitted if receiving a work-report guarantee fails (CE 135).

    Event ID (ID of the corresponding "receiving guarantee" event)
    Reason
*/
// Implemented in: peerCE135_workreportdistribution.go
// GuaranteeReceiveFailed creates payload for guarantee receive failed event (discriminator 111)
func (c *TelemetryClient) GuaranteeReceiveFailed(eventID uint64, reason string) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Reason (String<128>)
	payload = append(payload, encodeString(reason, 128)...)

	c.sendEvent(Telemetry_Guarantee_Receive_Failed, payload)
}

// DecodeGuaranteeReceiveFailed decodes a Guarantee Receive Failed event payload (discriminator 111)
func DecodeGuaranteeReceiveFailed(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)
	reason, _ := parseString(payload, offset)

	return fmt.Sprintf("event_id:%d|reason:%s", eventID, reason)
}

/*
### 112: Guarantee received

Emitted if receiving a work-report guarantee succeeds (CE 135). This should be emitted before the
guarantee is checked. If the guarantee is found to be invalid, a "peer misbehaved" event should be
emitted.

    Event ID (ID of the corresponding "receiving guarantee" event)
    Guarantee Outline
*/
// Implemented in: peerCE135_workreportdistribution.go
// GuaranteeReceived creates payload for guarantee received event (discriminator 112)
func (c *TelemetryClient) GuaranteeReceived(eventID uint64, guaranteeOutline GuaranteeOutline) {

	var payload []byte

	// Event ID (8 bytes, little-endian)
	payload = append(payload, Uint64ToBytes(eventID)...)

	// Guarantee Outline
	payload = append(payload, encodeGuaranteeOutline(guaranteeOutline)...)

	c.sendEvent(Telemetry_Guarantee_Received, payload)
}

// DecodeGuaranteeReceived decodes a Guarantee Received event payload (discriminator 112)
func DecodeGuaranteeReceived(payload []byte) string {
	eventID, offset := parseUint64(payload, 0)

	// Parse Guarantee Outline
	workReportHash, offset := parseHash(payload, offset)
	slot, offset := parseUint32(payload, offset)

	// Parse guarantors length
	guarantorsLength, _ := parseVariableLength(payload, offset)

	return fmt.Sprintf("event_id:%d|work_report_hash:%s|slot:%d|guarantors_count:%d", eventID, workReportHash, slot, guarantorsLength)
}

/*
### 113: Guarantee discarded

Emitted when a guarantee is discarded from the local guarantee pool.

	Guarantee Outline
	Guarantee Discard Reason

Guarantee Discard Reason =

	0 (Work-package reported on-chain) OR
	1 (Replaced by better guarantee) OR
	2 (Cannot be reported on-chain) OR
	3 (Too many guarantees) OR
	4 (Other)
	(Single byte)
*/

// TODO: IMPLEMENT THIS in node_author.go or guarantee pool management
// GuaranteeDiscarded creates payload for guarantee discarded event (discriminator 113)
func (c *TelemetryClient) GuaranteeDiscarded(guaranteeOutline GuaranteeOutline, discardReason byte) {

	var payload []byte

	// Guarantee Outline
	payload = append(payload, encodeGuaranteeOutline(guaranteeOutline)...)

	// Guarantee Discard Reason (1 byte)
	payload = append(payload, discardReason)

	c.sendEvent(Telemetry_Guarantee_Discarded, payload)
}

// DecodeGuaranteeDiscarded decodes a Guarantee Discarded event payload (discriminator 113)
func DecodeGuaranteeDiscarded(payload []byte) string {
	// Parse Guarantee Outline
	workReportHash, offset := parseHash(payload, 0)
	slot, offset := parseUint32(payload, offset)

	// Parse guarantors length
	guarantorsLength, offset := parseVariableLength(payload, offset)
	// Skip guarantor indices
	offset += int(guarantorsLength) * 2

	discardReason, _ := parseUint8(payload, offset)

	discardReasonStr := "unknown"
	switch discardReason {
	case 0:
		discardReasonStr = "reported_on_chain"
	case 1:
		discardReasonStr = "replaced_by_better"
	case 2:
		discardReasonStr = "cannot_report_on_chain"
	case 3:
		discardReasonStr = "too_many_guarantees"
	case 4:
		discardReasonStr = "other"
	}

	return fmt.Sprintf("work_report_hash:%s|slot:%d|guarantors_count:%d|discard_reason:%s", workReportHash, slot, guarantorsLength, discardReasonStr)
}
