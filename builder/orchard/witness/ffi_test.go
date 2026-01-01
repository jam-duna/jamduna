package witness

import (
	"bytes"
	"testing"
)

func TestOrchardFFI_Init(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()
	_ = newDeterministicWallet(ffi)

	// Test that we can get the service ID
	serviceID := ffi.GetServiceID()
	if serviceID == 0 {
		t.Error("Expected non-zero service ID")
	}
	t.Logf("Orchard Service ID: %d", serviceID)
}

func TestOrchardFFI_GenerateNullifier(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()

	// Test data
	skSpend := [32]byte{1}
	rho := [32]byte{2}
	commitment := [32]byte{3}

	// Generate nullifier
	nullifier1, err := ffi.GenerateNullifier(skSpend, rho, commitment)
	if err != nil {
		t.Fatalf("Failed to generate nullifier: %v", err)
	}

	// Generate same nullifier again - should be deterministic
	nullifier2, err := ffi.GenerateNullifier(skSpend, rho, commitment)
	if err != nil {
		t.Fatalf("Failed to generate second nullifier: %v", err)
	}

	if nullifier1 != nullifier2 {
		t.Error("Nullifier generation is not deterministic")
	}

	// Check that nullifier is not all zeros
	zeroNullifier := [32]byte{}
	if nullifier1 == zeroNullifier {
		t.Error("Nullifier should not be all zeros")
	}

	t.Logf("Generated nullifier: %x", nullifier1)
}

func TestOrchardFFI_GenerateCommitment(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()

	// Test data
	assetID := uint32(1)
	amount := Uint128{Lo: 1000}
	ownerPk := [32]byte{1}
	rho := [32]byte{2}
	noteRseed := [32]byte{3}
	unlockHeight := uint64(0)
	memoHash := [32]byte{4}

	// Generate commitment
	commitment1, err := ffi.GenerateCommitment(assetID, amount, ownerPk, rho, noteRseed, unlockHeight, memoHash)
	if err != nil {
		t.Fatalf("Failed to generate commitment: %v", err)
	}

	// Generate same commitment again - should be deterministic
	commitment2, err := ffi.GenerateCommitment(assetID, amount, ownerPk, rho, noteRseed, unlockHeight, memoHash)
	if err != nil {
		t.Fatalf("Failed to generate second commitment: %v", err)
	}

	if commitment1 != commitment2 {
		t.Error("Commitment generation is not deterministic")
	}

	// Check that commitment is not all zeros
	zeroCommitment := [32]byte{}
	if commitment1 == zeroCommitment {
		t.Error("Commitment should not be all zeros")
	}

	t.Logf("Generated commitment: %x", commitment1)
}

func TestOrchardFFI_LargeAmountCommitment(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()

	// Test data with large 128-bit amount (using both Hi and Lo limbs)
	assetID := uint32(1)
	largeAmount := Uint128{
		Lo: 0xFFFFFFFFFFFFFFFF, // Max uint64
		Hi: 0x123456789ABCDEF0, // High limb with data
	}
	ownerPk := [32]byte{1}
	rho := [32]byte{2}
	noteRseed := [32]byte{3}
	unlockHeight := uint64(0)
	memoHash := [32]byte{4}

	// Generate commitment with large amount
	commitment1, err := ffi.GenerateCommitment(assetID, largeAmount, ownerPk, rho, noteRseed, unlockHeight, memoHash)
	if err != nil {
		t.Fatalf("Failed to generate commitment with large amount: %v", err)
	}

	// Generate same commitment again - should be deterministic
	commitment2, err := ffi.GenerateCommitment(assetID, largeAmount, ownerPk, rho, noteRseed, unlockHeight, memoHash)
	if err != nil {
		t.Fatalf("Failed to generate second commitment: %v", err)
	}

	if commitment1 != commitment2 {
		t.Error("Large amount commitment generation is not deterministic")
	}

	// Generate commitment with different amount (only Lo limb different)
	smallerAmount := Uint128{
		Lo: 0xFFFFFFFFFFFFFFFE, // One less than max uint64
		Hi: 0x123456789ABCDEF0, // Same high limb
	}
	commitment3, err := ffi.GenerateCommitment(assetID, smallerAmount, ownerPk, rho, noteRseed, unlockHeight, memoHash)
	if err != nil {
		t.Fatalf("Failed to generate commitment with smaller amount: %v", err)
	}

	// Should produce different commitment
	if commitment1 == commitment3 {
		t.Error("Different amounts produced same commitment")
	}

	// Generate commitment with different Hi limb
	differentHiAmount := Uint128{
		Lo: 0xFFFFFFFFFFFFFFFF, // Same Lo limb
		Hi: 0x123456789ABCDEF1, // Different Hi limb
	}
	commitment4, err := ffi.GenerateCommitment(assetID, differentHiAmount, ownerPk, rho, noteRseed, unlockHeight, memoHash)
	if err != nil {
		t.Fatalf("Failed to generate commitment with different Hi limb: %v", err)
	}

	// Should produce different commitment
	if commitment1 == commitment4 {
		t.Error("Different Hi limbs produced same commitment")
	}

	t.Logf("Large amount commitment: %x", commitment1)
	t.Logf("Smaller amount commitment: %x", commitment3)
	t.Logf("Different Hi limb commitment: %x", commitment4)
}

