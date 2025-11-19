package bitbox

import (
	"testing"
)

func TestMetaMapEmpty(t *testing.T) {
	m := NewMetaMap(1000)

	// All buckets should start empty
	for i := 0; i < 1000; i++ {
		if !m.HintEmpty(i) {
			t.Errorf("Bucket %d should be empty initially", i)
		}
	}

	// Full count should be 0
	if count := m.FullCount(); count != 0 {
		t.Errorf("FullCount should be 0, got %d", count)
	}

	// Test SetEmpty explicitly
	m.SetFull(10, 0x123456789ABCDEF0)
	if m.HintEmpty(10) {
		t.Errorf("Bucket 10 should not be empty after SetFull")
	}

	m.SetEmpty(10)
	if !m.HintEmpty(10) {
		t.Errorf("Bucket 10 should be empty after SetEmpty")
	}
}

func TestMetaMapTombstone(t *testing.T) {
	m := NewMetaMap(1000)

	// Mark some buckets as tombstones
	m.SetTombstone(5)
	m.SetTombstone(100)
	m.SetTombstone(999)

	// Check tombstone detection
	if !m.HintTombstone(5) {
		t.Errorf("Bucket 5 should be a tombstone")
	}
	if !m.HintTombstone(100) {
		t.Errorf("Bucket 100 should be a tombstone")
	}
	if !m.HintTombstone(999) {
		t.Errorf("Bucket 999 should be a tombstone")
	}

	// Other buckets should not be tombstones
	if m.HintTombstone(6) {
		t.Errorf("Bucket 6 should not be a tombstone")
	}

	// Tombstones should not be empty
	if m.HintEmpty(5) {
		t.Errorf("Tombstone bucket should not be empty")
	}

	// Tombstones should not count as full
	if count := m.FullCount(); count != 0 {
		t.Errorf("Tombstones should not count as full, got %d", count)
	}
}

func TestMetaMapFull(t *testing.T) {
	m := NewMetaMap(1000)

	// Mark some buckets as full with different hashes
	hashes := []uint64{
		0x1234567890ABCDEF,
		0xFEDCBA0987654321,
		0x0000000000000001,
		0xFFFFFFFFFFFFFFFF,
		0x8000000000000000,
	}

	for i, hash := range hashes {
		m.SetFull(i, hash)
	}

	// Check full count
	if count := m.FullCount(); count != len(hashes) {
		t.Errorf("FullCount should be %d, got %d", len(hashes), count)
	}

	// Check that buckets are not empty
	for i := 0; i < len(hashes); i++ {
		if m.HintEmpty(i) {
			t.Errorf("Bucket %d should not be empty", i)
		}
		if m.HintTombstone(i) {
			t.Errorf("Bucket %d should not be a tombstone", i)
		}
	}

	// Verify the full entry format includes the MSB
	for i, hash := range hashes {
		expected := fullEntry(hash)
		if m.Bytes()[i] != expected {
			t.Errorf("Bucket %d should have metadata 0x%02x, got 0x%02x", i, expected, m.Bytes()[i])
		}
		// MSB should be set
		if m.Bytes()[i]&MetaFullMask == 0 {
			t.Errorf("Bucket %d should have MSB set", i)
		}
	}
}

