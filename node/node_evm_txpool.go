package node

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
)

// TxPoolStatus represents the status of a transaction in the pool
type TxPoolStatus int

const (
	TxStatusPending TxPoolStatus = iota
	TxStatusQueued
	TxStatusDropped
	TxStatusIncluded
)

// TxPoolEntry represents a transaction entry in the pool
type TxPoolEntry struct {
	Tx       *EthereumTransaction `json:"transaction"`
	Status   TxPoolStatus         `json:"status"`
	AddedAt  time.Time            `json:"addedAt"`
	Attempts int                  `json:"attempts"`
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
func (pool *TxPool) AddTransaction(tx *EthereumTransaction) error {
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
		log.Info(log.Node, "TxPool: Added transaction to queue", "hash", tx.Hash.String(), "from", tx.From.String(), "nonce", tx.Nonce)
	} else {
		pool.addToPending(entry)
		log.Info(log.Node, "TxPool: Added transaction to pending", "hash", tx.Hash.String(), "from", tx.From.String(), "nonce", tx.Nonce)
	}

	pool.stats.TotalReceived++
	return nil
}

// GetTransaction retrieves a transaction by hash
func (pool *TxPool) GetTransaction(hash common.Hash) (*EthereumTransaction, bool) {
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

// GetPendingTransactions returns all pending transactions
func (pool *TxPool) GetPendingTransactions() []*EthereumTransaction {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	txs := make([]*EthereumTransaction, 0, len(pool.pending))
	for _, entry := range pool.pending {
		txs = append(txs, entry.Tx)
	}
	return txs
}

// GetStats returns current pool statistics
func (pool *TxPool) GetStats() TxPoolStats {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	// Update current counts
	pool.stats.PendingCount = len(pool.pending)
	pool.stats.QueuedCount = len(pool.queued)

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
		log.Info(log.Node, "TxPool: Removed transaction from pending", "hash", hash.String())
		return true
	}

	// Check queued pool
	if entry, exists := pool.queued[hash]; exists {
		pool.removeFromQueued(entry)
		pool.stats.TotalProcessed++
		log.Info(log.Node, "TxPool: Removed transaction from queued", "hash", hash.String())
		return true
	}

	return false
}

// CleanupExpiredTransactions removes expired transactions
func (pool *TxPool) CleanupExpiredTransactions() {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	now := time.Now()
	expiredHashes := make([]common.Hash, 0)

	// Check pending transactions
	for hash, entry := range pool.pending {
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

// validateTransaction performs basic transaction validation
func (pool *TxPool) validateTransaction(tx *EthereumTransaction) error {
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
func (pool *TxPool) shouldQueue(tx *EthereumTransaction) bool {
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

// EthereumTransaction represents a parsed Ethereum transaction
type EthereumTransaction struct {
	Hash     common.Hash     `json:"hash"`
	From     common.Address  `json:"from"`
	To       *common.Address `json:"to"`
	Value    *big.Int        `json:"value"`
	Gas      uint64          `json:"gas"`
	GasPrice *big.Int        `json:"gasPrice"`
	Nonce    uint64          `json:"nonce"`
	Data     []byte          `json:"data"`
	V        *big.Int        `json:"v"`
	R        *big.Int        `json:"r"`
	S        *big.Int        `json:"s"`

	// JAM-specific fields
	ReceivedAt time.Time `json:"receivedAt"`
	Size       uint64    `json:"size"`

	// Store the original go-ethereum transaction for proper type handling
	inner *ethereumTypes.Transaction `json:"-"`
}
