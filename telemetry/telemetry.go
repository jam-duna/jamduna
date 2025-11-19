package telemetry


// Telemetry event type discriminators as defined in JIP-3
//
// All telemetry event discriminators are consolidated here for easy reference
const (
	// Meta events
	Telemetry_Dropped = 0

	// Status events (10-13)
	Telemetry_Status                  = 10
	Telemetry_Best_Block_Changed      = 11
	Telemetry_Finalized_Block_Changed = 12
	Telemetry_Sync_Status_Changed     = 13

	// Networking events (20-28)
	Telemetry_Connection_Refused = 20
	Telemetry_Connecting_In      = 21
	Telemetry_Connect_In_Failed  = 22
	Telemetry_Connected_In       = 23
	Telemetry_Connecting_Out     = 24
	Telemetry_Connect_Out_Failed = 25
	Telemetry_Connected_Out      = 26
	Telemetry_Disconnected       = 27
	Telemetry_Peer_Misbehaved    = 28

	// Block events (40-48)
	Telemetry_Authoring                   = 40
	Telemetry_Authoring_Failed            = 41
	Telemetry_Authored                    = 42
	Telemetry_Importing                   = 43
	Telemetry_Block_Verification_Failed   = 44
	Telemetry_Block_Verified              = 45
	Telemetry_Block_Execution_Failed      = 46
	Telemetry_Block_Executed              = 47
	Telemetry_Accumulate_Result_Available = 48

	// Block announcement events (60-68, 71)
	Telemetry_Block_Announcement_Stream_Opened = 60
	Telemetry_Block_Announcement_Stream_Closed = 61
	Telemetry_Block_Announced                  = 62
	Telemetry_Sending_Block_Request            = 63
	Telemetry_Receiving_Block_Request          = 64
	Telemetry_Block_Request_Failed             = 65
	Telemetry_Block_Request_Sent               = 66
	Telemetry_Block_Request_Received           = 67
	Telemetry_Block_Transferred                = 68
	Telemetry_Block_Announcement_Malformed     = 71

	// Safrole events (80-84)
	Telemetry_Generating_Tickets       = 80
	Telemetry_Ticket_Generation_Failed = 81
	Telemetry_Tickets_Generated        = 82
	Telemetry_Ticket_Transfer_Failed   = 83
	Telemetry_Ticket_Transferred       = 84

	// Work package/guarantee events (90-113)
	Telemetry_Work_Package_Submission        = 90
	Telemetry_Work_Package_Being_Shared      = 91
	Telemetry_Work_Package_Failed            = 92
	Telemetry_Duplicate_Work_Package         = 93
	Telemetry_Work_Package_Received          = 94
	Telemetry_Authorized                     = 95
	Telemetry_Extrinsic_Data_Received        = 96
	Telemetry_Imports_Received               = 97
	Telemetry_Sharing_Work_Package           = 98
	Telemetry_Work_Package_Sharing_Failed    = 99
	Telemetry_Bundle_Sent                    = 100
	Telemetry_Refined                        = 101
	Telemetry_Work_Report_Built              = 102
	Telemetry_Work_Report_Signature_Sent     = 103
	Telemetry_Work_Report_Signature_Received = 104
	Telemetry_Guarantee_Built                = 105
	Telemetry_Sending_Guarantee              = 106
	Telemetry_Guarantee_Send_Failed          = 107
	Telemetry_Guarantee_Sent                 = 108
	Telemetry_Guarantees_Distributed         = 109
	Telemetry_Receiving_Guarantee            = 110
	Telemetry_Guarantee_Receive_Failed       = 111
	Telemetry_Guarantee_Received             = 112
	Telemetry_Guarantee_Discarded            = 113

	// Assurance events (120-133)
	Telemetry_Sending_Shard_Request    = 120
	Telemetry_Receiving_Shard_Request  = 121
	Telemetry_Shard_Request_Failed     = 122
	Telemetry_Shard_Request_Sent       = 123
	Telemetry_Shard_Request_Received   = 124
	Telemetry_Shards_Transferred       = 125
	Telemetry_Distributing_Assurance   = 126
	Telemetry_Assurance_Send_Failed    = 127
	Telemetry_Assurance_Sent           = 128
	Telemetry_Assurance_Distributed    = 129
	Telemetry_Assurance_Receive_Failed = 130
	Telemetry_Assurance_Received       = 131
	Telemetry_Context_Available        = 132
	Telemetry_Assurance_Provided       = 133

	// Bundle recovery events (140-153)
	Telemetry_Sending_Bundle_Shard_Request   = 140
	Telemetry_Receiving_Bundle_Shard_Request = 141
	Telemetry_Bundle_Shard_Request_Failed    = 142
	Telemetry_Bundle_Shard_Request_Sent      = 143
	Telemetry_Bundle_Shard_Request_Received  = 144
	Telemetry_Bundle_Shard_Transferred       = 145
	Telemetry_Reconstructing_Bundle          = 146
	Telemetry_Bundle_Reconstructed           = 147
	Telemetry_Sending_Bundle_Request         = 148
	Telemetry_Receiving_Bundle_Request       = 149
	Telemetry_Bundle_Request_Failed          = 150
	Telemetry_Bundle_Request_Sent            = 151
	Telemetry_Bundle_Request_Received        = 152
	Telemetry_Bundle_Transferred             = 153

	// Segment events (160-178)
	Telemetry_Work_Package_Hash_Mapped        = 160
	Telemetry_Segments_Root_Mapped            = 161
	Telemetry_Sending_Segment_Shard_Request   = 162
	Telemetry_Receiving_Segment_Shard_Request = 163
	Telemetry_Segment_Shard_Request_Failed    = 164
	Telemetry_Segment_Shard_Request_Sent      = 165
	Telemetry_Segment_Shard_Request_Received  = 166
	Telemetry_Segment_Shards_Transferred      = 167
	Telemetry_Reconstructing_Segments         = 168
	Telemetry_Segment_Reconstruction_Failed   = 169
	Telemetry_Segments_Reconstructed          = 170
	Telemetry_Segment_Verification_Failed     = 171
	Telemetry_Segments_Verified               = 172
	Telemetry_Sending_Segment_Request         = 173
	Telemetry_Receiving_Segment_Request       = 174
	Telemetry_Segment_Request_Failed          = 175
	Telemetry_Segment_Request_Sent            = 176
	Telemetry_Segment_Request_Received        = 177
	Telemetry_Segments_Transferred            = 178

	// Preimage events (190-199)
	Telemetry_Preimage_Announcement_Failed = 190
	Telemetry_Preimage_Announced           = 191
	Telemetry_Announced_Preimage_Forgotten = 192
	Telemetry_Sending_Preimage_Request     = 193
	Telemetry_Receiving_Preimage_Request   = 194
	Telemetry_Preimage_Request_Failed      = 195
	Telemetry_Preimage_Request_Sent        = 196
	Telemetry_Preimage_Request_Received    = 197
	Telemetry_Preimage_Transferred         = 198
	Telemetry_Preimage_Discarded           = 199
)

/*
# JIP-3: Telemetry

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
    Gas = u64
    Exec Cost =
        Gas (Gas used) ++
        u64 (Elapsed wall-clock time in nanoseconds)


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
*/

/*
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

The node flags field should be treated as a bitmask. The following bits are defined:

- Bit 0 (LSB): 1 means the node uses a PVM recompiler, 0 means the node uses a PVM interpreter.

All other bits should be set to 0.
*/

// NodeInfo is now aliased from types package in telemetry_client.go
