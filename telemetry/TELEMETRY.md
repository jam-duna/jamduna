# JIP-3: Telemetry

[JIP-3 Sheet (10/28/25)](https://docs.google.com/spreadsheets/d/1_4el3j2W5FaGP8MaLRb7M5PHYwam57d06FByuwUKoY0/edit?gid=591416875#gid=591416875)

Specification for JAM node telemetry allowing integration into JAM Tart (Testing, Analytics and
Research Telemetry).

## Connection to telemetry server

Nodes shall provide a CLI option `--telemetry HOST:PORT`. Nodes, with said CLI option given, shall
make a TCP/IP connection to said endpoint.

## Message encoding

Data sent over the connection to the telemetry server shall consist of variable-length messages.
Each message is sent in two parts. First, the size (in bytes) of the message content is sent,
encoded as a little-endian 32-bit unsigned integer. Second, the message content itself is sent.
Note that this message encoding matches JAMNP-S.

Message content shall be encoded as per regular JAM serialization. That is, a message with fields
$a$, $b$, $c$, etc shall be encoded as $\mathcal{E}(a, b, c, ...)$. Fields of type `u8`, `u16`,
`u32`, and `u64` shall be encoded using $\mathcal{E}_1$, $\mathcal{E}_2$, $\mathcal{E}_4$, and
$\mathcal{E}_8$ respectively (ie they should use fixed-length encoding).

## Notation

In the type/message definitions below:

- Fields are identified by their type, eg `u32`, with their meaning given in parentheses when not
  self-evident.
- `[T; N]` means a fixed-length sequence of $N$ elements each of type $T$.
- `A ++ B ++ ...` means a tuple $(A, B, ...)$.
- `len++` preceding a sequence type indicates that the sequence should be explicitly prefixed by
  its length when encoding. The length should be encoded using variable-length general natural
  number serialization.

## Types

The following types are defined:

    bool = 0 (False) OR 1 (True)
    Option<T> = 0 OR (1 ++ T)
    String<N> = len++[u8] (len <= N, byte sequence must be valid UTF-8)
    Reason = String<128> (Freeform reason for eg failure of an operation, may be empty if unknown)

    Timestamp = u64 (Microseconds since the beginning of the Jam "Common Era")
    Event ID = u64

    JAM Parameters = Exactly as returned by the fetch host call defined in the GP

    Peer ID = [u8; 32] (Ed25519 public key)
    Peer Address = [u8; 16] ++ u16 (IPv6 address plus port)
    Connection Side = 0 (Local) OR 1 (Remote)

    Slot = u32
    Epoch Index = u32 (Slot / E)
    Validator Index = u16
    Core Index = u16
    Service ID = u32
    Shard Index = u16

    Hash = [u8; 32]
    Header Hash = Hash
    Work-Package Hash = Hash
    Work-Report Hash = Hash
    Erasure-Root = Hash
    Segments-Root = Hash
    Ed25519 Signature = [u8; 64]

    Block Outline =
        u32 (Size in bytes) ++
        Header Hash ++
        u32 (Number of tickets) ++
        u32 (Number of preimages) ++
        u32 (Total size of preimages in bytes) ++
        u32 (Number of guarantees) ++
        u32 (Number of assurances) ++
        u32 (Number of dispute verdicts)

    Gas = u64
    Exec Cost =
        Gas (Gas used) ++
        u64 (Elapsed wall-clock time in nanoseconds)
    Is-Authorized Cost =
        Exec Cost (Total) ++
        u64 (Time taken to load and compile the code, in nanoseconds) ++
        Exec Cost (Host calls)
    Refine Cost =
        Exec Cost (Total) ++
        u64 (Time taken to load and compile the code, in nanoseconds) ++
        Exec Cost (historical_lookup calls) ++
        Exec Cost (machine/expunge calls) ++
        Exec Cost (peek/poke/pages calls) ++
        Exec Cost (invoke calls) ++
        Exec Cost (Other host calls)
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

    Root Identifier = Segments-Root OR Work-Package Hash
    Import Spec =
        Root Identifier ++
        u16 (Export index, plus 2^15 if Root Identifier is a Work-Package Hash)
    Import Segment ID = u16 (Index in overall list of work-package imports, or for a proof page,
        2^15 plus index of a proven page)
    Work-Item Outline =
        Service ID ++
        u32 (Payload size) ++
        Gas (Refine gas limit) ++
        Gas (Accumulate gas limit) ++
        u32 (Sum of extrinsic lengths) ++
        len++[Import Spec] ++
        u16 (Number of exported segments)
    Work-Package Outline =
        u32 (Work-package size in bytes, excluding extrinsic data) ++
        Work-Package Hash ++
        Header Hash (Anchor) ++
        Slot (Lookup anchor slot) ++
        len++[Work-Package Hash] (Prerequisites) ++
        len++[Work-Item Outline]

    Work-Report Outline =
        Work-Report Hash ++
        u32 (Bundle size in bytes) ++
        Erasure-Root ++
        Segments-Root

    Guarantee Outline =
        Work-Report Hash ++
        Slot ++
        len++[Validator Index] (Guarantors)
    Guarantee Discard Reason =
        0 (Work-package reported on-chain) OR
        1 (Replaced by better guarantee) OR
        2 (Cannot be reported on-chain) OR
        3 (Too many guarantees) OR
        4 (Other)
        (Single byte)

    Announced Preimage Forget Reason =
        0 (Provided on-chain) OR
        1 (Not requested on-chain) OR
        2 (Failed to acquire preimage) OR
        3 (Too many announced preimages) OR
        4 (Bad length) OR
        5 (Other)
        (Single byte)
    Preimage Discard Reason =
        0 (Provided on-chain) OR
        1 (Not requested on-chain) OR
        2 (Too many preimages) OR
        3 (Other)
        (Single byte)

## Node information message

The first message sent on each connection to the telemetry server should contain information about
the connecting node:

    0 (Single byte, telemetry protocol version)
    JAM Parameters
    Header Hash (Genesis header hash)
    Peer ID
    Peer Address
    u32 (Node flags)
    String<32> (Name of node implementation, eg "PolkaJam")
    String<32> (Version of node implementation, eg "1.0")
    String<16> (Gray Paper version implemented by the node, eg "0.7.1")
    String<512> (Freeform note with additional information about the node)

**Implementation location:** `node/telemetry.go` - on initial connection establishment

The node flags field should be treated as a bitmask. The following bits are defined:

- Bit 0 (LSB): 1 means the node uses a PVM recompiler, 0 means the node uses a PVM interpreter.

All other bits should be set to 0.

## Event messages

Following the initial node information message, a message should be sent every time one of the
events defined below occurs.

For ease of implementation, "sent" events (such as "bundle sent") may be emitted once all of the
data has been queued at the QUIC level; it is not necessary to wait for the data to actually be
sent over the network or acknowledged by the peer.

### Universal fields

All event messages begin with a `Timestamp` field indicating the time of the event, followed by a
single-byte discriminator identifying the event type. For brevity, the timestamp and discriminator
are omitted in the event definitions below.

The discriminator value for each event type is given in the relevant section heading. For example,
the "connection refused" event has discriminator 20.

### Event IDs

Each event sent over a connection is implicitly given an ID:

- The first event is given ID 0.
- The event immediately following an event E is given the ID of event E plus N, where N is the
  number of dropped events if E is a "dropped" event, or 1 otherwise.

Event IDs are used to link related events. For example, the "connected out" event contains the ID
of the corresponding "connecting out" event.

## Meta events

### 0: Dropped

If, due to for example limited buffer space, a node needs to drop a contiguous series of events
before they can be transmitted over the connection to the telemetry server, the dropped events
should be replaced with a single "dropped" event:

    Timestamp (Timestamp of the last dropped event)
    u64 (Number of dropped events)

**Implementation location:** `node/telemetry.go` - in telemetry event buffer management

A dropped event may also be emitted as the first event on a connection, if the events emitted by
the node will not start with ID 0. In this case the number of dropped events should be set to the
ID of the following event and the timestamps should be filled with plausible values. A node may
wish to do this for example if it loses its connection to the telemetry server and then reconnects,
to avoid having to renumber events internally. Note that this is not intended to require any
special handling on the server.

Dropped events should not be common and except in the case just described are expected to only be
produced if the node or network become overloaded.

Note that each dropped event message contains _two_ `Timestamp` fields: the universal `Timestamp`
field included in all event messages, which should be taken from the _first_ dropped event, and the
`Timestamp` field defined above, which should be taken from the _last_ dropped event.

## Status events

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

**Implementation location:** `node/node.go` - periodic status ticker (every 2s)

#### Call sites -- ✅ shawn audited
- [node/node.go](../node/node.go#L1008) — `(*Node).runStatusTelemetry`

### 11: Best block changed

Emitted when the node's best block changes.

    Slot (New best slot)
    Header Hash (New best header hash)

**Implementation location:** `statedb/statedb.go` - in `UpdateBestBlock()` or when new blocks are accepted

#### Call sites -- ✅ sourabh audited
- [node/node.go](../node/node.go#L1571) — `(*NodeContent).addStateDB`
- [node/node.go](../node/node.go#L1590) — `(*NodeContent).addStateDB`

### 12: Finalized block changed

Emitted when the latest finalized block (from the node's perspective) changes.

    Slot (New finalized slot)
    Header Hash (New finalized header hash)

**Implementation location:** `node/node_request.go` - after block finalization in `runReceiveBlock`

#### Call sites -- ✅ shawn audited
- [node/node_request.go](../node/node_request.go#L400) — `(*Node).runReceiveBlock`

### 13: Sync status changed

Emitted when the node's sync status changes. This status is subjective, indicating whether or not
the node believes it is sufficiently synced with the network to be able to perform all of the
duties of a validator node (authoring, guaranteeing, assuring, auditing, and so on).

    bool (Does the node believe it is sufficiently in sync with the network?)

**Implementation location:** `node/sync.go` - when sync status transitions

#### Call sites -- ✅ sourabh audited
- [node/node.go](../node/node.go#L360) — `(*Node).SetIsSync`

## Networking events

These events concern JAMNP-S connections.

**Implementation locations for events 20-28:**
- Connection events (20-27): `node/network.go` - in connection handlers
- Peer misbehavior (28): Throughout networking code where protocol violations are detected

### 20: Connection refused 🔴 POST-MIGRATION

Emitted when a connection attempt from a peer is refused.

    Peer Address

> I think this one is for the blacklist. we currently don't have this kind of implementation


### 21: Connecting in

Emitted when a connection attempt from a peer is accepted. This event should be emitted as soon as
possible. In particular it should be emitted _before_ the connection handshake completes.

    Peer Address

#### Call sites -- ✅ shawn audited
- [node/node.go](../node/node.go#L1687) — `(*Node).handleConnection`

### 22: Connect in failed

Emitted when an incoming connection attempt fails.

    Event ID (ID of the corresponding "connecting in" event)
    Reason

#### Call sites -- ✅ shawn audited
- [node/node.go](../node/node.go#L1701) — `(*Node).handleConnection`

### 23: Connected in

Emitted when an incoming connection attempt succeeds.

    Event ID (ID of the corresponding "connecting in" event)
    Peer ID

#### Call sites -- ✅ shawn audited
- [node/node.go](../node/node.go#L1740) - `(*Node).handleConnection`

### 24: Connecting out

Emitted when an outgoing connection attempt is initiated.

    Peer ID
    Peer Address

#### Call sites -- ✅ shawn audited
- [node/peer.go](../node/peer.go#L135) — `(*Peer).openStream`

### 25: Connect out failed

Emitted when an outgoing connection attempt fails.

    Event ID (ID of the corresponding "connecting out" event)
    Reason

#### Call sites -- ✅ shawn audited
- [node/peer.go](../node/peer.go#L156) — `(*Peer).openStream`

### 26: Connected out

Emitted when an outgoing connection attempt succeeds.

    Event ID (ID of the corresponding "connecting out" event)

#### Call sites -- ✅ shawn audited
- [node/peer.go](../node/peer.go#L165) — `(*Peer).openStream`

### 27: Disconnected

Emitted when a connection to a peer is broken.

    Peer ID
    Option<Connection Side> (Terminator of the connection, may be omitted in case of eg a timeout)
    Reason

#### Call sites -- ✅ shawn audited
- [node/node.go](../node/node.go#L1656) — `(*Node).handleConnection`

### 28: Peer misbehaved 🔴 POST-MIGRATION
Emitted when a peer misbehaves. Misbehaviour is any behaviour which is objectively not compliant
with the network protocol or the GP. This includes for example sending a malformed message or an
invalid signature. This does _not_ include, for example, timing out (timeouts are subjective) or
prematurely closing a stream (this is permitted by the network protocol).

    Peer ID
    Reason

#### Call sites -- 
- [node/peerUP0_block.go](../node/peerUP0_block.go#L397) — `(*Node).runBlockAnnouncement`

> we need different treatment for this one. we probably should discuss some error expression with Emett

## Block authoring/importing events

These events concern the block authoring and importing pipelines. Note that some events are common
to both authoring and importing, eg "block executed".

**Implementation locations for events 40-47:**
- Authoring events (40-42, 47): `node/node.go` - in `AuthorBlock()` and block building logic
- Importing events (43-47): `node/node.go` - in `ImportBlock()` and block verification logic
- Execution events (46-47): `statedb/statedb.go` - in accumulation and state transition code

### 40: Authoring

Emitted when authoring of a new block begins.

    Slot
    Header Hash (Of the parent block)

#### Call sites -- ✅ sourabh audited 
- [statedb/statedb.go](../statedb/statedb.go#L766) — `(*StateDB).ProcessState`

### 41: Authoring failed

Emitted if block authoring fails for some reason.

    Event ID (ID of the corresponding "authoring" event)
    Reason

#### Call sites -- ✅ sourabh audited
- [statedb/statedb.go](../statedb/statedb.go#L784) — `(*StateDB).ProcessState`

### 42: Authored

Emitted when a new block has been authored. This should be emitted as soon the contents of the
block have been determined, ideally before accumulation is performed and the new state root is
computed (which is included only in the following block).

    Event ID (ID of the corresponding "authoring" event)
    Block Outline

#### Call sites -- ✅ sourabh audited
- [statedb/statedb.go](../statedb/statedb.go#L818) — `(*StateDB).ProcessState`

### 43: Importing

Emitted when importing of a block begins. This should not be emitted by the block author; the
author should emit the "authoring" event instead.

    Slot
    Block Outline

NOTE: this should have an EventID

#### Call sites -- ✅ sourabh audited
- [node/node.go](../node/node.go#L2161) — `(*Node).ApplyBlock`

### 44: Block verification failed

Emitted if verification of a block being imported fails for some reason. This includes if the block
is determined to be invalid, ie it does not satisfy all the validity conditions listed in the GP.
In this case, a "peer misbehaved" event should also be emitted for the peer which sent the block.

This event should never be emitted by the block author (authors should emit the "authoring failed"
event instead).

    Event ID (ID of the corresponding "importing" event)
    Reason

#### Call sites -- ✅ shawn audited
- [statedb/applystatetransition.go](../statedb/applystatetransition.go#L204) — `ApplyStateTransitionFromBlock`
- this one contains reason. need error defined
- we can have a 2 stage block verification. but currently we only have one in `s.VerifyBlockHeader`


### 45: Block verified -- 🔴 POST-MIGRATION

Emitted once a block being imported has been verified. That is, the block satisfies all the
validity conditions listed in the GP. This should be emitted as soon as this has been determined,
ideally before accumulation is performed and the new state root is computed. This should not be
emitted by the block author (the author should emit the "authored" event instead).

    Event ID (ID of the corresponding "importing" event)

#### Call sites 
- [statedb/applystatetransition.go](../statedb/applystatetransition.go#L209) — `ApplyStateTransitionFromBlock`

### 46: Block execution failed   -- 🔴 POST-MIGRATION

Emitted if execution of a block fails after authoring/verification. This can happen if, for
example, there is a collision amongst created service IDs during accumulation.

    Event ID (ID of the corresponding "authoring" or "importing" event)
    Reason

#### Call sites 
- [node/node.go](../node/node.go#L2193) — `(*Node).ApplyBlock`
- [statedb/statedb.go](../statedb/statedb.go#L856) — `(*StateDB).ProcessState`

### 47: Block executed 🔴 POST-MIGRATION

Emitted following successful execution of a block. This should be emitted by both the block author
and importers.

    Event ID (ID of the corresponding "authoring" or "importing" event)
    len++[Service ID ++ Accumulate Cost] (Accumulated services and the cost of their accumulate calls)

Each service should be listed at most once in the accumulated services list. The length of the
accumulated services list should not exceed 500. If more than 500 services are accumulated in a
block, the costs of the services with lowest total gas usage should be combined and reported with
service ID 0xffffffff (note that this is otherwise not a valid service ID). Ties should be broken
by combining services with greater IDs.

#### Call sites 
- [statedb/applystatetransition.go](../statedb/applystatetransition.go#L438) — `ApplyStateTransitionFromBlock`

## Block distribution events

These events concern announcement and transfer of blocks between peers.

**Implementation locations for events 60-68:**
- Block announcement streams (60-62): `node/network.go` - in UP 0 stream handlers
- Block requests (63-68): `node/network.go` - in CE 128 request/response handlers

### 60: Block announcement stream opened

Emitted when a block announcement stream (UP 0) is opened.

    Peer ID
    Connection Side (The side that opened the stream)

#### Call sites -- ✅ sourabh audited
- [node/peerUP0_block.go](../node/peerUP0_block.go#L200) — `(*Peer).GetOrInitBlockAnnouncementStream`
- [node/peerUP0_block.go](../node/peerUP0_block.go#L346) — `(*Node).onBlockAnnouncement`

### 61: Block announcement stream closed

Emitted when a block announcement stream (UP 0) is closed. This need not be emitted if the stream
is closed due to disconnection.

    Peer ID
    Connection Side (The side that closed the stream)
    Reason

#### Call sites --  ✅ sourabh audited
- [node/peerUP0_block.go](../node/peerUP0_block.go#L371) — `(*Node).runBlockAnnouncement`

### 62: Block announced

Emitted when a block announcement is sent to or received from a peer (UP 0).

    Peer ID
    Connection Side (Announcer)
    Slot
    Header Hash

#### Call sites -- ✅ sourabh audited 
- [node/peerUP0_block.go](../node/peerUP0_block.go#L422) — `(*Node).runBlockAnnouncement` (receiving)
- [node/node.go](../node/node.go#L1913) — broadcast loop (sending)

### 63: Sending block request

Emitted when a node begins sending a block request to a peer (CE 128).

    Peer ID (Recipient)
    Header Hash
    0 (Ascending exclusive) OR 1 (Descending inclusive) (Direction, single byte)
    u32 (Maximum number of blocks)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L95) — `(*Peer).SendBlockRequest`

### 64: Receiving block request

Emitted by the recipient when a node begins sending a block request (CE 128).

    Peer ID (Sender)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L165) — `(*NodeContent).onBlockRequest`

### 65: Block request failed

Emitted when a block request (CE 128) fails.

    Event ID (ID of the corresponding "sending block request" or "receiving block request" event)
    Reason

#### Call sites -- ✅ sourabh audited 
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L122) — `(*Peer).SendBlockRequest`
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L132) — `(*Peer).SendBlockRequest`
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L174) — `(*NodeContent).onBlockRequest`
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L191) — `(*NodeContent).onBlockRequest`
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L199) — `(*NodeContent).onBlockRequest`
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L234) — `(*NodeContent).onBlockRequest`

### 66: Block request sent

Emitted once a block request has been sent to a peer (CE 128). This should be emitted after the
intial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending block request" event)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L114) — `(*Peer).SendBlockRequest`

### 67: Block request received

Emitted once a block request has been received from a peer (CE 128).

    Event ID (ID of the corresponding "receiving block request" event)
    Header Hash
    0 (Ascending exclusive) OR 1 (Descending inclusive) (Direction, single byte)
    u32 (Maximum number of blocks)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L181) — `(*NodeContent).onBlockRequest`

### 68: Block transferred

Emitted when a block has been fully sent to or received from a peer (CE 128).

In the case of a received block, this event may be emitted before any checks are performed. If the
block is found to be invalid or to not match the request, a "peer misbehaved" event should be
emitted; emitting a "block request failed" event is optional.

    Event ID (ID of the corresponding "sending block request" or "receiving block request" event)
    Slot
    Block Outline
    bool (Last block for the request?)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L152) — `(*Peer).SendBlockRequest`
- [node/peerCE128_blockrequest.go](../node/peerCE128_blockrequest.go#L254) — `(*NodeContent).onBlockRequest`

## Safrole ticket events

These events concern generation and distribution of tickets for the Safrole lottery.

**Implementation locations for events 80-84:**
- Ticket generation (80-82): `statedb/safrole.go` - in VRF ticket generation logic
- Ticket transfer (83-84): `node/network.go` - in CE 131/132 handlers

### 80: Generating tickets

Emitted when generation of a new set of Safrole tickets begins.

    Epoch Index (The epoch the tickets are to be used in)

#### Call sites -- ✅ sourabh audited 
- [node/node_ticket.go](../node/node_ticket.go#L74) — `(*Node).GenerateTickets`

### 81: Ticket generation failed

Emitted if Safrole ticket generation fails.

    Event ID (ID of the corresponding "generating tickets" event)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/node_ticket.go](../node/node_ticket.go#L80) — `(*Node).GenerateTickets`

### 82: Tickets generated

Emitted once a set of Safrole tickets has been generated.

    Event ID (ID of the corresponding "generating tickets" event) 
    len++[[u8; 32]] (Ticket VRF outputs, index is attempt number)

#### Call sites -- ✅ sourabh audited
- [node/node_ticket.go](../node/node_ticket.go#L94) — `(*Node).GenerateTickets`

### 83: Ticket transfer failed

Emitted when a Safrole ticket send or receive fails (CE 131/132).

    Peer ID
    Connection Side (Sender)
    bool (Was CE 132 used?)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/peerCE131_ticketdistribution.go](../node/peerCE131_ticketdistribution.go#L130) — `(*Peer).SendTicketDistribution`
- [node/peerCE131_ticketdistribution.go](../node/peerCE131_ticketdistribution.go#L139) — `(*Peer).SendTicketDistribution`
- [node/peerCE131_ticketdistribution.go](../node/peerCE131_ticketdistribution.go#L158) — `(*Node).onTicketDistribution`

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

#### Call sites -- ✅ sourabh audited
- [node/peerCE131_ticketdistribution.go](../node/peerCE131_ticketdistribution.go#L123) — `(*Peer).SendTicketDistribution`
- [node/peerCE131_ticketdistribution.go](../node/peerCE131_ticketdistribution.go#L175) — `(*Node).onTicketDistribution`

## Guaranteeing events

These events concern the guaranteeing pipeline and guarantee pool.

**Implementation locations for events 90-113:**
- Work-package submission (90-109): `node/node_guarantor.go` - in guarantor pipeline
- Work-package sharing (91-103): `node/node_guarantor.go` - in primary/secondary guarantor logic
- Guarantee distribution (106-113): `node/network.go` - in CE 135 handlers and guarantee pool management
- Authorization/Refine/Build (95, 101-102): `node/node_guarantor.go` - in PVM execution callbacks

### 90: Work-package submission

Emitted when a builder opens a stream to submit a work-package (CE 133/146). This should be emitted
as soon as the stream is opened, before the work-package is read from the stream.

    Peer ID (Builder)
    bool (Using CE 146?)

#### Call sites -- ✅ shawn audited
- [node/peerCE133_workpackagesubmission.go](../node/peerCE133_workpackagesubmission.go#L101) — `(*Peer).SendWorkPackageSubmission`

### 91: Work-package being shared --  ✅ shawn audited

Emitted by the secondary guarantor when a work-package sharing stream is opened (CE 134). This
should be emitted as soon as the stream is opened, before any messages are read from the stream.

    Peer ID (Primary guarantor)

### 92: Work-package failed

Emitted if receiving a work-package from a builder or another guarantor fails, or processing of a
received work-package fails. This may be emitted at any point in the guarantor pipeline.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    Reason

#### Call sites --  ✅ shawn audited
- [node/peerCE133_workpackagesubmission.go](../node/peerCE133_workpackagesubmission.go#L109) — `(*Peer).SendWorkPackageSubmission`
- [node/peerCE133_workpackagesubmission.go](../node/peerCE133_workpackagesubmission.go#L122) — `(*Peer).SendWorkPackageSubmission`
- [node/peerCE133_workpackagesubmission.go](../node/peerCE133_workpackagesubmission.go#L139) — `(*Peer).SendWorkPackageSubmission`
- [node/peerCE133_workpackagesubmission.go](../node/peerCE133_workpackagesubmission.go#L164) — `(*Node).onWorkPackageSubmission`
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L266) — `(*Peer).ShareWorkPackage`
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L278) — `(*Peer).ShareWorkPackage`
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L287) — `(*Peer).ShareWorkPackage`
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L312) — `(*Peer).ShareWorkPackage`
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L322) — `(*Peer).ShareWorkPackage`
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L332) — `(*Peer).ShareWorkPackage`
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L375) — `(*Node).onWorkPackageShare`
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L418) — `(*Node).onWorkPackageShare`
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L498) — `(*Node).onWorkPackageShare`

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

#### Call sites --  ✅ shawn audited
- [node/peerCE133_workpackagesubmission.go](../node/peerCE133_workpackagesubmission.go#L230) — `(*Node).onWorkPackageSubmission`

### 94: Work-package received

Emitted once a work-package has been received from a builder or a primary guarantor (CE 133/134).
This should be emitted _before_ authorization is checked, and ideally before the extrinsic data and
imports are received/fetched. 

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    Core Index
    Work-Package Outline

#### Call sites -- ✅ shawn audited
- [node/peerCE133_workpackagesubmission.go](../node/peerCE133_workpackagesubmission.go#L260) — `(*Node).onWorkPackageSubmission`

### 95: Authorized ✅ shawn audited

Emitted once basic validity checks have been performed on a received work-package, including the
authorization check. This should be emitted by both primary (received the work-package from a
builder) and secondary (received the work-package from a primary) guarantors.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    Is-Authorized Cost

#### Call sites -- ✅ sourabh audited 
- [node/node_guarantee.go](../node/node_guarantee.go#L283) — `(*Node).processWPQueueItem`

### 96: Extrinsic data received

Emitted once the extrinsic data for a work-package has been received from a builder or a primary
guarantor (CE 133/134) and verified as consistent with the extrinsic hashes in the work-package.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE133_workpackagesubmission.go](../node/peerCE133_workpackagesubmission.go#L222) — `(*Node).onWorkPackageSubmission`

### 97: Imports received

Emitted once all the imports for a work-package have been fetched/received and verified.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)

#### Call sites -- ✅ sourabh audited
- [node/node_guarantee.go](../node/node_guarantee.go#L243) — `(*Node).processWPQueueItem`

### 98: Sharing work-package

Emitted by the primary guarantor when a work-package sharing stream is opened (CE 134).

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Secondary guarantor)

#### Call sites --  ✅ sourabh audited 
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L258) — `(*Peer).ShareWorkPackage`

### 99: Work-package sharing failed

Emitted if sharing a work-package with another guarantor fails (CE 134). Possible failures include
failure to send the bundle, failure to receive a work-report signature, or receipt of an invalid
work-report signature (in this case, a "peer misbehaved" event should also be emitted for the
secondary guarantor). This event should only be emitted by the primary guarantor; the secondary
guarantor should emit the "work-package failed" event on failure.

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Secondary guarantor)
    Reason

#### Call sites -- ✅ sourabh audited 
- [node/node_guarantee.go](../node/node_guarantee.go#L314) — `(*Node).processWPQueueItem`

### 100: Bundle sent

Emitted by the primary guarantor once a work-package bundle has been sent to a secondary guarantor
(CE 134).

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Secondary guarantor)

#### Call sites -- ✅ shawn audited
- [node/node_guarantee.go](../node/node_guarantee.go#L319) — `(*Node).processWPQueueItem`

### 101: Refined

Emitted once a work-package has been refined locally. This should be emitted by both primary and
secondary guarantors.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    len++[Refine Cost] (Cost of refine call for each work item)

#### Call sites -- ✅ sourabh audited (moved call site into executeWorkPackageBundle)
- [node/node_da] -- TODO

### 102: Work-report built

Emitted once a work-report has been built for a work-package. This should be emitted by both
primary and secondary guarantors.

    Event ID (ID of the corresponding "work-package submission" or "work-package being shared" event)
    Work-Report Outline

#### Call sites -- ✅ sourabh audited (moved call site into executeWorkPackageBundle)
- [node/node_da.go](../node/node_da.go#L621) — `(*NodeContent).executeWorkPackageBundle`

### 103: Work-report signature sent

Emitted once a work-report signature for a shared work-package has been sent to the primary
guarantor (CE 134). This is the final event in the guaranteeing pipeline for secondary guarantors.

    Event ID (ID of the corresponding "work-package being shared" event)

#### Call sites -- ✅ sourabh audited (there was a duplicate telemetry function and duplicate calls)
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L545) — `(*Node).onWorkPackageShare`

### 104: Work-report signature received

Emitted by the primary guarantor once a valid work-report signature has been received from a
secondary guarantor (CE 134). If an invalid work-report signature is received, a "work-package
sharing failed" event should be emitted instead, as well as a "peer misbehaved" event.

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Secondary guarantor)

#### Call sites -- ✅ sourabh audited
- [node/peerCE134_workpackageshare.go](../node/peerCE134_workpackageshare.go#L339) — `(*Peer).ShareWorkPackage`

### 105: Guarantee built

Emitted by the primary guarantor once a work-report guarantee has been built. If a secondary
guarantor is slow to send their signature, this event may be emitted twice: once for the guarantee
with just two signatures, and again for the guarantee with all three signatures.

    Event ID (ID of the corresponding "work-package submission" event)
    Guarantee Outline

#### Call sites -- ✅ sourabh audited
- [node/node_guarantee.go](../node/node_guarantee.go#L380) — `(*Node).processWPQueueItem`

### 106: Sending guarantee

Emitted when a guarantor begins sending a work-report guarantee to another validator, for potential
inclusion in a block (CE 135). This should reference the "guarantee built" event corresponding to
the guarantee that is being sent.

    Event ID (ID of the corresponding "guarantee built" event)
    Peer ID (Recipient)

#### Call sites -- ✅ sourabh audited
- [node/peerCE135_workreportdistribution.go](../node/peerCE135_workreportdistribution.go#L131) — `(*Peer).SendWorkReportDistribution`

### 107: Guarantee send failed

Emitted if sending a work-report guarantee fails (CE 135).

    Event ID (ID of the corresponding "sending guarantee" event)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/peerCE135_workreportdistribution.go](../node/peerCE135_workreportdistribution.go#L138) — `(*Peer).SendWorkReportDistribution`
- [node/peerCE135_workreportdistribution.go](../node/peerCE135_workreportdistribution.go#L154) — `(*Peer).SendWorkReportDistribution`
- [node/peerCE135_workreportdistribution.go](../node/peerCE135_workreportdistribution.go#L164) — `(*Peer).SendWorkReportDistribution`

### 108: Guarantee sent

Emitted if sending a work-report guarantee succeeds (CE 135).

    Event ID (ID of the corresponding "sending guarantee" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE135_workreportdistribution.go](../node/peerCE135_workreportdistribution.go#L171) — `(*Peer).SendWorkReportDistribution`

### 109: Guarantees distributed

Emitted by the primary guarantor once they have finished distributing the work-report guarantee(s).
This is the final event in the guaranteeing pipeline for primary guarantors.

This event may be emitted even if the guarantor was not successful in sending the guarantee(s) to
any other validator, although the guarantor may prefer to emit a "work-package failed" event in
that case.

    Event ID (ID of the corresponding "work-package submission" event)

#### Call sites -- ✅ sourabh audited
- [node/node_guarantee.go](../node/node_guarantee.go#L384) — `(*Node).processWPQueueItem`

### 110: Receiving guarantee

Emitted by the recipient when a guarantor begins sending a work-report guarantee (CE 135).

    Peer ID (Sender)

#### Call sites -- ✅ sourabh audited
- [node/peerCE135_workreportdistribution.go](../node/peerCE135_workreportdistribution.go#L183) — `(*Node).onWorkReportDistribution`

### 111: Guarantee receive failed

Emitted if receiving a work-report guarantee fails (CE 135).

    Event ID (ID of the corresponding "receiving guarantee" event)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/peerCE135_workreportdistribution.go](../node/peerCE135_workreportdistribution.go#L191) — `(*Node).onWorkReportDistribution`

### 112: Guarantee received

Emitted if receiving a work-report guarantee succeeds (CE 135). This should be emitted before the
guarantee is checked. If the guarantee is found to be invalid, a "peer misbehaved" event should be
emitted.

    Event ID (ID of the corresponding "receiving guarantee" event)
    Guarantee Outline

#### Call sites -- ✅ sourabh audited (moved location)
- [node/peerCE135_workreportdistribution.go](../node/peerCE135_workreportdistribution.go#L216) — `(*Node).onWorkReportDistribution`

### 113: Guarantee discarded -- 🔴 POST-MIGRATION

Emitted when a guarantee is discarded from the local guarantee pool.

    Guarantee Outline
    Guarantee Discard Reason

#### Call sites 
- [node/node.go](../node/node.go#L336) — `(*Node).handleGuaranteeDiscarded`
- we don't have such event I think?

## Availability distribution events

These events concern availability shard and assurance distribution.

**Implementation locations for events 120-131:**
- Shard requests (120-125): `node/network.go` - in CE 137 handlers
- Assurance distribution (126-131): `node/node_assurer.go` - in assurance logic and CE 141 handlers

### 120: Sending shard request

Emitted when an assurer begins sending a shard request to a guarantor (CE 137).

    Peer ID (Guarantor)
    Erasure-Root
    Shard Index

#### Call sites -- ✅ sourabh audited
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L93) — `(*Peer).SendFullShardRequest`

### 121: Receiving shard request

Emitted by the recipient when an assurer begins sending a shard request (CE 137).

    Peer ID (Assurer)

#### Call sites -- ✅ sourabh audited
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L162) — `(*Node).onFullShardRequest`

### 122: Shard request failed

Emitted when a shard request fails (CE 137). This should be emitted by both sides, ie the assurer
and the guarantor.

    Event ID (ID of the corresponding "sending shard request" or "receiving shard request" event)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L100) — `(*Peer).SendFullShardRequest`
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L115) — `(*Peer).SendFullShardRequest`
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L123) — `(*Peer).SendFullShardRequest`
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L142) — `(*Peer).SendFullShardRequest`
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L170) — `(*Node).onFullShardRequest`
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L185) — `(*Node).onFullShardRequest`
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L193) — `(*Node).onFullShardRequest`
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L214) — `(*Node).onFullShardRequest`

### 123: Shard request sent

Emitted once a shard request has been sent to a guarantor (CE 137). This should be emitted after
the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending shard request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L132) — `(*Peer).SendFullShardRequest`

### 124: Shard request received

Emitted once a shard request has been received from an assurer (CE 137).

    Event ID (ID of the corresponding "receiving shard request" event)
    Erasure-Root
    Shard Index

#### Call sites -- ✅ sourabh audited
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L177) — `(*Node).onFullShardRequest`

### 125: Shards transferred

Emitted when a shard request completes successfully (CE 137). This should be emitted by both sides,
ie the assurer and the guarantor.

    Event ID (ID of the corresponding "sending shard request" or "receiving shard request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L149) — `(*Peer).SendFullShardRequest`
- [node/peerCE137_fullshardrequest.go](../node/peerCE137_fullshardrequest.go#L221) — `(*Node).onFullShardRequest`

### 126: Distributing assurance

Emitted when an assurer begins distributing an assurance to other validators, for potential
inclusion in a block.

    Header Hash (Assurance anchor)
    [u8; ceil(C / 8)] (Availability bitfield; one bit per core, C is the total number of cores)

#### Call sites --  ✅ sourabh audited 
- [node/node.go](../node/node.go#L1826) — `(*Node).broadcast`

### 127: Assurance send failed

Emitted when an assurer fails to send an assurance to another validator (CE 141).

    Event ID (ID of the corresponding "distributing assurance" event)
    Peer ID (Recipient)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/peerCE141_assurance.go](../node/peerCE141_assurance.go#L109) — `(*Peer).SendAssurance`
- [node/peerCE141_assurance.go](../node/peerCE141_assurance.go#L118) — `(*Peer).SendAssurance`

### 128: Assurance sent

Emitted by assurers after sending an assurance to another validator (CE 141).

    Event ID (ID of the corresponding "distributing assurance" event)
    Peer ID (Recipient)

#### Call sites -- ✅ sourabh audited
- [node/peerCE141_assurance.go](../node/peerCE141_assurance.go#L125) — `(*Peer).SendAssurance`

### 129: Assurance distributed

Emitted once an assurer has finished distributing an assurance.

This event should be emitted even if the assurer was not successful in sending the assurance to any
other validator. The success of the distribution should be determined by the "assurance send
failed" and "assurance sent" events emitted by the assurer, as well as the "assurance receive
failed" and "assurance received" events emitted by the recipient validators.

    Event ID (ID of the corresponding "distributing assurance" event)

#### Call sites -- ✅ sourabh audited 
- [node/node.go](../node/node.go#L1836) — `(*Node).broadcast`

### 130: Assurance receive failed

Emitted when a validator fails to receive an assurance from a peer (CE 141).

    Peer ID (Sender)
    Reason

#### Call sites -- ✅ sourabh audited 
- [node/peerCE141_assurance.go](../node/peerCE141_assurance.go#L138) — `(*Node).onAssuranceDistribution`

### 131: Assurance received

Emitted when an assurance is received from a peer (CE 141). This should be emitted as soon as the
assurance is received, before checking if it is valid. If the assurance is found to be invalid, a
"peer misbehaved" event should be emitted. [PROBLEM: WE ARE NOT DOING THIS!!!]

    Peer ID (Sender)
    Header Hash (Assurance anchor)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE141_assurance.go](../node/peerCE141_assurance.go#L145) — `(*Node).onAssuranceDistribution`

## Bundle recovery events

These events concern recovery of work-package bundles for auditing.

**Implementation locations for events 140-153:**
- Bundle shard requests (140-145): `node/node_auditor.go` - in CE 138 handlers and bundle reconstruction
- Bundle requests (148-153): `node/node_auditor.go` - in CE 147 handlers

### 140: Sending bundle shard request

Emitted when an auditor begins sending a bundle shard request to an assurer (CE 138).

    Event ID (TODO, should reference auditing event)
    Peer ID (Assurer)
    Shard Index

#### Call sites -- ✅ sourabh audited 
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L47) — `(*Peer).SendBundleShardRequest`

### 141: Receiving bundle shard request

Emitted by the recipient when an auditor begins sending a bundle shard request (CE 138).

    Peer ID (Auditor)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L149) — `(*Node).onBundleShardRequest`

### 142: Bundle shard request failed

Emitted when a bundle shard request fails (CE 138). This should be emitted by both sides, ie the
auditor and the assurer.

    Event ID (ID of the corresponding "sending bundle shard request" or "receiving bundle shard request" event)
    Reason

#### Call sites -- ✅ sourabh audited 
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L54) — `(*Peer).SendBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L69) — `(*Peer).SendBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L79) — `(*Peer).SendBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L96) — `(*Peer).SendBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L114) — `(*Peer).SendBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L125) — `(*Peer).SendBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L156) — `(*Node).onBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L174) — `(*Node).onBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L182) — `(*Node).onBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L211) — `(*Node).onBundleShardRequest`

### 143: Bundle shard request sent

Emitted once a bundle shard request has been sent to an assurer (CE 138). This should be emitted
after the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending bundle shard request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L88) — `(*Peer).SendBundleShardRequest`

### 144: Bundle shard request received

Emitted once a bundle shard request has been received from an auditor (CE 138).

    Event ID (ID of the corresponding "receiving bundle shard request" event)
    Erasure-Root
    Shard Index

#### Call sites -- ✅ sourabh audited
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L163) — `(*Node).onBundleShardRequest`

### 145: Bundle shard transferred

Emitted when a bundle shard request completes successfully (CE 138). This should be emitted by both
sides, ie the auditor and the assurer.

    Event ID (ID of the corresponding "sending bundle shard request" or "receiving bundle shard request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L132) — `(*Peer).SendBundleShardRequest`
- [node/peerCE138_bundleshardrequest.go](../node/peerCE138_bundleshardrequest.go#L218) — `(*Node).onBundleShardRequest`

### 146: Reconstructing bundle

Emitted when reconstruction of a bundle from shards received from assurers begins.

    Event ID 
    bool (Is this a trivial reconstruction, using only original-data shards?)

#### Call sites -- ✅ sourabh audited
- [node/node.go](../node/node.go#L2624) — `(*NodeContent).reconstructPackageBundleSegments`

### 147: Bundle reconstructed

Emitted once a bundle has been successfully reconstructed from shards.

    Event ID

#### Call sites -- ✅ sourabh audited
- [node/node.go](../node/node.go#L2731) — `(*NodeContent).reconstructPackageBundleSegments`

### 148: Sending bundle request

Emitted when an auditor begins sending a bundle request to a guarantor (CE 147).

    Event ID 
    Peer ID (Guarantor)

#### Call sites -- ✅ sourabh audited
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L38) — `(*Peer).SendBundleRequest`

### 149: Receiving bundle request

Emitted by the recipient when an auditor begins sending a bundle request (CE 147).

    Peer ID (Auditor)

#### Call sites -- ✅ sourabh audited
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L111) — `(*Node).onBundleRequest`

### 150: Bundle request failed

Emitted when a bundle request fails (CE 147). This should be emitted by both sides, ie the auditor
and the guarantor.

    Event ID (ID of the corresponding "sending bundle request" or "receiving bundle request" event)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L45) — `(*Peer).SendBundleRequest`
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L55) — `(*Peer).SendBundleRequest`
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L73) — `(*Peer).SendBundleRequest`
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L83) — `(*Peer).SendBundleRequest`
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L118) — `(*Node).onBundleRequest`
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L145) — `(*Node).onBundleRequest`
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L161) — `(*Node).onBundleRequest`

### 151: Bundle request sent

Emitted once a bundle request has been sent to a guarantor (CE 147). This should be emitted after
the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending bundle request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L62) — `(*Peer).SendBundleRequest`

### 152: Bundle request received

Emitted once a bundle request has been received from an auditor (CE 147).

    Event ID (ID of the corresponding "receiving bundle request" event)
    Erasure-Root

#### Call sites -- ✅ sourabh audited
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L127) — `(*Node).onBundleRequest`

### 153: Bundle transferred

Emitted when a bundle request completes successfully (CE 147). This should be emitted by both
sides, ie the auditor and the guarantor.

    Event ID (ID of the corresponding "sending bundle request" or "receiving bundle request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L90) — `(*Peer).SendBundleRequest`
- [node/peerCE147_bundlerequest.go](../node/peerCE147_bundlerequest.go#L168) — `(*Node).onBundleRequest`

## Segment recovery events

These events concern recovery of segments exported by work-packages. Segments are recovered by
primary guarantors, hence these events reference "work-package submission" events.

**Implementation locations for events 160-178:**
- Segment shard requests (162-167): `node/node_guarantor.go` - in CE 139/140 handlers and segment reconstruction
- Segment requests (173-178): `node/node_guarantor.go` - in CE 148 handlers
- Segment verification (171-172): `node/node_guarantor.go` - after segment reconstruction

### 160: Work-package hash mapped

Emitted when a work-package hash is mapped to a segments-root for the purpose of segment recovery.

    Event ID (ID of the corresponding "work-package submission" event)
    Work-Package Hash
    Segments-Root

#### Call sites -- ✅ sourabh audited
- [node/node_da.go](../node/node_da.go#L269) — `(*NodeContent).VerifyBundle`

### 161: Segments-root mapped  🔴 POST-MIGRATION

Emitted when a segments-root is mapped to an erasure-root for the purpose of segment recovery.

    Event ID (ID of the corresponding "work-package submission" event)
    Segments-Root
    Erasure-Root

#### Call sites
- [node/node_da.go](../node/node_da.go#L277) — `(*NodeContent).VerifyBundle`

### 162: Sending segment shard request

Emitted when a guarantor begins sending a segment shard request to an assurer (CE 139/140).

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Assurer)
    bool (Using CE 140?)
    len++[Import Segment ID ++ Shard Index] (Segment shards being requested)

#### Call sites -- ✅ sourabh audited
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L146) — `(*Peer).SendSegmentShardRequest`

### 163: Receiving segment shard request

Emitted by the recipient when a node begins sending a segment shard request (CE 139/140).

    Peer ID (Sender)
    bool (Using CE 140?)

#### Call sites -- ✅ sourabh audited
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L234) — `(*Node).onSegmentShardRequest`

### 164: Segment shard request failed

Emitted when a segment shard request fails (CE 139/140). This should be emitted by both sides.

    Event ID (ID of the corresponding "sending segment shard request" or "receiving segment shard request" event)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L153) — `(*Peer).SendSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L170) — `(*Peer).SendSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L179) — `(*Peer).SendSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L196) — `(*Peer).SendSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L207) — `(*Peer).SendSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L243) — `(*Node).onSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L260) — `(*Node).onSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L269) — `(*Node).onSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L286) — `(*Node).onSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L298) — `(*Node).onSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L306) — `(*Node).onSegmentShardRequest`

### 165: Segment shard request sent

Emitted once a segment shard request has been sent to an assurer (CE 139/140). This should be
emitted after the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending segment shard request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L188) — `(*Peer).SendSegmentShardRequest`

### 166: Segment shard request received

Emitted once a segment shard request has been received (CE 139/140).

    Event ID (ID of the corresponding "receiving segment shard request" event)
    u16 (Number of segment shards requested)

#### Call sites -- ✅ sourabh audited
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L251) — `(*Node).onSegmentShardRequest`

### 167: Segment shards transferred

Emitted when a segment shard request completes successfully (CE 139/140). This should be emitted by
both sides.

    Event ID (ID of the corresponding "sending segment shard request" or "receiving segment shard request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L215) — `(*Peer).SendSegmentShardRequest`
- [node/peerCE139_140_segmentshardrequest.go](../node/peerCE139_140_segmentshardrequest.go#L315) — `(*Node).onSegmentShardRequest`

### 168: Reconstructing segments

Emitted when reconstruction of a set of segments from shards received from assurers begins.

    Event ID (ID of the corresponding "work-package submission" event)
    len++[Import Segment ID] (Segments being reconstructed)
    bool (Is this a trivial reconstruction, using only original-data shards?)

#### Call sites -- ✅ sourabh audited
- [node/node_guarantee.go](../node/node_guarantee.go#L122) — `(*NodeContent).buildBundle`

### 169: Segment reconstruction failed

Emitted if reconstruction of a set of segments fails.

    Event ID (ID of the corresponding "reconstructing segments" event)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/node_guarantee.go](../node/node_guarantee.go#L129) — `(*NodeContent).buildBundle`

### 170: Segments reconstructed

Emitted once a set of segments has been successfully reconstructed from shards.

    Event ID (ID of the corresponding "reconstructing segments" event)

#### Call sites -- ✅ sourabh audited
- [node/node_guarantee.go](../node/node_guarantee.go#L137) — `(*NodeContent).buildBundle`

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

#### Call sites -- ✅ sourabh audited
- [node/node.go](../node/node.go#L2588) — `(*NodeContent).reconstructSegments`

### 172: Segments verified

Emitted once a reconstructed segment has been successfully verified against the corresponding
segments-root. For efficiency, multiple segments may be reported in a single event.

    Event ID (ID of the corresponding "work-package submission" event)
    len++[u16] (Indices of the verified segments in the import list)

#### Call sites -- ✅ sourabh audited
- [node/node.go](../node/node.go#L2595) — `(*NodeContent).reconstructSegments`

### 173: Sending segment request

Emitted when a guarantor begins sending a segment request to a previous guarantor (CE 148). Note
that proof pages need not (and in fact cannot) be requested using this protocol, hence the use of
`u16` rather than `Import Segment ID` to identify each requested segment.

    Event ID (ID of the corresponding "work-package submission" event)
    Peer ID (Previous guarantor)
    len++[u16] (Indices of requested segments in overall list of work-package imports)

#### Call sites -- ✅ sourabh audited
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L132) — `(*Peer).SendSegmentRequest`

### 174: Receiving segment request

Emitted by the recipient when a guarantor begins sending a segment request (CE 148).

    Peer ID (Guarantor)

#### Call sites -- ✅ sourabh audited
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L231) — `(*Node).onSegmentRequest`

### 175: Segment request failed

Emitted when a segment request fails (CE 148). This should be emitted by both sides.

    Event ID (ID of the corresponding "sending segment request" or "receiving segment request" event)
    Reason

#### Call sites -- ✅ sourabh audited
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L139) — `(*Peer).SendSegmentRequest`
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L155) — `(*Peer).SendSegmentRequest`
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L177) — `(*Peer).SendSegmentRequest`
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L187) — `(*Peer).SendSegmentRequest`
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L197) — `(*Peer).SendSegmentRequest`
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L240) — `(*Node).onSegmentRequest`
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L271) — `(*Node).onSegmentRequest`
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L288) — `(*Node).onSegmentRequest`
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L298) — `(*Node).onSegmentRequest`

### 176: Segment request sent

Emitted once a segment request has been sent to a previous guarantor (CE 148). This should be
emitted after the initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending segment request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L162) — `(*Peer).SendSegmentRequest`

### 177: Segment request received

Emitted once a segment request has been received from a guarantor (CE 148).

    Event ID (ID of the corresponding "receiving segment request" event)
    u16 (Number of segments requested)

#### Call sites -- ✅ sourabh audited
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L247) — `(*Node).onSegmentRequest`

### 178: Segments transferred

Emitted when a segment request completes successfully (CE 148). This should be emitted by both
sides.

    Event ID (ID of the corresponding "sending segment request" or "receiving segment request" event)

#### Call sites -- ✅ sourabh audited
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L210) — `(*Peer).SendSegmentRequest`
- [node/peerCE148_segmentrequest.go](../node/peerCE148_segmentrequest.go#L306) — `(*Node).onSegmentRequest`

## Preimage distribution events

These events concern distribution of preimages for inclusion in blocks.

**Implementation locations for events 190-199:**
- Preimage announcements (190-192): `node/network.go` - in CE 142 handlers
- Preimage requests (193-198): `node/network.go` - in CE 143 handlers
- Preimage pool management (192, 199): `node/preimage_pool.go` - in pool logic

### 190: Preimage announcement failed

Emitted when a preimage announcement fails (CE 142).

    Peer ID
    Connection Side (Announcer)
    Reason

#### Call sites -- ✅ sourabh audited 
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L67) — `(*Peer).SendPreimageAnnouncement`
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L77) — `(*Peer).SendPreimageAnnouncement`
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L85) — `(*Peer).SendPreimageAnnouncement`
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L104) — `(*Node).onPreimageAnnouncement`

### 191: Preimage announced

Emitted when a preimage announcement is sent to or received from a peer (CE 142).

    Peer ID
    Connection Side (Announcer)
    Service ID (Requesting service)
    Hash
    u32 (Preimage length)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L60) — `(*Peer).SendPreimageAnnouncement`
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L111) — `(*Node).onPreimageAnnouncement`

### 192: Announced preimage forgotten 🔴 POST-MIGRATION

Emitted when a preimage announced by a peer is forgotten about. This event should not be emitted
for preimages the node managed to acquire (if such a preimage is discarded, a "preimage discarded"
event should be emitted instead).

    Service ID (Requesting service)
    Hash
    u32 (Preimage length)
    Announced Preimage Forget Reason

### 193: Sending preimage request 

Emitted when a validator begins sending a preimage request to a peer (CE 143).

    Peer ID (Recipient)
    Hash

#### Call sites -- ✅ sourabh audited 
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L171) — `(*Peer).SendPreimageRequest`

### 194: Receiving preimage request

Emitted by the recipient when a validator begins sending a preimage request (CE 143).

    Peer ID (Sender)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L226) — `(*NodeContent).onPreimageRequest`

### 195: Preimage request failed

Emitted when a preimage request (CE 143) fails.

    Event ID (ID of the corresponding "sending preimage request" or "receiving preimage request" event)
    Reason

#### Call sites -- ✅ sourabh audited 
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L178) — `(*Peer).SendPreimageRequest`
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L188) — `(*Peer).SendPreimageRequest`
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L205) — `(*Peer).SendPreimageRequest`
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L241) — `(*NodeContent).onPreimageRequest`
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L252) — `(*NodeContent).onPreimageRequest`

### 196: Preimage request sent

Emitted once a preimage request has been sent to a peer (CE 143). This should be emitted after the
initial message containing the request details has been transmitted.

    Event ID (ID of the corresponding "sending preimage request" event)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L195) — `(*Peer).SendPreimageRequest`

### 197: Preimage request received

Emitted once a preimage request has been received from a peer (CE 143).

    Event ID (ID of the corresponding "receiving preimage request" event)
    Hash

#### Call sites -- ✅ sourabh audited 
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L233) — `(*NodeContent).onPreimageRequest`

### 198: Preimage transferred

Emitted when a preimage has been fully sent to or received from a peer (CE 143).

    Event ID (ID of the corresponding "sending preimage request" or "receiving preimage request" event)
    u32 (Preimage length)

#### Call sites -- ✅ sourabh audited 
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L212) — `(*Peer).SendPreimageRequest`
- [node/peerCE142_peerimageannouncement.go](../node/peerCE142_peerimageannouncement.go#L259) — `(*NodeContent).onPreimageRequest`

### 199: Preimage discarded -- 🔴 POST-MIGRATION

Emitted when a preimage is discarded from the local preimage pool.

Note that in the case where the preimage was requested by multiple services, there may not be a
unique discard reason. For example, the preimage may have been provided to one service, while
another service may have stopped requesting it. In this case, either reason may be reported.

    Hash
    u32 (Preimage length)
    Preimage Discard Reason
