// Package rpc provides EVM user-facing RPC server functionality
package rpc

import (
	"encoding/json"
	"fmt"
	"net/rpc"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	evmwitness "github.com/colorfulnotion/jam/builder/evm/witness"
)

// EVMRPCServer provides Ethereum JSON-RPC API implementation
// Separated from JAM network RPC for clean namespace isolation
type EVMRPCServer struct {
	builder *evmwitness.EVMBuilder
}

// NewEVMRPCServer creates a new EVM RPC server instance
func NewEVMRPCServer(builder *evmwitness.EVMBuilder) *EVMRPCServer {
	return &EVMRPCServer{
		builder: builder,
	}
}

// ===== Network Metadata =====

// ChainId returns the chain ID for the current network
//
// Parameters: none
//
// Returns:
// - string: Chain ID as hex-encoded string (e.g., "0x1107" for JAM)
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'
func (s *EVMRPCServer) ChainId(req []string, res *string) error {
	log.Info(log.Node, "eth_chainId")

	// JAM chain ID is 0x1107 (4359)
	chainId := uint64(0x1107)
	*res = fmt.Sprintf("0x%x", chainId)

	log.Debug(log.Node, "eth_chainId: Returning chain ID", "chainId", *res)
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
func (s *EVMRPCServer) Accounts(req []string, res *string) error {
	log.Info(log.Node, "eth_accounts")

	// Return empty array for now - builders don't manage accounts directly
	accounts := []string{}

	accountsJSON, err := json.Marshal(accounts)
	if err != nil {
		return fmt.Errorf("failed to marshal accounts: %v", err)
	}

	*res = string(accountsJSON)

	log.Debug(log.Node, "eth_accounts: Returning accounts", "count", len(accounts))
	return nil
}

// GasPrice returns the current gas price
//
// Parameters: none
//
// Returns:
// - string: Gas price as hex-encoded string
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_gasPrice","params":[],"id":1}'
func (s *EVMRPCServer) GasPrice(req []string, res *string) error {
	log.Info(log.Node, "eth_gasPrice")

	// Default gas price for JAM
	gasPrice := uint64(1)
	*res = fmt.Sprintf("0x%x", gasPrice)

	log.Debug(log.Node, "eth_gasPrice: Returning gas price", "gasPrice", *res)
	return nil
}

// GetBalance returns the balance of the account of given address
//
// Parameters:
// - string: Address to check for balance
// - string: Block number ("latest", "earliest", "pending", or hex)
//
// Returns:
// - string: Balance as hex-encoded string
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0x742d35cc6bf8c5a9cf4e5db45b5c3a8b5b5e5b5e","latest"],"id":1}'
func (s *EVMRPCServer) GetBalance(req []string, res *string) error {
	log.Info(log.Node, "eth_getBalance")

	if len(req) < 2 {
		return fmt.Errorf("insufficient parameters: expected address and block number")
	}

	addressStr := req[0]
	blockNumber := req[1]

	// Parse address
	if len(addressStr) != 42 || addressStr[:2] != "0x" {
		return fmt.Errorf("invalid address format: %s", addressStr)
	}
	_ = common.HexToAddress(addressStr)

	// TODO: Implement balance lookup through builder
	// For now, return zero balance
	balance := "0x0"
	*res = balance

	log.Debug(log.Node, "eth_getBalance",
		"address", addressStr,
		"blockNumber", blockNumber,
		"balance", balance)

	return nil
}

// GetStorageAt returns the value from a storage position at a given address
//
// Parameters:
// - string: Address of the storage
// - string: Storage position
// - string: Block number ("latest", "earliest", "pending", or hex)
//
// Returns:
// - string: Storage value as hex-encoded string
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getStorageAt","params":["0x295a70b2de5e3953354a6a8344e616ed314d7251","0x0","latest"],"id":1}'
func (s *EVMRPCServer) GetStorageAt(req []string, res *string) error {
	log.Info(log.Node, "eth_getStorageAt")

	if len(req) < 3 {
		return fmt.Errorf("insufficient parameters: expected address, position, and block number")
	}

	addressStr := req[0]
	positionStr := req[1]
	blockNumber := req[2]

	// Parse address
	if len(addressStr) != 42 || addressStr[:2] != "0x" {
		return fmt.Errorf("invalid address format: %s", addressStr)
	}
	_ = common.HexToAddress(addressStr)

	// Parse storage position
	_ = common.HexToHash(positionStr)

	// TODO: Implement storage lookup through builder
	// For now, return zero value
	value := "0x0000000000000000000000000000000000000000000000000000000000000000"
	*res = value

	log.Debug(log.Node, "eth_getStorageAt",
		"address", addressStr,
		"position", positionStr,
		"blockNumber", blockNumber,
		"value", value)

	return nil
}

// GetTransactionCount returns the number of transactions sent from an address
//
// Parameters:
// - string: Address to get transaction count for
// - string: Block number ("latest", "earliest", "pending", or hex)
//
// Returns:
// - string: Transaction count as hex-encoded string
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0x742d35cc6bf8c5a9cf4e5db45b5c3a8b5b5e5b5e","latest"],"id":1}'
func (s *EVMRPCServer) GetTransactionCount(req []string, res *string) error {
	log.Info(log.Node, "eth_getTransactionCount")

	if len(req) < 2 {
		return fmt.Errorf("insufficient parameters: expected address and block number")
	}

	addressStr := req[0]
	blockNumber := req[1]

	// Parse address
	if len(addressStr) != 42 || addressStr[:2] != "0x" {
		return fmt.Errorf("invalid address format: %s", addressStr)
	}
	_ = common.HexToAddress(addressStr)

	// TODO: Implement nonce lookup through builder
	// For now, return zero nonce
	nonce := "0x0"
	*res = nonce

	log.Debug(log.Node, "eth_getTransactionCount",
		"address", addressStr,
		"blockNumber", blockNumber,
		"nonce", nonce)

	return nil
}

// GetCode returns code at a given address
//
// Parameters:
// - string: Address to get code for
// - string: Block number ("latest", "earliest", "pending", or hex)
//
// Returns:
// - string: Code as hex-encoded string
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getCode","params":["0x742d35cc6bf8c5a9cf4e5db45b5c3a8b5b5e5b5e","latest"],"id":1}'
func (s *EVMRPCServer) GetCode(req []string, res *string) error {
	log.Info(log.Node, "eth_getCode")

	if len(req) < 2 {
		return fmt.Errorf("insufficient parameters: expected address and block number")
	}

	addressStr := req[0]
	blockNumber := req[1]

	// Parse address
	if len(addressStr) != 42 || addressStr[:2] != "0x" {
		return fmt.Errorf("invalid address format: %s", addressStr)
	}
	_ = common.HexToAddress(addressStr)

	// TODO: Implement code lookup through builder
	// For now, return empty code
	code := "0x"
	*res = code

	log.Debug(log.Node, "eth_getCode",
		"address", addressStr,
		"blockNumber", blockNumber,
		"code", code)

	return nil
}

// BlockNumber returns the number of most recent block
//
// Parameters: none
//
// Returns:
// - string: Block number as hex-encoded string
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
func (s *EVMRPCServer) BlockNumber(req []string, res *string) error {
	log.Info(log.Node, "eth_blockNumber")

	// TODO: Implement latest block lookup through builder
	// For now, return block 0
	blockNumber := uint64(0)
	*res = fmt.Sprintf("0x%x", blockNumber)

	log.Debug(log.Node, "eth_blockNumber", "blockNumber", *res)
	return nil
}

// SendRawTransaction submits a signed transaction to the network
//
// Parameters:
// - string: Signed transaction data as hex-encoded string
//
// Returns:
// - string: Transaction hash as hex-encoded string
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["0xf86c8085174876e800825208945f7d8b..."],"id":1}'
func (s *EVMRPCServer) SendRawTransaction(req []string, res *string) error {
	log.Info(log.Node, "eth_sendRawTransaction")

	if len(req) < 1 {
		return fmt.Errorf("insufficient parameters: expected signed transaction data")
	}

	_ = req[0]

	// TODO: Implement transaction submission through builder
	// For now, return a dummy transaction hash
	txHash := "0x0000000000000000000000000000000000000000000000000000000000000000"
	*res = txHash

	log.Debug(log.Node, "eth_sendRawTransaction", "txHash", txHash)
	return nil
}

