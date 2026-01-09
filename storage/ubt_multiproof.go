package storage

import "sort"

// CompactTreeKey references a deduplicated stem.
type CompactTreeKey struct {
	StemIndex uint32
	Subindex  uint8
}

// MultiProof aggregates k keys with shared siblings.
type MultiProof struct {
	Keys     []CompactTreeKey
	Values   []*[32]byte
	Nodes    [][32]byte
	NodeRefs []uint32
	Stems    []Stem
	// MissingKeys records keys whose stems do not exist in the tree.
	// These require separate extension proofs and are not part of the multiproof.
	MissingKeys []TreeKey
}

func (mp *MultiProof) ExpandKey(i int) TreeKey {
	if i < 0 || i >= len(mp.Keys) {
		return TreeKey{}
	}
	stemIdx := int(mp.Keys[i].StemIndex)
	if stemIdx < 0 || stemIdx >= len(mp.Stems) {
		return TreeKey{}
	}
	return TreeKey{Stem: mp.Stems[stemIdx], Subindex: mp.Keys[i].Subindex}
}

// GenerateMultiProof builds a compressed witness for k keys in O(k log k).
// Keys whose stems are missing are recorded in MissingKeys and excluded from the proof.
func GenerateMultiProof(tree *UnifiedBinaryTree, keys []TreeKey) (*MultiProof, error) {
	if len(keys) == 0 {
		return &MultiProof{}, nil
	}

	sortedKeys := uniqueSortedKeys(keys)
	stems := make([]Stem, 0, len(sortedKeys))
	stemIndex := make(map[Stem]uint32)
	stemGroups := make(map[Stem][]uint8)
	values := make([]*[32]byte, 0, len(sortedKeys))
	compact := make([]CompactTreeKey, 0, len(sortedKeys))
	missingKeys := make([]TreeKey, 0)

	for _, key := range sortedKeys {
		stemNode, ok := tree.getStemNode(key.Stem)
		if !ok {
			missingKeys = append(missingKeys, key)
			continue
		}
		idx, ok := stemIndex[key.Stem]
		if !ok {
			idx = uint32(len(stems))
			stemIndex[key.Stem] = idx
			stems = append(stems, key.Stem)
		}
		stemGroups[key.Stem] = append(stemGroups[key.Stem], key.Subindex)

		if value, found := stemNode.Values[key.Subindex]; found {
			val := value
			values = append(values, &val)
		} else {
			values = append(values, nil)
		}
		compact = append(compact, CompactTreeKey{StemIndex: idx, Subindex: key.Subindex})
	}

	for stem := range stemGroups {
		subs := stemGroups[stem]
		sort.Slice(subs, func(i, j int) bool { return subs[i] < subs[j] })
		stemGroups[stem] = subs
	}

	allStems := tree.sortedStems()
	nodes := make([][32]byte, 0)
	collectMultiProofNodes(tree, allStems, stems, stemGroups, 0, Stem{}, &nodes)
	uniqueNodes, nodeRefs := deduplicateNodes(nodes)

	return &MultiProof{
		Keys:     compact,
		Values:   values,
		Nodes:    uniqueNodes,
		NodeRefs: nodeRefs,
		Stems:    stems,
		MissingKeys: missingKeys,
	}, nil
}

// VerifyMultiProof reconstructs the root using the multiproof nodes.
// MissingKeys must be empty; missing stems require extension proofs.
func VerifyMultiProof(mp *MultiProof, hasher Hasher, expectedRoot [32]byte) error {
	if len(mp.Keys) != len(mp.Values) {
		return ErrInvalidProof
	}
	if !stemsSorted(mp.Stems) {
		return ErrInvalidProof
	}
	if len(mp.MissingKeys) != 0 {
		return ErrInvalidProof
	}
	if err := validateMultiProofCanonical(mp); err != nil {
		return err
	}

	valueMap := make(map[Stem]map[uint8]*[32]byte)
	for i, key := range mp.Keys {
		stem := mp.Stems[key.StemIndex]
		m, ok := valueMap[stem]
		if !ok {
			m = make(map[uint8]*[32]byte)
			valueMap[stem] = m
		}
		m[key.Subindex] = mp.Values[i]
	}

	stems := make([]Stem, 0, len(valueMap))
	for stem := range valueMap {
		stems = append(stems, stem)
	}
	sort.Slice(stems, func(i, j int) bool { return stemLess(stems[i], stems[j]) })

	stream := nodeStream{nodes: mp.Nodes, refs: mp.NodeRefs}
	root, err := rebuildStemTrie(hasher, stems, valueMap, 0, Stem{}, &stream)
	if err != nil {
		return err
	}
	if stream.idx != stream.expectedCount() {
		return ErrInvalidProof
	}
	if root != expectedRoot {
		return ErrInvalidProof
	}
	return nil
}

