package node

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
)

// ===== Network Metadata =====

// ChainId returns the chain ID for the current network
//
// Parameters: none
//
// Returns:
// - string: Chain ID as hex-encoded string (e.g., "0x1" for mainnet)
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
func (j *Jam) ChainId(req []string, res *string) error {
	log.Info(log.Node, "ChainId")

	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	// Call internal method
	chainId := rollup.GetChainId()
	*res = fmt.Sprintf("0x%x", chainId)

	log.Debug(log.Node, "ChainId: Returning chain ID", "chainId", *res)
	return nil
}

// Accounts returns the list of addresses owned by the client
//
// Parameters: none
//
// Returns:
// - string: JSON array of 20-byte addresses
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_accounts","params":[],"id":1}'
func (j *Jam) Accounts(req []string, res *string) error {
	log.Info(log.Node, "Accounts")

	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	// Call internal method
	accounts := rollup.GetAccounts()

	// Convert to JSON array
	accountsJSON := "["
	for i, addr := range accounts {
		if i > 0 {
			accountsJSON += ","
		}
		accountsJSON += `"` + addr.String() + `"`
	}
	accountsJSON += "]"
	*res = accountsJSON

	log.Debug(log.Node, "Accounts: Returning account list", "count", len(accounts))
	return nil
}

// GasPrice returns the current gas price in wei
//
// Parameters: none
//
// Returns:
// - string: Gas price as hex-encoded uint256
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_gasPrice","params":[],"id":1}'
func (j *Jam) GasPrice(req []string, res *string) error {
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	// Call internal method
	gasPrice := rollup.GetGasPrice()
	*res = fmt.Sprintf("0x%x", gasPrice)
	return nil
}

// ===== Contract State =====

// GetBalance returns the balance of an account at a given block number
//
// Parameters:
// - address (string): Address to query as hex-encoded string
// - blockNumber (string): Block number ("latest", "earliest", "pending", or hex number)
//
// Returns:
// - string: Balance in wei as hex-encoded uint256
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266","latest"],"id":1}'
func (j *Jam) GetBalance(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	addressStr := req[0]
	blockNumber := req[1]

	log.Debug(log.Node, "GetBalance", "address", addressStr, "blockNumber", blockNumber)

	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	// Parse address
	address := common.HexToAddress(addressStr)

	// Call internal method
	balance, err := rollup.GetBalance(address, blockNumber)
	if err != nil {
		return fmt.Errorf("failed to get balance: %v", err)
	}

	*res = balance.String()
	log.Debug(log.Node, "GetBalance: Returning balance", "balance", *res)
	return nil
}

// GetStorageAt returns the value from a storage position at a given address
//
// Parameters:
// - address (string): Address of the contract as hex-encoded string
// - position (string): Storage position as hex-encoded uint256
// - blockNumber (string): Block number ("latest", "earliest", "pending", or hex number)
//
// Returns:
// - string: Storage value as hex-encoded bytes32
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getStorageAt","params":["0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266","0x0","latest"],"id":1}'
func (j *Jam) GetStorageAt(req []string, res *string) error {
	if len(req) != 3 {
		return fmt.Errorf("invalid number of arguments: expected 3, got %d", len(req))
	}
	addressStr := req[0]
	positionStr := req[1]
	blockNumber := req[2]

	log.Debug(log.Node, "GetStorageAt", "address", addressStr, "position", positionStr, "blockNumber", blockNumber)

	// Parse address and position
	address := common.HexToAddress(addressStr)
	position := common.HexToHash(positionStr)

	// Call internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	value, err := rollup.GetStorageAt(address, position, blockNumber)
	if err != nil {
		return fmt.Errorf("failed to get storage: %v", err)
	}

	*res = value.String()
	log.Debug(log.Node, "GetStorageAt: Returning storage value", "value", *res)
	return nil
}

// GetTransactionCount returns the number of transactions sent from an address
//
// Parameters:
// - address (string): Address to query as hex-encoded string
// - blockNumber (string): Block number ("latest", "earliest", "pending", or hex number)
//
// Returns:
// - string: Transaction count (nonce) as hex-encoded uint256
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266","latest"],"id":1}'
func (j *Jam) GetTransactionCount(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	addressStr := req[0]
	blockNumber := req[1]

	log.Debug(log.Node, "GetTransactionCount", "address", addressStr, "blockNumber", blockNumber)

	// Parse address
	address := common.HexToAddress(addressStr)

	// Call internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	nonce, err := rollup.GetTransactionCount(address, blockNumber)
	if err != nil {
		return fmt.Errorf("failed to get transaction count: %v", err)
	}

	*res = fmt.Sprintf("0x%x", nonce)
	log.Debug(log.Node, "GetTransactionCount: Returning nonce", "nonce", *res)
	return nil
}

