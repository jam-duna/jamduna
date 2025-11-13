package trie

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	storage "github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

const (
	debug              = "trie"
	stateKeySize       = 32 // the "actual" size is 31 but we use common.Hash with the 32 byte being 0 INCLUDING IN METADATA right now
	DefaultLevelDBPath = "/tmp/log/leveldb/bpt"
)

type KeyVal struct {
	Key        []byte `json:"k"`
	Value      []byte `json:"v"`
	StructType string `json:"struct_type,omitempty"`
	Metadata   string `json:"meta,omitempty"`
}

type KeyVals struct {
	KeyVals []KeyVal
}

type BMTProof []common.Hash

// Node represents a node in the Merkle Tree
type Node struct {
	Hash  []byte
	Key   []byte
	Left  *Node
	Right *Node
}

/*
Branch Node (64 bytes)
+-------------------------------------------------+
|    First 255 bits of left child node hash       |
+-------------------------------------------------+
|    Full 256 bits of right child node hash       |
+-------------------------------------------------+

Embedded-Value Leaf Node (64 bytes) <= data is small enough, let's store the value in left <0x1234 .... 0000>
+--------+------------------------------------------+
|  2 bits | 6 bits (value size) | 31 bytes (key)    |
+--------+------------------------------------------+
|              32 bytes (embedded value)            |
+---------------------------------------------------+

Regular Leaf Node (64 bytes) [K,V] -> V >= 32bytes. too long, only store Hash
+--------+------------------------------------------+
|  2 bits | 6 bits (0s) | 31 bytes (key)            |
+--------+------------------------------------------+
|               32 bytes (hash of value)            |
+---------------------------------------------------+
*/

// MerkleTree represents the entire Merkle Tree
type MerkleTree struct {
	Root       *Node
	db         *storage.StateDBStorage
	writeMutex sync.Mutex             // Protects concurrent writes to levelDB during Flush
	writeBatch map[common.Hash][]byte // Batched writes (key -> value) - always batched
	batchMutex sync.Mutex             // Protects the write batch

	// Staged operations - no hashing, no levelDB until Flush
	stagedInserts map[common.Hash][]byte // key -> value
	stagedDeletes map[common.Hash]bool   // key -> true
	stagedMutex   sync.Mutex             // Protects staged operations

	// Tree structure protection
	treeMutex sync.RWMutex // Protects Root and all Node structures
}

// normalizeKey32 ensures all keys are 32 bytes (zero-padded on right if shorter, truncated from right if longer)
// Note we cannot use common.BytesToHash because it pads on the left
func normalizeKey32(src []byte) common.Hash {
	var hash common.Hash
	copy(hash[:], src)
	return hash
}

// NewMerkleTree creates a new Merkle Tree from the provided data
func initLevelDB(optionalPath ...string) (*storage.StateDBStorage, error) {
	path := "/tmp/log/leveldb/bpt"
	if len(optionalPath) > 0 {
		path = optionalPath[0]
	}
	stateDBStorage, err := storage.NewStateDBStorage(path, storage.NewMockJAMDA(), nil, 0)
	return stateDBStorage, err
}

func InitLevelDB(optionalPath ...string) (*storage.StateDBStorage, error) {
	return initLevelDB(optionalPath...)
}

func DeleteLevelDB(optionalPath ...string) error {
	path := "/tmp/log/leveldb/bpt"
	if len(optionalPath) > 0 {
		path = optionalPath[0]
	}

	err := os.RemoveAll(path)
	if err != nil {
		return fmt.Errorf("failed to delete LevelDB at %s: %v", path, err)
	}
	return nil
}

func NewMerkleTree(data [][2][]byte, db *storage.StateDBStorage) *MerkleTree {
	t := &MerkleTree{
		Root:          nil,
		db:            db,
		writeBatch:    make(map[common.Hash][]byte),
		stagedInserts: make(map[common.Hash][]byte),
		stagedDeletes: make(map[common.Hash]bool),
	}
	if len(data) == 0 {
		return t
	}
	root := t.buildMerkleTree(data, 0)
	t.Root = root
	return t
}

// buildMerkleTree constructs the Merkle tree from key-value pairs and stores them in levelDB
func (t *MerkleTree) buildMerkleTree(kvs [][2][]byte, i int) *Node {
	// Base Case - Empty Data |d| = 0
	if len(kvs) == 0 {
		return &Node{Hash: make([]byte, 32)}
	}
	// V(d) = {(K,v)}
	if len(kvs) == 1 {
		encoded := leaf(kvs[0][0], kvs[0][1], t)
		// Store (Hash, Value) in levelDB for future lookup
		// computeHash(encoded) -> value mapping
		if t.db != nil {
			t.levelDBSetLeaf(encoded, kvs[0][1], kvs[0][0])
		}
		return &Node{Hash: computeHash(encoded), Key: kvs[0][0]}
	}
	// Recursive Case: B(M(l),M(r))
	l := make([][2][]byte, 0, len(kvs)/2+1)
	r := make([][2][]byte, 0, len(kvs)/2+1)
	for _, kv := range kvs {
		if bit(kv[0], i) {
			r = append(r, kv)
		} else {
			l = append(l, kv)
		}
	}

	left := t.buildMerkleTree(l, i+1)
	right := t.buildMerkleTree(r, i+1)
	encoded := branch(left.Hash, right.Hash)
	branchHash := computeHash(encoded)
	// Store branch nodes in levelDB
	if t.db != nil {
		t.levelDBSetBranch(branchHash, encoded)
	}
	return &Node{Hash: branchHash, Left: left, Right: right}
}

// Equation(D.3) in GP 0.6.2
// branch concatenates the left and right node hashes with a modified head
func branch(left, right []byte) []byte {
	if len(left) != 32 || len(right) != 32 {
		log.Crit(debug, "branch: input hashes must be 32 bytes")
	}
	head := left[0] & 0x7f                           // Set the LSB of the first byte of the left hash to 0
	left255bits := append([]byte{head}, left[1:]...) // Left: last 255 bits of
	concatenated := append(left255bits, right...)    // (l,r): 512 bits
	return concatenated
}

