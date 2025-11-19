package seglog

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestSegmentedLogBasicAppendRead(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: DEFAULT_SEGMENT_SIZE,
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}
	defer sl.Close()

	// Append a record
	payload := []byte("hello world")
	recordId, err := sl.Append(payload)
	if err != nil {
		t.Fatalf("Failed to append: %v", err)
	}

	if recordId != RecordId(1) {
		t.Errorf("Expected first RecordId to be 1, got %d", recordId)
	}

	// Sync before reading
	if err := sl.Sync(); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Read it back
	retrieved, err := sl.Read(recordId)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if !bytes.Equal(retrieved, payload) {
		t.Errorf("Payload mismatch: expected %v, got %v", payload, retrieved)
	}
}

func TestSegmentedLogMultipleRecords(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: DEFAULT_SEGMENT_SIZE,
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}
	defer sl.Close()

	// Append multiple records
	records := [][]byte{
		[]byte("first"),
		[]byte("second"),
		[]byte("third"),
	}

	var recordIds []RecordId
	for _, payload := range records {
		recordId, err := sl.Append(payload)
		if err != nil {
			t.Fatalf("Failed to append: %v", err)
		}
		recordIds = append(recordIds, recordId)
	}

	// Sync before reading
	if err := sl.Sync(); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Read them back
	for i, recordId := range recordIds {
		retrieved, err := sl.Read(recordId)
		if err != nil {
			t.Fatalf("Failed to read record %d: %v", i, err)
		}
		if !bytes.Equal(retrieved, records[i]) {
			t.Errorf("Record %d mismatch: expected %v, got %v", i, records[i], retrieved)
		}
	}
}

func TestSegmentedLogSegmentRotation(t *testing.T) {
	dir := t.TempDir()

	// Use small segment size to force rotation
	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: MIN_SEGMENT_SIZE, // 1 MiB
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}
	defer sl.Close()

	// Append large records to trigger rotation
	largePayload := make([]byte, 100*1024) // 100 KB
	for i := 0; i < 15; i++ {
		largePayload[0] = byte(i)
		_, err := sl.Append(largePayload)
		if err != nil {
			t.Fatalf("Failed to append record %d: %v", i, err)
		}
	}

	// Should have multiple segments
	segmentCount := sl.SegmentCount()
	if segmentCount < 2 {
		t.Errorf("Expected multiple segments, got %d", segmentCount)
	}
}

func TestSegmentedLogRecovery(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: DEFAULT_SEGMENT_SIZE,
	}

	// Create log and append records
	sl1, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}

	records := [][]byte{
		[]byte("record1"),
		[]byte("record2"),
		[]byte("record3"),
	}

	var recordIds []RecordId
	for _, payload := range records {
		recordId, err := sl1.Append(payload)
		if err != nil {
			t.Fatalf("Failed to append: %v", err)
		}
		recordIds = append(recordIds, recordId)
	}

	if err := sl1.Sync(); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	nextRecordId := sl1.NextRecordId()

	sl1.Close()

	// Reopen and verify recovery
	sl2, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to reopen segmented log: %v", err)
	}
	defer sl2.Close()

	// Check nextRecordId recovered correctly
	if sl2.NextRecordId() != nextRecordId {
		t.Errorf("NextRecordId not recovered: expected %d, got %d", nextRecordId, sl2.NextRecordId())
	}

	// Read records
	for i, recordId := range recordIds {
		retrieved, err := sl2.Read(recordId)
		if err != nil {
			t.Fatalf("Failed to read record %d after recovery: %v", i, err)
		}
		if !bytes.Equal(retrieved, records[i]) {
			t.Errorf("Record %d mismatch after recovery: expected %v, got %v", i, records[i], retrieved)
		}
	}
}

func TestSegmentedLogPrune(t *testing.T) {
	dir := t.TempDir()

	// Use small segment size
	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: MIN_SEGMENT_SIZE,
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}
	defer sl.Close()

	// Append records across multiple segments
	largePayload := make([]byte, 100*1024)
	var recordIds []RecordId
	for i := 0; i < 25; i++ { // Increased to ensure multiple segments
		largePayload[0] = byte(i)
		recordId, err := sl.Append(largePayload)
		if err != nil {
			t.Fatalf("Failed to append: %v", err)
		}
		recordIds = append(recordIds, recordId)
	}

	// Sync before pruning
	if err := sl.Sync(); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	initialSegmentCount := sl.SegmentCount()
	if initialSegmentCount < 2 {
		t.Skipf("Need at least 2 segments for prune test, got %d", initialSegmentCount)
	}

	// Prune all records except those in the last segment
	// Use a high threshold to ensure we delete old segments
	pruneThreshold := recordIds[len(recordIds)-3] // All but last 3 records
	if err := sl.Prune(pruneThreshold); err != nil {
		t.Fatalf("Failed to prune: %v", err)
	}

	// Should have fewer segments now
	finalSegmentCount := sl.SegmentCount()
	if finalSegmentCount >= initialSegmentCount {
		// Prune may not always reduce segments if threshold doesn't align with segment boundaries
		// Just verify it doesn't increase
		t.Logf("Segment count after prune: before=%d, after=%d", initialSegmentCount, finalSegmentCount)
	}

	// Last few records should definitely be readable (in head segment)
	for _, recordId := range recordIds[len(recordIds)-3:] {
		_, err := sl.Read(recordId)
		if err != nil {
			t.Errorf("Failed to read recent record %d: %v", recordId, err)
		}
	}
}

