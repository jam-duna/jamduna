package beatree

import (
	"bytes"
	"testing"

	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
)

// TestEncodeDecodeCell tests overflow cell encoding and decoding.
func TestEncodeDecodeCell(t *testing.T) {
	valueSize := uint64(100000)
	valueHash := [32]byte{1, 2, 3, 4, 5}
	pages := []allocator.PageNumber{
		allocator.PageNumber(10),
		allocator.PageNumber(20),
		allocator.PageNumber(30),
	}

	// Encode
	encoded := EncodeCell(valueSize, valueHash, pages)

	// Decode
	decodedSize, decodedHash, decodedPages, err := DecodeCell(encoded)
	if err != nil {
		t.Fatalf("DecodeCell failed: %v", err)
	}

	// Verify size
	if decodedSize != valueSize {
		t.Errorf("Value size mismatch: expected %d, got %d", valueSize, decodedSize)
	}

	// Verify hash
	if decodedHash != valueHash {
		t.Errorf("Value hash mismatch: expected %v, got %v", valueHash, decodedHash)
	}

	// Verify pages
	if len(decodedPages) != len(pages) {
		t.Fatalf("Pages length mismatch: expected %d, got %d", len(pages), len(decodedPages))
	}

	for i, pn := range pages {
		if decodedPages[i] != pn {
			t.Errorf("Page %d mismatch: expected %v, got %v", i, pn, decodedPages[i])
		}
	}
}

// TestDecodeCellTooSmall tests that DecodeCell rejects cells that are too small.
func TestDecodeCellTooSmall(t *testing.T) {
	// Cell must be at least 44 bytes
	tooSmall := make([]byte, 43)
	_, _, _, err := DecodeCell(tooSmall)
	if err == nil {
		t.Error("Expected error for cell that's too small")
	}
}

// TestDecodeCellNotMultipleOf4 tests that DecodeCell rejects misaligned cells.
func TestDecodeCellNotMultipleOf4(t *testing.T) {
	// Cell size must be multiple of 4
	notAligned := make([]byte, 45)
	_, _, _, err := DecodeCell(notAligned)
	if err == nil {
		t.Error("Expected error for cell not multiple of 4")
	}
}

// TestDecodeCellValueSizeExceeded tests size limit validation.
func TestDecodeCellValueSizeExceeded(t *testing.T) {
	// Create cell with excessive value size
	buf := make([]byte, 44)
	// Write value_size > MAX_OVERFLOW_VALUE_SIZE
	valueSize := uint64(MAX_OVERFLOW_VALUE_SIZE + 1)
	buf[0] = byte(valueSize)
	buf[1] = byte(valueSize >> 8)
	buf[2] = byte(valueSize >> 16)
	buf[3] = byte(valueSize >> 24)
	buf[4] = byte(valueSize >> 32)
	buf[5] = byte(valueSize >> 40)
	buf[6] = byte(valueSize >> 48)
	buf[7] = byte(valueSize >> 56)

	_, _, _, err := DecodeCell(buf)
	if err == nil {
		t.Error("Expected error for value size exceeding max")
	}
}

// TestTotalNeededPagesSmall tests page calculation for small values.
func TestTotalNeededPagesSmall(t *testing.T) {
	// Values that fit in 1-15 pages should return exact page count
	for i := 1; i <= MAX_OVERFLOW_CELL_NODE_POINTERS; i++ {
		size := OVERFLOW_BODY_SIZE * i
		expected := i
		result := TotalNeededPages(size)

		if result != expected {
			t.Errorf("TotalNeededPages(%d): expected %d, got %d", size, expected, result)
		}
	}
}

// TestTotalNeededPagesOneBeyondCell tests the boundary case.
func TestTotalNeededPagesOneBeyondCell(t *testing.T) {
	// One byte beyond MAX_OVERFLOW_CELL_NODE_POINTERS pages
	size := OVERFLOW_BODY_SIZE*MAX_OVERFLOW_CELL_NODE_POINTERS + 1
	expected := MAX_OVERFLOW_CELL_NODE_POINTERS + 1
	result := TotalNeededPages(size)

	if result != expected {
		t.Errorf("TotalNeededPages(%d): expected %d, got %d", size, expected, result)
	}
}

// TestTotalNeededPagesEncodedPageAddMore tests complex page number overhead.
func TestTotalNeededPagesEncodedPageAddMore(t *testing.T) {
	// Last page totally full (including one page number)
	size := OVERFLOW_BODY_SIZE*(MAX_OVERFLOW_CELL_NODE_POINTERS+1) - 4
	expected := MAX_OVERFLOW_CELL_NODE_POINTERS + 1
	result := TotalNeededPages(size)

	if result != expected {
		t.Errorf("TotalNeededPages(%d): expected %d, got %d", size, expected, result)
	}

	// Last page not full - needs extra page for page number
	size = OVERFLOW_BODY_SIZE*(MAX_OVERFLOW_CELL_NODE_POINTERS+1) - 3
	expected = MAX_OVERFLOW_CELL_NODE_POINTERS + 2
	result = TotalNeededPages(size)

	if result != expected {
		t.Errorf("TotalNeededPages(%d): expected %d, got %d", size, expected, result)
	}
}