func TestOrchardFFI_PoseidonHash(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()

	domain := "ffi_poseidon_v1"

	// Test single input
	input1 := [32]byte{1}
	hash1, err := ffi.PoseidonHash(domain, [][32]byte{input1})
	if err != nil {
		t.Fatalf("Failed to hash single input: %v", err)
	}

	// Test two inputs
	input2 := [32]byte{2}
	hash2, err := ffi.PoseidonHash(domain, [][32]byte{input1, input2})
	if err != nil {
		t.Fatalf("Failed to hash two inputs: %v", err)
	}

	// Test three inputs (exercise padded branch)
	input3 := [32]byte{3}
	hash3, err := ffi.PoseidonHash(domain, [][32]byte{input1, input2, input3})
	if err != nil {
		t.Fatalf("Failed to hash three inputs: %v", err)
	}

	// Hashes should be different
	if hash1 == hash2 || hash2 == hash3 {
		t.Error("Different inputs produced same hash")
	}

	// Test that hashing is deterministic
	hash1_repeat, err := ffi.PoseidonHash(domain, [][32]byte{input1})
	if err != nil {
		t.Fatalf("Failed to repeat hash: %v", err)
	}

	if hash1 != hash1_repeat {
		t.Error("Poseidon hash is not deterministic")
	}

	t.Logf("Single input hash: %x", hash1)
	t.Logf("Two input hash: %x", hash2)
}

func TestOrchardFFI_MockProofGeneration(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()

	inputData := []byte("test input data")

	// Test proof generation for each extrinsic type
	extrinsicTypes := []ExtrinsicType{
		DepositPublic,
		SubmitPrivate,
		WithdrawPublic,
		IssuanceV1,
		BatchAggV1,
	}

	for _, extrinsicType := range extrinsicTypes {
		t.Run(string(rune(extrinsicType)), func(t *testing.T) {
			proof, err := ffi.GenerateProof(extrinsicType, inputData)
			if err != nil {
				t.Fatalf("Failed to generate proof for type %d: %v", extrinsicType, err)
			}

			// FIXED: Updated for real Halo2 proof size.
			if len(proof) != OrchardProofSize {
				t.Errorf("Expected proof length %d, got %d", OrchardProofSize, len(proof))
			}

			// Verify the mock proof with the same input data used for generation
			err = ffi.VerifyProof(extrinsicType, proof, inputData)
			if err != nil {
				t.Fatalf("Failed to verify proof for type %d: %v", extrinsicType, err)
			}

			t.Logf("Generated and verified proof for extrinsic type %d", extrinsicType)
		})
	}
}

func TestOrchardFFI_ProcessExtrinsic(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()

	wallet := newDeterministicWallet(ffi)

	// Test data for a deposit
	extrinsicData := ExtrinsicData{
		Type:         DepositPublic,
		AssetID:      1,
		Amount:       Uint128{Lo: 1000, Hi: 0},
		OwnerPk:      [32]byte{0x01, 0x02, 0x03}, // Sample public key
		Rho:          [32]byte{0x04, 0x05, 0x06}, // Sample rho
		NoteRseed:    [32]byte{0x07, 0x08, 0x09}, // Sample note randomness
		UnlockHeight: 0,
		MemoHash:     [32]byte{0x0a, 0x0b, 0x0c}, // Sample memo hash
	}

	// Process the extrinsic (builder flow)
	result, err := ffi.ProcessExtrinsic(extrinsicData, wallet)
	if err != nil {
		t.Fatalf("Failed to process extrinsic: %v", err)
	}

	// Verify that we got valid outputs
	zeroBytes := [32]byte{}
	if result.Commitment == zeroBytes {
		t.Error("Commitment should not be all zeros")
	}
	if result.Nullifier == zeroBytes {
		t.Error("Nullifier should not be all zeros")
	}
	if len(result.Proof) == 0 {
		t.Error("Proof should not be empty")
	}

	t.Logf("Processed extrinsic:")
	t.Logf("  Commitment: %x", result.Commitment)
	t.Logf("  Nullifier: %x", result.Nullifier)
	t.Logf("  Proof length: %d", len(result.Proof))

	// Verify the extrinsic (guarantor flow)
	err = ffi.VerifyExtrinsic(extrinsicData, result, wallet)
	if err != nil {
		t.Fatalf("Failed to verify extrinsic: %v", err)
	}

	t.Log("Successfully verified extrinsic")
}

