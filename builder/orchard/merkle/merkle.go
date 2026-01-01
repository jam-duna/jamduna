// Package merkle provides Orchard Merkle tree implementation
package merkle

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	// MerkleTreeDepth is the depth of the Orchard commitment tree
	MerkleTreeDepth = 32

	// MaxTreeSize is the maximum number of leaves (2^32)
	MaxTreeSize = 1 << MerkleTreeDepth
)

// MerkleTree implements the Orchard commitment Merkle tree
type MerkleTree struct {
	mu sync.RWMutex

	// Tree state
	root   common.Hash   // Current Merkle root
	size   uint64        // Number of leaves
	leaves []common.Hash // Leaf storage

	// Cached internal nodes for efficiency
	// nodes[level][index] = hash
	nodes map[uint8]map[uint64]common.Hash

	// Zero hashes for empty subtrees at each level
	zeroHashes []common.Hash
}

// NewMerkleTree creates a new Orchard Merkle tree
func NewMerkleTree() *MerkleTree {
	tree := &MerkleTree{
		leaves:     make([]common.Hash, 0),
		nodes:      make(map[uint8]map[uint64]common.Hash),
		zeroHashes: make([]common.Hash, MerkleTreeDepth+1),
	}

	// Precompute zero hashes for empty subtrees
	// zeroHashes[0] = hash of empty leaf (all zeros)
	// zeroHashes[i] = hash(zeroHashes[i-1], zeroHashes[i-1])
	tree.zeroHashes[0] = common.Hash{} // Empty leaf
	for i := 1; i <= MerkleTreeDepth; i++ {
		tree.zeroHashes[i] = hashPair(tree.zeroHashes[i-1], tree.zeroHashes[i-1])
	}

	tree.root = tree.zeroHashes[MerkleTreeDepth]
	return tree
}

// GetRoot returns the current Merkle root
func (t *MerkleTree) GetRoot() common.Hash {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.root
}

// GetSize returns the number of leaves in the tree
func (t *MerkleTree) GetSize() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.size
}

// Append adds a single commitment to the tree
func (t *MerkleTree) Append(commitment common.Hash) error {
	return t.AppendBatch([]common.Hash{commitment})
}

// AppendBatch adds multiple commitments to the tree
func (t *MerkleTree) AppendBatch(commitments []common.Hash) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.size+uint64(len(commitments)) > MaxTreeSize {
		return fmt.Errorf("tree capacity exceeded: current=%d, adding=%d, max=%d",
			t.size, len(commitments), MaxTreeSize)
	}

	for _, commitment := range commitments {
		t.leaves = append(t.leaves, commitment)
		t.updatePath(t.size, commitment)
		t.size++
	}

	return nil
}

// updatePath updates internal nodes from leaf to root
func (t *MerkleTree) updatePath(index uint64, leaf common.Hash) {
	currentHash := leaf
	currentIndex := index

	for level := uint8(0); level < MerkleTreeDepth; level++ {
		// Initialize level map if needed
		if t.nodes[level] == nil {
			t.nodes[level] = make(map[uint64]common.Hash)
		}

		// Store current node
		t.nodes[level][currentIndex] = currentHash

		// Get sibling
		var sibling common.Hash
		if currentIndex%2 == 0 {
			// Left child - check if right sibling exists
			siblingIndex := currentIndex + 1
			if siblingHash, exists := t.nodes[level][siblingIndex]; exists {
				sibling = siblingHash
			} else {
				sibling = t.zeroHashes[level]
			}
			currentHash = hashPair(currentHash, sibling)
		} else {
			// Right child - left sibling must exist
			siblingIndex := currentIndex - 1
			sibling = t.nodes[level][siblingIndex]
			currentHash = hashPair(sibling, currentHash)
		}

		// Move to parent
		currentIndex = currentIndex / 2
	}

	t.root = currentHash
}

// GetLeaf returns the leaf at the given position
func (t *MerkleTree) GetLeaf(position uint64) (common.Hash, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if position >= t.size {
		return common.Hash{}, fmt.Errorf("leaf position %d out of bounds (size=%d)", position, t.size)
	}

	return t.leaves[position], nil
}

// GetSubtreeRoot returns the root of a subtree at the given depth and index
func (t *MerkleTree) GetSubtreeRoot(depth uint8, index uint64) (common.Hash, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if depth > MerkleTreeDepth {
		return common.Hash{}, fmt.Errorf("depth %d exceeds tree depth %d", depth, MerkleTreeDepth)
	}

	level := MerkleTreeDepth - depth
	if levelNodes, exists := t.nodes[level]; exists {
		if hash, exists := levelNodes[index]; exists {
			return hash, nil
		}
	}

	// Return zero hash if subtree is empty
	return t.zeroHashes[depth], nil
}

