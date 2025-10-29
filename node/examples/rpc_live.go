// +build ignore
// Live RPC testing against running node
// Run with: go run examples/rpc_live.go
// Add any argument to run once: go run examples/rpc_live.go once
//
// Prerequisites:
//   - Node running with: cd node && make evm_node
//   - Wait for genesis to complete before testing balance/nonce

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"net/rpc"
	"os"
	"time"

	"golang.org/x/crypto/sha3"
)

const (
	DefaultTCPPort = 11100
	RefreshSeconds = 10 // Refresh every 10 seconds (comprehensive tests take longer)
	IssuerAddress  = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266" // Alice from Hardhat
	USDMAddress    = "0x0000000000000000000000000000000000000001"
)

type TestResult struct {
	Name    string
	Success bool
	Error   error
	Output  string
}

func clearScreen() {
	fmt.Print("\033[2J\033[3J\033[H")
}

func main() {
	// Check if user wants to run once
	runOnce := len(os.Args) > 1

	fmt.Println("JAM EVM RPC Live Test")
	fmt.Println("=====================\n")

	// Connect to node
	validatorIndex := 0
	port := DefaultTCPPort + validatorIndex
	address := fmt.Sprintf("127.0.0.1:%d", port)

	fmt.Printf("Connecting to %s...\n", address)
	client, err := rpc.Dial("tcp", address)
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		fmt.Println("\nMake sure node is running with: make evm_node")
		os.Exit(1)
	}
	defer client.Close()
	fmt.Println(" Connected to RPC server\n")

	if !runOnce {
		fmt.Printf("Running continuously (refreshing every %d seconds)\n", RefreshSeconds)
		fmt.Println("Press Ctrl+C to stop")
		time.Sleep(2 * time.Second)

		// Loop forever with countdown
		countdown := RefreshSeconds
		for {
			clearScreen()
			runAllTests(client, countdown)

			// Countdown loop
			time.Sleep(1 * time.Second)
			countdown--
			if countdown <= 0 {
				countdown = RefreshSeconds
			}
		}
	} else {
		runAllTests(client, 0)
	}
}

func runAllTests(client *rpc.Client, countdown int) {
	results := []TestResult{}

	// Test 1: TxPoolStatus
	results = append(results, testTxPoolStatus(client))

	// Test 2: GetBalance (issuer)
	results = append(results, testGetBalance(client, IssuerAddress, "Issuer (Alice)"))

	// Test 3: GetBalance (zero address)
	results = append(results, testGetBalance(client, "0x0000000000000000000000000000000000000000", "Zero Address"))

	// Test 4: GetTransactionCount
	results = append(results, testGetTransactionCount(client, IssuerAddress))

	// Test 5: GetStorageAt (USDM contract)
	results = append(results, testGetStorageAt(client))

	// Test 6: TxPoolContent
	results = append(results, testTxPoolContent(client))

	// Test 7: TxPoolInspect
	results = append(results, testTxPoolInspect(client))

	// Test 8: GetTransactionReceipt
	results = append(results, testGetTransactionReceipt(client))

	// Test 9: GetTransactionByHash
	results = append(results, testGetTransactionByHash(client))

	// Test 10: GetBlockByNumber
	results = append(results, testGetBlockByNumber(client))

	// Test 11: GetBlockByHash
	results = append(results, testGetBlockByHash(client))

	// Print summary
	printSummary(results, countdown)
}

