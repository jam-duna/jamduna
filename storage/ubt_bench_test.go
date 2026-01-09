package storage

import (
	"encoding/binary"
	"testing"
)

const (
	benchStemCount     = 4096
	benchValuesPerStem = 1
)

func benchStemFromIndex(i int) Stem {
	var stem Stem
	binary.LittleEndian.PutUint32(stem[:4], uint32(i))
	binary.BigEndian.PutUint32(stem[27:], uint32(i))
	return stem
}

func benchKeysValues(stems, valuesPerStem int) ([]TreeKey, [][32]byte) {
	keys := make([]TreeKey, 0, stems*valuesPerStem)
	values := make([][32]byte, 0, stems*valuesPerStem)
	for i := 0; i < stems; i++ {
		stem := benchStemFromIndex(i)
		for j := 0; j < valuesPerStem; j++ {
			key := TreeKey{Stem: stem, Subindex: uint8(j)}
			var value [32]byte
			value[0] = byte((i + j) % 255)
			if value[0] == 0 {
				value[0] = 1
			}
			keys = append(keys, key)
			values = append(values, value)
		}
	}
	return keys, values
}

func buildBenchTree(stems, valuesPerStem int, incremental bool) (*UnifiedBinaryTree, []TreeKey) {
	keys, values := benchKeysValues(stems, valuesPerStem)
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile, Capacity: stems, Incremental: incremental})
	for i, key := range keys {
		tree.Insert(key, values[i])
	}
	return tree, keys
}

func BenchmarkUBTInsert(b *testing.B) {
	keys, values := benchKeysValues(benchStemCount, benchValuesPerStem)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile, Capacity: benchStemCount})
		for j, key := range keys {
			tree.Insert(key, values[j])
		}
	}
}

func BenchmarkUBTRootHashFullRebuild(b *testing.B) {
	tree, _ := buildBenchTree(benchStemCount, benchValuesPerStem, false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tree.RootHash()
	}
}

func BenchmarkUBTRootHashIncremental(b *testing.B) {
	tree, keys := buildBenchTree(benchStemCount, benchValuesPerStem, true)
	_ = tree.RootHash()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i % len(keys)
		var value [32]byte
		if i%2 == 0 {
			value[0] = 1
		} else {
			value[0] = 2
		}
		tree.Insert(keys[idx], value)
		_ = tree.RootHash()
	}
}

func BenchmarkUBTRootHashParallelThreshold(b *testing.B) {
	b.Run("threshold_off", func(b *testing.B) {
		tree, _ := buildBenchTree(benchStemCount, benchValuesPerStem, false)
		tree.SetParallelThreshold(0)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = tree.RootHash()
		}
	})

	b.Run("threshold_128", func(b *testing.B) {
		tree, _ := buildBenchTree(benchStemCount, benchValuesPerStem, false)
		tree.SetParallelThreshold(128)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = tree.RootHash()
		}
	})
}
