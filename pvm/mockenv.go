package pvm

import (
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

func (mh *MockHostEnv) ReadServiceBytes(s uint32) []byte {
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
		return nil
	}
	return value
}

func (mh *MockHostEnv) WriteServiceBytes(s uint32, v []byte) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	tree.SetService(255, s, v)
}

func (mh *MockHostEnv) ReadServiceStorage(s uint32, k []byte) []byte {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	storage, err := tree.GetServiceStorage(s, k)
	if err != nil {
		return nil
	} else {
		fmt.Printf("get value=%x, err=%v\n", storage, err)
		return storage
	}
}

func (mh *MockHostEnv) WriteServiceStorage(s uint32, k []byte, storage []byte) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	tree.SetServiceStorage(s, k, storage)
}

func (mh *MockHostEnv) ReadServicePreimageBlob(s uint32, blob_hash common.Hash) []byte {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	blob, err := tree.GetPreImageBlob(s, blob_hash.Bytes())
	if err != nil {
		return nil
	} else {
		fmt.Printf("get value=%x, err=%v\n", blob, err)
		return blob
	}
}

func (mh *MockHostEnv) WriteServicePreimageBlob(s uint32, blob []byte) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	tree.SetPreImageBlob(s, blob)
}

func (mh *MockHostEnv) ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) []uint32 {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	time_slots, err := tree.GetPreImageLookup(s, blob_hash, blob_length)
	if err != nil {
		return nil
	} else {
		fmt.Printf("get value=%x, err=%v\n", time_slots, err)
		return time_slots
	}
}

func (mh *MockHostEnv) WriteServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	tree.SetPreImageLookup(s, blob_hash, blob_length, time_slots)

}

func (mh *MockHostEnv) HistoricalLookup(s uint32, t uint32, blob_hash common.Hash) []byte {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	blob, err_v := tree.GetPreImageBlob(s, blob_hash.Bytes())
	if err_v != nil {
		return nil
	}

	blob_length := uint32(len(blob))

	//MK: william to fix & verify
	//hbytes := falseBytes(h.Bytes()[4:])
	//lbytes := uint32ToBytes(blob_length)
	//key := append(lbytes, hbytes...)
	//timeslots, err_t := tree.GetPreImageLookup(s, key)
	timeslots, err_t := tree.GetPreImageLookup(s, blob_hash, blob_length)
	if err_t != nil {
		return nil
	}

	if timeslots[0] == 0 {
		return nil
	} else if len(timeslots) == (12 + 1) {
		x := timeslots[0]
		y := timeslots[1]
		z := timeslots[2]
		if (x <= t && t < y) || (z <= t) {
			return blob
		} else {
			return nil
		}
	} else if len(timeslots) == (8 + 1) {
		x := timeslots[0]
		y := timeslots[1]
		if x <= t && t < y {
			return blob
		} else {
			return nil
		}
	} else {
		x := timeslots[0]
		if x <= t {
			return blob
		} else {
			return nil
		}
	}
}

func (mh *MockHostEnv) DeleteServiceStorageKey(s uint32, k []byte) error {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal(err)
	}
	err = tree.DeleteServiceStorage(s, k)
	if err != nil {
		log.Fatalf("Failed to delete k: %v, error: %v", k, err)
		return err
	}
	return nil
}

func (mh *MockHostEnv) DeleteServicePreimageKey(s uint32, blob_hash common.Hash) error {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal(err)
	}
	err = tree.DeletePreImageBlob(s, blob_hash.Bytes())
	if err != nil {
		log.Fatalf("Failed to delete blob_hash: %x, error: %v", blob_hash.Bytes(), err)
		return err
	}
	return nil
}

func (mh *MockHostEnv) DeleteServicePreimageLookupKey(s uint32, blob_hash common.Hash, blob_length uint32) error {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal(err)
	}
	err = tree.DeletePreImageLookup(s, blob_hash, blob_length)
	if err != nil {
		log.Fatalf("Failed to delete blob_hash: %v, blob_lookup_len: %d, error: %v", blob_hash, blob_length, err)
		return err
	}
	return nil
}

func (mh *MockHostEnv) SetX(obj interface{}) uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) GetX(c string) interface{} {
	return uint32(0)
}
