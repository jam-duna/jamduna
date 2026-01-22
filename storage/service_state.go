package storage

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/common"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

// ServiceStateManager handles multi-rollup EVM block tracking.
// Each EVM service maintains its own block history. Shared across NewSession instances.
//
// NOTE: In-memory indices (serviceUBTRoots, serviceJAMStateRoots, serviceBlockHashIndex,
// latestRollupBlock) are NOT persisted and must be rebuilt after restart.
type ServiceStateManager struct {
	// Persistence for block storage (block data is persisted, indices are not)
	kv *PersistenceStore

	// In-memory indices (not persisted - lost on restart)
	//
	// Service-scoped state roots
	// Key: "vr_{serviceID}_{blockNumber}" → state root
	serviceUBTRoots map[string]common.Hash

	// Service-scoped JAM state roots (links service block to JAM state)
	// Key: "jsr_{serviceID}_{blockNumber}" → ServiceBlockIndex
	serviceJAMStateRoots map[string]*types.ServiceBlockIndex

	// Service block hash index
	// Key: "bhash_{serviceID}_{blockHash}" → blockNumber
	serviceBlockHashIndex map[string]uint64

	// Latest rollup block number per service
	latestRollupBlock map[uint32]uint64

	// Mutex for all service-scoped maps
	mutex sync.RWMutex
}

// NewServiceStateManager creates a new service state manager.
func NewServiceStateManager(kv *PersistenceStore) *ServiceStateManager {
	return &ServiceStateManager{
		kv:                    kv,
		serviceUBTRoots:       make(map[string]common.Hash),
		serviceJAMStateRoots:  make(map[string]*types.ServiceBlockIndex),
		serviceBlockHashIndex: make(map[string]uint64),
		latestRollupBlock:     make(map[uint32]uint64),
	}
}

// --- Key Generation Helpers ---

func (m *ServiceStateManager) ubtRootKey(serviceID uint32, blockNumber uint32) string {
	return fmt.Sprintf("vr_%d_%d", serviceID, blockNumber)
}

func (m *ServiceStateManager) jamStateRootKey(serviceID uint32, blockNumber uint32) string {
	return fmt.Sprintf("jsr_%d_%d", serviceID, blockNumber)
}

func (m *ServiceStateManager) blockHashKey(serviceID uint32, blockHash common.Hash) string {
	return fmt.Sprintf("bhash_%d_%s", serviceID, blockHash.Hex())
}

// parseBlockNumber parses a block number string into uint32
// Supports: "latest", "earliest", "pending", or hex number (0x...)
func (m *ServiceStateManager) parseBlockNumber(blockNumber string, latestBlock uint64) (uint32, error) {
	switch blockNumber {
	case "latest", "":
		return uint32(latestBlock), nil
	case "earliest":
		return 0, nil
	case "pending":
		return uint32(latestBlock), nil
	default:
		// Try parsing as hex
		if len(blockNumber) > 2 && blockNumber[:2] == "0x" {
			blockNum, err := hex.DecodeString(blockNumber[2:])
			if err != nil {
				return 0, fmt.Errorf("invalid hex block number: %w", err)
			}
			var num uint64
			for _, b := range blockNum {
				num = (num << 8) | uint64(b)
			}
			return uint32(num), nil
		}
		return 0, fmt.Errorf("invalid block number format: %s", blockNumber)
	}
}

// --- Latest Block Tracking ---

// GetLatestBlock returns the latest block number for a service.
func (m *ServiceStateManager) GetLatestBlock(serviceID uint32) (uint64, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	block, exists := m.latestRollupBlock[serviceID]
	return block, exists
}

// --- UBT Root Lookup ---

// GetUBTRootForBlock returns the UBT root hash for a specific service block.
func (m *ServiceStateManager) GetUBTRootForBlock(serviceID uint32, blockNumber uint32) (common.Hash, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	key := m.ubtRootKey(serviceID, blockNumber)
	root, ok := m.serviceUBTRoots[key]
	return root, ok
}

// --- Block Hash Lookup ---

// GetBlockNumberByHash returns the block number for a given block hash.
func (m *ServiceStateManager) GetBlockNumberByHash(serviceID uint32, blockHash common.Hash) (uint64, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	key := m.blockHashKey(serviceID, blockHash)
	blockNum, exists := m.serviceBlockHashIndex[key]
	return blockNum, exists
}

// --- Block Storage ---

