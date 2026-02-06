package bmt

import (
	"bytes"
	"fmt"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/core"
	"github.com/jam-duna/jamduna/bmt/core/proof"
	"github.com/jam-duna/jamduna/bmt/overlay"
)

// Enhanced session types for transaction semantics and witness generation

// OperationType represents the type of database operation
type OperationType int

const (
	OpRead OperationType = iota
	OpWrite
	OpDelete
	OpReadThenWrite
)

// Operation represents a single database operation
type Operation struct {
	Type  OperationType
	Key   [32]byte
	Value []byte
	Old   []byte // For rollback support
}

// WriteSession provides transactional write operations
type WriteSession struct {
	db           *Nomt
	overlay      *overlay.LiveOverlay
	prevRoot     [32]byte
	operations   []Operation
	witnessMode  bool
	rollbackData map[[32]byte][]byte
	closed       bool
}

// ReadSession provides snapshot isolation for reads
type ReadSession struct {
	root     [32]byte
	tree     *beatreeWrapper
	overlays []*overlay.Overlay // Overlay chain for reads
	closed   bool
}

// PreparedSession represents a transaction ready for commit
type PreparedSession struct {
	db            *Nomt
	prevRoot      [32]byte
	newRoot       [32]byte
	changeset     map[beatree.Key]*beatree.Change
	operations    []Operation
	rollbackDelta map[beatree.Key][]byte // Not used currently, for future rollback support
	witnessMode   bool
	overlay       *overlay.Overlay
}

// Session factory methods

// BeginWrite creates a new write session
func (n *Nomt) BeginWrite() (*WriteSession, error) {
	return n.beginWriteInternal(false, nil)
}

// BeginWriteWithWitness creates a new write session with witness tracking
func (n *Nomt) BeginWriteWithWitness() (*WriteSession, error) {
	return n.beginWriteInternal(true, nil)
}

// BeginWriteWithOverlay creates a write session building on existing overlays
func (n *Nomt) BeginWriteWithOverlay(overlays []*overlay.Overlay) (*WriteSession, error) {
	return n.beginWriteInternal(false, overlays)
}

// BeginRead creates a read-only snapshot session
func (n *Nomt) BeginRead() (*ReadSession, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.closed {
		return nil, fmt.Errorf("database is closed")
	}

	// Capture current committed overlays for consistent reads
	overlays := make([]*overlay.Overlay, len(n.committedOverlays))
	copy(overlays, n.committedOverlays)

	return &ReadSession{
		root:     n.root,
		tree:     n.tree,
		overlays: overlays,
		closed:   false,
	}, nil
}

// BeginReadAt creates a read session at a specific root
func (n *Nomt) BeginReadAt(root [32]byte) (*ReadSession, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.closed {
		return nil, fmt.Errorf("database is closed")
	}

	// Find overlays up to this root
	// For now, just use empty overlays if root doesn't match current
	var overlays []*overlay.Overlay
	if root == n.root {
		overlays = make([]*overlay.Overlay, len(n.committedOverlays))
		copy(overlays, n.committedOverlays)
	}

	return &ReadSession{
		root:     root,
		tree:     n.tree,
		overlays: overlays,
		closed:   false,
	}, nil
}

// Internal helper for creating write sessions
func (n *Nomt) beginWriteInternal(witnessMode bool, baseOverlays []*overlay.Overlay) (*WriteSession, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return nil, fmt.Errorf("database is closed")
	}

	// Determine parent overlay
	var parent *overlay.Overlay
	if len(baseOverlays) > 0 {
		parent = baseOverlays[len(baseOverlays)-1]
	} else if len(n.committedOverlays) > 0 {
		parent = n.committedOverlays[len(n.committedOverlays)-1]
	}

	// Create new live overlay for this session
	sessionOverlay := overlay.NewLiveOverlay(n.root, parent)

	return &WriteSession{
		db:           n,
		overlay:      sessionOverlay,
		prevRoot:     n.root,
		operations:   make([]Operation, 0),
		witnessMode:  witnessMode,
		rollbackData: make(map[[32]byte][]byte),
		closed:       false,
	}, nil
}

// WriteSession API

// Insert adds or updates a key-value pair in the session
func (s *WriteSession) Insert(key [32]byte, value []byte) error {
	if s.closed {
		return fmt.Errorf("session is closed")
	}

	// Record operation for witness generation
	oldValue, _ := s.Get(key)
	s.operations = append(s.operations, Operation{
		Type:  OpWrite,
		Key:   key,
		Value: value,
		Old:   oldValue,
	})

	// Store rollback data
	s.rollbackData[key] = oldValue

	// Apply to overlay
	k := beatree.KeyFromBytes(key[:])
	change := beatree.NewInsertChange(value)
	return s.overlay.SetValue(k, change)
}

