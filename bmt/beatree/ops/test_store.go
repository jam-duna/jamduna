package ops

import (
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
	"github.com/colorfulnotion/jam/bmt/beatree/leaf"
)

// TestStore is a simple in-memory page store for testing.
// Stores leaf.Node objects directly without full serialization for simplicity.
type TestStore struct {
	mu    sync.RWMutex
	nodes map[allocator.PageNumber]*leaf.Node
	pages map[allocator.PageNumber][]byte // For overflow pages
	next  allocator.PageNumber
}

// NewTestStore creates a new in-memory test store.
func NewTestStore() *TestStore {
	return &TestStore{
		nodes: make(map[allocator.PageNumber]*leaf.Node),
		pages: make(map[allocator.PageNumber][]byte),
		next:  1, // Start at page 1
	}
}

// Alloc allocates a new page number.
func (s *TestStore) Alloc() allocator.PageNumber {
	s.mu.Lock()
	defer s.mu.Unlock()

	pageNum := s.next
	s.next++
	return pageNum
}

// FreePage marks a page as free (for testing, we just delete it).
func (s *TestStore) FreePage(page allocator.PageNumber) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.nodes, page)
}

// Page returns the raw page data for serialization-based access.
func (s *TestStore) Page(page allocator.PageNumber) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if we have raw page data
	if pageData, exists := s.pages[page]; exists {
		return pageData, nil
	}

	// Check if we have a leaf node that needs to be serialized
	if node, exists := s.nodes[page]; exists {
		// Serialize the node for backward compatibility
		serializedData, err := node.Serialize()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize node for page %d: %w", page, err)
		}

		// Store the serialized data for future access
		s.pages[page] = serializedData
		return serializedData, nil
	}

	return nil, fmt.Errorf("page %d not found", page)
}

// WritePage stores raw page data for serialization-based access.
func (s *TestStore) WritePage(page allocator.PageNumber, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store the raw page data
	s.pages[page] = make([]byte, len(data))
	copy(s.pages[page], data)

	// Remove any cached node object since we now have raw data
	delete(s.nodes, page)
}

// GetNode retrieves a node by page number.
func (s *TestStore) GetNode(page allocator.PageNumber) (*leaf.Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if we have a cached node object
	if node, exists := s.nodes[page]; exists {
		return node, nil
	}

	// Check if we have raw page data that needs to be deserialized
	if pageData, exists := s.pages[page]; exists {
		// Deserialize the page data back to a node
		node, err := leaf.DeserializeLeafNode(pageData)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize page %d: %w", page, err)
		}

		// Cache the deserialized node for future access
		s.mu.RUnlock()
		s.mu.Lock()
		s.nodes[page] = node
		s.mu.Unlock()
		s.mu.RLock()

		return node, nil
	}

	return nil, fmt.Errorf("page %d does not exist", page)
}

// SetNode stores a node at a page number.
func (s *TestStore) SetNode(page allocator.PageNumber, node *leaf.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nodes[page] = node
}