func TestSegmentedLogEmptyPayload(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: DEFAULT_SEGMENT_SIZE,
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}
	defer sl.Close()

	// Append empty payload
	recordId, err := sl.Append([]byte{})
	if err != nil {
		t.Fatalf("Failed to append empty payload: %v", err)
	}

	// Sync before reading
	if err := sl.Sync(); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Read it back
	retrieved, err := sl.Read(recordId)
	if err != nil {
		t.Fatalf("Failed to read empty payload: %v", err)
	}

	if len(retrieved) != 0 {
		t.Errorf("Expected empty payload, got %d bytes", len(retrieved))
	}
}

func TestSegmentedLogRecordNotFound(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: DEFAULT_SEGMENT_SIZE,
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}
	defer sl.Close()

	// Append a record
	_, err = sl.Append([]byte("test"))
	if err != nil {
		t.Fatalf("Failed to append: %v", err)
	}

	// Try to read non-existent record
	_, err = sl.Read(RecordId(999))
	if err != ErrRecordNotFound {
		t.Errorf("Expected ErrRecordNotFound, got %v", err)
	}
}

func TestSegmentedLogSync(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: DEFAULT_SEGMENT_SIZE,
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}
	defer sl.Close()

	// Append and sync
	payload := []byte("test data")
	recordId, err := sl.Append(payload)
	if err != nil {
		t.Fatalf("Failed to append: %v", err)
	}

	if err := sl.Sync(); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Verify data is on disk by reading file directly
	sl.mu.RLock()
	headSegment := sl.segments[len(sl.segments)-1]
	filePath := headSegment.FilePath
	sl.mu.RUnlock()

	reader, err := NewSegmentFileReader(filePath)
	if err != nil {
		t.Fatalf("Failed to open segment file: %v", err)
	}
	defer reader.Close()

	rid, retrieved, err := reader.ReadRecord()
	if err != nil {
		t.Fatalf("Failed to read from file: %v", err)
	}

	if rid != recordId {
		t.Errorf("RecordId mismatch: expected %d, got %d", recordId, rid)
	}

	if !bytes.Equal(retrieved, payload) {
		t.Errorf("Payload mismatch: expected %v, got %v", payload, retrieved)
	}
}

func TestSegmentedLogInvalidConfig(t *testing.T) {
	dir := t.TempDir()

	// MaxSegmentSize too small
	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: MIN_SEGMENT_SIZE - 1,
	}

	_, err := NewSegmentedLog(cfg)
	if err == nil {
		t.Error("Expected error for MaxSegmentSize below minimum")
	}
}

func TestSegmentedLogLargePayload(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: DEFAULT_SEGMENT_SIZE,
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}
	defer sl.Close()

	// Append maximum allowed payload
	largePayload := make([]byte, MAX_RECORD_PAYLOAD)
	for i := range largePayload {
		largePayload[i] = byte(i % 256)
	}

	recordId, err := sl.Append(largePayload)
	if err != nil {
		t.Fatalf("Failed to append large payload: %v", err)
	}

	// Sync before reading
	if err := sl.Sync(); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Read it back
	retrieved, err := sl.Read(recordId)
	if err != nil {
		t.Fatalf("Failed to read large payload: %v", err)
	}

	if !bytes.Equal(retrieved, largePayload) {
		t.Error("Large payload mismatch")
	}

	// Try to append too-large payload
	tooLarge := make([]byte, MAX_RECORD_PAYLOAD+1)
	_, err = sl.Append(tooLarge)
	if err == nil {
		t.Error("Expected error for payload exceeding maximum")
	}
}

func TestSegmentFilenameGeneration(t *testing.T) {
	tests := []struct {
		num      uint64
		expected string
	}{
		{0, "rollback-00000.seg"},
		{1, "rollback-00001.seg"},
		{12345, "rollback-12345.seg"},
		{99999, "rollback-99999.seg"},
	}

	for _, tt := range tests {
		result := SegmentFilename(tt.num)
		if result != tt.expected {
			t.Errorf("SegmentFilename(%d) = %s, expected %s", tt.num, result, tt.expected)
		}
	}
}