// Delete removes a key from the session
func (s *WriteSession) Delete(key [32]byte) error {
	if s.closed {
		return fmt.Errorf("session is closed")
	}

	// Get old value before deletion
	oldValue, _ := s.Get(key)

	// Record operation
	s.operations = append(s.operations, Operation{
		Type: OpDelete,
		Key:  key,
		Old:  oldValue,
	})

	// Store rollback data
	s.rollbackData[key] = oldValue

	// Apply to overlay
	k := beatree.KeyFromBytes(key[:])
	change := beatree.NewDeleteChange()
	return s.overlay.SetValue(k, change)
}

// Get retrieves a value from the session state (overlay + committed)
func (s *WriteSession) Get(key [32]byte) ([]byte, error) {
	if s.closed {
		return nil, fmt.Errorf("session is closed")
	}

	k := beatree.KeyFromBytes(key[:])

	// Record read operation if witnessing
	if s.witnessMode {
		s.operations = append(s.operations, Operation{
			Type: OpRead,
			Key:  key,
		})
	}

	// Check overlay first
	value, err := s.overlay.Lookup(k)
	if err != nil {
		return nil, err
	}
	if value != nil {
		return value, nil
	}

	// Fall back to committed tree
	return s.db.tree.Get(k)
}

// Root returns the current root hash (computed from overlay)
func (s *WriteSession) Root() [32]byte {
	if s.closed {
		return s.prevRoot
	}
	// Compute intermediate root from current overlay state
	return s.computeCurrentRoot()
}

// computeCurrentRoot computes root hash from overlay and committed data
func (s *WriteSession) computeCurrentRoot() [32]byte {
	// Build combined key-value pairs from overlay + committed tree
	kvPairs := s.buildCombinedKVPairs()

	if len(kvPairs) == 0 {
		return [32]byte{} // Empty tree
	}

	// Build GP tree and return root hash
	hasher := core.Blake2bBinaryHasher{}
	rootNode := core.BuildGpTree(kvPairs, 0, hasher.Hash)
	return [32]byte(rootNode.Hash)
}

// buildCombinedKVPairs builds key-value pairs from overlay + committed data
func (s *WriteSession) buildCombinedKVPairs() []core.KVPair {
	// Get all values from overlay
	overlayData := s.overlay.GetAllValues()

	// Create combined map (overlay overrides committed)
	combined := make(map[[32]byte][]byte)

	// First, get all committed data from tree
	// (In production, this would iterate through the tree efficiently)
	// For now, we only have overlay data in memory

	// Apply overlay changes
	for key, change := range overlayData {
		var k [32]byte
		copy(k[:], key[:])

		if change.IsInsert() {
			// Insert or update
			combined[k] = change.Value
		} else {
			// Delete - remove from combined set
			delete(combined, k)
		}
	}

	// Convert to sorted KV pairs
	kvPairs := make([]core.KVPair, 0, len(combined))
	for key, value := range combined {
		kvPairs = append(kvPairs, core.KVPair{
			Key:   core.KeyPath(key),
			Value: value,
		})
	}

	// Sort by key for deterministic tree construction
	for i := 0; i < len(kvPairs)-1; i++ {
		for j := i + 1; j < len(kvPairs); j++ {
			if bytes.Compare(kvPairs[i].Key[:], kvPairs[j].Key[:]) > 0 {
				kvPairs[i], kvPairs[j] = kvPairs[j], kvPairs[i]
			}
		}
	}

	return kvPairs
}

// computeRootFromChangeset computes root hash from a changeset
func (s *WriteSession) computeRootFromChangeset(changeset map[beatree.Key]*beatree.Change) [32]byte {
	// Build key-value pairs from changeset
	combined := make(map[[32]byte][]byte)

	for key, change := range changeset {
		var k [32]byte
		copy(k[:], key[:])

		if change.IsInsert() {
			combined[k] = change.Value
		}
		// Deletes are omitted (not in final state)
	}

	if len(combined) == 0 {
		return [32]byte{} // Empty tree
	}

	// Convert to sorted KV pairs
	kvPairs := make([]core.KVPair, 0, len(combined))
	for key, value := range combined {
		kvPairs = append(kvPairs, core.KVPair{
			Key:   core.KeyPath(key),
			Value: value,
		})
	}

	// Sort by key
	for i := 0; i < len(kvPairs)-1; i++ {
		for j := i + 1; j < len(kvPairs); j++ {
			if bytes.Compare(kvPairs[i].Key[:], kvPairs[j].Key[:]) > 0 {
				kvPairs[i], kvPairs[j] = kvPairs[j], kvPairs[i]
			}
		}
	}

	// Build GP tree and return root
	hasher := core.Blake2bBinaryHasher{}
	rootNode := core.BuildGpTree(kvPairs, 0, hasher.Hash)
	return [32]byte(rootNode.Hash)
}

