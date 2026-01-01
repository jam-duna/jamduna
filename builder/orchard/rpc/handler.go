package rpc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
)

// OrchardRPCHandler handles all Orchard (Zcash-compatible) JSON-RPC methods
type OrchardRPCHandler struct {
	orchardRollup *OrchardRollup
	wallet        OrchardWallet
}

type sendAmount struct {
	Address string      `json:"address"`
	Amount  json.Number `json:"amount"`
}

const (
	sendManyPayloadLimitBytes = 64 * 1024
	maxSendManyRecipients     = 256
)

var (
	weiPerNativeUnit = big.NewInt(1_000_000_000_000_000_000)
	maxWeiValue      = new(big.Int).SetUint64(^uint64(0))
)

// NewOrchardRPCHandler creates a new Orchard RPC handler
func NewOrchardRPCHandler(orchardRollup *OrchardRollup) *OrchardRPCHandler {
	return NewOrchardRPCHandlerWithWallet(orchardRollup, NewInMemoryWallet(orchardRollup))
}

// NewOrchardRPCHandlerWithWallet creates a new Orchard RPC handler with a wallet.
func NewOrchardRPCHandlerWithWallet(orchardRollup *OrchardRollup, wallet OrchardWallet) *OrchardRPCHandler {
	return &OrchardRPCHandler{
		orchardRollup: orchardRollup,
		wallet:        wallet,
	}
}

// GetOrchardRollup returns the rollup instance for direct access
func (h *OrchardRPCHandler) GetOrchardRollup() *OrchardRollup {
	return h.orchardRollup
}

// GetWallet returns the configured wallet for RPC calls.
func (h *OrchardRPCHandler) GetWallet() OrchardWallet {
	return h.wallet
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
func (h *OrchardRPCHandler) GetBlockchainInfo(req []string, res *string) error {
	log.Info(log.Node, "GetBlockchainInfo")

	orchardRollup := h.GetOrchardRollup()

	blockNumber, err := orchardRollup.LatestBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to get latest Orchard block number: %v", err)
	}

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

	jsonBytes, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal blockchain info: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "GetBlockchainInfo: Returning blockchain info")
	return nil
}

// ZGetBlockchainInfo returns extended blockchain info including shielded pool statistics
//
// Parameters: none
//
// Returns:
// - string: JSON object with extended chain info
//
// RPC Docs (z_getblockchaininfo):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_getblockchaininfo","params":[]}
// Response: {"result":{"blocks":950000,"orchard":{"commitmentTreeSize":12345678,"valuePools":{"chainValue":21000000.0}}}}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getblockchaininfo","params":[],"id":1}'
func (h *OrchardRPCHandler) ZGetBlockchainInfo(req []string, res *string) error {
	log.Info(log.Node, "ZGetBlockchainInfo")

	// Get rollup for this service
	orchardRollup := h.GetOrchardRollup()

	blockNumber, err := orchardRollup.LatestBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to get latest Orchard block number: %v", err)
	}

	// Get active notes for commitment tree size
	activeNotes := orchardRollup.ActiveNotes()

	// Calculate total value in shielded pool
	var totalValue uint64
	for _, note := range activeNotes {
		totalValue += note.Value
	}

	// Build extended blockchain info response
	info := map[string]interface{}{
		"blocks": blockNumber,
		"orchard": map[string]interface{}{
			"commitmentTreeSize": len(activeNotes),
			"valuePools": map[string]interface{}{
				"chainValue": float64(totalValue) / 1000000000000000000.0, // Convert wei to ETH
			},
		},
	}

	jsonBytes, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal extended blockchain info: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZGetBlockchainInfo: Returning extended blockchain info")
	return nil
}

