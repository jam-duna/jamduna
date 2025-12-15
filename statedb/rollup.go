package statedb

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"

	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-verkle"
)

const (
	DefaultJAMChainID = 0x1107
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
	storage            types.JAMStorage
	pvmBackend         string
}

// Getter methods for Rollup fields
func (r *Rollup) GetStateDB() *StateDB {
	return r.stateDB
}

func (r *Rollup) GetServiceID() uint32 {
	return r.serviceID
}

func (r *Rollup) GetBalance(address common.Address, blockNumber string) (common.Hash, error) {
	// Use service-scoped verkle tree lookup
	tree, ok := r.storage.GetVerkleNodeForServiceBlock(r.serviceID, blockNumber)
	if !ok {
		return common.Hash{}, fmt.Errorf("verkle tree not found for service %d block %s", r.serviceID, blockNumber)
	}
	// Read balance from Verkle tree
	balanceHash, err := r.storage.GetBalance(tree, address)
	if err != nil {
		return common.Hash{}, err
	}

	return balanceHash, nil
}

func (r *Rollup) GetTransactionCount(address common.Address, blockNumber string) (uint64, error) {
	// Use service-scoped verkle tree lookup
	tree, ok := r.storage.GetVerkleNodeForServiceBlock(r.serviceID, blockNumber)
	if !ok {
		return 0, fmt.Errorf("verkle tree not found for service %d block %s", r.serviceID, blockNumber)
	}
	nonce, err := r.storage.GetNonce(tree, address)
	if err != nil {
		return 0, err
	}

	return nonce, nil
}

