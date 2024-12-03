package types

type ChainSpec struct {
	NumValidators              int `json:"num_validators"`
	NumCores                   int `json:"num_cores"`
	SlotDuration               int `json:"slot_duration"`
	EpochDuration              int `json:"epoch_duration"`
	ContestDuration            int `json:"contest_duration"`
	TicketsPerValidator        int `json:"tickets_per_validator"`
	MaxTicketsPerExtrinsic     int `json:"max_tickets_per_extrinsic"`
	RotationPeriod             int `json:"rotation_period"`
	MaxAuthorizationPoolItems  int `json:"max_authorization_pool_items"`
	MaxAuthorizationQueueItems int `json:"max_authorization_queue_items"`
}
