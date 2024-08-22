package types

import (
// "github.com/colorfulnotion/jam/common"
// "encoding/json"
)

// Announcement  Section 17.3 Equations (196)-(199) TBD
type Announcement struct {
	Signature Ed25519Signature `json:"signature"`
}
