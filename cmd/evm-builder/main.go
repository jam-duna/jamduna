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
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
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

// Builder configuration defaults
const (
	// DefaultMaxTxsPerBundle is the default maximum transactions per work package bundle
	DefaultMaxTxsPerBundle = 5
)


// maxTxsPerBundleConfig holds the configured max transactions per bundle
var maxTxsPerBundleConfig int

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
		maxTxsPerBundle   int
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
			fmt.Printf("  Max Txs Per Bundle: %d\n", maxTxsPerBundle)

			// Set package-level config for two-phase bundle building
			maxTxsPerBundleConfig = maxTxsPerBundle

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

			// Create submitter callback - submits bundle to guarantor via CE146
			submitter := func(bundle *types.WorkPackageBundle, coreIndex uint16) (common.Hash, error) {
				wpHash := bundle.WorkPackage.Hash()
				err := n.SubmitBundleToCore(bundle, coreIndex)
				if err != nil {
					return common.Hash{}, err
				}
				return wpHash, nil
			}

			// Create bundle builder callback - rebuilds bundle with fresh RefineContext
			// NOTE: Rebuilds use snapshot activation like initial builds for correct state chaining
			bundleBuilder := func(item *queue.QueueItem, stats queue.QueueStats) (*types.WorkPackageBundle, error) {
				if item.Bundle == nil {
					return nil, fmt.Errorf("queue item has no bundle")
				}
				// Calculate dynamic anchor offset based on queue position
				// Leave 2 blocks headroom before anchor expires
				// Subtract 1 for each bundle pair ahead in queue (fresher anchor for later bundles)
				queuePosition := stats.QueuedCount + stats.InflightCount
				bundlePairsAhead := queuePosition / types.TotalCores
				anchorOffset := types.RecentHistorySize - 2 - bundlePairsAhead
				if anchorOffset < 1 {
					anchorOffset = 1
				}

				// Get fresh refine context with dynamic offset
				refineCtx, err := n.GetRefineContextWithBuffer(anchorOffset)
				if err != nil {
					return nil, fmt.Errorf("failed to get refine context: %w", err)
				}
				// Update work package with new context
				item.Bundle.WorkPackage.RefineContext = refineCtx

				// Restore original WorkItem metadata before rebuild.
				// BuildBundle modifies WorkItems[].Extrinsics (prepends UBT witnesses) and
				// WorkItems[].Payload (changes type from Builder to Transactions, adds witness count).
				// We must restore the original state to avoid double-prepending witnesses.
				if item.OriginalWorkItemExtrinsics != nil {
					for i := range item.Bundle.WorkPackage.WorkItems {
						if i < len(item.OriginalWorkItemExtrinsics) {
							item.Bundle.WorkPackage.WorkItems[i].Extrinsics = item.OriginalWorkItemExtrinsics[i]
							// Also restore payload to original Builder type with correct tx count
							// Original payload: PayloadTypeBuilder (0x00), tx_count, globalDepth=0, witnesses=0, BAL=zeros
							txCount := len(item.OriginalWorkItemExtrinsics[i])
							item.Bundle.WorkPackage.WorkItems[i].Payload = evmtypes.BuildPayload(
								evmtypes.PayloadTypeBuilder,
								txCount,
								0,             // globalDepth
								0,             // numWitnesses (will be set by BuildBundle)
								common.Hash{}, // BAL hash (will be computed by BuildBundle)
							)
						}
					}
				}

				// Deep copy OriginalExtrinsics before passing to BuildBundle.
				// BuildBundle modifies extrinsicsBlobs in place (prepends UBT witnesses),
				// so we must pass a copy to avoid corrupting OriginalExtrinsics for future rebuilds.
				extrinsicsCopy := make([]types.ExtrinsicsBlobs, len(item.OriginalExtrinsics))
				for i, blobs := range item.OriginalExtrinsics {
					extrinsicsCopy[i] = make(types.ExtrinsicsBlobs, len(blobs))
					for j, blob := range blobs {
						extrinsicsCopy[i][j] = make([]byte, len(blob))
						copy(extrinsicsCopy[i][j], blob)
					}
				}

				// === Multi-Snapshot UBT: Create and activate snapshot for rebuild ===
				// CreateSnapshotForBlock is idempotent - if snapshot already exists, it reuses it.
				// No need to delete snapshots before rebuild.
				blockNumber := item.BlockNumber

				// Create snapshot for this block, chaining from previous block's snapshot or canonical
				if err := evmstorage.CreateSnapshotForBlock(blockNumber); err != nil {
					// If parent state was superseded (out-of-order commit), we cannot rebuild.
					// The bundle's state is already committed via a later block - skip this rebuild.
					log.Error(log.EVM, "Rebuild: Cannot create snapshot - aborting rebuild",
						"blockNumber", blockNumber,
						"error", err)
					return nil, fmt.Errorf("cannot rebuild block %d: %w", blockNumber, err)
				} else {
					// Activate the snapshot for reads during BuildBundle
					if err := evmstorage.SetActiveSnapshot(blockNumber); err != nil {
						log.Warn(log.EVM, "Rebuild: Failed to activate snapshot",
							"blockNumber", blockNumber,
							"error", err)
					}
				}

				// Rebuild via StateDB.BuildBundle (skip writes - already applied)
				bundle, _, err := n.BuildBundle(item.Bundle.WorkPackage, extrinsicsCopy, item.CoreIndex, nil, true)

				// Always clear active snapshot after build
				evmstorage.ClearActiveSnapshot()

				if err != nil {
					// On rebuild failure, invalidate the snapshot we created
					evmstorage.InvalidateSnapshotsFrom(blockNumber)
					return nil, fmt.Errorf("failed to rebuild bundle: %w", err)
				}

				log.Info(log.EVM, "Rebuild: completed with snapshot",
					"blockNumber", blockNumber,
					"pendingSnapshots", evmstorage.GetPendingSnapshotCount())

				return bundle, nil
			}

			queueRunner := queue.NewRunner(queueState, sid, submitter, bundleBuilder, evmstorage)

			// Set up snapshot-aware callbacks for multi-snapshot UBT parallel bundle building.
			// These callbacks commit/invalidate UBT snapshots in addition to txpool cleanup.
			queueRunner.SetOnAccumulatedWithSnapshots(evmstorage, func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash) {
				// Called AFTER snapshot is committed to canonical state
				log.Info(log.Node, "🗑️  Removing accumulated transactions from txpool",
					"wpHash", wpHash.Hex(),
					"blockNumber", blockNumber,
					"txCount", len(txHashes))
				removed := txPool.RemoveTransactionsByHashes(txHashes)
				log.Info(log.Node, "✅ Removed accumulated transactions",
					"wpHash", wpHash.Hex(),
					"blockNumber", blockNumber,
					"removed", removed,
					"requested", len(txHashes))
			})

			// Set callback to invalidate snapshots and unlock transactions when bundles fail permanently
			// This also invalidates descendant snapshots that depend on the failed bundle
			queueRunner.SetOnFailedWithSnapshots(evmstorage, func(wpHash common.Hash, blockNumber uint64, txHashes []common.Hash) {
				// Called AFTER snapshots are invalidated
				log.Warn(log.Node, "🔓 Unlocking transactions from failed bundle",
					"wpHash", wpHash.Hex(),
					"blockNumber", blockNumber,
					"txCount", len(txHashes))
				unlocked := txPool.UnlockTransactionsFromBundle(txHashes)
				log.Info(log.Node, "🔓 Unlocked transactions from failed bundle",
					"blockNumber", blockNumber,
					"unlocked", unlocked,
					"requested", len(txHashes))
			})

			queueRunner.Start(context.Background())
			defer queueRunner.Stop()

			// Start block notification handler for bundle building
			go handleBlockNotifications(n, rollup, txPool, sid, queueRunner)

			// Start block event handler for guarantee/accumulation detection
			go handleBlockEvents(n, sid, queueRunner)

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
	runCmd.Flags().IntVar(&maxTxsPerBundle, "max-txs-per-bundle", DefaultMaxTxsPerBundle, "Maximum transactions per work package bundle")

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

