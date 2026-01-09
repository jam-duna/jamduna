package common

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

func TestGetEVMDevAccount(t *testing.T) {
	// Test hardcoded accounts (0-9)
	for i := 0; i < 10; i++ {
		addr, privKeyHex := GetEVMDevAccount(i)
		if addr == (Address{}) {
			t.Errorf("Account %d: got empty address", i)
		}
		if len(privKeyHex) != 64 {
			t.Errorf("Account %d: private key length = %d, want 64", i, len(privKeyHex))
		}

		// Verify private key derives to correct address
		privKey, err := crypto.HexToECDSA(privKeyHex)
		if err != nil {
			t.Errorf("Account %d: failed to parse private key: %v", i, err)
			continue
		}
		derivedAddr := crypto.PubkeyToAddress(privKey.PublicKey)
		if Address(derivedAddr) != addr {
			t.Errorf("Account %d: address mismatch: got %s, derived %s", i, addr.Hex(), derivedAddr.Hex())
		}
	}

	// Test deterministic accounts (>= 10)
	for i := 10; i < 20; i++ {
		addr, privKeyHex := GetEVMDevAccount(i)
		if addr == (Address{}) {
			t.Errorf("Account %d: got empty address", i)
		}
		if len(privKeyHex) != 64 {
			t.Errorf("Account %d: private key length = %d, want 64", i, len(privKeyHex))
		}

		// Verify determinism: same index should generate same account
		addr2, privKeyHex2 := GetEVMDevAccount(i)
		if addr != addr2 {
			t.Errorf("Account %d: not deterministic, got different addresses", i)
		}
		if privKeyHex != privKeyHex2 {
			t.Errorf("Account %d: not deterministic, got different private keys", i)
		}

		// Verify private key derives to correct address
		privKey, err := crypto.HexToECDSA(privKeyHex)
		if err != nil {
			t.Errorf("Account %d: failed to parse private key: %v", i, err)
			continue
		}
		derivedAddr := crypto.PubkeyToAddress(privKey.PublicKey)
		if Address(derivedAddr) != addr {
			t.Errorf("Account %d: address mismatch: got %s, derived %s", i, addr.Hex(), derivedAddr.Hex())
		}

		t.Logf("Account %d: addr=%s privKey=%s", i, addr.Hex(), privKeyHex)
	}
}

func TestDeterministicAccountUniqueness(t *testing.T) {
	// Verify that different indices generate different accounts
	seen := make(map[Address]int)
	for i := 0; i < 100; i++ {
		addr, _ := GetEVMDevAccount(i)
		if prevIdx, exists := seen[addr]; exists {
			t.Errorf("Collision: index %d and %d generated same address %s", prevIdx, i, addr.Hex())
		}
		seen[addr] = i
	}
}
