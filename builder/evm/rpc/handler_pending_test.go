package rpc

import (
	"math/big"
	"testing"

	"github.com/colorfulnotion/jam/common"
	evmtypes "github.com/colorfulnotion/jam/statedb/evmtypes"
)

func TestPendingTransactionResponse(t *testing.T) {
	to := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	tx := &evmtypes.EthereumTransaction{
		Hash:     common.HexToHash("0x01"),
		From:     common.HexToAddress("0x00000000000000000000000000000000000000bb"),
		To:       &to,
		Value:    big.NewInt(42),
		Gas:      21000,
		GasPrice: big.NewInt(1),
		Nonce:    7,
		Data:     []byte{0x01, 0x02},
		V:        big.NewInt(27),
		R:        big.NewInt(1),
		S:        big.NewInt(2),
	}

	resp := pendingTransactionResponse(tx)
	if resp["blockHash"] != nil {
		t.Fatalf("expected nil blockHash, got %v", resp["blockHash"])
	}
	if resp["blockNumber"] != nil {
		t.Fatalf("expected nil blockNumber, got %v", resp["blockNumber"])
	}
	if resp["transactionIndex"] != nil {
		t.Fatalf("expected nil transactionIndex, got %v", resp["transactionIndex"])
	}

	if resp["hash"] != tx.Hash.String() {
		t.Fatalf("expected hash %s, got %v", tx.Hash.String(), resp["hash"])
	}
	if resp["from"] != tx.From.String() {
		t.Fatalf("expected from %s, got %v", tx.From.String(), resp["from"])
	}
}
