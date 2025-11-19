package wal

import (
	"encoding/binary"
	"testing"
)

func TestWalClear(t *testing.T) {
	builder := NewWalBlobBuilder()
	builder.WriteStart()
	builder.WriteClear(999)

	blob := builder.Bytes()

	// Should be: start (1) + tag (1 byte) + bucket_index (8 bytes) = 10 bytes
	if len(blob) != 10 {
		t.Errorf("Clear entry with start should be 10 bytes, got %d", len(blob))
	}

	// First byte should be START tag
	if blob[0] != WalEntryTagStart {
		t.Errorf("First byte should be START tag (0x%02x), got 0x%02x", WalEntryTagStart, blob[0])
	}

	// Second byte should be CLEAR tag
	if blob[1] != WalEntryTagClear {
		t.Errorf("Second byte should be CLEAR tag (0x%02x), got 0x%02x", WalEntryTagClear, blob[1])
	}

	// Next 8 bytes should be bucket index (little endian)
	bucketIndex := binary.LittleEndian.Uint64(blob[2:10])
	if bucketIndex != 999 {
		t.Errorf("Bucket index should be 999, got %d", bucketIndex)
	}

	// Test reading it back
	reader := NewWalBlobReader(blob)
	entry, err := reader.Next()
	if err != nil {
		t.Fatalf("Failed to read clear entry: %v", err)
	}
	if entry.Type != WalEntryClear {
		t.Errorf("Entry type should be Clear")
	}
	if entry.BucketIndex != 999 {
		t.Errorf("Bucket index should be 999, got %d", entry.BucketIndex)
	}
}

func TestWalUpdate(t *testing.T) {
	builder := NewWalBlobBuilder()

	// Create test page ID and data
	pageId := make([]byte, 32)
	for i := range pageId {
		pageId[i] = byte(i + 10)
	}

	pageData := make([]byte, 16384)
	for i := range pageData {
		pageData[i] = byte((i * 7) % 256)
	}

	builder.WriteStart()
	builder.WriteUpdate(0, pageId, pageData)

	blob := builder.Bytes()

	// Should be: start (1) + tag (1) + bucket (8) + pageId (32) + pageData (16384) = 16426 bytes
	expectedLen := 1 + 1 + 8 + 32 + 16384
	if len(blob) != expectedLen {
		t.Errorf("Update entry should be %d bytes, got %d", expectedLen, len(blob))
	}

	// First byte should be START tag
	if blob[0] != WalEntryTagStart {
		t.Errorf("First byte should be START tag (0x%02x), got 0x%02x", WalEntryTagStart, blob[0])
	}

	// Second byte should be UPDATE tag
	if blob[1] != WalEntryTagUpdate {
		t.Errorf("Second byte should be UPDATE tag (0x%02x), got 0x%02x", WalEntryTagUpdate, blob[1])
	}

	// Next 8 bytes should be bucket index (0 in little endian)
	// Skip verification for simplicity

	// Next 32 bytes should be pageId (after start + update + bucket)
	for i := 0; i < 32; i++ {
		if blob[10+i] != pageId[i] {
			t.Errorf("PageId byte %d mismatch: expected %d, got %d", i, pageId[i], blob[10+i])
			break
		}
	}

	// Next 16384 bytes should be pageData
	for i := 0; i < 16384; i++ {
		if blob[42+i] != pageData[i] {
			t.Errorf("PageData byte %d mismatch: expected %d, got %d", i, pageData[i], blob[42+i])
			break
		}
	}

	// Test reading it back
	reader := NewWalBlobReader(blob)
	entry, err := reader.Next()
	if err != nil {
		t.Fatalf("Failed to read update entry: %v", err)
	}
	if entry.Type != WalEntryUpdate {
		t.Errorf("Entry type should be Update")
	}
	if len(entry.PageId) != 32 {
		t.Errorf("PageId should be 32 bytes, got %d", len(entry.PageId))
	}
	if len(entry.PageData) != 16384 {
		t.Errorf("PageData should be 16384 bytes, got %d", len(entry.PageData))
	}

	// Verify content
	for i := 0; i < 32; i++ {
		if entry.PageId[i] != pageId[i] {
			t.Errorf("PageId byte %d mismatch after read", i)
			break
		}
	}
	for i := 0; i < 16384; i++ {
		if entry.PageData[i] != pageData[i] {
			t.Errorf("PageData byte %d mismatch after read", i)
			break
		}
	}
}

func TestWalStartEnd(t *testing.T) {
	builder := NewWalBlobBuilder()

	// Write start marker
	builder.WriteStart()
	if builder.Len() != 1 {
		t.Errorf("After WriteStart, len should be 1, got %d", builder.Len())
	}

	blob1 := builder.Bytes()
	if blob1[0] != WalEntryTagStart {
		t.Errorf("Start marker should be 0x%02x, got 0x%02x", WalEntryTagStart, blob1[0])
	}

	// Write some data
	builder.WriteClear(42)

	// Write end marker
	builder.WriteEnd()
	blob2 := builder.Bytes()

	if blob2[len(blob2)-1] != WalEntryTagEnd {
		t.Errorf("End marker should be 0x%02x, got 0x%02x", WalEntryTagEnd, blob2[len(blob2)-1])
	}

	// Test reading with start/end markers
	reader := NewWalBlobReader(blob2)

	// Start marker should be consumed automatically
	entry, err := reader.Next()
	if err != nil {
		t.Fatalf("Failed to read entry: %v", err)
	}

	// Should get the Clear entry (start skipped)
	if entry.Type != WalEntryClear {
		t.Errorf("First entry should be Clear (start auto-skipped)")
	}

	// Next call should return nil (end marker consumed)
	entry2, err := reader.Next()
	if err != nil {
		t.Fatalf("Should not error after end: %v", err)
	}
	if entry2 != nil {
		t.Errorf("Should return nil after end marker")
	}
}

func TestWalInvalidPageIdSize(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("WriteUpdate should panic with wrong pageId size")
		}
	}()

	builder := NewWalBlobBuilder()
	shortPageId := make([]byte, 10) // Wrong size
	pageData := make([]byte, 16384)
	builder.WriteUpdate(0, shortPageId, pageData)
}

func TestWalInvalidPageDataSize(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("WriteUpdate should panic with wrong pageData size")
		}
	}()

	builder := NewWalBlobBuilder()
	pageId := make([]byte, 32)
	shortPageData := make([]byte, 1000) // Wrong size
	builder.WriteUpdate(0, pageId, shortPageData)
}