// GetBestBlockHash returns the hash of the current best block
//
// Parameters: none
//
// Returns:
// - string: Best block hash as hex string
//
// RPC Docs (getbestblockhash):
// Request: {"jsonrpc":"1.0","id":1,"method":"getbestblockhash","params":[]}
// Response: {"result":"000000..."}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getbestblockhash","params":[],"id":1}'
func (h *OrchardRPCHandler) GetBestBlockHash(req []string, res *string) error {
	log.Info(log.Node, "GetBestBlockHash")

	orchardRollup := h.GetOrchardRollup()

	latestHeight, err := orchardRollup.LatestBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to get latest Orchard block number: %v", err)
	}

	meta, err := orchardRollup.GetOrchardBlockMetadata(latestHeight)
	if err != nil {
		return fmt.Errorf("failed to get best block metadata: %v", err)
	}

	*res = meta.Hash.String()
	log.Debug(log.Node, "GetBestBlockHash: Returning best block hash", "height", latestHeight, "hash", *res)
	return nil
}

// GetBlockCount returns the current chain height
//
// Parameters: none
//
// Returns:
// - uint32: Current block number
//
// RPC Docs (getblockcount):
// Request: {"jsonrpc":"1.0","id":1,"method":"getblockcount","params":[]}
// Response: {"result":950000}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getblockcount","params":[],"id":1}'
func (h *OrchardRPCHandler) GetBlockCount(req []string, res *string) error {
	log.Info(log.Node, "GetBlockCount")

	orchardRollup := h.GetOrchardRollup()

	blockNumber, err := orchardRollup.LatestBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to get latest Orchard block number: %v", err)
	}

	*res = fmt.Sprintf("%d", blockNumber)
	log.Debug(log.Node, "GetBlockCount: Returning block count", "count", *res)
	return nil
}

// GetBlockHash returns the block hash at a given height
//
// Parameters:
// - height (uint32): Block height
//
// Returns:
// - string: Block hash as hex string
//
// RPC Docs (getblockhash):
// Request: {"jsonrpc":"1.0","id":1,"method":"getblockhash","params":[950000]}
// Response: {"result":"000000..."}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getblockhash","params":[100],"id":1}'
func (h *OrchardRPCHandler) GetBlockHash(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	heightStr := req[0]
	height, err := strconv.ParseUint(heightStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid height parameter: %v", err)
	}

	log.Info(log.Node, "GetBlockHash", "height", height)

	orchardRollup := h.GetOrchardRollup()

	meta, err := orchardRollup.GetOrchardBlockMetadata(uint32(height))
	if err != nil {
		return fmt.Errorf("failed to get block metadata: %v", err)
	}

	*res = meta.Hash.String()
	log.Debug(log.Node, "GetBlockHash: Returning block hash", "height", height, "hash", *res)
	return nil
}

// GetBlock returns full block data including Orchard bundle
//
// Parameters:
// - blockHash (string): Block hash as hex string
// - verbosity (int): Verbosity level (0=raw, 1=summary, 2=full)
//
// Returns:
// - string: Block data as JSON
//
// RPC Docs (getblock):
// Request: {"jsonrpc":"1.0","id":1,"method":"getblock","params":["000000...",2]}
// Response: {"result":{"hash":"000000...","height":950000,"tx":[{"txid":"abcd...","orchard":{"actions":[...],"proof":"..."}}]}}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getblock","params":["0x000000...","2"],"id":1}'
func (h *OrchardRPCHandler) GetBlock(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	blockHashStr := req[0]
	verbosityStr := req[1]

	verbosity, err := strconv.ParseInt(verbosityStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid verbosity parameter: %v", err)
	}

	log.Info(log.Node, "GetBlock", "blockHash", blockHashStr, "verbosity", verbosity)

	// Get rollup for this service
	orchardRollup := h.GetOrchardRollup()

	// Parse block hash and derive height (simplified)
	blockHash := common.HexToHash(blockHashStr)
	height := uint32(blockHash[31]) | uint32(blockHash[30])<<8 | uint32(blockHash[29])<<16 | uint32(blockHash[28])<<24

	meta, err := orchardRollup.GetOrchardBlockMetadata(height)
	if err != nil {
		return fmt.Errorf("failed to get block metadata: %v", err)
	}

	// Get events for this block (Orchard transactions)
	events := orchardRollup.History()
	var blockTransactions []map[string]interface{}

	for _, event := range events {
		if event.Block == height {
			tx := map[string]interface{}{
				"txid": event.Commitment.String(),
				"orchard": map[string]interface{}{
					"actions": []map[string]interface{}{
						{
							"nullifier":  event.Nullifier.String(),
							"commitment": event.Commitment.String(),
							"value":      event.Value,
							"kind":       event.Kind,
						},
					},
					"proof": "0x" + hex.EncodeToString(make([]byte, 32)), // Placeholder
				},
			}
			blockTransactions = append(blockTransactions, tx)
		}
	}

	block := map[string]interface{}{
		"hash":   meta.Hash.String(),
		"height": height,
		"tx":     blockTransactions,
	}

	jsonBytes, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "GetBlock: Returning block", "height", height)
	return nil
}

