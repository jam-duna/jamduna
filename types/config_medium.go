//go:build medium

package types

const (
	// Medium testnet : Tickets only
	Network                   = "medium"
	TotalValidators           = 48 // V = 1023: The total number of validators.
	TotalCores                = 16 // C = 341: The total number of cores.
	TicketEntriesPerValidator = 6  // N = 2: The number of ticket entries per validator.
	EpochLength               = 60 // E = 600: The length of an epoch in timeslots.
	TicketSubmissionEndSlot   = 50 // Y = 500: The number of slots into an epoch at which ticket-submission ends.
	MaxTicketsPerExtrinsic    = 3  // K = 16: The maximum number of tickets which may be submitted in a single extrinsic.
	SecondsPerEpoch           = EpochLength * SecondsPerSlot
)
