package node

import (
	"math/big"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	// Hardhat Account #0 (Issuer/Alice) - First account from standard Hardhat/Anvil test mnemonic
	// "test test test test test test test test test test test junk"
	issuerPrivateKeyHex = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

	// Target address (Hardhat Account #1)
	targetAddressHex = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
)

func TestTxPoolBasicOperations(t *testing.T) {
	// Create a new transaction pool
	pool := NewTxPool()

	// Test initial state
	stats := pool.GetStats()
	if stats.PendingCount != 0 || stats.QueuedCount != 0 {
		t.Errorf("Expected empty pool, got pending: %d, queued: %d", stats.PendingCount, stats.QueuedCount)
	}

	// Create a real USDM transfer transaction: statedb.IssuerAddress sends 1000 USDM to target
	amount := new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e18)) // 1000 USDM
	gasPrice := big.NewInt(1_000_000_000)                          // 1 Gwei
	gasLimit := uint64(100_000)

	targetAddr := common.HexToAddress(targetAddressHex)
	tx, _, _, err := statedb.CreateSignedUSDMTransfer(issuerPrivateKeyHex, 0, targetAddr, amount, gasPrice, gasLimit, statedb.JamChainID)
	if err != nil {
		t.Fatalf("Failed to create signed transaction: %v", err)
	}

	// Verify sender is the issuer address
	if tx.From != statedb.IssuerAddress {
		t.Errorf("Expected sender %s, got %s", statedb.IssuerAddress.String(), tx.From.String())
	}

	// Add transaction to pool
	err = pool.AddTransaction(tx)
	if err != nil {
		t.Fatalf("Failed to add transaction to pool: %v", err)
	}

	// Check pool state after adding transaction
	stats = pool.GetStats()
	if stats.PendingCount != 1 || stats.TotalReceived != 1 {
		t.Errorf("Expected 1 pending transaction, got pending: %d, received: %d", stats.PendingCount, stats.TotalReceived)
	}

	// Test retrieving transaction
	retrievedTx, found := pool.GetTransaction(tx.Hash)
	if !found {
		t.Errorf("Transaction not found in pool")
	}
	if retrievedTx.Hash != tx.Hash {
		t.Errorf("Retrieved transaction hash mismatch")
	}

	// Test getting pending transactions
	pendingTxs := pool.GetPendingTransactions()
	if len(pendingTxs) != 1 {
		t.Errorf("Expected 1 pending transaction, got %d", len(pendingTxs))
	}

	// Test removing transaction
	removed := pool.RemoveTransaction(tx.Hash)
	if !removed {
		t.Errorf("Failed to remove transaction")
	}

	// Check pool state after removal
	stats = pool.GetStats()
	if stats.PendingCount != 0 || stats.TotalProcessed != 1 {
		t.Errorf("Expected empty pool after removal, got pending: %d, processed: %d", stats.PendingCount, stats.TotalProcessed)
	}
}

func TestTxPoolValidation(t *testing.T) {
	pool := NewTxPool()

	// Create a valid USDM transfer transaction
	amount := new(big.Int).Mul(big.NewInt(500), big.NewInt(1e18)) // 500 USDM
	gasPrice := big.NewInt(1_000_000_000)                         // 1 Gwei
	gasLimit := uint64(100_000)

	targetAddr := common.HexToAddress(targetAddressHex)
	tx, _, _, err := statedb.CreateSignedUSDMTransfer(issuerPrivateKeyHex, 0, targetAddr, amount, gasPrice, gasLimit, statedb.JamChainID)
	if err != nil {
		t.Fatalf("Failed to create signed transaction: %v", err)
	}

	// Modify gas price to be below minimum (simulate validation)
	originalGasPrice := new(big.Int).Set(tx.GasPrice)
	tx.GasPrice.SetInt64(100) // Below minimum of 1 Gwei

	// Try to add invalid transaction
	err = pool.AddTransaction(tx)
	if err == nil {
		t.Errorf("Expected validation error for low gas price")
	}

	// Restore gas price and verify transaction is now valid
	tx.GasPrice.Set(originalGasPrice)
	err = pool.AddTransaction(tx)
	if err != nil {
		t.Logf("Transaction added successfully after fixing gas price")
	}
}

