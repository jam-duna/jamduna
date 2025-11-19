package bmt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/core"
	"github.com/colorfulnotion/jam/bmt/overlay"
	"github.com/colorfulnotion/jam/bmt/rollback"
	"github.com/colorfulnotion/jam/trie"
)

// Nomt is a high-performance, Merkle-tree-based key-value database.
// It provides:
//   - Snapshot isolation for reads
//   - Multiple readers, single writer (MRSW) concurrency
//   - Witness generation for Merkle proofs
//   - Rollback support via delta encoding
//   - Overlay-based uncommitted changes
type Nomt struct {
	mu sync.RWMutex

	// Database path
	path string

	// Beatree for persistent storage
	tree *beatreeWrapper

	// Rollback system for undo
	rollbackSys *rollback.Rollback

	// Current root hash
	root [32]byte

	// Active live overlay (single writer)
	liveOverlay *overlay.LiveOverlay

	// Committed overlays (for snapshot reads)
	committedOverlays []*overlay.Overlay

	// Value hasher
	valueHasher ValueHasher

	// Metrics
	metrics *Metrics

	// Closed flag
	closed bool
}

// Open opens or creates a Nomt database at the given path.
func Open(opts Options) (*Nomt, error) {
	// Create directory if needed
	if err := os.MkdirAll(opts.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open beatree
	bwOpts := beatreeOptions{Path: opts.Path}
	tree, err := openBeatree(bwOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to open beatree: %w", err)
	}

	// Set up rollback directory
	rollbackDir := opts.RollbackSegLogDir
	if rollbackDir == "" {
		rollbackDir = filepath.Join(opts.Path, "rollback")
	}

	// Create rollback system
	rollbackCfg := rollback.Config{
		SegLogDir:        rollbackDir,
		SegLogMaxSize:    opts.RollbackSegLogMaxSize,
		InMemoryCapacity: opts.RollbackInMemoryCapacity,
	}

	// Create lookup and apply functions for rollback
	lookupFunc := func(key beatree.Key) ([]byte, error) {
		return tree.Get(key)
	}

	applyFunc := func(changeset map[beatree.Key]*beatree.Change) error {
		return tree.ApplyChangeset(changeset)
	}

	rollbackSys, err := rollback.NewRollback(lookupFunc, applyFunc, rollbackCfg)
	if err != nil {
		tree.Close()
		return nil, fmt.Errorf("failed to create rollback system: %w", err)
	}

	// Get current root from tree
	root := tree.Root()

	// Create initial live overlay
	liveOverlay := overlay.NewLiveOverlay(root, nil)

	// Set up value hasher
	valueHasher := opts.ValueHasher
	if valueHasher == nil {
		valueHasher = &defaultHasher{}
	}

	return &Nomt{
		path:              opts.Path,
		tree:              tree,
		rollbackSys:       rollbackSys,
		root:              root,
		liveOverlay:       liveOverlay,
		committedOverlays: nil,
		valueHasher:       valueHasher,
		metrics:           NewMetrics(),
		closed:            false,
	}, nil
}

// Insert inserts or updates a key-value pair.
func (n *Nomt) Insert(key [32]byte, value []byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("database is closed")
	}

	// Record insert in live overlay
	k := beatree.KeyFromBytes(key[:])
	change := beatree.NewInsertChange(value)

	if err := n.liveOverlay.SetValue(k, change); err != nil {
		return fmt.Errorf("failed to insert: %w", err)
	}

	n.metrics.RecordWrite()
	return nil
}

// Delete deletes a key.
func (n *Nomt) Delete(key [32]byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("database is closed")
	}

	// Record delete in live overlay
	k := beatree.KeyFromBytes(key[:])
	change := beatree.NewDeleteChange()

	if err := n.liveOverlay.SetValue(k, change); err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}

	n.metrics.RecordWrite()
	return nil
}

// Get retrieves a value by key from the current state.
func (n *Nomt) Get(key [32]byte) ([]byte, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.closed {
		return nil, fmt.Errorf("database is closed")
	}

	k := beatree.KeyFromBytes(key[:])

	// Try live overlay first
	value, err := n.liveOverlay.Lookup(k)
	if err != nil {
		return nil, err
	}
	if value != nil {
		n.metrics.RecordRead(true) // Cache hit
		return value, nil
	}

	// Fall back to tree
	value, err = n.tree.Get(k)
	if err != nil {
		return nil, err
	}

	n.metrics.RecordRead(value != nil)
	return value, nil
}

