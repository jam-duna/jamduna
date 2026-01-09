package storage

import (
	"sort"
	"sync"
)

// StemStore provides persistent storage for stems and prefix queries.
type StemStore interface {
	GetStem(stem Stem) (*StemNode, bool)
	PutStem(stem Stem, node *StemNode) error
	DeleteStem(stem Stem) error
	IterByPrefix(prefix Stem, prefixBits int) StemIterator
	GetSubtreeHash(prefix Stem, prefixBits int) (hash [32]byte, ok bool)
}

// HashCacheStore persists derived hashes (optional).
type HashCacheStore interface {
	GetStemHash(stem Stem) (hash [32]byte, ok bool)
	PutStemHash(stem Stem, hash [32]byte) error
	GetNodeHash(depthBits uint16, prefix Stem) (hash [32]byte, ok bool)
	PutNodeHash(depthBits uint16, prefix Stem, hash [32]byte) error
}

// NodeCacheKey identifies a cached internal node hash.
// PathPrefix MUST be canonicalized for DepthBits.
type NodeCacheKey struct {
	DepthBits  uint16
	PathPrefix Stem
}

// StemIterator iterates stems in canonical order.
type StemIterator interface {
	Next() (stem Stem, node *StemNode, ok bool)
	Close() error
}

// MemoryUBTStore is an in-memory StemStore + HashCacheStore.
type MemoryUBTStore struct {
	stems      map[Stem]*StemNode
	stemHashes map[Stem][32]byte
	nodeHashes map[NodeCacheKey][32]byte
	hasher     Hasher
}

func NewMemoryUBTStore(hasher Hasher) *MemoryUBTStore {
	if hasher == nil {
		hasher = NewBlake3Hasher(EIPProfile)
	}
	return &MemoryUBTStore{
		stems:      make(map[Stem]*StemNode),
		stemHashes: make(map[Stem][32]byte),
		nodeHashes: make(map[NodeCacheKey][32]byte),
		hasher:     hasher,
	}
}

func (s *MemoryUBTStore) GetStem(stem Stem) (*StemNode, bool) {
	node, ok := s.stems[stem]
	if !ok {
		return nil, false
	}
	return cloneStemNode(node), true
}

func (s *MemoryUBTStore) PutStem(stem Stem, node *StemNode) error {
	s.stems[stem] = cloneStemNode(node)
	return nil
}

func (s *MemoryUBTStore) DeleteStem(stem Stem) error {
	delete(s.stems, stem)
	return nil
}

func (s *MemoryUBTStore) IterByPrefix(prefix Stem, prefixBits int) StemIterator {
	if prefixBits < 0 || prefixBits > 248 {
		return &sliceStemIterator{stems: nil, store: s}
	}
	stems := make([]Stem, 0, len(s.stems))
	for stem := range s.stems {
		if stemHasPrefix(stem, prefix, prefixBits) {
			stems = append(stems, stem)
		}
	}
	sort.Slice(stems, func(i, j int) bool {
		return stemLess(stems[i], stems[j])
	})
	return &sliceStemIterator{stems: stems, store: s}
}

func (s *MemoryUBTStore) GetSubtreeHash(prefix Stem, prefixBits int) (hash [32]byte, ok bool) {
	if prefixBits < 0 || prefixBits > 248 {
		return [32]byte{}, false
	}
	stems := make([]Stem, 0, len(s.stems))
	for stem := range s.stems {
		if stemHasPrefix(stem, prefix, prefixBits) {
			stems = append(stems, stem)
		}
	}
	if len(stems) == 0 {
		return [32]byte{}, true
	}
	return s.hashStemTrie(stems, prefixBits), true
}

func (s *MemoryUBTStore) GetStemHash(stem Stem) (hash [32]byte, ok bool) {
	hash, ok = s.stemHashes[stem]
	return hash, ok
}

func (s *MemoryUBTStore) PutStemHash(stem Stem, hash [32]byte) error {
	s.stemHashes[stem] = hash
	return nil
}

func (s *MemoryUBTStore) GetNodeHash(depthBits uint16, prefix Stem) (hash [32]byte, ok bool) {
	hash, ok = s.nodeHashes[NodeCacheKey{DepthBits: depthBits, PathPrefix: prefix}]
	return hash, ok
}

