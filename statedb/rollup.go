package statedb

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	storage "github.com/colorfulnotion/jam/storage"
	types "github.com/colorfulnotion/jam/types"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	numRounds    = 100
	txnsPerRound = 1

	verifyServiceProof = false
)

// Lazy initialization for bootstrap auth code hash
var (
	bootstrap_auth_codehash common.Hash
	bootstrapAuthOnce       sync.Once
)

// getBootstrapAuthCodeHash computes the bootstrap auth code hash lazily
func getBootstrapAuthCodeHash() common.Hash {
	bootstrapAuthOnce.Do(func() {
		// Only compute when actually needed
		authFilePath, err := common.GetFilePath(BootStrapNullAuthFile)
		if err != nil {
			log.Warn(log.SDB, "Failed to get bootstrap auth file path, using zero hash", "err", err)
			bootstrap_auth_codehash = common.Hash{}
			return
		}
		auth_code_bytes, err := os.ReadFile(authFilePath)
		if err != nil {
			log.Warn(log.SDB, "Failed to read bootstrap null auth file, using zero hash", "err", err)
			bootstrap_auth_codehash = common.Hash{}
			return
		}
		auth_code := AuthorizeCode{
			PackageMetaData:   []byte("bootstrap"),
			AuthorizationCode: auth_code_bytes,
		}
		auth_code_encoded_bytes, err := auth_code.Encode()
		if err != nil {
			log.Warn(log.SDB, "Failed to encode bootstrap auth code, using zero hash", "err", err)
			bootstrap_auth_codehash = common.Hash{}
			return
		}
		bootstrap_auth_codehash = common.Blake2Hash(auth_code_encoded_bytes)
	})
	return bootstrap_auth_codehash
}

type Rollup struct {
	stateDB            *StateDB
	serviceID          uint32
	previousGuarantees *[]types.Guarantee
	storage            *storage.StateDBStorage
	pvmBackend         string
}

// DefaultWorkPackage creates a work package with common default values
// Caller should override fields as needed for their specific use case
func DefaultWorkPackage(serviceID uint32, service *types.ServiceAccount) types.WorkPackage {
	return types.WorkPackage{
		AuthCodeHost:          0,
		AuthorizationCodeHash: getBootstrapAuthCodeHash(),
		AuthorizationToken:    nil,
		ConfigurationBlob:     nil,
		RefineContext:         types.RefineContext{}, // Caller should set this
		WorkItems: []types.WorkItem{
			{
				Service:            serviceID,
				CodeHash:           service.CodeHash,
				RefineGasLimit:     types.RefineGasAllocation,
				AccumulateGasLimit: types.AccumulationGasAllocation,
				ImportedSegments:   []types.ImportSegment{},
				ExportCount:        0,
			},
		},
	}
}

// PayloadType discriminator matching Rust enum
type PayloadType byte

const (
	PayloadTypeBuilder      PayloadType = 0x00
	PayloadTypeTransactions PayloadType = 0x01
	PayloadTypeGenesis      PayloadType = 0x02
	PayloadTypeCall         PayloadType = 0x03
)

