package statedb

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type AvailabilityAssignments [types.TotalCores]*Rho_state

type JamState struct {
	AuthorizationsPool       [types.TotalCores][]common.Hash              `json:"authorizations_pool"` // alpha The core αuthorizations pool. α eq 85
	AuthorizationQueue       types.AuthorizationQueue                     `json:"authorization_queue"` // phi - The authorization queue  φ eq 85
	RecentBlocks             RecentBlocks                                 `json:"beefy_pool"`          // beta - The core βeefy pool. β eq 81
	SafroleStateGamma        SafroleBasicState                            `json:"safrole_state_gamma"` // gamma - SafroleBasicState γ eq 48
	SafroleState             *SafroleState                                `json:"safrole"`
	AvailabilityAssignments  AvailabilityAssignments                      `json:"availability_assignments"`    // rho - AvailabilityAssignments ρ eq 118
	DisputesState            Psi_state                                    `json:"disputes_state"`              // psi - Disputes ψ eq 97
	PrivilegedServiceIndices types.Kai_state                              `json:"privileged_services_indices"` // kai - The privileged service indices. χ eq 96
	ValidatorStatistics      ValidatorStatistics                          `json:"pi"`                          // pi The validator statistics. π eq 171
	AccumulationQueue        [types.EpochLength][]types.AccumulationQueue `json:"ready_queue"`                 // theta - The accumulation queue  θ eq 164
	AccumulationHistory      [types.EpochLength]types.AccumulationHistory `json:"accumulated"`                 // xi - The accumulation history  ξ eq 162
}

/*
ReadyState                    [types.EpochLength][]Ready
AccumulatedHistory       [types.EpochLength]map[common.Hash]common.Hash // work-report hash to segment-root dictionary

type Ready struct { //AccumulationQueue
WorkReport       types.WorkReport
WorkPackageHashs []common.Hash
}
*/
func (b *Beta_state) MMR_Bytes() []byte {
	codec_bytes, err := json.Marshal(b.B)
	if err != nil {
		fmt.Println("Error serializing MMR", err)
	}
	return codec_bytes
}

// Types for Psi
type Psi_state struct {
	Psi_g [][]byte           `json:"good"`      // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_b [][]byte           `json:"bad"`       // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_w [][]byte           `json:"wonky"`     // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Psi_o []types.Ed25519Key `json:"offenders"` // SEQUENCE OF Ed25519Key (ByteArray32 in disputes.asn)
}

// Types for Rho
type Rho_state struct {
	WorkReport types.WorkReport `json:"report"`
	Timeslot   uint32           `json:"timeout"`
}

func (r *Rho_state) String() string {
	return types.ToJSON(r)
}

// Types for Gamma
type TicketsOrKeys struct {
	Tickets []*types.TicketBody `json:"tickets,omitempty"` //WinningTicket for primary
	Keys    []common.Hash       `json:"keys,omitempty"`    //BandersnatchKey for fallback
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

type GammaK []types.Validator
type GammaZ []byte

type SafroleBasicState struct {
	GammaK GammaK             `json:"gamma_k"` // γk: Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	GammaZ GammaZ             `json:"gamma_z"` // γz: Epoch’s root, a Bandersnatch ring root composed with one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	GammaS TicketsOrKeys      `json:"gamma_s"` // γs: Current epoch’s slot-sealer series (epoch N)
	GammaA []types.TicketBody `json:"gamma_a"` // γa: Ticket accumulator for the next epoch (epoch N+1)
}

func (sbs SafroleBasicState) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		GammaK GammaK             `json:"gamma_k"`
		GammaZ string             `json:"gamma_z"`
		GammaS TicketsOrKeys      `json:"gamma_s"`
		GammaA []types.TicketBody `json:"gamma_a"`
	}{
		GammaK: sbs.GammaK,
		GammaZ: common.HexString(sbs.GammaZ),
		GammaS: sbs.GammaS,
		GammaA: sbs.GammaA,
	})
}

