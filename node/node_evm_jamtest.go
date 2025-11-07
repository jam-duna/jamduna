package node

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	types "github.com/colorfulnotion/jam/types"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	numRounds    = 50
	txnsPerRound = 1
)

// EVMService manages EVM block submissions with witness tracking
type EVMService struct {
	n1      JNode
	service *types.TestService
}

// NewEVMService creates a new EVMService instance
func NewEVMService(n1 JNode, service *types.TestService) (evmservice *EVMService, err error) {
	evmservice = &EVMService{
		n1:      n1,
		service: service,
	}

	err = evmservice.SubmitEVMPayloadBlocks(0, 1)
	if err != nil {
		log.Error(log.Node, "SubmitEVMPayloadBlocks(0, 1) ERR", "err", err)
		return evmservice, err
	}
	return evmservice, nil
}

// getFunctionSelector computes the 4-byte function selector and event topic hash map
// Example: getFunctionSelector("fibonacci(uint256)", ["FibCache(uint256)", "FibComputed(uint256)"])
// returns ([4]byte{0x61, 0x04, 0x7f, 0xf4}, map[hash]"FibCache(uint256)", map[hash]"FibComputed(uint256)")
func getFunctionSelector(funcSig string, eventSigs []string) ([4]byte, map[common.Hash]string) {
	// Compute function selector
	funcHash := crypto.Keccak256([]byte(funcSig))
	var selector [4]byte
	copy(selector[:], funcHash[:4])

	// Compute event topic hashes and map to signatures
	topics := make(map[common.Hash]string, len(eventSigs))
	for _, eventSig := range eventSigs {
		eventHash := crypto.Keccak256([]byte(eventSig))
		topics[common.BytesToHash(eventHash[:32])] = eventSig
	}

	return selector, topics
}

// defaultTopics returns a map of topic hashes to event signatures for common ERC20/ERC1155 events
func defaultTopics() map[common.Hash]string {
	erc20Events := []string{
		"Transfer(address,address,uint256)", // Most common - token transfers
		"Approval(address,address,uint256)", // Second most common - approve allowances
		"Mint(address,uint256)",             // Minting new tokens
		"Burn(address,uint256)",             // Burning tokens
		"Burn(uint256)",
		"Mint(uint256)",
		"Deposit(address,uint256)",                                   // Wrapped tokens deposit
		"Withdrawal(address,uint256)",                                // Wrapped tokens withdrawal
		"Swap(address,address,uint256,uint256)",                      // Token swaps
		"TransferSingle(address,address,address,uint256,uint256)",    // ERC1155 single transfer
		"TransferBatch(address,address,address,uint256[],uint256[])", // ERC1155 batch transfer
		"ApprovalForAll(address,address,bool)",                       // ERC1155 approval for all
	}

	topics := make(map[common.Hash]string, len(erc20Events))
	for _, eventSig := range erc20Events {
		eventHash := crypto.Keccak256([]byte(eventSig))
		topics[common.BytesToHash(eventHash[:32])] = eventSig
	}

	return topics
}

func showEthereumLogs(txHash common.Hash, logs []EthereumLog, allTopics map[common.Hash]string) {
	for _, lg := range logs {
		if len(lg.Topics) == 0 {
			continue
		}
		// First topic is the event signature
		topicHash := common.HexToHash(lg.Topics[0])
		if eventSig, found := allTopics[topicHash]; found {
			// Build a string with event signature and decoded data
			topicStr := eventSig

			// Add indexed topics (Topics[1:])
			if len(lg.Topics) > 1 {
				for i := 1; i < len(lg.Topics); i++ {
					topicStr = topicStr + fmt.Sprintf("indexed[%d]=%s", i-1, lg.Topics[i])
				}
			}

			// Add data (non-indexed parameters)
			dataStr := lg.Data
			// Fix double 0x prefix issue
			if strings.HasPrefix(dataStr, "0x0x") {
				dataStr = dataStr[2:]
			}
			dataBytes := common.FromHex(dataStr)

			// Convert logIndex from hex to decimal
			logIndexInt := new(big.Int)
			logIndexInt.SetString(strings.TrimPrefix(lg.LogIndex, "0x"), 16)

			if len(dataBytes) > 0 && len(dataBytes)%32 == 0 {
				var decodedValues []string
				for i := 0; i < len(dataBytes); i += 32 {
					value := new(big.Int).SetBytes(dataBytes[i : i+32])
					decodedValues = append(decodedValues, value.String())
				}
				// Extract event name without parameter types for decoded output
				eventName := eventSig
				if idx := strings.Index(eventSig, "("); idx != -1 {
					eventName = eventSig[:idx]
				}
				decodedStr := fmt.Sprintf("%s(%s)", eventName, strings.Join(decodedValues, ","))
				log.Info(log.Node, decodedStr, "logIndex", logIndexInt.String(), "txHash", common.Str(txHash))
			} else if len(dataBytes) > 0 {
				log.Info(log.Node, dataStr, "logIndex", logIndexInt.String(), "txHash", common.Str(txHash))
			} else {
				log.Info(log.Node, topicStr, "logIndex", logIndexInt.String(), "txHash", common.Str(txHash))
			}
		} else {
			log.Warn(log.Node, fmt.Sprintf("%s-%s", txHash, lg.Topics[0]), "logIndex", lg.LogIndex, "data", lg.Data)
		}
	}
}

// parseIntParam parses an integer parameter, supporting decimal and hex (0x prefix)
func parseIntParam(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		// Hex format
		val, err := strconv.ParseInt(s[2:], 16, 64)
		if err != nil {
			return 0, err
		}
		return val, nil
	}
	// Decimal format
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return val, nil
}

