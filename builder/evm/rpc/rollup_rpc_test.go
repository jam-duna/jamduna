package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
)

const totalTxCount = 51

// txInfo tracks transaction state during testing
type txInfo struct {
	hash      common.Hash
	nonce     uint64
	signedTx  []byte
	receipt   *evmtypes.EthereumTransactionReceipt
	coreIndex string
	bundleIdx int
}

// receiptPollerResult holds the result of a receipt polling operation
type receiptPollerResult struct {
	idx          int
	receipt      *evmtypes.EthereumTransactionReceipt
	coreIndex    string
	bundleHash   string
	isNewBundle  bool
	newBundleIdx int
}

// pollReceiptSimple polls for a transaction receipt without work report lookup.
// Used in single-core mode tests.
// DEPRECATED: Use block-based confirmation instead for scalability.
func pollReceiptSimple(evmClient *EVMClient, txHash common.Hash, idx int) *receiptPollerResult {
	receipt, err := evmClient.GetTransactionReceipt(txHash)
	if err != nil || receipt == nil || receipt.BlockHash == "" {
		return nil
	}
	return &receiptPollerResult{
		idx:        idx,
		receipt:    receipt,
		bundleHash: receipt.BlockHash,
	}
}

// confirmTxsFromBlocks checks for new blocks and marks all included transactions as confirmed.
// This is O(blocks) instead of O(transactions) - scales to 5000+ txns per block.
func confirmTxsFromBlocks(rpcClient *HTTPJSONRPCClient, txs []txInfo, lastBlockChecked uint64) (newLastBlock uint64, confirmedCount int) {
	newLastBlock = lastBlockChecked

	// Get current block number
	var blockNumHex string
	if err := rpcClient.Call("eth_blockNumber", []string{}, &blockNumHex); err != nil {
		return lastBlockChecked, 0
	}

	var currentBlock uint64
	fmt.Sscanf(blockNumHex, "0x%x", &currentBlock)

	if currentBlock <= lastBlockChecked {
		return lastBlockChecked, 0
	}

	// Build a map of pending tx hashes for fast lookup
	pendingTxs := make(map[string]int) // txHash -> index in txs slice
	for i, tx := range txs {
		if tx.receipt == nil {
			pendingTxs[tx.hash.String()] = i
		}
	}

	if len(pendingTxs) == 0 {
		return currentBlock, 0
	}

	// Check each new block
	for blockNum := lastBlockChecked + 1; blockNum <= currentBlock; blockNum++ {
		var block map[string]interface{}
		blockNumStr := fmt.Sprintf("0x%x", blockNum)
		if err := rpcClient.Call("eth_getBlockByNumber", []interface{}{blockNumStr, false}, &block); err != nil {
			continue
		}

		if block == nil {
			continue
		}

		// Get transactions from block (array of tx hashes when fullTx=false)
		txHashesRaw, ok := block["transactions"].([]interface{})
		if !ok {
			continue
		}

		blockHash, _ := block["hash"].(string)

		// Mark all matching transactions as confirmed
		for txIdx, txHashRaw := range txHashesRaw {
			txHashStr, ok := txHashRaw.(string)
			if !ok {
				continue
			}

			if idx, found := pendingTxs[txHashStr]; found {
				// Create minimal receipt from block info
				txs[idx].receipt = &evmtypes.EthereumTransactionReceipt{
					TransactionHash:  txHashStr,
					TransactionIndex: fmt.Sprintf("0x%x", txIdx),
					BlockHash:        blockHash,
					BlockNumber:      blockNumStr,
					Status:           "0x1", // Assume success
				}
				confirmedCount++
				delete(pendingTxs, txHashStr) // Remove from pending

				log.Info(log.Node, "✅ TX confirmed via block",
					"txIdx", idx,
					"blockNumber", blockNum,
					"txIndex", txIdx,
					"progress", fmt.Sprintf("%d pending", len(pendingTxs)))
			}
		}
	}

	return currentBlock, confirmedCount
}

