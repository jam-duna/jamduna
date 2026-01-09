package storage

import (
	"sort"
	"sync"
)

const defaultParallelThreshold = 128

// Config controls tree construction.
type Config struct {
	Profile           Profile
	Hasher            Hasher
	KeyDeriver        KeyDeriver
	Capacity          int
	Incremental       bool
	ParallelThreshold int
}

// KeyValue is a typed batch entry.
type KeyValue struct {
	Key   TreeKey
	Value [32]byte
}

// UnifiedBinaryTree is an in-memory UBT keyed by 32-byte TreeKeys.
// This version supports optional incremental hashing and prefix cache persistence.
type UnifiedBinaryTree struct {
	root          NodeRef
	hasher        Hasher
	keyDeriver    KeyDeriver
	stems         map[Stem]NodeRef // stem -> node ref into stemNodes
	stemNodes     []StemNode
	internalNodes []InternalNode

	incrementalEnabled bool
	stemHashCache      map[Stem][32]byte
	nodeHashCache      map[NodeCacheKey][32]byte
	dirtyStemHashes    map[Stem]bool
	rootDirty          bool
	rootHashCached     [32]byte

	hashStore         HashCacheStore
	parallelThreshold int
}

func NewUnifiedBinaryTree(cfg Config) *UnifiedBinaryTree {
	if cfg.Hasher == nil {
		cfg.Hasher = NewBlake3Hasher(cfg.Profile)
	}
	if cfg.KeyDeriver == nil {
		cfg.KeyDeriver = defaultKeyDeriver(cfg.Profile)
	}
	stemCap := cfg.Capacity
	if stemCap < 0 {
		stemCap = 0
	}
	tree := &UnifiedBinaryTree{
		root:              EmptyNodeRef,
		hasher:            cfg.Hasher,
		keyDeriver:        cfg.KeyDeriver,
		stems:             make(map[Stem]NodeRef, stemCap),
		stemNodes:         make([]StemNode, 0, stemCap),
		parallelThreshold: defaultParallelThreshold,
	}
	if cfg.ParallelThreshold > 0 {
		tree.parallelThreshold = cfg.ParallelThreshold
	}
	if cfg.Incremental {
		tree.EnableIncrementalMode()
	}
	return tree
}

func (t *UnifiedBinaryTree) SetHashCacheStore(store HashCacheStore) {
	t.hashStore = store
}

func (t *UnifiedBinaryTree) SetParallelThreshold(n int) {
	t.parallelThreshold = n
}

func (t *UnifiedBinaryTree) EnableIncrementalMode() {
	if t.incrementalEnabled {
		return
	}
	t.incrementalEnabled = true
	if t.stemHashCache == nil {
		t.stemHashCache = make(map[Stem][32]byte)
	}
	if t.nodeHashCache == nil {
		t.nodeHashCache = make(map[NodeCacheKey][32]byte)
	}
	if t.dirtyStemHashes == nil {
		t.dirtyStemHashes = make(map[Stem]bool)
	}
	t.rootDirty = true
}

func (t *UnifiedBinaryTree) Insert(key TreeKey, value [32]byte) {
	if value == ([32]byte{}) {
		t.Delete(&key)
		return
	}
	stemRef, ok := t.stems[key.Stem]
	if !ok {
		node := NewStemNode(key.Stem)
		t.stemNodes = append(t.stemNodes, *node)
		stemRef = NodeRef{Kind: NodeStem, Index: uint32(len(t.stemNodes) - 1)}
		t.stems[key.Stem] = stemRef
	}

	stemNode := &t.stemNodes[int(stemRef.Index)]
	stemNode.SetValue(key.Subindex, value)
	if t.incrementalEnabled {
		t.markDirty(key.Stem)
	}
}