func (s *MemoryUBTStore) PutNodeHash(depthBits uint16, prefix Stem, hash [32]byte) error {
	s.nodeHashes[NodeCacheKey{DepthBits: depthBits, PathPrefix: prefix}] = hash
	return nil
}

// UBTOverlay stages changes on top of a base StemStore.
type UBTOverlay struct {
	base          StemStore
	hasher        Hasher
	stagedStems   map[Stem]*StemNode
	stagedDeletes map[Stem]bool
	stagedMutex   sync.Mutex
}

func NewUBTOverlay(base StemStore, hasher Hasher) *UBTOverlay {
	if hasher == nil {
		hasher = NewBlake3Hasher(EIPProfile)
	}
	return &UBTOverlay{
		base:          base,
		hasher:        hasher,
		stagedStems:   make(map[Stem]*StemNode),
		stagedDeletes: make(map[Stem]bool),
	}
}

func (o *UBTOverlay) Insert(key TreeKey, value [32]byte) {
	if value == ([32]byte{}) {
		o.Delete(key)
		return
	}
	o.stagedMutex.Lock()
	defer o.stagedMutex.Unlock()

	stemNode, ok := o.stagedStems[key.Stem]
	if !ok {
		baseNode, ok := o.base.GetStem(key.Stem)
		if ok {
			stemNode = cloneStemNode(baseNode)
		} else {
			stemNode = NewStemNode(key.Stem)
		}
	}
	stemNode.SetValue(key.Subindex, value)
	o.stagedStems[key.Stem] = stemNode
	delete(o.stagedDeletes, key.Stem)
}

func (o *UBTOverlay) Delete(key TreeKey) {
	o.stagedMutex.Lock()
	defer o.stagedMutex.Unlock()

	stemNode, ok := o.stagedStems[key.Stem]
	if !ok {
		baseNode, ok := o.base.GetStem(key.Stem)
		if !ok {
			return
		}
		stemNode = cloneStemNode(baseNode)
	}
	stemNode.DeleteValue(key.Subindex)
	if len(stemNode.Values) == 0 {
		delete(o.stagedStems, key.Stem)
		o.stagedDeletes[key.Stem] = true
		return
	}
	o.stagedStems[key.Stem] = stemNode
	delete(o.stagedDeletes, key.Stem)
}

func (o *UBTOverlay) Get(key TreeKey) (value [32]byte, found bool, err error) {
	o.stagedMutex.Lock()
	defer o.stagedMutex.Unlock()

	if o.stagedDeletes[key.Stem] {
		return [32]byte{}, false, nil
	}
	if stemNode, ok := o.stagedStems[key.Stem]; ok {
		value, found = stemNode.Values[key.Subindex]
		if !found {
			return [32]byte{}, false, nil
		}
		return value, true, nil
	}
	stemNode, ok := o.base.GetStem(key.Stem)
	if !ok {
		return [32]byte{}, false, nil
	}
	value, found = stemNode.Values[key.Subindex]
	if !found {
		return [32]byte{}, false, nil
	}
	return value, true, nil
}

// OverlayRoot computes the root including staged changes without committing.
func (o *UBTOverlay) OverlayRoot() ([32]byte, error) {
	stagedStems, stagedDeletes := o.snapshot()
	tree := NewUnifiedBinaryTree(Config{Hasher: o.hasher})

	iter := o.base.IterByPrefix(Stem{}, 0)
	for {
		stem, node, ok := iter.Next()
		if !ok {
			break
		}
		if stagedDeletes[stem] {
			continue
		}
		if _, ok := stagedStems[stem]; ok {
			continue
		}
		for idx, val := range node.Values {
			tree.Insert(TreeKey{Stem: stem, Subindex: idx}, val)
		}
	}
	if err := iter.Close(); err != nil {
		return [32]byte{}, err
	}

	for stem, node := range stagedStems {
		if stagedDeletes[stem] {
			continue
		}
		for idx, val := range node.Values {
			tree.Insert(TreeKey{Stem: stem, Subindex: idx}, val)
		}
	}

	return tree.RootHash(), nil
}