// Commit commits the current changes to persistent storage.
// Returns a FinishedSession that can generate witnesses.
func (n *Nomt) Commit() (*FinishedSession, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return nil, fmt.Errorf("database is closed")
	}

	// Freeze the live overlay
	frozenOverlay, err := n.liveOverlay.Freeze()
	if err != nil {
		return nil, fmt.Errorf("failed to freeze overlay: %w", err)
	}

	// Build changeset from overlay
	changeset := make(map[beatree.Key]*beatree.Change)
	for key, change := range frozenOverlay.Data().GetAllValues() {
		changeset[key] = change
	}

	// Commit rollback delta BEFORE applying changes
	recordId, err := n.rollbackSys.Commit(changeset)
	if err != nil {
		frozenOverlay.MarkDropped()
		return nil, fmt.Errorf("failed to commit rollback delta: %w", err)
	}

	// Apply changes to tree
	if err := n.tree.ApplyChangeset(changeset); err != nil {
		// Try to rollback
		n.rollbackSys.RollbackSingle()
		frozenOverlay.MarkDropped()
		return nil, fmt.Errorf("failed to apply changeset: %w", err)
	}

	// Sync changes to disk
	if err := n.tree.Sync(); err != nil {
		// Try to rollback
		n.rollbackSys.RollbackSingle()
		frozenOverlay.MarkDropped()
		return nil, fmt.Errorf("failed to sync changes: %w", err)
	}

	// Update root
	prevRoot := n.root
	n.root = n.tree.Root()

	// Mark overlay as committed
	frozenOverlay.MarkCommitted()

	// Add to committed overlays
	n.committedOverlays = append(n.committedOverlays, frozenOverlay)

	// Create new live overlay
	n.liveOverlay = overlay.NewLiveOverlay(n.root, frozenOverlay)

	n.metrics.RecordCommit()

	return &FinishedSession{
		prevRoot:   prevRoot,
		root:       n.root,
		overlay:    frozenOverlay,
		tree:       n.tree,
		rollbackId: recordId,
	}, nil
}

// Rollback rolls back the last commit.
func (n *Nomt) Rollback() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("database is closed")
	}

	// Drop current live overlay
	n.liveOverlay.Drop()

	// Roll back one transaction
	if err := n.rollbackSys.RollbackSingle(); err != nil {
		return fmt.Errorf("failed to rollback: %w", err)
	}

	// Sync rollback changes to disk
	if err := n.tree.Sync(); err != nil {
		return fmt.Errorf("failed to sync rollback: %w", err)
	}

	// Update root from tree
	n.root = n.tree.Root()

	// Get the parent overlay for the new live overlay
	// After rollback, we need the overlay that was before the one we just rolled back
	var parent *overlay.Overlay
	if len(n.committedOverlays) > 0 {
		// Remove the last committed overlay since we rolled back past it
		n.committedOverlays = n.committedOverlays[:len(n.committedOverlays)-1]

		// If there are still overlays, use the new last one as parent
		if len(n.committedOverlays) > 0 {
			parent = n.committedOverlays[len(n.committedOverlays)-1]
		}
	}

	// Create new live overlay
	n.liveOverlay = overlay.NewLiveOverlay(n.root, parent)

	return nil
}

// Root returns the current committed root hash.
func (n *Nomt) Root() [32]byte {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.root
}

// CurrentRoot computes the merkle root from the live overlay WITHOUT committing to disk.
// This mirrors Rust NOMT's Session::finish() + FinishedSession::root() workflow.
// Unlike Root(), this includes uncommitted changes from the live overlay.
// Works correctly even after commits by merging committed tree + overlay delta.
func (n *Nomt) CurrentRoot() ([32]byte, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// If no overlay, return committed root
	if n.liveOverlay == nil {
		return n.root, nil
	}

	// Build combined key-value pairs from committed tree + live overlay
	kvPairs, err := n.collectKVForPreview()
	if err != nil {
		return [32]byte{}, err
	}

	if len(kvPairs) == 0 {
		return [32]byte{}, nil // Empty tree
	}

	// Build GP tree from combined state (NO DISK I/O)
	hasher := core.Blake2bBinaryHasher{}
	rootNode := core.BuildGpTree(kvPairs, 0, hasher.Hash)
	return [32]byte(rootNode.Hash), nil
}

// collectKVForPreview merges committed tree state + live overlay into sorted KV pairs.
// This properly implements the merge logic that Rust's Session::finish() performs:
// 1. Start with all committed key-values as baseline
// 2. Apply overlay changes (inserts override, deletes remove)
// 3. Sort deterministically for tree building
func (n *Nomt) collectKVForPreview() ([]core.KVPair, error) {
	combined := make(map[[32]byte][]byte)

	// Step 1: Read all committed data from the tree
	// This uses beatreeWrapper.Enumerate() to iterate the persisted state
	err := n.tree.Enumerate(func(key [32]byte, value []byte) error {
		combined[key] = value
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate committed tree: %w", err)
	}

	// Step 2: Apply overlay changes (overlay overrides committed data)
	if n.liveOverlay != nil {
		overlayData := n.liveOverlay.GetAllValues()
		for key, change := range overlayData {
			var k [32]byte
			copy(k[:], key[:])

			if change.IsInsert() {
				// Insert or update - override committed value
				combined[k] = change.Value
			} else {
				// Delete - remove from combined set
				delete(combined, k)
			}
		}
	}

	// Step 3: Convert to sorted KV pairs for deterministic tree building
	kvPairs := make([]core.KVPair, 0, len(combined))
	for key, value := range combined {
		kvPairs = append(kvPairs, core.KVPair{
			Key:   core.KeyPath(key),
			Value: value,
		})
	}

	// Sort by key for deterministic tree construction
	sort.Slice(kvPairs, func(i, j int) bool {
		return bytes.Compare(kvPairs[i].Key[:], kvPairs[j].Key[:]) < 0
	})

	return kvPairs, nil
}

// Session creates a read-only snapshot session.
func (n *Nomt) Session() *Session {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return &Session{
		root: n.root,
		tree: n.tree,
	}
}

// Sync flushes pending writes to disk.
func (n *Nomt) Sync() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("database is closed")
	}

	if err := n.tree.Sync(); err != nil {
		return err
	}

	return n.rollbackSys.Sync()
}

