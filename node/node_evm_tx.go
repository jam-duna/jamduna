package node

import (
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	types "github.com/colorfulnotion/jam/types"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// EthereumTransactionResponse represents a complete Ethereum transaction for JSON-RPC responses
type EthereumTransactionResponse struct {
	Hash             string  `json:"hash"`
	Nonce            string  `json:"nonce"`
	BlockHash        *string `json:"blockHash"`
	BlockNumber      *string `json:"blockNumber"`
	TransactionIndex *string `json:"transactionIndex"`
	From             string  `json:"from"`
	To               *string `json:"to"`
	Value            string  `json:"value"`
	GasPrice         string  `json:"gasPrice"`
	Gas              string  `json:"gas"`
	Input            string  `json:"input"`
	V                string  `json:"v"`
	R                string  `json:"r"`
	S                string  `json:"s"`
}

// convertPayloadToEthereumTransaction converts the transaction payload to Ethereum format
func convertPayloadToEthereumTransaction(receipt *TransactionReceipt) (*EthereumTransactionResponse, error) {
	// The payload contains the RLP-encoded Ethereum transaction (from main.rs line 499: extrinsic.clone())
	// This can be a legacy tx, EIP-2930, or EIP-1559 transaction

	payload := receipt.Payload
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty payload")
	}

	// Decode the RLP-encoded Ethereum transaction using go-ethereum
	var tx ethereumTypes.Transaction
	err := tx.UnmarshalBinary(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode RLP transaction: %v", err)
	}

	// Extract transaction fields
	var to *string
	if tx.To() != nil {
		toAddr := tx.To().Hex()
		to = &toAddr
	}

	// Recover sender address from signature
	signer := ethereumTypes.LatestSignerForChainID(tx.ChainId())
	from, err := signer.Sender(&tx)
	if err != nil {
		return nil, fmt.Errorf("failed to recover sender: %v", err)
	}

	input := "0x" + hex.EncodeToString(tx.Data())

	// Extract v, r, s from signature
	v, r, s := tx.RawSignatureValues()

	return &EthereumTransactionResponse{
		Hash:             receipt.Hash.String(),
		Nonce:            fmt.Sprintf("0x%x", tx.Nonce()),
		BlockHash:        nil, // Will be set by caller
		BlockNumber:      nil, // Will be set by caller
		TransactionIndex: nil, // Will be set by caller
		From:             from.Hex(),
		To:               to,
		Value:            fmt.Sprintf("0x%x", tx.Value()),
		GasPrice:         fmt.Sprintf("0x%x", tx.GasPrice()),
		Gas:              fmt.Sprintf("0x%x", tx.Gas()),
		Input:            input,
		V:                fmt.Sprintf("0x%x", v),
		R:                fmt.Sprintf("0x%x", r),
		S:                fmt.Sprintf("0x%x", s),
	}, nil
}

// boolToHexStatus converts a boolean success status to hex status string
func boolToHexStatus(success bool) string {
	if success {
		return "0x1"
	}
	return "0x0"
}

