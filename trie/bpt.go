package trie

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
)

// Node represents a node in the Merkle Tree
type Node struct {
	Hash []byte
	Key  []byte
	// Value []byte
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

/*
TODO: eleminate the "Value" from Node. we need to store this map saperately
levelDB => Hash(apple) => apple
Hash(hash of value that's >= 32bytes) => value
*/

// MerkleTree represents the entire Merkle Tree
type MerkleTree struct {
	Root *Node
	// levelDBMap map[string][]byte // <>
	db *leveldb.DB
}

// NewMerkleTree creates a new Merkle Tree from the provided data
func initLevelDB(optionalPath ...string) (*leveldb.DB, error) {
	path := "/tmp/log/leveldb/bpt"
	if len(optionalPath) > 0 {
		path = optionalPath[0]
	}
	db, err := leveldb.OpenFile(path, nil)
	fmt.Printf("Initailized levelDB at: %s\n", path)
	return db, err
}

// NewMerkleTree creates a new Merkle Tree from the provided data
func NewMerkleTree(data [][2][]byte, optionalPath ...string) *MerkleTree {
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

// branch concatenates the left and right node hashes with a modified head
func branch(left, right []byte) []byte {
	if len(left) != 32 || len(right) != 32 {
		panic("branch: input hashes must be 32 bytes")
	}
	head := left[0] & 0xfe                           // Set the LSB of the first byte of the left hash to 0
	left255bits := append([]byte{head}, left[1:]...) // Left: last 255 bits of
	concatenated := append(left255bits, right...)    // (l,r): 512 bits
	return concatenated
}

// leaf encodes a key-value pair into a leaf node
func leaf(k, v []byte) []byte {
	// Embedded-value leaf node
	if len(v) <= 32 {
		head := byte(0b01 | (len(v) << 2))
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
		head := byte(0b11)
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

	if head&0b11 == 0b01 {
		// Embedded-value leaf node
		valueSize := int(head >> 2)
		value := leaf[32 : 32+valueSize]
		return key, value, true, nil
	} else if head&0b11 == 0b11 {
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
	bitIndex := i % 8             // the bit position within the byte
	b := k[byteIndex]             // target byte
	mask := byte(1 << (bitIndex)) // least significant bit first
	return (b & mask) != 0        // return set (1) or not (0)
}

// GetRootHash returns the root hash of the Merkle Tree
func (t *MerkleTree) GetRootHash() []byte {
	if t.Root == nil {
		return make([]byte, 32)
	}
	return t.Root.Hash
}

func InitMerkleTreeFromHash(root []byte, db *leveldb.DB) (*MerkleTree, error) {
	tree := &MerkleTree{Root: nil, db: db}
	rootNode, err := tree.levelDBGetBranch(root)
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
	value, err := t.levelDBGet(branchHash)

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

	return &Node{
		Hash:  branchHash,
		Left:  leftNode,
		Right: rightNode,
	}, nil
}

func (t *MerkleTree) levelDBSetLeaf(encodedLeaf, value []byte, key []byte) {
	_, _v, isEmbedded, _ := decodeLeaf(encodedLeaf)
	t.levelDBSet(append(computeHash(encodedLeaf), value...), key)
	if isEmbedded {
		// value-embedded leaf node: 2 bits | 6 bits (value size) | 31 bytes (key)
		// value is less or equal to 32 bytes
		// value can be recovered from encodedLeaf.
		t.levelDBSet(computeHash(encodedLeaf), encodedLeaf)
	} else {
		// regular leaf node: 2 bits | 2 bits | 6 bits (0s) | 31 bytes (key)
		// value is greater than 32 bytes
		// store additional hash(value) -> value
		t.levelDBSet(computeHash(encodedLeaf), encodedLeaf)
		t.levelDBSet(_v, value)
	}
}

func (t *MerkleTree) levelDBGetLeaf(nodeHash []byte) ([]byte, error) {
	encodedLeaf, err := t.levelDBGet(nodeHash)
	if err != nil {
		return nil, err
	}
	//recover encodedLeaf from nodeHash
	_, _v, isEmbedded, err := decodeLeaf(encodedLeaf)
	if err != nil {
		return nil, fmt.Errorf("leaf Err: %s", err)
	}
	if isEmbedded {
		// value-embedded leaf node: 2 bits | 6 bits (value size) | 31 bytes (key)
		// value is less or equal to 32 bytes, return exact value
		return _v, nil
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
	key := k
	err := t.db.Put(key, v, nil)
	//err := t.db.Put(k, v, nil)
	if err != nil {
		return fmt.Errorf("failed to set key %s: %v", key, err)
	}
	return nil
}

// levelDBGet gets the value for the given key from the levelDBMap
func (t *MerkleTree) levelDBGet(k []byte) ([]byte, error) {
	if t.db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	value, err := t.db.Get(k, nil)
	//value, err := t.db.Get(k, nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, fmt.Errorf("key not found: %s", k)
		}
		return nil, fmt.Errorf("failed to get key %s: %v", k, err)
	}
	return value, nil
}

func (t *MerkleTree) levelDBGetNode(nodeHash []byte) (*Node, error) {
	value, _ := t.db.Get([]byte(nodeHash), nil)
	zeroHash := make([]byte, 32)
	if compareBytes(nodeHash, zeroHash) || value == nil {
		return &Node{
			Hash: zeroHash,
		}, nil
	}
	leafKey, _ := t.levelDBGetLeaf(nodeHash)
	if leafKey != nil {
		leafValue, _ := t.db.Get([]byte(append(nodeHash, leafKey...)), nil)
		return &Node{
			Hash: nodeHash,
			Key:  leafValue,
		}, nil
	}

	if value != nil || !compareBytes(value, zeroHash) {
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
	value, _ := t.Get(node.Key)
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
	}
	t.Insert(stateKey, value)
}
func (t *MerkleTree) GetState(_stateIdentifier string) ([]byte, error) {
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
	}
	return t.Get(stateKey)
}

// PVM read
func (t *MerkleTree) SetService(i uint32, s uint32, v []byte) {
	stateKey := make([]byte, 32)
	stateKey[0] = 0xFF

	number := uint16(s)
	byteSlice := make([]byte, 2)
	binary.BigEndian.PutUint16(byteSlice, number)
	copy(stateKey[1:5], byteSlice)

	t.Insert(stateKey, v)
}

func (t *MerkleTree) GetService(i uint32, s uint32) ([]byte, error) {
	stateKey := make([]byte, 32)
	stateKey[0] = 0xFF

	number := uint16(s)
	byteSlice := make([]byte, 2)
	binary.BigEndian.PutUint16(byteSlice, number)
	copy(stateKey[1:5], byteSlice)
	fmt.Printf("Service Key: %x\n", stateKey)
	return t.Get(stateKey)
}

// PVM read preimage
func (t *MerkleTree) SetPreImage(s uint32, h []byte, v []byte) {
	// Set the key for the state
	stateKey := make([]byte, 32)
	sBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sBytes, s)

	for i := 0; i < 4; i++ {
		stateKey[2*i] = sBytes[i]
		if i < len(h) {
			stateKey[2*i+1] = h[i]
		}
	}
	for i := 4; i < 28; i++ {
		if i < len(h) {
			stateKey[i+4] = h[i]
		}
	}
	// Insert the value into the state
	t.Insert(stateKey, v)
}

func (t *MerkleTree) GetPreImage(s uint32, h []byte) ([]byte, error) {
	// Compute the key for the state
	stateKey := make([]byte, 32)
	sBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sBytes, s)

	for i := 0; i < 4; i++ {
		stateKey[2*i] = sBytes[i]
		if i < len(h) {
			stateKey[2*i+1] = h[i]
		}
	}
	for i := 4; i < 28; i++ {
		if i < len(h) {
			stateKey[i+4] = h[i]
		}
	}
	// Get the value from the state
	return t.Get(stateKey)
}