// Prepare freezes the session and prepares it for commit
func (s *WriteSession) Prepare() (*PreparedSession, error) {
	if s.closed {
		return nil, fmt.Errorf("session is closed")
	}
	s.closed = true

	// Freeze overlay
	frozenOverlay, err := s.overlay.Freeze()
	if err != nil {
		return nil, fmt.Errorf("failed to freeze overlay: %w", err)
	}

	// Build changeset for commit
	changeset := make(map[beatree.Key]*beatree.Change)
	for key, change := range frozenOverlay.Data().GetAllValues() {
		changeset[key] = change
	}

	// Compute new root from frozen overlay state
	newRoot := s.computeRootFromChangeset(changeset)

	// Convert rollback data to proper key type
	rollbackDelta := make(map[beatree.Key][]byte)
	for key, value := range s.rollbackData {
		k := beatree.KeyFromBytes(key[:])
		rollbackDelta[k] = value
	}

	return &PreparedSession{
		db:            s.db,
		prevRoot:      s.prevRoot,
		newRoot:       newRoot,
		changeset:     changeset,
		operations:    s.operations,
		rollbackDelta: rollbackDelta,
		witnessMode:   s.witnessMode,
		overlay:       frozenOverlay,
	}, nil
}

// GenerateWitness generates a witness for operations in this session
func (s *WriteSession) GenerateWitness() (*Witness, error) {
	if !s.witnessMode {
		return nil, fmt.Errorf("witness mode not enabled")
	}

	// Build GP tree from current state
	kvPairs := s.buildCombinedKVPairs()
	hasher := core.Blake2bBinaryHasher{}
	gpTree := core.BuildGpTree(kvPairs, 0, hasher.Hash)

	witness := &Witness{
		PrevRoot: s.prevRoot,
		Root:     [32]byte(gpTree.Hash), // Current computed root
		Keys:     make([][32]byte, 0),
		Proofs:   make([]MerkleProof, 0),
	}

	// Generate merkle proof for each operation
	for _, op := range s.operations {
		witness.Keys = append(witness.Keys, op.Key)

		// Generate proof for this key
		pathProof := s.generateProofForKey(gpTree, op.Key, hasher)

		// Convert to MerkleProof format
		merkleProof := MerkleProof{
			Key:   op.Key,
			Value: op.Value,
			Path:  convertProofNodes(pathProof.Path),
		}
		witness.Proofs = append(witness.Proofs, merkleProof)
	}

	return witness, nil
}

// generateProofForKey generates a proof for a specific key in the GP tree
func (s *WriteSession) generateProofForKey(tree *core.GpTrieNode, key [32]byte, hasher core.Blake2bBinaryHasher) *proof.PathProof {
	// Build proof by walking tree from root to key
	pathNodes := make([]proof.ProofNode, 0)

	// Walk the tree following the key's path
	current := tree
	depth := 0

	for current != nil && current.Left != nil && current.Right != nil {
		// Determine which child to follow based on key bit
		keyPath := core.KeyPath(key)
		bit := getGpBit(keyPath[:], depth)

		if bit {
			// Going right - left sibling
			pathNodes = append(pathNodes, proof.ProofNode{
				Node:         current.Right.Hash,
				Sibling:      current.Left.Hash,
				IsRightChild: true,
			})
			current = current.Right
		} else {
			// Going left - right sibling
			pathNodes = append(pathNodes, proof.ProofNode{
				Node:         current.Left.Hash,
				Sibling:      current.Right.Hash,
				IsRightChild: false,
			})
			current = current.Left
		}
		depth++
	}

	// Get value for this key
	value, _ := s.Get(key)

	return &proof.PathProof{
		Key:   core.KeyPath(key),
		Value: value,
		Path:  pathNodes,
		Root:  tree.Hash,
	}
}