// Equation(D.4) in GP 0.6.2
// leaf encodes a key-value pair into a leaf node
func leaf(k, v []byte, t *MerkleTree) []byte {
	var result [64]byte

	// Embedded-value leaf node
	if len(v) <= 32 {
		result[0] = byte(0b10000000 | len(v))
		if len(k) > 31 {
			copy(result[1:32], k[:31])
		} else {
			copy(result[1:1+len(k)], k)
			// Zero padding is already done by make()
		}
		copy(result[32:32+len(v)], v)
	} else {
		// Regular leaf node
		result[0] = byte(0b11000000)
		if len(k) > 31 {
			copy(result[1:32], k[:31])
		} else {
			copy(result[1:1+len(k)], k)
			// Zero padding is already done by make()
		}
		hash := computeHash(v)
		copy(result[32:64], hash)
	}
	return result[:]
}

// decodeLeaf decodes a leaf node into its key and value/hash
func decodeLeaf(leaf []byte) (k []byte, v []byte, isEmbedded bool, err error) {
	if len(leaf) != 64 {
		return nil, nil, false, fmt.Errorf("invalid leaf length %v", len(leaf))
	}

	head := leaf[0]
	key := leaf[1:32]

	if head&0b11000000 == 0b10000000 {
		// Embedded-value leaf node
		valueSize := int(head & 0b00111111) // Extract the value size from the lower 6 bits
		value := leaf[32 : 32+valueSize]
		return key, value, true, nil
	} else if head&0b11000000 == 0b11000000 {
		// Regular leaf node
		hash := leaf[32:64]
		return key, hash, false, nil
	} else {
		return nil, nil, false, fmt.Errorf("invalid leaf node header")
	}
}

func bit(k []byte, i int) bool {
	byteIndex := i / 8 // the byte index in the array where the bit is located
	if byteIndex >= len(k) {
		return false // return false if index is out of range
	}
	bitIndex := 7 - (i % 8)       // the bit position within the byte
	b := k[byteIndex]             // target byte
	mask := byte(1 << (bitIndex)) // least significant bit first
	return (b & mask) != 0        // return set (1) or not (0)
}

func (t *MerkleTree) GetRoot() common.Hash {
	t.treeMutex.RLock()
	defer t.treeMutex.RUnlock()

	if t.Root == nil {
		return normalizeKey32(make([]byte, 32))
	}
	return normalizeKey32(t.Root.Hash)
}

func InitMerkleTreeFromHash(root common.Hash, db *storage.StateDBStorage) (*MerkleTree, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	tree := &MerkleTree{
		Root:          nil,
		db:            db,
		writeBatch:    make(map[common.Hash][]byte),
		stagedInserts: make(map[common.Hash][]byte),
		stagedDeletes: make(map[common.Hash]bool),
	}
	if root == (common.Hash{}) {
		return tree, nil
	}
	rootNode, err := tree.levelDBGetNode(root)
	if err != nil {
		return nil, err
	}
	tree.Root = rootNode
	return tree, nil
}

// Flush commits all staged operations (computing hashes and rebuilding tree) and writes to levelDB
func (t *MerkleTree) Flush() (common.Hash, error) {
	// Step 1: Commit staged operations to tree (rebuild tree with hashing)
	t.stagedMutex.Lock()
	numOps := len(t.stagedInserts) + len(t.stagedDeletes)
	if numOps == 0 {
		t.stagedMutex.Unlock()
	} else {
		// Take a snapshot of staged ops to apply outside stagedMutex
		deletes := t.stagedDeletes
		inserts := t.stagedInserts

		// Clear staged maps
		t.stagedInserts = make(map[common.Hash][]byte)
		t.stagedDeletes = make(map[common.Hash]bool)
		t.stagedMutex.Unlock()

		// Now mutate the tree under treeMutex, without holding stagedMutex
		t.treeMutex.Lock()

		// Process deletes first
		for key := range deletes {
			if t.Root != nil {
				t.Root = t.deleteNode(t.Root, key[:], 0)
			}
		}

		// Process inserts/updates
		for key, value := range inserts {
			keySlice := key[:]

			node, err := t.findNode(t.Root, keySlice, 0)
			encodedLeaf := leaf(keySlice, value, t)
			t.levelDBSetLeaf(encodedLeaf, value, keySlice)

			if err != nil {
				// Insert new
				if t.Root == nil {
					t.Root = &Node{
						Hash: computeHash(encodedLeaf),
						Key:  keySlice,
					}
				} else {
					t.Root = t.insertNode(t.Root, keySlice, value, 0)
				}
			} else {
				// Update existing
				node.Hash = computeHash(leaf(keySlice, value, t))
				t.updateTree(t.Root, keySlice, value, 0)
			}
		}
		t.treeMutex.Unlock()
	}

	// Step 2: Flush the write batch to LevelDB
	t.batchMutex.Lock()
	defer t.batchMutex.Unlock()

	batchWriteLen, err := t.flushBatchLocked()
	root := t.GetRoot()
	log.Info(log.P, "Flush", "n", t.db.NodeID, "s+", root.String_short(), "batchWriteLen", batchWriteLen)
	return root, err
}

// flushBatchLocked flushes the write batch - must be called with mutex
func (t *MerkleTree) flushBatchLocked() (int, error) {
	if len(t.writeBatch) == 0 {
		return 0, nil
	}

	// Wait for exclusive access to levelDB writes -- levelDB CANNOT HANDLE CONCURRENT WRITES
	t.writeMutex.Lock()
	defer t.writeMutex.Unlock()

	// Write all batched items (deletes don't *actually* delete in leveldb)
	for key, value := range t.writeBatch {
		err := t.db.WriteRawKV(key[:], value)
		if err != nil {
			return 0, fmt.Errorf("failed to flush key: %v", err)
		}
	}
	batchWriteLen := len(t.writeBatch)
	t.writeBatch = make(map[common.Hash][]byte)
	return batchWriteLen, nil
}

func (t *MerkleTree) levelDBSetBranch(branchHash, value []byte) {
	/*
		Branch Node (64 bytes)
		+-------------------------------------------------+
		|    First 255 bits of left child node hash       |
		+-------------------------------------------------+
		|    Full 256 bits of right child node hash       |
		+-------------------------------------------------+
	*/
	// store Left hash and Right hash separately
	t.levelDBSet(branchHash, append([]byte("branch"), value...))
}

