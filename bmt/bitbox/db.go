package bitbox

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/jam-duna/jamduna/bmt/bitbox/wal"
	"github.com/jam-duna/jamduna/bmt/core"
	"github.com/jam-duna/jamduna/bmt/io"
)

// DB is a hash table database for storing pages by PageId.
type DB struct {
	mu          sync.RWMutex
	htFile      *os.File
	walFile     *os.File
	offsets     *HTOffsets
	metaMap     *MetaMap
	pagePool    *io.PagePool
	seed        uint64
	numBuckets  uint32
	walBuilder  *wal.WalBlobBuilder
	dirtyPages  map[BucketIndex][]byte      // In-memory dirty pages (page data only)
	bucketToId  map[BucketIndex]core.PageId // Bucket to PageId mapping
}

// Open opens or creates a bitbox database at the given path.
func Open(path string, numBuckets uint32) (*DB, error) {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	htPath := filepath.Join(path, "ht")
	walPath := filepath.Join(path, "wal")

	// Create page pool
	pagePool := io.NewPagePool()

	// Check if HT file exists
	var htFile *os.File
	var offsets *HTOffsets
	var metaMap *MetaMap
	var err error

	if _, err := os.Stat(htPath); os.IsNotExist(err) {
		// Create new HT file
		if err := CreateHTFile(htPath, numBuckets); err != nil {
			return nil, err
		}
	}

	// Open HT file
	htFile, offsets, metaMap, err = OpenHTFile(htPath, numBuckets, pagePool)
	if err != nil {
		return nil, err
	}

	// Open or create WAL file
	walFile, err := os.OpenFile(walPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		htFile.Close()
		return nil, fmt.Errorf("failed to open WAL file: %w", err)
	}

	db := &DB{
		htFile:      htFile,
		walFile:     walFile,
		offsets:     offsets,
		metaMap:     metaMap,
		pagePool:    pagePool,
		seed:        0x123456789ABCDEF0, // Default seed
		numBuckets:  numBuckets,
		walBuilder:  wal.NewWalBlobBuilder(),
		dirtyPages:  make(map[BucketIndex][]byte),
		bucketToId:  make(map[BucketIndex]core.PageId),
	}

	// Perform WAL recovery if needed
	if err := db.recover(); err != nil {
		db.Close()
		return nil, fmt.Errorf("WAL recovery failed: %w", err)
	}

	// Start a new WAL transaction
	db.walBuilder.WriteStart()

	return db, nil
}

// InsertPage inserts a page into the hash table.
func (db *DB) InsertPage(pageId core.PageId, pageData []byte) error {
	if len(pageData) != PageSize {
		return fmt.Errorf("page data must be %d bytes, got %d", PageSize, len(pageData))
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	// Allocate a bucket
	bucket, err := AllocateBucket(pageId, db.seed, db.metaMap)
	if err != nil {
		return err
	}

	// Store in dirty pages (in-memory)
	pageCopy := make([]byte, PageSize)
	copy(pageCopy, pageData)
	db.dirtyPages[bucket] = pageCopy
	db.bucketToId[bucket] = pageId

	// Write to WAL (with bucket index for proper recovery)
	encodedPageId := pageId.Encode()
	db.walBuilder.WriteUpdate(uint64(bucket), encodedPageId[:], pageData)

	return nil
}

// RetrievePage retrieves a page by PageId.
func (db *DB) RetrievePage(pageId core.PageId) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Probe for the page (probe.Next() loops internally)
	hash := HashPageId(pageId, db.seed)
	probe := NewProbeSequence(hash, db.metaMap.Len())
	result := probe.Next(db.metaMap)

	switch result.Type {
	case ProbeResultEmpty, ProbeResultExhausted:
		// Page not found
		return nil, fmt.Errorf("page not found")

	case ProbeResultPossibleHit:
		bucket := BucketIndex(result.BucketIndex)

		// Check dirty pages first
		if pageData, ok := db.dirtyPages[bucket]; ok {
			// Verify PageId matches (we always have bucketToId for dirty pages)
			if storedId, exists := db.bucketToId[bucket]; exists {
				if storedId.Encode() != pageId.Encode() {
					// Hash collision - wrong page
					return nil, fmt.Errorf("page not found (hash collision)")
				}
			}
			// Return a copy
			resultCopy := make([]byte, PageSize)
			copy(resultCopy, pageData)
			return resultCopy, nil
		}

		// Read from disk with PageId verification
		pageData, err := db.readBucketWithPageId(bucket, pageId)
		if err != nil {
			// Could be hash collision or read error
			return nil, err
		}

		return pageData, nil

	default:
		return nil, fmt.Errorf("unexpected probe result: %v", result.Type)
	}
}

