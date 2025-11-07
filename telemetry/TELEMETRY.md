# Jaeger Integration for JAM Telemetry

## Overview

This document describes how JAM's JIP-3 telemetry integrates with Jaeger for distributed tracing across nodes. Unlike traditional intra-node event tracking, this approach enables inter-node trace correlation using JAM protocol identifiers (work package hashes, header hashes, etc.) as trace IDs.

## Key Concept: Dual-Purpose Event IDs

The `GetEventID()` function supports two modes:

1. **Intra-node (JIP-3)**: Sequential `u64` Event IDs for local event linking within a single node
2. **Inter-node (Jaeger)**: JAM protocol identifiers (hashes) for distributed trace correlation across nodes

```go
// Intra-node: Sequential Event ID for JIP-3 telemetry server
eventID := telemetryClient.GetEventID()
// Returns: sequential u64 (e.g., 0, 1, 2, ...)
// Sends: JIP-3 event with u64 eventID to TART Server

// Inter-node: Use work package hash as trace identifier for Jaeger
eventID := telemetryClient.GetEventID(workPackageHash)
// Returns: u64 derived from hash (first 8 bytes)
// Sends:
//   - JIP-3 event with u64 eventID to TART Server
//   - OpenTelemetry span with full hash as traceID to Jaeger
```

**Important**: When a hash is provided to `GetEventID()`, the TelemetryClient:
1. Returns a u64 (first 8 bytes of hash) for use in JIP-3 messages to TART
2. Also sends the full hash as a trace ID to Jaeger for inter-node correlation

## Architecture

```
┌─────────────────────────────────┐         ┌─────────────────────────────────┐
│        JAM Node 1               │         │        JAM Node 2               │
│        (Guarantor)              │         │        (Auditor)                │
│                                 │         │                                 │
│  ┌───────────────────────────┐ │         │  ┌───────────────────────────┐ │
│  │   TelemetryClient         │ │         │  │   TelemetryClient         │ │
│  │                           │ │         │  │                           │ │
│  │ • GetEventID()     → u64  │ │         │  │ • GetEventID()     → u64  │ │
│  │ • GetEventID(hash) → u64  │ │         │  │ • GetEventID(hash) → u64  │ │
│  └─────┬─────────────────┬───┘ │         │  └─────┬─────────────────┬───┘ │
└────────┼─────────────────┼─────┘         └────────┼─────────────────┼─────┘
         │                 │                        │                 │
         │ JIP-3           │ OpenTelemetry          │ JIP-3           │ OpenTelemetry
         │ (u64 EventID)   │ (Trace ID from hash)   │ (u64 EventID)   │ (Trace ID from hash)
         │                 │                        │                 │
         ▼                 │                        ▼                 │
┌────────────────────┐     │                ┌────────────────────┐     │
│   TART Server      │     │                │   TART Server      │     │
│   (JIP-3)          │     │                │   (JIP-3)          │     │
│                    │     │                │                    │     │
│ • Intra-node       │     │                │ • Intra-node       │     │
│   event tracking   │     │                │   event tracking   │     │
│ • Sequential IDs   │     │                │   Sequential IDs   │     │
│ • Analysis/Debug   │     │                │ • Analysis/Debug   │     │
└────────────────────┘     │                └────────────────────┘     │
                           │                                           │
                           └───────────────┬───────────────────────────┘
                                           │
                                           │ OTLP
                                           │ (Spans with Trace ID)
                                           ▼
                                   ┌───────────────┐
                                   │    Jaeger     │
                                   │   Backend     │
                                   │               │
                                   │ • Inter-node  │
                                   │   correlation │
                                   │ • Trace IDs:  │
                                   │   - WP Hash   │
                                   │   - Header    │
                                   │   - Erasure   │
                                   └───────┬───────┘
                                           │
                                           │ http://localhost:16686
                                           ▼
                                   ┌───────────────┐
                                   │  Jaeger UI    │
                                   │ (Trace View)  │
                                   └───────────────┘
```

## Complete Event Mapping for Jaeger

This table maps all telemetry events (10-199) to their Jaeger trace IDs. Events with **no trace ID** are intra-node only (use sequential IDs for TART).

