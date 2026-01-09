// Package transparent provides UTXO snapshot export/import for fast sync
package transparent

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/syndtr/goleveldb/leveldb"
)

// UTXOSnapshot represents a point-in-time UTXO set export
type UTXOSnapshot struct {
	Height   uint32              `json:"height"`
	UTXORoot [32]byte            `json:"utxo_root"`
	UTXOs    []UTXOSnapshotEntry `json:"utxos"`
}

// UTXOSnapshotEntry is a (outpoint, utxo) pair
type UTXOSnapshotEntry struct {
	TxID         [32]byte `json:"txid"`
	Index        uint32   `json:"index"`
	Value        uint64   `json:"value"`
	ScriptPubKey []byte   `json:"script_pubkey"`
	Height       uint32   `json:"height"`
}

// ExportSnapshot creates a snapshot of current UTXO set
func (s *TransparentStore) ExportSnapshot(height uint32) (*UTXOSnapshot, error) {
	root, err := s.GetUTXORoot()
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXO root: %w", err)
	}

	// Get all UTXOs
	allUTXOs, err := s.GetAllUTXOs()
	if err != nil {
		return nil, fmt.Errorf("failed to get all UTXOs: %w", err)
	}

	// Convert to snapshot entries
	entries := make([]UTXOSnapshotEntry, 0, len(allUTXOs))
	for outpoint, utxo := range allUTXOs {
		entries = append(entries, UTXOSnapshotEntry{
			TxID:         outpoint.TxID,
			Index:        outpoint.Index,
			Value:        utxo.Value,
			ScriptPubKey: utxo.ScriptPubKey,
			Height:       utxo.Height,
		})
	}

	return &UTXOSnapshot{
		Height:   height,
		UTXORoot: root,
		UTXOs:    entries,
	}, nil
}

// ImportSnapshot loads a snapshot into the store (verifies root)
func (s *TransparentStore) ImportSnapshot(snapshot *UTXOSnapshot) error {
	// 1. Clear existing UTXO set
	if err := s.clearAllUTXOs(); err != nil {
		return fmt.Errorf("failed to clear UTXOs: %w", err)
	}

	// 2. Import all UTXOs from snapshot
	for _, entry := range snapshot.UTXOs {
		outpoint := OutPoint{
			TxID:  entry.TxID,
			Index: entry.Index,
		}
		utxo := UTXO{
			Value:        entry.Value,
			ScriptPubKey: entry.ScriptPubKey,
			Height:       entry.Height,
		}
		if err := s.AddUTXO(outpoint, utxo); err != nil {
			return fmt.Errorf("failed to add UTXO: %w", err)
		}
	}

	// 3. Verify root matches snapshot
	computedRoot, err := s.GetUTXORoot()
	if err != nil {
		return fmt.Errorf("failed to compute root: %w", err)
	}

	if computedRoot != snapshot.UTXORoot {
		return fmt.Errorf("snapshot root mismatch: expected %x, got %x",
			snapshot.UTXORoot, computedRoot)
	}

	return nil
}

// SaveSnapshotToFile writes snapshot to JSON file
func SaveSnapshotToFile(snapshot *UTXOSnapshot, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(snapshot)
}

// LoadSnapshotFromFile reads snapshot from JSON file
func LoadSnapshotFromFile(path string) (*UTXOSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var snapshot UTXOSnapshot
	if err := json.NewDecoder(f).Decode(&snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// clearAllUTXOs removes all UTXOs from store (used during import)
func (s *TransparentStore) clearAllUTXOs() error {
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	batch := new(leveldb.Batch)
	prefix := []byte("utxo_")

	for iter.Next() {
		key := iter.Key()
		if len(key) >= len(prefix) && string(key[:len(prefix)]) == string(prefix) {
			batch.Delete(key)
		}
	}

	return s.db.Write(batch, nil)
}