func (t *MerkleTree) levelDBSetLeaf(encodedLeaf, value []byte, key []byte) {
	_k, _v, isEmbedded, _ := decodeLeaf(encodedLeaf)
	encodedLeafHash := computeHash(encodedLeaf)
	encodedLeafHashVal := computeHash(append(encodedLeafHash, value...))
	//t.levelDBSet(key, encodedLeafHashVal)
	t.levelDBSet(encodedLeafHashVal, key)
	if isEmbedded {
		// value-embedded leaf node: 2 bits | 6 bits (value size) | 31 bytes (key)
		// value is less or equal to 32 bytes
		// value can be recovered from encodedLeaf.
		t.levelDBSet(encodedLeafHash, encodedLeaf)
	} else {
		// regular leaf node: 2 bits | 2 bits | 6 bits (0s) | 31 bytes (key)
		// value is greater than 32 bytes
		// store additional hash(value) -> value
		if len(_k) > 0 {

		}
		t.levelDBSet(encodedLeafHash, encodedLeaf)
		t.levelDBSet(_v, value)
	}
}

// getDBLeafValue retrieves the value from a leaf node
// Returns the actual value, not the key
func (t *MerkleTree) getDBLeafValue(nodeHash []byte) ([]byte, bool, error) {
	t.batchMutex.Lock()
	defer t.batchMutex.Unlock()

	encodedLeaf, ok, err := t.levelDBGetLocked(nodeHash)
	if err != nil {
		return nil, false, err
	} else if !ok {
		return nil, false, nil
	}
	//recover encodedLeaf from nodeHash
	_, _v, isEmbedded, err := decodeLeaf(encodedLeaf)
	if err != nil {
		return nil, false, fmt.Errorf("decodeLeaf leaf Err: %s", err)
	}
	if isEmbedded {
		// value-embedded leaf node: 2 bits | 6 bits (value size) | 31 bytes (key)
		// value is less or equal to 32 bytes, return exact value
		return _v, true, nil
	} else {
		// regular leaf node: 2 bits | 2 bits | 6 bits (0s) | 31 bytes (key)
		// value is greater than 32 bytes. lookup _v -> value
		return t.levelDBGetLocked(_v)
	}
}

// levelDBSet adds the key-value pair to the write batch
// Writes are not persisted until Flush() is called
func (t *MerkleTree) levelDBSet(k, v []byte) error {
	if t.db == nil {
		return fmt.Errorf("database is not initialized")
	}

	// Always add to batch
	t.batchMutex.Lock()
	var keyHash common.Hash
	copy(keyHash[:], k)
	t.writeBatch[keyHash] = v
	t.batchMutex.Unlock()

	return nil
}

// levelDBGetLocked is the internal version that assumes batchMutex is already held
func (t *MerkleTree) levelDBGetLocked(k []byte) ([]byte, bool, error) {
	if t.db == nil {
		return nil, false, fmt.Errorf("database is not initialized")
	}

	// Check write batch first (for reads of recently written but not yet flushed data)
	var keyHash common.Hash
	copy(keyHash[:], k)
	if value, exists := t.writeBatch[keyHash]; exists {
		return value, true, nil
	}

	// Not in batch, read from levelDB
	value, ok, err := t.db.ReadRawKV(k)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get key [%s]: %v", k, err)
	} else if !ok {
		return nil, false, nil
	}
	return value, true, nil
}

// levelDBGet gets the value for the given key - checks write batch first, then levelDB
func (t *MerkleTree) levelDBGet(k []byte) ([]byte, bool, error) {
	t.batchMutex.Lock()
	defer t.batchMutex.Unlock()
	return t.levelDBGetLocked(k)
}

func (t *MerkleTree) levelDBGetNode(nodeHash common.Hash) (*Node, error) {
	if nodeHash == (common.Hash{}) {
		return &Node{Hash: nodeHash[:]}, nil
	}

	// Try interpret nodeHash as leaf
	leafValue, _, _ := t.getDBLeafValue(nodeHash[:])
	if leafValue != nil {
		// Use levelDBGet to see batched writes
		encodedLeafHashVal := computeHash(append(nodeHash[:], leafValue...))
		leafKey, _, _ := t.levelDBGet(encodedLeafHashVal)
		return &Node{
			Hash: nodeHash[:],
			Key:  leafKey,
		}, nil
	}

	// Otherwise treat as branch - use levelDBGet to check batch
	value, ok, err := t.levelDBGet(nodeHash[:])
	if err != nil {
		return nil, err
	}
	zeroHash := common.Hash{}
	if !ok || bytes.Equal(value, zeroHash[:]) {
		return &Node{Hash: zeroHash[:]}, nil
	}

	// Decode branch node
	if value == nil {
		return nil, fmt.Errorf("value is nil for branch key: %s", nodeHash.Hex())
	}
	// Branch is stored as: []byte("branch") + 64 bytes = 6 + 64 = 70 bytes
	if len(value) < 70 {
		return nil, fmt.Errorf("branch value too short, expected 70 bytes but got %d bytes", len(value))
	}

	var leftHash common.Hash
	copy(leftHash[:], value[6:38])

	var rightHash common.Hash
	copy(rightHash[:], value[38:70])

	leftNode, err := t.levelDBGetNode(leftHash)
	if err != nil {
		return nil, err
	}

	rightNode, err := t.levelDBGetNode(rightHash)
	if err != nil {
		return nil, err
	}

	return &Node{
		Hash:  nodeHash[:],
		Left:  leftNode,
		Right: rightNode,
	}, nil
}

func (t *MerkleTree) Close() error {
	if t.db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return t.db.Close()
}

// StateDB DEBUG: GetBatchSize returns the current number of batched writes
func (t *MerkleTree) GetBatchSize() int {
	t.batchMutex.Lock()
	defer t.batchMutex.Unlock()
	return len(t.writeBatch)
}

// StateDB DEBUG: GetStagedSize returns the current number of staged operations
func (t *MerkleTree) GetStagedSize() int {
	t.stagedMutex.Lock()
	defer t.stagedMutex.Unlock()
	return len(t.stagedInserts) + len(t.stagedDeletes)
}

func (n *Node) String() string {
	s := fmt.Sprintf("Node Hash=%x, Key=%x\n", n.Hash, n.Key)
	return s
}

