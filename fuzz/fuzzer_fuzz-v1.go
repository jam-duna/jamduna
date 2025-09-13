//go:build asn1
// +build asn1

package fuzz

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// PeerInfoV1 ::= SEQUENCE {fuzz-version, features, app-version, jam-version, name}
type PeerInfoV1 struct {
	FuzzVersion uint8    `json:"fuzz_version"`
	Features    Features `json:"features"`
	AppVersion  Version  `json:"app_version"`
	JamVersion  Version  `json:"jam_version"`
	Name        string   `json:"name"`
}

// Implement PeerInfo interface
func (p *PeerInfoV1) GetName() string           { return p.Name }
func (p *PeerInfoV1) GetAppVersion() Version    { return p.AppVersion }
func (p *PeerInfoV1) GetJamVersion() Version    { return p.JamVersion }
func (p *PeerInfoV1) GetProtocolVersion() uint8 { return ProtocolV1 }
func (p *PeerInfoV1) GetFuzzVersion() uint8     { return p.FuzzVersion }
func (p *PeerInfoV1) GetFeatures() Features     { return p.Features }
func (p *PeerInfoV1) SetDefaults() {
	p.FuzzVersion = uint8(1)
	//p.Features = FeatureBundleRefinement
	p.Features = FeatureBlockAncestry | FeatureSimpleForking
}

// FeaturesDisplay represents feature flags for JSON output
type FeaturesDisplay struct {
	BlockAncestry    bool `json:"BlockAncestry"`
	SimpleForking    bool `json:"SimpleForking"`
	BundleRefinement bool `json:"BundleRefinement"`
	Exports          bool `json:"Exports"`
	Extension        bool `json:"Extension"`
}

// PeerInfoDisplay represents PeerInfo for JSON output
type PeerInfoDisplay struct {
	FuzzVersion int             `json:"FuzzVersion"`
	AppVersion  string          `json:"AppVersion"`
	JamVersion  string          `json:"JamVersion"`
	Features    FeaturesDisplay `json:"Features"`
	Name        string          `json:"Name"`
}