// Sync flushes all dirty pages to the hash table file and truncates the WAL.
func (db *DB) Sync() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Write end marker to WAL
	db.walBuilder.WriteEnd()

	// Write WAL to disk
	walData := db.walBuilder.Bytes()
	if err := db.walFile.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate WAL: %w", err)
	}
	if _, err := db.walFile.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek WAL: %w", err)
	}
	if _, err := db.walFile.Write(walData); err != nil {
		return fmt.Errorf("failed to write WAL: %w", err)
	}
	if err := db.walFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync WAL: %w", err)
	}

	// Write dirty pages to HT file with PageId prefix
	for bucket, pageData := range db.dirtyPages {
		pageId, ok := db.bucketToId[bucket]
		if !ok {
			return fmt.Errorf("missing PageId for bucket %d", bucket)
		}

		if err := db.writeBucketWithPageId(bucket, pageId, pageData); err != nil {
			return fmt.Errorf("failed to write bucket %d: %w", bucket, err)
		}
	}

	// Write updated MetaMap to HT file
	numMetaPages := numMetaBytePagesForBuckets(db.numBuckets)
	metaBytes := db.metaMap.Bytes()
	for i := uint32(0); i < numMetaPages; i++ {
		start := int(i) * PageSize
		end := start + PageSize
		if end > len(metaBytes) {
			end = len(metaBytes)
		}
		pageData := metaBytes[start:end]

		// Pad if needed
		if len(pageData) < PageSize {
			paddedPage := make([]byte, PageSize)
			copy(paddedPage, pageData)
			pageData = paddedPage
		}

		if err := io.WritePage(db.htFile, pageData, uint64(i)); err != nil {
			return fmt.Errorf("failed to write meta page: %w", err)
		}
	}

	// Sync HT file
	if err := db.htFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync HT file: %w", err)
	}

	// Truncate WAL (transaction complete)
	if err := db.walFile.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate WAL after sync: %w", err)
	}
	if err := db.walFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync WAL truncate: %w", err)
	}

	// Clear dirty pages and bucket mapping
	db.dirtyPages = make(map[BucketIndex][]byte)
	db.bucketToId = make(map[BucketIndex]core.PageId)

	// Start new transaction
	db.walBuilder = wal.NewWalBlobBuilder()
	db.walBuilder.WriteStart()

	return nil
}

// Close closes the database.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var errs []error

	// If there are dirty pages, write WAL to disk before closing
	if len(db.dirtyPages) > 0 {
		// Write end marker to WAL
		db.walBuilder.WriteEnd()

		// Write WAL to disk
		walData := db.walBuilder.Bytes()
		if err := db.walFile.Truncate(0); err != nil {
			errs = append(errs, fmt.Errorf("failed to truncate WAL on close: %w", err))
		} else if _, err := db.walFile.Seek(0, 0); err != nil {
			errs = append(errs, fmt.Errorf("failed to seek WAL on close: %w", err))
		} else if _, err := db.walFile.Write(walData); err != nil {
			errs = append(errs, fmt.Errorf("failed to write WAL on close: %w", err))
		} else if err := db.walFile.Sync(); err != nil {
			errs = append(errs, fmt.Errorf("failed to sync WAL on close: %w", err))
		}
	}

	if db.htFile != nil {
		if err := db.htFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if db.walFile != nil {
		if err := db.walFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing DB: %v", errs)
	}

	return nil
}

