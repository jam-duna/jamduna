package statedb

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	storage "github.com/colorfulnotion/jam/storage"
	types "github.com/colorfulnotion/jam/types"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	numRounds    = 100
	txnsPerRound = 1
)

type Rollup struct {
	stateDB            *StateDB
	serviceID          uint32
	previousGuarantees *[]types.Guarantee
	storage            *storage.StateDBStorage
	pvmBackend         string
}

// PayloadType discriminator matching Rust enum
type PayloadType byte

const (
	PayloadTypeBuilder      PayloadType = 0x00
	PayloadTypeTransactions PayloadType = 0x01
	PayloadTypeBlocks       PayloadType = 0x02
	PayloadTypeCall         PayloadType = 0x03
)

// buildPayload constructs a payload byte array for any payload type
func buildPayload(payloadType PayloadType, count int, numWitnesses int) []byte {
	payload := make([]byte, 7)
	payload[0] = byte(payloadType)
	binary.LittleEndian.PutUint32(payload[1:5], uint32(count))
	binary.LittleEndian.PutUint16(payload[5:7], uint16(numWitnesses))
	return payload
}

func NewRollup(testDir string, serviceID uint32) (*Rollup, error) {
	storage, err := initStorage(testDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Create genesis state, Create StateDB
	genesisTrace, err := MakeGenesisStateTransition(storage, 0, "jam", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create genesis state: %w", err)
	}

	statedb, err := NewStateDBFromStateTransitionPost(storage, genesisTrace)
	if err != nil {
		return nil, fmt.Errorf("failed to create StateDB from genesis: %w", err)
	}
	chain := Rollup{
		serviceID:          serviceID,
		stateDB:            statedb,
		storage:            storage,
		previousGuarantees: nil,
		pvmBackend:         BackendInterpreter, // BackendCompiler
	}
	return &chain, nil
}

// processWPQueueItems sets up test environment and processes a array of work packages into a block, with one WP per core
//
//	Technique: use fallback keys to seal, 3 validators sign each guarantee, 6 validators sign each assurance
func (c *Rollup) processWPQueueItems(wpqs []*types.WPQueueItem) error {
	// Generate validator secrets for signing similar to SetupNodes
	validators, validatorSecrets, err := GenerateValidatorSecretSet(types.TotalValidators)
	if err != nil {
		return fmt.Errorf("failed to generate validator secrets: %w", err)
	}
	statedb := c.stateDB
	// Get the  service code hash
	service, ok, err := statedb.GetService(uint32(c.serviceID))
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}
	if !ok {
		return fmt.Errorf("service %d not found", c.serviceID)
	}
	codeHash := service.CodeHash

	// Get bootstrap authorization code hash
	authPvm := common.GetFilePath(BootStrapNullAuthFile)
	authCodeBytes, err := os.ReadFile(authPvm)
	if err != nil {
		return fmt.Errorf("failed to read auth code: %w", err)
	}
	authCode := AuthorizeCode{
		PackageMetaData:   []byte("bootstrap"),
		AuthorizationCode: authCodeBytes,
	}
	authCodeEnc, err := authCode.Encode()
	if err != nil {
		return fmt.Errorf("failed to encode auth code: %w", err)
	}
	bootstrapAuthCodeHash := common.Blake2Hash(authCodeEnc)

	// Add bootstrap authorization to pool for all cores
	authorizerHash := common.Blake2Hash(append(bootstrapAuthCodeHash.Bytes(), []byte(nil)...))
	for i := 0; i < types.TotalCores; i++ {
		statedb.JamState.AuthorizationsPool[i][0] = authorizerHash
	}

	ctx := context.Background()
	guarantees := make([]types.Guarantee, types.TotalCores)
	for coreIndex, wpq := range wpqs {
		if wpq == nil {
			continue
		}
		wp := wpq.WorkPackage
		// Fill in work package fields with current state
		refineCtx := types.RefineContext{
			Anchor:           statedb.HeaderHash,
			StateRoot:        statedb.GetStateRoot(),
			BeefyRoot:        common.Hash{},
			LookupAnchor:     statedb.HeaderHash,
			LookupAnchorSlot: statedb.GetTimeslot(),
			Prerequisites:    []common.Hash{},
		}

		wp.AuthCodeHost = 0
		wp.AuthorizationToken = nil
		wp.AuthorizationCodeHash = bootstrapAuthCodeHash
		wp.ConfigurationBlob = nil
		wp.RefineContext = refineCtx

		// Fill in service and code hash for work items
		for i := range wp.WorkItems {
			wp.WorkItems[i].Service = uint32(c.serviceID)
			wp.WorkItems[i].CodeHash = codeHash
		}

		// Populate ExtrinsicData - one ExtrinsicsBlobs per WorkItem
		// For now, assume single WorkItem per WorkPackage (matches SubmitEVMTransactions pattern)
		extrinsicData := make([]types.ExtrinsicsBlobs, len(wp.WorkItems))
		if len(wp.WorkItems) > 0 {
			// Assign all extrinsics to the first work item
			extrinsicData[0] = wpq.Extrinsics
		}

		// Create work package bundle
		bundle := types.WorkPackageBundle{
			WorkPackage:       wp,
			ExtrinsicData:     extrinsicData,
			ImportSegmentData: [][][]byte{{}},
			Justification:     [][][]common.Hash{{}},
		}

		// Execute refine to get work report, Set work report core index
		workReport, err := statedb.ExecuteWorkPackageBundle(uint16(coreIndex), bundle, types.SegmentRootLookup{}, 0, false, 0, c.pvmBackend)
		if err != nil {
			return fmt.Errorf("failed ExecuteWorkPackageBundle: %v", err)
		}
		workReport.CoreIndex = uint(coreIndex)

		// Create guarantee with 3 validator signatures from validators assigned to core 0
		guarantee := types.Guarantee{
			Report:     workReport,
			Slot:       statedb.GetTimeslot(),
			Signatures: []types.GuaranteeCredential{},
		}
		_, assignments := statedb.CalculateAssignments(statedb.GetTimeslot())
		var coreValidators []uint16
		for idx, assignment := range assignments {
			if assignment.CoreIndex == uint16(coreIndex) {
				coreValidators = append(coreValidators, uint16(idx))
			}
		}

		// Sign with the 3 validators assigned to core
		for i := 0; i < 3 && i < len(coreValidators); i++ {
			validatorIdx := coreValidators[i]
			gc := workReport.Sign(validatorSecrets[validatorIdx].Ed25519Secret[:], validatorIdx)
			guarantee.Signatures = append(guarantee.Signatures, gc)
		}

		// Log block information
		for i, wd := range workReport.Results {
			fmt.Printf("  WorkDigest[%d] (ServiceID=%d): GasUsed=%d, Result=%x (%d bytes)\n", i, wd.ServiceID, wd.GasUsed, wd.Result.Ok, len(wd.Result.Ok))
			if len(wd.Result.Ok) >= 12 {
				export_count := binary.LittleEndian.Uint16(wd.Result.Ok[0:2])
				gas_used := binary.LittleEndian.Uint64(wd.Result.Ok[2:10])
				count := binary.LittleEndian.Uint16(wd.Result.Ok[10:12])
				fmt.Printf("    → Decoded: export_count=%d, gas_used=%d, write_count=%d\n", export_count, gas_used, count)
			}
		}

		guarantees[coreIndex] = guarantee
	}

	// Filter out empty guarantees (from nil wpqs)
	var activeGuarantees []types.Guarantee
	for i := range guarantees {
		if len(guarantees[i].Signatures) > 0 {
			activeGuarantees = append(activeGuarantees, guarantees[i])
		}
	}

	// Create block with guarantee and assurances
	block := types.NewBlock()
	header := types.NewBlockHeader()
	header.ParentHeaderHash = statedb.HeaderHash
	header.ParentStateRoot = statedb.GetStateRoot()
	header.Slot = statedb.GetTimeslot() + 1
	header.AuthorIndex = 0

	// Check if we need an epoch mark for this slot
	safrole := statedb.GetSafrole()
	if safrole.IsNewEpoch(header.Slot) {
		epochMark := safrole.GenerateEpochMarker()
		header.EpochMark = epochMark
	}

	block.Header = *header

	extrinsic := types.NewExtrinsic()
	extrinsic.Guarantees = activeGuarantees

	// Add assurances for previous block guarantee
	if c.previousGuarantees != nil {
		var assurances []types.Assurance
		// Create assurances only for cores that had guarantees
		for coreIndex := range *c.previousGuarantees {
			if len((*c.previousGuarantees)[coreIndex].Signatures) == 0 {
				continue
			}
			for i := 0; i < 6 && i < types.TotalValidators; i++ {
				validatorIdx := uint16(i)
				assurance := types.Assurance{
					Anchor:         header.ParentHeaderHash,
					Bitfield:       [types.Avail_bitfield_bytes]byte{},
					ValidatorIndex: validatorIdx,
				}
				// Set the bit for the core with the previous guarantee
				assurance.SetBitFieldBit(uint16(coreIndex), true)

				// Sign the assurance
				assurance.Sign(validatorSecrets[i].Ed25519Secret[:])

				assurances = append(assurances, assurance)
			}
		}
		extrinsic.Assurances = assurances
	}

	block.Extrinsic = extrinsic

	// Fallback sealing: Find validator index matching the fallback key
	_, currPhase := safrole.EpochAndPhase(header.Slot)
	fallbackKey := safrole.TicketsOrKeys.Keys[currPhase]
	var authorIndex uint16
	var validatorSecret *types.ValidatorSecret
	for i := 0; i < types.TotalValidators; i++ {
		if validators[i].Bandersnatch.Hash() == fallbackKey {
			authorIndex = uint16(i)
			validatorSecret = &validatorSecrets[i]
			break
		}
	}
	if validatorSecret == nil {
		return fmt.Errorf("could not find validator matching fallback key")
	}
	header.AuthorIndex = authorIndex
	block.Header = *header

	blockAuthorPriv, err := ConvertBanderSnatchSecret(validatorSecret.BandersnatchSecret)
	if err != nil {
		return fmt.Errorf("failed to convert bandersnatch secret: %w", err)
	}
	blockAuthorPub, err := ConvertBanderSnatchPub(validatorSecret.BandersnatchPub[:])
	if err != nil {
		return fmt.Errorf("failed to convert bandersnatch pub: %w", err)
	}

	sealedBlock, err := statedb.SealBlockWithEntropy(blockAuthorPub, blockAuthorPriv, authorIndex, header.Slot, block)
	if err != nil {
		return fmt.Errorf("failed to seal block: %w", err)
	}

	// Apply state transition
	newStateDB, err := ApplyStateTransitionFromBlock(0, statedb, ctx, sealedBlock, nil, "interpreter")
	if err != nil {
		return fmt.Errorf("failed to apply state transition: %w", err)
	}

	c.stateDB = newStateDB

	// Pipeline current guarantee for next block's assurances
	c.previousGuarantees = &guarantees

	return nil
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

// CallMath calls math functions on the deployed contract using the provided call strings
func (c *Rollup) CallMath(mathAddress common.Address, callStrings []string) (txBytes [][]byte, err error) {

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
		tx, err := ParseRawTransactionBytes(rlpBytes)
		if err != nil {
			return nil, nil, common.Hash{}, err
		}

		return tx, rlpBytes, txHash, nil
	}

	log.Info(log.Node, "Starting evmmath calls", "contract", mathAddress.String(), "numCalls", len(callStrings))

	// Get caller account (using issuer account)
	callerAddress, callerPrivKeyHex := common.GetEVMDevAccount(0)

	// Get initial nonce for the caller from current state
	initialNonce, err := c.stateDB.GetTransactionCount(callerAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction count for caller %s: %w", callerAddress.String(), err)
	}

	// Build transactions for math calls
	numCalls := len(callStrings)
	txBytes = make([][]byte, numCalls)
	alltopics := defaultTopics()

	for i, callString := range callStrings {
		currentNonce := initialNonce + uint64(i)

		calldata, topics, err := mathFunctionCall(callString)
		if err != nil {
			return nil, fmt.Errorf("failed to create math function call for %s: %w", callString, err)
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
			uint64(c.serviceID),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create evmmath call transaction for %s: %w", callString, err)
		}
		log.Info(log.Node, callString, "txHash", txHash.String())
		txBytes[i] = tx
	}

	return txBytes, nil
}

