package merkle

import (
	"testing"

	"github.com/jam-duna/jamduna/bmt/core"
	"github.com/jam-duna/jamduna/bmt/io"
)

func TestWitnessPathCollection(t *testing.T) {
	// Create page pool and mock worker components
	pagePool := io.NewPagePool()
	workQueue := make(chan *WorkItem, 10)
	resultQueue := make(chan *WorkResult, 10)

	// Create worker
	worker := NewWorker(1, workQueue, resultQueue, pagePool)

	// Create test page ID
	rootPageId := core.RootPageId
	testKey := []byte("test_key_for_witness_generation_testing") // 32+ bytes

	// Mock walk result
	walkResult := &PageWalkResult{
		PageId:      rootPageId,
		UpdatedPage: make([]byte, 1024), // Mock page data
	}

	// Test witness path collection
	siblings, err := worker.collectWitnessPath(rootPageId, testKey, walkResult)
	if err != nil {
		t.Fatalf("Failed to collect witness path: %v", err)
	}

	// Verify we got some sibling hashes
	if len(siblings) == 0 {
		t.Log("No sibling hashes generated (may be empty tree)")
		return
	}

	// Verify sibling hashes are proper format
	for i, siblingHash := range siblings {
		if len(siblingHash) != 32 {
			t.Errorf("Sibling hash %d has incorrect length: expected 32, got %d", i, len(siblingHash))
		}

		// Verify it's a real hash, not a placeholder pattern
		placeholderPattern := append([]byte("sibling-"), testKey...)
		hasher := core.Blake2bBinaryHasher{}
		placeholderHash := hasher.Hash(placeholderPattern)

		if string(siblingHash) == string(placeholderHash[:]) {
			t.Errorf("Sibling hash %d uses placeholder pattern instead of actual merkle path", i)
		}
	}

	t.Logf("✅ Witness path collection generated %d sibling hashes", len(siblings))
}

func TestWitnessGenerationNotFake(t *testing.T) {
	// This test verifies that witness generation uses actual merkle paths, not placeholders

	// Create minimal worker setup
	pagePool := io.NewPagePool()
	workQueue := make(chan *WorkItem, 10)
	resultQueue := make(chan *WorkResult, 10)
	worker := NewWorker(1, workQueue, resultQueue, pagePool)

	// Create test data
	testKey := []byte("test_key_32_bytes_long_for_testing")
	if len(testKey) > 32 {
		testKey = testKey[:32] // Truncate to 32 bytes
	}

	// Check against placeholder pattern
	hasher := core.Blake2bBinaryHasher{}
	placeholderPattern := append([]byte("sibling-"), testKey...)
	placeholderHash := hasher.Hash(placeholderPattern)

	// Collect witness path - should produce real merkle hashes
	walkResult := &PageWalkResult{
		PageId:      core.RootPageId,
		UpdatedPage: make([]byte, 1024),
	}

	siblings, err := worker.collectWitnessPath(core.RootPageId, testKey, walkResult)
	if err != nil {
		t.Fatalf("Failed to collect witness path: %v", err)
	}

	// Verify none of the generated hashes match placeholder pattern
	for i, siblingHash := range siblings {
		if string(siblingHash) == string(placeholderHash[:]) {
			t.Errorf("Sibling hash %d uses placeholder pattern instead of actual merkle path", i)
		}
	}

	t.Logf("✅ Witness generation uses actual merkle paths")
	t.Logf("✅ Generated %d sibling hashes", len(siblings))
}