package state

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"golang.org/x/crypto/blake2b"
)

// FinalizedBlockProvider supplies finalized blocks for cache updates.
type FinalizedBlockProvider interface {
	GetFinalizedBlock() (*types.Block, error)
}

// BuilderStateProvider supplies the builder-sourced Orchard state.
type BuilderStateProvider interface {
	CommitmentSnapshot() ([32]byte, uint64, [][32]byte, error)
	NullifierSnapshot() ([32]byte, uint64, error)
}

// OrchardStateCache tracks the latest finalized Orchard state for builders.
type OrchardStateCache struct {
	mu sync.RWMutex

	serviceID uint32
	builder   BuilderStateProvider

	commitmentRoot     [32]byte
	commitmentSize     uint64
	commitmentFrontier [][32]byte
	nullifierRoot      [32]byte
	nullifierSize      uint64

	lastFinalizedSlot uint64
	lastFinalizedHash common.Hash
	lastSyncAt        time.Time
}

// StateRootsSnapshot captures a consistent view of the Orchard roots.
type StateRootsSnapshot struct {
	CommitmentRoot     [32]byte
	CommitmentSize     uint64
	CommitmentFrontier [][32]byte
	NullifierRoot      [32]byte
	NullifierSize      uint64
	LastSyncAt         time.Time
}

// NewOrchardStateCache creates an empty cache bound to the given service.
func NewOrchardStateCache(serviceID uint32, builder BuilderStateProvider) *OrchardStateCache {
	return &OrchardStateCache{
		serviceID: serviceID,
		builder:   builder,
	}
}

// SyncFromStateDB rebuilds the cache using the current builder state.
func (c *OrchardStateCache) SyncFromStateDB() error {
	if c.builder == nil {
		return fmt.Errorf("builder state provider unavailable")
	}

	commitmentRoot, commitmentSize, commitmentFrontier, err := c.builder.CommitmentSnapshot()
	if err != nil {
		return err
	}

	nullifierRoot, nullifierSize, err := c.builder.NullifierSnapshot()
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.commitmentRoot = commitmentRoot
	c.commitmentSize = commitmentSize
	c.commitmentFrontier = make([][32]byte, len(commitmentFrontier))
	copy(c.commitmentFrontier, commitmentFrontier)
	c.nullifierRoot = nullifierRoot
	c.nullifierSize = nullifierSize
	c.lastSyncAt = time.Now()
	c.mu.Unlock()

	return nil
}

// StartFinalizedBlockTracker polls finalized blocks and refreshes state from builder storage.
func (c *OrchardStateCache) StartFinalizedBlockTracker(ctx context.Context, provider FinalizedBlockProvider, interval time.Duration) {
	if provider == nil {
		return
	}
	if interval <= 0 {
		interval = 6 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				block, err := provider.GetFinalizedBlock()
				if err != nil || block == nil {
					continue
				}
				slot := uint64(block.Header.Slot)
				if !c.shouldSyncFinalized(slot, block.Header.Hash()) {
					continue
				}
				if err := c.SyncFromStateDB(); err != nil {
					log.Warn(log.Node, "Orchard state cache sync failed", "err", err)
					continue
				}

				// Note: Builder state is sourced from local storage and work packages,
				// not reconstructed from finalized chain blocks.

				c.mu.Lock()
				c.lastFinalizedSlot = slot
				c.lastFinalizedHash = block.Header.Hash()
				c.mu.Unlock()
				log.Debug(log.Node, "Orchard state cache updated", "slot", slot)
			}
		}
	}()
}

// CommitmentRoot returns the cached commitment tree root.
func (c *OrchardStateCache) CommitmentRoot() [32]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.commitmentRoot
}

// CommitmentSize returns the cached commitment tree size.
func (c *OrchardStateCache) CommitmentSize() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.commitmentSize
}

// CommitmentFrontier returns a copy of the cached frontier.
func (c *OrchardStateCache) CommitmentFrontier() [][32]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([][32]byte, len(c.commitmentFrontier))
	copy(out, c.commitmentFrontier)
	return out
}

// NullifierRoot returns the cached nullifier root.
func (c *OrchardStateCache) NullifierRoot() [32]byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nullifierRoot
}

// LastSyncAt returns the timestamp of the latest successful sync.
func (c *OrchardStateCache) LastSyncAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSyncAt
}

// StateRootsSnapshot returns a consistent snapshot of state roots and frontier.
func (c *OrchardStateCache) StateRootsSnapshot() StateRootsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	frontierCopy := make([][32]byte, len(c.commitmentFrontier))
	copy(frontierCopy, c.commitmentFrontier)

	return StateRootsSnapshot{
		CommitmentRoot:     c.commitmentRoot,
		CommitmentSize:     c.commitmentSize,
		CommitmentFrontier: frontierCopy,
		NullifierRoot:      c.nullifierRoot,
		NullifierSize:      c.nullifierSize,
		LastSyncAt:         c.lastSyncAt,
	}
}

// LastFinalizedSlot returns the latest slot seen by the cache.
func (c *OrchardStateCache) LastFinalizedSlot() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastFinalizedSlot
}

func (c *OrchardStateCache) shouldSyncFinalized(slot uint64, hash common.Hash) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slot > c.lastFinalizedSlot || hash != c.lastFinalizedHash
}

// SparseNullifierTree maintains a sparse Merkle tree for nullifier tracking.
type SparseNullifierTree struct {
	depth      int
	leaves     map[uint64][32]byte
	positions  []uint64
	emptyRoots [][32]byte
	cachedRoot [32]byte
	rootCached bool
}

// SparseMerkleProof is a proof of membership or absence.
type SparseMerkleProof struct {
	Leaf     [32]byte
	Siblings [][32]byte
	Position uint64
	Root     [32]byte
}

