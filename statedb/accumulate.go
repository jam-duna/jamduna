package statedb

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/types"
)

type BeefyPool []Beta_state

type Peaks []*common.Hash
type MMR struct {
	Peaks Peaks `json:"peaks"`
}

type Beta_state struct {
	HeaderHash common.Hash   `json:"header_hash"`
	MMR        MMR           `json:"mmr"`
	StateRoot  common.Hash   `json:"state_root"`
	Reported   []common.Hash `json:"reported"`
}

func (s *StateDB) Accumulate() (serviceAccumulations []types.ServiceAccumulation, reported []common.Hash, err error) {
	xContext := types.NewXContext()
	s.SetXContext(xContext)
	reported = make([]common.Hash, 0)
	// ServiceAccumulation represents a service accumulation result.
	serviceAccumulations = make([]types.ServiceAccumulation, 0)
	for _, workReport := range s.AvailableWorkReport {
		wrangledWorkResults := make([]types.WrangledWorkResult, 0)
		// Wrangle results from work report
		for _, workResult := range workReport.Results {
			wrangledWorkResult := workResult.Wrangle(workReport.AuthOutput, workReport.AvailabilitySpec.WorkPackageHash)
			wrangledWorkResults = append(wrangledWorkResults, wrangledWorkResult)
			serviceIndex := workResult.Service
			code := s.ReadServicePreimageBlob(serviceIndex, workResult.CodeHash)
			Xs, _ := s.GetService(serviceIndex)
			X := s.GetXContext()
			X.S = Xs
			s.UpdateXContext(X)
			wrangledWorkResultsBytes := s.getWrangledWorkResultsBytes(wrangledWorkResults)
			vm := pvm.NewVMFromCode(serviceIndex, code, 0, s)
			r, _ := vm.ExecuteAccumulate(wrangledWorkResultsBytes)
			if r.Err == types.RESULT_OK {
				accumulationResult := common.Blake2Hash(r.Ok) // checks
				serviceAccumulation := types.ServiceAccumulation{
					ServiceIndex: serviceIndex,
					Result:       accumulationResult,
				}
				serviceAccumulations = append(serviceAccumulations, serviceAccumulation)
			}
			reported = append(reported, workReport.AvailabilitySpec.WorkPackageHash)
		}

	}
	return serviceAccumulations, reported, nil
}

// CalculateGasAttributable calculates the gas attributable for each service.
func CalculateGasAttributable(
	workReports map[int][]types.WorkReport, // workReports maps service index to their respective work reports
	privilegedServices []int, // privilegedServices contains indices of privileged services
	electiveAccumulationGas float64, // electiveAccumulationGas represents GA in the formula
) []types.GasAttributable {
	gasAttributable := []types.GasAttributable{}
	serviceGasTotals := make(map[int]float64)

	for serviceIndex, reports := range workReports {
		var gasSum float64
		for range reports {
			gasSum += 1 // TODO
		}
		serviceGasTotals[serviceIndex] = gasSum
	}

	for _, serviceIndex := range privilegedServices {
		if _, exists := serviceGasTotals[serviceIndex]; !exists {
			serviceGasTotals[serviceIndex] = 0
		}
	}

	var totalGasSum float64
	for _, gas := range serviceGasTotals {
		totalGasSum += gas
	}

	for serviceIndex, gasSum := range serviceGasTotals {
		gas := gasSum + electiveAccumulationGas*(1-(gasSum/totalGasSum))
		gasAttributable = append(gasAttributable, types.GasAttributable{
			ServiceIndex: serviceIndex,
			Gas:          gas,
		})
	}

	return gasAttributable
}
