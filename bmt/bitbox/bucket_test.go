package bitbox

import "testing"

func TestBucketIsValid(t *testing.T) {
	// Valid bucket indices
	validBuckets := []BucketIndex{0, 1, 100, 1000, 1<<32 - 1}
	for _, b := range validBuckets {
		if !b.IsValid() {
			t.Errorf("BucketIndex(%d) should be valid", b)
		}
	}

	// Invalid bucket index (sentinel)
	if InvalidBucketIndex.IsValid() {
		t.Errorf("InvalidBucketIndex should not be valid")
	}

	// Verify sentinel value is MaxUint64
	if InvalidBucketIndex != ^BucketIndex(0) {
		t.Errorf("InvalidBucketIndex should be MaxUint64")
	}
}

func TestSharedMaybeBucketIndex(t *testing.T) {
	// Create with nil (none)
	s1 := NewSharedMaybeBucketIndex(nil)
	if val := s1.Get(); val != nil {
		t.Errorf("Expected nil, got %v", val)
	}

	// Create with a value
	bucket := BucketIndex(42)
	s2 := NewSharedMaybeBucketIndex(&bucket)
	if val := s2.Get(); val == nil || *val != 42 {
		t.Errorf("Expected BucketIndex(42), got %v", val)
	}

	// Set a value
	s1.Set(BucketIndex(100))
	if val := s1.Get(); val == nil || *val != 100 {
		t.Errorf("Expected BucketIndex(100) after Set, got %v", val)
	}

	// Update an existing value
	s2.Set(BucketIndex(99))
	if val := s2.Get(); val == nil || *val != 99 {
		t.Errorf("Expected BucketIndex(99) after update, got %v", val)
	}

	// Test concurrent safety (basic check)
	// This doesn't guarantee thread safety but exercises the atomic operations
	s3 := NewSharedMaybeBucketIndex(nil)
	done := make(chan bool)

	go func() {
		for i := 0; i < 1000; i++ {
			s3.Set(BucketIndex(i))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			_ = s3.Get()
		}
		done <- true
	}()

	<-done
	<-done

	// Should have some value after all operations
	if val := s3.Get(); val == nil {
		t.Errorf("Expected a value after concurrent operations")
	}
}
