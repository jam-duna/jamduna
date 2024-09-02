package types

const (
	BlsSizeInBytes            = 144
	MetadataSizeInBytes       = 128
	ExtrinsicSignatureInBytes = 784
	TicketsVerifierKeyInBytes = 144
	ValidatorInByte           = 336
	EntropySize               = 4
)

const (
	JamCommonEra = 1704100800 //1200 UTC on January 1, 2024
)

const (
	PeriodSecond                     = 8              // A represents the period, in seconds, between audit tranches.
	MinElectiveServiceItemBalance    = 10             // B_I represents the additional minimum balance required per item of elective service state.
	MinElectiveServiceOctetBalance   = 1              // B_L represents the additional minimum balance required per octet of elective service state.
	BaseServiceBalance               = 100            // B_S = 100: The basic minimum balance which all services require.
	TotalCores                       = 2              // C = 341: The total number of cores.
	PreimageExpiryPeriod             = 28800          // D = 28,800: The period in timeslots after which an unreferenced preimage may be expunged.
	EpochLength                      = 10             // E = 600: The length of an epoch in timeslots.
	AuditBiasFactor                  = 2              // F = 2: The audit bias factor, the expected number of additional validators who will audit a work-report in the following tranche for each no-show in the previous.
	AccumulationGasAllocation                         // G_A: The total gas allocated to a core for Accumulation.
	IsAuthorizedGasAllocation                         // G_I : The gas allocated to invoke a work-package’s Is-Authorized logic.
	RefineGasAllocation                               // G_R: The total gas allocated for a work-package’s Refine logic.
	RecentHistorySize                = 8              // H = 8: The size of recent history, in blocks.
	MaxWorkItemsPerPackage           = 4              // I = 4: The maximum amount of work items in a package.
	MaxTicketsPerExtrinsic           = 16             // K = 16: The maximum number of tickets which may be submitted in a single extrinsic.
	LookupAnchorMaxAge               = 14400          // L = 14,400: The maximum age in timeslots of the lookup anchor.
	TransferMemoSize                 = 128            // M = 128: The size of a transfer memo in octets.
	TicketEntriesPerValidator        = 2              // N = 2: The number of ticket entries per validator.
	MaxAuthorizationPoolItems        = 8              // O = 8: The maximum number of items in the authorizations pool.
	SecondsPerSlot                   = 12             // P = 6: The slot period, in seconds.
	MaxAuthorizationQueueItems       = 80             // Q = 80: The maximum number of items in the authorizations queue.
	ValidatorCoreRotationPeriod      = 10             // R = 10: The rotation period of validator-core assignments, in timeslots.
	MaxServiceCodeSize               = 4000000        // S = 4,000,000: The maximum size of service code in octets.
	UnavailableWorkReplacementPeriod = 5              // U = 5: The period in timeslots after which reported but unavailable work may be replaced.
	TotalValidators                  = 6              // V = 1023: The total number of validators.
	W_C                              = 4              // W_C = 684: The basic size of our erasure-coded pieces. See equation 316.
	MaxManifestEntries               = 1 << 11        // W_M = 2^11: The maximum number of entries in a work-package manifest.
	MaxEncodedWorkPackageSize        = 12 * (1 << 20) // W_P = 12 * 2^20: The maximum size of an encoded work-package together with its extrinsic data and import implications, in octets.
	MaxEncodedWorkReportSize         = 96 * (1 << 10) // W_R = 96 * 2^10: The maximum size of an encoded work-report in octets.
	W_S                              = 6              // W_S = 6: The size of an exported segment in erasure-coded pieces.
	TicketSubmissionEndSlot          = 8              // Y = 500: The number of slots into an epoch at which ticket-submission ends.
	PVMDynamicAddressAlignmentFactor = 2              // Z_A = 2: The pvm dynamic address alignment factor. See equation 227.
	PVMInitInputDataSize             = 1 << 24        // Z_I = 2^24: The standard pvm program initialization input data size. See equation A.7.
	PVMInitPageSize                  = 1 << 14        // Z_P = 2^14: The standard pvm program initialization page size. See section A.7.
	PVMInitSegmentSize               = 1 << 16        // Z_Q = 2^16: The standard pvm program initialization segment size. See section A.7.
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
