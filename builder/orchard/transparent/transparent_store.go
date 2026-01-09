// Package orchard provides persistent transparent UTXO storage for the builder
package transparent

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/syndtr/goleveldb/leveldb"
	"golang.org/x/crypto/blake2b"
)

// TransparentStore manages persistent UTXO set and transaction history
type TransparentStore struct {
	db *leveldb.DB
}

// OutPoint identifies a transaction output
type OutPoint struct {
	TxID  [32]byte
	Index uint32
}

// UTXO represents an unspent transaction output
type UTXO struct {
	Value        uint64
	ScriptPubKey []byte
	Height       uint32
	IsCoinbase   bool
}

// NewTransparentStore opens or creates a persistent store
func NewTransparentStore(path string) (*TransparentStore, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open leveldb: %w", err)
	}
	return &TransparentStore{db: db}, nil
}

// AddUTXO stores a new unspent output
func (s *TransparentStore) AddUTXO(outpoint OutPoint, utxo UTXO) error {
	key := []byte(fmt.Sprintf("utxo_%s_%d", hex.EncodeToString(outpoint.TxID[:]), outpoint.Index))
	value := encodeUTXO(utxo)
	return s.db.Put(key, value, nil)
}

// SpendUTXO marks an output as spent (deletes from UTXO set)
func (s *TransparentStore) SpendUTXO(outpoint OutPoint) error {
	key := []byte(fmt.Sprintf("utxo_%s_%d", hex.EncodeToString(outpoint.TxID[:]), outpoint.Index))
	return s.db.Delete(key, nil)
}

// GetUTXO retrieves a UTXO by outpoint
func (s *TransparentStore) GetUTXO(outpoint OutPoint) (*UTXO, error) {
	key := []byte(fmt.Sprintf("utxo_%s_%d", hex.EncodeToString(outpoint.TxID[:]), outpoint.Index))
	value, err := s.db.Get(key, nil)
	if err != nil {
		return nil, err
	}
	return decodeUTXO(value)
}

// GetUTXORoot computes Merkle root of all UTXOs (sorted by outpoint)
func (s *TransparentStore) GetUTXORoot() ([32]byte, error) {
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	var utxos [][32]byte
	prefix := []byte("utxo_")

	for iter.Next() {
		key := iter.Key()
		if len(key) < len(prefix) || string(key[:len(prefix)]) != string(prefix) {
			continue
		}
		value := iter.Value()

		// Hash (key || value) as leaf
		h := blake2b.Sum256(append(key, value...))
		utxos = append(utxos, h)
	}

	if err := iter.Error(); err != nil {
		return [32]byte{}, err
	}

	// Sort leaves for deterministic root
	sort.Slice(utxos, func(i, j int) bool {
		for k := 0; k < 32; k++ {
			if utxos[i][k] != utxos[j][k] {
				return utxos[i][k] < utxos[j][k]
			}
		}
		return false
	})

	return computeMerkleRoot(utxos), nil
}

// GetUTXOSize returns number of UTXOs in set
func (s *TransparentStore) GetUTXOSize() (uint64, error) {
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	count := uint64(0)
	prefix := []byte("utxo_")

	for iter.Next() {
		key := iter.Key()
		if len(key) >= len(prefix) && string(key[:len(prefix)]) == string(prefix) {
			count++
		}
	}

	return count, iter.Error()
}

// GetAllUTXOs returns all UTXOs in the set (for migration)
func (s *TransparentStore) GetAllUTXOs() (map[OutPoint]UTXO, error) {
	iter := s.db.NewIterator(nil, nil)
	defer iter.Release()

	utxos := make(map[OutPoint]UTXO)
	prefix := []byte("utxo_")

	for iter.Next() {
		key := iter.Key()
		if len(key) < len(prefix) || string(key[:len(prefix)]) != string(prefix) {
			continue
		}

		// Parse outpoint from key: "utxo_<txid_hex>_<index>"
		outpoint, err := parseOutPointFromKey(string(key))
		if err != nil {
			continue // Skip malformed keys
		}

		utxo, err := decodeUTXO(iter.Value())
		if err != nil {
			continue // Skip malformed values
		}

		utxos[outpoint] = *utxo
	}

	return utxos, iter.Error()
}

