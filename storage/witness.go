package storage

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	evmverkle "github.com/colorfulnotion/jam/builder/evm/verkle"
	"github.com/colorfulnotion/jam/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-verkle"
)

// BuildVerkleWitness constructs a dual-proof Verkle witness from execution reads/writes
//
// This function generates TWO separate proofs:
// 1. Pre-state proof: Covers all READ keys (verkleReadLog)
// 2. Post-state proof: Covers all WRITE keys (from contractWitnessBlob)
//
// Steps:
// 1. Collect read keys from verkleReadLog (pre-state section)
// 2. Extract pre-values and generate pre-state multiproof
// 3. Apply writes to clone tree to compute post-state
// 4. Collect write keys from contractWitnessBlob (post-state section)
// 5. Extract post-values and generate post-state multiproof
// 6. Serialize dual-proof witness with both sections
//
// Parameters:
//   - verkleReadLog: All Verkle reads tracked during execution
//   - contractWitnessBlob: Serialized contract intents (code + storage shards)
//   - tree: Current Verkle tree state (pre-state)
//
// Returns:
//   - witnessBytes: Serialized VerkleWitness bytes (dual-proof format)
//   - postStateRoot: The post-state Verkle root hash
//   - postTree: The post-state Verkle tree (with writes applied)
//   - error: Any error encountered
func BuildVerkleWitness(
	verkleReadLog []types.VerkleRead,
	contractWitnessBlob []byte,
	tree verkle.VerkleNode,
) (witnessBytes []byte, postStateRoot common.Hash, postTree verkle.VerkleNode, err error) {
	log.Debug(log.EVM, "BuildVerkleWitness (dual-proof)", "readCount", len(verkleReadLog), "blobSize", len(contractWitnessBlob))

	// ===== PHASE 1: Pre-State Section (Reads) =====

	// Step 1: Collect all Verkle keys from reads
	readKeySet := make(map[common.Hash]struct{})
	for _, read := range verkleReadLog {
		readKeySet[read.VerkleKey] = struct{}{}
	}

	// Deduplicate read keys
	readKeys := make([][32]byte, 0, len(readKeySet))
	for key := range readKeySet {
		readKeys = append(readKeys, key)
	}

	// Sort read keys for deterministic proofs
	sort.Slice(readKeys, func(i, j int) bool {
		return bytes.Compare(readKeys[i][:], readKeys[j][:]) < 0
	})

	log.Trace(log.EVM, "BuildVerkleWitness: collected read keys", "readKeys", len(readKeys))

	// Step 2: Extract pre-values by reading tree
	// Separate present keys (for proof) from all keys (for witness)
	allReadKeys := readKeys
	allPreValues := make([][32]byte, len(readKeys))
	presentReadKeys := make([][32]byte, 0, len(readKeys))
	presentPreValues := make([][32]byte, 0, len(readKeys))
	presentIndices := make([]int, 0, len(readKeys))

	for i, key := range readKeys {
		value, err := tree.Get(key[:], nil)
		if err != nil {
			log.Warn(log.EVM, "BuildVerkleWitness: key read error, using zero value", "key", fmt.Sprintf("%x", key[:8]), "err", err)
			allPreValues[i] = [32]byte{} // Zero value for errors
		} else if value == nil {
			log.Trace(log.EVM, "BuildVerkleWitness: key absent, using zero value", "key", fmt.Sprintf("%x", key[:8]))
			allPreValues[i] = [32]byte{} // Zero value for absent keys
		} else {
			// Key is present - include in proof
			var val [32]byte
			if len(value) >= 32 {
				copy(val[:], value[:32])
			} else {
				copy(val[:], value)
			}
			allPreValues[i] = val
			presentReadKeys = append(presentReadKeys, key)
			presentPreValues = append(presentPreValues, val)
			presentIndices = append(presentIndices, i)
		}
	}

	log.Trace(log.EVM, "BuildVerkleWitness: collected read values", "totalKeys", len(allReadKeys), "presentKeys", len(presentReadKeys))

	// Step 3: Commit tree before generating proof (proof generation needs committed tree)
	preStateRoot := tree.Commit()
	preStateRootBytes := preStateRoot.Bytes()

	// Step 4: Generate pre-state multiproof (for ALL keys, including absent ones)
	// go-verkle handles absent keys via proof-of-absence stems
	allKeyBytes := make([][]byte, len(allReadKeys))
	for i, key := range allReadKeys {
		allKeyBytes[i] = key[:]
	}

	var preProof []byte
	if len(allKeyBytes) == 0 {
		preProof = []byte{}
		log.Trace(log.EVM, "BuildVerkleWitness: no read keys, skipping pre-state proof generation")
	} else {
		preProof, _, _, err = evmverkle.GenerateVerkleProof(tree, allKeyBytes)
		if err != nil {
			return nil, common.Hash{}, nil, fmt.Errorf("failed to generate pre-state proof: %w", err)
		}
		log.Info(log.EVM, "✅ Generated PRE-STATE proof",
			"proofLen", len(preProof),
			"totalKeys", len(allReadKeys),
			"presentKeys", len(presentReadKeys),
			"preStateRoot", fmt.Sprintf("%x", preStateRootBytes[:]))

		// NOTE: Go-side verification skipped because:
		// 1. Proof now includes root at commitments_by_path[0] (for Rust verifier)
		// 2. go-verkle's Verify() expects root as separate parameter, not in CommitmentsByPath
		// 3. Would require unmarshaling proof, removing root, then verifying - wasteful
		// 4. Rust guarantor will verify the proof cryptographically anyway
	}

	// ===== PHASE 2: Post-State Section (Writes) =====

	// Step 4: Apply writes to clone tree
	postTree = tree.Copy()
	if err := applyContractWritesToTree(contractWitnessBlob, postTree); err != nil {
		return nil, common.Hash{}, nil, fmt.Errorf("failed to apply contract writes: %v", err)
	}
	postStateRootVerkle := postTree.Commit()
	postStateRootBytes := postStateRootVerkle.Bytes()
	postStateRoot = common.BytesToHash(postStateRootBytes[:])

	// Step 5: Collect write keys from contractWitnessBlob
	writeKeySet := make(map[[32]byte]struct{})
	_, err = parseContractWitnessBlobForKeys(contractWitnessBlob, writeKeySet)
	if err != nil {
		return nil, common.Hash{}, nil, fmt.Errorf("failed to parse contract writes: %v", err)
	}

	writeKeys := make([][32]byte, 0, len(writeKeySet))
	for key := range writeKeySet {
		writeKeys = append(writeKeys, key)
	}

	// Sort write keys to ensure deterministic ordering for proof generation
	sort.Slice(writeKeys, func(i, j int) bool {
		return bytes.Compare(writeKeys[i][:], writeKeys[j][:]) < 0
	})

	log.Trace(log.EVM, "BuildVerkleWitness: collected write keys", "writeKeys", len(writeKeys))

	// Step 6: Extract pre-values and post-values for write keys
	// Use [][]byte to allow nil values for absent keys
	preValueSlices := make([][]byte, len(writeKeys))
	postValueSlices := make([][]byte, len(writeKeys))
	for i, key := range writeKeys {
		// Get pre-value from original tree (before writes)
		preVal, err := tree.Get(key[:], nil)
		if err != nil || preVal == nil {
			preValueSlices[i] = nil // nil for absent keys in pre-state
		} else {
			// Copy to ensure we have our own slice
			preValueSlices[i] = make([]byte, len(preVal))
			copy(preValueSlices[i], preVal)
		}

		// Get post-value from post-tree (after writes)
		postVal, err := postTree.Get(key[:], nil)
		if err != nil || postVal == nil {
			postValueSlices[i] = nil // nil for absent keys
		} else {
			postValueSlices[i] = make([]byte, len(postVal))
			copy(postValueSlices[i], postVal)
		}
	}

	// Step 7: Generate post-state multiproof (post-state only)
	// Use postTree for both inputs so the proof is anchored to the post root.
	writeKeyBytes := make([][]byte, len(writeKeys))
	for i, key := range writeKeys {
		writeKeyBytes[i] = key[:]
	}

	var postProof []byte
	if len(writeKeyBytes) == 0 {
		postProof = []byte{}
		log.Trace(log.EVM, "BuildVerkleWitness: no write keys, skipping post-state proof generation")
	} else {
		postProof, _, _, err = evmverkle.GenerateVerkleProof(postTree, writeKeyBytes)
		if err != nil {
			return nil, common.Hash{}, nil, fmt.Errorf("failed to generate post-state proof: %w", err)
		}
		log.Info(log.EVM, "✅ Generated POST-STATE proof",
			"proofLen", len(postProof),
			"writeKeys", len(writeKeys),
			"preStateRoot", fmt.Sprintf("%x", preStateRootBytes[:]),
			"postStateRoot", fmt.Sprintf("%x", postStateRootBytes[:]))

		// NOTE: Go-side verification skipped (same reason as pre-state proof above)
	}

	// ===== PHASE 3: Serialize Dual-Proof Witness =====

	// Extract metadata for both sections
	readMetadata := extractKeyMetadataFromReads(verkleReadLog, readKeys)
	writeMetadata := extractKeyMetadata(contractWitnessBlob, writeKeys, verkleReadLog)

	// Calculate total size:
	// Pre-section: 32B root + 4B count + (161B × reads) + 4B proof_len + proof
	// Post-section: 32B root + 4B count + (161B × writes) + 4B proof_len + proof
	// 161 bytes = 32 key + 1 type + 20 addr + 8 extra + 32 storage_key + 32 pre + 32 post + 4 txIndex
	totalSize := 32 + 4 + (161 * len(allReadKeys)) + 4 + len(preProof) +
		32 + 4 + (161 * len(writeKeys)) + 4 + len(postProof)
	result := make([]byte, 0, totalSize)

	// Serialize pre-state section (include ALL keys, even absent ones)
	result = append(result, preStateRootBytes[:]...)
	numReads := uint32(len(allReadKeys))
	result = append(result, byte(numReads>>24), byte(numReads>>16), byte(numReads>>8), byte(numReads))

	for i, key := range allReadKeys {
		result = append(result, key[:]...)
		meta := readMetadata[i]
		result = append(result, byte(meta.KeyType))
		result = append(result, meta.Address[:]...)
		result = append(result, byte(meta.Extra>>56), byte(meta.Extra>>48), byte(meta.Extra>>40), byte(meta.Extra>>32),
			byte(meta.Extra>>24), byte(meta.Extra>>16), byte(meta.Extra>>8), byte(meta.Extra))
		result = append(result, meta.StorageKey[:]...)
		result = append(result, allPreValues[i][:]...)

		// For read entries, post_value is same as pre_value (reads don't modify)
		result = append(result, allPreValues[i][:]...)

		// TxIndex (4 bytes, big-endian)
		result = append(result, byte(meta.TxIndex>>24), byte(meta.TxIndex>>16), byte(meta.TxIndex>>8), byte(meta.TxIndex))
	}

	preProofLen := uint32(len(preProof))
	result = append(result, byte(preProofLen>>24), byte(preProofLen>>16), byte(preProofLen>>8), byte(preProofLen))
	result = append(result, preProof...)

	// Serialize post-state section
	result = append(result, postStateRootBytes[:]...)
	numWrites := uint32(len(writeKeys))
	result = append(result, byte(numWrites>>24), byte(numWrites>>16), byte(numWrites>>8), byte(numWrites))

	for i, key := range writeKeys {
		result = append(result, key[:]...)
		meta := writeMetadata[i]
		result = append(result, byte(meta.KeyType))
		result = append(result, meta.Address[:]...)
		result = append(result, byte(meta.Extra>>56), byte(meta.Extra>>48), byte(meta.Extra>>40), byte(meta.Extra>>32),
			byte(meta.Extra>>24), byte(meta.Extra>>16), byte(meta.Extra>>8), byte(meta.Extra))
		result = append(result, meta.StorageKey[:]...)

		// For write entries, need both pre and post values
		// Serialize pre-value (nil = all zeros for absent keys)
		if preValueSlices[i] == nil {
			result = append(result, make([]byte, 32)...) // All zeros for nil (absent key)
		} else {
			var val [32]byte
			copy(val[:], preValueSlices[i])
			result = append(result, val[:]...)
		}

		// Serialize post-value
		if postValueSlices[i] == nil {
			result = append(result, make([]byte, 32)...)
		} else {
			var val [32]byte
			copy(val[:], postValueSlices[i])
			result = append(result, val[:]...)
		}

		// TxIndex (4 bytes, big-endian)
		result = append(result, byte(meta.TxIndex>>24), byte(meta.TxIndex>>16), byte(meta.TxIndex>>8), byte(meta.TxIndex))
	}

	postProofLen := uint32(len(postProof))
	result = append(result, byte(postProofLen>>24), byte(postProofLen>>16), byte(postProofLen>>8), byte(postProofLen))
	result = append(result, postProof...)

	log.Trace(log.EVM, "BuildVerkleWitness: dual-proof complete",
		"witnessSize", len(result),
		"readKeys", numReads,
		"writeKeys", numWrites,
		"preProofLen", preProofLen,
		"postProofLen", postProofLen)

	if outPath := os.Getenv("JAM_VERKLE_WITNESS_OUT"); outPath != "" {
		if err := os.WriteFile(outPath, result, 0o644); err != nil {
			log.Warn(log.EVM, "BuildVerkleWitness: failed to write witness file", "path", outPath, "err", err)
		} else {
			log.Info(log.EVM, "BuildVerkleWitness: wrote witness file", "path", outPath, "size", len(result))
		}
	}

	return result, postStateRoot, postTree, nil
}

