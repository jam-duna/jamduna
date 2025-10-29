//go:build ignore
// +build ignore

// Simple RPC test client - demonstrates basic RPC usage
// Run with: go run examples/rpc_simple.go
// Add any argument to run once: go run examples/rpc_simple.go once

package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/rpc"
	"os"
	"time"

	"github.com/colorfulnotion/jam/node"
)

const (
	DefaultTCPPort = 11100
	RefreshSeconds = 5 // Refresh every 5 seconds
)

func clearScreen() {
	fmt.Print("\033[2J\033[3J\033[H")
}

func main() {
	// Check if user wants to run once
	runOnce := len(os.Args) > 1

	// Connect to validator 0's RPC server
	validatorIndex := 0
	port := DefaultTCPPort + validatorIndex
	address := fmt.Sprintf("127.0.0.1:%d", port)

	fmt.Printf("Connecting to RPC server at %s...\n", address)

	client, err := rpc.Dial("tcp", address)
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		fmt.Printf("\nMake sure the node is running with: make evm_node\n")
		os.Exit(1)
	}
	defer client.Close()

	fmt.Printf(" Connected to RPC server!\n\n")

	if !runOnce {
		fmt.Printf("Running continuously (refreshing every %d seconds)\n", RefreshSeconds)
		fmt.Println("Press Ctrl+C to stop")
		time.Sleep(2 * time.Second)

		// Loop forever with countdown
		countdown := RefreshSeconds
		for {
			clearScreen()
			runTests(client, countdown)

			// Countdown loop
			time.Sleep(1 * time.Second)
			countdown--
			if countdown <= 0 {
				countdown = RefreshSeconds
			}
		}
	} else {
		runTests(client, 0)
	}
}

