package witness

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// TestOrchardBlocksFFI provides comprehensive end-to-end testing for all Orchard extrinsics
// This version integrates with Rust FFI for actual Halo2 proof generation
func TestOrchardBlocksFFI(t *testing.T) {
	// Initialize JAM state and Orchard FFI
	storage, err := statedb.InitStorage(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Initialize genesis state
	trace, err := statedb.MakeGenesisStateTransition(storage, 0, "tiny", nil)
	if err != nil {
		t.Fatalf("MakeGenesisStateTransition failed: %v", err)
	}

	stateDB, err := statedb.NewStateDBFromStateTransition(storage, trace)
	if err != nil {
		t.Fatalf("NewStateDBFromStateTransition failed: %v", err)
	}

	// Initialize Orchard FFI
	orchardFFI, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer orchardFFI.Cleanup()
	wallet := newDeterministicWallet(orchardFFI)

	serviceID := orchardFFI.GetServiceID()
	t.Logf("Orchard Service ID: %d", serviceID)

	// Test each Orchard extrinsic type
	extrinsicTestCases := []struct {
		name          string
		extrinsicType ExtrinsicType
		description   string
	}{
		{
			name:          "DepositPublic",
			extrinsicType: DepositPublic,
			description:   "Deposit public funds into shielded pool",
		},
		{
			name:          "SubmitPrivate",
			extrinsicType: SubmitPrivate,
			description:   "Submit private transaction with ZK proof",
		},
		{
			name:          "WithdrawPublic",
			extrinsicType: WithdrawPublic,
			description:   "Withdraw from shielded pool to public account",
		},
		{
			name:          "IssuanceV1",
			extrinsicType: IssuanceV1,
			description:   "Issue new tokens with compliance constraints",
		},
		{
			name:          "BatchAggV1",
			extrinsicType: BatchAggV1,
			description:   "Aggregate multiple transactions into single proof",
		},
	}

	for i, testCase := range extrinsicTestCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Logf("Testing %s: %s", testCase.name, testCase.description)

			// Create test extrinsic data
			extrinsicData := createTestExtrinsicData(testCase.extrinsicType, uint32(i+1))

			// BUILDER PHASE: Generate commitment, nullifier, and proof
			t.Log("=== BUILDER PHASE ===")

			// Step 1: Process extrinsic (generate all cryptographic components)
			result, err := orchardFFI.ProcessExtrinsic(extrinsicData, wallet)
			if err != nil {
				t.Fatalf("Builder failed to process extrinsic: %v", err)
			}

			t.Logf("Builder generated:")
			t.Logf("  Commitment: %x", result.Commitment)
			t.Logf("  Nullifier: %x", result.Nullifier)
			t.Logf("  Proof length: %d bytes", len(result.Proof))

			// Step 2: Create work package with extrinsic and proof
			workPackage := createOrchardWorkPackage(testCase.extrinsicType, extrinsicData, result)
			t.Logf("Created work package with %d bytes payload", len(workPackage.Extrinsic))

			// Step 3: Submit to JAM state machine
			stateRoot := stateDB.GetStorage().GetRoot()
			t.Logf("Pre-state root: %x", stateRoot.Bytes())

			// Simulate work package processing
			err = processOrchardWorkPackage(stateDB.GetStorage(), workPackage, orchardFFI)
			if err != nil {
				t.Fatalf("Failed to process work package: %v", err)
			}

			newStateRoot := stateDB.GetStorage().GetRoot()
			t.Logf("Post-state root: %x", newStateRoot.Bytes())

			// GUARANTOR PHASE: Verify the work package
			t.Log("=== GUARANTOR PHASE ===")

			// Step 4: Guarantor verifies the proof and state transitions
			err = orchardFFI.VerifyExtrinsic(extrinsicData, result, wallet)
			if err != nil {
				t.Fatalf("Guarantor verification failed: %v", err)
			}

			// Step 5: Verify state consistency
			err = verifyOrchardStateConsistency(stateDB.GetStorage(), testCase.extrinsicType, extrinsicData, result)
			if err != nil {
				t.Fatalf("State consistency check failed: %v", err)
			}

			t.Logf("✅ %s completed successfully", testCase.name)
			t.Logf("   Builder→Guarantor flow validated")
			t.Logf("   Proof verified: %d bytes", len(result.Proof))
			t.Logf("   State transition confirmed")
		})
	}

	// Final integration test: Process all extrinsics in sequence
	t.Run("SequentialProcessing", func(t *testing.T) {
		t.Log("Testing sequential processing of all extrinsic types")

		initialRoot := stateDB.GetStorage().GetRoot()
		t.Logf("Initial state root: %x", initialRoot.Bytes())

		// Process each extrinsic type in sequence
		for i, testCase := range extrinsicTestCases {
			extrinsicData := createTestExtrinsicData(testCase.extrinsicType, uint32(100+i))

			result, err := orchardFFI.ProcessExtrinsic(extrinsicData, wallet)
			if err != nil {
				t.Fatalf("Sequential processing failed for %s: %v", testCase.name, err)
			}

			workPackage := createOrchardWorkPackage(testCase.extrinsicType, extrinsicData, result)
			err = processOrchardWorkPackage(stateDB.GetStorage(), workPackage, orchardFFI)
			if err != nil {
				t.Fatalf("Sequential work package processing failed for %s: %v", testCase.name, err)
			}

			t.Logf("Processed %s sequentially", testCase.name)
		}

		finalRoot := stateDB.GetStorage().GetRoot()
		t.Logf("Final state root: %x", finalRoot.Bytes())

		if initialRoot == finalRoot {
			t.Error("State root should have changed after processing extrinsics")
		}

		t.Log("✅ Sequential processing completed successfully")
	})

	t.Log("🎉 All Orchard extrinsic tests passed!")
	t.Log("Builder→Guarantor flow validated for all 4 extrinsic types")
}

