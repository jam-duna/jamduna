package statedb

import (
	//"github.com/ethereum/go-ethereum/crypto"
	//"bytes"
	//"context"
	//"encoding/json"
	//"fmt"
	"crypto/rand"
	//"time"
	"math/big"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// BeefyRoot creates the Beefy root for the present block using service accumulations.
func BeefyRoot(serviceAccumulations []types.ServiceAccumulation, mmr trie.MerkleMountainRange) common.Hash {
	for _, accumulation := range serviceAccumulations {
		mmr.Append(accumulation.Result.Bytes())
	}
	return mmr.Root()
}

// IsAuthorizedPVM performs the is-authorized PVM function.
func IsAuthorizedPVM(workPackage types.WorkPackage) (bool, error) {
	// Ensure the work-package warrants the needed core-time
	// Ensure all segment-tree roots which form imported segment commitments are known and valid
	// Ensure that all preimage data referenced as commitments of extrinsic segments can be fetched

	// For demonstration, let's assume these checks are passed
	//for _, workItem := range workPackage.WorkItems {

	//}

	return true, nil
}

// FisherYatesShuffle shuffles a slice using the Fisher-Yates algorithm.
func FisherYatesShuffle(slice []int) []int {
	result := make([]int, len(slice))
	copy(result, slice)

	for i := len(result) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		result[i], result[j.Int64()] = result[j.Int64()], result[i]
	}

	return result
}

// RotationFunction calculates the rotation of a core.
func RotationFunction(n int, x int, c int) int {
	return (x + n) % c
}

// PermuteFunction permutes the validators assigned to cores.
func PermuteFunction(e int, t int, validators, cores []int) []int {
	shuffled := FisherYatesShuffle(validators)
	rotated := make([]int, len(cores))

	for i := 0; i < len(cores); i++ {
		rotated[i] = cores[RotationFunction(e, i, len(cores))]
	}

	result := make([]int, len(cores))
	for i := range cores {
		result[i] = shuffled[i%len(shuffled)]
	}

	return result
}

// GuarantorAssignment assigns validators to cores.
func GuarantorAssignment(validatorCount, coreCount, epochLength, rotationPeriod int) [][]int {
	validators := make([]int, validatorCount)
	for i := range validators {
		validators[i] = i
	}

	cores := make([]int, coreCount)
	for i := range cores {
		cores[i] = i
	}

	assignments := make([][]int, epochLength)
	for t := 0; t < epochLength; t++ {
		assignments[t] = PermuteFunction(rotationPeriod, t, validators, cores)
	}

	return assignments
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