// GenerateWitness generates a Merkle inclusion proof for the given position
func (t *MerkleTree) GenerateWitness(position uint64) (MerkleWitness, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if position >= t.size {
		return MerkleWitness{}, fmt.Errorf("position %d out of bounds (size=%d)", position, t.size)
	}

	witness := MerkleWitness{
		Position: position,
		Path:     make([]common.Hash, MerkleTreeDepth),
	}

	currentIndex := position
	for level := uint8(0); level < MerkleTreeDepth; level++ {
		// Get sibling
		var sibling common.Hash
		if currentIndex%2 == 0 {
			// Left child - get right sibling
			siblingIndex := currentIndex + 1
			if levelNodes, exists := t.nodes[level]; exists {
				if hash, exists := levelNodes[siblingIndex]; exists {
					sibling = hash
				} else {
					sibling = t.zeroHashes[level]
				}
			} else {
				sibling = t.zeroHashes[level]
			}
		} else {
			// Right child - get left sibling
			siblingIndex := currentIndex - 1
			if levelNodes, exists := t.nodes[level]; exists {
				sibling = levelNodes[siblingIndex]
			} else {
				return MerkleWitness{}, fmt.Errorf("missing sibling at level %d index %d", level, siblingIndex)
			}
		}

		witness.Path[level] = sibling
		currentIndex = currentIndex / 2
	}

	return witness, nil
}

// VerifyWitness verifies a Merkle inclusion proof
func (t *MerkleTree) VerifyWitness(witness MerkleWitness, leaf common.Hash, root common.Hash) bool {
	if len(witness.Path) != MerkleTreeDepth {
		return false
	}

	currentHash := leaf
	currentIndex := witness.Position

	for level := 0; level < MerkleTreeDepth; level++ {
		sibling := witness.Path[level]

		if currentIndex%2 == 0 {
			// Left child
			currentHash = hashPair(currentHash, sibling)
		} else {
			// Right child
			currentHash = hashPair(sibling, currentHash)
		}

		currentIndex = currentIndex / 2
	}

	return currentHash == root
}

// InitializeTree initializes the tree with initial commitments
func (t *MerkleTree) InitializeTree(initialCommitments []common.Hash) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.size > 0 {
		return fmt.Errorf("tree already initialized with %d leaves", t.size)
	}

	return t.AppendBatch(initialCommitments)
}

// CreateSnapshot creates a snapshot of the current tree state
func (t *MerkleTree) CreateSnapshot() (TreeSnapshot, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return TreeSnapshot{
		Root:        t.root,
		Size:        t.size,
		BlockHeight: 0, // To be set by caller
		Timestamp:   0, // To be set by caller
	}, nil
}

// RestoreFromSnapshot restores the tree from a snapshot
func (t *MerkleTree) RestoreFromSnapshot(snapshot TreeSnapshot) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// This is a simplified restoration - in production, you'd need to
	// restore the full tree state from persistent storage
	if snapshot.Size > uint64(len(t.leaves)) {
		return fmt.Errorf("snapshot size %d exceeds current tree size %d", snapshot.Size, len(t.leaves))
	}

	t.root = snapshot.Root
	t.size = snapshot.Size

	return nil
}

// GetTreeDelta returns commitments added between two roots
func (t *MerkleTree) GetTreeDelta(fromRoot, toRoot common.Hash) ([]common.Hash, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// This is a placeholder implementation
	// In production, you'd need to track root history and compute deltas
	return nil, fmt.Errorf("GetTreeDelta not yet implemented")
}

// hashPair computes the hash of two nodes
func hashPair(left, right common.Hash) common.Hash {
	// Use Poseidon hash for Orchard (in production, use proper Poseidon implementation)
	// For now, use Keccak256 as placeholder
	data := make([]byte, 64)
	copy(data[0:32], left[:])
	copy(data[32:64], right[:])
	return crypto.Keccak256Hash(data)
}

// SerializeWitness serializes a Merkle witness for transmission
func SerializeWitness(witness MerkleWitness) []byte {
	// Format: [position:8][path_len:2][path_hashes:32*len]
	buf := make([]byte, 10+len(witness.Path)*32)
	binary.BigEndian.PutUint64(buf[0:8], witness.Position)
	binary.BigEndian.PutUint16(buf[8:10], uint16(len(witness.Path)))

	offset := 10
	for _, hash := range witness.Path {
		copy(buf[offset:offset+32], hash[:])
		offset += 32
	}

	return buf
}

// DeserializeWitness deserializes a Merkle witness
func DeserializeWitness(data []byte) (MerkleWitness, error) {
	if len(data) < 10 {
		return MerkleWitness{}, fmt.Errorf("witness data too short: %d bytes", len(data))
	}

	position := binary.BigEndian.Uint64(data[0:8])
	pathLen := binary.BigEndian.Uint16(data[8:10])

	if len(data) != 10+int(pathLen)*32 {
		return MerkleWitness{}, fmt.Errorf("witness data length mismatch: expected %d, got %d",
			10+int(pathLen)*32, len(data))
	}

	witness := MerkleWitness{
		Position: position,
		Path:     make([]common.Hash, pathLen),
	}

	offset := 10
	for i := 0; i < int(pathLen); i++ {
		copy(witness.Path[i][:], data[offset:offset+32])
		offset += 32
	}

	return witness, nil
}
