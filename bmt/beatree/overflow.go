package beatree

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
)

// Overflow page constants
const (
	// PAGE_SIZE is the size of a page (16KB)
	PAGE_SIZE = 16384

	// HEADER_SIZE is the size of the overflow page header (4 bytes)
	OVERFLOW_HEADER_SIZE = 4

	// BODY_SIZE is the size of the overflow page body
	OVERFLOW_BODY_SIZE = PAGE_SIZE - OVERFLOW_HEADER_SIZE // 16380 bytes

	// MAX_PNS is the maximum number of page numbers that can fit in a page body
	MAX_PNS = OVERFLOW_BODY_SIZE / 4 // 4095 page numbers

	// MAX_OVERFLOW_CELL_NODE_POINTERS is the maximum number of page pointers in a cell
	MAX_OVERFLOW_CELL_NODE_POINTERS = 15

	// MAX_OVERFLOW_VALUE_SIZE is the maximum size of an overflow value (512 MB)
	MAX_OVERFLOW_VALUE_SIZE = 1 << 29
)

// OverflowCell represents an overflow cell stored in a leaf node.
// Format: value_size (8 bytes) + value_hash (32 bytes) + page_numbers (4 bytes each)
type OverflowCell struct {
	ValueSize uint64
	ValueHash [32]byte
	Pages     []allocator.PageNumber
}

// EncodeCell encodes an overflow cell to bytes.
// Format: value_size (8) + value_hash (32) + page_numbers (N * 4)
func EncodeCell(valueSize uint64, valueHash [32]byte, pages []allocator.PageNumber) []byte {
	if valueSize > MAX_OVERFLOW_VALUE_SIZE {
		panic("value size exceeds MAX_OVERFLOW_VALUE_SIZE")
	}

	// Allocate buffer: 8 (size) + 32 (hash) + len(pages)*4
	buf := make([]byte, 40+len(pages)*4)

	// Write value size (little-endian)
	binary.LittleEndian.PutUint64(buf[0:8], valueSize)

	// Write value hash
	copy(buf[8:40], valueHash[:])

	// Write page numbers (little-endian)
	offset := 40
	for _, pn := range pages {
		binary.LittleEndian.PutUint32(buf[offset:offset+4], pn.Value())
		offset += 4
	}

	return buf
}

// DecodeCell decodes an overflow cell from bytes.
// Returns (value_size, value_hash, page_numbers).
func DecodeCell(raw []byte) (uint64, [32]byte, []allocator.PageNumber, error) {
	// Minimum size: 8 (size) + 32 (hash) + 4 (at least one page)
	if len(raw) < 44 {
		return 0, [32]byte{}, nil, fmt.Errorf("overflow cell too small: %d bytes", len(raw))
	}

	// Must be multiple of 4
	if len(raw)%4 != 0 {
		return 0, [32]byte{}, nil, fmt.Errorf("overflow cell size not multiple of 4: %d bytes", len(raw))
	}

	// Read value size
	valueSize := binary.LittleEndian.Uint64(raw[0:8])
	if valueSize > MAX_OVERFLOW_VALUE_SIZE {
		return 0, [32]byte{}, nil, fmt.Errorf("value size exceeds max: %d", valueSize)
	}

	// Read value hash
	var valueHash [32]byte
	copy(valueHash[:], raw[8:40])

	// Read page numbers
	numPages := (len(raw) - 40) / 4
	pages := make([]allocator.PageNumber, numPages)
	offset := 40
	for i := 0; i < numPages; i++ {
		pn := binary.LittleEndian.Uint32(raw[offset : offset+4])
		pages[i] = allocator.PageNumberFromValue(pn)
		offset += 4
	}

	return valueSize, valueHash, pages, nil
}

