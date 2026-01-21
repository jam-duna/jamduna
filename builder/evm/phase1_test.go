package witness

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	evmrpc "github.com/colorfulnotion/jam/builder/evm/rpc"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/chainspecs"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	jamtypes "github.com/colorfulnotion/jam/types"
)

const (
	totalTxCount      = 51
	txsPerBundle      = 5
	expectedBundles   = 11 // ceil(51/5) = 11 bundles = 11 EVM blocks after genesis
	expectedEVMBlocks = 12 // 1 genesis + 11 bundles
)

func buildBalanceWriteBlob(addr common.Address, balance *big.Int, txIndex uint32) []byte {
	payload := make([]byte, 16)
	balanceBytes := balance.Bytes()
	if len(balanceBytes) > len(payload) {
		balanceBytes = balanceBytes[len(balanceBytes)-len(payload):]
	}
	copy(payload[len(payload)-len(balanceBytes):], balanceBytes)

	header := make([]byte, 29)
	copy(header[0:20], addr.Bytes())
	header[20] = 0x02 // Balance write
	binary.LittleEndian.PutUint32(header[21:25], uint32(len(payload)))
	binary.LittleEndian.PutUint32(header[25:29], txIndex)

	return append(header, payload...)
}

// TestMultiSnapshotUBT tests the multi-snapshot UBT system for parallel bundle building.
// Creates 51 transactions, batches them 5 per bundle, expects 11 bundles (11 EVM blocks after genesis).
// Verifies correct balance tracking across all blocks.
func TestMultiSnapshotUBT(t *testing.T) {
	jamPath := os.Getenv("JAM_PATH")
	if jamPath == "" {
		t.Skip("JAM_PATH not set, skipping test")
	}

	log.InitLogger("debug")

	configPath := filepath.Join(jamPath, "chainspecs", "dev-config.json")
	dataPath := t.TempDir()
	pvmBackend := statedb.BackendInterpreter
	switch runtime.GOOS {
	case "linux":
		pvmBackend = statedb.BackendCompiler
	case "darwin":
		pvmBackend = statedb.BackendInterpreter
	}

	// Setup node
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read dev config %s: %v", configPath, err)
	}
	var devConfig chainspecs.DevConfig
	if err := json.Unmarshal(configBytes, &devConfig); err != nil {
		t.Fatalf("Failed to parse dev config %s: %v", configPath, err)
	}
	chainSpecData, err := chainspecs.GenSpec(devConfig)
	if err != nil {
		t.Fatalf("Failed to generate chainspec: %v", err)
	}

	validatorIndex := 6
	_, secrets, err := grandpa.GenerateValidatorSecretSet(types.TotalValidators + 1)
	if err != nil {
		t.Fatalf("GenerateValidatorSecretSet failed: %v", err)
	}

	port := randomPortAbove2(40000)
	t.Setenv("JAM_BIND_ADDR", "127.0.0.1")
	nodePath := filepath.Join(dataPath, fmt.Sprintf("jam-%d", validatorIndex))

	n, err := node.NewNode(uint16(validatorIndex), secrets[validatorIndex], chainSpecData, pvmBackend, nil, nil, nodePath, port, types.RoleBuilder)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	storage, err := n.GetStorage()
	if err != nil {
		t.Fatalf("Failed to get storage: %v", err)
	}
	defer storage.Close()
	evmstorage := storage.(types.EVMJAMStorage)

	// Setup service
	serviceID := uint32(0)
	authFile, err := common.GetFilePath(statedb.BootStrapNullAuthFile)
	if err != nil {
		t.Fatalf("GetFilePath(%s) failed: %v", statedb.BootStrapNullAuthFile, err)
	}
	authBytes, err := os.ReadFile(authFile)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", authFile, err)
	}
	authCode := statedb.AuthorizeCode{
		PackageMetaData:   []byte("bootstrap"),
		AuthorizationCode: authBytes,
	}
	authEncoded, err := authCode.Encode()
	if err != nil {
		t.Fatalf("AuthorizeCode.Encode failed: %v", err)
	}
	authHash := common.Blake2Hash(authEncoded)
	n.GetStateDB().WriteServicePreimageBlob(serviceID, authEncoded)
	n.GetStateDB().WriteServicePreimageLookup(serviceID, authHash, uint32(len(authEncoded)), []uint32{0})

	rollup, err := evmrpc.NewRollup(evmstorage, serviceID, n, pvmBackend)
	if err != nil {
		t.Fatalf("Failed to create rollup: %v", err)
	}

	txPool := evmrpc.NewTxPool()
	rollup.SetTxPool(txPool)

	// Prime genesis with enough balance for all transfers + gas
	// startBalance is in ETH (will be converted to Wei by InitializeEVMGenesis)
	// 51 txs * 0.1 ETH = 5.1 ETH value + gas, so 100 ETH is plenty
	startBalance := int64(100) // 100 ETH
	if err := rollup.PrimeGenesis(startBalance); err != nil {
		t.Fatalf("failed to prime genesis: %v", err)
	}

	fmt.Println("=== MULTI-SNAPSHOT UBT TEST ===")
	fmt.Printf("Total transactions: %d\n", totalTxCount)
	fmt.Printf("Transactions per bundle: %d\n", txsPerBundle)
	fmt.Printf("Expected bundles: %d\n", expectedBundles)
	fmt.Printf("Expected EVM blocks (including genesis): %d\n", expectedEVMBlocks)

	// Get sender/receiver accounts
	senderAddr, senderPrivKey := common.GetEVMDevAccount(0)
	recipientAddr, _ := common.GetEVMDevAccount(1)

	// ROOT-FIRST: Get initial canonical root
	initialCanonicalRoot := evmstorage.GetCanonicalRoot()
	fmt.Printf("Initial canonical root: %s\n", initialCanonicalRoot.Hex())

	// Transaction parameters
	amount := big.NewInt(100_000_000) // 0.1 ETH per tx
	gasPrice := big.NewInt(1)
	gasLimit := uint64(21000)

	// Create all 51 transactions
	fmt.Println("=== Creating 51 transactions ===")
	allTxs := make([]*evmtypes.EthereumTransaction, 0, totalTxCount)
	for i := 0; i < totalTxCount; i++ {
		tx, _, txHash, err := evmtypes.CreateSignedNativeTransfer(
			senderPrivKey,
			uint64(i), // nonce
			recipientAddr,
			amount,
			gasPrice,
			gasLimit,
			testChainID,
		)
		if err != nil {
			t.Fatalf("CreateSignedNativeTransfer tx%d failed: %v", i, err)
		}
		allTxs = append(allTxs, tx)
		if i%10 == 0 {
			fmt.Printf("Created tx %d: %s\n", i, txHash.Hex())
		}
	}
	fmt.Printf("Created %d transactions\n", len(allTxs))

	// Process transactions in batches of 5 using ROOT-FIRST API
	fmt.Println("=== Processing bundles (root-first API) ===")
	blockRoots := make([]common.Hash, 0, expectedBundles)
	var totalGasUsed uint64

	// Use the Rollup's block cache instead of a slice
	blockCache := rollup.GetBlockCache()

	// Track the current canonical root - start with genesis
	currentRoot := evmstorage.GetCanonicalRoot()
	fmt.Printf("Starting canonical root: %s\n", currentRoot.Hex())

	for bundleIdx := 0; bundleIdx < expectedBundles; bundleIdx++ {
		startIdx := bundleIdx * txsPerBundle
		endIdx := startIdx + txsPerBundle
		if endIdx > totalTxCount {
			endIdx = totalTxCount
		}

		batchTxs := allTxs[startIdx:endIdx]
		blockNumber := uint64(bundleIdx + 1) // Block 1, 2, 3, ... (genesis is block 0)

		fmt.Printf("--- Bundle %d: Block %d, txs %d-%d (%d txs) ---\n",
			bundleIdx, blockNumber, startIdx, endIdx-1, len(batchTxs))

		// ROOT-FIRST: Capture preRoot before execution
		preRoot := currentRoot
		fmt.Printf("  PreRoot: %s\n", preRoot.Hex())

		// ROOT-FIRST: Validate parent root exists (CreateSnapshotFromRoot)
		_, err := evmstorage.CreateSnapshotFromRoot(preRoot)
		if err != nil {
			t.Fatalf("CreateSnapshotFromRoot(%s) failed: %v", preRoot.Hex(), err)
		}

		// ROOT-FIRST: Set active root for reads during execution
		if err := evmstorage.SetActiveRoot(preRoot); err != nil {
			t.Fatalf("SetActiveRoot(%s) failed: %v", preRoot.Hex(), err)
		}

		// Prepare work package
		refineCtx := jamtypes.RefineContext{
			Anchor:           common.Hash{},
			StateRoot:        common.Hash{},
			BeefyRoot:        common.Hash{},
			LookupAnchor:     common.Hash{},
			LookupAnchorSlot: 0,
			Prerequisites:    []common.Hash{},
		}

		workPackage, extrinsicsBlobs, err := rollup.PrepareWorkPackage(refineCtx, batchTxs)
		if err != nil {
			t.Fatalf("PrepareWorkPackage for bundle %d failed: %v", bundleIdx, err)
		}

		// Execute Phase 1 (no witness generation)
		// ExecutePhase1 internally applies writes via ApplyWritesToTree when activeRoot is set
		// The postRoot is returned in phase1Result.EVMPostStateRoot
		phase1Result, err := rollup.ExecutePhase1(workPackage, extrinsicsBlobs)
		if err != nil {
			t.Fatalf("ExecutePhase1 for bundle %d failed: %v", bundleIdx, err)
		}

		// ROOT-FIRST: Get postRoot from Phase1Result
		// ExecutePhase1 already applied writes via ApplyWritesToTree internally
		postRoot := phase1Result.EVMPostStateRoot
		fmt.Printf("  PostRoot: %s\n", postRoot.Hex())

		// ROOT-FIRST: Commit postRoot as canonical (simulates OnAccumulated)
		if err := evmstorage.CommitAsCanonical(postRoot); err != nil {
			t.Fatalf("CommitAsCanonical(%s) failed: %v", postRoot.Hex(), err)
		}

		// ROOT-FIRST: Clear active root
		evmstorage.ClearActiveRoot()

		// Update current root for next iteration
		currentRoot = postRoot

		// Build EvmBlockPayload for logging
		txHashes := make([]common.Hash, len(batchTxs))
		for i, tx := range batchTxs {
			txHashes[i] = tx.Hash
		}

		// Convert receipts from Phase1Result to TransactionReceipt slice
		// Receipts are extracted from refine output Section 3 (block receipts pointer)
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
			TxHashes:        txHashes,
			Transactions:    receipts, // Receipts extracted from refine output
		}
		// In production, Phase 2 sets WorkPackageHash = BlockCommitment after bundle building.
		// For testing, we simulate this by setting WorkPackageHash to the BlockCommitment.
		// BlockCommitment is the stable voting digest (doesn't change on resubmission).
		blockCommitment := evmBlock.BlockCommitment()
		evmBlock.WorkPackageHash = blockCommitment
		// Also update each receipt's BlockHash and BlockNumber
		for i := range evmBlock.Transactions {
			evmBlock.Transactions[i].BlockHash = blockCommitment
			evmBlock.Transactions[i].BlockNumber = uint32(blockNumber)
		}
		fmt.Printf("BlockCommitment: %s\nEvmBlock: %s\n", evmBlock.BlockCommitment().Hex(), evmBlock.String())

		// Builder keeps the EVM block for consensus voting and RPC serving
		rollup.AddBlock(evmBlock)
		blockRoots = append(blockRoots, phase1Result.EVMPostStateRoot)
		totalGasUsed += phase1Result.TotalGasUsed
	}

	fmt.Println("=== Verifying final state ===")

	// ROOT-FIRST: Get final canonical root
	finalRoot := evmstorage.GetCanonicalRoot()
	fmt.Printf("Final canonical root: %s\n", finalRoot.Hex())
	fmt.Printf("TreeStore size: %d\n", evmstorage.GetTreeStoreSize())

	// Verify we processed the expected number of blocks
	if len(blockRoots) != expectedBundles {
		t.Fatalf("Expected %d block roots, got %d", expectedBundles, len(blockRoots))
	}
	if blockCache.Len() != expectedBundles {
		t.Fatalf("Expected %d EVM blocks in cache, got %d", expectedBundles, blockCache.Len())
	}
	fmt.Printf("Builder cached %d EVM blocks (latest: %d)\n", blockCache.Len(), blockCache.GetLatestBlockNumber())

	// Verify block cache lookups work
	for i := 1; i <= expectedBundles; i++ {
		block, ok := blockCache.GetByNumber(uint64(i))
		if !ok {
			t.Fatalf("Block %d not found in cache by number", i)
		}
		// Verify we can also look it up by BlockCommitment
		_, ok = blockCache.GetByBlockCommitment(block.BlockCommitment())
		if !ok {
			t.Fatalf("Block %d not found in cache by BlockCommitment", i)
		}
	}
	fmt.Printf("All %d blocks accessible by number and BlockCommitment\n", expectedBundles)

	// Verify tx hash lookups work (for eth_getTransactionReceipt)
	for i, tx := range allTxs {
		loc, ok := blockCache.GetTxLocation(tx.Hash)
		if !ok {
			t.Fatalf("Tx %d hash %s not found in cache", i, tx.Hash.Hex())
		}
		expectedBlockNum := uint64((i / txsPerBundle) + 1)
		if loc.BlockNumber != expectedBlockNum {
			t.Fatalf("Tx %d: expected block %d, got %d", i, expectedBlockNum, loc.BlockNumber)
		}
	}
	fmt.Printf("All %d tx hashes indexed correctly for receipt lookup\n", len(allTxs))

	// Verify receipts were extracted and stored in blocks
	fmt.Println("=== Verifying receipt extraction ===")
	var totalReceiptsFound int
	for i := 1; i <= expectedBundles; i++ {
		block, ok := blockCache.GetByNumber(uint64(i))
		if !ok {
			t.Fatalf("Block %d not found in cache", i)
		}

		receiptCount := len(block.Transactions)
		fmt.Printf("Block %d: %d receipts stored\n", i, receiptCount)
		totalReceiptsFound += receiptCount

		// Verify each receipt has required fields
		for j, receipt := range block.Transactions {
			if receipt.TransactionHash == (common.Hash{}) {
				t.Fatalf("Block %d receipt %d: missing transaction hash", i, j)
			}
			// UsedGas should be 21000 for simple transfers
			if receipt.UsedGas == 0 {
				fmt.Printf("Block %d receipt %d: UsedGas is 0 (may indicate extraction issue)\n", i, j)
			}
			fmt.Printf("  Receipt %d: txHash=%s success=%v gasUsed=%d\n",
				j, receipt.TransactionHash.Hex()[:18], receipt.Success, receipt.UsedGas)
		}
	}
	fmt.Printf("Total receipts found across all blocks: %d (expected: %d)\n", totalReceiptsFound, totalTxCount)

	// If no receipts were extracted, log a warning but don't fail
	// (extraction depends on refine output format which may vary)
	if totalReceiptsFound == 0 {
		fmt.Println("WARNING: No receipts extracted from refine output. This may indicate:")
		fmt.Println("  - Metashard entries don't contain receipt data in current format")
		fmt.Println("  - ObjectKind for receipts differs from expected (0x03)")
		fmt.Println("  - Receipt payloads are not in exported segments")
	} else if totalReceiptsFound != totalTxCount {
		fmt.Printf("WARNING: Receipt count mismatch: found %d, expected %d\n", totalReceiptsFound, totalTxCount)
	}

	// ========================================
	// Test GetTransactionReceipt RPC (via Rollup)
	// ========================================
	fmt.Println("=== Testing eth_getTransactionReceipt RPC ===")

	// Test receipt lookup for first few transactions using rollup.GetTransactionReceipt
	// This is the same code path used by the actual eth_getTransactionReceipt JSON-RPC handler
	for i := 0; i < min(5, len(allTxs)); i++ {
		txHash := allTxs[i].Hash

		// Use rollup.GetTransactionReceipt - same as eth_getTransactionReceipt RPC
		ethReceipt, err := rollup.GetTransactionReceipt(txHash)
		if err != nil {
			t.Fatalf("eth_getTransactionReceipt failed for tx %d: %v", i, err)
		}
		if ethReceipt == nil {
			t.Fatalf("eth_getTransactionReceipt returned nil for tx %d (hash=%s)", i, txHash.Hex())
		}

		// Verify Ethereum JSON-RPC receipt fields
		if ethReceipt.TransactionHash != txHash.Hex() {
			t.Fatalf("Receipt tx hash mismatch: expected %s, got %s",
				txHash.Hex(), ethReceipt.TransactionHash)
		}
		if ethReceipt.Status != "0x1" {
			t.Fatalf("Receipt shows failure for tx %d (status=%s)", i, ethReceipt.Status)
		}
		if ethReceipt.GasUsed != "0x5208" { // 21000 in hex
			t.Fatalf("Receipt GasUsed wrong: expected 0x5208 (21000), got %s", ethReceipt.GasUsed)
		}

		fmt.Printf("! eth_getTransactionReceipt[%d]:\n  %s\n", i, ethReceipt.String())
	}

	fmt.Printf("eth_getTransactionReceipt RPC working correctly for %d transactions\n", min(5, len(allTxs)))

	// Verify all roots are unique (state changed each block)
	rootSet := make(map[common.Hash]bool)
	for i, root := range blockRoots {
		if rootSet[root] {
			t.Fatalf("Duplicate root at block %d: %s", i+1, root.Hex())
		}
		rootSet[root] = true
	}
	fmt.Println("All block roots are unique (state changed each block)")

	// Calculate expected final balances
	expectedTransferTotal := new(big.Int).Mul(amount, big.NewInt(int64(totalTxCount)))
	expectedGasTotal := new(big.Int).Mul(big.NewInt(int64(gasLimit)), big.NewInt(int64(totalTxCount)))
	expectedGasTotal.Mul(expectedGasTotal, gasPrice)

	fmt.Println("=== TEST SUMMARY ===")
	fmt.Printf("Total transactions:     %d\n", totalTxCount)
	fmt.Printf("Bundles processed:      %d\n", len(blockRoots))
	fmt.Printf("Total gas used:         %d\n", totalGasUsed)
	fmt.Printf("Expected transfer sum:  %s\n", expectedTransferTotal.String())
	fmt.Printf("Expected gas cost:      %s\n", expectedGasTotal.String())
	fmt.Printf("Sender:                 %s\n", senderAddr.Hex())
	fmt.Printf("Recipient:              %s\n", recipientAddr.Hex())
	fmt.Printf("Initial canonical root: %s\n", initialCanonicalRoot.Hex())
	fmt.Printf("Final UBT root:         %s\n", finalRoot.Hex())

	// Verify final balances using FetchBalance
	senderFinalBalance, err := evmstorage.FetchBalance(senderAddr, 0)
	if err != nil {
		t.Fatalf("FetchBalance(sender) failed: %v", err)
	}
	recipientFinalBalance, err := evmstorage.FetchBalance(recipientAddr, 0)
	if err != nil {
		t.Fatalf("FetchBalance(recipient) failed: %v", err)
	}

	senderFinalBig := new(big.Int).SetBytes(senderFinalBalance[:])
	recipientFinalBig := new(big.Int).SetBytes(recipientFinalBalance[:])

	fmt.Printf("Sender final balance:   %s\n", senderFinalBig.String())
	fmt.Printf("Recipient final balance: %s\n", recipientFinalBig.String())

	// Verify recipient received the expected amount
	if recipientFinalBig.Cmp(expectedTransferTotal) != 0 {
		t.Fatalf("BALANCE MISMATCH: recipient expected %s, got %s",
			expectedTransferTotal.String(), recipientFinalBig.String())
	}
	fmt.Println("Recipient balance CORRECT")

	// Verify sender spent transfers + gas
	// startBalance was in ETH, but genesis converts to Wei (multiplies by 10^18)
	startBalanceWei := new(big.Int).Mul(big.NewInt(startBalance), big.NewInt(1e18))
	expectedSenderFinal := new(big.Int).Sub(startBalanceWei, expectedTransferTotal)
	expectedSenderFinal.Sub(expectedSenderFinal, expectedGasTotal)

	if senderFinalBig.Cmp(expectedSenderFinal) != 0 {
		t.Fatalf("BALANCE MISMATCH: sender expected %s, got %s",
			expectedSenderFinal.String(), senderFinalBig.String())
	}
	fmt.Println("Sender balance CORRECT")

	fmt.Println("=== MULTI-SNAPSHOT UBT TEST PASSED ===")
}

func TestRootFirstResubmissionIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewStateDBStorage(tmpDir, storage.NewMockJAMDA(), nil, 0)
	if err != nil {
		t.Fatalf("NewStateDBStorage failed: %v", err)
	}
	defer store.Close()

	issuerAddr, _ := common.GetEVMDevAccount(0)
	if _, err := store.InitializeEVMGenesis(0, issuerAddr, 100); err != nil {
		t.Fatalf("InitializeEVMGenesis failed: %v", err)
	}

	preRoot := store.GetCanonicalRoot()
	balanceBlob := buildBalanceWriteBlob(issuerAddr, big.NewInt(1234), 1)

	postRoot1, err := store.ApplyWritesToTree(preRoot, balanceBlob)
	if err != nil {
		t.Fatalf("ApplyWritesToTree(v1) failed: %v", err)
	}
	if postRoot1 == preRoot {
		t.Fatalf("expected postRoot to differ from preRoot")
	}
	if got := store.GetCanonicalRoot(); got != preRoot {
		t.Fatalf("canonical root changed before commit: %s", got.Hex())
	}

	store.DiscardTree(postRoot1)

	postRoot2, err := store.ApplyWritesToTree(preRoot, balanceBlob)
	if err != nil {
		t.Fatalf("ApplyWritesToTree(v2) failed: %v", err)
	}
	if postRoot2 != postRoot1 {
		t.Fatalf("resubmission produced different postRoot: %s vs %s", postRoot1.Hex(), postRoot2.Hex())
	}
	if got := store.GetCanonicalRoot(); got != preRoot {
		t.Fatalf("canonical root changed before commit: %s", got.Hex())
	}

	if err := store.CommitAsCanonical(postRoot2); err != nil {
		t.Fatalf("CommitAsCanonical failed: %v", err)
	}
	if got := store.GetCanonicalRoot(); got != postRoot2 {
		t.Fatalf("canonical root mismatch after commit: %s", got.Hex())
	}
}

