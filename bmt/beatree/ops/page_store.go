package ops

import "github.com/colorfulnotion/jam/bmt/beatree/allocator"

// PageStore is the interface for page storage operations.
// This allows both the real Store and TestStore to be used.
type PageStore interface {
	Alloc() allocator.PageNumber
	FreePage(page allocator.PageNumber)
	Page(page allocator.PageNumber) ([]byte, error)
	WritePage(page allocator.PageNumber, data []byte)
}