| Event # | Event Name | Trace ID (Jaeger) | Notes |
|---------|------------|-------------------|-------|
| **10** | Status | None | Intra-node only |
| **11** | Best Block Changed | Header Hash | Block header changed |
| **12** | Finalized Block Changed | Header Hash | Block finalization |
| **13** | Sync Status Changed | None | Intra-node only |
| 14-19 | *[Gap]* | - | - |
| **20** | Connection Refused | Peer ID | P2P connection |
| **21** | Connecting In | Peer ID | P2P connection |
| **22** | Connect In Failed | Peer ID | P2P connection |
| **23** | Connected In | Peer ID | P2P connection |
| **24** | Connecting Out | Peer ID | P2P connection |
| **25** | Connect Out Failed | Peer ID | P2P connection |
| **26** | Connected Out | Peer ID | P2P connection |
| **27** | Disconnected | Peer ID | P2P connection |
| **28** | Peer Misbehaved | Peer ID | P2P connection |
| 29-39 | *[Gap]* | - | - |
| **40** | Authoring | Header Hash | Block authoring start |
| **41** | Authoring Failed | Header Hash | Block authoring failed |
| **42** | Authored | Header Hash | Block authoring complete |
| **43** | Importing | Header Hash | Block import start |
| **44** | Block Verification Failed | Header Hash | Block verification failed |
| **45** | Block Verified | Header Hash | Block verification complete |
| **46** | Block Execution Failed | Header Hash | Block execution failed |
| **47** | Block Executed | Header Hash | Block execution complete |
| **48** | Accumulate Result Available | Header Hash | Accumulate result ready |
| 49-59 | *[Gap]* | - | - |
| **60** | Block Announcement Stream Opened | Peer ID | Stream opened |
| **61** | Block Announcement Stream Closed | Peer ID | Stream closed |
| **62** | Block Announced | Header Hash | Block announcement |
| **63** | Sending Block Request | Header Hash | Request block |
| **64** | Receiving Block Request | Header Hash | Receive block request |
| **65** | Block Request Failed | Header Hash | Block request failed |
| **66** | Block Request Sent | Header Hash | Block request sent |
| **67** | Block Request Received | Header Hash | Block request received |
| **68** | Block Transferred | Header Hash | Block transfer complete |
| 69-70 | *[Gap]* | - | - |
| **71** | Block Announcement Malformed | None | Intra-node only |
| 72-79 | *[Gap]* | - | - |
| **80** | Generating Tickets | Epoch Index (u32) | Ticket generation start |
| **81** | Ticket Generation Failed | Epoch Index (u32) | Ticket generation failed |
| **82** | Tickets Generated | Epoch Index (u32) | Ticket generation complete |
| **83** | Ticket Transfer Failed | Epoch Index (u32) | Ticket transfer failed |
| **84** | Ticket Transferred | Epoch Index (u32) | Ticket transfer complete |
| 85-89 | *[Gap]* | - | - |
| **90** | Work Package Submission | Work Package Hash | WP submitted |
| **91** | Work Package Being Shared | Work Package Hash | WP sharing start |
| **92** | Work Package Failed | Work Package Hash | WP failed |
| **93** | Duplicate Work Package | Work Package Hash | WP duplicate detected |
| **94** | Work Package Received | Work Package Hash | WP received |
| **95** | Authorized | Work Package Hash | WP authorized |
| **96** | Extrinsic Data Received | Work Package Hash | Extrinsic data received |
| **97** | Imports Received | Work Package Hash | Imports received |
| **98** | Sharing Work Package | Work Package Hash | WP sharing |
| **99** | Work Package Sharing Failed | Work Package Hash | WP sharing failed |
| **100** | Bundle Sent | Work Package Hash | Bundle sent |
| **101** | Refined | Work Package Hash | Refinement complete |
| **102** | Work Report Built | Work Package Hash | Work report built |
| **103** | Work Report Signature Sent | Work Package Hash | Signature sent |
| **104** | Work Report Signature Received | Work Package Hash | Signature received |
| **105** | Guarantee Built | Work Package Hash | Guarantee built |
| **106** | Sending Guarantee | Work Package Hash | Sending guarantee |
| **107** | Guarantee Send Failed | Work Package Hash | Guarantee send failed |
| **108** | Guarantee Sent | Work Package Hash | Guarantee sent |
| **109** | Guarantees Distributed | Work Package Hash | Guarantees distributed |
| **110** | Receiving Guarantee | Work Package Hash | Receiving guarantee |
| **111** | Guarantee Receive Failed | Work Package Hash | Guarantee receive failed |
| **112** | Guarantee Received | Work Package Hash | Guarantee received |
| **113** | Guarantee Discarded | Work Package Hash | Guarantee discarded |
| 114-119 | *[Gap]* | - | - |
| **120** | Sending Shard Request | Erasure Root | Shard request start |
| **121** | Receiving Shard Request | Erasure Root | Shard request received |
| **122** | Shard Request Failed | Erasure Root | Shard request failed |
| **123** | Shard Request Sent | Erasure Root | Shard request sent |
| **124** | Shard Request Received | Erasure Root | Shard request received |
| **125** | Shards Transferred | Erasure Root | Shards transferred |
| **126** | Distributing Assurance | Erasure Root | Assurance distribution start |
| **127** | Assurance Send Failed | Erasure Root | Assurance send failed |
| **128** | Assurance Sent | Erasure Root | Assurance sent |
| **129** | Assurance Distributed | Erasure Root | Assurance distributed |
| **130** | Assurance Receive Failed | Erasure Root | Assurance receive failed |
| **131** | Assurance Received | Erasure Root | Assurance received |
| **132** | Context Available | Erasure Root | Context available |
| **133** | Assurance Provided | Erasure Root | Assurance provided |
| 134-139 | *[Gap]* | - | - |
| **140** | Sending Bundle Shard Request | Erasure Root | Bundle shard request start |
| **141** | Receiving Bundle Shard Request | Erasure Root | Bundle shard request received |
| **142** | Bundle Shard Request Failed | Erasure Root | Bundle shard request failed |
| **143** | Bundle Shard Request Sent | Erasure Root | Bundle shard request sent |
| **144** | Bundle Shard Request Received | Erasure Root | Bundle shard request received |
| **145** | Bundle Shard Transferred | Erasure Root | Bundle shard transferred |
| **146** | Reconstructing Bundle | Erasure Root | Bundle reconstruction start |
| **147** | Bundle Reconstructed | Erasure Root | Bundle reconstructed |
| **148** | Sending Bundle Request | Erasure Root | Bundle request start |
| **149** | Receiving Bundle Request | Erasure Root | Bundle request received |
| **150** | Bundle Request Failed | Erasure Root | Bundle request failed |
| **151** | Bundle Request Sent | Erasure Root | Bundle request sent |
| **152** | Bundle Request Received | Erasure Root | Bundle request received |
| **153** | Bundle Transferred | Erasure Root | Bundle transferred |
| 154-159 | *[Gap]* | - | - |
| **160** | Work Package Hash Mapped | Segments Root | WP hash mapped |
| **161** | Segments Root Mapped | Segments Root | Segments root mapped |
| **162** | Sending Segment Shard Request | Segments Root | Segment shard request start |
| **163** | Receiving Segment Shard Request | Segments Root | Segment shard request received |
| **164** | Segment Shard Request Failed | Segments Root | Segment shard request failed |
| **165** | Segment Shard Request Sent | Segments Root | Segment shard request sent |
| **166** | Segment Shard Request Received | Segments Root | Segment shard request received |
| **167** | Segment Shards Transferred | Segments Root | Segment shards transferred |
| **168** | Reconstructing Segments | Segments Root | Segment reconstruction start |
| **169** | Segment Reconstruction Failed | Segments Root | Segment reconstruction failed |
| **170** | Segments Reconstructed | Segments Root | Segments reconstructed |
| **171** | Segment Verification Failed | Segments Root | Segment verification failed |
| **172** | Segments Verified | Segments Root | Segments verified |
| **173** | Sending Segment Request | Segments Root | Segment request start |
| **174** | Receiving Segment Request | Segments Root | Segment request received |
| **175** | Segment Request Failed | Segments Root | Segment request failed |
| **176** | Segment Request Sent | Segments Root | Segment request sent |
| **177** | Segment Request Received | Segments Root | Segment request received |
| **178** | Segments Transferred | Segments Root | Segments transferred |
| 179-189 | *[Gap]* | - | - |
| **190** | Preimage Announcement Failed | Preimage Hash | Preimage announcement failed |
| **191** | Preimage Announced | Preimage Hash | Preimage announced |
| **192** | Announced Preimage Forgotten | Preimage Hash | Preimage forgotten |
| **193** | Sending Preimage Request | Preimage Hash | Preimage request start |
| **194** | Receiving Preimage Request | Preimage Hash | Preimage request received |
| **195** | Preimage Request Failed | Preimage Hash | Preimage request failed |
| **196** | Preimage Request Sent | Preimage Hash | Preimage request sent |
| **197** | Preimage Request Received | Preimage Hash | Preimage request received |
| **198** | Preimage Transferred | Preimage Hash | Preimage transferred |
| **199** | Preimage Discarded | Preimage Hash | Preimage discarded |

