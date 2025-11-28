package statedb

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/types"
)

const (
	numBlocks = 5
)

var algoPayloads = []byte{
	0xc2, 0xb8, 0xb4, 0xbb, 0xcb, 0xaa, 0x47, 0xd4, 0xe9, 0xdc, 0x39, 0xce, 0xb8, 0xbc, 0x75, 0x2b, 0x2b, 0x6b, 0x8c, 0x98, 0x88, 0xab, 0xb4, 0xc4, 0x9c, 0x59, 0xc2, 0xcb, 0xbd, 0xa2, 0x96, 0x94, 0xb1, 0x4d, 0xb6, 0xb7, 0xbc, 0x78, 0x72, 0x96, 0x85, 0x0a, 0xa7, 0x0d, 0x77, 0xb6, 0x02, 0xb1, 0xb3, 0xb4, 0xbd, 0xb7, 0xcc, 0xf5,
}

// TestAlgoBlocks generates a sequence of blocks with Algo service guarantees+assurances without any jamnp
func TestAlgoBlocks(t *testing.T) {
	log.InitLogger("info")
	log.EnableModule(log.Node)
	c, err := NewRollup(t.TempDir(), AlgoServiceCode)
	if err != nil {
		t.Fatalf("NewEVMService failed: %v", err)
	}

	auth_payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(auth_payload, uint32(AuthCopyServiceCode))
	service, ok, err := c.stateDB.GetService(c.serviceID)
	if err != nil || !ok {
		t.Fatalf("GetService failed: %v", err)
	}
	bundles := make([]*types.WorkPackageBundle, types.TotalCores)
	for n := 1; n <= numBlocks; n++ {
		wp := DefaultWorkPackage(c.serviceID, service)
		wp.RefineContext = c.stateDB.GetRefineContext()
		wp.WorkItems[0].Payload = GenerateAlgoPayload(n, false)
		wp.WorkItems[0].RefineGasLimit = types.RefineGasAllocation / 2
		wp.WorkItems[0].ExportCount = uint16(n)
		bundles[0] = &types.WorkPackageBundle{
			WorkPackage:   wp,
			ExtrinsicData: []types.ExtrinsicsBlobs{{}},
		}
		// Process all cores work packages
		err := c.processWorkPackageBundlesPipelined(bundles)
		if err != nil {
			t.Fatal(err)
		}
		// Advance timeslot for next block
		c.stateDB.JamState.SafroleState.Timeslot++

		// Log Algo processing info (Algo service doesn't have EVM-style blocks)
		k := common.ServiceStorageKey(c.serviceID, []byte{0})
		data, _, _ := c.storage.GetServiceStorage(c.serviceID, k)

		log.Info(log.Node, "Algo round processed", "round", n, "service", c.serviceID, "result", fmt.Sprintf("%x", data))
	}

}

func TestEVMBlocksMath(t *testing.T) {
	chain, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewEVMService failed: %v", err)
	}
	_, err = chain.SubmitEVMGenesis(61_000_000)
	if err != nil {
		t.Fatalf("SubmitEVMGenesis failed: %v", err)
	}
	txBytesMulticore := make([][][]byte, types.TotalCores)

	for i := 0; i <= 3; i++ {
		txBytes, _, err := chain.CallMath(evmtypes.MathAddress, []string{
			"fibonacci(256)",
			"fact(6)",
			"fact(7)",
			"nextPrime(100)",
			"integerSqrt(1000)",
			"gcd(48,18)",
			// "modExp(5,3,13)",
			// "isPrime(97)",
			// "jacobi(1001,9907)",
			// "binomial(5,2)",
			// "isQuadraticResidue(10,13)",
			// "rsaKeygen(512)",
			// "burnsideNecklace(5,3)",
			// "fermatFactor(5959)",
			// "narayana(4,2)",
			// "youngTableaux(4,2)",
		})
		if err != nil {
			t.Fatalf("CallMath failed: %v", err)
		}
		txBytesMulticore[0] = txBytes
		_, err = chain.SubmitEVMTransactions(txBytesMulticore)
		if err != nil {
			t.Fatalf("SubmitEVMTransactions failed: %v", err)
		}
		// log.Info(log.Node, "EVM block processed", "blockNumber", block.Number)
		// err = chain.ShowTxReceipts(block, block.TxHashes, "Math Transactions", alltopics)
		// if err != nil {
		// 	t.Fatalf("ShowTxReceipts failed: %v", err)
		// }

	}
}