// InsertRaw inserts a value even if it is zero.
// This is used for witness-only trees that need to model explicit zero values.
func (t *UnifiedBinaryTree) InsertRaw(key TreeKey, value [32]byte) {
	stemRef, ok := t.stems[key.Stem]
	if !ok {
		node := NewStemNode(key.Stem)
		t.stemNodes = append(t.stemNodes, *node)
		stemRef = NodeRef{Kind: NodeStem, Index: uint32(len(t.stemNodes) - 1)}
		t.stems[key.Stem] = stemRef
	}

	stemNode := &t.stemNodes[int(stemRef.Index)]
	stemNode.SetValueRaw(key.Subindex, value)
	if t.incrementalEnabled {
		t.markDirty(key.Stem)
	}
}

func (t *UnifiedBinaryTree) Get(key *TreeKey) (value [32]byte, found bool, err error) {
	stemRef, ok := t.stems[key.Stem]
	if !ok {
		return [32]byte{}, false, nil
	}
	stemNode := &t.stemNodes[int(stemRef.Index)]
	value, found = stemNode.Values[key.Subindex]
	if !found {
		return [32]byte{}, false, nil
	}
	return value, true, nil
}

func (t *UnifiedBinaryTree) Delete(key *TreeKey) {
	stemRef, ok := t.stems[key.Stem]
	if !ok {
		return
	}
	stemNode := &t.stemNodes[int(stemRef.Index)]
	stemNode.DeleteValue(key.Subindex)
	if len(stemNode.Values) == 0 {
		delete(t.stems, key.Stem)
		if t.stemHashCache != nil {
			delete(t.stemHashCache, key.Stem)
		}
	}
	if t.incrementalEnabled {
		t.markDirty(key.Stem)
	}
}

func (t *UnifiedBinaryTree) InsertBatch(entries []KeyValue) {
	for _, entry := range entries {
		t.Insert(entry.Key, entry.Value)
	}
}

func (t *UnifiedBinaryTree) Len() int {
	total := 0
	for _, stemRef := range t.stems {
		stemNode := &t.stemNodes[int(stemRef.Index)]
		total += len(stemNode.Values)
	}
	return total
}

func (t *UnifiedBinaryTree) StemCount() int {
	return len(t.stems)
}

func (t *UnifiedBinaryTree) IsEmpty() bool {
	return len(t.stems) == 0
}

func (t *UnifiedBinaryTree) ContainsKey(key *TreeKey) bool {
	_, found, _ := t.Get(key)
	return found
}

func (t *UnifiedBinaryTree) Iter() []KeyValue {
	stems := t.sortedStems()
	entries := make([]KeyValue, 0, t.Len())
	for _, stem := range stems {
		stemNode, ok := t.getStemNode(stem)
		if !ok {
			continue
		}
		for _, idx := range stemNode.SortedSubindices() {
			value := stemNode.Values[idx]
			entries = append(entries, KeyValue{
				Key:   TreeKey{Stem: stem, Subindex: idx},
				Value: value,
			})
		}
	}
	return entries
}

func (t *UnifiedBinaryTree) GenerateProof(key *TreeKey) (*Proof, error) {
	if key == nil {
		return nil, ErrInvalidProof
	}
	return GenerateStemProof(t, *key)
}

func (t *UnifiedBinaryTree) GenerateMultiProof(keys []TreeKey) (*MultiProof, error) {
	return GenerateMultiProof(t, keys)
}

func (t *UnifiedBinaryTree) RootHash() [32]byte {
	if len(t.stems) == 0 {
		return [32]byte{}
	}
	if t.incrementalEnabled && !t.rootDirty {
		return t.rootHashCached
	}

	stems := t.sortedStems()
	t.computeStemHashes(stems)
	root := t.hashStemTrie(stems, 0, Stem{})

	if t.incrementalEnabled {
		t.rootHashCached = root
		t.rootDirty = false
	}
	return root
}

