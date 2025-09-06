package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// RefineBundle is the payload for a refine-bundle message.
// Corresponds to: RefineBundle ::= SEQUENCE { core, bundle, segment-root-mappings, auth-gas-used, auth-trace}
type RefineBundle struct {
	Core                uint16            `json:"core"`
	Bundle              WorkPackageBundle `json:"bundle"`
	SegmentRootMappings SegmentRootLookup `json:"segment_root_mappings"`
	AuthGasUsed         uint              `json:"auth_gas_used"`
	AuthTrace           []byte            `json:"auth_trace"`
}

// MarshalJSON custom marshals RefineBundle with hex-encoded AuthTrace
func (rb RefineBundle) MarshalJSON() ([]byte, error) {
	type Alias RefineBundle
	return json.Marshal(&struct {
		*Alias
		AuthTrace string `json:"auth_trace"`
	}{
		Alias:     (*Alias)(&rb),
		AuthTrace: "0x" + hex.EncodeToString(rb.AuthTrace),
	})
}

// UnmarshalJSON custom unmarshals RefineBundle with hex-decoded AuthTrace
func (rb *RefineBundle) UnmarshalJSON(data []byte) error {
	type Alias RefineBundle
	aux := &struct {
		*Alias
		AuthTrace string `json:"auth_trace"`
	}{
		Alias: (*Alias)(rb),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Handle hex-encoded AuthTrace
	if aux.AuthTrace != "" {
		// Remove "0x" prefix if present
		hexStr := strings.TrimPrefix(aux.AuthTrace, "0x")
		decoded, err := hex.DecodeString(hexStr)
		if err != nil {
			return err
		}
		rb.AuthTrace = decoded
	}

	return nil
}

func (rb RefineBundle) String() string {
	jsonBytes, err := json.Marshal(rb)
	if err != nil {
		return fmt.Sprintf("RefineBundle{error: %v}", err)
	}
	return string(jsonBytes)
}

func (rb RefineBundle) StringL() string {
	jsonBytes, err := json.MarshalIndent(rb, "", "  ")
	if err != nil {
		return fmt.Sprintf("RefineBundle{error: %v}", err)
	}
	return string(jsonBytes)
}
