package types

import (
	"github.com/colorfulnotion/jam/common"
)

// T.P.G.A.D
type ExtrinsicData struct {
	Tickets    []Ticket    `json:"tickets"`
	Disputes   Dispute     `json:"disputes"`
	Preimages  []Preimages `json:"preimages"`
	Assurances []Assurance `json:"assurances"`
	Guarantees []Guarantee `json:"guarantees"`
}

func NewExtrinsic() ExtrinsicData {
	return ExtrinsicData{}
}

func (e *ExtrinsicData) Bytes() []byte {
	enc := Encode(e)
	return enc
}

func (e *ExtrinsicData) Hash() common.Hash {
	data := e.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.Blake2Hash(data)
}
