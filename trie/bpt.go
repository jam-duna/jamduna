package trie

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/syndtr/goleveldb/leveldb"
)

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

/*
TODO: eleminate the "Value" from Node. we need to store this map saperately
levelDB => Hash(apple) => apple
Hash(hash of value that's >= 32bytes) => value
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
	//db, err := leveldb.OpenFile(path, nil)
	fmt.Printf("Initailized levelDB at: %s\n", path)
	return stateDBStorage, err
}

func InitLevelDB(optionalPath ...string) (*storage.StateDBStorage, error) {
	return initLevelDB(optionalPath...)
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
		fmt.Printf("Root Hash is empty\n")
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
	value, err := t.levelDBGet(branchHash)
	if err != nil {
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
	//err := t.db.Put(key, v, nil)
	err := t.db.WriteKV(common.BytesToHash(key), v)
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
	//value, err := t.db.Get(k, nil)
	value, err := t.db.ReadKV(common.BytesToHash(k))
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, fmt.Errorf("key not found: %s", k)
		}
		return nil, fmt.Errorf("failed to get key %s: %v", k, err)
	}
	return RecoverNull(value), nil
}

func (t *MerkleTree) levelDBGetNode(nodeHash []byte) (*Node, error) {
	//value, _ := t.db.Get([]byte(nodeHash), nil)
	value, _ := t.db.ReadKV(common.BytesToHash(nodeHash))
	zeroHash := make([]byte, 32)
	if compareBytes(nodeHash, zeroHash) || value == nil {
		return &Node{
			Hash: zeroHash,
		}, nil
	}
	leafKey, _ := t.levelDBGetLeaf(nodeHash)
	if leafKey != nil {
		leafValue, _ := t.db.ReadKV(common.BytesToHash([]byte(append(nodeHash, leafKey...))))
		//leafValue, _ := t.db.Get([]byte(append(nodeHash, leafKey...)), nil)
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

func (n *Node) String() string {
	s := fmt.Sprintf("Node Hash=%x, Key=%x\n", n.Hash, n.Key)
	return s
}

func (t *MerkleTree) PrintTree(node *Node, level int) {
	t.printTree(node, level)
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

// EQ 290 - state-key constructor functions C
func ComputeC_i(i uint8) common.Hash {
	//i ∈ N_8 ↦ [i,0,0,...]
	stateKey := make([]byte, 32)
	stateKey[0] = byte(i)
	return common.BytesToHash(stateKey)
}

func ComputeC_is(i uint8, s uint32) common.Hash {
	//(i,s ∈ N_S) ↦ [i,n0,n1,n2,n3,0,0,...] where n = E4(s)
	stateKey := make([]byte, 32)
	stateKey[0] = i
	byteSlice := make([]byte, 4)
	binary.LittleEndian.PutUint32(byteSlice, s)
	for i := 1; i < 5; i++ {
		stateKey[i] = byteSlice[i-1]
	}
	return common.BytesToHash(stateKey)
}

func ComputeC_sh(s uint32, h []byte) common.Hash {
	//s: service_index
	//h: hash_component (assumed to be exact 32bytes)
	//(s,h) ↦ [n0,h0,n1,h1,n2,h2,n3,h3,h4,h5,...,h27] where n = E4(s)
	stateKey := make([]byte, 32)
	nBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(nBytes, s) // n = E4(s)

	for i := 0; i < 4; i++ {
		stateKey[2*i] = nBytes[i]
		if i < len(h) {
			stateKey[2*i+1] = h[i]
		}
	}
	for i := 4; i < 28; i++ {
		if i < len(h) {
			stateKey[i+4] = h[i]
		}
	}
	return common.BytesToHash(stateKey)
}

func (t *MerkleTree) SetService(i uint8, s uint32, v []byte) {
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
	service_account := ComputeC_is(i, s)
	stateKey := service_account.Bytes()
	t.Insert(stateKey, v)
}

func (t *MerkleTree) GetService(i uint8, s uint32) ([]byte, error) {
	service_account := ComputeC_is(i, s)
	stateKey := service_account.Bytes()
	return t.Get(stateKey)
}

// set a_l (with timeslot if we have E_P). For GP_0.3.5(158)
func (t *MerkleTree) SetPreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32, time_slots []uint32) {

	/*
		∀(s ↦ a) ∈ δ, ( ⎧⎩ h, l ⎫⎭ ↦ t)∈ a l ∶ C(s, E 4 (l)⌢(¬h 4∶ )) ↦ E(↕[E 4 (x) ∣ x <− t])
		(s, h) ↦ [n 0 ,h 0 ,n 1 ,h 1 ,n 2 ,h 2 ,n 3 ,h 3 ,h 4 ,h 5 ,...,h 27 ] where n = E 4 (s)

		s: service_index
		h: blob_hash
		l: blob_len
		t: time_slots
	*/

	lBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lBytes, blob_len) // E4(l)
	blob_h := blob_hash.Bytes()
	_h4 := falseBytes(blob_h[4:])                 // (¬h4:)
	l_and_h := append(lBytes, _h4...)             // (E4(l) ⌢ (¬h4:)
	account_lookuphash := ComputeC_sh(s, l_and_h) // C(s, (E4(l) ⌢ (¬h4:))
	stateKey := account_lookuphash.Bytes()

	/*
		Follow GP_0.3.5(270, 273, 274, 276, 291)
		Process State value(timeslots), covert []uint32 to []byte
	*/
	vBytes := []byte{}
	if len(time_slots) > 0 {
		time_slotsByte := make([]byte, len(time_slots)*4)

		// Convert time slots into byte
		for i, v := range time_slots {
			binary.LittleEndian.PutUint32(time_slotsByte[i*4:(i+1)*4], v)
		}
		vBytes = append([]byte{uint8(len(time_slots))}, time_slotsByte...)
	}
	// Insert the value into the state
	fmt.Printf("SetPreImageLookup stateKey=%x, vBytes=%v\n", stateKey, vBytes)
	t.Insert(stateKey, vBytes)
}

