package statedb

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"golang.org/x/crypto/blake2b"
)

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

// TransactionReceipt represents the parsed transaction receipt from storage
type TransactionReceipt struct {
	Hash          common.Hash
	Success       bool
	UsedGas       uint64
	Payload       []byte
	LogsData      []byte
	LogsBloom     [256]byte
	CumulativeGas uint64
	LogIndexStart uint64
}

// ParseRawReceipt parses the raw receipt data into a TransactionReceipt struct

// Receipt format (Rust services/evm/src/receipt.rs:203-228):
// [logs_payload_len:4][logs_payload:variable][version:1][tx_hash:32][tx_type:1][success:1][used_gas:8][tx_payload_len:4][tx_payload:variable]
func ParseRawReceipt(data []byte) (*TransactionReceipt, error) {
	if len(data) < 4+1+32+1+1+8+4 { // logs_len + version + hash + tx_type + success + gas + payload_len minimum
		return nil, fmt.Errorf("transaction receipt data too short: %d bytes", len(data))
	}

	offset := 0

	logsLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if len(data) < offset+int(logsLen) {
		return nil, fmt.Errorf("insufficient data for logs")
	}
	logsData := make([]byte, logsLen)
	copy(logsData, data[offset:offset+int(logsLen)])
	offset += int(logsLen)

	version := data[offset]
	offset += 1

	var hash common.Hash
	copy(hash[:], data[offset:offset+32])
	offset += 32

	txType := data[offset]
	offset += 1

	success := data[offset] == 1
	offset += 1

	usedGas := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	if len(data) < offset+4 {
		return nil, fmt.Errorf("insufficient data for payload length")
	}
	payloadLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if len(data) < offset+int(payloadLen) {
		return nil, fmt.Errorf("insufficient data for payload")
	}
	payload := make([]byte, payloadLen)
	copy(payload, data[offset:offset+int(payloadLen)])
	offset += int(payloadLen)

	var logsBloom [256]byte
	if len(data) >= offset+256 {
		copy(logsBloom[:], data[offset:offset+256])
		offset += 256
	}

	var cumulativeGas uint64
	if len(data) >= offset+8 {
		cumulativeGas = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	}

	var logIndexStart uint64
	if len(data) >= offset+8 {
		logIndexStart = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	}

	_ = version // Suppress unused variable warning
	_ = txType  // Suppress unused variable warning

	return &TransactionReceipt{
		Hash:          hash,
		Success:       success,
		UsedGas:       usedGas,
		Payload:       payload,
		LogsData:      logsData,
		LogsBloom:     logsBloom,
		CumulativeGas: cumulativeGas,
		LogIndexStart: logIndexStart,
	}, nil
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
