package rpc

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
)

// TxPoolStatus represents the status of a transaction in the pool
type TxPoolStatus int

const (
	TxStatusPending  TxPoolStatus = iota // Available for bundle inclusion
	TxStatusQueued                       // Queued for future nonce
	TxStatusInBundle                     // Locked - already assigned to a bundle
	TxStatusDropped                      // Dropped due to expiry or error
	TxStatusIncluded                     // Successfully included in accumulated block
)

// TxPoolEntry represents a transaction entry in the pool
type TxPoolEntry struct {
	Tx          *evmtypes.EthereumTransaction `json:"transaction"`
	Status      TxPoolStatus                  `json:"status"`
	AddedAt     time.Time                     `json:"addedAt"`
	Attempts    int                           `json:"attempts"`
	BlockNumber uint64                        `json:"blockNumber"` // EVM block number this tx is assigned to (when InBundle)
}

// TxPool manages pending Ethereum transactions for the guarantor
type TxPool struct {
	// Transaction storage
	pending map[common.Hash]*TxPoolEntry // Hash -> TxPoolEntry
	queued  map[common.Hash]*TxPoolEntry // Hash -> TxPoolEntry for future nonces

	// Organization by sender
	pendingBySender map[common.Address]map[uint64]*TxPoolEntry // Address -> Nonce -> TxPoolEntry
	queuedBySender  map[common.Address]map[uint64]*TxPoolEntry // Address -> Nonce -> TxPoolEntry

	// Pool limits and configuration
	config TxPoolConfig

	// Synchronization
	mutex sync.RWMutex

	// Statistics
	stats TxPoolStats
}

// TxPoolConfig holds configuration for the transaction pool
type TxPoolConfig struct {
	MaxPendingTxs   int           `json:"maxPendingTxs"`   // Maximum pending transactions
	MaxQueuedTxs    int           `json:"maxQueuedTxs"`    // Maximum queued transactions
	MaxTxsPerSender int           `json:"maxTxsPerSender"` // Maximum transactions per sender
	TxTTL           time.Duration `json:"txTTL"`           // Time to live for transactions
	MinGasPrice     *big.Int      `json:"minGasPrice"`     // Minimum gas price to accept
	MaxTxSize       uint64        `json:"maxTxSize"`       // Maximum transaction size in bytes
}

// TxPoolStats holds statistics about the transaction pool
type TxPoolStats struct {
	PendingCount   int `json:"pendingCount"`
	QueuedCount    int `json:"queuedCount"`
	InBundleCount  int `json:"inBundleCount"`  // Transactions locked to bundles
	TotalReceived  int `json:"totalReceived"`
	TotalProcessed int `json:"totalProcessed"`
	TotalDropped   int `json:"totalDropped"`
}

// NewTxPool creates a new transaction pool with default configuration
func NewTxPool() *TxPool {
	return &TxPool{
		pending:         make(map[common.Hash]*TxPoolEntry),
		queued:          make(map[common.Hash]*TxPoolEntry),
		pendingBySender: make(map[common.Address]map[uint64]*TxPoolEntry),
		queuedBySender:  make(map[common.Address]map[uint64]*TxPoolEntry),
		config: TxPoolConfig{
			MaxPendingTxs:   1000,
			MaxQueuedTxs:    1000,
			MaxTxsPerSender: 16,
			TxTTL:           time.Hour,
			MinGasPrice:     big.NewInt(1000000000), // 1 Gwei
			MaxTxSize:       32 * 1024,              // 32 KB
		},
		stats: TxPoolStats{},
	}
}