// PrettyString formats PeerInfo as indented JSON for display
func (p *PeerInfoV1) PrettyString(isIndented bool) string {
	display := PeerInfoDisplay{
		FuzzVersion: int(p.FuzzVersion),
		AppVersion:  fmt.Sprintf("%d.%d.%d", p.AppVersion.Major, p.AppVersion.Minor, p.AppVersion.Patch),
		JamVersion:  fmt.Sprintf("%d.%d.%d", p.JamVersion.Major, p.JamVersion.Minor, p.JamVersion.Patch),
		Features: FeaturesDisplay{
			BlockAncestry:    (p.Features & FeatureBlockAncestry) != 0,
			SimpleForking:    (p.Features & FeatureSimpleForking) != 0,
			BundleRefinement: (p.Features & FeatureBundleRefinement) != 0,
			Exports:          (p.Features & FeatureExports) != 0,
			Extension:        (p.Features & FeatureExtension) != 0,
		},
		Name: p.Name,
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
func (p *PeerInfoV1) Info() string {
	return fmt.Sprintf(`  Name: %s
  FuzzVersion: %d
  AppVersion: %d.%d.%d
  JAMVersion: %d.%d.%d
  Features: BlockAncestry=%v, SimpleForking=%v, BundleRefinement=%v, Export=%v, Extension=%v`,
		p.Name,
		p.FuzzVersion,
		p.AppVersion.Major, p.AppVersion.Minor, p.AppVersion.Patch,
		p.JamVersion.Major, p.JamVersion.Minor, p.JamVersion.Patch,
		(p.Features&FeatureBlockAncestry) != 0,
		(p.Features&FeatureSimpleForking) != 0,
		(p.Features&FeatureBundleRefinement) != 0,
		(p.Features&FeatureExports) != 0,
		(p.Features&FeatureExtension) != 0)
}

// --- V1 JAM Codec Functions ---
// V1 tag mapping: peer-info [0], initialize [1], state-root [2], import-block [3], get-state [4], state [5], error [255]

// Encode serializes a Message into its byte representation using the custom JAM codec.
// The format is: [1-byte tag][jam-encoded data] (except for Error which uses 255 as tag)
func Encode(msg *Message) ([]byte, error) {
	var (
		tag  byte
		data interface{}
	)

	switch {
	case msg.PeerInfo != nil:
		if peerInfoV1, ok := msg.PeerInfo.(*PeerInfoV1); ok {
			tag, data = 0, *peerInfoV1 // [0]
		} else {
			return nil, fmt.Errorf("invalid PeerInfo type for V1 protocol")
		}
	case msg.Initialize != nil:
		tag, data = 1, *msg.Initialize // [1]
	case msg.StateRoot != nil:
		tag, data = 2, *msg.StateRoot // [2]
	case msg.ImportBlock != nil:
		tag, data = 3, *msg.ImportBlock // [3]
	case msg.GetState != nil:
		tag, data = 4, *msg.GetState // [4]
	case msg.State != nil:
		tag, data = 5, *msg.State // [5]
	case msg.Error != nil:
		// Error ::= UTF8String - encode the error message
		tag, data = 255, *msg.Error // [255]
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

	// Special case for Error message (tag 255) - Error ::= UTF8String
	if tag == 255 {
		errorMsg := string(encodedBody)
		msg.Error = &errorMsg
		return msg, nil
	}

	var (
		targetType reflect.Type
		assign     func(interface{})
	)

	switch tag {
	case 0: // peer-info
		targetType = reflect.TypeOf(PeerInfoV1{})
		assign = func(v interface{}) {
			val := v.(PeerInfoV1)
			msg.PeerInfo = &val
		}
	case 1: // initialize
		targetType = reflect.TypeOf(Initialize{})
		assign = func(v interface{}) { val := v.(Initialize); msg.Initialize = &val }
	case 2: // state-root
		targetType = reflect.TypeOf(common.Hash{})
		assign = func(v interface{}) { val := v.(common.Hash); msg.StateRoot = &val }
	case 3: // import-block
		targetType = reflect.TypeOf(types.Block{})
		assign = func(v interface{}) { val := v.(types.Block); msg.ImportBlock = &val }
	case 4: // get-state
		targetType = reflect.TypeOf(common.Hash{})
		assign = func(v interface{}) { val := v.(common.Hash); msg.GetState = &val }
	case 5: // state
		targetType = reflect.TypeOf(statedb.StateKeyVals{})
		assign = func(v interface{}) { val := v.(statedb.StateKeyVals); msg.State = &val }
	default:
		return nil, fmt.Errorf("unknown message tag: %d", tag)
	}

	decodedStruct, _, err := types.Decode(encodedBody, targetType)
	if err != nil {
		return nil, fmt.Errorf("Failed to Decode msgType%d using Fuzz-V1", tag)

	}

	assign(decodedStruct)
	return msg, nil
}

// --- Fuzzer Helper Methods for V1 ---

func init() {
	protocolHandler = &V1ProtocolHandler{}
}

// getProtocolVersion returns the protocol version of the target
func (f *Fuzzer) getProtocolVersion() uint8 {
	if f.targetInfo == nil {
		return ProtocolV1
	}
	return f.targetInfo.GetProtocolVersion()
}

func (f *Fuzzer) supportsAncestry() bool {
	if f.targetInfo == nil {
		return false
	}
	// For V1, check FeatureBlockAncestry flag
	return (f.targetInfo.GetFeatures() & FeatureBlockAncestry) != 0
}

type V1ProtocolHandler struct{}

func (h *V1ProtocolHandler) Encode(msg *Message) ([]byte, error) {
	return Encode(msg)
}

func (h *V1ProtocolHandler) Decode(data []byte) (*Message, error) {
	return Decode(data)
}

func (h *V1ProtocolHandler) GetProtocolVersion() uint8 {
	return ProtocolV1
}