func RunTransfersRound(b *Rollup, transfers []TransferTriple) error {
	const numAccounts = 11 // 10 dev accounts + 1 coinbase

	// Coinbase address receives all transaction fees
	coinbaseAddress := common.HexToAddress("0xEaf3223589Ed19bcd171875AC1D0F99D31A5969c")

	// Get all account addresses
	accounts := make([]common.Address, numAccounts)
	for i := 0; i < 10; i++ {
		accounts[i], _ = common.GetEVMDevAccount(i)
	}
	accounts[10] = coinbaseAddress

	// Track initial balances and nonces for all accounts
	initialBalances := make([]*big.Int, numAccounts)
	initialNonces := make([]uint64, numAccounts)

	totalBefore := big.NewInt(0)
	for i := 0; i < numAccounts; i++ {
		balanceHash, err := b.stateDB.GetBalance(b.serviceID, accounts[i])
		if err != nil {
			log.Error(log.Node, "GetBalance ERR", "account", i, "err", err)
			return err
		}
		balance := new(big.Int).SetBytes(balanceHash.Bytes())
		initialBalances[i] = balance
		totalBefore = new(big.Int).Add(totalBefore, balance)

		nonce, err := b.stateDB.GetTransactionCount(b.serviceID, accounts[i])
		if err != nil {
			log.Error(log.Node, "GetTransactionCount ERR", "account", i, "err", err)
			return err
		}
		initialNonces[i] = nonce
		//fmt.Printf("Account[%d] address=%s balance=%s nonce=%d\n", i, accounts[i].String(), balance.String(), nonce)
	}
	// Build transactions for all transfers
	txBytesMulticore := make([][][]byte, types.TotalCores)
	txBytes := make([][]byte, len(transfers))

	// Track nonce per sender
	senderNonces := make(map[int]uint64)
	for _, transfer := range transfers {
		if _, exists := senderNonces[transfer.SenderIndex]; !exists {
			senderNonces[transfer.SenderIndex] = initialNonces[transfer.SenderIndex]
		}
	}

	txHashes := make([]common.Hash, len(transfers))
	// TODO: continue to add transfers to a core until it hits the work report limit of # of shard commitments
	for idx, transfer := range transfers {
		senderAddr, senderPrivKey := common.GetEVMDevAccount(transfer.SenderIndex)
		recipientAddr, _ := common.GetEVMDevAccount(transfer.ReceiverIndex)

		gasPrice := big.NewInt(1) // 1 wei
		gasLimit := uint64(2_000_000)

		currentNonce := senderNonces[transfer.SenderIndex]
		senderNonces[transfer.SenderIndex]++

		_, tx, txHash, err := CreateSignedUSDMTransfer(
			senderPrivKey,
			currentNonce,
			recipientAddr,
			transfer.Amount,
			gasPrice,
			gasLimit,
			uint64(b.serviceID),
		)
		if err != nil {
			log.Error(log.Node, "CreateSignedUSDMTransfer ERR", "idx", idx, "err", err)
			return err
		}

		log.Info(log.Node, "📤 Transfer created",
			"idx", idx,
			"from", fmt.Sprintf("Account[%d](%s)", transfer.SenderIndex, senderAddr.String()),
			"to", fmt.Sprintf("Account[%d](%s)", transfer.ReceiverIndex, recipientAddr.String()),
			"amount", transfer.Amount.String(),
			"nonce", currentNonce,
			"txHash", txHash.String())

		txBytes[idx] = tx
		txHashes[idx] = txHash
	}

	// Assign transactions to core 0

	txBytesMulticore[0] = txBytes
	txBytesMulticore[1] = nil

	// Submit multitransfer as work package
	var err error
	// Subsequent rounds: witness the previous block
	_, err = b.SubmitEVMTransactions(txBytesMulticore)
	if err != nil {
		log.Error(log.Node, "SubmitEVMTransactions ERR", "err", err)
		return err
	}

	// if err := evmtypes.VerifyBlockBMTProofs(prevblock, metadata); err != nil {
	// 	panic("VerifyBlockBMTProofs failed: " + err.Error())
	// }
	// if err := b.ShowTxReceipts(block, txHashes, fmt.Sprintf("Multitransfer (%d transfers)", len(transfers)), defaultTopics()); err != nil {
	// 	return fmt.Errorf("failed to show transfer receipts: %w", err)
	// }

	// Check balances and nonces after transfers
	totalAfter := big.NewInt(0)
	for i := 0; i < numAccounts; i++ {
		balanceHash, err := b.stateDB.GetBalance(b.serviceID, accounts[i])
		if err != nil {
			log.Error(log.Node, "GetBalance ERR", "account", i, "err", err)
			return err
		}
		balance := new(big.Int).SetBytes(balanceHash.Bytes())
		totalAfter = new(big.Int).Add(totalAfter, balance)

		nonce, err := b.stateDB.GetTransactionCount(b.serviceID, accounts[i])
		if err != nil {
			log.Error(log.Node, "GetTransactionCount ERR", "account", i, "err", err)
			return err
		}

		delta := new(big.Int).Sub(balance, initialBalances[i])
		deltaSign := ""
		if delta.Sign() > 0 {
			deltaSign = "+"
		}

		accountName := fmt.Sprintf("Account[%d]", i)
		switch i {
		case 0:
			accountName = "Account[0] (Issuer)"
		case 10:
			accountName = "Coinbase (Fee Recipient)"
		}
		log.Info(log.Node, fmt.Sprintf("  %s", accountName),
			"address", accounts[i].String(),
			"balance", balance.String(),
			"delta", fmt.Sprintf("%s%s", deltaSign, delta.String()),
			"nonce", nonce,
			"nonce_delta", int64(nonce)-int64(initialNonces[i]))
	}

	// Verify conservation of tokens
	if totalBefore.Cmp(totalAfter) == 0 {
		log.Info(log.Node, "✅ BALANCE CONSERVATION VERIFIED", "total", totalAfter.String())
	} else {
		diff := new(big.Int).Sub(totalAfter, totalBefore)
		log.Error(log.Node, "❌ BALANCE MISMATCH",
			"before", totalBefore.String(),
			"after", totalAfter.String(),
			"difference", diff.String())
		return fmt.Errorf("balance mismatch: before=%s, after=%s, difference=%s", totalBefore.String(), totalAfter.String(), diff.String())
	}

	// Verify coinbase collected fees
	coinbaseBalance, err := b.stateDB.GetBalance(b.serviceID, coinbaseAddress)
	if err != nil {
		return fmt.Errorf("GetBalance (coinbase) failed: %w", err)
	}
	coinbaseBalanceBig := new(big.Int).SetBytes(coinbaseBalance.Bytes())
	if coinbaseBalanceBig.Cmp(big.NewInt(0)) == 0 {
		return fmt.Errorf("coinbase balance is 0 - fee collection failed")
	}
	log.Info(log.Node, "Coinbase balance after round", "address", coinbaseAddress.String(), "balance", coinbaseBalanceBig.String())

	return nil
}