func (r *Rollup) GetCode(address common.Address, blockNumber string) ([]byte, error) {
	// Use service-scoped verkle tree lookup
	tree, ok := r.storage.GetVerkleNodeForServiceBlock(r.serviceID, blockNumber)
	if !ok {
		return nil, fmt.Errorf("verkle tree not found for service %d block %s", r.serviceID, blockNumber)
	}
	verkleTree, ok := tree.(verkle.VerkleNode)
	if !ok {
		return nil, fmt.Errorf("invalid tree type")
	}
	code, err := storage.ReadCode(verkleTree, address[:])
	if err != nil {
		return nil, fmt.Errorf("failed to read code from Verkle tree: %w", err)
	}
	return code, nil
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
func BuildPayload(payloadType PayloadType, count int, globalDepth uint8, numWitnesses int, blockAccessListHash common.Hash) []byte {
	payload := make([]byte, 40) // 1 + 4 + 1 + 2 + 32 = 40 bytes
	payload[0] = byte(payloadType)
	binary.LittleEndian.PutUint32(payload[1:5], uint32(count))
	payload[5] = globalDepth
	binary.LittleEndian.PutUint16(payload[6:8], uint16(numWitnesses))
	copy(payload[8:40], blockAccessListHash[:])
	return payload
}

func NewRollup(jamStorage types.JAMStorage, serviceID uint32) (*Rollup, error) {
	// Rollup is a lightweight query interface for service-scoped state
	// It does NOT create its own StateDB - that would interfere with the node's JAM state
	// For now, stateDB, previousGuarantees, and pvmBackend are left nil/empty
	// TODO: Refactor to remove these fields entirely and use node reference instead
	chain := Rollup{
		serviceID:          serviceID,
		stateDB:            nil, // TODO: Remove this field
		storage:            jamStorage,
		previousGuarantees: nil,
		pvmBackend:         BackendInterpreter,
	}
	return &chain, nil
}

// processWorkPackageBundles processes work packages in non-pipelined mode
// Block N: guarantees, Block N+1: assurances (two blocks per work package)
func (r *Rollup) processWorkPackageBundles(bundles []*types.WorkPackageBundle) error {
	validators, validatorSecrets, bootstrapAuthCodeHash, err := r.setupValidators()
	if err != nil {
		return err
	}

	// Execute bundles and create guarantees
	_, activeGuarantees, err := r.executeAndGuarantee(bundles, validatorSecrets, r.stateDB)
	if err != nil {
		return err
	}

	// Block 1: guarantees only
	extrinsic := types.NewExtrinsic()
	extrinsic.Guarantees = activeGuarantees
	if err := r.buildAndApplyBlock(context.Background(), validators, validatorSecrets, bootstrapAuthCodeHash, &extrinsic); err != nil {
		return err
	}

	// Block 2: assurances only
	extrinsic = types.NewExtrinsic()
	extrinsic.Assurances = r.createAssurances(0, validatorSecrets, r.stateDB)
	if err := r.buildAndApplyBlock(context.Background(), validators, validatorSecrets, bootstrapAuthCodeHash, &extrinsic); err != nil {
		return err
	}

	r.previousGuarantees = nil
	return nil
}

// processWorkPackageBundlesPipelined processes work packages in pipelined mode
// Block N contains: guarantees for current work + assurances for PREVIOUS block's work
// TODO: set up assurances bitfields correctly
func (r *Rollup) processWorkPackageBundlesPipelined(bundles []*types.WorkPackageBundle) error {
	validators, validatorSecrets, bootstrapAuthCodeHash, err := r.setupValidators()
	if err != nil {
		return err
	}

	// Execute bundles and create guarantees for current work
	guarantees, activeGuarantees, err := r.executeAndGuarantee(bundles, validatorSecrets, r.stateDB)
	if err != nil {
		return err
	}

	// Build extrinsic with current guarantees and previous assurances
	extrinsic := types.NewExtrinsic()
	extrinsic.Guarantees = activeGuarantees
	if r.previousGuarantees != nil {
		extrinsic.Assurances = r.createAssurancesForGuarantees(r.previousGuarantees, validatorSecrets, r.stateDB)
	}

	if err := r.buildAndApplyBlock(context.Background(), validators, validatorSecrets, bootstrapAuthCodeHash, &extrinsic); err != nil {
		return err
	}

	// Save guarantees for next block's assurances
	r.previousGuarantees = &guarantees
	return nil
}

// setupValidators generates validator secrets and bootstrap auth code hash
func (r *Rollup) setupValidators() ([]types.Validator, []types.ValidatorSecret, common.Hash, error) {
	validators, validatorSecrets, err := GenerateValidatorSecretSet(types.TotalValidators)
	if err != nil {
		return nil, nil, common.Hash{}, fmt.Errorf("failed to generate validator secrets: %w", err)
	}

	bootstrapAuthCodeHash, err := r.getBootstrapAuthCodeHash()
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	return validators, validatorSecrets, bootstrapAuthCodeHash, nil
}

// getBootstrapAuthCodeHash reads and hashes the bootstrap authorization code
func (r *Rollup) getBootstrapAuthCodeHash() (common.Hash, error) {
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
func (r *Rollup) executeAndGuarantee(bundles []*types.WorkPackageBundle, validatorSecrets []types.ValidatorSecret, statedb *StateDB) ([]types.Guarantee, []types.Guarantee, error) {
	guarantees := make([]types.Guarantee, types.TotalCores)
	activeGuarantees := make([]types.Guarantee, 0)

	for coreIndex, bundle := range bundles {
		if bundle == nil {
			continue
		}
		workReport, err := r.stateDB.ExecuteWorkPackageBundle(uint16(coreIndex), *bundle, types.SegmentRootLookup{}, 0, log.OtherGuarantor, 0, r.pvmBackend, "SKIP")
		if err != nil {
			return nil, nil, fmt.Errorf("failed ExecuteWorkPackageBundle: %v", err)
		}
		workReport.CoreIndex = uint(coreIndex)

		// Create guarantee with validator signatures
		guarantee := r.createGuarantee(workReport, uint16(coreIndex), validatorSecrets, statedb)
		guarantees[coreIndex] = guarantee

		if len(guarantee.Signatures) > 0 {
			activeGuarantees = append(activeGuarantees, guarantee)
		}
	}

	return guarantees, activeGuarantees, nil
}

// createGuarantee creates a guarantee with 3 validator signatures
func (r *Rollup) createGuarantee(workReport types.WorkReport, coreIndex uint16, validatorSecrets []types.ValidatorSecret, statedb *StateDB) types.Guarantee {
	guarantee := types.Guarantee{
		Report:     workReport,
		Slot:       r.stateDB.GetTimeslot(),
		Signatures: []types.GuaranteeCredential{},
	}

	_, assignments := r.stateDB.CalculateAssignments(r.stateDB.GetTimeslot())
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
func (r *Rollup) createAssurances(coreIndex uint16, validatorSecrets []types.ValidatorSecret, statedb *StateDB) []types.Assurance {
	assurances := make([]types.Assurance, 0)

	for i := 0; i < 6 && i < types.TotalValidators; i++ {
		assurance := types.Assurance{
			Anchor:         r.stateDB.HeaderHash,
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
func (r *Rollup) createAssurancesForGuarantees(guarantees *[]types.Guarantee, validatorSecrets []types.ValidatorSecret, statedb *StateDB) []types.Assurance {
	assurances := make([]types.Assurance, 0)

	for coreIndex := range *guarantees {
		if len((*guarantees)[coreIndex].Signatures) == 0 {
			continue
		}
		for i := 0; i < 6 && i < types.TotalValidators; i++ {
			assurance := types.Assurance{
				Anchor:         r.stateDB.HeaderHash,
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
func (r *Rollup) buildAndApplyBlock(ctx context.Context, validators []types.Validator, validatorSecrets []types.ValidatorSecret, bootstrapAuthCodeHash common.Hash, extrinsic *types.ExtrinsicData) error {
	s := r.stateDB
	targetJCE := r.stateDB.GetTimeslot() + 1

	// Find authorized block refiner
	validatorSecret, err := r.findAuthorizedValidator(validators, validatorSecrets, targetJCE)
	if err != nil {
		return err
	}

	// Build block
	sealedBlock, _ := s.BuildBlock(ctx, *validatorSecret, targetJCE, common.Hash{}, extrinsic)

	// Add bootstrap authorization to pool for all cores
	authorizerHash := common.Blake2Hash(append(bootstrapAuthCodeHash.Bytes(), []byte(nil)...))
	for i := 0; i < types.TotalCores; i++ {
		s.JamState.AuthorizationsPool[i][0] = authorizerHash
	}

	// Apply state transition
	newStateDB, err := ApplyStateTransitionFromBlock(0, r.stateDB, ctx, sealedBlock, nil, "interpreter", "")
	if err != nil {
		return fmt.Errorf("failed to apply state transition: %w", err)
	}

	r.stateDB = newStateDB
	return nil
}

// findAuthorizedValidator finds a validator authorized to build a block
func (r *Rollup) findAuthorizedValidator(validators []types.Validator, validatorSecrets []types.ValidatorSecret, targetJCE uint32) (*types.ValidatorSecret, error) {
	sf0, _ := r.stateDB.GetPosteriorSafroleEntropy(targetJCE)

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
func (r *Rollup) CallMath(mathAddress common.Address, callStrings []string) (txBytes [][]byte, alltopics map[common.Hash]string, err error) {

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
	initialNonce, err := r.GetTransactionCount(callerAddress, "latest")
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
			uint64(r.serviceID),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create evmmath call transaction for %s: %w", callString, err)
		}
		log.Info(log.Node, callString, "txHash", txHash.String())
		txBytes[i] = tx
	}

	return txBytes, alltopics, nil
}

// PackageMulticoreBundles packages raw transaction bytes into work package bundles for multiple cores
// Returns bundles ready for submission to the network
func (r *Rollup) PackageMulticoreBundles(evmTxsMulticore [][][]byte) ([]*types.WorkPackageBundle, error) {
	bundles := make([]*types.WorkPackageBundle, len(evmTxsMulticore))
	globalDepth, err := r.stateDB.ReadGlobalDepth(r.serviceID)
	if err != nil {
		return nil, fmt.Errorf("ReadGlobalDepth failed: %v", err)
	}
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

		service, ok, err := r.stateDB.GetService(r.serviceID)
		if err != nil || !ok {
			return nil, fmt.Errorf("EVM service not found: %v", err)
		}

		// Create work package
		wp := DefaultWorkPackage(r.serviceID, service)
		wp.WorkItems[0].Payload = BuildPayload(PayloadTypeBuilder, numTxExtrinsics, globalDepth, 0, common.Hash{})
		wp.WorkItems[0].Extrinsics = hashes

		//  BuildBundle should return a Bundle (with ImportedSegments)
		bundle2, _, err := r.stateDB.BuildBundle(wp, []types.ExtrinsicsBlobs{blobs}, uint16(coreIndex), nil, r.pvmBackend)
		if err != nil {
			return nil, fmt.Errorf("BuildBundle failed: %v", err)
		}
		bundles[coreIndex] = bundle2
	}

	return bundles, nil
}

// SubmitEVMTransactions creates and submits a work package with raw transactions, processes it, and returns the resulting block
func (r *Rollup) SubmitEVMTransactions(evmTxsMulticore [][][]byte) ([]*types.WorkPackageBundle, error) {
	// Package the transactions into bundles
	bundles, err := r.PackageMulticoreBundles(evmTxsMulticore)
	if err != nil {
		return nil, err
	}

	// Process the bundles
	err = r.processWorkPackageBundles(bundles)
	if err != nil {
		return nil, fmt.Errorf("processWorkPackageBundles failed: %w", err)
	}

	return bundles, nil
}

// ShowTxReceipts displays transaction receipts for the given transaction hashes
func (r *Rollup) ShowTxReceipts(evmBlock *evmtypes.EvmBlockPayload, txHashes []common.Hash, description string, allTopics map[common.Hash]string) error {
	log.Info(log.Node, "Showing transaction receipts", "description", description, "count", len(txHashes))

	gasUsedTotal := big.NewInt(0)
	txIndexByHash := make(map[common.Hash]int, len(evmBlock.TxHashes))
	for idx, hash := range evmBlock.TxHashes {
		txIndexByHash[hash] = idx
	}

	for _, txHash := range txHashes {
		receipt, err := r.getTransactionReceipt(txHash)
		if err != nil {
			return fmt.Errorf("failed to get transaction receipt for %s: %w", txHash.String(), err)
		}

		log.Info(log.Node, "✅ Transaction succeeded",
			"txHash", txHash.String(),
			"index", receipt.TransactionIndex,
			"gasUsed", receipt.UsedGas)

		logs, err := evmtypes.ParseLogsFromReceipt(receipt.LogsData, receipt.TransactionHash, receipt.BlockNumber, receipt.BlockHash, receipt.TransactionIndex, receipt.LogIndexStart)
		if err != nil {
			return fmt.Errorf("failed to parse logs from receipt for %s: %w", txHash.String(), err)
		}
		evmtypes.ShowEthereumLogs(txHash, logs, allTopics)
	}
	log.Info(log.Node, description, "txCount", len(txHashes), "gasUsedTotal", gasUsedTotal.String())
	return nil
}

func (r *Rollup) SubmitEVMGenesis(startBalance int64) error {
	log.Info(log.Node, "SubmitEVMGenesis - Initializing Verkle tree", "startBalance", startBalance)

	// Get StateDBStorage to access Verkle tree
	sdb, ok := r.stateDB.sdb.(*storage.StateDBStorage)
	if !ok {
		return fmt.Errorf("StateDB storage is not StateDBStorage")
	}

	// Initialize empty Verkle tree
	sdb.CurrentVerkleTree = verkle.New()

	// Convert startBalance to Wei (18 decimals)
	balanceWei := new(big.Int).Mul(big.NewInt(startBalance), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

	// Get issuer account address
	issuerAddress, _ := common.GetEVMDevAccount(0)

	log.Info(log.Node, "Initializing genesis account", "address", issuerAddress.Hex(), "balance", balanceWei.String())

	// 1. Insert account BasicData (balance + nonce)
	// BasicData format: [version(1) | reserved(4) | code_size(3) | nonce(8) | balance(16)] = 32 bytes
	var basicData [32]byte
	basicData[0] = 0 // version = 0

	// code_size = 0 (EOA, no code)
	basicData[5] = 0
	basicData[6] = 0
	basicData[7] = 0

	// nonce = 1 (8 bytes at offset 8-15, big-endian per EIP-6800)
	nonce := uint64(1)
	binary.BigEndian.PutUint64(basicData[8:16], nonce)

	// balance = balanceWei (16 bytes at offset 16-31, big-endian per EIP-6800)
	balanceBytes := balanceWei.Bytes()
	if len(balanceBytes) > 16 {
		return fmt.Errorf("balance too large: %d bytes", len(balanceBytes))
	}
	// big.Int.Bytes() returns big-endian, which is correct for EIP-6800
	// Copy to offset 16, right-aligned (pad left with zeros if needed)
	copy(basicData[32-len(balanceBytes):32], balanceBytes)

	// Insert BasicData into Verkle tree
	basicDataKey := storage.BasicDataKey(issuerAddress[:])
	if err := sdb.CurrentVerkleTree.Insert(basicDataKey, basicData[:], nil); err != nil {
		return fmt.Errorf("failed to insert BasicData: %w", err)
	}

	log.Info(log.Node, "Inserted BasicData", "key", common.Bytes2Hex(basicDataKey), "balance", balanceWei.String(), "nonce", nonce)

	// 2. Insert system contracts 0x01 and 0x02 code
	systemContracts := []struct {
		address common.Address
		code    []byte
	}{
		{
			address: common.HexToAddress("0x0000000000000000000000000000000000000001"),
			code:    []byte{0x60, 0x00, 0x60, 0x00, 0xf3}, // Minimal contract: PUSH1 0 PUSH1 0 RETURN
		},
		{
			address: common.HexToAddress("0x0000000000000000000000000000000000000002"),
			code:    []byte{0x60, 0x00, 0x60, 0x00, 0xf3}, // Minimal contract: PUSH1 0 PUSH1 0 RETURN
		},
	}

	for _, sc := range systemContracts {
		codeHash := crypto.Keccak256Hash(sc.code)

		// Insert code via InsertCode (handles chunking, code hash, and BasicData update)
		if err := storage.InsertCode(sdb.CurrentVerkleTree, sc.address[:], sc.code, codeHash[:]); err != nil {
			return fmt.Errorf("failed to insert code for %s: %w", sc.address.Hex(), err)
		}

		log.Info(log.Node, "Inserted system contract", "address", sc.address.Hex(), "codeSize", len(sc.code), "codeHash", codeHash.Hex())
	}

	// 3. Compute and log Verkle root
	verkleRoot := sdb.CurrentVerkleTree.Commit()
	verkleRootBytes := verkleRoot.Bytes()
	verkleRootHash := common.BytesToHash(verkleRootBytes[:])

	// Store the verkle tree at this root for future queries
	sdb.StoreVerkleTree(verkleRootHash, sdb.CurrentVerkleTree)

	// Create genesis block (block 0) for this service
	genesisBlock := &evmtypes.EvmBlockPayload{
		Number:              0,
		WorkPackageHash:     common.Hash{}, // Genesis has no work package hash
		SegmentRoot:         common.Hash{},
		PayloadLength:       148, // Just the header
		NumTransactions:     0,
		Timestamp:           0,
		GasUsed:             0,
		VerkleRoot:          verkleRootHash,
		TransactionsRoot:    common.Hash{},
		ReceiptRoot:         common.Hash{},
		BlockAccessListHash: common.Hash{},
		TxHashes:            []common.Hash{},
		ReceiptHashes:       []common.Hash{},
		Transactions:        []evmtypes.TransactionReceipt{},
	}

	// Store genesis block using the multi-rollup infrastructure
	if err := sdb.StoreServiceBlock(r.serviceID, genesisBlock, common.Hash{}, 0); err != nil {
		return fmt.Errorf("failed to store genesis block: %w", err)
	}

	log.Info(log.Node, "✅ SubmitEVMGenesis complete", "verkleRoot", verkleRootHash.Hex())

	// 4. Verify reads work
	balanceHash, err := r.GetBalance(issuerAddress, "latest")
	if err != nil {
		return fmt.Errorf("GetBalance failed: %w", err)
	}
	// Convert Hash to big.Int for comparison
	balanceRead := new(big.Int).SetBytes(balanceHash[:])
	if balanceRead.Cmp(balanceWei) != 0 {
		return fmt.Errorf("balance mismatch: expected %s, got %s", balanceWei.String(), balanceRead.String())
	}

	nonceRead, err := r.GetTransactionCount(issuerAddress, "latest")
	if err != nil {
		return fmt.Errorf("GetTransactionCount failed: %w", err)
	}
	if nonceRead != nonce {
		return fmt.Errorf("nonce mismatch: expected %d, got %d", nonce, nonceRead)
	}

	for _, sc := range systemContracts {
		code, err := r.GetCode(sc.address, "latest")
		if err != nil {
			return fmt.Errorf("GetCode failed for %s: %w", sc.address.Hex(), err)
		}
		if !bytes.Equal(code, sc.code) {
			return fmt.Errorf("code mismatch for %s", sc.address.Hex())
		}
	}

	log.Info(log.Node, "✅ Verified genesis state reads", "balance", balanceRead.String(), "nonce", nonceRead)

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
func (r *Rollup) createTransferTriplesForRound(roundNum int, txnsPerRound int) []TransferTriple {
	const numDevAccounts = 10
	transfers := make([]TransferTriple, 0, txnsPerRound)

	if roundNum == 0 {
		// Special case for round 0: issuer distributes to all accounts
		// Give each account 100 million wei (gasLimit=10M * gasPrice=1 + transfer amounts)
		for i := 1; i < numDevAccounts; i++ {
			amount := big.NewInt(100_000_000)
			transfers = append(transfers, TransferTriple{
				SenderIndex:   0,
				ReceiverIndex: i,
				Amount:        amount,
			})
		}
		return transfers
	}
	if roundNum == 1 {
		// Special case for round 1: secondary transfers between accounts
		// Each account sends 10000 wei to the next account (cycle: 1→2, 2→3, ..., 9→0)
		for i := 1; i < numDevAccounts; i++ {
			amount := big.NewInt(10_000)
			transfers = append(transfers, TransferTriple{
				SenderIndex:   i,
				ReceiverIndex: (i + 1) % numDevAccounts,
				Amount:        amount,
			})
		}
		return transfers
	}
	// Use deterministic seeded random number generator for reproducibility
	rng := rand.New(rand.NewSource(int64(roundNum)))

	for i := 0; i < txnsPerRound; i++ {
		// Pick random sender and receiver from accounts 1-9 (excluding issuer at 0)
		sender := rng.Intn(numDevAccounts-1) + 1   // Random from 1 to 9
		receiver := rng.Intn(numDevAccounts-1) + 1 // Random from 1 to 9

		// Ensure sender != receiver
		for sender == receiver {
			receiver = rng.Intn(numDevAccounts-1) + 1
		}

		// Use much smaller amounts to avoid OutOfFund errors
		// Amounts: 1000, 2000, 3000 wei (tiny amounts for testing)
		amount := big.NewInt(int64((i + 1) * 1000))

		log.Info(log.Node, "CREATETRANSFER", "Round", roundNum, "TxIndex", i, "sender", sender, "receiver", receiver, "amount", amount.String())

		transfers = append(transfers, TransferTriple{
			SenderIndex:   sender,
			ReceiverIndex: receiver,
			Amount:        amount,
		})
	}

	return transfers
}

// DeployContract deploys a contract and returns its address
func (r *Rollup) DeployContract(contractFile string) (common.Address, error) {
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
	nonce, err := r.GetTransactionCount(deployerAddress, "latest")
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
	signer := ethereumTypes.NewEIP155Signer(big.NewInt(int64(r.serviceID)))
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
	_, err = r.SubmitEVMTransactions(multiCoreTxBytes)
	if err != nil {
		return common.Address{}, fmt.Errorf("contract deployment failed: %w", err)
	}

	return contractAddress, nil
}

// SaveWorkPackageBundle saves a WorkPackageBundle to disk using Encode
func (r *Rollup) SaveWorkPackageBundle(bundle *types.WorkPackageBundle, filename string) error {
	encoded := bundle.Encode()
	return os.WriteFile(filename, encoded, 0644)
}

func (r *Rollup) GetChainId() uint64 {
	return uint64(DefaultJAMChainID)
}

func (r *Rollup) GetAccounts() []common.Address {
	return []common.Address{}
}

func (r *Rollup) GetGasPrice() uint64 {
	return 1
}

// boolToHexStatus converts a boolean success status to hex status string
func boolToHexStatus(success bool) string {
	if success {
		return "0x1"
	}
	return "0x0"
}

// GetTransactionReceipt fetches a transaction receipt
func (r *Rollup) getTransactionReceipt(txHash common.Hash) (*evmtypes.TransactionReceipt, error) {
	// Use ReadObject to get receipt from DA
	receiptObjectID := evmtypes.TxToObjectID(txHash)
	witness, found, err := r.stateDB.ReadObject(r.serviceID, receiptObjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to read transaction receipt: %v", err)
	}
	if !found {
		return nil, nil // Transaction not found
	}

	// Parse raw receipt
	receipt, err := evmtypes.ParseRawReceipt(witness)
	if err != nil {
		return nil, fmt.Errorf("failed to parse receipt: %v", err)
	}
	return receipt, nil
}

func (r *Rollup) GetTransactionReceipt(txHash common.Hash) (*evmtypes.EthereumTransactionReceipt, error) {
	receipt, err := r.getTransactionReceipt(txHash)
	if err != nil {
		return nil, err
	}

	// Parse transaction from receipt payload (RLP-encoded transaction)
	ethTx, err := evmtypes.ConvertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction from receipt: %v", err)
	}

	// Parse logs from receipt LogsData if available
	var logs []evmtypes.EthereumLog
	if len(receipt.LogsData) > 0 {
		logIndexStart := receipt.LogIndexStart

		logs, err = evmtypes.ParseLogsFromReceipt(receipt.LogsData, txHash,
			receipt.BlockNumber, receipt.BlockHash, receipt.TransactionIndex, logIndexStart)
		if err != nil {
			log.Warn(log.Node, "GetTransactionReceipt: Failed to parse logs", "error", err)
			logs = []evmtypes.EthereumLog{} // Use empty logs on parse failure
		}
	}

	// Determine contract address for contract creation
	var contractAddress *string
	if ethTx.To == nil {
		senderAddr := common.HexToAddress(ethTx.From)

		// Parse nonce
		var nonce uint64
		fmt.Sscanf(ethTx.Nonce, "0x%x", &nonce)

		// Calculate CREATE contract address using go-ethereum's built-in function
		// Convert our common.Address to go-ethereum's common.Address
		ethSenderAddr := ethereumCommon.Address(senderAddr)
		contractAddr := crypto.CreateAddress(ethSenderAddr, nonce).Hex()
		contractAddress = &contractAddr

		// Note: CREATE2 detection would require parsing input data to check for CREATE2 opcode
		// For now, we only handle CREATE transactions (when To == nil)
	}

	txType := "0x0"
	if payload := receipt.Payload; len(payload) > 0 && payload[0] < 0x80 {
		txType = fmt.Sprintf("0x%x", payload[0])
	}

	// Bloom filters removed - always use zero bytes for RPC compatibility
	logsBloom := evmtypes.ComputeLogsBloom(logs) // Returns zero bytes

	cumulativeGasUsed := receipt.CumulativeGas
	if cumulativeGasUsed == 0 {
		cumulativeGasUsed = receipt.UsedGas
	}

	// Build Ethereum receipt with transaction details from parsed RLP transaction
	ethReceipt := &evmtypes.EthereumTransactionReceipt{
		TransactionHash:   txHash.String(),
		TransactionIndex:  fmt.Sprintf("%d", receipt.TransactionIndex),
		BlockHash:         receipt.BlockHash.String(),
		BlockNumber:       fmt.Sprintf("%x", receipt.BlockNumber),
		From:              ethTx.From,
		To:                ethTx.To,
		CumulativeGasUsed: fmt.Sprintf("0x%x", cumulativeGasUsed),
		GasUsed:           fmt.Sprintf("0x%x", receipt.UsedGas),
		ContractAddress:   contractAddress,
		Logs:              logs,
		LogsBloom:         logsBloom,
		Status:            boolToHexStatus(receipt.Success),
		EffectiveGasPrice: ethTx.GasPrice,
		Type:              txType,
	}
	return ethReceipt, nil
}

// GetTransactionByHash fetches a transaction receipt by hash
// Returns raw TransactionReceipt and ObjectRef
func (r *Rollup) getTransactionByHash(txHash common.Hash) (*evmtypes.TransactionReceipt, error) {
	// Use ReadObject to get the transaction receipt with metadata from DA via meta-shard lookup
	// This includes the Ref field which contains block number and transaction index
	witness, found, err := r.stateDB.ReadObject(r.serviceID, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to read transaction receipt: %v", err)
	}
	if !found {
		return nil, nil // Transaction not found
	}

	// Parse the receipt data according to serialize_receipt format
	receipt, err := evmtypes.ParseRawReceipt(witness)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transaction receipt: %v", err)
	}

	return receipt, nil
}

func (r *Rollup) GetTransactionByHash(txHash common.Hash) (*evmtypes.EthereumTransactionResponse, error) {
	receipt, err := r.getTransactionByHash(txHash)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, nil // Transaction not found
	}

	// Convert the original payload to Ethereum transaction format
	ethTx, err := evmtypes.ConvertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload to Ethereum transaction: %v", err)
	}

	// Populate block metadata
	ethTx.BlockHash = fmt.Sprintf("0x%x", receipt.BlockHash)
	ethTx.BlockNumber = fmt.Sprintf("0x%x", receipt.BlockNumber)
	ethTx.TransactionIndex = fmt.Sprintf("0x%x", receipt.TransactionIndex)

	return ethTx, nil
}

// GetTransactionByHashFormatted fetches a transaction and returns it in Ethereum JSON-RPC format
func (r *Rollup) GetTransactionByHashFormatted(txHash common.Hash) (*evmtypes.EthereumTransactionResponse, error) {
	receipt, err := r.getTransactionByHash(txHash)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, nil // Transaction not found
	}

	// Convert the original payload to Ethereum transaction format
	ethTx, err := evmtypes.ConvertPayloadToEthereumTransaction(receipt)
	if err != nil {
		return nil, fmt.Errorf("failed to convert payload to Ethereum transaction: %v", err)
	}

	// Populate block/tx metadata
	ethTx.BlockHash = fmt.Sprintf("0x%x", receipt.BlockHash)
	ethTx.BlockNumber = fmt.Sprintf("0x%d", receipt.BlockNumber)
	ethTx.TransactionIndex = fmt.Sprintf("0x%x", receipt.TransactionIndex)

	return ethTx, nil
}

func (r *Rollup) GetTransactionByBlockHashAndIndex(blockHash common.Hash, index uint32) (*evmtypes.EthereumTransactionResponse, error) {
	// First, get the block to retrieve transaction hashes
	block, err := r.readBlockByHash(blockHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %v", err)
	}
	if block == nil {
		return nil, nil // Block not found
	}

	// Fetch the full transaction
	return r.GetTransactionByHashFormatted(block.TxHashes[index])
}

func (r *Rollup) GetTransactionByBlockNumberAndIndex(blockNumber string, index uint32) (*evmtypes.EthereumTransactionResponse, error) {

	// First, get the block to retrieve transaction hashes
	block, err := r.GetEVMBlockByNumber(blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %v", err)
	}
	if block == nil {
		return nil, nil // Block not found
	}

	// Check if index is in range
	if index >= uint32(len(block.TxHashes)) {
		return nil, fmt.Errorf("index out of range")
	}

	// Fetch the full transaction
	return r.GetTransactionByHashFormatted(block.TxHashes[index])
}

func (r *Rollup) GetLogs(fromBlock, toBlock uint32, addresses []common.Address, topics [][]common.Hash) ([]evmtypes.EthereumLog, error) {
	var allLogs []evmtypes.EthereumLog

	// Collect logs from the specified block range
	for blockNum := fromBlock; blockNum <= toBlock; blockNum++ {
		blockLogs, err := r.getLogsFromBlock(blockNum, addresses, topics)
		if err != nil {
			log.Warn(log.Node, "GetLogs: Failed to get logs from block", "blockNumber", blockNum, "error", err)
			continue // Skip failed blocks but continue processing
		}
		allLogs = append(allLogs, blockLogs...)
	}

	return allLogs, nil
}

func (r *Rollup) GetLatestBlockNumber() (uint32, error) {

	// Use same key as Rust: BLOCK_NUMBER_KEY = 0xFF repeated 32 times
	// Rust stores only the next block number (4 bytes LE)
	key := make([]byte, 32)
	for i := range key {
		key[i] = 0xFF
	}

	valueBytes, found, err := r.stateDB.ReadServiceStorage(r.serviceID, key)
	if err != nil {
		return 0, fmt.Errorf("failed to read block number from storage: %v", err)
	}
	if !found || len(valueBytes) < 4 {
		return 0, nil // Genesis state (block 0)
	}

	// Parse block_number (first 4 bytes, little-endian)
	blockNumber := binary.LittleEndian.Uint32(valueBytes[:4])
	if blockNumber > 0 {
		return blockNumber - 1, nil
	}
	return 0, nil
}

func (r *Rollup) GetBlockByHash(blockHash common.Hash, fullTx bool) (*evmtypes.EthereumBlock, error) {
	evmBlock, err := r.readBlockByHash(blockHash)
	if err != nil {
		// Block not found or error reading from DA
		return nil, err
	}

	// If fullTx requested, fetch full transaction objects
	if fullTx {
		transactions := make([]evmtypes.TransactionReceipt, len(evmBlock.TxHashes))
		for i, txHash := range evmBlock.TxHashes {
			ethTx, err := r.getTransactionByHash(txHash)
			if err != nil {
				log.Warn(log.Node, "GetBlockByHash: Failed to get transaction",
					"txHash", txHash.String(), "error", err)
				continue
			}
			transactions[i] = *ethTx
		}
		evmBlock.Transactions = transactions
	}
	ethBlock := evmBlock.ToEthereumBlock(evmBlock.Number, fullTx)

	return ethBlock, nil
}

func (r *Rollup) GetBlockByNumber(blockNumber string, fullTx bool) (*evmtypes.EthereumBlock, error) {
	evmBlock, err := r.GetEVMBlockByNumber(blockNumber)
	if err != nil {
		return nil, err
	}
	log.Trace(log.Node, "GetBlockByNumber: Fetched block", "number", evmBlock.Number, "b", types.ToJSON(evmBlock))
	// Generate metadata and convert EvmBlockPayload to Ethereum JSON-RPC format
	ethBlock := evmBlock.ToEthereumBlock(evmBlock.Number, fullTx)

	// If fullTx requested, fetch full transaction objects
	if fullTx {
		transactions := make([]evmtypes.EthereumTransactionResponse, 0, len(evmBlock.TxHashes))

		for i, txHash := range evmBlock.TxHashes {
			ethTx, err := r.GetTransactionByHashFormatted(txHash)
			if err != nil {
				log.Warn(log.Node, "GetBlockByNumber: Failed to get transaction", "txHash", txHash.String(), "error", err)
				continue
			}
			if ethTx != nil {
				ethTx.BlockHash = evmBlock.WorkPackageHash.String()
				ethTx.BlockNumber = fmt.Sprintf("0x%x", evmBlock.Number)
				ethTx.TransactionIndex = fmt.Sprintf("0x%x", i)
				transactions = append(transactions, *ethTx)
			}
		}

		ethBlock.Transactions = transactions
	}

	return ethBlock, nil
}

// GetBlockByNumber fetches a block by number and returns raw EvmBlockPayload
func (r *Rollup) GetEVMBlockByNumber(blockNumberStr string) (*evmtypes.EvmBlockPayload, error) {
	// 1. Parse and resolve the block number
	var targetBlockNumber uint32
	var err error

	switch blockNumberStr {
	case "latest":
		targetBlockNumber, err = r.GetLatestBlockNumber()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest block number: %v", err)
		}
		if targetBlockNumber < 1 {
			return nil, fmt.Errorf("block 1 not ready yet")
		}
	case "earliest":
		targetBlockNumber = 1 // Genesis block
	default:
		// Parse hex block number
		if len(blockNumberStr) >= 2 && blockNumberStr[:2] == "0x" {
			blockNum, parseErr := strconv.ParseUint(blockNumberStr[2:], 16, 32)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid block number format: %v", parseErr)
			}
			targetBlockNumber = uint32(blockNum)
		} else {
			return nil, fmt.Errorf("invalid block number format: %s", blockNumberStr)
		}
	}

	// 2. Read canonical block metadata from storage
	return r.ReadBlockByNumber(targetBlockNumber)
}

// CreateSignedNativeTransfer wraps evmtypes.CreateSignedNativeTransfer for native ETH transfers
func CreateSignedNativeTransfer(privateKeyHex string, nonce uint64, to common.Address, amount *big.Int, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
	return evmtypes.CreateSignedNativeTransfer(privateKeyHex, nonce, to, amount, gasPrice, gasLimit, chainID)
}

// CreateSignedUSDMTransfer wraps evmtypes.CreateSignedUSDMTransfer with UsdmAddress
func CreateSignedUSDMTransfer(privateKeyHex string, nonce uint64, to common.Address, amount *big.Int, gasPrice *big.Int, gasLimit uint64, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
	return evmtypes.CreateSignedUSDMTransfer(evmtypes.UsdmAddress, privateKeyHex, nonce, to, amount, gasPrice, gasLimit, chainID)
}

func (r *Rollup) ReadBlockByNumber(blockNumber uint32) (*evmtypes.EvmBlockPayload, error) {
	objectID := evmtypes.BlockNumberToObjectID(blockNumber)

	// Read objectID key to get blockNumber => wph (32 bytes) + timestamp (4 bytes) + segment_root (32 bytes) mapping
	valueBytes, found, err := r.stateDB.ReadServiceStorage(r.serviceID, objectID.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block number mapping: %v", err)
	}

	if !found || len(valueBytes) < 68 {
		return nil, fmt.Errorf("block %d [%s] not found %d", blockNumber, objectID, len(valueBytes))
	}

	// Parse work_package_hash (32 bytes) + timeslot (4 bytes, little-endian) + segment_root (32 bytes)
	var workPackageHash common.Hash
	copy(workPackageHash[:], valueBytes[:32])

	return r.readBlockByHash(workPackageHash)
}

// here, blockHash is actually a workpackagehash so we can read segments directly from DA
func (r *Rollup) readBlockByHash(workPackageHash common.Hash) (*evmtypes.EvmBlockPayload, error) {
	// read the block number + timestamp from the blockHash key
	var blockNumber uint32
	var blockTimestamp uint32
	var segmentRoot common.Hash
	valueBytes, found, err := r.stateDB.ReadServiceStorage(r.serviceID, workPackageHash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block hash mapping: %v", err)
	}
	if found && len(valueBytes) >= 8 {
		// Parse block number (4 bytes, little-endian)
		blockNumber = binary.LittleEndian.Uint32(valueBytes[:4])
		blockTimestamp = binary.LittleEndian.Uint32(valueBytes[4:8])
		segmentRoot = common.BytesToHash(valueBytes[8:40])
	}

	payload, err := r.stateDB.sdb.FetchJAMDASegments(workPackageHash, 0, 1, types.SegmentSize)
	if err != nil {
		return nil, fmt.Errorf("block not found: %s", workPackageHash.Hex())
	}

	block, err := evmtypes.DeserializeEvmBlockPayload(payload, true)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize block payload: %v", err)
	}
	// using block.PayloadLength figure out how many segments to read for full block
	segments := (block.PayloadLength + types.SegmentSize - 1) / types.SegmentSize
	if segments > 1 {
		remainingLength := block.PayloadLength - types.SegmentSize
		payload2, err := r.stateDB.sdb.FetchJAMDASegments(workPackageHash, 1, uint16(segments), remainingLength)
		if err != nil {
			return nil, fmt.Errorf("failed to read additional block segments: %v", err)
		}
		payload = append(payload, payload2...)
	}
	block, err = evmtypes.DeserializeEvmBlockPayload(payload, false)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize full block payload: %v", err)
	}
	block.WorkPackageHash = workPackageHash
	block.SegmentRoot = segmentRoot
	block.Timestamp = blockTimestamp
	block.Number = blockNumber
	return block, nil
}

// getLogsFromBlock retrieves logs from a specific block that match the filter criteria
func (r *Rollup) getLogsFromBlock(blockNumber uint32, addresses []common.Address, topics [][]common.Hash) ([]evmtypes.EthereumLog, error) {
	// 1. Get all transaction hashes from the block (use canonical metadata)
	evmBlock, err := r.ReadBlockByNumber(blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to read block %d: %v", blockNumber, err)
	}
	blockTxHashes := evmBlock.TxHashes

	var blockLogs []evmtypes.EthereumLog

	// For each transaction, get its receipt and extract logs
	for _, txHash := range blockTxHashes {
		// Get transaction receipt using ReadObject abstraction
		receiptObjectID := evmtypes.TxToObjectID(txHash)
		witness, found, err := r.stateDB.ReadObject(EVMServiceCode, receiptObjectID)
		if err != nil || !found {
			log.Warn(log.Node, "getLogsFromBlock: Failed to read receipt", "txHash", txHash.String(), "error", err)
			continue
		}

		receipt, err := evmtypes.ParseRawReceipt(witness)
		if err != nil {
			log.Warn(log.Node, "getLogsFromBlock: Failed to parse receipt", "txHash", txHash.String(), "error", err)
			continue
		}

		// Extract and filter logs from this transaction
		if len(receipt.LogsData) > 0 {
			txLogs, err := evmtypes.ParseLogsFromReceipt(
				receipt.LogsData,
				txHash,
				blockNumber,
				receipt.BlockHash,
				receipt.TransactionIndex,
				uint64(0),
			)
			if err != nil {
				log.Warn(log.Node, "getLogsFromBlock: Failed to parse logs", "txHash", txHash.String(), "error", err)
				continue
			}

			// Apply address and topic filters
			for _, ethLog := range txLogs {
				if evmtypes.MatchesLogFilter(ethLog, addresses, topics) {
					blockLogs = append(blockLogs, ethLog)
				}
			}
		}
	}

	return blockLogs, nil
}

// ReadObjectRef reads ObjectRef bytes from service storage and deserializes them
// Parameters:
// - stateDB: The stateDB to read from, if nil uses r.statedb
// - serviceCode: The service code to read from
// - objectID: The object ID (typically a transaction hash or other identifier)
// Returns:
// - ObjectRef: The deserialized ObjectRef struct
// - bool: true if found, false if not found
// - error: any error that occurred
func (r *Rollup) ReadObjectRef(serviceCode uint32, objectID common.Hash) (*types.ObjectRef, bool, error) {
	// Read raw ObjectRef bytes from service storage
	objectRefBytes, found, err := r.stateDB.ReadServiceStorage(serviceCode, objectID.Bytes())
	if err != nil {
		return nil, false, fmt.Errorf("failed to read ObjectRef from service storage: %v", err)
	}
	if !found {
		return nil, false, nil // ObjectRef not found
	}

	// Deserialize ObjectRef from storage data
	offset := 0
	objRef, err := types.DeserializeObjectRef(objectRefBytes, &offset)
	if err != nil {
		return nil, false, fmt.Errorf("failed to deserialize ObjectRef: %v", err)
	}

	return &objRef, true, nil
}

func (r *Rollup) GetStorageAt(address common.Address, position common.Hash, blockNumber string) (common.Hash, error) {
	value, err := r.ReadContractStorageValue(address, position)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to read storage: %v", err)
	}
	return value, nil
}