// GetCode returns the code at a given address
//
// Parameters:
// - address (string): Address to query as hex-encoded string
// - blockNumber (string): Block number ("latest", "earliest", "pending", or hex number)
//
// Returns:
// - string: Contract bytecode as hex-encoded bytes
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getCode","params":["0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266","latest"],"id":1}'
func (j *Jam) GetCode(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	addressStr := req[0]
	blockNumber := req[1]

	log.Debug(log.Node, "GetCode", "address", addressStr, "blockNumber", blockNumber)

	// Parse address
	address := common.HexToAddress(addressStr)

	// Call internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	code, err := rollup.GetCode(address, blockNumber)
	if err != nil {
		return fmt.Errorf("failed to get code: %v", err)
	}

	*res = "0x" + common.Bytes2Hex(code)
	log.Debug(log.Node, "GetCode: Returning code", "codeLen", len(code))
	return nil
}

// ===== Transaction Operations =====

// EstimateGas estimates the gas needed to execute a transaction
//
// Parameters:
// - txObj (object): Transaction call object with fields like from, to, data, value
// - blockNumber (string, optional): Block number for state context ("latest" if omitted)
//
// Returns:
// - string: Estimated gas as hex-encoded uint256
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_estimateGas","params":[{"from":"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266","to":"0x70997970C51812dc3A010C7d01b50e0d17dc79C8","value":"0x1000000000000000"}],"id":1}'
func (j *Jam) EstimateGas(req []string, res *string) error {
	if len(req) < 1 {
		return fmt.Errorf("invalid number of arguments: expected at least 1, got %d", len(req))
	}

	txObjJson := req[0]
	log.Info(log.Node, "EstimateGas", "txObj", txObjJson[:min(len(txObjJson), 100)])

	// Parse transaction object
	var txObj map[string]interface{}
	if err := json.Unmarshal([]byte(txObjJson), &txObj); err != nil {
		return fmt.Errorf("failed to parse transaction object: %v", err)
	}

	tx, err := evmtypes.ParseTransactionObject(txObj)
	if err != nil {
		return fmt.Errorf("failed to parse transaction fields: %v", err)
	}

	// Estimate gas using internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	gasEstimate, err := rollup.EstimateGas(
		tx.From,
		tx.To,
		tx.Gas,
		tx.GasPrice.Uint64(),
		tx.Value.Uint64(),
		tx.Data,
		statedb.BackendInterpreter,
	)
	if err != nil {
		return fmt.Errorf("failed to estimate gas: %v", err)
	}

	*res = fmt.Sprintf("0x%x", gasEstimate)
	log.Debug(log.Node, "EstimateGas: Returning estimate", "gas", *res)
	return nil
}

// Call simulates a transaction execution without submitting it to the network.
//
// Parameters:
// - transactionObject (object): Transaction call object with fields:
//   - from (optional): Sender address
//   - to (optional): Recipient address (null for contract creation)
//   - gas (optional): Gas limit (default: 21000)
//   - gasPrice (optional): Gas price in Wei (default: 1 Gwei)
//   - value (optional): Value in Wei (default: 0)
//   - data (optional): Transaction data (hex-encoded)
//
// - blockNumber (string): Block number for state context
//
// Returns:
// - string: Result data as hex-encoded bytes
//
// Implementation:
// - Creates simulated EVM environment using MajikBackend
// - Executes transaction in read-only mode against specified state
// - Returns execution result without modifying state
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x0000000000000000000000000000000000000001","data":"0x70a08231000000000000000000000000f39fd6e51aad88f6f4ce6ab8827279cfffb92266"},"latest"],"id":1}'
func (j *Jam) Call(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	txObjJson := req[0]
	blockNumberStr := req[1]

	log.Info(log.Node, "Call", "txObj", txObjJson[:min(len(txObjJson), 100)], "blockNumber", blockNumberStr)

	// 1. Parse the transaction object (to, from, data, gas, gasPrice, value)
	var txObj map[string]interface{}
	if err := json.Unmarshal([]byte(txObjJson), &txObj); err != nil {
		return fmt.Errorf("failed to parse transaction object: %v", err)
	}

	callTx, err := evmtypes.ParseTransactionObject(txObj)
	if err != nil {
		return fmt.Errorf("failed to parse transaction fields: %v", err)
	}

	log.Trace(log.Node, "Call: Parsed transaction", "to", func() string {
		if callTx.To != nil {
			return callTx.To.String()
		}
		return "nil"
	}(), "data_len", len(callTx.Data))

	// Execute call using internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	result, err := rollup.Call(
		callTx.From,
		callTx.To,
		callTx.Gas,
		callTx.GasPrice.Uint64(),
		callTx.Value.Uint64(),
		callTx.Data,
		blockNumberStr,
		statedb.BackendInterpreter,
	)
	if err != nil {
		log.Error(log.Node, "Call: Transaction simulation failed", "error", err)
		return fmt.Errorf("failed to execute simulation: %v", err)
	}

	// Return the result data as hex-encoded bytes
	*res = "0x" + common.Bytes2Hex(result)

	log.Info(log.Node, "Call: Transaction simulated successfully", "result_len", len(result))
	return nil
}

