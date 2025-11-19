package bitbox

import (
	"fmt"

	"github.com/colorfulnotion/jam/bmt/core"
)

// BucketExhaustion is returned when the hash table is too full to allocate a new bucket.
type BucketExhaustion struct{}

func (e BucketExhaustion) Error() string {
	return "bucket exhaustion: hash table is too full"
}

// AllocateBucket finds an empty bucket for the given PageId using triangular probing.
// Returns BucketExhaustion if the hash table is too full (probe limit exceeded).
func AllocateBucket(pageId core.PageId, seed uint64, metaMap *MetaMap) (BucketIndex, error) {
	hash := HashPageId(pageId, seed)
	probe := NewProbeSequence(hash, metaMap.Len())

	// probe.Next() has internal limit of metaMap.Len() iterations
	result := probe.Next(metaMap)

	switch result.Type {
	case ProbeResultEmpty:
		// Found an empty bucket - allocate it
		bucket := BucketIndex(result.BucketIndex)
		metaMap.SetFull(int(result.BucketIndex), hash)
		return bucket, nil

	case ProbeResultExhausted:
		// Hash table is too full
		return InvalidBucketIndex, BucketExhaustion{}

	case ProbeResultPossibleHit:
		// This shouldn't happen for a new PageId - table corrupted or collision
		return InvalidBucketIndex, fmt.Errorf("unexpected collision during allocation")

	case ProbeResultTombstone:
		// Tombstones should be skipped by probe.Next()
		return InvalidBucketIndex, fmt.Errorf("unexpected tombstone from probe")

	default:
		return InvalidBucketIndex, fmt.Errorf("unknown probe result type")
	}
}

// FreeBucket marks a bucket as a tombstone (deleted, can be reused).
func FreeBucket(bucket BucketIndex, metaMap *MetaMap) error {
	if !bucket.IsValid() {
		return fmt.Errorf("invalid bucket index")
	}

	metaMap.SetTombstone(int(bucket))
	return nil
}
