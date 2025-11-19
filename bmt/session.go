package bmt

import "github.com/colorfulnotion/jam/bmt/beatree"

// Session represents a read-only snapshot of the database.
// It provides snapshot isolation - all reads see the state at session creation time.
type Session struct {
	root [32]byte
	tree *beatreeWrapper
}

// Get retrieves a value by key from the snapshot.
func (s *Session) Get(key [32]byte) ([]byte, error) {
	k := beatree.KeyFromBytes(key[:])
	return s.tree.Get(k)
}

// Root returns the root hash of this snapshot.
func (s *Session) Root() [32]byte {
	return s.root
}
