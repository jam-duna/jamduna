package pvm

import (
	"encoding/binary"
	"fmt"
	"log"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
)

// MockHostEnv struct implements the HostEnv interface with mock responses
type MockHostEnv struct {
	db *storage.StateDBStorage
}

func NewMockHostEnvDB() *storage.StateDBStorage {
	test_db, _ := trie.InitLevelDB()
	return test_db
}

func NewMockHostEnv() *MockHostEnv {
	test_db := NewMockHostEnvDB()
	return &MockHostEnv{db: test_db}
}

func (mh *MockHostEnv) GetDB() *storage.StateDBStorage {
	return mh.db
}

func (mh *MockHostEnv) NewService(c []byte, l, b uint32, g, m uint64) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) UpgradeService(c []byte, g, m uint64) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) AddTransfer(memo []byte, a, g uint64, d uint32) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) ReadServiceBytes(s uint32) ([]byte, uint32, bool) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	value, err := tree.GetService(255, s)
	if err != nil {
		return nil, 0, false
	} else {
		fmt.Printf("get Service Account value=%x, err=%v\n", value, err)
	}
	return value, uint32(len(value)), true
}

func (mh *MockHostEnv) WriteServiceBytes(s uint32, v []byte) bool {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	tree.SetService(255, s, v)
	return true
}

func (mh *MockHostEnv) ReadServicePreimage(s uint32, h common.Hash) ([]byte, uint32, bool) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	value, err := tree.GetPreImage(s, h.Bytes())
	if err != nil {
		return nil, 0, false
	} else {
		fmt.Printf("get value=%x, err=%v\n", value, err)
		return value, uint32(len(value)), true
	}

}

func (mh *MockHostEnv) WriteServicePreimage(s uint32, k common.Hash, v []byte) bool {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	tree.SetPreImage(s, k.Bytes(), v)
	return true
}

func (mh *MockHostEnv) WriteServiceKey(s uint32, k common.Hash, v []byte) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) GetPreimage(k common.Hash, z uint32) (uint32, uint32) {
	return uint32(0), uint32(0)
}

func (mh *MockHostEnv) RequestPreimage2(h common.Hash, z uint32, y uint32) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) RequestPreimage(h common.Hash, z uint32) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) ForgetPreimage(h common.Hash, z uint32) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) HistoricalLookup(s uint32, t uint32, h common.Hash) ([]byte, uint32, bool) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	value, err_v := tree.GetPreImage(s, h.Bytes())
	if err_v != nil {
		return nil, 0, false
	}

	hbytes := falseBytes(h.Bytes()[4:])
	lbytes := uint32ToBytes(uint32(len(value)))
	key := append(lbytes, hbytes...)

	timeslots, err_t := tree.GetPreImage(s, key)
	if err_t != nil {
		return nil, 0, false
	}

	if timeslots[0] == 0 {
		return nil, 0, false
	} else if len(timeslots) == (12 + 1) {
		x := binary.LittleEndian.Uint32(timeslots[1:5])
		y := binary.LittleEndian.Uint32(timeslots[5:9])
		z := binary.LittleEndian.Uint32(timeslots[9:13])
		if (x <= t && t < y) || (z <= t) {
			return value, uint32(len(value)), true
		} else {
			return nil, 0, false
		}
	} else if len(timeslots) == (8 + 1) {
		x := binary.LittleEndian.Uint32(timeslots[1:5])
		y := binary.LittleEndian.Uint32(timeslots[5:9])
		if x <= t && t < y {
			return value, uint32(len(value)), true
		} else {
			return nil, 0, false
		}
	} else {
		x := binary.LittleEndian.Uint32(timeslots[1:5])
		if x <= t {
			return value, uint32(len(value)), true
		} else {
			return nil, 0, false
		}
	}
}

func (mh *MockHostEnv) GetImportItem(i uint32) ([]byte, uint32) {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, i)
	return bytes, uint32(4)
}

func (mh *MockHostEnv) ExportSegment(x []byte) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) DeleteKey(k common.Hash) error {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal(err)
	}
	err = tree.Delete(k.Bytes())
	if err != nil {
		log.Fatalf("Failed to delete key: %x, error: %v", k.Bytes(), err)
		return err
	}
	return nil
}

func (mh *MockHostEnv) ContainsKey(h []byte, z []byte) bool {
	return false // Assume the key does not exist
}

func (mh *MockHostEnv) SetKey(k common.Hash, v []byte, b0 uint32, numBytes int) error {
	return nil // Assume success
}

func (mh *MockHostEnv) AddKey(k common.Hash, v []byte) error {
	return nil // Assume success
}

func (mh *MockHostEnv) CreateVM(code []byte, i uint32) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) GetVM(n uint32) (*VM, bool) {
	return &VM{}, false // Return a dummy VM and indicate failure
}

func (mh *MockHostEnv) ExpungeVM(n uint32) bool {
	return false // Assume failure
}

func (mh *MockHostEnv) Designate(v []byte) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) Empower(m uint32, a uint32, v uint32) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) Assign(c []byte) uint32 {
	return uint32(0)
}
