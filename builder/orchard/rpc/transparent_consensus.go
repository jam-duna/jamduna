package rpc

import (
	"fmt"
	"math"
)

const (
	transparentLockTimeThreshold = 500000000
	transparentFinalSequence     = 0xFFFFFFFF
	transparentMaxInputs         = 1000
	transparentMaxOutputs        = 1000
)

// ValidateTransparentTxs enforces basic consensus rules for transparent txs.
// This is a builder-side gate to avoid including invalid txs in work packages.
func ValidateTransparentTxs(txs []*ZcashTxV5, utxoTree *TransparentUtxoTree, currentHeight uint32, currentTime uint32) error {
	spent := make(map[Outpoint]struct{})
	for _, tx := range txs {
		if _, err := ValidateTransparentTx(tx, utxoTree, currentHeight, currentTime, spent); err != nil {
			return err
		}
	}
	return nil
}

// ValidateTransparentTx validates a single transparent v5 tx against consensus rules.
// Returns the computed fee if successful.
func ValidateTransparentTx(
	tx *ZcashTxV5,
	utxoTree *TransparentUtxoTree,
	currentHeight uint32,
	currentTime uint32,
	spent map[Outpoint]struct{},
) (uint64, error) {
	if tx == nil {
		return 0, fmt.Errorf("transaction is nil")
	}
	if utxoTree == nil {
		return 0, fmt.Errorf("utxo tree unavailable")
	}
	if tx.ConsensusBranchID != ConsensusBranchNU5 {
		return 0, fmt.Errorf("unsupported consensus branch id: 0x%08x", tx.ConsensusBranchID)
	}
	if len(tx.Inputs) == 0 {
		return 0, fmt.Errorf("transaction has no inputs")
	}
	if len(tx.Outputs) == 0 {
		return 0, fmt.Errorf("transaction has no outputs")
	}
	if len(tx.Inputs) > transparentMaxInputs {
		return 0, fmt.Errorf("too many inputs: %d > %d", len(tx.Inputs), transparentMaxInputs)
	}
	if len(tx.Outputs) > transparentMaxOutputs {
		return 0, fmt.Errorf("too many outputs: %d > %d", len(tx.Outputs), transparentMaxOutputs)
	}

	if err := validateLockTime(tx, currentHeight, currentTime); err != nil {
		return 0, err
	}
	if err := validateExpiry(tx, currentHeight); err != nil {
		return 0, err
	}

	var sumInputs uint64
	for i, input := range tx.Inputs {
		if input.IsCoinbase() {
			return 0, fmt.Errorf("coinbase inputs are not allowed in mempool")
		}

		outpoint := Outpoint{
			Txid: input.PrevOutHash,
			Vout: input.PrevOutIndex,
		}
		if _, exists := spent[outpoint]; exists {
			return 0, fmt.Errorf("double-spend detected in transaction")
		}
		spent[outpoint] = struct{}{}

		utxo, ok := utxoTree.Get(outpoint)
		if !ok {
			return 0, fmt.Errorf("utxo not found for input %d", i)
		}
		var err error
		sumInputs, err = addUint64(sumInputs, utxo.Value)
		if err != nil {
			return 0, err
		}

		// Detect script type and verify accordingly
		if isP2PKH(utxo.ScriptPubKey) {
			if err := VerifyP2PKHSignature(tx, i, utxo.Value, utxo.ScriptPubKey, input.ScriptSig); err != nil {
				return 0, fmt.Errorf("P2PKH signature verify failed for input %d: %w", i, err)
			}
		} else if isP2SH(utxo.ScriptPubKey) {
			if err := VerifyP2SHSignature(tx, i, utxo.Value, utxo.ScriptPubKey, input.ScriptSig); err != nil {
				return 0, fmt.Errorf("P2SH signature verify failed for input %d: %w", i, err)
			}
		} else {
			return 0, fmt.Errorf("unsupported script type for input %d", i)
		}
	}

	var sumOutputs uint64
	for i, output := range tx.Outputs {
		var err error
		sumOutputs, err = addUint64(sumOutputs, output.Value)
		if err != nil {
			return 0, fmt.Errorf("output %d value overflow: %w", i, err)
		}
	}

	if sumOutputs > sumInputs {
		return 0, fmt.Errorf("outputs exceed inputs: %d > %d", sumOutputs, sumInputs)
	}

	return sumInputs - sumOutputs, nil
}

func validateLockTime(tx *ZcashTxV5, currentHeight uint32, currentTime uint32) error {
	if tx.LockTime == 0 {
		return nil
	}
	if !txUsesLockTime(tx) {
		return nil
	}

	if tx.LockTime < transparentLockTimeThreshold {
		if currentHeight == 0 {
			return nil
		}
		if currentHeight < tx.LockTime {
			return fmt.Errorf("locktime not yet reached: %d < %d", currentHeight, tx.LockTime)
		}
		return nil
	}

	if currentTime == 0 {
		return nil
	}
	if currentTime < tx.LockTime {
		return fmt.Errorf("locktime not yet reached: %d < %d", currentTime, tx.LockTime)
	}
	return nil
}

func validateExpiry(tx *ZcashTxV5, currentHeight uint32) error {
	if tx.ExpiryHeight == 0 {
		return nil
	}
	if currentHeight == 0 {
		return nil
	}
	if currentHeight > tx.ExpiryHeight {
		return fmt.Errorf("transaction expired: height %d > expiry %d", currentHeight, tx.ExpiryHeight)
	}
	return nil
}

func txUsesLockTime(tx *ZcashTxV5) bool {
	for _, input := range tx.Inputs {
		if input.Sequence != transparentFinalSequence {
			return true
		}
	}
	return false
}

func addUint64(a uint64, b uint64) (uint64, error) {
	if a > math.MaxUint64-b {
		return 0, fmt.Errorf("uint64 overflow")
	}
	return a + b, nil
}