// Copy returns a new tree with identical key/value contents.
// Hash caches are rebuilt lazily in the clone.
func (t *UnifiedBinaryTree) Copy() *UnifiedBinaryTree {
	clone := NewUnifiedBinaryTree(Config{
		Hasher:            t.hasher,
		KeyDeriver:        t.keyDeriver,
		Incremental:       t.incrementalEnabled,
		ParallelThreshold: t.parallelThreshold,
	})

	for _, entry := range t.Iter() {
		clone.InsertRaw(entry.Key, entry.Value)
	}

	return clone
}

func (t *UnifiedBinaryTree) sortedStems() []Stem {
	stems := make([]Stem, 0, len(t.stems))
	for stem := range t.stems {
		stems = append(stems, stem)
	}
	sort.Slice(stems, func(i, j int) bool {
		return stemLess(stems[i], stems[j])
	})
	return stems
}

func (t *UnifiedBinaryTree) computeStemHashes(stems []Stem) {
	var targets []Stem
	if t.incrementalEnabled && len(t.dirtyStemHashes) > 0 {
		targets = make([]Stem, 0, len(t.dirtyStemHashes))
		for stem := range t.dirtyStemHashes {
			if _, ok := t.stems[stem]; ok {
				targets = append(targets, stem)
			} else if t.stemHashCache != nil {
				delete(t.stemHashCache, stem)
			}
		}
	} else {
		targets = stems
	}

	if len(targets) == 0 {
		return
	}

	if t.parallelThreshold > 0 && len(targets) >= t.parallelThreshold {
		t.computeStemHashesParallel(targets)
	} else {
		for _, stem := range targets {
			t.computeStemHash(stem)
		}
	}

	if t.incrementalEnabled {
		for stem := range t.dirtyStemHashes {
			delete(t.dirtyStemHashes, stem)
		}
	}
}