// Insert fixed-length hashed key with value for the BPT
func (t *MerkleTree) Insert(key, value []byte) {
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

	if node == nil || compareBytes(node.Hash, nullNode.Hash) {
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
	existingValue, _ := t.Get(node.Key)

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
	if node == nil {
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
func (t *MerkleTree) Get(key []byte) ([]byte, error) {
	// if t.Root == nil {
	// 	return nil, errors.New("empty tree")
	// }
	return t.getValue(t.Root, key, 0)
}

func (t *MerkleTree) getValue(node *Node, key []byte, depth int) ([]byte, error) {
	if node == nil {
		return nil, errors.New("key not found")
	}

	// fmt.Printf("Searching key: %x at node key: %x at depth: %d\n", key, node.Key, depth)
	if compareBytes(node.Key, key) {
		//fmt.Printf("Found key: %x with Hash: %x/n", key, node.Hash)
		value, err := t.levelDBGetLeaf(node.Hash)
		if err != nil {
			return nil, fmt.Errorf("key not found: %x", key)
		}
		return value, nil
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
	if node == nil {
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

// Verify verifies the path to a specific key in the Merkle Tree
func (t *MerkleTree) Verify(key []byte, value []byte, rootHash []byte, path [][]byte) bool {
	if len(path) == 0 {
		return compareBytes(computeHash(leaf(key, value)), rootHash)
	}

	leafHash := computeHash(leaf(key, value))

	for i := len(path) - 1; i >= 0; i-- {
		if bit(key, i) {
			// fmt.Printf("computing hash (%x, %x):%x\n", path[i], leafHash, computeHash(branch(path[i], leafHash)))
			leafHash = computeHash(branch(path[i], leafHash))
		} else {
			// fmt.Printf("computing hash (%x, %x):%x\n", leafHash, path[i], computeHash(branch(leafHash, path[i])))
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
		return err
	}

	// Retrieve the value of the node to delete
	value, err := t.levelDBGetLeaf(node.Hash)
	if err != nil {
		return err
	}

	// Collect remaining nodes' key-value pairs
	remainingNodes := [][2][]byte{}
	t.collectRemainingNodes(t.Root, key, &remainingNodes)

	// Remove the node from levelDB
	nodeHash := computeHash(leaf(key, value))
	t.db.Delete(nodeHash, nil)
	t.Close()

	// Rebuild the tree without the deleted node
	tree := NewMerkleTree(nil)
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
	value, err := t.levelDBGetLeaf(node.Hash)
	if err == nil && node.Key != nil {
		*nodes = append(*nodes, [2][]byte{node.Key, value})
	}

	// Recursively collect from left and right subtrees
	t.collectRemainingNodes(node.Left, deleteKey, nodes)
	t.collectRemainingNodes(node.Right, deleteKey, nodes)
}

func isBranchNode(value []byte) bool {
	// Implement logic to determine if a node is a branch node
	return len(value) == 64 && value[0] != 0 && value[1] != 0
}
