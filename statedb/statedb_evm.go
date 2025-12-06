package statedb

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

// CreateSignedNativeTransfer wraps evmtypes.CreateSignedNativeTransfer for native ETH transfers
func CreateSignedNativeTransfer(privateKeyHex string, nonce uint64, to common.Address, amount *big.Int, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
	return evmtypes.CreateSignedNativeTransfer(privateKeyHex, nonce, to, amount, gasPrice, gasLimit, chainID)
}

// CreateSignedUSDMTransfer wraps evmtypes.CreateSignedUSDMTransfer with UsdmAddress
func CreateSignedUSDMTransfer(privateKeyHex string, nonce uint64, to common.Address, amount *big.Int, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
	return evmtypes.CreateSignedUSDMTransfer(evmtypes.UsdmAddress, privateKeyHex, nonce, to, amount, gasPrice, gasLimit, chainID)
}

// GetChainId returns the chain ID for the current network
func (n *StateDB) GetChainId() uint64 {
	return uint64(DefaultJAMChainID)
}

// GetAccounts returns the list of addresses owned by the client
func (n *StateDB) GetAccounts() []common.Address {
	// Node does not manage accounts - users should use wallets like MetaMask
	return []common.Address{}
}

// GetGasPrice returns the current gas price in wei
func (n *StateDB) GetGasPrice() uint64 {
	return 1
}