func (t *UnifiedBinaryTree) computeStemHashesParallel(stems []Stem) {
	type result struct {
		stem Stem
		hash [32]byte
	}

	results := make(chan result, len(stems))
	var wg sync.WaitGroup

	for _, stem := range stems {
		stem := stem
		wg.Add(1)
		go func() {
			defer wg.Done()
			hash, ok := t.computeStemHashNoCache(stem)
			if !ok {
				return
			}
			results <- result{stem: stem, hash: hash}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for res := range results {
		t.putStemHash(res.stem, res.hash)
	}
}

func (t *UnifiedBinaryTree) computeStemHash(stem Stem) [32]byte {
	hash, ok := t.computeStemHashNoCache(stem)
	if !ok {
		return [32]byte{}
	}
	t.putStemHash(stem, hash)
	return hash
}

func (t *UnifiedBinaryTree) computeStemHashNoCache(stem Stem) ([32]byte, bool) {
	stemNode, ok := t.getStemNode(stem)
	if !ok {
		return [32]byte{}, false
	}
	return stemNode.Hash(t.hasher), true
}

func (t *UnifiedBinaryTree) getStemNode(stem Stem) (*StemNode, bool) {
	stemRef, ok := t.stems[stem]
	if !ok {
		return nil, false
	}
	return &t.stemNodes[int(stemRef.Index)], true
}

func (t *UnifiedBinaryTree) hashStemTrie(stems []Stem, depthBits uint16, prefix Stem) [32]byte {
	prefix = CanonicalPrefix(prefix, depthBits)
	if t.incrementalEnabled {
		if cached, ok := t.getNodeHash(depthBits, prefix); ok {
			return cached
		}
	}

	if len(stems) == 0 {
		return [32]byte{}
	}
	if depthBits >= 248 {
		hash, ok := t.getStemHash(stems[0])
		if !ok {
			hash = t.computeStemHash(stems[0])
		}
		if t.incrementalEnabled {
			t.putNodeHash(depthBits, prefix, hash)
		}
		return hash
	}

	left := make([]Stem, 0, len(stems))
	right := make([]Stem, 0, len(stems))
	for _, stem := range stems {
		if stem.Bit(int(depthBits)) == 0 {
			left = append(left, stem)
		} else {
			right = append(right, stem)
		}
	}

	leftPrefix := prefix
	rightPrefix := prefix
	byteIndex := int(depthBits / 8)
	bitIndex := uint8(7 - (depthBits % 8))
	leftPrefix[byteIndex] &^= 1 << bitIndex
	rightPrefix[byteIndex] |= 1 << bitIndex

	leftHash := t.hashStemTrie(left, depthBits+1, leftPrefix)
	rightHash := t.hashStemTrie(right, depthBits+1, rightPrefix)
	hash := combineTrieHashes(t.hasher, leftHash, rightHash)
	if t.incrementalEnabled {
		t.putNodeHash(depthBits, prefix, hash)
	}
	return hash
}

func (t *UnifiedBinaryTree) markDirty(stem Stem) {
	if t.dirtyStemHashes == nil {
		t.dirtyStemHashes = make(map[Stem]bool)
	}
	t.dirtyStemHashes[stem] = true
	if t.stemHashCache != nil {
		delete(t.stemHashCache, stem)
	}
	t.invalidatePath(stem)
	t.rootDirty = true
}

func (t *UnifiedBinaryTree) invalidatePath(stem Stem) {
	if t.nodeHashCache == nil {
		return
	}
	for depth := uint16(0); depth < 248; depth++ {
		prefix := CanonicalPrefix(stem, depth)
		delete(t.nodeHashCache, NodeCacheKey{DepthBits: depth, PathPrefix: prefix})
	}
}

func (t *UnifiedBinaryTree) getStemHash(stem Stem) (hash [32]byte, ok bool) {
	if t.stemHashCache != nil {
		hash, ok = t.stemHashCache[stem]
		if ok {
			return hash, true
		}
	}
	if t.hashStore != nil {
		return t.hashStore.GetStemHash(stem)
	}
	return [32]byte{}, false
}

func (t *UnifiedBinaryTree) putStemHash(stem Stem, hash [32]byte) {
	if t.stemHashCache != nil {
		t.stemHashCache[stem] = hash
	}
	if t.hashStore != nil {
		_ = t.hashStore.PutStemHash(stem, hash)
	}
}

func (t *UnifiedBinaryTree) getNodeHash(depthBits uint16, prefix Stem) (hash [32]byte, ok bool) {
	if t.nodeHashCache != nil {
		hash, ok = t.nodeHashCache[NodeCacheKey{DepthBits: depthBits, PathPrefix: prefix}]
		if ok {
			return hash, true
		}
	}
	if t.hashStore != nil {
		return t.hashStore.GetNodeHash(depthBits, prefix)
	}
	return [32]byte{}, false
}

func (t *UnifiedBinaryTree) putNodeHash(depthBits uint16, prefix Stem, hash [32]byte) {
	if t.nodeHashCache != nil {
		t.nodeHashCache[NodeCacheKey{DepthBits: depthBits, PathPrefix: prefix}] = hash
	}
	if t.hashStore != nil {
		_ = t.hashStore.PutNodeHash(depthBits, prefix, hash)
	}
}

func combineTrieHashes(hasher Hasher, left, right [32]byte) [32]byte {
	if left == ([32]byte{}) {
		return right
	}
	if right == ([32]byte{}) {
		return left
	}
	return hasher.Hash64(&left, &right)
}

// CanonicalPrefix zeroes bits beyond depthBits.
func CanonicalPrefix(prefix Stem, depthBits uint16) Stem {
	if depthBits >= 248 {
		return prefix
	}
	byteIndex := int(depthBits / 8)
	rem := int(depthBits % 8)

	if rem == 0 {
		for i := byteIndex; i < len(prefix); i++ {
			prefix[i] = 0
		}
		return prefix
	}

	if byteIndex < len(prefix) {
		mask := byte(0xFF << (8 - rem))
		prefix[byteIndex] &= mask
		for i := byteIndex + 1; i < len(prefix); i++ {
			prefix[i] = 0
		}
	}
	return prefix
}
