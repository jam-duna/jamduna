package trie

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"

	bls "github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// fibStaticBuffer is the exact static buffer from services/fib/src/main.rs (4104 bytes)
// This is used to deterministically generate fib segments without needing PVM execution
// Loaded lazily from test/fib_static_buffer.hex (hex-encoded for easier inspection)
var (
	fibStaticBuffer     []byte
	fibStaticBufferOnce sync.Once
)

func loadFibStaticBuffer() []byte {
	fibStaticBufferOnce.Do(func() {
		hexData, err := os.ReadFile("test/fib_static_buffer.hex")
		if err != nil {
			panic(fmt.Sprintf("failed to load fib_static_buffer.hex: %v", err))
		}
		fibStaticBuffer = common.Hex2Bytes(string(hexData))
		if len(fibStaticBuffer) != 4104 {
			panic(fmt.Sprintf("fib_static_buffer.hex has wrong size: got %d bytes, expected 4104", len(fibStaticBuffer)))
		}
	})
	return fibStaticBuffer
}

// GenerateFibSegment generates the i-th fib segment deterministically
// Segment 0: the static buffer itself
// Segment i (i >= 1): (buffer[j] + (i+1)) % 256 for each byte j
func GenerateFibSegment(i int) []byte {
	buf := loadFibStaticBuffer()
	segment := make([]byte, types.SegmentSize)
	if i == 0 {
		copy(segment, buf)
	} else {
		// Segment i: (buffer[j] + (i+1)) % 256
		offset := uint32(i + 1)
		for j := 0; j < types.SegmentSize; j++ {
			segment[j] = byte((uint32(buf[j]) + offset) % 256)
		}
	}
	return segment
}

// GenerateFibSegments generates the first numSegments fib segments (0 to numSegments-1)
func GenerateFibSegments(numSegments int) [][]byte {
	segments := make([][]byte, numSegments)
	for i := 0; i < numSegments; i++ {
		segments[i] = GenerateFibSegment(i)
	}
	return segments
}

// TestGenerateFibExportedRoots generates the exported roots for fib segments 0 to maxN
// and saves them to exportedRoots.json
func TestGenerateFibExportedRoots(t *testing.T) {
	maxN := 3070 // Generate roots for 0 to 3072 segments (3073 entries)

	t.Logf("Generating exported roots for 0 to %d segments...\n", maxN)

	// Pre-generate all segments once (since they're deterministic)
	allSegments := GenerateFibSegments(maxN)

	exportedRoots := make([]common.Hash, maxN+1)

	// Root for 0 segments is the zero hash
	exportedRoots[0] = common.Hash{}

	// Generate roots for 1 to maxN segments
	for n := 1; n <= maxN; n++ {
		segs := allSegments[:n]
		tree := NewCDMerkleTree(segs)
		exportedRoots[n] = tree.RootHash()

		if n%256 == 0 || n <= 10 {
			t.Logf("Generated root for %d segments: %s\n", n, exportedRoots[n])
		}
	}

	// Save to JSON file
	jsonData, err := json.MarshalIndent(exportedRoots, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal exported roots: %v", err)
	}

	err = os.WriteFile("test/exportedRoots.json", jsonData, 0644)
	if err != nil {
		t.Fatalf("Failed to write exportedRoots.json: %v", err)
	}

	t.Logf("Successfully saved %d exported roots to test/exportedRoots.json\n", len(exportedRoots))
}

// buildBClubForTest computes b♣ (bundle club) - EC-encoded bundle hashes
// Duplicates statedb.BuildBClub to avoid circular import
func buildBClubForTest(b []byte) ([]common.Hash, [][]byte) {
	paddedB := common.PadToMultipleOfN(b, types.ECPieceSize)
	chunks, err := bls.Encode(paddedB, types.TotalValidators)
	if err != nil {
		panic(fmt.Sprintf("buildBClubForTest: bls.Encode failed: %v", err))
	}

	bClub := make([]common.Hash, types.TotalValidators)
	for i, chunk := range chunks {
		bClub[i] = common.Blake2Hash(chunk)
	}
	return bClub, chunks
}

