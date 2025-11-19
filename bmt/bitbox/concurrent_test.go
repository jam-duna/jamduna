package bitbox

import (
	"bytes"
	"sync"
	"testing"

	"github.com/colorfulnotion/jam/bmt/core"
)

func TestConcurrentInserts(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(tmpDir, 10000)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Insert pages concurrently
	numGoroutines := 10
	pagesPerGoroutine := 10
	var wg sync.WaitGroup

	// Store test data for verification
	testData := make(map[string][]byte)
	var testDataMu sync.Mutex

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := 0; i < pagesPerGoroutine; i++ {
				// Create unique PageId
				path := []byte{byte(goroutineID), byte(i)}
				pageId, err := core.NewPageId(path)
				if err != nil {
					t.Errorf("Failed to create PageId: %v", err)
					return
				}

				// Create page data
				pageData := make([]byte, PageSize)
				for j := range pageData {
					pageData[j] = byte((goroutineID*100 + i + j) % 256)
				}

				// Insert
				err = db.InsertPage(*pageId, pageData)
				if err != nil {
					t.Errorf("Failed to insert page: %v", err)
					return
				}

				// Store for verification
				encoded := pageId.Encode()
				key := string(encoded[:])
				testDataMu.Lock()
				testData[key] = pageData
				testDataMu.Unlock()
			}
		}(g)
	}

	wg.Wait()

	// Verify all pages were inserted
	expectedCount := numGoroutines * pagesPerGoroutine
	if len(testData) != expectedCount {
		t.Errorf("Expected %d unique pages, got %d", expectedCount, len(testData))
	}

	// Verify all pages can be retrieved
	for keyStr, expectedData := range testData {
		var pageIdBytes [32]byte
		copy(pageIdBytes[:], []byte(keyStr))
		pageId, err := core.DecodePageId(pageIdBytes)
		if err != nil {
			t.Fatalf("Failed to decode PageId: %v", err)
		}

		retrieved, err := db.RetrievePage(*pageId)
		if err != nil {
			t.Errorf("Failed to retrieve page: %v", err)
			continue
		}

		if !bytes.Equal(retrieved, expectedData) {
			t.Errorf("Retrieved data doesn't match for concurrent insert")
		}
	}
}
