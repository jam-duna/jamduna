package evmtypes

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/crypto/blake2b"
)

// EvmBlockPayload represents the unified EVM block structure exported to JAM DA
// This structure matches the Rust EvmBlockPayload exactly
type EvmBlockPayload struct {
	// Fixed fields (580 bytes) - used for block hash computation
	Number           uint64         // Offset 0, 8 bytes
	ParentHash       common.Hash    // Offset 8, 32 bytes
	LogsBloom        [256]byte      // Offset 40, 256 bytes
	TransactionsRoot common.Hash    // Offset 296, 32 bytes
	StateRoot        common.Hash    // Offset 328, 32 bytes
	LogIndexStart    uint64         // Offset 360, 8 bytes
	ReceiptsRoot     common.Hash    // Offset 368, 32 bytes
	MmrRoot          common.Hash    // Offset 400, 32 bytes
	ExtrinsicsHash   common.Hash    // Offset 432, 32 bytes
	ParentHeaderHash common.Hash    // Offset 464, 32 bytes
	Miner            common.Address // Offset 496, 20 bytes
	ExtraData        [32]byte       // Offset 516, 32 bytes (JAM entropy)
	Size             uint64         // Offset 548, 8 bytes
	GasLimit         uint64         // Offset 556, 8 bytes
	GasUsed          uint64         // Offset 564, 8 bytes
	Timestamp        uint64         // Offset 572, 8 bytes
	// Total fixed: 580 bytes

	// Variable-length fields (not part of hash computation)
	TxHashes      []common.Hash // Transaction hashes (for transactions_root BMT and tx lookup)
	ReceiptHashes []common.Hash // Receipt hashes (for receipts_root BMT verification)
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

// SerializeEvmBlockPayload serializes an EvmBlockPayload to bytes
// Matches the Rust serialize() format exactly
func SerializeEvmBlockPayload(payload *EvmBlockPayload) []byte {
	// Calculate total size: 580 (fixed) + 4 (tx_count) + len(TxHashes)*32 + 4 (receipt_count) + len(ReceiptHashes)*32
	size := 580 + 4 + len(payload.TxHashes)*32 + 4 + len(payload.ReceiptHashes)*32
	data := make([]byte, size)
	offset := 0

	// Serialize fixed fields (580 bytes total)
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

	binary.LittleEndian.PutUint64(data[offset:offset+8], payload.LogIndexStart)
	offset += 8

	copy(data[offset:offset+32], payload.ReceiptsRoot[:])
	offset += 32

	copy(data[offset:offset+32], payload.MmrRoot[:])
	offset += 32

	copy(data[offset:offset+32], payload.ExtrinsicsHash[:])
	offset += 32

	copy(data[offset:offset+32], payload.ParentHeaderHash[:])
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
	// Minimum size: 580 + 4 + 4 = 588 bytes (fixed fields + tx_count + receipt_count)
	if len(data) < 588 {
		return nil, fmt.Errorf("block payload too short: got %d bytes, need at least 588", len(data))
	}

	offset := 0
	payload := &EvmBlockPayload{}

	// Parse fixed fields (580 bytes total)
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

	payload.LogIndexStart = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	copy(payload.ReceiptsRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.MmrRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.ExtrinsicsHash[:], data[offset:offset+32])
	offset += 32

	copy(payload.ParentHeaderHash[:], data[offset:offset+32])
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

// VerifyBlockBMTProofs verifies that the BMT roots in a block are correctly computed
func VerifyBlockBMTProofs(block *EvmBlockPayload) error {
	// Verify Transactions Root
	if len(block.TxHashes) > 0 {
		computedTxRoot := ComputeBMTRootFromHashes(block.TxHashes)
		if computedTxRoot != block.TransactionsRoot {
			return fmt.Errorf("TransactionsRoot mismatch: computed=%s, stored=%s",
				computedTxRoot.String(), block.TransactionsRoot.String())
		}
	}

	// Verify Receipts Root
	if len(block.ReceiptHashes) > 0 {
		computedReceiptRoot := ComputeBMTRootFromHashes(block.ReceiptHashes)
		if computedReceiptRoot != block.ReceiptsRoot {
			return fmt.Errorf("ReceiptsRoot mismatch: computed=%s, stored=%s",
				computedReceiptRoot.String(), block.ReceiptsRoot.String())
		}
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

	tree := trie.NewMerkleTree(kvPairs, nil)
	if tree.Root == nil {
		return common.Hash{}
	}

	var root common.Hash
	copy(root[:], tree.Root.Hash)

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

		verified := trie.VerifyRaw(key[:], hash[:], root[:], proofPath)
		if !verified {
			log.Error(log.Node, "❌ BMT proof verification FAILED", "index", i, "key", common.BytesToHash(key[:]).String(), "value", hash.String(), "root", root.String(), "proofLen", len(proofPath))
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

// SerializeBlockNumber serializes block number and parent hash for BLOCK_NUMBER_KEY storage
// BLOCK_NUMBER_KEY = 0xFF..FF => block_number (4 bytes LE) || parent_hash (32 bytes)
func SerializeBlockNumber(blockNumber uint32, parentHash common.Hash) []byte {
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

// ComputeLogsBloom creates a bloom filter from logs
func ComputeLogsBloom(logs []EthereumLog) string {
	if len(logs) == 0 {
		// Return 256 bytes (512 hex chars) of zeros
		return common.Bytes2Hex(make([]byte, 256))
	}

	// Create a bloom filter (256 bytes = 2048 bits)
	var bloom ethereumTypes.Bloom

	// Add each log's address and topics to the bloom filter
	for _, log := range logs {
		// Add address to bloom
		address := common.HexToAddress(log.Address)
		bloom.Add(address.Bytes())

		// Add each topic to bloom
		for _, topic := range log.Topics {
			topicHash := common.HexToHash(topic)
			bloom.Add(topicHash.Bytes())
		}
	}

	return common.Bytes2Hex(bloom[:])
}
