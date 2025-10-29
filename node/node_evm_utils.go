package node

import (
	"encoding/hex"
	"math/big"

	"golang.org/x/crypto/sha3"
)

// ComputeMappingStorageKey computes the storage key for a Solidity mapping
// storage_key = keccak256(abi.encode(key, slot))
// For an address key: keccak256(leftPad32(address) + leftPad32(slot))
func ComputeMappingStorageKey(addressHex string, slotNumber uint64) string {
	// Remove 0x prefix if present
	if len(addressHex) >= 2 && addressHex[:2] == "0x" {
		addressHex = addressHex[2:]
	}

	// Left-pad address to 32 bytes (64 hex chars)
	addressBytes := make([]byte, 32)
	addrData, _ := hex.DecodeString(addressHex)
	copy(addressBytes[32-len(addrData):], addrData)

	// Encode slot number as 32 bytes
	slotBytes := make([]byte, 32)
	slotBig := new(big.Int).SetUint64(slotNumber)
	slotBig.FillBytes(slotBytes)

	// Concatenate: leftPad32(address) + leftPad32(slot)
	data := append(addressBytes, slotBytes...)

	// Compute keccak256
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	result := hash.Sum(nil)

	return "0x" + hex.EncodeToString(result)
}
