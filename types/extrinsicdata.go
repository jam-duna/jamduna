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

// TODO: 0.5 style
func (e *ExtrinsicData) Hash() common.Hash {
	//in 0.5.0 ---> first taking the Hash of each component within extrinsic_data -> E_T, E_P, E_G, E_A, E_D; given this is FIXED length. I would expect the encoded data here to be 160 bytes (32*5)
	//take hash again on this to compute final extrinsic_hash
	// data := e.Bytes()
	// if data == nil {
	// 	// Handle the error case
	// 	return common.Hash{}
	// }
	E_T_encoded, err := Encode(e.Tickets)
	if err != nil {
		return common.Hash{}
	}

	E_P_encoded, err := Encode(e.Preimages)
	if err != nil {
		return common.Hash{}
	}

	guranteesHashed := [][]byte{}
	for _, g := range e.Guarantees {
		guranteeHashed := g.ToGuaranteeHashed()
		guranteesHashed = append(guranteesHashed, guranteeHashed.Bytes())
	}

	E_G_encoded, err := Encode(guranteesHashed)
	if err != nil {
		return common.Hash{}
	}

	E_A_encoded, err := Encode(e.Assurances)
	if err != nil {
		return common.Hash{}
	}

	E_D_encoded, err := Encode(e.Disputes)
	if err != nil {
		return common.Hash{}
	}

	// Hash each component
	E_T_hash := common.Blake2Hash(E_T_encoded)
	E_P_hash := common.Blake2Hash(E_P_encoded)
	E_G_hash := common.Blake2Hash(E_G_encoded)
	E_A_hash := common.Blake2Hash(E_A_encoded)
	E_D_hash := common.Blake2Hash(E_D_encoded)

	data := [5][32]byte{}

	copy(data[0][:], E_T_hash.Bytes()[:])
	copy(data[1][:], E_P_hash.Bytes()[:])
	copy(data[2][:], E_G_hash.Bytes()[:])
	copy(data[3][:], E_A_hash.Bytes()[:])
	copy(data[4][:], E_D_hash.Bytes()[:])

	data_encoded, err := Encode(data)
	if err != nil {
		return common.Hash{}
	}

	return common.Blake2Hash(data_encoded)
}
