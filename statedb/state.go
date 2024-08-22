package statedb

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type JamState struct {
	Alpha  [types.C][types.O]common.Hash   `json:"alpha"`  // The core αuthorizations pool. α eq 85
	Beta   [types.H]Beta_state             `json:"beta"`   // The core βeefy pool. β eq 81
	Gamma  SafroleBasicState               `json:"gamma"`  // SafroleBasicState γ eq 48
	Delta  map[uint32]types.ServiceAccount `json:"delta"`  // The (prior) state of the service accounts. δ eq 89
	Eta    []common.Hash                   `json:"eta"`    // entropy η eq 66
	Iota   []types.Validator               `json:"iota"`   // The validator keys and metadata to be drawn from next ι eq 66
	Kappa  []types.Validator               `json:"kappa"`  // The validator κeys and metadata currently active. κ eq 66
	Lambda []types.Validator               `json:"lambda"` // The validator keys and metadata which were active in the prior epoch λ eq 66
	Rho    []Rho_state                     `json:"rho"`    // AvailabilityAssignments ρ eq 118
	Tau    uint32                          `json:"tau"`    // timeslot
	Phi    [types.C][types.Q]common.Hash   `json:"phi"`    // The authorization queue  φ eq 85
	Psi    Psi_state                       `json:"psi"`    // Disputes ψ eq 97
	Kai    Kai_state                       `json:"kai"`    // The privileged service indices. χ eq 96
	Pi     [2][types.V]Pi_state            `json:"pi"`     // The validator statistics. π eq 171
}

// types for Beta
type Beta_state struct {
	H common.Hash          `json:"h"`
	B []common.Hash        `json:"b"`
	S common.Hash          `json:"s"`
	P [types.C]common.Hash `json:"p"`
}

// types for Psi
type Psi_state struct {
	Psi_g [][]byte          `json:"psi_g"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_b [][]byte          `json:"psi_b"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_w [][]byte          `json:"psi_w"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_o []types.PublicKey `json:"psi_o"` // SEQUENCE OF Ed25519Key (ByteArray32 in disputes.asn)
}

// types for Rho
type Rho_state struct {
	DummyWorkReport []byte `json:"dummy-work-report"`
	Timeout         uint32 `json:"timeout"`
}

// types for Gamma
// TicketsOrKeys represents the choice between tickets and keys
type TicketsOrKeys struct {
	Tickets []*types.TicketBody `json:"tickets,omitempty"`
	Keys    []common.Hash       `json:"keys,omitempty"` //BandersnatchKey
}

/*
//γ ≡⎩γk, γz, γs, γa⎭
//γk :one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
//γz :epoch’s root, a Bandersnatch ring root composed with the one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
//γa :the ticket accumulator, a series of highest-scoring ticket identifiers to be used for the next epoch (epoch N+1)
//γs :current epoch’s slot-sealer series, which is either a full complement of E tickets or, in the case of a fallback mode, a series of E Bandersnatch keys (epoch N)
*/
type SafroleBasicState struct {

	// γk represents one Bandersnatch key of each of the next epoch’s validators (epoch N+1).
	GammaK []types.Validator `json:"gamma_k"`

	// γz represents the epoch’s root, a Bandersnatch ring root composed with one Bandersnatch key of each of the next epoch’s validators (epoch N+1).
	GammaZ []byte `json:"gamma_z"`

	// γs represents the current epoch’s slot-sealer series, which is either a full complement of E tickets or, in the case of fallback mode, a series of E Bandersnatch keys (epoch N).
	GammaS TicketsOrKeys `json:"gamma_s"`

	// γa represents the ticket accumulator, a series of least-scoring ticket identifiers to be used for the next epoch (epoch N+1).
	GammaA []types.TicketBody `json:"gamma_a"`
}

// types for Kai
type Kai_state struct {
	Kai_m uint32 `json:"kai_m"` //The index of the empower service
	Kai_a uint32 `json:"kai_a"` //The index of the designate service
	Kai_v uint32 `json:"kai_v"` //The index of the assign service
}

// types for Pi
// eq171
type Pi_state struct {
	BlockNumber        uint64 `json:"block_number"`    //The number of blocks produced by the validator.
	TicketNumber       uint64 `json:"ticket_number"`   //The number of tickets introduced by the validator
	PreimageNumber     uint64 `json:"preimage_number"` //The number of preimages introduced by the validator
	OctetsNumber       uint64 `json:"octets_number"`   //The total number of octets across all preimages introduced by the validator.
	ReportNumber       uint64 `json:"report_number"`   //The number of reports guaranteed by the validator
	AvailabilityNumber uint64 `json:"avalible_number"` //The number of availability assurances made by the validator
}
