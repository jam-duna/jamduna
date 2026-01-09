package wallet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	log "github.com/colorfulnotion/jam/log"
)

// OrchardRPCClient implements the RPCClient interface for Orchard RPC
type OrchardRPCClient struct {
	baseURL    string
	httpClient *http.Client

	// Statistics (protected by mutex)
	statsMu         sync.RWMutex
	totalCalls     int64
	successfulCalls int64
	errorCalls     int64
}

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      interface{} `json:"id"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewOrchardRPCClient creates a new RPC client
func NewOrchardRPCClient(baseURL string) *OrchardRPCClient {
	return &OrchardRPCClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SendMany calls z_sendmany RPC method
func (c *OrchardRPCClient) SendMany(from string, amounts []SendAmount) (string, error) {
	// Convert amounts to the expected format
	params := []interface{}{from, amounts}

	response, err := c.call("z_sendmany", params)
	if err != nil {
		return "", fmt.Errorf("z_sendmany RPC call failed: %w", err)
	}

	// Extract operation ID from response
	operationID, ok := response.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response format for z_sendmany: %v", response)
	}

	log.Debug(log.Node, "z_sendmany successful",
		"from", from,
		"recipients", len(amounts),
		"operation_id", operationID)

	return operationID, nil
}

// SendRawBundle calls z_sendraworchardbundle RPC method
func (c *OrchardRPCClient) SendRawBundle(hexBundle string) (string, error) {
	params := []interface{}{hexBundle}

	response, err := c.call("z_sendraworchardbundle", params)
	if err != nil {
		return "", fmt.Errorf("z_sendraworchardbundle RPC call failed: %w", err)
	}

	// Extract bundle ID from response
	bundleID, ok := response.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response format for z_sendraworchardbundle: %v", response)
	}

	log.Debug(log.Node, "z_sendraworchardbundle successful",
		"bundle_size", len(hexBundle),
		"bundle_id", bundleID)

	return bundleID, nil
}

// GetNewAddress calls z_getnewaddress RPC method
func (c *OrchardRPCClient) GetNewAddress() (string, error) {
	response, err := c.call("z_getnewaddress", []interface{}{})
	if err != nil {
		return "", fmt.Errorf("z_getnewaddress RPC call failed: %w", err)
	}

	// Extract address from response
	address, ok := response.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response format for z_getnewaddress: %v", response)
	}

	log.Debug(log.Node, "z_getnewaddress successful", "address", address)

	return address, nil
}

// GetMempoolInfo calls z_getmempoolinfo RPC method
func (c *OrchardRPCClient) GetMempoolInfo() (map[string]interface{}, error) {
	response, err := c.call("z_getmempoolinfo", []interface{}{})
	if err != nil {
		return nil, fmt.Errorf("z_getmempoolinfo RPC call failed: %w", err)
	}

	// Extract mempool info from response
	mempoolInfo, ok := response.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format for z_getmempoolinfo: %v", response)
	}

	log.Debug(log.Node, "z_getmempoolinfo successful", "info", mempoolInfo)

	return mempoolInfo, nil
}

// GetRawMempool calls z_getrawmempool RPC method
func (c *OrchardRPCClient) GetRawMempool(verbose bool) (interface{}, error) {
	params := []interface{}{verbose}

	response, err := c.call("z_getrawmempool", params)
	if err != nil {
		return nil, fmt.Errorf("z_getrawmempool RPC call failed: %w", err)
	}

	log.Debug(log.Node, "z_getrawmempool successful", "verbose", verbose)

	return response, nil
}

// GetBlockchainInfo calls getblockchaininfo RPC method
func (c *OrchardRPCClient) GetBlockchainInfo() (map[string]interface{}, error) {
	response, err := c.call("getblockchaininfo", []interface{}{})
	if err != nil {
		return nil, fmt.Errorf("getblockchaininfo RPC call failed: %w", err)
	}

	// Extract blockchain info from response
	blockchainInfo, ok := response.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format for getblockchaininfo: %v", response)
	}

	return blockchainInfo, nil
}

// GetOrchardBlockchainInfo calls z_getblockchaininfo RPC method
func (c *OrchardRPCClient) GetOrchardBlockchainInfo() (map[string]interface{}, error) {
	response, err := c.call("z_getblockchaininfo", []interface{}{})
	if err != nil {
		return nil, fmt.Errorf("z_getblockchaininfo RPC call failed: %w", err)
	}

	// Extract Orchard blockchain info from response
	orchardInfo, ok := response.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response format for z_getblockchaininfo: %v", response)
	}

	return orchardInfo, nil
}

// call performs the actual JSON-RPC call
func (c *OrchardRPCClient) call(method string, params interface{}) (interface{}, error) {
	c.statsMu.Lock()
	c.totalCalls++
	callID := c.totalCalls
	c.statsMu.Unlock()

	// Create JSON-RPC request
	request := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      callID,
	}

	// Marshal request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		c.statsMu.Lock()
		c.errorCalls++
		c.statsMu.Unlock()
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", c.baseURL, bytes.NewBuffer(requestBody))
	if err != nil {
		c.statsMu.Lock()
		c.errorCalls++
		c.statsMu.Unlock()
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	// Execute request
	start := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.statsMu.Lock()
		c.errorCalls++
		c.statsMu.Unlock()
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer httpResp.Body.Close()

	duration := time.Since(start)

	// Read response body
	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		c.statsMu.Lock()
		c.errorCalls++
		c.statsMu.Unlock()
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status
	if httpResp.StatusCode != http.StatusOK {
		c.statsMu.Lock()
		c.errorCalls++
		c.statsMu.Unlock()
		return nil, fmt.Errorf("HTTP error %d: %s", httpResp.StatusCode, string(responseBody))
	}

	// Parse JSON-RPC response
	var rpcResponse JSONRPCResponse
	if err := json.Unmarshal(responseBody, &rpcResponse); err != nil {
		c.statsMu.Lock()
		c.errorCalls++
		c.statsMu.Unlock()
		return nil, fmt.Errorf("failed to parse JSON-RPC response: %w", err)
	}

	// Check for RPC error
	if rpcResponse.Error != nil {
		c.statsMu.Lock()
		c.errorCalls++
		c.statsMu.Unlock()
		return nil, fmt.Errorf("RPC error %d: %s", rpcResponse.Error.Code, rpcResponse.Error.Message)
	}

	c.statsMu.Lock()
	c.successfulCalls++
	c.statsMu.Unlock()

	log.Debug(log.Node, "RPC call completed",
		"method", method,
		"duration", duration,
		"status", "success")

	return rpcResponse.Result, nil
}

// GetStats returns RPC client statistics
func (c *OrchardRPCClient) GetStats() map[string]interface{} {
	c.statsMu.RLock()
	totalCalls := c.totalCalls
	successfulCalls := c.successfulCalls
	errorCalls := c.errorCalls
	c.statsMu.RUnlock()

	successRate := float64(0)
	if totalCalls > 0 {
		successRate = float64(successfulCalls) / float64(totalCalls) * 100
	}

	return map[string]interface{}{
		"total_calls":     totalCalls,
		"successful_calls": successfulCalls,
		"error_calls":     errorCalls,
		"success_rate":    successRate,
		"base_url":        c.baseURL,
	}
}

// Ping tests connectivity to the RPC server
func (c *OrchardRPCClient) Ping() error {
	_, err := c.GetBlockchainInfo()
	if err != nil {
		return fmt.Errorf("RPC server ping failed: %w", err)
	}

	log.Info(log.Node, "RPC server ping successful", "url", c.baseURL)
	return nil
}

// SetTimeout configures the HTTP client timeout
func (c *OrchardRPCClient) SetTimeout(timeout time.Duration) {
	c.httpClient.Timeout = timeout
}

// MockRPCClient provides a mock implementation for testing
type MockRPCClient struct {
	sendManyResults    []string
	sendManyErrors     []error
	bundleResults      []string
	bundleErrors       []error
	addressResults     []string
	addressErrors      []error
	mempoolInfo        map[string]interface{}

	callIndex          int
	calls              []string // Track method calls
}

// NewMockRPCClient creates a new mock RPC client
func NewMockRPCClient() *MockRPCClient {
	return &MockRPCClient{
		sendManyResults: []string{"opid-12345"},
		sendManyErrors:  []error{nil},
		bundleResults:   []string{"bundle-67890"},
		bundleErrors:    []error{nil},
		addressResults:  []string{"orchard1address12345"},
		addressErrors:   []error{nil},
		mempoolInfo: map[string]interface{}{
			"size":         5,
			"bytes":        1024,
			"usage":        2048,
			"total_fee":    0.001,
			"max_mempool":  300000000,
			"mempool_min_fee": 0.00001,
		},
		calls: make([]string, 0),
	}
}

func (m *MockRPCClient) SendMany(from string, amounts []SendAmount) (string, error) {
	m.calls = append(m.calls, "z_sendmany")

	if m.callIndex < len(m.sendManyErrors) && m.sendManyErrors[m.callIndex] != nil {
		err := m.sendManyErrors[m.callIndex]
		m.callIndex++
		return "", err
	}

	if m.callIndex < len(m.sendManyResults) {
		result := m.sendManyResults[m.callIndex]
		m.callIndex++
		return result, nil
	}

	return "opid-default", nil
}

func (m *MockRPCClient) SendRawBundle(hexBundle string) (string, error) {
	m.calls = append(m.calls, "z_sendraworchardbundle")

	if m.callIndex < len(m.bundleErrors) && m.bundleErrors[m.callIndex] != nil {
		err := m.bundleErrors[m.callIndex]
		m.callIndex++
		return "", err
	}

	if m.callIndex < len(m.bundleResults) {
		result := m.bundleResults[m.callIndex]
		m.callIndex++
		return result, nil
	}

	return "bundle-default", nil
}

func (m *MockRPCClient) GetNewAddress() (string, error) {
	m.calls = append(m.calls, "z_getnewaddress")

	if m.callIndex < len(m.addressErrors) && m.addressErrors[m.callIndex] != nil {
		err := m.addressErrors[m.callIndex]
		m.callIndex++
		return "", err
	}

	if m.callIndex < len(m.addressResults) {
		result := m.addressResults[m.callIndex]
		m.callIndex++
		return result, nil
	}

	return "orchard1default", nil
}

func (m *MockRPCClient) GetMempoolInfo() (map[string]interface{}, error) {
	m.calls = append(m.calls, "z_getmempoolinfo")
	return m.mempoolInfo, nil
}

func (m *MockRPCClient) GetCalls() []string {
	return m.calls
}

func (m *MockRPCClient) ResetCalls() {
	m.calls = make([]string, 0)
	m.callIndex = 0
}