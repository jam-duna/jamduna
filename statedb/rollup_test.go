package statedb

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
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

	rand.Seed(12345)
	wpqs := make([]*types.WPQueueItem, types.TotalCores)
	for n := 1; n <= numBlocks; n++ {
		payload := make([]byte, 2)
		payload[0] = byte(n)
		payload[1] = algoPayloads[n-1]
		wp := types.WorkPackage{
			WorkItems: []types.WorkItem{
				{
					//Payload:            GenerateAlgoPayload(n, false),
					Payload:            payload,
					RefineGasLimit:     types.RefineGasAllocation / 2,
					AccumulateGasLimit: types.AccumulationGasAllocation / 2,
					ImportedSegments:   []types.ImportSegment{},
					ExportCount:        uint16(0),
				},
			},
		}
		wpqs[0] = &types.WPQueueItem{
			WorkPackage: wp,
			CoreIndex:   0,
		}
		// Process all cores work packages
		err := c.processWPQueueItems(wpqs)
		if err != nil {
			t.Fatal(err)
		}
		bn, err := c.stateDB.GetLatestBlockNumberForService(c.serviceID)
		if err != nil {
			t.Fatalf("GetLatestBlockNumber failed: %v", err)
		}
		// Read the previous block (bn-1) which has been finalized
		// The current block (bn) won't be finalized until the next round
		blockNumToRead := bn
		if bn > 0 {
			blockNumToRead = bn - 1
		}
		block, err := c.stateDB.GetBlockByNumber(c.serviceID, fmt.Sprintf("0x%x", blockNumToRead))
		if err != nil {
			t.Fatalf("GetBlockByNumber failed: %v", err)
		}
		log.Info(log.Node, "Algo block processed", "blockNumber", blockNumToRead, "round", n, "# txns", len(block.TxHashes), "# receipts", len(block.ReceiptHashes))
	}

}

func TestEVMBlocksMath(t *testing.T) {
	chain, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewEVMService failed: %v", err)
	}
	wpq, err := chain.SubmitEVMPayloadBlocks(0, 1)
	if err != nil {
		t.Fatalf("SubmitEVMPayloadBlocks failed: %v", err)
	}
	err = chain.processWPQueueItems([]*types.WPQueueItem{wpq})
	if err != nil {
		t.Fatalf("processWPQueueItems failed: %v", err)
	}
	txBytesMulticore := make([][][]byte, types.TotalCores)

	for i := 0; i <= 10; i++ {
		txBytes, err := chain.CallMath(MathAddress, []string{
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
		})
		if err != nil {
			t.Fatalf("CallMath failed: %v", err)
		}
		txBytesMulticore[0] = txBytes
		chain.SubmitEVMTransactions(txBytesMulticore)
	}
}

func RunTransfersRound(b *Rollup, transfers []TransferTriple) (witnesses map[common.Hash][][]byte, err error) {
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
		balanceHash, err := b.stateDB.GetBalance(accounts[i])
		if err != nil {
			log.Error(log.Node, "GetBalance ERR", "account", i, "err", err)
			return nil, err
		}
		balance := new(big.Int).SetBytes(balanceHash.Bytes())
		initialBalances[i] = balance
		totalBefore = new(big.Int).Add(totalBefore, balance)

		nonce, err := b.stateDB.GetTransactionCount(accounts[i])
		if err != nil {
			log.Error(log.Node, "GetTransactionCount ERR", "account", i, "err", err)
			return nil, err
		}
		initialNonces[i] = nonce
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

		gasPrice := big.NewInt(1_000_000_000) // 1 Gwei
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
		txHashes[idx] = txHash
	}

	// Assign transactions to core 0
	txBytesMulticore[0] = txBytes

	// Submit multitransfer as work package
	wpqs, err := b.SubmitEVMTransactions(txBytesMulticore)
	if err != nil {
		log.Error(log.Node, "SubmitEVMTransactions ERR", "err", err)
		return nil, err
	}
	b.processWPQueueItems(wpqs)
	if err := b.ShowTxReceipts(txHashes, fmt.Sprintf("Multitransfer (%d transfers)", len(transfers)), defaultTopics()); err != nil {
		return nil, fmt.Errorf("failed to show transfer receipts: %w", err)
	}

	// Check balances and nonces after transfers
	totalAfter := big.NewInt(0)
	for i := 0; i < numAccounts; i++ {
		balanceHash, err := b.stateDB.GetBalance(accounts[i])
		if err != nil {
			log.Error(log.Node, "GetBalance ERR", "account", i, "err", err)
			return nil, err
		}
		balance := new(big.Int).SetBytes(balanceHash.Bytes())
		totalAfter = new(big.Int).Add(totalAfter, balance)

		nonce, err := b.stateDB.GetTransactionCount(accounts[i])
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
		issuerBalance, err := b.n1.GetBalance(IssuerAddress, "latest")
		if err != nil {
			return nil, fmt.Errorf("GetBalance failed: %w", err)
		}

		err = b.checkBalanceOf(IssuerAddress, issuerBalance)
		if err != nil {
			return nil, fmt.Errorf("checkBalanceOf failed: %w", err)
		}

		issuerTxCount, err := b.GetTransactionCount(IssuerAddress, "latest")
		if err != nil {
			return nil, fmt.Errorf("GetTransactionCount failed: %w", err)
		}
		err = b.checkNonces(IssuerAddress, new(big.Int).SetUint64(issuerTxCount))
		if err != nil {
			return nil, fmt.Errorf("checkNonces failed: %w", err)
		}
	*/

	// Verify coinbase collected fees
	coinbaseBalance, err := b.stateDB.GetBalance(coinbaseAddress)
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

// TestEVMBlocksTransfers runs multiple rounds of transfers
func TestEVMBlocksTransfers(t *testing.T) {

	chain, err := NewRollup(t.TempDir(), EVMServiceCode)
	if err != nil {
		t.Fatalf("NewEVMService failed: %v", err)
	}

	wpq, err := chain.SubmitEVMPayloadBlocks(0, 1)
	if err != nil {
		t.Fatalf("SubmitEVMPayloadBlocks failed: %v", err)
	}
	err = chain.processWPQueueItems([]*types.WPQueueItem{wpq})
	if err != nil {
		t.Fatalf("processWPQueueItems failed: %v", err)
	}

	if err := chain.stateDB.EstimateGasTransfer(IssuerAddress, UsdmAddress, chain.pvmBackend); err != nil {
		t.Fatalf("EstimateGas failed: %v", err)
	}
	for round := 0; round < numRounds; round++ {
		isLastRound := (round == numRounds-1)
		log.Info(log.Node, "test_transfers - round", "round", round, "isLastRound", isLastRound)
		_, err := RunTransfersRound(chain, chain.createTransferTriplesForRound(round, txnsPerRound, isLastRound))
		if err != nil {
			panic(fmt.Errorf("transfer round %d failed: %w", round, err))
		}
	}
}

// if _, err := service.DeployContract("../services/evm/contracts/funds.bin"); err != nil {
// 	t.Fatalf("DeployContract failed: %v", err)
// }

/*
	if err := b.ShowTxReceipts(deployTxHashes, "Contract Deployment", make(map[common.Hash]string)); err != nil {
		return common.Address{}, fmt.Errorf("failed to show deployment receipts: %w", err)
	}

	// Verify contract deployment by checking if code exists at the calculated address
	deployedCode, err := b.GetCode(common.Address(contractAddress))
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
*/
