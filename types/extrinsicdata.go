package types

import (
	"fmt"

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

const (
	debugExtrinsicHash = false
)

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
	// Encode Tickets (E_T)
	E_T_encoded, err := Encode(e.Tickets)
	if err != nil {
		return common.Hash{}
	}
	// Encode Preimages (E_P)
	E_P_encoded, err := Encode(e.Preimages)
	if err != nil {
		return common.Hash{}
	}
	// Process Guarantees: encode each guarantee after converting to its hashed version (E_G)
	var guranteesHashed []GuaranteeHashed
	for _, g := range e.Guarantees {
		gh := g.ToGuaranteeHashed()
		guranteesHashed = append(guranteesHashed, gh)
	}

	E_G_encoded, err := Encode(guranteesHashed)
	if err != nil {
		return common.Hash{}
	}
	// Encode Assurances (E_A)
	E_A_encoded, err := Encode(e.Assurances)
	if err != nil {
		return common.Hash{}
	}
	// Encode Disputes (E_D)
	E_D_encoded, err := Encode(e.Disputes)
	if err != nil {
		return common.Hash{}
	}
	// Hash each component using Blake2
	E_T_hash := common.Blake2Hash(E_T_encoded)
	E_P_hash := common.Blake2Hash(E_P_encoded)
	E_G_hash := common.Blake2Hash(E_G_encoded)
	E_A_hash := common.Blake2Hash(E_A_encoded)
	E_D_hash := common.Blake2Hash(E_D_encoded)

	// Combine the five 32-byte hashes into a fixed array
	var data [5][32]byte
	copy(data[0][:], E_T_hash.Bytes()[:])
	copy(data[1][:], E_P_hash.Bytes()[:])
	copy(data[2][:], E_G_hash.Bytes()[:])
	copy(data[3][:], E_A_hash.Bytes()[:])
	copy(data[4][:], E_D_hash.Bytes()[:])

	// Encode the combined array
	data_encoded, err := Encode(data)
	if err != nil {
		fmt.Printf("Error encoding combined data: %v\n", err)
		return common.Hash{}
	}

	// Compute the final extrinsic hash from the encoded combined data
	finalHash := common.Blake2Hash(data_encoded)
	return finalHash
}