### Trace ID Summary

**Trace ID Types:**
- **Header Hash** (Events 11-12, 40-48, 62-68): Block-related operations
- **Peer ID** (Events 20-28, 60-61): P2P networking operations
- **Epoch Index (u32)** (Events 80-84): Ticket generation and transfer
- **Work Package Hash** (Events 90-113): Work package pipeline
- **Erasure Root** (Events 120-133, 140-153): Availability and bundle recovery
- **Segments Root** (Events 160-178): Segment distribution and recovery
- **Preimage Hash** (Events 190-199): Preimage distribution
- **None** (Events 10, 13, 71): Intra-node only, no cross-node correlation

**Gaps in Event Numbering:**
- 14-19 (6 events)
- 29-39 (11 events)
- 49-59 (11 events)
- 69-70 (2 events)
- 72-79 (8 events)
- 85-89 (5 events)
- 114-119 (6 events)
- 134-139 (6 events)
- 154-159 (6 events)
- 179-189 (11 events)

**Total:** 78 gaps out of 190 possible event numbers

## Event-to-Span Mapping

### Work Package Pipeline (Cross-Node Trace)

**Trace ID**: Work Package Hash
**Nodes Involved**: Builder → Primary Guarantor → Secondary Guarantors → Auditor

```
Builder Node:
  eventID := telemetryClient.GetEventID(wpHash)
  └─> 90: WorkPackageSubmission
      → TART: Event 90 with u64 eventID
      → Jaeger: Span "work-package-submission" with traceID=wpHash

Primary Guarantor Node:
  eventID := telemetryClient.GetEventID(wpHash)  // Same wpHash → same trace
  ├─> 94: WorkPackageReceived
  │   → TART: Event 94 with same u64
  │   → Jaeger: Span "work-package-received" with same traceID
  │
  ├─> 95: Authorized(eventID, isAuthorizedCost)
  │   → TART: Event 95, child of 94
  │   → Jaeger: Span "authorized"
  │
  ├─> 97: ImportsReceived(eventID)
  │   → Jaeger: Span "imports-received"
  │
  ├─> 98: SharingWorkPackage(eventID, secondary1PeerID)
  │   → Jaeger: Span "sharing-work-package"
  │
  ├─> 100: BundleSent(eventID, secondary1PeerID)
  │   → Jaeger: Span "bundle-sent"
  │
  ├─> 101: Refined(eventID, refineCosts)
  │   → Jaeger: Span "refined"
  │
  ├─> 102: WorkReportBuilt(eventID, workReportOutline)
  │   → Jaeger: Span "work-report-built"
  │
  ├─> 105: GuaranteeBuilt(eventID, guaranteeOutline)
  │   → Jaeger: Span "guarantee-built"
  │
  └─> 109: GuaranteesDistributed(eventID)
      → Jaeger: Span "guarantees-distributed"

Secondary Guarantor 1 Node:
  eventID := telemetryClient.GetEventID(wpHash)  // Same wpHash → joins trace
  ├─> 91: WorkPackageBeingShared(eventID, primaryPeerID)
  │   → Jaeger: Span "work-package-being-shared" with same traceID
  │
  ├─> 95: Authorized(eventID, isAuthorizedCost)
  │   → Jaeger: Span "authorized"
  │
  ├─> 101: Refined(eventID, refineCosts)
  │   → Jaeger: Span "refined"
  │
  ├─> 102: WorkReportBuilt(eventID, workReportOutline)
  │   → Jaeger: Span "work-report-built"
  │
  └─> 103: WorkReportSignatureSent(eventID)
      → Jaeger: Span "work-report-signature-sent"

Secondary Guarantor 2 Node:
  eventID := telemetryClient.GetEventID(wpHash)  // Same wpHash → joins trace
  ├─> 91: WorkPackageBeingShared(eventID, primaryPeerID)
  ├─> 95: Authorized(eventID, isAuthorizedCost)
  ├─> 101: Refined(eventID, refineCosts)
  ├─> 102: WorkReportBuilt(eventID, workReportOutline)
  └─> 103: WorkReportSignatureSent(eventID)

Auditor Node:
  eventID := telemetryClient.GetEventID(wpHash)  // Same wpHash → joins trace
  ├─> 140: SendingBundleShardRequest(eventID, peerID, shardIndex)
  │   → Jaeger: Span "sending-bundle-shard-request" with same traceID
  │
  ├─> 145: BundleShardsTransferred(eventID)
  │   → Jaeger: Span "bundle-shards-transferred"
  │
  ├─> 146: ReconstructingBundle(eventID, trivialReconstruction)
  │   → Jaeger: Span "reconstructing-bundle"
  │
  └─> 147: BundleReconstructed(eventID)
      → Jaeger: Span "bundle-reconstructed"
```