// SendRawTransaction submits a signed transaction to the Guarantor mempool.
//
// Parameters:
// - signedTxData (string): Signed transaction data as hex-encoded bytes
//
// Returns:
// - string: Transaction hash as hex-encoded bytes
//
// Implementation:
// - Parses and validates the signed transaction
// - Recovers sender address from signature
// - Adds transaction to Guarantor mempool for processing
// - Returns transaction hash for tracking
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["0xf86c808504a817c800825208940123456789abcdef0123456789abcdef0123456789880de0b6b3a76400008025a0c9cf86333bcb065d140032ecaab5d9281bde80f21b9687b3e94161de42d51895a0727a108a0b8d101465414033c3f705a9c7b826e596766046ee1183dbc8aeaa68"],"id":1}'
func (j *Jam) SendRawTransaction(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}
	signedTxDataHex := req[0]

	log.Info(log.Node, "SendRawTransaction", "signedTxData", signedTxDataHex[:min(len(signedTxDataHex), 64)])

	// Convert hex string to bytes
	signedTxData := common.FromHex(signedTxDataHex)

	// Submit transaction using internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	txHash, err := rollup.SendRawTransaction(signedTxData)
	if err != nil {
		log.Error(log.Node, "SendRawTransaction: Failed to submit transaction", "error", err)
		return fmt.Errorf("failed to submit transaction: %v", err)
	}

	*res = txHash.String()
	return nil
}

// ===== Transaction Queries =====

// GetTransactionReceipt fetches a transaction receipt from JAM State/DA.
//
// Parameters:
// - txHash (string): Transaction hash as hex-encoded string
//
// Returns:
// - object|null: Transaction receipt object or null if not found
//
// Implementation:
// - Parses the transaction hash
// - Queries JAM State/DA for the transaction receipt
// - Converts JAM transaction data to Ethereum receipt format
// - Returns receipt JSON or null if not found
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getTransactionReceipt","params":["0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"],"id":1}'
func (j *Jam) GetTransactionReceipt(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}
	txHashStr := req[0]

	log.Info(log.Node, "EthGetTransactionReceipt", "txHash", txHashStr)

	// Parse the transaction hash
	txHash := common.HexToHash(txHashStr)
	if txHash == (common.Hash{}) {
		log.Error(log.Node, "EthGetTransactionReceipt", "error", "invalid transaction hash", "txHash", txHashStr)
		*res = "null"
		return nil
	}

	// Get receipt using internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	ethReceipt, err := rollup.GetTransactionReceipt(txHash)
	if err != nil {
		log.Error(log.Node, "EthGetTransactionReceipt", "error", err, "txHash", txHashStr)
		return fmt.Errorf("failed to get transaction receipt: %v", err)
	}

	if ethReceipt == nil {
		log.Info(log.Node, "EthGetTransactionReceipt", "result", "transaction not found", "txHash", txHashStr)
		*res = "null"
		return nil
	}

	// Convert to JSON and return
	receiptJson, err := json.Marshal(ethReceipt)
	if err != nil {
		log.Error(log.Node, "EthGetTransactionReceipt", "error", "failed to marshal receipt", "err", err)
		return fmt.Errorf("failed to marshal receipt: %v", err)
	}

	*res = string(receiptJson)
	log.Info(log.Node, "EthGetTransactionReceipt", "result", "receipt found", "txHash", txHashStr)
	return nil
}

