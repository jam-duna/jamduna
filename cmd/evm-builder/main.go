// EVM Builder - Standalone EVM RPC server for JAM
// This binary runs as a builder node (validator index 6) that:
// 1. Syncs state from the JAM network using proper credentials
// 2. Exposes an Ethereum-compatible RPC on port 8545
// 3. Builds and submits work packages for EVM transactions
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

	evmrpc "github.com/colorfulnotion/jam/builder/evm/rpc"
	"github.com/colorfulnotion/jam/builder/queue"
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
		Use:   "evm-builder",
		Short: "JAM EVM Builder Node",
	}
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	var (
		dataPath          string
		evmRPCPort        int
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
		Short: "Start EVM builder node with proper network sync",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Starting JAM EVM Builder Node\n")
			fmt.Printf("  Validator Index: %d\n", validatorIndex)
			fmt.Printf("  Chain Spec: %s\n", chainSpec)
			fmt.Printf("  Data Path: %s\n", dataPath)
			fmt.Printf("  EVM RPC Port: %d\n", evmRPCPort)
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
			evmstorage := storage.(types.EVMJAMStorage)
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
			// 8. Setup EVM rollup and RPC server
			fmt.Printf("\n[7/7] Setting up EVM RPC server...\n")
			sid := uint32(serviceID)
			// Create rollup instance
			rollup, err := evmrpc.NewRollup(evmstorage, sid, n, pvmBackend)
			if err != nil {
				os.Exit(1)
			}

			txPool := evmrpc.NewTxPool()
			rollup.SetTxPool(txPool)

			handler := evmrpc.NewEVMRPCHandler(rollup, txPool)
			server := evmrpc.NewEVMHTTPServer(handler)
			if err := server.Start(evmRPCPort); err != nil {
				fmt.Printf("Failed to start EVM RPC server: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ EVM RPC server started on port %d\n", evmRPCPort)

			// Create queue runner for managed submission
			queueState := queue.NewQueueState(sid)

			// Submit callback - sends bundle to network
			submitFunc := func(bundle *types.WorkPackageBundle) (common.Hash, error) {
				n.SubmitBundleSameCore(bundle)
				return bundle.WorkPackage.Hash(), nil
			}

			// Build bundle callback - rebuilds bundle with fresh RefineContext
			buildBundleFunc := func(item *queue.QueueItem) (*types.WorkPackageBundle, error) {
				// Get fresh refine context
				refineCtx, err := n.GetRefineContextWithBuffer(EVMBuilderBuffer)
				if err != nil {
					return nil, err
				}
				// Update the work package with new refine context
				item.Bundle.WorkPackage.RefineContext = refineCtx

				// CRITICAL: Restore original WorkItems[].Extrinsics metadata before rebuilding
				// BuildBundle prepends Verkle witness metadata to this list, so we need to reset it
				if item.OriginalWorkItemExtrinsics != nil {
					for i, origExtrinsics := range item.OriginalWorkItemExtrinsics {
						if i < len(item.Bundle.WorkPackage.WorkItems) {
							// Deep copy the original metadata
							item.Bundle.WorkPackage.WorkItems[i].Extrinsics = make([]types.WorkItemExtrinsic, len(origExtrinsics))
							copy(item.Bundle.WorkPackage.WorkItems[i].Extrinsics, origExtrinsics)
						}
					}
				}

				// Use OriginalExtrinsics (transaction-only, no Verkle witness) for rebuilding
				// BuildBundle will prepend a fresh Verkle witness with the new RefineContext
				var extrinsicsForRebuild []types.ExtrinsicsBlobs
				if item.OriginalExtrinsics == nil {
					// Fallback: shouldn't happen if EnqueueBundleWithOriginalExtrinsics was used
					log.Warn(log.Node, "buildBundleFunc: OriginalExtrinsics is nil, using Bundle.ExtrinsicData (may cause issues)")
					extrinsicsForRebuild = item.Bundle.ExtrinsicData
				} else {
					// CRITICAL: Make a deep copy of OriginalExtrinsics to prevent mutation
					// BuildBundle modifies extrinsicsBlobs in place (prepends Verkle witness),
					// so we must copy to avoid corrupting OriginalExtrinsics for future resubmissions
					extrinsicsForRebuild = make([]types.ExtrinsicsBlobs, len(item.OriginalExtrinsics))
					for i, blobs := range item.OriginalExtrinsics {
						extrinsicsForRebuild[i] = make(types.ExtrinsicsBlobs, len(blobs))
						for j, blob := range blobs {
							// Deep copy each byte slice
							extrinsicsForRebuild[i][j] = make([]byte, len(blob))
							copy(extrinsicsForRebuild[i][j], blob)
						}
					}
				}

				// Rebuild the bundle with transaction-only extrinsics
				bundle, _, err := n.BuildBundle(item.Bundle.WorkPackage, extrinsicsForRebuild, 0, nil)
				return bundle, err
			}

			queueRunner := queue.NewRunner(queueState, sid, submitFunc, buildBundleFunc)
			queueRunner.Start(context.Background())
			defer queueRunner.Stop()

			// Start block notification handler for bundle building
			go handleBlockNotifications(n, rollup, txPool, sid, queueRunner)

			fmt.Printf("\n========================================\n")
			fmt.Printf("EVM Builder Ready!\n")
			fmt.Printf("  EVM RPC: http://localhost:%d\n", evmRPCPort)
			fmt.Printf("  Service ID: %d\n", serviceID)
			fmt.Printf("  Validator: %d\n", validatorIndex)
			fmt.Printf("========================================\n")

			// Wait for shutdown signal
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			<-sigChan

			fmt.Printf("\nShutting down EVM builder...\n")
		},
	}

	// Flags
	runCmd.Flags().StringVarP(&dataPath, "data-path", "d", filepath.Join(os.Getenv("HOME"), ".jamduna"), "Data directory (should match validator data path)")
	runCmd.Flags().IntVar(&evmRPCPort, "evm-rpc-port", 8545, "EVM RPC server port")
	runCmd.Flags().IntVar(&validatorIndex, "dev-validator", 6, "Validator index (use 6 for builder)")
	runCmd.Flags().IntVar(&serviceID, "service-id", 0, "EVM service ID")
	runCmd.Flags().StringVar(&chainSpec, "chain", "chainspec.json", "Chain spec file")
	// Default to compiler on Linux (fast), interpreter elsewhere (portable)
	defaultPVMBackend := statedb.BackendInterpreter
	if runtime.GOOS == "linux" {
		defaultPVMBackend = statedb.BackendCompiler
	}
	runCmd.Flags().StringVar(&pvmBackend, "pvm-backend", defaultPVMBackend, "PVM backend (interpreter, compiler)")
	runCmd.Flags().StringVar(&debug, "debug", "rotation,guarantees", "Debug modules to enable")
	runCmd.Flags().IntVar(&port, "port", 0, "Network port (default: 40000 + validator index)")
	runCmd.Flags().StringVar(&telemetryEndpoint, "telemetry", "", "Telemetry server endpoint (e.g., localhost:9999)")

	rootCmd.AddCommand(runCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// getValidatorSecret reads the seed file and generates validator credentials
// Same logic as jam.go CheckValidatorInfo
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
// Same logic as jam.go getValidatorFromChainSpec
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
// It considers the node synced if either:
// 1. There's a finalized block (normal case after ~5 blocks)
// 2. There's a latest block at chain head (new network with < 5 blocks)
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

// handleBlockNotifications monitors for new blocks and pending transactions
func handleBlockNotifications(n *node.Node, rollup *evmrpc.Rollup, txPool *evmrpc.TxPool, serviceID uint32, queueRunner *queue.Runner) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if txPool.Size() == 0 {
			continue
		}

		log.Info(log.Node, "Pending transactions detected", "count", txPool.Size(), "service_id", serviceID)

		if err := buildAndEnqueueWorkPackage(n, rollup, txPool, serviceID, queueRunner); err != nil {
			log.Error(log.Node, "Failed to build and enqueue work package", "err", err)
		}
	}
}

