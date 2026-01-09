package storage

import (
	"encoding/binary"
)

// WitnessValueKind selects which value column is verified.
type WitnessValueKind int

const (
	WitnessValuePre WitnessValueKind = iota
	WitnessValuePost
)

// UBTWitnessStats summarizes a verified witness section.
type UBTWitnessStats struct {
	Root           [32]byte
	EntryCount     int
	ProofKeys      int
	ProofNodes     int
	ProofNodeRefs  int
	ProofStems     int
	ExtensionCount int
}

type witnessEntry struct {
	key  [32]byte
	pre  [32]byte
	post [32]byte
}

// VerifyUBTWitnessSection validates a serialized witness section and returns stats.
func VerifyUBTWitnessSection(data []byte, expectedRoot *[32]byte, valueKind WitnessValueKind, hasher Hasher) (*UBTWitnessStats, error) {
	section, err := parseUBTWitnessSection(data)
	if err != nil {
		return nil, err
	}
	if expectedRoot != nil && section.root != *expectedRoot {
		return nil, ErrInvalidProof
	}
	if err := VerifyMultiProof(section.proof, hasher, section.root); err != nil {
		return nil, err
	}
	if err := verifyExtensionProofs(section.extensions, section.root, hasher); err != nil {
		return nil, err
	}
	if err := verifyEntriesAgainstProof(section, valueKind); err != nil {
		return nil, err
	}

	return &UBTWitnessStats{
		Root:           section.root,
		EntryCount:     len(section.entries),
		ProofKeys:      len(section.proof.Keys),
		ProofNodes:     len(section.proof.Nodes),
		ProofNodeRefs:  len(section.proof.NodeRefs),
		ProofStems:     len(section.proof.Stems),
		ExtensionCount: len(section.extensions),
	}, nil
}

type witnessSection struct {
	root       [32]byte
	entries    []witnessEntry
	proof      *MultiProof
	extensions []extensionProof
}

func parseUBTWitnessSection(data []byte) (*witnessSection, error) {
	if len(data) < 36 {
		return nil, ErrInvalidProof
	}
	var root [32]byte
	copy(root[:], data[:32])
	count := binary.BigEndian.Uint32(data[32:36])
	offset := 36

	entries := make([]witnessEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		if offset+161 > len(data) {
			return nil, ErrInvalidProof
		}
		var key [32]byte
		copy(key[:], data[offset:offset+32])
		offset += 32

		// Skip metadata: key_type(1) + address(20) + extra(8) + storageKey(32)
		offset += 1 + 20 + 8 + 32

		var pre [32]byte
		var post [32]byte
		copy(pre[:], data[offset:offset+32])
		offset += 32
		copy(post[:], data[offset:offset+32])
		offset += 32

		// Skip txIndex (4 bytes)
		offset += 4

		entries = append(entries, witnessEntry{
			key:  key,
			pre:  pre,
			post: post,
		})
	}

	if offset+4 > len(data) {
		return nil, ErrInvalidProof
	}
	proofLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4
	if offset+int(proofLen) > len(data) {
		return nil, ErrInvalidProof
	}

	proof, err := deserializeMultiProof(data[offset : offset+int(proofLen)])
	if err != nil {
		return nil, err
	}
	offset += int(proofLen)

	extensions, err := deserializeExtensionProofs(data[offset:])
	if err != nil {
		return nil, err
	}

	return &witnessSection{
		root:       root,
		entries:    entries,
		proof:      proof,
		extensions: extensions,
	}, nil
}

func deserializeMultiProof(data []byte) (*MultiProof, error) {
	if len(data) < 20 {
		return nil, ErrInvalidProof
	}
	offset := 0
	keysLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	valuesLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	nodesLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	refsLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	stemsLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	if keysLen != valuesLen {
		return nil, ErrInvalidProof
	}

	keys := make([]CompactTreeKey, 0, keysLen)
	for i := uint32(0); i < keysLen; i++ {
		if offset+4+1 > len(data) {
			return nil, ErrInvalidProof
		}
		stemIdx := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
		subindex := data[offset]
		offset++
		keys = append(keys, CompactTreeKey{
			StemIndex: stemIdx,
			Subindex:  subindex,
		})
	}

	values := make([]*[32]byte, 0, valuesLen)
	for i := uint32(0); i < valuesLen; i++ {
		if offset+1 > len(data) {
			return nil, ErrInvalidProof
		}
		present := data[offset]
		offset++
		if present == 0 {
			values = append(values, nil)
			continue
		}
		if offset+32 > len(data) {
			return nil, ErrInvalidProof
		}
		var val [32]byte
		copy(val[:], data[offset:offset+32])
		offset += 32
		values = append(values, &val)
	}

	nodes := make([][32]byte, 0, nodesLen)
	for i := uint32(0); i < nodesLen; i++ {
		if offset+32 > len(data) {
			return nil, ErrInvalidProof
		}
		var node [32]byte
		copy(node[:], data[offset:offset+32])
		offset += 32
		nodes = append(nodes, node)
	}

	nodeRefs := make([]uint32, 0, refsLen)
	for i := uint32(0); i < refsLen; i++ {
		if offset+4 > len(data) {
			return nil, ErrInvalidProof
		}
		ref := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4
		nodeRefs = append(nodeRefs, ref)
	}

	stems := make([]Stem, 0, stemsLen)
	for i := uint32(0); i < stemsLen; i++ {
		if offset+31 > len(data) {
			return nil, ErrInvalidProof
		}
		var stem Stem
		copy(stem[:], data[offset:offset+31])
		offset += 31
		stems = append(stems, stem)
	}

	if offset != len(data) {
		return nil, ErrInvalidProof
	}

	return &MultiProof{
		Keys:     keys,
		Values:   values,
		Nodes:    nodes,
		NodeRefs: nodeRefs,
		Stems:    stems,
	}, nil
}