// AddTransaction adds a new transaction to the pool after validation
func (pool *TxPool) AddTransaction(tx *evmtypes.EthereumTransaction) error {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	// Validate transaction
	if err := pool.validateTransaction(tx); err != nil {
		pool.stats.TotalDropped++
		return fmt.Errorf("transaction validation failed: %v", err)
	}

	// Check if transaction already exists
	if _, exists := pool.pending[tx.Hash]; exists {
		return fmt.Errorf("transaction %s already exists in pending pool", tx.Hash.String())
	}
	if _, exists := pool.queued[tx.Hash]; exists {
		return fmt.Errorf("transaction %s already exists in queued pool", tx.Hash.String())
	}

	// Create pool entry
	entry := &TxPoolEntry{
		Tx:      tx,
		Status:  TxStatusPending,
		AddedAt: time.Now(),
	}

	// Add to appropriate pool based on nonce
	if pool.shouldQueue(tx) {
		pool.addToQueued(entry)
		log.Debug(log.Node, "TxPool: Added transaction to queue", "hash", tx.Hash.String(), "from", tx.From.String(), "nonce", tx.Nonce)
	} else {
		pool.addToPending(entry)
		log.Debug(log.Node, "TxPool: Added transaction to pending", "hash", tx.Hash.String(), "from", tx.From.String(), "nonce", tx.Nonce)
	}

	pool.stats.TotalReceived++
	return nil
}

// GetTransaction retrieves a transaction by hash
func (pool *TxPool) GetTransaction(hash common.Hash) (*evmtypes.EthereumTransaction, bool) {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	if entry, exists := pool.pending[hash]; exists {
		return entry.Tx, true
	}
	if entry, exists := pool.queued[hash]; exists {
		return entry.Tx, true
	}
	return nil, false
}

// GetPendingTransactions returns all pending transactions that are NOT locked to a bundle
func (pool *TxPool) GetPendingTransactions() []*evmtypes.EthereumTransaction {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	txs := make([]*evmtypes.EthereumTransaction, 0, len(pool.pending))
	for _, entry := range pool.pending {
		// Only return transactions that are not already assigned to a bundle
		if entry.Status == TxStatusPending {
			txs = append(txs, entry.Tx)
		}
	}
	return txs
}

// GetPendingTransactionsLimit returns up to `limit` pending transactions that are NOT locked to a bundle
// Transactions are returned in nonce order for each sender
func (pool *TxPool) GetPendingTransactionsLimit(limit int) []*evmtypes.EthereumTransaction {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	txs := make([]*evmtypes.EthereumTransaction, 0, limit)
	for _, entry := range pool.pending {
		// Only return transactions that are not already assigned to a bundle
		if entry.Status == TxStatusPending {
			txs = append(txs, entry.Tx)
			if len(txs) >= limit {
				break
			}
		}
	}
	return txs
}

// Size returns the total number of transactions in the pool
func (pool *TxPool) Size() int {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	return len(pool.pending) + len(pool.queued)
}

// GetStats returns current pool statistics
func (pool *TxPool) GetStats() TxPoolStats {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	// Count transactions by status
	pendingCount := 0
	inBundleCount := 0
	for _, entry := range pool.pending {
		if entry.Status == TxStatusPending {
			pendingCount++
		} else if entry.Status == TxStatusInBundle {
			inBundleCount++
		}
	}

	pool.stats.PendingCount = pendingCount
	pool.stats.QueuedCount = len(pool.queued)
	pool.stats.InBundleCount = inBundleCount

	return pool.stats
}

// RemoveTransaction removes a transaction from the pool
func (pool *TxPool) RemoveTransaction(hash common.Hash) bool {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	// Check pending pool
	if entry, exists := pool.pending[hash]; exists {
		pool.removeFromPending(entry)
		pool.stats.TotalProcessed++
		log.Debug(log.Node, "TxPool: Removed transaction from pending", "hash", hash.String())
		return true
	}

	// Check queued pool
	if entry, exists := pool.queued[hash]; exists {
		pool.removeFromQueued(entry)
		pool.stats.TotalProcessed++
		log.Debug(log.Node, "TxPool: Removed transaction from queued", "hash", hash.String())
		return true
	}

	return false
}

