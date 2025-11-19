package wal

import (
	"bytes"
	"testing"
)

func TestBlobBuilder(t *testing.T) {
	builder := NewWalBlobBuilder()

	// Initially empty
	if builder.Len() != 0 {
		t.Errorf("New builder should be empty, got len=%d", builder.Len())
	}

	// Write start marker
	builder.WriteStart()
	if builder.Len() != 1 {
		t.Errorf("After WriteStart, len should be 1, got %d", builder.Len())
	}

	// Write a clear entry
	builder.WriteClear(42)
	expectedLen := 1 + 1 + 8 // start + tag + bucket_index
	if builder.Len() != expectedLen {
		t.Errorf("After WriteClear, len should be %d, got %d", expectedLen, builder.Len())
	}

	// Write an update entry
	pageId := make([]byte, 32)
	for i := range pageId {
		pageId[i] = byte(i)
	}
	pageData := make([]byte, 16384)
	for i := range pageData {
		pageData[i] = byte(i % 256)
	}
	builder.WriteUpdate(0, pageId, pageData)
	expectedLen = 1 + 1 + 8 + 1 + 8 + 32 + 16384 // start + clear + update(bucket+id+data)
	if builder.Len() != expectedLen {
		t.Errorf("After WriteUpdate, len should be %d, got %d", expectedLen, builder.Len())
	}

	// Write end marker
	builder.WriteEnd()
	expectedLen = 1 + 1 + 8 + 1 + 8 + 32 + 16384 + 1 // all + end
	if builder.Len() != expectedLen {
		t.Errorf("After WriteEnd, len should be %d, got %d", expectedLen, builder.Len())
	}

	// Get bytes
	blob := builder.Bytes()
	if len(blob) != expectedLen {
		t.Errorf("Blob length should be %d, got %d", expectedLen, len(blob))
	}

	// Verify tags
	if blob[0] != WalEntryTagStart {
		t.Errorf("First byte should be START tag")
	}
	if blob[1] != WalEntryTagClear {
		t.Errorf("Second byte should be CLEAR tag")
	}
	if blob[10] != WalEntryTagUpdate {
		t.Errorf("Tag at position 10 should be UPDATE tag")
	}
	if blob[len(blob)-1] != WalEntryTagEnd {
		t.Errorf("Last byte should be END tag")
	}
}

func TestBlobReader(t *testing.T) {
	// Create a WAL blob manually
	var buf []byte
	buf = append(buf, WalEntryTagStart)

	// Clear entry
	buf = append(buf, WalEntryTagClear)
	bucketBytes := make([]byte, 8)
	bucketBytes[0] = 100 // bucket index = 100 (little endian)
	buf = append(buf, bucketBytes...)

	// Update entry (format: TAG + bucket_index + pageId + pageData)
	buf = append(buf, WalEntryTagUpdate)

	// Bucket index (8 bytes, little endian)
	updateBucketBytes := make([]byte, 8)
	updateBucketBytes[0] = 200 // bucket index = 200
	buf = append(buf, updateBucketBytes...)

	pageId := make([]byte, 32)
	for i := range pageId {
		pageId[i] = byte(i * 2)
	}
	buf = append(buf, pageId...)
	pageData := make([]byte, 16384)
	for i := range pageData {
		pageData[i] = 0xFF
	}
	buf = append(buf, pageData...)

	// End marker
	buf = append(buf, WalEntryTagEnd)

	// Read the blob
	reader := NewWalBlobReader(buf)

	// First entry should be Clear
	entry1, err := reader.Next()
	if err != nil {
		t.Fatalf("Failed to read first entry: %v", err)
	}
	if entry1 == nil {
		t.Fatalf("First entry should not be nil")
	}
	if entry1.Type != WalEntryClear {
		t.Errorf("First entry should be Clear, got type %v", entry1.Type)
	}
	if entry1.BucketIndex != 100 {
		t.Errorf("Bucket index should be 100, got %d", entry1.BucketIndex)
	}

	// Second entry should be Update
	entry2, err := reader.Next()
	if err != nil {
		t.Fatalf("Failed to read second entry: %v", err)
	}
	if entry2 == nil {
		t.Fatalf("Second entry should not be nil")
	}
	if entry2.Type != WalEntryUpdate {
		t.Errorf("Second entry should be Update, got type %v", entry2.Type)
	}
	if len(entry2.PageId) != 32 {
		t.Errorf("PageId should be 32 bytes, got %d", len(entry2.PageId))
	}
	if entry2.PageId[0] != 0 || entry2.PageId[1] != 2 {
		t.Errorf("PageId content incorrect")
	}
	if len(entry2.PageData) != 16384 {
		t.Errorf("PageData should be 16384 bytes, got %d", len(entry2.PageData))
	}
	if entry2.PageData[0] != 0xFF {
		t.Errorf("PageData content incorrect")
	}

	// No more entries (end marker consumed)
	entry3, err := reader.Next()
	if err != nil {
		t.Fatalf("Should not error after end marker: %v", err)
	}
	if entry3 != nil {
		t.Errorf("Should return nil after end marker")
	}
}