// GetTransactionByBlockHashAndIndex fetches a transaction by block hash and index
//
// Parameters:
// - blockHash (string): Block hash as hex-encoded string
// - index (string): Transaction index in block as hex-encoded uint
//
// Returns:
// - object|null: Transaction object or null if not found
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getTransactionByBlockHashAndIndex","params":["0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef","0x0"],"id":1}'
func (j *Jam) GetTransactionByBlockHashAndIndex(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	blockHashStr := req[0]
	indexStr := req[1]

	log.Info(log.Node, "GetTransactionByBlockHashAndIndex", "blockHash", blockHashStr, "index", indexStr)

	// Parse block hash
	blockHash := common.HexToHash(blockHashStr)

	// Parse transaction index
	if len(indexStr) < 2 || indexStr[:2] != "0x" {
		return fmt.Errorf("invalid index format: %s", indexStr)
	}
	index, err := strconv.ParseUint(indexStr[2:], 16, 64)
	if err != nil {
		return fmt.Errorf("failed to parse index: %v", err)
	}

	// Get transaction using internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	ethTx, err := rollup.GetTransactionByBlockHashAndIndex(blockHash, uint32(index))
	if err != nil {
		log.Warn(log.Node, "GetTransactionByBlockHashAndIndex: Failed", "error", err)
		*res = "null"
		return nil
	}

	if ethTx == nil {
		*res = "null"
		return nil
	}

	// Convert to JSON
	txBytes, err := json.Marshal(ethTx)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}

	*res = string(txBytes)
	log.Info(log.Node, "GetTransactionByBlockHashAndIndex: Found transaction", "blockHash", blockHashStr, "index", index)
	return nil
}

// GetTransactionByBlockNumberAndIndex fetches a transaction by block number and index
//
// Parameters:
// - blockNumber (string): Block number ("latest", "earliest", "pending", or hex number)
// - index (string): Transaction index in block as hex-encoded uint
//
// Returns:
// - object|null: Transaction object or null if not found
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getTransactionByBlockNumberAndIndex","params":["latest","0x0"],"id":1}'
func (j *Jam) GetTransactionByBlockNumberAndIndex(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	blockNumberStr := req[0]
	indexStr := req[1]

	log.Info(log.Node, "GetTransactionByBlockNumberAndIndex", "blockNumber", blockNumberStr, "index", indexStr)

	// Parse transaction index
	if len(indexStr) < 2 || indexStr[:2] != "0x" {
		return fmt.Errorf("invalid index format: %s", indexStr)
	}
	index, err := strconv.ParseUint(indexStr[2:], 16, 64)
	if err != nil {
		return fmt.Errorf("failed to parse index: %v", err)
	}

	// Get transaction using internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	ethTx, err := rollup.GetTransactionByBlockNumberAndIndex(blockNumberStr, uint32(index))
	if err != nil {
		log.Warn(log.Node, "GetTransactionByBlockNumberAndIndex: Failed", "error", err)
		*res = "null"
		return nil
	}

	if ethTx == nil {
		*res = "null"
		return nil
	}

	// Convert to JSON
	txBytes, err := json.Marshal(ethTx)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}

	*res = string(txBytes)
	log.Info(log.Node, "GetTransactionByBlockNumberAndIndex: Found transaction", "blockNumber", blockNumberStr, "index", index)
	return nil
}