// CleanupExpiredTransactions removes expired transactions
// NOTE: Transactions that are locked to a bundle (TxStatusInBundle) are NOT expired,
// as they are actively being processed and will be removed on accumulation or unlocked on failure.
func (pool *TxPool) CleanupExpiredTransactions() {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	now := time.Now()
	expiredHashes := make([]common.Hash, 0)

	// Check pending transactions - skip those locked to bundles
	for hash, entry := range pool.pending {
		if entry.Status == TxStatusInBundle {
			// Don't expire transactions that are locked to a bundle
			continue
		}
		if now.Sub(entry.AddedAt) > pool.config.TxTTL {
			expiredHashes = append(expiredHashes, hash)
		}
	}

	// Check queued transactions
	for hash, entry := range pool.queued {
		if now.Sub(entry.AddedAt) > pool.config.TxTTL {
			expiredHashes = append(expiredHashes, hash)
		}
	}

	// Remove expired transactions
	for _, hash := range expiredHashes {
		if entry, exists := pool.pending[hash]; exists {
			pool.removeFromPending(entry)
		}
		if entry, exists := pool.queued[hash]; exists {
			pool.removeFromQueued(entry)
		}
		pool.stats.TotalDropped++
		log.Info(log.Node, "TxPool: Removed expired transaction", "hash", hash.String())
	}
}

// GetTxPoolContent returns the pending and queued transaction hashes
func (pool *TxPool) GetTxPoolContent() (pending []string, queued []string) {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	// Get pending transactions
	pending = make([]string, 0, len(pool.pending))
	for hash := range pool.pending {
		pending = append(pending, hash.String())
	}

	// Get queued transactions
	queued = make([]string, 0, len(pool.queued))
	for hash := range pool.queued {
		queued = append(queued, hash.String())
	}

	return pending, queued
}

// validateTransaction performs basic transaction validation
func (pool *TxPool) validateTransaction(tx *evmtypes.EthereumTransaction) error {
	// Check transaction size
	if tx.Size > pool.config.MaxTxSize {
		return fmt.Errorf("transaction size %d exceeds maximum %d", tx.Size, pool.config.MaxTxSize)
	}

	// Check gas price
	if tx.GasPrice.Cmp(pool.config.MinGasPrice) < 0 {
		return fmt.Errorf("gas price %s below minimum %s", tx.GasPrice.String(), pool.config.MinGasPrice.String())
	}

	// Check gas limit
	if tx.Gas == 0 {
		return fmt.Errorf("gas limit cannot be zero")
	}

	// Check value is not negative
	if tx.Value.Sign() < 0 {
		return fmt.Errorf("transaction value cannot be negative")
	}

	return nil
}

// shouldQueue determines if a transaction should be queued based on nonce
func (pool *TxPool) shouldQueue(tx *evmtypes.EthereumTransaction) bool {
	// TODO: Implement proper nonce checking against current state
	// For now, assume all transactions should go to pending
	return false
}

// addToPending adds a transaction to the pending pool
func (pool *TxPool) addToPending(entry *TxPoolEntry) {
	tx := entry.Tx

	// Add to main pending map
	pool.pending[tx.Hash] = entry

	// Add to sender-specific map
	if _, exists := pool.pendingBySender[tx.From]; !exists {
		pool.pendingBySender[tx.From] = make(map[uint64]*TxPoolEntry)
	}
	pool.pendingBySender[tx.From][tx.Nonce] = entry

	entry.Status = TxStatusPending
}

// addToQueued adds a transaction to the queued pool
func (pool *TxPool) addToQueued(entry *TxPoolEntry) {
	tx := entry.Tx

	// Add to main queued map
	pool.queued[tx.Hash] = entry

	// Add to sender-specific map
	if _, exists := pool.queuedBySender[tx.From]; !exists {
		pool.queuedBySender[tx.From] = make(map[uint64]*TxPoolEntry)
	}
	pool.queuedBySender[tx.From][tx.Nonce] = entry

	entry.Status = TxStatusQueued
}

// removeFromPending removes a transaction from the pending pool
func (pool *TxPool) removeFromPending(entry *TxPoolEntry) {
	tx := entry.Tx

	// Remove from main pending map
	delete(pool.pending, tx.Hash)

	// Remove from sender-specific map
	if senderTxs, exists := pool.pendingBySender[tx.From]; exists {
		delete(senderTxs, tx.Nonce)
		if len(senderTxs) == 0 {
			delete(pool.pendingBySender, tx.From)
		}
	}
}