type SimulatedTransfer struct {
	TokenAddress     common.Address
	FromAddress      common.Address
	ToAddress        common.Address
	Value            *big.Int
	TransactionHash  common.Hash
	BlockTimestamp   string
	BlockNumber      uint64
	LogIndex         uint64
	Symbol           string
	ValueDecimal     string
}

// mapAddressToShard maps an address to a shard ID using first 56 bits (7 bytes) of keccak256 hash
func mapAddressToShard(addr common.Address, numShards uint64) uint64 {
	hash := common.Keccak256(addr.Bytes())
	// Take first 7 bytes (56 bits) for shard routing
	prefix56 := binary.BigEndian.Uint64(append([]byte{0}, hash.Bytes()[:7]...))
	return prefix56 % numShards
}

// mapTxHashToShard maps a transaction hash to a shard ID
func mapTxHashToShard(txHash common.Hash, numShards uint64) uint64 {
	// Take first 7 bytes (56 bits) for shard routing
	prefix56 := binary.BigEndian.Uint64(append([]byte{0}, txHash.Bytes()[:7]...))
	return prefix56 % numShards
}

// TestEVMBlocksShardedByHour analyzes cores needed per hour for stablecoin transfers
func TestEVMBlocksShardedByHour(t *testing.T) {
	log.InitLogger("info")

	csvFiles := []string{
		"stablecoin/stablecoin_transfers_sorted_000000000720.csv.gz",
		"stablecoin/stablecoin_transfers_sorted_000000000721.csv.gz",
		"stablecoin/stablecoin_transfers_sorted_000000000722.csv.gz",
		"stablecoin/stablecoin_transfers_sorted_000000000723.csv.gz",
		"stablecoin/stablecoin_transfers_sorted_000000000724.csv.gz",
		"stablecoin/stablecoin_transfers_sorted_000000000725.csv.gz",
		"stablecoin/stablecoin_transfers_sorted_000000000726.csv.gz",
		"stablecoin/stablecoin_transfers_sorted_000000000727.csv.gz",
		"stablecoin/stablecoin_transfers_sorted_000000000728.csv.gz",
		"stablecoin/stablecoin_transfers_sorted_000000000729.csv.gz",
	}

	// Read all transfers
	allTransfers := make([]SimulatedTransfer, 0)
	for idx, csvFile := range csvFiles {
		transfers, err := readStablecoinTransfers(csvFile)
		if err != nil {
			log.Warn(log.Node, "CSV file not found, skipping", "file", csvFile, "err", err)
			continue
		}
		log.Info(log.Node, fmt.Sprintf("Loaded file %d/%d", idx+1, len(csvFiles)),
			"file", csvFile,
			"transfers", len(transfers),
			"cumulative", len(allTransfers)+len(transfers))
		allTransfers = append(allTransfers, transfers...)
	}

	if len(allTransfers) == 0 {
		t.Skip("No transfer data found")
	}

	log.Info(log.Node, "Total transfers loaded", "count", len(allTransfers))

	// Group transfers by hour
	hourlyTransfers := make(map[string][]SimulatedTransfer)
	for _, transfer := range allTransfers {
		// BlockTimestamp format: "2025-11-18 23:14:47 UTC"
		// Extract hour: "2025-11-18 23:00"
		if len(transfer.BlockTimestamp) >= 13 {
			hour := transfer.BlockTimestamp[:13] + ":00"
			hourlyTransfers[hour] = append(hourlyTransfers[hour], transfer)
		}
	}

	// Sort hours and analyze each
	hours := make([]string, 0, len(hourlyTransfers))
	for hour := range hourlyTransfers {
		hours = append(hours, hour)
	}

	// Simple string sort works for ISO format timestamps
	for i := 0; i < len(hours); i++ {
		for j := i + 1; j < len(hours); j++ {
			if hours[i] > hours[j] {
				hours[i], hours[j] = hours[j], hours[i]
			}
		}
	}

	log.Info(log.Node, "=== Cores Needed Per Hour ===")
	log.Info(log.Node, "Configuration", "maxShardsPerCore", 6000, "maxCores", 341)

	for _, hour := range hours {
		transfers := hourlyTransfers[hour]
		runs := bucketTransfersByCores(transfers, 341, 6000)

		totalCoresUsed := 0
		for _, run := range runs {
			coresInRun := 0
			for i := 0; i < 341; i++ {
				if len(run.CoreTransfers[i]) > 0 {
					coresInRun++
				}
			}
			if coresInRun > totalCoresUsed {
				totalCoresUsed = coresInRun
			}
		}

		log.Info(log.Node, hour,
			"transfers", len(transfers),
			"runs", len(runs),
			"cores_needed", totalCoresUsed)
	}
	log.Info(log.Node, "=========================")
}