// phase1BlockData holds all data needed to execute Phase 2 for a block
type phase1BlockData struct {
	blockNumber               uint64
	coreIdx                   uint16
	workPackage               types.WorkPackage
	extrinsicsBlobs           []types.ExtrinsicsBlobs
	originalExtrinsics        []types.ExtrinsicsBlobs
	originalWorkItemExtrinsics [][]types.WorkItemExtrinsic
	txHashes                  []common.Hash
	phase1Result              *evmtypes.Phase1Result
	evmBlock                  *evmtypes.EvmBlockPayload
}

// handleBlockNotifications monitors for pending transactions and builds parallel bundles
// Two-Phase Pipeline:
//   Phase 1: Pull ALL txns upfront, build ALL evmBlocks (no witnesses) - FAST
//   Phase 2: Generate witnesses and submit bundles to JAM - with verification
func handleBlockNotifications(n *node.Node, rollup *evmrpc.Rollup, txPool *evmrpc.TxPool, serviceID uint32, queueRunner *queue.Runner) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Use GetPendingOnlyCount to exclude transactions already in bundles
		pendingCount := txPool.GetPendingOnlyCount()
		if pendingCount == 0 {
			continue
		}

		// Get storage for snapshot management
		storage, _ := n.GetStorage()
		evmStorage := storage.(types.EVMJAMStorage)

		// ============================================================
		// PHASE 1: Pull ALL txns upfront, build ALL evmBlocks (no witnesses)
		// ============================================================

		// (1) Atomically get AND lock ALL pending transactions (sorted by sender+nonce)
		// This prevents race conditions and ensures deterministic ordering
		allPendingTxs, allTxHashes := txPool.GetAndLockPendingTransactionsSorted()
		if len(allPendingTxs) == 0 {
			continue
		}

		log.Info(log.Node, "🔷 PHASE 1: Building all evmBlocks...",
			"totalPendingTxs", len(allPendingTxs),
			"maxTxsPerBundle", maxTxsPerBundleConfig)

		// Track which transactions were successfully processed for cleanup on failure
		// (2) Build ALL evmBlocks from the pre-fetched transaction list
		var phase1Blocks []phase1BlockData
		txOffset := 0
		blockIdx := 0
		phase1Failed := false

		for txOffset < len(allPendingTxs) {
			// Calculate how many txns for this bundle
			endOffset := txOffset + maxTxsPerBundleConfig
			if endOffset > len(allPendingTxs) {
				endOffset = len(allPendingTxs)
			}
			batchTxs := allPendingTxs[txOffset:endOffset]
			batchHashes := allTxHashes[txOffset:endOffset]

			// Round-robin core assignment
			coreIdx := uint16(blockIdx % types.TotalCores)

			// Calculate anchor offset based on queue position
			stats := queueRunner.GetStats()
			queuePosition := stats.QueuedCount + stats.InflightCount + len(phase1Blocks)
			bundlePairsAhead := queuePosition / types.TotalCores
			anchorOffset := types.RecentHistorySize - 2 - bundlePairsAhead
			if anchorOffset < 1 {
				anchorOffset = 1
			}

			blockData, err := executePhase1ForBatch(n, rollup, queueRunner, batchTxs, batchHashes, coreIdx, anchorOffset)
			if err != nil {
				log.Warn(log.Node, "Phase 1 failed", "blockIdx", blockIdx, "core", coreIdx, "err", err)
				phase1Failed = true
				break
			}

			// Assign real block number to this batch's transactions (updates from BlockNumber=0)
			txPool.AssignBlockNumber(batchHashes, blockData.blockNumber)

			phase1Blocks = append(phase1Blocks, *blockData)
			txOffset = endOffset
			blockIdx++
		}

		// If Phase 1 failed or produced no blocks, unlock unprocessed transactions
		if phase1Failed || len(phase1Blocks) == 0 {
			// Unlock transactions that weren't assigned to a block
			unprocessedHashes := allTxHashes[txOffset:]
			if len(unprocessedHashes) > 0 {
				txPool.UnlockTransactionsFromBundle(unprocessedHashes)
				log.Info(log.Node, "🔓 Unlocked unprocessed transactions after Phase 1 failure",
					"count", len(unprocessedHashes))
			}
			if len(phase1Blocks) == 0 {
				continue
			}
		}

		log.Info(log.Node, "🔷 PHASE 1 complete: All evmBlocks ready",
			"blocksBuilt", len(phase1Blocks),
			"totalTxsProcessed", txOffset)

		// ============================================================
		// PHASE 2: Generate witnesses and submit ALL bundles
		// ============================================================
		log.Info(log.Node, "🔶 PHASE 2: Generating witnesses and submitting bundles...")

		bundlesSubmitted := 0
		for _, blockData := range phase1Blocks {
			if err := executePhase2ForBlock(n, rollup, txPool, serviceID, queueRunner, evmStorage, &blockData); err != nil {
				log.Warn(log.Node, "Phase 2 failed for block",
					"blockNumber", blockData.blockNumber,
					"core", blockData.coreIdx,
					"err", err)
				// Continue with other blocks - don't break
				continue
			}
			bundlesSubmitted++
		}

		stats := txPool.GetStats()
		log.Info(log.Node, "🔶 PHASE 2 complete: Bundles submitted",
			"bundlesSubmitted", bundlesSubmitted,
			"pendingTxs", stats.PendingCount,
			"inBundleTxs", stats.InBundleCount,
			"queuedTxs", stats.QueuedCount)
	}
}