// parseTransactionObject parses a JSON transaction object into an EthereumTransaction
func parseTransactionObject(txObj map[string]interface{}) (*EthereumTransaction, error) {
	tx := &EthereumTransaction{}

	// Parse 'to' field
	if toStr, ok := txObj["to"].(string); ok && toStr != "" {
		to := common.HexToAddress(toStr)
		tx.To = &to
	}

	// Parse 'from' field
	if fromStr, ok := txObj["from"].(string); ok && fromStr != "" {
		tx.From = common.HexToAddress(fromStr)
	}

	// Parse 'data' field
	if dataStr, ok := txObj["data"].(string); ok && dataStr != "" {
		if len(dataStr) >= 2 && dataStr[:2] == "0x" {
			tx.Data = common.FromHex(dataStr)
		}
	}

	// Parse 'value' field
	if valueStr, ok := txObj["value"].(string); ok && valueStr != "" {
		if len(valueStr) >= 2 && valueStr[:2] == "0x" {
			if value, success := big.NewInt(0).SetString(valueStr[2:], 16); success {
				tx.Value = value
			}
		}
	}
	if tx.Value == nil {
		tx.Value = big.NewInt(0)
	}

	// Parse 'gas' field
	if gasStr, ok := txObj["gas"].(string); ok && gasStr != "" {
		if len(gasStr) >= 2 && gasStr[:2] == "0x" {
			if gas, err := strconv.ParseUint(gasStr[2:], 16, 64); err == nil {
				tx.Gas = gas
			}
		}
	}
	if tx.Gas == 0 {
		tx.Gas = 21000 // Default gas limit
	}

	// Parse 'gasPrice' field
	if gasPriceStr, ok := txObj["gasPrice"].(string); ok && gasPriceStr != "" {
		if len(gasPriceStr) >= 2 && gasPriceStr[:2] == "0x" {
			if gasPrice, success := big.NewInt(0).SetString(gasPriceStr[2:], 16); success {
				tx.GasPrice = gasPrice
			}
		}
	}
	if tx.GasPrice == nil {
		tx.GasPrice = big.NewInt(1000000000) // 1 Gwei default
	}

	// Parse 'nonce' field
	if nonceStr, ok := txObj["nonce"].(string); ok && nonceStr != "" {
		if len(nonceStr) >= 2 && nonceStr[:2] == "0x" {
			if nonce, err := strconv.ParseUint(nonceStr[2:], 16, 64); err == nil {
				tx.Nonce = nonce
			}
		}
	}
	// Note: If nonce is 0, caller should check if it was explicitly provided or needs to be fetched

	return tx, nil
}

// getTransactionByHash reads a transaction object from storage using its hash
func (n *NodeContent) getTransactionByHash(txHash common.Hash) (*EthereumTransactionResponse, error) {
	// Use the new ReadObject abstraction to get the transaction receipt payload from DA
	// This does a two-step process:
	// 1. Read ObjectRef from EVM service storage using txHash as key
	// 2. Use ObjectRef to fetch actual receipt payload from DA using FetchJAMDASegments
	receiptObjectID := tx_to_objectID(txHash)
	witness, found, err := n.statedb.ReadStateWitness(statedb.EVMServiceCode, receiptObjectID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to read transaction receipt: %v", err)
	}
	if !found {
		return nil, nil // Transaction not found
	}

	// Parse the receipt data according to serialize_receipt format
	receipt, err := parseRawReceipt(witness.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction receipt: %v", err)
	}

	// Convert the original payload to Ethereum transaction format
	ethTx, err := convertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload to Ethereum transaction: %v", err)
	}

	// Search last 20 blocks to find block metadata
	const maxSearchDepth = 20
	blockNum, blockHash, txIndex, err := n.findBlockForTransaction(txHash, maxSearchDepth)
	if err != nil {
		// Transaction exists in DA but not in any block yet
		log.Warn(log.Node, "getTransactionByHash: Transaction found but block not found",
			"txHash", txHash.String(), "error", err)
		// Return transaction with nil block fields (pending state)
		return ethTx, nil
	}

	// Populate block metadata
	blockNumber := fmt.Sprintf("0x%x", blockNum)
	ethTx.BlockHash = &blockHash
	ethTx.BlockNumber = &blockNumber
	ethTx.TransactionIndex = &txIndex

	return ethTx, nil
}

