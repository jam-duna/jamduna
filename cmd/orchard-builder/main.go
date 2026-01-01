// Orchard Builder - Standalone Orchard RPC server for JAM
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	"time"

	orchardrpc "github.com/colorfulnotion/jam/builder/orchard/rpc"
	"github.com/colorfulnotion/jam/chainspecs"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "orchard-builder",
		Short: "JAM Orchard Builder",
	}
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	var (
		dataPath          string
		rpcPort           int
		chainSpec         string
		validatorIndex    int
		serviceID         int
		pvmBackend        string
		debug             string
		port              int
		telemetryEndpoint string
	)

	var runCmd = &cobra.Command{
		Use:   "run",
		Short: "Start Orchard builder node with network sync",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Starting JAM Orchard Builder Node\n")
			fmt.Printf("  Validator Index: %d\n", validatorIndex)
			fmt.Printf("  Chain Spec: %s\n", chainSpec)
			fmt.Printf("  Data Path: %s\n", dataPath)
			fmt.Printf("  Orchard RPC Port: %d\n", rpcPort)
			fmt.Printf("  Service ID: %d\n", serviceID)
			fmt.Printf("  PVM Backend: %s\n", pvmBackend)

			// Initialize logging
			log.InitLogger("debug")
			log.EnableModules(debug)

			// 1. Read chainspec to get validator list and peer addresses
			fmt.Printf("\n[1/7] Reading chainspec...\n")
			chainSpecData, err := chainspecs.ReadSpec(chainSpec)
			if err != nil {
				fmt.Printf("Failed to read chainspec %s: %v\n", chainSpec, err)
				os.Exit(1)
			}
			fmt.Printf("✓ Chainspec loaded\n")

			// 2. Extract validators from chainspec
			fmt.Printf("\n[2/7] Extracting validators...\n")
			validators, err := getValidatorFromChainSpec(*chainSpecData)
			if err != nil {
				fmt.Printf("Failed to get validators from chainspec: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Found %d validators\n", len(validators))

			// 3. Build peer list with addresses from chainspec
			fmt.Printf("\n[3/7] Building peer list...\n")
			peerList := make(map[uint16]*node.Peer)
			for i, v := range validators {
				peerAddr := fmt.Sprintf("127.0.0.1:%d", 40000+i)
				if ipAddr, p, err := common.MetadataToAddress(v.Metadata[:]); err == nil {
					peerAddr = fmt.Sprintf("%s:%d", ipAddr, p)
				}
				peerList[uint16(i)] = &node.Peer{
					PeerID:    uint16(i),
					PeerAddr:  peerAddr,
					Validator: v,
				}
			}
			fmt.Printf("✓ Peer list built with %d peers\n", len(peerList))

			// 4. Get validator secret from seed file
			fmt.Printf("\n[4/7] Loading validator credentials...\n")
			selfSecret, err := getValidatorSecret(validatorIndex, dataPath)
			if err != nil {
				fmt.Printf("Failed to get validator secret: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Loaded credentials for validator %d\n", validatorIndex)

			// Set port based on validator index if not explicitly set
			if port == 0 {
				port = 40000 + validatorIndex
			}
			fmt.Printf("  SanID: %s@0.0.0.0:%d\n", common.ToSAN(selfSecret.Ed25519Pub[:]), port)

			// 5. Build peers list for network
			peers := make([]string, 0, len(peerList))
			for _, peer := range peerList {
				peers = append(peers, common.ToSAN(peer.Validator.Ed25519[:]))
			}

			// Create node-specific data path
			nodePath := filepath.Join(dataPath, fmt.Sprintf("jam-%d", validatorIndex))

			// 6. Create node with proper credentials
			fmt.Printf("\n[5/7] Creating builder node...\n")
			fmt.Printf("  Port: %d\n", port)
			fmt.Printf("  Data: %s\n", nodePath)

			n, err := node.NewNode(uint16(validatorIndex), selfSecret, chainSpecData, pvmBackend, peers, peerList, nodePath, port, types.RoleBuilder)
			if err != nil {
				fmt.Printf("Failed to create node: %v\n", err)
				os.Exit(1)
			}

			storage, err := n.GetStorage()
			if err != nil {
				fmt.Printf("Failed to get storage: %v\n", err)
				os.Exit(1)
			}
			defer storage.Close()
			fmt.Printf("✓ Builder node created\n")

			// Initialize telemetry if endpoint is specified
			if telemetryEndpoint != "" {
				if err := n.InitTelemetry(telemetryEndpoint); err != nil {
					fmt.Printf("Warning: Failed to initialize telemetry: %v\n", err)
				} else {
					fmt.Printf("✓ Telemetry enabled: %s\n", telemetryEndpoint)
				}
			}

			// 7. Wait for node to sync with network
			fmt.Printf("\n[6/7] Waiting for network sync...\n")
			if err := waitForSync(n, 120*time.Second); err != nil {
				fmt.Printf("Failed to sync: %v\n", err)
				os.Exit(1)
			}

			// 8. Setup Orchard rollup and RPC server
			fmt.Printf("\n[7/7] Setting up Orchard RPC server...\n")
			sid := uint32(serviceID)
			rollup := orchardrpc.NewOrchardRollup(n, sid)

			// Create RPC handler and server
			handler := orchardrpc.NewOrchardRPCHandler(rollup)
			server := orchardrpc.NewOrchardHTTPServer(handler)
			if err := server.Start(rpcPort); err != nil {
				fmt.Printf("Failed to start Orchard RPC server: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ Orchard RPC server started on port %d\n", rpcPort)

			// Start block notification handler for bundle building
			// TODO: Implement transaction pool and auto-submission for Orchard
			// go handleBlockNotifications(n, rollup, sid)

			fmt.Printf("\n========================================\n")
			fmt.Printf("Orchard Builder Ready!\n")
			fmt.Printf("  Orchard RPC: http://localhost:%d\n", rpcPort)
			fmt.Printf("  Service ID: %d\n", serviceID)
			fmt.Printf("  Validator: %d\n", validatorIndex)
			fmt.Printf("========================================\n")

			// Wait for shutdown signal
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			<-sigChan

			fmt.Printf("\nShutting down Orchard builder...\n")
		},
	}

	// Flags
	runCmd.Flags().StringVarP(&dataPath, "data-path", "d", filepath.Join(os.Getenv("HOME"), ".jamduna"), "Data directory (should match validator data path)")
	runCmd.Flags().IntVar(&rpcPort, "rpc-port", 8232, "Orchard RPC server port")
	runCmd.Flags().IntVar(&validatorIndex, "dev-validator", 7, "Validator index (use 7 for orchard builder)")
	runCmd.Flags().IntVar(&serviceID, "service-id", 1, "Orchard service ID")
	runCmd.Flags().StringVar(&chainSpec, "chain", "chainspec.json", "Chain spec file")
	// Default to compiler on Linux (fast), interpreter elsewhere (portable)
	defaultPVMBackend := statedb.BackendInterpreter
	if runtime.GOOS == "linux" {
		defaultPVMBackend = statedb.BackendCompiler
	}
	runCmd.Flags().StringVar(&pvmBackend, "pvm-backend", defaultPVMBackend, "PVM backend (interpreter, compiler, sandbox)")
	runCmd.Flags().StringVar(&debug, "debug", "rotation,guarantees", "Debug modules to enable")
	runCmd.Flags().IntVar(&port, "port", 0, "Network port (default: 40000 + validator index)")
	runCmd.Flags().StringVar(&telemetryEndpoint, "telemetry", "", "Telemetry server endpoint (e.g., localhost:9999)")

	// Submit command - submits pre-generated work packages to JAM network
	var submitCmd = &cobra.Command{
		Use:   "submit [work-package-file]",
		Short: "Submit a work package to the JAM network",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			workPackagePath := args[0]
			fmt.Printf("Submitting work package: %s\n", workPackagePath)
			fmt.Printf("  Service ID: %d\n", serviceID)

			// Initialize logging
			log.InitLogger("debug")
			log.EnableModules(debug)

			// Read chainspec
			chainSpecData, err := chainspecs.ReadSpec(chainSpec)
			if err != nil {
				fmt.Printf("Failed to read chainspec: %v\n", err)
				os.Exit(1)
			}

			// Extract validators and build peer list
			validators, err := getValidatorFromChainSpec(*chainSpecData)
			if err != nil {
				fmt.Printf("Failed to get validators: %v\n", err)
				os.Exit(1)
			}

			peerList := make(map[uint16]*node.Peer)
			for i, v := range validators {
				peerAddr := fmt.Sprintf("127.0.0.1:%d", 40000+i)
				if ipAddr, p, err := common.MetadataToAddress(v.Metadata[:]); err == nil {
					peerAddr = fmt.Sprintf("%s:%d", ipAddr, p)
				}
				peerList[uint16(i)] = &node.Peer{
					PeerID:    uint16(i),
					PeerAddr:  peerAddr,
					Validator: v,
				}
			}

			// Get validator secret
			selfSecret, err := getValidatorSecret(validatorIndex, dataPath)
			if err != nil {
				fmt.Printf("Failed to get validator secret: %v\n", err)
				os.Exit(1)
			}

			if port == 0 {
				port = 40000 + validatorIndex
			}

			// Build peers list
			peers := make([]string, 0, len(peerList))
			for _, peer := range peerList {
				peers = append(peers, common.ToSAN(peer.Validator.Ed25519[:]))
			}

			nodePath := filepath.Join(dataPath, fmt.Sprintf("jam-%d", validatorIndex))

			// Create node
			n, err := node.NewNode(uint16(validatorIndex), selfSecret, chainSpecData, pvmBackend, peers, peerList, nodePath, port, types.RoleBuilder)
			if err != nil {
				fmt.Printf("Failed to create node: %v\n", err)
				os.Exit(1)
			}

			storage, err := n.GetStorage()
			if err != nil {
				fmt.Printf("Failed to get storage: %v\n", err)
				os.Exit(1)
			}
			defer storage.Close()

			fmt.Printf("✓ Node created, waiting for sync...\n")
			if err := waitForSync(n, 60*time.Second); err != nil {
				fmt.Printf("Warning: Sync timeout: %v\n", err)
			}

			// Create Orchard rollup
			sid := uint32(serviceID)
			rollup := orchardrpc.NewOrchardRollup(n, sid)

			// Submit work package with 30 second timeout
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			stateRoot, timeslot, err := rollup.SubmitWorkPackage(ctx, workPackagePath)
			if err != nil {
				fmt.Printf("❌ Work package submission failed: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✅ Work package submitted successfully!\n")
			fmt.Printf("  State Root: %s\n", stateRoot.Hex())
			fmt.Printf("  Timeslot: %d\n", timeslot)
		},
	}

	submitCmd.Flags().StringVarP(&dataPath, "data-path", "d", filepath.Join(os.Getenv("HOME"), ".jamduna"), "Data directory")
	submitCmd.Flags().IntVar(&validatorIndex, "dev-validator", 7, "Validator index")
	submitCmd.Flags().IntVar(&serviceID, "service-id", 1, "Orchard service ID")
	submitCmd.Flags().StringVar(&chainSpec, "chain", "chainspec.json", "Chain spec file")
	submitCmd.Flags().StringVar(&pvmBackend, "pvm-backend", defaultPVMBackend, "PVM backend")
	submitCmd.Flags().StringVar(&debug, "debug", "rotation,guarantees", "Debug modules")
	submitCmd.Flags().IntVar(&port, "port", 0, "Network port")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(submitCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// getValidatorSecret reads the seed file and generates validator credentials
func getValidatorSecret(validatorIndex int, dataPath string) (types.ValidatorSecret, error) {
	keysPath := filepath.Join(dataPath, "keys")
	seedFile := filepath.Join(keysPath, fmt.Sprintf("seed_%d", validatorIndex))

	seed, err := os.ReadFile(seedFile)
	if err != nil {
		return types.ValidatorSecret{}, fmt.Errorf("failed to read seed file %s: %w", seedFile, err)
	}

	if len(seed) < 32 {
		return types.ValidatorSecret{}, fmt.Errorf("seed file too short: %d bytes", len(seed))
	}
	seed = seed[:32]

	secret, err := grandpa.InitValidatorSecret(seed, seed, seed, []byte{})
	if err != nil {
		return types.ValidatorSecret{}, fmt.Errorf("failed to init validator secret: %w", err)
	}

	return secret, nil
}

// getValidatorFromChainSpec extracts validators from chainspec
func getValidatorFromChainSpec(networkFile chainspecs.ChainSpec) ([]types.Validator, error) {
	keyvals := networkFile.GenesisState
	currValidatorRawBytes := []byte{}

	for _, keyval := range keyvals {
		if keyval.Key[0] == 0x08 {
			currValidatorRawBytes = keyval.Value
			break
		}
	}

	if len(currValidatorRawBytes) == 0 {
		return nil, fmt.Errorf("no validators found in chainspec (key 0x08)")
	}

	currValidatorRaw, _, err := types.Decode(currValidatorRawBytes, reflect.TypeOf(types.Validators{}))
	if err != nil {
		return nil, fmt.Errorf("failed to decode validators: %w", err)
	}

	currValidators := currValidatorRaw.(types.Validators)
	return currValidators, nil
}

// waitForSync waits for the node to sync with the network
func waitForSync(n *node.Node, timeout time.Duration) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)
	startTime := time.Now()

	for {
		select {
		case <-timeoutChan:
			return fmt.Errorf("timeout waiting for sync after %v", timeout)
		case <-ticker.C:
			// Check for finalized block first (normal case)
			block, err := n.GetFinalizedBlock()
			if err == nil && block != nil && block.Header.Slot > 0 {
				elapsed := time.Since(startTime)
				fmt.Printf("✓ Synced! Finalized block: slot %d (took %v)\n", block.Header.Slot, elapsed.Round(time.Second))
				return nil
			}

			// Check for latest block at chain head (new network case)
			latestBlock := n.GetLatestBlockInfo()
			if latestBlock != nil && latestBlock.Slot > 0 {
				elapsed := time.Since(startTime)
				fmt.Printf("✓ Synced! At chain head: slot %d (took %v)\n", latestBlock.Slot, elapsed.Round(time.Second))
				return nil
			}

			elapsed := time.Since(startTime)
			fmt.Printf("  Waiting for sync... (%v elapsed)\n", elapsed.Round(time.Second))
		}
	}
}
