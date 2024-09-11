package types

type GuarantorAssignment struct {
	CoreIndex uint16    `json:"core_index"`
	Validator Validator `json:"validator"`
}