// BuildPayload constructs a payload byte array for any payload type
func BuildPayload(payloadType PayloadType, count int, numWitnesses int) []byte {
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

// processWorkPackageBundles processes work packages in non-pipelined mode
// Block N: guarantees, Block N+1: assurances (two blocks per work package)
func (c *Rollup) processWorkPackageBundles(bundles []*types.WorkPackageBundle) error {
	validators, validatorSecrets, bootstrapAuthCodeHash, err := c.setupValidators()
	if err != nil {
		return err
	}

	// Execute bundles and create guarantees
	_, activeGuarantees, err := c.executeAndGuarantee(bundles, validatorSecrets, c.stateDB)
	if err != nil {
		return err
	}

	// Block 1: guarantees only
	extrinsic := types.NewExtrinsic()
	extrinsic.Guarantees = activeGuarantees
	if err := c.buildAndApplyBlock(context.Background(), validators, validatorSecrets, bootstrapAuthCodeHash, &extrinsic); err != nil {
		return err
	}

	// Block 2: assurances only
	extrinsic = types.NewExtrinsic()
	extrinsic.Assurances = c.createAssurances(0, validatorSecrets, c.stateDB)
	if err := c.buildAndApplyBlock(context.Background(), validators, validatorSecrets, bootstrapAuthCodeHash, &extrinsic); err != nil {
		return err
	}

	c.previousGuarantees = nil
	return nil
}

// processWorkPackageBundlesPipelined processes work packages in pipelined mode
// Block N contains: guarantees for current work + assurances for PREVIOUS block's work
// TODO: set up assurances bitfields correctly
func (c *Rollup) processWorkPackageBundlesPipelined(bundles []*types.WorkPackageBundle) error {
	validators, validatorSecrets, bootstrapAuthCodeHash, err := c.setupValidators()
	if err != nil {
		return err
	}

	// Execute bundles and create guarantees for current work
	guarantees, activeGuarantees, err := c.executeAndGuarantee(bundles, validatorSecrets, c.stateDB)
	if err != nil {
		return err
	}

	// Build extrinsic with current guarantees and previous assurances
	extrinsic := types.NewExtrinsic()
	extrinsic.Guarantees = activeGuarantees
	if c.previousGuarantees != nil {
		extrinsic.Assurances = c.createAssurancesForGuarantees(c.previousGuarantees, validatorSecrets, c.stateDB)
	}

	if err := c.buildAndApplyBlock(context.Background(), validators, validatorSecrets, bootstrapAuthCodeHash, &extrinsic); err != nil {
		return err
	}

	// Save guarantees for next block's assurances
	c.previousGuarantees = &guarantees
	return nil
}

// setupValidators generates validator secrets and bootstrap auth code hash
func (c *Rollup) setupValidators() ([]types.Validator, []types.ValidatorSecret, common.Hash, error) {
	validators, validatorSecrets, err := GenerateValidatorSecretSet(types.TotalValidators)
	if err != nil {
		return nil, nil, common.Hash{}, fmt.Errorf("failed to generate validator secrets: %w", err)
	}

	bootstrapAuthCodeHash, err := c.getBootstrapAuthCodeHash()
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	return validators, validatorSecrets, bootstrapAuthCodeHash, nil
}

// getBootstrapAuthCodeHash reads and hashes the bootstrap authorization code
func (c *Rollup) getBootstrapAuthCodeHash() (common.Hash, error) {
	authPvm, err := common.GetFilePath(BootStrapNullAuthFile)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get auth file path: %w", err)
	}
	authCodeBytes, err := os.ReadFile(authPvm)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read auth code: %w", err)
	}
	authCode := AuthorizeCode{
		PackageMetaData:   []byte("bootstrap"),
		AuthorizationCode: authCodeBytes,
	}
	authCodeEnc, err := authCode.Encode()
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to encode auth code: %w", err)
	}
	return common.Blake2Hash(authCodeEnc), nil
}

// executeAndGuarantee executes bundles and creates signed guarantees
func (c *Rollup) executeAndGuarantee(bundles []*types.WorkPackageBundle, validatorSecrets []types.ValidatorSecret, statedb *StateDB) ([]types.Guarantee, []types.Guarantee, error) {
	guarantees := make([]types.Guarantee, types.TotalCores)
	activeGuarantees := make([]types.Guarantee, 0)

	for coreIndex, bundle := range bundles {
		if bundle == nil {
			continue
		}

		workReport, err := statedb.ExecuteWorkPackageBundle(uint16(coreIndex), *bundle, types.SegmentRootLookup{}, 0, log.OtherGuarantor, 0, c.pvmBackend)
		if err != nil {
			return nil, nil, fmt.Errorf("failed ExecuteWorkPackageBundle: %v", err)
		}
		workReport.CoreIndex = uint(coreIndex)

		// Create guarantee with validator signatures
		guarantee := c.createGuarantee(workReport, uint16(coreIndex), validatorSecrets, statedb)
		guarantees[coreIndex] = guarantee

		if len(guarantee.Signatures) > 0 {
			activeGuarantees = append(activeGuarantees, guarantee)
		}
	}

	return guarantees, activeGuarantees, nil
}

