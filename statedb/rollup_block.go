package statedb

import (
	"encoding/binary"
	"fmt"
	"strconv"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/crypto/blake2b"
)

// Ethereum internal methods implementation on StateDB
const (
	// Hardhat Account #0 (Issuer/Alice) - First account from standard Hardhat/Anvil test mnemonic
	// "test test test test test test test test test test test junk"
	IssuerPrivateKeyHex = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	JamChainID          = 42 // JAM chain ID for testing
)

// EthereumBlock represents an Ethereum block for JSON-RPC responses (hex-encoded strings)
type EthereumBlock struct {
	Number           string      `json:"number"`
	Hash             string      `json:"hash"`
	ParentHash       string      `json:"parentHash"`
	Nonce            string      `json:"nonce"`
	Sha3Uncles       string      `json:"sha3Uncles"`
	LogsBloom        string      `json:"logsBloom"`
	TransactionsRoot string      `json:"transactionsRoot"`
	StateRoot        string      `json:"stateRoot"`
	ReceiptsRoot     string      `json:"receiptsRoot"`
	Miner            string      `json:"miner"`
	Difficulty       string      `json:"difficulty"`
	TotalDifficulty  string      `json:"totalDifficulty"`
	ExtraData        string      `json:"extraData"`
	Size             string      `json:"size"`
	GasLimit         string      `json:"gasLimit"`
	GasUsed          string      `json:"gasUsed"`
	Timestamp        string      `json:"timestamp"`
	Transactions     interface{} `json:"transactions"`
	Uncles           []string    `json:"uncles"`
}

// ToEthereumBlock converts EvmBlockPayload to JSON-RPC EthereumBlock format
func (p *EvmBlockPayload) ToEthereumBlock(fullTx bool) *EthereumBlock {
	blockHash := p.ComputeHash()

	ethBlock := &EthereumBlock{
		Number:           fmt.Sprintf("0x%x", p.Number),
		Hash:             blockHash.String(),
		ParentHash:       p.ParentHash.String(),
		Nonce:            "0x0000000000000000",
		Sha3Uncles:       "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
		LogsBloom:        common.Bytes2Hex(p.LogsBloom[:]),
		TransactionsRoot: p.TransactionsRoot.String(),
		StateRoot:        p.StateRoot.String(),
		ReceiptsRoot:     p.ReceiptsRoot.String(),
		Miner:            p.Miner.String(),
		Difficulty:       "0x0",
		TotalDifficulty:  "0x0",
		ExtraData:        common.Bytes2Hex(p.ExtraData[:]),
		Size:             fmt.Sprintf("0x%x", p.Size),
		GasLimit:         fmt.Sprintf("0x%x", p.GasLimit),
		GasUsed:          fmt.Sprintf("0x%x", p.GasUsed),
		Timestamp:        fmt.Sprintf("0x%x", p.Timestamp),
		Uncles:           []string{},
	}

	// Convert transaction hashes to strings (TxHashes = transaction hashes in submission order)
	if !fullTx {
		txHashes := make([]string, len(p.TxHashes))
		for i, txHash := range p.TxHashes {
			// Rust guarantees TxHashes are emitted in submission order (extrinsic index order),
			// so we preserve that ordering here for RPC compatibility.
			txHashes[i] = txHash.String()
		}
		ethBlock.Transactions = txHashes
	} else {
		// TODO: Fetch full transaction objects
		ethBlock.Transactions = []interface{}{}
	}

	return ethBlock
}

// EvmBlockPayload represents the unified EVM block structure exported to JAM DA
// This structure matches the Rust EvmBlockPayload exactly
type EvmBlockPayload struct {
	// Fixed fields (476 bytes) - used for block hash computation
	Number           uint64         // Offset 0, 8 bytes
	ParentHash       common.Hash    // Offset 8, 32 bytes
	LogsBloom        [256]byte      // Offset 40, 256 bytes
	TransactionsRoot common.Hash    // Offset 296, 32 bytes
	StateRoot        common.Hash    // Offset 328, 32 bytes
	ReceiptsRoot     common.Hash    // Offset 360, 32 bytes
	Miner            common.Address // Offset 392, 20 bytes
	ExtraData        [32]byte       // Offset 412, 32 bytes (JAM entropy)
	Size             uint64         // Offset 444, 8 bytes
	GasLimit         uint64         // Offset 452, 8 bytes
	GasUsed          uint64         // Offset 460, 8 bytes
	Timestamp        uint64         // Offset 468, 8 bytes
	// Total fixed: 476 bytes

	// Variable-length fields (not part of hash computation)
	TxHashes      []common.Hash // Transaction hashes (for transactions_root BMT and tx lookup)
	ReceiptHashes []common.Hash // Receipt hashes (for receipts_root BMT verification)
}

