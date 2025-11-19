package statedb

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/types"
)

const (
	numBlocks = 53
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
	for n := 0; n <= numBlocks; n++ {
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
		var err error
		if n > 0 {
			err = c.processWorkPackageBundlesPipelined(bundles)
		} else {
			err = c.processWorkPackageBundles(bundles)
		}
		if err != nil {
			t.Fatal(err)
		}
		// Advance timeslot for next block
		c.stateDB.JamState.SafroleState.Timeslot++
		block, _, err := c.stateDB.GetBlockByNumber(c.serviceID, "latest")
		if err != nil {
			t.Fatalf("GetBlockByNumber failed: %v", err)
		}
		// Generate metadata for logging and verification
		metadata := evmtypes.NewEvmBlockMetadata(block)

		log.Info(log.Node, "Algo block processed", "blockNumber", block.Number, "round", n, "# txns", len(block.TxHashes), "# receipts", len(block.ReceiptHashes),
			"StateRoot", block.StateRoot,
			"TransactionsRoot", metadata.TransactionsRoot,
			"ReceiptsRoot", metadata.ReceiptsRoot,
			"LogIndexStart", block.LogIndexStart,
			"MmrRoot", metadata.MmrRoot,
			"ExtrinsicsHash", block.ExtrinsicsHash,
			"ParentHeaderHash", block.ParentHeaderHash)

		// BMT proof verification
		// if err := evmtypes.VerifyBlockBMTProofs(block, metadata); err != nil {
		// 	t.Fatalf("BMT proof verification failed for block %d: %v", block.Number, err)
		// }
	}

}

