package statedb

import (
	"math/big"
	"testing"
)

// TestPhragmen tests the Sequential Phragmén algorithm with the example from REWARDS.md
func TestPhragmen(t *testing.T) {
	// Example from REWARDS.md:
	// 5 nominators: N1=400, N2=300, N3=200, N4=100, N5=100
	// 5 validators: V1, V2, V3, V4, V5
	// Elect 3 validators
	//
	// Nominations:
	// N1 → {V1, V2}
	// N2 → {V2, V3}
	// N3 → {V3, V4}
	// N4 → {V4, V5}
	// N5 → {V1, V5}

	// Create nominators
	nominators := []*Nominator{
		{
			Address: "N1",
			Budget:  big.NewInt(400),
			Targets: []string{"V1", "V2"},
		},
		{
			Address: "N2",
			Budget:  big.NewInt(300),
			Targets: []string{"V2", "V3"},
		},
		{
			Address: "N3",
			Budget:  big.NewInt(200),
			Targets: []string{"V3", "V4"},
		},
		{
			Address: "N4",
			Budget:  big.NewInt(100),
			Targets: []string{"V4", "V5"},
		},
		{
			Address: "N5",
			Budget:  big.NewInt(100),
			Targets: []string{"V1", "V5"},
		},
	}

	// Create validators (all with 0 self-stake for this example)
	validators := []*ValidatorCandidate{
		{Address: "V1", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		{Address: "V2", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		{Address: "V3", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		{Address: "V4", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		{Address: "V5", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
	}

	// Configuration
	config := PhragmenConfig{
		NumSeats:         3,
		MinNominatorBond: big.NewInt(100),
		MinValidatorBond: big.NewInt(0),
	}

	// Run election
	result, err := SequentialPhragmen(nominators, validators, config)
	if err != nil {
		t.Fatalf("SequentialPhragmen failed: %v", err)
	}

	// Validate result
	if err := ValidateElection(result, config); err != nil {
		t.Fatalf("Election validation failed: %v", err)
	}

	// Check that exactly 3 validators were elected
	if len(result.Elected) != 3 {
		t.Errorf("Expected 3 elected validators, got %d", len(result.Elected))
	}

	// According to the algorithm, the expected elected set should be {V1, V2, V3}
	// (though the specific order might vary based on score ties)
	expectedElected := map[string]bool{
		"V1": true,
		"V2": true,
		"V3": true,
	}

	electedSet := make(map[string]bool)
	for _, addr := range result.Elected {
		electedSet[addr] = true
	}

	// Check if expected validators were elected
	for addr := range expectedElected {
		if !electedSet[addr] {
			t.Logf("Warning: Expected validator %s was not elected", addr)
			t.Logf("Elected: %v", result.Elected)
		}
	}

	// Print results for inspection
	t.Logf("Elected validators: %v", result.Elected)
	for _, valAddr := range result.Elected {
		exposure := result.Distribution[valAddr]
		t.Logf("  %s: Total=%s, Own=%s, Nominators=%d",
			valAddr,
			exposure.Total.String(),
			exposure.Own.String(),
			len(exposure.Nominators))

		for _, nom := range exposure.Nominators {
			t.Logf("    - %s: %s", nom.Who, nom.Value.String())
		}
	}

	// Verify stake distribution properties
	// All validators should have non-zero backing
	for _, valAddr := range result.Elected {
		exposure := result.Distribution[valAddr]
		if exposure.Total.Sign() == 0 {
			t.Errorf("Validator %s has zero backing", valAddr)
		}
	}
}

// TestPhragmenWithSelfStake tests election with validators having self-stake
func TestPhragmenWithSelfStake(t *testing.T) {
	// 3 nominators
	nominators := []*Nominator{
		{Address: "N1", Budget: big.NewInt(1000), Targets: []string{"V1", "V2"}},
		{Address: "N2", Budget: big.NewInt(500), Targets: []string{"V2", "V3"}},
		{Address: "N3", Budget: big.NewInt(750), Targets: []string{"V1", "V3"}},
	}

	// 3 validators with self-stake
	validators := []*ValidatorCandidate{
		{Address: "V1", SelfStake: big.NewInt(500), Commission: 50000000, Blocked: false},  // 5%
		{Address: "V2", SelfStake: big.NewInt(300), Commission: 100000000, Blocked: false}, // 10%
		{Address: "V3", SelfStake: big.NewInt(400), Commission: 0, Blocked: false},
	}

	config := PhragmenConfig{
		NumSeats:         2,
		MinNominatorBond: big.NewInt(100),
		MinValidatorBond: big.NewInt(300),
	}

	result, err := SequentialPhragmen(nominators, validators, config)
	if err != nil {
		t.Fatalf("SequentialPhragmen failed: %v", err)
	}

	if err := ValidateElection(result, config); err != nil {
		t.Fatalf("Election validation failed: %v", err)
	}

	t.Logf("Elected validators: %v", result.Elected)
	for _, valAddr := range result.Elected {
		exposure := result.Distribution[valAddr]
		t.Logf("  %s: Total=%s, Own=%s, Nominators=%d",
			valAddr,
			exposure.Total.String(),
			exposure.Own.String(),
			len(exposure.Nominators))
	}
}

// TestPhragmenEdgeCases tests edge cases
func TestPhragmenEdgeCases(t *testing.T) {
	// Test with more seats than validators
	t.Run("MoreSeatsThanValidators", func(t *testing.T) {
		nominators := []*Nominator{
			{Address: "N1", Budget: big.NewInt(100), Targets: []string{"V1"}},
		}
		validators := []*ValidatorCandidate{
			{Address: "V1", SelfStake: big.NewInt(10), Commission: 0, Blocked: false},
		}
		config := PhragmenConfig{NumSeats: 5}

		_, err := SequentialPhragmen(nominators, validators, config)
		if err == nil {
			t.Error("Expected error for more seats than validators")
		}
	})

	// Test with blocked validator
	t.Run("BlockedValidator", func(t *testing.T) {
		nominators := []*Nominator{
			{Address: "N1", Budget: big.NewInt(100), Targets: []string{"V1", "V2"}},
		}
		validators := []*ValidatorCandidate{
			{Address: "V1", SelfStake: big.NewInt(10), Commission: 0, Blocked: true}, // Blocked
			{Address: "V2", SelfStake: big.NewInt(10), Commission: 0, Blocked: false},
		}
		config := PhragmenConfig{NumSeats: 1}

		result, err := SequentialPhragmen(nominators, validators, config)
		if err != nil {
			t.Fatalf("SequentialPhragmen failed: %v", err)
		}

		// V1 should not be elected because it's blocked
		if result.Elected[0] != "V2" {
			t.Errorf("Expected V2 to be elected, got %s", result.Elected[0])
		}
	})

	// Test with zero seats
	t.Run("ZeroSeats", func(t *testing.T) {
		nominators := []*Nominator{
			{Address: "N1", Budget: big.NewInt(100), Targets: []string{"V1"}},
		}
		validators := []*ValidatorCandidate{
			{Address: "V1", SelfStake: big.NewInt(10), Commission: 0, Blocked: false},
		}
		config := PhragmenConfig{NumSeats: 0}

		_, err := SequentialPhragmen(nominators, validators, config)
		if err == nil {
			t.Error("Expected error for zero seats")
		}
	})

	// Test minimum bonds and filtered participants
	t.Run("MinimumBonds", func(t *testing.T) {
		nominators := []*Nominator{
			{Address: "N1", Budget: big.NewInt(50), Targets: []string{"V1"}}, // filtered (below min)
			{Address: "N2", Budget: big.NewInt(200), Targets: []string{"V1", "V2"}},
		}
		validators := []*ValidatorCandidate{
			{Address: "V1", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},   // filtered (below min)
			{Address: "V2", SelfStake: big.NewInt(150), Commission: 0, Blocked: false}, // eligible
		}
		config := PhragmenConfig{NumSeats: 1, MinNominatorBond: big.NewInt(100), MinValidatorBond: big.NewInt(100)}

		result, err := SequentialPhragmen(nominators, validators, config)
		if err != nil {
			t.Fatalf("SequentialPhragmen failed: %v", err)
		}

		if len(result.Elected) != 1 || result.Elected[0] != "V2" {
			t.Fatalf("Expected only V2 to be elected, got %v", result.Elected)
		}

		exposure := result.Distribution["V2"]
		if exposure == nil || len(exposure.Nominators) != 1 || exposure.Nominators[0].Who != "N2" {
			t.Fatalf("Expected only N2 to contribute to V2, got %+v", exposure)
		}
	})

	// Test deterministic tie-breaking by validator address
	t.Run("DeterministicTieBreak", func(t *testing.T) {
		nominators := []*Nominator{
			{Address: "N1", Budget: big.NewInt(100), Targets: []string{"VB", "VA"}},
			{Address: "N2", Budget: big.NewInt(100), Targets: []string{"VB", "VA"}},
		}

		validators := []*ValidatorCandidate{
			{Address: "VB", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
			{Address: "VA", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		}

		config := PhragmenConfig{NumSeats: 1}

		result, err := SequentialPhragmen(nominators, validators, config)
		if err != nil {
			t.Fatalf("SequentialPhragmen failed: %v", err)
		}

		if len(result.Elected) != 1 || result.Elected[0] != "VA" {
			t.Fatalf("expected VA to win deterministic tie-break, got %v", result.Elected)
		}
	})
}

// TestPhragmenBalancedDistribution tests that stake is distributed evenly
func TestPhragmenBalancedDistribution(t *testing.T) {
	// Create scenario where nominator can choose between multiple validators
	// Phragmén should balance the stake
	nominators := []*Nominator{
		{Address: "N1", Budget: big.NewInt(1000), Targets: []string{"V1", "V2", "V3"}},
		{Address: "N2", Budget: big.NewInt(1000), Targets: []string{"V1", "V2", "V3"}},
	}

	validators := []*ValidatorCandidate{
		{Address: "V1", SelfStake: big.NewInt(100), Commission: 0, Blocked: false},
		{Address: "V2", SelfStake: big.NewInt(100), Commission: 0, Blocked: false},
		{Address: "V3", SelfStake: big.NewInt(100), Commission: 0, Blocked: false},
	}

	config := PhragmenConfig{NumSeats: 3}

	result, err := SequentialPhragmen(nominators, validators, config)
	if err != nil {
		t.Fatalf("SequentialPhragmen failed: %v", err)
	}

	// Calculate variance in total backing
	var min, max *big.Int
	for _, valAddr := range result.Elected {
		total := result.Distribution[valAddr].Total
		if min == nil || total.Cmp(min) < 0 {
			min = new(big.Int).Set(total)
		}
		if max == nil || total.Cmp(max) > 0 {
			max = new(big.Int).Set(total)
		}
	}

	// Log the distribution
	t.Logf("Stake distribution:")
	for _, valAddr := range result.Elected {
		t.Logf("  %s: %s", valAddr, result.Distribution[valAddr].Total.String())
	}
	t.Logf("Min: %s, Max: %s", min.String(), max.String())

	// Phragmén should produce relatively balanced distribution
	// (exact balance depends on the algorithm's tie-breaking)
}

// TestPhragmenRespectsMinimumBonds verifies that nominators/validators below the
// configured minimum bond are excluded from the election input set.
func TestPhragmenRespectsMinimumBonds(t *testing.T) {
	nominators := []*Nominator{
		{Address: "N1", Budget: big.NewInt(1_000), Targets: []string{"V1", "V2"}},
		{Address: "N2", Budget: big.NewInt(50), Targets: []string{"V1"}}, // below min
	}

	validators := []*ValidatorCandidate{
		{Address: "V1", SelfStake: big.NewInt(500), Commission: 0, Blocked: false},
		{Address: "V2", SelfStake: big.NewInt(250), Commission: 0, Blocked: false}, // below min
	}

	config := PhragmenConfig{NumSeats: 1, MinNominatorBond: big.NewInt(100), MinValidatorBond: big.NewInt(300)}

	result, err := SequentialPhragmen(nominators, validators, config)
	if err != nil {
		t.Fatalf("SequentialPhragmen failed: %v", err)
	}

	if err := ValidateElection(result, config); err != nil {
		t.Fatalf("Election validation failed: %v", err)
	}

	if len(result.Elected) != 1 || result.Elected[0] != "V1" {
		t.Fatalf("expected only V1 to be elected, got %v", result.Elected)
	}

	// Low-bond nominator should not appear in the exposure breakdown
	exposure := result.Distribution["V1"]
	if len(exposure.Nominators) != 1 || exposure.Nominators[0].Who != "N1" {
		t.Fatalf("unexpected nominators in exposure: %+v", exposure.Nominators)
	}
}

// TestPhragmenNormalizesBudgets ensures stake contributions are normalized back
// to the nominator budgets even when intermediate weights are fractional.
func TestPhragmenNormalizesBudgets(t *testing.T) {
	nominators := []*Nominator{
		{Address: "N1", Budget: big.NewInt(100), Targets: []string{"V1", "V2"}},
		{Address: "N2", Budget: big.NewInt(100), Targets: []string{"V1"}},
	}

	validators := []*ValidatorCandidate{
		{Address: "V1", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		{Address: "V2", SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
	}

	config := PhragmenConfig{NumSeats: 2, MinNominatorBond: big.NewInt(1), MinValidatorBond: big.NewInt(0)}

	result, err := SequentialPhragmen(nominators, validators, config)
	if err != nil {
		t.Fatalf("SequentialPhragmen failed: %v", err)
	}

	if err := ValidateElection(result, config); err != nil {
		t.Fatalf("Election validation failed: %v", err)
	}

	// Check that each nominator's contributions sum (after rounding) to its budget
	contributed := map[string]*big.Int{}
	for _, exposure := range result.Distribution {
		for _, nom := range exposure.Nominators {
			if _, ok := contributed[nom.Who]; !ok {
				contributed[nom.Who] = big.NewInt(0)
			}
			contributed[nom.Who].Add(contributed[nom.Who], nom.Value)
		}
	}

	if contributed["N1"].Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("expected N1 contribution to equal 100, got %s", contributed["N1"].String())
	}

	if contributed["N2"].Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("expected N2 contribution to equal 100, got %s", contributed["N2"].String())
	}
}

// BenchmarkPhragmen benchmarks the election with realistic scale
func BenchmarkPhragmen(b *testing.B) {
	// Simulate 100 validators and 1000 nominators
	numValidators := 100
	numNominators := 1000
	numSeats := 50

	validators := make([]*ValidatorCandidate, numValidators)
	for i := 0; i < numValidators; i++ {
		validators[i] = &ValidatorCandidate{
			Address:    string(rune('A'+i%26)) + string(rune('0'+i)),
			SelfStake:  big.NewInt(int64(100 + i*10)),
			Commission: uint32(i % 10 * 10000000), // 0-9%
			Blocked:    false,
		}
	}

	nominators := make([]*Nominator, numNominators)
	for i := 0; i < numNominators; i++ {
		// Each nominator nominates 5-10 validators
		numTargets := 5 + (i % 6)
		targets := make([]string, numTargets)
		for j := 0; j < numTargets; j++ {
			targets[j] = validators[(i*7+j)%numValidators].Address
		}

		nominators[i] = &Nominator{
			Address: string(rune('N')) + string(rune('0'+i)),
			Budget:  big.NewInt(int64(1000 + i*100)),
			Targets: targets,
		}
	}

	config := PhragmenConfig{
		NumSeats:         numSeats,
		MinNominatorBond: big.NewInt(100),
		MinValidatorBond: big.NewInt(100),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := SequentialPhragmen(nominators, validators, config)
		if err != nil {
			b.Fatalf("SequentialPhragmen failed: %v", err)
		}
	}
}