// lookup a_l .. returning time slot. For GP_0.3.5(157)
func (t *MerkleTree) GetPreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32) ([]uint32, error) {

	lBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lBytes, blob_len) // E4(l)
	blob_h := blob_hash.Bytes()
	_h4 := falseBytes(blob_h[4:])                 // (¬h4:)
	l_and_h := append(lBytes, _h4...)             // (E4(l) ⌢ (¬h4:)
	account_lookuphash := ComputeC_sh(s, l_and_h) // C(s, (E4(l) ⌢ (¬h4:))
	stateKey := account_lookuphash.Bytes()

	/*
		Follow GP_0.3.5(270, 273, 274, 276, 291)
		Process State value(timeslots), covert []uint32 to []byte
	*/

	vByte, err := t.Get(stateKey)
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
	return time_slots, err
}

// Delete PreImageLookup key(hash)
func (t *MerkleTree) DeletePreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32) error {

	lBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lBytes, blob_len) // E4(l)
	blob_h := blob_hash.Bytes()
	_h4 := falseBytes(blob_h[4:])                 // (¬h4:)
	l_and_h := append(lBytes, _h4...)             // (E4(l) ⌢ (¬h4:)
	account_lookuphash := ComputeC_sh(s, l_and_h) // C(s, (E4(l) ⌢ (¬h4:))
	stateKey := account_lookuphash.Bytes()

	err := t.Delete(stateKey)

	return err
}

func (t *MerkleTree) SetServiceStorage(s uint32, k []byte, storage []byte) {
	/*
		∀(s ↦ a) ∈ δ, (h ↦ v) ∈ a s ∶ C(s, h) ↦ v
		(s, h) ↦ [n 0 ,h 0 ,n 1 ,h 1 ,n 2 ,h 2 ,n 3 ,h 3 ,h 4 ,h 5 ,...,h 27 ] where n = E 4 (s)

		s: service_index
		h: storage_key from H(E4(s) ⌢ vk ⋅⋅⋅+k )
		v: storage
	*/

	sBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sBytes, s)           // E4(s)
	h_and_v := append(sBytes, k...)                    //(E4(s) ⌢ vk ⋅⋅⋅+k )
	storage_key := common.Blake2Hash(h_and_v).Bytes()  // H(E4(s) ⌢ vk ⋅⋅⋅+k )
	account_storage_key := ComputeC_sh(s, storage_key) // C(s, (E4(l) ⌢ (¬h4:))
	stateKey := account_storage_key.Bytes()

	// Insert Stroage into trie
	t.Insert(stateKey, storage)
}

