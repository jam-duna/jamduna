package statedb

import (
	"fmt"
	"math"
	"math/big"
	"sort"

	"github.com/colorfulnotion/jam/types"
)

// Phragmén Election Algorithm Implementation
// Based on Sequential Phragmén method for NPoS validator election
// Reference: https://research.web3.foundation/Polkadot/protocols/NPoS/Overview

// Nominator represents a nominator with their stake and validator preferences
type Nominator struct {
	Address string   // Nominator's address
	Budget  *big.Int // Total stake (USDM tokens)
	Targets []string // List of nominated validator addresses
	Load    *big.Rat // Current load (updated during election)
}

// ValidatorCandidate represents a validator candidate
type ValidatorCandidate struct {
	Address    string   // Validator's address
	SelfStake  *big.Int // Validator's own stake
	Commission uint32   // Commission rate (0-1,000,000,000 = 0-100%)
	Blocked    bool     // Whether accepting new nominations
	Score      *big.Rat // Election score (updated during election)
}

// EdgeWeight represents stake allocation from a nominator to a validator
type EdgeWeight struct {
	Nominator string   // Nominator address
	Validator string   // Validator address
	Weight    *big.Rat // Stake amount (as rational for precision)
	Budget    *big.Int // Original nominator budget for normalization
}

// ElectionResult contains the outcome of the Phragmén election
type ElectionResult struct {
	Elected      []string             // Elected validator addresses
	Distribution map[string]*Exposure // Stake distribution per validator
	EdgeWeights  []EdgeWeight         // All nominator→validator allocations
}

// Exposure represents a validator's backing in an era
type Exposure struct {
	Total      *big.Int             // Total backing
	Own        *big.Int             // Validator's own stake
	Nominators []IndividualExposure // Nominator contributions
}

// IndividualExposure represents a single nominator's contribution
type IndividualExposure struct {
	Who   string   // Nominator address
	Value *big.Int // Stake amount
}

// PhragmenConfig holds election configuration
type PhragmenConfig struct {
	NumSeats         int      // Number of validators to elect
	MinNominatorBond *big.Int // Minimum nominator stake
	MinValidatorBond *big.Int // Minimum validator stake
}

// SequentialPhragmen runs the Sequential Phragmén election algorithm
//
// Input:
//   - nominators: List of nominators with their stakes and preferences
//   - validators: List of validator candidates
//   - config: Election configuration (number of seats, min bonds)
//
// Output:
//   - ElectionResult: Elected validators and stake distribution
//   - error: Any errors during election
//
// Algorithm:
//  1. Initialize all loads to 0
//  2. For each seat to elect:
//     a. Calculate scores for unelected validators
//     b. Select validator with lowest score
//     c. Update nominator loads
//     d. Record edge weights
//  3. Normalize weights and return results
func SequentialPhragmen(
	nominators []*Nominator,
	validators []*ValidatorCandidate,
	config PhragmenConfig,
) (*ElectionResult, error) {
	if config.NumSeats <= 0 {
		return nil, fmt.Errorf("numSeats must be positive")
	}

	// Filter out nominators that don't meet the minimum bond
	eligibleNominators := make([]*Nominator, 0, len(nominators))
	for _, nom := range nominators {
		if config.MinNominatorBond != nil && nom.Budget.Cmp(config.MinNominatorBond) < 0 {
			continue
		}
		eligibleNominators = append(eligibleNominators, nom)
	}

	if len(eligibleNominators) == 0 {
		minBond := "0"
		if config.MinNominatorBond != nil {
			minBond = config.MinNominatorBond.String()
		}
		return nil, fmt.Errorf("no nominators meet minimum bond of %s", minBond)
	}

	// Filter out validators that don't meet the minimum bond
	eligibleValidators := make([]*ValidatorCandidate, 0, len(validators))
	for _, val := range validators {
		selfStake := val.SelfStake
		if selfStake == nil {
			selfStake = big.NewInt(0)
		}

		if config.MinValidatorBond != nil && selfStake.Cmp(config.MinValidatorBond) < 0 {
			continue
		}

		eligibleValidators = append(eligibleValidators, val)
	}

	if config.NumSeats > len(eligibleValidators) {
		return nil, fmt.Errorf("numSeats (%d) exceeds eligible validator count (%d)", config.NumSeats, len(eligibleValidators))
	}

	nominators = eligibleNominators
	validators = eligibleValidators

	// Initialize
	elected := make([]string, 0, config.NumSeats)
	electedSet := make(map[string]bool)
	edgeWeights := make([]EdgeWeight, 0)

	// Initialize nominator loads to 0
	for _, nom := range nominators {
		nom.Load = big.NewRat(0, 1)
	}

	// Initialize validator scores to 0
	for _, val := range validators {
		val.Score = big.NewRat(0, 1)
	}

	// Create nominator map for quick lookup
	nomMap := make(map[string]*Nominator)
	for _, nom := range nominators {
		nomMap[nom.Address] = nom
	}

	// Election rounds: elect one validator at a time
	for round := 0; round < config.NumSeats; round++ {
		// Step 1: Calculate scores for unelected validators
		var bestCandidate *ValidatorCandidate
		var bestScore *big.Rat

		for _, val := range validators {
			// Skip already elected
			if electedSet[val.Address] {
				continue
			}

			// Skip blocked validators
			if val.Blocked {
				continue
			}

			// Calculate score: l_v = (1 + Σ(l_n * b_n)) / Σ(b_n)
			// where sum is over nominators backing this validator
			score := calculateValidatorScore(val, nominators)
			val.Score = score

			// Select validator with lowest score; break ties deterministically by address
			if bestCandidate == nil || score.Cmp(bestScore) < 0 || (score.Cmp(bestScore) == 0 && val.Address < bestCandidate.Address) {
				bestScore = score
				bestCandidate = val
			}
		}

		if bestCandidate == nil {
			return nil, fmt.Errorf("no valid candidate found in round %d", round)
		}

		// Step 2: Elect the best candidate
		elected = append(elected, bestCandidate.Address)
		electedSet[bestCandidate.Address] = true

		// Step 3: Update nominator loads and record edge weights
		for _, nom := range nominators {
			// Check if this nominator backs the elected validator
			backs := false
			for _, target := range nom.Targets {
				if target == bestCandidate.Address {
					backs = true
					break
				}
			}

			if !backs {
				continue
			}

			// Calculate edge weight: w_{n,v} = (l_v - l_n) * b_n
			diff := new(big.Rat).Sub(bestCandidate.Score, nom.Load)
			budgetRat := new(big.Rat).SetInt(nom.Budget)
			weight := new(big.Rat).Mul(diff, budgetRat)

			// Record edge weight (keep as rational for now)
			if weight.Sign() > 0 {
				edgeWeights = append(edgeWeights, EdgeWeight{
					Nominator: nom.Address,
					Validator: bestCandidate.Address,
					Weight:    new(big.Rat).Set(weight),
					Budget:    new(big.Int).Set(nom.Budget),
				})
			}

			// Update nominator load: l_n = l_v
			nom.Load = new(big.Rat).Set(bestCandidate.Score)
		}
	}

	// Step 4: Build exposure distribution (weights already calculated)
	distribution := buildExposureDistribution(elected, validators, edgeWeights)

	return &ElectionResult{
		Elected:      elected,
		Distribution: distribution,
		EdgeWeights:  edgeWeights,
	}, nil
}

