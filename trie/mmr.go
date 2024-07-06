package trie

import (
	"github.com/ethereum/go-ethereum/common"
)

// MerkleMountainRange represents the MMR structure
type MerkleMountainRange struct {
	peaks [][]byte
}

// NewMMR creates a new Merkle Mountain Range
func NewMMR() *MerkleMountainRange {
	return &MerkleMountainRange{
		peaks: make([][]byte, 0),
	}
}

// Append adds a new leaf to the MMR
func (mmr *MerkleMountainRange) Append(leaf []byte) {
	mmr.peaks = append(mmr.peaks, leaf)
	mmr.rebalance()
}

// rebalance maintains the MMR properties
func (mmr *MerkleMountainRange) rebalance() {
	for len(mmr.peaks) > 1 {
		n := len(mmr.peaks)
		if n < 2 || (n > 2 && mmr.peaks[n-2] == nil) {
			break
		}
		combined := append(mmr.peaks[n-2], mmr.peaks[n-1]...)
		mmr.peaks = mmr.peaks[:n-2]
		mmr.peaks = append(mmr.peaks, computeHash(combined))
	}
}

// Root returns the root hash of the MMR
func (mmr *MerkleMountainRange) Root() common.Hash {
	if len(mmr.peaks) == 0 {
		return common.BytesToHash([]byte{})
	}
	combined := mmr.peaks[0]
	for i := 1; i < len(mmr.peaks); i++ {
		combined = append(combined, mmr.peaks[i]...)
		combined = computeHash(combined)
	}
	return common.BytesToHash(combined)
}
