package types

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/jam-duna/jamduna/common"
)

type StateUpdate struct {
	WorkPackageUpdates map[common.Hash]*SubWorkPackageResult `json:"-"`
	ServiceUpdates     map[uint32]*ServiceUpdate             `json:"services,omitempty"`
	ForgetPreimages    map[uint32]common.Hash                `json:"-"`
}

func NewStateUpdate() *StateUpdate {
	return &StateUpdate{
		ServiceUpdates:     make(map[uint32]*ServiceUpdate),
		WorkPackageUpdates: make(map[common.Hash]*SubWorkPackageResult),
	}
}

func (su *StateUpdate) String() string {
	return ToJSON(su)
}

func (su *StateUpdate) GetServiceUpdates() map[uint32]*ServiceUpdate {
	return su.ServiceUpdates
}

type ServiceUpdate struct {
	ServiceInfo     *SubServiceInfoResult                `json:"info"`
	ServiceValue    map[string]*SubServiceValueResult    `json:"value"`
	ServicePreimage map[string]*SubServicePreimageResult `json:"preimage"`
	ServiceRequest  map[string]*SubServiceRequestResult  `json:"request"`
}

func NewServiceUpdate(s uint32) *ServiceUpdate {
	return &ServiceUpdate{
		ServiceInfo:     nil,
		ServiceValue:    make(map[string]*SubServiceValueResult),
		ServicePreimage: make(map[string]*SubServicePreimageResult),
		ServiceRequest:  make(map[string]*SubServiceRequestResult),
	}
}

// MarshalJSON for StateUpdate — emits a bare array of svcJSON
func (s *StateUpdate) MarshalJSON() ([]byte, error) {
	// local JSON‐shapes
	type valueJSON struct {
		Hash  common.Hash `json:"hash"`
		Key   string      `json:"key"`
		Value string      `json:"value"`
	}
	type preimageJSON struct {
		Hash common.Hash `json:"hash"`
		Len  int         `json:"len"`
	}
	type requestJSON struct {
		Key string `json:"key"`
	}
	type svcJSON struct {
		ServiceID string          `json:"serviceID"`
		Info      *ServiceAccount `json:"info,omitempty"`
		Value     []valueJSON     `json:"value,omitempty"`
		Preimage  []preimageJSON  `json:"preimage,omitempty"`
		Request   []requestJSON   `json:"request,omitempty"`
	}

	// 1) sort outer service IDs
	ids := make([]int, 0, len(s.ServiceUpdates))
	for sid := range s.ServiceUpdates {
		ids = append(ids, int(sid))
	}
	sort.Ints(ids)

	// 2) build each svcJSON, dropping any empty slices
	out := make([]svcJSON, 0, len(ids))
	for _, sid := range ids {
		upd := s.ServiceUpdates[uint32(sid)]
		e := svcJSON{ServiceID: strconv.Itoa(sid)}

		if upd.ServiceInfo != nil {
			e.Info = &upd.ServiceInfo.Info
		}

		if len(upd.ServiceValue) > 0 {
			ks := make([]string, 0, len(upd.ServiceValue))
			byKS := make(map[string]*SubServiceValueResult, len(upd.ServiceValue))
			for h, v := range upd.ServiceValue {
				//k := h.String()
				k := h
				ks = append(ks, k)
				byKS[k] = v
			}
			sort.Strings(ks)

			buf := make([]valueJSON, 0, len(ks))
			for _, k := range ks {
				v := byKS[k]
				buf = append(buf, valueJSON{Key: v.Key, Hash: v.Hash, Value: v.Value})
			}
			e.Value = buf
		}

		if len(upd.ServicePreimage) > 0 {
			ks := make([]string, 0, len(upd.ServicePreimage))
			byKS := make(map[string]*SubServicePreimageResult, len(upd.ServicePreimage))
			for h, v := range upd.ServicePreimage {
				//k := h.String()
				k := h
				ks = append(ks, k)
				byKS[k] = v
			}
			sort.Strings(ks)

			buf := make([]preimageJSON, 0, len(ks))
			for _, k := range ks {
				v := byKS[k]
				buf = append(buf, preimageJSON{Hash: v.Hash, Len: len(v.Preimage)})
			}
			e.Preimage = buf
		}

		if len(upd.ServiceRequest) > 0 {
			ks := make([]string, 0, len(upd.ServiceRequest))
			for h := range upd.ServiceRequest {
				ks = append(ks, h)
				//ks = append(ks, h.String())
			}
			sort.Strings(ks)

			buf := make([]requestJSON, 0, len(ks))
			for _, k := range ks {
				buf = append(buf, requestJSON{Key: k})
			}
			e.Request = buf
		}

		out = append(out, e)
	}

	// 3) marshal the slice directly
	return json.Marshal(out)
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

func (su *StateUpdate) AddServiceUpdate(serviceIndex uint32, serviceUpdate *ServiceUpdate) {
	su.ServiceUpdates[serviceIndex] = serviceUpdate
}
func (su *StateUpdate) GetForgets() []*SubServiceRequestResult {
	forgets := make([]*SubServiceRequestResult, 0)
	for _, upd := range su.ServiceUpdates {
		for _, v := range upd.ServiceRequest {
			if v != nil && (v.Timeslots == nil || len(v.Timeslots) == 2) {
				forgets = append(forgets, v)
			}
		}
	}
	return forgets
}