// EstimateGas estimates the gas needed to execute a transaction
func (n *NodeContent) EstimateGas(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte) (uint64, error) {
	// Build Ethereum transaction for simulation
	valueBig := new(big.Int).SetUint64(value)
	gasPriceBig := new(big.Int).SetUint64(gasPrice)
	tx := &EthereumTransaction{
		From:     from,
		To:       to,
		Gas:      gas,
		GasPrice: gasPriceBig,
		Value:    valueBig,
		Data:     data,
	}

	// Create simulation work package with payload "B"
	workPackage, extrinsic, err := n.createSimulationWorkPackage(tx)
	if err != nil {
		return 0, fmt.Errorf("failed to create simulation work package: %v", err)
	}

	// Execute simulation - executeSimulationWorkPackage now returns just the call output
	// We need to get the full result to parse ExecutionEffects for gas_used
	extrinsicBlobs := make(types.ExtrinsicsBlobs, 1)
	extrinsicBlobs[0] = extrinsic

	_, workReport, err := n.BuildBundle(*workPackage, []types.ExtrinsicsBlobs{extrinsicBlobs}, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to build bundle: %v", err)
	}

	if len(workReport.Results) == 0 || len(workReport.Results[0].Result.Ok) == 0 {
		return 0, fmt.Errorf("no result from simulation")
	}

	effects, err := types.DeserializeExecutionEffects(workReport.Results[0].Result.Ok)
	if err != nil {
		return 0, fmt.Errorf("failed to deserialize execution effects: %v", err)
	}

	return effects.GasUsed, nil
}

// Call simulates a transaction execution without submitting it
func (n *NodeContent) Call(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, blockNumber string) ([]byte, error) {
	// Build Ethereum transaction for simulation
	valueBig := new(big.Int).SetUint64(value)
	gasPriceBig := new(big.Int).SetUint64(gasPrice)
	tx := &EthereumTransaction{
		From:     from,
		To:       to,
		Gas:      gas,
		GasPrice: gasPriceBig,
		Value:    valueBig,
		Data:     data,
	}

	// Create simulation work package
	workPackage, extrinsic, err := n.createSimulationWorkPackage(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to create simulation work package: %v", err)
	}

	// Execute simulation
	result, err := n.executeSimulationWorkPackage(workPackage, [][]byte{extrinsic})
	if err != nil {
		return nil, fmt.Errorf("failed to execute simulation: %v", err)
	}

	return result, nil
}

// createSimulationWorkPackage creates a work package for transaction simulation
func (n *NodeContent) createSimulationWorkPackage(tx *EthereumTransaction) (*types.WorkPackage, []byte, error) {
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
	evmService, ok, err := n.statedb.GetService(statedb.EVMServiceCode)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get EVM service: %v", err)
	}
	if !ok {
		return nil, nil, fmt.Errorf("EVM service not found")
	}

	// 3. Create transaction hash
	txHash := common.Blake2Hash(extrinsic)
	// prepare genesis work package
	auth_payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(auth_payload, uint32(statedb.AuthCopyServiceCode))

	// 4. Create work package
	workPackage := &types.WorkPackage{
		AuthCodeHost:          0,
		AuthorizationToken:    nil,                     // null-authorizer for simulation
		AuthorizationCodeHash: bootstrap_auth_codehash, // Empty for simulation
		ConfigurationBlob:     nil,
		WorkItems: []types.WorkItem{
			{
				Service:            statedb.EVMServiceCode,
				CodeHash:           evmService.CodeHash,
				Payload:            types.PayloadCall, // "B" mode for EstimateGas/Call with unsigned call data
				RefineGasLimit:     types.RefineGasAllocation / 2,
				AccumulateGasLimit: types.AccumulationGasAllocation / 2,
				ImportedSegments:   []types.ImportSegment{}, // Empty for simulation
				ExportCount:        0,                       // No exports for static call
				Extrinsics: []types.WorkItemExtrinsic{
					{
						Hash: txHash,
						Len:  uint32(len(extrinsic)),
					},
				},
			},
		},
	}

	return workPackage, extrinsic, nil
}

