package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"

	evmtypes "github.com/jam-duna/jamduna/types"
	"github.com/jam-duna/jamduna/common"
	log "github.com/jam-duna/jamduna/log"
	"github.com/jam-duna/jamduna/types"
)

type valueEntry struct {
	value   [32]byte
	present bool
}

type extensionProof struct {
	missingStem     Stem
	stem            Stem
	stemHash        [32]byte
	divergenceDepth uint16
	siblings        [][32]byte
}

// BuildUBTWitness constructs dual UBT witnesses from execution reads/writes.
func BuildUBTWitness(
	ubtReadLog []types.UBTRead,
	contractWitnessBlob []byte,
	tree *UnifiedBinaryTree,
) (preWitness []byte, postWitness []byte, postStateRoot common.Hash, postTree *UnifiedBinaryTree, err error) {
	log.Debug(log.EVM, "BuildUBTWitness (dual-proof)", "readCount", len(ubtReadLog), "blobSize", len(contractWitnessBlob))

	if tree == nil {
		return nil, nil, common.Hash{}, nil, fmt.Errorf("nil UBT tree provided")
	}

	// ===== PHASE 1: Collect read/write keys + metadata =====
	readKeys, readMeta := collectUBTReadKeys(ubtReadLog)
	writeKeys, writeMeta, err := collectUBTWriteKeys(contractWitnessBlob)
	if err != nil {
		return nil, nil, common.Hash{}, nil, fmt.Errorf("failed to parse contract writes: %w", err)
	}

	allKeys := unionKeys(readKeys, writeKeys)

	// ===== PHASE 2: Pre-state values =====
	preValues := make(map[[32]byte]valueEntry, len(allKeys))
	for _, key := range allKeys {
		meta := chooseMetadata(key, readMeta, writeMeta)
		val, present := readValueFromUBT(tree, meta)
		preValues[key] = valueEntry{value: val, present: present}
	}

	preRoot := tree.RootHash()
	readKeysForProof, missingReadKeys := partitionKeysByStem(tree, readKeys)
	preProof, err := buildMultiProof(tree, readKeysForProof)
	if err != nil {
		return nil, nil, common.Hash{}, nil, fmt.Errorf("failed to generate pre-state multiproof: %w", err)
	}
	preExtensions, err := buildExtensionProofs(tree, missingReadKeys)
	if err != nil {
		return nil, nil, common.Hash{}, nil, fmt.Errorf("failed to generate pre-state extension proofs: %w", err)
	}

	// ===== PHASE 3: Post-state values + tree =====
	postTree = tree.Copy()
	if err := applyContractWritesToTree(contractWitnessBlob, postTree); err != nil {
		return nil, nil, common.Hash{}, nil, fmt.Errorf("failed to apply contract writes: %v", err)
	}

	postValues := make(map[[32]byte]valueEntry, len(writeKeys))
	for _, key := range writeKeys {
		meta := writeMeta[key]
		val, present := readValueFromUBT(postTree, meta)
		postValues[key] = valueEntry{value: val, present: present}
	}
	postRoot := postTree.RootHash()
	writeKeysForProof, missingWriteKeys := partitionKeysByStem(postTree, writeKeys)
	postProof, err := buildMultiProof(postTree, writeKeysForProof)
	if err != nil {
		return nil, nil, common.Hash{}, nil, fmt.Errorf("failed to generate post-state multiproof: %w", err)
	}
	postExtensions, err := buildExtensionProofs(postTree, missingWriteKeys)
	if err != nil {
		return nil, nil, common.Hash{}, nil, fmt.Errorf("failed to generate post-state extension proofs: %w", err)
	}
	postStateRoot = common.BytesToHash(postRoot[:])

	// ===== PHASE 4: Serialize witness sections =====
	preWitness, err = serializeUBTWitnessSection(preRoot, readKeys, readMeta, preValues, preValues, preProof, preExtensions)
	if err != nil {
		return nil, nil, common.Hash{}, nil, fmt.Errorf("failed to serialize pre-state witness: %w", err)
	}

	postWitness, err = serializeUBTWitnessSection(postRoot, writeKeys, writeMeta, preValues, postValues, postProof, postExtensions)
	if err != nil {
		return nil, nil, common.Hash{}, nil, fmt.Errorf("failed to serialize post-state witness: %w", err)
	}

	if outPath := os.Getenv("JAM_UBT_WITNESS_PRE_OUT"); outPath != "" {
		if err := os.WriteFile(outPath, preWitness, 0o644); err != nil {
			log.Warn(log.EVM, "BuildUBTWitness: failed to write pre witness file", "path", outPath, "err", err)
		} else {
			log.Info(log.EVM, "BuildUBTWitness: wrote pre witness file", "path", outPath, "size", len(preWitness))
		}
	}
	if outPath := os.Getenv("JAM_UBT_WITNESS_POST_OUT"); outPath != "" {
		if err := os.WriteFile(outPath, postWitness, 0o644); err != nil {
			log.Warn(log.EVM, "BuildUBTWitness: failed to write post witness file", "path", outPath, "err", err)
		} else {
			log.Info(log.EVM, "BuildUBTWitness: wrote post witness file", "path", outPath, "size", len(postWitness))
		}
	}

	log.Trace(log.EVM, "BuildUBTWitness: dual-proof complete",
		"preSize", len(preWitness),
		"postSize", len(postWitness),
		"readKeys", len(readKeys),
		"writeKeys", len(writeKeys))

	// Debug: Log raw witness bytes
	preHead := preWitness
	if len(preHead) > 128 {
		preHead = preHead[:128]
	}
	preTail := preWitness
	if len(preTail) > 128 {
		preTail = preTail[len(preTail)-128:]
	}
	log.Info(log.EVM, "BuildUBTWitness: pre-witness bytes",
		"len", len(preWitness),
		"head128", fmt.Sprintf("%x", preHead),
		"tail128", fmt.Sprintf("%x", preTail))

	postHead := postWitness
	if len(postHead) > 128 {
		postHead = postHead[:128]
	}
	postTail := postWitness
	if len(postTail) > 128 {
		postTail = postTail[len(postTail)-128:]
	}
	log.Info(log.EVM, "BuildUBTWitness: post-witness bytes",
		"len", len(postWitness),
		"head128", fmt.Sprintf("%x", postHead),
		"tail128", fmt.Sprintf("%x", postTail))

	return preWitness, postWitness, postStateRoot, postTree, nil
}

