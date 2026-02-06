package bitbox

import (
	"github.com/jam-duna/jamduna/bmt/core"
	"github.com/zeebo/xxh3"
)

// HashPageId hashes a PageId to a uint64 using xxhash3 with the given seed.
func HashPageId(pageId core.PageId, seed uint64) uint64 {
	// Encode the PageId to bytes
	encoded := pageId.Encode()

	// Hash using xxhash3 with seed (convert [32]byte to []byte)
	return xxh3.HashSeed(encoded[:], seed)
}