// SubmitEVMTransactions creates and submits a work package with raw transactions
func (b *Rollup) SubmitEVMTransactions(evmTxsMulticore [][][]byte) ([]*types.WPQueueItem, error) {
	wpqs := make([]*types.WPQueueItem, len(evmTxsMulticore))
	for coreIndex, evmTxs := range evmTxsMulticore {
		// Create extrinsics blobs and hashes from raw transactions
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
		service, ok, err := b.stateDB.GetService(b.serviceID)
		if err != nil || !ok {
			return nil, fmt.Errorf("EVM service not found: %v", err)
		}

		// Create work package
		wp := types.WorkPackage{
			AuthCodeHost:       0,
			AuthorizationToken: nil,
			ConfigurationBlob:  nil,
			WorkItems: []types.WorkItem{
				{
					Service:            b.serviceID,
					CodeHash:           service.CodeHash,
					Payload:            buildPayload(PayloadTypeTransactions, len(evmTxs), 0),
					RefineGasLimit:     types.RefineGasAllocation,
					AccumulateGasLimit: types.AccumulationGasAllocation,
					ImportedSegments:   []types.ImportSegment{},
					ExportCount:        0, // Let the BuildBundle determine this
					Extrinsics:         hashes,
				},
			},
		}

		// bundle, _, err := b.BuildBundle(wp, []types.ExtrinsicsBlobs{extrinsics}, 0, nil)
		// if err != nil {
		// 	return nil, fmt.Errorf("BuildBundle failed for %s: %v", identifier, err)
		// }
		wpqs[coreIndex] = &types.WPQueueItem{
			WorkPackage: wp,
			Extrinsics:  types.ExtrinsicsBlobs(evmTxs),
		}
	}

	// if err := c.ShowTxReceipts(txhashes, "Math Transactions", alltopics); err != nil {
	// 	return nil, fmt.Errorf("ShowTxReceipts ERR: %w", err)
	// }

	return wpqs, nil
}

