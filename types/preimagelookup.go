package types

import (
	"github.com/colorfulnotion/jam/common"
	"encoding/json"
	"fmt"
)

/*
Section 12.1. Preimage Integration. Prior to accumulation, we must first integrate all preimages provided in the lookup extrinsic. The lookup extrinsic is a sequence of pairs of service indices and data. These pairs must be ordered and without duplicates (equation 154 requires this). The data must have been solicited by a service but not yet be provided.  See equations 153-155.

`PreimageExtrinsic` ${\bf E}_P$:
*/
// LookupEntry represents a single entry in the lookup extrinsic.
type PreimageLookup struct {
	ServiceIndex uint32 `json:"service_index"`
	Data         []byte `json:"data"`
}


func (p PreimageLookup) DeepCopy() (PreimageLookup, error) {
	var copiedPreimageLookup PreimageLookup

	// Serialize the original PreimageLookup to JSON
	data, err := json.Marshal(p)
	if err != nil {
		return copiedPreimageLookup, err
	}

	// Deserialize the JSON back into a new PreimageLookup instance
	err = json.Unmarshal(data, &copiedPreimageLookup)
	if err != nil {
		return copiedPreimageLookup, err
	}

	return copiedPreimageLookup, nil
}

// Bytes returns the bytes of the PreimageLookup.
func (p *PreimageLookup) Bytes() []byte {
	enc, err := json.Marshal(p)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (p *PreimageLookup) Hash() common.Hash {
	data := p.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}

