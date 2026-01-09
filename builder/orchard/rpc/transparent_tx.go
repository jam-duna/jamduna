package rpc

// ⚠️ WARNING: THIS IMPLEMENTATION HAS CRITICAL ISSUES - NOT PRODUCTION READY
//
// Known issues that prevent web wallet integration:
// 1. TxID derivation uses Blake2 instead of Zcash's SHA256d (line 192)
//    - Web wallets compute txid using SHA256d
//    - Server computes txid using Blake2
//    - getrawtransaction lookups fail due to mismatch
//
// 2. No historical transaction retrieval (line 131)
//    - Only finds txs submitted via sendrawtransaction to THIS server
//    - Cannot fetch txs from blockchain
//    - Web wallet scanners broken
//
// 3. No transaction parsing (line 196, 215)
//    - parseRawTransaction returns empty Vin/Vout
//    - UTXO tracking non-functional
//    - Double-spend prevention broken
//
// 4. Race condition in GetMempool (line 151)
//    - Verbose mode returns map pointer while lock released
//    - Concurrent writes can panic during JSON encoding
//
// 5. No validation in SendRawTransaction (handler.go:1434)
//    - No size limits, script validation, signature checks
//    - No broadcast to network
//    - Accepts invalid transactions
//
// See builder/orchard/docs/TRANSPARENT_TX_ISSUES.md for complete details and fixes.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	orchardtransparent "github.com/colorfulnotion/jam/builder/orchard/transparent"
	log "github.com/colorfulnotion/jam/log"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// TransparentTransaction represents a raw Zcash transparent transaction
type TransparentTransaction struct {
	TxID          string     `json:"txid"`          // Transaction ID (hash)
	RawHex        string     `json:"hex"`           // Raw transaction in hex format
	Size          int        `json:"size"`          // Transaction size in bytes
	Version       int        `json:"version"`       // Transaction version
	LockTime      uint32     `json:"locktime"`      // Lock time
	Vin           []TxInput  `json:"vin"`           // Transaction inputs
	Vout          []TxOutput `json:"vout"`          // Transaction outputs
	BlockHeight   uint32     `json:"blockheight"`   // Block height (0 if in mempool)
	BlockHash     string     `json:"blockhash"`     // Block hash (empty if in mempool)
	Timestamp     time.Time  `json:"timestamp"`     // When added to mempool/block
	Confirmations int        `json:"confirmations"` // Number of confirmations
}

// TxInput represents a transaction input
type TxInput struct {
	TxID      string `json:"txid"`               // Previous transaction ID
	Vout      uint32 `json:"vout"`               // Output index
	ScriptSig string `json:"scriptSig"`          // Script signature
	Sequence  uint32 `json:"sequence"`           // Sequence number
	Coinbase  string `json:"coinbase,omitempty"` // Coinbase data (if coinbase tx)
}

// TxOutput represents a transaction output
type TxOutput struct {
	Value        float64      `json:"value"`        // Output value in ZEC
	N            uint32       `json:"n"`            // Output index
	ScriptPubKey ScriptPubKey `json:"scriptPubKey"` // Script public key
}

// ScriptPubKey represents the output script
type ScriptPubKey struct {
	Asm       string   `json:"asm"`                 // Script assembly
	Hex       string   `json:"hex"`                 // Script hex
	ReqSigs   int      `json:"reqSigs,omitempty"`   // Required signatures
	Type      string   `json:"type"`                // Script type (pubkeyhash, scripthash, etc.)
	Addresses []string `json:"addresses,omitempty"` // Addresses
}

// TransparentTxStore manages transparent transaction storage and mempool
type TransparentTxStore struct {
	mu sync.RWMutex

	// Transaction storage
	transactions map[string]*TransparentTransaction // txid -> transaction
	mempool      map[string]*TransparentTransaction // txid -> pending transactions

	// UTXO tracking for transparent addresses
	utxos map[string][]UTXO // address -> unspent outputs

	// Block storage (simplified)
	blockTxs map[uint32][]string // block height -> txids
	txIndex  map[string]uint32   // txid -> block height (in-memory cache)

	// Persistent storage
	indexDB   *leveldb.DB // Persistent txid -> block height index
	mempoolDB *leveldb.DB // Persistent mempool txs (txid -> json)
	utxoStore *orchardtransparent.TransparentStore
	utxoTree  *TransparentUtxoTree

	// Statistics
	totalTransactions uint64
	mempoolSize       uint64
}