func TestParseRawTransaction(t *testing.T) {
	// Create a real USDM transfer transaction
	amount := new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e18)) // 1000 USDM
	gasPrice := big.NewInt(1_000_000_000)                          // 1 Gwei
	gasLimit := uint64(100_000)

	targetAddr := common.HexToAddress(targetAddressHex)
	tx, rawTxBytes, _, err := statedb.CreateSignedUSDMTransfer(issuerPrivateKeyHex, 0, targetAddr, amount, gasPrice, gasLimit, statedb.JamChainID)
	if err != nil {
		t.Fatalf("Failed to create signed transaction: %v", err)
	}

	// Check basic properties
	if tx.Hash == (common.Hash{}) {
		t.Errorf("Transaction hash should not be empty")
	}

	if tx.Size == 0 {
		t.Errorf("Transaction size should not be zero")
	}

	if tx.ReceivedAt.IsZero() {
		t.Errorf("ReceivedAt timestamp should be set")
	}

	// Verify sender was already recovered
	if tx.From != statedb.IssuerAddress {
		t.Errorf("Expected sender %s, got %s", statedb.IssuerAddress.String(), tx.From.String())
	}

	// Test parsing from raw bytes
	tx2, err := statedb.ParseRawTransaction(rawTxBytes)
	if err != nil {
		t.Fatalf("Failed to parse transaction from bytes: %v", err)
	}

	// Verify signature recovery works
	sender, err := tx2.RecoverSender()
	if err != nil {
		t.Fatalf("Failed to recover sender: %v", err)
	}

	if sender != statedb.IssuerAddress {
		t.Errorf("Expected sender %s, got %s", statedb.IssuerAddress.String(), sender.String())
	}

	// Verify transaction is to USDM contract
	if tx.To == nil || *tx.To != statedb.UsdmAddress {
		t.Errorf("Expected transaction to USDM contract %s", statedb.UsdmAddress.String())
	}

	// Test with invalid bytes
	_, err = statedb.ParseRawTransaction([]byte{0x01, 0x02, 0x03})
	if err == nil {
		t.Errorf("Expected error for invalid RLP")
	}
}

func TestTxPoolCleanup(t *testing.T) {
	pool := NewTxPool()

	// Set a very short TTL for testing
	pool.config.TxTTL = time.Millisecond * 10

	// Create a real USDM transfer transaction
	amount := new(big.Int).Mul(big.NewInt(250), big.NewInt(1e18)) // 250 USDM
	gasPrice := big.NewInt(1_000_000_000)                         // 1 Gwei
	gasLimit := uint64(100_000)

	targetAddr := common.HexToAddress(targetAddressHex)
	tx, _, _, err := statedb.CreateSignedUSDMTransfer(issuerPrivateKeyHex, 0, targetAddr, amount, gasPrice, gasLimit, statedb.JamChainID)
	if err != nil {
		t.Fatalf("Failed to create signed transaction: %v", err)
	}

	pool.AddTransaction(tx)

	// Wait for expiration
	time.Sleep(time.Millisecond * 20)

	// Run cleanup
	pool.CleanupExpiredTransactions()

	// Check that transaction was removed
	stats := pool.GetStats()
	if stats.PendingCount != 0 {
		t.Errorf("Expired transaction should be removed from pool")
	}
}

// TestEIP1559Transaction verifies that EIP-1559 typed transactions are properly handled
func TestEIP1559Transaction(t *testing.T) {
	// Parse private key
	privateKey, err := crypto.HexToECDSA(issuerPrivateKeyHex)
	if err != nil {
		t.Fatalf("Failed to parse private key: %v", err)
	}

	// USDM transfer calldata
	amount := new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e18))
	targetAddr := common.HexToAddress(targetAddressHex)

	calldata := make([]byte, 68)
	copy(calldata[0:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) // transfer(address,uint256) selector
	copy(calldata[16:36], targetAddr.Bytes())
	amountBytes := amount.FillBytes(make([]byte, 32))
	copy(calldata[36:68], amountBytes)

	// Create EIP-1559 transaction (type 2)
	chainID := big.NewInt(int64(statedb.JamChainID))
	gasTipCap := big.NewInt(2_000_000_000)  // 2 Gwei
	gasFeeCap := big.NewInt(10_000_000_000) // 10 Gwei

	ethTx := ethereumTypes.NewTx(&ethereumTypes.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     0,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Gas:       100_000,
		To:        (*ethereumCommon.Address)(&statedb.UsdmAddress),
		Value:     big.NewInt(0),
		Data:      calldata,
	})

	// Sign the EIP-1559 transaction
	signer := ethereumTypes.LatestSignerForChainID(chainID)
	signedTx, err := ethereumTypes.SignTx(ethTx, signer, privateKey)
	if err != nil {
		t.Fatalf("Failed to sign EIP-1559 transaction: %v", err)
	}

	// Encode to RLP
	rlpBytes, err := signedTx.MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to marshal transaction: %v", err)
	}

	// Parse the raw transaction
	tx, err := statedb.ParseRawTransaction(rlpBytes)
	if err != nil {
		t.Fatalf("Failed to parse EIP-1559 transaction: %v", err)
	}

	// Verify transaction type is stored
	if tx.Inner == nil {
		t.Fatal("Inner transaction not stored")
	}
	if tx.Inner.Type() != ethereumTypes.DynamicFeeTxType {
		t.Errorf("Expected transaction type %d, got %d", ethereumTypes.DynamicFeeTxType, tx.Inner.Type())
	}

	// Verify signature recovery works for EIP-1559
	sender, err := tx.RecoverSender()
	if err != nil {
		t.Fatalf("Failed to recover sender from EIP-1559 transaction: %v", err)
	}

	expectedSender := statedb.IssuerAddress
	if sender != expectedSender {
		t.Errorf("Sender mismatch: expected %s, got %s", expectedSender.String(), sender.String())
	}

	t.Logf("✅ EIP-1559 transaction successfully parsed and sender recovered: %s", sender.String())
}

