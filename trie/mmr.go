package trie

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
)

type MMR struct {
	peaks [][]byte
}

func keccak256(data []byte) []byte {
	hash := crypto.Keccak256(data)
	return hash
}

func hashConcat(left, right []byte) []byte {
	combined := append(left, right...)
	return keccak256(combined)
}

// Append function for the MMR (as described in the image)
func (m *MMR) Append(data []byte) {
	m.peaks = appendToMMR(m.peaks, data)
}

func (m *MMR) Print() {
	for i, peak := range m.peaks {
		if peak == nil {
			fmt.Printf("Peak %d: nil\n", i)
		} else {
			fmt.Printf("Peak %d: %s\n", i, hex.EncodeToString(peak))
		}
	}
}

func (m *MMR) ComparePeaks(compare MMR) bool {
	for i, peak := range m.peaks {
		cpeak := compare.peaks[i]
		if peak == nil {
			if cpeak != nil {
				return false
			}
		} else {
			if bytes.Compare(cpeak, peak) != 0 {
				return false
			}
		}
	}
	return true
}

func appendToMMR(peaks [][]byte, l []byte) [][]byte {
	return P(peaks, l, 0)
}

// Recursive function P, combining roots
func P(r [][]byte, l []byte, n int) [][]byte {
	if n >= len(r) {
		return append(r, l)
	}
	if n < len(r) && r[n] == nil {
		return R(r, n, l)
	}
	return P(R(r, n, nil), hashConcat(r[n], l), n+1)
}

// Function R for updating peaks
func R(r [][]byte, i int, t []byte) [][]byte {
	s := make([][]byte, len(r))
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