// TotalNeededPages calculates the total number of pages needed to store a value.
// This accounts for both value bytes and page number overhead.
func TotalNeededPages(valueSize int) int {
	// Calculate pages needed for raw value
	neededPagesRawValue := neededPages(valueSize)

	// If fits in cell, we're done
	if neededPagesRawValue <= MAX_OVERFLOW_CELL_NODE_POINTERS {
		return neededPagesRawValue
	}

	// Calculate unused space that can store page numbers
	bytesLeft := (neededPagesRawValue * OVERFLOW_BODY_SIZE) - valueSize
	availablePageNumbers := bytesLeft / 4

	// Check if we have enough space for remaining page numbers
	if neededPagesRawValue <= MAX_OVERFLOW_CELL_NODE_POINTERS+availablePageNumbers {
		return neededPagesRawValue
	}

	// Calculate additional pages needed for page number storage
	// Based on Rust formula: rp = ceil(n / (BODY_SIZE - 4))
	// where n = (valueSize + (np - M) * 4 - (np * BODY_SIZE))
	n := valueSize + (neededPagesRawValue-MAX_OVERFLOW_CELL_NODE_POINTERS)*4 -
		(neededPagesRawValue * OVERFLOW_BODY_SIZE)

	requiredAdditionalPages := (n + OVERFLOW_BODY_SIZE - 3) / (OVERFLOW_BODY_SIZE - 4)

	return neededPagesRawValue + requiredAdditionalPages
}

// neededPages calculates how many pages are needed to store size bytes.
func neededPages(size int) int {
	return (size + OVERFLOW_BODY_SIZE - 1) / OVERFLOW_BODY_SIZE
}

// OverflowPage represents a single overflow page.
// Format:
//   n_pointers: u16 (2 bytes)
//   n_bytes: u16 (2 bytes)
//   pointers: [PageNumber; n_pointers] (n_pointers * 4 bytes)
//   bytes: [u8; n_bytes] (n_bytes bytes)
type OverflowPage struct {
	Pointers []allocator.PageNumber
	Bytes    []byte
}

// EncodeOverflowPage encodes an overflow page to 16KB page bytes.
func EncodeOverflowPage(page *OverflowPage) []byte {
	buf := make([]byte, PAGE_SIZE)

	nPointers := len(page.Pointers)
	nBytes := len(page.Bytes)

	if nPointers > MAX_PNS {
		panic(fmt.Sprintf("too many pointers: %d > %d", nPointers, MAX_PNS))
	}

	if nBytes > OVERFLOW_BODY_SIZE-nPointers*4 {
		panic(fmt.Sprintf("too many bytes: %d", nBytes))
	}

	// Write header
	binary.LittleEndian.PutUint16(buf[0:2], uint16(nPointers))
	binary.LittleEndian.PutUint16(buf[2:4], uint16(nBytes))

	// Write pointers
	offset := OVERFLOW_HEADER_SIZE
	for _, pn := range page.Pointers {
		binary.LittleEndian.PutUint32(buf[offset:offset+4], pn.Value())
		offset += 4
	}

	// Write bytes
	copy(buf[offset:offset+nBytes], page.Bytes)

	return buf
}

// DecodeOverflowPage decodes an overflow page from 16KB page bytes.
func DecodeOverflowPage(raw []byte) (*OverflowPage, error) {
	if len(raw) != PAGE_SIZE {
		return nil, fmt.Errorf("invalid page size: %d", len(raw))
	}

	// Read header
	nPointers := int(binary.LittleEndian.Uint16(raw[0:2]))
	nBytes := int(binary.LittleEndian.Uint16(raw[2:4]))

	if nPointers > MAX_PNS {
		return nil, fmt.Errorf("too many pointers: %d", nPointers)
	}

	if nBytes > OVERFLOW_BODY_SIZE-nPointers*4 {
		return nil, fmt.Errorf("too many bytes: %d", nBytes)
	}

	// Read pointers
	pointers := make([]allocator.PageNumber, nPointers)
	offset := OVERFLOW_HEADER_SIZE
	for i := 0; i < nPointers; i++ {
		pn := binary.LittleEndian.Uint32(raw[offset : offset+4])
		pointers[i] = allocator.PageNumberFromValue(pn)
		offset += 4
	}

	// Read bytes
	bytes := make([]byte, nBytes)
	copy(bytes, raw[offset:offset+nBytes])

	return &OverflowPage{
		Pointers: pointers,
		Bytes:    bytes,
	}, nil
}