// VerifyPostStateProof validates the builder's post-state proof against the guarantor's writes.
//
// This function performs THREE critical checks:
// 1. Bidirectional key set validation (builder keys == guarantor keys)
// 2. Value matching for all keys
// 3. Cryptographic proof validation
//
// IMPORTANT: This does NOT recompute the tree or apply writes.
// The cryptographic proof is sufficient to verify correctness.
//
// Parameters:
//   - claimedPostRoot: Builder's claimed post-state root
//   - builderWriteKeys: All keys included in builder's post-proof
//   - builderPostValues: Post-values for those keys (from builder's tree)
//   - postProofData: Serialized Verkle multiproof
//   - guarantorWrites: Guarantor's write set as key-value map (for comparison)
//
// Returns: nil if verification passes, error otherwise
func VerifyPostStateProof(
	claimedPostRoot [32]byte,
	builderWriteKeys [][32]byte,
	builderPostValues [][32]byte,
	postProofData []byte,
	guarantorWrites map[[32]byte][32]byte,
) error {
	log.Debug(log.EVM, "VerifyPostStateProof: starting",
		"builderKeys", len(builderWriteKeys),
		"guarantorKeys", len(guarantorWrites),
		"proofLen", len(postProofData))

	// Step 1: Build builder's key-value map for efficient comparison
	builderWrites := make(map[[32]byte][32]byte, len(builderWriteKeys))
	for i, key := range builderWriteKeys {
		builderWrites[key] = builderPostValues[i]
	}

	if len(builderWrites) == 0 {
		if len(guarantorWrites) != 0 {
			return fmt.Errorf("Write set size mismatch: builder=0 guarantor=%d", len(guarantorWrites))
		}
		log.Info(log.EVM, "VerifyPostStateProof: no writes present, skipping cryptographic verification")
		return nil
	}

	// Step 2: Verify key sets are IDENTICAL (bidirectional check)
	if len(builderWrites) != len(guarantorWrites) {
		return fmt.Errorf("Write set size mismatch: builder=%d guarantor=%d",
			len(builderWrites), len(guarantorWrites))
	}

	// Step 3: Verify every builder key exists in guarantor with matching value
	for key, builderValue := range builderWrites {
		guarantorValue, exists := guarantorWrites[key]
		if !exists {
			return fmt.Errorf("Builder wrote key %x but guarantor didn't", key[:8])
		}
		if !bytes.Equal(builderValue[:], guarantorValue[:]) {
			return fmt.Errorf("Value mismatch for key %x: builder=%x (len=%d) guarantor=%x (len=%d)",
				key[:8], builderValue[:], len(builderValue), guarantorValue[:], len(guarantorValue))
		}
	}

	// Step 4: Verify every guarantor key exists in builder (catches omissions)
	// This prevents malicious builders from omitting keys to hide writes
	for key := range guarantorWrites {
		if _, exists := builderWrites[key]; !exists {
			return fmt.Errorf("Guarantor wrote key %x but builder didn't include it in proof", key[:8])
		}
	}

	log.Debug(log.EVM, "VerifyPostStateProof: key sets and values match")

	// Step 5: Verify post-multiproof is cryptographically valid
	// This proves that claimedPostRoot is the correct root for these key-value pairs
	builderWriteKeyBytes := make([][]byte, len(builderWriteKeys))
	builderPostValueBytes := make([][]byte, len(builderPostValues))
	for i := range builderWriteKeys {
		builderWriteKeyBytes[i] = builderWriteKeys[i][:]
		builderPostValueBytes[i] = builderPostValues[i][:]
	}

	valid, err := evmverkle.VerifyVerkleProof(
		postProofData,
		claimedPostRoot[:],
		builderWriteKeyBytes,
		builderPostValueBytes,
	)
	if err != nil {
		return fmt.Errorf("Post-proof verification failed: %w", err)
	}
	if !valid {
		return fmt.Errorf("Post-proof cryptographic verification failed")
	}

	log.Info(log.EVM, "✅ Post-state verification PASSED",
		"postRoot", fmt.Sprintf("%x", claimedPostRoot[:8]),
		"verifiedKeys", len(builderWrites))

	// All checks passed:
	// - Write sets match exactly (no missing or extra keys)
	// - All values match
	// - Cryptographic proof is valid
	// Therefore: claimedPostRoot is correct WITHOUT recomputing the tree!
	return nil
}

