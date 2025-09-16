//go:build asn0
// +build asn0

package fuzz

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// PeerInfoV0 ::= SEQUENCE {app-version, jam-version, name}
type PeerInfoV0 struct {
	AppVersion Version `json:"app_version"`
	JamVersion Version `json:"jam_version"`
	Name       string  `json:"name"`
}

// Implement PeerInfo interface
func (p *PeerInfoV0) GetName() string           { return p.Name }
func (p *PeerInfoV0) GetAppVersion() Version    { return p.AppVersion }
func (p *PeerInfoV0) GetJamVersion() Version    { return p.JamVersion }
func (p *PeerInfoV0) GetProtocolVersion() uint8 { return ProtocolV0 }
func (p *PeerInfoV0) GetFuzzVersion() uint8     { return 0 } // V0 doesn't have FuzzVersion
func (p *PeerInfoV0) GetFeatures() Features     { return 0 } // V0 doesn't have Features
func (p *PeerInfoV0) SetDefaults()              {}

// PeerInfoDisplay represents PeerInfo for JSON output
type PeerInfoDisplay struct {
	Name       string `json:"Name"`
	AppVersion string `json:"AppVersion"`
	JamVersion string `json:"JamVersion"`
}

// PrettyString formats PeerInfo as indented JSON for display
func (p *PeerInfoV0) PrettyString(isIndented bool) string {
	display := PeerInfoDisplay{
		AppVersion: fmt.Sprintf("%d.%d.%d", p.AppVersion.Major, p.AppVersion.Minor, p.AppVersion.Patch),
		JamVersion: fmt.Sprintf("%d.%d.%d", p.JamVersion.Major, p.JamVersion.Minor, p.JamVersion.Patch),
		Name:       p.Name,
	}
	var err error
	var jsonBytes []byte
	if isIndented {
		jsonBytes, err = json.MarshalIndent(display, "", "   ")
		if err != nil {
			return fmt.Sprintf("%+v", p)
		}
	} else {
		jsonBytes, err = json.Marshal(display)
		if err != nil {
			return fmt.Sprintf("%+v", p)
		}
	}
	return string(jsonBytes)
}

// Info formats PeerInfo as clean text for display
func (p *PeerInfoV0) Info() string {
	return fmt.Sprintf(`  Name: %s
  AppVersion: %d.%d.%d
  JAMVersion: %d.%d.%d`,
		p.Name,
		p.AppVersion.Major, p.AppVersion.Minor, p.AppVersion.Patch,
		p.JamVersion.Major, p.JamVersion.Minor, p.JamVersion.Patch)
}

// --- V0 JAM Codec Functions ---
// V0 tag mapping: peer-info [0], import-block [1], set-state [2], get-state [3], state [4], state-root [5]

// Encode serializes a Message into its byte representation using the custom JAM codec.
// The format is: [1-byte tag][jam-encoded data]
func Encode(msg *Message) ([]byte, error) {
	var (
		tag  byte
		data interface{}
	)

	switch {
	case msg.PeerInfo != nil:
		if peerInfoV0, ok := msg.PeerInfo.(*PeerInfoV0); ok {
			tag, data = 0, *peerInfoV0
		} else {
			return nil, fmt.Errorf("invalid PeerInfo type for V0 protocol")
		}
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

// Decode parses a byte slice and reconstructs a Message using the custom JAM codec.
func Decode(data []byte) (*Message, error) {
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
		targetType = reflect.TypeOf(PeerInfoV0{})
		assign = func(v interface{}) {
			val := v.(PeerInfoV0)
			msg.PeerInfo = &val
		}
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
		return nil, fmt.Errorf("Failed to Decode msgType%d using Fuzz-V0", tag)
	}

	assign(decodedStruct)
	return msg, nil
}

// --- Fuzzer Helper Methods for V0 ---

func init() {
	protocolHandler = &V0ProtocolHandler{}
}

func (f *Fuzzer) getProtocolVersion() uint8 {
	if f.targetInfo == nil {
		return ProtocolV0
	}
	return f.targetInfo.GetProtocolVersion()
}

// For V0 builds, ancestry is not supported
func (f *Fuzzer) supportsAncestry() bool {
	return false
}

type V0ProtocolHandler struct{}

func (h *V0ProtocolHandler) Encode(msg *Message) ([]byte, error) {
	return Encode(msg)
}

func (h *V0ProtocolHandler) Decode(data []byte) (*Message, error) {
	return Decode(data)
}

func (h *V0ProtocolHandler) GetProtocolVersion() uint8 {
	return ProtocolV0
}

func CreateVersionedPeerInfo(appName string) PeerInfo {
	return &PeerInfoV0{
		JamVersion: ParseVersion(JAM_VERSION),
		Name:       appName,
		AppVersion: ParseVersion(APP_VERSION),
	}
}