// TestEVMBlocksSharded reads stablecoin transfer data and buckets into N cores with shard constraints.
// This test demonstrates the JAM sharding architecture described in SHARDING.md:
// - Each transfer touches 3 shards: from_address, to_address, and tx_hash
// - Shards are mapped using first 56 bits (7 bytes) of keccak256 hash
// - Transfers are assigned to cores ensuring each core updates at most maxShards (6K)
// - The test uses 1M total meta-shards as specified in SHARDING.md
// - Results show the distribution across cores and shard utilization
//
// To use real stablecoin data from GCS:
//   gsutil cat gs://wolk/stablecoin_transfers_sorted_000000000731.csv.gz | zcat > statedb/testdata/stablecoin_transfers_sample.csv
func TestEVMBlocksSharded(t *testing.T) {
	log.InitLogger("info")

	// Test with different core counts
	testCases := []struct {
		name         string
		numCores     int
		maxShards    int
		csvFiles     []string
		maxTransfers int // limit total transfers to process (0 = no limit)
	}{
		{"2-cores", 2, 6000, []string{
			"stablecoin/stablecoin_transfers_sorted_000000000720.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000721.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000722.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000723.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000724.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000725.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000726.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000727.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000728.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000729.csv.gz",
		}, 1_000_000}, // Limit to 1M transfers for 2-core test
		{"341-cores", 341, 6000, []string{
			"stablecoin/stablecoin_transfers_sorted_000000000720.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000721.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000722.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000723.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000724.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000725.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000726.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000727.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000728.csv.gz",
			"stablecoin/stablecoin_transfers_sorted_000000000729.csv.gz",
		}, 0}, // No limit for 341-core test
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			log.Info(log.Node, "Testing sharded transfers", "cores", tc.numCores, "maxShards", tc.maxShards, "files", len(tc.csvFiles))

			// Read and parse all CSV files
			allTransfers := make([]SimulatedTransfer, 0)
			for idx, csvFile := range tc.csvFiles {
				if tc.maxTransfers > 0 && len(allTransfers) >= tc.maxTransfers {
					log.Info(log.Node, "Reached transfer limit, stopping file loading",
						"limit", tc.maxTransfers,
						"loaded", len(allTransfers))
					break
				}

				transfers, err := readStablecoinTransfers(csvFile)
				if err != nil {
					log.Warn(log.Node, "CSV file not found, skipping", "file", csvFile, "err", err)
					continue
				}

				// Apply limit if specified
				if tc.maxTransfers > 0 && len(allTransfers)+len(transfers) > tc.maxTransfers {
					remaining := tc.maxTransfers - len(allTransfers)
					transfers = transfers[:remaining]
					log.Info(log.Node, fmt.Sprintf("Loaded file %d/%d (partial)", idx+1, len(tc.csvFiles)),
						"file", csvFile,
						"transfers", len(transfers),
						"cumulative", len(allTransfers)+len(transfers),
						"limit_reached", true)
				} else {
					log.Info(log.Node, fmt.Sprintf("Loaded file %d/%d", idx+1, len(tc.csvFiles)),
						"file", csvFile,
						"transfers", len(transfers),
						"cumulative", len(allTransfers)+len(transfers))
				}
				allTransfers = append(allTransfers, transfers...)
			}

			if len(allTransfers) == 0 {
				log.Warn(log.Node, "No transfers loaded, using sample data")
				allTransfers = createSampleTransfers()
			}

			log.Info(log.Node, "Total transfers loaded", "count", len(allTransfers))

			// Bucket transfers into cores with shard constraints (may create multiple runs)
			runs := bucketTransfersByCores(allTransfers, tc.numCores, tc.maxShards)

			// Print statistics for each run
			printMultiRunStats(runs, tc.numCores, tc.maxShards, len(allTransfers))
		})
	}
}

// readStablecoinTransfers reads CSV file (supports .gz) and parses transfer data
func readStablecoinTransfers(filepath string) ([]SimulatedTransfer, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var csvReader *csv.Reader

	// Check if file is gzipped
	if len(filepath) > 3 && filepath[len(filepath)-3:] == ".gz" {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer gzReader.Close()
		csvReader = csv.NewReader(gzReader)
	} else {
		csvReader = csv.NewReader(file)
	}

	// Read header
	_, err = csvReader.Read()
	if err != nil {
		return nil, err
	}

	transfers := make([]SimulatedTransfer, 0)
	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Parse CSV columns
		if len(record) < 10 {
			continue
		}

		value, ok := new(big.Int).SetString(record[3], 10)
		if !ok {
			continue
		}

		blockNum, err := strconv.ParseUint(record[6], 10, 64)
		if err != nil {
			continue
		}

		logIdx, err := strconv.ParseUint(record[7], 10, 64)
		if err != nil {
			continue
		}

		transfer := SimulatedTransfer{
			TokenAddress:    common.HexToAddress(record[0]),
			FromAddress:     common.HexToAddress(record[1]),
			ToAddress:       common.HexToAddress(record[2]),
			Value:           value,
			TransactionHash: common.HexToHash(record[4]),
			BlockTimestamp:  record[5],
			BlockNumber:     blockNum,
			LogIndex:        logIdx,
			Symbol:          record[8],
			ValueDecimal:    record[9],
		}

		transfers = append(transfers, transfer)
	}

	return transfers, nil
}

