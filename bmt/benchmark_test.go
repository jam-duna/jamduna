package bmt

import (
	"crypto/rand"
	"testing"
)

// BenchmarkInsertRandomKeys benchmarks insertion of random keys
func BenchmarkInsertRandomKeys(b *testing.B) {
	// Create temporary directory
	tmpDir := b.TempDir()

	// Create BMT instance
	opts := DefaultOptions(tmpDir)
	nomt, err := Open(opts)
	if err != nil {
		b.Fatalf("Failed to create NOMT: %v", err)
	}
	defer nomt.Close()

	// Generate random key-value pairs
	keys := make([][32]byte, b.N)
	values := make([][]byte, b.N)

	for i := 0; i < b.N; i++ {
		// Generate random 32-byte key
		rand.Read(keys[i][:])

		// Generate random value (1KB)
		values[i] = make([]byte, 1024)
		rand.Read(values[i])
	}

	// Reset timer after setup
	b.ResetTimer()

	// Benchmark insertions
	for i := 0; i < b.N; i++ {
		err := nomt.Insert(keys[i], values[i])
		if err != nil {
			b.Fatalf("Failed to insert: %v", err)
		}
	}
}

// BenchmarkLookupExistingKeys benchmarks lookup of existing keys
func BenchmarkLookupExistingKeys(b *testing.B) {
	// Create temporary directory
	tmpDir := b.TempDir()

	// Create BMT instance
	opts := DefaultOptions(tmpDir)
	nomt, err := Open(opts)
	if err != nil {
		b.Fatalf("Failed to create NOMT: %v", err)
	}
	defer nomt.Close()

	// Insert some keys first
	numKeys := 1000
	keys := make([][32]byte, numKeys)
	values := make([][]byte, numKeys)

	for i := 0; i < numKeys; i++ {
		// Generate random 32-byte key
		rand.Read(keys[i][:])

		// Generate random value
		values[i] = make([]byte, 256)
		rand.Read(values[i])

		// Insert key
		err := nomt.Insert(keys[i], values[i])
		if err != nil {
			b.Fatalf("Failed to insert: %v", err)
		}
	}

	// Reset timer after setup
	b.ResetTimer()

	// Benchmark lookups
	for i := 0; i < b.N; i++ {
		keyIndex := i % numKeys

		_, err := nomt.Get(keys[keyIndex])
		if err != nil {
			b.Fatalf("Failed to lookup: %v", err)
		}
	}
}

// BenchmarkOverflowValues benchmarks insertion/lookup of large overflow values
func BenchmarkOverflowValues(b *testing.B) {
	// Create temporary directory
	tmpDir := b.TempDir()

	// Create BMT instance
	opts := DefaultOptions(tmpDir)
	nomt, err := Open(opts)
	if err != nil {
		b.Fatalf("Failed to create NOMT: %v", err)
	}
	defer nomt.Close()

	// Generate large values (>1KB to trigger overflow)
	keys := make([][32]byte, b.N)
	values := make([][]byte, b.N)

	for i := 0; i < b.N; i++ {
		// Generate random 32-byte key
		rand.Read(keys[i][:])

		// Generate large value (5KB to ensure overflow)
		values[i] = make([]byte, 5*1024)
		rand.Read(values[i])
	}

	// Reset timer after setup
	b.ResetTimer()

	// Benchmark overflow value operations
	for i := 0; i < b.N; i++ {
		err := nomt.Insert(keys[i], values[i])
		if err != nil {
			b.Fatalf("Failed to insert overflow value: %v", err)
		}

		// Verify we can read it back
		readValue, err := nomt.Get(keys[i])
		if err != nil {
			b.Fatalf("Failed to lookup overflow value: %v", err)
		}

		if len(readValue) != len(values[i]) {
			b.Fatalf("Overflow value size mismatch: expected %d, got %d", len(values[i]), len(readValue))
		}
	}
}