// ReadContractStorageValue reads EVM contract storage, checking witness cache first
func (r *Rollup) ReadContractStorageValue(contractAddress common.Address, storageKey common.Hash) (common.Hash, error) {
	// Read from StateDBStorage witness cache (populated during PrepareBuilderWitnesses or import)
	value, found := r.stateDB.sdb.ReadStorageFromCache(contractAddress, storageKey)
	if found {
		return value, nil
	}
	// Not found in cache, return zero value
	return common.Hash{}, nil
}

// // EstimateGas tests the EstimateGas functionality with a USDM transfer
func (r *Rollup) EstimateGasTransfer(issuerAddress common.Address, usdmAddress common.Address, pvmBackend string) (uint64, error) {
	recipientAddr, _ := common.GetEVMDevAccount(1)
	transferAmount := big.NewInt(1000000) // 1M tokens (small test amount)

	// Create transfer calldata: transfer(address,uint256)
	estimateCalldata := make([]byte, 68)
	copy(estimateCalldata[0:4], []byte{0xa9, 0x05, 0x9c, 0xbb}) // transfer(address,uint256) selector
	copy(estimateCalldata[16:36], recipientAddr.Bytes())
	copy(estimateCalldata[36:68], transferAmount.FillBytes(make([]byte, 32)))

	estimatedGas, err := r.EstimateGas(issuerAddress, &usdmAddress, 100000, 1000000000, 0, estimateCalldata, pvmBackend)
	if err != nil {
		return 0, fmt.Errorf("EstimateGas failed: %w", err)
	}
	return estimatedGas, nil
}

