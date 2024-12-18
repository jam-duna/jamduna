package types

import (
	"encoding/json"
	"fmt"
)

type GuarantorAssignment struct {
	CoreIndex uint16    `json:"core_index"`
	Validator Validator `json:"validator"`
}

func (ga *GuarantorAssignment) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		CoreIndex uint16    `json:"core_index"`
		Validator Validator `json:"validator"`
	}{
		CoreIndex: ga.CoreIndex,
		Validator: ga.Validator,
	})
}
func (ga *GuarantorAssignment) String() string {
	data, err := json.Marshal(ga)
	if err != nil {
		return fmt.Sprintf("GuarantorAssignment{CoreIndex: %d, Validator: %v}", ga.CoreIndex, ga.Validator)
	}
	return string(data)
}

type GuarantorAssignments []GuarantorAssignment

func (g GuarantorAssignments) String() string {
	data, err := json.Marshal(g)
	if err != nil {
		return fmt.Sprintf("GuarantorAssignments: %v", []GuarantorAssignment(g))
	}
	return string(data)
}