func (t *MerkleTree) PrintAllKeyValues() {
	startKey := common.Hex2Bytes("0x0000000000000000000000000000000000000000000000000000000000000000")
	endKey := common.Hex2Bytes("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	maxSize := uint32(math.MaxUint32)
	foundKeyVal, _, _ := t.GetStateByRange(startKey, endKey, maxSize)

	keyVals := make([]KeyVal, 0)
	for _, keyValue := range foundKeyVal {
		fetchRealKey := t.GetRealKey(keyValue.Key, keyValue.Value)
		realValue := make([]byte, len(keyValue.Value))
		realKey := make([]byte, 32)
		copy(realKey, fetchRealKey)
		copy(realValue, keyValue.Value)

		keyVal := KeyVal{
			Key:        realKey,
			Value:      realValue,
			StructType: "",
			Metadata:   "",
		}
		keyVals = append(keyVals, keyVal)
	}
	sortedKeyVals := sortKeyValsByKey(keyVals)

	fmt.Printf("GetAllKeyValues right after %v\n", sortedKeyVals)
}

func (t *MerkleTree) PrintTree(node *Node, level int) {
	t.treeMutex.RLock()
	defer t.treeMutex.RUnlock()

	fmt.Printf("\n----------------PrintTree START----------------\n")
	t.printTree(node, level)
	fmt.Printf("\n----------------PrintTree END----------------\n")
}

// GetStateByRange retrieves key-value pairs within a range see CE129
func (t *MerkleTree) GetStateByRange(startKey []byte, endKey []byte, maxSize uint32) (foundKeyVal []types.StateKeyValue, boundaryNode [][]byte, err error) {
	t.treeMutex.RLock()
	defer t.treeMutex.RUnlock()

	if t.Root == nil {
		return []types.StateKeyValue{}, [][]byte{}, nil
	}

	foundKeyVal = make([]types.StateKeyValue, 0)
	currenSize := uint32(0)
	paddedStart := make([]byte, 32)
	copy(paddedStart, startKey)
	value, _, _ := t.getFromStorageLocked(paddedStart)
	found := false
	end := false
	if value != nil {
		// startKey exists, so use inclusive range comparison
		t.getTreeContentIncludeKey(t.Root, 0, startKey, endKey, &currenSize, maxSize, &foundKeyVal, &boundaryNode, &found, &end)
	} else {
		// startKey doesn't exist - collect all up to exact endKey
		t.getTreeContent(t.Root, 0, startKey, endKey, &currenSize, maxSize, &foundKeyVal, &boundaryNode)
	}
	return foundKeyVal, boundaryNode, err
}

func (t *MerkleTree) getTreeContentIncludeKey(node *Node, level int, startKey []byte, endKey []byte, currenSize *uint32, maxSize uint32, foundkv *[]types.StateKeyValue, boundaryNodes *[][]byte, findKey *bool, end *bool) (ok bool) {
	if node == nil || *currenSize >= maxSize || *end {
		return true
	}
	nodeType := "Branch"
	if node.Left == nil && node.Right == nil {
		nodeType = "Leaf"
	}
	*boundaryNodes = append(*boundaryNodes, node.Hash)

	value, _, _ := t.getFromStorageLocked(node.Key)

	// Copy the original key into the new slice
	if value != nil {
		paddedStart := make([]byte, len(node.Key))
		paddedEnd := make([]byte, len(node.Key))
		// Copy the original key into the new slice
		copy(paddedStart, startKey)
		copy(paddedEnd, endKey)

		if common.CompareBytes(node.Key, paddedStart) {
			fmt.Printf("Found Key: %x\n", node.Key)
			*boundaryNodes = make([][]byte, 0)
			*boundaryNodes, _ = t.getPathLocked(node.Key)
			*boundaryNodes = append(*boundaryNodes, node.Hash)
			*findKey = true
		}
		if *findKey {
			if (common.CompareKeys(paddedStart, node.Key) >= 0 && common.CompareKeys(paddedEnd, node.Key) >= 0) && (nodeType == "Leaf") {
				// check if the key is within range..
				// key is expected to 31 bytes
				var k_31 [31]byte
				copy(k_31[:], node.Key[:31])
				stateKeyValue := types.StateKeyValue{
					Key:   k_31,
					Len:   uint8(len(value)),
					Value: value,
				}
				addSize := len(stateKeyValue.Key) + len(stateKeyValue.Value)
				*foundkv = append(*foundkv, stateKeyValue)
				*currenSize += uint32(addSize)
			}
			if common.CompareBytes(paddedEnd, node.Key) {
				*end = true
				return true
			}
		}
	}
	if node.Left != nil || node.Right != nil {
		t.getTreeContentIncludeKey(node.Left, level+1, startKey, endKey, currenSize, maxSize, foundkv, boundaryNodes, findKey, end)
		t.getTreeContentIncludeKey(node.Right, level+1, startKey, endKey, currenSize, maxSize, foundkv, boundaryNodes, findKey, end)
	}
	return true
}

func (t *MerkleTree) getTreeContent(node *Node, level int, startKey []byte, endKey []byte, currenSize *uint32, maxSize uint32, foundkv *[]types.StateKeyValue, boundaryNodes *[][]byte) (ok bool) {
	if node == nil || *currenSize >= maxSize {
		return true
	}
	nodeType := "Branch"
	if node.Left == nil && node.Right == nil {
		nodeType = "Leaf"
	}
	*boundaryNodes = append(*boundaryNodes, node.Hash)
	value, _, _ := t.getFromStorageLocked(node.Key)
	if value != nil {
		paddedStart := make([]byte, len(node.Key))
		paddedEnd := make([]byte, len(node.Key))
		// Copy the original key into the new slice
		copy(paddedStart, startKey)
		copy(paddedEnd, endKey)
		if nodeType == "Leaf" {
			// check if the key is within range..
			// key is expected to 31 bytes
			var k_31 [31]byte
			copy(k_31[:], node.Key[:31])
			stateKeyValue := types.StateKeyValue{
				Key:   k_31,
				Len:   uint8(len(value)),
				Value: value,
			}
			addSize := len(stateKeyValue.Key) + len(stateKeyValue.Value)
			*foundkv = append(*foundkv, stateKeyValue)
			*currenSize += uint32(addSize)
		}
		if common.CompareBytes(paddedEnd, node.Key) {
			return true
		}
	}
	if node.Left != nil || node.Right != nil {
		t.getTreeContent(node.Left, level+1, startKey, endKey, currenSize, maxSize, foundkv, boundaryNodes)
		t.getTreeContent(node.Right, level+1, startKey, endKey, currenSize, maxSize, foundkv, boundaryNodes)
	}
	return true
}

func (t *MerkleTree) GetRealKey(key [31]byte, value []byte) []byte {
	encodedLeaf := leaf(key[:], value, t)
	nodeHash := computeHash(encodedLeaf)

	leafValue, _, _ := t.getDBLeafValue(nodeHash)
	if leafValue != nil {
		encodedLeafHashVal := computeHash(append(nodeHash, leafValue...))
		// Use levelDBGet to see batched writes
		realKey, _, _ := t.levelDBGet(encodedLeafHashVal)
		return realKey
	}
	return nil
}

func (t *MerkleTree) printTree(node *Node, level int) {
	if level == 0 && t.Root != nil {
		fmt.Printf("Root Hash: %x\n", t.Root.Hash)
	}
	if node == nil {
		fmt.Printf("%snode empty\n", strings.Repeat("  ", level))
		return
	}
	nodeType := "Branch"
	if node.Left == nil && node.Right == nil {
		nodeType = "Leaf"
	}
	fmt.Printf("%s[%s Node] Key: %x, Hash: %x\n", strings.Repeat("  ", level), nodeType, node.Key, node.Hash)
	value, _, _ := t.getFromStorageLocked(node.Key)
	if value != nil {
		fmt.Printf("%s  [Leaf Node] Value: %x\n", strings.Repeat("  ", level), value)
	}
	if node.Left != nil || node.Right != nil {
		fmt.Printf("%s  Left:\n", strings.Repeat("  ", level))
		t.printTree(node.Left, level+1)
		fmt.Printf("%s  Right:\n", strings.Repeat("  ", level))
		t.printTree(node.Right, level+1)
	}
}

func (t *MerkleTree) SetRawKeyVal(key [31]byte, value []byte) {
	t.Insert(key[:], value)
}

// SetStates sets multiple state values in a single batch operation
// mask indicating which states to set
func (t *MerkleTree) SetStates(values [16][]byte) {
	// Get current states to detect changes
	oldStates, _ := t.GetStates()

	// Only insert states that have actually changed
	for i := uint8(0); i < 16; i++ {
		if !bytes.Equal(oldStates[i], values[i]) {
			stateKey := make([]byte, 32)
			stateKey[0] = i + 1 // State indices are 1-16
			t.Insert(stateKey, values[i])
		}
	}
}

// GetStates retrieves all 16 state values
func (t *MerkleTree) GetStates() ([16][]byte, error) {
	var states [16][]byte
	for i := uint8(0); i < 16; i++ {
		stateKey := make([]byte, stateKeySize)
		stateKey[0] = i + 1 // State indices are 1-16
		value, ok, err := t.Get(stateKey)
		if err != nil {
			return states, err
		}
		if ok {
			states[i] = value
		}
	}
	return states, nil
}

// DeleteService (hash)
func (t *MerkleTree) DeleteService(s uint32) {
	service_account := common.ComputeC_is(s)
	stateKey := service_account.Bytes()
	t.Delete(stateKey)
}

func (t *MerkleTree) SetService(s uint32, v []byte) error {
	/*
		∀(s ↦ a) ∈ δ ∶ C(255, s) ↦ a c ⌢E 8 (a b ,a g ,a m ,a l )⌢E 4 (a i )
		i: 255
		s: service_index
		ac: service_accout_code_hash
		ab: service_accout_balance
		ag: service_accout_accumulate_gas
		am: service_accout_on_transfer_gas
		al: see GP_0.35(95)
		ai: see GP_0.35(95)

		(i, s ∈ N S ) ↦ [i, n 0 ,n 1 ,n 2 ,n 3 , 0, 0, . . . ] where n = E 4 (s)
	*/
	service_account := common.ComputeC_is(s)
	stateKey := service_account.Bytes()
	t.Insert(stateKey, v)
	return nil
}

func (t *MerkleTree) GetService(s uint32) ([]byte, bool, error) {
	service_account := common.ComputeC_is(s)
	stateKey := service_account.Bytes()
	value, ok, err := t.Get(stateKey)
	if err != nil {
		return nil, false, fmt.Errorf("GetService Error: %v", err)
	} else if !ok {
		return nil, ok, nil
	}
	return value, true, nil
}

// set a_l (with timeslot if we have E_P). For GP_0.3.5(158)
func (t *MerkleTree) SetPreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32, time_slots []uint32) error {

	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key) // C(s, (h,l))
	stateKey := account_lookuphash.Bytes()
	vBytes, err := types.Encode(time_slots)
	if err != nil {
		fmt.Printf("SetPreImageLookup Encode Error: %v\n", err)
	}
	// Insert the value into the state
	t.Insert(stateKey, vBytes)
	return nil
}

