package bitbox

const (
	// MetaEmpty indicates an empty bucket
	MetaEmpty uint8 = 0x00
	// MetaTombstone indicates a deleted bucket that can be reused
	MetaTombstone uint8 = 0x7F
	// MetaFullMask is the mask for the full bit (MSB)
	MetaFullMask uint8 = 0x80
)

// fullEntry creates a metadata byte for a full bucket.
// It sets the MSB and stores the top 7 bits of the hash for quick filtering.
func fullEntry(hash uint64) uint8 {
	return uint8(hash>>57) ^ MetaFullMask
}

// MetaMap is an in-memory metadata map for hash table buckets.
// Each bucket has 1 byte of metadata:
// - 0x00: empty
// - 0x7F: tombstone (deleted, can reuse)
// - 0x80 | (hash>>57): full, with top 7 bits of hash
type MetaMap struct {
	buckets int
	bitvec  []byte // len must be multiple of 4096 (page size)
}

// NewMetaMap creates a new MetaMap with the given number of buckets.
func NewMetaMap(buckets int) *MetaMap {
	// Round up to page boundary (4096 bytes)
	size := ((buckets + 4095) / 4096) * 4096
	return &MetaMap{
		buckets: buckets,
		bitvec:  make([]byte, size),
	}
}

// FromBytes creates a MetaMap from existing bytes.
func FromBytes(metaBytes []byte, buckets int) *MetaMap {
	if len(metaBytes)%4096 != 0 {
		panic("meta_map bytes must be multiple of 4096")
	}
	return &MetaMap{
		buckets: buckets,
		bitvec:  metaBytes,
	}
}

// Len returns the number of buckets.
func (m *MetaMap) Len() int {
	return m.buckets
}

// FullCount returns the number of full buckets.
func (m *MetaMap) FullCount() int {
	count := 0
	for _, b := range m.bitvec[:m.buckets] {
		if b&MetaFullMask != 0 {
			count++
		}
	}
	return count
}

// SetFull marks a bucket as full with the given hash.
func (m *MetaMap) SetFull(bucket int, hash uint64) {
	m.bitvec[bucket] = fullEntry(hash)
}

// SetTombstone marks a bucket as a tombstone (deleted).
func (m *MetaMap) SetTombstone(bucket int) {
	m.bitvec[bucket] = MetaTombstone
}

// SetEmpty marks a bucket as empty.
func (m *MetaMap) SetEmpty(bucket int) {
	m.bitvec[bucket] = MetaEmpty
}

// HintEmpty returns true if the bucket is definitely empty.
func (m *MetaMap) HintEmpty(bucket int) bool {
	return m.bitvec[bucket] == MetaEmpty
}

// HintTombstone returns true if the bucket is definitely a tombstone.
func (m *MetaMap) HintTombstone(bucket int) bool {
	return m.bitvec[bucket] == MetaTombstone
}

// HintNotMatch returns true if the bucket definitely does not match the hash.
// This is a fast filter using the top 7 bits of the hash.
func (m *MetaMap) HintNotMatch(bucket int, hash uint64) bool {
	return m.bitvec[bucket] != fullEntry(hash)
}

// PageIndex returns the page index of a bucket in the meta-map.
func (m *MetaMap) PageIndex(bucket int) int {
	return bucket / 4096
}

// PageSlice returns a page-sized slice (4096 bytes) of the metamap.
func (m *MetaMap) PageSlice(pageIndex int) []byte {
	start := pageIndex * 4096
	end := start + 4096
	return m.bitvec[start:end]
}

// Bytes returns the full metadata byte slice.
func (m *MetaMap) Bytes() []byte {
	return m.bitvec
}