// AddTransaction stores transaction at given height
func (s *TransparentStore) AddTransaction(height uint32, tx []byte, txid [32]byte) error {
	key := []byte(fmt.Sprintf("tx_%d_%s", height, hex.EncodeToString(txid[:])))
	return s.db.Put(key, tx, nil)
}

// GetTransaction retrieves transaction by height and txid
func (s *TransparentStore) GetTransaction(height uint32, txid [32]byte) ([]byte, error) {
	key := []byte(fmt.Sprintf("tx_%d_%s", height, hex.EncodeToString(txid[:])))
	return s.db.Get(key, nil)
}

// Close closes the database
func (s *TransparentStore) Close() error {
	return s.db.Close()
}

// encodeUTXO serializes UTXO to bytes
// Format: [value:8][height:4][scriptpubkey_len:4][scriptpubkey:N][coinbase:1]
func encodeUTXO(utxo UTXO) []byte {
	buf := make([]byte, 8+4+4+len(utxo.ScriptPubKey)+1)
	binary.LittleEndian.PutUint64(buf[0:8], utxo.Value)
	binary.LittleEndian.PutUint32(buf[8:12], utxo.Height)
	binary.LittleEndian.PutUint32(buf[12:16], uint32(len(utxo.ScriptPubKey)))
	copy(buf[16:], utxo.ScriptPubKey)
	if utxo.IsCoinbase {
		buf[16+len(utxo.ScriptPubKey)] = 1
	}
	return buf
}

// decodeUTXO deserializes UTXO from bytes
func decodeUTXO(data []byte) (*UTXO, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("invalid UTXO data: too short")
	}

	value := binary.LittleEndian.Uint64(data[0:8])
	height := binary.LittleEndian.Uint32(data[8:12])
	scriptLen := binary.LittleEndian.Uint32(data[12:16])

	if len(data) < 16+int(scriptLen) {
		return nil, fmt.Errorf("invalid UTXO data: truncated script")
	}

	scriptPubKey := make([]byte, scriptLen)
	copy(scriptPubKey, data[16:16+scriptLen])

	isCoinbase := false
	if len(data) >= 16+int(scriptLen)+1 {
		isCoinbase = data[16+scriptLen] != 0
	}

	return &UTXO{
		Value:        value,
		ScriptPubKey: scriptPubKey,
		Height:       height,
		IsCoinbase:   isCoinbase,
	}, nil
}

// parseOutPointFromKey parses OutPoint from key string "utxo_<txid_hex>_<index>"
func parseOutPointFromKey(key string) (OutPoint, error) {
	var op OutPoint

	// Expected format: "utxo_<64_hex_chars>_<index>"
	if len(key) < 5+64+1 {
		return op, fmt.Errorf("invalid key format")
	}

	txidHex := key[5 : 5+64] // Skip "utxo_" prefix
	txidBytes, err := hex.DecodeString(txidHex)
	if err != nil {
		return op, err
	}
	if len(txidBytes) != 32 {
		return op, fmt.Errorf("invalid txid length")
	}

	copy(op.TxID[:], txidBytes)

	// Parse index from remaining part
	var index uint32
	_, err = fmt.Sscanf(key[5+64:], "_%d", &index)
	if err != nil {
		return op, err
	}

	op.Index = index
	return op, nil
}

// computeMerkleRoot builds Merkle tree from sorted leaves
func computeMerkleRoot(leaves [][32]byte) [32]byte {
	if len(leaves) == 0 {
		return [32]byte{} // Empty tree
	}
	if len(leaves) == 1 {
		return leaves[0]
	}

	// Build Merkle tree bottom-up
	for len(leaves) > 1 {
		var nextLevel [][32]byte
		for i := 0; i < len(leaves); i += 2 {
			if i+1 < len(leaves) {
				h := blake2b.Sum256(append(leaves[i][:], leaves[i+1][:]...))
				nextLevel = append(nextLevel, h)
			} else {
				nextLevel = append(nextLevel, leaves[i]) // Odd leaf promoted
			}
		}
		leaves = nextLevel
	}
	return leaves[0]
}
