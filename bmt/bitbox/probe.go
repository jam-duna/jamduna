package bitbox

// ProbeResult represents the result of a single probe step.
type ProbeResult struct {
	Type       ProbeResultType
	BucketIndex uint64
}

// ProbeResultType indicates what was found at a bucket.
type ProbeResultType int

const (
	// ProbeResultPossibleHit means the bucket might contain our page (need to verify)
	ProbeResultPossibleHit ProbeResultType = iota
	// ProbeResultEmpty means the bucket is empty (page definitely not in table)
	ProbeResultEmpty
	// ProbeResultTombstone means the bucket is deleted (continue probing)
	ProbeResultTombstone
	// ProbeResultExhausted means all buckets were probed without finding empty slot
	ProbeResultExhausted
)

// ProbeSequence implements triangular probing for hash table lookups.
// Triangular probing gives the sequence: hash, hash+1, hash+3, hash+6, hash+10, ...
// where the step increases by 1 each time (0, 1, 2, 3, ...).
type ProbeSequence struct {
	hash   uint64
	bucket uint64
	step   uint64
}

// NewProbeSequence creates a new probe sequence for the given hash and meta-map.
func NewProbeSequence(hash uint64, metaMapLen int) *ProbeSequence {
	return &ProbeSequence{
		hash:   hash,
		bucket: hash % uint64(metaMapLen),
		step:   0,
	}
}

// Next advances the probe sequence and returns the next bucket to check.
// This implements triangular probing: bucket[i+1] = (bucket[i] + step) % len.
// Returns Empty when an empty bucket is found, or after probing all buckets.
func (p *ProbeSequence) Next(metaMap *MetaMap) ProbeResult {
	maxProbes := metaMap.Len()
	probeCount := 0

	for probeCount < maxProbes {
		// Triangular probing
		p.bucket = (p.bucket + p.step) % uint64(metaMap.Len())
		p.step++
		probeCount++

		// Check what's in this bucket
		if metaMap.HintEmpty(int(p.bucket)) {
			return ProbeResult{
				Type:        ProbeResultEmpty,
				BucketIndex: p.bucket,
			}
		}

		if metaMap.HintTombstone(int(p.bucket)) {
			// Skip tombstones and continue probing
			continue
		}

		if metaMap.HintNotMatch(int(p.bucket), p.hash) {
			// This bucket is full but hash doesn't match - continue probing
			continue
		}

		// Possible hit - need to verify by reading actual page
		return ProbeResult{
			Type:        ProbeResultPossibleHit,
			BucketIndex: p.bucket,
		}
	}

	// Probed all buckets without finding empty or match - return exhausted
	return ProbeResult{
		Type:        ProbeResultExhausted,
		BucketIndex: p.bucket,
	}
}