// calculateValidatorScore computes the score for a validator
// Score formula: l_v = (1 + Σ(l_n * b_n)) / Σ(b_n)
// where the sum is over nominators backing this validator
func calculateValidatorScore(val *ValidatorCandidate, nominators []*Nominator) *big.Rat {
	numerator := big.NewRat(1, 1) // Start with 1
	denominator := big.NewRat(0, 1)

	// Sum over nominators backing this validator
	for _, nom := range nominators {
		// Check if nominator backs this validator
		backs := false
		for _, target := range nom.Targets {
			if target == val.Address {
				backs = true
				break
			}
		}

		if !backs {
			continue
		}

		// Add l_n * b_n to numerator
		budgetRat := new(big.Rat).SetInt(nom.Budget)
		product := new(big.Rat).Mul(nom.Load, budgetRat)
		numerator.Add(numerator, product)

		// Add b_n to denominator
		denominator.Add(denominator, budgetRat)
	}

	// Add validator's own stake to denominator
	if val.SelfStake != nil && val.SelfStake.Sign() > 0 {
		selfStakeRat := new(big.Rat).SetInt(val.SelfStake)
		denominator.Add(denominator, selfStakeRat)
	}

	// Avoid division by zero
	if denominator.Sign() == 0 {
		return big.NewRat(math.MaxInt64, 1) // Very high score if no backing
	}

	// Return score = numerator / denominator
	score := new(big.Rat).Quo(numerator, denominator)
	return score
}