func BytesToTimeSlots(vByte []byte) (time_slots []uint32) {
	if len(vByte) == 0 {
		return make([]uint32, 0)
	}
	vByte = vByte[1:]
	time_slots = make([]uint32, (len(vByte) / 4))
	for i := 0; i < len(time_slots); i++ {
		time_slots[i] = binary.LittleEndian.Uint32(vByte[i*4 : (i+1)*4])
	}
	return
}

// lookup a_l .. returning time slot. For GP_0.3.5(157)
func (t *MerkleTree) GetPreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32) ([]uint32, bool, error) {

	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key) // C(s, (h,l))

	stateKey := account_lookuphash.Bytes()

	vByte, ok, err := t.Get(stateKey)
	if err != nil {
		return nil, ok, err
	} else if !ok {
		return nil, ok, nil
	}
	var time_slots []uint32

	if len(vByte) == 0 {
		time_slots = make([]uint32, 0)
	} else {
		vByte = vByte[1:]
		time_slots = make([]uint32, (len(vByte) / 4))
		for i := 0; i < len(time_slots); i++ {
			time_slots[i] = binary.LittleEndian.Uint32(vByte[i*4 : (i+1)*4])
		}
	}
	return time_slots, ok, err
}

// Delete PreImageLookup key(hash)
func (t *MerkleTree) DeletePreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32) {
	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key) // C(s, (h,l))
	stateKey := account_lookuphash.Bytes()
	t.Delete(stateKey)
}

