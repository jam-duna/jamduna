package ops

import (
	"testing"

	"github.com/colorfulnotion/jam/bmt/beatree"
)

func TestPrefixLen(t *testing.T) {
	tests := []struct {
		name     string
		a        beatree.Key
		b        beatree.Key
		expected int
	}{
		{
			name:     "identical keys",
			a:        beatree.Key{0xFF, 0xFF, 0xFF, 0xFF},
			b:        beatree.Key{0xFF, 0xFF, 0xFF, 0xFF},
			expected: 256,
		},
		{
			name:     "differ at first bit",
			a:        beatree.Key{0x00},
			b:        beatree.Key{0x80},
			expected: 0,
		},
		{
			name:     "differ at second bit",
			a:        beatree.Key{0x80},
			b:        beatree.Key{0xC0},
			expected: 1,
		},
		{
			name:     "differ at byte boundary",
			a:        beatree.Key{0xFF, 0x00},
			b:        beatree.Key{0xFF, 0x80},
			expected: 8,
		},
		{
			name:     "differ midway through second byte",
			a:        beatree.Key{0xFF, 0xF0},
			b:        beatree.Key{0xFF, 0xF8},
			expected: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrefixLen(&tt.a, &tt.b)
			if result != tt.expected {
				t.Errorf("PrefixLen() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestSeparate(t *testing.T) {
	tests := []struct {
		name     string
		a        beatree.Key
		b        beatree.Key
		expected beatree.Key
	}{
		{
			name:     "simple case - differ at first bit",
			a:        beatree.Key{0x00},
			b:        beatree.Key{0x80},
			expected: beatree.Key{0x80},
		},
		{
			name:     "differ at second bit",
			a:        beatree.Key{0x80},
			b:        beatree.Key{0xC0},
			expected: beatree.Key{0xC0},
		},
		{
			name:     "differ at byte boundary",
			a:        beatree.Key{0xFF, 0x00},
			b:        beatree.Key{0xFF, 0x80},
			expected: beatree.Key{0xFF, 0x80},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Separate(&tt.a, &tt.b)
			if result != tt.expected {
				t.Errorf("Separate() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSeparatorLen(t *testing.T) {
	tests := []struct {
		name     string
		key      beatree.Key
		expected int
	}{
		{
			name:     "all zeros",
			key:      beatree.Key{},
			expected: 1,
		},
		{
			name:     "single bit set at start",
			key:      beatree.Key{0x80},
			expected: 1,
		},
		{
			name:     "two bits set",
			key:      beatree.Key{0xC0},
			expected: 2,
		},
		{
			name:     "full first byte",
			key:      beatree.Key{0xFF},
			expected: 8,
		},
		{
			name:     "extends to second byte",
			key:      beatree.Key{0xFF, 0x80},
			expected: 9,
		},
		{
			name:     "all bits set",
			key:      func() beatree.Key { var k beatree.Key; for i := range k { k[i] = 0xFF }; return k }(),
			expected: 256,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SeparatorLen(&tt.key)
			if result != tt.expected {
				t.Errorf("SeparatorLen() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestReconstructKey(t *testing.T) {
	tests := []struct {
		name      string
		prefix    *RawPrefix
		separator RawSeparator
		expected  beatree.Key
	}{
		{
			name:   "no prefix, aligned separator",
			prefix: nil,
			separator: RawSeparator{
				Bytes:    []byte{0xAA, 0xBB, 0xCC, 0xDD, 0x00, 0x00, 0x00, 0x00},
				BitStart: 0,
				BitLen:   32,
			},
			expected: beatree.Key{0xAA, 0xBB, 0xCC, 0xDD},
		},
		{
			name: "byte-aligned prefix and separator",
			prefix: &RawPrefix{
				Bytes:  []byte{0xFF, 0xFF},
				BitLen: 16,
			},
			separator: RawSeparator{
				Bytes:    []byte{0xAA, 0xBB, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
				BitStart: 0,
				BitLen:   16,
			},
			expected: beatree.Key{0xFF, 0xFF, 0xAA, 0xBB},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReconstructKey(tt.prefix, tt.separator)
			if result != tt.expected {
				t.Errorf("ReconstructKey() = %v, want %v", result[:8], tt.expected[:8])
			}
		})
	}
}

func TestBitwiseMemcpy(t *testing.T) {
	t.Run("simple aligned copy", func(t *testing.T) {
		dst := make([]byte, 8)
		src := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0x00, 0x00, 0x00, 0x00}

		BitwiseMemcpy(dst, 0, src, 0, 32)

		expected := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0x00, 0x00, 0x00, 0x00}
		for i := range expected {
			if dst[i] != expected[i] {
				t.Errorf("BitwiseMemcpy() dst[%d] = 0x%02X, want 0x%02X", i, dst[i], expected[i])
			}
		}
	})

	t.Run("preserve destination bits outside range", func(t *testing.T) {
		dst := []byte{0xFF, 0xFF, 0xFF, 0xFF}
		src := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

		// Copy only 8 bits (1 byte)
		BitwiseMemcpy(dst, 0, src, 0, 8)

		expected := []byte{0x00, 0xFF, 0xFF, 0xFF}
		for i := range expected {
			if dst[i] != expected[i] {
				t.Errorf("BitwiseMemcpy() dst[%d] = 0x%02X, want 0x%02X", i, dst[i], expected[i])
			}
		}
	})
}