// ShowTxReceipts displays transaction receipts for the given transaction hashes
func (b *Rollup) ShowTxReceipts(txHashes []common.Hash, description string, allTopics map[common.Hash]string) error {
	log.Info(log.Node, "Showing transaction receipts", "description", description, "count", len(txHashes))

	gasUsedTotal := big.NewInt(0)

	for _, txHash := range txHashes {
		var (
			receipt *EthereumTransactionReceipt
			err     error
		)
		receipt, err = b.stateDB.GetTransactionReceipt(b.serviceID, txHash)
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

// // buildPayloadTransaction creates a conformant PayloadTransaction payload
// // Format: "T" (1 byte) + tx_count (u32 LE) + witness_count (u16 LE)
func (b *Rollup) ExportStateWitnesses(workReports []*types.WorkReport, saveToFile bool) error {
	witnesses, stateRoot, err := b.stateDB.GetStateWitnesses(workReports)
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

func (b *Rollup) SubmitEVMPayloadBlocks(startBlock uint32, endBlock uint32) (*types.WPQueueItem, error) {
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

	blobs := types.ExtrinsicsBlobs{}
	workItems := []types.WorkItemExtrinsic{}

	// Only include genesis USDM mappings if submitting genesis block
	isGenesis := startBlock == 0 && endBlock == 1
	if isGenesis {
		// Set USDM initial balances and nonces
		totalSupplyValue := new(big.Int).Mul(big.NewInt(61_000_000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		usdmInitialState := []MappingEntry{
			{Slot: 0, Key: IssuerAddress, Value: totalSupplyValue}, // balanceOf[issuer]
			{Slot: 1, Key: IssuerAddress, Value: big.NewInt(1)},    // nonces[issuer]
		}
		initializeMappings(&blobs, &workItems, UsdmAddress, usdmInitialState)
	}
	numExtrinsics := len(workItems)

	objects := []common.Hash{GetBlockNumberKey()}
	for blockNum := startBlock; blockNum < endBlock; blockNum++ {
		objects = append(objects, BlockNumberToObjectID(blockNum))
	}
	service, ok, err := b.stateDB.GetService(b.serviceID)
	if err != nil || !ok {
		return nil, fmt.Errorf("EVM service not found: %v", err)
	}

	// blockNumberKeyWitness, ok, stateRoot, err := b.ReadStateWitnessRaw(RollupCode, GetBlockNumberKey())
	// blockWitness, ok, blockStateRoot, err := b.ReadStateWitnessRaw(RollupCode, blockObjectID)
	// Create work package
	wp := types.WorkPackage{
		AuthCodeHost:       0,
		AuthorizationToken: nil,
		ConfigurationBlob:  nil,
		WorkItems: []types.WorkItem{
			{
				Service:            uint32(b.serviceID),
				CodeHash:           service.CodeHash,
				Payload:            buildPayload(PayloadTypeBlocks, numExtrinsics, 0),
				RefineGasLimit:     types.RefineGasAllocation / 2,
				AccumulateGasLimit: types.AccumulationGasAllocation / 2,
				ImportedSegments:   []types.ImportSegment{},
				ExportCount:        0, // Let the BuildBundle determine this
				Extrinsics:         workItems,
			},
		},
	}

	// bundle, _, err := b.BuildBundle(wp, []types.ExtrinsicsBlobs{blobs}, 0, objects)
	// if err != nil {
	// 	return fmt.Errorf("BuildBundle failed for %s: %v", identifier, err)
	// }

	wpq := &types.WPQueueItem{
		WorkPackage: wp,
		Extrinsics:  blobs,
	}

	// Note: Genesis verification should be performed AFTER processWPQueueItems has been called
	// to allow the bootstrap extrinsics to execute and apply state changes.
	// Verification can be done by calling b.stateDB.GetStorageAt() to check expected values.

	return wpq, nil
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
func (b *Rollup) createTransferTriplesForRound(roundNum int, txnsPerRound int, isLastRound bool) []TransferTriple {
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

// DeployContract deploys a contract and returns its address
func (b *Rollup) DeployContract(contractFile string) ([]*types.WPQueueItem, common.Address, error) {
	// Load contract bytecode from file
	contractBytecode, err := os.ReadFile(contractFile)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("failed to load contract bytecode from %s: %w", contractFile, err)
	}

	// Get deployer account (using issuer account)
	deployerAddress, deployerPrivKeyHex := common.GetEVMDevAccount(0)
	deployerPrivKey, err := crypto.HexToECDSA(deployerPrivKeyHex)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("failed to parse deployer private key: %w", err)
	}

	// Get current nonce for the deployer
	nonce, err := b.stateDB.GetTransactionCount(deployerAddress)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("failed to get transaction count: %w", err)
	}

	// Calculate contract address using CREATE opcode logic: keccak256(rlp([sender, nonce]))[12:]
	contractAddress := common.Address(crypto.CreateAddress(ethereumCommon.Address(deployerAddress), nonce))
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
	signer := ethereumTypes.NewEIP155Signer(big.NewInt(int64(b.serviceID)))
	signedTx, err := ethereumTypes.SignTx(ethTx, signer, deployerPrivKey)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("failed to sign contract deployment transaction: %w", err)
	}

	// Encode to RLP
	txBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("failed to encode contract deployment transaction: %w", err)
	}
	multiCoreTxBytes := make([][][]byte, 1)
	multiCoreTxBytes[0] = [][]byte{txBytes}
	// Submit contract deployment as work package
	wpqs, err := b.SubmitEVMTransactions(multiCoreTxBytes)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("contract deployment failed: %w", err)
	}

	return wpqs, contractAddress, nil
}

// BalanceOf calls the balanceOf(address) function on the USDM contract and validates the result
func (b *Rollup) checkBalanceOf(account common.Address, expectedBalance common.Hash) error {
	usdmAddress := common.HexToAddress("0x01")

	calldataBalanceOf := make([]byte, 36)
	balanceOfSelector, _ := getFunctionSelector("balanceOf(address)", []string{})
	copy(calldataBalanceOf[0:4], balanceOfSelector[:])
	// Encode address parameter (32 bytes, left-padded)
	copy(calldataBalanceOf[16:36], account[:])

	callResult, err := b.stateDB.Call(account, &usdmAddress, 100000, 1000000000, 0, calldataBalanceOf, "latest", b.pvmBackend)
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
func (b *Rollup) checkNonces(account common.Address, expectedNonce *big.Int) error {
	usdmAddress := common.HexToAddress("0x01")

	calldataNonces := make([]byte, 36)
	noncesSelector, _ := getFunctionSelector("nonces(address)", []string{})
	copy(calldataNonces[0:4], noncesSelector[:])
	// Encode address parameter (32 bytes, left-padded)
	copy(calldataNonces[16:36], account[:])

	nonceResult, err := b.stateDB.Call(account, &usdmAddress, 100000, 1000000000, 0, calldataNonces, "latest", b.pvmBackend)
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