// createSampleTransfers generates sample transfer data for testing
func createSampleTransfers() []SimulatedTransfer {
	transfers := make([]SimulatedTransfer, 0)

	// Sample USDC and USDT addresses
	usdcAddr := common.HexToAddress("0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48")
	usdtAddr := common.HexToAddress("0xdac17f958d2ee523a2206206994597c13d831ec7")

	// Generate some sample transfers
	for i := 0; i < 100; i++ {
		var tokenAddr common.Address
		var symbol string
		if i%2 == 0 {
			tokenAddr = usdcAddr
			symbol = "USDC"
		} else {
			tokenAddr = usdtAddr
			symbol = "USDT"
		}

		fromAddr := common.HexToAddress(fmt.Sprintf("0x%040d", i*2))
		toAddr := common.HexToAddress(fmt.Sprintf("0x%040d", i*2+1))
		txHash := common.HexToHash(fmt.Sprintf("0x%064d", i))

		transfer := SimulatedTransfer{
			TokenAddress:    tokenAddr,
			FromAddress:     fromAddr,
			ToAddress:       toAddr,
			Value:           big.NewInt(int64(100000 + i*1000)),
			TransactionHash: txHash,
			BlockTimestamp:  "2025-11-18 23:14:47 UTC",
			BlockNumber:     23829202 + uint64(i/10),
			LogIndex:        uint64(i % 10),
			Symbol:          symbol,
			ValueDecimal:    fmt.Sprintf("%d.%06d", 100+i, i*1000),
		}

		transfers = append(transfers, transfer)
	}

	return transfers
}

// RunResult represents the result of one run (assignment of transfers to cores)
type RunResult struct {
	CoreTransfers [][]SimulatedTransfer
	CoreShards    [][]uint64
	TotalShards   int
}

// bucketTransfersByCores distributes transfers across cores respecting shard constraints
// Returns multiple runs if needed to process all transfers
func bucketTransfersByCores(transfers []SimulatedTransfer, numCores int, maxShards int) []RunResult {
	// Calculate total number of shards based on SHARDING.md
	// Using 1M meta-shards as baseline (can be 1M-4M)
	const totalMetaShards = 1_000_000

	runs := make([]RunResult, 0)
	remainingTransfers := transfers

	for len(remainingTransfers) > 0 {
		// Track which shards each core is using for this run
		coreShardsMap := make([]map[uint64]bool, numCores)
		coreTransfersList := make([][]SimulatedTransfer, numCores)
		coreShardCounts := make([]int, numCores) // Cache shard counts per core

		for i := 0; i < numCores; i++ {
			coreShardsMap[i] = make(map[uint64]bool, maxShards/2) // Preallocate
			coreTransfersList[i] = make([]SimulatedTransfer, 0, len(remainingTransfers)/numCores)
		}

		unassignedTransfers := make([]SimulatedTransfer, 0)
		progressCounter := 0
		activeCores := 0 // Track how many cores we've started using

		// Process each remaining transfer and assign to a core
		for idx, transfer := range remainingTransfers {
			// Progress logging every 500K transfers
			if idx > 0 && idx%500000 == 0 {
				log.Info(log.Node, "Processing transfers", "done", idx, "total", len(remainingTransfers), "assigned", progressCounter, "cores_used", activeCores)
			}

			// Calculate shards touched by this transfer:
			// 1. From address shard
			// 2. To address shard
			// 3. Transaction hash shard (for receipt)
			fromShard := mapAddressToShard(transfer.FromAddress, totalMetaShards)
			toShard := mapAddressToShard(transfer.ToAddress, totalMetaShards)
			txShard := mapTxHashToShard(transfer.TransactionHash, totalMetaShards)

			// Deduplicate shards inline
			var uniqueShards [3]uint64
			numUnique := 0
			uniqueShards[numUnique] = fromShard
			numUnique++
			if toShard != fromShard {
				uniqueShards[numUnique] = toShard
				numUnique++
			}
			if txShard != fromShard && txShard != toShard {
				uniqueShards[numUnique] = txShard
				numUnique++
			}

			// GREEDY BIN PACKING: Try to fit into existing cores first, then add new core
			assigned := false
			bestCore := -1
			bestNewShards := numUnique + 1 // Start with worst case
			fullCores := 0

			// First pass: find best fitting existing core (minimizes new shards)
			for coreIdx := 0; coreIdx < activeCores; coreIdx++ {
				// Fast check: skip if core is at capacity
				if coreShardCounts[coreIdx] >= maxShards-2 { // Allow 3 shards max
					fullCores++
					continue
				}

				// Count how many new shards would be added
				newShards := 0
				for i := 0; i < numUnique; i++ {
					if !coreShardsMap[coreIdx][uniqueShards[i]] {
						newShards++
					}
				}

				// Check if this core can accommodate the transfer
				if coreShardCounts[coreIdx]+newShards <= maxShards {
					// Prefer cores that already have these shards (newShards=0 is best)
					if newShards < bestNewShards {
						bestCore = coreIdx
						bestNewShards = newShards
						if newShards == 0 {
							break // Perfect fit - stop searching
						}
					}
				}
			}

			// If no existing core works, try adding a new core
			if bestCore == -1 && activeCores < numCores {
				bestCore = activeCores
				activeCores++
			}

			// If all cores are full, break early to next run
			if bestCore == -1 && fullCores >= activeCores {
				// All cores saturated - put rest in unassigned and move to next run
				unassignedTransfers = append(unassignedTransfers, remainingTransfers[idx:]...)
				break
			}

			// Assign to best core
			if bestCore != -1 {
				coreIdx := bestCore
				for i := 0; i < numUnique; i++ {
					shard := uniqueShards[i]
					if !coreShardsMap[coreIdx][shard] {
						coreShardsMap[coreIdx][shard] = true
						coreShardCounts[coreIdx]++
					}
				}
				coreTransfersList[coreIdx] = append(coreTransfersList[coreIdx], transfer)
				assigned = true
				progressCounter++
			}

			if !assigned {
				// Could not assign in this run - will go to next run
				unassignedTransfers = append(unassignedTransfers, transfer)
			}
		}

		log.Info(log.Node, "Completed run assignment", "assigned", progressCounter, "unassigned", len(unassignedTransfers), "cores_used", activeCores)

		// Convert shard maps to slices and count total unique shards
		coreShardsList := make([][]uint64, numCores)
		totalUniqueShards := make(map[uint64]bool)
		for i := 0; i < numCores; i++ {
			shards := make([]uint64, 0, len(coreShardsMap[i]))
			for shard := range coreShardsMap[i] {
				shards = append(shards, shard)
				totalUniqueShards[shard] = true
			}
			coreShardsList[i] = shards
		}

		// Add this run to results
		runs = append(runs, RunResult{
			CoreTransfers: coreTransfersList,
			CoreShards:    coreShardsList,
			TotalShards:   len(totalUniqueShards),
		})

		// Move to next run with unassigned transfers
		if len(unassignedTransfers) == len(remainingTransfers) {
			// No progress made - this shouldn't happen but prevents infinite loop
			log.Error(log.Node, "No progress in assignment - stopping",
				"remaining", len(remainingTransfers))
			break
		}
		remainingTransfers = unassignedTransfers
	}

	return runs
}