// createGuarantee creates a guarantee with 3 validator signatures
func (c *Rollup) createGuarantee(workReport types.WorkReport, coreIndex uint16, validatorSecrets []types.ValidatorSecret, statedb *StateDB) types.Guarantee {
	guarantee := types.Guarantee{
		Report:     workReport,
		Slot:       statedb.GetTimeslot(),
		Signatures: []types.GuaranteeCredential{},
	}

	_, assignments := statedb.CalculateAssignments(statedb.GetTimeslot())
	var coreValidators []uint16
	for idx, assignment := range assignments {
		if assignment.CoreIndex == coreIndex {
			coreValidators = append(coreValidators, uint16(idx))
		}
	}

	// Sign with the 3 validators assigned to core
	for i := 0; i < 3 && i < len(coreValidators); i++ {
		validatorIdx := coreValidators[i]
		gc := workReport.Sign(validatorSecrets[validatorIdx].Ed25519Secret[:], validatorIdx)
		guarantee.Signatures = append(guarantee.Signatures, gc)
	}

	return guarantee
}

// createAssurances creates assurances for a core
func (c *Rollup) createAssurances(coreIndex uint16, validatorSecrets []types.ValidatorSecret, statedb *StateDB) []types.Assurance {
	assurances := make([]types.Assurance, 0)

	for i := 0; i < 6 && i < types.TotalValidators; i++ {
		assurance := types.Assurance{
			Anchor:         statedb.HeaderHash,
			Bitfield:       [types.Avail_bitfield_bytes]byte{},
			ValidatorIndex: uint16(i),
		}
		assurance.SetBitFieldBit(coreIndex, true)
		assurance.Sign(validatorSecrets[i].Ed25519Secret[:])
		assurances = append(assurances, assurance)
	}

	return assurances
}

// createAssurancesForGuarantees creates assurances for multiple guarantees
func (c *Rollup) createAssurancesForGuarantees(guarantees *[]types.Guarantee, validatorSecrets []types.ValidatorSecret, statedb *StateDB) []types.Assurance {
	assurances := make([]types.Assurance, 0)

	for coreIndex := range *guarantees {
		if len((*guarantees)[coreIndex].Signatures) == 0 {
			continue
		}
		for i := 0; i < 6 && i < types.TotalValidators; i++ {
			assurance := types.Assurance{
				Anchor:         statedb.HeaderHash,
				Bitfield:       [types.Avail_bitfield_bytes]byte{},
				ValidatorIndex: uint16(i),
			}
			assurance.SetBitFieldBit(uint16(coreIndex), true)
			assurance.Sign(validatorSecrets[i].Ed25519Secret[:])
			assurances = append(assurances, assurance)
		}
	}

	return assurances
}

// buildAndApplyBlock builds and applies a block with the given extrinsic
func (c *Rollup) buildAndApplyBlock(ctx context.Context, validators []types.Validator, validatorSecrets []types.ValidatorSecret, bootstrapAuthCodeHash common.Hash, extrinsic *types.ExtrinsicData) error {
	statedb := c.stateDB
	targetJCE := statedb.GetTimeslot() + 1

	// Find authorized block refiner
	validatorSecret, err := c.findAuthorizedValidator(validators, validatorSecrets, targetJCE)
	if err != nil {
		return err
	}

	// Build block
	sealedBlock, _ := statedb.BuildBlock(ctx, *validatorSecret, targetJCE, common.Hash{}, extrinsic)

	// Add bootstrap authorization to pool for all cores
	authorizerHash := common.Blake2Hash(append(bootstrapAuthCodeHash.Bytes(), []byte(nil)...))
	for i := 0; i < types.TotalCores; i++ {
		statedb.JamState.AuthorizationsPool[i][0] = authorizerHash
	}

	// Apply state transition
	newStateDB, err := ApplyStateTransitionFromBlock(0, statedb, ctx, sealedBlock, nil, "interpreter")
	if err != nil {
		return fmt.Errorf("failed to apply state transition: %w", err)
	}

	c.stateDB = newStateDB
	return nil
}

// findAuthorizedValidator finds a validator authorized to build a block
func (c *Rollup) findAuthorizedValidator(validators []types.Validator, validatorSecrets []types.ValidatorSecret, targetJCE uint32) (*types.ValidatorSecret, error) {
	sf0, _ := c.stateDB.GetPosteriorSafroleEntropy(targetJCE)

	for i := 0; i < types.TotalValidators; i++ {
		isAuthorized, _, _, _ := sf0.IsAuthorizedBuilder(targetJCE, common.Hash(validators[i].Bandersnatch), []common.Hash{})
		if isAuthorized {
			return &validatorSecrets[i], nil
		}
	}

	return nil, fmt.Errorf("could not find validator matching fallback key")
}

