//go:build asn1
// +build asn1

package fuzz

import (
	"encoding/json"
	"fmt"
)
// Features is a u32 where the MSB (bit 31) is reserved for future extensions
type Features uint32

// Feature bit constants
const (
	FeatureBlockAncestry    Features = 1          // 2^0
	FeatureSimpleForking    Features = 2          // 2^1
	FeatureBundleRefinement Features = 4          // 2^2
	FeatureExports          Features = 8          // 2^3
	FeatureExtension        Features = 2147483648 // 2^31 = 0x80000000
)

// PeerInfo ::= SEQUENCE {fuzz-version, app-version, jam-version, features, name}
type PeerInfo struct {
	FuzzVersion uint8    `json:"fuzz_version"`
	AppVersion  Version  `json:"app_version"`
	JamVersion  Version  `json:"jam_version"`
	Features    Features `json:"features"`
	Name        string   `json:"name"`
}

func (p *PeerInfo) SetASNSpecific() {
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
func (p PeerInfo) PrettyString(isIndented bool) string {
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
func (p PeerInfo) Info() string {
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