func countActiveCores(coreShardCounts []int) int {
	active := 0
	for _, count := range coreShardCounts {
		if count > 0 {
			active++
		}
	}
	return active
}

// printMultiRunStats prints statistics for all runs
func printMultiRunStats(runs []RunResult, numCores int, maxShards int, totalTransfersInput int) {
	log.Info(log.Node, "=== Multi-Run Sharding Statistics ===")
	log.Info(log.Node, "Configuration",
		"numCores", numCores,
		"maxShardsPerCore", maxShards,
		"totalRuns", len(runs))

	grandTotalTransfers := 0
	grandTotalUniqueShards := make(map[uint64]bool)

	for runIdx, run := range runs {
		log.Info(log.Node, fmt.Sprintf("--- Run %d ---", runIdx+1))

		runTotalTransfers := 0
		runActiveCores := 0
		runTotalShardSlots := 0

		for coreIdx := 0; coreIdx < numCores; coreIdx++ {
			numTransfers := len(run.CoreTransfers[coreIdx])
			numShards := len(run.CoreShards[coreIdx])

			if numTransfers > 0 {
				runActiveCores++
				runTotalTransfers += numTransfers
				runTotalShardSlots += numShards

				// Track unique shards across all runs
				for _, shard := range run.CoreShards[coreIdx] {
					grandTotalUniqueShards[shard] = true
				}

				// Only log first few and last few cores to avoid spam
				if runActiveCores <= 3 || coreIdx >= numCores-3 {
					log.Info(log.Node, fmt.Sprintf("  Core %d", coreIdx),
						"transfers", numTransfers,
						"shards", numShards,
						"utilization", fmt.Sprintf("%.1f%%", float64(numShards)*100.0/float64(maxShards)))
				}
			}
		}

		if runActiveCores > 6 {
			log.Info(log.Node, fmt.Sprintf("  ... (%d more active cores) ...", runActiveCores-6))
		}

		grandTotalTransfers += runTotalTransfers

		log.Info(log.Node, fmt.Sprintf("Run %d Summary", runIdx+1),
			"activeCores", runActiveCores,
			"transfers", runTotalTransfers,
			"uniqueShards", run.TotalShards,
			"totalShardSlots", runTotalShardSlots,
			"avgTransfersPerCore", runTotalTransfers/maxInt(runActiveCores, 1),
			"avgShardsPerCore", runTotalShardSlots/maxInt(runActiveCores, 1))
	}

	log.Info(log.Node, "=== Grand Total ===")
	log.Info(log.Node, "Overall Summary",
		"totalRuns", len(runs),
		"totalTransfersProcessed", grandTotalTransfers,
		"totalTransfersInput", totalTransfersInput,
		"coverage", fmt.Sprintf("%.1f%%", float64(grandTotalTransfers)*100.0/float64(totalTransfersInput)),
		"uniqueShardsAcrossAllRuns", len(grandTotalUniqueShards))
	log.Info(log.Node, "=========================")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// TestEVMGenesis runs the genesis process and validates the resulting state
func TestEVMBlocksTransfers(t *testing.T) {
	log.InitLogger("debug")
	chain, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewEVMService failed: %v", err)
	}
	initBalance := int64(61_000_000)
	_, err = chain.SubmitEVMGenesis(initBalance)
	if err != nil {
		t.Fatalf("SubmitEVMGenesis failed: %v", err)
	}

	balance, err := chain.stateDB.GetBalance(chain.serviceID, evmtypes.IssuerAddress)
	if err != nil {
		t.Fatalf("GetBalance failed: %v", err)
	}

	// Check that the balance matches initBalance (with 18 decimals)
	expectedBalance := new(big.Int).Mul(big.NewInt(initBalance), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	actualBalance := new(big.Int).SetBytes(balance[:])
	if actualBalance.Cmp(expectedBalance) != 0 {
		t.Fatalf("Balance mismatch: expected %s, got %s", expectedBalance.String(), actualBalance.String())
	}
	log.Info(log.Node, "✅ Genesis balance verified", "amount", actualBalance.String())

	nonce, err := chain.stateDB.GetTransactionCount(chain.serviceID, evmtypes.IssuerAddress)
	if err != nil {
		t.Fatalf("GetTransactionCount failed: %v", err)
	}

	// Check that the nonce is initialized to 1 (as set in genesis)
	expectedNonce := uint64(1)
	if nonce != expectedNonce {
		t.Fatalf("Nonce mismatch: expected %d, got %d", expectedNonce, nonce)
	}
	log.Info(log.Node, "✅ Genesis nonce verified", "nonce", nonce)

	log.Info(log.Node, "Issuer account after genesis", "address", evmtypes.IssuerAddress, "balance", balance, "nonce", nonce)

	// err = chain.checkBalanceOf(evmtypes.IssuerAddress, balance)
	// if err != nil {
	// 	log.Warn(log.Node, "checkBalanceOf warning (non-fatal)", "err", err)
	// }
	// err = chain.checkNonces(evmtypes.IssuerAddress, new(big.Int).SetUint64(nonce))
	// if err != nil {
	// 	log.Warn(log.Node, "checkNonces warning (non-fatal)", "err", err)
	// }

	/*
		gasUsed, err := chain.stateDB.EstimateGasTransfer(chain.serviceID, evmtypes.IssuerAddress, evmtypes.UsdmAddress, chain.pvmBackend)
		if err != nil {
			t.Fatalf("EstimateGasTransfer failed: %v", err)
		}
		log.Info(log.Node, "EstimateGasTransfer", "estimatedGas", gasUsed)
	*/
	for round := 0; round < 10; round++ {
		isLastRound := (round == numRounds-1)
		log.Info(log.Node, "test_transfers - round", "round", round, "isLastRound", isLastRound)
		err := RunTransfersRound(chain, chain.createTransferTriplesForRound(round, txnsPerRound, isLastRound))
		if err != nil {
			panic(fmt.Errorf("transfer round %d failed: %w", round, err))
		}
	}
}

func TestEVMBlocksDeployContract(t *testing.T) {
	log.InitLogger("info")
	b, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewEVMService failed: %v", err)
	}

	_, err = b.SubmitEVMGenesis(61_000_000)
	if err != nil {
		t.Fatalf("SubmitEVMGenesis failed: %v", err)
	}

	contractFile := "../services/evm/contracts/usdm.bin"
	contractAddress, err := b.DeployContract(contractFile)
	if err != nil {
		t.Fatalf("DeployContract failed: %v", err)
	}

	// if err := b.ShowTxReceipts(block, block.TxHashes, "Contract Deployment", make(map[common.Hash]string)); err != nil {
	// 	t.Fatalf("ShowTxReceipts failed: %v", err)
	// }

	// Verify contract deployment by checking if code exists at the calculated address
	deployedCode, err := b.stateDB.GetCode(b.serviceID, common.Address(contractAddress))
	if err != nil {
		t.Fatalf("GetCode err %v", err)
	}
	if len(deployedCode) == 0 {
		t.Fatalf("contract deployment failed: no code at address %s", contractAddress.Hex())
	}

	log.Info(log.Node, "Contract deployed successfully", "file", contractFile, "address", contractAddress, "codeSize", len(deployedCode))
}