func collectMultiProofNodes(tree *UnifiedBinaryTree, allStems []Stem, stems []Stem, groups map[Stem][]uint8, depth uint16, prefix Stem, nodes *[][32]byte) {
	if len(stems) == 0 {
		return
	}
	if depth >= 248 {
		if len(stems) != 1 {
			return
		}
		stem := stems[0]
		stemNode, ok := tree.getStemNode(stem)
		if !ok {
			return
		}
		subs := groups[stem]
		collectStemSubtreeNodes(stemNode, subs, 0, 0, tree.hasher, nodes)
		return
	}

	left := make([]Stem, 0, len(stems))
	right := make([]Stem, 0, len(stems))
	for _, stem := range stems {
		if stem.Bit(int(depth)) == 0 {
			left = append(left, stem)
		} else {
			right = append(right, stem)
		}
	}

	leftPrefix := prefix
	rightPrefix := prefix
	byteIndex := int(depth / 8)
	bitIndex := uint8(7 - (depth % 8))
	leftPrefix[byteIndex] &^= 1 << bitIndex
	rightPrefix[byteIndex] |= 1 << bitIndex

	if len(left) > 0 && len(right) > 0 {
		collectMultiProofNodes(tree, allStems, left, groups, depth+1, leftPrefix, nodes)
		collectMultiProofNodes(tree, allStems, right, groups, depth+1, rightPrefix, nodes)
		return
	}

	if len(left) > 0 {
		collectMultiProofNodes(tree, allStems, left, groups, depth+1, leftPrefix, nodes)
		sibling := subtreeHashForPrefix(tree, allStems, depth+1, rightPrefix)
		*nodes = append(*nodes, sibling)
		return
	}

	sibling := subtreeHashForPrefix(tree, allStems, depth+1, leftPrefix)
	*nodes = append(*nodes, sibling)
	collectMultiProofNodes(tree, allStems, right, groups, depth+1, rightPrefix, nodes)
}

func collectStemSubtreeNodes(node *StemNode, subindices []uint8, depth int, prefix uint8, hasher Hasher, nodes *[][32]byte) {
	if len(subindices) == 0 {
		return
	}
	if depth >= 8 {
		return
	}

	left := make([]uint8, 0, len(subindices))
	right := make([]uint8, 0, len(subindices))
	bitPos := 7 - depth
	for _, idx := range subindices {
		if (idx>>bitPos)&1 == 0 {
			left = append(left, idx)
		} else {
			right = append(right, idx)
		}
	}

	leftPrefix := prefix &^ (1 << bitPos)
	rightPrefix := prefix | (1 << bitPos)

	if len(left) > 0 && len(right) > 0 {
		collectStemSubtreeNodes(node, left, depth+1, leftPrefix, hasher, nodes)
		collectStemSubtreeNodes(node, right, depth+1, rightPrefix, hasher, nodes)
		return
	}

	if len(left) > 0 {
		collectStemSubtreeNodes(node, left, depth+1, leftPrefix, hasher, nodes)
		sibling := stemSubtreeHashForPrefix(node, hasher, rightPrefix, depth+1)
		*nodes = append(*nodes, sibling)
		return
	}

	sibling := stemSubtreeHashForPrefix(node, hasher, leftPrefix, depth+1)
	*nodes = append(*nodes, sibling)
	collectStemSubtreeNodes(node, right, depth+1, rightPrefix, hasher, nodes)
}

func subtreeHashForPrefix(tree *UnifiedBinaryTree, stems []Stem, depthBits uint16, prefix Stem) [32]byte {
	filtered := filterStemsByPrefix(stems, prefix, int(depthBits))
	if len(filtered) == 0 {
		return [32]byte{}
	}
	return tree.hashStemTrie(filtered, depthBits, prefix)
}