// Close closes the database.
func (n *Nomt) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return nil
	}

	n.closed = true

	// Drop live overlay
	n.liveOverlay.Drop()

	// Close rollback system
	if err := n.rollbackSys.Close(); err != nil {
		return err
	}

	// Close tree
	return n.tree.Close()
}

// Metrics returns performance metrics.
func (n *Nomt) Metrics() *Metrics {
	return n.metrics
}

// GenerateProof generates a merkle proof for a specific key in the committed state.
// This enables read-only proof generation without creating a write session.
//
// LIMITATION: Currently only works if the overlay still contains the data.
// Once data is committed and the overlay is cleared, this will fail.
// TODO: Implement proper beatree iteration to read committed data from disk.
func (n *Nomt) GenerateProof(key [32]byte) (*MerkleProof, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Get the value for this key from the current state
	value, err := n.Get(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get key: %w", err)
	}

	// Build all KV pairs from overlay (committed data is not yet accessible via iteration)
	kvPairs := n.buildAllKVPairs()

	// If no data in overlay, we can't generate a proof yet
	if len(kvPairs) == 0 {
		return nil, fmt.Errorf("cannot generate proof: overlay is empty (need beatree iteration support)")
	}

	// Temporarily construct an in-memory MerkleTree to generate the proof
	merkleTree := trie.NewMerkleTree(kvPairs)

	// Use the MerkleTree's Trace method to generate the BMT-format proof
	path, err := merkleTree.Trace(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to trace proof: %w", err)
	}

	// Convert [][]byte to []ProofNode format
	proofNodes := make([]ProofNode, len(path))
	for i, siblingHash := range path {
		var hash [32]byte
		copy(hash[:], siblingHash)
		proofNodes[i] = ProofNode{
			Hash: hash,
			Left: false, // Will be determined during verification based on key bits
		}
	}

	return &MerkleProof{
		Key:   key,
		Value: value,
		Path:  proofNodes,
	}, nil
}

// buildAllKVPairs builds KV pairs from current state (overlay + committed data)
// Uses Enumerate() to read all committed data from disk
func (n *Nomt) buildAllKVPairs() [][2][]byte {
	combined := make(map[[32]byte][]byte)

	// First, get committed data using Enumerate (reads from disk)
	if n.tree != nil {
		_ = n.tree.Enumerate(func(key [32]byte, value []byte) error {
			combined[key] = value
			return nil
		})
	}

	// Then apply overlay changes (which override committed data)
	if n.liveOverlay != nil {
		changes := n.liveOverlay.GetAllValues()
		for key, change := range changes {
			var k [32]byte
			copy(k[:], key[:])
			if change.IsDelete() {
				delete(combined, k)
			} else {
				combined[k] = change.Value
			}
		}
	}

	// Convert to [][2][]byte format expected by MerkleTree
	kvPairs := make([][2][]byte, 0, len(combined))
	for key, value := range combined {
		kvPairs = append(kvPairs, [2][]byte{key[:], value})
	}

	return kvPairs
}

// generateBMTProofPath generates a BMT-compatible proof path for a specific key
// Returns an array of ProofNodes where:
// - path[0] = sibling at depth 0 (near root)
// - path[len-1] = sibling at depth len-1 (near leaf)
// This matches the format expected by verify_merkle_proof in Rust
func (n *Nomt) generateBMTProofPath(tree *core.GpTrieNode, key [32]byte) []ProofNode {
	pathNodes := make([]ProofNode, 0)

	// Walk the tree following the key's path, appending siblings as we go
	current := tree
	depth := 0

	for current != nil && current.Left != nil && current.Right != nil {
		// Determine which child to follow based on key bit
		keyPath := core.KeyPath(key)
		bit := getGpBit(keyPath[:], depth)

		var siblingHash [32]byte
		if bit {
			// Going right - append left sibling hash
			copy(siblingHash[:], current.Left.Hash[:])
			current = current.Right
		} else {
			// Going left - append right sibling hash
			copy(siblingHash[:], current.Right.Hash[:])
			current = current.Left
		}

		// Append sibling at this depth
		// path[0] will be depth 0, path[1] will be depth 1, etc.
		pathNodes = append(pathNodes, ProofNode{
			Hash: siblingHash,
			Left: !bit, // If we went right (bit=1), sibling is left (Left=true)
		})
		depth++
	}

	return pathNodes
}

// defaultHasher provides simple value hashing.
type defaultHasher struct{}

func (h *defaultHasher) Hash(value []byte) [32]byte {
	// Simple hash implementation for value hashing
	var hash [32]byte
	copy(hash[:], value)
	return hash
}