// TestRewards tests the rewards service with validator rewards computation
func TestRewards(t *testing.T) {
	t.Skip("RewardsServiceCode not defined")
	log.InitLogger("info")
	log.EnableModule(log.Node)
	c, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewRollup failed: %v", err)
	}

	service, ok, err := c.stateDB.GetService(c.serviceID)
	if err != nil || !ok {
		t.Fatalf("GetService failed: %v", err)
	}

	bundles := make([]*types.WorkPackageBundle, types.TotalCores)

	// Test 1: Submit multiple validators with varied reward data
	log.Info(log.Node, "=== Test 1: Multiple validators with varied data ===")
	for validatorIdx := uint16(0); validatorIdx < 5; validatorIdx++ {
		wp := DefaultWorkPackage(c.serviceID, service)
		wp.RefineContext = c.stateDB.GetRefineContext()

		// Each validator submits their perspective of epoch 1 rewards
		payload := GenerateRewardsPayload(1, validatorIdx)

		wp.WorkItems[0].Payload = payload
		wp.WorkItems[0].RefineGasLimit = 10_000_000
		wp.WorkItems[0].ExportCount = 0

		bundles[0] = &types.WorkPackageBundle{
			WorkPackage:   wp,
			ExtrinsicData: []types.ExtrinsicsBlobs{{}},
		}

		err := c.processWorkPackageBundlesPipelined(bundles)
		if err != nil {
			t.Fatal(err)
		}

		// Note: State persistence is not fully working yet (read_service_storage returns empty)
		// This will be fixed when PVM host functions are wired up
		k := common.ServiceStorageKey(c.serviceID, []byte{0})
		data, _, _ := c.storage.GetServiceStorage(c.serviceID, k)
		log.Info(log.Node, "Validator submitted", "epoch", 1, "validator", validatorIdx, "storage_bytes", len(data))
	}

	// Test 2: Verify different epochs produce different message hashes
	log.Info(log.Node, "=== Test 2: Different epochs produce different hashes ===")
	for epoch := 2; epoch <= 3; epoch++ {
		wp := DefaultWorkPackage(c.serviceID, service)
		wp.RefineContext = c.stateDB.GetRefineContext()

		payload := GenerateRewardsPayload(epoch, 0)

		wp.WorkItems[0].Payload = payload
		wp.WorkItems[0].RefineGasLimit = 10_000_000
		wp.WorkItems[0].ExportCount = 0

		bundles[0] = &types.WorkPackageBundle{
			WorkPackage:   wp,
			ExtrinsicData: []types.ExtrinsicsBlobs{{}},
		}

		err := c.processWorkPackageBundlesPipelined(bundles)
		if err != nil {
			t.Fatal(err)
		}

		log.Info(log.Node, "Rewards epoch processed", "epoch", epoch, "service", c.serviceID)
	}
}

