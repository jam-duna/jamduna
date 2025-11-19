package store

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// FileLock represents an exclusive file lock on a store directory.
// Prevents multiple processes from opening the same store simultaneously.
type FileLock struct {
	file *os.File
	path string
}

// AcquireLock creates and locks a lockfile in the given directory.
// Returns error if the lock is already held by another process.
func AcquireLock(dirPath string) (*FileLock, error) {
	lockPath := filepath.Join(dirPath, "LOCK")

	// Open or create the lock file
	file, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try to acquire exclusive lock (non-blocking)
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		file.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, fmt.Errorf("store is locked by another process")
		}
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	return &FileLock{
		file: file,
		path: lockPath,
	}, nil
}

// Release releases the file lock and closes the file.
func (fl *FileLock) Release() error {
	if fl.file == nil {
		return nil
	}

	// Unlock
	if err := syscall.Flock(int(fl.file.Fd()), syscall.LOCK_UN); err != nil {
		fl.file.Close()
		return fmt.Errorf("failed to unlock: %w", err)
	}

	// Close file
	if err := fl.file.Close(); err != nil {
		return fmt.Errorf("failed to close lock file: %w", err)
	}

	fl.file = nil
	return nil
}