// SerializeEvmBlockPayload serializes an EvmBlockPayload to bytes
// Matches the Rust serialize() format exactly
func SerializeEvmBlockPayload(payload *EvmBlockPayload) []byte {
	// Calculate total size: 476 (fixed) + 4 (tx_count) + len(TxHashes)*32 + 4 (receipt_count) + len(ReceiptHashes)*32
	size := 476 + 4 + len(payload.TxHashes)*32 + 4 + len(payload.ReceiptHashes)*32
	data := make([]byte, size)
	offset := 0

	// Serialize fixed fields (476 bytes total)
	binary.LittleEndian.PutUint64(data[offset:offset+8], payload.Number)
	offset += 8

	copy(data[offset:offset+32], payload.ParentHash[:])
	offset += 32

	copy(data[offset:offset+256], payload.LogsBloom[:])
	offset += 256

	copy(data[offset:offset+32], payload.TransactionsRoot[:])
	offset += 32

	copy(data[offset:offset+32], payload.StateRoot[:])
	offset += 32

	copy(data[offset:offset+32], payload.ReceiptsRoot[:])
	offset += 32

	copy(data[offset:offset+20], payload.Miner[:])
	offset += 20

	copy(data[offset:offset+32], payload.ExtraData[:])
	offset += 32

	binary.LittleEndian.PutUint64(data[offset:offset+8], payload.Size)
	offset += 8

	binary.LittleEndian.PutUint64(data[offset:offset+8], payload.GasLimit)
	offset += 8

	binary.LittleEndian.PutUint64(data[offset:offset+8], payload.GasUsed)
	offset += 8

	binary.LittleEndian.PutUint64(data[offset:offset+8], payload.Timestamp)
	offset += 8

	// Serialize tx_hashes (transaction hashes for transactions_root BMT)
	binary.LittleEndian.PutUint32(data[offset:offset+4], uint32(len(payload.TxHashes)))
	offset += 4

	for _, txHash := range payload.TxHashes {
		copy(data[offset:offset+32], txHash[:])
		offset += 32
	}

	// Serialize receipt_hashes (canonical receipt RLP hashes for receipts_root BMT)
	binary.LittleEndian.PutUint32(data[offset:offset+4], uint32(len(payload.ReceiptHashes)))
	offset += 4

	for _, receiptHash := range payload.ReceiptHashes {
		copy(data[offset:offset+32], receiptHash[:])
		offset += 32
	}

	return data
}

// DeserializeEvmBlockPayload deserializes an EvmBlockPayload from bytes
// Matches the Rust serialize() format exactly
func DeserializeEvmBlockPayload(data []byte) (*EvmBlockPayload, error) {
	// Minimum size: 476 + 4 + 4 = 484 bytes (fixed fields + tx_count + receipt_count)
	if len(data) < 484 {
		return nil, fmt.Errorf("block payload too short: got %d bytes, need at least 484", len(data))
	}

	offset := 0
	payload := &EvmBlockPayload{}

	// Parse fixed fields (476 bytes total)
	payload.Number = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	copy(payload.ParentHash[:], data[offset:offset+32])
	offset += 32

	copy(payload.LogsBloom[:], data[offset:offset+256])
	offset += 256

	copy(payload.TransactionsRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.StateRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.ReceiptsRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.Miner[:], data[offset:offset+20])
	offset += 20

	copy(payload.ExtraData[:], data[offset:offset+32])
	offset += 32

	payload.Size = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	payload.GasLimit = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	payload.GasUsed = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	payload.Timestamp = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Parse tx_hashes (transaction hashes for transactions_root BMT)
	txCount := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	payload.TxHashes = make([]common.Hash, txCount)
	for i := uint32(0); i < txCount; i++ {
		if offset+32 > len(data) {
			return nil, fmt.Errorf("insufficient data for tx_hashes at index %d", i)
		}
		copy(payload.TxHashes[i][:], data[offset:offset+32])
		offset += 32
	}

	// Parse receipt_hashes (canonical receipt RLP hashes for receipts_root BMT)
	receiptCount := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	payload.ReceiptHashes = make([]common.Hash, receiptCount)
	for i := uint32(0); i < receiptCount; i++ {
		if offset+32 > len(data) {
			return nil, fmt.Errorf("insufficient data for receipt_hashes at index %d", i)
		}
		copy(payload.ReceiptHashes[i][:], data[offset:offset+32])
		offset += 32
	}

	return payload, nil
}

