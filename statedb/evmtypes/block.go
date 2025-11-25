package evmtypes

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

const (
	// HEADER_SIZE is the size of the block header in bytes (matching Rust implementation)
	// 4 (payload_length) + 4 (num_transactions) + 4 (timestamp) +
	// 8 (gas_used) + 32 (state_root) + 32 (transactions_root) + 32 (receipt_root) = 116 bytes
	HEADER_SIZE = 116
)

// EvmBlockPayload represents the unified EVM block structure exported to JAM DA
// This structure matches the Rust EvmBlockPayload exactly
type EvmBlockPayload struct {
	// Non-serialized fields (filled by host when reading from DA)
	Number uint32      // Block number read from blocknumber↔hash mapping
	Hash   common.Hash // Block Hash is the workpackagehash

	// Fixed fields - used for block hash computation (116 bytes)
	PayloadLength    uint32      // Offset 0, 4 bytes - raw payload size
	NumTransactions  uint32      // Offset 4, 4 bytes - transaction count
	Timestamp        uint32      // Offset 8, 4 bytes - JAM timeslot
	GasUsed          uint64      // Offset 12, 8 bytes - gas used
	StateRoot        common.Hash // Offset 20, 32 bytes - JAM state root
	TransactionsRoot common.Hash // Offset 52, 32 bytes - BMT root of tx hashes
	ReceiptRoot      common.Hash // Offset 84, 32 bytes - BMT root of canonical receipts
	// Total fixed: 116 bytes

	// Variable-length fields (not part of hash computation)
	TxHashes      []common.Hash        // Transaction hashes (32 bytes each)
	ReceiptHashes []common.Hash        // Receipt hashes (32 bytes each)
	Transactions  []TransactionReceipt `json:"-"`
}

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
	Transactions     interface{} `json:"transactions"` // Can be []string (hashes) or []EthereumTransactionResponse
	Uncles           []string    `json:"uncles"`
	BaseFeePerGas    *string     `json:"baseFeePerGas,omitempty"` // EIP-1559
}

// ToEthereumBlock converts EvmBlockPayload to JSON-RPC EthereumBlock format
// Requires the canonical blockNumber resolved from BLOCK_NUMBER_KEY / DA mapping
func (p *EvmBlockPayload) ToEthereumBlock(blockNumber uint32, fullTx bool) *EthereumBlock {

	ethBlock := &EthereumBlock{
		Number:           fmt.Sprintf("0x%x", blockNumber),
		Hash:             p.Hash.String(),
		ParentHash:       "0x0000000000000000000000000000000000000000000000000000000000000000", // No parent hash in new format
		Nonce:            "0x0000000000000000",
		Sha3Uncles:       "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
		LogsBloom:        common.Bytes2Hex(make([]byte, 256)), // Zero bloom
		TransactionsRoot: p.TransactionsRoot.String(),
		StateRoot:        p.StateRoot.String(),
		ReceiptsRoot:     p.ReceiptRoot.String(),
		Miner:            "0x0000000000000000000000000000000000000000",
		Difficulty:       "0x0",
		TotalDifficulty:  "0x0",
		ExtraData:        "0x",
		Size:             fmt.Sprintf("0x%x", p.PayloadLength),
		GasLimit:         "0x0", // No gas limit in new format
		GasUsed:          fmt.Sprintf("0x%x", p.GasUsed),
		Timestamp:        fmt.Sprintf("0x%x", p.Timestamp),
		Uncles:           []string{},
	}

	// Convert transaction hashes to strings
	if !fullTx {
		txHashes := make([]string, len(p.TxHashes))
		for i, txHash := range p.TxHashes {
			txHashes[i] = txHash.String()
		}
		ethBlock.Transactions = txHashes
	} else {
		// TODO: Fetch full transaction objects
		ethBlock.Transactions = []interface{}{}
	}

	return ethBlock
}

