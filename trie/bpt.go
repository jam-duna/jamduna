package trie

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

const (
	debug        = "trie"
	stateKeySize = 32 // the "actual" size is 31 but we use common.Hash with the 32 byte being 0 INCLUDING IN METADATA right now
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

// TODO: stanley to figure what this is
type BMTProof []common.Hash

// Node represents a node in the Merkle Tree
type Node struct {
	Hash  []byte
	Key   []byte
	Left  *Node
	Right *Node
}

// state-key constructor functions C(X)
const (
	C1  = "CoreAuthPool"
	C2  = "AuthQueue"
	C3  = "RecentBlocks"
	C4  = "safroleState"
	C5  = "PastJudgements"
	C6  = "Entropy"
	C7  = "NextEpochValidatorKeys"
	C8  = "CurrentValidatorKeys"
	C9  = "PriorEpochValidatorKeys"
	C10 = "PendingReports"
	C11 = "MostRecentBlockTimeslot"
	C12 = "PrivilegedServiceIndices"
	C13 = "ActiveValidator"
	C14 = "AccumulationQueue"
	C15 = "AccumulationHistory"
	C16 = "AccumulationOutputs"
)

const (
	LevelDBNull  = "null"
	LevelDBEmpty = ""
)

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
	Root *Node
	// levelDBMap map[string][]byte // <>
	db *storage.StateDBStorage
}