// buildSClubForTest computes s♣ (segment club) - WBT roots for each shard's segments
// Duplicates statedb.BuildSClub to avoid circular import
func buildSClubForTest(segments [][]byte) ([]common.Hash, [][]byte) {
	if len(segments) == 0 {
		return make([]common.Hash, types.TotalValidators), make([][]byte, types.TotalValidators)
	}

	// EC encode each segment
	encodedSegments := make([][][]byte, len(segments))
	for i, seg := range segments {
		encoded, err := bls.Encode(seg, types.TotalValidators)
		if err != nil {
			panic(fmt.Sprintf("buildSClubForTest: bls.Encode failed for segment %d: %v", i, err))
		}
		encodedSegments[i] = encoded
	}

	// Generate page proofs
	pageProofs, _ := GeneratePageProof(segments)

	// EC encode page proofs
	encodedPageProofs := make([][][]byte, len(pageProofs))
	for i, proof := range pageProofs {
		encoded, err := bls.Encode(proof, types.TotalValidators)
		if err != nil {
			panic(fmt.Sprintf("buildSClubForTest: bls.Encode failed for page proof %d: %v", i, err))
		}
		encodedPageProofs[i] = encoded
	}

	// Build sClub - for each validator, build WBT of their segment shards + page proof shards
	sClub := make([]common.Hash, types.TotalValidators)
	exportedShards := make([][]byte, types.TotalValidators)

	for vIdx := 0; vIdx < types.TotalValidators; vIdx++ {
		// Collect all segment shards for this validator
		var shardData [][]byte
		for segIdx := 0; segIdx < len(segments); segIdx++ {
			shardData = append(shardData, encodedSegments[segIdx][vIdx])
		}
		// Add page proof shards
		for ppIdx := 0; ppIdx < len(pageProofs); ppIdx++ {
			shardData = append(shardData, encodedPageProofs[ppIdx][vIdx])
		}

		// Build WBT for this validator's shards
		tree := NewWellBalancedTree(shardData, types.Blake2b)
		sClub[vIdx] = tree.RootHash()

		// Concatenate all shards for exportedShards
		var combined []byte
		for _, shard := range shardData {
			combined = append(combined, shard...)
		}
		exportedShards[vIdx] = combined
	}

	return sClub, exportedShards
}

// generateErasureTreeForTest generates the erasure tree from bClubs and sClubs
// Duplicates statedb.GenerateErasureTree to avoid circular import
func generateErasureTreeForTest(bClubs []common.Hash, sClubs []common.Hash) *WellBalancedTree {
	bundleSegmentPairs := common.BuildBundleSegmentPairs(bClubs, sClubs)
	return NewWellBalancedTree(bundleSegmentPairs, types.Blake2b)
}

// TestGenerateSyntheticFull137Resp generates syn_full137resp_N.json files
// These contain synthetic shards that can be used to verify exportedRoots computation
// The exportedShards are real (derived from fibStaticBuffer), bundle/erasureRoot are synthetic
func TestGenerateSyntheticFull137Resp(t *testing.T) {
	// Test multiple segment counts
	testCases := []int{1, 5, 10, 64, 100, 3072}

	for _, numSegments := range testCases {
		t.Run(fmt.Sprintf("N=%d", numSegments), func(t *testing.T) {
			resps, expectedRoot := generateSyntheticFull137Resp(t, numSegments)

			// Save syn_full137resp_N.json
			fn := fmt.Sprintf("test/syn_full137resp_%d.json", numSegments)
			err := savefile(fn, []byte(types.ToJSON(resps)))
			if err != nil {
				t.Fatalf("Failed to save %s: %v", fn, err)
			}
			t.Logf("Saved %s", fn)

			// Load exportedRoots.json and verify
			data, err := os.ReadFile("test/exportedRoots.json")
			if err != nil {
				t.Fatalf("Failed to read exportedRoots.json: %v", err)
			}
			var exportedRoots []common.Hash
			if err := json.Unmarshal(data, &exportedRoots); err != nil {
				t.Fatalf("Failed to unmarshal exportedRoots.json: %v", err)
			}

			if numSegments < len(exportedRoots) {
				if expectedRoot != exportedRoots[numSegments] {
					t.Errorf("Root mismatch for N=%d: got %s, expected %s", numSegments, expectedRoot, exportedRoots[numSegments])
				} else {
					t.Logf("✓ Root matches for N=%d: %s", numSegments, expectedRoot)
				}
			}
		})
	}
}

