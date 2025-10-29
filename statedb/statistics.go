package statedb

import (
	"bytes"
	"fmt"
	"math"

	jamerrors "github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
)

type accumulateStatistics struct {
	gasUsed        uint
	numWorkReports uint
}
type transferStatistics struct {
	gasUsed      uint
	numTransfers uint
}

func (n *JamState) tallyCoreStatistics(guarantees []types.Guarantee, newlyAvailable []types.WorkReport, assurances []types.Assurance) error {
	n.ValidatorStatistics.CoreStatistics = [types.TotalCores]types.CoreStatistics{}
	for _, guarantee := range guarantees { // w - R(..)
		g := guarantee.Report
		if g.CoreIndex >= types.TotalCores {
			return jamerrors.ErrGBadCoreIndex
		}
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
		cs.DALoad += uint(a.AvailabilitySpec.BundleLength + types.SegmentSize*uint32((65*a.AvailabilitySpec.ExportedSegmentLength+63)/64))
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
	return nil
}

func (n *JamState) tallyServiceStatistics(guarantees []types.Guarantee, preimages []types.Preimages, accumulateStats map[uint32]*accumulateStatistics) {
	stats := make(map[uint32]*types.ServiceStatistics)
	for _, g := range guarantees { // w -- R(...)
		for _, v := range g.Report.Results {
			cs, ok := stats[v.ServiceID]
			if !ok {
				cs = &types.ServiceStatistics{}
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
			cs = &types.ServiceStatistics{}
			stats[p.Requester] = cs
		}
		cs.NumPreimages += 1                      //p
		cs.NumBytesPreimages += uint(len(p.Blob)) //p
	}
	for s, a := range accumulateStats {
		//fmt.Printf("!!!! Accumulate stats for service %d: numWorkReports %d, gasUsed %d\n", s, a.numWorkReports, a.gasUsed)
		cs, ok := stats[s]
		if !ok {
			cs = &types.ServiceStatistics{}
			stats[s] = cs
		}
		cs.AccumulateNumWorkReports += a.numWorkReports
		//fmt.Printf("!!!! AAAA Accumulate stats for service %d: numWorkReports %d, gasUsed %d\n", s, cs.AccumulateNumWorkReports, cs.AccumulateGasUsed)

		if math.MaxUint64-cs.AccumulateGasUsed < a.gasUsed {
			fmt.Printf("service %d, old data %d, new data %d\n", s, cs.AccumulateGasUsed, a.gasUsed) // overflow check
		} else {
			cs.AccumulateGasUsed += a.gasUsed
		}

	}
	// incorporate stats into ValidatorStatistics
	n.ValidatorStatistics.ServiceStatistics = make(map[uint32]types.ServiceStatistics)
	for s, v := range stats {
		emptyStats := &types.ServiceStatistics{}
		emptyStatsByte, _ := types.Encode(emptyStats) // to ensure no padding bytes
		statsByte, _ := types.Encode(v)
		if v != nil {
			if bytes.Equal(emptyStatsByte, statsByte) {
				//log.Debug(log.SDB, "tallyServiceStatistics: ignoring empty stats for service", "service", s)
				continue
			}
			n.ValidatorStatistics.ServiceStatistics[s] = *v
		}
	}
}
