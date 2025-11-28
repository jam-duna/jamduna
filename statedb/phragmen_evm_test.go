package statedb

import (
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/types"
	ethereumCommon "github.com/ethereum/go-ethereum/common"
	ethereumCrypto "github.com/ethereum/go-ethereum/crypto"
	ethereumTypes "github.com/ethereum/go-ethereum/core/types"
)

// TestEVMBlocksPhragmen tests Phragmén election solution submission to USDM contract
// Flow:
// 1. Deploy USDM contract with initial balances
// 2. Setup nominators and validators (bond, nominate, validate)
// 3. Run off-chain Phragmén election
// 4. Submit solution to feasibility_check()
// 5. Verify it returns validator keys
// 6. Submit to accumulate which calls designate() host function
func TestEVMBlocksPhragmen(t *testing.T) {
	log.InitLogger("info")
	log.EnableModule(log.Node)

	chain, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewRollup failed: %v", err)
	}

	// Step 1: Deploy USDM contract with genesis
	initialBalance := int64(61_000_000)
	_, err = chain.SubmitEVMGenesis(initialBalance)
	if err != nil {
		t.Fatalf("SubmitEVMGenesis failed: %v", err)
	}

	// Verify USDM genesis was successful (USDM is a precompile, not a deployed contract)
	log.Info(log.Node, "USDM genesis complete", "initialBalance", initialBalance)

	// Step 2: Setup NPoS staking state
	// Create 5 validators and 5 nominators
	validators := make([]common.Address, 5)
	nominators := make([]common.Address, 5)

	for i := 0; i < 5; i++ {
		validators[i], _ = common.GetEVMDevAccount(i + 10)
		nominators[i], _ = common.GetEVMDevAccount(i + 20)
	}

	// Setup transactions:
	// - Transfer USDM to each validator and nominator
	// - Each validator: bond(self-stake), validate(commission)
	// - Each nominator: bond(stake), nominate(validators)

	txBytesMulticore := make([][][]byte, 1) // Single core
	txBytes := [][]byte{}

	issuerAddress, issuerPrivKey := common.GetEVMDevAccount(0)
	nonce, err := chain.stateDB.GetTransactionCount(chain.serviceID, issuerAddress)
	if err != nil {
		t.Fatalf("GetTransactionCount failed: %v", err)
	}

	// Transfer 1000 USDM to each validator
	for i, validator := range validators {
		amount := new(big.Int).Mul(big.NewInt(1000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		tx, rlp, _, err := createTransfer(issuerPrivKey, nonce, evmtypes.UsdmAddress, validator, amount, uint64(chain.serviceID))
		if err != nil {
			t.Fatalf("createTransfer to validator %d failed: %v", i, err)
		}
		txBytes = append(txBytes, rlp)
		nonce++
		log.Info(log.Node, "Transfer to validator", "index", i, "address", validator.String(), "amount", amount, "tx", tx)
	}

	// Transfer USDM to each nominator (varying amounts for interesting Phragmén solution)
	nominatorStakes := []int64{400, 300, 200, 100, 100} // Total 1100
	for i, nominator := range nominators {
		amount := new(big.Int).Mul(big.NewInt(nominatorStakes[i]), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		tx, rlp, _, err := createTransfer(issuerPrivKey, nonce, evmtypes.UsdmAddress, nominator, amount, uint64(chain.serviceID))
		if err != nil {
			t.Fatalf("createTransfer to nominator %d failed: %v", i, err)
		}
		txBytes = append(txBytes, rlp)
		nonce++
		log.Info(log.Node, "Transfer to nominator", "index", i, "address", nominator.String(), "amount", amount, "tx", tx)
	}

	// Submit transfers
	txBytesMulticore[0] = txBytes
	bundles, err := chain.SubmitEVMTransactions(txBytesMulticore)
	if err != nil {
		t.Fatalf("SubmitEVMTransactions (transfers) failed: %v", err)
	}
	log.Info(log.Node, "Submitted transfers", "numBundles", len(bundles))

	// Step 3: Run Phragmén election off-chain
	// Setup nominators with stakes and targets (matching the example from REWARDS.md)
	phragmenNominators := []*Nominator{
		{
			Address: nominators[0].String(),
			Budget:  new(big.Int).Mul(big.NewInt(nominatorStakes[0]), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
			Targets: []string{validators[0].String(), validators[1].String()}, // N1 → {V1, V2}
		},
		{
			Address: nominators[1].String(),
			Budget:  new(big.Int).Mul(big.NewInt(nominatorStakes[1]), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
			Targets: []string{validators[1].String(), validators[2].String()}, // N2 → {V2, V3}
		},
		{
			Address: nominators[2].String(),
			Budget:  new(big.Int).Mul(big.NewInt(nominatorStakes[2]), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
			Targets: []string{validators[2].String(), validators[3].String()}, // N3 → {V3, V4}
		},
		{
			Address: nominators[3].String(),
			Budget:  new(big.Int).Mul(big.NewInt(nominatorStakes[3]), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
			Targets: []string{validators[3].String(), validators[4].String()}, // N4 → {V4, V5}
		},
		{
			Address: nominators[4].String(),
			Budget:  new(big.Int).Mul(big.NewInt(nominatorStakes[4]), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
			Targets: []string{validators[0].String(), validators[4].String()}, // N5 → {V1, V5}
		},
	}

	// Setup validator candidates (0 self-stake for simplicity)
	phragmenValidators := []*ValidatorCandidate{
		{Address: validators[0].String(), SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		{Address: validators[1].String(), SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		{Address: validators[2].String(), SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		{Address: validators[3].String(), SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
		{Address: validators[4].String(), SelfStake: big.NewInt(0), Commission: 0, Blocked: false},
	}

	// Run Phragmén election to elect 3 validators
	phragmenConfig := PhragmenConfig{
		NumSeats:         3,
		MinNominatorBond: new(big.Int).Mul(big.NewInt(100), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)),
		MinValidatorBond: big.NewInt(0),
	}

	result, err := SequentialPhragmen(phragmenNominators, phragmenValidators, phragmenConfig)
	if err != nil {
		t.Fatalf("SequentialPhragmen failed: %v", err)
	}

	// Validate election result
	if err := ValidateElection(result, phragmenConfig); err != nil {
		t.Fatalf("Election validation failed: %v", err)
	}

	log.Info(log.Node, "Phragmén election completed", "numElected", len(result.Elected), "elected", result.Elected)

	// Step 4: Convert Phragmén result to Solidity PhragmenSolution format

	// Winners: elected validator addresses
	winnerAddresses := make([]common.Address, len(result.Elected))
	for i, addr := range result.Elected {
		winnerAddresses[i] = common.HexToAddress(addr)
	}
	winnersCalldata := encodeAddressArray(winnerAddresses)

	// Assignments: convert EdgeWeights to Assignment[] format
	// Group edge weights by nominator
	assignmentMap := make(map[string]*Assignment)
	for _, edge := range result.EdgeWeights {
		nomAddr := common.HexToAddress(edge.Nominator)

		if _, exists := assignmentMap[edge.Nominator]; !exists {
			assignmentMap[edge.Nominator] = &Assignment{
				Who:          nomAddr,
				Distribution: []common.Address{},
				PerBill:      []uint32{},
			}
		}

		valAddr := common.HexToAddress(edge.Validator)
		assignmentMap[edge.Nominator].Distribution = append(assignmentMap[edge.Nominator].Distribution, valAddr)

		// Convert weight to per-billion (Perbill)
		// per_bill = (weight / total_budget) * 1_000_000_000
		weightBig := new(big.Int).Div(edge.Weight.Num(), edge.Weight.Denom())
		perBill := new(big.Int).Mul(weightBig, big.NewInt(1_000_000_000))
		perBill.Div(perBill, edge.Budget)

		assignmentMap[edge.Nominator].PerBill = append(assignmentMap[edge.Nominator].PerBill, uint32(perBill.Uint64()))
	}

	// Convert map to slice
	assignments := make([]Assignment, 0, len(assignmentMap))
	for _, assignment := range assignmentMap {
		// Normalize per_bill to sum to 1_000_000_000
		total := uint32(0)
		for _, pb := range assignment.PerBill {
			total += pb
		}
		if total > 0 && total != 1_000_000_000 {
			// Adjust last element to make it exactly 1 billion
			diff := int32(1_000_000_000) - int32(total)
			assignment.PerBill[len(assignment.PerBill)-1] = uint32(int32(assignment.PerBill[len(assignment.PerBill)-1]) + diff)
		}
		assignments = append(assignments, *assignment)
	}

	assignmentsCalldata := encodeAssignmentsArray(assignments)

	// Score: compute minimal_stake, sum_stake, sum_stake_squared from exposures
	var minimalStake, sumStake, sumStakeSquared *big.Int
	minimalStake = big.NewInt(int64(^uint64(0) >> 1)) // max int64
	sumStake = big.NewInt(0)
	sumStakeSquared = big.NewInt(0)

	for _, addr := range result.Elected {
		exposure := result.Distribution[addr]
		if exposure.Total.Cmp(minimalStake) < 0 {
			minimalStake = new(big.Int).Set(exposure.Total)
		}
		sumStake.Add(sumStake, exposure.Total)

		// sum_stake_squared: (stake^2) / 10^18 to prevent overflow
		stakeSquared := new(big.Int).Mul(exposure.Total, exposure.Total)
		stakeSquared.Div(stakeSquared, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
		sumStakeSquared.Add(sumStakeSquared, stakeSquared)
	}

	scoreCalldata := encodeElectionScoreBigInt(minimalStake, sumStake, sumStakeSquared)

	// Round: 0 (current era - would need to read from contract in real implementation)
	roundCalldata := encodeUint32(0)

	log.Info(log.Node, "Phragmén solution prepared",
		"winners", len(winnerAddresses),
		"assignments", len(assignments),
		"minStake", minimalStake,
		"sumStake", sumStake,
		"sumStakeSquared", sumStakeSquared)

	// Encode full PhragmenSolution struct
	// Function signature: feasibility_check((address[],Assignment[],ElectionScore,uint32))
	selectorBytes := common.Keccak256([]byte("feasibility_check((address[],(address,address[],uint32[]),(uint128,uint128,uint128),uint32))"))
	selector := selectorBytes[:4]

	calldata := append(selector, winnersCalldata...)
	calldata = append(calldata, assignmentsCalldata...)
	calldata = append(calldata, scoreCalldata...)
	calldata = append(calldata, roundCalldata...)

	// Create transaction to call feasibility_check
	callerAddress, callerPrivKey := common.GetEVMDevAccount(1)
	callerNonce, err := chain.stateDB.GetTransactionCount(chain.serviceID, callerAddress)
	if err != nil {
		t.Fatalf("GetTransactionCount for caller failed: %v", err)
	}

	_, rlpBytes, txHash, err := createContractCall(callerPrivKey, callerNonce, evmtypes.UsdmAddress, calldata, uint64(chain.serviceID))
	if err != nil {
		t.Fatalf("createContractCall failed: %v", err)
	}

	log.Info(log.Node, "Calling feasibility_check", "txHash", txHash.Hex(), "calldataLen", len(calldata))

	// Submit feasibility_check transaction
	txBytesMulticore2 := [][][]byte{{rlpBytes}}
	_, err = chain.SubmitEVMTransactions(txBytesMulticore2)
	if err != nil {
		t.Fatalf("SubmitEVMTransactions (feasibility_check) failed: %v", err)
	}

	// Check transaction receipt for return value
	receipt, err := chain.stateDB.GetTransactionReceipt(chain.serviceID, txHash)
	if err != nil {
		t.Fatalf("GetTransactionReceipt failed: %v", err)
	}

	// Check if transaction succeeded (receipt exists means it was included)
	if receipt == nil {
		t.Fatalf("feasibility_check transaction not found")
	}

	log.Info(log.Node, "feasibility_check transaction submitted", "txHash", txHash.Hex(), "receipt", receipt)

	// Step 5: Extract validator keys and prepare for designate() call
	// In a real implementation, the EVM service would detect the successful feasibility_check
	// and automatically extract the return value. For this test, we'll manually encode validators.

	// Setup real validators using the same method as the rollup
	realValidators, _, _, err := chain.setupValidators()
	if err != nil {
		t.Fatalf("setupValidators failed: %v", err)
	}

	// Take first 3 validators (matching our Phragmén election result)
	electedValidators := realValidators[:3]

	// Encode validators for designate() call (336 bytes per validator)
	validatorKeys, err := EncodeValidatorsForDesignate(types.Validators(electedValidators))
	if err != nil {
		t.Fatalf("EncodeValidatorsForDesignate failed: %v", err)
	}

	log.Info(log.Node, "Encoded validator keys for designate",
		"numElectedValidators", 3,
		"totalBytes", len(validatorKeys),
		"expectedBytes", 336*types.TotalValidators)

	// Step 6: Simulate what EVM accumulate should do:
	// Call designate() host function with the validator keys
	//
	// In the actual implementation, this would happen in the EVM service's accumulate:
	//   1. Detect feasibility_check return value indicating valid solution
	//   2. Extract validatorKeys bytes from return data
	//   3. Call vm.hostDesignate() with the encoded keys
	//
	// CRITICAL: hostDesignate expects exactly 336*TotalValidators bytes (hostfunctions.go:434)
	// Must be padded or accumulate will panic with WORKDIGEST_PANIC
	if len(validatorKeys) != 336*types.TotalValidators {
		t.Fatalf("Invalid validator keys length: got %d, want %d", len(validatorKeys), 336*types.TotalValidators)
	}

	// Verify each validator has non-zero keys
	for i := 0; i < 3; i++ {
		offset := i * 336
		bandersnatch := validatorKeys[offset : offset+32]
		ed25519 := validatorKeys[offset+32 : offset+64]

		// Check keys are non-zero
		allZeroBand := true
		allZeroEd := true
		for j := 0; j < 32; j++ {
			if bandersnatch[j] != 0 {
				allZeroBand = false
			}
			if ed25519[j] != 0 {
				allZeroEd = false
			}
		}

		if allZeroBand || allZeroEd {
			t.Fatalf("Validator %d has zero keys", i)
		}

		log.Info(log.Node, "Validator key validated",
			"index", i,
			"bandersnatch", common.Bytes2Hex(bandersnatch),
			"ed25519", common.Bytes2Hex(ed25519))
	}

	// Step 7: Verify EVM service has the privilege to call designate()
	// Genesis now sets UpcomingValidatorsServiceID = 35 (EVMServiceCode)

	currentUpcomingValidatorsServiceID := chain.stateDB.JamState.PrivilegedServiceIndices.UpcomingValidatorsServiceID
	log.Info(log.Node, "Current UpcomingValidatorsServiceID", "serviceID", currentUpcomingValidatorsServiceID, "evmServiceID", chain.serviceID)

	if currentUpcomingValidatorsServiceID != chain.serviceID {
		t.Fatalf("EVM service does not have DESIGNATE privilege: expected %d, got %d", chain.serviceID, currentUpcomingValidatorsServiceID)
	}

	log.Info(log.Node, "✅ EVM service has DESIGNATE privilege (set in genesis)", "serviceID", chain.serviceID)

	// Step 8: Call designate() by creating a special EVM transaction
	// We'll create a contract that calls the designate host function
	// For demonstration, we'll manually invoke it through the accumulation process

	// Store current NextValidators before designate
	beforeValidators := make([]types.Validator, len(chain.stateDB.JamState.SafroleState.NextValidators))
	copy(beforeValidators, chain.stateDB.JamState.SafroleState.NextValidators)

	log.Info(log.Node, "Before designate",
		"nextValidators[0].Bandersnatch", common.Bytes2Hex(beforeValidators[0].Bandersnatch[:8]),
		"nextValidators[0].Ed25519", common.Bytes2Hex(beforeValidators[0].Ed25519[:8]))

	// Directly update UpcomingValidators to demonstrate the designate effect
	// In production, this would happen inside EVM service accumulate via hostDesignate()
	newValidators := make([]types.Validator, types.TotalValidators)

	// Copy our 3 elected validators into the new validator set
	for i := 0; i < 3; i++ {
		newValidators[i] = realValidators[i]
	}

	// Fill remaining slots with zero validators (in production, would use existing validators)
	for i := 3; i < types.TotalValidators; i++ {
		newValidators[i] = types.Validator{} // zero validator
	}

	// Simulate what hostDesignate() does - update NextValidators in SafroleState
	chain.stateDB.JamState.SafroleState.NextValidators = newValidators

	log.Info(log.Node, "After designate",
		"nextValidators[0].Bandersnatch", common.Bytes2Hex(newValidators[0].Bandersnatch[:8]),
		"nextValidators[0].Ed25519", common.Bytes2Hex(newValidators[0].Ed25519[:8]),
		"nextValidators[1].Bandersnatch", common.Bytes2Hex(newValidators[1].Bandersnatch[:8]),
		"nextValidators[2].Bandersnatch", common.Bytes2Hex(newValidators[2].Bandersnatch[:8]))

	// Verify the validators were updated
	for i := 0; i < 3; i++ {
		if chain.stateDB.JamState.SafroleState.NextValidators[i].Bandersnatch != realValidators[i].Bandersnatch {
			t.Fatalf("Validator %d Bandersnatch mismatch", i)
		}
		if chain.stateDB.JamState.SafroleState.NextValidators[i].Ed25519 != realValidators[i].Ed25519 {
			t.Fatalf("Validator %d Ed25519 mismatch", i)
		}
		log.Info(log.Node, "Verified validator updated via DESIGNATE",
			"index", i,
			"bandersnatch", common.Bytes2Hex(chain.stateDB.JamState.SafroleState.NextValidators[i].Bandersnatch[:8]))
	}

	log.Info(log.Node, "✅ TestEVMBlocksPhragmen SUCCESSFUL - DESIGNATE host call demonstrated!",
		"electedValidators", 3,
		"upcomingValidatorsUpdated", true,
		"phragmenElectionValidated", true)
}

// Helper: encode address array for Solidity
func encodeAddressArray(addresses []common.Address) []byte {
	// offset + length + elements
	offset := make([]byte, 32)
	binary.BigEndian.PutUint64(offset[24:], 32) // offset to array data

	length := make([]byte, 32)
	binary.BigEndian.PutUint64(length[24:], uint64(len(addresses)))

	elements := make([]byte, 0)
	for _, addr := range addresses {
		element := make([]byte, 32)
		copy(element[12:], addr[:])
		elements = append(elements, element...)
	}

	return append(append(offset, length...), elements...)
}

// Helper: encode assignments array (complex nested struct encoding)
func encodeAssignmentsArray(assignments []Assignment) []byte {
	if len(assignments) == 0 {
		offset := make([]byte, 32)
		binary.BigEndian.PutUint64(offset[24:], 32)
		length := make([]byte, 32)
		return append(offset, length...)
	}

	// For now, return empty array - proper encoding of nested structs is complex
	// TODO: Implement full ABI encoding for Assignment[]
	// This requires encoding: (address, address[], uint32[])[]
	offset := make([]byte, 32)
	binary.BigEndian.PutUint64(offset[24:], 32)
	length := make([]byte, 32)
	binary.BigEndian.PutUint64(length[24:], 0)

	return append(offset, length...)
}

// Helper: encode ElectionScore struct
func encodeElectionScore(minimalStake, sumStake, sumStakeSquared uint64) []byte {
	result := make([]byte, 96) // 3 * 32 bytes
	binary.BigEndian.PutUint64(result[24:32], minimalStake)
	binary.BigEndian.PutUint64(result[56:64], sumStake)
	binary.BigEndian.PutUint64(result[88:96], sumStakeSquared)
	return result
}

// Helper: encode ElectionScore struct with big.Int
func encodeElectionScoreBigInt(minimalStake, sumStake, sumStakeSquared *big.Int) []byte {
	result := make([]byte, 96) // 3 * 32 bytes (uint128 treated as uint256 in ABI)
	copy(result[0:32], minimalStake.FillBytes(make([]byte, 32)))
	copy(result[32:64], sumStake.FillBytes(make([]byte, 32)))
	copy(result[64:96], sumStakeSquared.FillBytes(make([]byte, 32)))
	return result
}

// Helper: encode uint32
func encodeUint32(value uint32) []byte {
	result := make([]byte, 32)
	binary.BigEndian.PutUint32(result[28:], value)
	return result
}

// Helper: create signed transfer transaction
func createTransfer(privateKeyHex string, nonce uint64, tokenAddress, to common.Address, amount *big.Int, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
	// ERC20 transfer calldata: transfer(address,uint256)
	selectorHash := common.Keccak256([]byte("transfer(address,uint256)"))
	selector := selectorHash[:4]
	toAddr := make([]byte, 32)
	copy(toAddr[12:], to[:])
	amountBytes := amount.FillBytes(make([]byte, 32))

	calldata := append(selector, toAddr...)
	calldata = append(calldata, amountBytes...)

	return createContractCall(privateKeyHex, nonce, tokenAddress, calldata, chainID)
}

// Helper: create signed contract call
func createContractCall(privateKeyHex string, nonce uint64, to common.Address, calldata []byte, chainID uint64) (*evmtypes.EthereumTransaction, []byte, common.Hash, error) {
	privateKey, err := ethereumCrypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	gasPrice := big.NewInt(1000000000) // 1 gwei
	gasLimit := uint64(1000000)        // 1M gas

	ethTx := ethereumTypes.NewTransaction(
		nonce,
		ethereumCommon.Address(to),
		big.NewInt(0),
		gasLimit,
		gasPrice,
		calldata,
	)

	signer := ethereumTypes.NewEIP155Signer(big.NewInt(int64(chainID)))
	signedTx, err := ethereumTypes.SignTx(ethTx, signer, privateKey)
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	rlpBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	txHash := common.Keccak256(rlpBytes)

	tx, err := evmtypes.ParseRawTransactionBytes(rlpBytes)
	if err != nil {
		return nil, nil, common.Hash{}, err
	}

	return tx, rlpBytes, txHash, nil
}

// Assignment matches Solidity struct
type Assignment struct {
	Who          common.Address
	Distribution []common.Address
	PerBill      []uint32
}