func testTxPoolStatus(client *rpc.Client) TestResult {
	fmt.Println("Test: TxPoolStatus")
	fmt.Println("-" + repeat("-", 50))

	var result string
	err := client.Call("jam.TxPoolStatus", []string{}, &result)

	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: "TxPoolStatus", Success: false, Error: err}
	}

	var stats map[string]interface{}
	json.Unmarshal([]byte(result), &stats)
	prettyJSON, _ := json.MarshalIndent(stats, "  ", "  ")

	fmt.Printf("Result:\n  %s\n", prettyJSON)

	// Check if all values are zero (txpool not initialized)
	pending, _ := stats["pendingCount"].(float64)
	queued, _ := stats["queuedCount"].(float64)
	received, _ := stats["totalReceived"].(float64)
	processed, _ := stats["totalProcessed"].(float64)

	if pending == 0 && queued == 0 && received == 0 && processed == 0 {
		fmt.Printf("❌ TxPool appears to be uninitialized (all zero)\n\n")
		return TestResult{Name: "TxPoolStatus", Success: false, Error: fmt.Errorf("txpool not initialized")}
	}

	fmt.Println()
	return TestResult{Name: "TxPoolStatus", Success: true, Output: result}
}

func testGetBalance(client *rpc.Client, address string, label string) TestResult {
	fmt.Printf("Test: GetBalance (%s)\n", label)
	fmt.Println("-" + repeat("-", 50))

	var balance string
	err := client.Call("jam.GetBalance", []string{address, "latest"}, &balance)

	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: fmt.Sprintf("GetBalance(%s)", label), Success: false, Error: err}
	}

	fmt.Printf(" Address: %s\n", address)
	fmt.Printf(" Balance: %s\n", balance)

	// Check if genesis has completed
	if address == IssuerAddress {
		// Expected: 61M * 10^18 = 0x314dc6448d9338c15b0a00000000
		expected := new(big.Int)
		expected.SetString("314dc6448d9338c15b0a00000000", 16)

		actual := new(big.Int)
		actual.SetString(balance[2:], 16) // Remove 0x prefix

		if actual.Cmp(big.NewInt(0)) == 0 {
			fmt.Printf("⚠️  Balance is zero - genesis may not have completed yet\n")
			fmt.Printf("   Wait for 'evm(0)' to appear in test output\n")
		} else if actual.Cmp(expected) == 0 {
			fmt.Printf(" Correct genesis balance (61M tokens)\n")
		} else {
			fmt.Printf("ℹ️  Balance: %s (in decimal)\n", actual.String())
		}
	}
	fmt.Println()

	return TestResult{Name: fmt.Sprintf("GetBalance(%s)", label), Success: true, Output: balance}
}

func testGetTransactionCount(client *rpc.Client, address string) TestResult {
	fmt.Println("Test: GetTransactionCount")
	fmt.Println("-" + repeat("-", 50))

	var nonce string
	err := client.Call("jam.GetTransactionCount", []string{address, "latest"}, &nonce)

	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: "GetTransactionCount", Success: false, Error: err}
	}

	fmt.Printf(" Address: %s\n", address)
	fmt.Printf(" Nonce:   %s\n", nonce)

	// Genesis sets nonce to 1
	if nonce == "0x1" || nonce == "0x0000000000000000000000000000000000000000000000000000000000000001" {
		fmt.Printf(" Correct genesis nonce\n")
	}
	fmt.Println()

	return TestResult{Name: "GetTransactionCount", Success: true, Output: nonce}
}

func testGetStorageAt(client *rpc.Client) TestResult {
	fmt.Println("Test: GetStorageAt (USDM Contract)")
	fmt.Println("-" + repeat("-", 50))

	// Compute storage key for balanceOf[issuer]
	// balanceOf mapping is at slot 0: storage_key = keccak256(abi.encode(address, slot))
	storageKey := computeMappingStorageKey(IssuerAddress, 0)

	var value string
	err := client.Call("jam.GetStorageAt", []string{USDMAddress, storageKey, "latest"}, &value)

	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: "GetStorageAt", Success: false, Error: err}
	}

	fmt.Printf(" Contract:     %s (USDM)\n", USDMAddress)
	fmt.Printf(" Slot:         balanceOf[issuer]\n")
	fmt.Printf(" Storage Key:  %s\n", storageKey)
	fmt.Printf(" Value (raw):  %s\n", value)

	// Decode the balance
	if len(value) > 2 {
		balanceInt := new(big.Int)
		balanceInt.SetString(value[2:], 16)
		fmt.Printf(" Balance:      %s Wei\n", balanceInt.String())
	}
	fmt.Println()

	return TestResult{Name: "GetStorageAt", Success: true, Output: value}
}