// buildExposureDistribution creates Exposure objects from edge weights
func buildExposureDistribution(
	elected []string,
	validators []*ValidatorCandidate,
	edgeWeights []EdgeWeight,
) map[string]*Exposure {
	distribution := make(map[string]*Exposure)

	// Create validator map for quick lookup
	valMap := make(map[string]*ValidatorCandidate)
	for _, val := range validators {
		valMap[val.Address] = val
	}

	// Initialize exposures for elected validators
	for _, valAddr := range elected {
		val := valMap[valAddr]
		selfStake := big.NewInt(0)
		if val.SelfStake != nil {
			selfStake = new(big.Int).Set(val.SelfStake)
		}
		distribution[valAddr] = &Exposure{
			Total:      new(big.Int).Set(selfStake),
			Own:        new(big.Int).Set(selfStake),
			Nominators: make([]IndividualExposure, 0),
		}
	}

	// Aggregate edge weights by validator, normalizing by nominator budget so that
	// each nominator's contributions sum to their full stake.
	nomStakes := make(map[string]map[string]*big.Rat) // validator -> nominator -> stake
	edgesByNominator := make(map[string][]EdgeWeight)
	for _, edge := range edgeWeights {
		edgesByNominator[edge.Nominator] = append(edgesByNominator[edge.Nominator], edge)
	}

	for nomAddr, edges := range edgesByNominator {
		totalWeight := big.NewRat(0, 1)
		for _, edge := range edges {
			totalWeight.Add(totalWeight, edge.Weight)
		}

		if totalWeight.Sign() == 0 {
			continue
		}

		for _, edge := range edges {
			if _, exists := nomStakes[edge.Validator]; !exists {
				nomStakes[edge.Validator] = make(map[string]*big.Rat)
			}

			share := new(big.Rat).Quo(edge.Weight, totalWeight)
			budgetRat := new(big.Rat).SetInt(edge.Budget)
			stake := new(big.Rat).Mul(share, budgetRat)

			if existing, ok := nomStakes[edge.Validator][nomAddr]; ok {
				nomStakes[edge.Validator][nomAddr] = new(big.Rat).Add(existing, stake)
			} else {
				nomStakes[edge.Validator][nomAddr] = stake
			}
		}
	}

	// Build exposure nominators list
	for valAddr, exposure := range distribution {
		nomMap := nomStakes[valAddr]

		for nomAddr, stakeRat := range nomMap {
			if stakeRat.Sign() > 0 {
				stakeInt := roundRatToInt(stakeRat)

				exposure.Nominators = append(exposure.Nominators, IndividualExposure{
					Who:   nomAddr,
					Value: stakeInt,
				})

				// Add to total
				exposure.Total.Add(exposure.Total, stakeInt)
			}
		}

		// Sort nominators by stake (descending)
		sort.Slice(exposure.Nominators, func(i, j int) bool {
			return exposure.Nominators[i].Value.Cmp(exposure.Nominators[j].Value) > 0
		})
	}

	return distribution
}

// ValidateElection checks if election result satisfies basic constraints
func ValidateElection(result *ElectionResult, config PhragmenConfig) error {
	if len(result.Elected) != config.NumSeats {
		return fmt.Errorf("expected %d validators, got %d", config.NumSeats, len(result.Elected))
	}

	// Check all elected validators have exposure
	for _, valAddr := range result.Elected {
		exposure, exists := result.Distribution[valAddr]
		if !exists {
			return fmt.Errorf("no exposure found for elected validator %s", valAddr)
		}

		// Check total matches own + nominators
		sum := new(big.Int).Set(exposure.Own)
		for _, nom := range exposure.Nominators {
			sum.Add(sum, nom.Value)
		}

		if sum.Cmp(exposure.Total) != 0 {
			return fmt.Errorf("exposure total mismatch for %s: calculated %s, stored %s",
				valAddr, sum.String(), exposure.Total.String())
		}
	}

	return nil
}

// roundRatToInt rounds a rational number to the nearest integer (half-up)
func roundRatToInt(r *big.Rat) *big.Int {
	num := new(big.Int).Set(r.Num())
	den := new(big.Int).Set(r.Denom())

	// half = den / 2
	half := new(big.Int).Div(den, big.NewInt(2))

	// num = num + half
	num.Add(num, half)

	// result = num / den
	return new(big.Int).Div(num, den)
}

// EncodeValidatorsForDesignate encodes a list of validators into the format expected by the bootstrap service
// This creates a byte array of 336 * TotalValidators bytes (343,728 bytes for 1023 validators)
// Each validator is encoded as:
//   - 32 bytes: Bandersnatch key
//   - 32 bytes: Ed25519 key
//   - 144 bytes: BLS key
//   - 128 bytes: metadata
//
// This byte array can be used as a payload for the bootstrap service's accumulate function,
// which will validate it in refine and call the designate host function in accumulate.
//
// Usage:
//
//	validators := []types.Validator{...} // from Phragmén election result
//	payload, err := EncodeValidatorsForDesignate(validators, types.TotalValidators)
//	// Submit payload to bootstrap service via work item
func EncodeValidatorsForDesignate(validators types.Validators) ([]byte, error) {
	const validatorSize = 336

	if len(validators) == 0 || len(validators) > types.TotalValidators {
		return nil, fmt.Errorf("invalid validator count: got %d, must be 1-%d", len(validators), types.TotalValidators)
	}

	// CRITICAL: hostDesignate always reads 336*TotalValidators bytes (hostfunctions.go:434)
	// Must pad to TotalValidators or accumulate will panic with WORKDIGEST_PANIC
	expectedSize := validatorSize * types.TotalValidators
	result := make([]byte, expectedSize) // Zero-initialized, padding is automatic

	for i, validator := range validators {
		offset := i * validatorSize

		// Copy Bandersnatch key (32 bytes)
		copy(result[offset:offset+32], validator.Bandersnatch[:])

		// Copy Ed25519 key (32 bytes)
		copy(result[offset+32:offset+64], validator.Ed25519[:])

		// Copy BLS key (144 bytes)
		copy(result[offset+64:offset+208], validator.Bls[:])

		// Copy metadata (128 bytes)
		copy(result[offset+208:offset+336], validator.Metadata[:])
	}
	// Remaining (TotalValidators - len(validators)) entries are zero-padded

	return result, nil
}