// executeSimulationWorkPackage executes a work package for transaction simulation
func (n *NodeContent) executeSimulationWorkPackage(workPackage *types.WorkPackage, extrinsics [][]byte) ([]byte, error) {
	// Copy extrinsics to ExtrinsicsBlobs
	extrinsicBlobs := make(types.ExtrinsicsBlobs, len(extrinsics))
	copy(extrinsicBlobs, extrinsics)

	// Execute the work package with proper parameters
	// Use core index 0 for simulation, current slot, and mark as not first guarantor
	// TODO: accept extrinsicsBlobs input as this only works for one work item
	_, workReport, err := n.BuildBundle(*workPackage, []types.ExtrinsicsBlobs{extrinsicBlobs}, 0)
	if err != nil {
		return nil, fmt.Errorf("BuildBundle failed: %v", err)
	}

	// Extract result from work report
	if len(workReport.Results) > 0 {
		// Return the output from the first work result
		result := workReport.Results[0].Result
		if len(result.Ok) > 0 {
			// Parse ExecutionEffects to extract gas_used and call output
			// Format: ExecutionEffects serialization + call output appended
			effects, err := types.DeserializeExecutionEffects(result.Ok)
			if err != nil {
				log.Warn(log.Node, "executeSimulationWorkPackage: failed to deserialize effects", "err", err)
				// Return raw result if deserialization fails
				return result.Ok, nil
			}

			// The call output is appended after the serialized ExecutionEffects
			// ExecutionEffects format: [export_count:2][gas_used:8][write_intents_count:2][write_intents...]
			// For payload "B", write_intents should be empty (count=0)
			// Header size: 2 + 8 + 2 = 12 bytes
			effectsHeaderSize := 12

			// For payload "B", there should be no write intents, so output starts immediately after header
			if len(result.Ok) > effectsHeaderSize {
				// Extract the call output (everything after ExecutionEffects header)
				callOutput := result.Ok[effectsHeaderSize:]
				log.Debug(log.Node, "executeSimulationWorkPackage: extracted call output",
					"gas_used", effects.GasUsed,
					"output_len", len(callOutput))
				return callOutput, nil
			}

			// No output, return empty
			return []byte{}, nil
		}
		if result.Err != 0 {
			return nil, fmt.Errorf("simulation error code: %d", result.Err)
		}
	}

	return nil, fmt.Errorf("no result from simulation")
}

// SendRawTransaction submits a signed transaction to the mempool
func (n *NodeContent) SendRawTransaction(signedTxData []byte) (common.Hash, error) {
	// Parse the raw transaction
	tx, err := ParseRawTransaction(signedTxData)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to parse transaction: %v", err)
	}

	// Recover sender from signature
	sender, err := tx.RecoverSender()
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to recover sender: %v", err)
	}
	tx.From = sender

	// Get or create transaction pool
	if n.txPool == nil {
		n.txPool = NewTxPool()
		log.Info(log.Node, "SendRawTransaction: Created new TxPool")
	}

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

	// Add transaction to mempool
	err = n.txPool.AddTransaction(tx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to add transaction to mempool: %v", err)
	}

	log.Info(log.Node, "SendRawTransaction: Transaction added to mempool",
		"hash", tx.Hash.String(),
		"from", tx.From.String(),
		"nonce", tx.Nonce)

	return tx.Hash, nil
}

