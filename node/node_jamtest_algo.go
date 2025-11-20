//go:build network_test
// +build network_test

package node

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/types"
)

func algo(n1 JNode, testServices map[string]*types.TestService, targetN int) {

	isSimple := targetN == 39
	service0 := testServices["algo"]

	algo_serviceIdx := service0.ServiceCode
	auth_serviceIdx := uint32(statedb.AuthCopyServiceCode)

	log.Info(log.Node, "ALGO START", "algo", algo_serviceIdx, "auth", auth_serviceIdx, "targetN", targetN, "isSimple", isSimple)

	for algoN := 1; algoN < targetN; algoN++ {
		imported := []types.ImportSegment{}
		algo_payload := statedb.GenerateAlgoPayload(algoN, isSimple)

		auth_payload := make([]byte, 4)
		binary.LittleEndian.PutUint32(auth_payload, uint32(auth_serviceIdx))
		wp := types.WorkPackage{
			AuthCodeHost:          0,
			AuthorizationToken:    nil, // null-authorizer
			AuthorizationCodeHash: getBootstrapAuthCodeHash(),
			ConfigurationBlob:     nil,
			WorkItems: []types.WorkItem{
				{
					Service:            algo_serviceIdx,
					CodeHash:           service0.CodeHash,
					Payload:            algo_payload,
					RefineGasLimit:     types.RefineGasAllocation / 2,
					AccumulateGasLimit: types.AccumulationGasAllocation,
					ImportedSegments:   imported,
					ExportCount:        uint16(algoN),
				},
			},
		}

		wpr := &types.WorkPackageBundle{
			WorkPackage:   wp,
			ExtrinsicData: []types.ExtrinsicsBlobs{},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		wr, err := RobustSubmitAndWaitForWorkPackageBundles(ctx, n1, []*types.WorkPackageBundle{wpr})
		cancel()
		if err != nil {
			log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
			return
		}
		k := common.ServiceStorageKey(algo_serviceIdx, []byte{0})
		data, _, _ := n1.GetServiceStorage(algo_serviceIdx, k)
		log.Info(log.Node, "algo", "workPackageHash", wr.AvailabilitySpec.WorkPackageHash, "exportedSegmentRoot", wr.AvailabilitySpec.ExportedSegmentRoot, "result", fmt.Sprintf("%x", data))
	}
}

func evm(n1 JNode, testServices map[string]*types.TestService, targetN int) {

	service0 := testServices["evm"]
	evm_serviceIdx := service0.ServiceCode

	log.Info(log.Node, "EVM START", "evm", evm_serviceIdx, "targetN", targetN)
	// Set USDM initial balances and nonces
	blobs := types.ExtrinsicsBlobs{}
	workItems := []types.WorkItemExtrinsic{}
	totalSupplyValue := new(big.Int).Mul(big.NewInt(61_000_000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	usdmInitialState := []statedb.MappingEntry{
		{Slot: 0, Key: evmtypes.IssuerAddress, Value: totalSupplyValue}, // balanceOf[issuer]
		{Slot: 1, Key: evmtypes.IssuerAddress, Value: big.NewInt(1)},    // nonces[issuer]
	}
	statedb.InitializeMappings(&blobs, &workItems, evmtypes.UsdmAddress, usdmInitialState)

	// Add witness for genesis block (block 0) and calculate segments needed
	witnesses := []types.StateWitnessRaw{}
	objectID := evmtypes.BlockNumberToObjectID(0)
	witness, ok, _, err := n1.ReadStateWitnessRaw(evm_serviceIdx, objectID)
	if err != nil {
		log.Warn(log.Node, "SubmitEVMGenesis: ReadStateWitnessRaw failed", "blockNum", 0, "objectID", objectID, "err", err)
		panic(fmt.Sprintf("ReadStateWitnessRaw failed for genesis block: %v", err))
	} else if ok {
		// Calculate number of segments needed for genesis block's payload
		//numSegments := (int(witness.PayloadLength) + types.SegmentSize - 1) / types.SegmentSize

		log.Info(log.Node, "Adding witness for genesis block", "blockNum", 0, "objectID", objectID, "payload_length", witness.PayloadLength)
		witnesses = append(witnesses, witness)
	} else {
		log.Warn(log.Node, "SubmitEVMGenesis: witness not found for genesis block", "objectID", objectID)
	}

	numExtrinsics := len(workItems)
	// Append witness as extrinsic
	for _, witness := range witnesses {
		witnessBytes := witness.SerializeWitnessRaw()
		blobs = append(blobs, witnessBytes)
		workItems = append(workItems, types.WorkItemExtrinsic{
			Hash: common.Blake2Hash(witnessBytes),
			Len:  uint32(len(witnessBytes)),
		})
	}

	witnessCount := len(witnesses)
	log.Info(log.Node, "SubmitEVMGenesis (genesis)", "numExtrinsics", numExtrinsics, "witnessCount", witnessCount)

	service, ok, err := n1.GetService(evm_serviceIdx)
	if err != nil || !ok {
		panic(fmt.Sprintf("EVM service not found: %v", err))
	}

	// Create work package with updated witness count and refine context
	wp := statedb.DefaultWorkPackage(evm_serviceIdx, service)
	wp.RefineContext, _ = n1.GetRefineContext()
	wp.WorkItems[0].Payload = statedb.BuildPayload(statedb.PayloadTypeBlocks, numExtrinsics, witnessCount)
	wp.WorkItems[0].Extrinsics = workItems

	// Genesis adds 2 exports: 1 StorageShard + 1 SSRMetadata for USDM contract
	wp.WorkItems[0].ExportCount = uint16(2)
	log.Info(log.Node, "WorkPackage RefineContext", "state_root", wp.RefineContext.StateRoot.Hex(), "anchor", wp.RefineContext.Anchor.Hex())
	wpr := &types.WorkPackageBundle{
		WorkPackage:   wp,
		ExtrinsicData: []types.ExtrinsicsBlobs{blobs},
	}
	ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
	_, err = RobustSubmitAndWaitForWorkPackageBundles(ctx, n1, []*types.WorkPackageBundle{wpr})
	cancel()
	if err != nil {
		log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
		return
	}

	// Create signed transactions
	prevBlockNumber := uint32(0)
	for evmN := 1; evmN < targetN; evmN++ {
		// Build transaction extrinsics
		numTxs := evmN
		txBytes := make([][]byte, numTxs)
		txHashes := make([]common.Hash, numTxs)

		// Create simple transfers from account 0 to account 1
		senderAddr, senderPrivKey := common.GetEVMDevAccount(0)
		recipientAddr, _ := common.GetEVMDevAccount(1)

		for i := 0; i < numTxs; i++ {
			gasPrice := big.NewInt(1) // 1 wei
			gasLimit := uint64(2_000_000)
			nonce := uint64(prevBlockNumber*uint32(targetN) + uint32(i))
			amount := big.NewInt(1) // 1 wei transfer

			_, tx, txHash, err := statedb.CreateSignedUSDMTransfer(
				senderPrivKey,
				nonce,
				recipientAddr,
				amount,
				gasPrice,
				gasLimit,
				uint64(evm_serviceIdx),
			)
			if err != nil {
				log.Error(log.Node, "CreateSignedUSDMTransfer ERR", "err", err)
				return
			}

			txBytes[i] = tx
			txHashes[i] = txHash
		}

		// Build extrinsics blobs and hashes
		blobs := make(types.ExtrinsicsBlobs, numTxs)
		extrinsics := make([]types.WorkItemExtrinsic, numTxs)
		for i, tx := range txBytes {
			blobs[i] = tx
			extrinsics[i] = types.WorkItemExtrinsic{
				Hash: common.Blake2Hash(tx),
				Len:  uint32(len(tx)),
			}
		}

		// Build payload (PayloadTypeTransactions, count, numWitnesses)
		numWitnesses := 0
		if prevBlockNumber > 0 {
			numWitnesses = 1 // Witness previous block
		}
		evm_payload := make([]byte, 7)
		evm_payload[0] = 0x01 // PayloadTypeTransactions
		binary.LittleEndian.PutUint32(evm_payload[1:5], uint32(numTxs))
		binary.LittleEndian.PutUint16(evm_payload[5:7], uint16(numWitnesses))

		wp := types.WorkPackage{
			AuthCodeHost:          0,
			AuthorizationToken:    nil,
			AuthorizationCodeHash: getBootstrapAuthCodeHash(),
			ConfigurationBlob:     nil,
			WorkItems: []types.WorkItem{
				{
					Service:            evm_serviceIdx,
					CodeHash:           service0.CodeHash,
					Payload:            evm_payload,
					RefineGasLimit:     types.RefineGasAllocation / 2,
					AccumulateGasLimit: types.AccumulationGasAllocation,
					ImportedSegments:   []types.ImportSegment{},
					ExportCount:        uint16(numTxs * 2), // Approximate: receipts + logs
					Extrinsics:         extrinsics,
				},
			},
		}

		rawObjectIDs := []common.Hash{} // TODO (fetch witnesses for previous block)
		bundle2, _, err := n1.BuildBundle(wp, []types.ExtrinsicsBlobs{blobs}, uint16(0), rawObjectIDs)
		if err != nil {
			panic(err)
		}

		wpr := &types.WorkPackageBundle{
			WorkPackage:   bundle2.WorkPackage,
			ExtrinsicData: bundle2.ExtrinsicData,
		}

		log.Info(log.Node, "Submitting EVM workpackage",
			"round", evmN,
			"numTxs", numTxs,
			"from", senderAddr.String(),
			"to", recipientAddr.String(),
			"prevBlock", prevBlockNumber)

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		wr, err := RobustSubmitAndWaitForWorkPackageBundles(ctx, n1, []*types.WorkPackageBundle{wpr})
		cancel()
		if err != nil {
			log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
			return
		}

		log.Info(log.Node, "EVM workpackage completed",
			"round", evmN,
			"workPackageHash", wr.AvailabilitySpec.WorkPackageHash,
			"exportedSegmentRoot", wr.AvailabilitySpec.ExportedSegmentRoot)

		prevBlockNumber++
	}
}
