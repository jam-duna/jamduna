package rpc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
)

// Outpoint identifies a specific UTXO (txid + output index)
type Outpoint struct {
	Txid [32]byte // Internal byte order (big-endian)
	Vout uint32
}

// UtxoData represents an unspent transaction output
type UtxoData struct {
	Value        uint64 // Zatoshis
	ScriptPubKey []byte
	Height       uint32 // Block height created (for coinbase maturity)
	IsCoinbase   bool
}

// UTXOMerkleProof proves a UTXO exists in the tree
type UTXOMerkleProof struct {
	Outpoint     Outpoint
	Value        uint64
	ScriptPubKey []byte
	Height       uint32
	IsCoinbase   bool
	TreePosition uint64
	Siblings     [][32]byte // Merkle path from leaf to root
}

// UtxoSnapshotEntry captures a full UTXO entry for snapshot serialization.
type UtxoSnapshotEntry struct {
	Outpoint     Outpoint
	Value        uint64
	ScriptPubKey []byte
	Height       uint32
	IsCoinbase   bool
}

// TransparentUtxoTree is a sorted Merkle tree of UTXOs
type TransparentUtxoTree struct {
	mu    sync.RWMutex
	utxos map[Outpoint]UtxoData
	root  [32]byte
	dirty bool
}

// NewTransparentUtxoTree creates a new UTXO tree
func NewTransparentUtxoTree() *TransparentUtxoTree {
	return &TransparentUtxoTree{
		utxos: make(map[Outpoint]UtxoData),
		dirty: true,
	}
}

// Insert adds a UTXO to the tree
func (t *TransparentUtxoTree) Insert(outpoint Outpoint, utxo UtxoData) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.utxos[outpoint] = utxo
	t.dirty = true
}

// Remove deletes a UTXO from the tree
func (t *TransparentUtxoTree) Remove(outpoint Outpoint) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.utxos[outpoint]; !exists {
		return fmt.Errorf("utxo not found: %x:%d", outpoint.Txid, outpoint.Vout)
	}

	delete(t.utxos, outpoint)
	t.dirty = true
	return nil
}

// GetRoot returns the Merkle root of all UTXOs
func (t *TransparentUtxoTree) GetRoot() ([32]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.dirty {
		return t.root, nil
	}

	if len(t.utxos) == 0 {
		t.root = [32]byte{}
		t.dirty = false
		return t.root, nil
	}

	// Get sorted outpoints
	outpoints := t.getSortedOutpoints()

	// Compute leaf hashes
	leaves := make([][32]byte, len(outpoints))
	for i, outpoint := range outpoints {
		utxo := t.utxos[outpoint]
		leaves[i] = hashUtxoLeaf(outpoint, utxo)
	}

	// Build Merkle tree
	t.root = buildUtxoMerkleRoot(leaves)
	t.dirty = false
	return t.root, nil
}

// GetProof generates a Merkle proof for a UTXO
func (t *TransparentUtxoTree) GetProof(outpoint Outpoint) (*UTXOMerkleProof, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	utxo, exists := t.utxos[outpoint]
	if !exists {
		return nil, fmt.Errorf("utxo not found")
	}

	// Get sorted outpoints
	outpoints := t.getSortedOutpoints()

	// Find position
	position := -1
	for i, op := range outpoints {
		if op == outpoint {
			position = i
			break
		}
	}
	if position == -1 {
		return nil, fmt.Errorf("utxo not in sorted set")
	}

	// Compute leaf hashes
	leaves := make([][32]byte, len(outpoints))
	for i, op := range outpoints {
		leaves[i] = hashUtxoLeaf(op, t.utxos[op])
	}

	// Generate proof
	siblings := computeMerklePath(leaves, position)

	return &UTXOMerkleProof{
		Outpoint:     outpoint,
		Value:        utxo.Value,
		ScriptPubKey: utxo.ScriptPubKey,
		Height:       utxo.Height,
		IsCoinbase:   utxo.IsCoinbase,
		TreePosition: uint64(position),
		Siblings:     siblings,
	}, nil
}

// VerifyProof verifies a Merkle proof against a root
func VerifyProof(proof *UTXOMerkleProof, expectedRoot [32]byte) bool {
	// Compute leaf hash
	leafHash := hashUtxoLeaf(proof.Outpoint, UtxoData{
		Value:        proof.Value,
		ScriptPubKey: proof.ScriptPubKey,
		Height:       proof.Height,
		IsCoinbase:   proof.IsCoinbase,
	})

	// Recompute root from proof
	computedRoot := recomputeRoot(leafHash, proof.TreePosition, proof.Siblings)

	return bytes.Equal(computedRoot[:], expectedRoot[:])
}

// getSortedOutpoints returns outpoints sorted lexicographically by internal txid
func (t *TransparentUtxoTree) getSortedOutpoints() []Outpoint {
	outpoints := make([]Outpoint, 0, len(t.utxos))
	for op := range t.utxos {
		outpoints = append(outpoints, op)
	}

	sort.Slice(outpoints, func(i, j int) bool {
		// Compare txid first (internal byte order)
		cmp := bytes.Compare(outpoints[i].Txid[:], outpoints[j].Txid[:])
		if cmp != 0 {
			return cmp < 0
		}
		// Then compare vout
		return outpoints[i].Vout < outpoints[j].Vout
	})

	return outpoints
}

