package io

import (
	"fmt"
	"os"
	"sync"
)

// Fsyncer coordinates asynchronous fsync operations.
// It allows multiple goroutines to request fsync and wait for completion.
type Fsyncer struct {
	name string
	file *os.File
	mu   sync.Mutex
	cond *sync.Cond

	// Fsync state
	pending  bool // True if fsync has been requested but not started
	inflight bool // True if fsync is currently running
	err      error // Last fsync error
}

// NewFsyncer creates a new Fsyncer for the given file.
func NewFsyncer(name string, file *os.File) *Fsyncer {
	f := &Fsyncer{
		name: name,
		file: file,
	}
	f.cond = sync.NewCond(&f.mu)
	return f
}

// Fsync requests an asynchronous fsync operation.
// This is non-blocking and returns immediately.
func (f *Fsyncer) Fsync() {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.inflight {
		// Fsync already in progress, just mark as pending
		f.pending = true
		return
	}

	// Start fsync in background
	f.inflight = true
	go f.doFsync()
}

// doFsync performs the actual fsync in a goroutine.
func (f *Fsyncer) doFsync() {
	for {
		// Perform the fsync
		err := f.file.Sync()

		f.mu.Lock()

		if err != nil {
			f.err = fmt.Errorf("fsync failed for %s: %w", f.name, err)
		} else {
			f.err = nil
		}

		// Check if another fsync was requested while we were syncing
		if !f.pending {
			// No more pending fsyncs, we're done
			f.inflight = false
			f.cond.Broadcast() // Wake up all waiters
			f.mu.Unlock()
			return
		}

		// Clear pending flag and do another fsync
		f.pending = false
		f.mu.Unlock()
	}
}

// Wait blocks until all pending fsync operations complete.
// Returns an error if any fsync failed.
func (f *Fsyncer) Wait() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Wait until no fsync is in flight
	for f.inflight {
		f.cond.Wait()
	}

	return f.err
}

// FsyncAndWait performs a synchronous fsync.
// This is equivalent to calling Fsync() followed by Wait().
func (f *Fsyncer) FsyncAndWait() error {
	f.Fsync()
	return f.Wait()
}