// findBlockForTransaction searches the last maxSearchDepth blocks to find which block contains txHash
// Returns: blockNumber, blockHash, txIndex, error
func (n *NodeContent) findBlockForTransaction(txHash common.Hash, maxSearchDepth uint32) (uint32, string, string, error) {
	latestBlock, err := n.getLatestBlockNumber()
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to get latest block number: %v", err)
	}

	// Determine search range
	startBlock := uint32(1)
	if latestBlock > maxSearchDepth {
		startBlock = latestBlock - maxSearchDepth
	}

	// Search backwards from latest block
	for blockNum := latestBlock; blockNum >= startBlock; blockNum-- {
		txHashes, err := n.readBlockFromStorage(blockNum)
		if err != nil {
			// Block not found or error reading - continue to previous block
			if blockNum == 1 {
				break // Reached genesis, stop
			}
			continue
		}

		// Check if txHash is in this block
		for i, hash := range txHashes {
			if hash == txHash {
				// Found it! Calculate block hash and return metadata
				blockHashBytes := common.Blake2Hash(binary.LittleEndian.AppendUint32(nil, blockNum))
				txIndex := fmt.Sprintf("0x%x", i)
				return blockNum, blockHashBytes.String(), txIndex, nil
			}
		}

		if blockNum == 1 {
			break // Avoid underflow when reaching genesis
		}
	}

	return 0, "", "", fmt.Errorf("transaction not found in last %d blocks", maxSearchDepth)
}

// GetTransactionReceipt fetches a transaction receipt
func (n *NodeContent) GetTransactionReceipt(txHash common.Hash) (*EthereumTransactionReceipt, error) {
	// Use ReadObject to get receipt from DA
	receiptObjectID := tx_to_objectID(txHash)
	witness, found, err := n.statedb.ReadStateWitness(statedb.EVMServiceCode, receiptObjectID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to read transaction receipt: %v", err)
	}

	if !found {
		return nil, nil // Transaction not found
	}

	// Parse raw receipt
	receipt, err := parseRawReceipt(witness.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse receipt: %v", err)
	}

	// Parse transaction from receipt payload (RLP-encoded transaction)
	ethTx, err := convertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction from receipt: %v", err)
	}

	// Search last 20 blocks to find which block contains this transaction
	const maxSearchDepth = 20
	blockNum, blockHash, txIndex, err := n.findBlockForTransaction(txHash, maxSearchDepth)
	if err != nil {
		// Transaction receipt exists in DA but not found in any block
		// This could happen if blocks haven't been written yet during accumulate
		log.Warn(log.Node, "GetTransactionReceipt: Transaction receipt exists but block not found",
			"txHash", txHash.String(), "error", err)
		return nil, fmt.Errorf("transaction block not found: %v", err)
	}

	blockNumber := fmt.Sprintf("0x%x", blockNum)

	// Calculate block-scoped log index by counting logs from all previous transactions in block
	var logIndexCounter uint64 = 0

	// Read the block to get all transaction hashes
	blockTxHashes, err := n.readBlockFromStorage(blockNum)
	if err == nil {
		// Count logs from transactions 0 to txIndex-1
		txIndexInt := 0
		fmt.Sscanf(txIndex, "0x%x", &txIndexInt)

		for i := 0; i < txIndexInt && i < len(blockTxHashes); i++ {
			prevTxHash := blockTxHashes[i]
			prevReceiptObjectID := tx_to_objectID(prevTxHash)
			prevWitness, found, err := n.statedb.ReadStateWitness(statedb.EVMServiceCode, prevReceiptObjectID, true)
			if err == nil && found {
				prevReceipt, err := parseRawReceipt(prevWitness.Payload)
				if err == nil && len(prevReceipt.LogsData) >= 2 {
					// Count logs in this receipt (first 2 bytes = log count)
					logCount := binary.LittleEndian.Uint16(prevReceipt.LogsData[0:2])
					logIndexCounter += uint64(logCount)
				}
			}
		}
	}

	// Parse logs from receipt LogsData if available
	var logs []EthereumLog
	if len(receipt.LogsData) > 0 {
		logs, err = parseLogsFromReceipt(receipt.LogsData, txHash, blockNumber, blockHash, txIndex, &logIndexCounter)
		if err != nil {
			log.Warn(log.Node, "GetTransactionReceipt: Failed to parse logs", "error", err)
			logs = []EthereumLog{} // Use empty logs on parse failure
		}
	}

	// Determine contract address for contract creation
	var contractAddress *string
	if ethTx.To == nil {
		// Contract creation - calculate contract address from sender + nonce
		// TODO: This needs proper nonce from the transaction
		contractAddr := "0x0000000000000000000000000000000000000000" // Placeholder
		contractAddress = &contractAddr
	}

	txType := "0x0"
	if payload := receipt.Payload; len(payload) > 0 && payload[0] < 0x80 {
		txType = fmt.Sprintf("0x%x", payload[0])
	}

	// Build Ethereum receipt with transaction details from parsed RLP transaction
	ethReceipt := &EthereumTransactionReceipt{
		TransactionHash:   txHash.String(),
		TransactionIndex:  txIndex,
		BlockHash:         blockHash,
		BlockNumber:       blockNumber,
		From:              ethTx.From,
		To:                ethTx.To,
		CumulativeGasUsed: fmt.Sprintf("0x%x", receipt.UsedGas), // TODO: Calculate cumulative gas across block
		GasUsed:           fmt.Sprintf("0x%x", receipt.UsedGas),
		ContractAddress:   contractAddress,
		Logs:              logs,
		LogsBloom:         "", // TODO
		Status:            boolToHexStatus(receipt.Success),
		EffectiveGasPrice: ethTx.GasPrice,
		Type:              txType,
	}

	return ethReceipt, nil
}