// Function aliases for compatibility
var getFunctionSelector = evmtypes.GetFunctionSelector
var defaultTopics = evmtypes.DefaultTopics

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
func (c *Rollup) CallMath(mathAddress common.Address, callStrings []string) (txBytes [][]byte, alltopics map[common.Hash]string, err error) {

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
	createSignedContractCall := func(privateKeyHex string, nonce uint64, to common.Address, calldata []byte, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
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
		tx, err := evmtypes.ParseRawTransactionBytes(rlpBytes)
		if err != nil {
			return nil, nil, common.Hash{}, err
		}

		return tx, rlpBytes, txHash, nil
	}

	log.Info(log.Node, "Starting evmmath calls", "contract", mathAddress.String(), "numCalls", len(callStrings))

	// Get caller account (using issuer account)
	callerAddress, callerPrivKeyHex := common.GetEVMDevAccount(0)

	// Get initial nonce for the caller from current state
	initialNonce, err := c.stateDB.GetTransactionCount(c.serviceID, callerAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get transaction count for caller %s: %w", callerAddress.String(), err)
	}

	// Build transactions for math calls
	numCalls := len(callStrings)
	txBytes = make([][]byte, numCalls)
	alltopics = defaultTopics()

	for i, callString := range callStrings {
		currentNonce := initialNonce + uint64(i)

		calldata, topics, err := mathFunctionCall(callString)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create math function call for %s: %w", callString, err)
		}
		// Merge topics into alltopics map
		for hash, sig := range topics {
			alltopics[hash] = sig
		}

		gasPrice := big.NewInt(1)         // 1 wei
		gasLimit := uint64(1_000_000_000) // 1B gas limit for complex math calculations

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
			return nil, nil, fmt.Errorf("failed to create evmmath call transaction for %s: %w", callString, err)
		}
		log.Info(log.Node, callString, "txHash", txHash.String())
		txBytes[i] = tx
	}

	return txBytes, alltopics, nil
}

// SubmitEVMTransactions creates and submits a work package with raw transactions, processes it, and returns the resulting block
func (b *Rollup) SubmitEVMTransactions(evmTxsMulticore [][][]byte) ([]*types.WorkPackageBundle, error) {
	bundles := make([]*types.WorkPackageBundle, len(evmTxsMulticore))

	for coreIndex, evmTxs := range evmTxsMulticore {
		if len(evmTxs) == 0 {
			bundles[coreIndex] = nil
			continue
		}

		// Create extrinsics blobs with transaction extrinsics only
		numTxExtrinsics := len(evmTxs)
		blobs := make(types.ExtrinsicsBlobs, numTxExtrinsics)
		hashes := make([]types.WorkItemExtrinsic, numTxExtrinsics)

		// Add transaction extrinsics
		for i, tx := range evmTxs {
			blobs[i] = tx
			hashes[i] = types.WorkItemExtrinsic{
				Hash: common.Blake2Hash(tx),
				Len:  uint32(len(tx)),
			}
		}

		service, ok, err := b.stateDB.GetService(b.serviceID)
		if err != nil || !ok {
			return nil, fmt.Errorf("EVM service not found: %v", err)
		}

		// Create work package
		wp := DefaultWorkPackage(b.serviceID, service)
		wp.WorkItems[0].Payload = BuildPayload(PayloadTypeTransactions, numTxExtrinsics, 0)
		wp.WorkItems[0].Extrinsics = hashes

		//  BuildBundle should return a Bundle (with ImportedSegments)
		bundle2, _, err := b.stateDB.BuildBundle(wp, []types.ExtrinsicsBlobs{blobs}, uint16(coreIndex), nil, b.pvmBackend)
		if err != nil {
			return nil, fmt.Errorf("BuildBundle failed: %v", err)
		}
		bundles[coreIndex] = bundle2
	}

	// Process the bundles
	err := b.processWorkPackageBundles(bundles)
	if err != nil {
		return nil, fmt.Errorf("processWorkPackageBundles failed: %w", err)
	}

	return bundles, nil
}

