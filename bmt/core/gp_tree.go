package core

// GpTrieNode is a simple binary trie node for GP compatibility testing.
type GpTrieNode struct {
	Hash  Node
	Key   *KeyPath
	Left  *GpTrieNode
	Right *GpTrieNode
}

// BuildGpTree builds a binary merkle patricia trie from key-value pairs using GP encoding.
//
// This mirrors the logic from JAM's Go BPT `buildMerkleTree` function.
//
// Arguments:
//   - kvs: Key-value pairs to insert (keys must be 32 bytes)
//   - depth: Current bit depth in the trie
//   - hasher: Hash function to use (Blake2b-256 for GP)
//
// Returns: Root node of the constructed trie with computed hash
func BuildGpTree(kvs []KVPair, depth int, hasher func([]byte) [32]byte) *GpTrieNode {
	// Base case: empty data
	if len(kvs) == 0 {
		return &GpTrieNode{
			Hash:  Terminator,
			Key:   nil,
			Left:  nil,
			Right: nil,
		}
	}

	// Base case: single key-value pair (leaf node)
	if len(kvs) == 1 {
		key := kvs[0].Key
		value := kvs[0].Value

		// Create GP leaf data
		var keyPath KeyPath
		copy(keyPath[:], key[:])

		valueHash := hasher(value)
		leafData := NewLeafDataGP(keyPath, value, valueHash)
		encoded := EncodeGpLeaf(leafData)
		leafHash := hasher(encoded[:])

		return &GpTrieNode{
			Hash:  leafHash,
			Key:   &key,
			Left:  nil,
			Right: nil,
		}
	}

	// Recursive case: split by bit at current depth
	var leftKVs, rightKVs []KVPair

	for _, kv := range kvs {
		if getGpBit(kv.Key[:], depth) {
			rightKVs = append(rightKVs, kv)
		} else {
			leftKVs = append(leftKVs, kv)
		}
	}

	// Recursively build left and right subtries
	leftNode := BuildGpTree(leftKVs, depth+1, hasher)
	rightNode := BuildGpTree(rightKVs, depth+1, hasher)

	// Encode branch using GP Equation D.3
	internalData := &InternalData{
		Left:  leftNode.Hash,
		Right: rightNode.Hash,
	}
	branchEncoded := EncodeGpInternal(internalData)
	branchHash := hasher(branchEncoded[:])

	return &GpTrieNode{
		Hash:  branchHash,
		Key:   nil,
		Left:  leftNode,
		Right: rightNode,
	}
}

// KVPair represents a key-value pair for tree building.
type KVPair struct {
	Key   KeyPath
	Value []byte
}

// getGpBit gets the i-th bit of a key (MSB first).
//
// This matches the `bit()` function from JAM's Go BPT:
//
//	func bit(k []byte, i int) bool {
//	    byteIndex := i / 8
//	    if byteIndex >= len(k) { return false }
//	    bitIndex := 7 - (i % 8)
//	    b := k[byteIndex]
//	    mask := byte(1 << (bitIndex))
//	    return (b & mask) != 0
//	}
func getGpBit(key []byte, bitIndex int) bool {
	if bitIndex >= 256 {
		return false
	}
	byteIndex := bitIndex / 8
	if byteIndex >= len(key) {
		return false
	}
	bitInByte := 7 - (bitIndex % 8)
	mask := byte(1 << bitInByte)
	return (key[byteIndex] & mask) != 0
}
