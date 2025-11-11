package trie

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

// ServiceProof encapsulates the complete proof chain for service data
// Two-layer verification:
//
//	(1) MMR proof verifies receipt inclusion against MMR super_peak
//	(2) BMT proof verifies MMR root is in JAM State at given state root
type ServiceProof struct {
	ServiceID  uint32 // Service ID
	StorageKey []byte // Storage key where MMR root is stored in JAM State

	// Layer 1: MMR proof (receipt → MMR root)
	MmrProof  MMRProof    // MMR inclusion proof for receipt
	SuperPeak common.Hash // MMR super_peak root (computed from peaks)

	// Layer 2: BMT proof (MMR root → JAM State)
	BmtProof  []common.Hash // Binary Merkle Trie proof path
	StateRoot common.Hash   // JAM State root
}

// Verify performs two-layer verification:
// (1) MMR proof verification: receipt is in MMR at SuperPeak
// (2) BMT proof verification: SuperPeak is in JAM State at StateRoot
func (p *ServiceProof) Verify() bool {
	// Layer 1: Verify MMR proof against super peak
	if !p.MmrProof.Verify(p.SuperPeak) {
		return false
	}

	// Layer 2: Verify BMT proof using the storage key from the proof
	mmrRootBytes := p.SuperPeak.Bytes()

	return Verify(p.ServiceID, p.StorageKey, mmrRootBytes, p.StateRoot.Bytes(), p.BmtProof)
}

// GenerateServiceProof generates a complete two-layer proof for service data
// Returns a ServiceProof that can be independently verified
func GenerateServiceProof(mmr *MMR, serviceID uint32, storageKey []byte, position uint64, leafHash common.Hash, store SubtreeStore, bmtProof [][]byte, stateRoot common.Hash) (*ServiceProof, error) {
	// Convert BMT proof to []common.Hash
	bmtPath := make([]common.Hash, len(bmtProof))
	for i, p := range bmtProof {
		bmtPath[i] = common.BytesToHash(p)
	}

	// Generate MMR proof using the subtree store and mmr.LeafCount()
	mmrProof, err := mmr.GenerateProof(position, leafHash, store, mmr.LeafCount())
	if err != nil {
		return nil, fmt.Errorf("failed to generate MMR proof: %w", err)
	}

	superPeak := mmr.SuperPeak()
	if superPeak == nil {
		return nil, fmt.Errorf("failed to compute MMR super peak")
	}

	proof := &ServiceProof{
		ServiceID:  serviceID,
		StorageKey: storageKey,

		MmrProof:  mmrProof,   // this holds the receipt inclusion proof
		SuperPeak: *superPeak, // this is what EVM service commits to in one or more accumulates

		BmtProof:  bmtPath,   // this is the proof that the mmr is a particular value in JAM State
		StateRoot: stateRoot, // this is what Grandpa finalizes
	}

	// Verify the proof before returning
	if !VerifyServiceProof(proof) {
		return nil, fmt.Errorf("proof verification failed")
	}

	return proof, nil
}

// VerifyServiceProof performs two-layer verification of a ServiceProof
func VerifyServiceProof(proof *ServiceProof) bool {
	// (1) MMR proof verification: receipt is in MMR at SuperPeak
	if !proof.MmrProof.Verify(proof.SuperPeak) {
		return false
	}

	// (2) BMT proof verification: SuperPeak is in JAM State at StateRoot
	mmrRootBytes := proof.SuperPeak.Bytes()

	return Verify(proof.ServiceID, proof.StorageKey, mmrRootBytes, proof.StateRoot.Bytes(), proof.BmtProof)
}