// UTXO represents an unspent transparent output
type UTXO struct {
	TxID          string  `json:"txid"`
	Vout          uint32  `json:"vout"`
	Address       string  `json:"address"`
	ScriptPubKey  string  `json:"scriptPubKey"`
	Amount        float64 `json:"amount"`
	Confirmations int     `json:"confirmations"`
}

// NewTransparentTxStore creates a new transparent transaction store
func NewTransparentTxStore(dataDir string) (*TransparentTxStore, error) {
	// Create data directory if it doesn't exist
	indexPath := filepath.Join(dataDir, "transparent_index")
	if err := os.MkdirAll(indexPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create index directory: %w", err)
	}
	mempoolPath := filepath.Join(dataDir, "transparent_mempool")
	if err := os.MkdirAll(mempoolPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create mempool directory: %w", err)
	}

	// Open persistent index database
	indexDB, err := leveldb.OpenFile(indexPath, &opt.Options{
		CompactionTableSize: 4 * 1024 * 1024, // 4MB
		WriteBuffer:         2 * 1024 * 1024, // 2MB
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open index database: %w", err)
	}
	mempoolDB, err := leveldb.OpenFile(mempoolPath, &opt.Options{
		CompactionTableSize: 4 * 1024 * 1024, // 4MB
		WriteBuffer:         2 * 1024 * 1024, // 2MB
	})
	if err != nil {
		_ = indexDB.Close()
		return nil, fmt.Errorf("failed to open mempool database: %w", err)
	}

	store := &TransparentTxStore{
		transactions: make(map[string]*TransparentTransaction),
		mempool:      make(map[string]*TransparentTransaction),
		utxos:        make(map[string][]UTXO),
		blockTxs:     make(map[uint32][]string),
		txIndex:      make(map[string]uint32),
		indexDB:      indexDB,
		mempoolDB:    mempoolDB,
	}

	// Load existing index into memory for fast lookups
	if err := store.loadIndexFromDB(); err != nil {
		indexDB.Close()
		mempoolDB.Close()
		return nil, fmt.Errorf("failed to load index: %w", err)
	}
	if err := store.loadMempoolFromDB(); err != nil {
		indexDB.Close()
		mempoolDB.Close()
		return nil, fmt.Errorf("failed to load mempool: %w", err)
	}

	log.Info(log.Node, "Initialized transparent transaction store",
		"indexedTxs", len(store.txIndex),
		"dataDir", dataDir)

	return store, nil
}

// AttachUtxoStore wires the persistent UTXO store and in-memory tree for confirmed updates.
func (s *TransparentTxStore) AttachUtxoStore(store *orchardtransparent.TransparentStore, tree *TransparentUtxoTree) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.utxoStore = store
	s.utxoTree = tree
}

// AddRawTransaction adds a raw transparent transaction to the store
func (s *TransparentTxStore) AddRawTransaction(txHex string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse the raw transaction
	tx, err := s.parseRawTransaction(txHex)
	if err != nil {
		return "", fmt.Errorf("failed to parse transaction: %w", err)
	}

	// Check if transaction already exists
	if _, exists := s.transactions[tx.TxID]; exists {
		if _, inMempool := s.mempool[tx.TxID]; inMempool {
			if err := s.persistMempoolTx(tx); err != nil {
				log.Warn(log.Node, "Failed to persist mempool tx", "txid", tx.TxID, "err", err)
			}
		}
		return tx.TxID, nil // Already have it
	}

	// Add to mempool
	tx.BlockHeight = 0
	tx.BlockHash = ""
	tx.Timestamp = time.Now()
	tx.Confirmations = 0

	s.mempool[tx.TxID] = tx
	s.transactions[tx.TxID] = tx
	s.mempoolSize = uint64(len(s.mempool))
	s.totalTransactions++

	// Update UTXO set
	s.updateUTXOSet(tx)
	if err := s.persistMempoolTx(tx); err != nil {
		log.Warn(log.Node, "Failed to persist mempool tx", "txid", tx.TxID, "err", err)
	}

	log.Info(log.Node, "Added transparent transaction to mempool",
		"txid", tx.TxID,
		"inputs", len(tx.Vin),
		"outputs", len(tx.Vout),
		"size", tx.Size)

	return tx.TxID, nil
}

