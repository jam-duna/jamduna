package statedb

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	log "github.com/colorfulnotion/jam/log"

	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-verkle"
)

// RollupNode is a self-contained rollup state machine for testing
// It owns its own StateDB and manages block processing, genesis initialization,
// and transaction execution. Used primarily in statedb tests.
type RollupNode struct {
	stateDB            *StateDB
	serviceID          uint32
	previousGuarantees *[]types.Guarantee
	storage            types.JAMStorage
	pvmBackend         string
}

// Getter methods for Rollup fields
func (r *RollupNode) GetStateDB() *StateDB {
	return r.stateDB
}

func (r *RollupNode) GetServiceID() uint32 {
	return r.serviceID
}

func (r *RollupNode) GetBalance(address common.Address, blockNumber string) (common.Hash, error) {
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

func (r *RollupNode) GetTransactionCount(address common.Address, blockNumber string) (uint64, error) {
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

func (r *RollupNode) GetCode(address common.Address, blockNumber string) ([]byte, error) {
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

// ReadBlockByNumber reads a block from storage by block number
func (r *RollupNode) ReadBlockByNumber(blockNumber uint32) (*evmtypes.EvmBlockPayload, error) {
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

// readBlockByHash reads a block from storage by work package hash
func (r *RollupNode) readBlockByHash(workPackageHash common.Hash) (*evmtypes.EvmBlockPayload, error) {
	// read the block number + timestamp from the blockHash key
	var blockNumber uint32
	var blockTimestamp uint32
	var segmentRoot common.Hash
	valueBytes, found, err := r.stateDB.ReadServiceStorage(r.serviceID, workPackageHash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read block hash mapping: %v", err)
	}
	if found && len(valueBytes) >= 8 {
		blockNumber = binary.LittleEndian.Uint32(valueBytes[0:4])
		blockTimestamp = binary.LittleEndian.Uint32(valueBytes[4:8])
		if len(valueBytes) >= 40 {
			copy(segmentRoot[:], valueBytes[8:40])
		}
	}

	payload, err := r.stateDB.sdb.FetchJAMDASegments(workPackageHash, 0, 1, types.SegmentSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read block segments: %v", err)
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

// GetEVMBlockByNumber fetches a block by number and returns raw EvmBlockPayload
func (r *RollupNode) GetEVMBlockByNumber(blockNumberStr string) (*evmtypes.EvmBlockPayload, error) {
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

// GetLatestBlockNumber returns the latest block number for this service
func (r *RollupNode) GetLatestBlockNumber() (uint32, error) {
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

func NewRollupNode(jamStorage types.JAMStorage, serviceID uint32) (*RollupNode, error) {
	// RollupNode is a self-contained rollup state machine for testing
	// It owns its own StateDB and manages block processing
	chain := RollupNode{
		serviceID:          serviceID,
		stateDB:            nil, // Initialized by SubmitEVMGenesis or test setup
		storage:            jamStorage,
		previousGuarantees: nil,
		pvmBackend:         BackendInterpreter,
	}
	return &chain, nil
}

// processWorkPackageBundles processes work packages in non-pipelined mode
// Block N: guarantees, Block N+1: assurances (two blocks per work package)
func (r *RollupNode) processWorkPackageBundles(bundles []*types.WorkPackageBundle) error {
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
func (r *RollupNode) processWorkPackageBundlesPipelined(bundles []*types.WorkPackageBundle) error {
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
func (r *RollupNode) setupValidators() ([]types.Validator, []types.ValidatorSecret, common.Hash, error) {
	validators, validatorSecrets, err := grandpa.GenerateValidatorSecretSet(types.TotalValidators)
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
func (r *RollupNode) getBootstrapAuthCodeHash() (common.Hash, error) {
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
func (r *RollupNode) executeAndGuarantee(bundles []*types.WorkPackageBundle, validatorSecrets []types.ValidatorSecret, statedb *StateDB) ([]types.Guarantee, []types.Guarantee, error) {
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
func (r *RollupNode) createGuarantee(workReport types.WorkReport, coreIndex uint16, validatorSecrets []types.ValidatorSecret, statedb *StateDB) types.Guarantee {
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
func (r *RollupNode) createAssurances(coreIndex uint16, validatorSecrets []types.ValidatorSecret, statedb *StateDB) []types.Assurance {
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
func (r *RollupNode) createAssurancesForGuarantees(guarantees *[]types.Guarantee, validatorSecrets []types.ValidatorSecret, statedb *StateDB) []types.Assurance {
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
func (r *RollupNode) buildAndApplyBlock(ctx context.Context, validators []types.Validator, validatorSecrets []types.ValidatorSecret, bootstrapAuthCodeHash common.Hash, extrinsic *types.ExtrinsicData) error {
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
func (r *RollupNode) findAuthorizedValidator(validators []types.Validator, validatorSecrets []types.ValidatorSecret, targetJCE uint32) (*types.ValidatorSecret, error) {
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
func (r *RollupNode) CallMath(mathAddress common.Address, callStrings []string) (txBytes [][]byte, alltopics map[common.Hash]string, err error) {

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
func (r *RollupNode) PackageMulticoreBundles(evmTxsMulticore [][][]byte) ([]*types.WorkPackageBundle, error) {
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
func (r *RollupNode) SubmitEVMTransactions(evmTxsMulticore [][][]byte) ([]*types.WorkPackageBundle, error) {
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
func (r *RollupNode) ShowTxReceipts(evmBlock *evmtypes.EvmBlockPayload, txHashes []common.Hash, description string, allTopics map[common.Hash]string) error {
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

func (r *RollupNode) SubmitEVMGenesis(startBalance int64) error {
	log.Info(log.Node, "SubmitEVMGenesis - Using MakeGenesisStateTransition", "startBalance", startBalance)

	// Use MakeGenesisStateTransition to create the full genesis state
	// This handles all JAM state initialization including services
	trace, err := MakeGenesisStateTransition(r.storage, 0, "tiny", nil)
	if err != nil {
		return fmt.Errorf("MakeGenesisStateTransition failed: %w", err)
	}

	// Get the stateDB from the genesis transition
	stateDB, err := NewStateDBFromStateTransition(r.storage, trace)
	if err != nil {
		return fmt.Errorf("NewStateDBFromStateTransition failed: %w", err)
	}

	r.stateDB = stateDB

	// Now add EVM-specific genesis: issuer account with startBalance
	sdb, ok := r.stateDB.sdb.(*storage.StateDBStorage)
	if !ok {
		return fmt.Errorf("StateDB storage is not StateDBStorage")
	}

	// Convert startBalance to Wei (18 decimals)
	balanceWei := new(big.Int).Mul(big.NewInt(startBalance), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))

	// Get issuer account address
	issuerAddress, _ := common.GetEVMDevAccount(0)

	log.Info(log.Node, "Adding EVM issuer account to genesis", "address", issuerAddress.Hex(), "balance", balanceWei.String())

	// Get current verkle tree
	currentTree := sdb.CurrentVerkleTree

	// Insert account BasicData (balance + nonce)
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
	// Copy to offset 16, right-aligned (pad left with zeros if needed)
	copy(basicData[32-len(balanceBytes):32], balanceBytes)

	// Insert BasicData into Verkle tree
	basicDataKey := storage.BasicDataKey(issuerAddress[:])
	if err := currentTree.Insert(basicDataKey, basicData[:], nil); err != nil {
		return fmt.Errorf("failed to insert BasicData: %w", err)
	}

	// Commit the updated verkle tree
	verkleRoot := currentTree.Commit()
	verkleRootBytes := verkleRoot.Bytes()
	verkleRootHash := common.BytesToHash(verkleRootBytes[:])

	// Store the updated verkle tree
	sdb.StoreVerkleTree(verkleRootHash, currentTree)

	// Update stateDB's state root
	r.stateDB.StateRoot = verkleRootHash

	// Create genesis block (block 0) for EVM service
	genesisBlock := &evmtypes.EvmBlockPayload{
		Number:              0,
		WorkPackageHash:     common.Hash{},
		SegmentRoot:         common.Hash{},
		PayloadLength:       148,
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

	// Store genesis block
	if err := sdb.StoreServiceBlock(r.serviceID, genesisBlock, common.Hash{}, 0); err != nil {
		return fmt.Errorf("failed to store genesis block: %w", err)
	}

	log.Info(log.Node, "✅ SubmitEVMGenesis complete", "verkleRoot", verkleRootHash.Hex(), "issuerBalance", balanceWei.String())
	return nil
}

// TransferTriple represents a transfer operation for testing
type TransferTriple struct {
	SenderIndex   int
	ReceiverIndex int
	Amount        *big.Int
}

// createTransferTriplesForRound intelligently generates test transfer cases based on round number
func (r *RollupNode) createTransferTriplesForRound(roundNum int, txnsPerRound int) []TransferTriple {
	const numDevAccounts = 10
	transfers := make([]TransferTriple, 0, txnsPerRound)

	if roundNum == 0 {
		// Special case for round 0: issuer distributes to all accounts
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
		sender := rng.Intn(numDevAccounts-1) + 1
		receiver := rng.Intn(numDevAccounts-1) + 1

		for sender == receiver {
			receiver = rng.Intn(numDevAccounts-1) + 1
		}

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
func (r *RollupNode) DeployContract(contractFile string) (common.Address, error) {
	contractBytecode, err := os.ReadFile(contractFile)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to load contract bytecode from %s: %w", contractFile, err)
	}

	deployerAddress, deployerPrivKeyHex := common.GetEVMDevAccount(0)
	deployerPrivKey, err := crypto.HexToECDSA(deployerPrivKeyHex)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to parse deployer private key: %w", err)
	}

	nonce, err := r.GetTransactionCount(deployerAddress, "latest")
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to get transaction count: %w", err)
	}

	contractAddress := common.Address(crypto.CreateAddress(ethereumCommon.Address(deployerAddress), nonce))
	gasPrice := big.NewInt(1_000_000_000)
	gasLimit := uint64(5_000_000)

	ethTx := ethereumTypes.NewContractCreation(
		nonce,
		big.NewInt(0),
		gasLimit,
		gasPrice,
		contractBytecode,
	)

	signer := ethereumTypes.NewEIP155Signer(big.NewInt(int64(r.serviceID)))
	signedTx, err := ethereumTypes.SignTx(ethTx, signer, deployerPrivKey)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to sign contract deployment transaction: %w", err)
	}

	txBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to encode contract deployment transaction: %w", err)
	}
	multiCoreTxBytes := make([][][]byte, 1)
	multiCoreTxBytes[0] = [][]byte{txBytes}

	_, err = r.SubmitEVMTransactions(multiCoreTxBytes)
	if err != nil {
		return common.Address{}, fmt.Errorf("contract deployment failed: %w", err)
	}

	return contractAddress, nil
}

// SaveWorkPackageBundle saves a WorkPackageBundle to disk
func (r *RollupNode) SaveWorkPackageBundle(bundle *types.WorkPackageBundle, filename string) error {
	encoded := bundle.Encode()
	return os.WriteFile(filename, encoded, 0644)
}

// getTransactionReceipt fetches a transaction receipt from RollupNode's stateDB
func (r *RollupNode) getTransactionReceipt(txHash common.Hash) (*evmtypes.TransactionReceipt, error) {
	receiptObjectID := evmtypes.TxToObjectID(txHash)
	witness, found, err := r.stateDB.ReadObject(r.serviceID, receiptObjectID)
	if err != nil {
		return nil, fmt.Errorf("ReadObject failed for receipt %s: %w", txHash.Hex(), err)
	}
	if !found {
		return nil, fmt.Errorf("receipt not found for tx %s", txHash.Hex())
	}

	receipt, err := evmtypes.ParseRawReceipt(witness)
	if err != nil {
		return nil, fmt.Errorf("failed to parse receipt: %w", err)
	}

	return receipt, nil
}