// CompareWriteKeySets checks that the builder's post-proof keys match the guarantor's write keys.
// Values are ignored; this is meant to close the "missing key-set validation" gap.
func CompareWriteKeySets(builderWriteKeys [][32]byte, guarantorWrites map[[32]byte][32]byte) error {
	builderSet := make(map[[32]byte]struct{}, len(builderWriteKeys))
	for _, key := range builderWriteKeys {
		builderSet[key] = struct{}{}
	}

	if len(builderSet) != len(guarantorWrites) {
		return fmt.Errorf("Write set size mismatch: builder=%d guarantor=%d", len(builderSet), len(guarantorWrites))
	}

	for key := range builderSet {
		if _, ok := guarantorWrites[key]; !ok {
			return fmt.Errorf("Builder wrote key %x but guarantor didn't", key[:8])
		}
	}
	for key := range guarantorWrites {
		if _, ok := builderSet[key]; !ok {
			return fmt.Errorf("Guarantor wrote key %x but builder didn't include it in proof", key[:8])
		}
	}

	return nil
}

// KeyMetadata stores metadata for a Verkle key
type KeyMetadata struct {
	KeyType    uint8    // 0=BasicData, 1=CodeHash, 2=CodeChunk, 3=Storage
	Address    [20]byte // Contract address
	Extra      uint64   // Additional data (chunk_id for code, etc.)
	StorageKey [32]byte // Full 32-byte storage key (only for KeyType=3)
	TxIndex    uint32   // Transaction index within work package (0=pre-exec, 1..n=txs)
}