// GetRawTransaction retrieves a raw transaction by txid from mempool or confirmed blocks
func (s *TransparentTxStore) GetRawTransaction(txid string, verbose bool) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// First check if transaction exists in storage (mempool or confirmed)
	tx, exists := s.transactions[txid]
	if !exists {
		// Check if we have it indexed but not loaded
		if blockHeight, indexed := s.txIndex[txid]; indexed && blockHeight > 0 {
			// Transaction exists but not loaded - this supports future blockchain scanning
			log.Info(log.Node, "Transaction found in index but not loaded", "txid", txid, "height", blockHeight)
			return nil, fmt.Errorf("transaction found at height %d but not loaded (blockchain scanning required)", blockHeight)
		}
		return nil, fmt.Errorf("transaction not found: %s", txid)
	}

	if !verbose {
		// Return just the hex string
		return tx.RawHex, nil
	}

	// Return full transaction object
	return tx, nil
}

// GetMempool returns all transactions in the mempool
func (s *TransparentTxStore) GetMempool(verbose bool) interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !verbose {
		// Return array of txids
		txids := make([]string, 0, len(s.mempool))
		for txid := range s.mempool {
			txids = append(txids, txid)
		}
		return txids
	}

	// Return detailed copy to avoid races while marshalling
	detailed := make(map[string]TransparentTransaction, len(s.mempool))
	for txid, tx := range s.mempool {
		detailed[txid] = cloneTransparentTransaction(tx)
	}
	return detailed
}

func (s *TransparentTxStore) snapshotMempoolTransactions() []TransparentTransaction {
	s.mu.RLock()
	defer s.mu.RUnlock()

	txs := make([]TransparentTransaction, 0, len(s.mempool))
	for _, tx := range s.mempool {
		txs = append(txs, cloneTransparentTransaction(tx))
	}
	return txs
}

// GetUTXOs returns unspent outputs for an address
func (s *TransparentTxStore) GetUTXOs(address string) []UTXO {
	s.mu.RLock()
	defer s.mu.RUnlock()

	utxos, exists := s.utxos[address]
	if !exists {
		return []UTXO{}
	}

	return utxos
}