// EstimateGas generates and returns an estimate of how much gas is necessary
//
// Parameters:
// - object: Transaction call object
//
// Returns:
// - string: Gas estimate as hex-encoded string
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_estimateGas","params":[{"to":"0x742d35cc...","data":"0xa9059cbb..."}],"id":1}'
func (s *EVMRPCServer) EstimateGas(req []string, res *string) error {
	log.Info(log.Node, "eth_estimateGas")

	// TODO: Implement gas estimation through builder
	// For now, return a default gas limit
	gasLimit := uint64(21000)
	*res = fmt.Sprintf("0x%x", gasLimit)

	log.Debug(log.Node, "eth_estimateGas", "gasLimit", *res)
	return nil
}

// Call executes a new message call immediately without creating a transaction
//
// Parameters:
// - object: Transaction call object
// - string: Block number ("latest", "earliest", "pending", or hex)
//
// Returns:
// - string: Return value as hex-encoded string
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x742d35cc...","data":"0x70a08231..."},"latest"],"id":1}'
func (s *EVMRPCServer) Call(req []string, res *string) error {
	log.Info(log.Node, "eth_call")

	// TODO: Implement call execution through builder
	// For now, return empty result
	result := "0x"
	*res = result

	log.Debug(log.Node, "eth_call", "result", result)
	return nil
}