**Visualization in Jaeger**: Single trace showing the complete work package journey across all nodes.

### Block Pipeline (Cross-Node Trace)

**Trace ID**: Header Hash
**Nodes Involved**: Author → Peer Nodes (Importers)

```
Author Node:
  eventID := telemetryClient.GetEventID(headerHash)
  ├─> 40: Authoring(eventID, slot, parentHash)
  │   → TART: Event 40 with u64 eventID
  │   → Jaeger: Span "authoring" with traceID=headerHash
  │
  ├─> 42: Authored(eventID, blockOutline)
  │   → TART: Event 42, child of 40
  │   → Jaeger: Span "authored"
  │
  ├─> 47: BlockExecuted(eventID, accumulatedServices)
  │   → Jaeger: Span "block-executed"
  │
  └─> 62: BlockAnnounced(peerID, connectionSide, slot, headerHash)
      → Jaeger: Span "block-announced" (broadcast)

Importer Node 1:
  eventID := telemetryClient.GetEventID(headerHash)  // Same headerHash → same trace
  ├─> 62: BlockAnnounced(peerID, connectionSide, slot, headerHash)
  │   → Jaeger: Span "block-announced" (received) with same traceID
  │
  ├─> 63: SendingBlockRequest(eventID, peerID, headerHash, direction, maxBlocks)
  │   → TART: Event 63
  │   → Jaeger: Span "sending-block-request"
  │
  ├─> 66: BlockRequestSent(eventID)
  │   → Jaeger: Span "block-request-sent"
  │
  ├─> 68: BlockTransferred(eventID, slot, blockOutline, isLast)
  │   → Jaeger: Span "block-transferred"
  │
  ├─> 43: Importing(eventID, slot, blockOutline)
  │   → TART: Event 43
  │   → Jaeger: Span "importing"
  │
  ├─> 45: BlockVerified(eventID)
  │   → Jaeger: Span "block-verified"
  │
  └─> 47: BlockExecuted(eventID, accumulatedServices)
      → Jaeger: Span "block-executed"

Importer Node 2:
  eventID := telemetryClient.GetEventID(headerHash)  // Same headerHash → same trace
  ├─> 62: BlockAnnounced(peerID, connectionSide, slot, headerHash)
  ├─> 63: SendingBlockRequest(eventID, peerID, headerHash, direction, maxBlocks)
  ├─> 66: BlockRequestSent(eventID)
  ├─> 68: BlockTransferred(eventID, slot, blockOutline, isLast)
  ├─> 43: Importing(eventID, slot, blockOutline)
  ├─> 45: BlockVerified(eventID)
  └─> 47: BlockExecuted(eventID, accumulatedServices)
```

### Availability Distribution (Cross-Node Trace)

**Trace ID**: Erasure Root
**Nodes Involved**: Guarantor → Assurers

```
Guarantor Node:
  eventID := telemetryClient.GetEventID(erasureRoot)
  └─> 120: ShardsAvailable(eventID, erasureRoot, numShards)
      → TART: Event 120 with u64 eventID
      → Jaeger: Span "shards-available" with traceID=erasureRoot

Assurer Node 1:
  eventID := telemetryClient.GetEventID(erasureRoot)  // Same erasureRoot → same trace
  ├─> 137: SendingShardRequest(eventID, peerID, erasureRoot, shardIndex)
  │   → TART: Event 137 with same u64
  │   → Jaeger: Span "sending-shard-request" with same traceID
  │
  ├─> 138: ShardTransferred(eventID, peerID, erasureRoot, shardIndex, size)
  │   → TART: Event 138, child of 137
  │   → Jaeger: Span "shard-transferred"
  │
  ├─> 139: DistributingAssurance(eventID, erasureRoot, shardIndex)
  │   → TART: Event 139, child of 138
  │   → Jaeger: Span "distributing-assurance"
  │
  └─> 141: AssuranceDistributed(eventID, erasureRoot, validatorIndices)
      → TART: Event 141, child of 139
      → Jaeger: Span "assurance-distributed"

Assurer Node 2:
  eventID := telemetryClient.GetEventID(erasureRoot)  // Same erasureRoot → same trace
  ├─> 137: SendingShardRequest(eventID, peerID, erasureRoot, shardIndex)
  │   → TART: Event 137 with same u64
  │   → Jaeger: Span "sending-shard-request" with same traceID
  │
  ├─> 138: ShardTransferred(eventID, peerID, erasureRoot, shardIndex, size)
  │   → Jaeger: Span "shard-transferred"
  │
  ├─> 139: DistributingAssurance(eventID, erasureRoot, shardIndex)
  │   → Jaeger: Span "distributing-assurance"
  │
  └─> 141: AssuranceDistributed(eventID, erasureRoot, validatorIndices)
      → Jaeger: Span "assurance-distributed"

Note: All Assurer nodes use the same erasureRoot, enabling Jaeger to visualize
parallel shard distribution across the network in a single trace.
```