// GetTransactionByHash fetches a transaction by hash
func (n *NodeContent) GetTransactionByHash(txHash common.Hash) (*EthereumTransactionResponse, error) {
	ethTx, err := n.getTransactionByHash(txHash)
	if err != nil {
		return nil, err
	}
	return ethTx, nil
}

// parseRawTransactionBytes parses a raw signed transaction from bytes
func parseRawTransactionBytes(rawTxBytes []byte) (*EthereumTransaction, error) {
	// Decode transaction (handles both legacy RLP and typed transactions)
	var ethTx ethereumTypes.Transaction
	if err := ethTx.UnmarshalBinary(rawTxBytes); err != nil {
		return nil, fmt.Errorf("failed to decode transaction: %v", err)
	}

	// Extract signature values
	v, r, s := ethTx.RawSignatureValues()

	// Convert 'to' address
	var to *common.Address
	if ethTx.To() != nil {
		addr := common.BytesToAddress(ethTx.To().Bytes())
		to = &addr
	}

	// Create our transaction structure
	tx := &EthereumTransaction{
		Hash:       common.BytesToHash(ethTx.Hash().Bytes()),
		From:       common.Address{}, // Will be recovered from signature
		To:         to,
		Value:      ethTx.Value(),
		Gas:        ethTx.Gas(),
		GasPrice:   ethTx.GasPrice(),
		Nonce:      ethTx.Nonce(),
		Data:       ethTx.Data(),
		V:          v,
		R:          r,
		S:          s,
		ReceivedAt: time.Now(),
		Size:       uint64(len(rawTxBytes)),
		inner:      &ethTx, // Store original transaction for type-aware operations
	}
	return tx, nil
}

// ParseRawTransaction parses a raw signed transaction from bytes
func ParseRawTransaction(rawTxBytes []byte) (*EthereumTransaction, error) {
	return parseRawTransactionBytes(rawTxBytes)
}

