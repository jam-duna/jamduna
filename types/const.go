package types

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"time"
)

const (
	BlsPubInBytes  = 144
	BlsPrivInBytes = 32

	Ed25519SeedInBytes = 32
	Ed25519PrivInBytes = 64 // Seed+Pub .. typically or always?
	Ed25519PubInBytes  = 32

	MetadataSizeInBytes = 128

	ExtrinsicSignatureInBytes = 784 //RingSignatureLen
	TicketsVerifierKeyInBytes = 144
	ValidatorInByte           = 336
	EntropySize               = 4
	IETFSignatureLen          = 96
	VRFOutputLen              = 32
	GFPointsPerPage           = 2052
)

const (
	JamCommonEra = 1735732800 //1200 UTC on January 1, 2025
)

const (
	SecondsPerEpoch = EpochLength * SecondsPerSlot
)

const (
	ValidatorsSuperMajority = int(2*TotalValidators/3 + 1)
	WonkyTrueThreshold      = int(TotalValidators / 3)
	WonkyFalseThreshold     = int(TotalValidators/3 + 1)
)

// codec
const (
	validators_super_majority = TotalValidators*2/3 + 1
	Avail_bitfield_bytes      = (TotalCores + 7) / 8
)

// time mode
// three mode input => "JAM", "TimeSlot"
var TimeUnitMode = "JAM"
var TimeSavingMode bool = true

const CoreLazyMode bool = true
const (
	Network = "jamduna" // "dev" (polkajam) or "jamduna"

	MinElectiveServiceItemBalance    = 10             // B_I represents the additional minimum balance required per item of elective service state.
	MinElectiveServiceOctetBalance   = 1              // B_L represents the additional minimum balance required per octet of elective service state.
	BaseServiceBalance               = 100            // B_S = 100: The basic minimum balance which all services require.
	TotalCores                       = 2              // C: The total number of cores.
	PreimageExpiryPeriod             = 32             // D: preimage expiry period in timeslots.
	EpochLength                      = 12             // E: The length of an epoch in timeslots.
	AccumulationGasAllocation        = 10_000_000     // G_A: The total gas allocated to a core for Accumulation.
	IsAuthorizedGasAllocation        = 50000000       // G_I: The gas allocated to invoke a work-package’s Is-Authorized logic.
	RefineGasAllocation              = 1000000000     // G_R: The total gas allocated for a work-package’s Refine logic.
	AccumulateGasAllocation_GT       = 20_000_000     // G_T: The total gas allocated across all cores for Accumulation. Should be no smaller than GA ⋅ C +∑g∈V(χg )(g).
	RecentHistorySize                = 8              // H = 8: The size of recent history, in blocks.
	MaxWorkItemsPerPackage           = 16             // I = 4: The maximum amount of work items in a package.
	MaxDependencyItemsInWorkReport   = 8              // J: The maximum number of dependency items in a work-report.
	MaxTicketsPerExtrinsic           = 3              // K: The maximum number of tickets which may be submitted in a single extrinsic.
	LookupAnchorMaxAge               = 14400          // L = 14,400: The maximum age in timeslots of the lookup anchor.
	TicketEntriesPerValidator        = 3              // N: The number of ticket entries per validator.
	MaxAuthorizationPoolItems        = 8              // O: The maximum number of items in the authorizations pool.
	SecondsPerSlot                   = 6              // P = 6: The slot period, in seconds.
	MaxAuthorizationQueueItems       = 80             // Q: The maximum number of items in the authorizations queue.
	RotationPeriod                   = 4              // R
	ValidatorCoreRotationPeriod      = 4              // R: The rotation period of validator-core assignments, in timeslots.
	ExtrinsicMaximumPerPackage       = 128            //T = 128: The maximum number of extrinsics in a work-package.
	UnavailableWorkReplacementPeriod = 5              // U = 5: The period in timeslots after which reported but unavailable work may be replaced.
	TotalValidators                  = 6              // V: The total number of validators.
	MaxIsAuthorizedCodeBytes         = 64000          // W_A: Max Is Auth Code in octets.
	MaxEncodedWorkPackageSize        = 13794305       // W_B = 12 * 2^20: The maximum size of an encoded work-package together with its extrinsic data and import implications, in octets.
	MaxServiceCodeSize               = 4_000_000      // W_C - Maximum size of a service code in octets.
	ECPieceSize                      = 4              // W_E
	MaxImports                       = 3072           // W_M = 2^11: The maximum number of imports in a work-package. 0.6.5
	NumECPiecesPerSegment            = 1026           // W_P
	MaxEncodedWorkReportSize         = 48 * (1 << 10) // W_R = 96 * 2^10: The maximum size of an encoded work-report in octets.
	TransferMemoSize                 = 128            // W_T = 128: The size of a transfer memo in octets.
	MaxExports                       = 3072           // W_X = 2^11: The maximum number of exports in a work-package. 0.6.5
	TicketSubmissionEndSlot          = 10             // Y: The number of slots into an epoch at which ticket-submission ends.

	SegmentSize                      = 4104 // W_A: The size of a segment in octets.
	ContestDuration                  = 60 * 60 * 24
	PeriodSecond                     = 4       // A = 8 represents the period, in seconds, between audit tranches.
	AuditBiasFactor                  = 2       // F = 2: The audit bias factor, the expected number of additional validators who will audit a work-report in the following tranche for each no-show in the previous.
	PVMDynamicAddressAlignmentFactor = 2       // Z_A = 2: The pvm dynamic address alignment factor. See equation 227.
	PVMInitInputDataSize             = 1 << 24 // Z_I = 2^24: The standard pvm program initialization input data size. See equation A.7.
	PVMInitPageSize                  = 1 << 14 // Z_P = 2^14: The standard pvm program initialization page size. See section A.7.
	PVMInitSegmentSize               = 1 << 16 // Z_Q = 2^16: The standard pvm program initialization segment size. See section A.7.
	RecoveryThreshold                = 2
)

