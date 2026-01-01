// Package verkle provides EVM Verkle state management for builders
package verkle

import (
	"math/big"

	"github.com/colorfulnotion/jam/common"
	"github.com/ethereum/go-verkle"
)

// VerkleStateStorage manages EVM Verkle state for builders
type VerkleStateStorage interface {
	// State access
	GetBalance(tree verkle.VerkleNode, addr common.Address) (common.Hash, error)
	GetNonce(tree verkle.VerkleNode, addr common.Address) (uint64, error)
	GetCode(tree verkle.VerkleNode, addr common.Address) ([]byte, error)
	GetStorageAt(tree verkle.VerkleNode, addr common.Address, key common.Hash) (common.Hash, error)

	// State modification
	SetBalance(tree verkle.VerkleNode, addr common.Address, balance *big.Int) error
	SetNonce(tree verkle.VerkleNode, addr common.Address, nonce uint64) error
	SetCode(tree verkle.VerkleNode, addr common.Address, code []byte) error
	SetStorageAt(tree verkle.VerkleNode, addr common.Address, key, value common.Hash) error

	// Witness generation
	GenerateWitness(tree verkle.VerkleNode, keys [][]byte) ([]byte, error)

	// Tree operations
	CreateTree() (verkle.VerkleNode, error)
	UpdateTree(tree verkle.VerkleNode, updates map[string][]byte) (verkle.VerkleNode, error)
	GetRoot(tree verkle.VerkleNode) common.Hash
}

// VerkleBuilder provides builder-specific Verkle operations
type VerkleBuilder interface {
	VerkleStateStorage

	// Builder-specific operations
	PrepareBatchUpdate(tree verkle.VerkleNode) error
	CommitBatch(tree verkle.VerkleNode) (common.Hash, error)
	GenerateProofForKeys(tree verkle.VerkleNode, keys [][]byte) (verkle.VerkleProof, error)
}