// RecoverSender recovers the sender address from transaction signature
func (tx *EthereumTransaction) RecoverSender() (common.Address, error) {
	// Validate signature values
	if tx.V == nil || tx.R == nil || tx.S == nil {
		return common.Address{}, fmt.Errorf("missing signature components")
	}

	// If we have the inner transaction, use it directly with LatestSignerForChainID
	// which automatically selects the correct signer for typed transactions
	if tx.inner != nil {
		// Use LatestSignerForChainID to support all transaction types (legacy, EIP-2930, EIP-1559)
		chainID := tx.inner.ChainId()
		if chainID == nil {
			// For unprotected transactions, try Homestead signer
			from, err := ethereumTypes.Sender(ethereumTypes.HomesteadSigner{}, tx.inner)
			if err != nil {
				return common.Address{}, fmt.Errorf("failed to recover sender: %v", err)
			}
			recoveredAddr := common.BytesToAddress(from.Bytes())
			log.Debug(log.Node, "RecoverSender (Homestead)", "hash", tx.Hash.String(), "from", recoveredAddr.String())
			return recoveredAddr, nil
		}

		signer := ethereumTypes.LatestSignerForChainID(chainID)
		from, err := ethereumTypes.Sender(signer, tx.inner)
		if err != nil {
			return common.Address{}, fmt.Errorf("failed to recover sender: %v", err)
		}
		recoveredAddr := common.BytesToAddress(from.Bytes())
		log.Debug(log.Node, "RecoverSender", "hash", tx.Hash.String(), "from", recoveredAddr.String(), "type", tx.inner.Type())
		return recoveredAddr, nil
	}

	// Fallback for legacy code path: reconstruct transaction
	// Create signer based on V value (EIP-155 or Homestead)
	var signer ethereumTypes.Signer
	if tx.V.Sign() != 0 && isProtectedV(tx.V) {
		// EIP-155 transaction with chain ID
		chainID := deriveChainId(tx.V)
		signer = ethereumTypes.NewEIP155Signer(chainID)
	} else {
		// Pre-EIP155 homestead transaction
		signer = ethereumTypes.HomesteadSigner{}
	}

	// Reconstruct the transaction for signature recovery
	var to *common.Address
	if tx.To != nil {
		ethAddr := (ethereumCommon.Address)(*tx.To)
		to = (*common.Address)(&ethAddr)
	}

	// Create go-ethereum transaction
	ethTx := ethereumTypes.NewTx(&ethereumTypes.LegacyTx{
		Nonce:    tx.Nonce,
		GasPrice: tx.GasPrice,
		Gas:      tx.Gas,
		To:       (*ethereumCommon.Address)(to),
		Value:    tx.Value,
		Data:     tx.Data,
		V:        tx.V,
		R:        tx.R,
		S:        tx.S,
	})

	// Recover sender address using the signer
	from, err := ethereumTypes.Sender(signer, ethTx)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to recover sender: %v", err)
	}

	recoveredAddr := common.BytesToAddress(from.Bytes())
	log.Debug(log.Node, "RecoverSender (legacy fallback)", "hash", tx.Hash.String(), "from", recoveredAddr.String())

	return recoveredAddr, nil
}

// isProtectedV checks if V value indicates an EIP-155 transaction
func isProtectedV(v *big.Int) bool {
	if v.BitLen() <= 8 {
		vInt := v.Uint64()
		return vInt != 27 && vInt != 28
	}
	// Anything larger than 8 bits must be protected (chain ID encoding)
	return true
}

// deriveChainId derives the chain ID from V value for EIP-155 transactions
func deriveChainId(v *big.Int) *big.Int {
	if v.BitLen() <= 64 {
		vInt := v.Uint64()
		if vInt == 27 || vInt == 28 {
			return new(big.Int)
		}
		return new(big.Int).SetUint64((vInt - 35) / 2)
	}
	// V = CHAIN_ID * 2 + 35 + {0, 1}
	// CHAIN_ID = (V - 35) / 2
	chainID := new(big.Int).Sub(v, big.NewInt(35))
	chainID.Div(chainID, big.NewInt(2))
	return chainID
}

// VerifySignature verifies the transaction signature against a public key
func (tx *EthereumTransaction) VerifySignature(pubkey *ecdsa.PublicKey) bool {
	// Recover the sender address from the signature
	sender, err := tx.RecoverSender()
	if err != nil {
		log.Warn(log.Node, "VerifySignature: failed to recover sender", "error", err)
		return false
	}

	// Derive address from public key using crypto.PubkeyToAddress
	expectedAddr := crypto.PubkeyToAddress(*pubkey)

	// Compare recovered sender with expected address
	isValid := sender == common.BytesToAddress(expectedAddr.Bytes())

	log.Debug(log.Node, "VerifySignature", "hash", tx.Hash.String(), "valid", isValid, "sender", sender.String())

	return isValid
}

