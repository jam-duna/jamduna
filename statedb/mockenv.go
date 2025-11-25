package statedb

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// MockHostEnv is a mock implementation of types.HostEnv for testing
type MockHostEnv struct {
	storage map[string][]byte
}

// NewMockHostEnv creates a new mock host environment for testing
func NewMockHostEnv() *MockHostEnv {
	return &MockHostEnv{
		storage: make(map[string][]byte),
	}
}

// GetService returns a stub service account
func (m *MockHostEnv) GetService(service uint32) (*types.ServiceAccount, bool, error) {
	return &types.ServiceAccount{}, false, nil
}

// ReadServiceStorage returns stub storage data
func (m *MockHostEnv) ReadServiceStorage(s uint32, k []byte) ([]byte, bool, error) {
	key := string(k)
	if val, ok := m.storage[key]; ok {
		return val, true, nil
	}
	return nil, false, nil
}

// GetParentStateRoot returns a stub state root
func (m *MockHostEnv) GetParentStateRoot() common.Hash {
	return common.Hash{}
}

// ReadServicePreimageBlob returns stub preimage blob data
func (m *MockHostEnv) ReadServicePreimageBlob(s uint32, blob_hash common.Hash) ([]byte, bool, error) {
	return nil, false, nil
}

// ReadServicePreimageLookup returns stub preimage lookup data
func (m *MockHostEnv) ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) ([]uint32, bool, error) {
	return nil, false, nil
}

// HistoricalLookup returns stub historical data
func (m *MockHostEnv) HistoricalLookup(a *types.ServiceAccount, t uint32, blob_hash common.Hash) []byte {
	return nil
}

// GetTimeslot returns a stub timeslot
func (m *MockHostEnv) GetTimeslot() uint32 {
	return 0
}

// GetHeader returns a stub block header
func (m *MockHostEnv) GetHeader() *types.BlockHeader {
	return &types.BlockHeader{}
}

// WriteServicePreimageBlob is a stub write method
func (m *MockHostEnv) WriteServicePreimageBlob(s uint32, blob []byte) {
	// Stub - does nothing
}

// ReadObject returns a stub state witness for object lookup
func (m *MockHostEnv) ReadObject(serviceID uint32, objectID common.Hash) (*types.StateWitness, bool, error) {
	return &types.StateWitness{}, false, nil
}

// GetWitnesses returns an empty map of witnesses for testing
func (m *MockHostEnv) GetWitnesses() map[common.Hash]*types.StateWitness {
	return make(map[common.Hash]*types.StateWitness)
}

// WriteServicePreimageLookup is a stub write method
func (m *MockHostEnv) WriteServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32) {
	// Stub - does nothing
}