func deserializeExtensionProofs(data []byte) ([]extensionProof, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) < 4 {
		return nil, ErrInvalidProof
	}
	offset := 0
	count := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	exts := make([]extensionProof, 0, count)
	for i := uint32(0); i < count; i++ {
		if offset+31+31+32+2 > len(data) {
			return nil, ErrInvalidProof
		}
		var missingStem Stem
		copy(missingStem[:], data[offset:offset+31])
		offset += 31
		var stem Stem
		copy(stem[:], data[offset:offset+31])
		offset += 31
		var stemHash [32]byte
		copy(stemHash[:], data[offset:offset+32])
		offset += 32
		if offset+2 > len(data) {
			return nil, ErrInvalidProof
		}
		divergence := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		// Sparse encoding: read bitmap (31 bytes) + non-zero siblings
		if offset+31 > len(data) {
			return nil, ErrInvalidProof
		}
		var bitmap [31]byte
		copy(bitmap[:], data[offset:offset+31])
		offset += 31

		// Reconstruct 248 siblings from bitmap
		siblings := make([][32]byte, 248)
		for i := 0; i < 248; i++ {
			bitSet := (bitmap[i/8] & (1 << (7 - i%8))) != 0
			if bitSet {
				if offset+32 > len(data) {
					return nil, ErrInvalidProof
				}
				copy(siblings[i][:], data[offset:offset+32])
				offset += 32
			}
			// else sibling remains zero (default)
		}

		exts = append(exts, extensionProof{
			missingStem:     missingStem,
			stem:            stem,
			stemHash:        stemHash,
			divergenceDepth: divergence,
			siblings:        siblings,
		})
	}

	if offset != len(data) {
		return nil, ErrInvalidProof
	}
	return exts, nil
}

func verifyExtensionProofs(exts []extensionProof, expectedRoot [32]byte, hasher Hasher) error {
	seen := make(map[Stem]struct{}, len(exts))
	for _, ext := range exts {
		if _, ok := seen[ext.missingStem]; ok {
			return ErrInvalidProof
		}
		seen[ext.missingStem] = struct{}{}

		if ext.divergenceDepth >= 248 {
			return ErrInvalidProof
		}
		if len(ext.siblings) != 248 {
			return ErrInvalidProof
		}

		node := ExtensionProofNode{
			Stem:            ext.stem,
			StemHash:        ext.stemHash,
			DivergenceDepth: ext.divergenceDepth,
		}
		if err := verifyExtensionPrefix(ext.missingStem, &node); err != nil {
			return err
		}

		current := ext.stemHash
		depth := uint16(247)
		for _, sibling := range ext.siblings {
			if ext.stem.Bit(int(depth)) == 0 {
				current = combineTrieHashes(hasher, current, sibling)
			} else {
				current = combineTrieHashes(hasher, sibling, current)
			}
			if depth == 0 {
				break
			}
			depth--
		}
		if current != expectedRoot {
			return ErrInvalidProof
		}
	}
	return nil
}

func verifyEntriesAgainstProof(section *witnessSection, valueKind WitnessValueKind) error {
	proofValues := make(map[[32]byte]*[32]byte, len(section.proof.Keys))
	for i := range section.proof.Keys {
		key := section.proof.ExpandKey(i).ToBytes()
		proofValues[key] = section.proof.Values[i]
	}
	extensionMap := make(map[Stem]extensionProof, len(section.extensions))
	for _, ext := range section.extensions {
		extensionMap[ext.missingStem] = ext
	}

	seenProof := make(map[[32]byte]struct{}, len(proofValues))
	seenExtension := make(map[Stem]struct{}, len(extensionMap))

	for _, entry := range section.entries {
		entryValue := entry.pre
		if valueKind == WitnessValuePost {
			entryValue = entry.post
		}

		if proofValue, ok := proofValues[entry.key]; ok {
			if proofValue == nil {
				if entryValue != ([32]byte{}) {
					return ErrInvalidProof
				}
			} else if entryValue != *proofValue {
				return ErrInvalidProof
			}
			seenProof[entry.key] = struct{}{}
		} else if _, ok := extensionMap[TreeKeyFromBytes(entry.key).Stem]; ok {
			if entryValue != ([32]byte{}) {
				return ErrInvalidProof
			}
			seenExtension[TreeKeyFromBytes(entry.key).Stem] = struct{}{}
		} else if section.root == ([32]byte{}) && len(section.proof.Keys) == 0 && len(section.extensions) == 0 {
			if entryValue != ([32]byte{}) {
				return ErrInvalidProof
			}
		} else {
			return ErrInvalidProof
		}

		if valueKind == WitnessValuePre && entry.pre != entry.post {
			return ErrInvalidProof
		}
	}

	if len(seenProof) != len(proofValues) {
		return ErrInvalidProof
	}
	if len(seenExtension) != len(extensionMap) {
		return ErrInvalidProof
	}

	return nil
}
