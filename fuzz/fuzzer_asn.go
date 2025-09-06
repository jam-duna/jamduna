package fuzz

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

const (
	GP_VERSION = "0.7.0"
)

// major.minor.patch
var (
	PATCH_VERSION = 8 // Bump this for patch releases
	APP_VERSION   = fmt.Sprintf("0.1.%v", PATCH_VERSION)
	JAM_VERSION   = fmt.Sprintf("%v.%v", GP_VERSION, PATCH_VERSION) // Tag as <0.7.0><.x> for our jam binary release
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

// --- High-Level Data Structures ---

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

// --- Top-Level Message Container ---

// Message ::= CHOICE { peer-info, import-block, set-state, get-state, state, state-root, refine-bundle, work-report }
// This is a tagged union where exactly one field should be non-nil.
type Message struct {
	PeerInfo     *PeerInfo             `json:"peer_info,omitempty"`     // Tag [0]
	ImportBlock  *types.Block          `json:"import_block,omitempty"`  // Tag [1]
	SetState     *HeaderWithState      `json:"set_state,omitempty"`     // Tag [2]
	GetState     *common.Hash          `json:"get_state,omitempty"`     // Tag [3]
	State        *statedb.StateKeyVals `json:"state,omitempty"`         // Tag [4]
	StateRoot    *common.Hash          `json:"state_root,omitempty"`    // Tag [5]
	RefineBundle *types.RefineBundle   `json:"refine_bundle,omitempty"` // Tag [6]
	WorkReport   *types.WorkReport     `json:"work_report,omitempty"`   // Tag [7]
	ReportHash   *common.Hash          `json:"report_hash,omitempty"`   // Tag [8]
}

// --- JAM Codec Functions ---

// encode serializes a Message into its byte representation using the custom JAM codec.
// The format is: [1-byte tag][jam-encoded data]
func encode(msg *Message) ([]byte, error) {
	var (
		tag  byte
		data interface{}
	)

	switch {
	case msg.PeerInfo != nil:
		tag, data = 0, *msg.PeerInfo
	case msg.ImportBlock != nil:
		tag, data = 1, *msg.ImportBlock
	case msg.SetState != nil:
		tag, data = 2, *msg.SetState
	case msg.GetState != nil:
		tag, data = 3, *msg.GetState
	case msg.State != nil:
		tag, data = 4, *msg.State
	case msg.StateRoot != nil:
		tag, data = 5, *msg.StateRoot
	case msg.RefineBundle != nil:
		tag, data = 6, *msg.RefineBundle
	case msg.WorkReport != nil:
		tag, data = 7, *msg.WorkReport
	case msg.ReportHash != nil:
		tag, data = 8, *msg.ReportHash
	default:
		return nil, fmt.Errorf("cannot encode empty message")
	}

	encodedBytes, err := types.Encode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to jam-encode data for tag %d: %w", tag, err)
	}

	return append([]byte{tag}, encodedBytes...), nil
}

// decode parses a byte slice and reconstructs a Message using the custom JAM codec.
func decode(data []byte) (*Message, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("cannot decode empty or invalid message")
	}

	tag := data[0]
	encodedBody := data[1:]
	msg := &Message{}

	var (
		targetType reflect.Type
		assign     func(interface{})
	)

	switch tag {
	case 0:
		targetType = reflect.TypeOf(PeerInfo{})
		assign = func(v interface{}) { val := v.(PeerInfo); msg.PeerInfo = &val }
	case 1:
		targetType = reflect.TypeOf(types.Block{})
		assign = func(v interface{}) { val := v.(types.Block); msg.ImportBlock = &val }
	case 2:
		targetType = reflect.TypeOf(HeaderWithState{})
		assign = func(v interface{}) { val := v.(HeaderWithState); msg.SetState = &val }
	case 3:
		targetType = reflect.TypeOf(common.Hash{})
		assign = func(v interface{}) { val := v.(common.Hash); msg.GetState = &val }
	case 4:
		targetType = reflect.TypeOf(statedb.StateKeyVals{})
		assign = func(v interface{}) { val := v.(statedb.StateKeyVals); msg.State = &val }
	case 5:
		targetType = reflect.TypeOf(common.Hash{})
		assign = func(v interface{}) { val := v.(common.Hash); msg.StateRoot = &val }
	case 6:
		targetType = reflect.TypeOf(types.RefineBundle{})
		assign = func(v interface{}) { val := v.(types.RefineBundle); msg.RefineBundle = &val }
	case 7:
		targetType = reflect.TypeOf(types.WorkReport{})
		assign = func(v interface{}) { val := v.(types.WorkReport); msg.WorkReport = &val }
	case 8:
		targetType = reflect.TypeOf(common.Hash{})
		assign = func(v interface{}) { val := v.(common.Hash); msg.ReportHash = &val }
	default:
		return nil, fmt.Errorf("unknown message tag: %d", tag)
	}

	decodedStruct, _, err := types.Decode(encodedBody, targetType)
	if err != nil {
		return nil, fmt.Errorf("failed to jam-decode data for tag %d: %w", tag, err)
	}

	assign(decodedStruct)
	return msg, nil
}