// SubmitEVMTransactions creates and submits a work package with raw transactions
func (b *EVMService) SubmitEVMTransactions(identifier string, evmTxs [][]byte) ([]common.Hash, error) {
	// Create extrinsics blobs and hashes from raw transactions
	extrinsics := types.ExtrinsicsBlobs(evmTxs)
	hashes := make([]types.WorkItemExtrinsic, len(evmTxs))
	txHashes := make([]common.Hash, len(evmTxs))
	for i, tx := range evmTxs {
		hashes[i] = types.WorkItemExtrinsic{
			Hash: common.Blake2Hash(tx),
			Len:  uint32(len(tx)),
		}
		// Calculate Ethereum transaction hash
		txHashes[i] = common.Keccak256(tx)
	}

	// Create work package
	wp := types.WorkPackage{
		AuthCodeHost:          0,
		AuthorizationToken:    nil,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ConfigurationBlob:     nil,
		WorkItems: []types.WorkItem{
			{
				Service:            statedb.EVMServiceCode,
				CodeHash:           b.service.CodeHash,
				Payload:            buildPayload(PayloadTypeTransactions, len(evmTxs), 0),
				RefineGasLimit:     types.RefineGasAllocation,
				AccumulateGasLimit: types.AccumulationGasAllocation,
				ImportedSegments:   []types.ImportSegment{},
				ExportCount:        0, // Let the BuildBundle determine this
				Extrinsics:         hashes,
			},
		},
	}

	bundle, _, err := b.n1.BuildBundle(wp, []types.ExtrinsicsBlobs{extrinsics}, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("BuildBundle failed for %s: %v", identifier, err)
	}

	// Create work package request
	wpr := &WorkPackageRequest{
		Identifier:      identifier,
		WorkPackage:     bundle.WorkPackage,
		ExtrinsicsBlobs: bundle.ExtrinsicData[0],
	}

	// Submit and wait for completion
	ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries*2)

	_, err = RobustSubmitAndWaitForWorkPackages(ctx, b.n1, []*WorkPackageRequest{wpr})
	cancel()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %v", identifier, err)
	}
	return txHashes, err
}

// ShowTxReceipts displays transaction receipts for the given transaction hashes
func (b *EVMService) ShowTxReceipts(txHashes []common.Hash, description string, allTopics map[common.Hash]string) error {
	log.Info(log.Node, "Showing transaction receipts", "description", description, "count", len(txHashes))

	gasUsedTotal := big.NewInt(0)

	for _, txHash := range txHashes {
		var (
			receipt *EthereumTransactionReceipt
			err     error
		)
		receipt, err = b.n1.GetTransactionReceipt(txHash)
		if err != nil {
			return fmt.Errorf("failed to get transaction receipt for %s: %w", txHash.String(), err)
		}
		if receipt == nil {
			return fmt.Errorf("Transaction receipt not available: %s", txHash.String())
		}
		if receipt.Status != "0x1" {
			return fmt.Errorf("transaction %s failed with status %s", txHash.String(), receipt.Status)
		}
		// Convert gasUsed from hex to decimal
		gasUsedInt := new(big.Int)
		gasUsedInt.SetString(strings.TrimPrefix(receipt.GasUsed, "0x"), 16)
		gasUsedTotal.Add(gasUsedTotal, gasUsedInt)

		log.Info(log.Node, "✅ Transaction succeeded",
			"txHash", txHash.String(),
			"len(logs)", len(receipt.Logs),
			"gasUsed", gasUsedInt.String())
		showEthereumLogs(txHash, receipt.Logs, allTopics)
	}

	log.Info(log.Node, description, "txCount", len(txHashes), "gasUsedTotal", gasUsedTotal.String())
	return nil
}

