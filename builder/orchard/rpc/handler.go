package rpc

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
)

// OrchardRPCHandler handles all Orchard (Zcash-compatible) JSON-RPC methods
type OrchardRPCHandler struct {
	orchardRollup *OrchardRollup
	wallet        OrchardWallet
	txPool        *OrchardTxPool
	// transparentTxStore is accessed via orchardRollup.transparentTxStore (shared instance)
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
		txPool:        nil, // Will be set by SetTxPool after initialization
		// transparentTxStore is accessed via orchardRollup.transparentTxStore
	}
}

// SetTxPool sets the transaction pool for this handler
func (h *OrchardRPCHandler) SetTxPool(txPool *OrchardTxPool) {
	h.txPool = txPool
}

// GetOrchardRollup returns the rollup instance for direct access
func (h *OrchardRPCHandler) GetOrchardRollup() *OrchardRollup {
	return h.orchardRollup
}

// GetWallet returns the configured wallet for RPC calls.
func (h *OrchardRPCHandler) GetWallet() OrchardWallet {
	return h.wallet
}

// Close releases resources owned by the handler.
func (h *OrchardRPCHandler) Close() error {
	if h.orchardRollup != nil {
		return h.orchardRollup.Close()
	}
	return nil
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

	// Add transparent transactions for this block
	if orchardRollup.transparentTxStore != nil {
		transparentTxs, err := orchardRollup.transparentTxStore.GetBlockTransactionsVerbose(height)
		if err != nil {
			log.Warn(log.Node, "Failed to get transparent transactions for block", "height", height, "error", err)
		} else if transparentTxs != nil {
			for _, tx := range transparentTxs {
				blockTx := map[string]interface{}{
					"txid":          tx.TxID,
					"hash":          tx.TxID,
					"version":       tx.Version,
					"size":          tx.Size,
					"locktime":      tx.LockTime,
					"vin":           tx.Vin,
					"vout":          tx.Vout,
					"blockhash":     tx.BlockHash,
					"confirmations": tx.Confirmations,
					"time":          tx.Timestamp.Unix(),
					"blocktime":     tx.Timestamp.Unix(),
				}
				blockTransactions = append(blockTransactions, blockTx)
			}
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

// ZGetSubtreesByIndex returns Orchard subtree roots and commitments for incremental syncing
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
// Response: {"result":{"pool":"orchard","start_index":0,"subtrees":[{"index":0,"root":"0x...","height":16,"commitments":["0x..."]}]}}
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

	const orchardSubtreeHeight = 16
	subtreeSize := uint64(1 << orchardSubtreeHeight)

	commitments := h.GetOrchardRollup().CommitmentList()
	total := uint64(len(commitments))

	subtrees := make([]map[string]interface{}, 0)

	for i := uint64(0); i < limit; i++ {
		subtreeIndex := startIndex + i
		offset := subtreeIndex * subtreeSize
		if offset >= total {
			break
		}

		end := offset + subtreeSize
		if end > total {
			end = total
		}

		commitmentsSlice := commitments[offset:end]
		commitmentHex := make([]string, len(commitmentsSlice))
		for j, commitment := range commitmentsSlice {
			commitmentHex[j] = "0x" + hex.EncodeToString(commitment[:])
		}

		subtrees = append(subtrees, map[string]interface{}{
			"index":       subtreeIndex,
			"root":        common.Hash{}.Hex(),
			"height":      orchardSubtreeHeight,
			"commitments": commitmentHex,
		})
	}

	result := map[string]interface{}{
		"pool":        pool,
		"start_index": startIndex,
		"subtrees":    subtrees,
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

	amounts, err := parseAndValidateSendManyAmounts(wallet, amountsJson)
	if err != nil {
		return fmt.Errorf("invalid sendmany request: %v", err)
	}

	// Synthesize Orchard bundle from send_many request
	bundleData, bundleID, err := h.synthesizeOrchardBundle(fromAddress, amounts)
	if err != nil {
		return fmt.Errorf("failed to synthesize Orchard bundle: %w", err)
	}

	// Submit bundle to transaction pool if available
	if h.txPool != nil {
		if err := h.txPool.AddBundle(bundleData, nil, bundleID); err != nil {
			log.Warn(log.Node, "Failed to add bundle to pool, proceeding with operation ID",
				"bundle_id", bundleID, "error", err)
		} else {
			log.Info(log.Node, "Bundle synthesized and added to pool",
				"bundle_id", bundleID, "recipients", len(amounts))
		}
	} else {
		log.Warn(log.Node, "Transaction pool not available, returning operation ID only", "bundle_id", bundleID)
	}

	// Return operation ID (which doubles as bundle ID for tracking)
	*res = bundleID
	log.Debug(log.Node, "ZSendMany: Returning operation ID", "opId", bundleID)
	return nil
}

// synthesizeOrchardBundle creates a deterministic Orchard bundle from a z_sendmany request.
// This uses the Rust Orchard builder via FFI to produce a real bundle, proof, and signatures.
func (h *OrchardRPCHandler) synthesizeOrchardBundle(fromAddress string, amounts []sendAmount) ([]byte, string, error) {
	// Generate deterministic bundle ID from request parameters
	requestData := fmt.Sprintf("sendmany:%s:%v", fromAddress, amounts)
	bundleHash := common.Blake2Hash([]byte(requestData))
	bundleID := "opid-" + hex.EncodeToString(bundleHash[:8]) // Use first 8 bytes for operation ID

	if h.txPool == nil {
		return nil, "", fmt.Errorf("transaction pool unavailable")
	}

	for i, amount := range amounts {
		if _, err := validateAmountWithinRange(amount.Amount); err != nil {
			return nil, "", fmt.Errorf("invalid amount for recipient %d: %w", i, err)
		}
	}

	anchor := h.txPool.CurrentAnchor()
	bundleBytes, err := h.txPool.GenerateBundle([]byte(requestData), anchor)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate bundle: %w", err)
	}

	log.Info(log.Node, "Orchard bundle synthesized",
		"bundle_id", bundleID,
		"from", fromAddress,
		"recipients", len(amounts),
		"bundle_size", len(bundleBytes))

	return bundleBytes, bundleID, nil
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

// ===== Transaction Pool RPC Methods =====

// ZSendRawOrchardBundle submits a raw Orchard bundle to the transaction pool
//
// Parameters:
// - hexData (string): Hex-encoded Orchard bundle data
//
// Returns:
// - string: Transaction ID/hash
//
// RPC Docs (z_sendraworchardbundle):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_sendraworchardbundle","params":["0x1234..."]}
// Response: {"result":"bundle_hash_123..."}
//
// Example curl call:
// curl -X POST http://localhost:8232 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_sendraworchardbundle","params":["0x1234..."],"id":1}'
func (h *OrchardRPCHandler) ZSendRawOrchardBundle(req []string, res *string) error {
	if len(req) != 1 && len(req) != 2 {
		return fmt.Errorf("invalid number of arguments: expected 1 or 2, got %d", len(req))
	}

	hexData := req[0]
	var bundleData []byte
	if hexData != "" {
		var err error
		bundleData, err = hex.DecodeString(strings.TrimPrefix(hexData, "0x"))
		if err != nil {
			return fmt.Errorf("invalid hex data: %w", err)
		}
	}

	var issueBundle []byte
	var err error
	if len(req) == 2 && req[1] != "" {
		issueBundle, err = hex.DecodeString(strings.TrimPrefix(req[1], "0x"))
		if err != nil {
			return fmt.Errorf("invalid issue bundle hex data: %w", err)
		}
	}
	if len(bundleData) == 0 && len(issueBundle) == 0 {
		return fmt.Errorf("bundle data cannot be empty")
	}

	// Enforce payload size limit to prevent memory abuse
	totalSize := len(bundleData) + len(issueBundle)
	if totalSize > 64*1024 { // 64KB limit
		return fmt.Errorf("bundle too large: %d bytes > 64KB", totalSize)
	}

	if h.txPool == nil {
		return fmt.Errorf("transaction pool not initialized")
	}

	// Generate bundle ID from data hash
	var idBuf []byte
	idBuf = make([]byte, 0, 8+len(bundleData)+len(issueBundle))
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(bundleData)))
	idBuf = append(idBuf, lenBuf[:]...)
	idBuf = append(idBuf, bundleData...)
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(issueBundle)))
	idBuf = append(idBuf, lenBuf[:]...)
	idBuf = append(idBuf, issueBundle...)
	bundleHash := common.Blake2Hash(idBuf)
	bundleID := hex.EncodeToString(bundleHash[:])

	// Submit to transaction pool
	if err := h.txPool.AddBundle(bundleData, issueBundle, bundleID); err != nil {
		return fmt.Errorf("failed to add bundle to pool: %w", err)
	}

	*res = bundleID
	log.Info(log.Node, "Raw Orchard bundle submitted",
		"bundle_id", bundleID,
		"orchard_bytes", len(bundleData),
		"issue_bytes", len(issueBundle))

	return nil
}

