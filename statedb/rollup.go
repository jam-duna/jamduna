package statedb

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"sync"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"

	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-verkle"
)

const (
	DefaultJAMChainID = 0x1107
)

// Lazy initialization for bootstrap auth code hash
var (
	bootstrap_auth_codehash common.Hash
	bootstrapAuthOnce       sync.Once
)

// getBootstrapAuthCodeHash computes the bootstrap auth code hash lazily
func getBootstrapAuthCodeHash() common.Hash {
	bootstrapAuthOnce.Do(func() {
		// Only compute when actually needed
		authFilePath, err := common.GetFilePath(BootStrapNullAuthFile)
		if err != nil {
			log.Warn(log.SDB, "Failed to get bootstrap auth file path, using zero hash", "err", err)
			bootstrap_auth_codehash = common.Hash{}
			return
		}
		auth_code_bytes, err := os.ReadFile(authFilePath)
		if err != nil {
			log.Warn(log.SDB, "Failed to read bootstrap null auth file, using zero hash", "err", err)
			bootstrap_auth_codehash = common.Hash{}
			return
		}
		auth_code := AuthorizeCode{
			PackageMetaData:   []byte("bootstrap"),
			AuthorizationCode: auth_code_bytes,
		}
		auth_code_encoded_bytes, err := auth_code.Encode()
		if err != nil {
			log.Warn(log.SDB, "Failed to encode bootstrap auth code, using zero hash", "err", err)
			bootstrap_auth_codehash = common.Hash{}
			return
		}
		bootstrap_auth_codehash = common.Blake2Hash(auth_code_encoded_bytes)
	})
	return bootstrap_auth_codehash
}

// StateProvider interface allows Rollup to access StateDB from a node
// Implemented by NodeContent in production, or can be nil for storage-only queries
type StateProvider interface {
	GetStateDB() *StateDB
}

// Rollup is a lightweight query interface for service-scoped state
// It does NOT own state - it queries from storage or node's StateDB via StateProvider
// Used in production nodes for RPC queries
type Rollup struct {
	serviceID uint32
	storage   types.JAMStorage
	node      StateProvider // Optional: provides access to node's StateDB for queries
}

// GetStateDB returns the StateDB from the node (if available)
func (r *Rollup) GetStateDB() *StateDB {
	if r.node != nil {
		return r.node.GetStateDB()
	}
	return nil
}

// Getter methods for Rollup fields
func (r *Rollup) GetServiceID() uint32 {
	return r.serviceID
}

func (r *Rollup) GetBalance(address common.Address, blockNumber string) (common.Hash, error) {
	// Use service-scoped verkle tree lookup
	tree, ok := r.storage.GetVerkleNodeForServiceBlock(r.serviceID, blockNumber)
	if !ok {
		return common.Hash{}, fmt.Errorf("verkle tree not found for service %d block %s", r.serviceID, blockNumber)
	}
	// Read balance from Verkle tree
	balanceHash, err := r.storage.GetBalance(tree, address)
	if err != nil {
		return common.Hash{}, err
	}

	return balanceHash, nil
}

func (r *Rollup) GetTransactionCount(address common.Address, blockNumber string) (uint64, error) {
	// Use service-scoped verkle tree lookup
	tree, ok := r.storage.GetVerkleNodeForServiceBlock(r.serviceID, blockNumber)
	if !ok {
		return 0, fmt.Errorf("verkle tree not found for service %d block %s", r.serviceID, blockNumber)
	}
	nonce, err := r.storage.GetNonce(tree, address)
	if err != nil {
		return 0, err
	}

	return nonce, nil
}