// GetLatestBlockNumber reads the current block number from EVM service storage (public interface)
func (n *StateDB) GetLatestBlockNumber(serviceID uint32) (uint32, error) {
	// Use same key as Rust: BLOCK_NUMBER_KEY = 0xFF repeated 32 times
	// Rust stores only the next block number (4 bytes LE)
	key := make([]byte, 32)
	for i := range key {
		key[i] = 0xFF
	}

	valueBytes, found, err := n.ReadServiceStorage(serviceID, key)
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

func (n *StateDB) ReadBlockByNumber(serviceID uint32, blockNumber uint32) (*evmtypes.EvmBlockPayload, error) {
	objectID := evmtypes.BlockNumberToObjectID(blockNumber)

	// Read objectID key to get blockNumber => wph (32 bytes) + timestamp (4 bytes) + segment_root (32 bytes) mapping
	valueBytes, found, err := n.ReadServiceStorage(serviceID, objectID.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block number mapping: %v", err)
	}

	if !found || len(valueBytes) < 68 {
		return nil, fmt.Errorf("block %d [%s] not found %d", blockNumber, objectID, len(valueBytes))
	}

	// Parse work_package_hash (32 bytes) + timeslot (4 bytes, little-endian) + segment_root (32 bytes)
	var workPackageHash common.Hash
	copy(workPackageHash[:], valueBytes[:32])

	return n.readBlockByHash(serviceID, workPackageHash)
}

// here, blockHash is actually a workpackagehash so we can read segments directly from DA
func (n *StateDB) readBlockByHash(serviceID uint32, workPackageHash common.Hash) (*evmtypes.EvmBlockPayload, error) {
	// read the block number + timestamp from the blockHash key
	var blockNumber uint32
	var blockTimestamp uint32
	var segmentRoot common.Hash
	valueBytes, found, err := n.ReadServiceStorage(serviceID, workPackageHash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block hash mapping: %v", err)
	}
	if found && len(valueBytes) >= 8 {
		// Parse block number (4 bytes, little-endian)
		blockNumber = binary.LittleEndian.Uint32(valueBytes[:4])
		blockTimestamp = binary.LittleEndian.Uint32(valueBytes[4:8])
		segmentRoot = common.BytesToHash(valueBytes[8:40])
	}

	payload, err := n.sdb.FetchJAMDASegments(workPackageHash, 0, 1, types.SegmentSize)
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
		payload2, err := n.sdb.FetchJAMDASegments(workPackageHash, 1, uint16(segments), remainingLength)
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

func (n *StateDB) GetBlockByHash(serviceID uint32, blockHash common.Hash, fullTx bool) (*evmtypes.EvmBlockPayload, error) {
	evmBlock, err := n.readBlockByHash(serviceID, blockHash)
	if err != nil {
		// Block not found or error reading from DA
		return nil, err
	}

	// If fullTx requested, fetch full transaction objects
	if fullTx {
		transactions := make([]evmtypes.TransactionReceipt, len(evmBlock.TxHashes))
		for i, txHash := range evmBlock.TxHashes {
			ethTx, err := n.getTransactionByHash(serviceID, txHash)
			if err != nil {
				log.Warn(log.Node, "GetBlockByHash: Failed to get transaction",
					"txHash", txHash.String(), "error", err)
				continue
			}
			transactions[i] = *ethTx
		}
		evmBlock.Transactions = transactions
	}

	return evmBlock, nil
}

// GetBlockByNumber fetches a block by number and returns raw EvmBlockPayload
func (n *StateDB) GetBlockByNumber(serviceID uint32, blockNumberStr string) (*evmtypes.EvmBlockPayload, error) {
	// 1. Parse and resolve the block number
	var targetBlockNumber uint32
	var err error

	switch blockNumberStr {
	case "latest":
		targetBlockNumber, err = n.GetLatestBlockNumber(serviceID)
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
	return n.ReadBlockByNumber(serviceID, targetBlockNumber)
}

// GetLogs fetches event logs matching a filter
func (n *StateDB) GetLogs(serviceID, fromBlock, toBlock uint32, addresses []common.Address, topics [][]common.Hash) ([]evmtypes.EthereumLog, error) {
	var allLogs []evmtypes.EthereumLog

	// Collect logs from the specified block range
	for blockNum := fromBlock; blockNum <= toBlock; blockNum++ {
		blockLogs, err := n.getLogsFromBlock(serviceID, blockNum, addresses, topics)
		if err != nil {
			log.Warn(log.Node, "GetLogs: Failed to get logs from block", "blockNumber", blockNum, "error", err)
			continue // Skip failed blocks but continue processing
		}
		allLogs = append(allLogs, blockLogs...)
	}

	return allLogs, nil
}

// getLogsFromBlock retrieves logs from a specific block that match the filter criteria
func (n *StateDB) getLogsFromBlock(serviceID uint32, blockNumber uint32, addresses []common.Address, topics [][]common.Hash) ([]evmtypes.EthereumLog, error) {
	// 1. Get all transaction hashes from the block (use canonical metadata)
	evmBlock, err := n.ReadBlockByNumber(serviceID, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to read block %d: %v", blockNumber, err)
	}
	blockTxHashes := evmBlock.TxHashes

	var blockLogs []evmtypes.EthereumLog

	// For each transaction, get its receipt and extract logs
	for _, txHash := range blockTxHashes {
		// Get transaction receipt using ReadObject abstraction
		receiptObjectID := evmtypes.TxToObjectID(txHash)
		witness, found, err := n.ReadObject(EVMServiceCode, receiptObjectID)
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

// getTransactionByHash reads a transaction receipt from storage using its hash
// Returns the raw TransactionReceipt and ObjectRef for metadata
func (n *StateDB) getTransactionByHash(serviceID uint32, txHash common.Hash) (*evmtypes.TransactionReceipt, error) {
	// Use ReadObject to get the transaction receipt with metadata from DA via meta-shard lookup
	// This includes the Ref field which contains block number and transaction index
	witness, found, err := n.ReadObject(serviceID, txHash)
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

// GetTransactionByHash fetches a transaction receipt by hash
// Returns raw TransactionReceipt and ObjectRef
func (n *StateDB) GetTransactionByHash(serviceID uint32, txHash common.Hash) (*evmtypes.TransactionReceipt, error) {
	return n.getTransactionByHash(serviceID, txHash)
}

// ReadObjectRef reads ObjectRef bytes from service storage and deserializes them
// Parameters:
// - stateDB: The stateDB to read from, if nil uses n.statedb
// - serviceCode: The service code to read from
// - objectID: The object ID (typically a transaction hash or other identifier)
// Returns:
// - ObjectRef: The deserialized ObjectRef struct
// - bool: true if found, false if not found
// - error: any error that occurred
func (stateDB *StateDB) ReadObjectRef(serviceCode uint32, objectID common.Hash) (*types.ObjectRef, bool, error) {

	// Read raw ObjectRef bytes from service storage
	objectRefBytes, found, err := stateDB.ReadServiceStorage(serviceCode, objectID.Bytes())
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

// GetBalance fetches the balance of an address at a specific verkleRoot
// verkleRoot: The Verkle tree root hash (typically from a block's stateRoot field)
func (b *StateDB) GetBalance(serviceID uint32, address common.Address, verkleRoot common.Hash) (common.Hash, error) {

	// Get the Verkle tree at the specified root
	if sdb, ok := b.sdb.(*storage.StateDBStorage); ok {
		tree, found := sdb.GetVerkleTreeAtRoot(verkleRoot)
		if !found {
			log.Warn(log.Node, "❌ GetBalance: Verkle tree NOT FOUND",
				"verkleRoot", verkleRoot.Hex())
			return common.Hash{}, fmt.Errorf("verkle tree not found for root %x", verkleRoot)
		}

		// Read from Verkle tree BasicData
		basicDataKey := evmtypes.BasicDataKey(address[:])
		basicData, err := tree.Get(basicDataKey[:], nil)
		if err != nil {
			log.Warn(log.Node, "❌ GetBalance: Tree.Get returned error",
				"address", address.String(),
				"verkleRoot", fmt.Sprintf("0x%x", verkleRoot[:]),
				"error", err)
			return common.Hash{}, nil
		}

		if len(basicData) < 32 {
			return common.Hash{}, nil
		}

		// Extract balance from BasicData (offset 16-31, 16 bytes, big-endian per EIP-6800)
		// Copy directly to Hash (already big-endian), right-aligned
		var balanceHash common.Hash
		copy(balanceHash[16:32], basicData[16:32])
		balanceValue := new(big.Int).SetBytes(balanceHash[:])

		log.Info(log.Node, "✅ GetBalance: Successfully read balance from Verkle tree",
			"address", address.String(),
			"verkleRoot", fmt.Sprintf("0x%x", verkleRoot[:]),
			"basicDataKey", fmt.Sprintf("0x%x", basicDataKey[:8]),
			"balance", balanceValue.String(),
			"balanceHex", fmt.Sprintf("0x%x", balanceHash[:]))

		return balanceHash, nil
	}

	log.Warn(log.Node, "❌ GetBalance: StateDBStorage not available")
	return common.Hash{}, fmt.Errorf("StateDBStorage not available")
}

// GetStorageAt reads contract storage at a specific position
func (stateDB *StateDB) GetStorageAt(serviceID uint32, address common.Address, position common.Hash) (common.Hash, error) {
	// Phase 4+: Read from witness cache first (populated during PrepareBuilderWitnesses or import)
	value, found := stateDB.readFromWitnessCache(address, position)
	if found {
		log.Debug(log.Node, "GetStorageAt: found in witness cache", "address", address.String(), "position", position.String())
		return value, nil
	}

	// Fallback: Read from DA via meta-shard lookup (for queries outside refine context)
	value, err := stateDB.readContractStorageValue(serviceID, address, position)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read storage: %v", err)
	}

	return value, nil
}

// GetTransactionCount fetches the nonce (transaction count) of an address at a specific verkleRoot
// verkleRoot: The Verkle tree root hash (typically from a block's stateRoot field)
func (stateDB *StateDB) GetTransactionCount(serviceID uint32, address common.Address, verkleRoot common.Hash) (uint64, error) {
	// Get the Verkle tree at the specified root
	if sdb, ok := stateDB.sdb.(*storage.StateDBStorage); ok {
		tree, found := sdb.GetVerkleTreeAtRoot(verkleRoot)
		if !found {
			log.Warn(log.Node, "❌ GetTransactionCount: Verkle tree NOT FOUND",
				"verkleRoot", verkleRoot.Hex())
			return 0, fmt.Errorf("verkle tree not found for root %x", verkleRoot)
		}

		// Read from Verkle tree BasicData
		basicDataKey := evmtypes.BasicDataKey(address[:])
		basicData, err := tree.Get(basicDataKey[:], nil)
		if err != nil {
			log.Warn(log.Node, "❌ GetTransactionCount: Tree.Get returned error",
				"address", address.String(),
				"verkleRoot", fmt.Sprintf("0x%x", verkleRoot[:]),
				"error", err)
			return 0, nil
		}

		if len(basicData) < 32 {
			return 0, nil
		}

		// Extract nonce from BasicData (offset 8-15, 8 bytes, big-endian per EIP-6800)
		nonce := binary.BigEndian.Uint64(basicData[8:16])
		return nonce, nil
	}

	// Old path: USDM contract storage
	// Compute storage key for nonce in USDM contract
	storageKey := evmtypes.ComputeNonceStorageKey(address)

	// Phase 4+: Read from witness cache first (populated during PrepareBuilderWitnesses or import)
	nonceHash, found := stateDB.readFromWitnessCache(evmtypes.UsdmAddress, storageKey)
	if !found {
		// Fallback: Read from DA via meta-shard lookup (for queries outside refine context)
		var err error
		nonceHash, err = stateDB.readContractStorageValue(serviceID, evmtypes.UsdmAddress, storageKey)
		if err != nil {
			return 0, fmt.Errorf("failed to read nonce: %v", err)
		}
	}

	// Convert hash to uint64
	nonce := new(big.Int).SetBytes(nonceHash.Bytes())
	return nonce.Uint64(), nil
}

// ReadContractStorageValue is the public method for reading EVM contract storage from JAM DA
func (stateDB *StateDB) ReadContractStorageValue(serviceID uint32, contractAddress common.Address, storageKey common.Hash) (common.Hash, error) {
	return stateDB.readContractStorageValue(serviceID, contractAddress, storageKey)
}

// readFromWitnessCache reads a storage value from the witness cache (Phase 4+)
// Returns (value, found) where found=true if the value was in the cache
func (stateDB *StateDB) readFromWitnessCache(contractAddress common.Address, storageKey common.Hash) (common.Hash, bool) {
	// Read from StateDBStorage witness cache
	value, found := stateDB.sdb.ReadStorageFromCache(contractAddress, storageKey)
	if found {
		log.Trace(log.Node, "readFromWitnessCache: found in StateDBStorage",
			"address", contractAddress.Hex(),
			"storageKey", storageKey.Hex(),
			"value", value.Hex())
	}
	return value, found
}

// readContractStorageValue reads a storage value from any contract at a specific storage key
// Post-SSR: Reads from witness cache (StateDBStorage), no DA fetches, no SSR routing
func (stateDB *StateDB) readContractStorageValue(serviceID uint32, contractAddress common.Address, storageKey common.Hash) (common.Hash, error) {
	// Post-SSR: Read directly from witness cache (no DA, no meta-shards)
	value, found := stateDB.sdb.ReadStorageFromCache(contractAddress, storageKey)
	if found {
		log.Debug(log.Node, "readContractStorageValue: Found in cache",
			"contractAddress", contractAddress.Hex(),
			"storageKey", storageKey.Hex(),
			"value", value.Hex())
		return value, nil
	}

	// Not in cache - return zero (storage slot is empty)
	log.Debug(log.Node, "readContractStorageValue: Not in cache, returning zero",
		"contractAddress", contractAddress.Hex(),
		"storageKey", storageKey.Hex())
	return common.Hash{}, nil
}

// GetCode returns the bytecode at a given address
func (stateDB *StateDB) GetCode(serviceID uint32, address common.Address) ([]byte, error) {
	// Check if Verkle tree is available (new Verkle path)
	if sdb, ok := stateDB.sdb.(*storage.StateDBStorage); ok && sdb.CurrentVerkleTree != nil {
		// Read from Verkle tree
		code, err := evmtypes.ReadCode(sdb.CurrentVerkleTree, address[:])
		if err != nil {
			return nil, fmt.Errorf("failed to read code from Verkle tree: %w", err)
		}
		log.Debug(log.Node, "GetCode from Verkle", "address", address.String(), "codeSize", len(code))
		return code, nil
	}

	// Old path: witness cache + DA
	// Phase 4+: Read from StateDBStorage witness cache first
	if code, found := stateDB.sdb.GetCode(address); found {
		log.Debug(log.Node, "GetCode: found in StateDBStorage witness cache", "address", address.String(), "codeSize", len(code))
		return code, nil
	}

	// Fallback: Read contract code from DA via meta-shard lookup (for queries outside refine context)
	codeObjectID := evmtypes.CodeToObjectID(address)
	witness, found, err := stateDB.ReadObject(serviceID, codeObjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to read contract code: %v", err)
	}
	if !found {
		// Return empty bytecode for EOA accounts
		return []byte{}, nil
	}

	return witness.Payload, nil
}

// GetTransactionReceipt fetches a transaction receipt
func (stateDB *StateDB) GetTransactionReceipt(serviceID uint32, txHash common.Hash) (*evmtypes.TransactionReceipt, error) {
	// Use ReadObject to get receipt from DA
	receiptObjectID := evmtypes.TxToObjectID(txHash)
	witness, found, err := stateDB.ReadObject(serviceID, receiptObjectID)
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

// // EstimateGas tests the EstimateGas functionality with a USDM transfer
func (stateDB *StateDB) EstimateGasTransfer(serviceID uint32, issuerAddress common.Address, usdmAddress common.Address, pvmBackend string) (uint64, error) {
	recipientAddr, _ := common.GetEVMDevAccount(1)
	transferAmount := big.NewInt(1000000) // 1M tokens (small test amount)

	// Create transfer calldata: transfer(address,uint256)
	estimateCalldata := make([]byte, 68)
	copy(estimateCalldata[0:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) // transfer(address,uint256) selector
	copy(estimateCalldata[16:36], recipientAddr.Bytes())
	copy(estimateCalldata[36:68], transferAmount.FillBytes(make([]byte, 32)))

	estimatedGas, err := stateDB.EstimateGas(serviceID, issuerAddress, &usdmAddress, 100000, 1000000000, 0, estimateCalldata, pvmBackend)
	if err != nil {
		return 0, fmt.Errorf("EstimateGas failed: %w", err)
	}
	return estimatedGas, nil
}

// EstimateGas estimates the gas needed to execute a transaction
func (stateDB *StateDB) EstimateGas(serviceID uint32, from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, pvmBackend string) (uint64, error) {
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
	workReport, err := stateDB.createSimulatedTx(serviceID, tx, pvmBackend)
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
func (stateDB *StateDB) Call(serviceID uint32, from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, blockNumber string, pvmBackend string) ([]byte, error) {
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
	wr, err := stateDB.createSimulatedTx(serviceID, tx, pvmBackend)
	if err != nil {
		return nil, fmt.Errorf("failed to execute simulation: %v", err)
	}

	return wr.Results[0].Result.Ok[:], nil
}

// createSimulatedTx creates a work package, uses BuildBundle to generate a work report for simulating a transaction
func (stateDB *StateDB) createSimulatedTx(serviceID uint32, tx *evmtypes.EthereumTransaction, pvmBackend string) (workReport *types.WorkReport, err error) {
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
	evmService, ok, err := stateDB.GetService(serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get EVM service: %v", err)
	}
	if !ok {
		return nil, fmt.Errorf("EVM service not found")
	}

	// 3. Create transaction hash
	txHash := common.Blake2Hash(extrinsic)

	// 4. Create work package
	workPackage := DefaultWorkPackage(serviceID, evmService)
	globalDepth, err := stateDB.ReadGlobalDepth(evmService.ServiceIndex)
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
	_, workReport, err = stateDB.BuildBundle(workPackage, []types.ExtrinsicsBlobs{types.ExtrinsicsBlobs{extrinsic}}, 0, nil, pvmBackend)
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
