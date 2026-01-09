package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

const devSeed = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art"

type WalletData struct {
	UnifiedAddress string `json:"unified_address"`
}

func deriveUA(serviceID uint32) (string, error) {
	tmp, err := os.CreateTemp("", "jam-wallet-*.json")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	tmp.Close()
	os.Remove(tmpName) // Remove so zcash-wallet can create it fresh
	defer os.Remove(tmpName)

	cmd := exec.Command(
		"./target/release/zcash-wallet",
		"restore",
		"--seed", devSeed,
		"--account", strconv.FormatUint(uint64(serviceID), 10),
		"--address-index", "0",
		"--output", tmpName,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("zcash-wallet command failed: %w\nOutput: %s", err, string(output))
	}

	data, err := os.ReadFile(tmpName)
	if err != nil {
		return "", fmt.Errorf("failed to read wallet file: %w", err)
	}

	var wallet WalletData
	if err := json.Unmarshal(data, &wallet); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	return wallet.UnifiedAddress, nil
}

func main() {
	fmt.Println("Deriving Zcash unified addresses using Go wrapper:")

	for _, serviceID := range []uint32{1, 2} {
		ua, err := deriveUA(serviceID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error deriving address for service_id=%d: %v\n", serviceID, err)
			os.Exit(1)
		}
		fmt.Printf("service_id=%d ua=%s\n", serviceID, ua)
	}
}
