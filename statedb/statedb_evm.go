package statedb

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/types"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// CreateSignedUSDMTransfer wraps evmtypes.CreateSignedUSDMTransfer with UsdmAddress
func CreateSignedUSDMTransfer(privateKeyHex string, nonce uint64, to common.Address, amount *big.Int, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
	return evmtypes.CreateSignedUSDMTransfer(evmtypes.UsdmAddress, privateKeyHex, nonce, to, amount, gasPrice, gasLimit, chainID)
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
	return 1
}

// GetLatestBlockNumber reads the current block number from EVM service storage (public interface)
func (n *StateDB) GetLatestBlockNumber(serviceID uint32) (uint32, error) {
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
	if blockNumber > 0 {
		return blockNumber - 1, nil
	}
	return 0, nil
}

func (n *StateDB) ReadBlockMetadata(serviceID uint32, blockHash common.Hash) (*evmtypes.EvmBlockMetadata, error) {
	// Read BlockMetadata from service storage using the block hash as key
	metadataBytes, found, err := n.ReadServiceStorage(serviceID, blockHash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block metadata for hash %s: %v", blockHash.String(), err)
	}
	if !found {
		return nil, fmt.Errorf("block metadata %s not found", blockHash.String())
	}

	metadata, err := evmtypes.DeserializeEvmBlockMetadata(metadataBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize metadata for blockhash %s: %v", blockHash, err)
	}
	return metadata, nil
}

func (n *StateDB) ReadBlockByNumber(serviceID uint32, blockNumber uint32) (*evmtypes.EvmBlockPayload, *evmtypes.EvmBlockMetadata, error) {
	// use ReadStateWitnessRaw to read block from JAM State by mapping the blockNumber to an objectID
	objectID := evmtypes.BlockNumberToObjectID(blockNumber)
	witness, found, _, err := n.ReadStateWitnessRaw(serviceID, objectID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read block for number %d: %v", blockNumber, err)
	}
	if !found {
		return nil, nil, fmt.Errorf("block %d not found", blockNumber)
	}
	// if the payloadlength is 64 bytes, then it's an ObjectRef
	if len(witness.Payload) == 64 {
		// Read ObjectRef directly
		witness, found, err := n.ReadStateWitnessRef(serviceID, objectID, true)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read object ref for block %d: %v", blockNumber, err)
		} else if !found {
			return nil, nil, fmt.Errorf("object ref for block %d not found", blockNumber)
		}
		payload := witness.Payload
		block, err := evmtypes.DeserializeEvmBlockPayload(payload)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to deserialize block ref for block %d: %v", blockNumber, err)
		}
		blockHash := block.ComputeHash()
		metadata, err := n.ReadBlockMetadata(serviceID, blockHash)
		if err != nil {
			metadata = &evmtypes.EvmBlockMetadata{}
		}

		return block, metadata, nil
	}
	block, err := evmtypes.DeserializeEvmBlockPayload(witness.Payload)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to deserialize block ref for block %d: %v", blockNumber, err)
	}
	blockHash := block.ComputeHash()
	metadata, err := n.ReadBlockMetadata(serviceID, blockHash)
	if err != nil {
		metadata = &evmtypes.EvmBlockMetadata{}
	}
	return block, metadata, nil
}

func (n *StateDB) readBlockByHash(serviceID uint32, blockHash common.Hash) (*evmtypes.EvmBlockPayload, error) {
	// First, try to read the block_hash → block_number mapping from storage
	blockNumberBytes, found, err := n.ReadServiceStorage(serviceID, blockHash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block hash mapping: %v", err)
	}

	if found && len(blockNumberBytes) >= 4 {
		// Parse block number (4 bytes, little-endian)
		blockNumber := binary.LittleEndian.Uint32(blockNumberBytes[:4])

		log.Info(log.Node, fmt.Sprintf("readBlockByHash: Found block number from hash mapping. Doing n.readBlockByNumber(%d)", blockNumber), "blockHash", blockHash.Hex(), "blockNumber", blockNumber)

		// Read the block by number (ignore metadata since this function only returns block)
		block, _, err := n.ReadBlockByNumber(serviceID, blockNumber)
		return block, err
	}

	return nil, fmt.Errorf("block not found: %s", blockHash.Hex())
}