// GetBlockHeader returns block header without transactions
//
// Parameters:
// - blockHash (string): Block hash as hex string
// - verbose (bool): Return JSON object if true, hex string if false
//
// Returns:
// - string: Block header data
//
// RPC Docs (getblockheader):
// Request: {"jsonrpc":"1.0","id":1,"method":"getblockheader","params":["000000...",true]}
// Response: {"result":{"hash":"000000...","height":950000,"time":1710000000,"previousblockhash":"000000..."}}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getblockheader","params":["0x000000...","true"],"id":1}'
func (h *OrchardRPCHandler) GetBlockHeader(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}
	blockHashStr := req[0]
	verboseStr := req[1]

	verbose, err := strconv.ParseBool(verboseStr)
	if err != nil {
		return fmt.Errorf("invalid verbose parameter: %v", err)
	}

	log.Info(log.Node, "GetBlockHeader", "blockHash", blockHashStr, "verbose", verbose)

	orchardRollup := h.GetOrchardRollup()

	// Parse block hash and derive height (simplified)
	blockHash := common.HexToHash(blockHashStr)
	height := uint32(blockHash[31]) | uint32(blockHash[30])<<8 | uint32(blockHash[29])<<16 | uint32(blockHash[28])<<24

	meta, err := orchardRollup.GetOrchardBlockMetadata(height)
	if err != nil {
		return fmt.Errorf("failed to get block metadata: %v", err)
	}

	if verbose {
		header := map[string]interface{}{
			"hash":              meta.Hash.String(),
			"height":            height,
			"time":              meta.Timestamp,
			"previousblockhash": meta.ParentHash.Hex(),
		}

		jsonBytes, err := json.Marshal(header)
		if err != nil {
			return fmt.Errorf("failed to marshal block header: %v", err)
		}
		*res = string(jsonBytes)
	} else {
		*res = meta.Hash.String()
	}

	log.Debug(log.Node, "GetBlockHeader: Returning block header", "height", height)
	return nil
}

// ===== Orchard Tree & State (Critical for Light Clients) =====