// EstimateGas tests the EstimateGas functionality with a USDM transfer
func (b *EVMService) EstimateGas(issuerAddress common.Address, usdmAddress common.Address) error {
	recipientAddr, _ := common.GetEVMDevAccount(1)
	transferAmount := big.NewInt(1000000) // 1M tokens (small test amount)

	// Create transfer calldata: transfer(address,uint256)
	estimateCalldata := make([]byte, 68)
	copy(estimateCalldata[0:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) // transfer(address,uint256) selector
	copy(estimateCalldata[16:36], recipientAddr.Bytes())
	copy(estimateCalldata[36:68], transferAmount.FillBytes(make([]byte, 32)))

	estimatedGas, err := b.n1.EstimateGas(issuerAddress, &usdmAddress, 100000, 1000000000, 0, estimateCalldata)
	if err != nil {
		return fmt.Errorf("EstimateGas failed: %w", err)
	}
	log.Info(log.Node, "EstimateGas result", "from", issuerAddress.String(), "to", recipientAddr.String(), "amount", transferAmount.String(), "estimatedGas", estimatedGas)
	return nil
}

// buildPayloadTransaction creates a conformant PayloadTransaction payload
// Format: "T" (1 byte) + tx_count (u32 LE) + witness_count (u16 LE)
func (b *EVMService) ExportStateWitnesses(workReports []*types.WorkReport, saveToFile bool) error {
	witnesses, stateRoot, err := b.n1.GetStateWitnesses(workReports)
	if err != nil {
		return err
	}

	if saveToFile {
		// Create witness files using fixed binary format
		for _, witness := range witnesses {
			// Use fixed binary format (not SCALE encoding) for Rust compatibility
			witnessBytes := witness.SerializeWitness()

			// Filename format: objectid-version-stateroot.bin
			// This allows Rust to parse the filename to get the state root for verification
			filename := fmt.Sprintf("%s-%d-%s.bin",
				witness.ObjectID.Hex()[2:], // Remove 0x prefix
				witness.Ref.Version,
				stateRoot.Hex()[2:]) // Remove 0x prefix

			if err := os.WriteFile(filename, witnessBytes, 0644); err != nil {
				return fmt.Errorf("ExportStateWitnesses: write file %s err %v", filename, err)
			}
		}

		log.Info(log.Node, "ExportStateWitnesses", "count", len(witnesses), "state_root", stateRoot.Hex())

	}
	return nil
}

func (b *EVMService) SubmitEVMPayloadBlocks(startBlock uint32, endBlock uint32) error {
	// MappingEntry represents a single mapping entry to initialize
	type MappingEntry struct {
		Slot  uint8
		Key   common.Address
		Value *big.Int
	}

	// Helper: append bootstrap extrinsic
	appendBootstrapExtrinsic := func(blobs *types.ExtrinsicsBlobs, workItems *[]types.WorkItemExtrinsic, extrinsic []byte) {
		*blobs = append(*blobs, extrinsic)
		*workItems = append(*workItems, types.WorkItemExtrinsic{
			Hash: common.Blake2Hash(extrinsic),
			Len:  uint32(len(extrinsic)),
		})
	}

	// Helper: build 'K' command extrinsic for storage
	buildKExtrinsic := func(address []byte, storageKey []byte, storageValue []byte) []byte {
		extrinsic := make([]byte, 1+20+32+32)
		extrinsic[0] = 0x4B
		copy(extrinsic[1:21], address)
		copy(extrinsic[21:53], storageKey)
		copy(extrinsic[53:85], storageValue)
		return extrinsic
	}

	// Helper: initialize Solidity mapping storage for the Genesis case
	initializeMappings := func(blobs *types.ExtrinsicsBlobs, workItems *[]types.WorkItemExtrinsic, contractAddr common.Address, entries []MappingEntry) {
		for _, entry := range entries {
			// Compute Solidity mapping storage key: keccak256(abi.encode(key, slot))
			keyInput := make([]byte, 64)
			copy(keyInput[12:32], entry.Key[:])
			keyInput[63] = entry.Slot
			storageKey := common.Keccak256(keyInput)

			// Encode value as 32-byte big-endian
			valueBytes := entry.Value.FillBytes(make([]byte, 32))

			appendBootstrapExtrinsic(blobs, workItems, buildKExtrinsic(contractAddr[:], storageKey[:], valueBytes))
		}
	}

	// Helper: verify mapping storage entries for the Genesis case
	verifyMappings := func(entries []MappingEntry) error {
		for _, entry := range entries {
			// Compute storage key
			keyInput := make([]byte, 64)
			copy(keyInput[12:32], entry.Key[:])
			keyInput[63] = entry.Slot
			storageKey := common.Keccak256(keyInput)

			// Get actual value from storage
			storageValue, err := b.n1.GetStorageAt(usdmAddress, storageKey, "latest")
			if err != nil {
				return fmt.Errorf("GetStorageAt failed for slot %d, key %s: %w", entry.Slot, entry.Key.Hex(), err)
			}

			actualValue := new(big.Int).SetBytes(storageValue[:])
			if actualValue.Cmp(entry.Value) != 0 {
				return fmt.Errorf("storage mismatch for slot %d, key %s: expected %s, got %s",
					entry.Slot, entry.Key.Hex(), entry.Value.String(), actualValue.String())
			}

			log.Info(log.Node, "Storage verified", "contract", usdmAddress.Hex(), "slot", entry.Slot,
				"key", entry.Key.Hex(), "value", entry.Value.String())
		}
		return nil
	}

	blobs := types.ExtrinsicsBlobs{}
	workItems := []types.WorkItemExtrinsic{}

	// Only include genesis USDM mappings if submitting genesis block
	isGenesis := startBlock == 0 && endBlock == 1
	var usdmInitialState []MappingEntry
	if isGenesis {
		// Set USDM initial balances and nonces
		totalSupplyValue := new(big.Int).Mul(big.NewInt(61_000_000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		usdmInitialState = []MappingEntry{
			{Slot: 0, Key: issuerAddress, Value: totalSupplyValue}, // balanceOf[issuer]
			{Slot: 1, Key: issuerAddress, Value: big.NewInt(1)},    // nonces[issuer]
		}
		initializeMappings(&blobs, &workItems, usdmAddress, usdmInitialState)
	}
	numExtrinsics := len(workItems)

	objects := []common.Hash{statedb.GetBlockNumberKey()}
	for blockNum := startBlock; blockNum < endBlock; blockNum++ {
		objects = append(objects, statedb.BlockNumberToObjectID(blockNum))
	}

	// blockNumberKeyWitness, ok, stateRoot, err := b.n1.ReadStateWitnessRaw(statedb.EVMServiceCode, statedb.GetBlockNumberKey())
	// blockWitness, ok, blockStateRoot, err := b.n1.ReadStateWitnessRaw(statedb.EVMServiceCode, blockObjectID)
	// Create work package
	wp := types.WorkPackage{
		AuthCodeHost:          0,
		AuthorizationToken:    nil,
		AuthorizationCodeHash: bootstrap_auth_codehash,
		ConfigurationBlob:     nil,
		WorkItems: []types.WorkItem{
			{
				Service:            uint32(statedb.EVMServiceCode),
				CodeHash:           b.service.CodeHash,
				Payload:            buildPayload(PayloadTypeBlocks, numExtrinsics, 0),
				RefineGasLimit:     types.RefineGasAllocation / 2,
				AccumulateGasLimit: types.AccumulationGasAllocation / 2,
				ImportedSegments:   []types.ImportSegment{},
				ExportCount:        0, // Let the BuildBundle determine this
				Extrinsics:         workItems,
			},
		},
	}

	identifier := fmt.Sprintf("blocks-%d-%d", startBlock, endBlock)
	if startBlock == 0 {
		identifier = "genesis"
	}

	bundle, _, err := b.n1.BuildBundle(wp, []types.ExtrinsicsBlobs{blobs}, 0, objects)
	if err != nil {
		return fmt.Errorf("BuildBundle failed for %s: %v", identifier, err)
	}

	// Create work package request
	wpr := &WorkPackageRequest{
		Identifier:      identifier,
		WorkPackage:     bundle.WorkPackage,
		ExtrinsicsBlobs: bundle.ExtrinsicData[0],
	}

	// Submit and wait for completion
	ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries*2)
	_, err = RobustSubmitAndWaitForWorkPackages(ctx, b.n1, []*WorkPackageRequest{wpr})
	cancel()
	if err != nil {
		panic(3333)
		return fmt.Errorf("genesis work package failed: %w", err)
	}

	if isGenesis && len(usdmInitialState) > 0 {
		err = verifyMappings(usdmInitialState)
		if err != nil {
			return fmt.Errorf("storage verification failed after retries: %w", err)
		}
	}

	log.Info(log.Node, "Genesis complete", "chainID", b.n1.GetChainId(), "usdm", usdmAddress.Hex())
	return nil
}

// DeployContract deploys a contract and returns its address
func (b *EVMService) DeployContract(contractFile string) (common.Address, error) {
	// Load contract bytecode from file
	contractBytecode, err := os.ReadFile(contractFile)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to load contract bytecode from %s: %w", contractFile, err)
	}

	// Get deployer account (using issuer account)
	deployerAddress, deployerPrivKeyHex := common.GetEVMDevAccount(0)
	deployerPrivKey, err := crypto.HexToECDSA(deployerPrivKeyHex)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to parse deployer private key: %w", err)
	}

	// Get current nonce for the deployer
	nonce, err := b.n1.GetTransactionCount(deployerAddress, "latest")
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to get transaction count: %w", err)
	}

	// Calculate contract address using CREATE opcode logic: keccak256(rlp([sender, nonce]))[12:]
	contractAddress := crypto.CreateAddress(ethereumCommon.Address(deployerAddress), nonce)

	// Create contract deployment transaction (to = nil for contract creation)
	gasPrice := big.NewInt(1_000_000_000) // 1 Gwei
	gasLimit := uint64(5_000_000)         // Higher gas limit for contract deployment

	ethTx := ethereumTypes.NewContractCreation(
		nonce,
		big.NewInt(0), // value = 0
		gasLimit,
		gasPrice,
		contractBytecode,
	)

	// Sign transaction
	signer := ethereumTypes.NewEIP155Signer(big.NewInt(int64(jamChainID)))
	signedTx, err := ethereumTypes.SignTx(ethTx, signer, deployerPrivKey)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to sign contract deployment transaction: %w", err)
	}

	// Encode to RLP
	txBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to encode contract deployment transaction: %w", err)
	}

	// Submit contract deployment as work package
	deployTxHashes, err := b.SubmitEVMTransactions("contract-deployment", [][]byte{txBytes})
	if err != nil {
		return common.Address{}, fmt.Errorf("contract deployment failed: %w", err)
	}
	if err := b.ShowTxReceipts(deployTxHashes, "Contract Deployment", make(map[common.Hash]string)); err != nil {
		return common.Address{}, fmt.Errorf("failed to show deployment receipts: %w", err)
	}

	// Verify contract deployment by checking if code exists at the calculated address
	deployedCode, err := b.n1.GetCode(common.Address(contractAddress), "latest")
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to get deployed contract code: %w", err)
	}
	if len(deployedCode) == 0 {
		return common.Address{}, fmt.Errorf("contract deployment failed: no code at address %s", contractAddress.Hex())
	}

	log.Info(log.Node, "Contract deployed successfully",
		"file", contractFile,
		"address", contractAddress.Hex(),
		"deployer", deployerAddress.String(),
		"nonce", nonce,
		"codeSize", len(deployedCode))

	return common.Address(contractAddress), nil
}