// ShowTxReceipts displays transaction receipts for the given transaction hashes
func (b *Rollup) ShowTxReceipts(evmBlock *evmtypes.EvmBlockPayload, txHashes []common.Hash, description string, allTopics map[common.Hash]string) error {
	log.Info(log.Node, "Showing transaction receipts", "description", description, "count", len(txHashes))

	gasUsedTotal := big.NewInt(0)
	txIndexByHash := make(map[common.Hash]int, len(evmBlock.TxHashes))
	for idx, hash := range evmBlock.TxHashes {
		txIndexByHash[hash] = idx
		fmt.Printf("ADDED txIndexByHash: %s -> %d\n", hash.String(), idx)
	}
	//	receiptCount := len(evmBlock.ReceiptHashes)

	for _, txHash := range txHashes {
		receipt, err := b.stateDB.GetTransactionReceipt(b.serviceID, txHash)
		if err != nil {
			return fmt.Errorf("failed to get transaction receipt for %s: %w", txHash.String(), err)
		}

		log.Info(log.Node, "✅ Transaction succeeded",
			"txHash", txHash.String(),
			"index", receipt.TransactionIndex,
			"gasUsed", receipt.UsedGas)
		//evmtypes.ShowEthereumLogs(txHash, receipt.Logs, allTopics)
		// if verifyServiceProof {
		// 	position := evmBlock.LogIndexStart + txIndex
		// 	proof, err := b.stateDB.GenerateServiceProof(b.serviceID, b.stateDB.GetMMRStorageKey(), position, evmBlock.LogIndexStart, evmBlock.ReceiptHashes)
		// 	if err != nil {
		// 		log.Info(log.Node, "No receipt inclusion proof available", "position", position, "txHash", common.Str(txHash), "err", err)
		// 	} else if proof.Verify() {
		// 		log.Info(log.Node, "✓ Receipt inclusion proven via MMR", "position", position, "txHash", common.Str(txHash))
		// 	} else {
		// 		log.Warn(log.Node, "✗ Receipt inclusion proof verification failed", "position", position, "txHash", common.Str(txHash))
		// 	}
		// }

	}
	log.Info(log.Node, description, "txCount", len(txHashes), "gasUsedTotal", gasUsedTotal.String())
	return nil
}

func AppendBootstrapExtrinsic(blobs *types.ExtrinsicsBlobs, workItems *[]types.WorkItemExtrinsic, extrinsic []byte) {
	*blobs = append(*blobs, extrinsic)
	*workItems = append(*workItems, types.WorkItemExtrinsic{
		Hash: common.Blake2Hash(extrinsic),
		Len:  uint32(len(extrinsic)),
	})
}

// Helper: build K extrinsic for genesis storage initialization
// Format: [address:20][storage_key:32][value:32] = 84 bytes (no command byte)
func BuildKExtrinsic(address []byte, storageKey []byte, storageValue []byte) []byte {
	extrinsic := make([]byte, 20+32+32)
	copy(extrinsic[0:20], address)
	copy(extrinsic[20:52], storageKey)
	copy(extrinsic[52:84], storageValue)
	return extrinsic
}

// Helper: initialize Solidity mapping storage for the Genesis case
func InitializeMappings(blobs *types.ExtrinsicsBlobs, workItems *[]types.WorkItemExtrinsic, contractAddr common.Address, entries []MappingEntry) {
	for _, entry := range entries {
		// Compute Solidity mapping storage key: keccak256(abi.encode(key, slot))
		keyInput := make([]byte, 64)
		copy(keyInput[12:32], entry.Key[:])
		keyInput[63] = entry.Slot
		storageKey := common.Keccak256(keyInput)

		// Encode value as 32-byte big-endian
		valueBytes := entry.Value.FillBytes(make([]byte, 32))

		AppendBootstrapExtrinsic(blobs, workItems, BuildKExtrinsic(contractAddr[:], storageKey[:], valueBytes))
	}
}

// MappingEntry represents a single mapping entry to initialize
type MappingEntry struct {
	Slot  uint8
	Key   common.Address
	Value *big.Int
}