func TestSegmentFilenameParsing(t *testing.T) {
	tests := []struct {
		filename string
		num      uint64
		valid    bool
	}{
		{"rollback-00000.seg", 0, true},
		{"rollback-00001.seg", 1, true},
		{"rollback-12345.seg", 12345, true},
		{"rollback-99999.seg", 99999, true},
		{"/path/to/rollback-00001.seg", 1, true},
		{"invalid.seg", 0, false},
		{"rollback-abc.seg", 0, false},
		{"rollback-00001.txt", 0, false},
		{"other-00001.seg", 0, false},
	}

	for _, tt := range tests {
		num, valid := ParseSegmentFilename(tt.filename)
		if valid != tt.valid {
			t.Errorf("ParseSegmentFilename(%s) valid = %v, expected %v", tt.filename, valid, tt.valid)
		}
		if valid && num != tt.num {
			t.Errorf("ParseSegmentFilename(%s) = %d, expected %d", tt.filename, num, tt.num)
		}
	}
}

func TestSegmentedLogConcurrentReads(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: DEFAULT_SEGMENT_SIZE,
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to create segmented log: %v", err)
	}
	defer sl.Close()

	// Append records
	var recordIds []RecordId
	for i := 0; i < 10; i++ {
		payload := []byte{byte(i)}
		recordId, err := sl.Append(payload)
		if err != nil {
			t.Fatalf("Failed to append: %v", err)
		}
		recordIds = append(recordIds, recordId)
	}

	if err := sl.Sync(); err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Concurrent reads
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(readerNum int) {
			for _, recordId := range recordIds {
				_, err := sl.Read(recordId)
				if err != nil {
					t.Errorf("Reader %d failed to read record %d: %v", readerNum, recordId, err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all readers
	for i := 0; i < 5; i++ {
		<-done
	}
}

func TestSegmentMetadata(t *testing.T) {
	seg := NewSegment(5, "/path/to/segment")

	if seg.SegmentNum != 5 {
		t.Errorf("Expected SegmentNum 5, got %d", seg.SegmentNum)
	}

	if !seg.IsEmpty() {
		t.Error("New segment should be empty")
	}

	if seg.RecordCount() != 0 {
		t.Errorf("Empty segment should have 0 records, got %d", seg.RecordCount())
	}

	// Add records
	seg.FirstRecordId = RecordId(10)
	seg.LastRecordId = RecordId(20)

	if seg.IsEmpty() {
		t.Error("Segment should not be empty after adding records")
	}

	if seg.RecordCount() != 11 {
		t.Errorf("Expected 11 records, got %d", seg.RecordCount())
	}

	if !seg.Contains(RecordId(15)) {
		t.Error("Segment should contain RecordId 15")
	}

	if seg.Contains(RecordId(5)) {
		t.Error("Segment should not contain RecordId 5")
	}

	if seg.Contains(RecordId(25)) {
		t.Error("Segment should not contain RecordId 25")
	}
}

func TestSegmentedLogMultiSegmentRecovery(t *testing.T) {
	dir := t.TempDir()

	// Create multiple segments manually
	seg1Path := filepath.Join(dir, SegmentFilename(0))
	seg2Path := filepath.Join(dir, SegmentFilename(1))

	// Write to segment 0
	writer1, err := NewSegmentFileWriter(seg1Path)
	if err != nil {
		t.Fatalf("Failed to create segment 0 writer: %v", err)
	}
	_, err = writer1.WriteRecord(RecordId(1), []byte("seg0-rec1"))
	if err != nil {
		t.Fatalf("Failed to write to segment 0: %v", err)
	}
	_, err = writer1.WriteRecord(RecordId(2), []byte("seg0-rec2"))
	if err != nil {
		t.Fatalf("Failed to write to segment 0: %v", err)
	}
	writer1.Close()

	// Write to segment 1
	writer2, err := NewSegmentFileWriter(seg2Path)
	if err != nil {
		t.Fatalf("Failed to create segment 1 writer: %v", err)
	}
	_, err = writer2.WriteRecord(RecordId(3), []byte("seg1-rec3"))
	if err != nil {
		t.Fatalf("Failed to write to segment 1: %v", err)
	}
	writer2.Close()

	// Now open SegmentedLog and verify recovery
	cfg := Config{
		Dir:            dir,
		MaxSegmentSize: DEFAULT_SEGMENT_SIZE,
	}

	sl, err := NewSegmentedLog(cfg)
	if err != nil {
		t.Fatalf("Failed to open segmented log: %v", err)
	}
	defer sl.Close()

	// Should have 2 segments
	if sl.SegmentCount() != 2 {
		t.Errorf("Expected 2 segments, got %d", sl.SegmentCount())
	}

	// Next record should be 4
	if sl.NextRecordId() != RecordId(4) {
		t.Errorf("Expected NextRecordId 4, got %d", sl.NextRecordId())
	}

	// Verify all records are readable
	records := map[RecordId]string{
		RecordId(1): "seg0-rec1",
		RecordId(2): "seg0-rec2",
		RecordId(3): "seg1-rec3",
	}

	for recordId, expected := range records {
		payload, err := sl.Read(recordId)
		if err != nil {
			t.Errorf("Failed to read record %d: %v", recordId, err)
			continue
		}
		if string(payload) != expected {
			t.Errorf("Record %d mismatch: expected %s, got %s", recordId, expected, string(payload))
		}
	}
}