type Parameters struct {
	// E_8: B_I, B_L, B_S
	MinElectiveServiceItemBalance  uint64 `json:"min_elective_service_item_balance"`  // B_I
	MinElectiveServiceOctetBalance uint64 `json:"min_elective_service_octet_balance"` // B_L
	BaseServiceBalance             uint64 `json:"base_service_balance"`               // B_S

	// E_2: C
	TotalCores uint16 `json:"total_cores"` // C

	// E_4: D, E
	PreimageExpiryPeriod uint32 `json:"preimage_expiry_period"` // D
	EpochLength          uint32 `json:"epoch_length"`           // E

	// E_8: G_A, G_I, G_R, G_T
	AccumulationGasAllocation  uint64 `json:"accumulation_gas_allocation"`  // G_A
	IsAuthorizedGasAllocation  uint64 `json:"is_authorized_gas_allocation"` // G_I
	RefineGasAllocation        uint64 `json:"refine_gas_allocation"`        // G_R
	AccumulateGasAllocation_GT uint64 `json:"accumulate_gas_allocation_gt"` // G_T

	// E_2: H, I, J, K
	RecentHistorySize              uint16 `json:"recent_history_size"`                 // H
	MaxWorkItemsPerPackage         uint16 `json:"max_work_items_per_package"`          // I
	MaxDependencyItemsInWorkReport uint16 `json:"max_dependency_items_in_work_report"` // J
	MaxTicketsPerExtrinsic         uint16 `json:"max_tickets_per_extrinsic"`           // K

	// E_4: L
	LookupAnchorMaxAge uint32 `json:"lookup_anchor_max_age"` // L

	// E_2: N, O
	TicketEntriesPerValidator uint16 `json:"ticket_entries_per_validator"` // N
	MaxAuthorizationPoolItems uint16 `json:"max_authorization_pool_items"` // O

	// E_2: P, Q, R, T, U, V
	SecondsPerSlot                   uint16 `json:"seconds_per_slot"`                    // P
	MaxAuthorizationQueueItems       uint16 `json:"max_authorization_queue_items"`       // Q
	RotationPeriod                   uint16 `json:"rotation_period"`                     // R
	ExtrinsicMaximumPerPackage       uint16 `json:"extrinsic_maximum_per_package"`       // T
	UnavailableWorkReplacementPeriod uint16 `json:"unavailable_work_replacement_period"` // U
	TotalValidators                  uint16 `json:"total_validators"`                    // V

	// E_4: W_A, W_B, W_C, W_E, W_G, W_M, W_P, W_R, W_T, W_X, Y
	MaxIsAuthorizedCodeBytes  uint32 `json:"max_is_authorized_code_bytes"`  // W_A
	MaxEncodedWorkPackageSize uint32 `json:"max_encoded_work_package_size"` // W_B
	MaxServiceCodeSize        uint32 `json:"max_service_code_size"`         // W_C
	ECPieceSize               uint32 `json:"ec_piece_size"`                 // W_E
	MaxImports                uint32 `json:"max_imports"`                   // W_M
	NumECPiecesPerSegment     uint32 `json:"num_ec_pieces_per_segment"`     // W_P
	MaxEncodedWorkReportSize  uint32 `json:"max_encoded_work_report_size"`  // W_R
	TransferMemoSize          uint32 `json:"transfer_memo_size"`            // W_T
	MaxExports                uint32 `json:"max_exports"`                   // W_X
	TicketSubmissionEndSlot   uint32 `json:"ticket_submission_end_slot"`    // Y
}

func (p *Parameters) String() string {
	prettyJSON, _ := json.MarshalIndent(p, "", "  ")
	return string(prettyJSON)
}