func (t *MerkleTree) GetServiceStorage(s uint32, k []byte) ([]byte, error) {

	sBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sBytes, s)           // E4(s)
	h_and_v := append(sBytes, k...)                    //(E4(s) ⌢ vk ⋅⋅⋅+k )
	storage_key := common.Blake2Hash(h_and_v).Bytes()  // H(E4(s) ⌢ vk ⋅⋅⋅+k )
	account_storage_key := ComputeC_sh(s, storage_key) // C(s, (E4(l) ⌢ (¬h4:))
	stateKey := account_storage_key.Bytes()

	// Get Storage from trie
	return t.Get(stateKey)
}

// Delete Storage key(hash)
func (t *MerkleTree) DeleteServiceStorage(s uint32, k []byte) error {
	/*
		∀(s ↦ a) ∈ δ, (h ↦ v) ∈ a s ∶ C(s, h) ↦ v
		(s, h) ↦ [n 0 ,h 0 ,n 1 ,h 1 ,n 2 ,h 2 ,n 3 ,h 3 ,h 4 ,h 5 ,...,h 27 ] where n = E 4 (s)

		s: service_index
		h: storage_hash
		v: storage
	*/

	sBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sBytes, s)           // E4(s)
	h_and_v := append(sBytes, k...)                    //(E4(s) ⌢ vk ⋅⋅⋅+k )
	storage_key := common.Blake2Hash(h_and_v).Bytes()  // H(E4(s) ⌢ vk ⋅⋅⋅+k )
	account_storage_key := ComputeC_sh(s, storage_key) // C(s, (E4(l) ⌢ (¬h4:))
	stateKey := account_storage_key.Bytes()

	err := t.Delete(stateKey)
	return err
}

// Set PreImage Blob for GP_0.3.5(158)
func (t *MerkleTree) SetPreImageBlob(s uint32, blob []byte) {
	/*
		∀(s ↦ a) ∈ δ, (h ↦ p) ∈ a p ∶ C(s, h) ↦ p
		(s, h) ↦ [n 0 ,h 0 ,n 1 ,h 1 ,n 2 ,h 2 ,n 3 ,h 3 ,h 4 ,h 5 ,...,h 27 ] where n = E 4 (s)

		s: service_index
		h: blob_hash
		p: blob
	*/

	blob_hash := common.ComputeHash(blob)
	account_preimage_hash := ComputeC_sh(s, blob_hash)
	stateKey := account_preimage_hash.Bytes()

	// Insert Preimage Blob into trie
	t.Insert(stateKey, blob)
}

func (t *MerkleTree) GetPreImageBlob(s uint32, blob_hash []byte) ([]byte, error) {

	account_preimage_hash := ComputeC_sh(s, blob_hash)
	stateKey := account_preimage_hash.Bytes()

	// Get Preimage Blob from trie
	return t.Get(stateKey)
}

// Delete PreImage Blob
func (t *MerkleTree) DeletePreImageBlob(s uint32, blob_hash []byte) error {
	account_preimage_hash := ComputeC_sh(s, blob_hash)
	stateKey := account_preimage_hash.Bytes()
	err := t.Delete(stateKey)
	return err
}

// Insert fixed-length hashed key with value for the BPT
func (t *MerkleTree) Insert(key, value []byte) {
	value = ValidateNull(value)
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
func (t *MerkleTree) Get(key []byte) ([]byte, error) {
	// if t.Root == nil {
	// 	return nil, errors.New("empty tree")
	// }
	value, err := t.getValue(t.Root, key, 0)
	return RecoverNull(value), err
}

func (t *MerkleTree) getValue(node *Node, key []byte, depth int) ([]byte, error) {
	if node == nil || depth > computeKeyLengthAsBit(key) {
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
	t.db.DeleteK(common.BytesToHash(nodeHash))
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

// Implement "¬"
func falseBytes(data []byte) []byte {
	result := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		result[i] = 0xFF - data[i]
		// result[i] = ^data[i]
	}
	return result
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

// Because leveldb can't store nil value, we need to set a null value
func ValidateNull(value []byte) []byte {
	if compareBytes(value, []byte(LevelDBEmpty)) {
		value = []byte(LevelDBNull)
		//fmt.Printf("Value is empty, setting null value\n")
	}
	return value
}

// If the value is null, return a null byte
func RecoverNull(value []byte) []byte {
	if compareBytes(value, []byte(LevelDBNull)) {
		return []byte(LevelDBEmpty)
	}
	return value
}
