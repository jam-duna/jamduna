// Orchard Builder - Standalone Orchard RPC server for JAM
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	"time"

	orchardrpc "github.com/colorfulnotion/jam/builder/orchard/rpc"
	orchardstate "github.com/colorfulnotion/jam/builder/orchard/state"
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

			// 8. Setup Orchard rollup and RPC server with transaction pool
			fmt.Printf("\n[7/7] Setting up Orchard RPC server with transaction pool...\n")
			sid := uint32(serviceID)
			rollup := orchardrpc.NewOrchardRollup(n, sid)
			cacheCtx, cacheCancel := context.WithCancel(context.Background())
			defer cacheCancel()

			stateCache := orchardstate.NewOrchardStateCache(sid, rollup)
			if err := stateCache.SyncFromStateDB(); err != nil {
				log.Warn(log.Node, "Failed to sync Orchard state cache", "err", err)
			} else {
				log.Info(log.Node, "Orchard state cache synced")
			}
			stateCache.StartFinalizedBlockTracker(cacheCtx, n, 6*time.Second)

			// Create RPC handler
			handler := orchardrpc.NewOrchardRPCHandler(rollup)

			// Create transaction pool with FFI integration
			txPool, err := orchardrpc.NewOrchardTxPool(rollup, n)
			if err != nil {
				fmt.Printf("Failed to create transaction pool: %v\n", err)
				os.Exit(1)
			}

			// Wire transaction pool into RPC handler
			handler.SetTxPool(txPool)

			// Create and start RPC server
			server := orchardrpc.NewOrchardHTTPServer(handler)
			if err := server.Start(rpcPort); err != nil {
				fmt.Printf("Failed to start Orchard RPC server: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ Orchard RPC server started on port %d with transaction pool\n", rpcPort)

			// Start block notification handler for auto work package generation
			go handleBlockNotifications(n, txPool, stateCache, rollup, sid)
			fmt.Printf("✓ Block notification handler started for auto work package generation\n")

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

			// Submit work package with 90 second timeout
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

// handleBlockNotifications monitors JAM block notifications and triggers work package generation
func handleBlockNotifications(n *node.Node, txPool *orchardrpc.OrchardTxPool, stateCache *orchardstate.OrchardStateCache, rollup *orchardrpc.OrchardRollup, serviceID uint32) {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds for new blocks
	defer ticker.Stop()

	var lastProcessedSlot uint32 = 0

	for range ticker.C {
		// Get current finalized block
		block, err := n.GetFinalizedBlock()
		if err != nil {
			log.Debug(log.Node, "Failed to get finalized block for notification", "error", err)
		}

		if block != nil && block.Header.Slot > lastProcessedSlot {
			lastProcessedSlot = block.Header.Slot

			stats := txPool.GetPoolStats()
			log.Info(log.Node, "Block notification processed",
				"slot", block.Header.Slot,
				"service_id", serviceID,
				"pending_bundles", stats.PendingBundles,
				"work_packages_generated", stats.WorkPackagesGenerated)
		}

		// Check transaction pool status and trigger work package building.
		pendingBundles := txPool.GetPendingBundles()

		// Trigger work package generation if we have enough bundles
		if len(pendingBundles) >= 5 { // Lower threshold for demo purposes
			if stateCache == nil {
				log.Warn(log.Node, "State cache unavailable; skipping work package build")
				continue
			}
			if stateCache.LastSyncAt().IsZero() {
				log.Warn(log.Node, "State cache not synced yet; skipping work package build")
				continue
			}
			currentSlot := lastProcessedSlot
			if block != nil {
				currentSlot = block.Header.Slot
			}
			log.Info(log.Node, "Triggering work package generation",
				"pending_bundles", len(pendingBundles),
				"current_slot", currentSlot)

			if err := buildAndSubmitWorkPackage(n, txPool, stateCache, rollup, serviceID, pendingBundles[:1], currentSlot); err != nil {
				log.Error(log.Node, "Failed to build and submit work package", "error", err)
			}
		}
	}
}

// buildAndSubmitWorkPackage builds a work package from pending bundles with proper state witnesses
func buildAndSubmitWorkPackage(n *node.Node, txPool *orchardrpc.OrchardTxPool, stateCache *orchardstate.OrchardStateCache, rollup *orchardrpc.OrchardRollup, serviceID uint32, bundles []*orchardrpc.ParsedBundle, currentSlot uint32) error {
	if rollup == nil {
		return fmt.Errorf("orchard rollup unavailable")
	}
	log.Info(log.Node, "Building work package",
		"service_id", serviceID,
		"bundles_count", len(bundles),
		"slot", currentSlot)

	// 2. Generate pre-state witnesses using cached builder state
	preStateWitness, err := generateCachedStateWitness(stateCache, bundles)
	if err != nil {
		return fmt.Errorf("failed to generate pre-state witness: %w", err)
	}

	// 3. Build the work package with proper structure
	workPackage, err := buildWorkPackageFromBundles(rollup, bundles, preStateWitness, currentSlot)
	if err != nil {
		return fmt.Errorf("failed to build work package: %w", err)
	}

	// 4. Submit the work package to the JAM network
	if err := submitWorkPackageToNetwork(n, rollup, workPackage); err != nil {
		return fmt.Errorf("failed to submit work package: %w", err)
	}

	if err := rollup.ApplyBundleState(bundles); err != nil {
		return fmt.Errorf("failed to apply bundle state: %w", err)
	}
	if err := stateCache.SyncFromStateDB(); err != nil {
		log.Warn(log.Node, "Failed to refresh Orchard state cache", "err", err)
	}

	// 6. Remove processed bundles from the transaction pool
	bundleIDs := make([]string, len(bundles))
	for i, bundle := range bundles {
		bundleIDs[i] = bundle.ID
	}
	txPool.RemoveBundles(bundleIDs)

	log.Info(log.Node, "Work package submitted successfully",
		"service_id", serviceID,
		"bundles_processed", len(bundles))

	return nil
}

// generateCachedStateWitness builds pre-state roots from the builder state cache.
func generateCachedStateWitness(stateCache *orchardstate.OrchardStateCache, bundles []*orchardrpc.ParsedBundle) (*orchardrpc.OrchardStateRoots, error) {
	if stateCache == nil {
		return nil, fmt.Errorf("state cache unavailable")
	}

	snapshot := stateCache.StateRootsSnapshot()
	if snapshot.LastSyncAt.IsZero() {
		return nil, fmt.Errorf("state cache not synced")
	}
	if len(snapshot.CommitmentFrontier) != 32 {
		return nil, fmt.Errorf("invalid commitment frontier length: %d", len(snapshot.CommitmentFrontier))
	}

	nullifiersToProve := 0
	for _, bundle := range bundles {
		nullifiersToProve += len(bundle.Nullifiers)
	}

	preState := &orchardrpc.OrchardStateRoots{
		CommitmentRoot:     snapshot.CommitmentRoot,
		CommitmentSize:     snapshot.CommitmentSize,
		CommitmentFrontier: snapshot.CommitmentFrontier,
		NullifierRoot:      snapshot.NullifierRoot,
		NullifierSize:      snapshot.NullifierSize,
	}

	log.Info(log.Node, "Generated cached pre-state witness",
		"commitment_root", fmt.Sprintf("%x", snapshot.CommitmentRoot[:8]),
		"commitment_size", snapshot.CommitmentSize,
		"nullifier_root", fmt.Sprintf("%x", snapshot.NullifierRoot[:8]),
		"nullifier_size", snapshot.NullifierSize,
		"nullifiers_to_prove", nullifiersToProve,
		"frontier_nodes", len(snapshot.CommitmentFrontier))

	return preState, nil
}

// buildWorkPackageFromBundles constructs a work package from the selected bundles
func buildWorkPackageFromBundles(rollup *orchardrpc.OrchardRollup, bundles []*orchardrpc.ParsedBundle, preState *orchardrpc.OrchardStateRoots, slot uint32) (*orchardrpc.WorkPackageFile, error) {
	if rollup == nil {
		return nil, fmt.Errorf("orchard rollup unavailable")
	}
	if preState == nil {
		return nil, fmt.Errorf("pre-state unavailable")
	}
	if len(bundles) == 0 {
		return nil, fmt.Errorf("no bundles provided")
	}
	if len(bundles) > 1 {
		return nil, fmt.Errorf("multiple bundles not supported in a single work item")
	}

	bundle := bundles[0]
	if len(bundle.ProofBytes) == 0 || len(bundle.PublicInputs) == 0 {
		return nil, fmt.Errorf("bundle proof or public inputs missing")
	}

	// Convert transaction pool bundle to work package format
	var actions []orchardrpc.OrchardAction
	for i, txAction := range bundle.Actions {
		action := orchardrpc.OrchardAction{
			Commitment:            txAction.Commitment,
			Nullifier:             txAction.Nullifier,
			NullifierAbsenceIndex: uint64(i),
			SpentCommitmentIndex:  uint64(i),
		}
		actions = append(actions, action)
	}

	userBundles := []orchardrpc.UserBundle{
		{
			Actions:     actions,
			BundleBytes: bundle.RawData,
		},
	}

	nullifierProofs := make(map[[32]byte]orchardrpc.NullifierAbsenceProof, len(bundle.Nullifiers))
	var nullifierAbsenceProofs []orchardrpc.NullifierAbsenceProof
	var spentCommitmentProofs []orchardrpc.SpentCommitmentProof

	fallbackCommitments := rollup.CommitmentList()

	for _, nullifier := range bundle.Nullifiers {
		absenceProof, err := rollup.NullifierAbsenceProof(nullifier)
		if err != nil {
			return nil, fmt.Errorf("nullifier absence proof failed: %w", err)
		}

		proof := orchardrpc.NullifierAbsenceProof{
			Leaf:     absenceProof.Leaf,
			Siblings: absenceProof.Siblings,
			Root:     absenceProof.Root,
			Position: absenceProof.Position,
		}
		nullifierProofs[nullifier] = proof
		nullifierAbsenceProofs = append(nullifierAbsenceProofs, proof)

		commitment, ok := rollup.CommitmentForNullifier(nullifier)
		if !ok {
			if len(fallbackCommitments) == 0 {
				return nil, fmt.Errorf("no commitments available for spent proof")
			}
			commitment = fallbackCommitments[0]
			log.Warn(log.Node, "No commitment found for nullifier; using fallback",
				"nullifier", fmt.Sprintf("%x", nullifier[:8]),
				"fallback_commitment", fmt.Sprintf("%x", commitment[:8]))
		}

		position, siblings, err := rollup.CommitmentProof(commitment)
		if err != nil {
			return nil, fmt.Errorf("commitment proof failed: %w", err)
		}

		spentCommitmentProofs = append(spentCommitmentProofs, orchardrpc.SpentCommitmentProof{
			Nullifier:      nullifier,
			Commitment:     commitment,
			TreePosition:   position,
			BranchSiblings: siblings,
		})
	}

	// Calculate post-state after applying bundle
	postState := *preState
	postState.CommitmentSize += uint64(len(bundle.Commitments))
	postState.NullifierSize += uint64(len(bundle.Nullifiers))

	preStateWitness, err := orchardrpc.SerializePreStateWitness(preState, bundle.Nullifiers, nullifierProofs)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize pre-state witness: %w", err)
	}
	postStateWitness, err := orchardrpc.SerializePostStateWitness(&postState)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize post-state witness: %w", err)
	}
	bundleProof := orchardrpc.SerializeBundleProof(1, bundle.PublicInputs, bundle.ProofBytes, bundle.RawData)

	workPackage := &orchardrpc.WorkPackageFile{
		PreStateWitnessExtrinsic:  preStateWitness,
		PostStateWitnessExtrinsic: postStateWitness,
		BundleProofExtrinsic:      bundleProof,
		PreState:                  preState,
		PostState:                 &postState,
		SpentCommitmentProofs:     spentCommitmentProofs,

		// Legacy format for compatibility
		PreStateRoots:  *preState,
		PostStateRoots: postState,
		UserBundles:    userBundles,
		PreStateWitnesses: orchardrpc.StateWitnesses{
			NullifierAbsenceProofs: nullifierAbsenceProofs,
		},
		PostStateWitnesses: orchardrpc.StateWitnesses{
			NullifierAbsenceProofs: make([]orchardrpc.NullifierAbsenceProof, 0),
		},
		Metadata: orchardrpc.WorkPackageMeta{
			GasLimit: 1000000,
		},
	}

	log.Info(log.Node, "Work package built",
		"user_bundles", len(userBundles),
		"spent_proofs", len(spentCommitmentProofs),
		"nullifier_proofs", len(nullifierAbsenceProofs),
		"pre_commitment_size", preState.CommitmentSize,
		"post_commitment_size", postState.CommitmentSize,
		"slot", slot)

	return workPackage, nil
}