// TestEncodeDecodeOverflowPage tests overflow page encoding.
func TestEncodeDecodeOverflowPage(t *testing.T) {
	page := &OverflowPage{
		Pointers: []allocator.PageNumber{
			allocator.PageNumber(100),
			allocator.PageNumber(200),
			allocator.PageNumber(300),
		},
		Bytes: []byte("hello world"),
	}

	// Encode
	encoded := EncodeOverflowPage(page)
	if len(encoded) != PAGE_SIZE {
		t.Fatalf("Encoded page has wrong size: %d", len(encoded))
	}

	// Decode
	decoded, err := DecodeOverflowPage(encoded)
	if err != nil {
		t.Fatalf("DecodeOverflowPage failed: %v", err)
	}

	// Verify pointers
	if len(decoded.Pointers) != len(page.Pointers) {
		t.Fatalf("Pointers length mismatch: expected %d, got %d",
			len(page.Pointers), len(decoded.Pointers))
	}

	for i, pn := range page.Pointers {
		if decoded.Pointers[i] != pn {
			t.Errorf("Pointer %d mismatch: expected %v, got %v", i, pn, decoded.Pointers[i])
		}
	}

	// Verify bytes
	if !bytes.Equal(decoded.Bytes, page.Bytes) {
		t.Errorf("Bytes mismatch: expected %v, got %v", page.Bytes, decoded.Bytes)
	}
}

// TestEncodeOverflowPageTooManyPointers tests pointer limit.
func TestEncodeOverflowPageTooManyPointers(t *testing.T) {
	// Create page with too many pointers
	pointers := make([]allocator.PageNumber, MAX_PNS+1)
	page := &OverflowPage{
		Pointers: pointers,
		Bytes:    []byte{},
	}

	// Should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for too many pointers")
		}
	}()

	EncodeOverflowPage(page)
}

// TestEncodeOverflowPageTooManyBytes tests byte limit.
func TestEncodeOverflowPageTooManyBytes(t *testing.T) {
	// Create page with too many bytes (given some pointers)
	page := &OverflowPage{
		Pointers: []allocator.PageNumber{allocator.PageNumber(1)},
		Bytes:    make([]byte, OVERFLOW_BODY_SIZE), // Too big with 1 pointer
	}

	// Should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic for too many bytes")
		}
	}()

	EncodeOverflowPage(page)
}

// TestDecodeOverflowPageInvalidSize tests size validation.
func TestDecodeOverflowPageInvalidSize(t *testing.T) {
	// Create buffer of wrong size
	wrongSize := make([]byte, PAGE_SIZE-1)
	_, err := DecodeOverflowPage(wrongSize)
	if err == nil {
		t.Error("Expected error for wrong page size")
	}
}

// TestOverflowPageMaxPointers tests maximum pointer capacity.
func TestOverflowPageMaxPointers(t *testing.T) {
	// Create page with MAX_PNS pointers and no bytes
	pointers := make([]allocator.PageNumber, MAX_PNS)
	for i := range pointers {
		pointers[i] = allocator.PageNumber(uint64(i))
	}

	page := &OverflowPage{
		Pointers: pointers,
		Bytes:    []byte{},
	}

	// Encode and decode
	encoded := EncodeOverflowPage(page)
	decoded, err := DecodeOverflowPage(encoded)
	if err != nil {
		t.Fatalf("Failed to encode/decode max pointers: %v", err)
	}

	// Verify all pointers preserved
	if len(decoded.Pointers) != MAX_PNS {
		t.Errorf("Expected %d pointers, got %d", MAX_PNS, len(decoded.Pointers))
	}

	for i := 0; i < MAX_PNS; i++ {
		if decoded.Pointers[i] != pointers[i] {
			t.Errorf("Pointer %d mismatch: expected %v, got %v",
				i, pointers[i], decoded.Pointers[i])
		}
	}
}

// TestOverflowPageMaxBytes tests maximum byte capacity.
func TestOverflowPageMaxBytes(t *testing.T) {
	// Create page with no pointers and max bytes
	data := make([]byte, OVERFLOW_BODY_SIZE)
	for i := range data {
		data[i] = byte(i % 256)
	}

	page := &OverflowPage{
		Pointers: []allocator.PageNumber{},
		Bytes:    data,
	}

	// Encode and decode
	encoded := EncodeOverflowPage(page)
	decoded, err := DecodeOverflowPage(encoded)
	if err != nil {
		t.Fatalf("Failed to encode/decode max bytes: %v", err)
	}

	// Verify all bytes preserved
	if len(decoded.Bytes) != OVERFLOW_BODY_SIZE {
		t.Errorf("Expected %d bytes, got %d", OVERFLOW_BODY_SIZE, len(decoded.Bytes))
	}

	if !bytes.Equal(decoded.Bytes, page.Bytes) {
		t.Error("Bytes not preserved in max capacity page")
	}
}