// GenerateRewardsPayload creates a SCALE-encoded SignedApprovalsTallyMessage
// SCALE encoding format (struct field order from types.rs):
// SignedApprovalsTallyMessage {
//   message: ApprovalsTallyMessage {
//     epoch: u64 (8 bytes LE)
//     lines: Vec<ApprovalTallyMessageLine> (compact length prefix + elements)
//   }
//   signature: [u8; 64] (64 bytes)
//   validator_index: u16 (2 bytes LE)
// }
func GenerateRewardsPayload(epoch int, validatorIndex uint16) []byte {
	const NUM_VALIDATORS = 1024

	// Create SCALE-encoded payload
	payload := make([]byte, 0, 20000) // Pre-allocate space

	// ===== ApprovalsTallyMessage =====
	// 1. Encode epoch (u64 as little-endian)
	epochBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(epochBytes, uint64(epoch))
	payload = append(payload, epochBytes...)

	// 2. Encode lines array (compact length prefix + elements)
	// SCALE compact encoding for 1024: for values 64-16383, use mode 01
	// 1024 << 2 | 01 = 4097 = 0x1001 in little-endian = 0x01 0x10
	payload = append(payload, 0x01, 0x10) // Compact encoding of 1024

	// Generate 1024 ApprovalTallyMessageLine entries
	// Vary data based on validator index and epoch to create realistic diversity
	for i := 0; i < NUM_VALIDATORS; i++ {
		// approval_usages (u32 little-endian) - varies by validator and epoch
		// Validator's perspective: different validators see different usage patterns
		baseApprovalUsage := uint32(100 + (int(validatorIndex) * 10) + (epoch * 5))
		approvalUsages := baseApprovalUsage + uint32(i%50) // Add variation per reported validator
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, approvalUsages)
		payload = append(payload, buf...)

		// noshows (u32 little-endian) - occasional noshows (1-2% of validators)
		noshows := uint32(0)
		if i%100 < 2 { // 2% of validators have noshows
			noshows = uint32(1 + (epoch % 3)) // 1-3 noshows
		}
		binary.LittleEndian.PutUint32(buf, noshows)
		payload = append(payload, buf...)

		// used_downloads (u32 little-endian) - varies by validator and epoch
		baseDownloads := uint32(50 + (int(validatorIndex) * 5) + (epoch * 2))
		usedDownloads := baseDownloads + uint32(i%30) // Add variation
		binary.LittleEndian.PutUint32(buf, usedDownloads)
		payload = append(payload, buf...)
	}

	// ===== SignedApprovalsTallyMessage =====
	// 3. Encode signature (64 bytes - dummy for now)
	signature := make([]byte, 64)
	for i := 0; i < 64; i++ {
		signature[i] = byte(i)
	}
	payload = append(payload, signature...)

	// 4. Encode validator_index (u16 little-endian)
	validatorIndexBytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(validatorIndexBytes, validatorIndex)
	payload = append(payload, validatorIndexBytes...)

	return payload
}