// StoreServiceBlock persists an EVM block for a specific service.
// Updates in-memory indices and persists block data to LevelDB.
func (m *ServiceStateManager) StoreServiceBlock(
	serviceID uint32,
	block *evmtypes.EvmBlockPayload,
	jamStateRoot common.Hash,
	jamSlot uint32,
) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	blockNumber := block.Number

	// Create ServiceBlockIndex
	index := &types.ServiceBlockIndex{
		ServiceID:    serviceID,
		BlockNumber:  blockNumber,
		BlockHash:    block.WorkPackageHash,
		UBTRoot:      block.UBTRoot,
		JAMStateRoot: jamStateRoot,
		JAMSlot:      jamSlot,
	}

	// Store UBT root mapping
	ubtKey := m.ubtRootKey(serviceID, blockNumber)
	m.serviceUBTRoots[ubtKey] = block.UBTRoot

	// Store JAM state root mapping
	jsrKey := m.jamStateRootKey(serviceID, blockNumber)
	m.serviceJAMStateRoots[jsrKey] = index

	// Store block hash index
	bhKey := m.blockHashKey(serviceID, block.WorkPackageHash)
	m.serviceBlockHashIndex[bhKey] = uint64(blockNumber)

	// Update latest block
	if currentLatest, exists := m.latestRollupBlock[serviceID]; !exists || uint64(blockNumber) > currentLatest {
		m.latestRollupBlock[serviceID] = uint64(blockNumber)
	}

	// Persist EvmBlockPayload to LevelDB
	// Key: "sblock_{serviceID}_{blockNumber}"
	blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNumber)
	blockBytes, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %w", err)
	}
	if err := m.kv.Put([]byte(blockKey), blockBytes); err != nil {
		return fmt.Errorf("failed to store block: %w", err)
	}

	// Index transaction hashes for fast eth_getTransactionReceipt lookup
	// Key: "stx_{serviceID}_{txHash}" -> "blockNumber:txIndex" (8 bytes)
	for i, txHash := range block.TxHashes {
		txKey := fmt.Sprintf("stx_%d_%s", serviceID, txHash.Hex())
		txValue := make([]byte, 8)
		binary.LittleEndian.PutUint32(txValue[0:4], blockNumber)
		binary.LittleEndian.PutUint32(txValue[4:8], uint32(i))
		if err := m.kv.Put([]byte(txKey), txValue); err != nil {
			log.Warn(log.EVM, "Failed to index transaction hash", "txHash", txHash.Hex(), "err", err)
			// Continue - don't fail the whole block store for index failure
		}
	}

	return nil
}

// GetServiceBlock retrieves an EVM block by service ID and block number string.
func (m *ServiceStateManager) GetServiceBlock(serviceID uint32, blockNumber string) (*evmtypes.EvmBlockPayload, error) {
	m.mutex.RLock()
	latestBlock, exists := m.latestRollupBlock[serviceID]
	m.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no blocks for service %d", serviceID)
	}

	// Parse block number
	blockNum, err := m.parseBlockNumber(blockNumber, latestBlock)
	if err != nil {
		return nil, err
	}

	// Retrieve from LevelDB
	blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNum)
	blockBytes, found, err := m.kv.Get([]byte(blockKey))
	if err != nil {
		return nil, fmt.Errorf("block read error: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("block not found")
	}

	var block evmtypes.EvmBlockPayload
	if err := json.Unmarshal(blockBytes, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	return &block, nil
}

// --- Transaction Lookup ---

// GetTxLocation returns the block number and tx index for a transaction hash.
// This is a fast O(1) lookup using the tx hash index.
func (m *ServiceStateManager) GetTxLocation(serviceID uint32, txHash common.Hash) (blockNumber uint32, txIndex uint32, found bool) {
	txKey := fmt.Sprintf("stx_%d_%s", serviceID, txHash.Hex())
	txValue, txFound, err := m.kv.Get([]byte(txKey))
	if err != nil {
		log.Error(log.EVM, "GetTxLocation read error", "serviceID", serviceID, "txHash", txHash.Hex(), "err", err)
		return 0, 0, false
	}
	if !txFound || len(txValue) != 8 {
		return 0, 0, false
	}
	blockNumber = binary.LittleEndian.Uint32(txValue[0:4])
	txIndex = binary.LittleEndian.Uint32(txValue[4:8])
	return blockNumber, txIndex, true
}

// GetTransactionByHash finds a transaction in a service's block history.
// Uses tx hash index for O(1) lookup instead of scanning all blocks.
func (m *ServiceStateManager) GetTransactionByHash(serviceID uint32, txHash common.Hash) (*types.Transaction, *types.BlockMetadata, error) {
	// First, try the tx hash index (fast path)
	txKey := fmt.Sprintf("stx_%d_%s", serviceID, txHash.Hex())
	txValue, found, err := m.kv.Get([]byte(txKey))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read tx index: %w", err)
	}
	if found && len(txValue) == 8 {
		// Index hit - extract blockNumber and txIndex
		blockNum := binary.LittleEndian.Uint32(txValue[0:4])
		txIndex := binary.LittleEndian.Uint32(txValue[4:8])

		// Load the block to get block hash
		blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNum)
		blockBytes, blockFound, err := m.kv.Get([]byte(blockKey))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read block %d: %w", blockNum, err)
		}
		if blockFound {
			var block evmtypes.EvmBlockPayload
			if err := json.Unmarshal(blockBytes, &block); err == nil {
				metadata := &types.BlockMetadata{
					BlockHash:   block.WorkPackageHash,
					BlockNumber: blockNum,
					TxIndex:     txIndex,
				}
				tx := &types.Transaction{
					Hash: txHash,
				}
				return tx, metadata, nil
			}
		}
	}

	// Fallback: scan blocks (for blocks stored before index was added)
	m.mutex.RLock()
	latestBlock, exists := m.latestRollupBlock[serviceID]
	m.mutex.RUnlock()

	if !exists {
		return nil, nil, fmt.Errorf("no blocks for service %d", serviceID)
	}

	for blockNum := latestBlock; blockNum >= 0; blockNum-- {
		blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNum)
		blockBytes, found, err := m.kv.Get([]byte(blockKey))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read block %d during scan: %w", blockNum, err)
		}
		if !found {
			if blockNum == 0 {
				break
			}
			continue
		}

		var block evmtypes.EvmBlockPayload
		if err := json.Unmarshal(blockBytes, &block); err != nil {
			if blockNum == 0 {
				break
			}
			continue
		}

		// Search transaction hashes in this block
		for txIdx, hash := range block.TxHashes {
			if hash == txHash {
				metadata := &types.BlockMetadata{
					BlockHash:   block.WorkPackageHash,
					BlockNumber: block.Number,
					TxIndex:     uint32(txIdx),
				}
				tx := &types.Transaction{
					Hash: txHash,
				}
				return tx, metadata, nil
			}
		}

		if blockNum == 0 {
			break
		}
	}

	return nil, nil, fmt.Errorf("transaction not found")
}

