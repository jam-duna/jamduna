//go:build small
// +build small

// go:build small

package configs

const (
	Network                   = "small"
	TotalValidators           = 24
	TotalCores                = 8
	TicketEntriesPerValidator = 3
	EpochLength               = 12
	TicketSubmissionEndSlot   = 10
	MaxTicketsPerExtrinsic    = 3
)