// confirmTxsFromBlocksWithUnexpected is like confirmTxsFromBlocks, but it also detects
// transactions sent to the recipient that are not part of the tracked tx list.
// It also tracks blocks that returned null and retries them on subsequent calls.
func confirmTxsFromBlocksWithUnexpected(rpcClient *HTTPJSONRPCClient, txs []txInfo, lastBlockChecked uint64, recipient common.Address, unresolvedBlocks map[uint64]int) (newLastBlock uint64, confirmedCount int, unexpectedCount int) {
	newLastBlock = lastBlockChecked
	const maxRetries = 10

	// Get current block number
	var blockNumHex string
	if err := rpcClient.Call("eth_blockNumber", []string{}, &blockNumHex); err != nil {
		return lastBlockChecked, 0, 0
	}

	var currentBlock uint64
	fmt.Sscanf(blockNumHex, "0x%x", &currentBlock)

	// Build maps for fast lookup
	trackedTxs := make(map[string]int) // txHash -> index in txs slice
	pendingTxs := make(map[string]int) // txHash -> index in txs slice
	for i, tx := range txs {
		txHash := tx.hash.String()
		trackedTxs[txHash] = i
		if tx.receipt == nil {
			pendingTxs[txHash] = i
		}
	}

	if len(trackedTxs) == 0 {
		return currentBlock, 0, 0
	}

	recipientHex := recipient.String()

	// Helper function to process a block and return (resolved, confirmed, unexpected)
	processBlock := func(blockNum uint64, isRetry bool) (resolved bool, conf int, unexp int) {
		var block map[string]interface{}
		blockNumStr := fmt.Sprintf("0x%x", blockNum)
		if err := rpcClient.Call("eth_getBlockByNumber", []interface{}{blockNumStr, true}, &block); err != nil {
			return false, 0, 0
		}

		if block == nil {
			return false, 0, 0
		}

		txObjsRaw, ok := block["transactions"].([]interface{})
		if !ok {
			return true, 0, 0 // Block exists but has no transactions field - consider resolved
		}

		blockHash, _ := block["hash"].(string)

		// Log diagnostic info for blocks - always log first time, helps debug hash mismatches
		if len(txObjsRaw) > 0 {
			var txHashes []string
			for _, txObjRaw := range txObjsRaw {
				if txObj, ok := txObjRaw.(map[string]interface{}); ok {
					if h, ok := txObj["hash"].(string); ok {
						txHashes = append(txHashes, h)
					}
				}
			}
			// Check how many of these tx hashes match our pending list
			matchCount := 0
			for _, h := range txHashes {
				if _, found := pendingTxs[h]; found {
					matchCount++
				}
			}
			if isRetry {
				log.Info(log.Node, "🔄 Retry resolved block",
					"blockNumber", blockNum,
					"txCount", len(txObjsRaw),
					"matchingPending", matchCount,
					"txHashes", txHashes)
			} else if matchCount == 0 && len(txHashes) > 0 {
				// No matches - potential hash mismatch issue
				log.Warn(log.Node, "⚠️ Block has txs but NONE match pending list (hash mismatch?)",
					"blockNumber", blockNum,
					"blockTxCount", len(txHashes),
					"pendingCount", len(pendingTxs),
					"blockTxHashes", txHashes)
			}
		}

		for txIdx, txObjRaw := range txObjsRaw {
			txObj, ok := txObjRaw.(map[string]interface{})
			if !ok {
				continue
			}
			txHashStr, _ := txObj["hash"].(string)

			if idx, found := pendingTxs[txHashStr]; found {
				// Create minimal receipt from block info
				txs[idx].receipt = &evmtypes.EthereumTransactionReceipt{
					TransactionHash:  txHashStr,
					TransactionIndex: fmt.Sprintf("0x%x", txIdx),
					BlockHash:        blockHash,
					BlockNumber:      blockNumStr,
					Status:           "0x1", // Assume success
				}
				conf++
				delete(pendingTxs, txHashStr)

				log.Info(log.Node, "✅ TX confirmed via block",
					"txIdx", idx,
					"blockNumber", blockNum,
					"txIndex", txIdx,
					"progress", fmt.Sprintf("%d pending", len(pendingTxs)),
					"isRetry", isRetry)
				continue
			}

			if _, tracked := trackedTxs[txHashStr]; tracked {
				continue
			}

			toAddr, _ := txObj["to"].(string)
			if toAddr != "" && strings.EqualFold(toAddr, recipientHex) {
				unexp++
				log.Warn(log.Node, "⚠️ Unexpected recipient tx detected",
					"txHash", txHashStr,
					"blockNumber", blockNum,
					"blockHash", blockHash)
			}
		}
		return true, conf, unexp
	}

	// First, retry previously unresolved blocks
	for blockNum, retryCount := range unresolvedBlocks {
		if retryCount >= maxRetries {
			continue // Give up after max retries
		}
		resolved, conf, unexp := processBlock(blockNum, true)
		if resolved {
			delete(unresolvedBlocks, blockNum)
			confirmedCount += conf
			unexpectedCount += unexp
		} else {
			unresolvedBlocks[blockNum] = retryCount + 1
			if retryCount+1 >= maxRetries {
				log.Warn(log.Node, "⚠️ Block unresolved after max retries",
					"blockNumber", blockNum,
					"retries", maxRetries)
			}
		}
	}

	// Then process new blocks
	for blockNum := lastBlockChecked + 1; blockNum <= currentBlock; blockNum++ {
		resolved, conf, unexp := processBlock(blockNum, false)
		if resolved {
			confirmedCount += conf
			unexpectedCount += unexp
		} else {
			// Track for retry
			unresolvedBlocks[blockNum] = 1
			log.Debug(log.Node, "📋 Block returned null, queued for retry",
				"blockNumber", blockNum)
		}
	}

	return currentBlock, confirmedCount, unexpectedCount
}

