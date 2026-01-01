package rpc

import (
	"fmt"
	"strconv"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	evmtypes "github.com/colorfulnotion/jam/statedb/evmtypes"
)

// EVMClient provides EVM-specific RPC client methods
type EVMClient struct {
	client RPCClient // Generic RPC client interface
}

// NewEVMClient creates a new EVM RPC client
func NewEVMClient(client RPCClient) *EVMClient {
	return &EVMClient{
		client: client,
	}
}

// RPCClient is the interface for making RPC calls
type RPCClient interface {
	Call(method string, params interface{}, result interface{}) error
}

// Ethereum internal methods stub implementation on EVMClient
// These will call the remote node via RPC when implemented

// GetChainId returns the chain ID for the current network
func (c *EVMClient) GetChainId() uint64 {
	var result string
	err := c.client.Call("eth_chainId", []string{}, &result)
	if err != nil {
		log.Warn(log.Node, "EVMClient.GetChainId RPC failed, using default", "err", err)
		return 0
	}

	// Parse hex string to uint64
	chainID, err := strconv.ParseUint(result[2:], 16, 64)
	if err != nil {
		log.Warn(log.Node, "EVMClient.GetChainId parse failed, using default", "err", err)
		return 0
	}

	return chainID
}

// GetAccounts returns the list of addresses owned by the client
func (c *EVMClient) GetAccounts() []common.Address {
	var result []string
	err := c.client.Call("eth_accounts", []string{}, &result)
	if err != nil {
		log.Warn(log.Node, "EVMClient.GetAccounts RPC failed", "err", err)
		return []common.Address{}
	}

	addresses := make([]common.Address, len(result))
	for i, addrStr := range result {
		addresses[i] = common.HexToAddress(addrStr)
	}
	return addresses
}

// GetGasPrice returns the current gas price in wei
func (c *EVMClient) GetGasPrice() uint64 {
	var result string
	err := c.client.Call("eth_gasPrice", []string{}, &result)
	if err != nil {
		log.Warn(log.Node, "EVMClient.GetGasPrice RPC failed, using default", "err", err)
		return 1000000000 // 1 gwei
	}

	gasPrice, err := strconv.ParseUint(result[2:], 16, 64)
	if err != nil {
		log.Warn(log.Node, "EVMClient.GetGasPrice parse failed, using default", "err", err)
		return 1000000000
	}

	return gasPrice
}

// GetBalance fetches the balance of an address from remote node
func (c *EVMClient) GetBalance(address common.Address, blockNumber string) (common.Hash, error) {
	var result string
	err := c.client.Call("eth_getBalance", []string{address.String(), blockNumber}, &result)
	if err != nil {
		return common.Hash{}, fmt.Errorf("eth_getBalance RPC failed: %v", err)
	}

	return common.HexToHash(result), nil
}

// GetStorageAt reads contract storage at a specific position from remote node
func (c *EVMClient) GetStorageAt(address common.Address, position common.Hash, blockNumber string) (common.Hash, error) {
	var result string
	err := c.client.Call("eth_getStorageAt", []string{address.String(), position.String(), blockNumber}, &result)
	if err != nil {
		return common.Hash{}, fmt.Errorf("eth_getStorageAt RPC failed: %v", err)
	}

	return common.HexToHash(result), nil
}

// GetTransactionCount fetches the nonce of an address from remote node
func (c *EVMClient) GetTransactionCount(address common.Address, blockNumber string) (uint64, error) {
	var result string
	err := c.client.Call("eth_getTransactionCount", []string{address.String(), blockNumber}, &result)
	if err != nil {
		return 0, fmt.Errorf("eth_getTransactionCount RPC failed: %v", err)
	}

	nonce, err := strconv.ParseUint(result[2:], 16, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse nonce: %v", err)
	}

	return nonce, nil
}

// GetCode returns the bytecode at a given address from remote node
func (c *EVMClient) GetCode(address common.Address, blockNumber string) ([]byte, error) {
	var result string
	err := c.client.Call("eth_getCode", []string{address.String(), blockNumber}, &result)
	if err != nil {
		return nil, fmt.Errorf("eth_getCode RPC failed: %v", err)
	}

	return common.FromHex(result), nil
}