// CallMath calls math functions on the deployed contract using the provided call strings
func (b *EVMService) CallMath(mathAddress common.Address, callStrings []string) error {
	// Helper: call getCachedFib view function to verify the result
	callGetCachedFib := func(n int) (*big.Int, error) {
		calldata := make([]byte, 36)
		selector, _ := getFunctionSelector("getCachedFib(uint256)", []string{"FibCache(uint256,uint256)", "FibComputed(uint256,uint256)"})
		copy(calldata[0:4], selector[:])
		// Encode n parameter (32 bytes, big-endian)
		nBytes := big.NewInt(int64(n)).FillBytes(make([]byte, 32))
		copy(calldata[4:36], nBytes)

		callResult, err := b.n1.Call(issuerAddress, &mathAddress, 100000, 1000000000, 0, calldata, "latest")
		if err != nil {
			return nil, fmt.Errorf("Call getCachedFib failed: %v", err)
		}

		// Parse the result - should be a 32-byte uint256
		if len(callResult) < 32 {
			return nil, fmt.Errorf("getCachedFib result too short: %d bytes", len(callResult))
		}

		result := new(big.Int).SetBytes(callResult[len(callResult)-32:])
		return result, nil
	}
	_ = callGetCachedFib // Mark as available for future use

	// mathCallSpec defines a mathematical function with its signature and events
	type mathCallSpec struct {
		signature string
		events    []string
	}

	// mathCalls maps function names to their specifications
	mathCalls := map[string]mathCallSpec{
		"fibonacci": {
			signature: "fibonacci(uint256)",
			events:    []string{"FibCache(uint256,uint256)", "FibComputed(uint256,uint256)"},
		},
		"modExp": {
			signature: "modExp(uint256,uint256,uint256)",
			events:    []string{"ModExpCache(uint256,uint256,uint256,uint256)", "ModExpComputed(uint256,uint256,uint256,uint256)"},
		},
		"gcd": {
			signature: "gcd(uint256,uint256)",
			events:    []string{"GcdCache(uint256,uint256,uint256)", "GcdComputed(uint256,uint256,uint256)"},
		},
		"integerSqrt": {
			signature: "integerSqrt(uint256)",
			events:    []string{"IntegerSqrtCache(uint256,uint256)", "IntegerSqrtComputed(uint256,uint256)"},
		},
		"fact": {
			signature: "fact(uint256)",
			events:    []string{"FactCache(uint256,uint256)", "FactComputed(uint256,uint256)"},
		},
		"isPrime": {
			signature: "isPrime(uint256)",
			events:    []string{"IsPrimeCache(uint256,bool)", "IsPrimeComputed(uint256,bool)"},
		},
		"nextPrime": {
			signature: "nextPrime(uint256)",
			events:    []string{"NextPrimeCache(uint256,uint256)", "NextPrimeComputed(uint256,uint256)"},
		},
		"jacobi": {
			signature: "jacobi(uint256,uint256)",
			events:    []string{"JacobiCache(uint256,uint256,int256)", "JacobiComputed(uint256,uint256,int256)"},
		},
		"binomial": {
			signature: "binomial(uint256,uint256)",
			events:    []string{"BinomialCache(uint256,uint256,uint256)", "BinomialComputed(uint256,uint256,uint256)"},
		},
		"isQuadraticResidue": {
			signature: "isQuadraticResidue(uint256,uint256)",
			events:    []string{"IsQuadraticResidueCache(uint256,uint256,bool)", "IsQuadraticResidueComputed(uint256,uint256,bool)"},
		},
		"rsaKeygen": {
			signature: "rsaKeygen(uint256)",
			events:    []string{"RsaKeygenCache(uint256,uint256,uint256)", "RsaKeygenComputed(uint256,uint256,uint256)"},
		},
		"burnsideNecklace": {
			signature: "burnsideNecklace(uint256,uint256)",
			events:    []string{"BurnsideNecklaceCache(uint256,uint256,uint256)", "BurnsideNecklaceComputed(uint256,uint256,uint256)"},
		},
		"fermatFactor": {
			signature: "fermatFactor(uint256)",
			events:    []string{"FermatFactorCache(uint256,uint256,uint256)", "FermatFactorComputed(uint256,uint256,uint256)"},
		},
		"narayana": {
			signature: "narayana(uint256,uint256)",
			events:    []string{"NarayanaCache(uint256,uint256,uint256)", "NarayanaComputed(uint256,uint256,uint256)"},
		},
		"youngTableaux": {
			signature: "youngTableaux(uint256,uint256)",
			events:    []string{"YoungTableauxCache(uint256,uint256,uint256)", "YoungTableauxComputed(uint256,uint256,uint256)"},
		},
	}

	// mathFunctionCall parses a function call string like "fibonacci(3)" and creates calldata
	mathFunctionCall := func(callString string) ([]byte, map[common.Hash]string, error) {
		// Parse function name and parameters
		openParen := strings.Index(callString, "(")
		closeParen := strings.LastIndex(callString, ")")

		if openParen == -1 || closeParen == -1 || closeParen < openParen {
			return nil, nil, fmt.Errorf("invalid call string format: %s", callString)
		}

		funcName := strings.TrimSpace(callString[:openParen])
		paramsStr := strings.TrimSpace(callString[openParen+1 : closeParen])

		// Look up function specification
		spec, exists := mathCalls[funcName]
		if !exists {
			return nil, nil, fmt.Errorf("unknown function: %s", funcName)
		}

		// Parse parameters
		var params []int64
		if paramsStr != "" {
			paramStrs := strings.Split(paramsStr, ",")
			params = make([]int64, len(paramStrs))
			for i, paramStr := range paramStrs {
				paramStr = strings.TrimSpace(paramStr)
				val, err := parseIntParam(paramStr)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to parse parameter %d (%s): %v", i, paramStr, err)
				}
				params[i] = val
			}
		}

		// Get function selector and topic map
		selector, topics := getFunctionSelector(spec.signature, spec.events)
		calldata := make([]byte, 0)
		calldata = append(calldata, selector[:]...)

		// Encode all parameters as 32-byte big-endian values
		for _, param := range params {
			paramBytes := big.NewInt(param).FillBytes(make([]byte, 32))
			calldata = append(calldata, paramBytes[:]...)
		}

		return calldata, topics, nil
	}

	// Helper: create signed transaction that calls a contract method
	createSignedContractCall := func(privateKeyHex string, nonce uint64, to common.Address, calldata []byte, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*EthereumTransaction, []byte, common.Hash, error) {
		// Parse private key
		privateKey, err := crypto.HexToECDSA(privateKeyHex)
		if err != nil {
			return nil, nil, common.Hash{}, err
		}

		// Create transaction to contract
		ethTx := ethereumTypes.NewTransaction(
			nonce,
			ethereumCommon.Address(to),
			big.NewInt(0), // value = 0 for contract call
			gasLimit,
			gasPrice,
			calldata,
		)

		// Sign transaction
		signer := ethereumTypes.NewEIP155Signer(big.NewInt(int64(chainID)))
		signedTx, err := ethereumTypes.SignTx(ethTx, signer, privateKey)
		if err != nil {
			return nil, nil, common.Hash{}, err
		}

		// Encode to RLP
		rlpBytes, err := signedTx.MarshalBinary()
		if err != nil {
			return nil, nil, common.Hash{}, err
		}

		// Calculate transaction hash
		txHash := common.Keccak256(rlpBytes)

		// Parse into EthereumTransaction
		tx, err := parseRawTransactionBytes(rlpBytes)
		if err != nil {
			return nil, nil, common.Hash{}, err
		}

		return tx, rlpBytes, txHash, nil
	}

	log.Info(log.Node, "Starting evmmath calls", "contract", mathAddress.String(), "numCalls", len(callStrings))

	// Get caller account (using issuer account)
	callerAddress := issuerAddress
	_, callerPrivKeyHex := common.GetEVMDevAccount(0)

	// Get initial nonce for the caller
	initialNonce, err := b.n1.GetTransactionCount(callerAddress, "latest")
	if err != nil {
		return fmt.Errorf("failed to get transaction count: %w", err)
	}

	// Build transactions for math calls
	numCalls := len(callStrings)
	txBytes := make([][]byte, numCalls)
	alltopics := defaultTopics()

	for i, callString := range callStrings {
		currentNonce := initialNonce + uint64(i)

		calldata, topics, err := mathFunctionCall(callString)
		if err != nil {
			return fmt.Errorf("failed to create math function call for %s: %w", callString, err)
		}
		// Merge topics into alltopics map
		for hash, sig := range topics {
			alltopics[hash] = sig
		}

		gasPrice := big.NewInt(1_000_000_000) // 1 Gwei
		gasLimit := uint64(1_000_000_000)     // 1B gas limit for complex math calculations

		_, tx, txHash, err := createSignedContractCall(
			callerPrivKeyHex,
			currentNonce,
			mathAddress,
			calldata,
			gasPrice,
			gasLimit,
			jamChainID,
		)
		if err != nil {
			return fmt.Errorf("failed to create evmmath call transaction for %s: %w", callString, err)
		}
		log.Info(log.Node, callString, "txHash", txHash.String())
		txBytes[i] = tx
	}

	// Submit math calls as work package
	txhashes, err := b.SubmitEVMTransactions(fmt.Sprintf("math-calls-%d", numCalls), txBytes)
	if err != nil {
		return fmt.Errorf("evmmath calls failed: %w", err)
	}
	if err := b.ShowTxReceipts(txhashes, "Math Transactions", alltopics); err != nil {
		return fmt.Errorf("ShowTxReceipts ERR: %w", err)
	}

	log.Info(log.Node, "evmmath calls completed successfully", "numCalls", numCalls)
	time.Sleep(6000 * time.Second)
	return nil
}

