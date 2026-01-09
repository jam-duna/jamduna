package rpc

import (
	"strings"
	"testing"

	orchardwitness "github.com/colorfulnotion/jam/builder/orchard/witness"
)

type fakeCommitmentDecoder struct {
	commitments [][32]byte
}

func (f fakeCommitmentDecoder) DecodeBundle(_ []byte) (*orchardwitness.DecodedBundle, error) {
	return &orchardwitness.DecodedBundle{
		Commitments: f.commitments,
	}, nil
}

func (f fakeCommitmentDecoder) DecodeBundleV6(_ []byte, _ []byte) (*orchardwitness.DecodedBundleV6, error) {
	return &orchardwitness.DecodedBundleV6{
		Commitments: f.commitments,
	}, nil
}

func TestProcessBundleAsyncRejectsCommitmentMismatch(t *testing.T) {
	expected := [][32]byte{{1}}
	actual := [][32]byte{{2}}

	pool := &OrchardTxPool{
		rollup:             &OrchardRollup{},
		commitmentDecoder:  fakeCommitmentDecoder{commitments: expected},
	}

	bundle := &ParsedBundle{
		ID:          "bundle-mismatch",
		RawData:     []byte{0x01},
		BundleType:  OrchardBundleZSA,
		Commitments: actual,
	}

	err := pool.processBundleAsync(bundle)
	if err == nil || !strings.Contains(err.Error(), "commitment mismatch") {
		t.Fatalf("expected commitment mismatch error, got %v", err)
	}
}
