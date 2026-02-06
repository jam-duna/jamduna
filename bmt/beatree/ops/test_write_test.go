package ops_test

import (
	"os"
	"testing"
	
	"github.com/jam-duna/jamduna/bmt/beatree/allocator"
	"github.com/jam-duna/jamduna/bmt/io"
)

func TestWritePageError(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "write_test_*.dat")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	pagePool := io.NewPagePool()
	store := allocator.NewStore(tmpFile, pagePool)

	// Allocate a page
	page := store.AllocPage()
	t.Logf("Allocated page %v", page)

	// Create test data
	data := make([]byte, io.PageSize)
	for i := range data {
		data[i] = byte(i % 256)
	}

	// Write page
	err = store.WritePage(page, data)
	if err != nil {
		t.Fatalf("WritePage failed: %v", err)
	}
	t.Logf("WritePage succeeded for page %v", page)

	// Read back
	readData, err := store.ReadPage(page)
	if err != nil {
		t.Fatalf("ReadPage failed: %v", err)
	}
	t.Logf("ReadPage succeeded for page %v, got %d bytes", page, len(readData))
}
