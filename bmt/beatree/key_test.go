package beatree

import (
	"testing"
)

func TestKeyCompare(t *testing.T) {
	key1 := Key{0x00, 0x00, 0x00, 0x01}
	key2 := Key{0x00, 0x00, 0x00, 0x02}
	key3 := Key{0x00, 0x00, 0x00, 0x01}

	if key1.Compare(key2) >= 0 {
		t.Error("key1 should be less than key2")
	}

	if key2.Compare(key1) <= 0 {
		t.Error("key2 should be greater than key1")
	}

	if key1.Compare(key3) != 0 {
		t.Error("key1 should equal key3")
	}
}

func TestKeyEqual(t *testing.T) {
	key1 := Key{0x01, 0x02, 0x03}
	key2 := Key{0x01, 0x02, 0x03}
	key3 := Key{0x01, 0x02, 0x04}

	if !key1.Equal(key2) {
		t.Error("key1 should equal key2")
	}

	if key1.Equal(key3) {
		t.Error("key1 should not equal key3")
	}
}

func TestKeyFromBytes(t *testing.T) {
	bytes := make([]byte, 32)
	for i := range bytes {
		bytes[i] = byte(i)
	}

	key := KeyFromBytes(bytes)
	for i := range bytes {
		if key[i] != bytes[i] {
			t.Errorf("key[%d] = %d, want %d", i, key[i], bytes[i])
		}
	}
}

func TestKeyFromBytesPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("KeyFromBytes should panic with invalid length")
		}
	}()

	KeyFromBytes([]byte{1, 2, 3}) // Should panic
}