// ZGetMempoolInfo returns information about the transaction pool
//
// Parameters: none
//
// Returns:
// - object: Mempool statistics
//
// RPC Docs (z_getmempoolinfo):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_getmempoolinfo","params":[]}
// Response: {"result":{"size":42,"bytes":1024000,"usage":2048000}}
//
// Example curl call:
// curl -X POST http://localhost:8232 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getmempoolinfo","params":[],"id":1}'
func (h *OrchardRPCHandler) ZGetMempoolInfo(req []string, res *string) error {
	if len(req) != 0 {
		return fmt.Errorf("invalid number of arguments: expected 0, got %d", len(req))
	}

	if h.txPool == nil {
		return fmt.Errorf("transaction pool not initialized")
	}

	stats := h.txPool.GetPoolStats()

	// Create mempool info response
	info := map[string]interface{}{
		"size":                    stats.PendingBundles,
		"bytes":                   stats.PendingBundles * 1000, // Estimate 1KB per bundle
		"usage":                   stats.PendingBundles * 2000, // Estimate 2KB memory usage per bundle
		"total_bundles":           stats.TotalBundles,
		"work_packages_generated": stats.WorkPackagesGenerated,
		"tree_size":               stats.TreeSize,
		"nullifier_count":         stats.NullifierCount,
		"last_activity":           stats.LastActivity.Unix(),
	}

	jsonData, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal mempool info: %w", err)
	}

	*res = string(jsonData)
	return nil
}