// hashUtxoLeaf computes the hash of a UTXO leaf
// Leaf = SHA256d(outpoint || value || script_hash || height)
func hashUtxoLeaf(outpoint Outpoint, utxo UtxoData) [32]byte {
	buf := make([]byte, 0, 36+8+32+4+1)

	// Outpoint (36 bytes)
	buf = append(buf, outpoint.Txid[:]...)
	var voutBuf [4]byte
	binary.LittleEndian.PutUint32(voutBuf[:], outpoint.Vout)
	buf = append(buf, voutBuf[:]...)

	// Value (8 bytes)
	var valueBuf [8]byte
	binary.LittleEndian.PutUint64(valueBuf[:], utxo.Value)
	buf = append(buf, valueBuf[:]...)

	// ScriptPubKey hash (32 bytes)
	scriptHash := sha256.Sum256(utxo.ScriptPubKey)
	buf = append(buf, scriptHash[:]...)

	// Height (4 bytes)
	var heightBuf [4]byte
	binary.LittleEndian.PutUint32(heightBuf[:], utxo.Height)
	buf = append(buf, heightBuf[:]...)

	// Coinbase flag (1 byte)
	if utxo.IsCoinbase {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}

	// Double SHA256
	first := sha256.Sum256(buf)
	second := sha256.Sum256(first[:])
	return second
}

// buildUtxoMerkleRoot builds a Bitcoin/Zcash-style Merkle root
func buildUtxoMerkleRoot(leaves [][32]byte) [32]byte {
	if len(leaves) == 0 {
		return [32]byte{}
	}
	if len(leaves) == 1 {
		return leaves[0]
	}

	currentLevel := leaves
	for len(currentLevel) > 1 {
		nextLevel := make([][32]byte, (len(currentLevel)+1)/2)

		for i := 0; i < len(currentLevel); i += 2 {
			left := currentLevel[i]
			right := left // Duplicate if odd
			if i+1 < len(currentLevel) {
				right = currentLevel[i+1]
			}

			// Double SHA256 of concatenation
			combined := append(left[:], right[:]...)
			first := sha256.Sum256(combined)
			second := sha256.Sum256(first[:])
			nextLevel[i/2] = second
		}

		currentLevel = nextLevel
	}

	return currentLevel[0]
}

// computeMerklePath computes the Merkle path (siblings) for a leaf
func computeMerklePath(leaves [][32]byte, position int) [][32]byte {
	if len(leaves) == 0 || position < 0 || position >= len(leaves) {
		return nil
	}

	siblings := make([][32]byte, 0)
	currentLevel := leaves
	currentPos := position

	for len(currentLevel) > 1 {
		// Find sibling
		var sibling [32]byte
		if currentPos%2 == 0 {
			// Left node - sibling is right
			if currentPos+1 < len(currentLevel) {
				sibling = currentLevel[currentPos+1]
			} else {
				sibling = currentLevel[currentPos] // Duplicate
			}
		} else {
			// Right node - sibling is left
			sibling = currentLevel[currentPos-1]
		}
		siblings = append(siblings, sibling)

		// Move to next level
		nextLevel := make([][32]byte, (len(currentLevel)+1)/2)
		for i := 0; i < len(currentLevel); i += 2 {
			left := currentLevel[i]
			right := left
			if i+1 < len(currentLevel) {
				right = currentLevel[i+1]
			}
			combined := append(left[:], right[:]...)
			first := sha256.Sum256(combined)
			second := sha256.Sum256(first[:])
			nextLevel[i/2] = second
		}

		currentLevel = nextLevel
		currentPos = currentPos / 2
	}

	return siblings
}

// recomputeRoot recomputes the Merkle root from a leaf and its siblings
func recomputeRoot(leafHash [32]byte, position uint64, siblings [][32]byte) [32]byte {
	currentHash := leafHash
	currentPos := position

	for _, sibling := range siblings {
		var combined []byte
		if currentPos%2 == 0 {
			// Left node
			combined = append(currentHash[:], sibling[:]...)
		} else {
			// Right node
			combined = append(sibling[:], currentHash[:]...)
		}

		first := sha256.Sum256(combined)
		second := sha256.Sum256(first[:])
		currentHash = second
		currentPos = currentPos / 2
	}

	return currentHash
}

// Size returns the number of UTXOs in the tree
func (t *TransparentUtxoTree) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.utxos)
}

// Contains checks if a UTXO exists in the tree
func (t *TransparentUtxoTree) Contains(outpoint Outpoint) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, exists := t.utxos[outpoint]
	return exists
}

// Get retrieves a UTXO from the tree
func (t *TransparentUtxoTree) Get(outpoint Outpoint) (UtxoData, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	utxo, exists := t.utxos[outpoint]
	return utxo, exists
}

// Snapshot returns all UTXOs in deterministic sorted order.
func (t *TransparentUtxoTree) Snapshot() []UtxoSnapshotEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	outpoints := t.getSortedOutpoints()
	snapshot := make([]UtxoSnapshotEntry, 0, len(outpoints))
	for _, op := range outpoints {
		utxo := t.utxos[op]
		snapshot = append(snapshot, UtxoSnapshotEntry{
			Outpoint:     op,
			Value:        utxo.Value,
			ScriptPubKey: utxo.ScriptPubKey,
			Height:       utxo.Height,
			IsCoinbase:   utxo.IsCoinbase,
		})
	}
	return snapshot
}
