package verkle

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/ethereum/go-verkle"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

// ProofService provides verkle proof generation with caching
type ProofService struct {
	checkpointMgr *CheckpointTreeManager
	proofCache    *lru.LRU[string, []byte] // cacheKey → proof
}

// NewProofService creates a new proof service
func NewProofService(
	checkpointMgr *CheckpointTreeManager,
	cacheSize int,
	cacheTTL time.Duration,
) *ProofService {
	return &ProofService{
		checkpointMgr: checkpointMgr,
		proofCache:    lru.NewLRU[string, []byte](cacheSize, nil, cacheTTL),
	}
}

// GenerateProof creates verkle proof for keys at specific block height
func (ps *ProofService) GenerateProof(
	height uint64,
	keys [][]byte,
) ([]byte, error) {
	// 1. Check proof cache
	cacheKey := ps.computeCacheKey(height, keys)
	if _, ok := ps.proofCache.Get(cacheKey); ok {
		log.Warn(log.SDB, "🚨 PROOF CACHE HIT - IGNORING FOR DEBUG", "height", height, "keys", len(keys))
		// TEMPORARY: Disable cache to ensure fresh proofs for debugging
		// return proof, nil
	}

	// 2. Find nearest checkpoint ≤ height from actual available checkpoints
	cpHeight, cpTree, err := ps.findNearestCheckpoint(height)
	if err != nil {
		return nil, err
	}

	// 3. CRITICAL: Copy checkpoint tree to avoid mutation
	workingTree := cpTree.Copy()

	// 5. Replay deltas from checkpoint to target height (on copy)
	if height > cpHeight {
		for h := cpHeight + 1; h <= height; h++ {
			block, err := ps.checkpointMgr.blockLoader(h)
			if err != nil {
				return nil, fmt.Errorf("failed to load block %d: %w", h, err)
			}

			if block.VerkleStateDelta == nil || block.VerkleStateDelta.NumEntries == 0 {
				return nil, fmt.Errorf("block %d missing verkle delta", h)
			}

			// Convert evmtypes.VerkleStateDelta to VerkleStateDelta
			storageDelta := &VerkleStateDelta{
				NumEntries: block.VerkleStateDelta.NumEntries,
				Entries:    block.VerkleStateDelta.Entries,
			}

			expectedRoot := common.BytesToHash(block.VerkleRoot[:])
			// Use standalone helper instead of dummy StateDBStorage{}
			replayedTree, err := replayVerkleStateDelta(workingTree, storageDelta, expectedRoot)
			if err != nil {
				return nil, fmt.Errorf("delta replay failed at block %d: %w", h, err)
			}

			workingTree = replayedTree
		}
	}

	// 5b. Verify final root matches target height (CRITICAL: prevents proof for wrong state)
	targetBlock, err := ps.checkpointMgr.blockLoader(height)
	if err != nil {
		return nil, fmt.Errorf("failed to load target block %d: %w", height, err)
	}
	expectedFinalRoot := common.BytesToHash(targetBlock.VerkleRoot[:])
	workingTree.Commit()
	workingHashBytes := workingTree.Hash().Bytes()
	actualFinalRoot := common.BytesToHash(workingHashBytes[:])
	if actualFinalRoot != expectedFinalRoot {
		return nil, fmt.Errorf("final root mismatch at height %d: expected %x, got %x",
			height, expectedFinalRoot, actualFinalRoot)
	}

	// 6. Verify all keys exist in working tree (validates proof will be meaningful)
	for _, key := range keys {
		_, err := workingTree.Get(key, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get value for key %x: %w", key, err)
		}
	}

	// 7. Generate proof from working tree (values implicit in tree)
	// Note: MakeVerkleMultiProof API signature is (preroot, postroot, keys, resolver)
	// For read-only proofs, preroot and postroot are the same (workingTree)
	proof, _, _, _, err := verkle.MakeVerkleMultiProof(workingTree, workingTree, keys, nil)
	if err != nil {
		return nil, fmt.Errorf("proof generation failed: %w", err)
	}

	// Serialize proof
	serializedProof, _, err := verkle.SerializeProof(proof)
	if err != nil {
		return nil, fmt.Errorf("proof serialization failed: %w", err)
	}

	// CRITICAL: Prepend root commitment to CommitmentsByPath for Rust verifier
	// - go-verkle's MakeVerkleMultiProof intentionally EXCLUDES the root from CommitmentsByPath
	// - Rust verifier REQUIRES root at commitments_by_path[0] for cryptographic binding
	// - We add it ONCE at generation time, never at verification time
	rootCommitment := workingTree.Commit().Bytes()

	newCommitments := make([][32]byte, 0, len(serializedProof.CommitmentsByPath)+1)
	var rootArray [32]byte
	copy(rootArray[:], rootCommitment[:])
	newCommitments = append(newCommitments, rootArray)
	newCommitments = append(newCommitments, serializedProof.CommitmentsByPath...)

	serializedProof.CommitmentsByPath = newCommitments

	proofBytes, err := marshalCompactProof(serializedProof)
	if err != nil {
		return nil, fmt.Errorf("proof compact marshaling failed: %w", err)
	}

	// 7. Cache proof
	ps.proofCache.Add(cacheKey, proofBytes)

	log.Trace(log.SDB, "Generated proof",
		"height", height,
		"checkpoint", cpHeight,
		"replay_blocks", height-cpHeight,
		"keys", len(keys),
		"proof_size", len(proofBytes))

	return proofBytes, nil
}

// computeCacheKey creates a unique cache key from height and keys
func (ps *ProofService) computeCacheKey(height uint64, keys [][]byte) string {
	hasher := sha256.New()
	binary.Write(hasher, binary.LittleEndian, height)

	// Sort keys for deterministic cache key
	sortedKeys := make([][]byte, len(keys))
	copy(sortedKeys, keys)
	sort.Slice(sortedKeys, func(i, j int) bool {
		return bytes.Compare(sortedKeys[i], sortedKeys[j]) < 0
	})

	for _, key := range sortedKeys {
		hasher.Write(key)
	}

	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// findNearestCheckpoint finds the highest available checkpoint ≤ height
func (ps *ProofService) findNearestCheckpoint(height uint64) (uint64, verkle.VerkleNode, error) {
	// Get all actually available checkpoints
	available := ps.checkpointMgr.ListCheckpoints()
	if len(available) == 0 {
		return 0, nil, fmt.Errorf("no checkpoints available")
	}

	// Find highest checkpoint ≤ height
	var bestHeight uint64
	var bestTree verkle.VerkleNode
	found := false

	for _, cpHeight := range available {
		if cpHeight <= height && cpHeight >= bestHeight {
			// Try to load this checkpoint
			tree, err := ps.checkpointMgr.GetCheckpoint(cpHeight)
			if err != nil {
				// Skip if can't load (shouldn't happen but be defensive)
				log.Warn(log.SDB, "Failed to load checkpoint", "height", cpHeight, "err", err)
				continue
			}
			bestHeight = cpHeight
			bestTree = tree
			found = true
		}
	}

	if !found {
		return 0, nil, fmt.Errorf("no checkpoint available for height %d (available: %v)", height, available)
	}

	return bestHeight, bestTree, nil
}

// GetCacheStats returns proof cache statistics
func (ps *ProofService) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"cache_size": ps.proofCache.Len(),
	}
}

// NOTE: ensureRootInProof() has been removed.
// Root commitment is now added directly at proof generation time (inline)
// in GenerateProof() and GenerateVerkleProofWithTransition().
// This ensures the root is prepended ONCE at generation, never at verification.
