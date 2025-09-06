//go:build !asn1
// +build !asn1

package fuzz

import (
	"encoding/json"
	"fmt"
)

// PeerInfo ::= SEQUENCE {name, app-version, fuzz-version}
type PeerInfo struct {
	Name       string  `json:"name"`
	AppVersion Version `json:"app_version"`
	JamVersion Version `json:"jam_version"`
}

func (p *PeerInfo) SetASNSpecific() {}

// PeerInfoDisplay represents PeerInfo for JSON output
type PeerInfoDisplay struct {
	Name       string `json:"Name"`
	AppVersion string `json:"AppVersion"`
	JamVersion string `json:"JamVersion"`
}

// PrettyString formats PeerInfo as indented JSON for display
func (p PeerInfo) PrettyString(isIndented bool) string {
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
func (p PeerInfo) Info() string {
	return fmt.Sprintf(`  Name: %s
  AppVersion: %d.%d.%d
  JAMVersion: %d.%d.%d`,
		p.Name,
		p.AppVersion.Major, p.AppVersion.Minor, p.AppVersion.Patch,
		p.JamVersion.Major, p.JamVersion.Minor, p.JamVersion.Patch)
}
