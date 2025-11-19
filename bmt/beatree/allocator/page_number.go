package allocator

import "fmt"

// PageNumber identifies a page within the beatree storage.
// Page numbers are sequential and start from 0.
type PageNumber uint64

const (
	// InvalidPageNumber represents an invalid/unallocated page.
	InvalidPageNumber PageNumber = 0xFFFFFFFFFFFFFFFF
)

// IsValid returns true if the page number is valid.
func (pn PageNumber) IsValid() bool {
	return pn != InvalidPageNumber
}

// String returns a string representation of the page number.
func (pn PageNumber) String() string {
	if !pn.IsValid() {
		return "PageNumber(invalid)"
	}
	return fmt.Sprintf("PageNumber(%d)", uint64(pn))
}

// Value returns the raw uint32 value of the page number.
// Panics if the page number doesn't fit in uint32.
func (pn PageNumber) Value() uint32 {
	if pn > 0xFFFFFFFF {
		panic(fmt.Sprintf("page number %d too large for uint32", pn))
	}
	return uint32(pn)
}

// PageNumberFromValue creates a PageNumber from a uint32 value.
func PageNumberFromValue(val uint32) PageNumber {
	return PageNumber(val)
}