type TransferTriple struct {
	SenderIndex   int
	ReceiverIndex int
	Amount        *big.Int
}

// createTransferTriple intelligently generates test transfer cases based on round number
// It creates diverse transfer patterns to test different scenarios:
// - Early rounds: issuer (account 0) distributes to other accounts
// - Middle rounds: mix of transfers between non-issuer accounts
// - Last round: intentionally large amounts to test insufficient balance handling
// - Amounts vary by round to create interesting test cases
func createTransferTriplesForRound(roundNum int, txnsPerRound int, isLastRound bool) []TransferTriple {
	const numDevAccounts = 10 // Dev accounts 0-9
	transfers := make([]TransferTriple, 0, txnsPerRound)

	for i := 0; i < txnsPerRound; i++ {
		var sender, receiver int
		var amount *big.Int

		// Last round: intentionally test insufficient balance with huge amounts
		if isLastRound {
			// Send from issuer to various accounts with impossibly large amounts
			// This matches the original batch5 behavior: test insufficient balance handling
			sender = 0
			receiver = (numDevAccounts - 1) - (i % numDevAccounts)
			if receiver == 0 {
				receiver = 1
			}
			// Use the original hardcoded huge amount: 0x7540a0434b17f96f (~8.4e18)
			amount = big.NewInt(int64(0x7540a0434b17f96f))
		} else {
			// Pattern selection for normal rounds based on round number
			switch roundNum % 4 {
			case 0:
				// Round 0, 4, 8...: Issuer distributes to accounts
				sender = 0
				receiver = (i % (numDevAccounts - 1)) + 1
				amount = new(big.Int).Mul(big.NewInt(int64(0x1000000+i+roundNum*0x100)), big.NewInt(1e12))

			case 1:
				// Round 1, 5, 9...: Mix of issuer and secondary transfers
				if i == 0 {
					sender = 0
					receiver = (4 + roundNum) % numDevAccounts
					if receiver == 0 {
						receiver = 1
					}
				} else {
					sender = (i + 1) % numDevAccounts
					if sender == 0 {
						sender = 2
					}
					receiver = (i + 4 + roundNum) % numDevAccounts
				}
				amount = new(big.Int).Mul(big.NewInt(int64(0x2000000+i+roundNum*0x1000)), big.NewInt(1e9))

			case 2:
				// Round 2, 6, 10...: Circular transfers between non-issuer accounts
				sender = ((i + 4 + roundNum) % (numDevAccounts - 1)) + 1
				receiver = ((i + 5 + roundNum) % (numDevAccounts - 1)) + 1
				amount = new(big.Int).Mul(big.NewInt(int64(0x300000+i*100+roundNum*0x10000)), big.NewInt(1e9))

			case 3:
				// Round 3, 7, 11...: Issuer funds accounts
				sender = 0
				receiver = ((7 + i + roundNum) % (numDevAccounts - 1)) + 1
				amount = new(big.Int).Mul(big.NewInt(int64(0x40000>>(i%8))), big.NewInt(1e6))
			}
		}

		// Ensure sender != receiver
		if sender == receiver {
			receiver = (receiver + 1) % numDevAccounts
			if receiver == sender {
				receiver = (receiver + 1) % numDevAccounts
			}
		}

		transfers = append(transfers, TransferTriple{
			SenderIndex:   sender,
			ReceiverIndex: receiver,
			Amount:        amount,
		})
	}

	return transfers
}

