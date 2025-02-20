package trie

import (
	"bytes"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type MMR struct {
	Peaks types.Peaks `json:"peaks"`
}

func NewMMR() *MMR {
	return &MMR{
		Peaks: make([]*common.Hash, 0),
	}
}

func hashConcat(left, right *common.Hash) *common.Hash {
	if left == nil && right == nil {
		return nil
	}
	if left == nil {
		left = &common.Hash{}
	}
	if right == nil {
		right = &common.Hash{}
	}
	combined := append(left.Bytes(), right.Bytes()...)
	r := common.Keccak256(combined)
	return &r
}

// Append function for the MMR (as described in the image)
func (m *MMR) Append(data *common.Hash) {
	m.Peaks = appendToMMR(m.Peaks, data)
}

func (m *MMR) String() string {
	out := ""
	q := ""
	for i, peak := range m.Peaks {
		if peak == nil {
			out += fmt.Sprintf("%s%d: nil", q, i)
		} else {
			out += fmt.Sprintf("%s%d: %v", q, i, *peak)
		}
		q = ", "
	}
	return out
}

func (m *MMR) ComparePeaks(compare MMR) bool {
	for i, peak := range m.Peaks {
		cpeak := compare.Peaks[i]
		if peak == nil {
			if cpeak != nil {
				return false
			}
		} else {
			if bytes.Compare(cpeak.Bytes(), peak.Bytes()) != 0 {
				return false
			}
		}
	}
	return true
}

func appendToMMR(Peaks []*common.Hash, l *common.Hash) []*common.Hash {
	return P(Peaks, l, 0)
}

// Recursive function P, combining roots
func P(r []*common.Hash, l *common.Hash, n int) []*common.Hash {
	if n >= len(r) {
		return append(r, l)
	}
	if n < len(r) && r[n] == nil {
		return R(r, n, l)
	}
	return P(R(r, n, nil), hashConcat(r[n], l), n+1)
}

// Function R for updating Peaks
func R(r []*common.Hash, i int, t *common.Hash) []*common.Hash {
	s := make([]*common.Hash, len(r))
	for j := 0; j < len(r); j++ {
		if i == j {
			if t == nil {
				s[j] = nil
			} else {
				s[j] = t
			}
		} else {
			s[j] = r[j]
		}
	}
	return s
}

func (M MMR) SuperPeak() *common.Hash {
	// Helper function to compute SuperPeak recursively
	var computeSuperPeak func(hashes []*common.Hash) *common.Hash
	computeSuperPeak = func(hashes []*common.Hash) *common.Hash {
		// Remove nil values from hashes
		nonNilHashes := make([]*common.Hash, 0)
		for _, h := range hashes {
			if h != nil {
				nonNilHashes = append(nonNilHashes, h)
			}
		}

		// Base cases
		if len(nonNilHashes) == 0 {
			// Return empty hash (e.g., zero hash)
			zeroHash := common.Keccak256([]byte{})
			return &zeroHash
		}
		if len(nonNilHashes) == 1 {
			// Return the single hash
			return nonNilHashes[0]
		}

		// Recursive computation
		left := computeSuperPeak(nonNilHashes[:len(nonNilHashes)-1])
		right := nonNilHashes[len(nonNilHashes)-1]
		combined := append([]byte("peak"), append(left.Bytes(), right.Bytes()...)...)
		superPeakHash := common.Keccak256(combined)
		return &superPeakHash
	}

	// Start computation on the current MMR peaks
	return computeSuperPeak(M.Peaks)
}
