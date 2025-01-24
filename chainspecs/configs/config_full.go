//go:build full
// +build full

// go:build full

package configs

const (
	// Full testnet : Tickets only
	Network                   = "full"
	TotalValidators           = 1023 // V = 1023: The total number of validators.
	TotalCores                = 341  // C = 341: The total number of cores.
	TicketEntriesPerValidator = 2    // N = 2: The number of ticket entries per validator.
	EpochLength               = 600  // E = 600: The length of an epoch in timeslots.
	TicketSubmissionEndSlot   = 500  // Y = 500: The number of slots into an epoch at which ticket-submission ends.
	MaxTicketsPerExtrinsic    = 16   // K = 16: The maximum number of tickets which may be submitted in a single extrinsic.
)
