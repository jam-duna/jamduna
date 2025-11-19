package io

import (
	"fmt"
	"os"
	"syscall"
)

// Pread reads len(buf) bytes from file at offset.
// It's a wrapper around syscall.Pread that handles EINTR and short reads.
// Loops until the full buffer is read or EOF/error occurs.
func Pread(file *os.File, buf []byte, offset int64) (int, error) {
	fd := int(file.Fd())
	totalRead := 0

	for totalRead < len(buf) {
		n, err := syscall.Pread(fd, buf[totalRead:], offset+int64(totalRead))
		if err == syscall.EINTR {
			// Interrupted system call, retry
			continue
		}
		if err != nil {
			return totalRead, fmt.Errorf("pread failed: %w", err)
		}
		if n == 0 {
			// EOF reached
			return totalRead, nil
		}
		totalRead += n
	}

	return totalRead, nil
}

// Pwrite writes len(buf) bytes to file at offset.
// It's a wrapper around syscall.Pwrite that handles EINTR and short writes.
// Loops until the full buffer is written or error occurs.
func Pwrite(file *os.File, buf []byte, offset int64) (int, error) {
	fd := int(file.Fd())
	totalWritten := 0

	for totalWritten < len(buf) {
		n, err := syscall.Pwrite(fd, buf[totalWritten:], offset+int64(totalWritten))
		if err == syscall.EINTR {
			// Interrupted system call, retry
			continue
		}
		if err != nil {
			return totalWritten, fmt.Errorf("pwrite failed: %w", err)
		}
		if n == 0 {
			// This shouldn't happen for writes, but handle it
			return totalWritten, fmt.Errorf("pwrite returned 0 bytes written")
		}
		totalWritten += n
	}

	return totalWritten, nil
}

// ReadPage reads a full page from file at the given page number.
func ReadPage(file *os.File, pagePool *PagePool, pageNumber uint64) (Page, error) {
	offset := int64(pageNumber * PageSize)
	page := pagePool.Alloc()

	n, err := Pread(file, page, offset)
	if err != nil {
		pagePool.Dealloc(page)
		return nil, err
	}

	if n != PageSize {
		pagePool.Dealloc(page)
		return nil, fmt.Errorf("short read: expected %d bytes, got %d", PageSize, n)
	}

	return page, nil
}

// WritePage writes a full page to file at the given page number.
func WritePage(file *os.File, page Page, pageNumber uint64) error {
	if len(page) != PageSize {
		return fmt.Errorf("invalid page size: expected %d, got %d", PageSize, len(page))
	}

	offset := int64(pageNumber * PageSize)
	n, err := Pwrite(file, page, offset)
	if err != nil {
		return err
	}

	if n != PageSize {
		return fmt.Errorf("short write: expected %d bytes, wrote %d", PageSize, n)
	}

	return nil
}
