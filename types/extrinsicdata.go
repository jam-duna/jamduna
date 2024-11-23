package types

import (
	"github.com/colorfulnotion/jam/common"
)

// T.P.G.A.D
type ExtrinsicData struct {
	Tickets    []Ticket    `json:"tickets"`
	Preimages  []Preimages `json:"preimages"`
	Guarantees []Guarantee `json:"guarantees"`
	Assurances []Assurance `json:"assurances"`
	Disputes   Dispute     `json:"disputes"`
}

func NewExtrinsic() ExtrinsicData {
	return ExtrinsicData{}
}

func (e *ExtrinsicData) Bytes() []byte {
	enc, err := Encode(e)
	if err != nil {
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
	return common.Blake2Hash(data)
}
