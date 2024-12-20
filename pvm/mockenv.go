package pvm

import (
	"fmt"
	"log"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
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

// func (mh *MockHostEnv) GetXContext() *types.XContext {
// 	return new(types.XContext)
// }

// func (mh *MockHostEnv) SetXContext(x *types.XContext) {}

// func (mh *MockHostEnv) UpdateXContext(x *types.XContext) {}

func (mh *MockHostEnv) GetDB() *storage.StateDBStorage {
	return mh.db
}

func (mh *MockHostEnv) GetService(c uint32) (*types.ServiceAccount, bool, error) {
	return nil, false, nil
}

func (mh *MockHostEnv) ReadServiceStorage(s uint32, k []byte) (storage []byte, ok bool, err error) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	storage, ok, err = tree.GetServiceStorage(s, k)
	if err != nil || !ok {
		return
	} else {
		fmt.Printf("get value=%x, err=%v\n", storage, err)
		return
	}
}

func (mh *MockHostEnv) ReadServicePreimageBlob(s uint32, blob_hash common.Hash) (blob []byte, ok bool, err error) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	blob, ok, err = tree.GetPreImageBlob(s, blob_hash)
	if err != nil || !ok {
		return
	} else {
		fmt.Printf("get value=%x, err=%v\n", blob, err)
		return
	}
}

func (mh *MockHostEnv) ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) (time_slots []uint32, ok bool, err error) {
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	time_slots, ok, err = tree.GetPreImageLookup(s, blob_hash, blob_length)
	if err != nil || !ok {
		return
	} else {
		fmt.Printf("get value=%x, err=%v\n", time_slots, err)
		return
	}
}

func (mh *MockHostEnv) HistoricalLookup(s *types.ServiceAccount, t uint32, blob_hash common.Hash) []byte {
	/*
	db := mh.GetDB()
	rootHash, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	blob, ok, err_v := tree.GetPreImageBlob(s, blob_hash)
	if err_v != nil || !ok {
		return nil
	}

	blob_length := uint32(len(blob))

	//MK: william to fix & verify
	//hbytes := falseBytes(h.Bytes()[4:])
	//lbytes := uint32ToBytes(blob_length)
	//key := append(lbytes, hbytes...)
	//timeslots, err_t := tree.GetPreImageLookup(s, key)
	timeslots, ok, err_t := tree.GetPreImageLookup(s, blob_hash, blob_length)
	if err_t != nil || !ok {
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
	*/
	return nil
}

func (mh *MockHostEnv) GetTimeslot() uint32 {
	return uint32(0)
}

func (mh *MockHostEnv) WriteServicePreimageBlob(s uint32, blob []byte) {
	// TODO: elimnate the need for this by adjusting genesis.go
}

func (mh *MockHostEnv) WriteServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32) {
	// TODO: elimnate the need for this by adjusting genesis.go
}