## Implementation

### TelemetryClient Changes

The `GetEventID()` method now accepts an optional `interface{}` parameter and handles dual output:

```go
// telemetry/telemetry_client.go

type ServerType int

const (
    ServerTypeTART ServerType = iota
    ServerTypeJaeger
)

type TelemetryClient struct {
    addr        string
    conn        net.Conn
    serverType  ServerType
    nextEventID uint64
    eventIDMu   sync.Mutex

    // OpenTelemetry components (only used for Jaeger)
    tracer      trace.Tracer
    activeSpans map[uint64]trace.Span  // eventID -> active span
    mu          sync.RWMutex
}

// GetEventID returns an event ID for telemetry.
// - If no argument: generates sequential u64 (TART) or no-op (Jaeger - context required)
// - If argument provided:
//   * TART server: generates sequential u64 (ignores context)
//   * Jaeger server: derives u64 from hash/u64/u32 context
func (c *TelemetryClient) GetEventID(context ...interface{}) uint64 {
    if c.serverType == ServerTypeTART {
        // TART: Always use sequential Event ID
        c.eventIDMu.Lock()
        defer c.eventIDMu.Unlock()
        id := c.nextEventID
        c.nextEventID++
        return id
    }

    // Jaeger: Require context
    if len(context) == 0 {
        return 0 // No event ID without context for Jaeger
    }

    // Handle direct u64
    if eventID, ok := context[0].(uint64); ok {
        return eventID
    }

    // Convert hash/u32 to u64
    return hashToEventID(context[0])
}

func hashToEventID(identifier interface{}) uint64 {
    switch v := identifier.(type) {
    case [32]byte:
        // Use first 8 bytes of hash as u64
        return binary.BigEndian.Uint64(v[:8])
    case []byte:
        if len(v) >= 8 {
            return binary.BigEndian.Uint64(v[:8])
        }
    case uint32: // For epoch index
        return uint64(v)
    }
    return 0
}

// When sending telemetry events, the client sends to either TART or Jaeger:
func (c *TelemetryClient) sendEvent(eventType uint8, eventID uint64, data []byte) error {
    if c.serverType == ServerTypeTART {
        // Send JIP-3 event to TART Server (with sequential u64 eventID)
        if c.conn != nil {
            return c.sendJIP3Event(eventType, eventID, data)
        }
        return nil
    }

    // Send OpenTelemetry span to Jaeger (with eventID from hash/u64/u32)
    if c.tracer != nil {
        return c.sendSpanToJaeger(eventType, eventID, data)
    }

    return nil
}

// sendSpanToJaeger creates and sends an OpenTelemetry span to Jaeger
// Constructs a 16-byte trace ID from the eventID (which is derived from hash/u64/u32):
//   * First 8 bytes: the eventID (big-endian)
//   * Second 8 bytes: zeros (padding to meet Jaeger's 16-byte requirement)
func (c *TelemetryClient) sendSpanToJaeger(eventType uint8, eventID uint64, data []byte) error {
    // Construct 16-byte trace ID: eventID in first 8 bytes, zeros in second 8 bytes
    traceID := make([]byte, 16)
    binary.BigEndian.PutUint64(traceID[0:8], eventID)
    // traceID[8:16] is already zero-initialized

    // Get or create span for this event
    c.mu.Lock()
    span, exists := c.activeSpans[eventID]
    if !exists {
        // Create new span with trace ID
        spanName := eventTypeToSpanName(eventType)
        ctx := context.Background()

        // Create span context with our trace ID
        spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
            TraceID:    trace.TraceID(traceID),
            SpanID:     trace.SpanID(generateSpanID()),
            TraceFlags: trace.FlagsSampled,
        })
        ctx = trace.ContextWithSpanContext(ctx, spanCtx)

        // Start the span
        ctx, span = c.tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindInternal))
        c.activeSpans[eventID] = span
    }
    c.mu.Unlock()

    // Add event attributes
    c.addEventAttributes(span, eventType, data)

    // End span immediately (can be enhanced to track span lifecycle)
    span.End()

    // Clean up
    c.mu.Lock()
    delete(c.activeSpans, eventID)
    c.mu.Unlock()

    return nil
}

func generateSpanID() [8]byte {
    var id [8]byte
    rand.Read(id[:])
    return id
}

func eventTypeToSpanName(eventType uint8) string {
    names := map[uint8]string{
        90:  "work-package-submission",
        94:  "work-package-received",
        95:  "authorized",
        101: "refined",
        40:  "authoring",
        42:  "authored",
        43:  "importing",
        62:  "block-announced",
        120: "shards-available",
        137: "sending-shard-request",
        138: "shard-transferred",
        141: "assurance-distributed",
    }
    if name, ok := names[eventType]; ok {
        return name
    }
    return fmt.Sprintf("event-%d", eventType)
}

func (c *TelemetryClient) addEventAttributes(span trace.Span, eventType uint8, data []byte) {
    span.SetAttributes(
        attribute.String("jam.event_type", fmt.Sprintf("CE%d", eventType)),
        attribute.Int64("jam.timestamp", time.Now().UnixNano()),
    )

    // Event-specific attributes parsed from data
    if len(data) == 0 {
        return
    }

    switch eventType {
    case 94: // WorkPackageReceived
        if len(data) >= 34 {
            span.SetAttributes(
                attribute.String("jam.work_package_hash", hex.EncodeToString(data[:32])),
                attribute.Int("jam.core_index", int(binary.LittleEndian.Uint16(data[32:34]))),
            )
        }
    case 62: // BlockAnnounced
        if len(data) >= 36 {
            span.SetAttributes(
                attribute.String("jam.header_hash", hex.EncodeToString(data[:32])),
                attribute.Int64("jam.slot", int64(binary.LittleEndian.Uint32(data[32:36]))),
            )
        }
    case 101: // Refined
        if len(data) >= 8 {
            span.SetAttributes(
                attribute.Int64("jam.gas_used", int64(binary.LittleEndian.Uint64(data[:8]))),
            )
        }
    }
}
```