// extractKeyMetadataFromReads extracts metadata from VerkleReadLog
// This provides address and type information directly from the read operations
func extractKeyMetadataFromReads(reads []types.VerkleRead, keys [][32]byte) []KeyMetadata {
	metadata := make([]KeyMetadata, len(keys))
	keyToIndex := make(map[[32]byte]int)
	seen := make([]bool, len(keys))

	for i, key := range keys {
		keyToIndex[key] = i
		// Default to zero address (will be overwritten if found in reads)
		metadata[i] = KeyMetadata{KeyType: 0, Address: [20]byte{}, Extra: 0}
	}

	// Map reads to metadata
	for _, read := range reads {
		if idx, ok := keyToIndex[read.VerkleKey]; ok {
			// Preserve the earliest tx_index for duplicate reads of the same key
			if !seen[idx] || read.TxIndex < metadata[idx].TxIndex {
				metadata[idx] = KeyMetadata{
					KeyType:    read.KeyType,
					Address:    read.Address,
					Extra:      read.Extra,
					StorageKey: read.StorageKey,
					TxIndex:    read.TxIndex,
				}
				seen[idx] = true
			}
		}
	}

	return metadata
}

// extractKeyMetadata derives metadata for each Verkle key
// This is needed because Pedersen hash is one-way - we can't reverse engineer the key
// TxIndex is copied from verkleReadLog for keys that were read
func extractKeyMetadata(contractBlob []byte, keys [][32]byte, verkleReadLog []types.VerkleRead) []KeyMetadata {
	// Build metadata map by parsing the contract blob
	metadata := make([]KeyMetadata, len(keys))
	keyToIndex := make(map[[32]byte]int)

	for i, key := range keys {
		keyToIndex[key] = i
	}

	// First pass: Build a map from VerkleKey -> TxIndex from reads
	keyToTxIndex := make(map[[32]byte]uint32)
	for _, read := range verkleReadLog {
		keyToTxIndex[read.VerkleKey] = read.TxIndex
	}

	// Parse contract blob to extract metadata
	if len(contractBlob) > 0 {
		offset := 0
		for offset < len(contractBlob) {
			if len(contractBlob)-offset < 29 {
				break
			}

			var address [20]byte
			copy(address[:], contractBlob[offset:offset+20])
			kind := contractBlob[offset+20]
			payloadLen := uint32(contractBlob[offset+21]) | uint32(contractBlob[offset+22])<<8 |
				uint32(contractBlob[offset+23])<<16 | uint32(contractBlob[offset+24])<<24
			txIndex := uint32(contractBlob[offset+25]) | uint32(contractBlob[offset+26])<<8 |
				uint32(contractBlob[offset+27])<<16 | uint32(contractBlob[offset+28])<<24
			offset += 29

			if len(contractBlob)-offset < int(payloadLen) {
				break
			}

			payload := contractBlob[offset : offset+int(payloadLen)]
			offset += int(payloadLen)

			// Extract keys and metadata based on kind
			// Use txIndex from blob for writes (fixes write-only key attribution)
			switch kind {
			case 0x00: // Code
				// BasicData key
				basicKey := evmverkle.BasicDataKey(address[:])
				var key32 [32]byte
				copy(key32[:], basicKey)
				if idx, ok := keyToIndex[key32]; ok {
					metadata[idx] = KeyMetadata{
						KeyType: 0,
						Address: address,
						Extra:   0,
						TxIndex: txIndex, // Use blob's tx_index for writes
					}
				}

				// CodeHash key
				codeHashKey := evmverkle.CodeHashKey(address[:])
				copy(key32[:], codeHashKey)
				if idx, ok := keyToIndex[key32]; ok {
					metadata[idx] = KeyMetadata{
						KeyType: 1,
						Address: address,
						Extra:   0,
						TxIndex: txIndex,
					}
				}

				// Code chunks
				chunks := evmverkle.ChunkifyCode(payload)
				numChunks := len(chunks) / 32
				for chunkID := uint64(0); chunkID < uint64(numChunks); chunkID++ {
					chunkKey := evmverkle.CodeChunkKey(address[:], chunkID)
					copy(key32[:], chunkKey)
					if idx, ok := keyToIndex[key32]; ok {
						metadata[idx] = KeyMetadata{
							KeyType: 2,
							Address: address,
							Extra:   chunkID,
							TxIndex: txIndex,
						}
					}
				}

			case 0x01: // Storage
				entries, _ := evmtypes.ParseShardPayload(payload)
				for _, entry := range entries {
					storageKey := evmverkle.StorageSlotKey(address[:], entry.KeyH[:])
					var key32 [32]byte
					copy(key32[:], storageKey)
					if idx, ok := keyToIndex[key32]; ok {
						// Store the full 32-byte storage slot in metadata
						metadata[idx] = KeyMetadata{
							KeyType:    3,
							Address:    address,
							Extra:      0,
							StorageKey: entry.KeyH, // Full 32-byte storage key
							TxIndex:    txIndex,
						}
					}
				}

			case 0x02: // Balance
				// Balance writes update BasicData (balance field)
				basicKey := evmverkle.BasicDataKey(address[:])
				var key32 [32]byte
				copy(key32[:], basicKey)
				if idx, ok := keyToIndex[key32]; ok {
					metadata[idx] = KeyMetadata{
						KeyType: 0,
						Address: address,
						Extra:   0,
						TxIndex: txIndex,
					}
				}

			case 0x06: // Nonce
				// Nonce writes also update BasicData (nonce field)
				basicKey := evmverkle.BasicDataKey(address[:])
				var key32 [32]byte
				copy(key32[:], basicKey)
				if idx, ok := keyToIndex[key32]; ok {
					metadata[idx] = KeyMetadata{
						KeyType: 0,
						Address: address,
						Extra:   0,
						TxIndex: txIndex,
					}
				}
			}
		}
	}

	// For any keys without metadata from contract blob, try to infer from reads
	// (e.g., balance/nonce reads that aren't writes)
	for i := range metadata {
		if metadata[i].KeyType == 0 && metadata[i].Address == [20]byte{} {
			// Key has no metadata - this is likely a read-only BasicData or similar
			// Default to BasicData with zero address (will be refined in actual usage)
			metadata[i] = KeyMetadata{KeyType: 0, Address: [20]byte{}, Extra: 0}
		}
	}

	return metadata
}

