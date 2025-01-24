//go:build 2xlarge
// +build 2xlarge

// go:build 2xlarge

package configs

const (
	// 2xlarge testnet : Tickets only
	Network                   = "2xlarge"
	TotalValidators           = 384 // V = 1023: The total number of validators.
	TotalCores                = 128 // C = 341: The total number of cores.
	TicketEntriesPerValidator = 2   // N = 2: The number of ticket entries per validator.
	EpochLength               = 300 // E = 600: The length of an epoch in timeslots.
	TicketSubmissionEndSlot   = 250 // Y = 500: The number of slots into an epoch at which ticket-submission ends.
	MaxTicketsPerExtrinsic    = 16  // K = 16: The maximum number of tickets which may be submitted in a single extrinsic.
)
