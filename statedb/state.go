package statedb

import (
	"encoding/json"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/types"
)

type AuthorizationQueue [types.TotalCores][]common.Hash

// type BeefyPool [types.RecentHistorySize]Beta_state
type BeefyPool []Beta_state
type AvailabilityAssignments [types.TotalCores]*Rho_state

type JamState struct {
	AuthorizationsPool       [types.TotalCores][]common.Hash    `json:"authorizations_pool"` // alpha The core αuthorizations pool. α eq 85
	AuthorizationQueue       AuthorizationQueue                 `json:"authorization_queue"` // phi - The authorization queue  φ eq 85
	BeefyPool                BeefyPool                          `json:"beefy_pool"`          // beta - The core βeefy pool. β eq 81
	SafroleStateGamma        SafroleBasicState                  `json:"safrole_state_gamma"` // gamma - SafroleBasicState γ eq 48
	SafroleState             *SafroleState                      `json:"safrole"`
	PriorServiceAccountState map[uint32]types.ServiceAccount    `json:"prior_service_account_state"` // delta - The (prior) state of the service accounts. δ eq 89
	AvailabilityAssignments  AvailabilityAssignments            `json:"availability_assignments"`    // rho - AvailabilityAssignments ρ eq 118
	DisputesState            Psi_state                          `json:"disputes_state"`              // psi - Disputes ψ eq 97
	PrivilegedServiceIndices Kai_state                          `json:"privileged_services_indices"` // kai - The privileged service indices. χ eq 96
	ValidatorStatistics      [2][types.TotalValidators]Pi_state `json:"validator_statistics"`        // pi The validator statistics. π eq 171
}

type Peaks []*common.Hash

// Types for Beta
type MMR struct {
	Peaks Peaks `json:"peaks"`
}

// type Reported []common.Hash
type Beta_state struct {
	HeaderHash common.Hash   `json:"header_hash"`
	MMR        MMR           `json:"mmr"`
	StateRoot  common.Hash   `json:"state_root"`
	Reported   []common.Hash `json:"reported"`
}

func (b *Beta_state) MMR_Bytes() []byte {
	codec_bytes, err := json.Marshal(b.MMR)
	if err != nil {
		fmt.Println("Error serializing MMR", err)
	}
	return codec_bytes
}

