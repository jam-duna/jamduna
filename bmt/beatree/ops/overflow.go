package ops

import (
	"fmt"

	"github.com/jam-duna/jamduna/bmt/beatree"
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

// ReadOverflow reads a large value from pages referenced by an overflow cell.
// This uses blocking I/O to load all pages synchronously.
// Works with any PageStore implementation (TestStore for tests, allocator.Store for production).
//
// Future enhancement: async I/O support for better concurrency.
func ReadOverflow(cell []byte, leafStore PageStore) ([]byte, error) {
	// Decode the cell to get value size and page numbers
	valueSize, _, cellPages, err := beatree.DecodeCell(cell)
	if err != nil {
		return nil, fmt.Errorf("failed to decode overflow cell: %w", err)
	}

	// Calculate total pages needed
	totalPages := beatree.TotalNeededPages(int(valueSize))

	// Allocate result buffer
	value := make([]byte, 0, valueSize)

	// Collect all page numbers (start with cell pages, then read more from pages)
	pageNumbers := make([]allocator.PageNumber, 0, totalPages)
	pageNumbers = append(pageNumbers, cellPages...)

	// Read pages and collect both data and additional page numbers
	for i := 0; i < totalPages; i++ {
		if i >= len(pageNumbers) {
			return nil, fmt.Errorf("not enough page numbers: need %d, have %d", totalPages, len(pageNumbers))
		}

		// Read the page
		pageData, err := readPage(leafStore, pageNumbers[i])
		if err != nil {
			return nil, fmt.Errorf("failed to read overflow page %d: %w", pageNumbers[i], err)
		}

		// Decode overflow page
		overflowPage, err := beatree.DecodeOverflowPage(pageData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode overflow page: %w", err)
		}

		// Add any page numbers from this page
		pageNumbers = append(pageNumbers, overflowPage.Pointers...)

		// Add data bytes from this page
		value = append(value, overflowPage.Bytes...)
	}

	// Verify we got the right amount of data
	if len(value) != int(valueSize) {
		return nil, fmt.Errorf("overflow value size mismatch: expected %d, got %d", valueSize, len(value))
	}

	// Verify we got the right number of pages
	if len(pageNumbers) != totalPages {
		return nil, fmt.Errorf("overflow page count mismatch: expected %d, got %d", totalPages, len(pageNumbers))
	}

	return value, nil
}

// WriteOverflow writes a large value to overflow pages and returns the cell data.
// This allocates pages, chunks the value, and writes all overflow pages.
//
// Returns the encoded cell (to be stored in the leaf) and the list of allocated pages.
// Works with any PageStore implementation (TestStore for tests, allocator.Store for production).
//
// Future enhancement: async I/O support for better concurrency.
func WriteOverflow(value []byte, valueHash [32]byte, leafStore PageStore) ([]byte, []allocator.PageNumber, error) {
	if len(value) == 0 {
		return nil, nil, fmt.Errorf("cannot write empty overflow value")
	}

	if len(value) > beatree.MAX_OVERFLOW_VALUE_SIZE {
		return nil, nil, fmt.Errorf("value too large: %d > %d", len(value), beatree.MAX_OVERFLOW_VALUE_SIZE)
	}

	// Calculate how many pages we need
	totalPages := beatree.TotalNeededPages(len(value))
	cellPageCount := min(totalPages, beatree.MAX_OVERFLOW_CELL_NODE_POINTERS)

	// Allocate all pages
	allPages := make([]allocator.PageNumber, totalPages)
	for i := 0; i < totalPages; i++ {
		allPages[i] = leafStore.Alloc()
	}

	// Cell pages (first N pages, up to MAX_OVERFLOW_CELL_NODE_POINTERS)
	cellPages := allPages[:cellPageCount]

	// Remaining pages that need to be referenced from overflow pages
	otherPages := allPages[cellPageCount:]
	toWrite := otherPages

	// Write data to pages
	valueRemaining := value
	for _, pageNum := range allPages {
		if len(valueRemaining) == 0 {
			break
		}

		// Build overflow page
		page := &beatree.OverflowPage{}

		// Write as many page numbers as possible (up to MAX_PNS)
		pnsToWrite := min(len(toWrite), beatree.MAX_PNS)
		if pnsToWrite > 0 {
			page.Pointers = make([]allocator.PageNumber, pnsToWrite)
			copy(page.Pointers, toWrite[:pnsToWrite])
			toWrite = toWrite[pnsToWrite:]
		}

		// Calculate how many value bytes we can fit
		availableBytes := beatree.OVERFLOW_BODY_SIZE - len(page.Pointers)*4
		bytesToWrite := min(availableBytes, len(valueRemaining))

		// Write value bytes
		page.Bytes = make([]byte, bytesToWrite)
		copy(page.Bytes, valueRemaining[:bytesToWrite])
		valueRemaining = valueRemaining[bytesToWrite:]

		// Encode and write the page
		pageData := beatree.EncodeOverflowPage(page)
		if err := writePage(leafStore, pageNum, pageData); err != nil {
			return nil, nil, fmt.Errorf("failed to write overflow page: %w", err)
		}
	}

	// Verify all data was written
	if len(valueRemaining) != 0 {
		return nil, nil, fmt.Errorf("failed to write all data: %d bytes remaining", len(valueRemaining))
	}

	// Encode the cell
	cell := beatree.EncodeCell(uint64(len(value)), valueHash, cellPages)

	return cell, allPages, nil
}

// readPage reads a page from the store using the PageStore interface.
func readPage(store PageStore, pageNum allocator.PageNumber) ([]byte, error) {
	pageData, err := store.Page(pageNum)
	if err != nil {
		return nil, fmt.Errorf("failed to read page %d: %w", pageNum, err)
	}

	return pageData, nil
}

// writePage writes a page to the store using the PageStore interface.
func writePage(store PageStore, pageNum allocator.PageNumber, data []byte) error {
	if len(data) != beatree.PAGE_SIZE {
		return fmt.Errorf("invalid page size: %d (expected %d)", len(data), beatree.PAGE_SIZE)
	}

	store.WritePage(pageNum, data)
	return nil
}
