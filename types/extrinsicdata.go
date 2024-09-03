package types

import (
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

// T.P.G.A.D
type ExtrinsicData struct {
	Tickets  []Ticket `json:"tickets"`
	Disputes Dispute  `json:"disputes"`
	//PreimageLookups []PreimageLookup `json:"preimage_lookups"`
	Preimages  []Preimages `json:"preimages"`
	Assurances []Assurance `json:"assurances"`
	Guarantees []Guarantee `json:"guarantees"`
}

type SExtrinsicData struct {
	Tickets    []STicket    `json:"tickets"`
	Disputes   SDispute     `json:"disputes"`
	Preimages  []SPreimages `json:"preimages"`
	Assurances []SAssurance `json:"assurances"`
	Guarantees []SGuarantee `json:"guarantees"`
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
	tickets := make([]Ticket, len(s.Tickets))
	var err error
	for i, ticket := range s.Tickets {
		tickets[i], err = ticket.Deserialize()
		if err != nil {
			return ExtrinsicData{}, err
		}
	}
	disputes, err := s.Disputes.Deserialize()
	if err != nil {
		return ExtrinsicData{}, err
	}
	preimages := make([]Preimages, len(s.Preimages))
	for i, preimage := range s.Preimages {
		preimages[i], err = preimage.Deserialize()
		if err != nil {
			return ExtrinsicData{}, err
		}
	}
	assurances := make([]Assurance, len(s.Assurances))
	for i, assurance := range s.Assurances {
		assurances[i], err = assurance.Deserialize()
		if err != nil {
			return ExtrinsicData{}, err
		}
	}
	guarantees := make([]Guarantee, len(s.Guarantees))
	for i, guarantee := range s.Guarantees {
		guarantees[i], err = guarantee.Deserialize()
		if err != nil {
			return ExtrinsicData{}, err
		}
	}

	return ExtrinsicData{
		Tickets:    tickets,
		Disputes:   disputes,
		Preimages:  preimages,
		Assurances: assurances,
		Guarantees: guarantees,
	}, nil
}