// convertProofNodes converts core proof nodes to MerkleProof path nodes
func convertProofNodes(nodes []proof.ProofNode) []ProofNode {
	result := make([]ProofNode, len(nodes))
	for i, node := range nodes {
		result[i] = ProofNode{
			Hash: [32]byte(node.Sibling), // Store sibling hash
			Left: !node.IsRightChild,     // Left if not right child
		}
	}
	return result
}

// getGpBit gets the i-th bit of a key (MSB first)
func getGpBit(key []byte, bitIndex int) bool {
	byteIndex := bitIndex / 8
	if byteIndex >= len(key) {
		return false
	}
	bitPos := 7 - (bitIndex % 8)
	b := key[byteIndex]
	mask := byte(1 << bitPos)
	return (b & mask) != 0
}

// VerifyWitness verifies a witness against the expected root hash
func VerifyWitness(witness *Witness, expectedRoot [32]byte) error {
	if witness == nil {
		return fmt.Errorf("witness is nil")
	}

	hasher := &core.GpNodeHasher{}

	// Verify each proof in the witness
	for i, merkleProof := range witness.Proofs {
		// Convert MerkleProof to proof.PathProof
		pathProof := &proof.PathProof{
			Key:   core.KeyPath(merkleProof.Key),
			Value: merkleProof.Value,
			Path:  convertToProofNodes(merkleProof.Path),
			Root:  core.Node(expectedRoot),
		}

		// Verify this proof
		if !pathProof.Verify(hasher) {
			return fmt.Errorf("proof verification failed for key %d (%x)", i, merkleProof.Key)
		}
	}

	return nil
}

// convertToProofNodes converts MerkleProof path nodes to core proof nodes
func convertToProofNodes(nodes []ProofNode) []proof.ProofNode {
	result := make([]proof.ProofNode, len(nodes))
	for i, node := range nodes {
		result[i] = proof.ProofNode{
			Node:         core.Node(node.Hash),
			Sibling:      core.Node(node.Hash),
			IsRightChild: !node.Left,
		}
	}
	return result
}

// PreparedSession API

// Commit commits the prepared session to the database
func (p *PreparedSession) Commit() error {
	p.db.mu.Lock()
	defer p.db.mu.Unlock()

	// Verify root hasn't changed (optimistic concurrency control)
	if p.db.root != p.prevRoot {
		return fmt.Errorf("stale transaction: root changed from %x to %x", p.prevRoot, p.db.root)
	}

	// Commit rollback delta first
	recordId, err := p.db.rollbackSys.Commit(p.changeset)
	if err != nil {
		p.overlay.MarkDropped()
		return fmt.Errorf("rollback commit failed: %w", err)
	}

	// Apply changeset to tree
	if err := p.db.tree.ApplyChangeset(p.changeset); err != nil {
		p.db.rollbackSys.RollbackSingle()
		p.overlay.MarkDropped()
		return fmt.Errorf("changeset apply failed: %w", err)
	}

	// Sync to disk
	if err := p.db.tree.Sync(); err != nil {
		p.db.rollbackSys.RollbackSingle()
		p.overlay.MarkDropped()
		return fmt.Errorf("sync failed: %w", err)
	}

	// Update database state
	p.db.root = p.db.tree.Root()
	p.newRoot = p.db.root // Update with actual committed root

	p.overlay.MarkCommitted()
	p.db.committedOverlays = append(p.db.committedOverlays, p.overlay)
	p.db.liveOverlay = overlay.NewLiveOverlay(p.db.root, p.overlay)

	p.db.metrics.RecordCommit()

	// Store rollback ID for reference
	_ = recordId

	return nil
}

// ToOverlay converts the prepared session to an overlay without committing
func (p *PreparedSession) ToOverlay() *overlay.Overlay {
	return p.overlay
}

// Witness generates a witness if session was created with witness mode
func (p *PreparedSession) Witness() (*Witness, error) {
	if !p.witnessMode {
		return nil, fmt.Errorf("witness mode not enabled")
	}

	// Build GP tree from changeset to generate proofs
	kvPairs := p.buildKVPairsFromChangeset()
	hasher := core.Blake2bBinaryHasher{}
	gpTree := core.BuildGpTree(kvPairs, 0, hasher.Hash)

	// Collect all keys from operations
	keys := make([][32]byte, 0, len(p.operations))
	proofs := make([]MerkleProof, 0, len(p.operations))

	for _, op := range p.operations {
		keys = append(keys, op.Key)

		// Generate proof for this key
		pathProof := p.generateProofForKey(gpTree, op.Key, hasher)

		// Convert to MerkleProof format
		merkleProof := MerkleProof{
			Key:   op.Key,
			Value: op.Value,
			Path:  convertProofNodes(pathProof.Path),
		}
		proofs = append(proofs, merkleProof)
	}

	return &Witness{
		PrevRoot: p.prevRoot,
		Root:     p.newRoot,
		Keys:     keys,
		Proofs:   proofs,
	}, nil
}