// ComputeHash computes the Blake2b-256 hash of the fixed header fields (476 bytes)
// This hash becomes the block's Object ID in JAM DA
// Matches the Rust compute_hash() implementation exactly
func (p *EvmBlockPayload) ComputeHash() common.Hash {
	hasher, _ := blake2b.New256(nil)

	// Hash all fixed fields in order (same as Rust)
	binary.Write(hasher, binary.LittleEndian, p.Number)
	hasher.Write(p.ParentHash[:])
	hasher.Write(p.LogsBloom[:])
	hasher.Write(p.TransactionsRoot[:])
	hasher.Write(p.StateRoot[:])
	hasher.Write(p.ReceiptsRoot[:])
	hasher.Write(p.Miner[:])
	hasher.Write(p.ExtraData[:])
	binary.Write(hasher, binary.LittleEndian, p.Size)
	binary.Write(hasher, binary.LittleEndian, p.GasLimit)
	binary.Write(hasher, binary.LittleEndian, p.GasUsed)
	binary.Write(hasher, binary.LittleEndian, p.Timestamp)

	var hash common.Hash
	copy(hash[:], hasher.Sum(nil))
	return hash
}

// String returns a JSON representation of the EvmBlockPayload
func (p *EvmBlockPayload) String() string {
	return types.ToJSON(p)
}

// BlockNumberToObjectID converts block number to 32-byte ObjectID (matches Rust implementation)
// Rust: key = [0xFF; 32] with block_number in last 4 bytes (little-endian)
func BlockNumberToObjectID(blockNumber uint32) common.Hash {
	var key [32]byte
	for i := range key {
		key[i] = 0xFF
	}
	binary.LittleEndian.PutUint32(key[28:32], blockNumber)
	return common.BytesToHash(key[:])
}

// blockNumberToObjectID is the internal version (lowercase for package-local use)
func blockNumberToObjectID(blockNumber uint32) common.Hash {
	return BlockNumberToObjectID(blockNumber)
}

// serializeBlockNumber serializes block number and parent hash for BLOCK_NUMBER_KEY storage
// BLOCK_NUMBER_KEY = 0xFF..FF => block_number (4 bytes LE) || parent_hash (32 bytes)
func serializeBlockNumber(blockNumber uint32, parentHash common.Hash) []byte {
	value := make([]byte, 36)
	binary.LittleEndian.PutUint32(value[0:4], blockNumber)
	copy(value[4:36], parentHash[:])
	return value
}

// GetBlockNumberKey returns the storage key for BLOCK_NUMBER_KEY (0xFF repeated 32 times)
func GetBlockNumberKey() common.Hash {
	var key common.Hash
	for i := range key {
		key[i] = 0xFF
	}
	return key
}

// getBlockNumberKey is the internal version (lowercase for package-local use)
func getBlockNumberKey() common.Hash {
	return GetBlockNumberKey()
}