// GetTransactionReceipt returns the receipt of a transaction by transaction hash
//
// Parameters:
// - string: Transaction hash
//
// Returns:
// - object: Transaction receipt object or null
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getTransactionReceipt","params":["0x..."],"id":1}'
func (s *EVMRPCServer) GetTransactionReceipt(req []string, res *string) error {
	log.Info(log.Node, "eth_getTransactionReceipt")

	if len(req) < 1 {
		return fmt.Errorf("insufficient parameters: expected transaction hash")
	}

	txHashStr := req[0]

	// TODO: Implement receipt lookup through builder
	// For now, return null
	*res = "null"

	log.Debug(log.Node, "eth_getTransactionReceipt", "txHash", txHashStr, "receipt", "null")
	return nil
}

// GetTransactionByHash returns the information about a transaction by hash
//
// Parameters:
// - string: Transaction hash
//
// Returns:
// - object: Transaction object or null
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["0x..."],"id":1}'
func (s *EVMRPCServer) GetTransactionByHash(req []string, res *string) error {
	log.Info(log.Node, "eth_getTransactionByHash")

	if len(req) < 1 {
		return fmt.Errorf("insufficient parameters: expected transaction hash")
	}

	txHashStr := req[0]

	// TODO: Implement transaction lookup through builder
	// For now, return null
	*res = "null"

	log.Debug(log.Node, "eth_getTransactionByHash", "txHash", txHashStr, "transaction", "null")
	return nil
}

// GetBlockByNumber returns information about a block by block number
//
// Parameters:
// - string: Block number ("latest", "earliest", "pending", or hex)
// - bool: If true, returns full transaction objects; if false, only hashes
//
// Returns:
// - object: Block object or null
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",false],"id":1}'
func (s *EVMRPCServer) GetBlockByNumber(req []string, res *string) error {
	log.Info(log.Node, "eth_getBlockByNumber")

	if len(req) < 2 {
		return fmt.Errorf("insufficient parameters: expected block number and full transaction flag")
	}

	blockNumberStr := req[0]
	fullTxStr := req[1]

	// TODO: Implement block lookup through builder
	// For now, return null
	*res = "null"

	log.Debug(log.Node, "eth_getBlockByNumber",
		"blockNumber", blockNumberStr,
		"fullTx", fullTxStr,
		"block", "null")
	return nil
}

// GetBlockByHash returns information about a block by hash
//
// Parameters:
// - string: Block hash
// - bool: If true, returns full transaction objects; if false, only hashes
//
// Returns:
// - object: Block object or null
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0x...",false],"id":1}'
func (s *EVMRPCServer) GetBlockByHash(req []string, res *string) error {
	log.Info(log.Node, "eth_getBlockByHash")

	if len(req) < 2 {
		return fmt.Errorf("insufficient parameters: expected block hash and full transaction flag")
	}

	blockHashStr := req[0]
	fullTxStr := req[1]

	// TODO: Implement block lookup through builder
	// For now, return null
	*res = "null"

	log.Debug(log.Node, "eth_getBlockByHash",
		"blockHash", blockHashStr,
		"fullTx", fullTxStr,
		"block", "null")
	return nil
}

// GetLogs returns an array of logs matching given filter object
//
// Parameters:
// - object: Filter object with fromBlock, toBlock, address, topics
//
// Returns:
// - array: Array of log objects
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"latest","toBlock":"latest"}],"id":1}'
func (s *EVMRPCServer) GetLogs(req []string, res *string) error {
	log.Info(log.Node, "eth_getLogs")

	// TODO: Implement log filtering through builder
	// For now, return empty array
	logs := []interface{}{}
	logsJSON, err := json.Marshal(logs)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %v", err)
	}

	*res = string(logsJSON)

	log.Debug(log.Node, "eth_getLogs", "count", len(logs))
	return nil
}

// RegisterEthereumRPC registers all Ethereum RPC methods with the given RPC server
// This provides the 'eth' namespace for EVM operations
func RegisterEthereumRPC(server *rpc.Server, builder *evmwitness.EVMBuilder) error {
	evmRPC := NewEVMRPCServer(builder)

	// Register with 'eth' namespace
	err := server.RegisterName("eth", evmRPC)
	if err != nil {
		return fmt.Errorf("failed to register eth RPC namespace: %v", err)
	}

	log.Info(log.Node, "Registered Ethereum RPC namespace", "namespace", "eth")
	return nil
}