// buildKVPairsFromChangeset builds sorted KV pairs from prepared changeset
func (p *PreparedSession) buildKVPairsFromChangeset() []core.KVPair {
	combined := make(map[[32]byte][]byte)

	for key, change := range p.changeset {
		var k [32]byte
		copy(k[:], key[:])

		if change.IsInsert() {
			combined[k] = change.Value
		}
	}

	kvPairs := make([]core.KVPair, 0, len(combined))
	for key, value := range combined {
		kvPairs = append(kvPairs, core.KVPair{
			Key:   core.KeyPath(key),
			Value: value,
		})
	}

	// Sort by key
	for i := 0; i < len(kvPairs)-1; i++ {
		for j := i + 1; j < len(kvPairs); j++ {
			if bytes.Compare(kvPairs[i].Key[:], kvPairs[j].Key[:]) > 0 {
				kvPairs[i], kvPairs[j] = kvPairs[j], kvPairs[i]
			}
		}
	}

	return kvPairs
}

// generateProofForKey generates a proof for a specific key
func (p *PreparedSession) generateProofForKey(tree *core.GpTrieNode, key [32]byte, hasher core.Blake2bBinaryHasher) *proof.PathProof {
	pathNodes := make([]proof.ProofNode, 0)

	current := tree
	depth := 0

	for current != nil && current.Left != nil && current.Right != nil {
		keyPath := core.KeyPath(key)
		bit := getGpBit(keyPath[:], depth)

		if bit {
			pathNodes = append(pathNodes, proof.ProofNode{
				Node:         current.Right.Hash,
				Sibling:      current.Left.Hash,
				IsRightChild: true,
			})
			current = current.Right
		} else {
			pathNodes = append(pathNodes, proof.ProofNode{
				Node:         current.Left.Hash,
				Sibling:      current.Right.Hash,
				IsRightChild: false,
			})
			current = current.Left
		}
		depth++
	}

	// Get value from operations
	var value []byte
	for _, op := range p.operations {
		if op.Key == key {
			value = op.Value
			break
		}
	}

	return &proof.PathProof{
		Key:   core.KeyPath(key),
		Value: value,
		Path:  pathNodes,
		Root:  tree.Hash,
	}
}

// Root returns the new root hash
func (p *PreparedSession) Root() [32]byte {
	return p.newRoot
}

// PrevRoot returns the previous root hash
func (p *PreparedSession) PrevRoot() [32]byte {
	return p.prevRoot
}

// Rollback discards the prepared session
func (p *PreparedSession) Rollback() {
	p.overlay.MarkDropped()
}

// ReadSession API

// Get retrieves a value from the read session snapshot
func (s *ReadSession) Get(key [32]byte) ([]byte, error) {
	if s.closed {
		return nil, fmt.Errorf("session is closed")
	}

	k := beatree.KeyFromBytes(key[:])

	// Check overlay chain first (most recent to oldest)
	for i := len(s.overlays) - 1; i >= 0; i-- {
		value, err := s.overlays[i].Lookup(k)
		if err == nil && value != nil {
			return value, nil
		}
	}

	// Fall back to committed tree
	return s.tree.Get(k)
}

// GenerateProof generates a merkle proof for a key at this snapshot
func (s *ReadSession) GenerateProof(key [32]byte) (*MerkleProof, error) {
	if s.closed {
		return nil, fmt.Errorf("session is closed")
	}

	// Get the value
	value, err := s.Get(key)
	if err != nil {
		return nil, err
	}

	// Note: ReadSession proof generation could be enhanced by storing
	// the GP tree snapshot, but for now we return placeholder proof
	// since read sessions don't modify state

	return &MerkleProof{
		Key:   key,
		Value: value,
		Path:  nil, // Placeholder - read sessions don't have full tree access
	}, nil
}

// Root returns the root hash of this snapshot
func (s *ReadSession) Root() [32]byte {
	return s.root
}

// Close closes the read session
func (s *ReadSession) Close() error {
	s.closed = true
	return nil
}