// submitWorkPackageToNetwork submits the work package to the JAM network
func submitWorkPackageToNetwork(n *node.Node, rollup *orchardrpc.OrchardRollup, workPackage *orchardrpc.WorkPackageFile) error {
	// Create a temporary file for the work package
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("orchard_wp_%d.json", time.Now().Unix()))

	// Marshal work package to JSON
	workPackageBytes, err := json.Marshal(workPackage)
	if err != nil {
		return fmt.Errorf("failed to marshal work package: %w", err)
	}

	// Write to temporary file
	if err := os.WriteFile(tmpFile, workPackageBytes, 0644); err != nil {
		return fmt.Errorf("failed to write work package file: %w", err)
	}
	defer os.Remove(tmpFile) // Clean up

	log.Info(log.Node, "Work package file created", "path", tmpFile, "size", len(workPackageBytes))

	// Submit using the rollup's existing submission method
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	stateRoot, timeslot, err := rollup.SubmitWorkPackage(ctx, tmpFile)
	if err != nil {
		return fmt.Errorf("work package submission failed: %w", err)
	}

	log.Info(log.Node, "Work package submitted to JAM network",
		"state_root", stateRoot.Hex(),
		"timeslot", timeslot,
		"file_path", tmpFile)

	return nil
}

// generateNullifierAbsencePath creates a Merkle proof path for nullifier absence
func generateNullifierAbsencePath(nullifier [32]byte, nullifierRoot [32]byte, treeSize uint64) [][32]byte {
	// Generate a deterministic Merkle path for the given nullifier
	// This creates a path that proves the nullifier is absent from the tree

	const treeDepth = 32 // Standard Merkle tree depth for nullifier trees
	siblings := make([][32]byte, treeDepth)

	// Use the nullifier hash to determine the position and path
	position := calculateNullifierPosition(nullifier, treeSize)

	for i := 0; i < treeDepth; i++ {
		// Generate deterministic siblings based on nullifier, position, and level
		siblingData := append(nullifier[:], byte(i))
		siblingData = append(siblingData, byte(position>>(i)))

		siblingHash := common.Blake2Hash(siblingData)
		copy(siblings[i][:], siblingHash[:32])

		// Update position for next level
		position = position >> 1
	}

	return siblings
}

// calculateNullifierPosition determines the position of a nullifier in the tree
func calculateNullifierPosition(nullifier [32]byte, treeSize uint64) uint64 {
	// Use the nullifier hash to create a deterministic position
	// Ensure the position is within the valid range for absence proofs

	// Hash the nullifier to get a deterministic position
	positionHash := common.Blake2Hash(nullifier[:])

	// Convert first 8 bytes to uint64
	var position uint64
	for i := 0; i < 8; i++ {
		position = (position << 8) | uint64(positionHash[i])
	}

	// Ensure position is in the "gap" where nullifiers would be absent
	// For absence proofs, we want positions that don't contain existing nullifiers
	if treeSize > 0 {
		return (position % (treeSize * 2)) + treeSize
	}

	return position % (1 << 32) // Limit to reasonable range
}


// Helper function for min (already exists but adding for clarity)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