func TestOrchardFFI_AllExtrinsicTypes(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()
	wallet := newDeterministicWallet(ffi)

	extrinsicTypes := []ExtrinsicType{
		DepositPublic,
		SubmitPrivate,
		WithdrawPublic,
		IssuanceV1,
		BatchAggV1,
	}

	typeNames := []string{
		"DepositPublic",
		"SubmitPrivate",
		"WithdrawPublic",
		"IssuanceV1",
		"BatchAggV1",
	}

	for i, extrinsicType := range extrinsicTypes {
		t.Run(typeNames[i], func(t *testing.T) {
			// Create test data for this extrinsic type
			extrinsicData := ExtrinsicData{
				Type:         extrinsicType,
				AssetID:      uint32(i + 1), // Different asset for each type
				Amount:       Uint128{Lo: uint64((i + 1) * 1000)},
				OwnerPk:      [32]byte{byte(i + 1)},
				Rho:          [32]byte{byte(i + 2)},
				NoteRseed:    [32]byte{byte(i + 3)},
				UnlockHeight: uint64(i),
				MemoHash:     [32]byte{byte(i + 4)},
			}

			// Process and verify the extrinsic
			result, err := ffi.ProcessExtrinsic(extrinsicData, wallet)
			if err != nil {
				t.Fatalf("Failed to process %s: %v", typeNames[i], err)
			}

			err = ffi.VerifyExtrinsic(extrinsicData, result, wallet)
			if err != nil {
				t.Fatalf("Failed to verify %s: %v", typeNames[i], err)
			}

			t.Logf("Successfully processed and verified %s", typeNames[i])
		})
	}
}

func TestOrchardFFI_ErrorHandling(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()
	domain := "ffi_poseidon_v1"

	// Test invalid proof verification
	invalidProof := []byte{0x00, 0x01, 0x02} // Too short
	publicInputs := []byte("test")

	err = ffi.VerifyProof(DepositPublic, invalidProof, publicInputs)
	if err == nil {
		t.Error("Expected verification to fail with invalid proof")
	}

	// Test invalid input for Poseidon hash
	_, err = ffi.PoseidonHash(domain, [][32]byte{}) // Empty input
	if err == nil {
		t.Error("Expected Poseidon hash to fail with empty input")
	}

	// Test with maximum supported inputs (should succeed)
	maxInputs := make([][32]byte, 10)
	for i := range maxInputs {
		maxInputs[i][0] = byte(i + 1) // Different inputs
	}
	_, err = ffi.PoseidonHash(domain, maxInputs)
	if err != nil {
		t.Errorf("Poseidon hash should succeed with 10 inputs: %v", err)
	}

	// Test too many inputs for Poseidon hash (beyond 10)
	tooManyInputs := make([][32]byte, 11)
	_, err = ffi.PoseidonHash(domain, tooManyInputs)
	if err == nil {
		t.Error("Expected Poseidon hash to fail with too many inputs (>10)")
	}

	t.Log("Error handling tests passed")
}

// Benchmark tests
func BenchmarkOrchardFFI_GenerateCommitment(b *testing.B) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		b.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()
	_ = newDeterministicWallet(ffi)

	// Test data
	assetID := uint32(1)
	amount := Uint128{Lo: 1000}
	ownerPk := [32]byte{1}
	rho := [32]byte{2}
	noteRseed := [32]byte{3}
	unlockHeight := uint64(0)
	memoHash := [32]byte{4}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ffi.GenerateCommitment(assetID, amount, ownerPk, rho, noteRseed, unlockHeight, memoHash)
		if err != nil {
			b.Fatalf("Failed to generate commitment: %v", err)
		}
	}
}

func BenchmarkOrchardFFI_GenerateNullifier(b *testing.B) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		b.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()

	skSpend := [32]byte{1}
	rho := [32]byte{2}
	commitment := [32]byte{3}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ffi.GenerateNullifier(skSpend, rho, commitment)
		if err != nil {
			b.Fatalf("Failed to generate nullifier: %v", err)
		}
	}
}

