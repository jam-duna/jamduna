package types

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

// T.P.G.A.D
type ExtrinsicData struct {
	Tickets         []Ticket         `json:"tickets"`
	PreimageLookups []PreimageLookup `json:"preimage_lookups"`
	Guarantees      []Guarantee      `json:"guarantees"`
	Assurances      []Assurance      `json:"assurances"`
	Disputes        []Dispute        `json:"disputes"`
}

func NewExtrinsic() ExtrinsicData {
	return ExtrinsicData{}
}

func (e *ExtrinsicData) Bytes() []byte {
	enc, err := json.Marshal(e)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (e *ExtrinsicData) Hash() common.Hash {
	data := e.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}