### Usage Patterns in Code

This section maps the telemetry event constants to their expected trace ID contexts for Jaeger integration.

#### Status Events (10-13)
```go
// Event 10: Status - Intra-node only
telemetryClient.Status(...)  // No GetEventID() call

// Event 11: Best Block Changed - Header Hash
eventID := telemetryClient.GetEventID(headerHash)
telemetryClient.BestBlockChanged(eventID, slot, headerHash)

// Event 12: Finalized Block Changed - Header Hash
eventID := telemetryClient.GetEventID(headerHash)
telemetryClient.FinalizedBlockChanged(eventID, slot, headerHash)

// Event 13: Sync Status Changed - Intra-node only
telemetryClient.SyncStatusChanged(...)  
```

#### Networking Events (20-28)
```go
// All use Peer ID as trace context
eventID := telemetryClient.GetEventID(peerID)

// Event 20-28: Connection lifecycle
telemetryClient.ConnectionRefused(eventID, peerID, reason)
telemetryClient.ConnectingIn(eventID, peerID)
telemetryClient.ConnectInFailed(eventID, peerID, error)
telemetryClient.ConnectedIn(eventID, peerID)
telemetryClient.ConnectingOut(eventID, peerID)
telemetryClient.ConnectOutFailed(eventID, peerID, error)
telemetryClient.ConnectedOut(eventID, peerID)
telemetryClient.Disconnected(eventID, peerID)
telemetryClient.PeerMisbehaved(eventID, peerID, reason)
```

#### Block Events (40-48)
```go
// All use Header Hash as trace context
eventID := telemetryClient.GetEventID(headerHash)

// Event 40-48: Block authoring and execution
telemetryClient.Authoring(slot, parentHash)
telemetryClient.AuthoringFailed(eventID, error)
telemetryClient.Authored(eventID, blockOutline)
telemetryClient.Importing(slot, headerHash)
telemetryClient.BlockVerificationFailed(eventID, error)
telemetryClient.BlockVerified(eventID)
telemetryClient.BlockExecutionFailed(eventID, error)
telemetryClient.BlockExecuted(eventID, accumulatedServices)
telemetryClient.AccumulateResultAvailable(headerHash)
```

#### Block Announcement Events (60-68, 71)
```go
// Events 60-61: Use Peer ID
eventID := telemetryClient.GetEventID(peerID)
telemetryClient.BlockAnnouncementStreamOpened(eventID, peerID)
telemetryClient.BlockAnnouncementStreamClosed(eventID, peerID)

// Events 62-68: Use Header Hash
eventID := telemetryClient.GetEventID(headerHash)
telemetryClient.BlockAnnounced(peerID, slot, headerHash)
telemetryClient.SendingBlockRequest(peerID, headerHash)
telemetryClient.ReceivingBlockRequest(peerID, headerHash)
telemetryClient.BlockRequestFailed(eventID, error)
telemetryClient.BlockRequestSent(eventID, peerID)
telemetryClient.BlockRequestReceived(eventID, peerID)
telemetryClient.BlockTransferred(eventID, peerID, size)

// Event 71: Intra-node only
telemetryClient.BlockAnnouncementMalformed(eventID, peerID)  // TODO
```

#### Safrole Events (80-84)
```go
// All use Epoch Index (u32) as trace context
eventID := telemetryClient.GetEventID(epochIndex)

// Event 80-84: Ticket generation and transfer
telemetryClient.GeneratingTickets(epochIndex)
telemetryClient.TicketGenerationFailed(eventID, error)
telemetryClient.TicketsGenerated(eventID, count)
telemetryClient.TicketTransferFailed(eventID, peerID, error)
telemetryClient.TicketTransferred(eventID, peerID, count)
```