// SerializeEvmBlockPayload serializes an EvmBlockPayload to bytes
// Matches the Rust serialize() format exactly
func SerializeEvmBlockPayload(payload *EvmBlockPayload) []byte {
	// Calculate total size: 120 (fixed) + 4 (tx_count) + len(TxHashes)*32 + 4 (receipt_count) + len(ReceiptHashes)*32
	size := 116 + 4 + len(payload.TxHashes)*32 + 4 + len(payload.ReceiptHashes)*32
	data := make([]byte, size)
	offset := 0

	// Serialize fixed header (116 bytes total)
	binary.LittleEndian.PutUint32(data[offset:offset+4], payload.PayloadLength)
	offset += 4

	binary.LittleEndian.PutUint32(data[offset:offset+4], payload.NumTransactions)
	offset += 4

	binary.LittleEndian.PutUint32(data[offset:offset+4], payload.Timestamp)
	offset += 4

	binary.LittleEndian.PutUint64(data[offset:offset+8], payload.GasUsed)
	offset += 8

	copy(data[offset:offset+32], payload.StateRoot[:])
	offset += 32

	copy(data[offset:offset+32], payload.TransactionsRoot[:])
	offset += 32

	copy(data[offset:offset+32], payload.ReceiptRoot[:])
	offset += 32

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
func DeserializeEvmBlockPayload(data []byte, headerOnly bool) (*EvmBlockPayload, error) {
	// Minimum size: 116 + 4 + 4 = 124 bytes (fixed fields + tx_count + receipt_count)
	if len(data) < 124 {
		return nil, fmt.Errorf("block payload too short: got %d bytes, need at least 124", len(data))
	}

	offset := 0
	payload := &EvmBlockPayload{}

	// Parse fixed header (116 bytes total)
	payload.PayloadLength = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	payload.NumTransactions = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	payload.Timestamp = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	payload.GasUsed = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	copy(payload.StateRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.TransactionsRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.ReceiptRoot[:], data[offset:offset+32])
	offset += 32

	if headerOnly {
		// Skip variable-length fields
		return payload, nil
	}
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

// String returns a JSON representation of the EvmBlockPayload
func (p *EvmBlockPayload) String() string {
	return types.ToJSON(p)
}

// VerifyBlockBMTProofs verifies that the BMT roots in block metadata are correctly computed
func VerifyBlockBMTProofs(block *EvmBlockPayload) error {
	// Verify Transactions Root
	if len(block.TxHashes) > 0 {
		computedTxRoot := ComputeBMTRootFromHashes(block.TxHashes)
		if computedTxRoot != block.TransactionsRoot {
			return fmt.Errorf("TransactionsRoot mismatch: computed=%s, stored=%s",
				computedTxRoot.String(), block.TransactionsRoot.String())
		}
		log.Info(log.Node, "✅ TransactionsRoot verified", "transactions_root", block.TransactionsRoot.String())
	}

	// Verify Receipts Root
	if len(block.ReceiptHashes) > 0 {
		computedReceiptRoot := ComputeBMTRootFromHashes(block.ReceiptHashes)
		if computedReceiptRoot != block.ReceiptRoot {
			return fmt.Errorf("ReceiptsRoot mismatch: computed=%s, stored=%s",
				computedReceiptRoot.String(), block.ReceiptRoot.String())
		}
		log.Info(log.Node, "✅ ReceiptsRoot verified", "receipts_root", block.ReceiptRoot.String())
	}

	return nil
}

// ComputeBMTRootFromHashes computes the BMT root from a list of hashes indexed by position
// and optionally verifies each proof path when debugBMTProofs is enabled
func ComputeBMTRootFromHashes(hashes []common.Hash) common.Hash {
	if len(hashes) == 0 {
		// Empty BMT root - use blake2b of empty bytes
		return common.Blake2Hash([]byte{})
	}

	kvPairs := make([][2][]byte, len(hashes))
	for i, hash := range hashes {
		var key [32]byte
		// Use little-endian encoding at the start of the key (natural position)
		binary.LittleEndian.PutUint32(key[0:4], uint32(i))

		kvPairs[i] = [2][]byte{key[:], hash[:]}
	}

	tree := trie.NewMerkleTree(kvPairs)
	root := tree.GetRoot()

	numFailures := 0
	for i, hash := range hashes {
		var key [32]byte
		binary.LittleEndian.PutUint32(key[0:4], uint32(i))

		rawProof, err := tree.Trace(key[:])
		if err != nil {
			log.Error(log.Node, "❌ Failed to trace path", "index", i, "key", common.BytesToHash(key[:]).String(), "err", err)
			numFailures++
			continue
		}

		proofPath := make([]common.Hash, len(rawProof))
		for j, p := range rawProof {
			proofPath[j] = common.BytesToHash(p)
		}
		serviceID := uint32(35) // TODO
		verified := trie.Verify(serviceID, key[:], hash[:], root[:], proofPath)
		if !verified {
			//log.Error(log.Node, "❌ BMT proof verification FAILED", "index", i, "key", common.BytesToHash(key[:]).String(), "value", hash.String(), "root", root.String(), "proofLen", len(proofPath))
			numFailures++
		}
	}

	return root
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

// SerializeBlockNumber serializes block number for BLOCK_NUMBER_KEY storage
// BLOCK_NUMBER_KEY = 0xFF..FF => block_number (4 bytes LE)
func SerializeBlockNumber(blockNumber uint32) []byte {
	value := make([]byte, 4)
	binary.LittleEndian.PutUint32(value, blockNumber)
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

// ComputeLogsBloom removed - bloom filters eliminated from JAM DA architecture
// Returns zero bytes (512 hex chars) for Ethereum RPC compatibility
func ComputeLogsBloom(logs []EthereumLog) string {
	// Return 256 bytes (512 hex chars) of zeros
	return common.Bytes2Hex(make([]byte, 256))
}