func rebuildStemTrie(hasher Hasher, stems []Stem, valueMap map[Stem]map[uint8]*[32]byte, depth uint16, prefix Stem, stream *nodeStream) ([32]byte, error) {
	if len(stems) == 0 {
		return [32]byte{}, nil
	}
	if depth >= 248 {
		if len(stems) != 1 {
			return [32]byte{}, ErrInvalidProof
		}
		stem := stems[0]
		values := valueMap[stem]
		root, err := rebuildStemSubtree(hasher, values, 0, 0, stream)
		if err != nil {
			return [32]byte{}, err
		}
		return hasher.HashStemNode(&stem, &root), nil
	}

	left := make([]Stem, 0, len(stems))
	right := make([]Stem, 0, len(stems))
	for _, stem := range stems {
		if stem.Bit(int(depth)) == 0 {
			left = append(left, stem)
		} else {
			right = append(right, stem)
		}
	}

	leftPrefix := prefix
	rightPrefix := prefix
	byteIndex := int(depth / 8)
	bitIndex := uint8(7 - (depth % 8))
	leftPrefix[byteIndex] &^= 1 << bitIndex
	rightPrefix[byteIndex] |= 1 << bitIndex

	if len(left) > 0 && len(right) > 0 {
		leftHash, err := rebuildStemTrie(hasher, left, valueMap, depth+1, leftPrefix, stream)
		if err != nil {
			return [32]byte{}, err
		}
		rightHash, err := rebuildStemTrie(hasher, right, valueMap, depth+1, rightPrefix, stream)
		if err != nil {
			return [32]byte{}, err
		}
		return combineTrieHashes(hasher, leftHash, rightHash), nil
	}

	if len(left) > 0 {
		leftHash, err := rebuildStemTrie(hasher, left, valueMap, depth+1, leftPrefix, stream)
		if err != nil {
			return [32]byte{}, err
		}
		sibling, err := stream.consume()
		if err != nil {
			return [32]byte{}, err
		}
		return combineTrieHashes(hasher, leftHash, sibling), nil
	}

	sibling, err := stream.consume()
	if err != nil {
		return [32]byte{}, err
	}
	rightHash, err := rebuildStemTrie(hasher, right, valueMap, depth+1, rightPrefix, stream)
	if err != nil {
		return [32]byte{}, err
	}
	return combineTrieHashes(hasher, sibling, rightHash), nil
}

func rebuildStemSubtree(hasher Hasher, values map[uint8]*[32]byte, depth int, prefix uint8, stream *nodeStream) ([32]byte, error) {
	if depth >= 8 {
		val, ok := values[prefix]
		if !ok || val == nil {
			return [32]byte{}, nil
		}
		h := hasher.Hash32(val)
		return h, nil
	}

	left := make([]uint8, 0)
	right := make([]uint8, 0)
	bitPos := 7 - depth
	for key := range values {
		if (key>>bitPos)&1 == 0 {
			left = append(left, key)
		} else {
			right = append(right, key)
		}
	}

	leftPrefix := prefix &^ (1 << bitPos)
	rightPrefix := prefix | (1 << bitPos)

	if len(left) > 0 && len(right) > 0 {
		leftMap := filterValueMap(values, left)
		leftHash, err := rebuildStemSubtree(hasher, leftMap, depth+1, leftPrefix, stream)
		if err != nil {
			return [32]byte{}, err
		}
		rightMap := filterValueMap(values, right)
		rightHash, err := rebuildStemSubtree(hasher, rightMap, depth+1, rightPrefix, stream)
		if err != nil {
			return [32]byte{}, err
		}
		return hasher.Hash64(&leftHash, &rightHash), nil
	}

	if len(left) > 0 {
		leftMap := filterValueMap(values, left)
		leftHash, err := rebuildStemSubtree(hasher, leftMap, depth+1, leftPrefix, stream)
		if err != nil {
			return [32]byte{}, err
		}
		sibling, err := stream.consume()
		if err != nil {
			return [32]byte{}, err
		}
		return hasher.Hash64(&leftHash, &sibling), nil
	}

	sibling, err := stream.consume()
	if err != nil {
		return [32]byte{}, err
	}
	rightMap := filterValueMap(values, right)
	rightHash, err := rebuildStemSubtree(hasher, rightMap, depth+1, rightPrefix, stream)
	if err != nil {
		return [32]byte{}, err
	}
	return hasher.Hash64(&sibling, &rightHash), nil
}

func filterValueMap(values map[uint8]*[32]byte, keys []uint8) map[uint8]*[32]byte {
	out := make(map[uint8]*[32]byte, len(keys))
	for _, key := range keys {
		out[key] = values[key]
	}
	return out
}

type nodeStream struct {
	nodes [][32]byte
	refs  []uint32
	idx   int
}