// GetTransactionReceipt retrieves a full transaction receipt from stored block data.
// This is the primary method for eth_getTransactionReceipt when using builder/LevelDB as primary source.
// Returns the TransactionReceipt with all fields populated (Success, UsedGas, Logs, etc.)
// Returns nil if not found.
func (m *ServiceStateManager) GetTransactionReceipt(serviceID uint32, txHash common.Hash) (*evmtypes.TransactionReceipt, error) {
	// First, find the block containing this transaction
	blockNum, txIndex, found := m.GetTxLocation(serviceID, txHash)
	if !found {
		return nil, nil // Transaction not indexed
	}

	// Load the block
	blockKey := fmt.Sprintf("sblock_%d_%d", serviceID, blockNum)
	blockBytes, blockFound, err := m.kv.Get([]byte(blockKey))
	if err != nil {
		return nil, fmt.Errorf("block %d read error: %w", blockNum, err)
	}
	if !blockFound {
		return nil, fmt.Errorf("block %d not found", blockNum)
	}

	var block evmtypes.EvmBlockPayload
	if err := json.Unmarshal(blockBytes, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	// Check if receipt data is available
	if int(txIndex) >= len(block.Transactions) {
		// Receipt data not stored (old blocks or missing data)
		// Return minimal receipt with just the metadata
		return &evmtypes.TransactionReceipt{
			TransactionHash:  txHash,
			BlockHash:        block.WorkPackageHash,
			BlockNumber:      blockNum,
			TransactionIndex: txIndex,
			Timestamp:        block.Timestamp,
		}, nil
	}

	// Get the full receipt
	receipt := block.Transactions[txIndex]

	// Fill in block context if not already set
	if receipt.BlockHash == (common.Hash{}) {
		receipt.BlockHash = block.WorkPackageHash
	}
	if receipt.BlockNumber == 0 {
		receipt.BlockNumber = blockNum
	}
	if receipt.TransactionIndex == 0 && txIndex > 0 {
		receipt.TransactionIndex = txIndex
	}
	if receipt.Timestamp == 0 {
		receipt.Timestamp = block.Timestamp
	}

	return &receipt, nil
}

// --- Block Retrieval ---

// GetBlockByNumber retrieves full EVM block by service ID and block number string.
func (m *ServiceStateManager) GetBlockByNumber(serviceID uint32, blockNumber string) (*types.EVMBlock, error) {
	block, err := m.GetServiceBlock(serviceID, blockNumber)
	if err != nil {
		return nil, err
	}

	// Convert EvmBlockPayload to EVMBlock
	transactions := make([]types.Transaction, len(block.TxHashes))
	for i, txHash := range block.TxHashes {
		transactions[i] = types.Transaction{Hash: txHash}
	}

	receipts := make([]types.Receipt, len(block.Transactions))
	for i, txReceipt := range block.Transactions {
		receipts[i] = types.Receipt{
			TransactionHash: txReceipt.TransactionHash,
			Success:         txReceipt.Success,
			UsedGas:         txReceipt.UsedGas,
		}
	}

	evmBlock := &types.EVMBlock{
		BlockNumber:  block.Number,
		BlockHash:    block.WorkPackageHash,
		ParentHash:   common.Hash{}, // No parent hash in EvmBlockPayload
		Timestamp:    block.Timestamp,
		UBTRoot:      block.UBTRoot,
		Transactions: transactions,
		Receipts:     receipts,
	}

	return evmBlock, nil
}

// GetBlockByHash retrieves full EVM block by service ID and block hash.
func (m *ServiceStateManager) GetBlockByHash(serviceID uint32, blockHash common.Hash) (*types.EVMBlock, error) {
	blockNum, exists := m.GetBlockNumberByHash(serviceID, blockHash)
	if !exists {
		return nil, fmt.Errorf("block hash not found")
	}

	return m.GetBlockByNumber(serviceID, fmt.Sprintf("0x%x", blockNum))
}
