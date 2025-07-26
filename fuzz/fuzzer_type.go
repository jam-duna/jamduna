package fuzz

import (
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
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

// PeerInfo ::= SEQUENCE { name, app-version, jam-version }
type PeerInfo struct {
	Name       string  `json:"name"`
	AppVersion Version `json:"app_version"`
	JamVersion Version `json:"jam_version"`
}

// --- Message Payloads ---

// HeaderWithState is the payload for a set-state message.
// Corresponds to: SetState ::= SEQUENCE { header, state }
type HeaderWithState struct {
	Header types.BlockHeader    `json:"header"`
	State  statedb.StateKeyVals `json:"state"`
}

// --- Top-Level Message Container ---

// Message ::= CHOICE { peer-info, import-block, set-state, get-state, state, state-root }
// This is a tagged union where exactly one field should be non-nil.
type Message struct {
	PeerInfo    *PeerInfo             `json:"peer_info,omitempty"`    // Tag [0]
	ImportBlock *types.Block          `json:"import_block,omitempty"` // Tag [1]
	SetState    *HeaderWithState      `json:"set_state,omitempty"`    // Tag [2]
	GetState    *common.Hash          `json:"get_state,omitempty"`    // Tag [3]
	State       *statedb.StateKeyVals `json:"state,omitempty"`        // Tag [4]
	StateRoot   *common.Hash          `json:"state_root,omitempty"`   // Tag [5]
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
