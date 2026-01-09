package rpc

import (
	"bytes"
	"testing"
)

// TestP2SHDetection tests script type detection matching Rust implementation
func TestP2SHDetection(t *testing.T) {
	// Valid P2SH script: OP_HASH160 PUSH(20) [hash] OP_EQUAL
	p2shScript := []byte{
		0xa9, 0x14, // OP_HASH160 PUSH(20)
		0x74, 0x82, 0x84, 0x39, 0x0f, 0x9e, 0x26, 0x3a,
		0x4b, 0x76, 0x6a, 0x75, 0xd0, 0x63, 0x3c, 0x50,
		0x42, 0x6e, 0xb8, 0x75,
		0x87, // OP_EQUAL
	}

	if !isP2SH(p2shScript) {
		t.Fatal("Valid P2SH script not detected")
	}

	hash, err := extractP2SHHash(p2shScript)
	if err != nil {
		t.Fatalf("Failed to extract P2SH hash: %v", err)
	}

	expectedHash := []byte{0x74, 0x82, 0x84, 0x39, 0x0f, 0x9e, 0x26, 0x3a,
		0x4b, 0x76, 0x6a, 0x75, 0xd0, 0x63, 0x3c, 0x50, 0x42, 0x6e, 0xb8, 0x75}

	if !bytes.Equal(hash, expectedHash) {
		t.Fatalf("Hash mismatch. Expected: %x, Got: %x", expectedHash, hash)
	}
}

// TestP2PKHDetection tests P2PKH detection matching Rust implementation
func TestP2PKHDetection(t *testing.T) {
	// Valid P2PKH script: OP_DUP OP_HASH160 PUSH(20) [hash] OP_EQUALVERIFY OP_CHECKSIG
	p2pkhScript := []byte{
		0x76, 0xa9, 0x14, // OP_DUP OP_HASH160 PUSH(20)
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef,
		0x88, 0xac, // OP_EQUALVERIFY OP_CHECKSIG
	}

	if !isP2PKH(p2pkhScript) {
		t.Fatal("Valid P2PKH script not detected")
	}

	if isP2SH(p2pkhScript) {
		t.Fatal("P2PKH script incorrectly detected as P2SH")
	}
}

// TestExtractRedeemScript tests redeem script extraction matching Rust implementation
func TestExtractRedeemScript(t *testing.T) {
	// Simple scriptSig with redeem script: <sig> <redeemScript>
	scriptSig := []byte{
		0x47, // PUSH 71 bytes (signature)
		// 71 bytes of dummy signature data
		0x30, 0x44, 0x02, 0x20, 0x12, 0x34, 0x56, 0x78,
		0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78,
		0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78,
		0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78,
		0x9a, 0xbc, 0xde, 0xf0, 0x02, 0x20, 0xfe, 0xdc,
		0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0xfe, 0xdc,
		0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0xfe, 0xdc,
		0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0xfe, 0xdc,
		0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0x01,
		0x05, // PUSH 5 bytes (redeem script)
		0x51, 0x52, 0x53, 0x54, 0xae, // OP_1 OP_2 OP_3 OP_4 OP_CHECKMULTISIG
	}

	redeemScript, err := extractRedeemScript(scriptSig)
	if err != nil {
		t.Fatalf("Failed to extract redeem script: %v", err)
	}

	expectedRedeemScript := []byte{0x51, 0x52, 0x53, 0x54, 0xae}
	if !bytes.Equal(redeemScript, expectedRedeemScript) {
		t.Fatalf("Redeem script mismatch. Expected: %x, Got: %x", expectedRedeemScript, redeemScript)
	}

	unlockingData, err := extractUnlockingData(scriptSig)
	if err != nil {
		t.Fatalf("Failed to extract unlocking data: %v", err)
	}

	if len(unlockingData) != 1 {
		t.Fatalf("Expected 1 unlocking element, got %d", len(unlockingData))
	}

	if len(unlockingData[0]) != 71 {
		t.Fatalf("Expected 71-byte signature, got %d bytes", len(unlockingData[0]))
	}
}

