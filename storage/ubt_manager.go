package storage

import (
	"fmt"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
)

// UBTTreeManager manages UBT trees by root hash (shared across sessions).
type UBTTreeManager struct {
	treeStore     map[common.Hash]*UnifiedBinaryTree
	mutex         sync.RWMutex
	canonicalRoot common.Hash
}

func NewUBTTreeManager() *UBTTreeManager {
	return &UBTTreeManager{treeStore: make(map[common.Hash]*UnifiedBinaryTree)}
}

func (m *UBTTreeManager) GetCanonicalRoot() common.Hash {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.canonicalRoot
}

func (m *UBTTreeManager) SetCanonicalRoot(root common.Hash) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.treeStore[root]; !exists {
		return fmt.Errorf("cannot set canonical root to %s: tree not found", root.Hex())
	}
	m.canonicalRoot = root
	return nil
}

func (m *UBTTreeManager) GetTree(root common.Hash) (*UnifiedBinaryTree, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	tree, exists := m.treeStore[root]
	return tree, exists
}

func (m *UBTTreeManager) GetCanonicalTree() *UnifiedBinaryTree {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.treeStore[m.canonicalRoot]
}

func (m *UBTTreeManager) HasTree(root common.Hash) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	_, exists := m.treeStore[root]
	return exists
}

func (m *UBTTreeManager) StoreTree(tree *UnifiedBinaryTree) common.Hash {
	rootBytes := tree.RootHash()
	root := common.BytesToHash(rootBytes[:])
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.treeStore[root]; !exists {
		m.treeStore[root] = tree
	}
	return root
}

func (m *UBTTreeManager) StoreTreeWithRoot(root common.Hash, tree *UnifiedBinaryTree) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.treeStore[root] = tree
}

func (m *UBTTreeManager) NewTree(root common.Hash) (*UnifiedBinaryTree, error) {
	m.mutex.RLock()
	tree, exists := m.treeStore[root]
	m.mutex.RUnlock()
	if !exists {
		return nil, fmt.Errorf("no tree found at root %s", root.Hex())
	}
	return tree.Copy(), nil
}

// DiscardTree removes a tree (will not discard canonical root).
func (m *UBTTreeManager) DiscardTree(root common.Hash) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if root == m.canonicalRoot {
		log.Warn(log.EVM, "DiscardTree: refusing canonical", "root", root.Hex())
		return false
	}
	if _, exists := m.treeStore[root]; exists {
		delete(m.treeStore, root)
		return true
	}
	return false
}

func (m *UBTTreeManager) CommitAsCanonical(root common.Hash) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.treeStore[root]; !exists {
		return fmt.Errorf("cannot commit root %s as canonical: tree not found", root.Hex())
	}
	m.canonicalRoot = root
	log.Info(log.EVM, "CommitAsCanonical", "root", root.Hex(), "treeStoreSize", len(m.treeStore))
	return nil
}

func (m *UBTTreeManager) GetTreeStoreSize() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.treeStore)
}

func (m *UBTTreeManager) ListRoots() []common.Hash {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	roots := make([]common.Hash, 0, len(m.treeStore))
	for r := range m.treeStore {
		roots = append(roots, r)
	}
	return roots
}

func (m *UBTTreeManager) Reset(genesisTree *UnifiedBinaryTree) common.Hash {
	rootBytes := genesisTree.RootHash()
	root := common.BytesToHash(rootBytes[:])
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.treeStore = make(map[common.Hash]*UnifiedBinaryTree)
	m.treeStore[root] = genesisTree
	m.canonicalRoot = root
	return root
}