func (n *StateDB) GetBlockByHash(serviceID uint32, blockHash common.Hash, fullTx bool) (*evmtypes.EthereumBlock, error) {
	// Use new canonical path: fetch EvmBlockPayload from DA via ReadStateWitness
	evmBlock, err := n.readBlockByHash(serviceID, blockHash)
	if err != nil {
		// Block not found or error reading from DA
		return nil, err
	}

	// Generate metadata and convert EvmBlockPayload to Ethereum JSON-RPC format
	metadata := evmtypes.NewEvmBlockMetadata(evmBlock)
	ethBlock := evmBlock.ToEthereumBlock(metadata, fullTx)

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
func (n *StateDB) GetTransactionByBlockHashAndIndex(serviceID uint32, blockHash common.Hash, index uint32) (*evmtypes.EthereumTransactionResponse, error) {
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
func (n *StateDB) GetTransactionByBlockNumberAndIndex(serviceID uint32, blockNumberStr string, index uint32) (*evmtypes.EthereumTransactionResponse, error) {
	// First, get the block to retrieve transaction hashes
	block, _, err := n.GetBlockByNumber(serviceID, blockNumberStr)
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
func (n *StateDB) GetBlockByNumber(serviceID uint32, blockNumberStr string) (*evmtypes.EvmBlockPayload, *evmtypes.EvmBlockMetadata, error) {
	// 1. Parse and resolve the block number
	var targetBlockNumber uint32
	var err error

	switch blockNumberStr {
	case "latest":
		targetBlockNumber, err = n.GetLatestBlockNumber(serviceID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
	case "tentative":
		targetBlockNumber, err = n.GetLatestBlockNumber(serviceID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
		targetBlockNumber += 1

	case "earliest":
		targetBlockNumber = 1 // Genesis block
	case "pending":
		// For pending, return latest for now
		targetBlockNumber, err = n.GetLatestBlockNumber(serviceID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
	default:
		// Parse hex block number
		if len(blockNumberStr) >= 2 && blockNumberStr[:2] == "0x" {
			blockNum, parseErr := strconv.ParseUint(blockNumberStr[2:], 16, 32)
			if parseErr != nil {
				return nil, nil, fmt.Errorf("invalid block number format: %v", parseErr)
			}
			targetBlockNumber = uint32(blockNum)
		} else {
			return nil, nil, fmt.Errorf("invalid block number format: %s", blockNumberStr)
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
	evmBlock, _, err := n.ReadBlockByNumber(serviceID, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to read block %d: %v", blockNumber, err)
	}
	blockTxHashes := evmBlock.TxHashes
	blockHashHex := evmBlock.ComputeHash().String()

	var blockLogs []evmtypes.EthereumLog

	// For each transaction, get its receipt and extract logs
	for txIndex, txHash := range blockTxHashes {
		// Get transaction receipt using ReadObject abstraction
		receiptObjectID := evmtypes.TxToObjectID(txHash)
		witness, found, err := n.ReadStateWitnessRef(EVMServiceCode, receiptObjectID, false)
		if err != nil || !found {
			log.Warn(log.Node, "getLogsFromBlock: Failed to read receipt", "txHash", txHash.String(), "error", err)
			continue
		}

		receipt, err := evmtypes.ParseRawReceipt(witness.Payload)
		if err != nil {
			log.Warn(log.Node, "getLogsFromBlock: Failed to parse receipt", "txHash", txHash.String(), "error", err)
			continue
		}
		ref := witness.Ref

		// Extract and filter logs from this transaction
		if len(receipt.LogsData) > 0 {
			txLogs, err := evmtypes.ParseLogsFromReceipt(
				receipt.LogsData,
				txHash,
				fmt.Sprintf("0x%x", blockNumber),
				blockHashHex,
				fmt.Sprintf("0x%x", txIndex),
				uint64(ref.LogIndex),
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
func (n *StateDB) getTransactionByHash(serviceID uint32, txHash common.Hash) (*evmtypes.TransactionReceipt, *types.ObjectRef, error) {
	// Use ReadStateWitnessRef to get the transaction receipt with metadata from DA
	// This includes the Ref field which contains block number and transaction index
	receiptObjectID := evmtypes.TxToObjectID(txHash)
	witness, found, err := n.ReadStateWitnessRef(serviceID, receiptObjectID, false)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read transaction receipt: %v", err)
	}
	if !found {
		return nil, nil, nil // Transaction not found
	}

	// Parse the receipt data according to serialize_receipt format
	receipt, err := evmtypes.ParseRawReceipt(witness.Payload)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse transaction receipt: %v", err)
	}

	return receipt, &witness.Ref, nil
}

// GetTransactionByHash fetches a transaction receipt by hash
// Returns raw TransactionReceipt and ObjectRef
func (n *StateDB) GetTransactionByHash(serviceID uint32, txHash common.Hash) (*evmtypes.TransactionReceipt, *types.ObjectRef, error) {
	return n.getTransactionByHash(serviceID, txHash)
}

// GetTransactionByHashFormatted fetches a transaction and returns it in Ethereum JSON-RPC format
func (n *StateDB) GetTransactionByHashFormatted(serviceID uint32, txHash common.Hash) (*evmtypes.EthereumTransactionResponse, error) {
	receipt, ref, err := n.getTransactionByHash(serviceID, txHash)
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

	// Get block metadata from the receipt's Ref field
	evmBlock, _, err := n.ReadBlockByNumber(serviceID, ref.EvmBlock)
	if err != nil {
		// If block can't be read, return transaction without block metadata (pending state)
		log.Warn(log.Node, "GetTransactionByHashFormatted: Failed to read block metadata",
			"txHash", txHash.String(), "blockNum", ref.EvmBlock, "error", err)
		return ethTx, nil
	}

	// Populate block metadata
	blockHash := evmBlock.ComputeHash().String()
	blockNumber := fmt.Sprintf("0x%x", ref.EvmBlock)
	txIndex := fmt.Sprintf("0x%x", ref.TxSlot)
	ethTx.BlockHash = &blockHash
	ethTx.BlockNumber = &blockNumber
	ethTx.TransactionIndex = &txIndex

	return ethTx, nil
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

// GetBalance fetches the balance of an address from JAM State/DA
func (b *StateDB) GetBalance(serviceID uint32, address common.Address) (common.Hash, error) {

	// Compute storage key for balance in USDM contract
	storageKey := evmtypes.ComputeBalanceStorageKey(address)
	log.Debug(log.Node, "GetBalance", "address", address.String(), "storageKey", storageKey.String(), "UsdmAddress", evmtypes.UsdmAddress.String())

	// Read from SSR-sharded storage
	value, err := b.readContractStorageValue(serviceID, evmtypes.UsdmAddress, storageKey)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read balance: %v", err)
	}

	log.Debug(log.Node, "GetBalance result", "address", address.String(), "value", value.String())
	return value, nil
}

// GetStorageAt reads contract storage at a specific position
func (stateDB *StateDB) GetStorageAt(serviceID uint32, address common.Address, position common.Hash) (common.Hash, error) {
	// Read from SSR-sharded storage
	value, err := stateDB.readContractStorageValue(serviceID, address, position)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read storage: %v", err)
	}

	return value, nil
}

// GetTransactionCount fetches the nonce (transaction count) of an address
func (stateDB *StateDB) GetTransactionCount(serviceID uint32, address common.Address) (uint64, error) {
	// Compute storage key for nonce in USDM contract
	storageKey := evmtypes.ComputeNonceStorageKey(address)

	// Read from SSR-sharded storage
	nonceHash, err := stateDB.readContractStorageValue(serviceID, evmtypes.UsdmAddress, storageKey)
	if err != nil {
		return 0, fmt.Errorf("failed to read nonce: %v", err)
	}

	// Convert hash to uint64
	nonce := new(big.Int).SetBytes(nonceHash.Bytes())
	return nonce.Uint64(), nil
}

// readContractStorageValue reads a storage value from any contract at a specific storage key
func (stateDB *StateDB) readContractStorageValue(serviceID uint32, contractAddress common.Address, storageKey common.Hash) (common.Hash, error) {
	// 1. Read SSR metadata to resolve shard ID
	ssrObjectID := evmtypes.SsrToObjectID(contractAddress)
	witness, found, err := stateDB.ReadStateWitnessRef(serviceID, ssrObjectID, true)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read SSR metadata for %s: %v", contractAddress.String(), err)
	}
	if !found {
		log.Trace(log.Node, "readContractStorageValue: SSR metadata not found, returning zero",
			"contractAddress", contractAddress.String(), "storageKey", common.Bytes2Hex(storageKey.Bytes()))
		return common.Hash{}, nil // Contract doesn't exist yet
	}

	// 2. Parse SSR metadata to get global_depth and entries
	ssrData, err := evmtypes.ParseSSRMetadata(witness.Payload)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to parse SSR metadata: %v", err)
	}

	// 3. Resolve shard ID for the storage key
	shardID := evmtypes.ResolveShardID(ssrData, storageKey)

	log.Debug(log.Node, "readContractStorageValue: Resolved shard",
		"storageKey", common.Bytes2Hex(storageKey.Bytes()),
		"shardLD", shardID.Ld,
		"shardPrefix", common.Bytes2Hex(shardID.Prefix56[:]))

	// 4. Read the shard from DA
	shardObjectID := evmtypes.ShardToObjectID(contractAddress, shardID.ToBytes())
	witness, found, err = stateDB.ReadStateWitnessRef(serviceID, shardObjectID, true)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read shard: %v", err)
	}
	if !found {
		log.Debug(log.Node, "readContractStorageValue: Shard not found, returning zero",
			"shardObjectID", common.Bytes2Hex(shardObjectID.Bytes()))
		return common.Hash{}, nil // Shard doesn't exist yet
	}
	shardPayload := witness.Payload
	// 5. Parse the shard payload to extract the storage value
	value, err := evmtypes.ParseSSRPayloadForStorageKey(shardPayload, storageKey)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to parse shard payload: %v", err)
	}

	return value, nil
}

// GetCode returns the bytecode at a given address
func (stateDB *StateDB) GetCode(serviceID uint32, address common.Address) ([]byte, error) {
	// Resolve block number to state

	// Read contract code from DA
	codeObjectID := evmtypes.CodeToObjectID(address)
	witness, found, err := stateDB.ReadStateWitnessRef(serviceID, codeObjectID, true)
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
func (stateDB *StateDB) GetTransactionReceipt(serviceID uint32, txHash common.Hash) (*evmtypes.EthereumTransactionReceipt, error) {
	// Use ReadObject to get receipt from DA
	receiptObjectID := evmtypes.TxToObjectID(txHash)
	witness, found, err := stateDB.ReadStateWitnessRef(serviceID, receiptObjectID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to read transaction receipt: %v", err)
	}
	if !found {
		return nil, nil // Transaction not found
	}

	// Parse raw receipt
	receipt, err := evmtypes.ParseRawReceipt(witness.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse receipt: %v", err)
	}
	ref := witness.Ref

	// Parse transaction from receipt payload (RLP-encoded transaction)
	ethTx, err := evmtypes.ConvertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction from receipt: %v", err)
	}

	evmBlock, _, err := stateDB.ReadBlockByNumber(serviceID, ref.EvmBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to read block metadata for receipt: %v", err)
	}
	blockHash := evmBlock.ComputeHash().String()
	blockNumber := fmt.Sprintf("0x%x", ref.EvmBlock)
	txIndex := fmt.Sprintf("0x%x", ref.TxSlot)
	// Parse logs from receipt LogsData if available
	var logs []evmtypes.EthereumLog
	if len(receipt.LogsData) > 0 {
		logIndexStart := receipt.LogIndexStart

		logs, err = evmtypes.ParseLogsFromReceipt(receipt.LogsData, txHash, blockNumber,
			blockHash, txIndex, logIndexStart)
		if err != nil {
			log.Warn(log.Node, "GetTransactionReceipt: Failed to parse logs", "error", err)
			logs = []evmtypes.EthereumLog{} // Use empty logs on parse failure
		}
	}

	// Determine contract address for contract creation
	var contractAddress *string
	if ethTx.To == nil {
		senderAddr := ethereumCommon.HexToAddress(ethTx.From)

		// Parse nonce
		var nonce uint64
		fmt.Sscanf(ethTx.Nonce, "0x%x", &nonce)

		// Calculate CREATE contract address using go-ethereum's built-in function
		contractAddr := crypto.CreateAddress(senderAddr, nonce).Hex()
		contractAddress = &contractAddr

		// Note: CREATE2 detection would require parsing input data to check for CREATE2 opcode
		// For now, we only handle CREATE transactions (when To == nil)
	}

	txType := "0x0"
	if payload := receipt.Payload; len(payload) > 0 && payload[0] < 0x80 {
		txType = fmt.Sprintf("0x%x", payload[0])
	}

	var logsBloom string
	if receipt.LogsBloom != [256]byte{} {
		logsBloom = ethereumCommon.Bytes2Hex(receipt.LogsBloom[:])
	} else {
		log.Warn(log.Node, "GetTransactionReceipt: Receipt missing logs bloom, computing from logs")
		logsBloom = evmtypes.ComputeLogsBloom(logs)
	}

	cumulativeGasUsed := receipt.CumulativeGas
	if cumulativeGasUsed == 0 {
		cumulativeGasUsed = receipt.UsedGas
	}

	// Build Ethereum receipt with transaction details from parsed RLP transaction
	ethReceipt := &evmtypes.EthereumTransactionReceipt{
		TransactionHash:   txHash.String(),
		TransactionIndex:  txIndex,
		BlockHash:         blockHash,
		BlockNumber:       blockNumber,
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
	gasUsed := uint64(intent.Effect.RefInfo.GasUsed)
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
	workPackage.WorkItems[0].Payload = BuildPayload(PayloadTypeCall, 1, 0)
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