// Flush commits staged changes to the base store and returns the new root.
func (o *UBTOverlay) Flush() ([32]byte, error) {
	stagedStems, stagedDeletes := o.snapshotAndClear()

	for stem := range stagedDeletes {
		if err := o.base.DeleteStem(stem); err != nil {
			return [32]byte{}, err
		}
	}
	for stem, node := range stagedStems {
		if err := o.base.PutStem(stem, node); err != nil {
			return [32]byte{}, err
		}
	}

	return o.rootFromStore()
}

func (o *UBTOverlay) snapshot() (map[Stem]*StemNode, map[Stem]bool) {
	o.stagedMutex.Lock()
	defer o.stagedMutex.Unlock()

	stagedStems := make(map[Stem]*StemNode, len(o.stagedStems))
	for stem, node := range o.stagedStems {
		stagedStems[stem] = cloneStemNode(node)
	}
	stagedDeletes := make(map[Stem]bool, len(o.stagedDeletes))
	for stem := range o.stagedDeletes {
		stagedDeletes[stem] = true
	}
	return stagedStems, stagedDeletes
}

func (o *UBTOverlay) snapshotAndClear() (map[Stem]*StemNode, map[Stem]bool) {
	o.stagedMutex.Lock()
	defer o.stagedMutex.Unlock()

	stagedStems := make(map[Stem]*StemNode, len(o.stagedStems))
	for stem, node := range o.stagedStems {
		stagedStems[stem] = cloneStemNode(node)
	}
	stagedDeletes := make(map[Stem]bool, len(o.stagedDeletes))
	for stem := range o.stagedDeletes {
		stagedDeletes[stem] = true
	}

	o.stagedStems = make(map[Stem]*StemNode)
	o.stagedDeletes = make(map[Stem]bool)

	return stagedStems, stagedDeletes
}

func (o *UBTOverlay) rootFromStore() ([32]byte, error) {
	tree := NewUnifiedBinaryTree(Config{Hasher: o.hasher})
	iter := o.base.IterByPrefix(Stem{}, 0)
	for {
		stem, node, ok := iter.Next()
		if !ok {
			break
		}
		for idx, val := range node.Values {
			tree.Insert(TreeKey{Stem: stem, Subindex: idx}, val)
		}
	}
	if err := iter.Close(); err != nil {
		return [32]byte{}, err
	}
	return tree.RootHash(), nil
}

func (s *MemoryUBTStore) hashStemTrie(stems []Stem, depth int) [32]byte {
	if len(stems) == 0 {
		return [32]byte{}
	}
	if depth >= 248 {
		node, ok := s.stems[stems[0]]
		if !ok {
			return [32]byte{}
		}
		return node.Hash(s.hasher)
	}
	left := make([]Stem, 0, len(stems))
	right := make([]Stem, 0, len(stems))
	for _, stem := range stems {
		if stem.Bit(depth) == 0 {
			left = append(left, stem)
		} else {
			right = append(right, stem)
		}
	}
	leftHash := s.hashStemTrie(left, depth+1)
	rightHash := s.hashStemTrie(right, depth+1)
	return combineTrieHashes(s.hasher, leftHash, rightHash)
}

type sliceStemIterator struct {
	stems []Stem
	store *MemoryUBTStore
	idx   int
}

func (it *sliceStemIterator) Next() (stem Stem, node *StemNode, ok bool) {
	if it.idx >= len(it.stems) {
		return Stem{}, nil, false
	}
	stem = it.stems[it.idx]
	it.idx++
	node = it.store.stems[stem]
	if node == nil {
		return Stem{}, nil, false
	}
	return stem, cloneStemNode(node), true
}

func (it *sliceStemIterator) Close() error {
	return nil
}

func cloneStemNode(node *StemNode) *StemNode {
	if node == nil {
		return nil
	}
	clone := NewStemNode(node.Stem)
	for idx, val := range node.Values {
		clone.Values[idx] = val
	}
	return clone
}

func stemHasPrefix(stem Stem, prefix Stem, prefixBits int) bool {
	if prefixBits < 0 {
		return false
	}
	if prefixBits == 0 {
		return true
	}
	if prefixBits > 248 {
		return false
	}
	for i := 0; i < prefixBits; i++ {
		if stem.Bit(i) != prefix.Bit(i) {
			return false
		}
	}
	return true
}

func stemLess(a, b Stem) bool {
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return false
}
