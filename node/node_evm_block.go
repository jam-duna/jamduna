package node

import (
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
)

// Ethereum internal methods implementation on NodeContent
const (
	// Chain ID for JAM testnet
	jamChainID = 0x1107

	// Hardhat Account #0 (Issuer/Alice) - First account from standard Hardhat/Anvil test mnemonic
	// "test test test test test test test test test test test junk"
	issuerPrivateKeyHex = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
)

// GetChainId returns the chain ID for the current network
func (n *NodeContent) GetChainId() uint64 {
	// For now return JAM testnet chain ID: 0x1107 = 4359
	return jamChainID
}

// GetAccounts returns the list of addresses owned by the client
func (n *NodeContent) GetAccounts() []common.Address {
	// Node does not manage accounts - users should use wallets like MetaMask
	return []common.Address{}
}

// GetGasPrice returns the current gas price in wei
func (n *NodeContent) GetGasPrice() uint64 {
	// TODO: Implement dynamic gas pricing based on network conditions
	// For now return fixed 1 gwei
	return 1000000000 // 1 gwei in wei
}

// GetLatestBlockNumber reads the current block number from EVM service storage (public interface)
func (n *NodeContent) GetLatestBlockNumber() (uint32, error) {
	return n.getLatestBlockNumber()
}

// getLatestBlockNumber reads the current block number from EVM service storage (internal)
func (n *NodeContent) getLatestBlockNumber() (uint32, error) {
	// Use same key as Rust: BLOCK_NUMBER_KEY = 0xFF repeated 32 times
	// Rust stores: block_number (4 bytes LE) + parent_hash (32 bytes) = 36 bytes total
	key := make([]byte, 32)
	for i := range key {
		key[i] = 0xFF
	}

	valueBytes, found, err := n.statedb.ReadServiceStorage(statedb.EVMServiceCode, key)
	if err != nil {
		return 0, fmt.Errorf("failed to read block number from storage: %v", err)
	}
	if !found || len(valueBytes) < 36 {
		return 0, nil // Genesis state (block 0)
	}

	// Parse block_number (first 4 bytes, little-endian)
	blockNumber := binary.LittleEndian.Uint32(valueBytes[:4])
	// parent_hash is at valueBytes[4:36] but we don't need it for this function

	return blockNumber, nil
}

// resolveBlockNumberToState resolves a block number string to a stateDB
// resolveBlockNumberToState resolves a block number string to a StateDB
// For historical blocks, reconstructs state from the block's state root
func (n *NodeContent) resolveBlockNumberToState(blockNumberStr string) (*statedb.StateDB, error) {
	switch blockNumberStr {
	case "latest", "pending":
		// Use current state for latest/pending
		return n.statedb, nil

	case "earliest":
		// Load genesis state (block 1)
		return n.getHistoricalState(1)

	default:
		// Parse hex block number
		if len(blockNumberStr) >= 2 && blockNumberStr[:2] == "0x" {
			blockNum, err := strconv.ParseUint(blockNumberStr[2:], 16, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid block number format: %v", err)
			}
			return n.getHistoricalState(uint32(blockNum))
		}

		return nil, fmt.Errorf("invalid block number format: %s", blockNumberStr)
	}
}

// getHistoricalState reconstructs StateDB from a block's state root
func (n *NodeContent) getHistoricalState(blockNumber uint32) (*statedb.StateDB, error) {
	// Read block metadata to get state root
	evmBlock, err := n.readBlockByNumber(blockNumber)
	if err != nil {
		log.Warn(log.Node, "Failed to read block metadata, using current state",
			"blockNumber", blockNumber, "error", err)
		return n.statedb, nil
	}

	// Reconstruct StateDB from the block's state root
	historicalState, err := statedb.NewStateDBFromStateRoot(evmBlock.StateRoot, n.statedb.GetStorage())
	if err != nil {
		log.Warn(log.Node, "Failed to reconstruct historical state, using current state",
			"blockNumber", blockNumber, "stateRoot", evmBlock.StateRoot.Hex(), "error", err)
		return n.statedb, nil
	}

	return historicalState, nil
}

// readBlockByNumber reads block using blockNumberToObjectID key from JAM State or from JAM DA if its archived there
func (n *NodeContent) readBlockByNumber(blockNumber uint32) (*statedb.EvmBlockPayload, error) {
	// use ReadStateWitnessRaw to read block from JAM State by mapping the blockNumber to an objectID
	objectID := blockNumberToObjectID(blockNumber)
	witness, found, _, err := n.statedb.ReadStateWitnessRaw(statedb.EVMServiceCode, objectID)
	if err != nil {
		return nil, fmt.Errorf("failed to read block for number %d: %v", blockNumber, err)
	}
	if !found {
		return nil, fmt.Errorf("block %d not found", blockNumber)
	}

	// if the payloadlength is 64 bytes, then it's an ObjectRef
	if len(witness.Payload) == 64 {
		// Read ObjectRef directly
		witness, found, err := n.statedb.ReadStateWitnessRef(statedb.EVMServiceCode, objectID, true)
		if err != nil {
			return nil, fmt.Errorf("failed to read object ref for block %d: %v", blockNumber, err)
		} else if !found {
			return nil, fmt.Errorf("object ref for block %d not found", blockNumber)
		}
		payload := witness.Payload
		block, err := statedb.DeserializeEvmBlockPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize block ref for block %d: %v", blockNumber, err)
		}
		return block, nil
	}
	block, err := statedb.DeserializeEvmBlockPayload(witness.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize block ref for block %d: %v", blockNumber, err)
	}
	return block, nil
}