func runTests(client *rpc.Client, countdown int) {
	var err error

	// Test 1: TxPoolStatus (doesn't require seeded state)
	fmt.Println(repeat("=", 60))
	fmt.Println("Test 1: TxPoolStatus")
	fmt.Println(repeat("=", 60))
	var poolStatus string
	err = client.Call("jam.TxPoolStatus", []string{}, &poolStatus)
	if err != nil {
		fmt.Printf("❌ TxPoolStatus failed: %v\n", err)
	} else {
		fmt.Printf(" TxPoolStatus succeeded:\n")
		var stats map[string]interface{}
		if json.Unmarshal([]byte(poolStatus), &stats) == nil {
			prettyJSON, _ := json.MarshalIndent(stats, "  ", "  ")
			fmt.Printf("  %s\n", prettyJSON)
		} else {
			fmt.Printf("  %s\n", poolStatus)
		}
	}

	// Test 2: GetBalance (requires seeded state from genesis)
	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("Test 2: GetBalance (Alice - Issuer)")
	fmt.Println(repeat("=", 60))
	issuerAddr := "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	var balance string
	err = client.Call("jam.GetBalance", []string{issuerAddr, "latest"}, &balance)
	if err != nil {
		fmt.Printf("❌ GetBalance failed: %v\n", err)
	} else {
		fmt.Printf(" GetBalance succeeded:\n")
		fmt.Printf("  Address: %s\n", issuerAddr)
		fmt.Printf("  Balance: %s\n", balance)
		if balance == "0x0" || balance == "0x0000000000000000000000000000000000000000000000000000000000000000" {
			fmt.Printf("  ⚠️  Balance is zero - state may not be seeded yet\n")
			fmt.Printf("  💡 Wait for the test to process genesis transactions\n")
		}
	}

	// Test 3: GetTransactionCount
	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("Test 3: GetTransactionCount (Alice)")
	fmt.Println(repeat("=", 60))
	var nonce string
	err = client.Call("jam.GetTransactionCount", []string{issuerAddr, "latest"}, &nonce)
	if err != nil {
		fmt.Printf("❌ GetTransactionCount failed: %v\n", err)
	} else {
		fmt.Printf(" GetTransactionCount succeeded:\n")
		fmt.Printf("  Address: %s\n", issuerAddr)
		fmt.Printf("  Nonce:   %s\n", nonce)
	}

	// Test 4: GetStorageAt (USDM contract - balanceOf[issuer])
	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("Test 4: GetStorageAt (USDM contract)")
	fmt.Println(repeat("=", 60))
	usdmAddr := "0x0000000000000000000000000000000000000001"

	// Compute storage key for balanceOf[issuer]
	// balanceOf mapping is at slot 0: storage_key = keccak256(abi.encode(address, slot))
	balanceSlot := node.ComputeMappingStorageKey(issuerAddr, 0)

	var storageValue string
	err = client.Call("jam.GetStorageAt", []string{usdmAddr, balanceSlot, "latest"}, &storageValue)
	if err != nil {
		fmt.Printf("❌ GetStorageAt failed: %v\n", err)
	} else {
		fmt.Printf(" GetStorageAt succeeded:\n")
		fmt.Printf("  Contract:     %s (USDM)\n", usdmAddr)
		fmt.Printf("  Slot:         balanceOf[issuer]\n")
		fmt.Printf("  Storage Key:  %s\n", balanceSlot)
		fmt.Printf("  Value (raw):  %s\n", storageValue)

		// Decode the balance
		if len(storageValue) > 2 {
			balanceInt := new(big.Int)
			balanceInt.SetString(storageValue[2:], 16)
			fmt.Printf("  Balance:      %s Wei\n", balanceInt.String())
		}
	}

	// Test 5: TxPoolContent
	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("Test 5: TxPoolContent")
	fmt.Println(repeat("=", 60))
	var poolContent string
	err = client.Call("jam.TxPoolContent", []string{}, &poolContent)
	if err != nil {
		fmt.Printf("❌ TxPoolContent failed: %v\n", err)
	} else {
		fmt.Printf(" TxPoolContent succeeded:\n")
		var content map[string]interface{}
		if json.Unmarshal([]byte(poolContent), &content) == nil {
			pending := content["pending"].([]interface{})
			queued := content["queued"].([]interface{})
			fmt.Printf("  Pending: %d transactions\n", len(pending))
			fmt.Printf("  Queued:  %d transactions\n", len(queued))
		}
	}

	// Test 6: Get transaction receipt from latest block
	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("Test 6: GetTransactionReceipt (from latest block)")
	fmt.Println(repeat("=", 60))

	// Get latest block to find real transaction hashes
	var blockJSON string
	err = client.Call("jam.GetBlockByNumber", []string{"latest", "false"}, &blockJSON)
	if err != nil || blockJSON == "null" {
		fmt.Printf("ℹ️  No blocks yet, skipping receipt test\n")
	} else {
		var block map[string]interface{}
		if json.Unmarshal([]byte(blockJSON), &block) == nil {
			if txs, ok := block["transactions"].([]interface{}); ok && len(txs) > 0 {
				if txHash, ok := txs[0].(string); ok {
					var receipt string
					err = client.Call("jam.GetTransactionReceipt", []string{txHash}, &receipt)
					if err != nil {
						fmt.Printf("❌ GetTransactionReceipt failed: %v\n", err)
					} else {
						fmt.Printf(" GetTransactionReceipt succeeded:\n")
						fmt.Printf("  TxHash: %s\n", txHash[:16]+"...")
						if receipt == "null" {
							fmt.Printf("  Result: Transaction receipt not found\n")
						} else {
							fmt.Printf("  Result: Receipt found (%d bytes)\n", len(receipt))
						}
					}
				}
			} else {
				fmt.Printf("ℹ️  No transactions in latest block\n")
			}
		}
	}

	// Summary
	fmt.Println("\n" + repeat("=", 60))
	fmt.Println("Summary")
	fmt.Println(repeat("=", 60))
	fmt.Println(" RPC server is running and accepting connections")
	fmt.Println(" Basic RPC methods are functional")
	fmt.Println("")
	fmt.Println("Next steps:")
	fmt.Println("  1. Wait for genesis to complete (watch the test output)")
	fmt.Println("  2. Run this test again to see seeded balances")
	fmt.Println("  3. After transactions are processed, test GetTransactionReceipt with real hashes")
	fmt.Println("")

	// Show countdown if in continuous mode
	if countdown > 0 {
		fmt.Printf("Refreshing in %d seconds... (Press Ctrl+C to stop)\n", countdown)
	} else {
		fmt.Println("To monitor the test progress:")
		fmt.Println("  - Watch the terminal where 'make evm_node' is running")
		fmt.Println("  - Look for 'Genesis' or 'evm(0)' messages indicating state initialization")
	}
}

func repeat(s string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += s
	}
	return result
}