// generateSyntheticFull137Resp creates Full137Resp data for N segments
// Returns the responses and the expected CDT root for those segments
func generateSyntheticFull137Resp(t *testing.T, numSegments int) ([]Full137Resp, common.Hash) {
	// Step 1: Generate fib segments deterministically
	segments := GenerateFibSegments(numSegments)
	t.Logf("Generated %d segments", len(segments))

	// Step 2: Compute the expected exportedSegmentRoot (CDT of segments)
	segmentTree := NewCDMerkleTree(segments)
	expectedRoot := segmentTree.RootHash()
	t.Logf("Expected exportedSegmentRoot: %s", expectedRoot.Hex())

	// Step 3: Build synthetic bundle (doesn't matter for exportedRoots)
	syntheticBundle := []byte("synthetic-bundle-for-testing")
	bClubs, bundleShards := buildBClubForTest(syntheticBundle)

	// Step 4: Build sClub from segments
	sClubs, exportedShards := buildSClubForTest(segments)

	// Step 5: Generate erasure tree (for synthetic erasureRoot)
	erasureTree := generateErasureTreeForTest(bClubs, sClubs)
	syntheticErasureRoot := erasureTree.RootHash()
	t.Logf("Synthetic erasureRoot: %s", syntheticErasureRoot.Hex())

	// Step 6: Build Full137Resp for each shard with faithful EncodedPath
	resps := make([]Full137Resp, types.TotalValidators)
	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		// Get proof path using Trace - must be valid for the erasure tree we built
		treeLen, leafHash, path, _, err := erasureTree.Trace(shardIdx)
		if err != nil {
			t.Fatalf("Failed to trace shard %d: %v", shardIdx, err)
		}

		// Encode the justification path
		encodedPath, err := common.EncodeJustification(path, treeLen)
		if err != nil {
			t.Fatalf("Failed to encode justification for shard %d: %v", shardIdx, err)
		}

		// Verify the proof is valid before storing
		recoveredRoot, verified, _ := VerifyWBT(treeLen, shardIdx, syntheticErasureRoot, leafHash, path)
		if !verified {
			t.Fatalf("EncodedPath verification failed for shard %d: recovered %s, expected %s",
				shardIdx, recoveredRoot.Hex(), syntheticErasureRoot.Hex())
		}

		resps[shardIdx] = Full137Resp{
			TreeLen:        types.TotalValidators,
			ShardIdx:       shardIdx,
			ErasureRoot:    syntheticErasureRoot,
			BundleShard:    bundleShards[shardIdx],
			ExportedShards: exportedShards[shardIdx],
			EncodedPath:    encodedPath,
		}
	}

	return resps, expectedRoot
}

// reconstructSegmentsFromShards reconstructs original segments from EC-encoded shards
func reconstructSegmentsFromShards(resps []Full137Resp, numSegments int) ([][]byte, error) {
	if len(resps) < 2 {
		return nil, fmt.Errorf("need at least 2 shards for reconstruction, got %d", len(resps))
	}

	// Collect exportedShards from each validator
	shards := make([][]byte, len(resps))
	for i, resp := range resps {
		shards[i] = resp.ExportedShards
	}

	// Calculate chunk size per segment shard
	chunkSize := types.SegmentSize / (types.TotalValidators / 3) // 2048 bytes per chunk

	// Create indexes array [0, 1, 2, 3, 4, 5] for all validators
	indexes := make([]uint32, types.TotalValidators)
	for i := range indexes {
		indexes[i] = uint32(i)
	}

	// Reconstruct each segment
	segments := make([][]byte, numSegments)
	for segIdx := 0; segIdx < numSegments; segIdx++ {
		// Extract the shard chunks for this segment
		segmentShards := make([][]byte, types.TotalValidators)
		for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
			start := segIdx * chunkSize
			end := start + chunkSize
			if end > len(shards[shardIdx]) {
				return nil, fmt.Errorf("shard %d too short for segment %d", shardIdx, segIdx)
			}
			segmentShards[shardIdx] = shards[shardIdx][start:end]
		}

		// EC decode to recover original segment
		decoded, err := bls.Decode(segmentShards, types.TotalValidators, indexes, types.SegmentSize)
		if err != nil {
			return nil, fmt.Errorf("failed to decode segment %d: %v", segIdx, err)
		}
		segments[segIdx] = decoded
	}

	return segments, nil
}