#### Work Package Events (90-113)
```go
// All use Work Package Hash as trace context
eventID := telemetryClient.GetEventID(workPackageHash)

// Event 90-113: Work package and guarantee pipeline
telemetryClient.WorkPackageSubmission(eventID, workPackage)
telemetryClient.WorkPackageBeingShared(eventID, peerID)
telemetryClient.WorkPackageFailed(eventID, error)
telemetryClient.DuplicateWorkPackage(eventID, workPackageHash)
telemetryClient.WorkPackageReceived(eventID, peerID, coreIndex)
telemetryClient.Authorized(eventID, isAuthorizedCost)
telemetryClient.ExtrinsicDataReceived(eventID, size)
telemetryClient.ImportsReceived(eventID, count)
telemetryClient.SharingWorkPackage(eventID, peerID)
telemetryClient.WorkPackageSharingFailed(eventID, peerID, error)
telemetryClient.BundleSent(eventID, peerID)
telemetryClient.Refined(eventID, refineCosts)
telemetryClient.WorkReportBuilt(eventID, workReport)
telemetryClient.WorkReportSignatureSent(eventID, peerID)
telemetryClient.WorkReportSignatureReceived(eventID, peerID)
telemetryClient.GuaranteeBuilt(eventID, guarantee)
telemetryClient.SendingGuarantee(eventID, validatorIndex)
telemetryClient.GuaranteeSendFailed(eventID, validatorIndex, error)
telemetryClient.GuaranteeSent(eventID, validatorIndex)
telemetryClient.GuaranteesDistributed(eventID, count)
telemetryClient.ReceivingGuarantee(eventID, peerID)
telemetryClient.GuaranteeReceiveFailed(eventID, peerID, error)
telemetryClient.GuaranteeReceived(eventID, peerID, guarantee)
telemetryClient.GuaranteeDiscarded(eventID, reason)
```

#### Assurance Events (120-133)
```go
// All use Erasure Root as trace context
eventID := telemetryClient.GetEventID(erasureRoot)

// Event 120-133: Shard request and assurance distribution
telemetryClient.SendingShardRequest(peerID, erasureRoot, shardIndex)
telemetryClient.ReceivingShardRequest(peerID, erasureRoot, shardIndex)
telemetryClient.ShardRequestFailed(eventID, peerID, error)
telemetryClient.ShardRequestSent(eventID, peerID)
telemetryClient.ShardRequestReceived(eventID, peerID)
telemetryClient.ShardsTransferred(eventID, peerID, count)
telemetryClient.DistributingAssurance(erasureRoot, shardIndex)
telemetryClient.AssuranceSendFailed(eventID, validatorIndex, error)
telemetryClient.AssuranceSent(eventID, validatorIndex)
telemetryClient.AssuranceDistributed(eventID, validatorIndices)
telemetryClient.AssuranceReceiveFailed(eventID, peerID, error)
telemetryClient.AssuranceReceived(eventID, peerID)
telemetryClient.ContextAvailable(erasureRoot)
telemetryClient.AssuranceProvided(erasureRoot)
```

#### Bundle Recovery Events (140-153)
```go
// All use Erasure Root as trace context
eventID := telemetryClient.GetEventID(erasureRoot)

// Event 140-153: Bundle shard request and reconstruction
telemetryClient.SendingBundleShardRequest(peerID, erasureRoot, shardIndex)
telemetryClient.ReceivingBundleShardRequest(peerID, erasureRoot, shardIndex)
telemetryClient.BundleShardRequestFailed(eventID, peerID, error)
telemetryClient.BundleShardRequestSent(eventID, peerID)
telemetryClient.BundleShardRequestReceived(eventID, peerID)
telemetryClient.BundleShardTransferred(eventID, peerID, size)
telemetryClient.ReconstructingBundle(erasureRoot)
telemetryClient.BundleReconstructed(erasureRoot)
telemetryClient.SendingBundleRequest(peerID, erasureRoot)
telemetryClient.ReceivingBundleRequest(peerID, erasureRoot)
telemetryClient.BundleRequestFailed(eventID, peerID, error)
telemetryClient.BundleRequestSent(eventID, peerID)
telemetryClient.BundleRequestReceived(eventID, peerID)
telemetryClient.BundleTransferred(eventID, peerID, size)
```

#### Segment Events (160-178)
```go
// All use Segments Root as trace context
eventID := telemetryClient.GetEventID(segmentsRoot)

// Event 160-178: Segment mapping, shard request, and reconstruction
telemetryClient.WorkPackageHashMapped(workPackageHash, segmentsRoot)
telemetryClient.SegmentsRootMapped(segmentsRoot)
telemetryClient.SendingSegmentShardRequest(peerID, segmentsRoot, shardIndex)
telemetryClient.ReceivingSegmentShardRequest(peerID, segmentsRoot, shardIndex)
telemetryClient.SegmentShardRequestFailed(eventID, peerID, error)
telemetryClient.SegmentShardRequestSent(eventID, peerID)
telemetryClient.SegmentShardRequestReceived(eventID, peerID)
telemetryClient.SegmentShardsTransferred(eventID, peerID, count)
telemetryClient.ReconstructingSegments(segmentsRoot)
telemetryClient.SegmentReconstructionFailed(eventID, error)
telemetryClient.SegmentsReconstructed(segmentsRoot)
telemetryClient.SegmentVerificationFailed(eventID, error)
telemetryClient.SegmentsVerified(segmentsRoot)
telemetryClient.SendingSegmentRequest(peerID, segmentsRoot)
telemetryClient.ReceivingSegmentRequest(peerID, segmentsRoot)
telemetryClient.SegmentRequestFailed(eventID, peerID, error)
telemetryClient.SegmentRequestSent(eventID, peerID)
telemetryClient.SegmentRequestReceived(eventID, peerID)
telemetryClient.SegmentsTransferred(eventID, peerID, size)
```