func (r *Rollup) GetCode(address common.Address, blockNumber string) ([]byte, error) {
	// Use service-scoped verkle tree lookup
	tree, ok := r.storage.GetVerkleNodeForServiceBlock(r.serviceID, blockNumber)
	if !ok {
		return nil, fmt.Errorf("verkle tree not found for service %d block %s", r.serviceID, blockNumber)
	}
	verkleTree, ok := tree.(verkle.VerkleNode)
	if !ok {
		return nil, fmt.Errorf("invalid tree type")
	}
	code, err := storage.ReadCode(verkleTree, address[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read code from Verkle tree: %w", err)
	}
	return code, nil
}

// DefaultWorkPackage creates a work package with common default values
// Caller should override fields as needed for their specific use case
func DefaultWorkPackage(serviceID uint32, service *types.ServiceAccount) types.WorkPackage {
	return types.WorkPackage{
		AuthCodeHost:          0,
		AuthorizationCodeHash: getBootstrapAuthCodeHash(),
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

// PayloadType discriminator matching Rust enum
type PayloadType byte

const (
	PayloadTypeBuilder      PayloadType = 0x00
	PayloadTypeTransactions PayloadType = 0x01
	PayloadTypeGenesis      PayloadType = 0x02
	PayloadTypeCall         PayloadType = 0x03
)

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

func NewRollup(jamStorage types.JAMStorage, serviceID uint32, node StateProvider) (*Rollup, error) {
	// Rollup is a lightweight query interface for service-scoped state
	// It does NOT own state - just provides query methods
	// node can be nil for storage-only queries (backward compatibility)
	rollup := Rollup{
		serviceID: serviceID,
		storage:   jamStorage,
		node:      node,
	}
	return &rollup, nil
}

func (r *Rollup) GetChainId() uint64 {
	return uint64(DefaultJAMChainID)
}

func (r *Rollup) GetAccounts() []common.Address {
	return []common.Address{}
}

func (r *Rollup) GetGasPrice() uint64 {
	return 1
}

// boolToHexStatus converts a boolean success status to hex status string
func boolToHexStatus(success bool) string {
	if success {
		return "0x1"
	}
	return "0x0"
}

// GetTransactionReceipt fetches a transaction receipt
func (r *Rollup) getTransactionReceipt(txHash common.Hash) (*evmtypes.TransactionReceipt, error) {
	// Use ReadObject to get receipt from DA
	receiptObjectID := evmtypes.TxToObjectID(txHash)
	witness, found, err := r.GetStateDB().ReadObject(r.serviceID, receiptObjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to read transaction receipt: %v", err)
	}
	if !found {
		return nil, nil // Transaction not found
	}

	// Parse raw receipt
	receipt, err := evmtypes.ParseRawReceipt(witness)
	if err != nil {
		return nil, fmt.Errorf("failed to parse receipt: %v", err)
	}
	return receipt, nil
}

func (r *Rollup) GetTransactionReceipt(txHash common.Hash) (*evmtypes.EthereumTransactionReceipt, error) {
	receipt, err := r.getTransactionReceipt(txHash)
	if err != nil {
		return nil, err
	}

	// Parse transaction from receipt payload (RLP-encoded transaction)
	ethTx, err := evmtypes.ConvertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction from receipt: %v", err)
	}

	// Parse logs from receipt LogsData if available
	var logs []evmtypes.EthereumLog
	if len(receipt.LogsData) > 0 {
		logIndexStart := receipt.LogIndexStart

		logs, err = evmtypes.ParseLogsFromReceipt(receipt.LogsData, txHash,
			receipt.BlockNumber, receipt.BlockHash, receipt.TransactionIndex, logIndexStart)
		if err != nil {
			log.Warn(log.Node, "GetTransactionReceipt: Failed to parse logs", "error", err)
			logs = []evmtypes.EthereumLog{} // Use empty logs on parse failure
		}
	}

	// Determine contract address for contract creation
	var contractAddress *string
	if ethTx.To == nil {
		senderAddr := common.HexToAddress(ethTx.From)

		// Parse nonce
		var nonce uint64
		fmt.Sscanf(ethTx.Nonce, "0x%x", &nonce)

		// Calculate CREATE contract address using go-ethereum's built-in function
		// Convert our common.Address to go-ethereum's common.Address
		ethSenderAddr := ethereumCommon.Address(senderAddr)
		contractAddr := crypto.CreateAddress(ethSenderAddr, nonce).Hex()
		contractAddress = &contractAddr

		// Note: CREATE2 detection would require parsing input data to check for CREATE2 opcode
		// For now, we only handle CREATE transactions (when To == nil)
	}

	txType := "0x0"
	if payload := receipt.Payload; len(payload) > 0 && payload[0] < 0x80 {
		txType = fmt.Sprintf("0x%x", payload[0])
	}

	// Bloom filters removed - always use zero bytes for RPC compatibility
	logsBloom := evmtypes.ComputeLogsBloom(logs) // Returns zero bytes

	cumulativeGasUsed := receipt.CumulativeGas
	if cumulativeGasUsed == 0 {
		cumulativeGasUsed = receipt.UsedGas
	}

	// Build Ethereum receipt with transaction details from parsed RLP transaction
	ethReceipt := &evmtypes.EthereumTransactionReceipt{
		TransactionHash:   txHash.String(),
		TransactionIndex:  fmt.Sprintf("%d", receipt.TransactionIndex),
		BlockHash:         receipt.BlockHash.String(),
		BlockNumber:       fmt.Sprintf("%x", receipt.BlockNumber),
		From:              ethTx.From,
		To:                ethTx.To,
		CumulativeGasUsed: fmt.Sprintf("0x%x", cumulativeGasUsed),
		GasUsed:           fmt.Sprintf("0x%x", receipt.UsedGas),
		ContractAddress:   contractAddress,
		Logs:              logs,
		LogsBloom:         logsBloom,
		Status:            boolToHexStatus(receipt.Success),
		EffectiveGasPrice: ethTx.GasPrice,
		Type:              txType,
	}
	return ethReceipt, nil
}

// GetTransactionByHash fetches a transaction receipt by hash
// Returns raw TransactionReceipt and ObjectRef
func (r *Rollup) getTransactionByHash(txHash common.Hash) (*evmtypes.TransactionReceipt, error) {
	// Use ReadObject to get the transaction receipt with metadata from DA via meta-shard lookup
	// This includes the Ref field which contains block number and transaction index
	witness, found, err := r.GetStateDB().ReadObject(r.serviceID, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to read transaction receipt: %v", err)
	}
	if !found {
		return nil, nil // Transaction not found
	}

	// Parse the receipt data according to serialize_receipt format
	receipt, err := evmtypes.ParseRawReceipt(witness)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction receipt: %v", err)
	}

	return receipt, nil
}

func (r *Rollup) GetTransactionByHash(txHash common.Hash) (*evmtypes.EthereumTransactionResponse, error) {
	receipt, err := r.getTransactionByHash(txHash)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, nil // Transaction not found
	}

	// Convert the original payload to Ethereum transaction format
	ethTx, err := evmtypes.ConvertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload to Ethereum transaction: %v", err)
	}

	// Populate block metadata
	ethTx.BlockHash = fmt.Sprintf("0x%x", receipt.BlockHash)
	ethTx.BlockNumber = fmt.Sprintf("0x%x", receipt.BlockNumber)
	ethTx.TransactionIndex = fmt.Sprintf("0x%x", receipt.TransactionIndex)

	return ethTx, nil
}

