package wal

import (
	"encoding/binary"
	"fmt"
)

// WalBlobReader reads entries from a WAL blob.
type WalBlobReader struct {
	buf          []byte
	pos          int
	seenStart    bool
	needsValidation bool
}

// NewWalBlobReader creates a new WAL blob reader.
func NewWalBlobReader(buf []byte) *WalBlobReader {
	return &WalBlobReader{
		buf:             buf,
		pos:             0,
		seenStart:       false,
		needsValidation: len(buf) > 0, // Only validate non-empty blobs
	}
}

// Next reads the next entry from the WAL.
// Returns nil when no more entries.
func (r *WalBlobReader) Next() (*WalEntry, error) {
	// Skip start markers and process entries sequentially
	for r.pos < len(r.buf) {
		if r.pos >= len(r.buf) {
			return nil, nil // EOF
		}

		tag := r.buf[r.pos]
		r.pos++

		switch tag {
		case WalEntryTagStart:
			// Start marker found
			r.seenStart = true
			continue

		case WalEntryTagEnd:
			// End marker, no more entries
			return nil, nil

		case WalEntryTagClear:
			// Validate that we saw a start marker first
			if r.needsValidation && !r.seenStart {
				return nil, fmt.Errorf("WAL blob missing start marker")
			}
			if r.pos+8 > len(r.buf) {
				return nil, fmt.Errorf("truncated clear entry")
			}
			bucketIndex := binary.LittleEndian.Uint64(r.buf[r.pos : r.pos+8])
			r.pos += 8
			return &WalEntry{
				Type:        WalEntryClear,
				BucketIndex: bucketIndex,
			}, nil

		case WalEntryTagUpdate:
			// Validate that we saw a start marker first
			if r.needsValidation && !r.seenStart {
				return nil, fmt.Errorf("WAL blob missing start marker")
			}
			// Format: bucket_index (8) + pageId (32) + pageData (16384)
			if r.pos+8+32+16384 > len(r.buf) {
				return nil, fmt.Errorf("truncated update entry")
			}

			bucketIndex := binary.LittleEndian.Uint64(r.buf[r.pos : r.pos+8])
			r.pos += 8

			pageId := make([]byte, 32)
			copy(pageId, r.buf[r.pos:r.pos+32])
			r.pos += 32

			pageData := make([]byte, 16384)
			copy(pageData, r.buf[r.pos:r.pos+16384])
			r.pos += 16384

			return &WalEntry{
				Type:        WalEntryUpdate,
				BucketIndex: bucketIndex,
				PageId:      pageId,
				PageData:    pageData,
			}, nil

		default:
			return nil, fmt.Errorf("unknown WAL entry tag: %d", tag)
		}
	}

	return nil, nil // EOF
}