func (b *Rollup) SubmitEVMGenesis(startBalance int64) (*evmtypes.EvmBlockPayload, error) {
	// Set USDM initial balances and nonces
	blobs := types.ExtrinsicsBlobs{}
	workItems := []types.WorkItemExtrinsic{}
	totalSupplyValue := new(big.Int).Mul(big.NewInt(startBalance), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	usdmInitialState := []MappingEntry{
		{Slot: 0, Key: evmtypes.IssuerAddress, Value: totalSupplyValue}, // balanceOf[issuer]
		{Slot: 1, Key: evmtypes.IssuerAddress, Value: big.NewInt(1)},    // nonces[issuer]
	}
	InitializeMappings(&blobs, &workItems, evmtypes.UsdmAddress, usdmInitialState)

	numExtrinsics := len(workItems)
	log.Info(log.Node, "SubmitEVMGenesis (genesis)", "numExtrinsics", numExtrinsics)

	service, ok, err := b.stateDB.GetService(b.serviceID)
	if err != nil || !ok {
		return nil, fmt.Errorf("EVM service not found: %v", err)
	}

	// Create work package with updated witness count and refine context
	wp := DefaultWorkPackage(b.serviceID, service)
	wp.RefineContext = b.stateDB.GetRefineContext()
	wp.WorkItems[0].Payload = BuildPayload(PayloadTypeGenesis, numExtrinsics, 0)
	wp.WorkItems[0].Extrinsics = workItems

	// Genesis exports: Storage SSRs + MetaShard + MetaSSR + Block
	// The exact count depends on how many storage shards are created
	wp.WorkItems[0].ExportCount = uint16(4)
	/*
		1 SSR metadata object (13 bytes)
		1 Storage shard (162 bytes: 2 entries × 64 bytes + overhead)
		1 Block object (124 bytes)
		1 Meta-shard (249 bytes: 3 entries × 69 bytes + overhead)
	*/
	log.Info(log.Node, "WorkPackage RefineContext", "state_root", wp.RefineContext.StateRoot.Hex(), "anchor", wp.RefineContext.Anchor.Hex())
	bundle := &types.WorkPackageBundle{
		WorkPackage:   wp,
		ExtrinsicData: []types.ExtrinsicsBlobs{blobs},
	}

	err = b.processWorkPackageBundles([]*types.WorkPackageBundle{bundle})
	if err != nil {
		return nil, fmt.Errorf("processWorkPackageBundles failed: %w", err)
	}

	// Genesis bootstrap complete - return nil block (genesis doesn't produce transactions)
	return nil, nil
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
func (b *Rollup) DeployContract(contractFile string) (common.Address, error) {
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
	nonce, err := b.stateDB.GetTransactionCount(b.serviceID, deployerAddress)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to get transaction count: %w", err)
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
		return common.Address{}, fmt.Errorf("failed to sign contract deployment transaction: %w", err)
	}

	// Encode to RLP
	txBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to encode contract deployment transaction: %w", err)
	}
	multiCoreTxBytes := make([][][]byte, 1)
	multiCoreTxBytes[0] = [][]byte{txBytes}
	// Submit contract deployment as work package
	_, err = b.SubmitEVMTransactions(multiCoreTxBytes)
	if err != nil {
		return common.Address{}, fmt.Errorf("contract deployment failed: %w", err)
	}

	return contractAddress, nil
}

// BalanceOf calls the balanceOf(address) function on the USDM contract and validates the result
func (b *Rollup) checkBalanceOf(account common.Address, expectedBalance common.Hash) error {
	usdmAddress := common.HexToAddress("0x01")

	calldataBalanceOf := make([]byte, 36)
	balanceOfSelector, _ := getFunctionSelector("balanceOf(address)", []string{})
	copy(calldataBalanceOf[0:4], balanceOfSelector[:])
	// Encode address parameter (32 bytes, left-padded)
	copy(calldataBalanceOf[16:36], account[:])

	callResult, err := b.stateDB.Call(b.serviceID, account, &usdmAddress, 100000, 1000000000, 0, calldataBalanceOf, "latest", b.pvmBackend)
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

	nonceResult, err := b.stateDB.Call(b.serviceID, account, &usdmAddress, 100000, 1000000000, 0, calldataNonces, "latest", b.pvmBackend)
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
