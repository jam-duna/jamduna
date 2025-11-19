package bmt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/colorfulnotion/jam/bmt/beatree"
	"github.com/colorfulnotion/jam/bmt/beatree/allocator"
	"github.com/colorfulnotion/jam/bmt/beatree/ops"
	"github.com/colorfulnotion/jam/bmt/core"
	"github.com/colorfulnotion/jam/bmt/io"
	"github.com/colorfulnotion/jam/bmt/store"
)

// beatreeWrapper wraps beatree components for real disk-backed operations.
type beatreeWrapper struct {
	shared *beatree.Shared
	store  *store.Store

	// Metadata file path for root persistence
	metadataFile string
}

// beatreeMetadata stores persistent metadata for the beatree
type beatreeMetadata struct {
	RootPageNumber uint64 `json:"root_page_number"`
}

// openBeatree creates a real beatree with disk persistence.
func openBeatree(opts beatreeOptions) (*beatreeWrapper, error) {
	// Create beatree directory
	beatreeDir := filepath.Join(opts.Path, "beatree")
	if err := os.MkdirAll(beatreeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create beatree directory: %w", err)
	}

	// Open store with real file backing
	storePath := filepath.Join(beatreeDir, "data")
	storeInstance, err := store.Open(storePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open store: %w", err)
	}

	// Create page pool for I/O operations
	pagePool := io.NewPagePool() // Page pool for memory management

	// Create allocator stores for branch and leaf nodes
	branchFile, err := os.OpenFile(filepath.Join(beatreeDir, "branches"), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		storeInstance.Close()
		return nil, fmt.Errorf("failed to open branch file: %w", err)
	}

	leafFile, err := os.OpenFile(filepath.Join(beatreeDir, "leaves"), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		branchFile.Close()
		storeInstance.Close()
		return nil, fmt.Errorf("failed to open leaf file: %w", err)
	}

	branchStore := allocator.NewStore(branchFile, pagePool)
	leafStore := allocator.NewStore(leafFile, pagePool)

	// Create shared beatree state with real storage
	shared := beatree.NewShared(branchStore, leafStore)

	wrapper := &beatreeWrapper{
		shared:       shared,
		store:        storeInstance,
		metadataFile: filepath.Join(beatreeDir, "metadata.json"),
	}

	// Try to recover existing state from the store
	if err := wrapper.recoverBeatreeState(); err != nil {
		// Close resources on recovery failure
		branchFile.Close()
		leafFile.Close()
		storeInstance.Close()
		return nil, fmt.Errorf("failed to recover beatree state: %w", err)
	}

	return wrapper, nil
}

