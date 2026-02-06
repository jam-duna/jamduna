package bmt

import (
	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/overlay"
	"github.com/jam-duna/jamduna/bmt/seglog"
)

// FinishedSession represents a committed transaction.
// It can generate witnesses (Merkle proofs) for the committed changes.
type FinishedSession struct {
	prevRoot   [32]byte
	root       [32]byte
	overlay    *overlay.Overlay
	tree       *beatreeWrapper
	rollbackId seglog.RecordId
}

// PrevRoot returns the root hash before this transaction.
func (fs *FinishedSession) PrevRoot() [32]byte {
	return fs.prevRoot
}

// Root returns the root hash after this transaction.
func (fs *FinishedSession) Root() [32]byte {
	return fs.root
}

// RollbackId returns the rollback record ID for this transaction.
func (fs *FinishedSession) RollbackId() seglog.RecordId {
	return fs.rollbackId
}

// Witness generates a Merkle proof for the keys modified in this transaction.
// The witness can be used to verify the state transition.
type Witness struct {
	PrevRoot [32]byte
	Root     [32]byte
	Keys     [][32]byte
	Proofs   []MerkleProof
}

// MerkleProof represents a Merkle proof for a key-value pair.
type MerkleProof struct {
	Key   [32]byte
	Value []byte // nil if deleted
	Path  []ProofNode
}

// ProofNode represents a node in a Merkle proof path.
type ProofNode struct {
	Hash  [32]byte
	Left  bool // true if this is a left sibling
}

// GenerateWitness generates a witness for this transaction.
// NOTE: This is the legacy session API. For actual Merkle proof generation,
// use the enhanced sessions API in sessions_enhanced.go
func (fs *FinishedSession) GenerateWitness() (*Witness, error) {
	// Collect all keys modified in this transaction
	values := fs.overlay.Data().GetAllValues()
	keys := make([][32]byte, 0, len(values))

	for key := range values {
		var k [32]byte
		copy(k[:], key[:])
		keys = append(keys, k)
	}

	// Placeholder proofs for legacy API
	proofs := make([]MerkleProof, len(keys))
	for i, key := range keys {
		k := beatree.KeyFromBytes(key[:])
		value, _ := fs.tree.Get(k)

		proofs[i] = MerkleProof{
			Key:   key,
			Value: value,
			Path:  nil, // Placeholder - use enhanced sessions API for actual proofs
		}
	}

	return &Witness{
		PrevRoot: fs.prevRoot,
		Root:     fs.root,
		Keys:     keys,
		Proofs:   proofs,
	}, nil
}
