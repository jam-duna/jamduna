package statedb

import (
	"encoding/json"
	"fmt"
	"reflect"

	"maps"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

type AvailabilityAssignments [types.TotalCores]*CoreState

type JamState struct {
	AuthorizationsPool       [types.TotalCores][]common.Hash              `json:"authorizations_pool"`         // alpha The core αuthorizations pool. α eq 85
	AuthorizationQueue       types.AuthorizationQueue                     `json:"authorization_queue"`         // phi - The authorization queue  φ eq 85
	RecentBlocks             RecentBlocks                                 `json:"beefy_pool"`                  // beta - The core βeefy pool. β eq 81
	SafroleBasicState        SafroleBasicState                            `json:"safrole_state_gamma"`         // gamma - SafroleBasicState γ eq 48
	SafroleState             *SafroleState                                `json:"safrole"`                     // safrole - SafroleState
	AvailabilityAssignments  AvailabilityAssignments                      `json:"availability_assignments"`    // availability_assignment - AvailabilityAssignments ρ eq 118
	DisputesState            DisputeState                                 `json:"disputes_state"`              // psi - Disputes ψ eq 97
	PrivilegedServiceIndices types.PrivilegedServiceState                 `json:"privileged_services_indices"` // kai - The privileged service indices. χ eq 96
	ValidatorStatistics      types.ValidatorStatistics                    `json:"pi"`                          // pi The validator statistics. π eq 171
	AccumulationQueue        [types.EpochLength][]types.AccumulationQueue `json:"ready_queue"`                 // theta - The accumulation queue  θ eq 164
	AccumulationHistory      [types.EpochLength]types.AccumulationHistory `json:"accumulated"`                 // xi - The accumulation history  ξ eq 162
	AccumulationOutputs      []types.AccumulationOutput                   `json:"theta"`                       // theta - The accumulation outputs  ω eq 163
}

func (b *HistoryState) MMR_Bytes() []byte {
	codec_bytes, err := json.Marshal(b.B)
	if err != nil {
		fmt.Println("Error serializing MMR", err)
	}
	return codec_bytes
}

// Types for DisputesState
type DisputeState struct {
	GoodSet   [][]byte           `json:"good"`      // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	BadSet    [][]byte           `json:"bad"`       // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	WonkySet  [][]byte           `json:"wonky"`     // SEQUENCE OF WorkReportHash (ByteArray32 in disputes.asn)
	Offenders []types.Ed25519Key `json:"offenders"` // SEQUENCE OF Ed25519Key (ByteArray32 in disputes.asn)
}

// Types for AvailabilityAssignments
type CoreState struct {
	WorkReport types.WorkReport `json:"report"`
	Timeslot   uint32           `json:"timeout"`
}

func (r *CoreState) ShortString() string {
	if r == nil {
		return "null"
	}
	tmp := struct {
		WPHash   common.Hash `json:"wp_hash"`
		Timeslot uint32      `json:"timeout"`
	}{
		WPHash:   r.WorkReport.GetWorkPackageHash(),
		Timeslot: r.Timeslot,
	}
	return types.ToJSON(tmp)
}

func (r *CoreState) String() string {
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

type NextValidators []types.Validator
type RingCommitment []byte

type SafroleBasicState struct {
	NextValidators    NextValidators     `json:"gamma_k"` // γk: Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	RingCommitment    RingCommitment     `json:"gamma_z"` // γz: Epoch’s root, a Bandersnatch ring root composed with one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	SlotSealerSeries  TicketsOrKeys      `json:"gamma_s"` // γs: Current epoch’s slot-sealer series (epoch N)
	TicketAccumulator []types.TicketBody `json:"gamma_a"` // γa: Ticket accumulator for the next epoch (epoch N+1)
}

func (sbs SafroleBasicState) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		NextValidators    NextValidators     `json:"gamma_k"`
		RingCommitment    string             `json:"gamma_z"`
		SlotSealerSeries  TicketsOrKeys      `json:"gamma_s"`
		TicketAccumulator []types.TicketBody `json:"gamma_a"`
	}{
		NextValidators:    sbs.NextValidators,
		RingCommitment:    common.HexString(sbs.RingCommitment),
		SlotSealerSeries:  sbs.SlotSealerSeries,
		TicketAccumulator: sbs.TicketAccumulator,
	})
}

