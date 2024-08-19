package node

import (
	"crypto/rand"
	//"crypto/sha256"
	//"errors"
	"fmt"
	"math/big"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/trie"
)

/*
// AvailabilitySpecifier creates an availability specifier.
func AvailabilitySpecifier(packageHash common.Hash, octetSequence []byte, workPackage WorkPackage, mt trie.MerkleTree) common.Hash {
	// Add the work package data to the Merkle tree
	mt.AddData(workPackage.AuthorizationToken)
	mt.AddData(workPackage.ParamBlob)
	mt.AddData(workPackage.Context)
	for _, workItem := range workPackage.WorkItems {
		mt.AddData(workItem.PayloadBlob)
	}

	// Calculate the Merkle root
	root := mt.GetRoot()

	// Concatenate the package hash and Merkle root to create the availability specifier
	specifier := append(packageHash.Bytes(), root.Bytes()...)
	specifierHash := sha256.Sum256(specifier)

	return common.BytesToHash(specifierHash[:])
}

// PagedProofs function accepts a sequence of segments and returns a sequence of paged-proofs.
func PagedProofs(segments []common.Hash, mt trie.MerkleTree) ([][][]byte, error) {
	var proofs [][][]byte
	for i, segment := range segments {
		mt.AddData(segment.Bytes())
		proof, err := mt.GetProof(i)
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, proof)
	}
	return proofs, nil
}
*/
// isValidSegmentTreeRoot is a placeholder for checking the validity of a segment-tree root.
func isValidSegmentTreeRoot(root common.Hash) bool {
	// Add the logic to validate the segment-tree root
	return true
}

// canFetchPreimageData is a placeholder for checking if preimage data can be fetched.
func canFetchPreimageData(dataSegments []common.Hash) bool {
	// Add the logic to fetch and validate preimage data
	return true
}

// IsAuthorizedPVM performs the is-authorized PVM function.
func IsAuthorizedPVM(workPackage WorkPackage) (bool, error) {
	// Ensure the work-package warrants the needed core-time
	// Ensure all segment-tree roots which form imported segment commitments are known and valid
	// Ensure that all preimage data referenced as commitments of extrinsic segments can be fetched

	// For demonstration, let's assume these checks are passed
	//for _, workItem := range workPackage.WorkItems {

	//}

	return true, nil
}

// RefinePVM performs the refine PVM function.
func (n *Node) RefinePVM(pvm *pvm.VM, workPackage WorkPackage, workItem WorkItem) (string, error) {
	// For demonstration, simply return "refined_result"
	return "refined_result", nil
}

// Accumulate function performs the accumulation of a single service.
func (n *Node) Accumulate(serviceIndex int, state AccumulationState) (AccumulationState, error) {
	// Wrangle results for the service (simplified for demonstration)
	wrangledResults := state.WorkReports // Assuming wrangled results are the work reports

	// Calculate gas limit for the service
	// TODO: gasLimit := 1000

	// Create the arguments for the VM invocation
	args := AccumulationState{
		ServiceIndices:    []int{serviceIndex},
		WorkReports:       wrangledResults,
		DeferredTransfers: state.DeferredTransfers,
	}

	// Call the virtual machine
	code := []byte{}
	vm := pvm.NewVMFromCode(code, 0, n.NewNodeHostEnv())
	err := vm.Execute()
	if err != nil {
		return AccumulationState{}, err
	}
	newState := args

	return newState, nil
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
	workReports map[int][]WorkReport, // workReports maps service index to their respective work reports
	privilegedServices []int, // privilegedServices contains indices of privileged services
	electiveAccumulationGas float64, // electiveAccumulationGas represents GA in the formula
) []GasAttributable {
	gasAttributable := []GasAttributable{}
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
		gasAttributable = append(gasAttributable, GasAttributable{
			ServiceIndex: serviceIndex,
			Gas:          gas,
		})
	}

	return gasAttributable
}

// BeefyRoot creates the Beefy root for the present block using service accumulations.
func BeefyRoot(serviceAccumulations []ServiceAccumulation, mmr trie.MerkleMountainRange) common.Hash {
	for _, accumulation := range serviceAccumulations {
		mmr.Append(accumulation.Result.Bytes())
	}
	return mmr.Root()
}

// ProcessWorkResults processes the work results using the PVM.
func (n *Node) ProcessWorkResults(workPackage WorkPackage) (string, error) {
	isAuthorized, err := IsAuthorizedPVM(workPackage)
	if err != nil {
		return "", err
	}

	if !isAuthorized {
		return "error: not authorized", nil
	}
	// TODO
	code := []byte{}
	pvm := pvm.NewVMFromCode(code, 0, n.NewNodeHostEnv())
	var results []string
	for _, workItem := range workPackage.WorkItems {
		refinedResult, err := n.RefinePVM(pvm, workPackage, workItem)
		if err != nil {
			return "", err
		}
		results = append(results, refinedResult)
	}

	// For demonstration, return the concatenated results
	return fmt.Sprintf("results: %v", results), nil
}

// Tally updates the statistics for validators based on their activities.
func (sr *StatisticalReporter) Tally(validatorIndex int, activity string, count int) {
	switch activity {
	case "blocks":
		sr.CurrentEpochStats[validatorIndex].BlocksProduced += count
	case "tickets":
		sr.CurrentEpochStats[validatorIndex].TicketsIntroduced += count
	case "preimages":
		sr.CurrentEpochStats[validatorIndex].PreimagesIntroduced += count
	case "octets":
		sr.CurrentEpochStats[validatorIndex].OctetsIntroduced += count
	case "reports":
		sr.CurrentEpochStats[validatorIndex].ReportsGuaranteed += count
	case "assurances":
		sr.CurrentEpochStats[validatorIndex].AvailabilityAssurances += count
	default:
		fmt.Println("Unknown activity:", activity)
	}
}