// parseRawTransaction parses a raw hex transaction into structured format
func (s *TransparentTxStore) parseRawTransaction(txHex string) (*TransparentTransaction, error) {
	// Remove 0x prefix if present
	txHex = stripHexPrefix(txHex)

	// Decode hex
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}

	parsedTx, err := ParseZcashTxV5(txBytes)
	if err != nil {
		return nil, fmt.Errorf("parse v5 tx: %w", err)
	}
	txid := txIDToDisplay(parsedTx.TxID)

	parser := &txParser{data: txBytes}

	versionRaw, err := parser.readUint32LE()
	if err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}
	overwintered := (versionRaw & 0x80000000) != 0
	version := int(versionRaw & 0x7fffffff)
	if !overwintered {
		version = int(versionRaw)
	}
	if overwintered {
		if _, err := parser.readUint32LE(); err != nil {
			return nil, fmt.Errorf("failed to read version group id: %w", err)
		}
	}

	vinCount, err := parser.readVarInt()
	if err != nil {
		return nil, fmt.Errorf("failed to read input count: %w", err)
	}
	if vinCount > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("too many inputs: %d", vinCount)
	}

	vin := make([]TxInput, 0, int(vinCount))
	for i := uint64(0); i < vinCount; i++ {
		prevHash, err := parser.readBytes(32)
		if err != nil {
			return nil, fmt.Errorf("failed to read input prev hash: %w", err)
		}
		vout, err := parser.readUint32LE()
		if err != nil {
			return nil, fmt.Errorf("failed to read input vout: %w", err)
		}
		scriptLen, err := parser.readVarInt()
		if err != nil {
			return nil, fmt.Errorf("failed to read input script length: %w", err)
		}
		scriptBytes, err := parser.readBytes(int(scriptLen))
		if err != nil {
			return nil, fmt.Errorf("failed to read input script: %w", err)
		}
		sequence, err := parser.readUint32LE()
		if err != nil {
			return nil, fmt.Errorf("failed to read input sequence: %w", err)
		}

		isCoinbase := isCoinbaseInput(prevHash, vout)
		input := TxInput{
			TxID:      reverseHex(prevHash),
			Vout:      vout,
			ScriptSig: hex.EncodeToString(scriptBytes),
			Sequence:  sequence,
		}
		if isCoinbase {
			input.Coinbase = input.ScriptSig
		}
		vin = append(vin, input)
	}

	voutCount, err := parser.readVarInt()
	if err != nil {
		return nil, fmt.Errorf("failed to read output count: %w", err)
	}
	if voutCount > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("too many outputs: %d", voutCount)
	}

	vout := make([]TxOutput, 0, int(voutCount))
	for i := uint64(0); i < voutCount; i++ {
		value, err := parser.readUint64LE()
		if err != nil {
			return nil, fmt.Errorf("failed to read output value: %w", err)
		}
		scriptLen, err := parser.readVarInt()
		if err != nil {
			return nil, fmt.Errorf("failed to read output script length: %w", err)
		}
		scriptBytes, err := parser.readBytes(int(scriptLen))
		if err != nil {
			return nil, fmt.Errorf("failed to read output script: %w", err)
		}

		output := TxOutput{
			Value: float64(value) / 100000000.0,
			N:     uint32(i),
			ScriptPubKey: ScriptPubKey{
				Hex:  hex.EncodeToString(scriptBytes),
				Type: "nonstandard",
			},
		}
		vout = append(vout, output)
	}

	var lockTime uint32
	if parser.remaining() >= 4 {
		lockTime, err = parser.readUint32LE()
		if err != nil {
			return nil, fmt.Errorf("failed to read locktime: %w", err)
		}
	}

	tx := &TransparentTransaction{
		TxID:      txid,
		RawHex:    txHex,
		Size:      len(txBytes),
		Version:   version,
		LockTime:  lockTime,
		Vin:       vin,
		Vout:      vout,
		Timestamp: time.Now(),
	}

	return tx, nil
}

// updateUTXOSet updates the UTXO set based on a new transaction
func (s *TransparentTxStore) updateUTXOSet(tx *TransparentTransaction) {
	// Remove spent UTXOs (from inputs)
	for _, input := range tx.Vin {
		if input.Coinbase != "" {
			continue // Skip coinbase inputs
		}

		// Find and remove the UTXO being spent
		for addr, utxos := range s.utxos {
			newUTXOs := make([]UTXO, 0, len(utxos))
			for _, utxo := range utxos {
				if utxo.TxID == input.TxID && utxo.Vout == input.Vout {
					// This UTXO is being spent, don't add it to new list
					continue
				}
				newUTXOs = append(newUTXOs, utxo)
			}
			s.utxos[addr] = newUTXOs
		}
	}

	// Add new UTXOs (from outputs)
	for _, output := range tx.Vout {
		for _, addr := range output.ScriptPubKey.Addresses {
			utxo := UTXO{
				TxID:          tx.TxID,
				Vout:          output.N,
				Address:       addr,
				ScriptPubKey:  output.ScriptPubKey.Hex,
				Amount:        output.Value,
				Confirmations: tx.Confirmations,
			}
			s.utxos[addr] = append(s.utxos[addr], utxo)
		}
	}
}