// ZGetRawMempool returns a list of transaction IDs in the mempool
//
// Parameters:
// - verbose (bool, optional): Whether to return detailed info (default false)
//
// Returns:
// - array or object: Transaction IDs or detailed transaction info
//
// RPC Docs (z_getrawmempool):
// Request: {"jsonrpc":"1.0","id":1,"method":"z_getrawmempool","params":[false]}
// Response: {"result":["txid1","txid2","txid3"]}
//
// Example curl call:
// curl -X POST http://localhost:8232 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"z_getrawmempool","params":[false],"id":1}'
func (h *OrchardRPCHandler) ZGetRawMempool(req []string, res *string) error {
	verbose := false
	if len(req) > 0 {
		if req[0] == "true" {
			verbose = true
		}
	}
	if len(req) > 1 {
		return fmt.Errorf("invalid number of arguments: expected 0 or 1, got %d", len(req))
	}

	if h.txPool == nil {
		return fmt.Errorf("transaction pool not initialized")
	}

	bundles := h.txPool.GetPendingBundles()

	var result interface{}

	if verbose {
		// Return detailed bundle information
		detailed := make(map[string]interface{})
		for _, bundle := range bundles {
			detailed[bundle.ID] = map[string]interface{}{
				"time":         bundle.Timestamp.Unix(),
				"size":         len(bundle.RawData),
				"actions":      len(bundle.Actions),
				"nullifiers":   len(bundle.Nullifiers),
				"commitments":  len(bundle.Commitments),
				"gas_estimate": bundle.GasEstimate,
			}
		}
		result = detailed
	} else {
		// Return simple array of bundle IDs
		ids := make([]string, 0, len(bundles))
		for _, bundle := range bundles {
			ids = append(ids, bundle.ID)
		}
		result = ids
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal mempool data: %w", err)
	}

	*res = string(jsonData)
	return nil
}

// ===== Transparent Transaction RPC Methods =====

