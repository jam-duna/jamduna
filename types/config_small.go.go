//go:build small

package types

const (
	Network                   = "small"
	TotalValidators           = 24
	TotalCores                = 8
	TicketEntriesPerValidator = 3
	EpochLength               = 12
	TicketSubmissionEndSlot   = 10
	MaxTicketsPerExtrinsic    = 3
)