// Insert Storage Value into the trie
func (t *MerkleTree) SetServiceStorage(s uint32, k []byte, storageValue []byte) error {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()
	t.Insert(stateKey, storageValue)
	return nil
}

func (t *MerkleTree) GetServiceStorage(s uint32, k []byte) ([]byte, bool, error) {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()

	// Get Storage from trie
	value, ok, err := t.Get(stateKey)
	if !ok || err != nil {
		return nil, ok, err
	}
	return value, true, nil
}

// same as above but with proof
func (t *MerkleTree) GetServiceStorageWithProof(s uint32, k []byte) ([]byte, [][]byte, common.Hash, bool, error) {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()

	// Get Storage from trie
	value, ok, err := t.Get(stateKey)
	if !ok || err != nil {
		return nil, nil, common.Hash{}, ok, err
	}
	stateRoot := t.GetRoot()
	proof, err := t.Trace(stateKey)
	if err != nil {
		return nil, nil, common.Hash{}, false, err
	}
	return value, proof, stateRoot, true, nil
}

// Delete Storage key(hash)
func (t *MerkleTree) DeleteServiceStorage(s uint32, k []byte) {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()
	t.Delete(stateKey)
}

// Set PreImage Blob for GP_0.3.5(158)
func (t *MerkleTree) SetPreImageBlob(s uint32, blob []byte) error {
	/*
		∀(s ↦ a) ∈ δ, (h ↦ p) ∈ a p ∶ C(s, h) ↦ p
		(s, h) ↦ [n 0 ,h 0 ,n 1 ,h 1 ,n 2 ,h 2 ,n 3 ,h 3 ,h 4 ,h 5 ,...,h 27 ] where n = E 4 (s)

		s: service_index
		h: blob_hash
		p: blob
	*/

	blobHash := common.Blake2Hash(blob)
	ap_internal_key := common.Compute_preimageBlob_internal(blobHash)
	account_preimage_hash := common.ComputeC_sh(s, ap_internal_key)

	stateKey := account_preimage_hash.Bytes()
	// Insert Preimage Blob into trie
	t.Insert(stateKey, blob)
	return nil
}

func (t *MerkleTree) GetPreImageBlob(s uint32, blobHash common.Hash) (value []byte, ok bool, err error) {
	ap_internal_key := common.Compute_preimageBlob_internal(blobHash)
	account_preimage_hash := common.ComputeC_sh(s, ap_internal_key)
	stateKey := account_preimage_hash.Bytes()
	value, ok, err = t.Get(stateKey)
	if !ok || err != nil {
		return nil, ok, err
	}
	// Get Preimage Blob from trie
	return value, ok, nil
}

// Delete PreImage Blob
func (t *MerkleTree) DeletePreImageBlob(s uint32, blobHash common.Hash) {
	ap_internal_key := common.Compute_preimageBlob_internal(blobHash)
	account_preimage_hash := common.ComputeC_sh(s, ap_internal_key)
	stateKey := account_preimage_hash.Bytes()
	t.Delete(stateKey)
}

// Insert stages the operation without computing hashes or touching levelDB
func (t *MerkleTree) Insert(keyBytes []byte, value []byte) {
	key := normalizeKey32(keyBytes)

	t.stagedMutex.Lock()
	defer t.stagedMutex.Unlock()

	t.stagedInserts[key] = value
	delete(t.stagedDeletes, key) // Cancel any pending delete
}

// insertNode is the internal version that actually performs the insertion with hashing
func (t *MerkleTree) insertNode(node *Node, key, value []byte, depth int) *Node {
	nullNode := Node{Hash: make([]byte, 32)}

	if node == nil || compareBytes(node.Hash, nullNode.Hash) || depth > computeKeyLengthAsBit(key) {
		return &Node{
			Hash: computeHash(leaf(key, value, t)),
			Key:  key,
		}
	}

	if node.Left == nil && node.Right == nil {
		if compareBytes(node.Key, key) {
			node.Hash = computeHash(leaf(key, value, t))
			return node
		}
		return t.createBranchNode(node, key, value, depth)
	}

	if bit(key, depth) {
		node.Right = t.insertNode(node.Right, key, value, depth+1)
	} else {
		node.Left = t.insertNode(node.Left, key, value, depth+1)
	}

	leftHash := make([]byte, 32)
	rightHash := make([]byte, 32)

	if node.Left != nil {
		leftHash = node.Left.Hash
	} else {
		node.Left = &Node{Hash: make([]byte, 32)}
	}

	if node.Right != nil {
		rightHash = node.Right.Hash
	} else {
		node.Right = &Node{Hash: make([]byte, 32)}
	}

	node.Hash = computeHash(branch(leftHash, rightHash))
	var branchData [64]byte
	copy(branchData[:32], leftHash)
	copy(branchData[32:64], rightHash)
	t.levelDBSetBranch(node.Hash, branchData[:])

	return node
}

func (t *MerkleTree) createBranchNode(node *Node, key, value []byte, depth int) *Node {
	existingKey := node.Key
	existingValue, _, _ := t.getFromStorageLocked(node.Key)

	// Clear the key since this is now a branch
	node.Key = nil

	if bit(existingKey, depth) {
		node.Right = &Node{
			Hash: computeHash(leaf(existingKey, existingValue, t)),
			Key:  existingKey,
		}
	} else {
		node.Left = &Node{
			Hash: computeHash(leaf(existingKey, existingValue, t)),
			Key:  existingKey,
		}
	}

	if bit(key, depth) {
		node.Right = t.insertNode(node.Right, key, value, depth+1)
	} else {
		node.Left = t.insertNode(node.Left, key, value, depth+1)
	}

	leftHash := make([]byte, 32)
	rightHash := make([]byte, 32)

	if node.Left != nil {
		leftHash = node.Left.Hash
	} else {
		node.Left = &Node{Hash: make([]byte, 32)}
	}

	if node.Right != nil {
		rightHash = node.Right.Hash
	} else {
		node.Right = &Node{Hash: make([]byte, 32)}
	}

	node.Hash = computeHash(branch(leftHash, rightHash))
	var branchData [64]byte
	copy(branchData[:32], leftHash)
	copy(branchData[32:64], rightHash)
	t.levelDBSetBranch(node.Hash, branchData[:])

	return node
}

