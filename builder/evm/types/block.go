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
	// 8 (gas_used) + 32 (state_root) + 32 (transactions_root) + 32 (receipt_root) + 32 (block_access_list_hash) = 148 bytes
	HEADER_SIZE = 148
)

// UBTStateDelta represents the UBT state delta (UBT Week 1)
// PayloadType represents the type of EVM service payload
type PayloadType byte

const (
	PayloadTypeBuilder      PayloadType = 0x00
	PayloadTypeTransactions PayloadType = 0x01
	PayloadTypeGenesis      PayloadType = 0x02
	PayloadTypeCall         PayloadType = 0x03
)

type UBTStateDelta struct {
	NumEntries uint32
	Entries    []byte // Flattened key-value pairs
}

// Serialize serializes UBTStateDelta to bytes
func (d *UBTStateDelta) Serialize() []byte {
	if d == nil {
		return nil
	}
	buffer := make([]byte, 4+len(d.Entries))
	binary.LittleEndian.PutUint32(buffer[0:4], d.NumEntries)
	copy(buffer[4:], d.Entries)
	return buffer
}

// DeserializeUBTStateDelta deserializes UBTStateDelta from bytes
func DeserializeUBTStateDelta(data []byte) (*UBTStateDelta, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("delta too short: got %d bytes, need at least 4", len(data))
	}
	numEntries := binary.LittleEndian.Uint32(data[0:4])
	expectedLen := 4 + (numEntries * 64)
	if uint32(len(data)) != expectedLen {
		return nil, fmt.Errorf("invalid delta length: got %d, expected %d", len(data), expectedLen)
	}
	return &UBTStateDelta{
		NumEntries: numEntries,
		Entries:    data[4:],
	}, nil
}

// EvmBlockPayload represents the unified EVM block structure exported to JAM DA
// This structure matches the Rust EvmBlockPayload exactly
type EvmBlockPayload struct {
	// Non-serialized fields (filled by host when reading from DA)
	Number          uint32      // Block number read from blocknumber↔hash mapping
	WorkPackageHash common.Hash // Block Hash is the workpackagehash
	SegmentRoot     common.Hash

	// Fixed fields - used for block hash computation (148 bytes)
	PayloadLength       uint32      // Offset 0, 4 bytes - raw payload size
	NumTransactions     uint32      // Offset 4, 4 bytes - transaction count
	Timestamp           uint32      // Offset 8, 4 bytes - JAM timeslot
	GasUsed             uint64      // Offset 12, 8 bytes - gas used
	UBTRoot             common.Hash // Offset 20, 32 bytes - UBT state root (state commitment)
	TransactionsRoot    common.Hash // Offset 52, 32 bytes - BMT root of tx hashes
	ReceiptRoot         common.Hash // Offset 84, 32 bytes - BMT root of canonical receipts
	BlockAccessListHash common.Hash // Offset 116, 32 bytes - Blake2b hash of BlockAccessList
	// Total fixed: 148 bytes

	// Variable-length fields (not part of hash computation)
	TxHashes      []common.Hash        // Transaction hashes (32 bytes each)
	ReceiptHashes []common.Hash        // Receipt hashes (32 bytes each)
	Transactions  []TransactionReceipt // Full receipt data for each transaction

	// UBT state delta for delta-based state verification (UBT-CODEX.md)
	// Optional field - enables replay verification without full re-execution
	UBTStateDelta *UBTStateDelta `json:"ubt_delta,omitempty"`
}

// String returns a JSON representation of the EvmBlockPayload for marshal/unmarshal compatibility.
func (p *EvmBlockPayload) String() string {
	return types.ToJSONHexIndent(p)
}

// BlockCommitment returns the hash that builders should vote on.
// This is computed from the deterministic block fields only (excludes witness data).
func (p *EvmBlockPayload) BlockCommitment() common.Hash {
	// Serialize only the fields that are deterministic and must match across builders
	// This is the 148-byte fixed header
	data := make([]byte, 148)
	offset := 0

	binary.LittleEndian.PutUint32(data[offset:offset+4], p.PayloadLength)
	offset += 4
	binary.LittleEndian.PutUint32(data[offset:offset+4], p.NumTransactions)
	offset += 4
	binary.LittleEndian.PutUint32(data[offset:offset+4], p.Timestamp)
	offset += 4
	binary.LittleEndian.PutUint64(data[offset:offset+8], p.GasUsed)
	offset += 8
	copy(data[offset:offset+32], p.UBTRoot[:])
	offset += 32
	copy(data[offset:offset+32], p.TransactionsRoot[:])
	offset += 32
	copy(data[offset:offset+32], p.ReceiptRoot[:])
	offset += 32
	copy(data[offset:offset+32], p.BlockAccessListHash[:])

	return common.Blake2Hash(data)
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
		Hash:             p.WorkPackageHash.String(),
		ParentHash:       "0x0000000000000000000000000000000000000000000000000000000000000000", // No parent hash in new format
		Nonce:            "0x0000000000000000",
		Sha3Uncles:       p.SegmentRoot.String(),
		LogsBloom:        common.Bytes2Hex(make([]byte, 256)), // Zero bloom
		TransactionsRoot: p.TransactionsRoot.String(),
		StateRoot:        p.UBTRoot.String(),
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
		txns := make([]EthereumTransactionResponse, len(p.TxHashes))
		for i, txn := range p.Transactions {
			resp, err := ConvertPayloadToEthereumTransaction(&txn)
			if err != nil {
				log.Error(log.Node, "Failed to convert transaction", "err", err)
				continue
			}
			txns[i] = *resp
		}
		ethBlock.Transactions = txns
	}

	return ethBlock
}

