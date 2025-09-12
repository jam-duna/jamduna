//go:build asn0r
// +build asn0r

package fuzz

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// PeerInfoV0r ::= SEQUENCE {fuzz-version, app-version, jam-version, features, name}
type PeerInfoV0r struct {
	FuzzVersion uint8    `json:"fuzz_version"`
	AppVersion  Version  `json:"app_version"`
	JamVersion  Version  `json:"jam_version"`
	Features    Features `json:"features"`
	Name        string   `json:"name"`
}

// Implement PeerInfo interface
func (p *PeerInfoV0r) GetName() string           { return p.Name }
func (p *PeerInfoV0r) GetAppVersion() Version    { return p.AppVersion }
func (p *PeerInfoV0r) GetJamVersion() Version    { return p.JamVersion }
func (p *PeerInfoV0r) GetProtocolVersion() uint8 { return ProtocolV0r }
func (p *PeerInfoV0r) GetFuzzVersion() uint8     { return p.FuzzVersion }
func (p *PeerInfoV0r) GetFeatures() Features     { return p.Features }
func (p *PeerInfoV0r) SetDefaults() {
	p.FuzzVersion = uint8(1)
	p.Features = FeatureBundleRefinement
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
func (p *PeerInfoV0r) PrettyString(isIndented bool) string {
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
func (p *PeerInfoV0r) Info() string {
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

// --- V0r JAM Codec Functions ---
// V0r tag mapping: peer-info [0], import-block [1], set-state [2], get-state [3], state [4], state-root [5], refine-bundle [6], work-report [7], get-exports [8], segments [9]

// Encode serializes a Message into its byte representation using the custom JAM codec.
// The format is: [1-byte tag][jam-encoded data]
func Encode(msg *Message) ([]byte, error) {
	var (
		tag  byte
		data interface{}
	)

	switch {
	case msg.PeerInfo != nil:
		if peerInfoV0r, ok := msg.PeerInfo.(*PeerInfoV0r); ok {
			tag, data = 0, *peerInfoV0r
		} else {
			return nil, fmt.Errorf("invalid PeerInfo type for V0r protocol")
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
	case msg.RefineBundle != nil:
		tag, data = 6, *msg.RefineBundle
	case msg.WorkReport != nil:
		tag, data = 7, *msg.WorkReport
	case msg.GetExports != nil:
		tag, data = 8, *msg.GetExports
	case msg.Segments != nil:
		tag, data = 9, *msg.Segments
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
		targetType = reflect.TypeOf(PeerInfoV0r{})
		assign = func(v interface{}) {
			val := v.(PeerInfoV0r)
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
	case 6:
		targetType = reflect.TypeOf(types.RefineBundle{})
		assign = func(v interface{}) { val := v.(types.RefineBundle); msg.RefineBundle = &val }
	case 7:
		targetType = reflect.TypeOf(types.WorkReport{})
		assign = func(v interface{}) { val := v.(types.WorkReport); msg.WorkReport = &val }
	case 8:
		targetType = reflect.TypeOf(common.Hash{})
		assign = func(v interface{}) { val := v.(common.Hash); msg.GetExports = &val }
	case 9:
		targetType = reflect.TypeOf([][]byte{})
		assign = func(v interface{}) { val := v.([][]byte); msg.Segments = &val }
	default:
		return nil, fmt.Errorf("unknown message tag: %d", tag)
	}

	decodedStruct, _, err := types.Decode(encodedBody, targetType)
	if err != nil {
		return nil, fmt.Errorf("Failed to Decode msgType%d using Fuzz-V0R", tag)

	}

	assign(decodedStruct)
	return msg, nil
}

// --- Fuzzer Helper Methods for V0r ---

func init() {
	protocolHandler = &V0rProtocolHandler{}
}

func (f *Fuzzer) getProtocolVersion() uint8 {
	if f.targetInfo == nil {
		return ProtocolV0r // Default to V0r for V0r builds
	}
	return f.targetInfo.GetProtocolVersion()
}

// supportsAncestry checks if target supports ancestry feature
// For V0r builds, check if FeatureBlockAncestry is set
func (f *Fuzzer) supportsAncestry() bool {
	if f.targetInfo == nil {
		return false
	}
	// V0r has Features field like V1, but is not V1 protocol
	return (f.targetInfo.GetFeatures() & FeatureBlockAncestry) != 0
}

type V0rProtocolHandler struct{}

func (h *V0rProtocolHandler) Encode(msg *Message) ([]byte, error) {
	return Encode(msg)
}

func (h *V0rProtocolHandler) Decode(data []byte) (*Message, error) {
	return Decode(data)
}

func (h *V0rProtocolHandler) GetProtocolVersion() uint8 {
	return ProtocolV0r
}