// waitForStableAssignment waits until the submitting node is actually assigned to a core
func waitForStableAssignment(n1 JNode) (uint16, error) {
	nodeImpl, ok := n1.(*Node)
	if !ok {
		// For non-Node implementations, we can't check assignments directly
		// Just wait a bit and hope for the best
		time.Sleep(2 * time.Second)
		return 0, nil
	}

	maxAttempts := 15
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}

		slot := common.GetWallClockJCE(fudgeFactorJCE)
		_, assignments := nodeImpl.statedb.CalculateAssignments(slot)

		// Find which core this node is assigned to
		nodeEd25519 := nodeImpl.GetEd25519Key()
		for _, assignment := range assignments {
			if types.Ed25519Key(assignment.Validator.Ed25519.PublicKey()) == nodeEd25519 {
				log.Info(log.Node, "Found valid core assignment", "coreIndex", assignment.CoreIndex, "slot", slot, "attempt", attempt+1)
				return assignment.CoreIndex, nil
			}
		}

		log.Warn(log.Node, "Node not in current assignments, waiting...", "slot", slot, "attempt", attempt+1)
	}

	return 0, fmt.Errorf("node never appeared in core assignments after %d attempts", maxAttempts)
}

// BalanceOf calls the balanceOf(address) function on the USDM contract and validates the result
func (b *EVMService) checkBalanceOf(account common.Address, expectedBalance common.Hash) error {
	usdmAddress := common.HexToAddress("0x01")

	calldataBalanceOf := make([]byte, 36)
	balanceOfSelector, _ := getFunctionSelector("balanceOf(address)", []string{})
	copy(calldataBalanceOf[0:4], balanceOfSelector[:])
	// Encode address parameter (32 bytes, left-padded)
	copy(calldataBalanceOf[16:36], account[:])

	callResult, err := b.n1.Call(account, &usdmAddress, 100000, 1000000000, 0, calldataBalanceOf, "latest")
	if err != nil {
		return fmt.Errorf("Call balanceOf ERR: %v", err)
	}

	// Parse the result - should be a 32-byte uint256
	if len(callResult) < 32 {
		fmt.Printf("warn: Call balanceOf result too short: %d\n", len(callResult))
		return fmt.Errorf("Call balanceOf result too short: %d", len(callResult))
	}

	actualBalance := new(big.Int).SetBytes(callResult[len(callResult)-32:])
	expectedBalanceBig := new(big.Int).SetBytes(expectedBalance[:])
	log.Trace(log.Node, "Call balanceOf result", "address", account.String(), "balance", actualBalance.String(), "expected", expectedBalanceBig.String())
	if actualBalance.Cmp(expectedBalanceBig) != 0 {
		return fmt.Errorf("Call balanceOf MISMATCH: expected %s, actual %s", expectedBalanceBig.String(), actualBalance.String())
	}
	return nil
}