// NewMerkleTree creates a new Merkle Tree from the provided data
func initLevelDB(optionalPath ...string) (*storage.StateDBStorage, error) {
	path := "/tmp/log/leveldb/bpt"
	if len(optionalPath) > 0 {
		path = optionalPath[0]
	}
	stateDBStorage, err := storage.NewStateDBStorage(path)
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

// NewMerkleTree creates a new Merkle Tree from the provided data
func NewMerkleTreeWithPath(data [][2][]byte, optionalPath ...string) *MerkleTree {
	path := "/tmp/log/leveldb/bpt"
	if len(optionalPath) > 0 {
		path = optionalPath[0]
	}
	db, err := initLevelDB(path)
	if err != nil {
		fmt.Printf("levelDB ERR: %v", err)
	}
	if data == nil || len(data) == 0 {
		return &MerkleTree{Root: nil, db: db}
	}
	root := buildMerkleTree(data, 0)
	return &MerkleTree{Root: root, db: db}
}

func NewMerkleTree(data [][2][]byte, db *storage.StateDBStorage) *MerkleTree {
	if data == nil || len(data) == 0 {
		return &MerkleTree{Root: nil, db: db}
	}
	root := buildMerkleTree(data, 0)
	return &MerkleTree{Root: root, db: db}
}

// buildMerkleTree constructs the Merkle tree from key-value pairs
func buildMerkleTree(kvs [][2][]byte, i int) *Node {
	// Base Case - Empty Data |d| = 0
	if len(kvs) == 0 {
		return &Node{Hash: make([]byte, 32)}
	}
	// V(d) = {(K,v)}
	if len(kvs) == 1 {
		encoded := leaf(kvs[0][0], kvs[0][1])
		//TODO: we should store (Hash, Value) in levelDB for future lookup
		//computeHash(encoded) -> kvs[0][1]
		//kvs[0][1] -> computeHash(encoded) X NOT like this
		//will only store the value if less than 32 bytes
		return &Node{Hash: computeHash(encoded), Key: kvs[0][0]}
	}
	// Recursive Case: B(M(l),M(r))
	var l, r [][2][]byte
	for _, kv := range kvs {
		if bit(kv[0], i) {
			r = append(r, kv)
		} else {
			l = append(l, kv)
		}
	}

	left := buildMerkleTree(l, i+1)
	right := buildMerkleTree(r, i+1)
	encoded := branch(left.Hash, right.Hash)
	return &Node{Hash: computeHash(encoded), Left: left, Right: right}
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
func leaf(k, v []byte) []byte {
	// Embedded-value leaf node
	if len(v) <= 32 {
		head := byte(0b10000000 | len(v))
		tmpk := make([]byte, len(k))
		copy(tmpk, k)
		if len(tmpk) > 31 {
			tmpk = tmpk[:31]
		} else {
			tmpk = append(tmpk, make([]byte, 31-len(tmpk))...)
		}
		value := append(v, make([]byte, 32-len(v))...)
		return append([]byte{head}, append(tmpk, value...)...)
	} else {
		// Regular leaf node
		head := byte(0b11000000)
		tmpk := make([]byte, len(k))
		copy(tmpk, k)
		if len(tmpk) > 31 {
			tmpk = tmpk[:31]
		} else {
			tmpk = append(tmpk, make([]byte, 31-len(tmpk))...)
		}
		hash := computeHash(v)
		return append([]byte{head}, append(tmpk, hash...)...)
	}
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
	if t.Root == nil {
		return common.BytesToHash(make([]byte, 32))
	}
	return common.BytesToHash(t.Root.Hash)
}

// GetRootHash returns the root hash of the Merkle Tree
func (t *MerkleTree) GetRootHash() []byte {
	if t.Root == nil {
		return make([]byte, 32)
	}
	return t.Root.Hash
}

func InitMerkleTreeFromHash(root []byte, db *storage.StateDBStorage) (*MerkleTree, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	tree := &MerkleTree{Root: nil, db: db}
	if compareBytes(root, common.Hex2Bytes("0000000000000000000000000000000000000000000000000000000000000000")) {
		return &MerkleTree{Root: nil, db: db}, nil
	}
	rootNode, err := tree.levelDBGetNode(root)
	if err != nil {
		return nil, err
	}
	tree.Root = rootNode
	return tree, nil
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

func (t *MerkleTree) levelDBGetBranch(branchHash []byte) (*Node, error) {
	value, ok, err := t.levelDBGet(branchHash)
	if err != nil || !ok {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("value is nil for key: %s", branchHash)
	}
	if len(value) < 38 {
		return nil, fmt.Errorf("value is too short, expected at least 38 bytes but got %d bytes", len(value))
	}
	leftHash := make([]byte, 32)
	copy(leftHash, value[6:38])
	rightHash := value[38:]
	copy(rightHash, value[38:])

	leftNode, err := t.levelDBGetNode(leftHash)
	if err != nil {
		return nil, err
	}

	rightNode, err := t.levelDBGetNode(rightHash)
	if err != nil {
		return nil, err
	}

	n := Node{
		Hash:  branchHash,
		Left:  leftNode,
		Right: rightNode,
	}
	return &n, nil
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

func (t *MerkleTree) levelDBGetLeaf(nodeHash []byte) ([]byte, bool, error) {
	encodedLeaf, ok, err := t.levelDBGet(nodeHash)
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
		return t.levelDBGet(_v)
	}
}

// levelDBSet sets the value for the given key in the levelDBMap
func (t *MerkleTree) levelDBSet(k, v []byte) error {
	if t.db == nil {
		return fmt.Errorf("database is not initialized")
	}
	err := t.db.WriteRawKV(k, v)
	if err != nil {
		return fmt.Errorf("failed to set key %s: %v", k, err)
	}
	return nil
}

func (t *MerkleTree) LevelDBGet(k []byte) ([]byte, bool, error) {
	return t.levelDBGet(k)
}

// levelDBGet gets the value for the given key from the levelDBMap
func (t *MerkleTree) levelDBGet(k []byte) ([]byte, bool, error) {
	if t.db == nil {
		return nil, false, fmt.Errorf("database is not initialized")
	}
	//value, err := t.db.Get(k, nil)
	value, ok, err := t.db.ReadRawKV(k)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get key [%s]: %v", k, err)
	} else if !ok {
		return nil, false, nil
	}
	return value, true, nil
}

func (t *MerkleTree) levelDBGetNode(nodeHash []byte) (*Node, error) {
	//value, _ := t.db.Get([]byte(nodeHash), nil)
	value, _, _ := t.db.ReadRawKV(nodeHash)
	zeroHash := make([]byte, 32)
	if compareBytes(nodeHash, zeroHash) || value == nil {
		return &Node{
			Hash: zeroHash,
		}, nil
	}
	leafKey, _, _ := t.levelDBGetLeaf(nodeHash)
	if leafKey != nil {
		leafValue, _, _ := t.db.ReadRawKV(computeHash([]byte(append(nodeHash, leafKey...))))
		//leafValue, _ := t.db.Get([]byte(append(nodeHash, leafKey...)), nil)
		return &Node{
			Hash: nodeHash,
			Key:  leafValue,
		}, nil
	}

	if !compareBytes(value, zeroHash) {
		return t.levelDBGetBranch(nodeHash)
	} else {
		return &Node{
			Hash: zeroHash,
		}, nil
	}
}

// Close closes the levelDB connection
func (t *MerkleTree) Close() error {
	if t.db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return t.db.Close()
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
		metaKey := fmt.Sprintf("meta_%x", realKey)
		metaKeyBytes, err := types.Encode(metaKey)
		if err != nil {
			fmt.Printf("PrintAllKeyValues Encode Error: %v\n", err)
		}
		if err != nil {
			fmt.Printf("PrintAllKeyValues Encode Error: %v\n", err)
		}
		metaValueBytes, ok, err := t.levelDBGet(metaKeyBytes)
		if err != nil {
			fmt.Printf("PrintAllKeyValues levelDBGet Error: %v\n", err)
			return
		} else if !ok {
			fmt.Printf("PrintAllKeyValues Key not found: %v\n", metaKey)
			return
		}
		metaValueDecode, _, err := types.Decode(metaValueBytes, reflect.TypeOf(""))
		if err != nil {
			fmt.Printf("PrintAllKeyValues Decode Error: %v\n", err)
			return
		}
		metaValue := metaValueDecode.(string)
		metaValues := strings.SplitN(metaValue, "|", 2)

		keyVal := KeyVal{
			Key:        realKey,
			Value:      realValue,
			StructType: metaValues[0],
			Metadata:   metaValues[1],
		}
		keyVals = append(keyVals, keyVal)
	}
	sortedKeyVals := sortKeyValsByKey(keyVals)

	fmt.Printf("GetAllKeyValues right after %v\n", sortedKeyVals)
	// for _, kv := range keyVals.KeyVals {
	// 	fmt.Printf("[Key] %x\n[Value] %x\n", kv[0], kv[1])
	// }
}

func (t *MerkleTree) PrintTree(node *Node, level int) {
	fmt.Printf("\n----------------PrintTree START----------------\n")
	t.printTree(node, level)
	fmt.Printf("\n----------------PrintTree END----------------\n")
}

// maximum size: The total encoded length of the response
// TODO: this should accept 31 byte keys and return 31 byte keys
func (t *MerkleTree) GetStateByRange(starKey []byte, endKey []byte, maxSize uint32) (foundKeyVal []types.StateKeyValue, boundaryNode [][]byte, err error) {
	foundKeyVal = make([]types.StateKeyValue, 0)
	currenSize := uint32(0)
	paddedStart := make([]byte, 32)
	copy(paddedStart, starKey)
	value, _, _ := t.Get(paddedStart)
	found := false
	end := false
	if value != nil {
		t.getTreeContentIncludeKey(t.Root, 0, starKey, endKey, &currenSize, maxSize, &foundKeyVal, &boundaryNode, &found, &end)
	} else {
		t.getTreeContent(t.Root, 0, starKey, endKey, &currenSize, maxSize, &foundKeyVal, &boundaryNode)
	}
	return foundKeyVal, boundaryNode, err
}

func (t *MerkleTree) getTreeContentIncludeKey(node *Node, level int, starKey []byte, endKey []byte, currenSize *uint32, maxSize uint32, foundkv *[]types.StateKeyValue, boundaryNodes *[][]byte, findKey *bool, end *bool) (ok bool) {
	if *currenSize >= maxSize || *end {
		return true
	}
	nodeType := "Branch"
	if node.Left == nil && node.Right == nil {
		nodeType = "Leaf"
	}
	*boundaryNodes = append(*boundaryNodes, node.Hash)

	value, _, _ := t.Get(node.Key)

	// Copy the original key into the new slice
	if value != nil {
		paddedStart := make([]byte, len(node.Key))
		paddedEnd := make([]byte, len(node.Key))
		// Copy the original key into the new slice
		copy(paddedStart, starKey)
		copy(paddedEnd, endKey)

		if common.CompareBytes(node.Key, paddedStart) {
			fmt.Printf("Found Key: %x\n", node.Key)
			*boundaryNodes = make([][]byte, 0)
			*boundaryNodes, _ = t.GetPath(node.Key)
			*boundaryNodes = append(*boundaryNodes, node.Hash)
			*findKey = true
		}
		if *findKey {
			if (common.CompareKeys(paddedStart, node.Key) >= 0 && common.CompareKeys(paddedEnd, node.Key) >= 0) && (nodeType == "Leaf") {
				// check if the key is within range..
				// key is expected to 31 bytes

				var k_31 [31]byte
				copy(k_31[:], node.Key[:31])
				if len(k_31) != 31 {
					log.Crit(debug, "branch: input hashes must be 32 bytes")
				}
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
		t.getTreeContentIncludeKey(node.Left, level+1, starKey, endKey, currenSize, maxSize, foundkv, boundaryNodes, findKey, end)
		t.getTreeContentIncludeKey(node.Right, level+1, starKey, endKey, currenSize, maxSize, foundkv, boundaryNodes, findKey, end)
	}
	return true
}

func (t *MerkleTree) getTreeContent(node *Node, level int, starKey []byte, endKey []byte, currenSize *uint32, maxSize uint32, foundkv *[]types.StateKeyValue, boundaryNodes *[][]byte) (ok bool) {
	if *currenSize >= maxSize {
		return true
	}
	nodeType := "Branch"
	if node.Left == nil && node.Right == nil {
		nodeType = "Leaf"
	}
	*boundaryNodes = append(*boundaryNodes, node.Hash)
	value, _, _ := t.Get(node.Key)
	if value != nil {
		paddedStart := make([]byte, len(node.Key))
		paddedEnd := make([]byte, len(node.Key))
		// Copy the original key into the new slice
		copy(paddedStart, starKey)
		copy(paddedEnd, endKey)
		if nodeType == "Leaf" {
			// check if the key is within range..
			// key is expected to 31 bytes
			var k_31 [31]byte
			copy(k_31[:], node.Key[:31])
			if len(k_31) != 31 {
				log.Crit(debug, "Key is not 31 bytes")
			}
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
		t.getTreeContent(node.Left, level+1, starKey, endKey, currenSize, maxSize, foundkv, boundaryNodes)
		t.getTreeContent(node.Right, level+1, starKey, endKey, currenSize, maxSize, foundkv, boundaryNodes)
	}
	return true
}

func (t *MerkleTree) GetRealKey(key [31]byte, value []byte) []byte {
	encodedLeaf := leaf(key[:], value)
	nodeHash := computeHash(encodedLeaf)

	leafKey, _, _ := t.levelDBGetLeaf(nodeHash)
	if leafKey != nil {
		realKey, _, _ := t.db.ReadRawKV(computeHash([]byte(append(nodeHash, leafKey...))))
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
	value, _, _ := t.Get(node.Key)
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

func (t *MerkleTree) SetState(_stateIdentifier string, value []byte) {
	stateKey := make([]byte, 32)

	switch _stateIdentifier {
	case C1:
		stateKey[0] = 0x01
	case C2:
		stateKey[0] = 0x02
	case C3:
		stateKey[0] = 0x03
	case C4:
		stateKey[0] = 0x04
	case C5:
		stateKey[0] = 0x05
	case C6:
		stateKey[0] = 0x06
	case C7:
		stateKey[0] = 0x07
	case C8:
		stateKey[0] = 0x08
	case C9:
		stateKey[0] = 0x09
	case C10:
		stateKey[0] = 0x0A
	case C11:
		stateKey[0] = 0x0B
	case C12:
		stateKey[0] = 0x0C
	case C13:
		stateKey[0] = 0x0D
	case C14:
		stateKey[0] = 0x0E
	case C15:
		stateKey[0] = 0x0F
	case C16:
		stateKey[0] = 0x10
	}
	t.Insert(stateKey, value)
}

func (t *MerkleTree) GetState(_stateIdentifier string) ([]byte, error) {
	stateKey := make([]byte, stateKeySize)

	switch _stateIdentifier {
	case C1:
		stateKey[0] = 0x01
	case C2:
		stateKey[0] = 0x02
	case C3:
		stateKey[0] = 0x03
	case C4:
		stateKey[0] = 0x04
	case C5:
		stateKey[0] = 0x05
	case C6:
		stateKey[0] = 0x06
	case C7:
		stateKey[0] = 0x07
	case C8:
		stateKey[0] = 0x08
	case C9:
		stateKey[0] = 0x09
	case C10:
		stateKey[0] = 0x0A
	case C11:
		stateKey[0] = 0x0B
	case C12:
		stateKey[0] = 0x0C
	case C13:
		stateKey[0] = 0x0D
	case C14:
		stateKey[0] = 0x0E
	case C15:
		stateKey[0] = 0x0F
	case C16:
		stateKey[0] = 0x10
	}
	value, ok, err := t.Get(stateKey)
	if !ok || err != nil {
		// fmt.Printf("GetState stateKey=%x Error %v, %v\n", stateKey, ok, err)
	}
	return value, err
}

func (t *MerkleTree) SetService(s uint32, v []byte) (err error) {
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

	metaKey := fmt.Sprintf("meta_%x", stateKey)
	metaKeyBytes, err := types.Encode(metaKey)
	if err != nil {
		fmt.Printf("SetService metaKey Encode Error: %v\n", err)
	}

	metaVal := fmt.Sprintf("service_account|s=%d", s)
	metaValBytes, err := types.Encode(metaVal)
	if err != nil {
		fmt.Printf("SetService metaValBytes Encode Error: %v\n", err)
	}
	t.levelDBSet(metaKeyBytes, metaValBytes)
	t.Insert(stateKey, v)
	return nil // TODO
}

func (t *MerkleTree) GetService(s uint32) ([]byte, bool, error) {
	service_account := common.ComputeC_is(s)
	stateKey := service_account.Bytes()
	value, ok, err := t.Get(stateKey)
	if err != nil {
		return nil, false, fmt.Errorf("GetService Error: %v", err) //Need to differentiate not found vs leveldb error
	} else if !ok {
		return nil, ok, nil
	}

	return value, true, nil
}

// set a_l (with timeslot if we have E_P). For GP_0.3.5(158)
func (t *MerkleTree) SetPreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32, time_slots []uint32) (err error) {

	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key) // C(s, (h,l))
	stateKey := account_lookuphash.Bytes()

	vBytes, err := types.Encode(time_slots)
	if err != nil {
		fmt.Printf("SetPreImageLookup Encode Error: %v\n", err)
	}
	// Insert the value into the state

	metaKey := fmt.Sprintf("meta_%x", stateKey)
	metaKeyBytes, err := types.Encode(metaKey)
	if err != nil {
		fmt.Printf("SetPreImageLookup Encode Error: %v\n", err)
	}
	metaVal := fmt.Sprintf("account_lookup|s=%d|h=%s l=%d", s, blob_hash, blob_len)
	metaValBytes, err := types.Encode(metaVal)
	if err != nil {
		fmt.Printf("SetPreImageLookup metaValBytes Encode Error: %v\n", err)
	}
	t.levelDBSet(metaKeyBytes, metaValBytes)
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

	/*
		Follow GP_0.3.5(270, 273, 274, 276, 291)
		Process State value(timeslots), covert []uint32 to []byte
	*/

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
func (t *MerkleTree) DeletePreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32) error {

	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key) // C(s, (h,l))
	stateKey := account_lookuphash.Bytes()

	err := t.Delete(stateKey)

	return err
}

// Insert Storage Value into the trie
func (t *MerkleTree) SetServiceStorage(s uint32, k []byte, storageValue []byte) (err error) {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()

	metaKey := fmt.Sprintf("meta_%x", stateKey)
	metaKeyBytes, err := types.Encode(metaKey)
	if err != nil {
		fmt.Printf("SetServiceStorage Encode Error: %v\n", err)
	}
	metaVal := fmt.Sprintf("account_storage|s=%d|hk=%s k=%s", s, account_storage_key, k)
	metaValBytes, err := types.Encode(metaVal)
	if err != nil {
		fmt.Printf("SetServiceStorage metaValBytes Encode Error: %v\n", err)
	}
	t.levelDBSet(metaKeyBytes, metaValBytes)
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

// Delete Storage key(hash)
func (t *MerkleTree) DeleteServiceStorage(s uint32, k []byte) error {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()
	err := t.Delete(stateKey)
	return err
}

// Set PreImage Blob for GP_0.3.5(158)
func (t *MerkleTree) SetPreImageBlob(s uint32, blob []byte) (err error) {
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

	metaKey := fmt.Sprintf("meta_%x", stateKey)
	metaKeyBytes, err := types.Encode(metaKey)
	if err != nil {
		fmt.Printf("SetPreImageBlob Encode Error: %v\n", err)
	}
	metaVal := fmt.Sprintf("account_preimage|s=%d|h=%v|plen=%d", s, blobHash, len(blob))
	metaValBytes, err := types.Encode(metaVal)
	if err != nil {
		fmt.Printf("SetPreImageBlob metaValBytes Encode Error: %v\n", err)
	}
	t.levelDBSet(metaKeyBytes, metaValBytes)
	// Insert Preimage Blob into trie
	t.Insert(stateKey, blob)
	return
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
func (t *MerkleTree) DeletePreImageBlob(s uint32, blobHash common.Hash) error {
	ap_internal_key := common.Compute_preimageBlob_internal(blobHash)
	account_preimage_hash := common.ComputeC_sh(s, ap_internal_key)

	stateKey := account_preimage_hash.Bytes()
	err := t.Delete(stateKey)
	return err
}

// Insert fixed-length hashed key with value for the BPT
func (t *MerkleTree) Insert(key31 []byte, value []byte) {
	key := make([]byte, 32)
	copy(key[:], key31[:])
	node, err := t.findNode(t.Root, key, 0)
	if err != nil {
		encodedLeaf := leaf(key, value)
		t.levelDBSetLeaf(encodedLeaf, value, key)
		if t.Root == nil {
			t.Root = &Node{
				Hash: computeHash(encodedLeaf),
				Key:  key,
			}

		} else {
			t.Root = t.insertNode(t.Root, key, value, 0)
		}
	} else {
		encodedLeaf := leaf(key, value)
		t.levelDBSetLeaf(encodedLeaf, value, key)
		t.updateNode(node, key, value)
	}
}

func (t *MerkleTree) insertNode(node *Node, key, value []byte, depth int) *Node {
	nullNode := Node{Hash: make([]byte, 32)}

	if node == nil || compareBytes(node.Hash, nullNode.Hash) || depth > computeKeyLengthAsBit(key) {
		return &Node{
			Hash: computeHash(leaf(key, value)),
			Key:  key,
		}
	}

	if node.Left == nil && node.Right == nil {
		if compareBytes(node.Key, key) {
			node.Hash = computeHash(leaf(key, value))
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
	t.levelDBSetBranch(node.Hash, append(leftHash, rightHash...))
	return node
}

func (t *MerkleTree) createBranchNode(node *Node, key, value []byte, depth int) *Node {
	existingKey := node.Key
	//existingValue, _ := t.GetValue_stanley(node.Key) // new but wrong
	existingValue, _, _ := t.Get(node.Key)

	// why do you need to null here?
	node.Key = nil

	if bit(existingKey, depth) {
		node.Right = &Node{
			Hash: computeHash(leaf(existingKey, existingValue)),
			Key:  existingKey,
		}
	} else {
		node.Left = &Node{
			Hash: computeHash(leaf(existingKey, existingValue)),
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
	t.levelDBSetBranch(node.Hash, append(leftHash, rightHash...))
	return node
}

func (t *MerkleTree) Modify(key, value []byte) error {
	node, err := t.findNode(t.Root, key, 0)
	if err != nil {
		return err
	}
	nodeHash := computeHash(leaf(key, value))
	t.levelDBSet(nodeHash, value)
	t.updateNode(node, key, value)
	return nil
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

func (t *MerkleTree) updateNode(node *Node, key, value []byte) {
	node.Hash = computeHash(leaf(key, value))
	t.updateTree(t.Root, key, value, 0)
}

func (t *MerkleTree) updateTree(node *Node, key, value []byte, depth int) {
	if node == nil || depth > computeKeyLengthAsBit(key) {
		return
	}
	if compareBytes(node.Key, key) {
		node.Hash = computeHash(leaf(key, value))
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
	t.levelDBSetBranch(node.Hash, append(leftHash, rightHash...))
}

// Get retrieves the value of a specific key in the Merkle Tree
// Add ok for detecting if the key is found or not
func (t *MerkleTree) Get(key []byte) ([]byte, bool, error) {
	value, ok, err := t.getValue(t.Root, key, 0)
	if err != nil {
		return nil, ok, err
	} else if !ok {
		return nil, ok, nil
	}
	return value, true, nil
}

func (t *MerkleTree) GetValue(key []byte) ([]byte, error) {
	value, ok, err := t.getValue(t.Root, key, 0)
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
		valueLeaf, okLeaf, errLeaf := t.levelDBGetLeaf(node.Hash)
		if errLeaf != nil {
			return nil, false, fmt.Errorf("GetValue: Error %v", errLeaf)
		} else if !okLeaf {
			return nil, false, nil
		} else if valueLeaf != nil {
			return valueLeaf, true, nil
		}

		if t.db != nil {
			valueRaw, okRaw, errRaw := t.db.ReadRawKV(node.Key)
			if errRaw != nil {
				return nil, false, fmt.Errorf("ReadRawKV: Error %v", errRaw)
			} else if !okRaw {
				return nil, false, nil
			} else if valueRaw != nil {
				return valueRaw, true, nil
			}
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
func (t *MerkleTree) Trace(key []byte) ([][]byte, error) {
	if t.Root == nil {
		return nil, errors.New("empty tree")
	}
	var path [][]byte
	err := t.tracePath(t.Root, key, 0, &path)
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

func (t *MerkleTree) GetPath(key []byte) ([][]byte, error) {
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
		return t.tracePath(node.Right, key, depth+1, path)
	} else {
		if node.Hash != nil {
			*path = append(*path, node.Hash)
		} else {
			*path = append(*path, make([]byte, 32))
		}
		return t.tracePath(node.Left, key, depth+1, path)
	}
}

// Verify verifies the path to a specific key in the Merkle Tree
func (t *MerkleTree) Verify(key []byte, value []byte, rootHash []byte, path [][]byte) bool {
	if len(path) == 0 {
		return compareBytes(computeHash(leaf(key, value)), rootHash)
	}

	leafHash := computeHash(leaf(key, value))

	for i := len(path) - 1; i >= 0; i-- {
		if bit(key, i) {
			leafHash = computeHash(branch(path[i], leafHash))
		} else {
			leafHash = computeHash(branch(leafHash, path[i]))
		}
	}
	return compareBytes(leafHash, rootHash)
}

// Delete removes a leaf node by key and reinserts remaining nodes into the tree
func (t *MerkleTree) Delete(key []byte) error {
	// Find the node to delete
	node, err := t.findNode(t.Root, key, 0)
	if err != nil {
		//fmt.Printf("Delete: key not found: %x\n", key)
		return err
	}

	// Retrieve the value of the node to delete
	value, ok, err := t.levelDBGetLeaf(node.Hash)
	if err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("Delete: key not found: %x", key)
	}

	// Collect remaining nodes' key-value pairs
	remainingNodes := [][2][]byte{}
	t.collectRemainingNodes(t.Root, key, &remainingNodes)

	// Remove the node from levelDB
	nodeHash := computeHash(leaf(key, value))
	if false {
		fmt.Printf("Psuedo Delete: key=%x, value=%x, nodeHash=%x in accessible??\n", key, value, nodeHash)
	}
	//t.db.DeleteK(common.BytesToHash(nodeHash))
	//t.Close()
	// Rebuild the tree without the deleted node
	tree := NewMerkleTree(nil, t.db)
	for _, kv := range remainingNodes {
		tree.Insert(kv[0], kv[1])
	}
	t.Root = tree.Root
	t.db = tree.db
	return nil
}

// collectRemainingNodes collects all key-value pairs except for the one to be deleted
func (t *MerkleTree) collectRemainingNodes(node *Node, deleteKey []byte, nodes *[][2][]byte) {
	if node == nil {
		return
	}

	// Skip the node to be deleted
	if compareBytes(node.Key, deleteKey) {
		return
	}

	// Retrieve the value for the current node
	value, _, err := t.levelDBGetLeaf(node.Hash)
	if err == nil && node.Key != nil {
		if len(value) > 0 {
			*nodes = append(*nodes, [2][]byte{node.Key, value})
		} else {
			// ignore deleted leaf node from collectRemainingNodes (i.e node with value []byte being empty)
		}
	}

	// Recursively collect from left and right subtrees
	t.collectRemainingNodes(node.Left, deleteKey, nodes)
	t.collectRemainingNodes(node.Right, deleteKey, nodes)
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
		fmt.Printf("Node1 %v\n", node1.String())
		fmt.Printf("Node2 %v\n", node2.String())
		return false
	}
	if node1 != nil && node2 == nil {
		fmt.Printf("Node1 Not empty. Node2 Empty\n")
		fmt.Printf("Node1 %v\n", node1.String())
		fmt.Printf("Node2 %v\n", node2.String())
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