// recover performs WAL recovery on startup.
func (db *DB) recover() error {
	// Read WAL file
	stat, err := db.walFile.Stat()
	if err != nil {
		return err
	}

	if stat.Size() == 0 {
		// Empty WAL, nothing to recover
		return nil
	}

	walData := make([]byte, stat.Size())
	if _, err := db.walFile.ReadAt(walData, 0); err != nil {
		return fmt.Errorf("failed to read WAL: %w", err)
	}

	// Parse WAL entries
	reader := wal.NewWalBlobReader(walData)

	for {
		entry, err := reader.Next()
		if err != nil {
			return fmt.Errorf("failed to parse WAL entry: %w", err)
		}
		if entry == nil {
			break // End of WAL
		}

		switch entry.Type {
		case wal.WalEntryClear:
			// Clear a bucket
			bucket := BucketIndex(entry.BucketIndex)
			FreeBucket(bucket, db.metaMap)
			// Remove from PageId mapping
			delete(db.bucketToId, bucket)

		case wal.WalEntryUpdate:
			// Recover an update - use bucket index from WAL
			bucket := BucketIndex(entry.BucketIndex)

			// Decode PageId
			var pageIdBytes [32]byte
			copy(pageIdBytes[:], entry.PageId)
			pageId, err := core.DecodePageId(pageIdBytes)
			if err != nil {
				return fmt.Errorf("failed to decode PageId from WAL: %w", err)
			}

			// Compute hash and mark bucket as full in metadata
			hash := HashPageId(*pageId, db.seed)
			db.metaMap.SetFull(int(bucket), hash)

			// Track PageId mapping (for verification during this session)
			db.bucketToId[bucket] = *pageId

			// Write page data with PageId to HT file
			if err := db.writeBucketWithPageId(bucket, *pageId, entry.PageData); err != nil {
				return fmt.Errorf("failed to write page during recovery: %w", err)
			}
		}
	}

	// Write updated MetaMap to HT file after recovery
	numMetaPages := numMetaBytePagesForBuckets(db.numBuckets)
	metaBytes := db.metaMap.Bytes()
	for i := uint32(0); i < numMetaPages; i++ {
		start := int(i) * PageSize
		end := start + PageSize
		if end > len(metaBytes) {
			end = len(metaBytes)
		}
		pageData := metaBytes[start:end]

		// Pad if needed
		if len(pageData) < PageSize {
			paddedPage := make([]byte, PageSize)
			copy(paddedPage, pageData)
			pageData = paddedPage
		}

		if err := io.WritePage(db.htFile, pageData, uint64(i)); err != nil {
			return fmt.Errorf("failed to write meta page during recovery: %w", err)
		}
	}

	// Sync HT file after recovery
	if err := db.htFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync HT after recovery: %w", err)
	}

	// Truncate WAL (recovery complete)
	if err := db.walFile.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate WAL after recovery: %w", err)
	}

	// Clear dirty pages and bucket mapping (all recovered pages are now on disk)
	db.dirtyPages = make(map[BucketIndex][]byte)
	db.bucketToId = make(map[BucketIndex]core.PageId)

	// Reinitialize WAL builder with fresh start marker
	db.walBuilder = wal.NewWalBlobBuilder()
	db.walBuilder.WriteStart()

	return nil
}

// writeBucketWithPageId writes a bucket's data with PageId prefix
// Format: [32-byte PageId][16KB page data] = 16416 bytes total
// Uses two separate Pwrite calls to avoid heap allocation
func (db *DB) writeBucketWithPageId(bucket BucketIndex, pageId core.PageId, pageData []byte) error {
	if len(pageData) != PageSize {
		return fmt.Errorf("invalid page data size: expected %d, got %d", PageSize, len(pageData))
	}

	// Calculate file offset (each bucket is 16416 bytes, after meta pages)
	dataStartOffset := int64(db.offsets.dataPageOffset) * PageSize
	bucketOffset := dataStartOffset + (int64(bucket) * (32 + PageSize))

	// Write PageId first (32 bytes)
	encodedId := pageId.Encode()
	if _, err := io.Pwrite(db.htFile, encodedId[:], bucketOffset); err != nil {
		return fmt.Errorf("failed to write PageId: %w", err)
	}

	// Write page data second (16KB)
	if _, err := io.Pwrite(db.htFile, pageData, bucketOffset+32); err != nil {
		return fmt.Errorf("failed to write page data: %w", err)
	}

	return nil
}

// readBucketWithPageId reads a bucket's data and verifies the PageId
// Returns the page data (16KB) if PageId matches, error otherwise
func (db *DB) readBucketWithPageId(bucket BucketIndex, expectedPageId core.PageId) ([]byte, error) {
	// Check metadata first to avoid reading uninitialized/freed buckets
	if db.metaMap.HintEmpty(int(bucket)) {
		return nil, fmt.Errorf("bucket is empty (not allocated)")
	}
	if db.metaMap.HintTombstone(int(bucket)) {
		return nil, fmt.Errorf("bucket is tombstone (deleted)")
	}

	// Calculate file offset
	dataStartOffset := int64(db.offsets.dataPageOffset) * PageSize
	bucketOffset := dataStartOffset + (int64(bucket) * (32 + PageSize))

	// Read bucket data (PageId + page data)
	buf := make([]byte, 32+PageSize)
	_, err := io.Pread(db.htFile, buf, bucketOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to read bucket: %w", err)
	}

	// Decode and verify PageId
	var storedIdBytes [32]byte
	copy(storedIdBytes[:], buf[:32])
	storedId, err := core.DecodePageId(storedIdBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode stored PageId: %w", err)
	}

	if storedId.Encode() != expectedPageId.Encode() {
		return nil, fmt.Errorf("PageId mismatch: hash collision")
	}

	// Return page data only
	pageData := make([]byte, PageSize)
	copy(pageData, buf[32:])
	return pageData, nil
}
