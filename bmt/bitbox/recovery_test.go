package bitbox

import (
	"bytes"
	"testing"

	"github.com/colorfulnotion/jam/bmt/core"
)

func TestWalRecovery(t *testing.T) {
	tmpDir := t.TempDir()

	// Open DB and insert pages
	db, err := Open(tmpDir, 1000)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}

	// Insert pages
	testData := make(map[string][]byte)
	for i := 0; i < 3; i++ {
		pageId, _ := core.NewPageId([]byte{byte(i * 10)})
		pageData := make([]byte, PageSize)
		for j := range pageData {
			pageData[j] = byte((i * 100 + j) % 256)
		}
		err = db.InsertPage(*pageId, pageData)
		if err != nil {
			t.Fatalf("Failed to insert page %d: %v", i, err)
		}

		// Store for verification
		encoded := pageId.Encode()
		key := string(encoded[:])
		testData[key] = pageData
	}

	// Close without syncing (WAL contains un-synced data)
	db.Close()

	// Reopen - should trigger recovery
	db2, err := Open(tmpDir, 1000)
	if err != nil {
		t.Fatalf("Failed to reopen DB for recovery: %v", err)
	}
	defer db2.Close()

	// Verify all pages were recovered
	for keyStr, expectedData := range testData {
		var pageIdBytes [32]byte
		copy(pageIdBytes[:], []byte(keyStr))
		pageId, _ := core.DecodePageId(pageIdBytes)

		retrieved, err := db2.RetrievePage(*pageId)
		if err != nil {
			t.Fatalf("Failed to retrieve recovered page: %v", err)
		}

		if !bytes.Equal(retrieved, expectedData) {
			t.Errorf("Recovered page data doesn't match original")
		}
	}
}

func TestWalRecoveryEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	// Open DB (creates empty WAL)
	db, err := Open(tmpDir, 100)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	db.Close()

	// Reopen with empty WAL
	db2, err := Open(tmpDir, 100)
	if err != nil {
		t.Fatalf("Failed to reopen with empty WAL: %v", err)
	}
	defer db2.Close()

	// Should work fine with no pages
	pageId, err := core.NewPageId([]byte{42}) // Valid child index (0-63)
	if err != nil {
		t.Fatalf("Failed to create PageId: %v", err)
	}
	_, err = db2.RetrievePage(*pageId)
	if err == nil {
		t.Errorf("Should fail to retrieve non-existent page")
	}
}

func TestWalRecoveryPartial(t *testing.T) {
	tmpDir := t.TempDir()

	// Open DB
	db, err := Open(tmpDir, 1000)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}

	// Insert a page
	pageId, _ := core.NewPageId([]byte{7})
	pageData := make([]byte, PageSize)
	for i := range pageData {
		pageData[i] = 0xAB
	}
	err = db.InsertPage(*pageId, pageData)
	if err != nil {
		t.Fatalf("Failed to insert page: %v", err)
	}

	// Sync this page
	err = db.Sync()
	if err != nil {
		t.Fatalf("Failed to sync: %v", err)
	}

	// Insert another page (not synced)
	pageId2, _ := core.NewPageId([]byte{8})
	pageData2 := make([]byte, PageSize)
	for i := range pageData2 {
		pageData2[i] = 0xCD
	}
	err = db.InsertPage(*pageId2, pageData2)
	if err != nil {
		t.Fatalf("Failed to insert second page: %v", err)
	}

	// Close without syncing second page
	db.Close()

	// Reopen
	db2, err := Open(tmpDir, 1000)
	if err != nil {
		t.Fatalf("Failed to reopen: %v", err)
	}
	defer db2.Close()

	// First page should be retrievable (was synced)
	retrieved1, err := db2.RetrievePage(*pageId)
	if err != nil {
		t.Fatalf("Failed to retrieve first page: %v", err)
	}
	if !bytes.Equal(retrieved1, pageData) {
		t.Errorf("First page data mismatch")
	}

	// Second page should also be retrievable (recovered from WAL)
	retrieved2, err := db2.RetrievePage(*pageId2)
	if err != nil {
		t.Fatalf("Failed to retrieve second page after recovery: %v", err)
	}
	if !bytes.Equal(retrieved2, pageData2) {
		t.Errorf("Second page data mismatch after recovery")
	}
}