// GetTransactionByHash fetches a transaction by hash from JAM State/DA.
//
// Parameters:
// - txHash (string): Transaction hash as hex-encoded string
//
// Returns:
// - object|null: Transaction object or null if not found
//
// Implementation:
// - Parses the transaction hash
// - Queries JAM State/DA for the transaction data
// - Converts JAM transaction data to Ethereum transaction format
// - Returns transaction JSON or null if not found
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"],"id":1}'
func (j *Jam) GetTransactionByHash(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}
	txHashStr := req[0]

	log.Info(log.Node, "EthGetTransactionByHash", "txHash", txHashStr)

	// Parse the transaction hash
	txHash := common.HexToHash(txHashStr)
	if txHash == (common.Hash{}) {
		log.Error(log.Node, "EthGetTransactionByHash", "error", "invalid transaction hash", "txHash", txHashStr)
		*res = "null"
		return nil
	}

	// Get transaction using internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	ethTx, err := rollup.GetTransactionByHash(txHash)
	if err != nil {
		log.Error(log.Node, "EthGetTransactionByHash", "error", err, "txHash", txHashStr)
		*res = "null"
		return nil
	}

	if ethTx == nil {
		log.Info(log.Node, "EthGetTransactionByHash", "result", "transaction not found", "txHash", txHashStr)
		*res = "null"
		return nil
	}

	// Convert to JSON
	txBytes, err := json.Marshal(ethTx)
	if err != nil {
		log.Error(log.Node, "EthGetTransactionByHash", "error", "failed to marshal transaction", "err", err)
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}

	*res = string(txBytes)
	log.Info(log.Node, "EthGetTransactionByHash", "result", "transaction found", "txHash", txHashStr)
	return nil
}

// GetLogs fetches event logs matching a filter from JAM State/DA.
//
// Parameters:
// - filterObject (object): Filter criteria with fields:
//   - fromBlock (optional): Starting block ("latest", "earliest", "pending", or hex number)
//   - toBlock (optional): Ending block ("latest", "earliest", "pending", or hex number)
//   - address (optional): Contract address(es) to filter by
//   - topics (optional): Array of topics to filter by
//
// Returns:
// - array: Array of matching log objects
//
// Implementation:
// - Parses the filter object (fromBlock, toBlock, address, topics)
// - Queries JAM State/DA for matching event logs
// - Filters logs by address and topic criteria
// - Returns the matching logs in Ethereum format
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0x1","toBlock":"latest","address":"0x0000000000000000000000000000000000000001"}],"id":1}'
func (j *Jam) GetLogs(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}
	filterJson := req[0]

	log.Info(log.Node, "EthGetLogs", "filter", filterJson)

	// 1. Parse the filter object
	var filter evmtypes.LogFilter
	if err := json.Unmarshal([]byte(filterJson), &filter); err != nil {
		return fmt.Errorf("failed to parse filter object: %v", err)
	}

	// 2. Resolve block range
	var fromBlock, toBlock uint32
	var err error

	// Resolve fromBlock
	if filter.FromBlock == nil {
		fromBlock = 1 // Start from genesis if not specified
	} else {
		fromBlock, err = j.parseBlockParameter(filter.FromBlock)
		if err != nil {
			return fmt.Errorf("invalid fromBlock: %v", err)
		}
	}

	// Resolve toBlock
	if filter.ToBlock == nil {
		toBlock, err = j.GetLatestBlockNumber()
		if err != nil {
			return fmt.Errorf("failed to get latest block: %v", err)
		}
	} else {
		toBlock, err = j.parseBlockParameter(filter.ToBlock)
		if err != nil {
			return fmt.Errorf("invalid toBlock: %v", err)
		}
	}

	// Validate range
	if fromBlock > toBlock {
		return fmt.Errorf("fromBlock (%d) cannot be greater than toBlock (%d)", fromBlock, toBlock)
	}

	// 3. Parse addresses from filter
	var addresses []common.Address
	if filter.Address != nil {
		switch addr := filter.Address.(type) {
		case string:
			addresses = append(addresses, common.HexToAddress(addr))
		case []interface{}:
			for _, a := range addr {
				if aStr, ok := a.(string); ok {
					addresses = append(addresses, common.HexToAddress(aStr))
				} else {
					return fmt.Errorf("invalid address in array: %v", a)
				}
			}
		default:
			return fmt.Errorf("invalid address filter type: %T", filter.Address)
		}
	}

	// 4. Parse topics from filter
	var topics [][]common.Hash
	if len(filter.Topics) > 0 {
		for i, topicFilter := range filter.Topics {
			if topicFilter == nil {
				// null means match any topic at this position
				topics = append(topics, []common.Hash{})
				continue
			}

			switch topic := topicFilter.(type) {
			case string:
				// Single topic - exact match required
				topics = append(topics, []common.Hash{common.HexToHash(topic)})
			case []interface{}:
				// Array of topics - OR match (any of these topics)
				var topicHashes []common.Hash
				for _, t := range topic {
					if tStr, ok := t.(string); ok {
						topicHashes = append(topicHashes, common.HexToHash(tStr))
					} else {
						return fmt.Errorf("invalid topic in array at position %d: %v", i, t)
					}
				}
				topics = append(topics, topicHashes)
			default:
				return fmt.Errorf("invalid topic filter type at position %d: %T", i, topicFilter)
			}
		}
	}

	log.Debug(log.Node, "GetLogs: Parameters", "fromBlock", fromBlock, "toBlock", toBlock, "addresses", len(addresses), "topics", len(topics))

	// 5. Call internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	allLogs, err := rollup.GetLogs(fromBlock, toBlock, addresses, topics)
	if err != nil {
		return fmt.Errorf("failed to get logs: %v", err)
	}

	// 6. Convert to JSON and return
	logsJson, err := json.Marshal(allLogs)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %v", err)
	}

	*res = string(logsJson)
	log.Info(log.Node, "GetLogs: Found logs", "count", len(allLogs), "fromBlock", fromBlock, "toBlock", toBlock)
	return nil
}