// EstimateGas estimates the gas needed to execute a transaction via remote node
func (c *EVMClient) EstimateGas(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte) (uint64, error) {
	txObj := map[string]interface{}{
		"from": from.String(),
	}
	if to != nil {
		txObj["to"] = to.String()
	}
	if gas > 0 {
		txObj["gas"] = fmt.Sprintf("0x%x", gas)
	}
	if gasPrice > 0 {
		txObj["gasPrice"] = fmt.Sprintf("0x%x", gasPrice)
	}
	if value > 0 {
		txObj["value"] = fmt.Sprintf("0x%x", value)
	}
	if len(data) > 0 {
		txObj["data"] = common.Bytes2Hex(data)
	}

	var result string
	err := c.client.Call("eth_estimateGas", []interface{}{txObj}, &result)
	if err != nil {
		return 0, fmt.Errorf("eth_estimateGas RPC failed: %v", err)
	}

	estimatedGas, err := strconv.ParseUint(result[2:], 16, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse gas estimate: %v", err)
	}

	return estimatedGas, nil
}

// Call simulates a transaction execution via remote node
func (c *EVMClient) Call(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, blockNumber string) ([]byte, error) {
	txObj := map[string]interface{}{
		"from": from.String(),
	}
	if to != nil {
		txObj["to"] = to.String()
	}
	if gas > 0 {
		txObj["gas"] = fmt.Sprintf("0x%x", gas)
	}
	if gasPrice > 0 {
		txObj["gasPrice"] = fmt.Sprintf("0x%x", gasPrice)
	}
	if value > 0 {
		txObj["value"] = fmt.Sprintf("0x%x", value)
	}
	if len(data) > 0 {
		txObj["data"] = common.Bytes2Hex(data)
	}

	var result string
	err := c.client.Call("eth_call", []interface{}{txObj, blockNumber}, &result)
	if err != nil {
		return nil, fmt.Errorf("eth_call RPC failed: %v", err)
	}

	return common.FromHex(result), nil
}

// SendRawTransaction submits a signed transaction via remote node
func (c *EVMClient) SendRawTransaction(signedTxData []byte) (common.Hash, error) {
	signedTxHex := common.Bytes2Hex(signedTxData)

	var result string
	err := c.client.Call("eth_sendRawTransaction", []string{signedTxHex}, &result)
	if err != nil {
		return common.Hash{}, fmt.Errorf("eth_sendRawTransaction RPC failed: %v", err)
	}

	return common.HexToHash(result), nil
}

// GetTransactionReceipt fetches a transaction receipt from remote node
func (c *EVMClient) GetTransactionReceipt(txHash common.Hash) (*evmtypes.EthereumTransactionReceipt, error) {
	var receipt evmtypes.EthereumTransactionReceipt
	err := c.client.Call("eth_getTransactionReceipt", []string{txHash.String()}, &receipt)
	if err != nil {
		return nil, fmt.Errorf("eth_getTransactionReceipt RPC failed: %v", err)
	}

	return &receipt, nil
}

// GetTransactionByHash fetches a transaction by hash from remote node
func (c *EVMClient) GetTransactionByHash(txHash common.Hash) (*evmtypes.EthereumTransactionResponse, error) {
	var tx evmtypes.EthereumTransactionResponse
	err := c.client.Call("eth_getTransactionByHash", []string{txHash.String()}, &tx)
	if err != nil {
		return nil, fmt.Errorf("eth_getTransactionByHash RPC failed: %v", err)
	}

	return &tx, nil
}