func (n *NodeContent) readBlockByHash(blockHash common.Hash) (*statedb.EvmBlockPayload, error) {
	// First, try to read the block_hash → block_number mapping from storage
	blockNumberBytes, found, err := n.statedb.ReadServiceStorage(statedb.EVMServiceCode, blockHash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block hash mapping: %v", err)
	}

	if found && len(blockNumberBytes) >= 4 {
		// Parse block number (4 bytes, little-endian)
		blockNumber := binary.LittleEndian.Uint32(blockNumberBytes[:4])

		log.Info(log.Node, fmt.Sprintf("readBlockByHash: Found block number from hash mapping. Doing n.readBlockByNumber(%d)", blockNumber), "blockHash", blockHash.Hex(), "blockNumber", blockNumber)

		// Read the block by number
		return n.readBlockByNumber(blockNumber)
	}

	// Fallback: Mapping doesn't exist (old blocks before migration)
	// Search recent blocks to find the matching hash
	log.Warn(log.Node, "readBlockByHash: Hash mapping not found, falling back to search",
		"blockHash", blockHash.Hex())

	latestBlock, err := n.getLatestBlockNumber()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest block number: %v", err)
	}

	// Search last 100 blocks
	const maxSearchDepth = 100
	startBlock := uint32(1)
	if latestBlock > maxSearchDepth {
		startBlock = latestBlock - maxSearchDepth
	}

	for blockNum := latestBlock; blockNum >= startBlock; blockNum-- {
		evmBlock, err := n.readBlockByNumber(blockNum)
		if err != nil {
			if blockNum == 1 {
				break
			}
			continue
		}

		computedHash := evmBlock.ComputeHash()
		if computedHash == blockHash {
			log.Info(log.Node, "readBlockByHash: Found block via FALLBACK search", "blockHash", blockHash.Hex(), "blockNumber", blockNum)
			return evmBlock, nil
		}

		if blockNum == 1 {
			break
		}
		payload := witness.Payload
		block, err := statedb.DeserializeEvmBlockPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize block ref for block hash %s: %v", blockHash, err)
		}
		return block, nil
	}
	block, err := statedb.DeserializeEvmBlockPayload(witness.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize block ref for block hash %s: %v", blockHash, err)
	}
	return block, nil
}

// blockNumberToObjectID converts block number to 32-byte ObjectID (matches Rust implementation)
// Rust: key = [0xFF; 32] with block_number in last 4 bytes (little-endian)
func blockNumberToObjectID(blockNumber uint32) common.Hash {
	var key [32]byte
	for i := range key {
		key[i] = 0xFF
	}
	binary.LittleEndian.PutUint32(key[28:32], blockNumber)
	return common.BytesToHash(key[:])
}

func (n *NodeContent) GetBlockByHash(blockHash common.Hash, fullTx bool) (*statedb.EthereumBlock, error) {
	// Use new canonical path: fetch EvmBlockPayload from DA via ReadStateWitness
	evmBlock, err := n.readBlockByHash(blockHash)
	if err != nil {
		// Block not found or error reading from DA
		return nil, err
	}

	return nil, fmt.Errorf("block not found: %s", blockHash.Hex())
}

// blockNumberToObjectID converts block number to 32-byte ObjectID (matches Rust implementation)
// Rust: key = [0xFF; 32] with block_number in last 4 bytes (little-endian)
func blockNumberToObjectID(blockNumber uint32) common.Hash {
	var key [32]byte
	for i := range key {
		key[i] = 0xFF
	}
	binary.LittleEndian.PutUint32(key[28:32], blockNumber)
	return common.BytesToHash(key[:])
}