// SerializeEvmBlockPayload serializes an EvmBlockPayload to bytes
// Matches the Rust serialize() format exactly
// Optional UBTStateDelta is appended if present
func SerializeEvmBlockPayload(payload *EvmBlockPayload) []byte {
	// Calculate base size: 148 (fixed) + len(TxHashes)*32 + len(ReceiptHashes)*32
	baseSize := 148 + len(payload.TxHashes)*32 + len(payload.ReceiptHashes)*32

	// Calculate delta size if present
	deltaSize := 0
	var deltaBytes []byte
	if payload.UBTStateDelta != nil {
		deltaBytes = payload.UBTStateDelta.Serialize()
		deltaSize = len(deltaBytes)
	}

	// Total size includes: base + 4 bytes (delta length) + delta data + 4 bytes (reserved proof length)
	totalSize := baseSize + 4 + deltaSize + 4
	data := make([]byte, totalSize)
	offset := 0

	// Serialize fixed header (148 bytes total)
	binary.LittleEndian.PutUint32(data[offset:offset+4], payload.PayloadLength)
	offset += 4

	binary.LittleEndian.PutUint32(data[offset:offset+4], payload.NumTransactions)
	offset += 4

	binary.LittleEndian.PutUint32(data[offset:offset+4], payload.Timestamp)
	offset += 4

	binary.LittleEndian.PutUint64(data[offset:offset+8], payload.GasUsed)
	offset += 8

	copy(data[offset:offset+32], payload.UBTRoot[:])
	offset += 32

	copy(data[offset:offset+32], payload.TransactionsRoot[:])
	offset += 32

	copy(data[offset:offset+32], payload.ReceiptRoot[:])
	offset += 32

	copy(data[offset:offset+32], payload.BlockAccessListHash[:])
	offset += 32

	// Serialize tx_hashes (no count field - use num_transactions from header)
	for _, txHash := range payload.TxHashes {
		copy(data[offset:offset+32], txHash[:])
		offset += 32
	}

	// Serialize receipt_hashes (no count field - use num_transactions from header)
	for _, receiptHash := range payload.ReceiptHashes {
		copy(data[offset:offset+32], receiptHash[:])
		offset += 32
	}

	// Serialize optional UBTStateDelta
	// Format: 4 bytes (delta_length) + delta_data
	// If delta_length == 0, no delta present
	binary.LittleEndian.PutUint32(data[offset:offset+4], uint32(deltaSize))
	offset += 4
	if deltaSize > 0 {
		copy(data[offset:offset+deltaSize], deltaBytes)
		offset += deltaSize
	}

	// Serialize reserved proof length (UBT-only; must be 0)
	binary.LittleEndian.PutUint32(data[offset:offset+4], 0)
	offset += 4

	return data
}