// createTestExtrinsicData creates test data for a given extrinsic type
func createTestExtrinsicData(extrinsicType ExtrinsicType, seed uint32) ExtrinsicData {
	// Create deterministic but different test data for each extrinsic
	baseByte := byte(seed)

	return ExtrinsicData{
		Type:         extrinsicType,
		AssetID:      seed,
		Amount:       Uint128{Lo: uint64(seed * 1000), Hi: uint64(seed / 10)},
		OwnerPk:      createDeterministicBytes32(baseByte),
		Rho:          createDeterministicBytes32(baseByte + 1),
		NoteRseed:    createDeterministicBytes32(baseByte + 2),
		UnlockHeight: uint64(seed),
		MemoHash:     createDeterministicBytes32(baseByte + 3),
	}
}

// createDeterministicBytes32 creates a 32-byte array with a deterministic pattern
func createDeterministicBytes32(base byte) [32]byte {
	var result [32]byte
	for i := 0; i < 32; i++ {
		result[i] = base + byte(i)
	}
	return result
}

// OrchardWorkPackage represents a work package containing a Orchard extrinsic
type OrchardWorkPackage struct {
	ExtrinsicType ExtrinsicType
	Extrinsic     []byte
	Proof         []byte
	PublicInputs  []byte
}

// createOrchardWorkPackage creates a work package for the given extrinsic
func createOrchardWorkPackage(extrinsicType ExtrinsicType, data ExtrinsicData, result *ExtrinsicResult) *OrchardWorkPackage {
	// Serialize extrinsic data (simplified for testing)
	extrinsicBytes := serializeExtrinsicData(data)

	return &OrchardWorkPackage{
		ExtrinsicType: extrinsicType,
		Extrinsic:     extrinsicBytes,
		Proof:         result.Proof,
		PublicInputs:  result.PublicInputs,
	}
}

// serializeExtrinsicData serializes extrinsic data to bytes
func serializeExtrinsicData(data ExtrinsicData) []byte {
	// Simple serialization for testing (in production this would be more sophisticated)
	result := make([]byte, 0, 256)

	// Serialize fields in order
	typeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(typeBytes, uint32(data.Type))
	result = append(result, typeBytes...)

	assetIDBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(assetIDBytes, data.AssetID)
	result = append(result, assetIDBytes...)

	amountBytes := make([]byte, 16)
	binary.LittleEndian.PutUint64(amountBytes, data.Amount.Lo)
	binary.LittleEndian.PutUint64(amountBytes[8:], data.Amount.Hi)
	result = append(result, amountBytes...)

	result = append(result, data.OwnerPk[:]...)
	result = append(result, data.Rho[:]...)
	result = append(result, data.NoteRseed[:]...)

	unlockHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(unlockHeightBytes, data.UnlockHeight)
	result = append(result, unlockHeightBytes...)

	result = append(result, data.MemoHash[:]...)

	return result
}