func (t *MerkleTree) findNode(node *Node, key []byte, depth int) (*Node, error) {
	if node == nil {
		return nil, errors.New("key not found")
	}
	if compareBytes(node.Key, key) {
		return node, nil
	}
	if bit(key, depth) {
		return t.findNode(node.Right, key, depth+1)
	} else {
		return t.findNode(node.Left, key, depth+1)
	}
}

func (t *MerkleTree) updateTree(node *Node, key, value []byte, depth int) {
	if node == nil || depth > computeKeyLengthAsBit(key) {
		return
	}
	if compareBytes(node.Key, key) {
		node.Hash = computeHash(leaf(key, value, t))
		return
	}
	if bit(key, depth) {
		t.updateTree(node.Right, key, value, depth+1)
	} else {
		t.updateTree(node.Left, key, value, depth+1)
	}
	leftHash := make([]byte, 32)
	rightHash := make([]byte, 32)
	if node.Left != nil {
		leftHash = node.Left.Hash
	} else {
		node.Left = &Node{Hash: make([]byte, 32)}
	}
	if node.Right != nil {
		rightHash = node.Right.Hash
	} else {
		node.Right = &Node{Hash: make([]byte, 32)}
	}

	node.Hash = computeHash(branch(leftHash, rightHash))
	var branchData [64]byte
	copy(branchData[:32], leftHash)
	copy(branchData[32:64], rightHash)
	t.levelDBSetBranch(node.Hash, branchData[:])
}

// Get retrieves the value - checks staged operations first, then committed tree
func (t *MerkleTree) Get(keyBytes []byte) ([]byte, bool, error) {
	key := normalizeKey32(keyBytes)

	// First check staged operations
	t.stagedMutex.Lock()

	// Check if key is staged for deletion
	if t.stagedDeletes[key] {
		t.stagedMutex.Unlock()
		return nil, false, nil
	}

	// Check if key has a staged insert
	if value, exists := t.stagedInserts[key]; exists {
		t.stagedMutex.Unlock()
		return value, true, nil
	}
	t.stagedMutex.Unlock()

	// Not in staged operations, check committed tree
	t.treeMutex.RLock()
	defer t.treeMutex.RUnlock()
	return t.getFromStorageLocked(key[:])
}

// getFromStorageLocked retrieves value from the committed tree (assumes caller holds tree lock - R or W)
func (t *MerkleTree) getFromStorageLocked(key []byte) ([]byte, bool, error) {
	value, ok, err := t.getValue(t.Root, key, 0)
	if err != nil {
		return nil, ok, err
	} else if !ok {
		return nil, ok, nil
	}
	return value, true, nil
}

// getFromStorage is the public version that acquires the lock
func (t *MerkleTree) getFromStorage(key []byte) ([]byte, bool, error) {
	t.treeMutex.RLock()
	defer t.treeMutex.RUnlock()
	return t.getFromStorageLocked(key)
}

func (t *MerkleTree) GetValue(key []byte) ([]byte, error) {
	value, ok, err := t.Get(key)
	if err != nil || !ok {
		return nil, fmt.Errorf("GetValue key not found: %x", key)
	}
	return value, err
}

func (t *MerkleTree) getValue(node *Node, key []byte, depth int) ([]byte, bool, error) {
	if node == nil || depth > computeKeyLengthAsBit(key) {
		return nil, false, nil
	}

	if compareBytes(node.Key, key) {
		leafValue, okLeaf, errLeaf := t.getDBLeafValue(node.Hash)
		if errLeaf != nil {
			return nil, false, fmt.Errorf("GetValue: Error %v", errLeaf)
		} else if !okLeaf {
			return nil, false, nil
		} else if leafValue != nil {
			return leafValue, true, nil
		}
		return nil, false, fmt.Errorf("unexpected error: key:%x", key)
	}

	if bit(key, depth) {
		return t.getValue(node.Right, key, depth+1)
	} else {
		return t.getValue(node.Left, key, depth+1)
	}
}

// Trace traces the path to a specific key in the Merkle Tree and returns the sibling hashes along the path
func (t *MerkleTree) Trace(keyBytes []byte) ([][]byte, error) {
	key := normalizeKey32(keyBytes)

	t.treeMutex.RLock()
	defer t.treeMutex.RUnlock()

	if t.Root == nil {
		return nil, errors.New("empty tree")
	}
	var path [][]byte
	err := t.tracePath(t.Root, key[:], 0, &path)
	if err != nil {
		return nil, err
	}
	return path, nil
}

func (t *MerkleTree) tracePath(node *Node, key []byte, depth int, path *[][]byte) error {
	if node == nil || depth > computeKeyLengthAsBit(key) {
		return errors.New("key not found")
	}

	if compareBytes(node.Key, key) {
		return nil
	}

	if bit(key, depth) {
		if node.Left != nil {
			*path = append(*path, node.Left.Hash)
		} else {
			*path = append(*path, make([]byte, 32))
		}
		return t.tracePath(node.Right, key, depth+1, path)
	} else {
		if node.Right != nil {
			*path = append(*path, node.Right.Hash)
		} else {
			*path = append(*path, make([]byte, 32))
		}
		return t.tracePath(node.Left, key, depth+1, path)
	}
}

func (t *MerkleTree) GetPath(keyBytes []byte) ([][]byte, error) {
	key := normalizeKey32(keyBytes)

	t.treeMutex.RLock()
	defer t.treeMutex.RUnlock()
	return t.getPathLocked(key[:])
}

func (t *MerkleTree) getPathLocked(key []byte) ([][]byte, error) {
	if t.Root == nil {
		return nil, errors.New("empty tree")
	}
	var path [][]byte
	err := t.getPath(t.Root, key, 0, &path)
	if err != nil {
		return nil, err
	}
	return path, nil
}

