package trie

import (
	"bytes"
	"encoding/binary"
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

// NewMMRFromBytes constructs an MMR from the serialized representation
// produced by the Rust implementation (services/evm/src/mmr.rs::serialize).
// Format:
//
//	peak_count (4 bytes little-endian)
//	followed by peak_count entries of:
//	  flag (1 byte: 0 = None, 1 = Some)
//	  hash (32 bytes, little-endian order as emitted by serialize)
func NewMMRFromBytes(data []byte) (*MMR, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("mmr: data too short for peak count (len=%d)", len(data))
	}

	peakCount := int(binary.LittleEndian.Uint32(data[:4]))
	expectedLen := 4 + peakCount*33
	if len(data) != expectedLen {
		return nil, fmt.Errorf("mmr: unexpected length: got %d want %d", len(data), expectedLen)
	}

	peaks := make(types.Peaks, peakCount)
	offset := 4

	for i := 0; i < peakCount; i++ {
		flag := data[offset]
		offset++

		var hash common.Hash
		copy(hash[:], data[offset:offset+32])
		offset += 32

		if flag == 1 {
			// Store a copy to avoid referencing loop variable
			h := hash
			peaks[i] = &h
		} else {
			peaks[i] = nil
		}
	}

	return &MMR{
		Peaks: peaks,
	}, nil
}

func hashConcat(left, right *common.Hash) *common.Hash {
	switch {
	case left == nil && right == nil:
		return nil
	case left == nil:
		b := right.Bytes()
		r := common.Keccak256(b)
		return &r
	case right == nil:
		b := left.Bytes()
		r := common.Keccak256(b)
		return &r
	default:
		lb, rb := left.Bytes(), right.Bytes()
		combined := make([]byte, 0, len(lb)+len(rb))
		combined = append(combined, lb...)
		combined = append(combined, rb...)
		r := common.Keccak256(combined)
		return &r
	}
}

// bagPeaks combines two peak hashes using the "peak" domain separator
func bagPeaks(left, right common.Hash) common.Hash {
	combined := append([]byte("peak"), append(left.Bytes(), right.Bytes()...)...)
	return common.Keccak256(combined)
}

// SubtreeStore provides access to subtree root hashes without requiring ALL leaves.
type SubtreeStore interface {
	// returns the root hash of a complete binary subtree
	// that spans [start, start+size). The subtree is a PERFECT binary tree
	GetSubtreeRoot(start int, size int) (common.Hash, error)
}

// Append a leaf!  Equation(E.8) A in GP 0.6.2 --
func (m *MMR) Append(data *common.Hash) {
	var leaf common.Hash
	if data != nil {
		leaf = *data
	}

	leafCopy := leaf
	m.Peaks = appendToMMR(m.Peaks, &leafCopy)
}

// LeafCount - helper to count the total number of leaves represented by the current peaks
// ie the sum of 2^h for all non-nil peaks[h] -- this is useful so we don't have to manually track leaf counts.
func (m *MMR) LeafCount() int {
	total := 0
	for h, p := range m.Peaks {
		if p != nil {
			total += 1 << uint(h)
		}
	}
	return total
}

func (m *MMR) String() string {
	out := ""
	q := "["
	for _, peak := range m.Peaks {
		if peak == nil {
			out += fmt.Sprintf("%snil", q)
		} else {
			out += fmt.Sprintf("%s%v", q, *peak)
		}
		q = ","
	}
	out += "]"
	return out
}

func (m *MMR) ComparePeaks(compare MMR) bool {
	if len(m.Peaks) != len(compare.Peaks) {
		return false
	}
	for i, peak := range m.Peaks {
		cpeak := compare.Peaks[i]
		switch {
		case peak == nil && cpeak == nil:
			continue
		case peak == nil || cpeak == nil:
			return false
		default:
			if !bytes.Equal(cpeak.Bytes(), peak.Bytes()) {
				return false
			}
		}
	}
	return true
}

func appendToMMR(Peaks []*common.Hash, l *common.Hash) []*common.Hash {
	return P(Peaks, l, 0)
}

// Recursive function P, combining roots -- Equation(E.8) P in GP 0.6.2
func P(r []*common.Hash, l *common.Hash, n int) []*common.Hash {
	if n >= len(r) {
		return append(r, l)
	}
	if n < len(r) && r[n] == nil {
		return R(r, n, l)
	}
	oldVal := r[n]
	return P(R(r, n, nil), hashConcat(oldVal, l), n+1)
}

// Equation(E.8) R in GP 0.6.2 -- now O(1)
func R(r []*common.Hash, i int, t *common.Hash) []*common.Hash {
	if i >= len(r) { // grow
		n := make([]*common.Hash, i+1)
		copy(n, r)
		r = n
	}
	r[i] = t
	return r
}