func TestBlobRoundtrip(t *testing.T) {
	// Build a WAL blob
	builder := NewWalBlobBuilder()
	builder.WriteStart()

	// Add multiple entries
	builder.WriteClear(123)
	builder.WriteClear(456)

	pageId1 := make([]byte, 32)
	for i := range pageId1 {
		pageId1[i] = byte(i)
	}
	pageData1 := make([]byte, 16384)
	for i := range pageData1 {
		pageData1[i] = byte(i % 256)
	}
	builder.WriteUpdate(0, pageId1, pageData1)

	pageId2 := make([]byte, 32)
	for i := range pageId2 {
		pageId2[i] = byte(255 - i)
	}
	pageData2 := make([]byte, 16384)
	for i := range pageData2 {
		pageData2[i] = byte((i * 3) % 256)
	}
	builder.WriteUpdate(0, pageId2, pageData2)

	builder.WriteEnd()

	// Read it back
	blob := builder.Bytes()
	reader := NewWalBlobReader(blob)

	// Entry 1: Clear(123)
	e1, err := reader.Next()
	if err != nil || e1 == nil {
		t.Fatalf("Failed to read entry 1: %v", err)
	}
	if e1.Type != WalEntryClear || e1.BucketIndex != 123 {
		t.Errorf("Entry 1 mismatch: type=%v, bucket=%d", e1.Type, e1.BucketIndex)
	}

	// Entry 2: Clear(456)
	e2, err := reader.Next()
	if err != nil || e2 == nil {
		t.Fatalf("Failed to read entry 2: %v", err)
	}
	if e2.Type != WalEntryClear || e2.BucketIndex != 456 {
		t.Errorf("Entry 2 mismatch: type=%v, bucket=%d", e2.Type, e2.BucketIndex)
	}

	// Entry 3: Update(pageId1, pageData1)
	e3, err := reader.Next()
	if err != nil || e3 == nil {
		t.Fatalf("Failed to read entry 3: %v", err)
	}
	if e3.Type != WalEntryUpdate {
		t.Errorf("Entry 3 should be Update")
	}
	if !bytes.Equal(e3.PageId, pageId1) {
		t.Errorf("Entry 3 pageId mismatch")
	}
	if !bytes.Equal(e3.PageData, pageData1) {
		t.Errorf("Entry 3 pageData mismatch")
	}

	// Entry 4: Update(pageId2, pageData2)
	e4, err := reader.Next()
	if err != nil || e4 == nil {
		t.Fatalf("Failed to read entry 4: %v", err)
	}
	if e4.Type != WalEntryUpdate {
		t.Errorf("Entry 4 should be Update")
	}
	if !bytes.Equal(e4.PageId, pageId2) {
		t.Errorf("Entry 4 pageId mismatch")
	}
	if !bytes.Equal(e4.PageData, pageData2) {
		t.Errorf("Entry 4 pageData mismatch")
	}

	// No more entries
	e5, err := reader.Next()
	if err != nil {
		t.Fatalf("Should not error after all entries: %v", err)
	}
	if e5 != nil {
		t.Errorf("Should return nil after all entries")
	}
}

func TestBlobReaderMissingStartMarker(t *testing.T) {
	// Create a WAL blob WITHOUT a start marker - should fail
	var buf []byte

	// Clear entry (without start marker)
	buf = append(buf, WalEntryTagClear)
	bucketBytes := make([]byte, 8)
	bucketBytes[0] = 100
	buf = append(buf, bucketBytes...)

	reader := NewWalBlobReader(buf)

	// Should fail with missing start marker error
	entry, err := reader.Next()
	if err == nil {
		t.Errorf("Expected error for missing start marker, got nil")
	}
	if entry != nil {
		t.Errorf("Expected nil entry for missing start marker, got %v", entry)
	}
	if err != nil && err.Error() != "WAL blob missing start marker" {
		t.Errorf("Expected 'WAL blob missing start marker' error, got: %v", err)
	}
}

func TestBlobReaderMissingStartMarkerUpdate(t *testing.T) {
	// Create a WAL blob with Update but no start marker
	var buf []byte

	buf = append(buf, WalEntryTagUpdate)
	pageId := make([]byte, 32)
	buf = append(buf, pageId...)
	pageData := make([]byte, 16384)
	buf = append(buf, pageData...)

	reader := NewWalBlobReader(buf)

	entry, err := reader.Next()
	if err == nil {
		t.Errorf("Expected error for missing start marker, got nil")
	}
	if entry != nil {
		t.Errorf("Expected nil entry for missing start marker, got %v", entry)
	}
	if err != nil && err.Error() != "WAL blob missing start marker" {
		t.Errorf("Expected 'WAL blob missing start marker' error, got: %v", err)
	}
}

func TestBlobReaderEmptyBlob(t *testing.T) {
	// Empty blob should work fine (no validation needed)
	reader := NewWalBlobReader([]byte{})

	entry, err := reader.Next()
	if err != nil {
		t.Errorf("Empty blob should not error, got: %v", err)
	}
	if entry != nil {
		t.Errorf("Empty blob should return nil entry, got %v", entry)
	}
}