// executePhase1ForBatch executes Phase 1 for a batch of transactions: build evmBlock without witnesses.
// Takes pre-fetched and pre-locked transactions (already sorted by sender+nonce).
// Returns phase1BlockData containing all info needed for Phase 2.
func executePhase1ForBatch(n *node.Node, rollup *evmrpc.Rollup, queueRunner *queue.Runner, batchTxs []*evmtypes.EthereumTransaction, batchHashes []common.Hash, coreIdx uint16, anchorOffset int) (*phase1BlockData, error) {
	if len(batchTxs) == 0 {
		return nil, fmt.Errorf("no transactions in batch")
	}

	// Get refine context with the specified anchor offset
	refineCtx, err := n.GetRefineContextWithBuffer(anchorOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to get refine context: %w", err)
	}

	// Reserve block number upfront so each Phase 1 call gets a unique number
	blockNumber := queueRunner.ReserveNextBlockNumber()

	log.Info(log.Node, "🔷 Phase 1: Building evmBlock",
		"blockNumber", blockNumber,
		"core", coreIdx,
		"txCount", len(batchTxs),
		"anchorSlot", refineCtx.LookupAnchorSlot)

	// Prepare work package and extrinsics from batch transactions
	workPackage, extrinsicsBlobs, err := rollup.PrepareWorkPackage(refineCtx, batchTxs)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare work package: %w", err)
	}

	// Save original extrinsics BEFORE any modification (deep copy)
	originalExtrinsics := make([]types.ExtrinsicsBlobs, len(extrinsicsBlobs))
	for i, blobs := range extrinsicsBlobs {
		originalExtrinsics[i] = make(types.ExtrinsicsBlobs, len(blobs))
		for j, blob := range blobs {
			originalExtrinsics[i][j] = make([]byte, len(blob))
			copy(originalExtrinsics[i][j], blob)
		}
	}

	// Save original WorkItems[].Extrinsics metadata
	originalWorkItemExtrinsics := make([][]types.WorkItemExtrinsic, len(workPackage.WorkItems))
	for i, wi := range workPackage.WorkItems {
		originalWorkItemExtrinsics[i] = make([]types.WorkItemExtrinsic, len(wi.Extrinsics))
		copy(originalWorkItemExtrinsics[i], wi.Extrinsics)
	}

	// Execute Phase 1: EVM without witnesses
	// Note: No snapshot needed here - Phase 2 pins to pre-state root directly
	phase1Result, err := rollup.ExecutePhase1(workPackage, originalExtrinsics)
	if err != nil {
		return nil, fmt.Errorf("Phase 1 execution failed: %w", err)
	}

	// Build EvmBlockPayload
	var receipts []evmtypes.TransactionReceipt
	for _, r := range phase1Result.Receipts {
		if r != nil {
			receipts = append(receipts, *r)
		}
	}

	evmBlock := &evmtypes.EvmBlockPayload{
		Number:          uint32(blockNumber),
		NumTransactions: uint32(len(batchTxs)),
		GasUsed:         phase1Result.TotalGasUsed,
		UBTRoot:         phase1Result.EVMPostStateRoot,
		TxHashes:        batchHashes,
		Transactions:    receipts,
	}

	blockCommitment := evmBlock.BlockCommitment()
	log.Info(log.Node, "🔷 Phase 1 complete: evmBlock ready",
		"blockNumber", blockNumber,
		"blockCommitment", blockCommitment.Hex(),
		"preStateRoot", phase1Result.EVMPreStateRoot.Hex(),
		"postStateRoot", phase1Result.EVMPostStateRoot.Hex(),
		"receipts", len(receipts))

	// Store block in cache for RPC serving (receipts available immediately)
	rollup.AddBlock(evmBlock)

	return &phase1BlockData{
		blockNumber:                blockNumber,
		coreIdx:                    coreIdx,
		workPackage:                workPackage,
		extrinsicsBlobs:            extrinsicsBlobs,
		originalExtrinsics:         originalExtrinsics,
		originalWorkItemExtrinsics: originalWorkItemExtrinsics,
		txHashes:                   batchHashes,
		phase1Result:               phase1Result,
		evmBlock:                   evmBlock,
	}, nil
}