func (s *nodeStream) consume() ([32]byte, error) {
	if len(s.refs) > 0 {
		if s.idx >= len(s.refs) {
			return [32]byte{}, ErrInvalidProof
		}
		ref := s.refs[s.idx]
		s.idx += 1
		if int(ref) >= len(s.nodes) {
			return [32]byte{}, ErrInvalidProof
		}
		return s.nodes[ref], nil
	}
	if s.idx >= len(s.nodes) {
		return [32]byte{}, ErrInvalidProof
	}
	node := s.nodes[s.idx]
	s.idx += 1
	return node, nil
}

func (s *nodeStream) expectedCount() int {
	if len(s.refs) > 0 {
		return len(s.refs)
	}
	return len(s.nodes)
}

func deduplicateNodes(nodes [][32]byte) ([][32]byte, []uint32) {
	if len(nodes) == 0 {
		return nil, nil
	}
	seen := make(map[[32]byte]uint32, len(nodes))
	unique := make([][32]byte, 0, len(nodes))
	refs := make([]uint32, 0, len(nodes))
	for _, node := range nodes {
		if idx, ok := seen[node]; ok {
			refs = append(refs, idx)
			continue
		}
		idx := uint32(len(unique))
		unique = append(unique, node)
		seen[node] = idx
		refs = append(refs, idx)
	}
	return unique, refs
}

func validateMultiProofCanonical(mp *MultiProof) error {
	if err := validateMultiProofKeys(mp); err != nil {
		return err
	}
	if len(mp.Keys) == 0 {
		if len(mp.Stems) != 0 {
			return ErrInvalidProof
		}
	} else {
		referenced := make([]bool, len(mp.Stems))
		for _, key := range mp.Keys {
			referenced[key.StemIndex] = true
		}
		for _, used := range referenced {
			if !used {
				return ErrInvalidProof
			}
		}
	}
	if len(mp.NodeRefs) > 0 {
		if err := validateNodeRefs(mp); err != nil {
			return err
		}
	} else if len(mp.Nodes) > 0 {
		seen := make(map[[32]byte]struct{}, len(mp.Nodes))
		for _, node := range mp.Nodes {
			if _, ok := seen[node]; ok {
				return ErrInvalidProof
			}
			seen[node] = struct{}{}
		}
	}
	return nil
}

func validateMultiProofKeys(mp *MultiProof) error {
	if len(mp.Keys) == 0 {
		return nil
	}

	var prev TreeKey
	hasPrev := false
	for _, key := range mp.Keys {
		if int(key.StemIndex) >= len(mp.Stems) {
			return ErrInvalidProof
		}
		stem := mp.Stems[key.StemIndex]
		current := TreeKey{Stem: stem, Subindex: key.Subindex}
		if hasPrev {
			if stemLess(current.Stem, prev.Stem) {
				return ErrInvalidProof
			}
			if stemLess(prev.Stem, current.Stem) {
				// ok
			} else if current.Subindex <= prev.Subindex {
				return ErrInvalidProof
			}
		}
		prev = current
		hasPrev = true
	}
	return nil
}

func validateNodeRefs(mp *MultiProof) error {
	if len(mp.Nodes) == 0 {
		if len(mp.NodeRefs) != 0 {
			return ErrInvalidProof
		}
		return nil
	}
	seen := make(map[[32]byte]struct{}, len(mp.Nodes))
	for _, node := range mp.Nodes {
		if _, ok := seen[node]; ok {
			return ErrInvalidProof
		}
		seen[node] = struct{}{}
	}
	referenced := make([]bool, len(mp.Nodes))
	for _, ref := range mp.NodeRefs {
		if int(ref) >= len(mp.Nodes) {
			return ErrInvalidProof
		}
		referenced[ref] = true
	}
	for _, used := range referenced {
		if !used {
			return ErrInvalidProof
		}
	}
	if len(mp.NodeRefs) > 0 {
		traversal := make([][32]byte, len(mp.NodeRefs))
		for i, ref := range mp.NodeRefs {
			traversal[i] = mp.Nodes[ref]
		}
		unique, refs := deduplicateNodes(traversal)
		if len(unique) != len(mp.Nodes) {
			return ErrInvalidProof
		}
		for i := range unique {
			if unique[i] != mp.Nodes[i] {
				return ErrInvalidProof
			}
		}
		if len(refs) != len(mp.NodeRefs) {
			return ErrInvalidProof
		}
		for i := range refs {
			if refs[i] != mp.NodeRefs[i] {
				return ErrInvalidProof
			}
		}
	}
	return nil
}
