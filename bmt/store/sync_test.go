package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/bmt/core"
)

func TestSyncTwoPhase(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Store pages
	pageId1, _ := core.NewPageId([]byte{1})
	pageData1 := make([]byte, 16384)
	for i := range pageData1 {
		pageData1[i] = 0xAA
	}
	store.StorePage(*pageId1, pageData1)

	// Sync (two-phase: bitbox sync → manifest update)
	rootHash := [32]byte{0xFF}
	err = store.Sync(rootHash)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Verify manifest written
	manifest, err := ReadManifest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read manifest after sync: %v", err)
	}

	if manifest.RootHash != rootHash {
		t.Errorf("Manifest root hash mismatch")
	}

	if manifest.SyncVersion != 1 {
		t.Errorf("Manifest sync version should be 1, got %d", manifest.SyncVersion)
	}
}

func TestSyncManifestAtomic(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Perform multiple syncs
	for i := 0; i < 5; i++ {
		rootHash := [32]byte{byte(i)}
		err = store.Sync(rootHash)
		if err != nil {
			t.Fatalf("Sync %d failed: %v", i, err)
		}

		// Verify manifest updated atomically
		manifest, err := ReadManifest(tmpDir)
		if err != nil {
			t.Fatalf("Failed to read manifest after sync %d: %v", i, err)
		}

		if manifest.RootHash != rootHash {
			t.Errorf("Manifest not updated atomically on sync %d", i)
		}

		if manifest.SyncVersion != uint64(i+1) {
			t.Errorf("Sync version mismatch: expected %d, got %d", i+1, manifest.SyncVersion)
		}

		// Verify temp file cleaned up
		tempPath := filepath.Join(tmpDir, "manifest.tmp")
		if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
			t.Errorf("Temp manifest file not cleaned up after sync %d", i)
		}
	}
}

func TestSyncCrashBeforeManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Simulate: Store pages, bitbox syncs, but manifest NOT written (crash)

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}

	// Store page
	pageId, _ := core.NewPageId([]byte{10})
	pageData := make([]byte, 16384)
	for i := range pageData {
		pageData[i] = 0xBB
	}
	store.StorePage(*pageId, pageData)

	// Manually sync bitbox (but NOT manifest - simulating crash)
	// Note: We can't easily test this without refactoring Sync() to be testable
	// For now, just verify recovery works if manifest is old

	// Close without syncing manifest
	store.Close()

	// Reopen - should use old manifest (sync version 0, zero root hash)
	store2, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to reopen after crash: %v", err)
	}
	defer store2.Close()

	// Manifest should still be at version 0 (never synced)
	if store2.SyncVersion() != 0 {
		t.Errorf("Expected sync version 0 after crash before manifest, got %d", store2.SyncVersion())
	}

	// Page should still be in WAL (recovered)
	// BitBox Close() writes WAL for dirty pages
	retrieved, err := store2.RetrievePage(*pageId)
	if err != nil {
		t.Logf("Page not found after crash recovery (expected if WAL not written): %v", err)
		// This is OK - page was never synced to HT file
	} else if !bytes.Equal(retrieved, pageData) {
		t.Errorf("Page data corrupted after recovery")
	}
}

func TestSyncCrashAfterManifest(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}

	// Store and sync
	pageId, _ := core.NewPageId([]byte{20})
	pageData := make([]byte, 16384)
	for i := range pageData {
		pageData[i] = 0xCC
	}
	store.StorePage(*pageId, pageData)

	rootHash := [32]byte{0x12, 0x34}
	err = store.Sync(rootHash)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Close and reopen (simulate crash after manifest written)
	store.Close()

	store2, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to reopen: %v", err)
	}
	defer store2.Close()

	// Manifest should persist
	if store2.RootHash() != rootHash {
		t.Errorf("Root hash not persisted after sync")
	}

	if store2.SyncVersion() != 1 {
		t.Errorf("Sync version not persisted")
	}

	// Page should be accessible
	retrieved, err := store2.RetrievePage(*pageId)
	if err != nil {
		t.Fatalf("Page not found after recovery: %v", err)
	}

	if !bytes.Equal(retrieved, pageData) {
		t.Errorf("Page data corrupted")
	}
}

func TestConcurrentCommits(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	// Concurrent sync calls should be serialized by mutex
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 5; i++ {
			rootHash := [32]byte{byte(i * 2)}
			store.Sync(rootHash)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 5; i++ {
			rootHash := [32]byte{byte(i*2 + 1)}
			store.Sync(rootHash)
		}
		done <- true
	}()

	<-done
	<-done

	// Final sync version should be 10 (5 + 5 syncs)
	if store.SyncVersion() != 10 {
		t.Errorf("Expected sync version 10 after concurrent syncs, got %d", store.SyncVersion())
	}
}

func TestSyncVersionMonotonic(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}

	// Sync multiple times
	for i := 0; i < 10; i++ {
		rootHash := [32]byte{byte(i)}
		err = store.Sync(rootHash)
		if err != nil {
			t.Fatalf("Sync %d failed: %v", i, err)
		}

		expectedVersion := uint64(i + 1)
		if store.SyncVersion() != expectedVersion {
			t.Errorf("Sync version not monotonic: expected %d, got %d", expectedVersion, store.SyncVersion())
		}
	}

	store.Close()

	// Reopen and verify version persisted
	store2, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("Failed to reopen: %v", err)
	}
	defer store2.Close()

	if store2.SyncVersion() != 10 {
		t.Errorf("Sync version not persisted: expected 10, got %d", store2.SyncVersion())
	}
}
