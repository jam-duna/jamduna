package rpc

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	log "github.com/colorfulnotion/jam/log"
)

// OrchardHTTPServer wraps the RPC handler and provides HTTP server functionality
type OrchardHTTPServer struct {
	handler *OrchardRPCHandler
}

// NewOrchardHTTPServer creates a new Orchard HTTP server
func NewOrchardHTTPServer(handler *OrchardRPCHandler) *OrchardHTTPServer {
	return &OrchardHTTPServer{
		handler: handler,
	}
}

// Start starts the Orchard RPC server on the specified port
func (s *OrchardHTTPServer) Start(port int) error {
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

	log.Info(log.Node, "Orchard RPC server started", "port", port, "address", fmt.Sprintf("http://localhost:%d", port))

	// Serve in goroutine with dedicated mux
	go func() {
		if err := http.Serve(listener, mux); err != nil {
			log.Error(log.Node, "Orchard RPC server error", "error", err)
		}
	}()

	return nil
}

// handleJSONRPC handles incoming JSON-RPC requests
func (s *OrchardHTTPServer) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
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
	case "getblockchaininfo":
		err = s.handler.GetBlockchainInfo(stringParams, &result)
	case "z_getblockchaininfo":
		err = s.handler.ZGetBlockchainInfo(stringParams, &result)
	case "getbestblockhash":
		err = s.handler.GetBestBlockHash(stringParams, &result)
	case "getblockcount":
		err = s.handler.GetBlockCount(stringParams, &result)
	case "getblockhash":
		err = s.handler.GetBlockHash(stringParams, &result)
	case "getblock":
		err = s.handler.GetBlock(stringParams, &result)
	case "getblockheader":
		err = s.handler.GetBlockHeader(stringParams, &result)
	case "z_gettreestate":
		err = s.handler.ZGetTreeState(stringParams, &result)
	case "z_getsubtreesbyindex":
		err = s.handler.ZGetSubtreesByIndex(stringParams, &result)
	case "z_getnotescount":
		err = s.handler.ZGetNotesCount(stringParams, &result)
	case "z_getnewaddress":
		err = s.handler.ZGetNewAddress(stringParams, &result)
	case "z_listaddresses":
		err = s.handler.ZListAddresses(stringParams, &result)
	case "z_validateaddress":
		err = s.handler.ZValidateAddress(stringParams, &result)
	case "z_getbalance":
		err = s.handler.ZGetBalance(stringParams, &result)
	case "z_listunspent":
		err = s.handler.ZListUnspent(stringParams, &result)
	case "z_listreceivedbyaddress":
		err = s.handler.ZListReceivedByAddress(stringParams, &result)
	case "z_listnotes":
		err = s.handler.ZListNotes(stringParams, &result)
	case "z_sendmany":
		err = s.handler.ZSendMany(stringParams, &result)
	case "z_sendmanywithchangeto":
		err = s.handler.ZSendManyWithChangeTo(stringParams, &result)
	case "z_viewtransaction":
		err = s.handler.ZViewTransaction(stringParams, &result)
	case "z_sendraworchardbundle":
		err = s.handler.ZSendRawOrchardBundle(stringParams, &result)
	case "z_getmempoolinfo":
		err = s.handler.ZGetMempoolInfo(stringParams, &result)
	case "z_getrawmempool":
		err = s.handler.ZGetRawMempool(stringParams, &result)
	case "getrawtransaction":
		err = s.handler.GetRawTransaction(stringParams, &result)
	case "sendrawtransaction":
		err = s.handler.SendRawTransaction(stringParams, &result)
	case "gettransparenttxdata":
		err = s.handler.GetTransparentTxData(stringParams, &result)
	case "getrawmempool":
		err = s.handler.GetRawMempool(stringParams, &result)
	case "getmempoolinfo":
		err = s.handler.GetMempoolInfo(stringParams, &result)
	default:
		err = fmt.Errorf("unknown method: %s", req.Method)
	}

	// Build JSON-RPC response
	var response map[string]interface{}
	if err != nil {
		response = map[string]interface{}{
			"jsonrpc": "1.0",
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
			"jsonrpc": "1.0",
			"result":  resultValue,
			"id":      req.ID,
		}
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
