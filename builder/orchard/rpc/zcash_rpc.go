// Package rpc provides Orchard user-facing RPC server functionality
package rpc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/rpc"

	log "github.com/colorfulnotion/jam/log"
)

// OrchardRPCServer provides Zcash-compatible JSON-RPC API implementation
// Separated from JAM network RPC for clean namespace isolation
// NOTE: This is a legacy stub - production code should use OrchardRPCHandler instead
type OrchardRPCServer struct {
	// No dependencies - this is a stub implementation
}

// NewOrchardRPCServer creates a new Orchard RPC server instance
func NewOrchardRPCServer() *OrchardRPCServer {
	return &OrchardRPCServer{}
}

// ===== Blockchain & Chain State =====

// GetBlockchainInfo returns high-level blockchain and consensus state
//
// Parameters: none
//
// Returns:
// - string: JSON object with chain info
//
// RPC Docs (getblockchaininfo):
// Request: {"jsonrpc":"1.0","id":1,"method":"getblockchaininfo","params":[]}
// Response: {"result":{"chain":"main","blocks":950000,"headers":950000,"bestblockhash":"000000...","upgrades":{"orchard":{"activationheight":900000,"status":"active"}}}}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getblockchaininfo","params":[],"id":1}'
func (s *OrchardRPCServer) GetBlockchainInfo(req []string, res *string) error {
	log.Info(log.Node, "getblockchaininfo")

	// TODO: Get latest block from builder
	blockNumber := uint32(0)

	// Build blockchain info response
	info := map[string]interface{}{
		"chain":   "orchard",
		"blocks":  blockNumber,
		"headers": blockNumber,
		"upgrades": map[string]interface{}{
			"orchard": map[string]interface{}{
				"activationheight": 1,
				"status":           "active",
			},
		},
	}

	infoJSON, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal blockchain info: %v", err)
	}

	*res = string(infoJSON)
	log.Debug(log.Node, "getblockchaininfo", "blocks", blockNumber)
	return nil
}

// ZGetBlockchainInfo returns Zcash-specific blockchain info
//
// Parameters: none
//
// Returns:
// - string: JSON object with Zcash chain info
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getblockchaininfo","params":[],"id":1}'
func (s *OrchardRPCServer) ZGetBlockchainInfo(req []string, res *string) error {
	log.Info(log.Node, "z_getblockchaininfo")

	// TODO: Get tree state from builder
	treeRoot := "0000000000000000000000000000000000000000000000000000000000000000"
	treeSize := uint64(0)

	info := map[string]interface{}{
		"chain": "orchard",
		"orchard": map[string]interface{}{
			"commitments": map[string]interface{}{
				"finalRoot": treeRoot,
				"finalState": map[string]interface{}{
					"commitments": treeSize,
				},
			},
		},
	}

	infoJSON, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal z blockchain info: %v", err)
	}

	*res = string(infoJSON)
	log.Debug(log.Node, "z_getblockchaininfo", "treeSize", treeSize)
	return nil
}

// GetBestBlockHash returns the hash of the best (tip) block in the longest blockchain
//
// Parameters: none
//
// Returns:
// - string: Block hash
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getbestblockhash","params":[],"id":1}'
func (s *OrchardRPCServer) GetBestBlockHash(req []string, res *string) error {
	log.Info(log.Node, "getbestblockhash")

	// TODO: Get latest block hash from builder
	// For now, return a placeholder hash
	bestBlockHash := "0000000000000000000000000000000000000000000000000000000000000000"
	*res = fmt.Sprintf("\"%s\"", bestBlockHash)

	log.Debug(log.Node, "getbestblockhash", "hash", bestBlockHash)
	return nil
}

// GetBlockCount returns the number of blocks in the longest blockchain
//
// Parameters: none
//
// Returns:
// - number: Block count
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getblockcount","params":[],"id":1}'
func (s *OrchardRPCServer) GetBlockCount(req []string, res *string) error {
	log.Info(log.Node, "getblockcount")

	// TODO: Get block count from builder
	blockCount := uint32(0)
	*res = fmt.Sprintf("%d", blockCount)

	log.Debug(log.Node, "getblockcount", "count", blockCount)
	return nil
}

// ===== Address Management =====

// ZGetNewAddress generates a new shielded address
//
// Parameters:
// - string (optional): Account name
//
// Returns:
// - string: New shielded address
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getnewaddress","params":[],"id":1}'
func (s *OrchardRPCServer) ZGetNewAddress(req []string, res *string) error {
	log.Info(log.Node, "z_getnewaddress")

	// Generate a random 32-byte address for demonstration
	addressBytes := make([]byte, 32)
	_, err := rand.Read(addressBytes)
	if err != nil {
		return fmt.Errorf("failed to generate random address: %v", err)
	}

	// Encode as hex with 'orchard' prefix
	address := fmt.Sprintf("orchard%s", hex.EncodeToString(addressBytes))
	*res = fmt.Sprintf("\"%s\"", address)

	log.Debug(log.Node, "z_getnewaddress", "address", address)
	return nil
}

// ZListAddresses returns the list of shielded addresses belonging to the wallet
//
// Parameters: none
//
// Returns:
// - array: Array of shielded addresses
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_listaddresses","params":[],"id":1}'
func (s *OrchardRPCServer) ZListAddresses(req []string, res *string) error {
	log.Info(log.Node, "z_listaddresses")

	// TODO: Get addresses from builder wallet
	addresses := []string{}

	addressesJSON, err := json.Marshal(addresses)
	if err != nil {
		return fmt.Errorf("failed to marshal addresses: %v", err)
	}

	*res = string(addressesJSON)
	log.Debug(log.Node, "z_listaddresses", "count", len(addresses))
	return nil
}