// EstimateGas estimates the gas needed to execute a transaction
func (r *Rollup) EstimateGas(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, pvmBackend string) (uint64, error) {
	// Build Ethereum transaction for simulation
	valueBig := new(big.Int).SetUint64(value)
	gasPriceBig := new(big.Int).SetUint64(gasPrice)
	tx := &evmtypes.EthereumTransaction{
		From:     from,
		To:       to,
		Gas:      gas,
		GasPrice: gasPriceBig,
		Value:    valueBig,
		Data:     data,
	}

	// Create simulation work package with payload "B"
	workReport, err := r.createSimulatedTx(tx, pvmBackend)
	if err != nil {
		return 0, fmt.Errorf("failed to create simulation work package: %v", err)
	}
	if len(workReport.Results) == 0 || len(workReport.Results[0].Result.Ok) == 0 {
		return 0, fmt.Errorf("no result from simulation")
	}

	effects, err := types.DeserializeExecutionEffects(workReport.Results[0].Result.Ok)
	if err != nil {
		return 0, fmt.Errorf("failed to deserialize execution effects: %v", err)
	}

	intent := effects.WriteIntents[0]
	gasUsed := uint64(0) // TODO
	log.Info(log.SDB, "intent.Effect.ObjectID", "object_id", intent.Effect.ObjectID.String(),
		"gas_used", gasUsed)

	return gasUsed, nil
}

