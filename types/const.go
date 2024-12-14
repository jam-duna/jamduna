package types

import (
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
)

const (
	JamCommonEra = 1704100800 //1200 UTC on January 1, 2024
)

const (
	FixedSegmentSizeG = W_S * W_E
)

const (
	// Tiny testnet : Tickets only
	TotalValidators           = 6  // V = 1023: The total number of validators.
	TotalCores                = 2  // C = 341: The total number of cores.
	TicketEntriesPerValidator = 3  // N = 2: The number of ticket entries per validator.
	EpochLength               = 12 // E = 600: The length of an epoch in timeslots.
	TicketSubmissionEndSlot   = 10 // Y = 500: The number of slots into an epoch at which ticket-submission ends.
	MaxTicketsPerExtrinsic    = 3  // K = 16: The maximum number of tickets which may be submitted in a single extrinsic.
	SecondsPerEpoch           = EpochLength * SecondsPerSlot
)

// time mode
// three mode input => "JAM", "TimeStamp", "TimeSlot"
const TimeUnitMode = "JAM"
const TimeSavingMode bool = true
const CoreLazyMode bool = true
const (
	// Medium: configure these
	W_S = 6 // W_S = 6: The size of an exported segment in erasure-coded pieces.
	W_E = 4 // W_E = 684: The basic size of our erasure-coded pieces. See equation 316.

	PeriodSecond                     = 4              // A=8 represents the period, in seconds, between audit tranches.
	MinElectiveServiceItemBalance    = 10             // B_I represents the additional minimum balance required per item of elective service state.
	MinElectiveServiceOctetBalance   = 1              // B_L represents the additional minimum balance required per octet of elective service state.
	BaseServiceBalance               = 100            // B_S = 100: The basic minimum balance which all services require.
	PreimageExpiryPeriod             = 28800          // D = 28,800: The period in timeslots after which an unreferenced preimage may be expunged.
	AuditBiasFactor                  = 2              // F = 2: The audit bias factor, the expected number of additional validators who will audit a work-report in the following tranche for each no-show in the previous.
	AccumulationGasAllocation        = 10000000       // G_A: The total gas allocated to a core for Accumulation.
	IsAuthorizedGasAllocation        = 1000000        // G_I: The gas allocated to invoke a work-package’s Is-Authorized logic.
	RefineGasAllocation              = 500000000      // G_R: The total gas allocated for a work-package’s Refine logic.
	AccumulateGasAllocation          = 341000000      //GT = 341, 000, 000: The total gas allocated across all cores for Accumulation. Should be no smaller than GA ⋅ C +∑g∈V(χg )(g).
	RecentHistorySize                = 8              // H = 8: The size of recent history, in blocks.
	MaxWorkItemsPerPackage           = 4              // I = 4: The maximum amount of work items in a package.
	LookupAnchorMaxAge               = 14400          // L = 14,400: The maximum age in timeslots of the lookup anchor.
	TransferMemoSize                 = 128            // M = 128: The size of a transfer memo in octets.
	MaxAuthorizationPoolItems        = 2              // O = 8: The maximum number of items in the authorizations pool.
	SecondsPerSlot                   = 6              // P = 6: The slot period, in seconds.
	MaxAuthorizationQueueItems       = 6              // Q = 80: The maximum number of items in the authorizations queue.
	ValidatorCoreRotationPeriod      = 4              // R = 10: The rotation period of validator-core assignments, in timeslots.
	UnavailableWorkReplacementPeriod = 5              // U = 5: The period in timeslots after which reported but unavailable work may be replaced.
	MaxServiceCodeSize               = 4000000        // W_C = 4,000,000: The maximum size of service code in octets.
	MaxManifestEntries               = 1 << 11        // W_M = 2^11: The maximum number of entries in a work-package manifest.
	MaxEncodedWorkPackageSize        = 12 * (1 << 20) // W_P = 12 * 2^20: The maximum size of an encoded work-package together with its extrinsic data and import implications, in octets.
	MaxEncodedWorkReportSize         = 96 * (1 << 10) // W_R = 96 * 2^10: The maximum size of an encoded work-report in octets.
	PVMDynamicAddressAlignmentFactor = 2              // Z_A = 2: The pvm dynamic address alignment factor. See equation 227.
	PVMInitInputDataSize             = 1 << 24        // Z_I = 2^24: The standard pvm program initialization input data size. See equation A.7.
	PVMInitPageSize                  = 1 << 14        // Z_P = 2^14: The standard pvm program initialization page size. See section A.7.
	PVMInitSegmentSize               = 1 << 16        // Z_Q = 2^16: The standard pvm program initialization segment size. See section A.7.
)

const (
	QuicIndividualTimeout = 8000 * time.Millisecond
	QuicOverallTimeout    = 10000 * time.Millisecond
)

const (
	ValidatorsSuperMajority = int(2*TotalValidators/3 + 1)
	WonkyTrueThreshold      = int(TotalValidators / 3)
	WonkyFalseThreshold     = int(TotalValidators/3 + 1)
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

// codec
const (
	validators_super_majority = TotalValidators*2/3 + 1
	Avail_bitfield_bytes      = (TotalCores + 7) / 8
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