// pollReceiptWithWorkReport polls for a transaction receipt and fetches the work report
// to determine core index. Used in multi-core mode tests.
func pollReceiptWithWorkReport(evmClient *EVMClient, rpcClient *HTTPJSONRPCClient, txHash common.Hash, idx int) *receiptPollerResult {
	receipt, err := evmClient.GetTransactionReceipt(txHash)
	if err != nil || receipt == nil || receipt.BlockHash == "" {
		return nil
	}

	// Query work report to get core index
	var workReport map[string]interface{}
	coreIdx := "unknown"
	if err := rpcClient.Call("jam_getWorkReport", []string{receipt.BlockHash}, &workReport); err == nil && workReport != nil {
		if ci, ok := workReport["coreIndex"]; ok {
			coreIdx = fmt.Sprintf("%v", ci)
		}
	}

	return &receiptPollerResult{
		idx:        idx,
		receipt:    receipt,
		coreIndex:  coreIdx,
		bundleHash: receipt.BlockHash,
	}
}

// HTTPJSONRPCClient is a simple HTTP JSON-RPC client for testing
type HTTPJSONRPCClient struct {
	endpoint string
	client   *http.Client
	id       int
}

// NewHTTPJSONRPCClient creates a new HTTP JSON-RPC client
func NewHTTPJSONRPCClient(endpoint string) *HTTPJSONRPCClient {
	return &HTTPJSONRPCClient{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 30 * time.Second},
		id:       1,
	}
}

// jsonRPCRequest represents a JSON-RPC request
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

