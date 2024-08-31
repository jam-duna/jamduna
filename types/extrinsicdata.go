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
	Disputes        Dispute          `json:"disputes"`
}

type SExtrinsicData struct {
	Tickets         []STicket        `json:"tickets"`
	PreimageLookups []PreimageLookup `json:"preimage_lookups"`
	Guarantees      []Guarantee      `json:"guarantees"`
	Assurances      []SAssurance     `json:"assurances"`
	Disputes        SDispute         `json:"disputes"`
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

func (s *SExtrinsicData) Deserialize() (ExtrinsicData, error) {
	assurances := make([]Assurance, len(s.Assurances))
	for i, sa := range s.Assurances {
		a, err := sa.Deserialize()
		if err != nil {
			return ExtrinsicData{}, err
		}
		assurances[i] = a
	}

	tickets := make([]Ticket, len(s.Tickets))
	for i, sa := range s.Tickets {
		a, err := sa.Deserialize()
		if err != nil {
			return ExtrinsicData{}, err
		}
		tickets[i] = a
	}

	dispute, err := s.Disputes.Deserialize()
	if err != nil {
		return ExtrinsicData{}, err
	}

	return ExtrinsicData{
		Tickets:         tickets, // Assuming STicket is the same as Ticket
		PreimageLookups: s.PreimageLookups,
		Guarantees:      s.Guarantees,
		Assurances:      assurances,
		Disputes:        dispute, // Assuming Disputes is a slice of one element
	}, nil
}
