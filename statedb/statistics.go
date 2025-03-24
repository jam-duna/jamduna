package statedb

import (
	"fmt"
	"math"
	"reflect"
	"sort"

	"github.com/colorfulnotion/jam/types"
)

type ValidatorStatistics struct {
	Current           [types.TotalValidators]ValidatorStatisticState `json:"vals_current"`
	Last              [types.TotalValidators]ValidatorStatisticState `json:"vals_last"`
	CoreStatistics    [types.TotalCores]CoreStatistics               `json:"cores"`
	ServiceStatistics map[uint32]ServiceStatistics                   `json:"services"`
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
	Current        [types.TotalValidators]ValidatorStatisticState `json:"vals_current"`
	Last           [types.TotalValidators]ValidatorStatisticState `json:"vals_last"`
	CoreStatistics [types.TotalCores]CoreStatistics               `json:"cores"`
	ServiceStatics ServiceStatisticsKeyPairs                      `json:"services"`
}

// GP 6.4 Eq 13.6
type CoreStatistics struct {
	GasUsed             uint `json:"gas_used"`        // u
	NumImportedSegments uint `json:"imports"`         // i
	NumExtrinsics       uint `json:"extrinsic_count"` // x
	NumBytesExtrinsics  uint `json:"extrinsic_size"`  // z
	NumExportedSegments uint `json:"exports"`         // e
	TotalBundleLength   uint `json:"bundle_size"`     // b
	DALoad              uint `json:"da_load"`         // d
	NumAssurances       uint `json:"popularity"`      // p
}

// GP 6.4 Eq 13.7
type ServiceStatistics struct {
	NumPreimages             uint `json:"provided_count"`        //p
	NumBytesPreimages        uint `json:"provided_size"`         //p
	NumResults               uint `json:"refinement_count"`      //n
	RefineGasUsed            uint `json:"refinement_gas_used"`   //u
	NumImportedSegments      uint `json:"imports"`               //i
	NumExportedSegments      uint `json:"exports"`               //e
	NumBytesExtrinsics       uint `json:"extrinsic_size"`        //z
	NumExtrinsics            uint `json:"extrinsic_count"`       //x
	AccumulateNumWorkReports uint `json:"accumulate_count"`      //a
	AccumulateGasUsed        uint `json:"accumulate_gas_used"`   //a
	TransferNumTransfers     uint `json:"on_transfers_count"`    //t
	TransferGasUsed          uint `json:"on_transfers_gas_used"` //t
}

type accumulateStatistics struct {
	gasUsed        uint
	numWorkReports uint
}
type transferStatistics struct {
	gasUsed      uint
	numTransfers uint
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
		cs.TotalBundleLength += uint(g.AvailabilitySpec.BundleLength) // b
	}
	for _, a := range newlyAvailable { // W -- D(..)
		cs := &(n.ValidatorStatistics.CoreStatistics[a.CoreIndex])
		cs.DALoad += uint(a.AvailabilitySpec.BundleLength + uint32(65*a.AvailabilitySpec.ExportedSegmentLength/64))
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

func (n *JamState) tallyServiceStatistics(guarantees []types.Guarantee, preimages []types.Preimages, accumulateStats map[uint32]*accumulateStatistics, transferStats map[uint32]*transferStatistics) {
	stats := make(map[uint32]*ServiceStatistics)
	for _, g := range guarantees { // w -- R(...)
		for _, v := range g.Report.Results {
			cs, ok := stats[v.ServiceID]
			if !ok {
				cs = &ServiceStatistics{}
				stats[v.ServiceID] = cs
			}
			cs.RefineGasUsed += v.GasUsed                   //u
			cs.NumImportedSegments += v.NumImportedSegments //i
			cs.NumExportedSegments += v.NumExportedSegments //e
			cs.NumExtrinsics += v.NumExtrinsics             //x
			cs.NumBytesExtrinsics += v.NumBytesExtrinsics   //z
			cs.NumResults += 1
		}

	}
	for _, p := range preimages {
		cs, ok := stats[p.Requester]
		if !ok {
			cs = &ServiceStatistics{}
			stats[p.Requester] = cs
		}
		cs.NumPreimages += 1                      //p
		cs.NumBytesPreimages += uint(len(p.Blob)) //p
	}
	for s, a := range accumulateStats { // I
		cs, ok := stats[s]
		if !ok {
			cs = &ServiceStatistics{}
			stats[s] = cs
		}
		cs.AccumulateNumWorkReports += a.numWorkReports

		if math.MaxUint64-cs.AccumulateGasUsed < a.gasUsed {
			fmt.Printf("service %d, og data %d, new data %d\n", s, cs.AccumulateGasUsed, a.gasUsed) // overflow check
		} else {
			cs.AccumulateGasUsed += a.gasUsed
		}
	}
	for s, t := range transferStats { // X
		cs, ok := stats[s]
		if !ok {
			cs = &ServiceStatistics{}
			stats[s] = cs
		}
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