// IsBlockObjectID checks if an ObjectID is a Block object (0xFF×28 + block_number in last 4 bytes)
// Returns (isBlockObject, blockNumber)
//
// Note: Block number 0xFFFFFFFF is reserved for BLOCK_NUMBER_KEY (all 0xFF bytes).
// This is the sentinel value used in genesis that overflows to 0 on the first increment.
// Therefore, objectID with all 0xFF bytes is NOT a block object.
func IsBlockObjectID(objectID common.Hash) (bool, uint32) {
	// Check if BLOCK_NUMBER_KEY (all 0xFF) - this is the sentinel, not a block
	if objectID == GetBlockNumberKey() {
		return false, 0
	}

	// Check if first 28 bytes are 0xFF
	for i := 0; i < 28; i++ {
		if objectID[i] != 0xFF {
			return false, 0
		}
	}

	// Extract block number from last 4 bytes
	// Note: This can be any value 0x00000000 to 0xFFFFFFFE
	// (0xFFFFFFFF would make objectID == BLOCK_NUMBER_KEY, which we already excluded)
	blockNumber := binary.LittleEndian.Uint32(objectID[28:32])
	return true, blockNumber
}

// GetChainId returns the chain ID for the current network

func (n *StateDB) GetChainId() uint64 {
	return uint64(EVMServiceCode)
}

// GetAccounts returns the list of addresses owned by the client
func (n *StateDB) GetAccounts() []common.Address {
	// Node does not manage accounts - users should use wallets like MetaMask
	return []common.Address{}
}

// GetGasPrice returns the current gas price in wei
func (n *StateDB) GetGasPrice() uint64 {
	// TODO: Implement dynamic gas pricing based on network conditions
	// For now return fixed 1 gwei
	return 1000000000 // 1 gwei in wei
}

// GetLatestBlockNumber reads the current block number from EVM service storage (public interface)
func (n *StateDB) GetLatestBlockNumber(serviceID uint32) (uint32, error) {
	return n.GetLatestBlockNumberForService(serviceID)
}

// GetLatestBlockNumberForService reads the current block number from specified service storage
func (n *StateDB) GetLatestBlockNumberForService(serviceID uint32) (uint32, error) {
	return n.getLatestBlockNumber(serviceID)
}

// getLatestBlockNumber reads the current block number from service storage (internal)
func (n *StateDB) getLatestBlockNumber(serviceID uint32) (uint32, error) {
	// Use same key as Rust: BLOCK_NUMBER_KEY = 0xFF repeated 32 times
	// Rust stores: block_number (4 bytes LE) + parent_hash (32 bytes) = 36 bytes total
	key := make([]byte, 32)
	for i := range key {
		key[i] = 0xFF
	}

	valueBytes, found, err := n.ReadServiceStorage(serviceID, key)
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

// readBlockByNumber reads block using blockNumberToObjectID key from JAM State or from JAM DA if its archived there
func (n *StateDB) ReadBlockByNumber(serviceID uint32, blockNumber uint32) (*EvmBlockPayload, error) {
	// use ReadStateWitnessRaw to read block from JAM State by mapping the blockNumber to an objectID
	objectID := blockNumberToObjectID(blockNumber)
	witness, found, _, err := n.ReadStateWitnessRaw(serviceID, objectID)
	if err != nil {
		return nil, fmt.Errorf("failed to read block for number %d: %v", blockNumber, err)
	}
	if !found {
		return nil, fmt.Errorf("block %d not found", blockNumber)
	}

	// if the payloadlength is 64 bytes, then it's an ObjectRef
	if len(witness.Payload) == 64 {
		// Read ObjectRef directly
		witness, found, err := n.ReadStateWitnessRef(serviceID, objectID, true)
		if err != nil {
			return nil, fmt.Errorf("failed to read object ref for block %d: %v", blockNumber, err)
		} else if !found {
			return nil, fmt.Errorf("object ref for block %d not found", blockNumber)
		}
		payload := witness.Payload
		block, err := DeserializeEvmBlockPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize block ref for block %d: %v", blockNumber, err)
		}
		return block, nil
	}
	block, err := DeserializeEvmBlockPayload(witness.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize block ref for block %d: %v", blockNumber, err)
	}
	return block, nil
}