func collectUBTReadKeys(reads []types.UBTRead) (keys [][32]byte, meta map[[32]byte]KeyMetadata) {
	meta = make(map[[32]byte]KeyMetadata, len(reads))
	keySet := make(map[[32]byte]struct{}, len(reads))

	for _, read := range reads {
		var key [32]byte
		copy(key[:], read.TreeKey[:])
		keySet[key] = struct{}{}

		var addr [20]byte
		copy(addr[:], read.Address[:])

		var storageKey [32]byte
		copy(storageKey[:], read.StorageKey[:])

		entry := KeyMetadata{
			KeyType:    read.KeyType,
			Address:    addr,
			Extra:      read.Extra,
			StorageKey: storageKey,
			TxIndex:    read.TxIndex,
		}

		if existing, ok := meta[key]; !ok || entry.TxIndex < existing.TxIndex {
			meta[key] = entry
		}
	}

	keys = make([][32]byte, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})
	return keys, meta
}

func collectUBTWriteKeys(blob []byte) (keys [][32]byte, meta map[[32]byte]KeyMetadata, err error) {
	meta = make(map[[32]byte]KeyMetadata)
	if len(blob) == 0 {
		return nil, meta, nil
	}

	offset := 0
	for offset < len(blob) {
		if len(blob)-offset < 29 {
			return nil, nil, fmt.Errorf("invalid contract witness blob: insufficient header bytes at offset %d", offset)
		}

		var address [20]byte
		copy(address[:], blob[offset:offset+20])
		kind := blob[offset+20]
		payloadLength := uint32(blob[offset+21]) | uint32(blob[offset+22])<<8 |
			uint32(blob[offset+23])<<16 | uint32(blob[offset+24])<<24
		txIndex := uint32(blob[offset+25]) | uint32(blob[offset+26])<<8 |
			uint32(blob[offset+27])<<16 | uint32(blob[offset+28])<<24
		offset += 29

		if len(blob)-offset < int(payloadLength) {
			return nil, nil, fmt.Errorf("invalid contract witness blob: insufficient payload bytes at offset %d", offset)
		}
		payload := blob[offset : offset+int(payloadLength)]
		offset += int(payloadLength)

		switch kind {
		case 0x01: // Storage shard
			entries, err := evmtypes.ParseShardPayload(payload)
			if err != nil {
				log.Warn(log.EVM, "collectUBTWriteKeys: failed to parse storage shard", "err", err)
				continue
			}
			for _, entry := range entries {
				var slotKey [32]byte
				copy(slotKey[:], entry.KeyH[:])
				key := GetStorageSlotKey(defaultUBTProfile, common.BytesToAddress(address[:]), slotKey)
				keyBytes := key.ToBytes()
				metaKey := KeyMetadata{
					KeyType:    3,
					Address:    address,
					StorageKey: slotKey,
					TxIndex:    txIndex,
				}
				upsertMetadata(meta, keyBytes, metaKey)
			}

		case 0x00: // Code
			codeHashKey := GetCodeHashKey(defaultUBTProfile, common.BytesToAddress(address[:]))
			upsertMetadata(meta, codeHashKey.ToBytes(), KeyMetadata{KeyType: 1, Address: address, TxIndex: txIndex})

			chunks := ChunkifyCode(payload)
			numChunks := len(chunks) / 32
			for chunkID := uint64(0); chunkID < uint64(numChunks); chunkID++ {
				chunkKey := GetCodeChunkKey(defaultUBTProfile, common.BytesToAddress(address[:]), chunkID)
				upsertMetadata(meta, chunkKey.ToBytes(), KeyMetadata{
					KeyType: 2,
					Address: address,
					Extra:   chunkID,
					TxIndex: txIndex,
				})
			}

			basicDataKey := GetBasicDataKey(defaultUBTProfile, common.BytesToAddress(address[:]))
			upsertMetadata(meta, basicDataKey.ToBytes(), KeyMetadata{KeyType: 0, Address: address, TxIndex: txIndex})

		case 0x02: // Balance
			basicDataKey := GetBasicDataKey(defaultUBTProfile, common.BytesToAddress(address[:]))
			upsertMetadata(meta, basicDataKey.ToBytes(), KeyMetadata{KeyType: 0, Address: address, TxIndex: txIndex})

		case 0x06: // Nonce
			basicDataKey := GetBasicDataKey(defaultUBTProfile, common.BytesToAddress(address[:]))
			upsertMetadata(meta, basicDataKey.ToBytes(), KeyMetadata{KeyType: 0, Address: address, TxIndex: txIndex})

		default:
			log.Warn(log.EVM, "collectUBTWriteKeys: unknown kind", "kind", kind)
		}
	}

	keys = make([][32]byte, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})
	return keys, meta, nil
}

