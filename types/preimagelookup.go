package types

import (
//"github.com/colorfulnotion/jam/common"
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