// ZGetTreeState returns Orchard note commitment tree root and size at a given block
//
// Parameters:
// - height (uint32): Block height
//
// Returns:
// - string: JSON object with tree state
//
// RPC Docs (z_gettreestate):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_gettreestate","params":[950000]}
// Response: {"result":{"orchard":{"commitmentTreeSize":12345678,"commitmentTreeRoot":"abc123..."}}}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_gettreestate","params":[100],"id":1}'
func (h *OrchardRPCHandler) ZGetTreeState(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	heightStr := req[0]
	height, err := strconv.ParseUint(heightStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid height parameter: %v", err)
	}

	log.Info(log.Node, "ZGetTreeState", "height", height)

	// Get rollup for this service
	orchardRollup := h.GetOrchardRollup()

	treeStateData, err := orchardRollup.GetCommitmentTreeState()
	if err != nil {
		return fmt.Errorf("failed to get commitment tree state: %v", err)
	}

	treeState := map[string]interface{}{
		"orchard": map[string]interface{}{
			"commitmentTreeSize": treeStateData.Size,
			"commitmentTreeRoot": treeStateData.Root.String(),
		},
	}

	jsonBytes, err := json.Marshal(treeState)
	if err != nil {
		return fmt.Errorf("failed to marshal tree state: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZGetTreeState: Returning tree state", "height", height, "size", treeStateData.Size)
	return nil
}

// ZGetSubtreesByIndex returns Orchard subtree roots for incremental syncing
//
// Parameters:
// - pool (string): Pool name ("orchard")
// - startIndex (uint32): Starting subtree index
// - limit (uint32): Maximum number of subtrees to return
//
// Returns:
// - string: JSON object with subtree data
//
// RPC Docs (z_getsubtreesbyindex):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_getsubtreesbyindex","params":["orchard",0,10]}
// Response: {"result":{"subtrees":[{"root":"aaa...","end_height":905000}]}}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getsubtreesbyindex","params":["orchard",0,10],"id":1}'
func (h *OrchardRPCHandler) ZGetSubtreesByIndex(req []string, res *string) error {
	if len(req) != 3 {
		return fmt.Errorf("invalid number of arguments: expected 3, got %d", len(req))
	}

	pool := req[0]
	startIndexStr := req[1]
	limitStr := req[2]

	if pool != "orchard" {
		return fmt.Errorf("unsupported pool: %s", pool)
	}

	startIndex, err := strconv.ParseUint(startIndexStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid startIndex parameter: %v", err)
	}

	limit, err := strconv.ParseUint(limitStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid limit parameter: %v", err)
	}

	log.Info(log.Node, "ZGetSubtreesByIndex", "pool", pool, "startIndex", startIndex, "limit", limit)

	// For PoC, return simplified subtree structure
	subtrees := []map[string]interface{}{
		{
			"root":       "0xabc123...",
			"end_height": 905000,
		},
	}

	result := map[string]interface{}{
		"subtrees": subtrees,
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal subtrees: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZGetSubtreesByIndex: Returning subtrees", "count", len(subtrees))
	return nil
}

// ZGetNotesCount returns total Orchard note count
//
// Parameters: none
//
// Returns:
// - string: JSON object with note counts
//
// RPC Docs (z_getnotescount):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_getnotescount","params":[]}
// Response: {"result":{"orchard":12345678}}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getnotescount","params":[],"id":1}'
func (h *OrchardRPCHandler) ZGetNotesCount(req []string, res *string) error {
	log.Info(log.Node, "ZGetNotesCount")

	// Get rollup for this service
	orchardRollup := h.GetOrchardRollup()

	// Get active notes
	activeNotes := orchardRollup.ActiveNotes()

	result := map[string]interface{}{
		"orchard": len(activeNotes),
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal notes count: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZGetNotesCount: Returning notes count", "count", len(activeNotes))
	return nil
}

// ===== Wallet / Shielded (Orchard) =====

// ZGetNewAddress generates a new Orchard shielded address
//
// Parameters:
// - addressType (string): Address type ("orchard")
//
// Returns:
// - string: New shielded address
//
// RPC Docs (z_getnewaddress):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_getnewaddress","params":["orchard"]}
// Response: {"result":"u1abc..."}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getnewaddress","params":["orchard"],"id":1}'
func (h *OrchardRPCHandler) ZGetNewAddress(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	addressType := req[0]
	if addressType != "orchard" {
		return fmt.Errorf("unsupported address type: %s", addressType)
	}

	log.Info(log.Node, "ZGetNewAddress", "addressType", addressType)

	wallet := h.GetWallet()
	if wallet == nil {
		return fmt.Errorf("wallet not configured")
	}

	address, err := wallet.NewAddress()
	if err != nil {
		return fmt.Errorf("failed to generate address: %v", err)
	}

	*res = address
	log.Debug(log.Node, "ZGetNewAddress: Generated new address", "address", address)
	return nil
}

// ZListAddresses lists Orchard addresses in the wallet
//
// Parameters: none
//
// Returns:
// - string: JSON array of addresses
//
// RPC Docs (z_listaddresses):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_listaddresses","params":[]}
// Response: {"result":["u1abc...","u1def..."]}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_listaddresses","params":[],"id":1}'
func (h *OrchardRPCHandler) ZListAddresses(req []string, res *string) error {
	log.Info(log.Node, "ZListAddresses")

	wallet := h.GetWallet()
	if wallet == nil {
		return fmt.Errorf("wallet not configured")
	}

	addresses := wallet.ListAddresses()

	jsonBytes, err := json.Marshal(addresses)
	if err != nil {
		return fmt.Errorf("failed to marshal addresses: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZListAddresses: Returning addresses", "count", len(addresses))
	return nil
}

// ZValidateAddress validates a Zcash address
//
// Parameters:
// - address (string): Address to validate
//
// Returns:
// - string: JSON object with validation result
//
// RPC Docs (z_validateaddress):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_validateaddress","params":["u1abc..."]}
// Response: {"result":{"isvalid":true,"address_type":"orchard"}}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_validateaddress","params":["u1abc..."],"id":1}'
func (h *OrchardRPCHandler) ZValidateAddress(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	address := req[0]
	log.Info(log.Node, "ZValidateAddress", "address", address)

	wallet := h.GetWallet()
	if wallet == nil {
		return fmt.Errorf("wallet not configured")
	}

	isValid := wallet.ValidateAddress(address)

	result := map[string]interface{}{
		"isvalid":      isValid,
		"address_type": "orchard",
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal validation result: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZValidateAddress: Returning validation result", "valid", isValid)
	return nil
}

// ZGetBalance returns the Orchard balance for an address
//
// Parameters:
// - address (string): Address to query
// - minconf (int): Minimum confirmations
//
// Returns:
// - float64: Balance
//
// RPC Docs (z_getbalance):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_getbalance","params":["u1abc...",1]}
// Response: {"result":12.345}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getbalance","params":["u1abc...",1],"id":1}'
func (h *OrchardRPCHandler) ZGetBalance(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}

	address := req[0]
	minconfStr := req[1]

	minconf, err := strconv.ParseInt(minconfStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid minconf parameter: %v", err)
	}

	log.Info(log.Node, "ZGetBalance", "address", address, "minconf", minconf)

	wallet := h.GetWallet()
	if err := validateAddressFormat(wallet, address); err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}
	if minconf < 0 {
		return fmt.Errorf("minconf must be non-negative")
	}

	activeNotes := wallet.NotesByAddress(address)
	var totalBalance uint64
	for _, note := range activeNotes {
		totalBalance += note.Value
	}

	// Convert wei to ETH
	balance := float64(totalBalance) / 1000000000000000000.0

	*res = fmt.Sprintf("%.6f", balance)
	log.Debug(log.Node, "ZGetBalance: Returning balance", "balance", balance)
	return nil
}

// ZListUnspent lists unspent Orchard notes in the wallet
//
// Parameters: none
//
// Returns:
// - string: JSON array of unspent notes
//
// RPC Docs (z_listunspent):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_listunspent","params":[]}
// Response: {"result":[{"address":"u1abc...","amount":1.23,"txid":"abcd..."}]}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_listunspent","params":[],"id":1}'
func (h *OrchardRPCHandler) ZListUnspent(req []string, res *string) error {
	log.Info(log.Node, "ZListUnspent")

	wallet := h.GetWallet()
	if wallet == nil {
		return fmt.Errorf("wallet not configured")
	}

	activeNotes := wallet.UnspentNotes()
	var unspentNotes []map[string]interface{}

	for _, note := range activeNotes {
		unspent := map[string]interface{}{
			"address": note.Address,
			"amount":  float64(note.Value) / 1000000000000000000.0,
			"txid":    note.Commitment.String(),
		}
		unspentNotes = append(unspentNotes, unspent)
	}

	jsonBytes, err := json.Marshal(unspentNotes)
	if err != nil {
		return fmt.Errorf("failed to marshal unspent notes: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZListUnspent: Returning unspent notes", "count", len(unspentNotes))
	return nil
}

// ZListReceivedByAddress lists Orchard notes received by an address
//
// Parameters:
// - address (string): Address to query
// - minconf (int): Minimum confirmations
//
// Returns:
// - string: JSON array of received notes
//
// RPC Docs (z_listreceivedbyaddress):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_listreceivedbyaddress","params":["u1abc...",1]}
// Response: {"result":[{"amount":1.23,"txid":"abcd..."}]}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_listreceivedbyaddress","params":["u1abc...",1],"id":1}'
func (h *OrchardRPCHandler) ZListReceivedByAddress(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}

	address := req[0]
	minconfStr := req[1]

	minconf, err := strconv.ParseInt(minconfStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid minconf parameter: %v", err)
	}

	log.Info(log.Node, "ZListReceivedByAddress", "address", address, "minconf", minconf)

	wallet := h.GetWallet()
	if err := validateAddressFormat(wallet, address); err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}
	if minconf < 0 {
		return fmt.Errorf("minconf must be non-negative")
	}

	notes := wallet.NotesByAddress(address)
	var receivedNotes []map[string]interface{}

	for _, note := range notes {
		received := map[string]interface{}{
			"amount": float64(note.Value) / 1000000000000000000.0,
			"txid":   note.Commitment.String(),
		}
		receivedNotes = append(receivedNotes, received)
	}

	jsonBytes, err := json.Marshal(receivedNotes)
	if err != nil {
		return fmt.Errorf("failed to marshal received notes: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZListReceivedByAddress: Returning received notes", "count", len(receivedNotes))
	return nil
}

// ZListNotes lists Orchard notes known to the wallet
//
// Parameters: none
//
// Returns:
// - string: JSON array of all notes
//
// RPC Docs (z_listnotes):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_listnotes","params":[]}
// Response: {"result":[{"address":"u1abc...","value":1.23,"spent":false}]}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_listnotes","params":[],"id":1}'
func (h *OrchardRPCHandler) ZListNotes(req []string, res *string) error {
	log.Info(log.Node, "ZListNotes")

	wallet := h.GetWallet()
	if wallet == nil {
		return fmt.Errorf("wallet not configured")
	}

	activeNotes := wallet.Notes()
	var allNotes []map[string]interface{}

	for _, note := range activeNotes {
		noteInfo := map[string]interface{}{
			"address": note.Address,
			"value":   float64(note.Value) / 1000000000000000000.0,
			"spent":   false, // Active notes are unspent
		}
		allNotes = append(allNotes, noteInfo)
	}

	jsonBytes, err := json.Marshal(allNotes)
	if err != nil {
		return fmt.Errorf("failed to marshal notes: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZListNotes: Returning all notes", "count", len(allNotes))
	return nil
}

// ZSendMany sends Orchard funds asynchronously
//
// Parameters:
// - fromAddress (string): Source address
// - amounts ([]object): Array of recipient/amount pairs
//
// Returns:
// - string: Operation ID
//
// RPC Docs (z_sendmany):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_sendmany","params":["u1abc...",[{"address":"u1def...","amount":1.0}]]}
// Response: {"result":"opid-1234"}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_sendmany","params":["u1abc...",[{"address":"u1def...","amount":1.0}]],"id":1}'
func (h *OrchardRPCHandler) ZSendMany(req []string, res *string) error {
	if len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 2, got %d", len(req))
	}

	fromAddress := req[0]
	amountsJson := req[1]

	log.Info(log.Node, "ZSendMany", "fromAddress", fromAddress, "amounts", amountsJson)

	wallet := h.GetWallet()
	if err := validateAddressFormat(wallet, fromAddress); err != nil {
		return fmt.Errorf("invalid source address: %w", err)
	}
	if _, err := parseAndValidateSendManyAmounts(wallet, amountsJson); err != nil {
		return fmt.Errorf("invalid sendmany request: %v", err)
	}

	// Generate operation ID
	randomBytes, err := generateRandomBytes(4)
	if err != nil {
		return fmt.Errorf("failed to generate operation id: %v", err)
	}
	opId := "opid-" + hex.EncodeToString(randomBytes)

	*res = opId
	log.Debug(log.Node, "ZSendMany: Returning operation ID", "opId", opId)
	return nil
}

// ZSendManyWithChangeTo sends Orchard funds with explicit change address
//
// Parameters:
// - fromAddress (string): Source address
// - amounts ([]object): Array of recipient/amount pairs
// - minconf (int): Minimum confirmations
// - fee (float64): Transaction fee
// - changeAddress (string): Change address
//
// Returns:
// - string: Operation ID
//
// RPC Docs (z_sendmanywithchangeto):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_sendmanywithchangeto","params":["u1abc...",[{"address":"u1def...","amount":1.0}],1,0.0001,"u1change..."]}
// Response: {"result":"opid-5678"}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_sendmanywithchangeto","params":["u1abc...",[{"address":"u1def...","amount":1.0}],1,0.0001,"u1change..."],"id":1}'
func (h *OrchardRPCHandler) ZSendManyWithChangeTo(req []string, res *string) error {
	if len(req) != 5 {
		return fmt.Errorf("invalid number of arguments: expected 5, got %d", len(req))
	}

	fromAddress := req[0]
	amountsJson := req[1]
	minconfStr := req[2]
	feeStr := req[3]
	changeAddress := req[4]

	minconf, err := strconv.ParseInt(minconfStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid minconf parameter: %v", err)
	}

	fee, err := strconv.ParseFloat(feeStr, 64)
	if err != nil {
		return fmt.Errorf("invalid fee parameter: %v", err)
	}

	log.Info(log.Node, "ZSendManyWithChangeTo", "fromAddress", fromAddress, "amounts", amountsJson, "minconf", minconf, "fee", fee, "changeAddress", changeAddress)

	if minconf < 0 {
		return fmt.Errorf("minconf must be non-negative")
	}
	if math.IsNaN(fee) || math.IsInf(fee, 0) || fee < 0 {
		return fmt.Errorf("fee must be a finite, non-negative value")
	}
	wallet := h.GetWallet()
	if err := validateAddressFormat(wallet, fromAddress); err != nil {
		return fmt.Errorf("invalid source address: %w", err)
	}
	if err := validateAddressFormat(wallet, changeAddress); err != nil {
		return fmt.Errorf("invalid change address: %w", err)
	}
	if _, err := parseAndValidateSendManyAmounts(wallet, amountsJson); err != nil {
		return fmt.Errorf("invalid sendmany request: %v", err)
	}

	// Generate operation ID
	randomBytes, err := generateRandomBytes(4)
	if err != nil {
		return fmt.Errorf("failed to generate operation id: %v", err)
	}
	opId := "opid-" + hex.EncodeToString(randomBytes)

	*res = opId
	log.Debug(log.Node, "ZSendManyWithChangeTo: Returning operation ID", "opId", opId)
	return nil
}

// ZViewTransaction views decrypted Orchard transaction details
//
// Parameters:
// - txid (string): Transaction ID
//
// Returns:
// - string: JSON object with transaction details
//
// RPC Docs (z_viewtransaction):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_viewtransaction","params":["abcd..."]}
// Response: {"result":{"txid":"abcd...","orchard_notes":[...]}}
//
// Example curl call:
// curl -X POST http://localhost:8545 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_viewtransaction","params":["0xabcd..."],"id":1}'
func (h *OrchardRPCHandler) ZViewTransaction(req []string, res *string) error {
	if len(req) != 1 {
		return fmt.Errorf("invalid number of arguments: expected 1, got %d", len(req))
	}

	txid := req[0]
	log.Info(log.Node, "ZViewTransaction", "txid", txid)

	// Get rollup for this service
	orchardRollup := h.GetOrchardRollup()

	// Find transaction in history
	events := orchardRollup.History()
	var txDetails map[string]interface{}

	for _, event := range events {
		if event.Commitment.String() == txid {
			txDetails = map[string]interface{}{
				"txid": txid,
				"orchard_notes": []map[string]interface{}{
					{
						"commitment": event.Commitment.String(),
						"nullifier":  event.Nullifier.String(),
						"value":      event.Value,
						"kind":       event.Kind,
					},
				},
			}
			break
		}
	}

	if txDetails == nil {
		*res = "null"
		return nil
	}

	jsonBytes, err := json.Marshal(txDetails)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction details: %v", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "ZViewTransaction: Returning transaction details", "txid", txid)
	return nil
}

// ===== Helper Functions =====

func validateAddressFormat(wallet OrchardWallet, address string) error {
	if wallet == nil {
		return fmt.Errorf("wallet not configured")
	}
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("address is required")
	}
	if !wallet.ValidateAddress(address) {
		return fmt.Errorf("invalid address format: %s", address)
	}
	return nil
}

func parseAndValidateSendManyAmounts(wallet OrchardWallet, raw string) ([]sendAmount, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("amounts payload is empty")
	}
	if len(raw) > sendManyPayloadLimitBytes {
		return nil, fmt.Errorf("amounts payload exceeds %d bytes", sendManyPayloadLimitBytes)
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()

	var amounts []sendAmount
	if err := decoder.Decode(&amounts); err != nil {
		return nil, fmt.Errorf("failed to parse amounts: %v", err)
	}

	if len(amounts) == 0 {
		return nil, fmt.Errorf("no recipients specified")
	}
	if len(amounts) > maxSendManyRecipients {
		return nil, fmt.Errorf("too many recipients: %d (max %d)", len(amounts), maxSendManyRecipients)
	}

	for i, amt := range amounts {
		if err := validateAddressFormat(wallet, amt.Address); err != nil {
			return nil, fmt.Errorf("invalid recipient address at index %d: %w", i, err)
		}
		if _, err := validateAmountWithinRange(amt.Amount); err != nil {
			return nil, fmt.Errorf("invalid amount for recipient %d: %v", i, err)
		}
	}

	return amounts, nil
}

func validateAmountWithinRange(amount json.Number) (uint64, error) {
	amountStr := strings.TrimSpace(amount.String())
	if amountStr == "" {
		return 0, fmt.Errorf("amount is missing")
	}

	rat, ok := new(big.Rat).SetString(amountStr)
	if !ok {
		return 0, fmt.Errorf("amount is not a valid number")
	}
	if rat.Sign() <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}

	weiRat := new(big.Rat).Mul(rat, new(big.Rat).SetInt(weiPerNativeUnit))
	if !weiRat.IsInt() {
		return 0, fmt.Errorf("amount precision exceeds 18 decimals")
	}

	weiInt := weiRat.Num()
	if weiInt.Sign() <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}
	if weiInt.Cmp(maxWeiValue) > 0 {
		return 0, fmt.Errorf("amount exceeds maximum supported value")
	}

	return weiInt.Uint64(), nil
}

func generateRandomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}
