package fuzz

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/statedb"
	"github.com/jam-duna/jamduna/types"
)

const (
	GP_VERSION      = "0.7.2"
	FUZZ_VERSION_V1 = 1
	FUZZ_VERSION_V0 = 0
)

// major.minor.patch
var (
	PATCH_VERSION = 13 // Bump this for patch releases
	APP_VERSION   = fmt.Sprintf("0.3.%v", PATCH_VERSION)
	JAM_VERSION   = fmt.Sprintf("%v.%v", GP_VERSION, PATCH_VERSION) // Tag as <0.7.2><.x> for our jam binary release
)

/*
Message Flow
- The protocol follows a strict request-response pattern where:
- The fuzzer always initiates requests
- The target must respond to each request before the next request is sent
- Responses are mandatory and must match the expected message type for each request
- State roots are compared after each block import to detect discrepancies
- Full state retrieval via GetState is only performed when state root mismatches are detected
- Unexpected or malformed messages result in blunt session termination
- The fuzzer may implement timeouts for target responses
- The fuzzing session is terminated by the fuzzer closing the connection. No explicit termination message is sent.
*/

/* V0
| Request | Response | Purpose |
|----------------|-------------------|---------|
| `PeerInfo` | `PeerInfo` | Handshake and versioning exchange |
| `SetState` | `StateRoot` | Initialize or reset target state |
| `ImportBlock` | `StateRoot` | Process block and return resulting state root |
| `GetState` | `State` | Retrieve posterior state associated to given header hash |
*/

/* V0r
| Request | Response | Purpose |
|----------------|-------------------|---------|
| `PeerInfo` | `PeerInfo` | Handshake and versioning exchange |
| `SetState` | `StateRoot` | Initialize or reset target state |
| `ImportBlock` | `StateRoot` | Process block and return resulting state root |
| `GetState` | `State` | Retrieve posterior state associated to given header hash |
| `RefineBundle` | `WorkReport` | Compute work report given work package bundle |
| `GetExports` | `Segments` | Return Exported segments for Work Package Hash or Exported Segment Root |
*/

/* V1
| Request        | Response     | Description |
|----------------|--------------|-------------|
| `PeerInfo`     | `PeerInfo`   | Handshake and versioning exchange |
| `Initialize`   | `StateRoot`  | Initialize or reset target state |
| `ImportBlock`  | `StateRoot`  | Import block and return resulting state root |
| `GetState`     | `State`      | Retrieve posterior state associated to given header hash |

| Request        | Response | Description |
|----------------|----------|-------------|
| `ImportBlock`  | `Error`  | Import block failure |

*/

// --- High-Level Data Structures ---

var (
	MaxAncestorsLengthA = 24 // Maximum ancestry depth for V1 protocol (tiny chains)
)

const (
	ProtocolV0  uint8 = 0 // V0 protocol (SetState-based)
	ProtocolV0r uint8 = 2 // V0 with refinement extensions (SetState-based + refinement)
	ProtocolV1  uint8 = 1 // Official V1 spec (Initialize-based)
)

type PeerInfo interface {
	GetName() string
	GetAppVersion() Version
	GetJamVersion() Version
	PrettyString(withColors bool) string

	// Protocol version detection
	GetProtocolVersion() uint8

	// V1-specific methods (V0 implementations return defaults)
	GetFuzzVersion() uint8
	GetFeatures() Features

	// Internal methods for protocol handling
	SetDefaults()

	// Additional methods for compatibility
	Info() string
}

type ProtocolHandler interface {
	Encode(msg *Message) ([]byte, error)
	Decode(data []byte) (*Message, error)
	GetProtocolVersion() uint8
}

var protocolHandler ProtocolHandler

func GetProtocolHandler() ProtocolHandler {
	if protocolHandler == nil {
		panic("protocolHandler not initialized - check build tags")
	}
	return protocolHandler
}

// Features is a u32 where the MSB (bit 31) is reserved for future extensions
type Features uint32

// Feature flags used across protocol versions
const (
	FeatureBlockAncestry    Features = 1          // 2^0
	FeatureSimpleForking    Features = 2          // 2^1
	FeatureBundleRefinement Features = 4          // 2^2
	FeatureExports          Features = 8          // 2^3
	FeatureExtension        Features = 2147483648 // 2^31 = 0x80000000
)

// Version ::= SEQUENCE { major, minor, patch }
type Version struct {
	Major uint8 `json:"major"`
	Minor uint8 `json:"minor"`
	Patch uint8 `json:"patch"`
}

// ParseVersion parses a version string in the format "major.minor.patch" into a Version struct
func ParseVersion(versionStr string) Version {
	parts := strings.Split(versionStr, ".")
	if len(parts) < 3 {
		return Version{0, 0, 0}
	}

	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])

	return Version{
		Major: uint8(major),
		Minor: uint8(minor),
		Patch: uint8(patch),
	}
}

// --- Message Payloads ---

// HeaderWithState is the payload for a set-state message.
// Corresponds to: SetState ::= SEQUENCE { header, state }
type HeaderWithState struct {
	Header types.BlockHeader    `json:"header"`
	State  statedb.StateKeyVals `json:"state"`
}

// --- V1 Specific Types ---

// AncestryItem ::= SEQUENCE { slot TimeSlot, header-hash HeaderHash }
type AncestryItem struct {
	Slot       uint32      `json:"slot"`
	HeaderHash common.Hash `json:"header_hash"`
}

// Ancestry ::= SEQUENCE (SIZE(0..24)) OF AncestryItem
type Ancestry []AncestryItem

// Initialize ::= SEQUENCE { header Header, keyvals State, ancestry Ancestry }
type Initialize struct {
	Header   types.BlockHeader    `json:"header"`
	KeyVals  statedb.StateKeyVals `json:"keyvals"`
	Ancestry Ancestry             `json:"ancestry"`
}

// --- Unified Message Container (All Versions) ---

// Message contains ALL possible message types from V0, V0r, and V1 protocols.
// Version-specific encode/decode functions handle the appropriate tag mappings.
type Message struct {
	// Core messages (V0, V0r, V1)
	PeerInfo    PeerInfo              `json:"peer_info,omitempty"`    // Tag varies by version
	ImportBlock *types.Block          `json:"import_block,omitempty"` // Tag varies by version
	GetState    *common.Hash          `json:"get_state,omitempty"`    // Tag varies by version
	State       *statedb.StateKeyVals `json:"state,omitempty"`        // Tag varies by version
	StateRoot   *common.Hash          `json:"state_root,omitempty"`   // Tag varies by version

	// V0/V0r messages
	SetState *HeaderWithState `json:"set_state,omitempty"` // Tag [2] in V0/V0r

	// V1 messages
	Initialize *Initialize `json:"initialize,omitempty"` // Tag [1] in V1
	Error      *string     `json:"error,omitempty"`      // Tag [255] in V1

	// V0r extended messages
	RefineBundle *types.RefineBundle `json:"refine_bundle,omitempty"` // Tag [6] in V0r
	WorkReport   *types.WorkReport   `json:"work_report,omitempty"`   // Tag [7] in V0r
	GetExports   *common.Hash        `json:"get_exports,omitempty"`   // Tag [8] in V0r
	Segments     *[][]byte           `json:"segments,omitempty"`      // Tag [9] in V0r
}

func CreatePeerInfo(appName string) PeerInfo {
	return CreateVersionedPeerInfo(appName)
}