func (sbs *SafroleBasicState) UnmarshalJSON(data []byte) error {
	var s struct {
		NextValidators    NextValidators     `json:"gamma_k"`
		RingCommitment    string             `json:"gamma_z"`
		SlotSealerSeries  TicketsOrKeys      `json:"gamma_s"`
		TicketAccumulator []types.TicketBody `json:"gamma_a"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	sbs.NextValidators = s.NextValidators
	sbs.RingCommitment = common.FromHex(s.RingCommitment)
	sbs.SlotSealerSeries = s.SlotSealerSeries
	sbs.TicketAccumulator = s.TicketAccumulator
	return nil
}

// Types for Pi

func NewJamState() *JamState {
	return &JamState{
		//AvailabilityAssignments:  make([types.TotalCores]*CoreState),
		SafroleState: NewSafroleState(),
	}
}

// Copy creates a deep copy of the JamState struct
func (original *JamState) Copy() *JamState {
	copyState := &JamState{
		AuthorizationsPool:       original.AuthorizationsPool,
		RecentBlocks:             original.RecentBlocks,
		SafroleBasicState:        original.SafroleBasicState,
		DisputesState:            original.DisputesState,
		PrivilegedServiceIndices: original.PrivilegedServiceIndices.Copy(),
		ValidatorStatistics:      *original.ValidatorStatistics.Copy(),
		SafroleState:             original.SafroleState.Copy(),
		//AvailabilityAssignments:  make([types.TotalCores]*CoreState),
		AuthorizationQueue:  original.AuthorizationQueue,
		AccumulationQueue:   original.AccumulationQueue,
		AccumulationHistory: original.AccumulationHistory,
		AccumulationOutputs: original.AccumulationOutputs,
	}

	for i, availability_assignment := range original.AvailabilityAssignments {
		if availability_assignment != nil {
			copyState.AvailabilityAssignments[i] = &CoreState{
				WorkReport: availability_assignment.WorkReport,
				Timeslot:   availability_assignment.Timeslot,
			}
		}
	}

	return copyState
}

// clearAvailabilityAssignmentsByCore clears the AvailabilityAssignments state for a specific core
func (state *JamState) String() string {
	return types.ToJSON(state)
}

func (n *JamState) ResetTallyStatistics() {

	copy(n.ValidatorStatistics.Last[:], n.ValidatorStatistics.Current[:])
	n.ValidatorStatistics.Current = [types.TotalValidators]types.ValidatorStatisticState{
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
		log.Error(log.SDB, "tallyStatistics", "validatorIndex", validatorIndex, "cnt", cnt)
	}
}

func (j *JamState) newPartialState() *types.PartialState {
	p := j.PrivilegedServiceIndices
	p.AlwaysAccServiceID = make(map[uint32]uint64)
	maps.Copy(p.AlwaysAccServiceID, j.PrivilegedServiceIndices.AlwaysAccServiceID)
	return &types.PartialState{
		ServiceAccounts:    make(map[uint32]*types.ServiceAccount),
		UpcomingValidators: j.SafroleState.DesignatedValidators,
		QueueWorkReport:    j.AuthorizationQueue,
		PrivilegedState:    p,
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

func (a *DisputeState) UnmarshalJSON(data []byte) error {
	var s struct {
		GoodSet   []string `json:"good"`
		BadSet    []string `json:"bad"`
		WonkySet  []string `json:"wonky"`
		Offenders []string `json:"offenders"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	for _, v := range s.GoodSet {
		a.GoodSet = append(a.GoodSet, common.FromHex(v))
	}
	for _, v := range s.BadSet {
		a.BadSet = append(a.BadSet, common.FromHex(v))
	}
	for _, v := range s.WonkySet {
		a.WonkySet = append(a.WonkySet, common.FromHex(v))
	}
	for _, v := range s.Offenders {
		a.Offenders = append(a.Offenders, types.Ed25519Key(common.FromHex(v)))
	}

	return nil
}

func (a DisputeState) MarshalJSON() ([]byte, error) {
	goodSet := []string{}
	for _, v := range a.GoodSet {
		goodSet = append(goodSet, common.HexString(v))
	}
	badSet := []string{}
	for _, v := range a.BadSet {
		badSet = append(badSet, common.HexString(v))
	}
	wonkySet := []string{}
	for _, v := range a.WonkySet {
		wonkySet = append(wonkySet, common.HexString(v))
	}
	offenders := []string{}
	for _, v := range a.Offenders {
		offenders = append(offenders, common.HexString(v[:]))
	}
	return json.Marshal(&struct {
		GoodSet   []string `json:"good"`
		BadSet    []string `json:"bad"`
		WonkySet  []string `json:"wonky"`
		Offenders []string `json:"offenders"`
	}{
		GoodSet:   goodSet,
		BadSet:    badSet,
		WonkySet:  wonkySet,
		Offenders: offenders,
	})
}

func StateDecodeToJson(encodedBytes []byte, state string) (string, error) {
	if len(encodedBytes) == 0 {
		return "", fmt.Errorf("encodedBytes is empty")
	}
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
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(HistoryState{}))
	case "c4":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(SafroleBasicState{}))
	case "c4-gamma_s":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(TicketsOrKeys{}))
	case "c5":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(DisputeState{}))
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
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.PrivilegedServiceState{}))
	case "c13":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.ValidatorStatistics{}))
	case "c14":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf(types.ValidatorStatistics{}))
	case "c15":
		decodedStruct, _, err = types.Decode(encodedBytes, reflect.TypeOf([types.EpochLength][]types.AccumulationQueue{}))
	default:
		//fmt.Printf("StateDecodeToJson unk [%s]\n", state)
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
