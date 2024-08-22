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
	A   = 8              // A represents the period, in seconds, between audit tranches.
	B_I = 10             // BI represents the additional minimum balance required per item of elective service state.
	B_L = 1              // BL represents the additional minimum balance required per octet of elective service state.
	B_S = 100            // BS = 100: The basic minimum balance which all services require.
	C   = 341            // C = 341: The total number of cores.
	D   = 28800          // D = 28, 800: The period in timeslots after which an unreferenced preimage may be expunged.
	E   = 600            // E = 600: The length of an epoch in timeslots.
	F   = 2              // F = 2: The audit bias factor, the expected number of additional validators who will audit a work-report in the following tranche for each no-show in the previous.
	G_A                  // GA: The total gas allocated to a core for Accumulation.
	G_I                  // GI : The gas allocated to invoke a work-package’s Is-Authorized logic.
	G_R                  // GR: The total gas allocated for a work-package’s Refine logic.
	H   = 8              // H = 8: The size of recent history, in blocks.
	I   = 4              // I = 4: The maximum amount of work items in a package.
	K   = 16             // K = 16: The maximum number of tickets which may be submitted in a single extrinsic.
	L   = 14400          // L = 14, 400: The maximum age in timeslots of the lookup anchor.
	M   = 128            // M = 128: The size of a transfer memo in octets.
	N   = 2              // N = 2: The number of ticket entries per validator.
	O   = 8              // O = 8: The maximum number of items in the authorizations pool.
	P   = 6              // P = 6: The slot period, in seconds.
	Q   = 80             // Q = 80: The maximum number of items in the authorizations queue.
	R   = 10             // R = 10: The rotation period of validator-core assignments, in timeslots.
	S   = 4000000        // S = 4, 000, 000: The maximum size of service code in octets.
	U   = 5              // U = 5: The period in timeslots after which reported but unavailable work may be replaced.
	V   = 1023           // V = 1023: The total number of validators.
	W_C = 684            // WC = 684: The basic size of our erasure-coded pieces. See equation 316.
	W_M = 1 << 11        // WM = 2^11: The maximum number of entries in a work-package manifest.
	W_P = 12 * (1 << 20) // WP = 12 * 2^20: The maximum size of an encoded work-package together with its extrinsic data and import implications, in octets.
	W_R = 96 * (1 << 10) // WR = 96 * 2^10: The maximum size of an encoded work-report in octets.
	W_S = 6              // WS = 6: The size of an exported segment in erasure-coded pieces.
	Y   = 500            // Y = 500: The number of slots into an epoch at which ticket-submission ends.
	Z_A = 2              // ZA = 2: The pvm dynamic address alignment factor. See equation 227.
	Z_I = 1 << 24        // ZI = 2^24: The standard pvm program initialization input data size. See equation A.7.
	Z_P = 1 << 14        // ZP = 2^14: The standard pvm program initialization page size. See section A.7.
	Z_Q = 1 << 16        // ZQ = 2^16: The standard pvm program initialization segment size. See section A.7.
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