// Nonces calls the nonces(address) function on the USDM contract and validates the result
func (b *EVMService) checkNonces(account common.Address, expectedNonce *big.Int) error {
	usdmAddress := common.HexToAddress("0x01")

	calldataNonces := make([]byte, 36)
	noncesSelector, _ := getFunctionSelector("nonces(address)", []string{})
	copy(calldataNonces[0:4], noncesSelector[:])
	// Encode address parameter (32 bytes, left-padded)
	copy(calldataNonces[16:36], account[:])

	nonceResult, err := b.n1.Call(account, &usdmAddress, 100000, 1000000000, 0, calldataNonces, "latest")
	if err != nil {
		return fmt.Errorf("Call nonces ERR: %v", err)
	}

	// Parse the result - should be a 32-byte uint256
	if len(nonceResult) < 32 {
		return fmt.Errorf("Call nonces result too short: %d", len(nonceResult))
	}

	actualNonce := new(big.Int).SetBytes(nonceResult[len(nonceResult)-32:])
	if actualNonce.Cmp(expectedNonce) != 0 {
		return fmt.Errorf("Call nonces MISMATCH: expected %s, actual %s", expectedNonce.String(), actualNonce.String())
	}
	return nil
}

func (b *EVMService) RunTransfersRound(transfers []TransferTriple) (witnesses map[common.Hash][][]byte, err error) {
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
		balanceHash, err := b.n1.GetBalance(accounts[i], "latest")
		if err != nil {
			log.Error(log.Node, "GetBalance ERR", "account", i, "err", err)
			return nil, err
		}
		balance := new(big.Int).SetBytes(balanceHash.Bytes())
		initialBalances[i] = balance
		totalBefore = new(big.Int).Add(totalBefore, balance)

		nonce, err := b.n1.GetTransactionCount(accounts[i], "latest")
		if err != nil {
			log.Error(log.Node, "GetTransactionCount ERR", "account", i, "err", err)
			return nil, err
		}
		initialNonces[i] = nonce
	}

	// Build transactions for all transfers
	txBytes := make([][]byte, len(transfers))

	// Track nonce per sender
	senderNonces := make(map[int]uint64)
	for _, transfer := range transfers {
		if _, exists := senderNonces[transfer.SenderIndex]; !exists {
			senderNonces[transfer.SenderIndex] = initialNonces[transfer.SenderIndex]
		}
	}

	for idx, transfer := range transfers {
		senderAddr, senderPrivKey := common.GetEVMDevAccount(transfer.SenderIndex)
		recipientAddr, _ := common.GetEVMDevAccount(transfer.ReceiverIndex)

		gasPrice := big.NewInt(1_000_000_000) // 1 Gwei
		gasLimit := uint64(2_000_000)

		currentNonce := senderNonces[transfer.SenderIndex]
		senderNonces[transfer.SenderIndex]++

		_, tx, txHash, err := createSignedUSDMTransfer(
			senderPrivKey,
			currentNonce,
			recipientAddr,
			transfer.Amount,
			gasPrice,
			gasLimit,
			jamChainID,
		)
		if err != nil {
			log.Error(log.Node, "createSignedUSDMTransfer ERR", "idx", idx, "err", err)
			return nil, err
		}

		log.Info(log.Node, "📤 Transfer created",
			"idx", idx,
			"from", fmt.Sprintf("Account[%d](%s)", transfer.SenderIndex, senderAddr.String()),
			"to", fmt.Sprintf("Account[%d](%s)", transfer.ReceiverIndex, recipientAddr.String()),
			"amount", transfer.Amount.String(),
			"nonce", currentNonce,
			"txHash", txHash.String())

		txBytes[idx] = tx
	}

	// Submit multitransfer as work package
	transferTxHashes, err := b.SubmitEVMTransactions(fmt.Sprintf("multitransfer-%d", len(transfers)), txBytes)
	if err != nil {
		log.Error(log.Node, "SubmitEVMTransactions ERR", "err", err)
		return nil, err
	}
	fmt.Printf(fmt.Sprintf("Multitransfer (%d transfers) [%v]", len(transferTxHashes), transferTxHashes))
	/*
		if err := b.ShowTxReceipts(transferTxHashes, fmt.Sprintf("Multitransfer (%d transfers)", len(transfers)), defaultTopics()); err != nil {
			return nil, fmt.Errorf("failed to show transfer receipts: %w", err)
		}
	*/

	// Check balances and nonces after transfers
	totalAfter := big.NewInt(0)
	for i := 0; i < numAccounts; i++ {
		balanceHash, err := b.n1.GetBalance(accounts[i], "latest")
		if err != nil {
			log.Error(log.Node, "GetBalance ERR", "account", i, "err", err)
			return nil, err
		}
		balance := new(big.Int).SetBytes(balanceHash.Bytes())
		totalAfter = new(big.Int).Add(totalAfter, balance)

		nonce, err := b.n1.GetTransactionCount(accounts[i], "latest")
		if err != nil {
			log.Error(log.Node, "GetTransactionCount ERR", "account", i, "err", err)
			return nil, err
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
		log.Trace(log.Node, fmt.Sprintf("  %s", accountName),
			"address", accounts[i].String(),
			"balance", balance.String(),
			"delta", fmt.Sprintf("%s%s", deltaSign, delta.String()),
			"nonce", nonce,
			"nonce_delta", int64(nonce)-int64(initialNonces[i]))
	}

	// Verify conservation of tokens
	if totalBefore.Cmp(totalAfter) == 0 {
		log.Debug(log.Node, "✅ BALANCE CONSERVATION VERIFIED", "total", totalAfter.String())
	} else {
		diff := new(big.Int).Sub(totalAfter, totalBefore)
		log.Error(log.Node, "❌ BALANCE MISMATCH",
			"before", totalBefore.String(),
			"after", totalAfter.String(),
			"difference", diff.String())
		return nil, fmt.Errorf("balance mismatch: before=%s, after=%s, difference=%s", totalBefore.String(), totalAfter.String(), diff.String())
	}

	// Verify issuer balance and nonce via contract calls

	/*
		issuerBalance, err := b.n1.GetBalance(issuerAddress, "latest")
		if err != nil {
			return nil, fmt.Errorf("GetBalance failed: %w", err)
		}

		err = b.checkBalanceOf(issuerAddress, issuerBalance)
		if err != nil {
			return nil, fmt.Errorf("checkBalanceOf failed: %w", err)
		}

		issuerTxCount, err := b.n1.GetTransactionCount(issuerAddress, "latest")
		if err != nil {
			return nil, fmt.Errorf("GetTransactionCount failed: %w", err)
		}
		err = b.checkNonces(issuerAddress, new(big.Int).SetUint64(issuerTxCount))
		if err != nil {
			return nil, fmt.Errorf("checkNonces failed: %w", err)
		}
	*/

	// Verify coinbase collected fees
	coinbaseBalance, err := b.n1.GetBalance(coinbaseAddress, "latest")
	if err != nil {
		return nil, fmt.Errorf("GetBalance (coinbase) failed: %w", err)
	}
	coinbaseBalanceBig := new(big.Int).SetBytes(coinbaseBalance.Bytes())
	if coinbaseBalanceBig.Cmp(big.NewInt(0)) == 0 {
		return nil, fmt.Errorf("coinbase balance is 0 - fee collection failed")
	}
	log.Info(log.Node, "Coinbase balance after round", "address", coinbaseAddress.String(), "balance", coinbaseBalanceBig.String())

	return witnesses, nil
}