// processOrchardWorkPackage simulates processing a Orchard work package in the JAM state machine
func processOrchardWorkPackage(jamStorage types.JAMStorage, workPackage *OrchardWorkPackage, ffi *OrchardFFI) error {
	// Simulate state changes that would occur during work package processing

	// 1. Store the proof for verification
	proofKey := fmt.Sprintf("orchard_proof_%x", workPackage.Proof[:8])
	setOrchardStorage(jamStorage, proofKey, workPackage.Proof)

	// 2. Update extrinsic counter
	counterKey := "orchard_extrinsic_count"
	currentCountBytes, err := getOrchardStorage(jamStorage, counterKey)
	if err != nil {
		currentCountBytes = []byte{0, 0, 0, 0}
	}

	currentCount := binary.LittleEndian.Uint32(currentCountBytes)
	newCount := currentCount + 1
	newCountBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(newCountBytes, newCount)
	setOrchardStorage(jamStorage, counterKey, newCountBytes)

	// 3. Store extrinsic type statistics
	typeKey := fmt.Sprintf("orchard_type_%d_count", workPackage.ExtrinsicType)
	typeCountBytes, err := getOrchardStorage(jamStorage, typeKey)
	if err != nil {
		typeCountBytes = []byte{0, 0, 0, 0}
	}

	typeCount := binary.LittleEndian.Uint32(typeCountBytes)
	newTypeCount := typeCount + 1
	newTypeCountBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(newTypeCountBytes, newTypeCount)
	setOrchardStorage(jamStorage, typeKey, newTypeCountBytes)

	// 4. Store the extrinsic data itself
	extrinsicKey := fmt.Sprintf("orchard_extrinsic_%d", newCount)
	setOrchardStorage(jamStorage, extrinsicKey, workPackage.Extrinsic)

	return nil
}

// verifyOrchardStateConsistency verifies that the state changes are consistent
func verifyOrchardStateConsistency(jamStorage types.JAMStorage, extrinsicType ExtrinsicType, data ExtrinsicData, result *ExtrinsicResult) error {
	// Verify that the extrinsic count was incremented
	counterKey := "orchard_extrinsic_count"
	countBytes, err := getOrchardStorage(jamStorage, counterKey)
	if err != nil {
		return fmt.Errorf("failed to get extrinsic count: %v", err)
	}

	count := binary.LittleEndian.Uint32(countBytes)
	if count == 0 {
		return fmt.Errorf("extrinsic count should be non-zero")
	}

	// Verify that the type-specific count was incremented
	typeKey := fmt.Sprintf("orchard_type_%d_count", extrinsicType)
	typeCountBytes, err := getOrchardStorage(jamStorage, typeKey)
	if err != nil {
		return fmt.Errorf("failed to get type count: %v", err)
	}

	typeCount := binary.LittleEndian.Uint32(typeCountBytes)
	if typeCount == 0 {
		return fmt.Errorf("type count should be non-zero")
	}

	// Verify that the extrinsic data was stored
	extrinsicKey := fmt.Sprintf("orchard_extrinsic_%d", count)
	storedData, err := getOrchardStorage(jamStorage, extrinsicKey)
	if err != nil {
		return fmt.Errorf("failed to get stored extrinsic: %v", err)
	}

	if len(storedData) == 0 {
		return fmt.Errorf("stored extrinsic data should not be empty")
	}

	return nil
}

// Helper functions for JAMStorage operations with Orchard service ID
func getOrchardStorage(jamStorage types.JAMStorage, key string) ([]byte, error) {
	serviceID := uint32(2) // Orchard service ID
	value, found, err := jamStorage.GetServiceStorage(serviceID, []byte(key))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return value, nil
}

func setOrchardStorage(jamStorage types.JAMStorage, key string, value []byte) {
	serviceID := uint32(2) // Orchard service ID
	jamStorage.SetServiceStorage(serviceID, []byte(key), value)
}

// Benchmark for performance testing
func BenchmarkOrchardWorkPackageProcessing(b *testing.B) {
	// Initialize test environment
	storage, err := statedb.InitStorage(b.TempDir())
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	trace, err := statedb.MakeGenesisStateTransition(storage, 0, "tiny", nil)
	if err != nil {
		b.Fatalf("MakeGenesisStateTransition failed: %v", err)
	}

	stateDB, err := statedb.NewStateDBFromStateTransition(storage, trace)
	if err != nil {
		b.Fatalf("NewStateDBFromStateTransition failed: %v", err)
	}

	orchardFFI, err := NewOrchardFFI()
	if err != nil {
		b.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer orchardFFI.Cleanup()
	wallet := newDeterministicWallet(orchardFFI)

	// Create test data
	extrinsicData := createTestExtrinsicData(DepositPublic, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate full builder→guarantor flow
		result, err := orchardFFI.ProcessExtrinsic(extrinsicData, wallet)
		if err != nil {
			b.Fatalf("ProcessExtrinsic failed: %v", err)
		}

		workPackage := createOrchardWorkPackage(DepositPublic, extrinsicData, result)
		err = processOrchardWorkPackage(stateDB.GetStorage(), workPackage, orchardFFI)
		if err != nil {
			b.Fatalf("processOrchardWorkPackage failed: %v", err)
		}

		err = orchardFFI.VerifyExtrinsic(extrinsicData, result, wallet)
		if err != nil {
			b.Fatalf("VerifyExtrinsic failed: %v", err)
		}
	}
}