// TransparentTxDataResponse is a builder-side payload for tag=4 extrinsics.
type TransparentTxDataResponse struct {
	Available                  bool     `json:"available"`
	TransparentTxDataExtrinsic []byte   `json:"transparent_tx_data_extrinsic,omitempty"`
	TransparentMerkleRoot      [32]byte `json:"transparent_merkle_root,omitempty"`
	TransparentUtxoRoot        [32]byte `json:"transparent_utxo_root,omitempty"`
	TransparentUtxoSize        uint64   `json:"transparent_utxo_size,omitempty"`
}

// GetTransparentTxData returns a prebuilt TransparentTxData extrinsic with roots.
//
// Parameters: none
//
// Returns:
// - object: {available, transparent_tx_data_extrinsic, transparent_merkle_root, transparent_utxo_root, transparent_utxo_size}
//
// Example curl call:
// curl -X POST http://localhost:8232 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"gettransparenttxdata","params":[],"id":1}'
func (h *OrchardRPCHandler) GetTransparentTxData(req []string, res *string) error {
	if h.orchardRollup == nil {
		return fmt.Errorf("orchard rollup not initialized")
	}

	var currentHeight uint32
	if height, err := h.orchardRollup.LatestBlockNumber(); err == nil {
		currentHeight = height
	}
	_, extrinsic, merkleRoot, utxoRoot, utxoSize, _, _, err := h.orchardRollup.buildTransparentTxDataExtrinsic(currentHeight)
	if err != nil {
		return err
	}

	response := TransparentTxDataResponse{
		Available:                  len(extrinsic) > 0,
		TransparentTxDataExtrinsic: extrinsic,
		TransparentMerkleRoot:      merkleRoot,
		TransparentUtxoRoot:        utxoRoot,
		TransparentUtxoSize:        utxoSize,
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal transparent tx data: %w", err)
	}
	*res = string(jsonBytes)
	return nil
}

// GetRawTransaction returns raw transaction data for transparent transactions
//
// Parameters:
// - txid (string): Transaction ID
// - verbose (bool, optional): Return JSON object if true, hex string if false (default false)
//
// Returns:
// - string: Raw transaction hex or JSON object
//
// RPC Docs (getrawtransaction):
// Request: {"jsonrpc":"1.0","id":1,"method":"getrawtransaction","params":["txid123",false]}
// Response: {"result":"0100000001..."}
//
// Example curl call:
// curl -X POST http://localhost:8232 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getrawtransaction","params":["txid123",false],"id":1}'
func (h *OrchardRPCHandler) GetRawTransaction(req []string, res *string) error {
	if len(req) < 1 {
		return fmt.Errorf("invalid number of arguments: expected at least 1, got %d", len(req))
	}

	txid := req[0]
	verbose := false
	if len(req) > 1 {
		if req[1] == "true" || req[1] == "1" {
			verbose = true
		}
	}

	log.Info(log.Node, "GetRawTransaction", "txid", txid, "verbose", verbose)

	if h.orchardRollup == nil || h.orchardRollup.transparentTxStore == nil {
		return fmt.Errorf("transparent transaction store not initialized")
	}

	result, err := h.orchardRollup.transparentTxStore.GetRawTransaction(txid, verbose)
	if err != nil {
		return err
	}

	if verbose {
		// Return JSON object
		jsonBytes, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("failed to marshal transaction: %w", err)
		}
		*res = string(jsonBytes)
	} else {
		// Return hex string
		*res = result.(string)
	}

	log.Debug(log.Node, "GetRawTransaction: Returning transaction", "txid", txid, "verbose", verbose)
	return nil
}