func BenchmarkOrchardFFI_ProcessExtrinsic(b *testing.B) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		b.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()

	wallet := newDeterministicWallet(ffi)

	extrinsicData := ExtrinsicData{
		Type:         DepositPublic,
		AssetID:      1,
		Amount:       Uint128{Lo: 1000},
		OwnerPk:      [32]byte{1},
		Rho:          [32]byte{2},
		NoteRseed:    [32]byte{3},
		UnlockHeight: 0,
		MemoHash:     [32]byte{4},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := ffi.ProcessExtrinsic(extrinsicData, wallet)
		if err != nil {
			b.Fatalf("Failed to process extrinsic: %v", err)
		}
		_ = result // Avoid unused variable warning
	}
}

// TestOrchardFFI_FullBuilderGuarantorFlow tests the complete builder→guarantor flow
// This is the integration test that demonstrates real end-to-end Orchard operation
func TestOrchardFFI_FullBuilderGuarantorFlow(t *testing.T) {
	ffi, err := NewOrchardFFI()
	if err != nil {
		t.Fatalf("Failed to initialize Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()
	wallet := newDeterministicWallet(ffi)

	serviceID := ffi.GetServiceID()
	t.Logf("Orchard Service ID: %d", serviceID)

	// Test all Orchard extrinsic types
	extrinsicTypes := []struct {
		name          string
		extrinsicType ExtrinsicType
		description   string
	}{
		{"DepositPublic", DepositPublic, "Deposit public funds into shielded pool"},
		{"SubmitPrivate", SubmitPrivate, "Submit private transaction with ZK proof"},
		{"WithdrawPublic", WithdrawPublic, "Withdraw from shielded pool to public account"},
		{"IssuanceV1", IssuanceV1, "Issue new tokens with compliance constraints"},
		{"BatchAggV1", BatchAggV1, "Aggregate multiple transactions into single proof"},
	}

	for i, testCase := range extrinsicTypes {
		t.Run(testCase.name, func(t *testing.T) {
			t.Logf("Testing %s: %s", testCase.name, testCase.description)

			// Create test extrinsic data
			seed := uint32(i + 1)
			extrinsicData := ExtrinsicData{
				Type:         testCase.extrinsicType,
				AssetID:      seed,
				Amount:       Uint128{Lo: uint64(seed * 1000), Hi: uint64(seed / 10)},
				OwnerPk:      createTestBytes32(byte(seed)),
				Rho:          createTestBytes32(byte(seed + 1)),
				NoteRseed:    createTestBytes32(byte(seed + 2)),
				UnlockHeight: uint64(seed),
				MemoHash:     createTestBytes32(byte(seed + 3)),
			}

			// BUILDER PHASE: Generate commitment, nullifier, and proof
			t.Log("=== BUILDER PHASE ===")
			result, err := ffi.ProcessExtrinsic(extrinsicData, wallet)
			if err != nil {
				t.Fatalf("Builder failed to process extrinsic: %v", err)
			}

			t.Logf("Builder generated:")
			t.Logf("  Commitment: %x", result.Commitment)
			t.Logf("  Nullifier: %x", result.Nullifier)
			t.Logf("  Proof length: %d bytes", len(result.Proof))
			t.Logf("  Public inputs length: %d bytes", len(result.PublicInputs))

			// GUARANTOR PHASE: Verify the proof and state transitions
			t.Log("=== GUARANTOR PHASE ===")
			err = ffi.VerifyExtrinsic(extrinsicData, result, wallet)
			if err != nil {
				t.Fatalf("Guarantor verification failed: %v", err)
			}

			// Verify that different extrinsic types produce different results
			if i > 0 {
				// Generate the same data with a different extrinsic type
				differentTypeData := extrinsicData
				differentTypeData.Type = extrinsicTypes[0].extrinsicType // Use first type
				if differentTypeData.Type != testCase.extrinsicType {
					differentResult, err := ffi.ProcessExtrinsic(differentTypeData, wallet)
					if err != nil {
						t.Fatalf("Failed to process different type: %v", err)
					}
					// Proofs should be different for different extrinsic types
					if bytes.Equal(result.Proof, differentResult.Proof) {
						t.Error("Different extrinsic types produced identical proofs")
					}
				}
			}

			t.Logf("✅ %s completed successfully", testCase.name)
			t.Logf("   Builder→Guarantor flow validated")
			t.Logf("   Proof verified: %d bytes", len(result.Proof))
		})
	}

	t.Log("🎉 All Orchard extrinsic tests passed!")
	t.Log("Builder→Guarantor flow validated for all 5 extrinsic types")
}

// createTestBytes32 creates a deterministic 32-byte array for testing
func createTestBytes32(base byte) [32]byte {
	var result [32]byte
	for i := 0; i < 32; i++ {
		result[i] = base + byte(i)
	}
	return result
}