// ZValidateAddress validates a shielded address
//
// Parameters:
// - string: Address to validate
//
// Returns:
// - object: Validation result
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_validateaddress","params":["orchard..."],"id":1}'
func (s *OrchardRPCServer) ZValidateAddress(req []string, res *string) error {
	log.Info(log.Node, "z_validateaddress")

	if len(req) < 1 {
		return fmt.Errorf("insufficient parameters: expected address")
	}

	address := req[0]

	// Basic validation - check if it starts with 'orchard'
	isValid := len(address) > 7 && address[:7] == "orchard"

	result := map[string]interface{}{
		"isvalid": isValid,
	}

	if isValid {
		result["address"] = address
		result["type"] = "shielded"
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal validation result: %v", err)
	}

	*res = string(resultJSON)
	log.Debug(log.Node, "z_validateaddress", "address", address, "valid", isValid)
	return nil
}

// ===== Balance & UTXO Management =====

// ZGetBalance returns the total balance of shielded funds
//
// Parameters:
// - string (optional): Address to get balance for
//
// Returns:
// - number: Balance amount
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getbalance","params":[],"id":1}'
func (s *OrchardRPCServer) ZGetBalance(req []string, res *string) error {
	log.Info(log.Node, "z_getbalance")

	// TODO: Get balance from builder
	balance := 0.0
	*res = fmt.Sprintf("%.8f", balance)

	log.Debug(log.Node, "z_getbalance", "balance", balance)
	return nil
}

// ZListUnspent returns array of unspent shielded notes
//
// Parameters:
// - number (optional): Minimum confirmations
// - number (optional): Maximum confirmations
// - bool (optional): Include watch-only addresses
// - array (optional): Array of addresses to filter
//
// Returns:
// - array: Array of unspent note objects
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_listunspent","params":[],"id":1}'
func (s *OrchardRPCServer) ZListUnspent(req []string, res *string) error {
	log.Info(log.Node, "z_listunspent")

	// TODO: Get unspent notes from builder
	notes := []interface{}{}

	notesJSON, err := json.Marshal(notes)
	if err != nil {
		return fmt.Errorf("failed to marshal notes: %v", err)
	}

	*res = string(notesJSON)
	log.Debug(log.Node, "z_listunspent", "count", len(notes))
	return nil
}

// ===== Transaction Operations =====

// ZSendMany sends funds to multiple recipients
//
// Parameters:
// - string: From address
// - array: Array of recipient objects with address and amount
// - number (optional): Minimum confirmations
// - number (optional): Fee
//
// Returns:
// - string: Operation ID
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_sendmany","params":["orchard...,[{"address":"orchard...","amount":0.01}]"],"id":1}'
func (s *OrchardRPCServer) ZSendMany(req []string, res *string) error {
	log.Info(log.Node, "z_sendmany")

	if len(req) < 2 {
		return fmt.Errorf("insufficient parameters: expected from address and recipients array")
	}

	fromAddress := req[0]
	recipientsJSON := req[1]

	// TODO: Parse recipients and create transaction through builder

	// Generate operation ID
	opBytes := make([]byte, 16)
	_, err := rand.Read(opBytes)
	if err != nil {
		return fmt.Errorf("failed to generate operation ID: %v", err)
	}

	opID := hex.EncodeToString(opBytes)
	*res = fmt.Sprintf("\"%s\"", opID)

	log.Debug(log.Node, "z_sendmany",
		"from", fromAddress,
		"recipients", recipientsJSON,
		"opid", opID)
	return nil
}

// ===== Tree State Operations =====

// ZGetTreeState returns the Orchard tree state at a given block
//
// Parameters:
// - string: Block hash or height
//
// Returns:
// - object: Tree state with commitments info
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_gettreestate","params":["latest"],"id":1}'
func (s *OrchardRPCServer) ZGetTreeState(req []string, res *string) error {
	log.Info(log.Node, "z_gettreestate")

	// TODO: Get tree state from builder
	treeRoot := "0000000000000000000000000000000000000000000000000000000000000000"
	treeSize := uint64(0)

	treeState := map[string]interface{}{
		"orchard": map[string]interface{}{
			"commitments": map[string]interface{}{
				"finalRoot": treeRoot,
				"finalState": map[string]interface{}{
					"commitments": treeSize,
				},
			},
		},
	}

	treeStateJSON, err := json.Marshal(treeState)
	if err != nil {
		return fmt.Errorf("failed to marshal tree state: %v", err)
	}

	*res = string(treeStateJSON)
	log.Debug(log.Node, "z_gettreestate", "size", treeSize, "root", treeRoot)
	return nil
}

// RegisterOrchardRPC registers all Orchard RPC methods with the given RPC server
// This provides both the 'z' namespace for shielded operations and standard blockchain methods
// NOTE: This is a legacy stub - production code should use OrchardRPCHandler instead
func RegisterOrchardRPC(server *rpc.Server) error {
	orchardRPC := NewOrchardRPCServer()

	// Register with multiple namespaces for compatibility
	// Standard blockchain methods (no namespace prefix)
	err := server.RegisterName("", orchardRPC)
	if err != nil {
		return fmt.Errorf("failed to register standard RPC methods: %v", err)
	}

	// Zcash-compatible shielded methods with 'z' prefix
	err = server.RegisterName("z", orchardRPC)
	if err != nil {
		return fmt.Errorf("failed to register z RPC namespace: %v", err)
	}

	log.Info(log.Node, "Registered Orchard RPC namespaces", "namespaces", "default,z")
	return nil
}
