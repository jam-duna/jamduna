package bitbox

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/bmt/core"
)

func TestDbOpen(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Open database
	db, err := Open(tmpDir, 100)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Verify files were created
	htPath := filepath.Join(tmpDir, "ht")
	walPath := filepath.Join(tmpDir, "wal")

	if _, err := os.Stat(htPath); os.IsNotExist(err) {
		t.Errorf("HT file should exist")
	}

	if _, err := os.Stat(walPath); os.IsNotExist(err) {
		t.Errorf("WAL file should exist")
	}

	// Re-open the database
	db.Close()
	db2, err := Open(tmpDir, 100)
	if err != nil {
		t.Fatalf("Failed to re-open DB: %v", err)
	}
	defer db2.Close()
}

func TestDbInsertPage(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir, 1000)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Create a page
	pageId, _ := core.NewPageId([]byte{1, 2, 3})
	pageData := make([]byte, PageSize)
	for i := range pageData {
		pageData[i] = byte(i % 256)
	}

	// Insert the page
	err = db.InsertPage(*pageId, pageData)
	if err != nil {
		t.Fatalf("Failed to insert page: %v", err)
	}

	// Verify it was added to dirty pages
	if len(db.dirtyPages) != 1 {
		t.Errorf("Should have 1 dirty page, got %d", len(db.dirtyPages))
	}
}

func TestDbRetrievePage(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir, 1000)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Create and insert a page
	pageId, _ := core.NewPageId([]byte{5, 10, 15})
	pageData := make([]byte, PageSize)
	for i := range pageData {
		pageData[i] = byte((i * 3) % 256)
	}

	err = db.InsertPage(*pageId, pageData)
	if err != nil {
		t.Fatalf("Failed to insert page: %v", err)
	}

	// Retrieve the page
	retrieved, err := db.RetrievePage(*pageId)
	if err != nil {
		t.Fatalf("Failed to retrieve page: %v", err)
	}

	// Verify content
	if !bytes.Equal(retrieved, pageData) {
		t.Errorf("Retrieved page data doesn't match original")
	}
}

func TestDbSync(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir, 1000)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}

	// Insert some pages
	for i := 0; i < 5; i++ {
		pageId, _ := core.NewPageId([]byte{byte(i)})
		pageData := make([]byte, PageSize)
		for j := range pageData {
			pageData[j] = byte((i + j) % 256)
		}
		err = db.InsertPage(*pageId, pageData)
		if err != nil {
			t.Fatalf("Failed to insert page %d: %v", i, err)
		}
	}

	// Sync to disk
	err = db.Sync()
	if err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Verify dirty pages were cleared
	if len(db.dirtyPages) != 0 {
		t.Errorf("Dirty pages should be cleared after sync, got %d", len(db.dirtyPages))
	}

	// Close and reopen
	db.Close()
	db2, err := Open(tmpDir, 1000)
	if err != nil {
		t.Fatalf("Failed to reopen DB: %v", err)
	}
	defer db2.Close()

	// Verify pages are still accessible
	pageId, _ := core.NewPageId([]byte{2})
	retrieved, err := db2.RetrievePage(*pageId)
	if err != nil {
		t.Fatalf("Failed to retrieve page after reopen: %v", err)
	}

	// Verify content
	expectedData := make([]byte, PageSize)
	for j := range expectedData {
		expectedData[j] = byte((2 + j) % 256)
	}

	if !bytes.Equal(retrieved, expectedData) {
		t.Errorf("Page data doesn't match after reopen")
	}
}