// Equation(E.10) A in GP 0.6.2
func (M MMR) SuperPeak() *common.Hash {
	nonNilPeaks := make([]common.Hash, 0)
	for h := len(M.Peaks) - 1; h >= 0; h-- {
		if M.Peaks[h] != nil {
			nonNilPeaks = append(nonNilPeaks, *M.Peaks[h])
		}
	}

	if len(nonNilPeaks) == 0 {
		zeroHash := common.Keccak256([]byte{})
		return &zeroHash
	}
	if len(nonNilPeaks) == 1 {
		return &nonNilPeaks[0]
	}

	acc := nonNilPeaks[0]
	for i := 1; i < len(nonNilPeaks); i++ {
		acc = bagPeaks(acc, nonNilPeaks[i])
	}
	return &acc
}

// MMRProof contains sibling hashes and peaks for MMR inclusion verification
type MMRProof struct {
	Position    uint64        // Position in MMR (leaf index)
	LeafHash    common.Hash   // The receipt_hash being proven
	Siblings    []common.Hash // Sibling hashes encountered while climbing to peak
	SiblingLeft []bool        // True if sibling is on the left, false if on right
	Peaks       []common.Hash // Hashes of all peaks in left-to-right order
	PeakIndex   int           // Index of the peak containing the leaf
}

// Verify verifies an MMR inclusion proof by climbing the tree
// and checking that the computed root matches the expected root.
//
// The verification process:
// 1. Starts with the leaf hash at the given position
// 2. Combines it with sibling hashes (using keccak256) to climb to a peak
// 3. Substitutes the computed peak and bags all peaks left-to-right
//
// Returns true if the proof is valid, false otherwise.
func (proof MMRProof) Verify(expectedRoot common.Hash) bool {
	// Climb to peak by combining leaf with siblings
	cur := proof.LeafHash
	for i, sib := range proof.Siblings {
		if i < len(proof.SiblingLeft) && proof.SiblingLeft[i] {
			// Sibling is on the left
			cur = common.Keccak256(append(sib.Bytes(), cur.Bytes()...))
		} else {
			// Sibling is on the right
			cur = common.Keccak256(append(cur.Bytes(), sib.Bytes()...))
		}
	}

	// Substitute computed peak and bag all peaks (left -> right)
	if len(proof.Peaks) == 0 {
		return cur == expectedRoot
	}

	peaks := make([]common.Hash, len(proof.Peaks))
	copy(peaks, proof.Peaks)
	if proof.PeakIndex >= 0 && proof.PeakIndex < len(peaks) {
		peaks[proof.PeakIndex] = cur
	}

	// Bag peaks left-to-right using the same domain separator as SuperPeak
	acc := peaks[0]
	for i := 1; i < len(peaks); i++ {
		acc = bagPeaks(acc, peaks[i])
	}
	return acc == expectedRoot
}

type peakSegment struct {
	start int
	size  int
}

// listPeaksAndSegments returns peaks and segments in LEFT-TO-RIGHT order.
// The leftmost peak is the largest (highest height).
func (m *MMR) listPeaksAndSegments(totalLeaves int) ([]common.Hash, []peakSegment) {
	peaks := make([]common.Hash, 0)
	segs := make([]peakSegment, 0)

	// Walk heights from high -> low so we emit peaks from left -> right.
	// (leftmost peak has the largest height)
	currentStart := 0
	for h := len(m.Peaks) - 1; h >= 0; h-- {
		peak := m.Peaks[h]
		if peak == nil {
			continue
		}
		size := 1 << uint(h)
		// This peak covers [currentStart, currentStart+size)
		segs = append(segs, peakSegment{start: currentStart, size: size})
		peaks = append(peaks, *peak)
		currentStart += size
	}
	return peaks, segs
}