func testTxPoolContent(client *rpc.Client) TestResult {
	fmt.Println("Test: TxPoolContent")
	fmt.Println("-" + repeat("-", 50))

	var result string
	err := client.Call("jam.TxPoolContent", []string{}, &result)

	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: "TxPoolContent", Success: false, Error: err}
	}

	var content map[string]interface{}
	json.Unmarshal([]byte(result), &content)

	pending := content["pending"].([]interface{})
	queued := content["queued"].([]interface{})

	fmt.Printf(" Pending: %d transactions\n", len(pending))
	fmt.Printf(" Queued:  %d transactions\n", len(queued))

	if len(pending) == 0 && len(queued) == 0 {
		fmt.Printf("❌ TxPool is empty (not initialized)\n\n")
		return TestResult{Name: "TxPoolContent", Success: false, Error: fmt.Errorf("txpool empty")}
	}

	fmt.Println()
	return TestResult{Name: "TxPoolContent", Success: true, Output: result}
}

func testTxPoolInspect(client *rpc.Client) TestResult {
	fmt.Println("Test: TxPoolInspect")
	fmt.Println("-" + repeat("-", 50))

	var result string
	err := client.Call("jam.TxPoolInspect", []string{}, &result)

	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: "TxPoolInspect", Success: false, Error: err}
	}

	fmt.Printf(" Pool Summary:\n%s\n", result)

	// Check if result is just empty arrays
	if result == `{"pending":[],"queued":[]}` || result == "" {
		fmt.Printf("❌ TxPool is empty (not initialized)\n\n")
		return TestResult{Name: "TxPoolInspect", Success: false, Error: fmt.Errorf("txpool empty")}
	}

	fmt.Println()
	return TestResult{Name: "TxPoolInspect", Success: true, Output: result}
}

func testGetTransactionReceipt(client *rpc.Client) TestResult {
	fmt.Println("Test: GetTransactionReceipt")
	fmt.Println("-" + repeat("-", 50))

	// First, get the latest block to find real transaction hashes
	var blockJSON string
	err := client.Call("jam.GetBlockByNumber", []string{"latest", "false"}, &blockJSON)
	if err != nil {
		fmt.Printf("❌ Failed to get latest block: %v\n\n", err)
		return TestResult{Name: "GetTransactionReceipt", Success: false, Error: err}
	}

	if blockJSON == "null" {
		fmt.Printf("ℹ️  No blocks yet, skipping receipt test\n\n")
		return TestResult{Name: "GetTransactionReceipt", Success: true, Output: "null"}
	}

	// Parse block to extract transaction hashes
	var block map[string]interface{}
	if err := json.Unmarshal([]byte(blockJSON), &block); err != nil {
		fmt.Printf("❌ Failed to parse block: %v\n\n", err)
		return TestResult{Name: "GetTransactionReceipt", Success: false, Error: err}
	}

	transactions, ok := block["transactions"].([]interface{})
	if !ok || len(transactions) == 0 {
		fmt.Printf("ℹ️  No transactions in latest block\n\n")
		return TestResult{Name: "GetTransactionReceipt", Success: true, Output: "null"}
	}

	// Get receipt for a random transaction in the block
	randomIndex := rand.Intn(len(transactions))
	txHash, ok := transactions[randomIndex].(string)
	if !ok {
		fmt.Printf("❌ Invalid transaction hash format\n\n")
		return TestResult{Name: "GetTransactionReceipt", Success: false, Error: fmt.Errorf("invalid tx hash")}
	}

	var result string
	err = client.Call("jam.GetTransactionReceipt", []string{txHash}, &result)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: "GetTransactionReceipt", Success: false, Error: err}
	}

	fmt.Printf(" Receipt for tx %s:\n", txHash)
	if result == "null" {
		fmt.Printf("ℹ️  Receipt not found (transaction may not be finalized yet)\n")
	} else {
		var receipt map[string]interface{}
		json.Unmarshal([]byte(result), &receipt)
		prettyJSON, _ := json.MarshalIndent(receipt, "  ", "  ")
		fmt.Printf("  %s\n", prettyJSON)
	}
	fmt.Println()

	return TestResult{Name: "GetTransactionReceipt", Success: true, Output: result}
}

