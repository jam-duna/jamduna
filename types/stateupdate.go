package types

import "github.com/colorfulnotion/jam/common"

type StateUpdate struct {
	WorkPackageUpdates map[common.Hash]*SubWorkPackageResult // workpackage hash
	ServiceUpdates     map[uint32]*ServiceUpdate
}

func NewStateUpdate() *StateUpdate {
	return &StateUpdate{
		ServiceUpdates:     make(map[uint32]*ServiceUpdate),
		WorkPackageUpdates: make(map[common.Hash]*SubWorkPackageResult),
	}
}

type ServiceUpdate struct {
	ServiceInfo     *SubServiceInfoResult
	ServiceValue    map[common.Hash]*SubServiceValueResult    // storage key
	ServicePreimage map[common.Hash]*SubServicePreimageResult // preimage hash
	ServiceRequest  map[common.Hash]*SubServiceRequestResult  // preimage hash
}

func NewServiceUpdate(s uint32) *ServiceUpdate {
	return &ServiceUpdate{
		ServiceInfo:     nil,
		ServiceValue:    make(map[common.Hash]*SubServiceValueResult),
		ServicePreimage: make(map[common.Hash]*SubServicePreimageResult),
		ServiceRequest:  make(map[common.Hash]*SubServiceRequestResult),
	}
}

func (su *StateUpdate) AddServiceUpdate(serviceIndex uint32, serviceUpdate *ServiceUpdate) {
	su.ServiceUpdates[serviceIndex] = serviceUpdate
}

type WSPayload struct {
	Method string      `json:"method"`
	Result interface{} `json:"result"`
}
type SubBlockResult struct {
	BlockHash  common.Hash `json:"blockHash"`
	HeaderHash common.Hash `json:"headerHash"`
}

type SubStatisticsResult struct {
	HeaderHash common.Hash         `json:"headerHash"`
	Statistics ValidatorStatistics `json:"statistics"`
	Slot       uint32              `json:"slot"`
}

type SubServiceValueResult struct {
	ServiceID  uint32      `json:"serviceID"`
	Key        string      `json:"key"`
	Hash       common.Hash `json:"hash"`
	Value      string      `json:"value"`
	HeaderHash common.Hash `json:"headerHash"`
	Slot       uint32      `json:"slot"`
}

type SubServiceRequestResult struct {
	ServiceID  uint32      `json:"serviceID"`
	Hash       common.Hash `json:"hash"`
	Timeslots  []uint32    `json:"timeslots"`
	HeaderHash common.Hash `json:"headerHash"`
	Slot       uint32      `json:"slot"`
}

type SubServicePreimageResult struct {
	ServiceID  uint32      `json:"serviceID"`
	Hash       common.Hash `json:"hash"`
	Preimage   string      `json:"preimage"`
	HeaderHash common.Hash `json:"headerHash"`
	Slot       uint32      `json:"slot"`
}

type SubServiceInfoResult struct {
	ServiceID  uint32         `json:"serviceID"`
	Info       ServiceAccount `json:"info"`
	HeaderHash common.Hash    `json:"headerHash"`
	Slot       uint32         `json:"slot"`
}

type SubWorkPackageResult struct {
	WorkPackageHash common.Hash `json:"workPackageHash"`
	Status          string      `json:"status"`
	HeaderHash      common.Hash `json:"headerHash"`
	Slot            uint32      `json:"slot"`
}