// NewSparseNullifierTree creates a sparse tree for the given depth.
func NewSparseNullifierTree(depth int) *SparseNullifierTree {
	return &SparseNullifierTree{
		depth:      depth,
		leaves:     make(map[uint64][32]byte),
		positions:  make([]uint64, 0),
		emptyRoots: buildEmptySparseRoots(depth),
	}
}

// Insert marks a nullifier as spent.
func (t *SparseNullifierTree) Insert(nullifier [32]byte) {
	position := sparsePositionFor(nullifier, t.depth)
	if _, exists := t.leaves[position]; exists {
		return
	}
	t.leaves[position] = hashSparseLeaf(nullifier)
	t.rootCached = false
	t.positions = insertSortedPosition(t.positions, position)
}

// Contains checks if a nullifier is already recorded.
func (t *SparseNullifierTree) Contains(nullifier [32]byte) bool {
	position := sparsePositionFor(nullifier, t.depth)
	_, ok := t.leaves[position]
	return ok
}

// Len returns the number of inserted nullifiers.
func (t *SparseNullifierTree) Len() int {
	return len(t.leaves)
}

// Root returns the current sparse tree root.
func (t *SparseNullifierTree) Root() [32]byte {
	if t.rootCached {
		return t.cachedRoot
	}
	if len(t.leaves) == 0 {
		t.cachedRoot = t.emptyRoots[t.depth]
		t.rootCached = true
		return t.cachedRoot
	}
	t.cachedRoot = t.subtreeHash(t.depth, 0)
	t.rootCached = true
	return t.cachedRoot
}

// ProveAbsence builds a proof that a nullifier is not present.
func (t *SparseNullifierTree) ProveAbsence(nullifier [32]byte) SparseMerkleProof {
	position := sparsePositionFor(nullifier, t.depth)
	return SparseMerkleProof{
		Leaf:     sparseEmptyLeaf(),
		Siblings: t.siblingHashes(position),
		Position: position,
		Root:     t.Root(),
	}
}

// ProveMembership builds a proof that a nullifier is present.
func (t *SparseNullifierTree) ProveMembership(nullifier [32]byte) SparseMerkleProof {
	position := sparsePositionFor(nullifier, t.depth)
	return SparseMerkleProof{
		Leaf:     hashSparseLeaf(nullifier),
		Siblings: t.siblingHashes(position),
		Position: position,
		Root:     t.Root(),
	}
}

// Verify checks the proof against its root.
func (p SparseMerkleProof) Verify() bool {
	current := p.Leaf
	position := p.Position
	for _, sibling := range p.Siblings {
		var left [32]byte
		var right [32]byte
		if position&1 == 0 {
			left = current
			right = sibling
		} else {
			left = sibling
			right = current
		}
		current = hashSparseNode(left, right)
		position >>= 1
	}
	return current == p.Root
}

func (t *SparseNullifierTree) siblingHashes(position uint64) [][32]byte {
	siblings := make([][32]byte, 0, t.depth)
	for height := 0; height < t.depth; height++ {
		prefix := position >> height
		siblingPrefix := prefix ^ 1
		siblings = append(siblings, t.subtreeHash(height, siblingPrefix))
	}
	return siblings
}

func (t *SparseNullifierTree) subtreeHash(height int, prefix uint64) [32]byte {
	if !t.hasLeafInSubtree(height, prefix) {
		return t.emptyRoots[height]
	}
	if height == 0 {
		if leaf, ok := t.leaves[prefix]; ok {
			return leaf
		}
		return sparseEmptyLeaf()
	}
	left := t.subtreeHash(height-1, prefix<<1)
	right := t.subtreeHash(height-1, (prefix<<1)|1)
	return hashSparseNode(left, right)
}

func (t *SparseNullifierTree) hasLeafInSubtree(height int, prefix uint64) bool {
	if len(t.positions) == 0 {
		return false
	}
	start := prefix << height
	end := (prefix + 1) << height
	idx := sort.Search(len(t.positions), func(i int) bool {
		return t.positions[i] >= start
	})
	return idx < len(t.positions) && t.positions[idx] < end
}

func insertSortedPosition(positions []uint64, position uint64) []uint64 {
	idx := sort.Search(len(positions), func(i int) bool {
		return positions[i] >= position
	})
	if idx < len(positions) && positions[idx] == position {
		return positions
	}
	positions = append(positions, 0)
	copy(positions[idx+1:], positions[idx:])
	positions[idx] = position
	return positions
}

func buildEmptySparseRoots(depth int) [][32]byte {
	roots := make([][32]byte, depth+1)
	roots[0] = sparseEmptyLeaf()
	for height := 0; height < depth; height++ {
		roots[height+1] = hashSparseNode(roots[height], roots[height])
	}
	return roots
}

func sparsePositionFor(value [32]byte, depth int) uint64 {
	prefix := binary.LittleEndian.Uint64(value[:8])
	if depth < 64 {
		prefix &= (uint64(1) << depth) - 1
	}
	return prefix
}

func sparseEmptyLeaf() [32]byte {
	return hashSparseLeaf([32]byte{})
}

func hashSparseLeaf(value [32]byte) [32]byte {
	data := make([]byte, 0, 1+len(value))
	data = append(data, 0x00)
	data = append(data, value[:]...)
	sum := blake2b.Sum256(data)
	return sum
}

func hashSparseNode(left [32]byte, right [32]byte) [32]byte {
	data := make([]byte, 0, 1+len(left)+len(right))
	data = append(data, 0x01)
	data = append(data, left[:]...)
	data = append(data, right[:]...)
	sum := blake2b.Sum256(data)
	return sum
}
