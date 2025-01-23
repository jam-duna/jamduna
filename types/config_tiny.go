// // go:build tiny

package types

const (
	// Tiny testnet : Tickets only
	Network                   = "tiny"
	TotalValidators           = 6  // V = 1023: The total number of validators.
	TotalCores                = 2  // C = 341: The total number of cores.
	TicketEntriesPerValidator = 3  // N = 2: The number of ticket entries per validator.
	EpochLength               = 12 // E = 600: The length of an epoch in timeslots.
	TicketSubmissionEndSlot   = 10 // Y = 500: The number of slots into an epoch at which ticket-submission ends.
	MaxTicketsPerExtrinsic    = 3  // K = 16: The maximum number of tickets which may be submitted in a single extrinsic.
)