func (sbs *SafroleBasicState) UnmarshalJSON(data []byte) error {
	var s struct {
		GammaK GammaK             `json:"gamma_k"`
		GammaZ string             `json:"gamma_z"`
		GammaS TicketsOrKeys      `json:"gamma_s"`
		GammaA []types.TicketBody `json:"gamma_a"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	sbs.GammaK = s.GammaK
	sbs.GammaZ = common.FromHex(s.GammaZ)
	sbs.GammaS = s.GammaS
	sbs.GammaA = s.GammaA
	return nil
}

// Types for Pi
type ValidatorStatisticState struct {
	BlocksProduced         uint32 `json:"blocks"`          // The number of blocks produced by the validator.
	TicketsIntroduced      uint32 `json:"tickets"`         // The number of tickets introduced by the validator.
	PreimagesIntroduced    uint32 `json:"pre_images"`      // The number of preimages introduced by the validator.
	OctetsIntroduced       uint32 `json:"pre_images_size"` // The total number of octets across all preimages introduced by the validator.
	ReportsGuaranteed      uint32 `json:"guarantees"`      // The number of reports guaranteed by the validator.
	AvailabilityAssurances uint32 `json:"assurances"`      // The number of availability assurances made by the validator.
}

func NewJamState() *JamState {
	return &JamState{
		//AvailabilityAssignments:  make([types.TotalCores]*Rho_state),
		SafroleState: NewSafroleState(),
	}
}

// Copy creates a deep copy of the JamState struct
func (original *JamState) Copy() *JamState {
	copyState := &JamState{
		AuthorizationsPool:       original.AuthorizationsPool,
		RecentBlocks:             original.RecentBlocks,
		SafroleStateGamma:        original.SafroleStateGamma,
		DisputesState:            original.DisputesState,
		PrivilegedServiceIndices: original.PrivilegedServiceIndices,
		ValidatorStatistics:      *original.ValidatorStatistics.Copy(),
		SafroleState:             original.SafroleState.Copy(),
		//AvailabilityAssignments:  make([types.TotalCores]*Rho_state),
		AuthorizationQueue:  original.AuthorizationQueue,
		AccumulationQueue:   original.AccumulationQueue,
		AccumulationHistory: original.AccumulationHistory,
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
func (state *JamState) String() string {
	return types.ToJSON(state)
}

func (n *JamState) ResetTallyStatistics() {

	copy(n.ValidatorStatistics.Last[:], n.ValidatorStatistics.Current[:])
	n.ValidatorStatistics.Current = [types.TotalValidators]ValidatorStatisticState{
		{BlocksProduced: 0, TicketsIntroduced: 0, PreimagesIntroduced: 0, OctetsIntroduced: 0, ReportsGuaranteed: 0, AvailabilityAssurances: 0},
	}
}

// tallyStatistics updates the statistics for validators based on their activities.
func (n *JamState) tallyStatistics(validatorIndex uint32, activity string, cnt uint32) {
	// Update the statistics for the validator based on the activity

	switch activity {
	case "blocks":
		n.ValidatorStatistics.Current[validatorIndex].BlocksProduced += cnt
	case "tickets":
		n.ValidatorStatistics.Current[validatorIndex].TicketsIntroduced += cnt
	case "preimages":
		n.ValidatorStatistics.Current[validatorIndex].PreimagesIntroduced += cnt
	case "octets":
		n.ValidatorStatistics.Current[validatorIndex].OctetsIntroduced += cnt
	case "reports":
		n.ValidatorStatistics.Current[validatorIndex].ReportsGuaranteed += cnt
	case "assurances":
		n.ValidatorStatistics.Current[validatorIndex].AvailabilityAssurances += cnt
	default:
		fmt.Println("Unknown activity:", activity)
	}
}

func (j *JamState) newPartialState() *types.PartialState {
	return &types.PartialState{
		D:                  make(map[uint32]*types.ServiceAccount),
		UpcomingValidators: j.SafroleState.DesignedValidators,
		QueueWorkReport:    j.AuthorizationQueue,
		PrivilegedState:    j.PrivilegedServiceIndices,
	}
}

func (j *JamState) GetID() uint16 {
	return j.SafroleState.Id
}

func (j *JamState) GetValidatorStats() string {
	out := ""
	for i := 0; i < types.TotalValidators; i++ {
		v := ""
		pi := j.ValidatorStatistics.Current[i]
		if pi.BlocksProduced > 0 {
			v += fmt.Sprintf("b=%d", pi.BlocksProduced)
		}
		if pi.TicketsIntroduced > 0 {
			v += fmt.Sprintf("|t=%d", pi.TicketsIntroduced)
		}
		if pi.PreimagesIntroduced > 0 {
			v += fmt.Sprintf("|p=%d", pi.PreimagesIntroduced)
		}
		if pi.OctetsIntroduced > 0 {
			v += fmt.Sprintf("|o=%d", pi.OctetsIntroduced)
		}
		if pi.ReportsGuaranteed > 0 {
			v += fmt.Sprintf("|r=%d", pi.ReportsGuaranteed)
		}
		if pi.AvailabilityAssurances > 0 {
			v += fmt.Sprintf("|a=%d", pi.AvailabilityAssurances)
		}
		if len(v) > 0 {
			out += fmt.Sprintf("%d:[%s] ", i, v)
		}
	}
	return out

}

func (a *Psi_state) UnmarshalJSON(data []byte) error {
	var s struct {
		Psi_g []string `json:"good"`
		Psi_b []string `json:"bad"`
		Psi_w []string `json:"wonky"`
		Psi_o []string `json:"offenders"`
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
		Psi_g []string `json:"good"`
		Psi_b []string `json:"bad"`
		Psi_w []string `json:"wonky"`
		Psi_o []string `json:"offenders"`
	}{
		Psi_g: psi_g,
		Psi_b: psi_b,
		Psi_w: psi_w,
		Psi_o: psi_o,
	})
}

func StateDecodeToJson(encodedBytes []byte, state string) (string, error) {
	var decodedStruct interface{}
	var err error
	switch state {
	case "c1":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.TotalCores][]common.Hash{}))
	case "c2":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.AuthorizationQueue{}))
	case "c3":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(RecentBlocks{}))
	case "c3-beta":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(Beta_state{}))
	case "c4":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(SafroleBasicState{}))
	case "c4-gamma_s":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(TicketsOrKeys{}))
	case "c5":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(Psi_state{}))
	case "c6":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(Entropy{}))
	case "c7":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Validators{}))
	case "c8":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Validators{}))
	case "c9":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Validators{}))
	case "c10":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(AvailabilityAssignments{}))
	case "c11":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(uint32(0)))
	case "c12":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.Kai_state{}))
	case "c13":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(ValidatorStatistics{}))
	case "c14":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.EpochLength][]types.AccumulationQueue{}))
	case "c15":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.EpochLength]types.AccumulationHistory{}))
	}
	if err != nil {
		return "", err
	}
	// Convert decoded structure → JSON (indented)
	decodedJSON, err := json.MarshalIndent(decodedStruct, "", "    ")
	if err != nil {
		return "", err
	}

	return string(decodedJSON), nil
}
