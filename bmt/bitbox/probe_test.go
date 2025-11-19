package bitbox

import (
	"testing"
)

func TestTriangularProbing(t *testing.T) {
	// Create a small meta map for testing
	m := NewMetaMap(100)

	// Hash that maps to bucket 10
	hash := uint64(10)
	probe := NewProbeSequence(hash, m.Len())

	// Verify initial bucket
	if probe.bucket != 10 {
		t.Errorf("Initial bucket should be 10, got %d", probe.bucket)
	}
	if probe.step != 0 {
		t.Errorf("Initial step should be 0, got %d", probe.step)
	}

	// Track the sequence of buckets visited
	buckets := []uint64{probe.bucket} // Start with initial bucket

	// Make several probes to verify triangular sequence
	for i := 0; i < 10; i++ {
		result := probe.Next(m)
		if result.Type != ProbeResultEmpty {
			// Keep probing until we find empty
			continue
		}
		buckets = append(buckets, result.BucketIndex)
		break
	}

	// Triangular sequence should be: bucket[0] = 10
	// Then: 10+0=10, 10+1=11, 11+2=13, 13+3=16, 16+4=20, 20+5=25, ...
	// The steps are: 0, 1, 2, 3, 4, 5, ... (triangular numbers)

	// First probe advances by step (0), so stays at 10
	// But Next() increments step first, so actually:
	// bucket = (10 + 0) % 100 = 10, step becomes 1
	// bucket = (10 + 1) % 100 = 11, step becomes 2
	// bucket = (11 + 2) % 100 = 13, step becomes 3
	// etc.

	// Since the map is empty, first call to Next should return bucket 10
	probe2 := NewProbeSequence(hash, m.Len())
	result := probe2.Next(m)
	if result.Type != ProbeResultEmpty {
		t.Errorf("First probe should find empty bucket")
	}
	if result.BucketIndex != 10 {
		t.Errorf("First probe should return bucket 10, got %d", result.BucketIndex)
	}
}

func TestProbeNoCollision(t *testing.T) {
	// Empty meta map - first probe should always find empty bucket
	m := NewMetaMap(1000)

	testCases := []uint64{0, 1, 10, 100, 500, 999}

	for _, hash := range testCases {
		probe := NewProbeSequence(hash, m.Len())
		result := probe.Next(m)

		if result.Type != ProbeResultEmpty {
			t.Errorf("First probe with hash %d should find empty bucket, got type %v", hash, result.Type)
		}

		expectedBucket := hash % uint64(m.Len())
		if result.BucketIndex != expectedBucket {
			t.Errorf("First probe with hash %d should return bucket %d, got %d", hash, expectedBucket, result.BucketIndex)
		}
	}
}

func TestProbeWithCollision(t *testing.T) {
	m := NewMetaMap(100)

	// Use hash that maps to bucket 10
	hash := uint64(10)

	// Mark bucket 10 as full with a DIFFERENT hash (top 7 bits must differ)
	// This simulates a collision
	differentHash := uint64(1<<58) | 10 // Different top 7 bits, same bucket (% 100 = 10)
	m.SetFull(10, differentHash)

	// Probe should skip bucket 10 and continue
	probe := NewProbeSequence(hash, m.Len())
	result := probe.Next(m)

	// Should not return bucket 10 (it's occupied by different hash)
	// Should probe further and find an empty bucket
	if result.Type != ProbeResultEmpty {
		t.Errorf("Should eventually find empty bucket, got type %v", result.Type)
	}

	if result.BucketIndex == 10 {
		t.Errorf("Should skip bucket 10 (occupied), but got bucket 10")
	}

	// Verify it found a bucket in the triangular sequence
	// Starting at 10: next buckets are 10, 11, 13, 16, 20, 25, 31, ...
	validBuckets := map[uint64]bool{10: true}
	bucket := uint64(10)
	step := uint64(1)
	for i := 0; i < 20; i++ {
		bucket = (bucket + step) % uint64(m.Len())
		step++
		validBuckets[bucket] = true
	}

	if !validBuckets[result.BucketIndex] {
		t.Errorf("Bucket %d not in expected triangular sequence from 10", result.BucketIndex)
	}
}

func TestProbeWithTombstone(t *testing.T) {
	m := NewMetaMap(100)

	hash := uint64(20)

	// Mark bucket 20 as tombstone
	m.SetTombstone(20)

	// Probe should skip tombstones and continue
	probe := NewProbeSequence(hash, m.Len())
	result := probe.Next(m)

	// Should skip the tombstone and find an empty bucket
	if result.Type != ProbeResultEmpty {
		t.Errorf("Should find empty bucket after skipping tombstone, got type %v", result.Type)
	}

	// Should not return bucket 20 (it's a tombstone)
	if result.BucketIndex == 20 {
		t.Errorf("Should skip tombstone at bucket 20")
	}
}

func TestProbeWithMultipleCollisions(t *testing.T) {
	m := NewMetaMap(100)

	hash := uint64(50)

	// Fill several buckets in the probe sequence with different hashes (top 7 bits differ)
	// Triangular sequence from 50: 50, 51, 53, 56, 60, 65, ...
	m.SetFull(50, (uint64(1)<<58)|50) // Different hash (top 7 bits differ)
	m.SetFull(51, (uint64(2)<<58)|51)
	m.SetFull(53, (uint64(3)<<58)|53)

	// Probe should skip all these and find empty bucket
	probe := NewProbeSequence(hash, m.Len())
	result := probe.Next(m)

	if result.Type != ProbeResultEmpty {
		t.Errorf("Should find empty bucket, got type %v", result.Type)
	}

	// Should skip 50, 51, 53 and find a later bucket
	if result.BucketIndex == 50 || result.BucketIndex == 51 || result.BucketIndex == 53 {
		t.Errorf("Should skip occupied buckets 50, 51, 53, but got %d", result.BucketIndex)
	}

	// Should be bucket 56 or later in sequence
	if result.BucketIndex < 56 {
		t.Errorf("Expected bucket 56 or later, got %d", result.BucketIndex)
	}
}

func TestProbePossibleHit(t *testing.T) {
	m := NewMetaMap(100)

	hash := uint64(30)

	// Mark bucket 30 as full with the SAME hash (top 7 bits)
	m.SetFull(30, hash)

	// Probe should return PossibleHit (needs verification by reading actual page)
	probe := NewProbeSequence(hash, m.Len())
	result := probe.Next(m)

	if result.Type != ProbeResultPossibleHit {
		t.Errorf("Same hash should return PossibleHit, got type %v", result.Type)
	}

	if result.BucketIndex != 30 {
		t.Errorf("PossibleHit should be at bucket 30, got %d", result.BucketIndex)
	}
}

func TestProbeWraparound(t *testing.T) {
	// Test that probing wraps around the meta map correctly
	m := NewMetaMap(10) // Small map to force wraparound

	hash := uint64(9) // Last bucket

	// Probe sequence from 9 should wrap: 9, (9+1)%10=0, (0+2)%10=2, (2+3)%10=5, (5+4)%10=9, ...
	probe := NewProbeSequence(hash, m.Len())

	buckets := make(map[uint64]bool)
	for i := 0; i < 20; i++ {
		result := probe.Next(m)
		if result.Type == ProbeResultEmpty {
			buckets[result.BucketIndex] = true
			break
		}
	}

	// Should have wrapped around successfully
	if len(buckets) == 0 {
		t.Errorf("Should have found at least one bucket")
	}
}
