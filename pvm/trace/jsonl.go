package trace

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
)

// JSONLTraceWriter writes TraceStep records as JSON Lines (one JSON object per line).
// It is safe for concurrent use by multiple goroutines.
type JSONLTraceWriter struct {
	mu     sync.Mutex
	enc    *json.Encoder
	buf    *bufio.Writer
	closer io.Closer // optional, only set when we own the underlying writer
	closed bool
}

// ErrTraceWriterClosed is returned when WriteStep is called after Close.
var ErrTraceWriterClosed = errors.New("jsonl trace writer is closed")

// NewJSONLTraceWriter creates a JSONLTraceWriter using the provided io.Writer.
// The writer passed in is NOT closed by JSONLTraceWriter. Close() will only
// flush the internal buffer.
func NewJSONLTraceWriter(w io.Writer) *JSONLTraceWriter {
	buf := bufio.NewWriterSize(w, 64*1024) // 64KB buffer for better throughput
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)

	return &JSONLTraceWriter{
		enc: enc,
		buf: buf,
		// closer is nil here, so Close() won't close the underlying writer.
	}
}

// NewJSONLTraceWriterFile opens the given file path for writing (truncate or create)
// and returns a JSONLTraceWriter that owns the file.
// Close() will flush and close the underlying file.
func NewJSONLTraceWriterFile(path string) (*JSONLTraceWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	buf := bufio.NewWriterSize(f, 64*1024)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)

	return &JSONLTraceWriter{
		enc:    enc,
		buf:    buf,
		closer: f,
	}, nil
}

// NewJSONLTraceWriterStdout creates a JSONLTraceWriter that writes to stdout.
// The writer is NOT buffered heavily to ensure immediate output visibility.
// Close() will only flush the buffer, not close stdout.
func NewJSONLTraceWriterStdout() *JSONLTraceWriter {
	buf := bufio.NewWriterSize(os.Stdout, 4*1024) // Smaller buffer for more immediate output
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)

	return &JSONLTraceWriter{
		enc: enc,
		buf: buf,
		// closer is nil - we don't close stdout
	}
}

// WriteStep encodes a single TraceStep as a JSON object followed by a newline.
// This method is safe to call from multiple goroutines concurrently.
func (w *JSONLTraceWriter) WriteStep(step *TraceStep) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrTraceWriterClosed
	}

	// json.Encoder.Encode() writes a single JSON value followed by '\n'.
	if err := w.enc.Encode(step); err != nil {
		return err
	}

	return nil
}

// Flush forces buffered data to be written to the underlying writer.
// This does NOT close the writer.
func (w *JSONLTraceWriter) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrTraceWriterClosed
	}

	return w.buf.Flush()
}

// Close flushes any buffered data. If the JSONLTraceWriter owns the underlying
// writer (created via NewJSONLTraceWriterFile), it will also close it.
func (w *JSONLTraceWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	// First flush any remaining data.
	if err := w.buf.Flush(); err != nil {
		// Even if flush fails, try to close the closer if we own it.
		if w.closer != nil {
			_ = w.closer.Close()
		}
		return err
	}

	// If we own the underlying writer, close it.
	if w.closer != nil {
		return w.closer.Close()
	}

	return nil
}
