package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/colorfulnotion/jam/bmt/bitbox"
	"github.com/colorfulnotion/jam/bmt/core"
)

// Store coordinates BitBox (hash table page storage) with atomic sync.
// It provides atomic sync operations and crash recovery.
// NOTE: Beatree integration pending - currently only manages BitBox
type Store struct {
	mu sync.RWMutex

	dirPath  string
	lock     *FileLock
	manifest *Manifest

	// Storage layers
	bitbox *bitbox.DB // Hash table page storage

	// Sync state
	syncVersion uint64 // Current sync version (matches manifest after sync)

	// PageId allocation state (simple counter for now)
	nextPageId uint64
}

// Open opens or creates a store at the given directory path.
// Acquires exclusive file lock to prevent concurrent access.
func Open(dirPath string) (*Store, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	// Acquire exclusive lock
	lock, err := AcquireLock(dirPath)
	if err != nil {
		return nil, err
	}

	// Try to read existing manifest
	manifest, err := ReadManifest(dirPath)
	if err != nil {
		// No manifest - this is a new store
		manifest = &Manifest{
			RootHash:    [32]byte{}, // Zero hash for empty tree
			SyncVersion: 0,
		}
	}

	// Open BitBox (hash table page storage)
	bitboxPath := filepath.Join(dirPath, "bitbox")
	bb, err := bitbox.Open(bitboxPath, 1000000) // 1M buckets
	if err != nil {
		lock.Release()
		return nil, fmt.Errorf("failed to open bitbox: %w", err)
	}

	store := &Store{
		dirPath:     dirPath,
		lock:        lock,
		manifest:    manifest,
		bitbox:      bb,
		syncVersion: manifest.SyncVersion,
		nextPageId:  1, // Start PageId allocation at 1
	}

	return store, nil
}

// StorePage stores a page in BitBox.
// Page must be exactly 16KB.
func (s *Store) StorePage(pageId core.PageId, pageData []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.bitbox.InsertPage(pageId, pageData)
}

// RetrievePage retrieves a page from BitBox.
func (s *Store) RetrievePage(pageId core.PageId) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.bitbox.RetrievePage(pageId)
}

// AllocatePageId allocates a new PageId.
// Uses a simple counter encoded as base-64 child indices.
// Each index represents a 6-bit child index (0-63), allowing up to 64^42 unique PageIds.
func (s *Store) AllocatePageId() (*core.PageId, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Encode counter as base-64 digits
	// Calculate minimum path length needed
	counter := s.nextPageId
	if counter == 0 {
		return nil, fmt.Errorf("cannot allocate PageId 0 (reserved for root)")
	}

	// Determine path length (number of base-64 digits needed)
	pathLen := 0
	temp := counter
	for temp > 0 {
		pathLen++
		temp /= 64
	}

	// Build path from counter (right to left, then reverse)
	path := make([]byte, pathLen)
	temp = counter
	for i := pathLen - 1; i >= 0; i-- {
		path[i] = byte(temp % 64)
		temp /= 64
	}

	pageId, err := core.NewPageId(path)
	if err != nil {
		return nil, err
	}

	s.nextPageId++
	return pageId, nil
}

// Sync performs a two-phase sync:
// 1. Sync BitBox (write dirty pages to HT file, truncate WAL)
// 2. Update manifest atomically (write root hash + version)
//
// If crash occurs before manifest write: BitBox is synced, but old manifest still valid
// If crash occurs after manifest write: New manifest is valid, all changes committed
func (s *Store) Sync(rootHash [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Phase 1: Sync BitBox (flush dirty pages, truncate WAL)
	if err := s.bitbox.Sync(); err != nil {
		return fmt.Errorf("bitbox sync failed: %w", err)
	}

	// Phase 2: Update manifest atomically
	newManifest := &Manifest{
		RootHash:    rootHash,
		SyncVersion: s.syncVersion + 1,
	}

	if err := WriteManifest(s.dirPath, newManifest); err != nil {
		// BitBox is synced but manifest not updated - this is safe
		// On recovery, old manifest will be used (consistent state)
		return fmt.Errorf("manifest write failed: %w", err)
	}

	// Update in-memory state
	s.manifest = newManifest
	s.syncVersion = newManifest.SyncVersion

	return nil
}

// RootHash returns the current root hash from the manifest.
func (s *Store) RootHash() [32]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.manifest.RootHash
}

// SyncVersion returns the current sync version.
func (s *Store) SyncVersion() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.syncVersion
}

// Close closes the store and releases the file lock.
// Does NOT automatically sync - call Sync() first if needed.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errs []error

	// Close BitBox
	if s.bitbox != nil {
		if err := s.bitbox.Close(); err != nil {
			errs = append(errs, fmt.Errorf("bitbox close failed: %w", err))
		}
	}

	// Release lock
	if s.lock != nil {
		if err := s.lock.Release(); err != nil {
			errs = append(errs, fmt.Errorf("lock release failed: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %v", errs)
	}

	return nil
}