func upsertMetadata(meta map[[32]byte]KeyMetadata, keyBytes [32]byte, entry KeyMetadata) {
	if existing, ok := meta[keyBytes]; !ok || entry.TxIndex < existing.TxIndex {
		meta[keyBytes] = entry
	}
}

func unionKeys(a [][32]byte, b [][32]byte) [][32]byte {
	set := make(map[[32]byte]struct{}, len(a)+len(b))
	for _, key := range a {
		set[key] = struct{}{}
	}
	for _, key := range b {
		set[key] = struct{}{}
	}
	out := make([][32]byte, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		return bytes.Compare(out[i][:], out[j][:]) < 0
	})
	return out
}

func chooseMetadata(key [32]byte, readMeta map[[32]byte]KeyMetadata, writeMeta map[[32]byte]KeyMetadata) KeyMetadata {
	if meta, ok := readMeta[key]; ok {
		return meta
	}
	if meta, ok := writeMeta[key]; ok {
		return meta
	}
	return KeyMetadata{}
}

func readValueFromUBT(tree *UnifiedBinaryTree, meta KeyMetadata) ([32]byte, bool) {
	key, ok := treeKeyFromMetadata(meta)
	if !ok {
		return [32]byte{}, false
	}
	value, found, _ := tree.Get(&key)
	if !found {
		return [32]byte{}, false
	}
	return value, true
}

func treeKeyFromMetadata(meta KeyMetadata) (TreeKey, bool) {
	address := common.BytesToAddress(meta.Address[:])
	switch meta.KeyType {
	case 0: // BasicData
		return GetBasicDataKey(defaultUBTProfile, address), true
	case 1: // CodeHash
		return GetCodeHashKey(defaultUBTProfile, address), true
	case 2: // CodeChunk
		return GetCodeChunkKey(defaultUBTProfile, address, meta.Extra), true
	case 3: // Storage
		return GetStorageSlotKey(defaultUBTProfile, address, meta.StorageKey), true
	default:
		return TreeKey{}, false
	}
}

