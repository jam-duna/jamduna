// +build ignore
// Continuous RPC monitor - refreshes every N seconds
// Run with: go run examples/rpc_monitor_watch.go
// Press Ctrl+C to stop

package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/rpc"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultTCPPort = 11100
	RefreshSeconds = 3  // Refresh every 3 seconds
	maxWidth       = 82 // Content width (excluding borders) - increased to fit full tx hashes
)

func clearScreen() {
	// ANSI escape codes:
	// \033[2J - clear entire screen
	// \033[3J - clear scrollback buffer
	// \033[H  - move cursor to home (0,0)
	fmt.Print("\033[2J\033[3J\033[H")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// padLine ensures content is exactly maxWidth characters
// Pads with spaces or truncates as needed
// Note: This counts runes (Unicode characters), not bytes
func padLine(content string) string {
	runeCount := utf8.RuneCountInString(content)
	if runeCount > maxWidth {
		// Truncate to maxWidth runes
		runes := []rune(content)
		return string(runes[:maxWidth])
	}
	// Pad with spaces to reach maxWidth
	return content + strings.Repeat(" ", maxWidth-runeCount)
}

// repeat returns a string with character repeated n times
func repeat(char string, n int) string {
	return strings.Repeat(char, n)
}

func main() {
	// Connect once
	client, err := rpc.Dial("tcp", "127.0.0.1:11100")
	if err != nil {
		fmt.Printf("❌ Not connected: %v\n", err)
		fmt.Println("Make sure node is running: cd node && make evm_node")
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("✅ Connected to RPC server")
	fmt.Printf("Refreshing every %d seconds (Press Ctrl+C to stop)\n\n", RefreshSeconds)
	time.Sleep(2 * time.Second)

	// Loop forever
	countdown := RefreshSeconds
	for {
		clearScreen()
		displayState(client, countdown)

		// Sleep for 1 second and decrement countdown
		time.Sleep(1 * time.Second)
		countdown--
		if countdown <= 0 {
			countdown = RefreshSeconds
		}
	}
}

func displayState(client *rpc.Client, countdown int) {
	now := time.Now().Format("15:04:05")

	// Top border
	fmt.Printf("╔%s╗\n", repeat("═", maxWidth))
	fmt.Printf("║%s║\n", padLine(fmt.Sprintf("              JAM RPC Live Monitor - %s", now)))
	fmt.Printf("╠%s╣\n", repeat("═", maxWidth))

	// Pool status
	var poolStatus string
	if err := client.Call("jam.TxPoolStatus", []string{}, &poolStatus); err == nil {
		var stats map[string]interface{}
		json.Unmarshal([]byte(poolStatus), &stats)

		fmt.Printf("║%s║\n", padLine(" Transaction Pool"))
		fmt.Printf("║%s║\n", padLine(fmt.Sprintf("   Pending:   %.0f", stats["pendingCount"].(float64))))
		fmt.Printf("║%s║\n", padLine(fmt.Sprintf("   Queued:    %.0f", stats["queuedCount"].(float64))))
		fmt.Printf("║%s║\n", padLine(fmt.Sprintf("   Received:  %.0f", stats["totalReceived"].(float64))))
		fmt.Printf("║%s║\n", padLine(fmt.Sprintf("   Processed: %.0f", stats["totalProcessed"].(float64))))
		fmt.Printf("╠%s╣\n", repeat("═", maxWidth))
	}

	// Latest block
	var blockStr string
	if err := client.Call("jam.GetBlockByNumber", []string{"latest", "false"}, &blockStr); err == nil && blockStr != "null" {
		var block map[string]interface{}
		json.Unmarshal([]byte(blockStr), &block)

		blockNum := block["number"].(string)
		blockNumInt := new(big.Int)
		blockNumInt.SetString(blockNum[2:], 16)

		txs := block["transactions"].([]interface{})

		fmt.Printf("║%s║\n", padLine(" Latest Block"))
		fmt.Printf("║%s║\n", padLine(fmt.Sprintf("   Number:       #%s", blockNumInt.String())))
		fmt.Printf("║%s║\n", padLine(fmt.Sprintf("   Transactions: %d", len(txs))))

		// Show ALL transaction hashes
		if len(txs) > 0 {
			fmt.Printf("║%s║\n", padLine("   Transaction Hashes:"))
			for i, txHashInterface := range txs {
				txHash := txHashInterface.(string)
				// Show full transaction hash (66 chars)
				// Format: "   1. 0x..." for single digit, "  10. 0x..." for double digit
				fmt.Printf("║%s║\n", padLine(fmt.Sprintf("   %d. %s", i+1, txHash)))
			}
		}
		fmt.Printf("╠%s╣\n", repeat("═", maxWidth))
	} else {
		fmt.Printf("║%s║\n", padLine(" Latest Block"))
		fmt.Printf("║%s║\n", padLine("   No blocks yet"))
		fmt.Printf("╠%s╣\n", repeat("═", maxWidth))
	}

	// EVM Dev Accounts - scan all 10 accounts
	accounts := []struct {
		name    string
		address string
	}{
		{"Issuer (Alice)", "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"},
		{"Account #1", "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"},
		{"Account #2", "0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC"},
		{"Account #3", "0x90F79bf6EB2c4f870365E785982E1f101E93b906"},
		{"Account #4", "0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65"},
		{"Account #5", "0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc"},
		{"Account #6", "0x976EA74026E726554dB657fA54763abd0C3a0aa9"},
		{"Account #7", "0x14dC79964da2C08b23698B3D3cc7Ca32193d9955"},
		{"Account #8", "0x23618e81E3f5cdF7f54C3d65f7FBc0aBf5B21E8f"},
		{"Account #9", "0xa0Ee7A142d267C1f36714E4a8F75612F20a79720"},
	}

	fmt.Printf("║%s║\n", padLine(" EVM Dev Accounts"))
	for i, acc := range accounts {
		var balance string
		client.Call("jam.GetBalance", []string{acc.address, "latest"}, &balance)

		balanceInt := new(big.Int)
		if len(balance) > 2 {
			balanceInt.SetString(balance[2:], 16)
		} else {
			balanceInt.SetInt64(0)
		}

		var nonce string
		client.Call("jam.GetTransactionCount", []string{acc.address, "latest"}, &nonce)
		nonceInt := new(big.Int)
		if len(nonce) > 2 {
			nonceInt.SetString(nonce[2:], 16)
		} else {
			nonceInt.SetInt64(0)
		}

		// Show raw Wei balance
		if i == 0 {
			fmt.Printf("║%s║\n", padLine(fmt.Sprintf("   %s", acc.name)))
		} else {
			fmt.Printf("║%s║\n", padLine(fmt.Sprintf("   %s", acc.name)))
		}
		fmt.Printf("║%s║\n", padLine(fmt.Sprintf("     Balance: %s Wei  Nonce: %s", balanceInt.String(), nonceInt.String())))
	}

	fmt.Printf("╚%s╝\n", repeat("═", maxWidth))

	// Fixed-width refresh message (always shows "Refreshing in X seconds...")
	refreshMsg := fmt.Sprintf("\nRefreshing in %d seconds... (Press Ctrl+C to stop)", countdown)
	// Pad to ensure consistent width (prevents screen clearing issues)
	fmt.Printf("%-80s\n", refreshMsg)
}