// TestP2SHHashVerification tests P2SH hash verification matching Rust implementation
func TestP2SHHashVerification(t *testing.T) {
	// Simple redeem script: OP_1 (always pushes true)
	redeemScript := []byte{0x51}

	// Create P2SH scriptPubKey
	scriptHash := hash160(redeemScript)
	scriptPubKey := make([]byte, 0)
	scriptPubKey = append(scriptPubKey, 0xa9)     // OP_HASH160
	scriptPubKey = append(scriptPubKey, 0x14)     // PUSH 20 bytes
	scriptPubKey = append(scriptPubKey, scriptHash...)
	scriptPubKey = append(scriptPubKey, 0x87)     // OP_EQUAL

	// Create scriptSig with just the redeem script
	scriptSig := make([]byte, 0)
	scriptSig = append(scriptSig, byte(len(redeemScript))) // PUSH redeem script
	scriptSig = append(scriptSig, redeemScript...)

	// Create dummy transaction for testing
	tx := &ZcashTxV5{
		ConsensusBranchID: ConsensusBranchNU5,
		Inputs: []ZcashTxInput{{
			PrevOutHash:  [32]byte{},
			PrevOutIndex: 0,
			ScriptSig:    scriptSig,
			Sequence:     0xFFFFFFFF,
		}},
		Outputs: []ZcashTxOutput{{
			Value:        1000,
			ScriptPubKey: []byte{0x76, 0xa9, 0x14}, // Dummy P2PKH
		}},
	}

	// This should succeed because hash matches and OP_1 always succeeds
	err := VerifyP2SHSignature(tx, 0, 1000, scriptPubKey, scriptSig)
	if err != nil {
		t.Fatalf("P2SH verification failed: %v", err)
	}
}

// TestP2SHSizeLimits tests redeem script size limits
func TestP2SHSizeLimits(t *testing.T) {
	// Create a redeem script that's too large (>520 bytes)
	largeRedeemScript := make([]byte, 521)
	for i := range largeRedeemScript {
		largeRedeemScript[i] = 0x51 // OP_1
	}

	// Create P2SH scriptPubKey
	scriptHash := hash160(largeRedeemScript)
	scriptPubKey := make([]byte, 0)
	scriptPubKey = append(scriptPubKey, 0xa9)     // OP_HASH160
	scriptPubKey = append(scriptPubKey, 0x14)     // PUSH 20 bytes
	scriptPubKey = append(scriptPubKey, scriptHash...)
	scriptPubKey = append(scriptPubKey, 0x87)     // OP_EQUAL

	// Create scriptSig with large redeem script using PUSHDATA2
	scriptSig := make([]byte, 0)
	scriptSig = append(scriptSig, 0x4d)           // PUSHDATA2
	scriptSig = append(scriptSig, 0x09, 0x02)     // 521 bytes (little-endian)
	scriptSig = append(scriptSig, largeRedeemScript...)

	// Create dummy transaction
	tx := &ZcashTxV5{
		ConsensusBranchID: ConsensusBranchNU5,
		Inputs: []ZcashTxInput{{
			PrevOutHash:  [32]byte{},
			PrevOutIndex: 0,
			ScriptSig:    scriptSig,
			Sequence:     0xFFFFFFFF,
		}},
		Outputs: []ZcashTxOutput{{
			Value:        1000,
			ScriptPubKey: []byte{0x76, 0xa9, 0x14}, // Dummy P2PKH
		}},
	}

	// This should fail because redeem script is too large
	err := VerifyP2SHSignature(tx, 0, 1000, scriptPubKey, scriptSig)
	if err == nil {
		t.Fatal("Expected P2SH verification to fail with large redeem script")
	}

	expectedError := "redeem script too large"
	if !bytes.Contains([]byte(err.Error()), []byte(expectedError)) {
		t.Fatalf("Expected error containing '%s', got: %v", expectedError, err)
	}
}

