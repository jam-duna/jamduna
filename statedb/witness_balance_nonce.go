package statedb

import (
	"fmt"

	evmverkle "github.com/colorfulnotion/jam/builder/evm/verkle"
	log "github.com/colorfulnotion/jam/log"
	"github.com/ethereum/go-verkle"
)

// applyBalanceWrites applies balance writes to the Verkle tree
// Updates the balance field in BasicData (offset 16-31, 16 bytes, big-endian)
func applyBalanceWrites(address []byte, balanceBytes []byte, tree verkle.VerkleNode) error {
	if len(balanceBytes) != 16 {
		return fmt.Errorf("invalid balance length: %d, expected 16", len(balanceBytes))
	}

	// Read existing BasicData
	basicDataKey := evmverkle.BasicDataKey(address)
	basicData := make([]byte, 32)

	existing, err := tree.Get(basicDataKey, nil)
	if err == nil && len(existing) >= 32 {
		copy(basicData, existing[:32])
	}

	log.Info(log.SDB, "applyBalanceWrites", "address", fmt.Sprintf("0x%x", address), "balance", fmt.Sprintf("%x", balanceBytes), "existingBasicData", fmt.Sprintf("%x", basicData))

	// Update balance field (offset 16-31, 16 bytes, big-endian)
	copy(basicData[16:32], balanceBytes)

	// Insert updated BasicData
	if err := tree.Insert(basicDataKey, basicData[:], nil); err != nil {
		return fmt.Errorf("failed to insert BasicData: %v", err)
	}

	log.Info(log.SDB, "✅ Balance updated in Verkle", "address", fmt.Sprintf("0x%x", address), "newBasicData", fmt.Sprintf("%x", basicData))
	return nil
}

// applyNonceWrites applies nonce writes to the Verkle tree
// Updates the nonce field in BasicData (offset 8-15, 8 bytes, big-endian)
func applyNonceWrites(address []byte, nonceBytes []byte, tree verkle.VerkleNode) error {
	if len(nonceBytes) != 8 {
		return fmt.Errorf("invalid nonce length: %d, expected 8", len(nonceBytes))
	}

	log.Info(log.SDB, "applyNonceWrites", "address", fmt.Sprintf("0x%x", address), "nonce", fmt.Sprintf("%x", nonceBytes))

	// Read existing BasicData
	basicDataKey := evmverkle.BasicDataKey(address)
	basicData := make([]byte, 32)

	existing, err := tree.Get(basicDataKey, nil)
	if err == nil && len(existing) >= 32 {
		copy(basicData, existing[:32])
	}

	// Update nonce field (offset 8-15, 8 bytes, big-endian)
	copy(basicData[8:16], nonceBytes)

	// Insert updated BasicData
	if err := tree.Insert(basicDataKey, basicData[:], nil); err != nil {
		return fmt.Errorf("failed to insert BasicData: %v", err)
	}

	log.Info(log.SDB, "✅ Nonce updated in Verkle", "address", fmt.Sprintf("0x%x", address))
	return nil
}