// ===== Block Queries =====

// BlockNumber returns the number of the most recent block
//
// Parameters: none
//
// Returns:
// - string: Block number as hex-encoded uint256
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
func (j *Jam) BlockNumber(req []string, res *string) error {
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	blockNumber, err := rollup.GetLatestBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to get latest block number: %v", err)
	}

	*res = fmt.Sprintf("0x%x", blockNumber)
	return nil
}

// GetBlockByHash fetches a JAM block with all EVM transactions across cores.
//
// Parameters:
// - blockHash (string): Block hash as hex-encoded string
// - fullTx (boolean): Include full transaction objects if true, otherwise just hashes
//
// Returns:
// - object|null: Block object with transactions or null if not found
//
// Implementation:
// - Parses the block hash
// - Queries JAM State for the block data
// - Collects all EVM transactions from all cores in that block
// - Converts to Ethereum block format
// - Includes full transaction objects if fullTx is true, otherwise just hashes
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",true],"id":1}'
func (j *Jam) GetBlockByHash(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	blockHash := req[0]
	fullTxStr := req[1]

	fullTx, err := strconv.ParseBool(fullTxStr)
	if err != nil {
		return fmt.Errorf("invalid fullTx parameter: %v", err)
	}

	log.Info(log.Node, "EthGetBlockByHash", "blockHash", blockHash, "fullTx", fullTx)

	// Parse the block hash
	hash := common.HexToHash(blockHash)

	// Call the implementation
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	block, err := rollup.GetBlockByHash(hash, fullTx)
	if err != nil {
		return fmt.Errorf("failed to get block by hash: %v", err)
	}

	if block == nil {
		*res = "null"
		return nil
	}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %v", err)
	}

	*res = string(jsonBytes)
	return nil
}

// GetBlockByNumber fetches a JAM block with all EVM transactions across cores.
//
// Parameters:
// - blockNumber (string): Block number ("latest", "earliest", "pending", or hex number)
// - fullTx (boolean): Include full transaction objects if true, otherwise just hashes
//
// Returns:
// - object|null: Block object with transactions or null if not found
//
// Implementation:
// - Parses and resolves the block number (latest, earliest, pending, or specific number)
// - Queries JAM State for the block data
// - Collects all EVM transactions from all cores in that block
// - Converts to Ethereum block format
// - Includes full transaction objects if fullTx is true, otherwise just hashes
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",true],"id":1}'
func (j *Jam) GetBlockByNumber(req []string, res *string) error {
	log.Info(log.Node, "GetBlockByNumber", "req", req)
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	blockNumber := req[0]
	fullTxStr := req[1]

	fullTx, err := strconv.ParseBool(fullTxStr)
	if err != nil {
		log.Warn(log.Node, "GetBlockByNumber: Failed to get block", "error", err)
		return fmt.Errorf("invalid fullTx parameter: %v", err)
	}

	log.Info(log.Node, "GetBlockByNumber", "blockNumber", blockNumber, "fullTx", fullTx)

	// Call internal method
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return fmt.Errorf("failed to get rollup: %v", err)
	}

	ethBlock, err := rollup.GetBlockByNumber(blockNumber, fullTx)
	if err != nil {
		log.Warn(log.Node, "GetBlockByNumber: Failed to get block", "error", err)
		*res = "null"
		return nil
	}

	// Convert to JSON
	blockJSON, err := json.Marshal(ethBlock)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %v", err)
	}

	*res = string(blockJSON)
	return nil
}

