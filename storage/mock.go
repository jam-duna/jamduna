package storage

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

// MockStorage provides a minimal storage implementation for testing
type MockStorage struct {
	trieDB *MerkleTree
}

// NewMockStorage creates a temporary trie storage for testing
func NewMockStorage() (*MockStorage, error) {
	// Create empty trie database
	trieDB := NewMerkleTree(nil)

	return &MockStorage{
		trieDB: trieDB,
	}, nil
}

func (m *MockStorage) Close() error {
	if m.trieDB != nil {
		m.trieDB = nil
	}
	return nil
}

// Delete implements JAMStorage interface
func (m *MockStorage) Delete(key []byte) error {
	if m.trieDB == nil {
		return fmt.Errorf("mock storage is closed")
	}
	return m.trieDB.Delete(key)
}

// Insert implements JAMStorage interface
func (m *MockStorage) Insert(key []byte, value []byte) {
	if m.trieDB == nil {
		return
	}
	m.trieDB.Insert(key, value)
}

// Get implements JAMStorage interface
func (m *MockStorage) Get(key []byte) ([]byte, bool, error) {
	if m.trieDB == nil {
		return nil, false, fmt.Errorf("mock storage is closed")
	}
	return m.trieDB.Get(key)
}

// Flush implements JAMStorage interface
func (m *MockStorage) Flush() error {
	if m.trieDB == nil {
		return fmt.Errorf("mock storage is closed")
	}
	_, err := m.trieDB.Flush()
	return err
}

// GetRoot implements JAMStorage interface
func (m *MockStorage) GetRoot() []byte {
	if m.trieDB == nil {
		return make([]byte, 32)
	}
	root := m.trieDB.GetRoot()
	return root[:]
}

// OverlayRoot implements JAMStorage interface - compute root from staged overlay without commit
func (m *MockStorage) OverlayRoot() (common.Hash, error) {
	if m.trieDB == nil {
		return common.Hash{}, nil
	}
	return m.trieDB.GetRoot(), nil
}