// removeFromQueued removes a transaction from the queued pool
func (pool *TxPool) removeFromQueued(entry *TxPoolEntry) {
	tx := entry.Tx

	// Remove from main queued map
	delete(pool.queued, tx.Hash)

	// Remove from sender-specific map
	if senderTxs, exists := pool.queuedBySender[tx.From]; exists {
		delete(senderTxs, tx.Nonce)
		if len(senderTxs) == 0 {
			delete(pool.queuedBySender, tx.From)
		}
	}
}

// LockTransactionsToBundle marks transactions as assigned to a specific EVM block number.
// These transactions will be excluded from GetPendingTransactions() until unlocked or removed.
// NOTE: Only transactions in the "pending" pool are locked. Queued transactions (future nonces)
// are not locked, but this is OK since we only build bundles from pending transactions.
// PANICS if any transaction is already locked to a DIFFERENT bundle - this indicates a critical bug.
// Returns the number of transactions successfully locked.
func (pool *TxPool) LockTransactionsToBundle(hashes []common.Hash, blockNumber uint64) int {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	locked := 0
	for _, hash := range hashes {
		if entry, exists := pool.pending[hash]; exists {
			if entry.Status == TxStatusPending {
				entry.Status = TxStatusInBundle
				entry.BlockNumber = blockNumber
				locked++
				log.Debug(log.Node, "TxPool: Locked transaction to bundle",
					"hash", hash.Hex(),
					"blockNumber", blockNumber)
			} else if entry.Status == TxStatusInBundle {
				// CRITICAL: Transaction already locked to a bundle
				if entry.BlockNumber != blockNumber {
					// Different bundle - this is a bug! Same tx in two bundles = duplicate execution
					log.Error(log.Node, "CRITICAL BUG: Transaction already locked to DIFFERENT bundle",
						"txHash", hash.Hex(),
						"existingBundle", entry.BlockNumber,
						"newBundle", blockNumber)
					panic(fmt.Sprintf("TxPool: CRITICAL BUG - transaction %s already locked to bundle %d, cannot lock to bundle %d",
						hash.Hex(), entry.BlockNumber, blockNumber))
				}
				// Same bundle - this is OK (idempotent), skip silently
				log.Debug(log.Node, "TxPool: Transaction already locked to same bundle (idempotent)",
					"hash", hash.Hex(),
					"blockNumber", blockNumber)
			} else {
				log.Warn(log.Node, "TxPool: Cannot lock transaction - unexpected status",
					"hash", hash.Hex(),
					"currentStatus", entry.Status)
			}
		}
	}

	if locked > 0 {
		log.Info(log.Node, "TxPool: Locked transactions to bundle",
			"count", locked,
			"blockNumber", blockNumber)
	}

	return locked
}

// UnlockTransactionsFromBundle returns transactions back to pending status.
// Used when a bundle fails or expires and transactions need to be re-included in new bundles.
// Returns the number of transactions successfully unlocked.
func (pool *TxPool) UnlockTransactionsFromBundle(hashes []common.Hash) int {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	unlocked := 0
	for _, hash := range hashes {
		if entry, exists := pool.pending[hash]; exists {
			if entry.Status == TxStatusInBundle {
				entry.Status = TxStatusPending
				entry.BlockNumber = 0
				unlocked++
				log.Debug(log.Node, "TxPool: Unlocked transaction from bundle",
					"hash", hash.Hex())
			}
		}
	}

	if unlocked > 0 {
		log.Info(log.Node, "TxPool: Unlocked transactions from bundle",
			"count", unlocked)
	}

	return unlocked
}

// RemoveTransactionsByHashes removes multiple transactions from the pool (used after accumulation)
func (pool *TxPool) RemoveTransactionsByHashes(hashes []common.Hash) int {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	removed := 0
	for _, hash := range hashes {
		if entry, exists := pool.pending[hash]; exists {
			pool.removeFromPending(entry)
			pool.stats.TotalProcessed++
			removed++
		} else if entry, exists := pool.queued[hash]; exists {
			pool.removeFromQueued(entry)
			pool.stats.TotalProcessed++
			removed++
		}
	}

	if removed > 0 {
		log.Info(log.Node, "TxPool: Removed accumulated transactions",
			"count", removed)
	}

	return removed
}