// parseContractWitnessBlobForKeys extracts Verkle keys from contract witness blob
// and adds them to the provided keySet.
//
// Format: Multiple blobs, each: [20B address][1B kind][4B payload_length][...payload...]
// Kind 0x00 = storage shard, Kind 0x01 = code
//
// Returns: Number of write keys added
func parseContractWitnessBlobForKeys(blob []byte, keySet map[[32]byte]struct{}) (int, error) {
	if len(blob) == 0 {
		return 0, nil
	}

	offset := 0
	writeKeyCount := 0

	for offset < len(blob) {
		// Need at least 29 bytes for header: 20B address + 1B kind + 4B length + 4B tx_index
		if len(blob)-offset < 29 {
			return writeKeyCount, fmt.Errorf("invalid contract witness blob: insufficient header bytes at offset %d", offset)
		}

		// Parse header
		address := blob[offset : offset+20]
		kind := blob[offset+20]
		payloadLength := uint32(blob[offset+21]) | uint32(blob[offset+22])<<8 |
			uint32(blob[offset+23])<<16 | uint32(blob[offset+24])<<24
		_ = uint32(blob[offset+25]) | uint32(blob[offset+26])<<8 |
			uint32(blob[offset+27])<<16 | uint32(blob[offset+28])<<24 // txIndex (unused here, but needed for blob parsing)
		offset += 29

		// Check payload length
		if len(blob)-offset < int(payloadLength) {
			return writeKeyCount, fmt.Errorf("invalid contract witness blob: insufficient payload bytes at offset %d", offset)
		}

		payload := blob[offset : offset+int(payloadLength)]
		offset += int(payloadLength)

		// Extract Verkle keys based on kind
		switch kind {
		case 0x01: // Storage shard
			// Parse storage shard to extract keys
			keys, err := extractStorageVerkleKeys(address, payload)
			if err != nil {
				log.Warn(log.EVM, "parseContractWitnessBlobForKeys: failed to extract storage keys", "err", err)
				continue
			}
			for _, key := range keys {
				keySet[key] = struct{}{}
				writeKeyCount++
			}

		case 0x00: // Code
			// Add code hash key
			codeHashKey := evmverkle.CodeHashKey(address)
			var key32 [32]byte
			copy(key32[:], codeHashKey)
			keySet[key32] = struct{}{}
			writeKeyCount++

			// Add code chunk keys
			chunks := evmverkle.ChunkifyCode(payload)
			numChunks := len(chunks) / 32
			for chunkID := uint64(0); chunkID < uint64(numChunks); chunkID++ {
				chunkKey := evmverkle.CodeChunkKey(address, chunkID)
				copy(key32[:], chunkKey)
				keySet[key32] = struct{}{}
				writeKeyCount++
			}

			// Also add BasicData key (for code_size update)
			basicDataKey := evmverkle.BasicDataKey(address)
			copy(key32[:], basicDataKey)
			keySet[key32] = struct{}{}
			writeKeyCount++

		case 0x02: // Balance
			// Add BasicData key (for balance update)
			basicDataKey := evmverkle.BasicDataKey(address)
			var key32 [32]byte
			copy(key32[:], basicDataKey)
			keySet[key32] = struct{}{}
			writeKeyCount++

		case 0x06: // Nonce
			// Add BasicData key (for nonce update)
			basicDataKey := evmverkle.BasicDataKey(address)
			var key32 [32]byte
			copy(key32[:], basicDataKey)
			keySet[key32] = struct{}{}
			writeKeyCount++

		default:
			log.Warn(log.EVM, "parseContractWitnessBlobForKeys: unknown kind", "kind", kind)
		}
	}

	return writeKeyCount, nil
}