// GetLogs fetches event logs matching a filter from remote node
func (c *EVMClient) GetLogs(fromBlock, toBlock uint32, addresses []common.Address, topics [][]common.Hash) ([]evmtypes.EthereumLog, error) {
	filter := map[string]interface{}{
		"fromBlock": fmt.Sprintf("0x%x", fromBlock),
		"toBlock":   fmt.Sprintf("0x%x", toBlock),
	}

	if len(addresses) > 0 {
		addrStrs := make([]string, len(addresses))
		for i, addr := range addresses {
			addrStrs[i] = addr.String()
		}
		filter["address"] = addrStrs
	}

	if len(topics) > 0 {
		topicsArray := make([]interface{}, len(topics))
		for i, topicGroup := range topics {
			if len(topicGroup) > 0 {
				topicStrs := make([]string, len(topicGroup))
				for j, topic := range topicGroup {
					topicStrs[j] = topic.String()
				}
				topicsArray[i] = topicStrs
			} else {
				topicsArray[i] = nil
			}
		}
		filter["topics"] = topicsArray
	}

	var logs []evmtypes.EthereumLog
	err := c.client.Call("eth_getLogs", []interface{}{filter}, &logs)
	if err != nil {
		return nil, fmt.Errorf("eth_getLogs RPC failed: %v", err)
	}

	return logs, nil
}

// GetLatestBlockNumber fetches the latest block number from remote node
func (c *EVMClient) GetLatestBlockNumber() (uint32, error) {
	var result string
	err := c.client.Call("eth_blockNumber", []interface{}{}, &result)
	if err != nil {
		return 0, fmt.Errorf("eth_blockNumber RPC failed: %v", err)
	}

	// Parse hex string to uint32
	if len(result) < 2 || result[:2] != "0x" {
		return 0, fmt.Errorf("invalid block number format: %s", result)
	}

	blockNum, err := strconv.ParseUint(result[2:], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("failed to parse block number: %v", err)
	}

	return uint32(blockNum), nil
}

// GetTransactionByBlockHashAndIndex fetches a transaction by block hash and index from remote node
func (c *EVMClient) GetTransactionByBlockHashAndIndex(serviceID uint32, blockHash common.Hash, index uint32) (*evmtypes.EthereumTransactionResponse, error) {
	indexHex := fmt.Sprintf("0x%x", index)
	var tx evmtypes.EthereumTransactionResponse
	err := c.client.Call("eth_getTransactionByBlockHashAndIndex", []interface{}{blockHash.String(), indexHex}, &tx)
	if err != nil {
		return nil, fmt.Errorf("eth_getTransactionByBlockHashAndIndex RPC failed: %v", err)
	}

	return &tx, nil
}

// GetTransactionByBlockNumberAndIndex fetches a transaction by block number and index from remote node
func (c *EVMClient) GetTransactionByBlockNumberAndIndex(serviceID uint32, blockNumber string, index uint32) (*evmtypes.EthereumTransactionResponse, error) {
	indexHex := fmt.Sprintf("0x%x", index)
	var tx evmtypes.EthereumTransactionResponse
	err := c.client.Call("eth_getTransactionByBlockNumberAndIndex", []interface{}{blockNumber, indexHex}, &tx)
	if err != nil {
		return nil, fmt.Errorf("eth_getTransactionByBlockNumberAndIndex RPC failed: %v", err)
	}

	return &tx, nil
}

// GetBlockByHash fetches a block by hash from remote node
func (c *EVMClient) GetBlockByHash(blockHash common.Hash, fullTx bool) (*evmtypes.EthereumBlock, error) {
	var block evmtypes.EthereumBlock
	err := c.client.Call("eth_getBlockByHash", []interface{}{blockHash.String(), fullTx}, &block)
	if err != nil {
		return nil, fmt.Errorf("eth_getBlockByHash RPC failed: %v", err)
	}

	return &block, nil
}

// GetBlockByNumber fetches a block by number from remote node
func (c *EVMClient) GetBlockByNumber(blockNumber string, fullTx bool) (*evmtypes.EthereumBlock, error) {
	var block evmtypes.EthereumBlock
	err := c.client.Call("eth_getBlockByNumber", []interface{}{blockNumber, fullTx}, &block)
	if err != nil {
		return nil, fmt.Errorf("eth_getBlockByNumber RPC failed: %v", err)
	}

	return &block, nil
}
