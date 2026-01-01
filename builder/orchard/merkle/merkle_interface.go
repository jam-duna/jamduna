// Package merkle provides Orchard Merkle tree management for builders
package merkle

import "github.com/ethereum/go-ethereum/common"

// MerkleTreeStorage manages Orchard Merkle trees for builders
type MerkleTreeStorage interface {
	// Tree operations
	GetRoot() common.Hash
	GetSize() uint64
	Append(commitment common.Hash) error
	AppendBatch(commitments []common.Hash) error

	// Witness generation
	GenerateWitness(position uint64) (MerkleWitness, error)
	VerifyWitness(witness MerkleWitness, leaf common.Hash, root common.Hash) bool

	// Tree navigation
	GetLeaf(position uint64) (common.Hash, error)
	GetSubtreeRoot(depth uint8, index uint64) (common.Hash, error)
}

// MerkleWitness represents a Merkle inclusion proof
type MerkleWitness struct {
	Position uint64        // Leaf position
	Path     []common.Hash // Sibling hashes from leaf to root
}

// OrchardMerkleBuilder provides builder-specific Merkle operations
type OrchardMerkleBuilder interface {
	MerkleTreeStorage

	// Builder-specific operations
	InitializeTree(initialCommitments []common.Hash) error
	CreateSnapshot() (TreeSnapshot, error)
	RestoreFromSnapshot(snapshot TreeSnapshot) error
	GetTreeDelta(fromRoot, toRoot common.Hash) ([]common.Hash, error)
}

// TreeSnapshot represents a point-in-time Merkle tree state
type TreeSnapshot struct {
	Root        common.Hash
	Size        uint64
	BlockHeight uint64
	Timestamp   uint64
}

// BatchMerkleUpdate represents a batch of Merkle tree updates
type BatchMerkleUpdate struct {
	Commitments []common.Hash
	StartIndex  uint64
	BatchRoot   common.Hash
}