// ToJSON returns a JSON representation of the transaction
func (tx *EthereumTransaction) ToJSON() string {
	// Create a JSON-compatible representation with proper hex encoding
	jsonTx := struct {
		Hash     string  `json:"hash"`
		From     string  `json:"from"`
		To       *string `json:"to"`
		Value    string  `json:"value"`
		Gas      string  `json:"gas"`
		GasPrice string  `json:"gasPrice"`
		Nonce    string  `json:"nonce"`
		Data     string  `json:"data"`
		V        string  `json:"v,omitempty"`
		R        string  `json:"r,omitempty"`
		S        string  `json:"s,omitempty"`
	}{
		Hash:     tx.Hash.String(),
		From:     tx.From.String(),
		Value:    "0x" + tx.Value.Text(16),
		Gas:      fmt.Sprintf("0x%x", tx.Gas),
		GasPrice: "0x" + tx.GasPrice.Text(16),
		Nonce:    fmt.Sprintf("0x%x", tx.Nonce),
		Data:     "0x" + hex.EncodeToString(tx.Data),
	}

	// Set 'to' field (null for contract creation)
	if tx.To != nil {
		toStr := tx.To.String()
		jsonTx.To = &toStr
	}

	// Include signature values if present
	if tx.V != nil && tx.V.Sign() != 0 {
		jsonTx.V = "0x" + tx.V.Text(16)
	}
	if tx.R != nil && tx.R.Sign() != 0 {
		jsonTx.R = "0x" + tx.R.Text(16)
	}
	if tx.S != nil && tx.S.Sign() != 0 {
		jsonTx.S = "0x" + tx.S.Text(16)
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(jsonTx)
	if err != nil {
		log.Warn(log.Node, "ToJSON: failed to marshal transaction", "error", err)
		return "{}"
	}

	return string(jsonBytes)
}

// createSignedUSDMTransfer creates a signed transaction that transfers USDM tokens
// Returns the parsed EthereumTransaction, RLP-encoded bytes, transaction hash, and error
func createSignedUSDMTransfer(privateKeyHex string, nonce uint64, to common.Address, amount *big.Int, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*EthereumTransaction, []byte, common.Hash, error) {
	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	// USDM transfer function signature: transfer(address,uint256)
	// Function selector: 0xa9059cbb
	calldata := make([]byte, 68)
	copy(calldata[0:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) // transfer(address,uint256) selector

	// Encode recipient address (32 bytes, left-padded)
	copy(calldata[16:36], to.Bytes())

	// Encode amount (32 bytes)
	amountBytes := amount.FillBytes(make([]byte, 32))
	copy(calldata[36:68], amountBytes)

	// Create transaction to USDM contract
	ethTx := ethereumTypes.NewTransaction(
		nonce,
		ethereumCommon.Address(usdmAddress),
		big.NewInt(0), // value = 0 for token transfer
		gasLimit,
		gasPrice,
		calldata,
	)

	// Sign transaction
	signer := ethereumTypes.NewEIP155Signer(big.NewInt(int64(chainID)))
	signedTx, err := ethereumTypes.SignTx(ethTx, signer, privateKey)
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	// Encode to RLP
	rlpBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	// Calculate transaction hash (Ethereum uses Keccak256)
	txHash := common.Keccak256(rlpBytes)

	// Parse into EthereumTransaction
	tx, err := parseRawTransactionBytes(rlpBytes)
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	// Recover sender from signature
	sender, err := tx.RecoverSender()
	if err != nil {
		return nil, nil, common.Hash{}, err
	}
	tx.From = sender

	return tx, rlpBytes, txHash, nil
}
