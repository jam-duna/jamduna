package wal

import (
	"encoding/binary"
)

// WalBlobBuilder builds a WAL blob in memory.
type WalBlobBuilder struct {
	buf []byte
}

// NewWalBlobBuilder creates a new WAL blob builder.
func NewWalBlobBuilder() *WalBlobBuilder {
	return &WalBlobBuilder{
		buf: make([]byte, 0, 1024*1024), // 1MB initial capacity
	}
}

// WriteClear writes a clear entry (delete bucket).
func (w *WalBlobBuilder) WriteClear(bucketIndex uint64) {
	w.buf = append(w.buf, WalEntryTagClear)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], bucketIndex)
	w.buf = append(w.buf, b[:]...)
}

// WriteUpdate writes an update entry (insert/update page).
// Format: TAG (1) + bucket_index (8) + pageId (32) + pageData (16384)
func (w *WalBlobBuilder) WriteUpdate(bucketIndex uint64, pageId []byte, pageData []byte) {
	if len(pageId) != 32 {
		panic("pageId must be 32 bytes")
	}
	if len(pageData) != 16384 {
		panic("pageData must be 16KB")
	}

	w.buf = append(w.buf, WalEntryTagUpdate)

	// Write bucket index
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], bucketIndex)
	w.buf = append(w.buf, b[:]...)

	w.buf = append(w.buf, pageId...)
	w.buf = append(w.buf, pageData...)
}

// WriteStart writes a start marker.
func (w *WalBlobBuilder) WriteStart() {
	w.buf = append(w.buf, WalEntryTagStart)
}

// WriteEnd writes an end marker.
func (w *WalBlobBuilder) WriteEnd() {
	w.buf = append(w.buf, WalEntryTagEnd)
}

// Bytes returns the complete WAL blob.
func (w *WalBlobBuilder) Bytes() []byte {
	return w.buf
}

// Len returns the current size of the WAL blob.
func (w *WalBlobBuilder) Len() int {
	return len(w.buf)
}
