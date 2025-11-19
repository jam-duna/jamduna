package storage

import (
	"fmt"
	"os"

	"github.com/colorfulnotion/jam/bmt"
	"github.com/colorfulnotion/jam/common"
)

// MockStorage provides a minimal storage implementation for testing
type MockStorage struct {
	bmtDB  *bmt.Nomt
	tmpDir string
}

// NewMockStorage creates a temporary BMT storage for testing
func NewMockStorage() (*MockStorage, error) {
	tmpDir, err := os.MkdirTemp("", "bmt-mock-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %v", err)
	}

	// Create BMT database with default options
	opts := bmt.DefaultOptions(tmpDir)
	bmtDB, err := bmt.Open(opts)
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to create mock BMT database: %v", err)
	}

	return &MockStorage{
		bmtDB:  bmtDB,
		tmpDir: tmpDir,
	}, nil
}

func (m *MockStorage) Close() error {
	if m.bmtDB != nil {
		err := m.bmtDB.Close()
		m.bmtDB = nil
		// Clean up temp directory
		if m.tmpDir != "" {
			os.RemoveAll(m.tmpDir)
			m.tmpDir = ""
		}
		return err
	}
	return nil
}

// Delete implements JAMStorage interface
func (m *MockStorage) Delete(key []byte) error {
	if m.bmtDB == nil {
		return fmt.Errorf("mock storage is closed")
	}
	var key32 [32]byte
	copy(key32[:], key)
	return m.bmtDB.Delete(key32)
}

// Insert implements JAMStorage interface
func (m *MockStorage) Insert(key []byte, value []byte) {
	if m.bmtDB == nil {
		return
	}
	var key32 [32]byte
	copy(key32[:], key)
	m.bmtDB.Insert(key32, value)
}

// Get implements JAMStorage interface
func (m *MockStorage) Get(key []byte) ([]byte, bool, error) {
	if m.bmtDB == nil {
		return nil, false, fmt.Errorf("mock storage is closed")
	}
	var key32 [32]byte
	copy(key32[:], key)
	value, err := m.bmtDB.Get(key32)
	if err != nil {
		return nil, false, err
	}
	return value, value != nil, nil
}

// Flush implements JAMStorage interface
func (m *MockStorage) Flush() error {
	if m.bmtDB == nil {
		return fmt.Errorf("mock storage is closed")
	}
	_, err := m.bmtDB.Commit()
	return err
}

// GetRoot implements JAMStorage interface
func (m *MockStorage) GetRoot() []byte {
	if m.bmtDB == nil {
		return make([]byte, 32)
	}
	root := m.bmtDB.Root()
	return root[:]
}

// OverlayRoot implements JAMStorage interface - compute root from staged overlay without commit
func (m *MockStorage) OverlayRoot() (common.Hash, error) {
	if m.bmtDB == nil {
		return common.Hash{}, nil
	}
	currentRoot, err := m.bmtDB.CurrentRoot()
	if err != nil {
		return common.Hash{}, err
	}
	return common.BytesToHash(currentRoot[:]), nil
}