// DeserializeEvmBlockPayload deserializes an EvmBlockPayload from bytes
// Matches the Rust serialize() format exactly
func DeserializeEvmBlockPayload(data []byte, headerOnly bool) (*EvmBlockPayload, error) {
	// Minimum size: 148 bytes (fixed header)
	if len(data) < 148 {
		return nil, fmt.Errorf("block payload too short: got %d bytes, need at least 148", len(data))
	}

	offset := 0
	payload := &EvmBlockPayload{}

	// Parse fixed header (148 bytes total)
	payload.PayloadLength = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	payload.NumTransactions = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	payload.Timestamp = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	payload.GasUsed = binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	copy(payload.UBTRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.TransactionsRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.ReceiptRoot[:], data[offset:offset+32])
	offset += 32

	copy(payload.BlockAccessListHash[:], data[offset:offset+32])
	offset += 32

	if headerOnly {
		// Skip variable-length fields
		return payload, nil
	}
	// Parse tx_hashes (transaction hashes for transactions_root BMT)
	// Use num_transactions from header (no separate count field)
	txCount := payload.NumTransactions
	payload.TxHashes = make([]common.Hash, txCount)
	for i := uint32(0); i < txCount; i++ {
		if offset+32 > len(data) {
			return nil, fmt.Errorf("insufficient data for tx_hashes at index %d", i)
		}
		copy(payload.TxHashes[i][:], data[offset:offset+32])
		offset += 32
	}

	// Parse receipt_hashes (canonical receipt RLP hashes for receipts_root BMT)
	// Use num_transactions from header (no separate count field)
	receiptCount := payload.NumTransactions
	payload.ReceiptHashes = make([]common.Hash, receiptCount)
	for i := uint32(0); i < receiptCount; i++ {
		if offset+32 > len(data) {
			return nil, fmt.Errorf("insufficient data for receipt_hashes at index %d", i)
		}
		copy(payload.ReceiptHashes[i][:], data[offset:offset+32])
		offset += 32
	}

	// Parse optional UBTStateDelta (if present)
	// Format: 4 bytes (delta_length) + delta_data
	if offset+4 <= len(data) {
		deltaLen := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4

		if deltaLen > 0 {
			if offset+int(deltaLen) > len(data) {
				return nil, fmt.Errorf("insufficient data for ubt delta: need %d bytes, have %d", deltaLen, len(data)-offset)
			}
			delta, err := DeserializeUBTStateDelta(data[offset : offset+int(deltaLen)])
			if err != nil {
				return nil, fmt.Errorf("failed to deserialize ubt delta: %w", err)
			}
			payload.UBTStateDelta = delta
			offset += int(deltaLen)
		}
	}

	// Parse reserved proof length (UBT-only; must be 0)
	if offset+4 <= len(data) {
		proofLen := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4

		if proofLen > 0 {
			return nil, fmt.Errorf("proof field is reserved and must be 0 in UBT blocks")
		}
	}

	return payload, nil
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

// BuildPayload constructs a payload byte array for any payload type
func BuildPayload(payloadType PayloadType, count int, globalDepth uint8, numWitnesses int, blockAccessListHash common.Hash) []byte {
	payload := make([]byte, 40) // 1 + 4 + 1 + 2 + 32 = 40 bytes
	payload[0] = byte(payloadType)
	binary.LittleEndian.PutUint32(payload[1:5], uint32(count))
	payload[5] = globalDepth
	binary.LittleEndian.PutUint16(payload[6:8], uint16(numWitnesses))
	copy(payload[8:40], blockAccessListHash[:])
	return payload
}

// DefaultWorkPackage creates a work package with common default values
func DefaultWorkPackage(serviceID uint32, service *types.ServiceAccount) types.WorkPackage {
	return types.WorkPackage{
		AuthCodeHost:          0,
		AuthorizationCodeHash: common.Hash{}, // Caller should set if needed
		AuthorizationToken:    nil,
		ConfigurationBlob:     nil,
		RefineContext:         types.RefineContext{}, // Caller should set this
		WorkItems: []types.WorkItem{
			{
				Service:            serviceID,
				CodeHash:           service.CodeHash,
				RefineGasLimit:     types.RefineGasAllocation,
				AccumulateGasLimit: types.AccumulationGasAllocation,
				ImportedSegments:   []types.ImportSegment{},
				ExportCount:        0,
			},
		},
	}
}

// BuilderBlockCache provides keyed access to EVM blocks for RPC serving.
// Primary index is BlockCommitment (what builders vote on).
// Secondary indexes for common RPC queries: block number, block hash, tx hash.
type BuilderBlockCache struct {
	// Primary index: BlockCommitment -> block (what builders vote on)
	byBlockCommitment map[common.Hash]*EvmBlockPayload

	// Secondary indexes for RPC
	byNumber map[uint64]*EvmBlockPayload   // eth_getBlockByNumber
	byHash   map[common.Hash]*EvmBlockPayload // eth_getBlockByHash (WorkPackageHash)
	byTxHash map[common.Hash]*TxLocation      // eth_getTransactionReceipt

	// Track latest for "latest" block queries
	latestBlockNumber uint64

	// Retention limit (prune older blocks)
	maxBlocks int
}

// TxLocation stores the location of a transaction for receipt lookup
type TxLocation struct {
	BlockNumber uint64
	TxIndex     uint32
	BlockCommitment common.Hash // Reference to the block
}

// NewBuilderBlockCache creates a new block cache with specified retention
func NewBuilderBlockCache(maxBlocks int) *BuilderBlockCache {
	if maxBlocks <= 0 {
		maxBlocks = 1000 // Default: keep 1000 blocks
	}
	return &BuilderBlockCache{
		byBlockCommitment: make(map[common.Hash]*EvmBlockPayload),
		byNumber:       make(map[uint64]*EvmBlockPayload),
		byHash:         make(map[common.Hash]*EvmBlockPayload),
		byTxHash:       make(map[common.Hash]*TxLocation),
		maxBlocks:      maxBlocks,
	}
}

// AddBlock adds a block to the cache with all indexes
func (c *BuilderBlockCache) AddBlock(block *EvmBlockPayload) {
	if block == nil {
		return
	}

	votingDigest := block.BlockCommitment()
	blockNumber := uint64(block.Number)

	// Primary index
	c.byBlockCommitment[votingDigest] = block

	// Secondary indexes
	c.byNumber[blockNumber] = block
	if block.WorkPackageHash != (common.Hash{}) {
		c.byHash[block.WorkPackageHash] = block
	}

	// Index transactions for receipt lookup
	for i, txHash := range block.TxHashes {
		c.byTxHash[txHash] = &TxLocation{
			BlockNumber:  blockNumber,
			TxIndex:      uint32(i),
			BlockCommitment: votingDigest,
		}
	}

	// Update latest
	if blockNumber > c.latestBlockNumber {
		c.latestBlockNumber = blockNumber
	}

	// Prune if over limit
	c.pruneIfNeeded()
}

// GetByBlockCommitment returns block by its voting digest (primary key)
func (c *BuilderBlockCache) GetByBlockCommitment(digest common.Hash) (*EvmBlockPayload, bool) {
	block, ok := c.byBlockCommitment[digest]
	return block, ok
}

// GetByNumber returns block by number (eth_getBlockByNumber)
func (c *BuilderBlockCache) GetByNumber(number uint64) (*EvmBlockPayload, bool) {
	block, ok := c.byNumber[number]
	return block, ok
}

// GetByHash returns block by hash (eth_getBlockByHash)
func (c *BuilderBlockCache) GetByHash(hash common.Hash) (*EvmBlockPayload, bool) {
	block, ok := c.byHash[hash]
	return block, ok
}

// GetLatest returns the latest block
func (c *BuilderBlockCache) GetLatest() (*EvmBlockPayload, bool) {
	return c.GetByNumber(c.latestBlockNumber)
}

// GetLatestBlockNumber returns the latest block number
func (c *BuilderBlockCache) GetLatestBlockNumber() uint64 {
	return c.latestBlockNumber
}

// GetTxLocation returns the location of a transaction for receipt lookup
func (c *BuilderBlockCache) GetTxLocation(txHash common.Hash) (*TxLocation, bool) {
	loc, ok := c.byTxHash[txHash]
	return loc, ok
}

// GetByTxHash returns the block and tx index for a transaction hash
// This is used by GetTransactionReceipt to lookup receipts from local cache
func (c *BuilderBlockCache) GetByTxHash(txHash common.Hash) (*EvmBlockPayload, uint32, bool) {
	loc, ok := c.byTxHash[txHash]
	if !ok {
		return nil, 0, false
	}
	block, ok := c.byNumber[loc.BlockNumber]
	if !ok {
		return nil, 0, false
	}
	return block, loc.TxIndex, true
}

// Len returns the number of blocks in cache
func (c *BuilderBlockCache) Len() int {
	return len(c.byBlockCommitment)
}

// pruneIfNeeded removes oldest blocks if over maxBlocks limit
func (c *BuilderBlockCache) pruneIfNeeded() {
	if len(c.byNumber) <= c.maxBlocks {
		return
	}

	// Find oldest block numbers to prune
	var oldest uint64 = c.latestBlockNumber
	for bn := range c.byNumber {
		if bn < oldest {
			oldest = bn
		}
	}

	// Remove blocks older than (latest - maxBlocks)
	threshold := c.latestBlockNumber - uint64(c.maxBlocks)
	for bn := oldest; bn < threshold; bn++ {
		c.removeBlock(bn)
	}
}

// removeBlock removes a block and its indexes
func (c *BuilderBlockCache) removeBlock(blockNumber uint64) {
	block, ok := c.byNumber[blockNumber]
	if !ok {
		return
	}

	// Remove from all indexes
	delete(c.byNumber, blockNumber)
	delete(c.byBlockCommitment, block.BlockCommitment())
	if block.WorkPackageHash != (common.Hash{}) {
		delete(c.byHash, block.WorkPackageHash)
	}

	// Remove tx indexes
	for _, txHash := range block.TxHashes {
		delete(c.byTxHash, txHash)
	}
}
