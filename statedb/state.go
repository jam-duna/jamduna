package statedb

import (
	//"crypto/rand"
	//"fmt"
	//"math/big"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	//"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

type JamState struct {
	AuthorizationsPool       [types.TotalCores][types.MaxAuthorizationPoolItems]common.Hash  `json:"alpha"` // The core αuthorizations pool. α eq 85
	BeefyPool                [types.RecentHistorySize]Beta_state                             `json:"beta"`  // The core βeefy pool. β eq 81
	SafroleStateGamma        SafroleBasicState                                               `json:"gamma"` // SafroleBasicState γ eq 48
	SafroleState             *SafroleState                                                   `json:"safrole"`
	PriorServiceAccountState map[uint32]types.ServiceAccount                                 `json:"delta"` // The (prior) state of the service accounts. δ eq 89
	AvailabilityAssignments  [types.TotalCores]*Rho_state                                    `json:"rho"`   // AvailabilityAssignments ρ eq 118
	AuthorizationQueue       [types.TotalCores][types.MaxAuthorizationQueueItems]common.Hash `json:"phi"`   // The authorization queue  φ eq 85
	DisputesState            Psi_state                                                       `json:"psi"`   // Disputes ψ eq 97
	PrivilegedServiceIndices Kai_state                                                       `json:"kai"`   // The privileged service indices. χ eq 96
	ValidatorStatistics      [2][types.TotalValidators]Pi_state                              `json:"pi"`    // The validator statistics. π eq 171
}

// Types for Beta
type Beta_state struct {
	H common.Hash                   `json:"h"`
	B []common.Hash                 `json:"b"`
	S common.Hash                   `json:"s"`
	P [types.TotalCores]common.Hash `json:"p"`
}

// Types for Psi
type Psi_state struct {
	Psi_g [][]byte          `json:"psi_g"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_b [][]byte          `json:"psi_b"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_w [][]byte          `json:"psi_w"` // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_o []types.PublicKey `json:"psi_o"` // SEQUENCE OF Ed25519Key (ByteArray32 in disputes.asn)
}

// Types for Rho
type Rho_state struct {
	WorkReport types.WorkReport `json:"workreport"`
	Timeslot   uint32           `json:"timeslot"`
}

// Types for Gamma
type TicketsOrKeys struct {
	Tickets []*types.TicketBody `json:"tickets,omitempty"`
	Keys    []common.Hash       `json:"keys,omitempty"` //BandersnatchKey
}

type SafroleBasicState struct {
	GammaK []types.Validator  `json:"gamma_k"` // γk: Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	GammaZ []byte             `json:"gamma_z"` // γz: Epoch’s root, a Bandersnatch ring root composed with one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	GammaS TicketsOrKeys      `json:"gamma_s"` // γs: Current epoch’s slot-sealer series (epoch N)
	GammaA []types.TicketBody `json:"gamma_a"` // γa: Ticket accumulator for the next epoch (epoch N+1)
}

// Types for Kai
type Kai_state struct {
	Kai_m uint32 `json:"kai_m"` // The index of the empower service
	Kai_a uint32 `json:"kai_a"` // The index of the designate service
	Kai_v uint32 `json:"kai_v"` // The index of the assign service
}

// Types for Pi
type Pi_state struct {
	BlockNumber        uint64 `json:"block_number"`        // The number of blocks produced by the validator.
	TicketNumber       uint64 `json:"ticket_number"`       // The number of tickets introduced by the validator.
	PreimageNumber     uint64 `json:"preimage_number"`     // The number of preimages introduced by the validator.
	OctetsNumber       uint64 `json:"octets_number"`       // The total number of octets across all preimages introduced by the validator.
	ReportNumber       uint64 `json:"report_number"`       // The number of reports guaranteed by the validator.
	AvailabilityNumber uint64 `json:"availability_number"` // The number of availability assurances made by the validator.
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
func (state *JamState) clearRhoByCore(core uint32) {
	state.AvailabilityAssignments[core] = nil
}

// setRhoByWorkReport sets the Rho state for a specific core with a WorkReport and timeslot
func (state *JamState) setRhoByWorkReport(core uint32, w types.WorkReport, t uint32) {
	state.AvailabilityAssignments[core] = &Rho_state{
		WorkReport: w,
		Timeslot:   t,
	}
}

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
	vm := pvm.NewVMFromCode(code, 0, nil) // Assuming `nil` for HostEnv
	err := vm.Execute()
	if err != nil {
		return types.AccumulationState{}, err
	}

	return args, nil
}

// Tally updates the statistics for validators based on their activities.
// func (n *JamState) Tally(validatorIndex int, activity string, count int) {
// 	switch activity {
// 	case "blocks":
// 		n.CurrentEpochStats[validatorIndex].BlocksProduced += count
// 	case "tickets":
// 		n.CurrentEpochStats[validatorIndex].TicketsIntroduced += count
// 	case "preimages":
// 		n.CurrentEpochStats[validatorIndex].PreimagesIntroduced += count
// 	case "octets":
// 		n.CurrentEpochStats[validatorIndex].OctetsIntroduced += count
// 	case "reports":
// 		n.CurrentEpochStats[validatorIndex].ReportsGuaranteed += count
// 	case "assurances":
// 		n.CurrentEpochStats[validatorIndex].AvailabilityAssurances += count
// 	default:
// 		fmt.Println("Unknown activity:", activity)
// 	}
// }