// GetLatestBlockNumber returns the number of most recent block
//
// Parameters: none
//
// Returns:
// - string: Block number as hex-encoded string (e.g., "0x1")
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
func (j *Jam) GetLatestBlockNumber() (uint32, error) {
	// Call public method (same as BlockNumber/eth_blockNumber)
	// Get rollup for this service
	rollup, err := j.GetRollup()
	if err != nil {
		return 0, fmt.Errorf("failed to get rollup: %v", err)
	}

	blockNumber, err := rollup.GetLatestBlockNumber()
	if err != nil {
		return 0, fmt.Errorf("failed to get latest block number: %v", err)
	}

	return blockNumber, nil
}

// ===== Helper Functions =====

// parseBlockParameter parses a block parameter (string or number) to uint32
func (j *Jam) parseBlockParameter(param interface{}) (uint32, error) {
	switch v := param.(type) {
	case string:
		switch v {
		case "latest":
			return j.GetLatestBlockNumber()
		case "earliest":
			return 1, nil // Genesis block
		case "pending":
			return j.GetLatestBlockNumber() // Use latest for pending
		default:
			// Parse hex number
			if len(v) >= 2 && v[:2] == "0x" {
				blockNum, err := strconv.ParseUint(v[2:], 16, 32)
				if err != nil {
					return 0, fmt.Errorf("invalid hex block number: %v", err)
				}
				return uint32(blockNum), nil
			}
			return 0, fmt.Errorf("invalid block parameter: %s", v)
		}
	case float64:
		return uint32(v), nil
	default:
		return 0, fmt.Errorf("unsupported block parameter type: %T", param)
	}
}

// isZeroAddress checks if an address is the zero address
func isZeroAddress(addr []byte) bool {
	for _, b := range addr {
		if b != 0 {
			return false
		}
	}
	return true
}

// Helper function for Go versions that don't have min builtin
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ===== Transaction Pool RPC Methods =====

// TxPoolStatus returns statistics about the transaction pool
//
// Parameters: none
//
// Returns:
// - string: JSON object with pool statistics
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"jam_txPoolStatus","params":[],"id":1}'
func (j *Jam) TxPoolStatus(req []string, res *string) error {
	log.Info(log.Node, "TxPoolStatus")

	if j.txPool == nil {
		*res = `{"pendingCount":0,"queuedCount":0,"totalReceived":0,"totalProcessed":0}`
		return nil
	}

	stats := j.txPool.GetStats()

	result := map[string]interface{}{
		"pendingCount":   stats.PendingCount,
		"queuedCount":    stats.QueuedCount,
		"totalReceived":  stats.TotalReceived,
		"totalProcessed": stats.TotalProcessed,
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal pool status: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "TxPoolStatus: Returning pool status", "result", *res)
	return nil
}

// TxPoolContent returns the pending and queued transactions
//
// Parameters: none
//
// Returns:
// - string: JSON object with pending and queued transaction arrays
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"jam_txPoolContent","params":[],"id":1}'
func (j *Jam) TxPoolContent(req []string, res *string) error {
	log.Info(log.Node, "TxPoolContent")

	if j.txPool == nil {
		*res = `{"pending":[],"queued":[]}`
		return nil
	}

	// Get pending and queued transactions from pool
	pending, queued := j.txPool.GetTxPoolContent()

	result := map[string]interface{}{
		"pending": pending,
		"queued":  queued,
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal pool content: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "TxPoolContent: Returning pool content", "pendingCount", len(pending), "queuedCount", len(queued))
	return nil
}

// TxPoolInspect returns a human-readable summary of the transaction pool
//
// Parameters: none
//
// Returns:
// - string: Human-readable pool summary
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"jam_txPoolInspect","params":[],"id":1}'
func (j *Jam) TxPoolInspect(req []string, res *string) error {
	log.Info(log.Node, "TxPoolInspect")

	if j.txPool == nil {
		*res = "Transaction Pool: 0 pending, 0 queued"
		return nil
	}

	stats := j.txPool.GetStats()

	*res = fmt.Sprintf("Transaction Pool:\n  Pending: %d\n  Queued: %d\n  Total Received: %d\n  Total Processed: %d",
		stats.PendingCount,
		stats.QueuedCount,
		stats.TotalReceived,
		stats.TotalProcessed,
	)

	log.Debug(log.Node, "TxPoolInspect: Returning pool inspect", "result", *res)
	return nil
}
