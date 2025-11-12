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
			AuthorizationCodeHash: bootstrap_auth_codehash,
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

		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("ALGO(%d)", algoN),
			WorkPackage:     wp,
			ExtrinsicsBlobs: types.ExtrinsicsBlobs{},
		}

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		wr, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
		cancel()
		if err != nil {
			log.Error(log.Node, "SubmitAndWaitForWorkPackages ERR", "err", err)
			return
		}
		k := common.ServiceStorageKey(algo_serviceIdx, []byte{0})
		data, _, _ := n1.GetServiceStorage(algo_serviceIdx, k)
		log.Info(log.Node, wpr.Identifier, "workPackageHash", wr.AvailabilitySpec.WorkPackageHash, "exportedSegmentRoot", wr.AvailabilitySpec.ExportedSegmentRoot, "result", fmt.Sprintf("%x", data))
		//log.Info(log.Node, wpr.Identifier, "workPackageHash", wp.Hash(), "wr", wr.String())
	}
}

func evm(n1 JNode, testServices map[string]*types.TestService, targetN int) {

	service0 := testServices["evm"]
	evm_serviceIdx := service0.ServiceCode

	log.Info(log.Node, "EVM START", "evm", evm_serviceIdx, "targetN", targetN)

	// Note: EVM service and USDM contract are already set up in genesis
	// No need for a genesis workpackage here

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
			AuthorizationCodeHash: bootstrap_auth_codehash,
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

		wpr := &WorkPackageRequest{
			Identifier:      fmt.Sprintf("EVM(%d) - %d txs", evmN, numTxs),
			WorkPackage:     wp,
			ExtrinsicsBlobs: blobs,
		}

		log.Info(log.Node, "Submitting EVM workpackage",
			"round", evmN,
			"numTxs", numTxs,
			"from", senderAddr.String(),
			"to", recipientAddr.String(),
			"prevBlock", prevBlockNumber)

		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout*maxRobustTries)
		wr, err := RobustSubmitAndWaitForWorkPackages(ctx, n1, []*WorkPackageRequest{wpr})
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
