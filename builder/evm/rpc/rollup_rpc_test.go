package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	statedbevmtypes "github.com/colorfulnotion/jam/statedb/evmtypes"
)

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
	// This initializes LOCAL verkle tree state - no work package submission
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

	// Test 5: Send a transfer transaction
	t.Run("SendTransfer", func(t *testing.T) {
		senderAddr, senderPrivKey := common.GetEVMDevAccount(0)
		recipientAddr, _ := common.GetEVMDevAccount(1)

		// Get current nonce
		nonce, err := evmClient.GetTransactionCount(senderAddr, "latest")
		if err != nil {
			t.Fatalf("GetTransactionCount failed: %v", err)
		}

		// Get sender balance first
		senderBalanceBefore, err := evmClient.GetBalance(senderAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (sender before) failed: %v", err)
		}
		senderBalanceBeforeInt := new(big.Int).SetBytes(senderBalanceBefore.Bytes())

		// Get recipient balance before
		recipientBalanceBefore, err := evmClient.GetBalance(recipientAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (recipient before) failed: %v", err)
		}
		recipientBalanceBeforeInt := new(big.Int).SetBytes(recipientBalanceBefore.Bytes())

		log.Info(log.Node, "Before transfer",
			"sender", senderAddr.String(),
			"senderBalance", senderBalanceBeforeInt.String(),
			"recipient", recipientAddr.String(),
			"recipientBalance", recipientBalanceBeforeInt.String(),
			"nonce", nonce)

		// Create and sign transfer
		amount := new(big.Int).Mul(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)) // 1 ETH
		gasPrice := big.NewInt(1_000_000_000)                                                            // 1 gwei (minimum gas price)
		gasLimit := uint64(21000)

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
			t.Fatalf("CreateSignedNativeTransfer failed: %v", err)
		}

		log.Info(log.Node, "📤 Sending transfer",
			"txHash", txHash.String(),
			"from", senderAddr.String(),
			"to", recipientAddr.String(),
			"amount", amount.String())

		// Send transaction
		returnedHash, err := evmClient.SendRawTransaction(signedTx)
		if err != nil {
			t.Fatalf("SendRawTransaction failed: %v", err)
		}

		log.Info(log.Node, "✅ Transaction sent", "returnedHash", returnedHash.String())

		// Wait for transaction to be included (poll every 6s to match JAM block time)
		log.Info(log.Node, "⏳ Waiting for transaction to be included (polling every 6s)...")
		var receipt *statedbevmtypes.EthereumTransactionReceipt
		for i := 0; i < 20; i++ { // Wait up to 120 seconds (20 * 6s)
			time.Sleep(6 * time.Second)
			receipt, err = evmClient.GetTransactionReceipt(txHash)
			if err == nil && receipt != nil && receipt.BlockHash != "" {
				break
			}
			log.Info(log.Node, "⏳ Receipt not yet available, waiting...", "attempt", i+1)
		}

		if receipt == nil || receipt.BlockHash == "" {
			t.Fatal("Transaction was not included in a block within 120 seconds")
		}

		log.Info(log.Node, "✅ Transaction included",
			"blockNumber", receipt.BlockNumber,
			"status", receipt.Status,
			"gasUsed", receipt.GasUsed)

		// Verify balances changed
		senderBalanceAfter, err := evmClient.GetBalance(senderAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (sender after) failed: %v", err)
		}
		senderBalanceAfterInt := new(big.Int).SetBytes(senderBalanceAfter.Bytes())

		recipientBalanceAfter, err := evmClient.GetBalance(recipientAddr, "latest")
		if err != nil {
			t.Fatalf("GetBalance (recipient after) failed: %v", err)
		}
		recipientBalanceAfterInt := new(big.Int).SetBytes(recipientBalanceAfter.Bytes())

		log.Info(log.Node, "After transfer",
			"senderBalance", senderBalanceAfterInt.String(),
			"recipientBalance", recipientBalanceAfterInt.String())

		// Verify recipient received the amount
		expectedRecipientBalance := new(big.Int).Add(recipientBalanceBeforeInt, amount)
		if recipientBalanceAfterInt.Cmp(expectedRecipientBalance) != 0 {
			t.Errorf("Recipient balance mismatch: expected %s, got %s",
				expectedRecipientBalance.String(), recipientBalanceAfterInt.String())
		} else {
			log.Info(log.Node, "✅ Recipient balance verified")
		}

		// Verify sender balance decreased by amount + gas
		gasUsedVal, err := strconv.ParseUint(receipt.GasUsed[2:], 16, 64)
		if err != nil {
			t.Fatalf("Failed to parse gasUsed: %v", err)
		}
		gasUsed := new(big.Int).SetUint64(gasUsedVal)
		gasCost := new(big.Int).Mul(gasUsed, gasPrice)
		expectedSenderBalance := new(big.Int).Sub(senderBalanceBeforeInt, amount)
		expectedSenderBalance = new(big.Int).Sub(expectedSenderBalance, gasCost)

		if senderBalanceAfterInt.Cmp(expectedSenderBalance) != 0 {
			t.Errorf("Sender balance mismatch: expected %s, got %s",
				expectedSenderBalance.String(), senderBalanceAfterInt.String())
		} else {
			log.Info(log.Node, "✅ Sender balance verified")
		}
	})
}

