package statedb

import (
	"crypto/rand"
	"math/big"
)

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
