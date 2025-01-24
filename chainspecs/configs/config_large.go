//go:build large
// +build large

// go:build large

package configs

const (
	// large testnet : Tickets only
	Network                   = "large"
	TotalValidators           = 96  // V = 1023: The total number of validators.
	TotalCores                = 32  // C = 341: The total number of cores.
	TicketEntriesPerValidator = 2   // N = 2: The number of ticket entries per validator.
	EpochLength               = 120 // E = 600: The length of an epoch in timeslots.
	TicketSubmissionEndSlot   = 100 // Y = 500: The number of slots into an epoch at which ticket-submission ends.
	MaxTicketsPerExtrinsic    = 8   // K = 16: The maximum number of tickets which may be submitted in a single extrinsic.
)