// Call simulates a transaction execution without submitting it
func (r *Rollup) Call(from common.Address, to *common.Address, gas uint64, gasPrice uint64, value uint64, data []byte, blockNumber string, pvmBackend string) ([]byte, error) {
	// Build Ethereum transaction for simulation
	valueBig := new(big.Int).SetUint64(value)
	gasPriceBig := new(big.Int).SetUint64(gasPrice)
	tx := &evmtypes.EthereumTransaction{
		From:     from,
		To:       to,
		Gas:      gas,
		GasPrice: gasPriceBig,
		Value:    valueBig,
		Data:     data,
	}

	// Execute simulation
	wr, err := r.createSimulatedTx(tx, pvmBackend)
	if err != nil {
		return nil, fmt.Errorf("failed to execute simulation: %v", err)
	}

	return wr.Results[0].Result.Ok[:], nil
}

// createSimulatedTx creates a work package, uses BuildBundle to generate a work report for simulating a transaction
func (r *Rollup) createSimulatedTx(tx *evmtypes.EthereumTransaction, pvmBackend string) (workReport *types.WorkReport, err error) {
	// 1. Convert Ethereum transaction to JAM extrinsic format
	// Extrinsic format: caller(20) + target(20) + gas_limit(32) + gas_price(32) + value(32) + call_kind(4) + data_len(8) + data
	dataLen := len(tx.Data)
	extrinsicSize := 148 + dataLen // 20+20+32+32+32+4+8 + data
	extrinsic := make([]byte, extrinsicSize)

	offset := 0

	// caller (20 bytes)
	copy(extrinsic[offset:offset+20], tx.From.Bytes())
	offset += 20

	// target (20 bytes) - use zero address for contract creation
	if tx.To != nil {
		copy(extrinsic[offset:offset+20], tx.To.Bytes())
	} else {
		// Contract creation - use zero address
		copy(extrinsic[offset:offset+20], make([]byte, 20))
	}
	offset += 20

	// gas_limit (32 bytes, big-endian)
	gasLimitBytes := make([]byte, 32)
	binary.BigEndian.PutUint64(gasLimitBytes[24:32], tx.Gas)
	copy(extrinsic[offset:offset+32], gasLimitBytes)
	offset += 32

	// gas_price (32 bytes, big-endian)
	gasPriceBytes := tx.GasPrice.FillBytes(make([]byte, 32))
	copy(extrinsic[offset:offset+32], gasPriceBytes)
	offset += 32

	// value (32 bytes, big-endian)
	valueBytes := tx.Value.FillBytes(make([]byte, 32))
	copy(extrinsic[offset:offset+32], valueBytes)
	offset += 32

	// call_kind (4 bytes, little-endian) - 0 = CALL, 1 = CREATE
	callKind := uint32(0) // CALL
	if tx.To == nil {
		callKind = 1 // CREATE
	}
	binary.LittleEndian.PutUint32(extrinsic[offset:offset+4], callKind)
	offset += 4

	// data_len (8 bytes, little-endian)
	binary.LittleEndian.PutUint64(extrinsic[offset:offset+8], uint64(dataLen))
	offset += 8

	// data
	copy(extrinsic[offset:], tx.Data)

	// 2. Get the EVM service info
	evmService, ok, err := r.stateDB.GetService(r.serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get EVM service: %v", err)
	}
	if !ok {
		return nil, fmt.Errorf("EVM service not found")
	}

	// 3. Create transaction hash
	txHash := common.Blake2Hash(extrinsic)

	// 4. Create work package
	workPackage := DefaultWorkPackage(r.serviceID, evmService)
	globalDepth, err := r.stateDB.ReadGlobalDepth(evmService.ServiceIndex)
	if err != nil {
		return nil, fmt.Errorf("ReadGlobalDepth failed: %v", err)
	}
	workPackage.WorkItems[0].Payload = BuildPayload(PayloadTypeCall, 1, globalDepth, 0, common.Hash{})
	workPackage.WorkItems[0].Extrinsics = []types.WorkItemExtrinsic{
		{
			Hash: txHash,
			Len:  uint32(len(extrinsic)),
		},
	}

	// Execute the work package with proper parameters
	// Use core index 0 for simulation, current slot, and mark as not first guarantor
	_, workReport, err = r.stateDB.BuildBundle(workPackage, []types.ExtrinsicsBlobs{types.ExtrinsicsBlobs{extrinsic}}, 0, nil, pvmBackend)
	if err != nil {
		return nil, fmt.Errorf("BuildBundle failed: %v", err)
	}
	if workReport == nil {
		return nil, fmt.Errorf("BuildBundle returned nil work report")
	}

	// Extract result from work report
	if len(workReport.Results) > 0 {
		// Return the output from the first work result
		result := workReport.Results[0].Result
		if len(result.Ok) > 0 {
			// Parse ExecutionEffects to extract call output
			// Format: ExecutionEffects serialization + call output appended
			_, err := types.DeserializeExecutionEffects(result.Ok)
			if err != nil {
				log.Warn(log.Node, "createSimulatedTx: failed to deserialize effects", "err", err)
				// Return raw result if deserialization fails
				return workReport, nil
			}

			// The call output is appended after the serialized ExecutionEffects
			// ExecutionEffects format: [write_intents_count:2][write_intents...]
			// For payload "B", write_intents should be empty (count=0)
			// Header size: 2 bytes
			effectsHeaderSize := 2

			// For payload "B", there should be no write intents, so output starts immediately after header
			if len(result.Ok) > effectsHeaderSize {
				// Extract the call output (everything after ExecutionEffects header)
				callOutput := result.Ok[effectsHeaderSize:]
				log.Debug(log.Node, "createSimulatedTx: extracted call output",
					"gas_used", 0,
					"output_len", len(callOutput))
				return workReport, nil
			}

			// No output, return empty
			return nil, fmt.Errorf("no call output from simulation")
		}
		if result.Err != 0 {
			return nil, fmt.Errorf("simulation error code: %d", result.Err)
		}
	}

	return nil, fmt.Errorf("no result from simulation")
}