// TestVerifySyntheticExportedRoots loads syn_full137resp files, reconstructs segments,
// and verifies the CDT root matches exportedRoots.json
func TestVerifySyntheticExportedRoots(t *testing.T) {
	testCases := []int{1, 5, 10, 64, 100, 3072}

	// Load exportedRoots.json once
	data, err := os.ReadFile("test/exportedRoots.json")
	if err != nil {
		t.Fatalf("Failed to read exportedRoots.json: %v", err)
	}
	var exportedRoots []common.Hash
	if err := json.Unmarshal(data, &exportedRoots); err != nil {
		t.Fatalf("Failed to unmarshal exportedRoots.json: %v", err)
	}

	for _, numSegments := range testCases {
		t.Run(fmt.Sprintf("N=%d", numSegments), func(t *testing.T) {
			// Load syn_full137resp_N.json
			fn := fmt.Sprintf("test/syn_full137resp_%d.json", numSegments)
			data, err := os.ReadFile(fn)
			if err != nil {
				t.Skipf("Skipping: %s not found", fn)
				return
			}

			var resps []Full137Resp
			if err := json.Unmarshal(data, &resps); err != nil {
				t.Fatalf("Failed to unmarshal %s: %v", fn, err)
			}

			// Reconstruct segments from shards
			segments, err := reconstructSegmentsFromShards(resps, numSegments)
			if err != nil {
				t.Fatalf("Failed to reconstruct segments: %v", err)
			}

			// Compute CDT root
			tree := NewCDMerkleTree(segments)
			computedRoot := tree.RootHash()

			// Compare with expected
			if numSegments < len(exportedRoots) {
				expectedRoot := exportedRoots[numSegments]
				if computedRoot != expectedRoot {
					t.Errorf("Root mismatch for N=%d: computed %s, expected %s", numSegments, computedRoot.Hex(), expectedRoot.Hex())
				} else {
					t.Logf("✓ Verified N=%d: %s", numSegments, computedRoot.Hex())
				}
			}
		})
	}
}

// TestVerifyFibSegments verifies that the deterministic segment generation matches
// the first few known entries from exportedRoots.json
func TestVerifyFibSegments(t *testing.T) {
	// Load existing exported roots
	data, err := os.ReadFile("test/exportedRoots.json")
	if err != nil {
		t.Fatalf("Failed to read exportedRoots.json: %v", err)
	}

	var existingRoots []common.Hash
	if err := json.Unmarshal(data, &existingRoots); err != nil {
		t.Fatalf("Failed to parse exportedRoots.json: %v", err)
	}

	t.Logf("Loaded %d existing roots\n", len(existingRoots))

	// Verify deterministic generation matches existing roots
	maxVerify := len(existingRoots)
	if maxVerify > 3072 {
		maxVerify = 3072 // Only verify first 3072 to save time
	}

	allSegments := GenerateFibSegments(maxVerify)
	debugAllRoot := true
	for n := 0; n <= maxVerify; n++ {
		var computedRoot common.Hash
		if n == 0 {
			computedRoot = common.Hash{}
		} else {
			tree := NewCDMerkleTree(allSegments[:n])
			computedRoot = tree.RootHash()
		}

		if computedRoot != existingRoots[n] {
			t.Errorf("Root mismatch at n=%d: computed %s, expected %s", n, computedRoot.Hex(), existingRoots[n].Hex())
		} else if n <= 10 || n == maxVerify-1 || debugAllRoot {
			t.Logf("Root verified for n=%d: %s\n", n, computedRoot.Hex())
		}
	}
}
