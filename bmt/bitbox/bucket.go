package bitbox

import "sync/atomic"

// BucketIndex is the index of a bucket within the hash table.
type BucketIndex uint64

// IsValid returns true if the bucket index is valid (not the sentinel).
func (b BucketIndex) IsValid() bool {
	return b != InvalidBucketIndex
}

// InvalidBucketIndex is a sentinel value for invalid/unallocated buckets.
const InvalidBucketIndex BucketIndex = ^BucketIndex(0) // MaxUint64

// SharedMaybeBucketIndex is an atomically-mutable optional bucket index.
// This is used as a shared placeholder for a bucket that will be allocated
// in the future but hasn't yet.
//
// We assume no bucket indices reach MaxUint64, as this would require 2^76
// bytes (64 billion TB) of physical storage.
type SharedMaybeBucketIndex struct {
	val atomic.Uint64
}

// NewSharedMaybeBucketIndex creates a new shared optional bucket index.
func NewSharedMaybeBucketIndex(bucket *BucketIndex) *SharedMaybeBucketIndex {
	s := &SharedMaybeBucketIndex{}
	if bucket == nil {
		s.val.Store(uint64(InvalidBucketIndex))
	} else {
		s.val.Store(uint64(*bucket))
	}
	return s
}

// Set atomically sets the bucket index (Relaxed ordering).
func (s *SharedMaybeBucketIndex) Set(bucket BucketIndex) {
	s.val.Store(uint64(bucket))
}

// Get atomically gets the bucket index (Relaxed ordering).
// Returns nil if not set.
func (s *SharedMaybeBucketIndex) Get() *BucketIndex {
	val := s.val.Load()
	if val == uint64(InvalidBucketIndex) {
		return nil
	}
	b := BucketIndex(val)
	return &b
}