// SendRawTransaction submits a signed transaction to the mempool
func (n *Rollup) SendRawTransaction(signedTxData []byte) (common.Hash, error) {
	// Parse the raw transaction
	tx, err := evmtypes.ParseRawTransaction(signedTxData)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to parse transaction: %v", err)
	}

	// Recover sender from signature
	sender, err := tx.RecoverSender()
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to recover sender: %v", err)
	}
	tx.From = sender

	// Validate signature - sender recovery already done above, verify it's valid
	if sender == (common.Address{}) {
		return common.Hash{}, fmt.Errorf("invalid signature: unable to recover sender address")
	}

	// Validate nonce against current state
	currentNonce, err := n.GetTransactionCount(sender, "latest")
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get current nonce for validation: %v", err)
	}
	if tx.Nonce < currentNonce {
		return common.Hash{}, fmt.Errorf("nonce too low: transaction nonce %d, account nonce %d", tx.Nonce, currentNonce)
	}

	// Validate balance - sender must have enough to cover value + gas costs
	balance, err := n.GetBalance(sender, "latest")
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get balance for validation: %v", err)
	}
	balanceBig := new(big.Int).SetBytes(balance.Bytes())

	// Calculate total cost: value + (gas * gasPrice)
	gasCost := new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas), tx.GasPrice)
	totalCost := new(big.Int).Add(tx.Value, gasCost)

	if balanceBig.Cmp(totalCost) < 0 {
		return common.Hash{}, fmt.Errorf("insufficient funds: balance %s, required %s (value %s + gas cost %s)",
			balanceBig.String(), totalCost.String(), tx.Value.String(), gasCost.String())
	}

	// Validate gas limit against block gas limit (RefineGasAllocation per work item)
	maxGasLimit := uint64(types.RefineGasAllocation)
	if tx.Gas > maxGasLimit {
		return common.Hash{}, fmt.Errorf("gas limit too high: transaction gas %d exceeds maximum %d", tx.Gas, maxGasLimit)
	}

	// Minimum gas for basic transaction is 1000
	const minTxGas = 1000
	if tx.Gas < minTxGas {
		return common.Hash{}, fmt.Errorf("gas limit too low: transaction gas %d is below minimum %d", tx.Gas, minTxGas)
	}

	// TODO: Add transaction to mempool organized by BAL
	// err = n.txPool.AddTransaction(tx)
	// if err != nil {
	// 	return common.Hash{}, fmt.Errorf("failed to add transaction to mempool: %v", err)
	// }

	log.Info(log.Node, "SendRawTransaction TODO Transaction added to mempool",
		"hash", tx.Hash.String(),
		"from", tx.From.String(),
		"nonce", tx.Nonce)

	return tx.Hash, nil
}