// GetTransactionByHashFormatted fetches a transaction and returns it in Ethereum JSON-RPC format
func (r *Rollup) GetTransactionByHashFormatted(txHash common.Hash) (*evmtypes.EthereumTransactionResponse, error) {
	receipt, err := r.getTransactionByHash(txHash)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, nil // Transaction not found
	}

	// Convert the original payload to Ethereum transaction format
	ethTx, err := evmtypes.ConvertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload to Ethereum transaction: %v", err)
	}

	// Populate block/tx metadata
	ethTx.BlockHash = fmt.Sprintf("0x%x", receipt.BlockHash)
	ethTx.BlockNumber = fmt.Sprintf("0x%d", receipt.BlockNumber)
	ethTx.TransactionIndex = fmt.Sprintf("0x%x", receipt.TransactionIndex)

	return ethTx, nil
}

func (r *Rollup) GetTransactionByBlockHashAndIndex(blockHash common.Hash, index uint32) (*evmtypes.EthereumTransactionResponse, error) {
	// First, get the block to retrieve transaction hashes
	block, err := r.readBlockByHash(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %v", err)
	}
	if block == nil {
		return nil, nil // Block not found
	}

	// Fetch the full transaction
	return r.GetTransactionByHashFormatted(block.TxHashes[index])
}

func (r *Rollup) GetTransactionByBlockNumberAndIndex(blockNumber string, index uint32) (*evmtypes.EthereumTransactionResponse, error) {

	// First, get the block to retrieve transaction hashes
	block, err := r.GetEVMBlockByNumber(blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %v", err)
	}
	if block == nil {
		return nil, nil // Block not found
	}

	// Check if index is in range
	if index >= uint32(len(block.TxHashes)) {
		return nil, fmt.Errorf("index out of range")
	}

	// Fetch the full transaction
	return r.GetTransactionByHashFormatted(block.TxHashes[index])
}

func (r *Rollup) GetLogs(fromBlock, toBlock uint32, addresses []common.Address, topics [][]common.Hash) ([]evmtypes.EthereumLog, error) {
	var allLogs []evmtypes.EthereumLog

	// Collect logs from the specified block range
	for blockNum := fromBlock; blockNum <= toBlock; blockNum++ {
		blockLogs, err := r.getLogsFromBlock(blockNum, addresses, topics)
		if err != nil {
			log.Warn(log.Node, "GetLogs: Failed to get logs from block", "blockNumber", blockNum, "error", err)
			continue // Skip failed blocks but continue processing
		}
		allLogs = append(allLogs, blockLogs...)
	}

	return allLogs, nil
}