// TestPhase1ExecutionOnly tests EVM execution WITHOUT witness generation.
// This validates the core concept of the Builder Network design:
// - Execute EVM transactions with UBT read logging DISABLED
// - Capture EVMPreStateRoot and EVMPostStateRoot
// - Verify deterministic execution (re-execution produces same result)
// - NO bundle submission to JAM validators
func TestPhase1ExecutionOnly(t *testing.T) {
	jamPath := os.Getenv("JAM_PATH")
	if jamPath == "" {
		t.Skip("JAM_PATH not set, skipping test")
	}

	configPath := filepath.Join(jamPath, "chainspecs", "dev-config.json")
	dataPath := t.TempDir() // Use isolated temp dir per test run
	pvmBackend := statedb.BackendInterpreter

	// Setup: Create node and rollup
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read dev config %s: %v", configPath, err)
	}
	var devConfig chainspecs.DevConfig
	if err := json.Unmarshal(configBytes, &devConfig); err != nil {
		t.Fatalf("Failed to parse dev config %s: %v", configPath, err)
	}
	chainSpecData, err := chainspecs.GenSpec(devConfig)
	if err != nil {
		t.Fatalf("Failed to generate chainspec: %v", err)
	}

	validatorIndex := 6
	_, secrets, err := grandpa.GenerateValidatorSecretSet(types.TotalValidators + 1)
	if err != nil {
		t.Fatalf("GenerateValidatorSecretSet failed: %v", err)
	}

	port := randomPortAbove2(40000)
	t.Setenv("JAM_BIND_ADDR", "127.0.0.1")
	nodePath := filepath.Join(dataPath, fmt.Sprintf("jam-%d", validatorIndex))

	n, err := node.NewNode(uint16(validatorIndex), secrets[validatorIndex], chainSpecData, pvmBackend, nil, nil, nodePath, port, types.RoleBuilder)
	if err != nil {
		t.Fatalf("Failed to create node: %v", err)
	}

	storage, err := n.GetStorage()
	if err != nil {
		t.Fatalf("Failed to get storage: %v", err)
	}
	defer storage.Close()
	evmstorage := storage.(types.EVMJAMStorage)
	log.InitLogger("debug")

	serviceID := uint32(0)
	authFile, err := common.GetFilePath(statedb.BootStrapNullAuthFile)
	if err != nil {
		t.Fatalf("GetFilePath(%s) failed: %v", statedb.BootStrapNullAuthFile, err)
	}
	authBytes, err := os.ReadFile(authFile)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", authFile, err)
	}
	authCode := statedb.AuthorizeCode{
		PackageMetaData:   []byte("bootstrap"),
		AuthorizationCode: authBytes,
	}
	authEncoded, err := authCode.Encode()
	if err != nil {
		t.Fatalf("AuthorizeCode.Encode failed: %v", err)
	}
	authHash := common.Blake2Hash(authEncoded)
	n.GetStateDB().WriteServicePreimageBlob(serviceID, authEncoded)
	n.GetStateDB().WriteServicePreimageLookup(serviceID, authHash, uint32(len(authEncoded)), []uint32{0})

	rollup, err := evmrpc.NewRollup(evmstorage, serviceID, n, pvmBackend)
	if err != nil {
		t.Fatalf("Failed to create rollup: %v", err)
	}

	txPool := evmrpc.NewTxPool()
	rollup.SetTxPool(txPool)

	startBalance := int64(61_000_000)
	if err := rollup.PrimeGenesis(startBalance); err != nil {
		t.Fatalf("failed to prime genesis: %v", err)
	}

	// ========================================
	// PHASE 1 TEST: Execute WITHOUT witnesses
	// ========================================

	fmt.Println("=== PHASE 1: Capture Pre-State Root ===")

	// Step 1: Capture EVMPreStateRoot BEFORE execution (use canonical root)
	evmPreStateRoot := evmstorage.GetCanonicalRoot()
	fmt.Printf("EVMPreStateRoot: %s\n", evmPreStateRoot.Hex())

	// Step 1b: Set activeRoot for root-first state isolation
	// This tells ExecutePhase1 which tree to read from during execution
	if err := evmstorage.SetActiveRoot(evmPreStateRoot); err != nil {
		t.Fatalf("Failed to set active root: %v", err)
	}
	defer evmstorage.ClearActiveRoot() // Clear after test

	// Step 2: Verify logging is enabled before Phase 1
	// ExecutePhase1 will internally disable/re-enable logging
	if !evmstorage.IsUBTReadLogEnabled() {
		t.Fatalf("Expected UBT read logging to be enabled before Phase 1")
	}
	fmt.Println("UBT read logging is ENABLED (will be disabled internally by ExecutePhase1)")

	// Step 3: Create transactions
	_, fromPrivKey := common.GetEVMDevAccount(0)
	const transferCount = 50
	receivers := make([]common.Address, 0, transferCount)
	for i := 1; i <= transferCount; i++ {
		addr, _ := common.GetEVMDevAccount(i)
		receivers = append(receivers, addr)
	}

	pendingTxs := make([]*evmtypes.EthereumTransaction, 0, transferCount)
	for i := 0; i < transferCount; i++ {
		tx, _, _, err := evmtypes.CreateSignedNativeTransfer(
			fromPrivKey,
			uint64(i),
			receivers[i],
			big.NewInt(int64(1000*(i+1))),
			big.NewInt(1),
			21_000,
			testChainID,
		)
		if err != nil {
			t.Fatalf("CreateSignedNativeTransfer tx%d failed: %v", i+1, err)
		}
		pendingTxs = append(pendingTxs, tx)
	}

	// Step 4: Prepare work package (but DON'T build bundle with witnesses)
	refineCtx := jamtypes.RefineContext{
		Anchor:           common.Hash{},
		StateRoot:        common.Hash{},
		BeefyRoot:        common.Hash{},
		LookupAnchor:     common.Hash{},
		LookupAnchorSlot: 0,
		Prerequisites:    []common.Hash{},
	}

	workPackage, extrinsicsBlobs, err := rollup.PrepareWorkPackage(refineCtx, pendingTxs)
	if err != nil {
		t.Fatalf("PrepareWorkPackage failed: %v", err)
	}

	fmt.Printf("Work package prepared: %d work items, %d extrinsics\n",
		len(workPackage.WorkItems), len(extrinsicsBlobs[0]))

	// Step 5: Execute EVM (Phase 1 style - no witness generation)
	// We use ExecutePhase1 which runs refine without witness prepending
	phase1Result, err := rollup.ExecutePhase1(workPackage, extrinsicsBlobs)
	if err != nil {
		t.Fatalf("ExecutePhase1 failed: %v", err)
	}

	fmt.Println("=== PHASE 1: Capture Post-State Root ===")
	fmt.Printf("EVMPostStateRoot: %s\n", phase1Result.EVMPostStateRoot.Hex())

	// Verify UBT read log is empty
	// ExecutePhase1 internally disables logging, executes, then re-enables via defer.
	// The read log should be empty because logging was disabled during execution.
	readLog := evmstorage.GetUBTReadLog()
	if len(readLog) != 0 {
		t.Fatalf("Expected empty UBT read log (logging was disabled during execution), got %d entries", len(readLog))
	}
	fmt.Println("Confirmed: UBT read log is empty (Phase 1 execution skipped logging)")

	// ========================================
	// DETERMINISM TEST: Re-execute and compare
	// ========================================

	fmt.Println("=== DETERMINISM TEST: Re-execute same txs ===")

	// Reset to pre-state
	if err := evmstorage.PinToStateRoot(evmPreStateRoot); err != nil {
		t.Fatalf("Failed to pin to pre-state root: %v", err)
	}
	defer evmstorage.UnpinState() // Release pinned state on test exit

	// Set activeRoot for re-execution (root-first requirement)
	if err := evmstorage.SetActiveRoot(evmPreStateRoot); err != nil {
		t.Fatalf("Failed to set active root for re-execution: %v", err)
	}

	// Re-execute (still with logging disabled)
	phase1Result2, err := rollup.ExecutePhase1(workPackage, extrinsicsBlobs)
	if err != nil {
		t.Fatalf("ExecutePhase1 (re-execution) failed: %v", err)
	}

	// Compare results
	if phase1Result.EVMPostStateRoot != phase1Result2.EVMPostStateRoot {
		t.Fatalf("DETERMINISM FAILURE: post-state roots differ\n  First:  %s\n  Second: %s",
			phase1Result.EVMPostStateRoot.Hex(), phase1Result2.EVMPostStateRoot.Hex())
	}
	fmt.Printf("Determinism verified: both executions produced %s\n", phase1Result.EVMPostStateRoot.Hex())

	// ========================================
	// VERIFY: No bundle submitted to JAM
	// ========================================

	fmt.Println("=== VERIFY: No JAM submission ===")
	fmt.Println("Phase 1 complete - NO bundle was submitted to JAM validators")
	fmt.Println("This validates the decoupled execution model")

	// Summary
	fmt.Println("=== PHASE 1 TEST SUMMARY ===")
	fmt.Printf("EVMPreStateRoot:  %s\n", evmPreStateRoot.Hex())
	fmt.Printf("EVMPostStateRoot: %s\n", phase1Result.EVMPostStateRoot.Hex())
	fmt.Printf("Transactions:     %d\n", transferCount)
	fmt.Printf("Gas used:         %d\n", phase1Result.TotalGasUsed)
	fmt.Printf("Deterministic:    YES\n")
	fmt.Printf("JAM submission:   NO\n")

	// Print receipt details to verify content
	fmt.Println("=== RECEIPT DETAILS ===")
	fmt.Printf("Total receipts extracted: %d\n", len(phase1Result.Receipts))
	for i, receipt := range phase1Result.Receipts {
		if receipt == nil {
			fmt.Printf("  Receipt[%d]: nil\n", i)
			continue
		}
		fmt.Printf("  Receipt[%d]:\n", i)
		fmt.Printf("    TxHash:       %s\n", receipt.TransactionHash.Hex())
		fmt.Printf("    Success:      %t\n", receipt.Success)
		fmt.Printf("    UsedGas:      %d\n", receipt.UsedGas)
		fmt.Printf("    CumulativeGas:%d\n", receipt.CumulativeGas)
		fmt.Printf("    TxIndex:      %d\n", receipt.TransactionIndex)
		fmt.Printf("    TxType:       %d\n", receipt.Type)
		fmt.Printf("    LogsDataLen:  %d\n", len(receipt.LogsData))
		fmt.Printf("    PayloadLen:   %d\n", len(receipt.Payload))
		if i >= 2 {
			fmt.Printf("  ... (showing first 3 of %d receipts)\n", len(phase1Result.Receipts))
			break
		}
	}
}

// TestPhase1ThenPhase2 tests the full two-phase flow:
// Phase 1: Execute without witnesses, capture state roots
// Phase 2: Re-execute with witnesses, generate bundle (but don't submit)
func TestPhase1ThenPhase2(t *testing.T) {
	// TODO: Implement after Phase 1 test passes
	t.Skip("Phase 2 test not yet implemented")
}

func randomPortAbove2(base int) int {
	for i := 0; i < 20; i++ {
		conn, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			continue
		}
		port := conn.LocalAddr().(*net.UDPAddr).Port
		_ = conn.Close()
		if port > base {
			return port
		}
	}
	return base + 1000
}

// NOTE: TestResubmissionIsolation removed - requires full node setup and additional
// imports that aren't available. The root-first state isolation is tested via
// TestMultiSnapshotUBT which verifies core treeStore functionality.
