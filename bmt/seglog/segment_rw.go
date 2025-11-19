package seglog

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// SegmentFileWriter provides buffered writing to a segment file.
// Records are written with 4K alignment for O_DIRECT compatibility.
type SegmentFileWriter struct {
	file   *os.File
	writer *bufio.Writer
	offset uint64
}

// NewSegmentFileWriter creates a new segment file writer.
// The file is created with 0644 permissions and truncated.
func NewSegmentFileWriter(filePath string) (*SegmentFileWriter, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create segment file: %w", err)
	}

	return &SegmentFileWriter{
		file:   file,
		writer: bufio.NewWriterSize(file, 64*1024), // 64KB buffer
		offset: 0,
	}, nil
}

// OpenSegmentFileWriter opens an existing segment file for appending.
func OpenSegmentFileWriter(filePath string) (*SegmentFileWriter, error) {
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open segment file: %w", err)
	}

	// Get current size
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return &SegmentFileWriter{
		file:   file,
		writer: bufio.NewWriterSize(file, 64*1024), // 64KB buffer
		offset: uint64(info.Size()),
	}, nil
}

// WriteRecord writes a record to the segment file.
// Format: [4B payload_length][8B record_id][payload][padding to 4K alignment]
// Returns the file offset where the record was written.
func (w *SegmentFileWriter) WriteRecord(recordId RecordId, payload []byte) (uint64, error) {
	payloadLen := uint32(len(payload))

	// Validate payload size
	if payloadLen > MAX_RECORD_PAYLOAD {
		return 0, fmt.Errorf("payload size %d exceeds maximum %d", payloadLen, MAX_RECORD_PAYLOAD)
	}

	// Calculate total record size with alignment
	recordSize := HEADER_SIZE + len(payload)
	alignedSize := ((recordSize + RECORD_ALIGNMENT - 1) / RECORD_ALIGNMENT) * RECORD_ALIGNMENT

	// Write header: payload_length (4 bytes)
	var headerBuf [HEADER_SIZE]byte
	binary.LittleEndian.PutUint32(headerBuf[0:4], payloadLen)
	binary.LittleEndian.PutUint64(headerBuf[4:12], uint64(recordId))

	recordOffset := w.offset

	// Write header
	if _, err := w.writer.Write(headerBuf[:]); err != nil {
		return 0, fmt.Errorf("failed to write record header: %w", err)
	}

	// Write payload
	if _, err := w.writer.Write(payload); err != nil {
		return 0, fmt.Errorf("failed to write record payload: %w", err)
	}

	// Write padding to alignment boundary
	paddingSize := alignedSize - recordSize
	if paddingSize > 0 {
		padding := make([]byte, paddingSize)
		if _, err := w.writer.Write(padding); err != nil {
			return 0, fmt.Errorf("failed to write padding: %w", err)
		}
	}

	w.offset += uint64(alignedSize)
	return recordOffset, nil
}

// Sync flushes buffered data to disk.
func (w *SegmentFileWriter) Sync() error {
	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush buffer: %w", err)
	}
	if err := w.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}
	return nil
}

// Close flushes and closes the segment file.
func (w *SegmentFileWriter) Close() error {
	if err := w.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush on close: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}
	return nil
}

// Offset returns the current write offset in the file.
func (w *SegmentFileWriter) Offset() uint64 {
	return w.offset
}

// SegmentFileReader provides buffered reading from a segment file.
type SegmentFileReader struct {
	file   *os.File
	reader *bufio.Reader
	offset uint64
}

// NewSegmentFileReader creates a new segment file reader.
func NewSegmentFileReader(filePath string) (*SegmentFileReader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open segment file: %w", err)
	}

	return &SegmentFileReader{
		file:   file,
		reader: bufio.NewReaderSize(file, 64*1024), // 64KB buffer
		offset: 0,
	}, nil
}

// ReadRecord reads the next record from the segment file.
// Returns (recordId, payload, nil) on success.
// Returns (InvalidRecordId, nil, io.EOF) at end of file.
func (r *SegmentFileReader) ReadRecord() (RecordId, []byte, error) {
	// Read header
	var headerBuf [HEADER_SIZE]byte
	n, err := io.ReadFull(r.reader, headerBuf[:])
	if err == io.EOF {
		return InvalidRecordId, nil, io.EOF
	}
	if err == io.ErrUnexpectedEOF {
		return InvalidRecordId, nil, io.EOF
	}
	if err != nil {
		return InvalidRecordId, nil, fmt.Errorf("failed to read record header: %w", err)
	}
	if n != HEADER_SIZE {
		return InvalidRecordId, nil, fmt.Errorf("incomplete header read: got %d bytes, expected %d", n, HEADER_SIZE)
	}

	// Parse header
	payloadLen := binary.LittleEndian.Uint32(headerBuf[0:4])
	recordId := RecordId(binary.LittleEndian.Uint64(headerBuf[4:12]))

	// Validate payload length
	if payloadLen > MAX_RECORD_PAYLOAD {
		return InvalidRecordId, nil, fmt.Errorf("invalid payload length %d", payloadLen)
	}

	// Read payload
	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r.reader, payload); err != nil {
			return InvalidRecordId, nil, fmt.Errorf("failed to read payload: %w", err)
		}
	}

	// Calculate and skip padding
	recordSize := HEADER_SIZE + int(payloadLen)
	alignedSize := ((recordSize + RECORD_ALIGNMENT - 1) / RECORD_ALIGNMENT) * RECORD_ALIGNMENT
	paddingSize := alignedSize - recordSize

	if paddingSize > 0 {
		if _, err := r.reader.Discard(paddingSize); err != nil {
			return InvalidRecordId, nil, fmt.Errorf("failed to skip padding: %w", err)
		}
	}

	r.offset += uint64(alignedSize)
	return recordId, payload, nil
}

// Seek moves the read position to the specified offset.
func (r *SegmentFileReader) Seek(offset uint64) error {
	if _, err := r.file.Seek(int64(offset), io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}
	r.reader.Reset(r.file)
	r.offset = offset
	return nil
}

// Close closes the segment file reader.
func (r *SegmentFileReader) Close() error {
	if err := r.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}
	return nil
}

// Offset returns the current read offset in the file.
func (r *SegmentFileReader) Offset() uint64 {
	return r.offset
}