// extractStorageVerkleKeys extracts Verkle keys from a storage shard payload
// Payload format: Variable-length entries, each: [32B key][32B value]
func extractStorageVerkleKeys(address []byte, payload []byte) ([][32]byte, error) {
	entries, err := evmtypes.ParseShardPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse shard payload: %v", err)
	}

	keys := make([][32]byte, 0, len(entries))
	for _, entry := range entries {
		// Derive Verkle key from storage key
		storageKey := evmverkle.StorageSlotKey(address, entry.KeyH[:])
		var key32 [32]byte
		copy(key32[:], storageKey)
		keys = append(keys, key32)
	}

	return keys, nil
}

// ExtractWriteMapFromContractWitness applies a contract witness blob to a cloned tree and
// returns the resulting write set as a map from verkle key -> post-value.
func ExtractWriteMapFromContractWitness(tree verkle.VerkleNode, blob []byte) (map[[32]byte][32]byte, error) {
	writeMap := make(map[[32]byte][32]byte)

	if len(blob) == 0 {
		return writeMap, nil
	}

	if tree == nil {
		return nil, fmt.Errorf("nil verkle tree provided")
	}

	// Collect all write keys from the blob
	writeKeySet := make(map[[32]byte]struct{})
	if _, err := parseContractWitnessBlobForKeys(blob, writeKeySet); err != nil {
		return nil, err
	}

	// Apply writes to a cloned tree to compute post-values
	postTree := tree.Copy()
	if err := applyContractWritesToTree(blob, postTree); err != nil {
		return nil, err
	}

	for key := range writeKeySet {
		value, err := postTree.Get(key[:], nil)
		if err != nil {
			return nil, fmt.Errorf("failed to read post-value for key %x: %v", key[:8], err)
		}
		if value == nil {
			return nil, fmt.Errorf("missing post-value for key %x after applying writes", key[:8])
		}

		var val32 [32]byte
		copy(val32[:], value[:])
		writeMap[key] = val32
	}

	return writeMap, nil
}