func (s *TransparentTxStore) applyConfirmedTransaction(tx *TransparentTransaction, height uint32) error {
	if tx == nil || s.utxoStore == nil {
		return nil
	}

	raw, err := hex.DecodeString(stripHexPrefix(tx.RawHex))
	if err != nil {
		return fmt.Errorf("decode confirmed tx: %w", err)
	}
	parsed, err := ParseZcashTxV5(raw)
	if err != nil {
		return fmt.Errorf("parse confirmed tx: %w", err)
	}

	isCoinbase := len(parsed.Inputs) > 0 && parsed.Inputs[0].IsCoinbase()

	for _, input := range parsed.Inputs {
		if input.IsCoinbase() {
			continue
		}
		outpoint := orchardtransparent.OutPoint{
			TxID:  input.PrevOutHash,
			Index: input.PrevOutIndex,
		}
		if err := s.utxoStore.SpendUTXO(outpoint); err != nil {
			return fmt.Errorf("spend utxo: %w", err)
		}
		if s.utxoTree != nil {
			if err := s.utxoTree.Remove(Outpoint{
				Txid: input.PrevOutHash,
				Vout: input.PrevOutIndex,
			}); err != nil {
				return fmt.Errorf("remove utxo from tree: %w", err)
			}
		}
	}

	for i, output := range parsed.Outputs {
		outpoint := orchardtransparent.OutPoint{
			TxID:  parsed.TxID,
			Index: uint32(i),
		}
		utxo := orchardtransparent.UTXO{
			Value:        output.Value,
			ScriptPubKey: output.ScriptPubKey,
			Height:       height,
			IsCoinbase:   isCoinbase,
		}
		if err := s.utxoStore.AddUTXO(outpoint, utxo); err != nil {
			return fmt.Errorf("add utxo: %w", err)
		}
		if s.utxoTree != nil {
			s.utxoTree.Insert(Outpoint{
				Txid: parsed.TxID,
				Vout: uint32(i),
			}, UtxoData{
				Value:        output.Value,
				ScriptPubKey: output.ScriptPubKey,
				Height:       height,
				IsCoinbase:   isCoinbase,
			})
		}
	}

	return nil
}

// ConfirmTransaction moves a transaction from mempool to a block
func (s *TransparentTxStore) ConfirmTransaction(txid string, blockHeight uint32, blockHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, exists := s.mempool[txid]
	if !exists {
		return fmt.Errorf("transaction not in mempool: %s", txid)
	}

	if err := s.applyConfirmedTransaction(tx, blockHeight); err != nil {
		return err
	}

	// Update transaction
	tx.BlockHeight = blockHeight
	tx.BlockHash = blockHash
	tx.Confirmations = 1

	// Move from mempool to block
	delete(s.mempool, txid)
	if err := s.deleteMempoolTx(txid); err != nil {
		log.Warn(log.Node, "Failed to remove mempool tx", "txid", txid, "err", err)
	}
	s.blockTxs[blockHeight] = append(s.blockTxs[blockHeight], txid)
	s.mempoolSize = uint64(len(s.mempool))

	// Add to block height index for historical lookups (memory and disk)
	s.txIndex[txid] = blockHeight
	if err := s.persistTxIndex(txid, blockHeight); err != nil {
		log.Error(log.Node, "Failed to persist tx index", "txid", txid, "height", blockHeight, "error", err)
	}

	log.Info(log.Node, "Confirmed transparent transaction",
		"txid", txid,
		"block", blockHeight,
		"blockhash", blockHash)

	return nil
}

// GetMempoolInfo returns mempool statistics
func (s *TransparentTxStore) GetMempoolInfo() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalSize := 0
	for _, tx := range s.mempool {
		totalSize += tx.Size
	}

	return map[string]interface{}{
		"size":  len(s.mempool),
		"bytes": totalSize,
		"usage": totalSize * 2, // Rough estimate of memory usage
	}
}

// stripHexPrefix removes 0x prefix from hex string
func stripHexPrefix(s string) string {
	if len(s) >= 2 && s[0:2] == "0x" {
		return s[2:]
	}
	return s
}

type txParser struct {
	data   []byte
	offset int
}

func (p *txParser) remaining() int {
	return len(p.data) - p.offset
}

func (p *txParser) readBytes(n int) ([]byte, error) {
	if n < 0 || p.remaining() < n {
		return nil, fmt.Errorf("unexpected end of transaction")
	}
	start := p.offset
	p.offset += n
	return p.data[start:p.offset], nil
}

