package trie

import (
	"math/rand"
	"testing"

	"github.com/colorfulnotion/jam/common"
)

// Recent History Test
func TestMMRAppend(t *testing.T) {
	mmr := MMR{}
	a1 := common.HexToHash("0x8720b97ddd6acc0f6eb66e095524038675a4e4067adc10ec39939eaefc47d842")
	a2 := common.HexToHash("0x7076c31882a5953e097aef8378969945e72807c4705e53a0c5aacc9176f0d56b")
	a3 := common.HexToHash("0x658b919f734bd39262c10589aa1afc657471d902a6a361c044f78de17d660bc6")
	a4 := common.HexToHash("0xa983417440b618f29ed0b7fa65212fce2d363cb2b2c18871a05c4f67217290b0")
	a5 := common.HexToHash("0x658b919f734bd39262c10589aa1afc657471d902a6a361c044f78de17d660bc6")

	expected := []MMR{
		{
			Peaks: []*common.Hash{
				&(a1),
			},
		},
		{
			Peaks: []*common.Hash{
				nil,
				&(a2),
			},
		},
		{
			Peaks: []*common.Hash{
				nil,
				nil,
				nil,
				&(a3),
			},
		},
		{
			Peaks: []*common.Hash{
				&(a4),
				nil,
				nil,
				&(a5),
			},
		},
	}

	mmr = MMR{}
	t3a := common.HexToHash("0xf986bfeff7411437ca6a23163a96b5582e6739f261e697dc6f3c05a1ada1ed0c")
	t3b := common.HexToHash("0xca29f72b6d40cfdb5814569cf906b3d369ae5f56b63d06f2b6bb47be191182a6")
	t3c := common.HexToHash("0xe17766e385ad36f22ff2357053ab8af6a6335331b90de2aa9c12ec9f397fa414")
	mmr.Peaks = []*common.Hash{
		&(t3a),
		&(t3b),
		&(t3c),
	}
	t3r := common.HexToHash("0x8223d5eaa57ccef85993b7180a593577fd38a65fb41e4bcea2933d8b202905f0")
	mmr.Append(&t3r)
	if mmr.ComparePeaks(expected[2]) == false {
		t.Fatalf("Test3 FAIL")
	}
}

// TestMMRProof tests MMR proof generation and verification
// Each loop appends 100 new leaves, then verifies a randomly chosen leaf among the most recent 500 appends.
func TestMMRProof(t *testing.T) {
	const (
		batchSize    = 100 // number of new leaves per iteration
		totalBatches = 50  // total iterations (overall leaves = 5000)
		windowSize   = 500 // restrict proof checks to latest window
	)

	mmr := NewMMR()
	leaves := make([]common.Hash, 0, batchSize*totalBatches)
	rng := rand.New(rand.NewSource(1))

	for batch := 0; batch < totalBatches; batch++ {
		// Append a new batch of leaves
		for i := 0; i < batchSize; i++ {
			globalIndex := batch*batchSize + i
			hash := common.BytesToHash([]byte{
				byte(globalIndex >> 16),
				byte(globalIndex >> 8),
				byte(globalIndex),
				byte(batch),
			})
			leaves = append(leaves, hash)
			mmr.Append(&hash)
		}

		totalLeaves := len(leaves)
		if totalLeaves == 0 {
			continue
		}

		start := totalLeaves - windowSize
		if start < 0 {
			start = 0
		}
		position := uint64(start + rng.Intn(totalLeaves-start))

		store := LeafStore{Leaves: leaves}
		proof, err := mmr.GenerateProof(position, leaves[position], store, mmr.LeafCount())
		if err != nil {
			t.Fatalf("GenerateProof failed: %v", err)
		}
		superPeak := mmr.SuperPeak()
		if superPeak == nil {
			t.Fatalf("Super peak is nil after %d leaves", totalLeaves)
		}

		if !proof.Verify(*superPeak) {
			t.Fatalf("Proof verification failed after %d leaves at position %d", totalLeaves, position)
		}

		if proof.LeafHash != leaves[position] {
			t.Fatalf("Proof leaf hash mismatch at position %d", position)
		}
	}
}
