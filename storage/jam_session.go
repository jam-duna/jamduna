package storage

import (
	"github.com/colorfulnotion/jam/common"
)

// JAMTrieSession represents an isolated trie session with its own root and staged operations.
// Multiple sessions can share a single JAMTrieBackend for committed node data.
type JAMTrieSession struct {
	backend    *JAMTrieBackend // Shared across sessions
	trie       *MerkleTree     // Underlying trie engine
	parentRoot common.Hash     // Root this session was created from
}

func NewJAMTrieSession(backend *JAMTrieBackend) *JAMTrieSession {
	if backend == nil {
		backend = NewJAMTrieBackend(nil)
	}
	return &JAMTrieSession{
		backend:    backend,
		trie:       NewMerkleTreeWithBackend(nil, backend),
		parentRoot: common.Hash{},
	}
}

func NewJAMTrieSessionFromRoot(backend *JAMTrieBackend, root common.Hash) (*JAMTrieSession, error) {
	if backend == nil {
		backend = NewJAMTrieBackend(nil)
	}
	trie := NewMerkleTreeWithBackend(nil, backend)
	if root != (common.Hash{}) {
		if err := trie.SetRoot(root); err != nil {
			return nil, err
		}
	}
	return &JAMTrieSession{
		backend:    backend,
		trie:       trie,
		parentRoot: root,
	}, nil
}

func (s *JAMTrieSession) Backend() *JAMTrieBackend {
	return s.backend
}

func (s *JAMTrieSession) Trie() *MerkleTree {
	return s.trie
}

func (s *JAMTrieSession) Root() common.Hash {
	return s.trie.GetRoot()
}

func (s *JAMTrieSession) ParentRoot() common.Hash {
	return s.parentRoot
}

func (s *JAMTrieSession) SetRoot(root common.Hash) error {
	return s.trie.SetRoot(root)
}

func (s *JAMTrieSession) Insert(key, value []byte) {
	s.trie.Insert(key, value)
}

func (s *JAMTrieSession) Delete(key []byte) error {
	return s.trie.Delete(key)
}

func (s *JAMTrieSession) Get(key []byte) ([]byte, bool, error) {
	return s.trie.Get(key)
}

func (s *JAMTrieSession) Finish() (common.Hash, error) {
	return s.trie.Flush()
}

func (s *JAMTrieSession) Rollback() {
	s.trie.ClearStagedOps()
}

// CopySession creates a new session sharing the backend but with isolated staged ops.
func (s *JAMTrieSession) CopySession() *JAMTrieSession {
	trieCopy := s.trie.Copy()
	return &JAMTrieSession{
		backend:    s.backend,
		trie:       trieCopy,
		parentRoot: s.trie.GetRoot(),
	}
}

func (s *JAMTrieSession) HasStagedChanges() bool {
	return s.trie.GetStagedSize() > 0
}