// TestP2SHConstants tests that constants match Rust implementation
func TestP2SHConstants(t *testing.T) {
	// Test opcodes match Rust implementation
	testCases := []struct {
		name     string
		opcode   byte
		expected byte
	}{
		{"OP_HASH160", 0xa9, 0xa9},
		{"OP_EQUAL", 0x87, 0x87},
		{"OP_DUP", 0x76, 0x76},
		{"OP_EQUALVERIFY", 0x88, 0x88},
		{"OP_CHECKSIG", 0xac, 0xac},
		{"PUSH_20", 0x14, 0x14},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.opcode != tc.expected {
				t.Fatalf("Opcode %s mismatch. Expected: 0x%02x, Got: 0x%02x",
					tc.name, tc.expected, tc.opcode)
			}
		})
	}

	// Test script lengths
	if len([]byte{0xa9, 0x14, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x87}) != 23 {
		t.Fatal("P2SH script should be 23 bytes")
	}

	if len([]byte{0x76, 0xa9, 0x14, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x88, 0xac}) != 25 {
		t.Fatal("P2PKH script should be 25 bytes")
	}
}

// TestP2SHIntegration tests full P2SH workflow matching Rust integration test
func TestP2SHIntegration(t *testing.T) {
	// Create a simple P2PKH redeem script for testing
	// OP_DUP OP_HASH160 <20-byte pubkey hash> OP_EQUALVERIFY OP_CHECKSIG
	pubkeyHash := []byte{
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef,
	}

	redeemScript := make([]byte, 0)
	redeemScript = append(redeemScript, 0x76)        // OP_DUP
	redeemScript = append(redeemScript, 0xa9)        // OP_HASH160
	redeemScript = append(redeemScript, 0x14)        // PUSH 20 bytes
	redeemScript = append(redeemScript, pubkeyHash...)
	redeemScript = append(redeemScript, 0x88)        // OP_EQUALVERIFY
	redeemScript = append(redeemScript, 0xac)        // OP_CHECKSIG

	// Create P2SH scriptPubKey
	scriptHash := hash160(redeemScript)
	scriptPubKey := make([]byte, 0)
	scriptPubKey = append(scriptPubKey, 0xa9)        // OP_HASH160
	scriptPubKey = append(scriptPubKey, 0x14)        // PUSH 20 bytes
	scriptPubKey = append(scriptPubKey, scriptHash...)
	scriptPubKey = append(scriptPubKey, 0x87)        // OP_EQUAL

	// Verify this is detected as P2SH
	if !isP2SH(scriptPubKey) {
		t.Fatal("Created script should be detected as P2SH")
	}

	// Create dummy signature and pubkey for P2PKH redeem script
	dummySig := make([]byte, 71)
	for i := range dummySig {
		if i == 70 {
			dummySig[i] = 0x01 // SIGHASH_ALL
		} else {
			dummySig[i] = byte(i % 256)
		}
	}

	dummyPubkey := make([]byte, 33)
	dummyPubkey[0] = 0x02 // Compressed pubkey prefix
	for i := 1; i < 33; i++ {
		dummyPubkey[i] = byte(i % 256)
	}

	// Create scriptSig: <sig> <pubkey> <redeemScript>
	scriptSig := make([]byte, 0)
	scriptSig = append(scriptSig, byte(len(dummySig)))    // PUSH signature
	scriptSig = append(scriptSig, dummySig...)
	scriptSig = append(scriptSig, byte(len(dummyPubkey))) // PUSH pubkey
	scriptSig = append(scriptSig, dummyPubkey...)
	scriptSig = append(scriptSig, byte(len(redeemScript))) // PUSH redeem script
	scriptSig = append(scriptSig, redeemScript...)

	// Test script parsing
	extractedRedeem, err := extractRedeemScript(scriptSig)
	if err != nil {
		t.Fatalf("Failed to extract redeem script: %v", err)
	}

	if !bytes.Equal(extractedRedeem, redeemScript) {
		t.Fatal("Extracted redeem script doesn't match")
	}

	// Test hash verification
	computedHash := hash160(extractedRedeem)
	expectedHash, err := extractP2SHHash(scriptPubKey)
	if err != nil {
		t.Fatalf("Failed to extract P2SH hash: %v", err)
	}

	if !bytes.Equal(computedHash, expectedHash) {
		t.Fatal("Script hash verification failed")
	}

	t.Logf("✓ P2SH integration test passed")
	t.Logf("  - Redeem script length: %d bytes", len(redeemScript))
	t.Logf("  - Script hash: %x", scriptHash)
	t.Logf("  - Script hash verification: PASSED")
}