func (t *MerkleTree) getPath(node *Node, key []byte, depth int, path *[][]byte) error {
	if node == nil || depth > computeKeyLengthAsBit(key) {
		return errors.New("key not found")
	}

	if compareBytes(node.Key, key) {
		*path = append(*path, node.Hash)
		return nil
	}

	if bit(key, depth) {
		if node.Hash != nil {
			*path = append(*path, node.Hash)
		} else {
			*path = append(*path, make([]byte, 32))
		}
		return t.getPath(node.Right, key, depth+1, path)
	} else {
		if node.Hash != nil {
			*path = append(*path, node.Hash)
		} else {
			*path = append(*path, make([]byte, 32))
		}
		return t.getPath(node.Left, key, depth+1, path)
	}
}

// leafStandalone is a standalone version of leaf for use in verification functions
// that don't have access to a MerkleTree instance
func leafStandalone(k, v []byte) []byte {
	var result [64]byte

	// Embedded-value leaf node
	if len(v) <= 32 {
		result[0] = byte(0b10000000 | len(v))
		if len(k) > 31 {
			copy(result[1:32], k[:31])
		} else {
			copy(result[1:1+len(k)], k)
		}
		copy(result[32:32+len(v)], v)
	} else {
		// Regular leaf node
		result[0] = byte(0b11000000)
		if len(k) > 31 {
			copy(result[1:32], k[:31])
		} else {
			copy(result[1:1+len(k)], k)
		}
		hash := computeHash(v)
		copy(result[32:64], hash)
	}
	return result[:]
}

// Verify verifies the path to a specific key in the Merkle Tree
func Verify(serviceID uint32, key []byte, value []byte, rootHash []byte, path []common.Hash) bool {
	// Compute opaque key from service ID and key
	opaqueKey := common.Compute_storage_opaqueKey(serviceID, key)

	if len(path) == 0 {
		leafHashSingle := computeHash(leafStandalone(opaqueKey, value))
		return compareBytes(leafHashSingle, rootHash)
	}

	leafHash := computeHash(leafStandalone(opaqueKey, value))

	for i := len(path) - 1; i >= 0; i-- {
		if bit(opaqueKey, i) {
			leafHash = computeHash(branch(path[i][:], leafHash))
		} else {
			leafHash = computeHash(branch(leafHash, path[i][:]))
		}
	}
	return compareBytes(leafHash, rootHash)
}

// VerifyRaw verifies a Merkle proof for a raw key (without service ID encoding)
// Used primarily for testing low-level trie operations
func VerifyRaw(key []byte, value []byte, rootHash []byte, path []common.Hash) bool {
	if len(path) == 0 {
		leafHashSingle := computeHash(leafStandalone(key, value))
		return compareBytes(leafHashSingle, rootHash)
	}

	leafHash := computeHash(leafStandalone(key, value))

	for i := len(path) - 1; i >= 0; i-- {
		if bit(key, i) {
			leafHash = computeHash(branch(path[i][:], leafHash))
		} else {
			leafHash = computeHash(branch(leafHash, path[i][:]))
		}
	}
	return compareBytes(leafHash, rootHash)
}

// Delete stages a deletion without modifying the tree
func (t *MerkleTree) Delete(keyBytes []byte) {
	key := normalizeKey32(keyBytes)

	t.stagedMutex.Lock()
	defer t.stagedMutex.Unlock()

	t.stagedDeletes[key] = true
	delete(t.stagedInserts, key) // Cancel any pending insert
}

// deleteNode is the internal version that actually performs the deletion
func (t *MerkleTree) deleteNode(node *Node, key []byte, depth int) *Node {
	if node == nil || depth > computeKeyLengthAsBit(key) {
		return node
	}

	// Found the leaf to delete
	if compareBytes(node.Key, key) {
		return nil
	}

	// Not a leaf, recurse
	if bit(key, depth) {
		node.Right = t.deleteNode(node.Right, key, depth+1)
	} else {
		node.Left = t.deleteNode(node.Left, key, depth+1)
	}

	// After deletion, check if this branch can be collapsed
	if node.Left == nil && node.Right == nil {
		return nil
	}
	if node.Left == nil {
		return node.Right
	}
	if node.Right == nil {
		return node.Left
	}

	// Recompute branch hash
	node.Hash = computeHash(branch(node.Left.Hash, node.Right.Hash))
	var branchData [64]byte
	copy(branchData[:32], node.Left.Hash)
	copy(branchData[32:64], node.Right.Hash)
	t.levelDBSetBranch(node.Hash, branchData[:])

	return node
}

// compareBytes compares two Tries
func CompareTrees(node1, node2 *Node) bool {
	return compareTrees(node1, node2)
}

func compareTrees(node1, node2 *Node) bool {
	if node1 == nil && node2 == nil {
		return true
	}
	if node1 == nil && node2 != nil {
		fmt.Printf("Node1 empty. Node2 Not Empty\n")
		fmt.Printf("Node1 <nil>\n")
		fmt.Printf("Node2 %v\n", node2.String())
		return false
	}
	if node1 != nil && node2 == nil {
		fmt.Printf("Node1 Not empty. Node2 Empty\n")
		fmt.Printf("Node1 %v\n", node1.String())
		fmt.Printf("Node2 <nil>\n")
		return false
	}
	if !compareBytes(node1.Hash, node2.Hash) {
		fmt.Printf("Node Hash Mismatch N1=%x N2=%x\n", node1.Hash, node2.Hash)
		fmt.Printf("Node1 %v\n", node1.String())
		fmt.Printf("Node2 %v\n", node2.String())
		return false
	}
	if !compareBytes(node1.Key, node2.Key) {
		fmt.Printf("Node Key Mismatch N1=%x N2=%x\n", node1.Key, node2.Key)
		fmt.Printf("Node1 %v\n", node1.String())
		fmt.Printf("Node2 %v\n", node2.String())
		return false
	}
	return compareTrees(node1.Left, node2.Left) && compareTrees(node1.Right, node2.Right)
}

// Compute the key length in bits. byte -> 8bits
func computeKeyLengthAsBit(key []byte) int {
	return len(key) * 8
}

func sortKeyValsByKey(tmpKeyVals []KeyVal) []KeyVal {
	sort.Slice(tmpKeyVals, func(i, j int) bool {
		return bytes.Compare(tmpKeyVals[i].Key, tmpKeyVals[j].Key) < 0
	})
	return tmpKeyVals
}