func TestEVMBlocksMath(t *testing.T) {
	chain, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewEVMService failed: %v", err)
	}
	_, _, err = chain.SubmitEVMGenesis()
	if err != nil {
		t.Fatalf("SubmitEVMGenesis failed: %v", err)
	}
	txBytesMulticore := make([][][]byte, types.TotalCores)
	n := uint32(0)
	for i := 0; i <= 3; i++ {
		txBytes, alltopics, err := chain.CallMath(evmtypes.MathAddress, []string{
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
		block, err := chain.SubmitEVMTransactions(txBytesMulticore, n, n+1)
		if err != nil {
			t.Fatalf("SubmitEVMTransactions failed: %v", err)
		}
		log.Info(log.Node, "EVM block processed", "blockNumber", block.Number)
		err = chain.ShowTxReceipts(block, block.TxHashes, "Math Transactions", alltopics)
		if err != nil {
			t.Fatalf("ShowTxReceipts failed: %v", err)
		}

	}
}

func RunTransfersRound(b *Rollup, prevBlockNumber uint32, transfers []TransferTriple) (uint32, error) {
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
			return 0, err
		}
		balance := new(big.Int).SetBytes(balanceHash.Bytes())
		initialBalances[i] = balance
		totalBefore = new(big.Int).Add(totalBefore, balance)

		nonce, err := b.stateDB.GetTransactionCount(b.serviceID, accounts[i])
		if err != nil {
			log.Error(log.Node, "GetTransactionCount ERR", "account", i, "err", err)
			return 0, err
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
			return 0, err
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
	var block *evmtypes.EvmBlockPayload
	var err error
	// Subsequent rounds: witness the previous block
	block, err = b.SubmitEVMTransactions(txBytesMulticore, prevBlockNumber, prevBlockNumber+1)
	if err != nil {
		log.Error(log.Node, "SubmitEVMTransactions ERR", "err", err)
		return 0, err
	}
	if len(block.TxHashes) != len(txBytes) {
		return 0, fmt.Errorf("mismatch in number of tx hashes in finalized block: expected %d, got %d",
			len(txBytes), len(block.TxHashes))
	}

	log.Info(log.Node, "EVM block submitted", "blockNumber", block.Number,
		"# txns", len(block.TxHashes), "# receipts", len(block.ReceiptHashes),
		"StateRoot", block.StateRoot,
		"LogIndexStart", block.LogIndexStart,
		"ExtrinsicsHash", block.ExtrinsicsHash,
		"ParentHeaderHash", block.ParentHeaderHash)

	// PREVIOUS BLOCK VERIFICATION
	prevblock, metadata, err := b.stateDB.GetBlockByNumber(b.serviceID, fmt.Sprintf("0x%x", prevBlockNumber))
	if err != nil {

		return 0, fmt.Errorf("GetBlockByNumber failed for block %d: %w", prevBlockNumber, err)
	}
	log.Info(log.Node, "Finalized Block", "prevBlockNumber", prevBlockNumber, "blockNumber", prevblock.Number,
		"# txns", len(prevblock.TxHashes), "# receipts", len(prevblock.ReceiptHashes),
		"TransactionsRoot", metadata.TransactionsRoot,
		"ReceiptsRoot", metadata.ReceiptsRoot,
		"MmrRoot", metadata.MmrRoot)
	// if err := evmtypes.VerifyBlockBMTProofs(prevblock, metadata); err != nil {
	// 	panic("VerifyBlockBMTProofs failed: " + err.Error())
	// }
	// if err := b.ShowTxReceipts(block, txHashes, fmt.Sprintf("Multitransfer (%d transfers)", len(transfers)), defaultTopics()); err != nil {
	// 	return 0, fmt.Errorf("failed to show transfer receipts: %w", err)
	// }

	// Check balances and nonces after transfers
	totalAfter := big.NewInt(0)
	for i := 0; i < numAccounts; i++ {
		balanceHash, err := b.stateDB.GetBalance(b.serviceID, accounts[i])
		if err != nil {
			log.Error(log.Node, "GetBalance ERR", "account", i, "err", err)
			return 0, err
		}
		balance := new(big.Int).SetBytes(balanceHash.Bytes())
		totalAfter = new(big.Int).Add(totalAfter, balance)

		nonce, err := b.stateDB.GetTransactionCount(b.serviceID, accounts[i])
		if err != nil {
			log.Error(log.Node, "GetTransactionCount ERR", "account", i, "err", err)
			return 0, err
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
		return 0, fmt.Errorf("balance mismatch: before=%s, after=%s, difference=%s", totalBefore.String(), totalAfter.String(), diff.String())
	}

	// Verify coinbase collected fees
	coinbaseBalance, err := b.stateDB.GetBalance(b.serviceID, coinbaseAddress)
	if err != nil {
		return 0, fmt.Errorf("GetBalance (coinbase) failed: %w", err)
	}
	coinbaseBalanceBig := new(big.Int).SetBytes(coinbaseBalance.Bytes())
	if coinbaseBalanceBig.Cmp(big.NewInt(0)) == 0 {
		return 0, fmt.Errorf("coinbase balance is 0 - fee collection failed")
	}
	log.Info(log.Node, "Coinbase balance after round", "address", coinbaseAddress.String(), "balance", coinbaseBalanceBig.String())

	return uint32(block.Number), nil
}

// TestEVMBlocksTransfers runs multiple rounds of transfers
func TestEVMBlocksTransfers(t *testing.T) {
	log.InitLogger("info")
	chain, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewEVMService failed: %v", err)
	}

	_, _, err = chain.SubmitEVMGenesis()
	if err != nil {
		t.Fatalf("SubmitEVMGenesis failed: %v", err)
	}
	/*
		balance, err := chain.stateDB.GetBalance(chain.serviceID, evmtypes.IssuerAddress)
		if err != nil {
			t.Fatalf("GetBalance failed: %v", err)
		}

		nonce, err := chain.stateDB.GetTransactionCount(chain.serviceID, evmtypes.IssuerAddress)
		if err != nil {
			t.Fatalf("GetTransactionCount failed: %v", err)
		}

		log.Info(log.Node, "Issuer account after transfers", "address", evmtypes.IssuerAddress, "balance", balance, "nonce", nonce)

		err = chain.checkBalanceOf(evmtypes.IssuerAddress, balance)
		if err != nil {
			log.Warn(log.Node, "checkBalanceOf warning (non-fatal)", "err", err)
		}
		err = chain.checkNonces(evmtypes.IssuerAddress, new(big.Int).SetUint64(nonce))
		if err != nil {
			log.Warn(log.Node, "checkNonces warning (non-fatal)", "err", err)
		}
		gasUsed, err := chain.stateDB.EstimateGasTransfer(chain.serviceID, evmtypes.IssuerAddress, evmtypes.UsdmAddress, chain.pvmBackend)
		if err != nil {
			t.Fatalf("EstimateGasTransfer failed: %v", err)
		}
		log.Info(log.Node, "EstimateGasTransfer", "estimatedGas", gasUsed)
	*/
	prevBlockNumber := uint32(0)
	for round := 0; round < 5; round++ {
		isLastRound := (round == numRounds-1)
		log.Info(log.Node, "test_transfers - round", "round", round, "isLastRound", isLastRound)
		newBlockNumber, err := RunTransfersRound(chain, prevBlockNumber, chain.createTransferTriplesForRound(round, txnsPerRound, isLastRound))
		if err != nil {
			panic(fmt.Errorf("transfer round %d failed: %w", round, err))
		}
		prevBlockNumber = newBlockNumber
	}
}

func TestEVMBlocksDeployContract(t *testing.T) {
	log.InitLogger("info")
	b, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewEVMService failed: %v", err)
	}

	_, _, err = b.SubmitEVMGenesis()
	if err != nil {
		t.Fatalf("SubmitEVMGenesis failed: %v", err)
	}

	contractFile := "../services/evm/contracts/usdm.bin"
	block, contractAddress, err := b.DeployContract(contractFile)
	if err != nil {
		t.Fatalf("DeployContract failed: %v", err)
	}

	if err := b.ShowTxReceipts(block, block.TxHashes, "Contract Deployment", make(map[common.Hash]string)); err != nil {
		t.Fatalf("ShowTxReceipts failed: %v", err)
	}

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