// ApplyContractWrites applies all contract writes to the current Verkle tree
// This includes code deployments and storage updates.
func (store *StateDBStorage) ApplyContractWrites(blob []byte) error {
	if len(blob) == 0 {
		return nil
	}

	if store.CurrentVerkleTree == nil {
		return fmt.Errorf("no current verkle tree available")
	}

	offset := 0

	for offset < len(blob) {
		// Parse header
		if len(blob)-offset < 29 {
			return fmt.Errorf("invalid contract witness blob: insufficient header bytes at offset %d", offset)
		}

		address := blob[offset : offset+20]
		kind := blob[offset+20]
		payloadLength := uint32(blob[offset+21]) | uint32(blob[offset+22])<<8 |
			uint32(blob[offset+23])<<16 | uint32(blob[offset+24])<<24
		_ = uint32(blob[offset+25]) | uint32(blob[offset+26])<<8 |
			uint32(blob[offset+27])<<16 | uint32(blob[offset+28])<<24 // txIndex (unused here, but needed for blob parsing)
		offset += 29

		if len(blob)-offset < int(payloadLength) {
			return fmt.Errorf("invalid contract witness blob: insufficient payload bytes at offset %d", offset)
		}

		payload := blob[offset : offset+int(payloadLength)]
		offset += int(payloadLength)

		// Apply writes based on kind
		switch kind {
		case 0x00: // Code
			if err := applyCodeWrites(address, payload, store.CurrentVerkleTree); err != nil {
				return fmt.Errorf("failed to apply code writes: %v", err)
			}

		case 0x01: // Storage shard
			if err := applyStorageWrites(address, payload, store.CurrentVerkleTree); err != nil {
				return fmt.Errorf("failed to apply storage writes: %v", err)
			}

		case 0x02: // Balance
			if err := applyBalanceWrites(address, payload, store.CurrentVerkleTree); err != nil {
				return fmt.Errorf("failed to apply balance writes: %v", err)
			}

		case 0x06: // Nonce
			if err := applyNonceWrites(address, payload, store.CurrentVerkleTree); err != nil {
				return fmt.Errorf("failed to apply nonce writes: %v", err)
			}

		default:
			log.Warn(log.EVM, "ApplyContractWrites: unknown kind", "kind", kind)
		}
	}

	return nil
}

// applyContractWritesToTree applies all contract writes to the given Verkle tree
// This is used by BuildVerkleWitness to apply writes to a cloned tree
func applyContractWritesToTree(blob []byte, tree verkle.VerkleNode) error {
	if len(blob) == 0 {
		return nil
	}

	offset := 0

	for offset < len(blob) {
		// Parse header
		if len(blob)-offset < 29 {
			return fmt.Errorf("invalid contract witness blob: insufficient header bytes at offset %d", offset)
		}

		address := blob[offset : offset+20]
		kind := blob[offset+20]
		payloadLength := uint32(blob[offset+21]) | uint32(blob[offset+22])<<8 |
			uint32(blob[offset+23])<<16 | uint32(blob[offset+24])<<24
		_ = uint32(blob[offset+25]) | uint32(blob[offset+26])<<8 |
			uint32(blob[offset+27])<<16 | uint32(blob[offset+28])<<24 // txIndex (unused here, but needed for blob parsing)
		offset += 29

		if len(blob)-offset < int(payloadLength) {
			return fmt.Errorf("invalid contract witness blob: insufficient payload bytes at offset %d", offset)
		}

		payload := blob[offset : offset+int(payloadLength)]
		offset += int(payloadLength)

		// Apply writes based on kind
		switch kind {
		case 0x00: // Code
			if err := applyCodeWrites(address, payload, tree); err != nil {
				return fmt.Errorf("failed to apply code writes: %v", err)
			}

		case 0x01: // Storage shard
			if err := applyStorageWrites(address, payload, tree); err != nil {
				return fmt.Errorf("failed to apply storage writes: %v", err)
			}

		case 0x02: // Balance
			if err := applyBalanceWrites(address, payload, tree); err != nil {
				return fmt.Errorf("failed to apply balance writes: %v", err)
			}

		case 0x06: // Nonce
			if err := applyNonceWrites(address, payload, tree); err != nil {
				return fmt.Errorf("failed to apply nonce writes: %v", err)
			}

		default:
			log.Warn(log.EVM, "applyContractWritesToTree: unknown kind", "kind", kind)
		}
	}

	return nil
}