func testGetTransactionByHash(client *rpc.Client) TestResult {
	fmt.Println("Test: GetTransactionByHash")
	fmt.Println("-" + repeat("-", 50))

	// First, get the latest block to find real transaction hashes
	var blockJSON string
	err := client.Call("jam.GetBlockByNumber", []string{"latest", "false"}, &blockJSON)
	if err != nil {
		fmt.Printf("❌ Failed to get latest block: %v\n\n", err)
		return TestResult{Name: "GetTransactionByHash", Success: false, Error: err}
	}

	if blockJSON == "null" {
		fmt.Printf("ℹ️  No blocks yet, skipping transaction test\n\n")
		return TestResult{Name: "GetTransactionByHash", Success: true, Output: "null"}
	}

	// Parse block to extract transaction hashes
	var block map[string]interface{}
	if err := json.Unmarshal([]byte(blockJSON), &block); err != nil {
		fmt.Printf("❌ Failed to parse block: %v\n\n", err)
		return TestResult{Name: "GetTransactionByHash", Success: false, Error: err}
	}

	transactions, ok := block["transactions"].([]interface{})
	if !ok || len(transactions) == 0 {
		fmt.Printf("ℹ️  No transactions in latest block\n\n")
		return TestResult{Name: "GetTransactionByHash", Success: true, Output: "null"}
	}

	// Get transaction for a random hash in the block
	randomIndex := rand.Intn(len(transactions))
	txHash, ok := transactions[randomIndex].(string)
	if !ok {
		fmt.Printf("❌ Invalid transaction hash format\n\n")
		return TestResult{Name: "GetTransactionByHash", Success: false, Error: fmt.Errorf("invalid tx hash")}
	}

	var result string
	err = client.Call("jam.GetTransactionByHash", []string{txHash}, &result)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: "GetTransactionByHash", Success: false, Error: err}
	}

	fmt.Printf(" Transaction %s:\n", txHash)
	if result == "null" {
		fmt.Printf("ℹ️  Transaction not found\n")
	} else {
		var tx map[string]interface{}
		json.Unmarshal([]byte(result), &tx)
		prettyJSON, _ := json.MarshalIndent(tx, "  ", "  ")
		fmt.Printf("  %s\n", prettyJSON)
	}
	fmt.Println()

	return TestResult{Name: "GetTransactionByHash", Success: true, Output: result}
}

func testGetBlockByNumber(client *rpc.Client) TestResult {
	fmt.Println("Test: GetBlockByNumber")
	fmt.Println("-" + repeat("-", 50))

	var result string
	err := client.Call("jam.GetBlockByNumber", []string{"latest", "false"}, &result)

	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: "GetBlockByNumber", Success: false, Error: err}
	}

	if result == "null" {
		fmt.Printf("ℹ️  No blocks yet\n\n")
	} else {
		var block map[string]interface{}
		json.Unmarshal([]byte(result), &block)
		prettyJSON, _ := json.MarshalIndent(block, "  ", "  ")
		fmt.Printf(" Block:\n  %s\n\n", prettyJSON)
	}

	return TestResult{Name: "GetBlockByNumber", Success: true, Output: result}
}