func (p *txParser) readUint32LE() (uint32, error) {
	b, err := p.readBytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (p *txParser) readUint64LE() (uint64, error) {
	b, err := p.readBytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

func (p *txParser) readVarInt() (uint64, error) {
	b, err := p.readBytes(1)
	if err != nil {
		return 0, err
	}
	prefix := b[0]
	switch prefix {
	case 0xfd:
		raw, err := p.readBytes(2)
		if err != nil {
			return 0, err
		}
		return uint64(binary.LittleEndian.Uint16(raw)), nil
	case 0xfe:
		raw, err := p.readBytes(4)
		if err != nil {
			return 0, err
		}
		return uint64(binary.LittleEndian.Uint32(raw)), nil
	case 0xff:
		raw, err := p.readBytes(8)
		if err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint64(raw), nil
	default:
		return uint64(prefix), nil
	}
}

func txIDFromBytes(txBytes []byte) string {
	first := sha256.Sum256(txBytes)
	second := sha256.Sum256(first[:])
	reversed := reverseBytes(second[:])
	return hex.EncodeToString(reversed)
}

func reverseHex(b []byte) string {
	return hex.EncodeToString(reverseBytes(b))
}

func reverseBytes(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}

func isCoinbaseInput(prevHash []byte, vout uint32) bool {
	if vout != 0xffffffff {
		return false
	}
	for _, b := range prevHash {
		if b != 0 {
			return false
		}
	}
	return true
}

// ===== Transparent Transaction Merkle Tree (Zcash/Bitcoin-style) =====

// GetMerkleRoot computes the Merkle root of all transparent transactions at a given block height
// This provides a cryptographic commitment to all transparent transactions in a block,
// following the standard Zcash/Bitcoin Merkle tree construction.
func (s *TransparentTxStore) GetMerkleRoot(blockHeight uint32) ([32]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	txids, exists := s.blockTxs[blockHeight]
	if !exists || len(txids) == 0 {
		// Empty tree - return zero hash
		return [32]byte{}, nil
	}

	// Convert txid strings to [32]byte hashes
	// IMPORTANT: txid strings are stored in little-endian (display format) after reversal,
	// but Merkle tree construction requires internal byte order (big-endian).
	// We must reverse back to internal order for Zcash/Bitcoin compatibility.
	leaves := make([][32]byte, len(txids))
	for i, txid := range txids {
		txidBytes, err := hex.DecodeString(txid)
		if err != nil {
			return [32]byte{}, fmt.Errorf("invalid txid hex: %w", err)
		}
		if len(txidBytes) != 32 {
			return [32]byte{}, fmt.Errorf("invalid txid length: %d", len(txidBytes))
		}
		// Reverse from display order (little-endian) to internal order (big-endian)
		reversed := reverseBytes(txidBytes)
		copy(leaves[i][:], reversed)
	}

	return buildMerkleRoot(leaves), nil
}

// buildMerkleRoot constructs a Bitcoin/Zcash-style Merkle tree from transaction IDs
// Returns the root hash that commits to all transactions
func buildMerkleRoot(leaves [][32]byte) [32]byte {
	if len(leaves) == 0 {
		return [32]byte{} // Empty tree
	}

	if len(leaves) == 1 {
		return leaves[0] // Single transaction
	}

	// Build tree level by level
	currentLevel := leaves

	for len(currentLevel) > 1 {
		nextLevel := make([][32]byte, (len(currentLevel)+1)/2)

		for i := 0; i < len(currentLevel); i += 2 {
			left := currentLevel[i]
			right := left // Duplicate last node if odd number (Bitcoin/Zcash convention)

			if i+1 < len(currentLevel) {
				right = currentLevel[i+1]
			}

			// Concatenate left || right
			combined := make([]byte, 64)
			copy(combined[0:32], left[:])
			copy(combined[32:64], right[:])

			// Double SHA-256 hash (Bitcoin/Zcash standard)
			firstHash := sha256.Sum256(combined)
			secondHash := sha256.Sum256(firstHash[:])

			nextLevel[i/2] = secondHash
		}

		currentLevel = nextLevel
	}

	return currentLevel[0]
}

// ConfirmTransactions moves transactions from mempool to a block at the given height
// This is called when a block is finalized and transparent transactions should be confirmed
func (s *TransparentTxStore) ConfirmTransactions(txids []string, blockHeight uint32, blockHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, txid := range txids {
		tx, exists := s.mempool[txid]
		if !exists {
			// Transaction not in mempool - might already be confirmed or doesn't exist
			continue
		}

		if err := s.applyConfirmedTransaction(tx, blockHeight); err != nil {
			return err
		}

		// Update transaction
		tx.BlockHeight = blockHeight
		tx.BlockHash = blockHash
		tx.Confirmations = 1

		// Move from mempool to block
		delete(s.mempool, txid)
		if err := s.deleteMempoolTx(txid); err != nil {
			log.Warn(log.Node, "Failed to remove mempool tx", "txid", txid, "err", err)
		}

		// Add to block height index for historical lookups (memory and disk)
		s.txIndex[txid] = blockHeight
		if err := s.persistTxIndex(txid, blockHeight); err != nil {
			log.Error(log.Node, "Failed to persist tx index", "txid", txid, "height", blockHeight, "error", err)
		}

		log.Info(log.Node, "Confirmed transparent transaction",
			"txid", txid,
			"block", blockHeight,
			"blockhash", blockHash)
	}

	// Add to block index
	s.blockTxs[blockHeight] = txids
	s.mempoolSize = uint64(len(s.mempool))

	return nil
}

// GetBlockTransactions returns all transaction IDs for a given block height
func (s *TransparentTxStore) GetBlockTransactions(blockHeight uint32) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	txids, exists := s.blockTxs[blockHeight]
	if !exists {
		return []string{}
	}

	// Return copy to avoid race conditions
	result := make([]string, len(txids))
	copy(result, txids)
	return result
}

// GetBlockTransactionsVerbose returns full transaction details for all transactions in a block
func (s *TransparentTxStore) GetBlockTransactionsVerbose(blockHeight uint32) ([]*TransparentTransaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	txids, exists := s.blockTxs[blockHeight]
	if !exists {
		return nil, nil // No transactions in this block
	}

	result := make([]*TransparentTransaction, 0, len(txids))
	for _, txid := range txids {
		tx, exists := s.transactions[txid]
		if !exists {
			// This shouldn't happen - log warning but continue
			log.Warn(log.Node, "Transaction in block index but not in storage", "txid", txid, "block", blockHeight)
			continue
		}
		// Return copy to avoid race conditions
		txCopy := cloneTransparentTransaction(tx)
		result = append(result, &txCopy)
	}

	return result, nil
}

func cloneTransparentTransaction(tx *TransparentTransaction) TransparentTransaction {
	if tx == nil {
		return TransparentTransaction{}
	}
	clone := *tx
	if tx.Vin != nil {
		clone.Vin = append([]TxInput(nil), tx.Vin...)
	}
	if tx.Vout != nil {
		clone.Vout = append([]TxOutput(nil), tx.Vout...)
	}
	return clone
}

// rebuildMempoolUTXOs rebuilds the address-indexed UTXO cache from mempool transactions.
// It applies outputs first, then removes spent outpoints, so dependency order doesn't matter.
func (s *TransparentTxStore) rebuildMempoolUTXOs(txs []*TransparentTransaction) {
	s.utxos = make(map[string][]UTXO)

	for _, tx := range txs {
		for _, output := range tx.Vout {
			for _, addr := range output.ScriptPubKey.Addresses {
				utxo := UTXO{
					TxID:          tx.TxID,
					Vout:          output.N,
					Address:       addr,
					ScriptPubKey:  output.ScriptPubKey.Hex,
					Amount:        output.Value,
					Confirmations: 0,
				}
				s.utxos[addr] = append(s.utxos[addr], utxo)
			}
		}
	}

	for _, tx := range txs {
		for _, input := range tx.Vin {
			if input.Coinbase != "" {
				continue
			}
			for addr, utxos := range s.utxos {
				newUTXOs := make([]UTXO, 0, len(utxos))
				for _, utxo := range utxos {
					if utxo.TxID == input.TxID && utxo.Vout == input.Vout {
						continue
					}
					newUTXOs = append(newUTXOs, utxo)
				}
				s.utxos[addr] = newUTXOs
			}
		}
	}
}

// Persistent index management methods

// loadIndexFromDB loads the txid -> block height index from persistent storage into memory
func (s *TransparentTxStore) loadIndexFromDB() error {
	if s.indexDB == nil {
		return nil
	}
	iter := s.indexDB.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
		txid := string(iter.Key())

		if len(iter.Value()) != 4 {
			log.Warn(log.Node, "Invalid block height entry in index", "txid", txid, "size", len(iter.Value()))
			continue
		}

		blockHeight := binary.LittleEndian.Uint32(iter.Value())
		s.txIndex[txid] = blockHeight
	}

	return iter.Error()
}

