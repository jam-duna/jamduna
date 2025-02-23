package pvm

import (
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

func (mh *MockHostEnv) ReadServiceStorage(s uint32, k common.Hash) (storage []byte, ok bool, err error) {
	db := mh.GetDB()
	_, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	// fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	storage, ok, err = tree.GetServiceStorage(s, k)
	if err != nil || !ok {
		return nil, false, err
	}
	return nil, false, err
}

func (mh *MockHostEnv) ReadServicePreimageBlob(s uint32, blob_hash common.Hash) (blob []byte, ok bool, err error) {
	db := mh.GetDB()
	_, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	// fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	blob, ok, err = tree.GetPreImageBlob(s, blob_hash)
	if err != nil || !ok {
		return nil, false, err
	}
	return nil, false, err
}

func (mh *MockHostEnv) ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) (time_slots []uint32, ok bool, err error) {
	db := mh.GetDB()
	_, tree, err := trie.Initial_bpt(db)
	//tree := trie.NewMerkleTree(nil, db)
	defer tree.Close()
	// fmt.Printf("Root Hash=%x, err=%v\n", rootHash, err)
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	time_slots, ok, err = tree.GetPreImageLookup(s, blob_hash, blob_length)
	if err != nil || !ok {
		return nil, false, err
	}
	return nil, false, err
}

func (mh *MockHostEnv) HistoricalLookup(a *types.ServiceAccount, t uint32, blob_hash common.Hash) []byte {
	blob := a.Preimage[common.Blake2Hash(blob_hash.Bytes())].Preimage
	timeslots := a.Lookup[blob_hash].T
	if len(timeslots) == 0 {
		return nil
	} else if len(timeslots) == 1 {
		if timeslots[0] <= t {
			return blob
		} else {
			return nil
		}
	} else if len(timeslots) == 2 {
		if timeslots[0] <= t && t < timeslots[1] {
			return blob
		} else {
			return nil
		}
	} else {
		if timeslots[0] <= t && t < timeslots[1] || timeslots[2] <= t {
			return blob
		} else {
			return nil
		}
	}
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