func buildMultiProof(tree *UnifiedBinaryTree, keys [][32]byte) ([]byte, error) {
	if len(keys) == 0 {
		return serializeMultiProof(&MultiProof{})
	}
	treeKeys := make([]TreeKey, 0, len(keys))
	for _, key := range keys {
		treeKeys = append(treeKeys, TreeKeyFromBytes(key))
	}
	proof, err := tree.GenerateMultiProof(treeKeys)
	if err != nil {
		return nil, err
	}
	return serializeMultiProof(proof)
}

func serializeMultiProof(mp *MultiProof) ([]byte, error) {
	if len(mp.MissingKeys) != 0 {
		return nil, ErrInvalidProof
	}
	if len(mp.Keys) != len(mp.Values) {
		return nil, ErrInvalidProof
	}

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(mp.Keys))); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(mp.Values))); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(mp.Nodes))); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(mp.NodeRefs))); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(mp.Stems))); err != nil {
		return nil, err
	}

	for _, key := range mp.Keys {
		if err := binary.Write(&buf, binary.LittleEndian, key.StemIndex); err != nil {
			return nil, err
		}
		if err := buf.WriteByte(key.Subindex); err != nil {
			return nil, err
		}
	}

	for _, val := range mp.Values {
		if val == nil {
			if err := buf.WriteByte(0); err != nil {
				return nil, err
			}
			continue
		}
		if err := buf.WriteByte(1); err != nil {
			return nil, err
		}
		if _, err := buf.Write(val[:]); err != nil {
			return nil, err
		}
	}

	for _, node := range mp.Nodes {
		if _, err := buf.Write(node[:]); err != nil {
			return nil, err
		}
	}

	for _, ref := range mp.NodeRefs {
		if err := binary.Write(&buf, binary.LittleEndian, ref); err != nil {
			return nil, err
		}
	}

	for _, stem := range mp.Stems {
		if _, err := buf.Write(stem[:]); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func partitionKeysByStem(tree *UnifiedBinaryTree, keys [][32]byte) (existing [][32]byte, missing [][32]byte) {
	existing = make([][32]byte, 0, len(keys))
	missing = make([][32]byte, 0)
	for _, key := range keys {
		tk := TreeKeyFromBytes(key)
		if _, ok := tree.getStemNode(tk.Stem); ok {
			existing = append(existing, key)
		} else {
			missing = append(missing, key)
		}
	}
	return existing, missing
}

func buildExtensionProofs(tree *UnifiedBinaryTree, keys [][32]byte) ([]extensionProof, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	stemKeys := make(map[Stem][32]byte, len(keys))
	for _, keyBytes := range keys {
		key := TreeKeyFromBytes(keyBytes)
		if existing, ok := stemKeys[key.Stem]; !ok || bytes.Compare(keyBytes[:], existing[:]) < 0 {
			stemKeys[key.Stem] = keyBytes
		}
	}
	stems := make([]Stem, 0, len(stemKeys))
	for stem := range stemKeys {
		stems = append(stems, stem)
	}
	sort.Slice(stems, func(i, j int) bool {
		return stemLess(stems[i], stems[j])
	})

	exts := make([]extensionProof, 0, len(stems))
	for _, stem := range stems {
		keyBytes := stemKeys[stem]
		key := TreeKeyFromBytes(keyBytes)
		proof, err := tree.GenerateProof(&key)
		if err != nil {
			return nil, err
		}
		if len(proof.Path) == 0 {
			continue
		}
		var extNode *ExtensionProofNode
		siblings := make([][32]byte, 0, 248)
		for _, node := range proof.Path {
			switch n := node.(type) {
			case *ExtensionProofNode:
				if extNode != nil {
					return nil, ErrInvalidProof
				}
				extNode = n
			case *InternalProofNode:
				siblings = append(siblings, n.Sibling)
			default:
				return nil, ErrInvalidProof
			}
		}
		if extNode == nil || len(siblings) != 248 {
			return nil, ErrInvalidProof
		}
		exts = append(exts, extensionProof{
			missingStem:     stem,
			stem:            extNode.Stem,
			stemHash:        extNode.StemHash,
			divergenceDepth: extNode.DivergenceDepth,
			siblings:        siblings,
		})
	}
	return exts, nil
}

func serializeExtensionProofs(exts []extensionProof) ([]byte, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(exts))); err != nil {
		return nil, err
	}
	for _, ext := range exts {
		if _, err := buf.Write(ext.missingStem[:]); err != nil {
			return nil, err
		}
		if _, err := buf.Write(ext.stem[:]); err != nil {
			return nil, err
		}
		if _, err := buf.Write(ext.stemHash[:]); err != nil {
			return nil, err
		}
		if err := binary.Write(&buf, binary.BigEndian, ext.divergenceDepth); err != nil {
			return nil, err
		}
		if len(ext.siblings) != 248 {
			return nil, ErrInvalidProof
		}

		// Sparse encoding: bitmap (31 bytes) + non-zero siblings
		// Bitmap has 248 bits (one per sibling), packed into 31 bytes
		var bitmap [31]byte
		nonZeroSiblings := make([][32]byte, 0, 248)

		for i, sibling := range ext.siblings {
			if sibling != ([32]byte{}) {
				// Set bit i in bitmap
				bitmap[i/8] |= (1 << (7 - i%8))
				nonZeroSiblings = append(nonZeroSiblings, sibling)
			}
		}

		// Write bitmap
		if _, err := buf.Write(bitmap[:]); err != nil {
			return nil, err
		}

		// Write non-zero siblings
		for _, sibling := range nonZeroSiblings {
			if _, err := buf.Write(sibling[:]); err != nil {
				return nil, err
			}
		}
	}
	return buf.Bytes(), nil
}

