package core

// update.go provides trie update operations.
//
// For GP (Gray Paper) mode, the tree building functionality is implemented
// in gp_tree.go via BuildGpTree(). This file provides a common interface
// and utilities for trie updates.
//
// The full generic build_trie() algorithm from Rust NOMT is deferred as
// gp_tree.go covers all JAM-specific needs.

import (
	"bytes"
	"sort"
)

// Operation represents a key-value operation (insert or delete).
type Operation struct {
	Key   KeyPath
	Value []byte // nil for delete
}

// SortOperations sorts operations by key (lexicographic order).
func SortOperations(ops []Operation) {
	sort.Slice(ops, func(i, j int) bool {
		return bytes.Compare(ops[i].Key[:], ops[j].Key[:]) < 0
	})
}

// SharedBits returns the number of shared prefix bits between two keys.
// This uses MSB-first bit ordering (matching TriePosition).
func SharedBits(a, b KeyPath) int {
	shared := 0
	for i := 0; i < len(a) && i < len(b); i++ {
		xor := a[i] ^ b[i]
		if xor == 0 {
			shared += 8
			continue
		}

		// Count leading zeros in XOR to find first differing bit
		for bit := 7; bit >= 0; bit-- {
			if (xor & (1 << bit)) != 0 {
				return shared
			}
			shared++
		}
		break
	}
	return shared
}

// IsDelete returns true if this operation is a delete.
func (op *Operation) IsDelete() bool {
	return op.Value == nil
}

// IsInsert returns true if this operation is an insert.
func (op *Operation) IsInsert() bool {
	return op.Value != nil
}