func (r *Rollup) GetLatestBlockNumber() (uint32, error) {

	// Use same key as Rust: BLOCK_NUMBER_KEY = 0xFF repeated 32 times
	// Rust stores only the next block number (4 bytes LE)
	key := make([]byte, 32)
	for i := range key {
		key[i] = 0xFF
	}

	valueBytes, found, err := r.GetStateDB().ReadServiceStorage(r.serviceID, key)
	if err != nil {
		return 0, fmt.Errorf("failed to read block number from storage: %v", err)
	}
	if !found || len(valueBytes) < 4 {
		return 0, nil // Genesis state (block 0)
	}

	// Parse block_number (first 4 bytes, little-endian)
	blockNumber := binary.LittleEndian.Uint32(valueBytes[:4])
	if blockNumber > 0 {
		return blockNumber - 1, nil
	}
	return 0, nil
}

func (r *Rollup) GetBlockByHash(blockHash common.Hash, fullTx bool) (*evmtypes.EthereumBlock, error) {
	evmBlock, err := r.readBlockByHash(blockHash)
	if err != nil {
		// Block not found or error reading from DA
		return nil, err
	}

	// If fullTx requested, fetch full transaction objects
	if fullTx {
		transactions := make([]evmtypes.TransactionReceipt, len(evmBlock.TxHashes))
		for i, txHash := range evmBlock.TxHashes {
			ethTx, err := r.getTransactionByHash(txHash)
			if err != nil {
				log.Warn(log.Node, "GetBlockByHash: Failed to get transaction",
					"txHash", txHash.String(), "error", err)
				continue
			}
			transactions[i] = *ethTx
		}
		evmBlock.Transactions = transactions
	}
	ethBlock := evmBlock.ToEthereumBlock(evmBlock.Number, fullTx)

	return ethBlock, nil
}

func (r *Rollup) GetBlockByNumber(blockNumber string, fullTx bool) (*evmtypes.EthereumBlock, error) {
	evmBlock, err := r.GetEVMBlockByNumber(blockNumber)
	if err != nil {
		return nil, err
	}
	log.Trace(log.Node, "GetBlockByNumber: Fetched block", "number", evmBlock.Number, "b", types.ToJSON(evmBlock))
	// Generate metadata and convert EvmBlockPayload to Ethereum JSON-RPC format
	ethBlock := evmBlock.ToEthereumBlock(evmBlock.Number, fullTx)

	// If fullTx requested, fetch full transaction objects
	if fullTx {
		transactions := make([]evmtypes.EthereumTransactionResponse, 0, len(evmBlock.TxHashes))

		for i, txHash := range evmBlock.TxHashes {
			ethTx, err := r.GetTransactionByHashFormatted(txHash)
			if err != nil {
				log.Warn(log.Node, "GetBlockByNumber: Failed to get transaction", "txHash", txHash.String(), "error", err)
				continue
			}
			if ethTx != nil {
				ethTx.BlockHash = evmBlock.WorkPackageHash.String()
				ethTx.BlockNumber = fmt.Sprintf("0x%x", evmBlock.Number)
				ethTx.TransactionIndex = fmt.Sprintf("0x%x", i)
				transactions = append(transactions, *ethTx)
			}
		}

		ethBlock.Transactions = transactions
	}

	return ethBlock, nil
}