func serializeUBTWitnessSection(
	root [32]byte,
	keys [][32]byte,
	meta map[[32]byte]KeyMetadata,
	preValues map[[32]byte]valueEntry,
	postValues map[[32]byte]valueEntry,
	proof []byte,
	extensions []extensionProof,
) ([]byte, error) {
	extensionBytes, err := serializeExtensionProofs(extensions)
	if err != nil {
		return nil, err
	}
	count := uint32(len(keys))
	totalSize := 32 + 4 + (161 * len(keys)) + 4 + len(proof) + len(extensionBytes)
	result := make([]byte, 0, totalSize)

	result = append(result, root[:]...)
	result = append(result, byte(count>>24), byte(count>>16), byte(count>>8), byte(count))

	for _, key := range keys {
		result = append(result, key[:]...)

		entry := meta[key]
		result = append(result, entry.KeyType)
		result = append(result, entry.Address[:]...)
		result = append(result, byte(entry.Extra>>56), byte(entry.Extra>>48), byte(entry.Extra>>40), byte(entry.Extra>>32),
			byte(entry.Extra>>24), byte(entry.Extra>>16), byte(entry.Extra>>8), byte(entry.Extra))
		result = append(result, entry.StorageKey[:]...)

		pre := [32]byte{}
		if val, ok := preValues[key]; ok && val.present {
			pre = val.value
		}
		post := [32]byte{}
		if val, ok := postValues[key]; ok && val.present {
			post = val.value
		}

		result = append(result, pre[:]...)
		result = append(result, post[:]...)

		result = append(result, byte(entry.TxIndex>>24), byte(entry.TxIndex>>16), byte(entry.TxIndex>>8), byte(entry.TxIndex))
	}

	proofLen := uint32(len(proof))
	result = append(result, byte(proofLen>>24), byte(proofLen>>16), byte(proofLen>>8), byte(proofLen))
	result = append(result, proof...)

	result = append(result, extensionBytes...)

	return result, nil
}

// ExtractUBTWriteMapFromContractWitness applies a contract witness blob to a cloned tree and
// returns the resulting write set as a map from UBT key -> post-value.
func ExtractUBTWriteMapFromContractWitness(tree *UnifiedBinaryTree, blob []byte) (map[[32]byte][32]byte, error) {
	writeMap := make(map[[32]byte][32]byte)

	if len(blob) == 0 {
		return writeMap, nil
	}
	if tree == nil {
		return nil, fmt.Errorf("nil UBT tree provided")
	}

	writeKeys, writeMeta, err := collectUBTWriteKeys(blob)
	if err != nil {
		return nil, err
	}

	postTree := tree.Copy()
	if err := applyContractWritesToTree(blob, postTree); err != nil {
		return nil, err
	}

	for _, key := range writeKeys {
		meta := writeMeta[key]
		val, present := readValueFromUBT(postTree, meta)
		if !present {
			val = [32]byte{}
		}
		writeMap[key] = val
	}

	return writeMap, nil
}