// recoverBeatreeState attempts to recover the beatree root from persistent metadata.
func (bw *beatreeWrapper) recoverBeatreeState() error {
	// Try to load metadata from file
	data, err := os.ReadFile(bw.metadataFile)
	if err != nil {
		// If file doesn't exist, this is a new database
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	// Deserialize metadata
	var metadata beatreeMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	// Restore the root page number
	bw.shared.SetRoot(allocator.PageNumber(metadata.RootPageNumber))

	return nil
}

// saveMetadata saves the current root page number to the metadata file.
func (bw *beatreeWrapper) saveMetadata() error {
	metadata := beatreeMetadata{
		RootPageNumber: uint64(bw.shared.Root()),
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return os.WriteFile(bw.metadataFile, data, 0644)
}

// Get retrieves a value by key using beatree iterator.
func (bw *beatreeWrapper) Get(key beatree.Key) ([]byte, error) {
	// Get snapshot of staging areas
	primaryStaging := bw.shared.PrimaryStaging()
	secondaryStaging := bw.shared.SecondaryStaging()
	root := bw.shared.Root()

	// Create iterator for just this one key [key, key+1)
	endKey := key
	// Increment end key by 1 to create exclusive upper bound
	for i := len(endKey) - 1; i >= 0; i-- {
		if endKey[i] < 255 {
			endKey[i]++
			break
		}
		endKey[i] = 0
	}

	iter := beatree.NewIterator(
		primaryStaging,
		secondaryStaging,
		bw.shared.BranchStore(),
		bw.shared.LeafStore(),
		root,
		key,
		&endKey,
	)

	// Try to get the key
	foundKey, value, valid := iter.Next()
	if !valid {
		// Key not found
		return nil, nil
	}

	// Verify it's the exact key we're looking for
	if bytes.Equal(foundKey[:], key[:]) {
		return value, nil
	}

	// Key not found
	return nil, nil
}

// ApplyChangeset applies a set of changes using real beatree operations.
func (bw *beatreeWrapper) ApplyChangeset(changeset map[beatree.Key]*beatree.Change) error {
	// Commit changes to shared state (adds to primary staging)
	bw.shared.CommitChanges(changeset)

	return nil
}

// Root returns the current root hash from real beatree.
func (bw *beatreeWrapper) Root() [32]byte {
	// Collect all key-value pairs using Enumerate (reads from disk + staging)
	var kvPairs []core.KVPair


	err := bw.Enumerate(func(key [32]byte, value []byte) error {
		kvPair := core.KVPair{
			Key:   core.KeyPath(key),
			Value: value,
		}
		kvPairs = append(kvPairs, kvPair)
		return nil
	})


	if err != nil || len(kvPairs) == 0 {
		// Empty tree or error - return zero root
		return [32]byte{}
	}

	// Sort by key for deterministic tree construction
	for i := 0; i < len(kvPairs)-1; i++ {
		for j := i + 1; j < len(kvPairs); j++ {
			if bytes.Compare(kvPairs[i].Key[:], kvPairs[j].Key[:]) > 0 {
				kvPairs[i], kvPairs[j] = kvPairs[j], kvPairs[i]
			}
		}
	}

	// Create Blake2b hasher
	hasher := core.Blake2bBinaryHasher{}

	// Build GP tree and get root node
	rootNode := core.BuildGpTree(kvPairs, 0, hasher.Hash)

	// Return root hash
	return [32]byte(rootNode.Hash)
}

// Enumerate calls fn for every key/value in the committed tree.
// This reads from the actual beatree storage (disk-backed), not from the in-memory overlay.
// Returns error if fn returns an error, stopping iteration.
func (bw *beatreeWrapper) Enumerate(fn func(key [32]byte, value []byte) error) error {
	// Get snapshot of staging areas for the iterator
	primaryStaging := bw.shared.PrimaryStaging()
	secondaryStaging := bw.shared.SecondaryStaging()

	// Get current root
	root := bw.shared.Root()

	// Create iterator over the entire tree [0, nil) = all keys
	startKey := beatree.Key{} // Zero key (minimum)
	iter := beatree.NewIterator(
		primaryStaging,
		secondaryStaging,
		bw.shared.BranchStore(),
		bw.shared.LeafStore(),
		root,
		startKey,
		nil, // No end bound - iterate all keys
	)

	// Iterate and call fn for each key-value pair
	count := 0
	for {
		key, value, valid := iter.Next()
		if !valid {
			break
		}

		count++
		keyBytes := [32]byte(key)
		if err := fn(keyBytes, value); err != nil {
			return err
		}
	}

	return nil
}


// pageStoreAdapter adapts allocator.Store to ops.PageStore interface
type pageStoreAdapter struct {
	*allocator.Store
}

func (ps *pageStoreAdapter) Alloc() allocator.PageNumber {
	return ps.Store.AllocPage()
}

func (ps *pageStoreAdapter) Page(page allocator.PageNumber) ([]byte, error) {
	ioPage, err := ps.Store.ReadPage(page)
	if err != nil {
		return nil, err
	}
	// Return the page data directly (caller is responsible for deallocation)
	return ioPage, nil
}

func (ps *pageStoreAdapter) WritePage(page allocator.PageNumber, data []byte) {
	// Convert []byte to io.Page for the Store's WritePage method
	ioPage := ps.Store.PagePool().Alloc()
	copy(ioPage, data)
	ps.Store.WritePage(page, ioPage)
}

// Sync flushes changes to disk using real storage.
func (bw *beatreeWrapper) Sync() error {
	// Take staged changeset and commit to persistent storage
	changeset := bw.shared.TakeStagedChangeset()

	if len(changeset) == 0 {
		// No changes to sync
		bw.shared.ClearSecondaryStaging()
		return nil
	}

	// Apply changeset to beatree using ops.Update (writes to actual pages)
	currentRoot := bw.shared.Root()

	// Create adapter for leaf store
	leafStoreAdapter := &pageStoreAdapter{Store: bw.shared.LeafStore()}

	// Use ops.Update to apply changes with copy-on-write
	result, err := ops.Update(
		currentRoot,
		changeset,
		nil, // index not used for simple updates
		leafStoreAdapter,
	)
	if err != nil {
		return fmt.Errorf("failed to update beatree: %w", err)
	}

	// Update the root to the new root from the update operation
	bw.shared.SetRoot(result.NewRoot)


	// Save metadata (root page number) to disk
	if err := bw.saveMetadata(); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	// Clear secondary staging after successful persistence
	defer bw.shared.ClearSecondaryStaging()

	// Sync the underlying store to ensure durability
	rootHash := bw.computeRootHash()


	return bw.store.Sync(rootHash)
}

// computeRootHash computes root hash using the Root() method.
func (bw *beatreeWrapper) computeRootHash() [32]byte {
	return bw.Root()
}

// Close closes the beatree and all underlying resources.
func (bw *beatreeWrapper) Close() error {
	// Close the store
	if err := bw.store.Close(); err != nil {
		return fmt.Errorf("failed to close store: %w", err)
	}

	// No explicit close needed for shared state
	return nil
}

// beatree.Options stub
type beatreeOptions struct {
	Path string
}

func defaultBeatreeOptions() beatreeOptions {
	return beatreeOptions{}
}
