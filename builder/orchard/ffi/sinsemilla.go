package ffi

/*
#cgo LDFLAGS: -L${SRCDIR} -lorchard_ffi
#cgo darwin LDFLAGS: -Wl,-rpath,${SRCDIR} -Wl,-rpath,${SRCDIR}/target/release/deps
#include "orchard_ffi.h"
#include <stdlib.h>
*/
import "C"
import (
	"errors"
	"unsafe"
)

const treeDepth = 32

// Error codes from FFI
const (
	FFI_SUCCESS               = 0
	FFI_ERROR_NULL_POINTER   = -1
	FFI_ERROR_INVALID_INPUT  = -2
	FFI_ERROR_COMPUTATION_FAILED = -3
)

// SinsemillaMerkleTree maintains Orchard commitment tree state
type SinsemillaMerkleTree struct {
	root     [32]byte
	size     uint64
	frontier [][32]byte
}

// NewSinsemillaMerkleTree creates a new empty tree
func NewSinsemillaMerkleTree() *SinsemillaMerkleTree {
	tree := &SinsemillaMerkleTree{
		root:     [32]byte{},
		size:     0,
		frontier: make([][32]byte, 0),
	}
	_ = tree.ComputeRoot(nil)
	return tree
}

// ComputeRoot computes Merkle root from leaves using Sinsemilla
func (t *SinsemillaMerkleTree) ComputeRoot(leaves [][32]byte) error {
	var leavesFlat []byte
	if len(leaves) > 0 {
		leavesFlat = make([]byte, len(leaves)*32)
		for i, leaf := range leaves {
			copy(leavesFlat[i*32:(i+1)*32], leaf[:])
		}
	}

	var rootOut [32]byte
	frontierOut := make([]byte, treeDepth*32)
	var frontierLenOut C.size_t

	var leavesPtr *C.uchar
	if len(leavesFlat) > 0 {
		leavesPtr = (*C.uchar)(unsafe.Pointer(&leavesFlat[0]))
	}

	result := C.sinsemilla_compute_root_and_frontier(
		leavesPtr,
		C.size_t(len(leaves)),
		(*C.uchar)(unsafe.Pointer(&rootOut[0])),
		(*C.uchar)(unsafe.Pointer(&frontierOut[0])),
		&frontierLenOut,
	)

	if result != FFI_SUCCESS {
		return ffiErrorToGo(result)
	}

	t.root = rootOut
	t.size = uint64(len(leaves))
	t.frontier = decodeFrontier(frontierOut, int(frontierLenOut))
	return nil
}

// AppendWithFrontier efficiently appends new leaves using frontier
func (t *SinsemillaMerkleTree) AppendWithFrontier(newLeaves [][32]byte) error {
	if len(newLeaves) == 0 {
		return nil
	}

	// Flatten frontier for C call
	var frontierFlat []byte
	if len(t.frontier) > 0 {
		frontierFlat = make([]byte, len(t.frontier)*32)
		for i, f := range t.frontier {
			copy(frontierFlat[i*32:(i+1)*32], f[:])
		}
	}

	// Flatten new leaves for C call
	newLeavesFlat := make([]byte, len(newLeaves)*32)
	for i, leaf := range newLeaves {
		copy(newLeavesFlat[i*32:(i+1)*32], leaf[:])
	}

	var newRootOut [32]byte
	newFrontierOut := make([]byte, treeDepth*32)
	var newFrontierLenOut C.size_t

	var frontierPtr *C.uchar
	if len(frontierFlat) > 0 {
		frontierPtr = (*C.uchar)(unsafe.Pointer(&frontierFlat[0]))
	}

	result := C.sinsemilla_append_with_frontier_update(
		(*C.uchar)(unsafe.Pointer(&t.root[0])),
		C.uint64_t(t.size),
		frontierPtr,
		C.size_t(len(t.frontier)),
		(*C.uchar)(unsafe.Pointer(&newLeavesFlat[0])),
		C.size_t(len(newLeaves)),
		(*C.uchar)(unsafe.Pointer(&newRootOut[0])),
		(*C.uchar)(unsafe.Pointer(&newFrontierOut[0])),
		&newFrontierLenOut,
	)

	if result != FFI_SUCCESS {
		return ffiErrorToGo(result)
	}

	t.root = newRootOut
	t.size += uint64(len(newLeaves))
	t.frontier = decodeFrontier(newFrontierOut, int(newFrontierLenOut))

	return nil
}

// GenerateProof generates Merkle branch proof for commitment at position
func (t *SinsemillaMerkleTree) GenerateProof(commitment [32]byte, position uint64, allCommitments [][32]byte) ([][32]byte, error) {
	if position >= uint64(len(allCommitments)) {
		return nil, errors.New("position out of bounds")
	}

	// Flatten all commitments for C call
	commitmentsFlat := make([]byte, len(allCommitments)*32)
	for i, c := range allCommitments {
		copy(commitmentsFlat[i*32:(i+1)*32], c[:])
	}

	// Allocate max proof size (tree depth = 32)
	maxProofSize := treeDepth
	proofOut := make([]byte, maxProofSize*32)
	var proofLenOut C.size_t

	result := C.sinsemilla_generate_proof(
		(*C.uchar)(unsafe.Pointer(&commitment[0])),
		C.uint64_t(position),
		(*C.uchar)(unsafe.Pointer(&commitmentsFlat[0])),
		C.size_t(len(allCommitments)),
		(*C.uchar)(unsafe.Pointer(&proofOut[0])),
		&proofLenOut,
	)

	if result != FFI_SUCCESS {
		return nil, ffiErrorToGo(result)
	}

	// Convert flat proof back to [][32]byte
	proofLen := int(proofLenOut)
	proof := make([][32]byte, proofLen)
	for i := 0; i < proofLen; i++ {
		copy(proof[i][:], proofOut[i*32:(i+1)*32])
	}

	return proof, nil
}

func decodeFrontier(flat []byte, length int) [][32]byte {
	if length == 0 {
		return nil
	}
	out := make([][32]byte, length)
	for i := 0; i < length; i++ {
		copy(out[i][:], flat[i*32:(i+1)*32])
	}
	return out
}

// GetRoot returns the current tree root
func (t *SinsemillaMerkleTree) GetRoot() [32]byte {
	return t.root
}

// GetSize returns the current tree size (number of leaves)
func (t *SinsemillaMerkleTree) GetSize() uint64 {
	return t.size
}

// GetFrontier returns the current frontier (for efficient updates)
func (t *SinsemillaMerkleTree) GetFrontier() [][32]byte {
	return t.frontier
}

// SetFrontier sets the frontier (used during state sync)
func (t *SinsemillaMerkleTree) SetFrontier(frontier [][32]byte) {
	t.frontier = make([][32]byte, len(frontier))
	copy(t.frontier, frontier)
}

// RestoreState restores the tree root/size/frontier from a checkpoint.
func (t *SinsemillaMerkleTree) RestoreState(root [32]byte, size uint64, frontier [][32]byte) {
	t.root = root
	t.size = size
	t.SetFrontier(frontier)
}

// ffiErrorToGo converts FFI error codes to Go errors
func ffiErrorToGo(code C.int) error {
	switch code {
	case FFI_ERROR_NULL_POINTER:
		return errors.New("FFI null pointer error")
	case FFI_ERROR_INVALID_INPUT:
		return errors.New("FFI invalid input error")
	case FFI_ERROR_COMPUTATION_FAILED:
		return errors.New("FFI computation failed")
	default:
		return errors.New("unknown FFI error")
	}
}