func TestMetaMapHints(t *testing.T) {
	m := NewMetaMap(1000)

	// Use a specific hash
	hash := uint64(0x8FABCDEF12345678)
	m.SetFull(42, hash)

	// Same hash should not hint "not match"
	if m.HintNotMatch(42, hash) {
		t.Errorf("Same hash should not hint 'not match'")
	}

	// Different hashes should (usually) hint "not match"
	// The top 7 bits must differ for a guaranteed mismatch
	differentHash := hash ^ (0x7F << 57) // Flip top 7 bits
	if !m.HintNotMatch(42, differentHash) {
		t.Errorf("Different hash (top 7 bits differ) should hint 'not match'")
	}

	// Test the hash filter mechanism
	// fullEntry stores top 7 bits of hash XOR'd with 0x80
	// So two hashes with same top 7 bits will NOT hint mismatch
	sameTop7Bits := hash & ^(uint64(0x7F) << 57) // Clear top 7 bits
	sameTop7Bits |= (hash & (uint64(0x7F) << 57)) // Set same top 7 bits
	if m.HintNotMatch(42, sameTop7Bits) {
		t.Errorf("Hash with same top 7 bits should not hint 'not match'")
	}

	// Empty bucket WILL hint "not match" because metadata is 0x00, not fullEntry(hash)
	if !m.HintNotMatch(0, hash) {
		t.Errorf("Empty bucket should hint 'not match' (metadata doesn't match fullEntry)")
	}

	// Tombstone WILL hint "not match" because metadata is 0x7F, not fullEntry(hash)
	m.SetTombstone(10)
	if !m.HintNotMatch(10, hash) {
		t.Errorf("Tombstone should hint 'not match' (metadata doesn't match fullEntry)")
	}
}

func TestMetaMapPageSlice(t *testing.T) {
	m := NewMetaMap(10000) // Multiple pages

	// Mark some buckets
	m.SetFull(0, 0x1111111111111111)
	m.SetFull(4095, 0x2222222222222222)
	m.SetFull(4096, 0x3333333333333333)
	m.SetFull(8191, 0x4444444444444444)

	// Get page slices
	page0 := m.PageSlice(0)
	page1 := m.PageSlice(1)

	if len(page0) != 4096 {
		t.Errorf("Page slice should be 4096 bytes, got %d", len(page0))
	}
	if len(page1) != 4096 {
		t.Errorf("Page slice should be 4096 bytes, got %d", len(page1))
	}

	// Verify contents
	if page0[0] != fullEntry(0x1111111111111111) {
		t.Errorf("Page 0, bucket 0 incorrect")
	}
	if page0[4095] != fullEntry(0x2222222222222222) {
		t.Errorf("Page 0, bucket 4095 incorrect")
	}
	if page1[0] != fullEntry(0x3333333333333333) {
		t.Errorf("Page 1, bucket 4096 incorrect")
	}
	if page1[4095] != fullEntry(0x4444444444444444) {
		t.Errorf("Page 1, bucket 8191 incorrect")
	}
}

func TestMetaMapFromBytes(t *testing.T) {
	// Create a meta map and populate it
	m1 := NewMetaMap(1000)
	m1.SetFull(0, 0xAAAAAAAAAAAAAAAA)
	m1.SetTombstone(100)
	m1.SetFull(999, 0xBBBBBBBBBBBBBBBB)

	// Get the bytes
	bytes := m1.Bytes()

	// Create a new meta map from those bytes
	m2 := FromBytes(bytes, 1000)

	// Should have same content
	if !m2.HintNotMatch(0, 0xAAAAAAAAAAAAAAAA) == !m1.HintNotMatch(0, 0xAAAAAAAAAAAAAAAA) {
		// Double negation checks they're equal
	} else {
		t.Errorf("Bucket 0 mismatch")
	}

	if !m2.HintTombstone(100) {
		t.Errorf("Bucket 100 should be tombstone")
	}

	if m2.HintNotMatch(999, 0xBBBBBBBBBBBBBBBB) {
		t.Errorf("Bucket 999 should match hash")
	}

	if m2.FullCount() != m1.FullCount() {
		t.Errorf("Full count mismatch: %d vs %d", m2.FullCount(), m1.FullCount())
	}
}

func TestMetaMapLen(t *testing.T) {
	m := NewMetaMap(5000)
	if m.Len() != 5000 {
		t.Errorf("Len should be 5000, got %d", m.Len())
	}

	// Bytes should be rounded up to page boundary
	expectedSize := ((5000 + 4095) / 4096) * 4096
	if len(m.Bytes()) != expectedSize {
		t.Errorf("Bytes size should be %d, got %d", expectedSize, len(m.Bytes()))
	}
}