// RunTransfersRounds runs multiple rounds of transfers
func (b *EVMService) RunTransfersRounds(numRounds, txnsPerRound int) error {
	for round := 0; round < numRounds; round++ {
		isLastRound := (round == numRounds-1)
		log.Info(log.Node, "test_transfers - round", "round", round, "isLastRound", isLastRound)
		waitForStableAssignment(b.n1)
		_, err := b.RunTransfersRound(createTransferTriplesForRound(round, txnsPerRound, isLastRound))
		if err != nil {
			return fmt.Errorf("transfer round %d failed: %w", round, err)
		}
	}
	return nil
}

func EvmTest(t *testing.T, n1 JNode, testServices map[string]*types.TestService, target string) {
	service, err := NewEVMService(n1, testServices["evm"])
	if err != nil {
		log.Error(log.Node, "NewEVMService ERR", "err", err)
		t.Fatalf("NewEVMService failed: %v", err)
	}

	switch target {
	case "fib":
		for i := 0; i <= 10; i++ {
			if err := service.CallMath(mathAddress, []string{
				"fibonacci(256)",
				"fact(6)",
				// "fact(7)",
				//"nextPrime(100)",
				//"integerSqrt(1000)",
				// "gcd(48,18)",
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
			}); err != nil {
				t.Fatalf("CallMath failed: %v", err)
			}
		}
	case "contracts":
		if _, err := service.DeployContract("../services/evm/contracts/funds.bin"); err != nil {
			t.Fatalf("DeployContract failed: %v", err)
		}
	case "gas":
		if err := service.EstimateGas(issuerAddress, usdmAddress); err != nil {
			t.Fatalf("EstimateGas failed: %v", err)
		}
	case "transfers":
		if err := service.RunTransfersRounds(numRounds, txnsPerRound); err != nil {
			t.Fatalf("RunTransfersRounds failed: %v", err)
		}
	}
}