// GetBlockByNumber fetches a block by number and returns raw EvmBlockPayload
func (r *Rollup) GetEVMBlockByNumber(blockNumberStr string) (*evmtypes.EvmBlockPayload, error) {
	// 1. Parse and resolve the block number
	var targetBlockNumber uint32
	var err error
	log.Info(log.Node, "GetEVMBlockByNumber: Fetching block", "blockNumberStr", blockNumberStr)
	switch blockNumberStr {
	case "latest":
		targetBlockNumber, err = r.GetLatestBlockNumber()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
		if targetBlockNumber < 1 {
			return nil, fmt.Errorf("block 1 not ready yet")
		}
	case "earliest":
		targetBlockNumber = 1 // Genesis block
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
	log.Info(log.Node, "GetEVMBlockByNumber: Fetching block", "targetBlockNumber", targetBlockNumber)
	return r.ReadBlockByNumber(targetBlockNumber)
}

// CreateSignedNativeTransfer wraps evmtypes.CreateSignedNativeTransfer for native ETH transfers
func CreateSignedNativeTransfer(privateKeyHex string, nonce uint64, to common.Address, amount *big.Int, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
	return evmtypes.CreateSignedNativeTransfer(privateKeyHex, nonce, to, amount, gasPrice, gasLimit, chainID)
}

// CreateSignedUSDMTransfer wraps evmtypes.CreateSignedUSDMTransfer with UsdmAddress
func CreateSignedUSDMTransfer(privateKeyHex string, nonce uint64, to common.Address, amount *big.Int, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
	return evmtypes.CreateSignedUSDMTransfer(evmtypes.UsdmAddress, privateKeyHex, nonce, to, amount, gasPrice, gasLimit, chainID)
}

func (r *Rollup) ReadBlockByNumber(blockNumber uint32) (*evmtypes.EvmBlockPayload, error) {
	objectID := evmtypes.BlockNumberToObjectID(blockNumber)
	// Read objectID key to get blockNumber => wph (32 bytes) + timestamp (4 bytes) + segment_root (32 bytes) mapping
	valueBytes, found, err := r.GetStateDB().ReadServiceStorage(r.serviceID, objectID.Bytes())
	if err != nil {
		log.Error(log.Node, "ReadBlockByNumber: Failed to read block number mapping", "blockNumber", blockNumber, "error", err)
		return nil, fmt.Errorf("failed to read block number mapping: %v", err)
	}

	fmt.Printf("ReadBlockByNumber: blockNumber=%d objectID=%s %x\n", blockNumber, objectID.String(), valueBytes)
	if !found || len(valueBytes) < 68 {
		return nil, fmt.Errorf("block %d [%s] not found %d", blockNumber, objectID, len(valueBytes))
	}

	// Parse work_package_hash (32 bytes) + timeslot (4 bytes, little-endian) + segment_root (32 bytes)
	var workPackageHash common.Hash
	copy(workPackageHash[:], valueBytes[:32])

	return r.readBlockByHash(workPackageHash)
}

// here, blockHash is actually a workpackagehash so we can read segments directly from DA
func (r *Rollup) readBlockByHash(workPackageHash common.Hash) (*evmtypes.EvmBlockPayload, error) {
	// read the block number + timestamp from the blockHash key
	var blockNumber uint32
	var blockTimestamp uint32
	var segmentRoot common.Hash
	valueBytes, found, err := r.GetStateDB().ReadServiceStorage(r.serviceID, workPackageHash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block hash mapping: %v", err)
	}
	if found && len(valueBytes) >= 8 {
		// Parse block number (4 bytes, little-endian)
		blockNumber = binary.LittleEndian.Uint32(valueBytes[:4])
		blockTimestamp = binary.LittleEndian.Uint32(valueBytes[4:8])
		segmentRoot = common.BytesToHash(valueBytes[8:40])
	}

	payload, err := r.GetStateDB().sdb.FetchJAMDASegments(workPackageHash, 0, 1, types.SegmentSize)
	if err != nil {
		return nil, fmt.Errorf("block not found: %s", workPackageHash.Hex())
	}

	block, err := evmtypes.DeserializeEvmBlockPayload(payload, true)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize block payload: %v", err)
	}
	// using block.PayloadLength figure out how many segments to read for full block
	segments := (block.PayloadLength + types.SegmentSize - 1) / types.SegmentSize
	if segments > 1 {
		remainingLength := block.PayloadLength - types.SegmentSize
		payload2, err := r.GetStateDB().sdb.FetchJAMDASegments(workPackageHash, 1, uint16(segments), remainingLength)
		if err != nil {
			return nil, fmt.Errorf("failed to read additional block segments: %v", err)
		}
		payload = append(payload, payload2...)
	}
	block, err = evmtypes.DeserializeEvmBlockPayload(payload, false)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize full block payload: %v", err)
	}
	block.WorkPackageHash = workPackageHash
	block.SegmentRoot = segmentRoot
	block.Timestamp = blockTimestamp
	block.Number = blockNumber
	return block, nil
}

// getLogsFromBlock retrieves logs from a specific block that match the filter criteria
func (r *Rollup) getLogsFromBlock(blockNumber uint32, addresses []common.Address, topics [][]common.Hash) ([]evmtypes.EthereumLog, error) {
	// 1. Get all transaction hashes from the block (use canonical metadata)
	evmBlock, err := r.ReadBlockByNumber(blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to read block %d: %v", blockNumber, err)
	}
	blockTxHashes := evmBlock.TxHashes

	var blockLogs []evmtypes.EthereumLog

	// For each transaction, get its receipt and extract logs
	for _, txHash := range blockTxHashes {
		// Get transaction receipt using ReadObject abstraction
		receiptObjectID := evmtypes.TxToObjectID(txHash)
		witness, found, err := r.GetStateDB().ReadObject(EVMServiceCode, receiptObjectID)
		if err != nil || !found {
			log.Warn(log.Node, "getLogsFromBlock: Failed to read receipt", "txHash", txHash.String(), "error", err)
			continue
		}

		receipt, err := evmtypes.ParseRawReceipt(witness)
		if err != nil {
			log.Warn(log.Node, "getLogsFromBlock: Failed to parse receipt", "txHash", txHash.String(), "error", err)
			continue
		}

		// Extract and filter logs from this transaction
		if len(receipt.LogsData) > 0 {
			txLogs, err := evmtypes.ParseLogsFromReceipt(
				receipt.LogsData,
				txHash,
				blockNumber,
				receipt.BlockHash,
				receipt.TransactionIndex,
				uint64(0),
			)
			if err != nil {
				log.Warn(log.Node, "getLogsFromBlock: Failed to parse logs", "txHash", txHash.String(), "error", err)
				continue
			}

			// Apply address and topic filters
			for _, ethLog := range txLogs {
				if evmtypes.MatchesLogFilter(ethLog, addresses, topics) {
					blockLogs = append(blockLogs, ethLog)
				}
			}
		}
	}

	return blockLogs, nil
}

