package trie

import (
	"github.com/colorfulnotion/jam/common"
)

type BMTInterface interface {
	Insert(key31 []byte, value []byte) error
	Delete(key []byte) error
	Get(key []byte) ([]byte, bool, error)
	Trace(key []byte) ([][]byte, error)
	Verify(key []byte, value []byte, rootHash []byte, path []common.Hash) bool
	Flush() error
}