// Types for Psi
type Psi_state struct {
	Psi_g [][]byte           `json:"psi_g"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_b [][]byte           `json:"psi_b"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_w [][]byte           `json:"psi_w"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_o []types.Ed25519Key `json:"psi_o"` // SEQUENCE OF Ed25519Key (ByteArray32 in disputes.asn)
}

// Types for Rho
type Rho_state struct {
	WorkReport types.WorkReport `json:"workreport"`
	Timeslot   uint32           `json:"timeout"`
}

// Types for Gamma
type TicketsOrKeys struct {
	Tickets []*types.TicketBody `json:"tickets,omitempty"`
	// Tickets *types.TicketsMark	  `json:"tickets,omitempty"`
	Keys []common.Hash `json:"keys,omitempty"` //BandersnatchKey
}

type CTicketsOrKeys struct {
	Tickets *types.TicketsMark              `json:"tickets,omitempty"`
	Keys    *[types.EpochLength]common.Hash `json:"keys,omitempty"`
}

func (t TicketsOrKeys) TicketLen() int {
	if t.Tickets != nil {
		return len(t.Tickets)
	}
	return 0
}

type SafroleBasicState struct {
	GammaK []types.Validator  `json:"gamma_k"` // γk: Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	GammaA []types.TicketBody `json:"gamma_a"` // γa: Ticket accumulator for the next epoch (epoch N+1)
	GammaS TicketsOrKeys      `json:"gamma_s"` // γs: Current epoch’s slot-sealer series (epoch N)
	GammaZ []byte             `json:"gamma_z"` // γz: Epoch’s root, a Bandersnatch ring root composed with one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
}

// Types for Kai
type Kai_state struct {
	Kai_m uint32 `json:"chi_m"` // The index of the empower service
	Kai_a uint32 `json:"chi_a"` // The index of the designate service
	Kai_v uint32 `json:"chi_v"` // The index of the assign service
}

// Types for Pi
type Pi_state struct {
	BlocksProduced         uint32 `json:"block_number"`        // The number of blocks produced by the validator.
	TicketsIntroduced      uint32 `json:"ticket_number"`       // The number of tickets introduced by the validator.
	PreimagesIntroduced    uint32 `json:"preimage_number"`     // The number of preimages introduced by the validator.
	OctetsIntroduced       uint32 `json:"octets_number"`       // The total number of octets across all preimages introduced by the validator.
	ReportsGuaranteed      uint32 `json:"report_number"`       // The number of reports guaranteed by the validator.
	AvailabilityAssurances uint32 `json:"availability_number"` // The number of availability assurances made by the validator.
}

func NewJamState() *JamState {
	return &JamState{
		PriorServiceAccountState: make(map[uint32]types.ServiceAccount),
		//AvailabilityAssignments:  make([types.TotalCores]*Rho_state),
		SafroleState: NewSafroleState(),
	}
}

// Copy creates a deep copy of the JamState struct
func (original *JamState) Copy() *JamState {
	copyState := &JamState{
		AuthorizationsPool:       original.AuthorizationsPool,
		BeefyPool:                original.BeefyPool,
		SafroleStateGamma:        original.SafroleStateGamma,
		DisputesState:            original.DisputesState,
		PrivilegedServiceIndices: original.PrivilegedServiceIndices,
		ValidatorStatistics:      original.ValidatorStatistics,
		SafroleState:             original.SafroleState.Copy(),
		PriorServiceAccountState: make(map[uint32]types.ServiceAccount),
		//AvailabilityAssignments:  make([types.TotalCores]*Rho_state),
		AuthorizationQueue: original.AuthorizationQueue,
	}

	for key, value := range original.PriorServiceAccountState {
		copyState.PriorServiceAccountState[key] = value
	}

	for i, rhoState := range original.AvailabilityAssignments {
		if rhoState != nil {
			copyState.AvailabilityAssignments[i] = &Rho_state{
				WorkReport: rhoState.WorkReport,
				Timeslot:   rhoState.Timeslot,
			}
		}
	}

	return copyState
}

// clearRhoByCore clears the Rho state for a specific core
// func clearRhoByCore(core uint32, cores map[uint32]*Rho_state) (r *Rho_state) {
// 	r = cores[core]=nil

// 	state.AvailabilityAssignments[core] = nil
// 	return r
// }

// Accumulate performs the accumulation of a single service.
func (state *JamState) Accumulate(serviceIndex int, accumulationState types.AccumulationState) (types.AccumulationState, error) {
	// Wrangle results for the service (simplified for demonstration)
	//wrangledResults := state.SafroleState.CurrValidators // Assuming wrangled results are the current validators

	// Calculate gas limit for the service
	// TODO: gasLimit := 1000

	// Create the arguments for the VM invocation
	args := types.AccumulationState{
		ServiceIndices: []int{serviceIndex},
		//WorkReports:       wrangledResults,
		DeferredTransfers: accumulationState.DeferredTransfers,
	}

	// Call the virtual machine
	code := []byte{}
	vm := pvm.NewVMFromCode(uint32(serviceIndex), code, 0, nil) // Assuming `nil` for HostEnv
	err := vm.Execute(types.EntryPointAccumulate)
	if err != nil {
		return types.AccumulationState{}, err
	}

	return args, nil
}

// tallyStatistics updates the statistics for validators based on their activities.
func (n *JamState) tallyStatistics(validatorIndex uint32, activity string, cnt uint32) {
	switch activity {
	case "blocks":
		n.ValidatorStatistics[0][validatorIndex].BlocksProduced += cnt
	case "tickets":
		n.ValidatorStatistics[0][validatorIndex].TicketsIntroduced += cnt
	case "preimages":
		n.ValidatorStatistics[0][validatorIndex].PreimagesIntroduced += cnt
	case "octets":
		n.ValidatorStatistics[0][validatorIndex].OctetsIntroduced += cnt
	case "reports":
		n.ValidatorStatistics[0][validatorIndex].ReportsGuaranteed += cnt
	case "assurances":
		n.ValidatorStatistics[0][validatorIndex].AvailabilityAssurances += cnt
	default:
		fmt.Println("Unknown activity:", activity)
	}
}

// func (n *JamState) GetAuthQueueBytes() ([]byte, error) {
// 	// AuthorizationsPool
// 	codec_bytes, err := json.Marshal(n.AuthorizationsPool)
// 	if err != nil {
// 		fmt.Println("Error serializing AuthQueue", err)
// 	}
// 	return codec_bytes, nil
// }

// func (n *JamState) GetPrivilegedServicesIndicesBytes() ([]byte, error) {
// 	// PrivilegedServiceIndices
// 	codec_bytes, err := json.Marshal(n.PrivilegedServiceIndices)
// 	if err != nil {
// 		fmt.Println("Error serializing AuthQueue", err)
// 	}
// 	return codec_bytes, nil
// }

// func (n *JamState) GetRecentBlocksBytes() ([]byte, error) {
// 	// BeefyPool
// 	codec_bytes, err := json.Marshal(n.BeefyPool)
// 	if err != nil {
// 		fmt.Println("Error serializing RecentBlocks", err)
// 	}
// 	return codec_bytes, nil
// }

// func (n *JamState) GetPiBytes() ([]byte, error) {
// 	// use scale to encode the Rho_state
// 	//use json marshal to get the bytes
// 	codec_bytes, err := json.Marshal(n.ValidatorStatistics)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return codec_bytes, nil
// }

// func (j *JamState) GetRhoBytes() ([]byte, error) {
// 	// use scale to encode the Rho_state
// 	//use json marshal to get the bytes
// 	codec_bytes, err := json.Marshal(j.AvailabilityAssignments)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return codec_bytes, nil
// }

func (a *Psi_state) UnmarshalJSON(data []byte) error {
	var s struct {
		Psi_g []string `json:"psi_g"`
		Psi_b []string `json:"psi_b"`
		Psi_w []string `json:"psi_w"`
		Psi_o []string `json:"psi_o"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	for _, v := range s.Psi_g {
		a.Psi_g = append(a.Psi_g, common.FromHex(v))
	}
	for _, v := range s.Psi_b {
		a.Psi_b = append(a.Psi_b, common.FromHex(v))
	}
	for _, v := range s.Psi_w {
		a.Psi_w = append(a.Psi_w, common.FromHex(v))
	}
	for _, v := range s.Psi_o {
		a.Psi_o = append(a.Psi_o, types.Ed25519Key(common.FromHex(v)))
	}

	return nil
}

func (a Psi_state) MarshalJSON() ([]byte, error) {
	psi_g := []string{}
	for _, v := range a.Psi_g {
		psi_g = append(psi_g, common.HexString(v))
	}
	psi_b := []string{}
	for _, v := range a.Psi_b {
		psi_b = append(psi_b, common.HexString(v))
	}
	psi_w := []string{}
	for _, v := range a.Psi_w {
		psi_w = append(psi_w, common.HexString(v))
	}
	psi_o := []string{}
	for _, v := range a.Psi_o {
		psi_o = append(psi_o, common.HexString(v[:]))
	}
	return json.Marshal(&struct {
		Psi_g []string `json:"psi_g"`
		Psi_b []string `json:"psi_b"`
		Psi_w []string `json:"psi_w"`
		Psi_o []string `json:"psi_o"`
	}{
		Psi_g: psi_g,
		Psi_b: psi_b,
		Psi_w: psi_w,
		Psi_o: psi_o,
	})
}
