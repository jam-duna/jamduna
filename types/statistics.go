package types

import (
	"encoding/json"
	"reflect"
	"sort"
)

type ValidatorStatisticState struct {
	BlocksProduced         uint32 `json:"blocks"`          // The number of blocks produced by the validator.
	TicketsIntroduced      uint32 `json:"tickets"`         // The number of tickets introduced by the validator.
	PreimagesIntroduced    uint32 `json:"pre_images"`      // The number of preimages introduced by the validator.
	OctetsIntroduced       uint32 `json:"pre_images_size"` // The total number of octets across all preimages introduced by the validator.
	ReportsGuaranteed      uint32 `json:"guarantees"`      // The number of reports guaranteed by the validator.
	AvailabilityAssurances uint32 `json:"assurances"`      // The number of availability assurances made by the validator.
}
type ValidatorStatistics struct {
	Current           [TotalValidators]ValidatorStatisticState `json:"vals_curr_stats"`
	Last              [TotalValidators]ValidatorStatisticState `json:"vals_last_stats"`
	CoreStatistics    [TotalCores]CoreStatistics               `json:"cores"`
	ServiceStatistics map[uint32]ServiceStatistics             `json:"services"`
}

func (v *ValidatorStatistics) Copy() *ValidatorStatistics {
	newStats := ValidatorStatistics{}
	copy(newStats.Current[:], v.Current[:])
	copy(newStats.Last[:], v.Last[:])
	copy(newStats.CoreStatistics[:], v.CoreStatistics[:])
	newStats.ServiceStatistics = make(map[uint32]ServiceStatistics)
	for k, v := range v.ServiceStatistics {
		newStats.ServiceStatistics[k] = v
	}
	return &newStats
}

type ServiceStatisticsKeyPair struct {
	ServiceIndex      uint              `json:"id"` // The index of the service.
	ServiceStatistics ServiceStatistics `json:"record"`
}

type ServiceStatisticsKeyPairs []ServiceStatisticsKeyPair

func (s *ServiceStatisticsKeyPairs) Sort() {
	// sort by key (service index)
	sort.Slice(*s, func(i, j int) bool {
		return (*s)[i].ServiceIndex < (*s)[j].ServiceIndex
	})
}

type TrueStatistics struct {
	Current        [TotalValidators]ValidatorStatisticState `json:"vals_curr_stats"`
	Last           [TotalValidators]ValidatorStatisticState `json:"vals_last_stats"`
	CoreStatistics [TotalCores]CoreStatistics               `json:"cores"`
	ServiceStatics ServiceStatisticsKeyPairs                `json:"services,omitempty"`
}

// GP 6.4 Eq 13.6
type CoreStatistics struct {
	DALoad              uint `json:"da_load"`         // d
	NumAssurances       uint `json:"popularity"`      // p
	NumImportedSegments uint `json:"imports"`         // i
	NumExportedSegments uint `json:"exports"`         // e
	NumBytesExtrinsics  uint `json:"extrinsic_size"`  // z
	NumExtrinsics       uint `json:"extrinsic_count"` // x
	TotalBundleLength   uint `json:"bundle_size"`     // b
	GasUsed             uint `json:"gas_used"`        // u
}

// GP 6.4 Eq 13.7
type ServiceStatistics struct {
	NumPreimages             uint `json:"provided_count"`      //p
	NumBytesPreimages        uint `json:"provided_size"`       //p
	NumResults               uint `json:"refinement_count"`    //r-n
	RefineGasUsed            uint `json:"refinement_gas_used"` //r-u
	NumImportedSegments      uint `json:"imports"`             //i
	NumExportedSegments      uint `json:"exports"`             //e
	NumBytesExtrinsics       uint `json:"extrinsic_size"`      //z
	NumExtrinsics            uint `json:"extrinsic_count"`     //x
	AccumulateNumWorkReports uint `json:"accumulate_count"`    //a
	AccumulateGasUsed        uint `json:"accumulate_gas_used"` //a
}

func (v *ValidatorStatistics) Encode() []byte {
	trueStatistics := TrueStatistics{}
	trueStatistics.Current = v.Current
	trueStatistics.Last = v.Last
	trueStatistics.CoreStatistics = v.CoreStatistics
	trueStatistics.ServiceStatics = make(ServiceStatisticsKeyPairs, 0)
	for k, v := range v.ServiceStatistics {
		trueStatistics.ServiceStatics = append(trueStatistics.ServiceStatics, ServiceStatisticsKeyPair{uint(k), v})
	}
	trueStatistics.ServiceStatics.Sort()
	encoded, err := Encode(trueStatistics)
	if err != nil {
		return nil
	}
	return encoded
}

func (v *ValidatorStatistics) Decode(data []byte) (interface{}, uint32) {
	decoded, dataLen, err := Decode(data, reflect.TypeOf(TrueStatistics{}))
	if err != nil {
		return nil, 0
	}
	trueStatistics := decoded.(TrueStatistics)
	recoveredStats := ValidatorStatistics{}
	recoveredStats.Current = trueStatistics.Current
	recoveredStats.Last = trueStatistics.Last
	recoveredStats.CoreStatistics = trueStatistics.CoreStatistics
	recoveredStats.ServiceStatistics = make(map[uint32]ServiceStatistics)
	for _, v := range trueStatistics.ServiceStatics {
		recoveredStats.ServiceStatistics[uint32(v.ServiceIndex)] = v.ServiceStatistics
	}
	return recoveredStats, dataLen
}

func (v *ValidatorStatistics) UnmarshalJSON(data []byte) error {
	trueStatistics := TrueStatistics{}
	if err := json.Unmarshal(data, &trueStatistics); err != nil {
		return err
	}
	v.Current = trueStatistics.Current
	v.Last = trueStatistics.Last
	v.CoreStatistics = trueStatistics.CoreStatistics
	v.ServiceStatistics = make(map[uint32]ServiceStatistics)
	for _, service := range trueStatistics.ServiceStatics {
		v.ServiceStatistics[uint32(service.ServiceIndex)] = service.ServiceStatistics
	}
	return nil
}

func (v *ValidatorStatistics) MarshalJSON() ([]byte, error) {
	trueStatistics := TrueStatistics{}
	trueStatistics.Current = v.Current
	trueStatistics.Last = v.Last
	trueStatistics.CoreStatistics = v.CoreStatistics
	trueStatistics.ServiceStatics = make(ServiceStatisticsKeyPairs, 0)
	for k, v := range v.ServiceStatistics {
		trueStatistics.ServiceStatics = append(trueStatistics.ServiceStatics, ServiceStatisticsKeyPair{uint(k), v})
	}
	trueStatistics.ServiceStatics.Sort()
	return json.Marshal(trueStatistics)
}

func (v *TrueStatistics) String() string {
	return ToJSON(v)
}

func (v *ValidatorStatistics) String() string {
	return ToJSON(v)
}

func (v *ServiceStatistics) String() string {
	return ToJSON(v)
}