// ReadObjectRef reads ObjectRef bytes from service storage and deserializes them
// Parameters:
// - stateDB: The stateDB to read from, if nil uses r.statedb
// - serviceCode: The service code to read from
// - objectID: The object ID (typically a transaction hash or other identifier)
// Returns:
// - ObjectRef: The deserialized ObjectRef struct
// - bool: true if found, false if not found
// - error: any error that occurred
func (r *Rollup) ReadObjectRef(serviceCode uint32, objectID common.Hash) (*types.ObjectRef, bool, error) {
	// Read raw ObjectRef bytes from service storage
	objectRefBytes, found, err := r.GetStateDB().ReadServiceStorage(serviceCode, objectID.Bytes())
	if err != nil {
		return nil, false, fmt.Errorf("failed to read ObjectRef from service storage: %v", err)
	}
	if !found {
		return nil, false, nil // ObjectRef not found
	}

	// Deserialize ObjectRef from storage data
	offset := 0
	objRef, err := types.DeserializeObjectRef(objectRefBytes, &offset)
	if err != nil {
		return nil, false, fmt.Errorf("failed to deserialize ObjectRef: %v", err)
	}

	return &objRef, true, nil
}

func (r *Rollup) GetStorageAt(address common.Address, position common.Hash, blockNumber string) (common.Hash, error) {
	value, err := r.ReadContractStorageValue(address, position)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read storage: %v", err)
	}
	return value, nil
}

// ReadContractStorageValue reads EVM contract storage, checking witness cache first
func (r *Rollup) ReadContractStorageValue(contractAddress common.Address, storageKey common.Hash) (common.Hash, error) {
	// Read from StateDBStorage witness cache (populated during PrepareBuilderWitnesses or import)
	value, found := r.GetStateDB().sdb.ReadStorageFromCache(contractAddress, storageKey)
	if found {
		return value, nil
	}
	// Not found in cache, return zero value
	return common.Hash{}, nil
}