// GenerateProof generates an MMR inclusion proof for a leaf at given position
// It does not need all the leaves -- it uses a SubtreeStore to fetch internal node hashes without requiring all leaves
//
// Inputs:
//   - position: 0-based leaf index in the global MMR ordering -- EVMBlock logStartIndex
//   - leafHash: hash of the leaf at this position -- ReceiptHash
//   - store: provides subtree root hashes -- this needs to go into our Storage
//   - totalLeaves: total number of leaves in the MMR == caller should use LeafCount
//
// Returns an MMRProof structure with all necessary data for verification.
func (m *MMR) GenerateProof(position uint64, leafHash common.Hash, store SubtreeStore, totalLeaves int) (MMRProof, error) {
	proof := MMRProof{
		Position:  position,
		LeafHash:  leafHash,
		PeakIndex: -1,
	}

	// Validate inputs
	if totalLeaves <= 0 {
		return proof, nil
	}
	if position >= uint64(totalLeaves) {
		return proof, fmt.Errorf("position %d out of bounds (totalLeaves=%d)", position, totalLeaves)
	}

	peaks, segments := m.listPeaksAndSegments(totalLeaves)

	// Sanity check: verify peaks cover exactly totalLeaves
	covered := 0
	for _, s := range segments {
		covered += s.size
	}
	if covered != totalLeaves {
		return proof, fmt.Errorf("peaks cover %d leaves, expected %d", covered, totalLeaves)
	}

	// Find which peak contains this position
	idx := -1
	for i, seg := range segments {
		if int(position) >= seg.start && int(position) < seg.start+seg.size {
			idx = i
			break
		}
	}

	proof.Peaks = peaks
	proof.PeakIndex = idx

	if idx == -1 {
		return proof, fmt.Errorf("position %d not covered by current peaks (internal error)", position)
	}

	// Climb inside the peak, collecting siblings using the store
	seg := segments[idx]
	start := seg.start
	size := seg.size
	off := int(position) - start

	siblings := make([]common.Hash, 0, 64)
	sibLeft := make([]bool, 0, 64)

	for size > 1 {
		half := size / 2
		if off < half {
			// We are in left child; sibling is the right subtree root
			sib, err := store.GetSubtreeRoot(start+half, half)
			if err != nil {
				return proof, fmt.Errorf("get subtree [%d,%d): %w", start+half, half, err)
			}
			siblings = append(siblings, sib)
			sibLeft = append(sibLeft, false) // sibling on the right
			// Move into left child
			size = half
			// start unchanged
		} else {
			// We are in right child; sibling is the left subtree root
			sib, err := store.GetSubtreeRoot(start, half)
			if err != nil {
				return proof, fmt.Errorf("get subtree [%d,%d): %w", start, half, err)
			}
			siblings = append(siblings, sib)
			sibLeft = append(sibLeft, true) // sibling on the left
			// Move into right child
			start += half
			off -= half
			size = half
		}
	}

	// Reverse so siblings are ordered from leaf-level upwards (as Verify expects)
	for i := 0; i < len(siblings)/2; i++ {
		j := len(siblings) - 1 - i
		siblings[i], siblings[j] = siblings[j], siblings[i]
		sibLeft[i], sibLeft[j] = sibLeft[j], sibLeft[i]
	}

	proof.Siblings = siblings
	proof.SiblingLeft = sibLeft
	return proof, nil
}

// LeafStore is a SubtreeStore implementation backed by an array of leaves
// TODO: make StateDBStorage implement GetSubtreeRoot vs put it into accumulate
type LeafStore struct {
	Leaves []common.Hash
}

func (s LeafStore) GetSubtreeRoot(start, size int) (common.Hash, error) {
	if start < 0 || start+size > len(s.Leaves) {
		return common.Hash{}, fmt.Errorf("subtree range [%d, %d) out of bounds (have %d leaves)", start, start+size, len(s.Leaves))
	}
	if size == 1 {
		return s.Leaves[start], nil
	}
	half := size / 2
	left, err := s.GetSubtreeRoot(start, half)
	if err != nil {
		return common.Hash{}, err
	}
	right, err := s.GetSubtreeRoot(start+half, half)
	if err != nil {
		return common.Hash{}, err
	}
	return common.Keccak256(append(left.Bytes(), right.Bytes()...)), nil
}

// LeafStoreWithOffset is a LeafStore that handles global MMR positions by translating them to local indices
type LeafStoreWithOffset struct {
	Leaves []common.Hash
	Offset uint64 // Global offset (e.g., LogIndexStart)
}

func (s LeafStoreWithOffset) GetSubtreeRoot(start, size int) (common.Hash, error) {
	// start is a global position, translate to local index
	if uint64(start) < s.Offset {
		return common.Hash{}, fmt.Errorf("start %d is before offset %d", start, s.Offset)
	}
	localStart := int(uint64(start) - s.Offset)

	if localStart < 0 || localStart+size > len(s.Leaves) {
		return common.Hash{}, fmt.Errorf("subtree range [%d, %d) out of bounds (have %d leaves, offset=%d)", start, start+size, len(s.Leaves), s.Offset)
	}
	if size == 1 {
		return s.Leaves[localStart], nil
	}
	half := size / 2
	left, err := s.GetSubtreeRoot(start, half)
	if err != nil {
		return common.Hash{}, err
	}
	right, err := s.GetSubtreeRoot(start+half, half)
	if err != nil {
		return common.Hash{}, err
	}
	return common.Keccak256(append(left.Bytes(), right.Bytes()...)), nil
}
