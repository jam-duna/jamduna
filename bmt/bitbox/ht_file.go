package bitbox

import (
	"fmt"
	"os"

	"github.com/jam-duna/jamduna/bmt/io"
)

const PageSize = 16384 // 16KB pages
const PageIdSize = 32  // PageId is 32 bytes

// HTOffsets tracks the layout of the hash table file.
// The HT file layout is:
// - Meta byte pages (MetaMap data, 1 byte per bucket, packed into pages)
// - Data pages (actual page data for each bucket)
type HTOffsets struct {
	dataPageOffset uint64 // Number of meta pages before data section
}

// NewHTOffsets creates HTOffsets for the given number of buckets.
func NewHTOffsets(numBuckets uint32) *HTOffsets {
	numMetaPages := numMetaBytePagesForBuckets(numBuckets)
	return &HTOffsets{
		dataPageOffset: uint64(numMetaPages),
	}
}

// DataPageIndex returns the file page index for the ix'th data bucket.
func (h *HTOffsets) DataPageIndex(bucketIndex uint64) uint64 {
	return h.dataPageOffset + bucketIndex
}

// MetaBytesIndex returns the file page index for the ix'th meta page.
func (h *HTOffsets) MetaBytesIndex(pageIndex uint64) uint64 {
	return pageIndex
}

// numMetaBytePagesForBuckets calculates how many 16KB pages are needed
// to store 1 byte of metadata for each bucket.
func numMetaBytePagesForBuckets(numBuckets uint32) uint32 {
	// Each page holds 16384 bytes of metadata
	// Round up: (numBuckets + 16383) / 16384
	return (numBuckets + PageSize - 1) / PageSize
}

// ExpectedFileLen returns the expected file length for a hash table
// with the given number of buckets.
// Format: [Meta pages (16KB each)][Buckets (16416 bytes each: 32-byte PageId + 16KB data)]
func ExpectedFileLen(numBuckets uint32) int64 {
	numMetaPages := numMetaBytePagesForBuckets(numBuckets)
	metaSize := int64(numMetaPages) * PageSize
	bucketSize := int64(numBuckets) * (32 + PageSize) // PageId + Page Data per bucket
	return metaSize + bucketSize
}

// CreateHTFile creates a new hash table file with the given number of buckets.
func CreateHTFile(path string, numBuckets uint32) error {
	// Create the file
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("failed to create HT file: %w", err)
	}
	defer file.Close()

	// Calculate required size
	fileLen := ExpectedFileLen(numBuckets)

	// Truncate to the required size (allocates space)
	if err := file.Truncate(fileLen); err != nil {
		return fmt.Errorf("failed to truncate HT file: %w", err)
	}

	// Sync to disk
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync HT file: %w", err)
	}

	return nil
}

// OpenHTFile opens an existing hash table file and reads the MetaMap.
func OpenHTFile(path string, numBuckets uint32, pagePool *io.PagePool) (*os.File, *HTOffsets, *MetaMap, error) {
	// Open the file
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to open HT file: %w", err)
	}

	// Check file size
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, nil, fmt.Errorf("failed to stat HT file: %w", err)
	}

	expectedLen := ExpectedFileLen(numBuckets)
	if stat.Size() != expectedLen {
		file.Close()
		// format: buckets are 16,416 bytes (32-byte PageId prefix + 16KB data)
		return nil, nil, nil, fmt.Errorf("HT file size mismatch (possible format version incompatibility): expected %d, got %d", expectedLen, stat.Size())
	}

	// Create HTOffsets
	offsets := NewHTOffsets(numBuckets)

	// Read meta pages
	numMetaPages := numMetaBytePagesForBuckets(numBuckets)
	metaBytes := make([]byte, 0, int(numMetaPages)*PageSize)

	for i := uint32(0); i < numMetaPages; i++ {
		page, err := io.ReadPage(file, pagePool, uint64(i))
		if err != nil {
			file.Close()
			return nil, nil, nil, fmt.Errorf("failed to read meta page %d: %w", i, err)
		}
		metaBytes = append(metaBytes, page...)
		pagePool.Dealloc(page)
	}

	// Create MetaMap
	metaMap := FromBytes(metaBytes, int(numBuckets))

	return file, offsets, metaMap, nil
}