// EVMBuilderBuffer is the anchor buffer depth for EVM work packages.
// Larger buffer = more time for work package to reach validators before anchor expires.
// With RecentHistorySize=8, buffer=3 means validators can be up to 5 blocks ahead.
const EVMBuilderBuffer = 3

// buildAndEnqueueWorkPackage builds a work package from pending txs and enqueues for submission
// Routes through NodeContent.BuildBundle -> StateDB.BuildBundle which executes refine
func buildAndEnqueueWorkPackage(n *node.Node, rollup *evmrpc.Rollup, txPool *evmrpc.TxPool, serviceID uint32, queueRunner *queue.Runner) error {
	// Get pending transactions
	pendingTxs := txPool.GetPendingTransactions()
	if len(pendingTxs) == 0 {
		return fmt.Errorf("no pending transactions")
	}

	// Get refine context with EVM-specific buffer (larger than default for more tolerance)
	refineCtx, err := n.GetRefineContextWithBuffer(EVMBuilderBuffer)
	if err != nil {
		return fmt.Errorf("failed to get refine context: %w", err)
	}

	// Prepare work package and extrinsics from pending transactions
	// extrinsicsBlobs contains ONLY the transaction RLP data (no Verkle witness yet)
	workPackage, extrinsicsBlobs, err := rollup.PrepareWorkPackage(refineCtx, pendingTxs)
	if err != nil {
		return fmt.Errorf("failed to prepare work package: %w", err)
	}

	// Save original extrinsics BEFORE BuildBundle prepends the Verkle witness
	// This is needed for rebuilding on resubmission
	// CRITICAL: Must deep copy to prevent mutation by BuildBundle
	originalExtrinsics := make([]types.ExtrinsicsBlobs, len(extrinsicsBlobs))
	for i, blobs := range extrinsicsBlobs {
		originalExtrinsics[i] = make(types.ExtrinsicsBlobs, len(blobs))
		for j, blob := range blobs {
			originalExtrinsics[i][j] = make([]byte, len(blob))
			copy(originalExtrinsics[i][j], blob)
		}
	}

	// Save original WorkItems[].Extrinsics metadata BEFORE BuildBundle prepends Verkle witness metadata
	// This is needed for rebuilding on resubmission - BuildBundle also prepends metadata entries
	originalWorkItemExtrinsics := make([][]types.WorkItemExtrinsic, len(workPackage.WorkItems))
	for i, wi := range workPackage.WorkItems {
		originalWorkItemExtrinsics[i] = make([]types.WorkItemExtrinsic, len(wi.Extrinsics))
		copy(originalWorkItemExtrinsics[i], wi.Extrinsics)
	}

	// Build bundle via NodeContent.BuildBundle -> StateDB.BuildBundle (executes refine)
	// NOTE: BuildBundle prepends a Verkle witness to each work item's extrinsics
	bundle, workReport, err := n.BuildBundle(workPackage, extrinsicsBlobs, 0, nil)
	if err != nil {
		return fmt.Errorf("failed to build bundle: %w", err)
	}

	// Clear processed transactions from pool
	for _, tx := range pendingTxs {
		txPool.RemoveTransaction(tx.Hash)
	}

	// Enqueue to queue runner with original extrinsics for resubmission support
	blockNumber, err := queueRunner.EnqueueBundleWithOriginalExtrinsics(bundle, originalExtrinsics, originalWorkItemExtrinsics)
	if err != nil {
		return fmt.Errorf("failed to enqueue bundle: %w", err)
	}

	log.Info(log.Node, "Work package enqueued",
		"wp_hash", bundle.WorkPackage.Hash().Hex(),
		"work_report_hash", workReport.Hash().Hex(),
		"block_number", blockNumber,
		"service_id", serviceID)

	return nil
}
