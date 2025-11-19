package store

import (
	"bytes"
	"testing"

	"github.com/colorfulnotion/jam/bmt/core"
)

func TestStoreOpen(t *testing.T) {
	tmpDir := t.TempDir()

	// Open new store
	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Should have zero root hash (empty tree)
	rootHash := store.RootHash()
	expectedHash := [32]byte{}
	if rootHash != expectedHash {
		t.Errorf("Expected zero root hash, got %v", rootHash)
	}

	// Should have sync version 0
	if store.SyncVersion() != 0 {
		t.Errorf("Expected sync version 0, got %d", store.SyncVersion())
	}
}

func TestStorePage(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Create a page
	pageId, err := core.NewPageId([]byte{1, 2, 3})
	if err != nil {
		t.Fatalf("Failed to create PageId: %v", err)
	}

	pageData := make([]byte, 16384)
	for i := range pageData {
		pageData[i] = byte(i % 256)
	}

	// Store page in bitbox
	err = store.StorePage(*pageId, pageData)
	if err != nil {
		t.Fatalf("Failed to store page: %v", err)
	}

	// Retrieve page
	retrieved, err := store.RetrievePage(*pageId)
	if err != nil {
		t.Fatalf("Failed to retrieve page: %v", err)
	}

	// Verify content
	if !bytes.Equal(retrieved, pageData) {
		t.Errorf("Page data mismatch")
	}
}

func TestStoreSync(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}

	// Store a page
	pageId, _ := core.NewPageId([]byte{5})
	pageData := make([]byte, 16384)
	for i := range pageData {
		pageData[i] = 0xAB
	}
	store.StorePage(*pageId, pageData)

	// Sync with a new root hash
	newRootHash := [32]byte{1, 2, 3, 4, 5}
	err = store.Sync(newRootHash)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Verify root hash updated
	if store.RootHash() != newRootHash {
		t.Errorf("Root hash not updated after sync")
	}

	// Verify sync version incremented
	if store.SyncVersion() != 1 {
		t.Errorf("Expected sync version 1, got %d", store.SyncVersion())
	}

	store.Close()

	// Reopen store
	store2, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer store2.Close()

	// Verify manifest persisted
	if store2.RootHash() != newRootHash {
		t.Errorf("Root hash not persisted")
	}

	if store2.SyncVersion() != 1 {
		t.Errorf("Sync version not persisted")
	}

	// Verify page is still accessible
	retrieved, err := store2.RetrievePage(*pageId)
	if err != nil {
		t.Fatalf("Failed to retrieve page after reopen: %v", err)
	}

	if !bytes.Equal(retrieved, pageData) {
		t.Errorf("Page data corrupted after reopen")
	}
}

func TestStoreReopen(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and close store
	store1, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}

	// Store some pages
	for i := 0; i < 5; i++ {
		pageId, _ := core.NewPageId([]byte{byte(i)})
		pageData := make([]byte, 16384)
		for j := range pageData {
			pageData[j] = byte((i * 100 + j) % 256)
		}
		store1.StorePage(*pageId, pageData)
	}

	// Sync
	rootHash := [32]byte{0xFF, 0xFF}
	store1.Sync(rootHash)
	store1.Close()

	// Reopen
	store2, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer store2.Close()

	// Verify root hash
	if store2.RootHash() != rootHash {
		t.Errorf("Root hash mismatch after reopen")
	}

	// Verify all pages accessible
	for i := 0; i < 5; i++ {
		pageId, _ := core.NewPageId([]byte{byte(i)})
		retrieved, err := store2.RetrievePage(*pageId)
		if err != nil {
			t.Errorf("Failed to retrieve page %d after reopen: %v", i, err)
			continue
		}

		// Verify content
		expectedData := make([]byte, 16384)
		for j := range expectedData {
			expectedData[j] = byte((i * 100 + j) % 256)
		}

		if !bytes.Equal(retrieved, expectedData) {
			t.Errorf("Page %d data mismatch after reopen", i)
		}
	}
}

func TestAllocatePageId(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Allocate first page
	pageId1, err := store.AllocatePageId()
	if err != nil {
		t.Fatalf("Failed to allocate first page: %v", err)
	}

	// Allocate second page
	pageId2, err := store.AllocatePageId()
	if err != nil {
		t.Fatalf("Failed to allocate second page: %v", err)
	}

	// Verify PageIds are different
	encoded1 := pageId1.Encode()
	encoded2 := pageId2.Encode()

	if bytes.Equal(encoded1[:], encoded2[:]) {
		t.Errorf("Allocated PageIds should be different")
	}

	// Verify PageIds are valid (can be decoded)
	decoded1, err := core.DecodePageId(encoded1)
	if err != nil {
		t.Errorf("Failed to decode PageId1: %v", err)
	}

	decoded2, err := core.DecodePageId(encoded2)
	if err != nil {
		t.Errorf("Failed to decode PageId2: %v", err)
	}

	// Verify round-trip encoding
	if decoded1.Encode() != encoded1 {
		t.Errorf("PageId1 round-trip encoding mismatch")
	}

	if decoded2.Encode() != encoded2 {
		t.Errorf("PageId2 round-trip encoding mismatch")
	}
}

func TestAllocatePageIdBeyond64(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Allocate 100 PageIds to verify we don't hit the 6-bit constraint at 64
	// This is the critical test - the old implementation would fail at PageId 64
	for i := 1; i <= 100; i++ {
		pageId, err := store.AllocatePageId()
		if err != nil {
			t.Fatalf("Failed to allocate PageId %d: %v", i, err)
		}

		// Verify PageId can be encoded and decoded
		encoded := pageId.Encode()
		decoded, err := core.DecodePageId(encoded)
		if err != nil {
			t.Fatalf("Failed to decode PageId %d: %v", i, err)
		}

		// Verify round-trip
		if decoded.Encode() != encoded {
			t.Errorf("PageId %d round-trip encoding mismatch", i)
		}

		// Verify depth is as expected (counter in base-64)
		expectedDepth := 0
		temp := uint64(i)
		for temp > 0 {
			expectedDepth++
			temp /= 64
		}
		if pageId.Depth() != expectedDepth {
			t.Errorf("PageId %d has wrong depth: got %d, expected %d", i, pageId.Depth(), expectedDepth)
		}
	}
}