// executePhase2ForBlock executes Phase 2 for a block: generate witnesses and submit bundle.
func executePhase2ForBlock(n *node.Node, rollup *evmrpc.Rollup, txPool *evmrpc.TxPool, serviceID uint32, queueRunner *queue.Runner, evmStorage types.EVMJAMStorage, blockData *phase1BlockData) error {
	blockCommitment := blockData.evmBlock.BlockCommitment()

	log.Info(log.Node, "🔶 Phase 2: Generating witnesses",
		"blockNumber", blockData.blockNumber,
		"blockCommitment", blockCommitment.Hex(),
		"preStateRoot", blockData.phase1Result.EVMPreStateRoot.Hex())

	// Pin to pre-state root for witness generation
	if err := evmStorage.PinToStateRoot(blockData.phase1Result.EVMPreStateRoot); err != nil {
		txPool.UnlockTransactionsFromBundle(blockData.txHashes)
		return fmt.Errorf("CRITICAL: failed to pin to pre-state root: %w", err)
	}

	// Build bundle with witness generation (skip writes - Phase 1 already applied them)
	bundle, workReport, err := n.BuildBundle(blockData.workPackage, blockData.extrinsicsBlobs, blockData.coreIdx, nil, true)

	// Unpin after build
	evmStorage.UnpinState()

	if err != nil {
		txPool.UnlockTransactionsFromBundle(blockData.txHashes)
		return fmt.Errorf("Phase 2 bundle build failed: %w", err)
	}

	// Update the cached evmBlock with the BlockCommitment as the BlockHash
	// This is needed for eth_getTransactionReceipt to return correct BlockHash
	// We use blockCommitment (voting digest) rather than bundle.WorkPackage.Hash() because:
	// - WorkPackageHash includes RefineContext which changes on resubmission
	// - BlockCommitment is derived from deterministic 148-byte header and is stable
	blockData.evmBlock.WorkPackageHash = blockCommitment
	// Also update each receipt's BlockHash and BlockNumber
	for i := range blockData.evmBlock.Transactions {
		blockData.evmBlock.Transactions[i].BlockHash = blockCommitment
		blockData.evmBlock.Transactions[i].BlockNumber = uint32(blockData.blockNumber)
	}
	// Also update the block cache's byHash index
	rollup.GetBlockCache().UpdateBlockHash(blockData.evmBlock, blockCommitment)

	// Enqueue bundle using pre-reserved block number from Phase 1
	err = queueRunner.EnqueueBundleWithReservedBlockNumber(
		bundle,
		blockData.originalExtrinsics,
		blockData.originalWorkItemExtrinsics,
		blockData.coreIdx,
		blockData.txHashes,
		blockData.blockNumber,
	)
	if err != nil {
		txPool.UnlockTransactionsFromBundle(blockData.txHashes)
		return fmt.Errorf("failed to enqueue bundle: %w", err)
	}

	log.Info(log.Node, "🔶 Phase 2 complete: Bundle submitted",
		"blockNumber", blockData.blockNumber,
		"wpHash", bundle.WorkPackage.Hash().Hex(),
		"workReportHash", workReport.Hash().Hex(),
		"blockCommitment", blockCommitment.Hex(),
		"core", blockData.coreIdx,
		"serviceID", serviceID,
		"txCount", len(blockData.txHashes))

	return nil
}

