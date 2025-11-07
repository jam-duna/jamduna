package statedb

import (
	"fmt"
	"log"

	"github.com/colorfulnotion/jam/common"
	storage "github.com/colorfulnotion/jam/storage"
	trie "github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

// MockHostEnv struct implements the HostEnv interface with mock responses
type MockHostEnv struct {
	id               uint16
	db               *storage.StateDBStorage
	historicalLookup map[common.Hash][]byte
}

func NewMockHostEnvDB() *storage.StateDBStorage {
	test_db, _ := trie.InitLevelDB()
	return test_db
}

func NewMockHostEnv() *MockHostEnv {
	test_db := NewMockHostEnvDB()
	return &MockHostEnv{db: test_db, id: 0, historicalLookup: make(map[common.Hash][]byte)}
}

func (mh *MockHostEnv) GetID() uint16 {
	return mh.id
}

func (mh *MockHostEnv) GetDB() *storage.StateDBStorage {
	return mh.db
}

func (mh *MockHostEnv) GetService(c uint32) (*types.ServiceAccount, bool, error) {
	return nil, false, nil
}

func (mh *MockHostEnv) ReadServiceStorage(s uint32, k []byte) (storage []byte, ok bool, err error) {
	db := mh.GetDB()
	_, tree, err := trie.Initial_bpt(db)
	defer tree.Close()
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
	defer tree.Close()
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
	if err != nil {
		log.Fatal("fail to connect to BPT")
	}
	time_slots, ok, err = tree.GetPreImageLookup(s, blob_hash, blob_length)
	if err != nil || !ok {
		return nil, false, err
	}
	return nil, false, err
}

// ReadStateWitness fetches StateWitness for given objectID
func (mh *MockHostEnv) ReadStateWitnessRef(serviceID uint32, objectID common.Hash, FetchJAMDASegments bool) (types.StateWitness, bool, error) {
	// Mock implementation: return dummy StateWitness
	witness := types.StateWitness{
		ObjectID: objectID,
		Ref: types.ObjectRef{
			ServiceID:       0,
			WorkPackageHash: common.Hash{},
			IndexStart:      0,
			IndexEnd:        0,
			Version:         0,
			PayloadLength:   0,
		},
		Path: []common.Hash{},
	}
	if FetchJAMDASegments {
		// Mock: just set empty payload
		witness.Payload = []byte{}
	}
	return witness, true, nil
}

// FetchJAMDASegments fetches DA payload using WorkPackageHash and segment indices from ObjectRef
// This triggers availability reads (CE139) from JAM DA
func (mh *MockHostEnv) FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (rawSegments []byte, payload []byte, err error) {
	// Mock implementation: return empty data
	return []byte{}, []byte{}, nil
}

func (mh *MockHostEnv) SetHistoricalLookup(objectid common.Hash, b []byte) common.Hash {
	fmt.Printf("SetHistoricalLookup: objectid=%s len=%d\n", objectid, len(b))
	mh.historicalLookup[objectid] = b
	return objectid
}

func (mh *MockHostEnv) HistoricalLookup(a *types.ServiceAccount, t uint32, blob_hash common.Hash) (b []byte) {
	b = mh.historicalLookup[blob_hash]
	return b
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

func (mh *MockHostEnv) GetParentStateRoot() common.Hash {
	return common.Hash{}
}