// // EstimateGas tests the EstimateGas functionality with a USDM transfer
func (r *Rollup) EstimateGasTransfer(issuerAddress common.Address, usdmAddress common.Address, pvmBackend string) (uint64, error) {
	recipientAddr, _ := common.GetEVMDevAccount(1)
	transferAmount := big.NewInt(1000000) // 1M tokens (small test amount)

	// Create transfer calldata: transfer(address,uint256)
	estimateCalldata := make([]byte, 68)
	copy(estimateCalldata[0:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) // transfer(address,uint256) selector
	copy(estimateCalldata[16:36], recipientAddr.Bytes())
	copy(estimateCalldata[36:68], transferAmount.FillBytes(make([]byte, 32)))

	estimatedGas, err := r.EstimateGas(issuerAddress, &usdmAddress, 100000, 1000000000, 0, estimateCalldata, pvmBackend)
	if err != nil {
		return 0, fmt.Errorf("EstimateGas failed: %w", err)
	}
	return estimatedGas, nil
}

// EstimateGas estimates the gas needed to execute a transaction
func (r *Rollup) EstimateGas(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, pvmBackend string) (uint64, error) {
	// Build Ethereum transaction for simulation
	valueBig := new(big.Int).SetUint64(value)
	gasPriceBig := new(big.Int).SetUint64(gasPrice)
	tx := &evmtypes.EthereumTransaction{
		From:     from,
		To:       to,
		Gas:      gas,
		GasPrice: gasPriceBig,
		Value:    valueBig,
		Data:     data,
	}

	// Create simulation work package with payload "B"
	workReport, err := r.createSimulatedTx(tx, pvmBackend)
	if err != nil {
		return 0, fmt.Errorf("failed to create simulation work package: %v", err)
	}
	if len(workReport.Results) == 0 || len(workReport.Results[0].Result.Ok) == 0 {
		return 0, fmt.Errorf("no result from simulation")
	}

	effects, err := types.DeserializeExecutionEffects(workReport.Results[0].Result.Ok)
	if err != nil {
		return 0, fmt.Errorf("failed to deserialize execution effects: %v", err)
	}

	intent := effects.WriteIntents[0]
	gasUsed := uint64(0) // TODO
	log.Info(log.SDB, "intent.Effect.ObjectID", "object_id", intent.Effect.ObjectID.String(),
		"gas_used", gasUsed)

	return gasUsed, nil
}

// Call simulates a transaction execution without submitting it
func (r *Rollup) Call(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, blockNumber string, pvmBackend string) ([]byte, error) {
	// Build Ethereum transaction for simulation
	valueBig := new(big.Int).SetUint64(value)
	gasPriceBig := new(big.Int).SetUint64(gasPrice)
	tx := &evmtypes.EthereumTransaction{
		From:     from,
		To:       to,
		Gas:      gas,
		GasPrice: gasPriceBig,
		Value:    valueBig,
		Data:     data,
	}

	// Execute simulation
	wr, err := r.createSimulatedTx(tx, pvmBackend)
	if err != nil {
		return nil, fmt.Errorf("failed to execute simulation: %v", err)
	}

	return wr.Results[0].Result.Ok[:], nil
}

// createSimulatedTx creates a work package, uses BuildBundle to generate a work report for simulating a transaction
func (r *Rollup) createSimulatedTx(tx *evmtypes.EthereumTransaction, pvmBackend string) (workReport *types.WorkReport, err error) {
	// 1. Convert Ethereum transaction to JAM extrinsic format
	// Extrinsic format: caller(20) + target(20) + gas_limit(32) + gas_price(32) + value(32) + call_kind(4) + data_len(8) + data
	dataLen := len(tx.Data)
	extrinsicSize := 148 + dataLen // 20+20+32+32+32+4+8 + data
	extrinsic := make([]byte, extrinsicSize)

	offset := 0

	// caller (20 bytes)
	copy(extrinsic[offset:offset+20], tx.From.Bytes())
	offset += 20

	// target (20 bytes) - use zero address for contract creation
	if tx.To != nil {
		copy(extrinsic[offset:offset+20], tx.To.Bytes())
	} else {
		// Contract creation - use zero address
		copy(extrinsic[offset:offset+20], make([]byte, 20))
	}
	offset += 20

	// gas_limit (32 bytes, big-endian)
	gasLimitBytes := make([]byte, 32)
	binary.BigEndian.PutUint64(gasLimitBytes[24:32], tx.Gas)
	copy(extrinsic[offset:offset+32], gasLimitBytes)
	offset += 32

	// gas_price (32 bytes, big-endian)
	gasPriceBytes := tx.GasPrice.FillBytes(make([]byte, 32))
	copy(extrinsic[offset:offset+32], gasPriceBytes)
	offset += 32

	// value (32 bytes, big-endian)
	valueBytes := tx.Value.FillBytes(make([]byte, 32))
	copy(extrinsic[offset:offset+32], valueBytes)
	offset += 32

	// call_kind (4 bytes, little-endian) - 0 = CALL, 1 = CREATE
	callKind := uint32(0) // CALL
	if tx.To == nil {
		callKind = 1 // CREATE
	}
	binary.LittleEndian.PutUint32(extrinsic[offset:offset+4], callKind)
	offset += 4

	// data_len (8 bytes, little-endian)
	binary.LittleEndian.PutUint64(extrinsic[offset:offset+8], uint64(dataLen))
	offset += 8

	// data
	copy(extrinsic[offset:], tx.Data)

	// 2. Get the EVM service info
	evmService, ok, err := r.GetStateDB().GetService(r.serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get EVM service: %v", err)
	}
	if !ok {
		return nil, fmt.Errorf("EVM service not found")
	}

	// 3. Create transaction hash
	txHash := common.Blake2Hash(extrinsic)

	// 4. Create work package
	workPackage := DefaultWorkPackage(r.serviceID, evmService)
	globalDepth, err := r.GetStateDB().ReadGlobalDepth(evmService.ServiceIndex)
	if err != nil {
		return nil, fmt.Errorf("ReadGlobalDepth failed: %v", err)
	}
	workPackage.WorkItems[0].Payload = BuildPayload(PayloadTypeCall, 1, globalDepth, 0, common.Hash{})
	workPackage.WorkItems[0].Extrinsics = []types.WorkItemExtrinsic{
		{
			Hash: txHash,
			Len:  uint32(len(extrinsic)),
		},
	}

	// Execute the work package with proper parameters
	// Use core index 0 for simulation, current slot, and mark as not first guarantor
	_, workReport, err = r.GetStateDB().BuildBundle(workPackage, []types.ExtrinsicsBlobs{types.ExtrinsicsBlobs{extrinsic}}, 0, nil, pvmBackend)
	if err != nil {
		return nil, fmt.Errorf("BuildBundle failed: %v", err)
	}
	if workReport == nil {
		return nil, fmt.Errorf("BuildBundle returned nil work report")
	}

	// Extract result from work report
	if len(workReport.Results) > 0 {
		// Return the output from the first work result
		result := workReport.Results[0].Result
		if len(result.Ok) > 0 {
			// Parse ExecutionEffects to extract call output
			// Format: ExecutionEffects serialization + call output appended
			_, err := types.DeserializeExecutionEffects(result.Ok)
			if err != nil {
				log.Warn(log.Node, "createSimulatedTx: failed to deserialize effects", "err", err)
				// Return raw result if deserialization fails
				return workReport, nil
			}

			// The call output is appended after the serialized ExecutionEffects
			// ExecutionEffects format: [write_intents_count:2][write_intents...]
			// For payload "B", write_intents should be empty (count=0)
			// Header size: 2 bytes
			effectsHeaderSize := 2

			// For payload "B", there should be no write intents, so output starts immediately after header
			if len(result.Ok) > effectsHeaderSize {
				// Extract the call output (everything after ExecutionEffects header)
				callOutput := result.Ok[effectsHeaderSize:]
				log.Debug(log.Node, "createSimulatedTx: extracted call output",
					"gas_used", 0,
					"output_len", len(callOutput))
				return workReport, nil
			}

			// No output, return empty
			return nil, fmt.Errorf("no call output from simulation")
		}
		if result.Err != 0 {
			return nil, fmt.Errorf("simulation error code: %d", result.Err)
		}
	}

	return nil, fmt.Errorf("no result from simulation")
}

// SendRawTransaction submits a signed transaction to the mempool
func (n *Rollup) SendRawTransaction(signedTxData []byte) (common.Hash, error) {
	// Parse the raw transaction
	tx, err := evmtypes.ParseRawTransaction(signedTxData)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to parse transaction: %v", err)
	}

	// Recover sender from signature
	sender, err := tx.RecoverSender()
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to recover sender: %v", err)
	}
	tx.From = sender

	// Validate signature - sender recovery already done above, verify it's valid
	if sender == (common.Address{}) {
		return common.Hash{}, fmt.Errorf("invalid signature: unable to recover sender address")
	}

	// Validate nonce against current state
	currentNonce, err := n.GetTransactionCount(sender, "latest")
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get current nonce for validation: %v", err)
	}
	if tx.Nonce < currentNonce {
		return common.Hash{}, fmt.Errorf("nonce too low: transaction nonce %d, account nonce %d", tx.Nonce, currentNonce)
	}

	// Validate balance - sender must have enough to cover value + gas costs
	balance, err := n.GetBalance(sender, "latest")
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get balance for validation: %v", err)
	}
	balanceBig := new(big.Int).SetBytes(balance.Bytes())

	// Calculate total cost: value + (gas * gasPrice)
	gasCost := new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas), tx.GasPrice)
	totalCost := new(big.Int).Add(tx.Value, gasCost)

	if balanceBig.Cmp(totalCost) < 0 {
		return common.Hash{}, fmt.Errorf("insufficient funds: balance %s, required %s (value %s + gas cost %s)",
			balanceBig.String(), totalCost.String(), tx.Value.String(), gasCost.String())
	}

	// Validate gas limit against block gas limit (RefineGasAllocation per work item)
	maxGasLimit := uint64(types.RefineGasAllocation)
	if tx.Gas > maxGasLimit {
		return common.Hash{}, fmt.Errorf("gas limit too high: transaction gas %d exceeds maximum %d", tx.Gas, maxGasLimit)
	}

	// Minimum gas for basic transaction is 1000
	const minTxGas = 1000
	if tx.Gas < minTxGas {
		return common.Hash{}, fmt.Errorf("gas limit too low: transaction gas %d is below minimum %d", tx.Gas, minTxGas)
	}

	// TODO: Add transaction to mempool organized by BAL
	// err = n.txPool.AddTransaction(tx)
	// if err != nil {
	// 	return common.Hash{}, fmt.Errorf("failed to add transaction to mempool: %v", err)
	// }

	log.Info(log.Node, "SendRawTransaction TODO Transaction added to mempool",
		"hash", tx.Hash.String(),
		"from", tx.From.String(),
		"nonce", tx.Nonce)

	return tx.Hash, nil
}