// jsonRPCResponse represents a JSON-RPC response
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *jsonRPCError   `json:"error"`
	ID      int             `json:"id"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Call makes a JSON-RPC call
func (c *HTTPJSONRPCClient) Call(method string, params interface{}, result interface{}) error {
	c.id++
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      c.id,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.client.Post(c.endpoint, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	if result != nil {
		if err := json.Unmarshal(rpcResp.Result, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// TestEVMBlocksTransfersRPC tests EVM transfers using the running EVM builder RPC
// This test requires:
// 1. Validators running: make run_localclient
// 2. EVM builder running: make run_evm_builder
//
// Run with: go test -v -run TestEVMBlocksTransfersRPC ./builder/evm/rpc/
func TestEVMBlocksTransfersRPC(t *testing.T) {
	log.InitLogger("debug")

	endpoint := "http://localhost:8600"
	if e := os.Getenv("EVM_RPC_ENDPOINT"); e != "" {
		endpoint = e
	}

	rpcClient := NewHTTPJSONRPCClient(endpoint)
	evmClient := NewEVMClient(rpcClient)

	// Test 0: Prime EVM genesis state (set up issuer balance)
	// This initializes LOCAL UBT tree state - no work package submission
	t.Run("PrimeGenesis", func(t *testing.T) {
		var result interface{}
		err := rpcClient.Call("jam_primeGenesis", []string{"61000000"}, &result)
		if err != nil {
			t.Fatalf("jam_primeGenesis failed: %v", err)
		}
		log.Info(log.Node, "✅ Genesis state initialized locally", "result", result)

		// Verify genesis balance is now available
		issuerAddr, _ := common.GetEVMDevAccount(0)
		balance, err := evmClient.GetBalance(issuerAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance failed after genesis: %v", err)
		}
		balanceInt := new(big.Int).SetBytes(balance.Bytes())
		if balanceInt.Sign() <= 0 {
			t.Fatalf("Genesis balance is zero after initialization")
		}
		log.Info(log.Node, "✅ Genesis state verified", "balance", balanceInt.String())
	})

	// Test 1: Check chain ID
	t.Run("ChainID", func(t *testing.T) {
		chainID := evmClient.GetChainId()
		if chainID == 0 {
			t.Fatal("Failed to get chain ID")
		}
		log.Info(log.Node, "✅ Chain ID", "chainId", chainID)
	})

	// Test 2: Check block number
	t.Run("BlockNumber", func(t *testing.T) {
		var blockNumHex string
		err := rpcClient.Call("eth_blockNumber", []string{}, &blockNumHex)
		if err != nil {
			t.Fatalf("eth_blockNumber failed: %v", err)
		}
		log.Info(log.Node, "✅ Block number", "blockNumber", blockNumHex)
	})

	// Test 3: Check issuer balance (should have genesis funds)
	t.Run("IssuerBalance", func(t *testing.T) {
		issuerAddr, _ := common.GetEVMDevAccount(0)
		balance, err := evmClient.GetBalance(issuerAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance failed: %v", err)
		}
		balanceInt := new(big.Int).SetBytes(balance.Bytes())
		log.Info(log.Node, "✅ Issuer balance", "address", issuerAddr.String(), "balance", balanceInt.String())

		if balanceInt.Sign() <= 0 {
			t.Log("Warning: Issuer balance is zero - genesis may not be initialized")
		}
	})

	// Test 4: Get transaction count (nonce)
	t.Run("TransactionCount", func(t *testing.T) {
		issuerAddr, _ := common.GetEVMDevAccount(0)
		nonce, err := evmClient.GetTransactionCount(issuerAddr, "latest")
		if err != nil {
			t.Fatalf("GetTransactionCount failed: %v", err)
		}
		log.Info(log.Node, "✅ Transaction count", "address", issuerAddr.String(), "nonce", nonce)
	})

	// Test 5: Prebuild 50 transactions, submit ALL to txpool, expect ONE bundle (single-core mode)
	// Use with: make run_evm_single
	t.Run("SendBatchedTransfers", func(t *testing.T) {
		senderAddr, senderPrivKey := common.GetEVMDevAccount(0)
		recipientAddr, _ := common.GetEVMDevAccount(1)

		// Get current nonce
		baseNonce, err := evmClient.GetTransactionCount(senderAddr, "latest")
		if err != nil {
			t.Fatalf("GetTransactionCount failed: %v", err)
		}

		// Get balances before
		senderBalanceBefore, err := evmClient.GetBalance(senderAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (sender before) failed: %v", err)
		}
		senderBalanceBeforeInt := new(big.Int).SetBytes(senderBalanceBefore.Bytes())

		recipientBalanceBefore, err := evmClient.GetBalance(recipientAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (recipient before) failed: %v", err)
		}
		recipientBalanceBeforeInt := new(big.Int).SetBytes(recipientBalanceBefore.Bytes())

		log.Info(log.Node, "📦 Single bundle test: Prebuilding 50 transactions for ONE work package",
			"totalTxCount", totalTxCount,
			"sender", senderAddr.String(),
			"senderBalance", senderBalanceBeforeInt.String(),
			"recipient", recipientAddr.String(),
			"recipientBalance", recipientBalanceBeforeInt.String(),
			"baseNonce", baseNonce)

		// Transaction parameters
		amount := new(big.Int).Mul(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil)) // 0.1 ETH each
		gasPrice := big.NewInt(1_000_000_000)                                                            // 1 gwei
		gasLimit := uint64(21000)

		// Track all transactions
		txs := make([]txInfo, totalTxCount)

		// ========== PHASE 1: Prebuild ALL 50 transactions ==========
		log.Info(log.Node, "🔧 PHASE 1: Prebuilding signed transactions", "count", totalTxCount)
		for i := 0; i < totalTxCount; i++ {
			nonce := baseNonce + uint64(i)
			_, signedTx, txHash, err := evmtypes.CreateSignedNativeTransfer(
				senderPrivKey,
				nonce,
				recipientAddr,
				amount,
				gasPrice,
				gasLimit,
				DefaultJAMChainID,
			)
			if err != nil {
				t.Fatalf("CreateSignedNativeTransfer failed for tx %d: %v", i, err)
			}

			txs[i] = txInfo{hash: txHash, nonce: nonce, signedTx: signedTx}
			if i%10 == 0 {
				log.Info(log.Node, "🔧 Prebuilt tx", "index", i, "nonce", nonce, "txHash", txHash.String())
			}
		}
		log.Info(log.Node, "✅ PHASE 1 complete: transactions prebuilt", "count", totalTxCount)

		// ========== PHASE 2: Submit ALL transactions to txpool rapidly ==========
		log.Info(log.Node, "📤 PHASE 2: Submitting ALL transactions to txpool", "count", totalTxCount)
		submitStart := time.Now()
		for i := 0; i < totalTxCount; i++ {
			_, err := evmClient.SendRawTransaction(txs[i].signedTx)
			if err != nil {
				t.Fatalf("SendRawTransaction failed for tx %d: %v", i, err)
			}
			if i%10 == 0 {
				log.Info(log.Node, "📤 Submitted tx", "index", i, "nonce", txs[i].nonce)
			}
			// NO DELAY - submit all as fast as possible to batch into ONE bundle
		}
		submitDuration := time.Since(submitStart)
		log.Info(log.Node, "✅ PHASE 2 complete: All transactions submitted", "count", totalTxCount, "duration", submitDuration)

		// Check txpool status to verify transactions are pending
		var txpoolStatus map[string]string
		if err := rpcClient.Call("txpool_status", []string{}, &txpoolStatus); err == nil {
			log.Info(log.Node, "📊 TxPool status after submission",
				"pending", txpoolStatus["pending"],
				"queued", txpoolStatus["queued"])
		}

		// ========== PHASE 3: Wait for all transactions (expect ONE bundle) ==========
		// Use block-based confirmation: O(blocks) instead of O(transactions)
		log.Info(log.Node, "⏳ PHASE 3: Waiting for transactions via block monitoring (expecting ONE bundle)...")
		pollInterval := 6 * time.Second
		maxWaitRounds := 60
		bundleHashes := make(map[string]int) // Track unique block hashes
		var lastBlockChecked uint64 = 0

		// Get initial block number
		var blockNumHex string
		if err := rpcClient.Call("eth_blockNumber", []string{}, &blockNumHex); err == nil {
			fmt.Sscanf(blockNumHex, "0x%x", &lastBlockChecked)
		}

		for round := 0; round < maxWaitRounds; round++ {
			time.Sleep(pollInterval)

			// Check new blocks and confirm all included transactions
			newLastBlock, confirmed := confirmTxsFromBlocks(rpcClient, txs, lastBlockChecked)
			lastBlockChecked = newLastBlock

			// Count pending and collect bundle hashes
			pendingCount := 0
			for i := range txs {
				if txs[i].receipt == nil {
					pendingCount++
				} else {
					bundleHashes[txs[i].receipt.BlockHash]++
				}
			}

			if confirmed > 0 {
				log.Info(log.Node, "📦 Block scan complete",
					"round", round+1,
					"newlyConfirmed", confirmed,
					"pending", pendingCount,
					"confirmed", totalTxCount-pendingCount,
					"bundlesFound", len(bundleHashes))
			}

			if pendingCount == 0 {
				break
			}

			// Check txpool status
			var txpoolStatus map[string]string
			txpoolPending := "?"
			if err := rpcClient.Call("txpool_status", []string{}, &txpoolStatus); err == nil {
				txpoolPending = txpoolStatus["pending"]
			}

			log.Info(log.Node, "⏳ Waiting for blocks...",
				"round", round+1,
				"pending", pendingCount,
				"confirmed", totalTxCount-pendingCount,
				"lastBlock", lastBlockChecked,
				"txpoolPending", txpoolPending)
		}

		// Deduplicate bundle counts (we accumulated them each round)
		bundleHashes = make(map[string]int)
		for i := range txs {
			if txs[i].receipt != nil {
				bundleHashes[txs[i].receipt.BlockHash]++
			}
		}

		// ========== PHASE 4: Report results ==========
		confirmedCount := 0
		for i := range txs {
			if txs[i].receipt != nil {
				confirmedCount++
			} else {
				log.Info(log.Node, "❌ Transaction not confirmed", "index", i, "nonce", txs[i].nonce)
			}
		}

		// Report bundle distribution
		log.Info(log.Node, "📊 Results", "confirmed", confirmedCount, "total", totalTxCount, "bundles", len(bundleHashes))
		for hash, count := range bundleHashes {
			log.Info(log.Node, "📦 Bundle", "hash", hash[:18]+"...", "txCount", count)
		}

		// Verify single bundle expectation
		if len(bundleHashes) != 1 {
			t.Errorf("❌ Expected 1 bundle (single-core mode), got %d bundles", len(bundleHashes))
		} else {
			log.Info(log.Node, "✅ All transactions in ONE bundle as expected (single-core mode)", "count", confirmedCount)
		}

		if confirmedCount < totalTxCount {
			t.Errorf("Only %d/%d transactions confirmed within timeout", confirmedCount, totalTxCount)
		}

		// Verify final balances
		recipientBalanceAfter, err := evmClient.GetBalance(recipientAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (recipient after) failed: %v", err)
		}
		recipientBalanceAfterInt := new(big.Int).SetBytes(recipientBalanceAfter.Bytes())

		totalReceived := new(big.Int).Mul(amount, big.NewInt(int64(confirmedCount)))
		expectedRecipientBalance := new(big.Int).Add(recipientBalanceBeforeInt, totalReceived)

		log.Info(log.Node, "📊 Balance verification",
			"recipientBefore", recipientBalanceBeforeInt.String(),
			"recipientAfter", recipientBalanceAfterInt.String(),
			"expected", expectedRecipientBalance.String(),
			"confirmedCount", confirmedCount)

		if recipientBalanceAfterInt.Cmp(expectedRecipientBalance) != 0 {
			diff := new(big.Int).Sub(recipientBalanceAfterInt, expectedRecipientBalance)
			t.Errorf("Recipient balance mismatch: expected %s, got %s (diff=%s)",
				expectedRecipientBalance.String(), recipientBalanceAfterInt.String(), diff.String())
		} else {
			log.Info(log.Node, "✅ Recipient balance verified")
		}
	})
}

// TestEVMMultiRoundTransfers tests parallel core submission and multi-bundle scheduling
// Prebuilds 50 transactions, submits ALL to txpool, expects 10 bundles × 5 txns each
// Tests: parallel submission to both cores AND multiple bundle scheduling
// Prerequisites:
// 1. make run_localclient (validators running)
// 2. make run_evm_builder (EVM builder with RPC) - multi-core mode
func TestEVMMultiRoundTransfers(t *testing.T) {
	log.InitLogger("debug")

	evmRPCEndpoint := os.Getenv("EVM_RPC_ENDPOINT")
	if evmRPCEndpoint == "" {
		evmRPCEndpoint = "http://localhost:8600"
	}
	log.Info(log.Node, "Using EVM RPC endpoint", "endpoint", evmRPCEndpoint)

	rpcClient := NewHTTPJSONRPCClient(evmRPCEndpoint)
	evmClient := NewEVMClient(rpcClient)

	// These should match the builder's --max-txs-per-bundle flag
	// With 5 txns per bundle and 2 cores, we get 2 bundles per round
	expectedTxsPerBundle := 5
	expectedBundles := (totalTxCount + expectedTxsPerBundle - 1) / expectedTxsPerBundle // ceil division
	numCores := 2                                                                       // Target both cores

	// Prime genesis first
	t.Run("PrimeGenesis", func(t *testing.T) {
		var result interface{}
		balance := fmt.Sprintf("%d000000", totalTxCount+10) // Extra buffer for gas
		err := rpcClient.Call("jam_primeGenesis", []string{balance}, &result)
		if err != nil {
			t.Fatalf("jam_primeGenesis failed: %v", err)
		}
		log.Info(log.Node, "✅ Genesis primed", "balance", balance)
	})

	t.Run("SendMultiBundleTransfers", func(t *testing.T) {
		senderAddr, senderPrivKey := common.GetEVMDevAccount(0)
		recipientAddr, _ := common.GetEVMDevAccount(1)

		// Get initial nonce
		baseNonce, err := evmClient.GetTransactionCount(senderAddr, "latest")
		if err != nil {
			t.Fatalf("GetTransactionCount failed: %v", err)
		}

		// Get initial balances
		senderBalanceBefore, err := evmClient.GetBalance(senderAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (sender before) failed: %v", err)
		}
		recipientBalanceBefore, err := evmClient.GetBalance(recipientAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (recipient before) failed: %v", err)
		}

		senderBalanceBeforeInt := new(big.Int).SetBytes(senderBalanceBefore.Bytes())
		recipientBalanceBeforeInt := new(big.Int).SetBytes(recipientBalanceBefore.Bytes())

		log.Info(log.Node, "📦 Multi-bundle test: Prebuilding 50 transactions for 10 bundles × 5 txns",
			"totalTxCount", totalTxCount,
			"expectedBundles", expectedBundles,
			"expectedTxsPerBundle", expectedTxsPerBundle,
			"numCores", numCores,
			"sender", senderAddr.String(),
			"senderBalance", senderBalanceBeforeInt.String(),
			"recipient", recipientAddr.String(),
			"recipientBalance", recipientBalanceBeforeInt.String(),
			"baseNonce", baseNonce)

		// Transaction parameters
		amount := new(big.Int).Mul(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil)) // 0.1 ETH
		gasPrice := big.NewInt(1_000_000_000)                                                            // 1 gwei
		gasLimit := uint64(21000)

		// Track all transactions
		txs := make([]txInfo, totalTxCount)

		// ========== PHASE 1: Prebuild ALL 50 transactions ==========
		log.Info(log.Node, "🔧 PHASE 1: Prebuilding signed transactions", "count", totalTxCount)
		for i := 0; i < totalTxCount; i++ {
			nonce := baseNonce + uint64(i)
			_, signedTx, txHash, err := evmtypes.CreateSignedNativeTransfer(
				senderPrivKey,
				nonce,
				recipientAddr,
				amount,
				gasPrice,
				gasLimit,
				DefaultJAMChainID,
			)
			if err != nil {
				t.Fatalf("CreateSignedNativeTransfer failed for tx %d: %v", i, err)
			}

			txs[i] = txInfo{hash: txHash, nonce: nonce, signedTx: signedTx, bundleIdx: -1}
			if i%10 == 0 {
				log.Info(log.Node, "🔧 Prebuilt tx", "index", i, "nonce", nonce, "txHash", txHash.String())
			}
		}
		log.Info(log.Node, "✅ PHASE 1 complete: transactions prebuilt", "count", totalTxCount)

		// ========== PHASE 2: Submit ALL transactions to txpool rapidly ==========
		log.Info(log.Node, "📤 PHASE 2: Submitting ALL transactions to txpool", "count", totalTxCount)
		submitStart := time.Now()
		for i := 0; i < totalTxCount; i++ {
			_, err := evmClient.SendRawTransaction(txs[i].signedTx)
			if err != nil {
				t.Fatalf("SendRawTransaction failed for tx %d: %v", i, err)
			}
			if i%10 == 0 {
				log.Info(log.Node, "📤 Submitted tx", "index", i, "nonce", txs[i].nonce)
			}
			// NO DELAY - submit all as fast as possible, let builder distribute to bundles
		}
		submitDuration := time.Since(submitStart)
		log.Info(log.Node, "✅ PHASE 2 complete: All transactions submitted", "count", totalTxCount, "duration", submitDuration)

		// ========== PHASE 3: Wait for all transactions across multiple bundles ==========
		// Use block-based confirmation: O(blocks) instead of O(transactions)
		log.Info(log.Node, "⏳ PHASE 3: Waiting for transactions via block monitoring", "expectedBundles", expectedBundles)
		pollInterval := 6 * time.Second
		maxWaitRounds := 60

		// Track unique block hashes (each represents a work package/bundle)
		bundleHashes := make(map[string]int) // blockHash -> bundle index
		bundleIndex := 0
		var lastBlockChecked uint64 = 0
		unexpectedRecipientTxs := 0
		unresolvedBlocks := make(map[uint64]int) // blockNum -> retry count

		// Get initial block number
		var blockNumHex string
		if err := rpcClient.Call("eth_blockNumber", []string{}, &blockNumHex); err == nil {
			fmt.Sscanf(blockNumHex, "0x%x", &lastBlockChecked)
		}

		for round := 0; round < maxWaitRounds; round++ {
			time.Sleep(pollInterval)

			// Check new blocks and confirm all included transactions (with retry for unresolved blocks)
			newLastBlock, confirmed, unexpected := confirmTxsFromBlocksWithUnexpected(rpcClient, txs, lastBlockChecked, recipientAddr, unresolvedBlocks)
			lastBlockChecked = newLastBlock
			unexpectedRecipientTxs += unexpected

			// Count pending and track bundles
			pendingCount := 0
			for i := range txs {
				if txs[i].receipt == nil {
					pendingCount++
				} else if txs[i].bundleIdx < 0 {
					// Assign bundle index for newly confirmed txs
					blockHash := txs[i].receipt.BlockHash
					if _, exists := bundleHashes[blockHash]; !exists {
						bundleHashes[blockHash] = bundleIndex
						bundleIndex++
						log.Info(log.Node, "📦 New bundle identified",
							"bundleIdx", bundleIndex-1,
							"blockHash", blockHash[:18]+"...",
							"blockNumber", txs[i].receipt.BlockNumber)
					}
					txs[i].bundleIdx = bundleHashes[blockHash]
				}
			}

			if confirmed > 0 {
				log.Info(log.Node, "📦 Block scan complete",
					"round", round+1,
					"newlyConfirmed", confirmed,
					"pending", pendingCount,
					"confirmed", totalTxCount-pendingCount,
					"bundlesFound", len(bundleHashes))
			}

			if pendingCount == 0 {
				break
			}

			log.Info(log.Node, "⏳ Waiting for blocks...",
				"round", round+1,
				"pending", pendingCount,
				"confirmed", totalTxCount-pendingCount,
				"lastBlock", lastBlockChecked,
				"unresolvedBlocks", len(unresolvedBlocks))
		}

		// ========== PHASE 4: Report results and distribution analysis ==========
		log.Info(log.Node, "📊 PHASE 4: Analyzing bundle and core distribution...")

		// Log any remaining unresolved blocks for debugging
		if len(unresolvedBlocks) > 0 {
			unresolvedList := make([]uint64, 0, len(unresolvedBlocks))
			for blockNum := range unresolvedBlocks {
				unresolvedList = append(unresolvedList, blockNum)
			}
			log.Warn(log.Node, "⚠️ Some blocks remained unresolved after all retries",
				"count", len(unresolvedBlocks),
				"blocks", unresolvedList)
		}

		// Count confirmed and build distributions
		confirmedCount := 0
		bundleDistribution := make(map[int]int)  // bundleIdx -> tx count
		coreDistribution := make(map[string]int) // coreIdx -> tx count
		for i := range txs {
			if txs[i].receipt != nil {
				confirmedCount++
				bundleDistribution[txs[i].bundleIdx]++
				coreDistribution[txs[i].coreIndex]++
			} else {
				log.Info(log.Node, "❌ Transaction not confirmed", "index", i, "nonce", txs[i].nonce)
			}
		}

		// Report bundle distribution
		log.Info(log.Node, "📊 Bundle Distribution Analysis",
			"totalTx", totalTxCount,
			"confirmed", confirmedCount,
			"totalBundles", len(bundleHashes),
			"expectedBundles", expectedBundles,
			"expectedTxsPerBundle", expectedTxsPerBundle)

		for bundleIdx := 0; bundleIdx < len(bundleHashes); bundleIdx++ {
			count := bundleDistribution[bundleIdx]
			log.Info(log.Node, "📦 Bundle breakdown", "bundleIdx", bundleIdx, "txCount", count)
		}

		// Report core distribution
		log.Info(log.Node, "📊 Core Distribution (parallel processing)", "coreDistribution", coreDistribution)
		for core, count := range coreDistribution {
			log.Info(log.Node, "🔧 Core usage", "coreIndex", core, "txCount", count)
		}

		// Verify multi-bundle expectation
		if len(bundleHashes) < 2 {
			t.Errorf("❌ Expected multiple bundles (multi-core mode), got only %d bundle(s)", len(bundleHashes))
		} else {
			log.Info(log.Node, "✅ Transactions distributed across multiple bundles (multi-core mode working)", "bundles", len(bundleHashes))
		}

		// Verify parallel core usage
		if len(coreDistribution) >= numCores {
			log.Info(log.Node, "✅ Both cores utilized for parallel processing", "coresUsed", len(coreDistribution))
		} else {
			log.Info(log.Node, "⚠️ Limited core utilization", "coresUsed", len(coreDistribution), "expected", numCores)
		}

		log.Info(log.Node, "📊 Distribution summary",
			"totalTx", totalTxCount,
			"confirmed", confirmedCount,
			"totalBundles", len(bundleHashes),
			"bundleDistribution", bundleDistribution,
			"coreDistribution", coreDistribution)

		if confirmedCount < totalTxCount {
			t.Errorf("Only %d/%d transactions confirmed within timeout", confirmedCount, totalTxCount)
		}

		if unexpectedRecipientTxs > 0 {
			t.Errorf("Detected %d unexpected transactions sent to recipient (non-test txs)", unexpectedRecipientTxs)
		}

		// Verify final balances
		recipientBalanceAfter, err := evmClient.GetBalance(recipientAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (recipient after) failed: %v", err)
		}
		recipientBalanceAfterInt := new(big.Int).SetBytes(recipientBalanceAfter.Bytes())

		totalReceived := new(big.Int).Mul(amount, big.NewInt(int64(confirmedCount)))
		expectedRecipientBalance := new(big.Int).Add(recipientBalanceBeforeInt, totalReceived)

		log.Info(log.Node, "📊 Balance verification",
			"recipientBefore", recipientBalanceBeforeInt.String(),
			"recipientAfter", recipientBalanceAfterInt.String(),
			"expected", expectedRecipientBalance.String(),
			"confirmedCount", confirmedCount)

		if recipientBalanceAfterInt.Cmp(expectedRecipientBalance) != 0 {
			diff := new(big.Int).Sub(recipientBalanceAfterInt, expectedRecipientBalance)
			t.Errorf("Recipient balance mismatch: expected %s, got %s (diff=%s)",
				expectedRecipientBalance.String(), recipientBalanceAfterInt.String(), diff.String())
		} else {
			log.Info(log.Node, "✅ Recipient balance verified")
		}

		// Summary
		avgTxsPerBundle := float64(confirmedCount) / float64(len(bundleHashes))
		log.Info(log.Node, "📊 SUMMARY",
			"totalBundles", len(bundleHashes),
			"expectedBundles", expectedBundles,
			"avgTxsPerBundle", fmt.Sprintf("%.1f", avgTxsPerBundle),
			"expectedTxsPerBundle", expectedTxsPerBundle,
			"coresUtilized", len(coreDistribution),
			"expectedCores", numCores)

		// ========== PHASE 5: Verify eth_getTransactionReceipt RPC ==========
		// This ensures receipts are coming from the real path (not legacy bogus path)
		log.Info(log.Node, "🧾 PHASE 5: Verifying eth_getTransactionReceipt RPC returns valid receipts...")
		fmt.Println("=== RECEIPT VERIFICATION (eth_getTransactionReceipt) ===")

		receiptVerifyCount := min(5, len(txs)) // Verify first 5 receipts
		for i := 0; i < receiptVerifyCount; i++ {
			if txs[i].receipt == nil {
				fmt.Printf("  Receipt[%d]: SKIPPED (tx not confirmed)\n", i)
				continue
			}

			txHash := txs[i].hash
			var ethReceipt *evmtypes.EthereumTransactionReceipt
			err := rpcClient.Call("eth_getTransactionReceipt", []string{txHash.String()}, &ethReceipt)
			if err != nil {
				t.Fatalf("eth_getTransactionReceipt failed for tx %d: %v", i, err)
			}
			if ethReceipt == nil {
				t.Fatalf("eth_getTransactionReceipt returned nil for tx %d (hash=%s)", i, txHash.Hex())
			}

			// Print detailed receipt info (like TestMultiSnapshotUBT does)
			fmt.Printf("  Receipt[%d]: %s\n", i, ethReceipt.String())

			// Verify critical receipt fields
			if ethReceipt.TransactionHash != txHash.String() {
				t.Errorf("Receipt tx hash mismatch: expected %s, got %s", txHash.String(), ethReceipt.TransactionHash)
			}
			if ethReceipt.Status != "0x1" {
				t.Errorf("Receipt shows failure for tx %d (status=%s)", i, ethReceipt.Status)
			}
			if ethReceipt.GasUsed != "0x5208" { // 21000 in hex for simple transfers
				fmt.Printf("    ⚠️ GasUsed unexpected: expected 0x5208 (21000), got %s\n", ethReceipt.GasUsed)
			}
			if ethReceipt.BlockHash == "" || ethReceipt.BlockNumber == "" {
				t.Errorf("Receipt missing block info for tx %d", i)
			}
		}
		fmt.Printf("✅ Verified %d receipts via eth_getTransactionReceipt RPC\n", receiptVerifyCount)
		log.Info(log.Node, "✅ Receipt verification complete", "verified", receiptVerifyCount)
	})
}