// applyStorageWrites applies storage writes to the Verkle tree
func applyStorageWrites(address []byte, payload []byte, tree verkle.VerkleNode) error {
	entries, err := evmtypes.ParseShardPayload(payload)
	if err != nil {
		return fmt.Errorf("failed to parse shard payload: %v", err)
	}

	for _, entry := range entries {
		verkleKey := evmverkle.StorageSlotKey(address, entry.KeyH[:])
		if err := tree.Insert(verkleKey, entry.Value[:], nil); err != nil {
			return fmt.Errorf("failed to insert storage slot: %v", err)
		}
	}

	return nil
}

// applyCodeWrites applies code writes to the Verkle tree
// This includes: BasicData (code_size), CodeHash, and CodeChunks
func applyCodeWrites(address []byte, code []byte, tree verkle.VerkleNode) error {
	// 1. Insert code chunks
	if err := evmverkle.InsertCodeChunks(tree, address, code); err != nil {
		return fmt.Errorf("failed to insert code chunks: %v", err)
	}

	// 2. Insert code hash
	codeHashKey := evmverkle.CodeHashKey(address)
	var codeHash [32]byte
	if len(code) == 0 {
		copy(codeHash[:], evmverkle.GetEmptyCodeHash())
	} else {
		hash := crypto.Keccak256(code)
		copy(codeHash[:], hash)
	}
	if err := tree.Insert(codeHashKey, codeHash[:], nil); err != nil {
		return fmt.Errorf("failed to insert code hash: %v", err)
	}

	// 3. Update BasicData with code_size
	basicDataKey := evmverkle.BasicDataKey(address)
	basicData := make([]byte, 32)

	// Read existing BasicData first (to preserve balance/nonce)
	existing, err := tree.Get(basicDataKey, nil)
	if err == nil && len(existing) >= 32 {
		copy(basicData, existing[:32])
	}

	// Update code_size field (offset 5-7, 3 bytes, big-endian uint24)
	codeSize := uint32(len(code))
	basicData[5] = byte(codeSize >> 16)
	basicData[6] = byte(codeSize >> 8)
	basicData[7] = byte(codeSize)

	if err := tree.Insert(basicDataKey, basicData, nil); err != nil {
		return fmt.Errorf("failed to update BasicData: %v", err)
	}

	return nil
}

// applyBalanceWrites applies balance writes to the Verkle tree
func applyBalanceWrites(address []byte, payload []byte, tree verkle.VerkleNode) error {
	if len(payload) != 16 {
		return fmt.Errorf("invalid balance payload: expected 16 bytes, got %d", len(payload))
	}

	// Balance is stored in BasicData (offset 16-31, 16 bytes)
	// Per EIP-6800: nonce at offset 8, balance at offset 16
	basicDataKey := evmverkle.BasicDataKey(address)
	basicData := make([]byte, 32)

	// Read existing BasicData first (to preserve nonce/code_size)
	existing, err := tree.Get(basicDataKey, nil)
	if err == nil && len(existing) >= 32 {
		copy(basicData, existing[:32])
	}

	// Update balance field (offset 16-31, 16 bytes)
	copy(basicData[16:32], payload[:16])

	if err := tree.Insert(basicDataKey, basicData, nil); err != nil {
		return fmt.Errorf("failed to update BasicData with balance: %v", err)
	}

	return nil
}

// applyNonceWrites applies nonce writes to the Verkle tree
func applyNonceWrites(address []byte, payload []byte, tree verkle.VerkleNode) error {
	if len(payload) != 8 {
		return fmt.Errorf("invalid nonce payload: expected 8 bytes, got %d", len(payload))
	}

	// Nonce is stored in BasicData (offset 8-15, 8 bytes)
	// Per EIP-6800: version at 0-4, code_size at 5-7, nonce at 8-15, balance at 16-31
	basicDataKey := evmverkle.BasicDataKey(address)
	basicData := make([]byte, 32)

	// Read existing BasicData first (to preserve balance/code_size)
	existing, err := tree.Get(basicDataKey, nil)
	if err == nil && len(existing) >= 32 {
		copy(basicData, existing[:32])
	}

	// Update nonce field (offset 8-15, 8 bytes)
	copy(basicData[8:16], payload[:8])

	if err := tree.Insert(basicDataKey, basicData, nil); err != nil {
		return fmt.Errorf("failed to update BasicData with nonce: %v", err)
	}

	return nil
}