// loadMempoolFromDB loads mempool transactions into memory and rebuilds the UTXO cache.
func (s *TransparentTxStore) loadMempoolFromDB() error {
	if s.mempoolDB == nil {
		return nil
	}

	iter := s.mempoolDB.NewIterator(nil, nil)
	defer iter.Release()

	loaded := make([]*TransparentTransaction, 0)
	for iter.Next() {
		txid := string(iter.Key())
		var tx TransparentTransaction
		if err := json.Unmarshal(iter.Value(), &tx); err != nil {
			log.Warn(log.Node, "Invalid mempool entry", "txid", txid, "err", err)
			continue
		}
		if tx.TxID == "" {
			tx.TxID = txid
		}
		tx.BlockHeight = 0
		tx.BlockHash = ""
		tx.Confirmations = 0

		txCopy := tx
		s.mempool[txid] = &txCopy
		s.transactions[txid] = &txCopy
		loaded = append(loaded, &txCopy)
	}

	if err := iter.Error(); err != nil {
		return err
	}

	s.mempoolSize = uint64(len(s.mempool))
	if len(s.transactions) > 0 {
		s.totalTransactions = uint64(len(s.transactions))
	}
	s.rebuildMempoolUTXOs(loaded)
	return nil
}

func (s *TransparentTxStore) persistMempoolTx(tx *TransparentTransaction) error {
	if s.mempoolDB == nil || tx == nil {
		return nil
	}
	data, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	return s.mempoolDB.Put([]byte(tx.TxID), data, nil)
}