// 0.7.0 Bytes encodes the AccountState as a byte slice
func ParameterBytes() ([]byte, error) {
	var buf bytes.Buffer

	writeUint64 := func(value uint64) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}
	writeUint32 := func(value uint32) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}
	writeUint16 := func(value uint16) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}

	// E_8: B_I, B_L, B_S
	if err := writeUint64(MinElectiveServiceItemBalance); err != nil { // B_I
		return nil, err
	}
	if err := writeUint64(MinElectiveServiceOctetBalance); err != nil { // B_L
		return nil, err
	}
	if err := writeUint64(BaseServiceBalance); err != nil { // B_S
		return nil, err
	}
	// E_2: C
	if err := writeUint16(TotalCores); err != nil { // C
		return nil, err
	}
	// E_4: D, E
	if err := writeUint32(PreimageExpiryPeriod); err != nil { // D
		return nil, err
	}
	if err := writeUint32(EpochLength); err != nil { // E
		return nil, err
	}
	// E_8: G_A, G_I, G_R, G_T
	if err := writeUint64(AccumulationGasAllocation); err != nil { // G_A
		return nil, err
	}
	if err := writeUint64(IsAuthorizedGasAllocation); err != nil { // G_I
		return nil, err
	}
	if err := writeUint64(RefineGasAllocation); err != nil { // G_R
		return nil, err
	}
	if err := writeUint64(AccumulateGasAllocation_GT); err != nil { // G_T
		return nil, err
	}
	// E_2: H, I, J, K
	if err := writeUint16(RecentHistorySize); err != nil { // H
		return nil, err
	}
	if err := writeUint16(MaxWorkItemsPerPackage); err != nil { // I
		return nil, err
	}
	if err := writeUint16(MaxDependencyItemsInWorkReport); err != nil { // J
		return nil, err
	}
	if err := writeUint16(MaxTicketsPerExtrinsic); err != nil { // K (NEW in 0.7.0 [polkajam uses in 0.6.7])
		return nil, err
	}

	// E_4: L
	if err := writeUint32(LookupAnchorMaxAge); err != nil { // L
		return nil, err
	}
	// E_2: N, O
	if err := writeUint16(TicketEntriesPerValidator); err != nil { // N (NEW in 0.7.0 [polkajam uses in 0.6.7])
		return nil, err
	}
	if err := writeUint16(MaxAuthorizationPoolItems); err != nil { // O
		return nil, err
	}

	// E_2: P, Q, R, T, U, V
	if err := writeUint16(SecondsPerSlot); err != nil { // P
		return nil, err
	}
	if err := writeUint16(MaxAuthorizationQueueItems); err != nil { // Q
		return nil, err
	}
	if err := writeUint16(RotationPeriod); err != nil { // R
		return nil, err
	}
	if err := writeUint16(ExtrinsicMaximumPerPackage); err != nil { // T
		return nil, err
	}
	if err := writeUint16(UnavailableWorkReplacementPeriod); err != nil { // U
		return nil, err
	}
	if err := writeUint16(TotalValidators); err != nil { // V
		return nil, err
	}
	// E_4: W_A, W_B, W_C, W_E, W_G, W_M, W_P, W_R, W_T, W_X, Y
	if err := writeUint32(MaxIsAuthorizedCodeBytes); err != nil { // W_A
		return nil, err
	}
	if err := writeUint32(MaxEncodedWorkPackageSize); err != nil { // W_B
		return nil, err
	}
	if err := writeUint32(MaxServiceCodeSize); err != nil { // W_C
		return nil, err
	}
	if err := writeUint32(ECPieceSize); err != nil { // W_E
		return nil, err
	}
	if err := writeUint32(MaxImports); err != nil { // W_M
		return nil, err
	}
	if err := writeUint32(NumECPiecesPerSegment); err != nil { // W_P
		return nil, err
	}
	if err := writeUint32(MaxEncodedWorkReportSize); err != nil { // W_R
		return nil, err
	}
	if err := writeUint32(TransferMemoSize); err != nil { // W_T
		return nil, err
	}
	if err := writeUint32(MaxExports); err != nil { // W_X
		return nil, err
	}
	if err := writeUint32(TicketSubmissionEndSlot); err != nil { // Y
		return nil, err
	}

	return buf.Bytes(), nil
}

const (
	QuicIndividualTimeout = 8000 * time.Millisecond
	QuicOverallTimeout    = 10000 * time.Millisecond
)

//X: Signing Contexts.

const (
	X_A     = "jam_available"
	X_B     = "jam_beefy"
	X_E     = "jam_entropy"
	X_F     = "jam_fallback_seal"
	X_G     = "jam_guarantee"
	X_I     = "jam_announce"
	X_T     = "jam_ticket_seal"
	X_U     = "jam_audit"
	X_True  = "jam_valid"
	X_False = "jam_invalid"
)

// extrinsic tidy up constants
const (
	MaxEpochsToKeepSelfTickets = 5
)

// Hash Type
const (
	Keccak  string = "keccak"
	Blake2b string = "blake2b"
)