func (n *StateDB) readBlockByHash(serviceID uint32, blockHash common.Hash) (*EvmBlockPayload, error) {
	// First, try to read the block_hash → block_number mapping from storage
	blockNumberBytes, found, err := n.ReadServiceStorage(serviceID, blockHash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block hash mapping: %v", err)
	}

	if found && len(blockNumberBytes) >= 4 {
		// Parse block number (4 bytes, little-endian)
		blockNumber := binary.LittleEndian.Uint32(blockNumberBytes[:4])

		log.Info(log.Node, fmt.Sprintf("readBlockByHash: Found block number from hash mapping. Doing n.readBlockByNumber(%d)", blockNumber), "blockHash", blockHash.Hex(), "blockNumber", blockNumber)

		// Read the block by number
		return n.ReadBlockByNumber(serviceID, blockNumber)
	}

	// Fallback: Mapping doesn't exist (old blocks before migration)
	// Search recent blocks to find the matching hash
	log.Warn(log.Node, "readBlockByHash: Hash mapping not found, falling back to search",
		"blockHash", blockHash.Hex())

	latestBlock, err := n.getLatestBlockNumber(serviceID)
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
		evmBlock, err := n.ReadBlockByNumber(serviceID, blockNum)
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
	}

	return nil, fmt.Errorf("block not found: %s", blockHash.Hex())
}

func (n *StateDB) GetBlockByHash(serviceID uint32, blockHash common.Hash, fullTx bool) (*EthereumBlock, error) {
	// Use new canonical path: fetch EvmBlockPayload from DA via ReadStateWitness
	evmBlock, err := n.readBlockByHash(serviceID, blockHash)
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
			ethTx, err := n.GetTransactionByHashFormatted(serviceID, txHash)
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
func (n *StateDB) GetTransactionByBlockHashAndIndex(serviceID uint32, blockHash common.Hash, index uint32) (*EthereumTransactionResponse, error) {
	// First, get the block to retrieve transaction hashes
	block, err := n.GetBlockByHash(serviceID, blockHash, false)
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
	return n.GetTransactionByHashFormatted(serviceID, txHash)
}

// GetTransactionByBlockNumberAndIndex fetches a transaction by block number and transaction index
func (n *StateDB) GetTransactionByBlockNumberAndIndex(serviceID uint32, blockNumberStr string, index uint32) (*EthereumTransactionResponse, error) {
	// First, get the block to retrieve transaction hashes
	block, err := n.GetBlockByNumber(serviceID, blockNumberStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %v", err)
	}
	if block == nil {
		return nil, nil // Block not found
	}

	// Check if index is in range
	if index >= uint32(len(block.TxHashes)) {
		return nil, nil // Index out of range
	}

	// Get the transaction hash at the specified index
	txHash := block.TxHashes[index]

	// Fetch the full transaction
	return n.GetTransactionByHashFormatted(serviceID, txHash)
}

// GetBlockByNumber fetches a block by number and returns raw EvmBlockPayload
func (n *StateDB) GetBlockByNumber(serviceID uint32, blockNumberStr string) (*EvmBlockPayload, error) {
	// 1. Parse and resolve the block number
	var targetBlockNumber uint32
	var err error

	switch blockNumberStr {
	case "latest":
		targetBlockNumber, err = n.getLatestBlockNumber(serviceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
	case "earliest":
		targetBlockNumber = 1 // Genesis block
	case "pending":
		// For pending, return latest for now
		targetBlockNumber, err = n.getLatestBlockNumber(serviceID)
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

	// 2. Read canonical block metadata from storage
	return n.ReadBlockByNumber(serviceID, targetBlockNumber)
}

// GetBlockByNumberFormatted fetches a block and returns it in Ethereum JSON-RPC format
func (n *StateDB) GetBlockByNumberFormatted(serviceID uint32, blockNumberStr string, fullTx bool) (*EthereumBlock, error) {
	// Read the block
	evmBlock, err := n.GetBlockByNumber(serviceID, blockNumberStr)
	if err != nil {
		return nil, err
	}

	// Convert EvmBlockPayload to Ethereum JSON-RPC format
	ethBlock := evmBlock.ToEthereumBlock(fullTx)

	// If fullTx requested, fetch full transaction objects
	if fullTx {
		transactions := make([]interface{}, 0, len(evmBlock.TxHashes))
		blockHashStr := evmBlock.ComputeHash().String()
		blockNumberStrHex := fmt.Sprintf("0x%x", evmBlock.Number)

		for i, txHash := range evmBlock.TxHashes {
			ethTx, err := n.GetTransactionByHashFormatted(serviceID, txHash)
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
