package statedb

import (
	"reflect"
	"sort"

	"github.com/colorfulnotion/jam/types"
)

type ValidatorStatistics struct {
	Current           [types.TotalValidators]ValidatorStatisticState `json:"current"`
	Last              [types.TotalValidators]ValidatorStatisticState `json:"last"`
	CoreStatistics    [types.TotalCores]CoreStatistics               `json:"core_statistics"`
	ServiceStatistics map[uint32]ServiceStatistics                   `json:"service_statistics"`
}

type ServiceStatisticsKeyPair struct {
	ServiceIndex      uint32
	ServiceStatistics ServiceStatistics
}

type ServiceStatisticsKeyPairs []ServiceStatisticsKeyPair

func (s *ServiceStatisticsKeyPairs) Sort() {
	// sort by key (service index)
	sort.Slice(*s, func(i, j int) bool {
		return (*s)[i].ServiceIndex < (*s)[j].ServiceIndex
	})
}

type TrueStatistics struct {
	Current        [types.TotalValidators]ValidatorStatisticState `json:"current"`
	Last           [types.TotalValidators]ValidatorStatisticState `json:"last"`
	CoreStatistics [types.TotalCores]CoreStatistics               `json:"core_statistics"`
	ServiceStatics ServiceStatisticsKeyPairs                      `json:"service_statistics"`
}

// GP 6.4 Eq 13.6
type CoreStatistics struct {
	DALoad              uint32 `json:"da_load"`         // d
	NumAssurances       uint32 `json:"popularity"`      // p
	NumImportedSegments uint32 `json:"imports"`         // i
	NumExportedSegments uint32 `json:"exports"`         // e
	NumBytesExtrinsics  uint32 `json:"extrinsic_size"`  // z
	NumExtrinsics       uint32 `json:"extrinsic_count"` // x
	TotalBundleLength   uint32 `json:"bundle_size"`     // b
	GasUsed             uint64 `json:"gas_used"`        // u
}

// GP 6.4 Eq 13.7
type ServiceStatistics struct {
	NumPreimages             uint32 `json:"provided_count"`        //p
	NumBytesPreimages        uint32 `json:"provided_size"`         //p
	NumResults               uint32 `json:"refinement_count"`      //n
	RefineGasUsed            uint64 `json:"refinement_gas_used"`   //u
	NumImportedSegments      uint32 `json:"imports"`               //i
	NumExportedSegments      uint32 `json:"exports"`               //e
	NumBytesExtrinsics       uint32 `json:"extrinsic_size"`        //z
	NumExtrinsics            uint32 `json:"extrinsic_count"`       //x
	AccumulateNumWorkReports uint32 `json:"accumulate_count"`      //a
	AccumulateGasUsed        uint64 `json:"accumulate_gas_used"`   //a
	TransferNumTransfers     uint32 `json:"on_transfers_count"`    //t
	TransferGasUsed          uint64 `json:"on_transfers_gas_used"` //t
}

type accumulateStatistics struct {
	gasUsed        uint64
	numWorkReports uint32
}
type transferStatistics struct {
	gasUsed      uint64
	numTransfers uint32
}

func (n *JamState) tallyCoreStatistics(guarantees []types.Guarantee, newlyAvailable []types.WorkReport, assurances []types.Assurance) {
	for _, guarantee := range guarantees { // w - R(..)
		g := guarantee.Report
		cs := &(n.ValidatorStatistics.CoreStatistics[g.CoreIndex])
		for _, v := range g.Results {
			cs.GasUsed += v.GasUsed                         //u
			cs.NumImportedSegments += v.NumImportedSegments //i
			cs.NumExportedSegments += v.NumExportedSegments //e
			cs.NumExtrinsics += v.NumExtrinsics             //x
			cs.NumBytesExtrinsics += v.NumBytesExtrinsics   //z
		}
		cs.TotalBundleLength += g.AvailabilitySpec.BundleLength // b
	}
	for _, a := range newlyAvailable { // W -- D(..)
		cs := &(n.ValidatorStatistics.CoreStatistics[a.CoreIndex])
		cs.DALoad += a.AvailabilitySpec.BundleLength + uint32(65*a.AvailabilitySpec.ExportedSegmentLength/64)
	}

	for _, a := range assurances {
		// p
		for c := 0; c < types.TotalCores; c++ {
			if a.GetBitFieldBit(uint16(c)) {
				cs := &(n.ValidatorStatistics.CoreStatistics[c])
				cs.NumAssurances += 1
			}
		}
	}
	return
}

func (n *JamState) tallyServiceStatistics(guarantees []types.Guarantee, preimages []types.Preimages, accumulateStats map[uint32]accumulateStatistics, transferStats map[uint32]*transferStatistics) {
	stats := make(map[uint32]*ServiceStatistics)
	for _, g := range guarantees { // w -- R(...)
		for _, v := range g.Report.Results {
			if stats[v.ServiceID] == nil {
				stats[v.ServiceID] = &ServiceStatistics{}
			}
			cs := stats[v.ServiceID]
			cs.RefineGasUsed += v.GasUsed                   //u
			cs.NumImportedSegments += v.NumImportedSegments //i
			cs.NumExportedSegments += v.NumExportedSegments //e
			cs.NumExtrinsics += v.NumExtrinsics             //x
			cs.NumBytesExtrinsics += v.NumBytesExtrinsics   //z
			cs.NumResults += 1
		}
	}
	for _, p := range preimages {
		if stats[p.Requester] == nil {
			stats[p.Requester] = &ServiceStatistics{}
		}
		cs := stats[p.Requester]
		cs.NumPreimages += 1                        //p
		cs.NumBytesPreimages += uint32(len(p.Blob)) //p
	}
	for s, a := range accumulateStats { // I
		if stats[s] == nil {
			stats[s] = &ServiceStatistics{}
		}
		cs := stats[s]
		cs.AccumulateNumWorkReports += a.numWorkReports
		cs.AccumulateGasUsed += a.gasUsed
	}
	for s, t := range transferStats { // X
		if stats[s] == nil {
			stats[s] = &ServiceStatistics{}
		}
		cs := stats[s]
		cs.TransferGasUsed += t.gasUsed
		cs.TransferNumTransfers += t.numTransfers
	}
	// incorporate stats into ValidatorStatistics
	n.ValidatorStatistics.ServiceStatistics = make(map[uint32]ServiceStatistics)
	for k, v := range stats {
		if v != nil {
			n.ValidatorStatistics.ServiceStatistics[k] = *v
		}
	}
}

func (v *ValidatorStatistics) Encode() []byte {
	trueStatistics := TrueStatistics{}
	trueStatistics.Current = v.Current
	trueStatistics.Last = v.Last
	trueStatistics.CoreStatistics = v.CoreStatistics
	trueStatistics.ServiceStatics = make(ServiceStatisticsKeyPairs, 0)
	for k, v := range v.ServiceStatistics {
		trueStatistics.ServiceStatics = append(trueStatistics.ServiceStatics, ServiceStatisticsKeyPair{k, v})
	}
	trueStatistics.ServiceStatics.Sort()
	encoded, err := types.Encode(trueStatistics)
	if err != nil {
		return nil
	}
	return encoded
}

func (v *ValidatorStatistics) Decode(data []byte) (interface{}, uint32) {
	decoded, dataLen, err := types.Decode(data, reflect.TypeOf(TrueStatistics{}))
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
		recoveredStats.ServiceStatistics[v.ServiceIndex] = v.ServiceStatistics
	}
	return &recoveredStats, dataLen
}