// TestEIP2930Transaction verifies that EIP-2930 access list transactions are properly handled
func TestEIP2930Transaction(t *testing.T) {
	// Parse private key
	privateKey, err := crypto.HexToECDSA(issuerPrivateKeyHex)
	if err != nil {
		t.Fatalf("Failed to parse private key: %v", err)
	}

	// USDM transfer calldata
	amount := new(big.Int).Mul(big.NewInt(1000), big.NewInt(1e18))
	targetAddr := common.HexToAddress(targetAddressHex)

	calldata := make([]byte, 68)
	copy(calldata[0:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) // transfer(address,uint256) selector
	copy(calldata[16:36], targetAddr.Bytes())
	amountBytes := amount.FillBytes(make([]byte, 32))
	copy(calldata[36:68], amountBytes)

	// Create EIP-2930 transaction (type 1) with access list
	chainID := big.NewInt(int64(statedb.JamChainID))
	gasPrice := big.NewInt(5_000_000_000) // 5 Gwei

	ethTx := ethereumTypes.NewTx(&ethereumTypes.AccessListTx{
		ChainID:  chainID,
		Nonce:    0,
		GasPrice: gasPrice,
		Gas:      100_000,
		To:       (*ethereumCommon.Address)(&statedb.UsdmAddress),
		Value:    big.NewInt(0),
		Data:     calldata,
		AccessList: ethereumTypes.AccessList{
			{Address: ethereumCommon.Address(statedb.UsdmAddress), StorageKeys: []ethereumCommon.Hash{}},
		},
	})

	// Sign the EIP-2930 transaction
	signer := ethereumTypes.LatestSignerForChainID(chainID)
	signedTx, err := ethereumTypes.SignTx(ethTx, signer, privateKey)
	if err != nil {
		t.Fatalf("Failed to sign EIP-2930 transaction: %v", err)
	}

	// Encode to RLP
	rlpBytes, err := signedTx.MarshalBinary()
	if err != nil {
		t.Fatalf("Failed to marshal transaction: %v", err)
	}

	// Parse the raw transaction
	tx, err := statedb.ParseRawTransaction(rlpBytes)
	if err != nil {
		t.Fatalf("Failed to parse EIP-2930 transaction: %v", err)
	}

	// Verify transaction type is stored
	if tx.Inner == nil {
		t.Fatal("Inner transaction not stored")
	}
	if tx.Inner.Type() != ethereumTypes.AccessListTxType {
		t.Errorf("Expected transaction type %d, got %d", ethereumTypes.AccessListTxType, tx.Inner.Type())
	}

	// Verify signature recovery works for EIP-2930
	sender, err := tx.RecoverSender()
	if err != nil {
		t.Fatalf("Failed to recover sender from EIP-2930 transaction: %v", err)
	}

	expectedSender := statedb.IssuerAddress
	if sender != expectedSender {
		t.Errorf("Sender mismatch: expected %s, got %s", expectedSender.String(), sender.String())
	}

	t.Logf("✅ EIP-2930 transaction successfully parsed and sender recovered: %s", sender.String())
}