func testGetBlockByHash(client *rpc.Client) TestResult {
	fmt.Println("Test: GetBlockByHash")
	fmt.Println("-" + repeat("-", 50))

	// First get latest block to extract its hash
	var latestBlockJSON string
	err := client.Call("jam.GetBlockByNumber", []string{"latest", "false"}, &latestBlockJSON)
	if err != nil || latestBlockJSON == "null" {
		fmt.Printf("ℹ️  No blocks available yet\n\n")
		return TestResult{Name: "GetBlockByHash", Success: true, Output: "null"}
	}

	var latestBlock map[string]interface{}
	if err := json.Unmarshal([]byte(latestBlockJSON), &latestBlock); err != nil {
		fmt.Printf("❌ Failed to parse latest block: %v\n\n", err)
		return TestResult{Name: "GetBlockByHash", Success: false, Error: err}
	}

	blockHash, ok := latestBlock["hash"].(string)
	if !ok {
		fmt.Printf("❌ Block hash not found in latest block\n\n")
		return TestResult{Name: "GetBlockByHash", Success: false, Error: fmt.Errorf("missing block hash")}
	}

	fmt.Printf(" Fetching block %s:\n", blockHash)

	var result string
	err = client.Call("jam.GetBlockByHash", []string{blockHash, "false"}, &result)
	if err != nil {
		fmt.Printf("❌ Failed: %v\n\n", err)
		return TestResult{Name: "GetBlockByHash", Success: false, Error: err}
	}

	if result == "null" {
		fmt.Printf("❌ Block not found\n\n")
		return TestResult{Name: "GetBlockByHash", Success: false, Error: fmt.Errorf("block not found")}
	}

	var block map[string]interface{}
	json.Unmarshal([]byte(result), &block)
	prettyJSON, _ := json.MarshalIndent(block, "  ", "  ")
	fmt.Printf("  %s\n\n", prettyJSON)

	return TestResult{Name: "GetBlockByHash", Success: true, Output: result}
}

func computeMappingStorageKey(addressHex string, slotNumber uint64) string {
	// Remove 0x prefix if present
	if len(addressHex) >= 2 && addressHex[:2] == "0x" {
		addressHex = addressHex[2:]
	}

	// Left-pad address to 32 bytes (64 hex chars)
	addressBytes := make([]byte, 32)
	addrData, _ := hex.DecodeString(addressHex)
	copy(addressBytes[32-len(addrData):], addrData)

	// Encode slot number as 32 bytes
	slotBytes := make([]byte, 32)
	slotBig := new(big.Int).SetUint64(slotNumber)
	slotBig.FillBytes(slotBytes)

	// Concatenate: leftPad32(address) + leftPad32(slot)
	data := append(addressBytes, slotBytes...)

	// Compute keccak256
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	result := hash.Sum(nil)

	return "0x" + hex.EncodeToString(result)
}

func printSummary(results []TestResult, countdown int) {
	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("Test Summary")
	fmt.Println(repeat("=", 60))

	passed := 0
	failed := 0

	for _, r := range results {
		if r.Success {
			fmt.Printf(" %s\n", r.Name)
			passed++
		} else {
			fmt.Printf("❌ %s: %v\n", r.Name, r.Error)
			failed++
		}
	}

	fmt.Printf("\nTotal: %d tests, %d passed, %d failed\n", len(results), passed, failed)

	// Show countdown if in continuous mode
	if countdown > 0 {
		fmt.Printf("\nRefreshing in %d seconds... (Press Ctrl+C to stop)\n", countdown)
	} else {
		if failed == 0 {
			fmt.Println("\n✅ All tests passed!")
			fmt.Println("\nNext steps:")
			fmt.Println("  1. Monitor test output for transaction processing")
			fmt.Println("  2. Re-run this test after transactions complete")
			fmt.Println("  3. Test GetTransactionReceipt with real tx hashes")
		} else {
			fmt.Println("\n⚠️  Some tests failed")
		}
	}
}

func repeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