func (s *TransparentTxStore) deleteMempoolTx(txid string) error {
	if s.mempoolDB == nil {
		return nil
	}
	return s.mempoolDB.Delete([]byte(txid), nil)
}

// persistTxIndex saves a txid -> block height mapping to persistent storage
func (s *TransparentTxStore) persistTxIndex(txid string, blockHeight uint32) error {
	if s.indexDB == nil {
		return fmt.Errorf("index database not initialized")
	}
	key := []byte(txid)
	value := make([]byte, 4)
	binary.LittleEndian.PutUint32(value, blockHeight)

	return s.indexDB.Put(key, value, nil)
}

// GetTransactionBlockHeight returns the block height for a given txid
// Returns 0 if transaction is in mempool or not found
func (s *TransparentTxStore) GetTransactionBlockHeight(txid string) (uint32, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check in-memory cache first
	if height, exists := s.txIndex[txid]; exists {
		return height, true
	}

	// Check if it's in mempool (height 0)
	if _, exists := s.mempool[txid]; exists {
		return 0, true
	}

	return 0, false
}

// Close closes the persistent storage and cleans up resources
func (s *TransparentTxStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	if s.indexDB != nil {
		err = s.indexDB.Close()
	}
	if s.mempoolDB != nil {
		if closeErr := s.mempoolDB.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

// GetIndexStats returns statistics about the persistent index
func (s *TransparentTxStore) GetIndexStats() (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]interface{}{
		"memoryIndexSize": len(s.txIndex),
		"mempoolSize":     len(s.mempool),
	}

	if s.indexDB != nil {
		// Get basic LevelDB property statistics
		if sizes, err := s.indexDB.SizeOf(nil); err == nil {
			stats["diskStats"] = map[string]interface{}{
				"diskUsage": sizes,
			}
		}
	}

	return stats, nil
}
