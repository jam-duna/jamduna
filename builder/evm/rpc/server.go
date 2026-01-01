package rpc

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	log "github.com/colorfulnotion/jam/log"
)

// EVMHTTPServer wraps the RPC handler and provides HTTP server functionality
type EVMHTTPServer struct {
	handler *EVMRPCHandler
}

// NewEVMHTTPServer creates a new EVM HTTP server
func NewEVMHTTPServer(handler *EVMRPCHandler) *EVMHTTPServer {
	return &EVMHTTPServer{
		handler: handler,
	}
}

// Start starts the EVM RPC server on the specified port
func (s *EVMHTTPServer) Start(port int) error {
	// Create HTTP mux for this server
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.handleJSONRPC(w, r)
	})

	// Start HTTP server
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	log.Info(log.Node, "EVM RPC server started", "port", port, "address", fmt.Sprintf("http://localhost:%d", port))

	// Serve in goroutine with dedicated mux
	go func() {
		if err := http.Serve(listener, mux); err != nil {
			log.Error(log.Node, "EVM RPC server error", "error", err)
		}
	}()

	return nil
}

// handleJSONRPC handles incoming JSON-RPC requests
func (s *EVMHTTPServer) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON-RPC request
	var req struct {
		JSONRPC string        `json:"jsonrpc"`
		Method  string        `json:"method"`
		Params  []interface{} `json:"params"`
		ID      interface{}   `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON-RPC request", http.StatusBadRequest)
		return
	}

	// Convert params to string array
	var stringParams []string
	for _, param := range req.Params {
		switch v := param.(type) {
		case string:
			stringParams = append(stringParams, v)
		case map[string]interface{}, []interface{}:
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				http.Error(w, "Failed to marshal param", http.StatusBadRequest)
				return
			}
			stringParams = append(stringParams, string(jsonBytes))
		default:
			stringParams = append(stringParams, fmt.Sprintf("%v", v))
		}
	}

	// Route to appropriate method
	var result string
	var err error

	switch req.Method {
	case "eth_chainId":
		err = s.handler.ChainId(stringParams, &result)
	case "eth_accounts":
		err = s.handler.Accounts(stringParams, &result)
	case "eth_gasPrice":
		err = s.handler.GasPrice(stringParams, &result)
	case "eth_getBalance":
		err = s.handler.GetBalance(stringParams, &result)
	case "eth_getStorageAt":
		err = s.handler.GetStorageAt(stringParams, &result)
	case "eth_getTransactionCount":
		err = s.handler.GetTransactionCount(stringParams, &result)
	case "eth_getCode":
		err = s.handler.GetCode(stringParams, &result)
	case "eth_estimateGas":
		err = s.handler.EstimateGas(stringParams, &result)
	case "eth_call":
		err = s.handler.Call(stringParams, &result)
	case "eth_sendRawTransaction":
		err = s.handler.SendRawTransaction(stringParams, &result)
	case "eth_getTransactionReceipt":
		err = s.handler.GetTransactionReceipt(stringParams, &result)
	case "eth_getTransactionByHash":
		err = s.handler.GetTransactionByHash(stringParams, &result)
	case "eth_getTransactionByBlockHashAndIndex":
		err = s.handler.GetTransactionByBlockHashAndIndex(stringParams, &result)
	case "eth_getTransactionByBlockNumberAndIndex":
		err = s.handler.GetTransactionByBlockNumberAndIndex(stringParams, &result)
	case "eth_getLogs":
		err = s.handler.GetLogs(stringParams, &result)
	case "eth_blockNumber":
		err = s.handler.BlockNumber(stringParams, &result)
	case "eth_getBlockByHash":
		err = s.handler.GetBlockByHash(stringParams, &result)
	case "eth_getBlockByNumber":
		err = s.handler.GetBlockByNumber(stringParams, &result)
	case "jam_txPoolStatus":
		err = s.handler.TxPoolStatus(stringParams, &result)
	case "jam_txPoolContent":
		err = s.handler.TxPoolContent(stringParams, &result)
	case "jam_txPoolInspect":
		err = s.handler.TxPoolInspect(stringParams, &result)
	case "jam_primeGenesis":
		err = s.handler.PrimeGenesis(stringParams, &result)
	case "jam_getWorkReport":
		err = s.handler.GetWorkReport(stringParams, &result)
	default:
		err = fmt.Errorf("unknown method: %s", req.Method)
	}

	// Build JSON-RPC response
	var response map[string]interface{}
	if err != nil {
		response = map[string]interface{}{
			"jsonrpc": "2.0",
			"error": map[string]interface{}{
				"code":    -32603,
				"message": err.Error(),
			},
			"id": req.ID,
		}
	} else {
		// Try to parse result as JSON, if it fails use as string
		var resultValue interface{}
		if err := json.Unmarshal([]byte(result), &resultValue); err != nil {
			// Not JSON, use as string
			resultValue = result
		}
		response = map[string]interface{}{
			"jsonrpc": "2.0",
			"result":  resultValue,
			"id":      req.ID,
		}
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