// handleBlockEvents monitors imported blocks for guarantees and accumulations
// This is critical for the queue to know when work packages are guaranteed/accumulated
func handleBlockEvents(n *node.Node, serviceID uint32, queueRunner *queue.Runner) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastProcessedSlot uint32
	// Track which work package hashes we've already notified as accumulated
	// to avoid duplicate notifications
	notifiedAccumulated := make(map[common.Hash]struct{})

	for range ticker.C {
		stateDB := n.GetStateDB()
		if stateDB == nil {
			continue
		}
		block := stateDB.GetBlock()
		if block == nil {
			continue
		}

		currentSlot := block.TimeSlot()
		if currentSlot <= lastProcessedSlot {
			continue
		}
		lastProcessedSlot = currentSlot

		// Check for guarantees (E_G) in this block
		if len(block.Extrinsic.Guarantees) > 0 {
			log.Info(log.Node, "🔒 Block has E_G (guarantees)",
				"slot", currentSlot,
				"count", len(block.Extrinsic.Guarantees))
		}
		for _, guarantee := range block.Extrinsic.Guarantees {
			wpHash := guarantee.Report.GetWorkPackageHash()
			log.Info(log.Node, "🔒 E_G: Guarantee detected",
				"slot", currentSlot,
				"wpHash", wpHash.Hex(),
				"coreIndex", guarantee.Report.CoreIndex)
			queueRunner.HandleGuaranteed(wpHash)

			// Store work report for segment retrieval via FetchJAMDASegments
			// This is critical - without this, blocks cannot be read back after accumulation
			// because the builder cache is cleared after guarantee persistence
			if err := n.StoreWorkReport(guarantee.Report); err != nil {
				log.Error(log.Node, "handleBlockEvents: StoreWorkReport failed", "wpHash", wpHash.Hex(), "err", err)
			} else {
				log.Info(log.Node, "💾 Stored work report for segment retrieval", "wpHash", wpHash.Hex(), "exportedSegmentRoot", guarantee.Report.AvailabilitySpec.ExportedSegmentRoot.Hex())
			}
		}

		// Check for accumulations in AccumulationHistory
		// AccumulationHistory is a sliding window where [EpochLength-1] is the most recent.
		// We scan all slots to catch any accumulations we might have missed.
		jamState := stateDB.GetJamState()
		if jamState != nil {
			// Log accumulation history state periodically for debugging
			var totalInHistory int
			for i := 0; i < types.EpochLength; i++ {
				totalInHistory += len(jamState.AccumulationHistory[i].WorkPackageHash)
			}
			if totalInHistory > 0 {
				log.Debug(log.Node, "📊 AccumulationHistory state",
					"slot", currentSlot,
					"totalWPsInHistory", totalInHistory,
					"latestSlotCount", len(jamState.AccumulationHistory[types.EpochLength-1].WorkPackageHash))
			}

			// Check all history slots for accumulations (not just the latest)
			// This ensures we catch accumulations even if we missed a slot
			for historyIdx := 0; historyIdx < types.EpochLength; historyIdx++ {
				for _, wpHash := range jamState.AccumulationHistory[historyIdx].WorkPackageHash {
					if _, already := notifiedAccumulated[wpHash]; !already {
						log.Info(log.Node, "📦 Accumulation detected",
							"slot", currentSlot,
							"historyIndex", historyIdx,
							"wpHash", wpHash.Hex())
						queueRunner.HandleAccumulated(wpHash)
						notifiedAccumulated[wpHash] = struct{}{}
					}
				}
			}
		}
	}
}

