// Package transparent provides chain scanning to rebuild UTXO state
package transparent

import (
	"encoding/hex"
	"fmt"

	"github.com/colorfulnotion/jam/log"
	"golang.org/x/crypto/blake2b"
)

// Scanner rebuilds transparent state by scanning JAM blocks
type Scanner struct {
	store *TransparentStore
}

// NewScanner creates a new chain scanner
func NewScanner(store *TransparentStore) *Scanner {
	return &Scanner{
		store: store,
	}
}

// ScanBlock processes a single block and updates UTXO set
// Returns number of transparent transactions processed
func (s *Scanner) ScanBlock(height uint32, blockData []byte) (int, error) {
	txCount := 0

	// Parse block to extract transparent transactions
	// NOTE: This is a placeholder - real implementation would parse actual JAM block format
	// and extract transparent extrinsics (tag=4 in Orchard work packages)

	// For now, this provides the infrastructure for future integration
	log.Debug("Scanning block for transparent transactions", "height", height, "blockSize", len(blockData))

	return txCount, nil
}

// ScanBlockRange scans a range of blocks [startHeight, endHeight]
// Checkpoints progress every checkpointInterval blocks
func (s *Scanner) ScanBlockRange(
	startHeight uint32,
	endHeight uint32,
	checkpointInterval uint32,
	getBlock func(uint32) ([]byte, error),
) error {
	log.Info("Starting transparent chain scan",
		"startHeight", startHeight,
		"endHeight", endHeight,
		"checkpointInterval", checkpointInterval)

	totalTxs := 0

	for h := startHeight; h <= endHeight; h++ {
		// Get block data
		blockData, err := getBlock(h)
		if err != nil {
			return fmt.Errorf("failed to get block %d: %w", h, err)
		}

		// Process block
		txCount, err := s.ScanBlock(h, blockData)
		if err != nil {
			return fmt.Errorf("failed to scan block %d: %w", h, err)
		}

		totalTxs += txCount

		// Checkpoint progress
		if checkpointInterval > 0 && h%checkpointInterval == 0 {
			if err := s.logCheckpoint(h); err != nil {
				log.Warn("Failed to log checkpoint", "height", h, "err", err)
			}
		}
	}

	log.Info("Chain scan complete",
		"startHeight", startHeight,
		"endHeight", endHeight,
		"totalTransactions", totalTxs)

	return nil
}

// logCheckpoint logs current UTXO set state at checkpoint
func (s *Scanner) logCheckpoint(height uint32) error {
	root, err := s.store.GetUTXORoot()
	if err != nil {
		return err
	}

	size, err := s.store.GetUTXOSize()
	if err != nil {
		return err
	}

	log.Info("Chain scan checkpoint",
		"height", height,
		"utxoRoot", hex.EncodeToString(root[:8]),
		"utxoCount", size)

	return nil
}

// ProcessTransparentTransaction applies a transparent transaction to UTXO set
// This is a helper for when transparent transaction format is defined
func (s *Scanner) ProcessTransparentTransaction(
	height uint32,
	txBytes []byte,
) error {
	// Compute txid
	txid := blake2b.Sum256(txBytes)

	// Parse transaction (placeholder - needs real transparent tx format)
	// For now, we just store the transaction
	if err := s.store.AddTransaction(height, txBytes, txid); err != nil {
		return fmt.Errorf("failed to store transaction: %w", err)
	}

	// TODO: When transparent transaction format is defined:
	// 1. Parse inputs and spend corresponding UTXOs
	// 2. Parse outputs and create new UTXOs
	// 3. Verify transaction validity

	return nil
}

// RebuildFromGenesis rebuilds entire UTXO set from genesis
// This clears existing UTXO set and rescans all blocks
func (s *Scanner) RebuildFromGenesis(
	endHeight uint32,
	getBlock func(uint32) ([]byte, error),
) error {
	log.Info("Rebuilding UTXO set from genesis", "endHeight", endHeight)

	// Clear existing UTXO set
	if err := s.store.clearAllUTXOs(); err != nil {
		return fmt.Errorf("failed to clear UTXOs: %w", err)
	}

	// Scan from genesis (height 0)
	return s.ScanBlockRange(0, endHeight, 1000, getBlock)
}

// VerifyUTXORoot verifies UTXO root matches expected value
// Used to validate scan results against JAM consensus state
func (s *Scanner) VerifyUTXORoot(expectedRoot [32]byte) error {
	actualRoot, err := s.store.GetUTXORoot()
	if err != nil {
		return err
	}

	if actualRoot != expectedRoot {
		return fmt.Errorf("UTXO root mismatch: expected %x, got %x",
			expectedRoot, actualRoot)
	}

	log.Info("UTXO root verification passed", "root", hex.EncodeToString(actualRoot[:]))
	return nil
}