#### Preimage Events (190-199)
```go
// All use Preimage Hash as trace context
eventID := telemetryClient.GetEventID(preimageHash)

// Event 190-199: Preimage announcement and request
telemetryClient.PreimageAnnouncementFailed(eventID, error)
telemetryClient.PreimageAnnounced(preimageHash, size)
telemetryClient.AnnouncedPreimageForgotten(preimageHash)
telemetryClient.SendingPreimageRequest(peerID, preimageHash)
telemetryClient.ReceivingPreimageRequest(peerID, preimageHash)
telemetryClient.PreimageRequestFailed(eventID, peerID, error)
telemetryClient.PreimageRequestSent(eventID, peerID)
telemetryClient.PreimageRequestReceived(eventID, peerID)
telemetryClient.PreimageTransferred(eventID, peerID, size)
telemetryClient.PreimageDiscarded(preimageHash)
```

## Initialization

The TelemetryClient needs to be initialized with an OpenTelemetry tracer:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/jaeger"
    "go.opentelemetry.io/otel/sdk/trace"
)

func NewTelemetryClient(addr string, serverType ServerType) (*TelemetryClient, error) {
    client := &TelemetryClient{
        addr:       addr,
        serverType: serverType,
    }

    if serverType == ServerTypeTART {
        // Connect to TART Server
        conn, err := net.Dial("tcp", addr)
        if err != nil {
            return nil, err
        }
        client.conn = conn
    } else {
        // Create Jaeger exporter
        exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(addr)))
        if err != nil {
            return nil, err
        }

        // Create trace provider
        tp := trace.NewTracerProvider(
            trace.WithBatcher(exporter),
            trace.WithSampler(trace.AlwaysSample()),
        )
        otel.SetTracerProvider(tp)

        // Create tracer
        client.tracer = tp.Tracer("jam-telemetry")
        client.activeSpans = make(map[uint64]trace.Span)
    }

    return client, nil
}
```

## Deployment

### Option 1: Standalone Adapter (Recommended)

```yaml
# docker-compose.yaml
services:
  jam-node-1:
    image: jam-node:latest
    command: --telemetry adapter:9615

  jam-node-2:
    image: jam-node:latest
    command: --telemetry adapter:9615

  telemetry-adapter:
    image: jam-telemetry-adapter:latest
    ports:
      - "9615:9615"  # JIP-3 listener
    environment:
      - JAEGER_ENDPOINT=http://jaeger:14268/api/traces

  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "16686:16686"  # UI
      - "14268:14268"  # HTTP collector
    environment:
      - COLLECTOR_OTLP_ENABLED=true
```

### Option 2: Embedded Adapter

Add Jaeger exporter directly to JAM node (requires more code changes).

## Benefits

1. **Cross-Node Visibility**: See complete work package journey across builder, guarantors, and auditors
2. **Performance Analysis**: Identify bottlenecks in multi-node workflows
3. **Debugging**: Trace failures across node boundaries
4. **Latency Analysis**: Measure time between nodes (e.g., submission to guarantee)
5. **Correlation**: Link block authoring on one node to importing on others

## Example Traces in Jaeger

### Work Package Trace
```
Trace ID: 0x1a2b3c4d... (Work Package Hash)
Duration: 850ms

Spans:
├─ work-package-submission (Builder, 0-10ms)
├─ work-package-received (Guarantor-1, 15-20ms)
├─ authorized (Guarantor-1, 20-120ms)
├─ imports-received (Guarantor-1, 120-180ms)
├─ sharing-work-package (Guarantor-1 → Guarantor-2, 180-200ms)
│  ├─ work-package-being-shared (Guarantor-2, 190-195ms)
│  ├─ authorized (Guarantor-2, 195-280ms)
│  ├─ refined (Guarantor-2, 280-450ms)
│  └─ work-report-signature-sent (Guarantor-2, 450-455ms)
├─ refined (Guarantor-1, 200-500ms)
├─ work-report-built (Guarantor-1, 500-550ms)
├─ guarantee-built (Guarantor-1, 550-600ms)
└─ guarantees-distributed (Guarantor-1, 600-850ms)
```

### Block Trace
```
Trace ID: 0x9f8e7d6c... (Header Hash)
Duration: 1200ms

Spans:
├─ authoring (Validator-1, 0-800ms)
│  ├─ authored (Validator-1, 500-600ms)
│  └─ block-executed (Validator-1, 600-800ms)
├─ block-announced (Validator-1 → Network, 800-805ms)
├─ importing (Validator-2, 810-1100ms)
│  ├─ block-verified (Validator-2, 850-900ms)
│  └─ block-executed (Validator-2, 900-1100ms)
└─ importing (Validator-3, 820-1200ms)
   ├─ block-verified (Validator-3, 870-920ms)
   └─ block-executed (Validator-3, 920-1200ms)
```

## Configuration

```yaml
# adapter-config.yaml
adapter:
  listen_address: "0.0.0.0:9615"
  buffer_size: 10000

jaeger:
  endpoint: "http://jaeger:14268/api/traces"
  service_name: "jam-network"

opentelemetry:
  batch_timeout: "5s"
  batch_size: 512
  max_export_batch_size: 512

trace_correlation:
  # Map Event ID prefixes to trace types
  work_package: true
  block: true
  availability: true
  preimage: true
```

## Next Steps

1. Implement adapter core (JIP-3 TCP listener)
2. Build event-to-span conversion logic
3. Add trace correlation by identifier type
4. Test with multi-node scenarios
5. Deploy and validate traces in Jaeger UI