func (n *NodeContent) GetBlockByHash(blockHash common.Hash, fullTx bool) (*statedb.EthereumBlock, error) {
	// Use new canonical path: fetch EvmBlockPayload from DA via ReadStateWitness
	evmBlock, err := n.readBlockByHash(blockHash)
	if err != nil {
		// Block not found or error reading from DA
		return nil, err
	}

	// Convert EvmBlockPayload to Ethereum JSON-RPC format
	ethBlock := evmBlock.ToEthereumBlock(fullTx)

	// If fullTx requested, fetch full transaction objects
	if fullTx {
		transactions := make([]interface{}, 0, len(evmBlock.TxHashes))
		blockHashStr := blockHash.String()
		blockNumberStr := fmt.Sprintf("0x%x", evmBlock.Number)

		for i, txHash := range evmBlock.TxHashes {
			ethTx, err := n.GetTransactionByHash(txHash)
			if err != nil {
				log.Warn(log.Node, "GetBlockByHash: Failed to get transaction",
					"txHash", txHash.String(), "error", err)
				continue
			}
			if ethTx != nil {
				// Set block context
				ethTx.BlockHash = &blockHashStr
				ethTx.BlockNumber = &blockNumberStr
				txIndex := fmt.Sprintf("0x%x", i)
				ethTx.TransactionIndex = &txIndex

				transactions = append(transactions, ethTx)
			}
		}

		ethBlock.Transactions = transactions
	}

	return ethBlock, nil
}

// GetTransactionByBlockHashAndIndex fetches a transaction by block hash and transaction index
func (n *NodeContent) GetTransactionByBlockHashAndIndex(blockHash common.Hash, index uint32) (*EthereumTransactionResponse, error) {
	// First, get the block to retrieve transaction hashes
	block, err := n.GetBlockByHash(blockHash, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %v", err)
	}
	if block == nil {
		return nil, nil // Block not found
	}

	// Extract transaction hashes from block
	txHashes, ok := block.Transactions.([]string)
	if !ok {
		return nil, fmt.Errorf("unexpected transaction format in block")
	}

	// Check if index is in range
	if index >= uint32(len(txHashes)) {
		return nil, nil // Index out of range
	}

	// Get the transaction hash at the specified index
	txHash := common.HexToHash(txHashes[index])

	// Fetch the full transaction
	return n.GetTransactionByHash(txHash)
}

// GetTransactionByBlockNumberAndIndex fetches a transaction by block number and transaction index
func (n *NodeContent) GetTransactionByBlockNumberAndIndex(blockNumberStr string, index uint32) (*EthereumTransactionResponse, error) {
	// First, get the block to retrieve transaction hashes
	block, err := n.GetBlockByNumber(blockNumberStr, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %v", err)
	}
	if block == nil {
		return nil, nil // Block not found
	}

	// Extract transaction hashes from block
	txHashes, ok := block.Transactions.([]string)
	if !ok {
		return nil, fmt.Errorf("unexpected transaction format in block")
	}

	// Check if index is in range
	if index >= uint32(len(txHashes)) {
		return nil, nil // Index out of range
	}

	// Get the transaction hash at the specified index
	txHash := common.HexToHash(txHashes[index])

	// Fetch the full transaction
	return n.GetTransactionByHash(txHash)
}

// GetBlockByNumber fetches a block by number
func (n *NodeContent) GetBlockByNumber(blockNumberStr string, fullTx bool) (*statedb.EthereumBlock, error) {
	// 1. Parse and resolve the block number
	var targetBlockNumber uint32
	var err error

	switch blockNumberStr {
	case "latest":
		targetBlockNumber, err = n.getLatestBlockNumber()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
	case "earliest":
		targetBlockNumber = 1 // Genesis block
	case "pending":
		// For pending, return latest for now
		targetBlockNumber, err = n.getLatestBlockNumber()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
	default:
		// Parse hex block number
		if len(blockNumberStr) >= 2 && blockNumberStr[:2] == "0x" {
			blockNum, parseErr := strconv.ParseUint(blockNumberStr[2:], 16, 32)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid block number format: %v", parseErr)
			}
			targetBlockNumber = uint32(blockNum)
		} else {
			return nil, fmt.Errorf("invalid block number format: %s", blockNumberStr)
		}
	}

	// 2. Read canonical block metadata from storage (new path)
	evmBlock, err := n.readBlockByNumber(targetBlockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to read block metadata: %v", err)
	}

	// 3. Convert EvmBlockPayload to Ethereum JSON-RPC format
	ethBlock := evmBlock.ToEthereumBlock(fullTx)

	// 4. If fullTx requested, fetch full transaction objects
	if fullTx {
		transactions := make([]interface{}, 0, len(evmBlock.TxHashes))
		blockHashStr := evmBlock.ComputeHash().String()
		blockNumberStrHex := fmt.Sprintf("0x%x", evmBlock.Number)

		for i, txHash := range evmBlock.TxHashes {
			ethTx, err := n.GetTransactionByHash(txHash)
			if err != nil {
				log.Warn(log.Node, "GetBlockByNumber: Failed to get transaction",
					"txHash", txHash.String(), "error", err)
				continue
			}
			if ethTx != nil {
				// Set block context
				ethTx.BlockHash = &blockHashStr
				ethTx.BlockNumber = &blockNumberStrHex
				txIndex := fmt.Sprintf("0x%x", i)
				ethTx.TransactionIndex = &txIndex

				transactions = append(transactions, ethTx)
			}
		}

		ethBlock.Transactions = transactions
	}

	return ethBlock, nil
}