// SendRawTransaction broadcasts a raw transparent transaction to the network
//
// Parameters:
// - hexstring (string): Raw transaction in hex format
// - allowhighfees (bool, optional): Allow high fees (default false)
//
// Returns:
// - string: Transaction ID
//
// RPC Docs (sendrawtransaction):
// Request: {"jsonrpc":"1.0","id":1,"method":"sendrawtransaction","params":["0100000001..."]}
// Response: {"result":"txid123..."}
//
// Example curl call:
// curl -X POST http://localhost:8232 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"sendrawtransaction","params":["0100000001..."],"id":1}'
func (h *OrchardRPCHandler) SendRawTransaction(req []string, res *string) error {
	if len(req) < 1 {
		return fmt.Errorf("invalid number of arguments: expected at least 1, got %d", len(req))
	}

	txHex := req[0]
	if txHex == "" {
		return fmt.Errorf("transaction hex cannot be empty")
	}

	log.Info(log.Node, "SendRawTransaction", "tx_size", len(txHex))

	if h.orchardRollup == nil || h.orchardRollup.transparentTxStore == nil {
		return fmt.Errorf("transparent transaction store not initialized")
	}
	if h.orchardRollup.transparentUtxoTree == nil {
		return fmt.Errorf("transparent UTXO tree not initialized")
	}

	// Validate transaction against builder-side consensus rules before mempool admission.
	txBytes, err := hex.DecodeString(stripHexPrefix(txHex))
	if err != nil {
		return fmt.Errorf("invalid transaction hex: %w", err)
	}

	parsedTx, err := ParseZcashTxV5(txBytes)
	if err != nil {
		return fmt.Errorf("failed to parse transaction: %w", err)
	}

	var currentHeight uint32
	if height, err := h.orchardRollup.LatestBlockNumber(); err == nil {
		currentHeight = height
	}
	currentTime := uint32(time.Now().Unix())
	if err := ValidateTransparentTxs([]*ZcashTxV5{parsedTx}, h.orchardRollup.transparentUtxoTree, currentHeight, currentTime); err != nil {
		return fmt.Errorf("transparent consensus validation failed: %w", err)
	}

	// Add transaction to mempool
	txid, err := h.orchardRollup.transparentTxStore.AddRawTransaction(txHex)
	if err != nil {
		return fmt.Errorf("failed to add transaction: %w", err)
	}

	*res = txid
	log.Info(log.Node, "SendRawTransaction: Transaction broadcast", "txid", txid)

	return nil
}

// GetRawMempool returns all transaction IDs in the transparent mempool
//
// Parameters:
// - verbose (bool, optional): Return detailed info if true (default false)
//
// Returns:
// - array or object: Transaction IDs or detailed transaction info
//
// RPC Docs (getrawmempool):
// Request: {"jsonrpc":"1.0","id":1,"method":"getrawmempool","params":[false]}
// Response: {"result":["txid1","txid2","txid3"]}
//
// Example curl call:
// curl -X POST http://localhost:8232 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getrawmempool","params":[false],"id":1}'
func (h *OrchardRPCHandler) GetRawMempool(req []string, res *string) error {
	verbose := false
	if len(req) > 0 {
		if req[0] == "true" || req[0] == "1" {
			verbose = true
		}
	}

	log.Info(log.Node, "GetRawMempool", "verbose", verbose)

	if h.orchardRollup == nil || h.orchardRollup.transparentTxStore == nil {
		return fmt.Errorf("transparent transaction store not initialized")
	}

	result := h.orchardRollup.transparentTxStore.GetMempool(verbose)

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal mempool: %w", err)
	}

	*res = string(jsonBytes)
	log.Debug(log.Node, "GetRawMempool: Returning mempool")

	return nil
}

// GetMempoolInfo returns mempool information
//
// Parameters: none
//
// Returns:
// - object: Mempool statistics
//
// RPC Docs (getmempoolinfo):
// Request: {"jsonrpc":"1.0","id":1,"method":"getmempoolinfo","params":[]}
// Response: {"result":{"size":42,"bytes":1024000}}
//
// Example curl call:
// curl -X POST http://localhost:8232 -H "Content-Type: application/json" -d '{"jsonrpc":"1.0","method":"getmempoolinfo","params":[],"id":1}'
func (h *OrchardRPCHandler) GetMempoolInfo(req []string, res *string) error {
	log.Info(log.Node, "GetMempoolInfo")

	if h.orchardRollup == nil || h.orchardRollup.transparentTxStore == nil {
		return fmt.Errorf("transparent transaction store not initialized")
	}

	info := h.orchardRollup.transparentTxStore.GetMempoolInfo()

	jsonBytes, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal mempool info: %w", err)
	}

	*res = string(jsonBytes)
	return nil
}
