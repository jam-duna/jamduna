package witness

import (
	"testing"

	orchardmerkle "github.com/colorfulnotion/jam/builder/orchard/merkle"
	"github.com/ethereum/go-ethereum/common"
)

type stubWallet struct {
	nullifier [32]byte
}

func (w stubWallet) NullifierFor(_, _, _ [32]byte) ([32]byte, error) {
	return w.nullifier, nil
}

func (w stubWallet) ProofFor(ExtrinsicType, []byte) ([]byte, []byte, error) {
	return nil, nil, nil
}

type stubCommitmentProvider struct {
	commitment [32]byte
}

func (p stubCommitmentProvider) CommitmentFor(
	uint32,
	Uint128,
	[32]byte,
	[32]byte,
	[32]byte,
	uint64,
	[32]byte,
) ([32]byte, error) {
	return p.commitment, nil
}

func TestOrchardBuilderDepositUsesWalletNullifier(t *testing.T) {
	tree := orchardmerkle.NewMerkleTree()
	commitment := [32]byte{0xBB}
	expectedNullifier := [32]byte{0xAA}
	builder := NewOrchardBuilderWithWallet(
		1,
		tree,
		nil,
		stubWallet{nullifier: expectedNullifier},
		stubCommitmentProvider{commitment: commitment},
	)

	note, err := builder.Deposit(common.Address{}, OrchardNoteInput{
		AssetID:     1,
		Amount:      Uint128{Lo: 10},
		OwnerPubKey: [32]byte{0x01},
		Rho:         [32]byte{0x02},
	})
	if err != nil {
		t.Fatalf("Deposit failed: %v", err)
	}

	if note.Nullifier != common.BytesToHash(expectedNullifier[:]) {
		t.Fatalf("unexpected nullifier: got %x", note.Nullifier)
	}
}

func TestOrchardBuilderDepositRequiresWallet(t *testing.T) {
	tree := orchardmerkle.NewMerkleTree()
	builder := NewOrchardBuilderWithWallet(1, tree, nil, nil, stubCommitmentProvider{})

	_, err := builder.Deposit(common.Address{}, OrchardNoteInput{
		AssetID:     1,
		Amount:      Uint128{Lo: 10},
		OwnerPubKey: [32]byte{0x01},
		Rho:         [32]byte{0x02},
	})
	if err == nil {
		t.Fatal("expected error when wallet is not configured")
	}
}
