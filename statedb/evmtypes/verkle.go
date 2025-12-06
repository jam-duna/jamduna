// Copyright 2023 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package evmtypes

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-verkle"
	"github.com/holiman/uint256"
)

var (
	errInvalidRootType = errors.New("invalid node type for root")
)

// ChunkedCode represents a sequence of 32-bytes chunks of code (31 bytes of which
// are actual code, and 1 byte is the pushdata offset).
type ChunkedCode []byte

// Copy the values here so as to avoid an import cycle
const (
	PUSH1  = byte(0x60)
	PUSH32 = byte(0x7f)
)

// ChunkifyCode generates the chunked version of an array representing EVM bytecode
// This properly handles PUSH instructions that may overflow chunk boundaries.
func ChunkifyCode(code []byte) ChunkedCode {
	var (
		chunkOffset = 0 // offset in the chunk
		chunkCount  = len(code) / 31
		codeOffset  = 0 // offset in the code
	)
	if len(code)%31 != 0 {
		chunkCount++
	}
	chunks := make([]byte, chunkCount*32)
	for i := 0; i < chunkCount; i++ {
		// number of bytes to copy, 31 unless the end of the code has been reached.
		end := 31 * (i + 1)
		if len(code) < end {
			end = len(code)
		}
		copy(chunks[i*32+1:], code[31*i:end]) // copy the code itself

		// chunk offset = taken from the last chunk.
		if chunkOffset > 31 {
			// skip offset calculation if push data covers the whole chunk
			chunks[i*32] = 31
			chunkOffset = 1
			continue
		}
		chunks[32*i] = byte(chunkOffset)
		chunkOffset = 0

		// Check each instruction and update the offset it should be 0 unless
		// a PUSH-N overflows.
		for ; codeOffset < end; codeOffset++ {
			if code[codeOffset] >= PUSH1 && code[codeOffset] <= PUSH32 {
				codeOffset += int(code[codeOffset] - PUSH1 + 1)
				if codeOffset+1 >= 31*(i+1) {
					codeOffset++
					chunkOffset = codeOffset - 31*(i+1)
					break
				}
			}
		}
	}
	return chunks
}

// Constants for verkle tree layout (from EIP-6800)
const (
	VERKLE_NODE_WIDTH        = 256
	BasicDataLeafKey         = 0
	CodeHashLeafKey          = 1
	HEADER_STORAGE_OFFSET    = 64
	CODE_OFFSET              = 128
	BasicDataVersionOffset   = 0
	BasicDataCodeSizeOffset  = 5  // 3 bytes starting at offset 5
	BasicDataNonceOffset     = 8  // 8 bytes
	BasicDataBalanceOffset   = 16 // 16 bytes
)

// Fetch types for hostFetchVerkle
const (
	FETCH_BALANCE   = 0
	FETCH_NONCE     = 1
	FETCH_CODE      = 2
	FETCH_CODE_HASH = 3
	FETCH_STORAGE   = 4
)

// VerkleRead represents a single read operation from the Verkle tree
// This is tracked during EVM execution to build the witness multiproof
type VerkleRead struct {
	VerkleKey   [32]byte // Full 32-byte verkle key (31-byte stem + 1-byte suffix)
	Address     [20]byte // Address being read (for metadata extraction)
	KeyType     uint8    // 0=BasicData, 1=CodeHash, 2=CodeChunk, 3=Storage
	Extra       uint64   // ChunkID for code chunks
	StorageKey  [32]byte // Full storage key for storage reads
	// Note: We do NOT store Value here - BuildVerkleWitness re-reads from tree for security
}

// GetTreeKey computes a verkle tree key using polynomial commitment (Pedersen hash)
// This is the core function for deriving verkle keys from addresses and tree indices.
//
// Parameters:
//   - address: Ethereum address (will be padded to 32 bytes)
//   - treeIndex: Determines which "subtree" (account data, storage, code, etc.)
//   - subIndex: The suffix byte (0-255) within that subtree
//
// Returns: 32-byte verkle tree key (31-byte stem + 1-byte suffix)
func GetTreeKey(address []byte, treeIndex *uint256.Int, subIndex byte) []byte {
	// Ensure address is 32 bytes (pad with zeros on left if needed)
	var addr32 [32]byte
	if len(address) <= 32 {
		copy(addr32[32-len(address):], address)
	} else {
		copy(addr32[:], address[len(address)-32:])
	}

	// Create polynomial with 5 coefficients
	// poly = [0, addr_low, addr_high, treeIndex_low, treeIndex_high]
	var poly [5]verkle.Fr

	// poly[0] stays zero

	// poly[1] = address bytes 0-15 (little-endian)
	if err := verkle.FromLEBytes(&poly[1], addr32[:16]); err != nil {
		panic(err)
	}

	// poly[2] = address bytes 16-31 (little-endian)
	if err := verkle.FromLEBytes(&poly[2], addr32[16:32]); err != nil {
		panic(err)
	}

	// Get tree index as 32 bytes
	treeIndexBytes := treeIndex.Bytes32()

	// poly[3] = tree index bytes 16-31 (big-endian)
	verkle.FromBytes(&poly[3], treeIndexBytes[16:])

	// poly[4] = tree index bytes 0-15 (big-endian)
	verkle.FromBytes(&poly[4], treeIndexBytes[:16])

	// Commit to polynomial using verkle config
	cfg := verkle.GetConfig()
	commitment := cfg.CommitToPoly(poly[:], 0)

	// Add the pre-computed index0Point constant
	commitment.Add(commitment, getIndex0Point())

	// Convert point to hash and set suffix
	hash := verkle.HashPointToBytes(commitment)
	hash[31] = subIndex

	return hash[:]
}

// getIndex0Point returns the commitment to [1,0,0,0,0]
// This is cached for performance
var index0Point *verkle.Point

func getIndex0Point() *verkle.Point {
	if index0Point == nil {
		var poly [5]verkle.Fr
		poly[0].SetOne() // [1, 0, 0, 0, 0]

		cfg := verkle.GetConfig()
		index0Point = cfg.CommitToPoly(poly[:], 0)
	}
	return index0Point
}

// BasicDataKey derives the verkle key for account basic data (suffix 0)
// Per EIP-6800 layout:
//   Offset 0:    version (1 byte)
//   Offset 1-4:  reserved (4 bytes)
//   Offset 5-7:  code_size (3 bytes, big-endian uint24)
//   Offset 8-15: nonce (8 bytes, big-endian uint64)
//   Offset 16-31: balance (16 bytes, big-endian)
func BasicDataKey(address []byte) []byte {
	return GetTreeKey(address, uint256.NewInt(0), BasicDataLeafKey)
}

// CodeHashKey derives the verkle key for account code hash (suffix 1)
func CodeHashKey(address []byte) []byte {
	return GetTreeKey(address, uint256.NewInt(0), CodeHashLeafKey)
}

// CodeChunkKey derives the verkle key for a specific code chunk
// Per EIP-6800: pos = CODE_OFFSET + chunk_id, then tree_index = pos // 256, sub_index = pos % 256
//
// Parameters:
//   - address: Contract address
//   - chunkID: Chunk number (0, 1, 2, ...)
func CodeChunkKey(address []byte, chunkID uint64) []byte {
	// pos = CODE_OFFSET + chunk_id
	pos := CODE_OFFSET + chunkID

	// tree_index = pos // VERKLE_NODE_WIDTH
	treeIndex := uint256.NewInt(pos / VERKLE_NODE_WIDTH)

	// sub_index = pos % VERKLE_NODE_WIDTH
	subIndex := byte(pos % VERKLE_NODE_WIDTH)

	return GetTreeKey(address, treeIndex, subIndex)
}

// StorageSlotKey derives the verkle key for a contract storage slot
// Per EIP-6800: Storage slots 0-63 go to HEADER_STORAGE_OFFSET, 64+ go to MAIN_STORAGE_OFFSET
//
// Parameters:
//   - address: Contract address
//   - storageKey: 32-byte storage key (e.g., from Keccak256 of mapping key)
func StorageSlotKey(address []byte, storageKey []byte) []byte {
	// Convert storage key to uint256
	var storageKeyInt uint256.Int
	storageKeyInt.SetBytes(storageKey)

	// Calculate position based on EIP-6800
	var pos uint256.Int
	threshold := uint256.NewInt(CODE_OFFSET - HEADER_STORAGE_OFFSET) // 64

	if storageKeyInt.Lt(threshold) {
		// For storage slots 0-63: pos = HEADER_STORAGE_OFFSET + storage_key
		pos.SetUint64(HEADER_STORAGE_OFFSET)
		pos.Add(&pos, &storageKeyInt)
	} else {
		// For storage slots 64+: pos = MAIN_STORAGE_OFFSET + storage_key
		// MAIN_STORAGE_OFFSET = 256**31
		mainOffset := new(uint256.Int).Exp(uint256.NewInt(256), uint256.NewInt(31))
		pos.Set(mainOffset)
		pos.Add(&pos, &storageKeyInt)
	}

	// tree_index = pos // VERKLE_NODE_WIDTH
	treeIndex := new(uint256.Int).Div(&pos, uint256.NewInt(VERKLE_NODE_WIDTH))

	// sub_index = pos % VERKLE_NODE_WIDTH
	var subIndexCalc uint256.Int
	subIndexCalc.Mod(&pos, uint256.NewInt(VERKLE_NODE_WIDTH))
	subIndex := byte(subIndexCalc.Uint64())

	return GetTreeKey(address, treeIndex, subIndex)
}

// InsertCodeChunks inserts chunked contract code into the verkle tree
// This uses the ChunkifyCode function to properly handle PUSH instructions
// and batches chunks efficiently per stem.
//
// Parameters:
//   - tree: Verkle tree root node (must be *verkle.InternalNode)
//   - address: Contract address
//   - code: Full contract bytecode
func InsertCodeChunks(tree verkle.VerkleNode, address []byte, code []byte) error {
	if len(code) == 0 {
		return nil
	}

	var (
		chunks = ChunkifyCode(code)
		values [][]byte
		key    []byte
		err    error
	)

	root, ok := tree.(*verkle.InternalNode)
	if !ok {
		return errInvalidRootType
	}

	for i, chunknr := 0, uint64(0); i < len(chunks); i, chunknr = i+32, chunknr+1 {
		groupOffset := (chunknr + 128) % 256
		if groupOffset == 0 /* start of new group */ || chunknr == 0 /* first chunk in header group */ {
			values = make([][]byte, verkle.NodeWidth)
			key = CodeChunkKey(address, chunknr)
		}
		values[groupOffset] = chunks[i : i+32]

		if groupOffset == 255 || len(chunks)-i <= 32 {
			err = root.InsertValuesAtStem(key[:31], values, nil)
			if err != nil {
				return fmt.Errorf("InsertCodeChunks (addr=%x) error: %w", address[:], err)
			}
		}
	}
	return nil
}

// RollBackAccount removes the account info + code from the tree, unlike simple deletion
// that overwrites with zeros. The first 64 storage slots are also removed.
//
// This is useful for reverting failed transactions or handling CREATE2 failures.
func RollBackAccount(tree verkle.VerkleNode, address []byte, codeSize uint32) error {
	root, ok := tree.(*verkle.InternalNode)
	if !ok {
		return errInvalidRootType
	}

	basicDataKey := BasicDataKey(address)

	// Delete the account header + first 64 slots + first 128 code chunks
	_, err := root.DeleteAtStem(basicDataKey[:31], nil)
	if err != nil {
		return fmt.Errorf("error rolling back account header: %w", err)
	}

	// Delete all further code chunks (beyond the first 128)
	for i, chunknr := uint64(31*128), uint64(128); i < uint64(codeSize); i, chunknr = i+31*256, chunknr+256 {
		// Evaluate group key at the start of a new group
		offset := uint256.NewInt(chunknr)
		key := CodeChunkKey(address, offset.Uint64())

		if _, err = root.DeleteAtStem(key[:31], nil); err != nil {
			return fmt.Errorf("error deleting code chunk stem (addr=%x, offset=%d) error: %w", address, offset, err)
		}
	}
	return nil
}

// DeletePrefundedAccount implements the corner case where a prefunded account
// (one with only version=0, empty code hash, and no storage) that is CREATE2-d
// and then SELFDESTRUCT-d should have its balance drained to 0.
//
// This is a workaround until the verkle spec clarifies account deletion behavior.
func DeletePrefundedAccount(tree verkle.VerkleNode, address []byte) error {
	root, ok := tree.(*verkle.InternalNode)
	if !ok {
		return errInvalidRootType
	}

	basicDataKey := BasicDataKey(address)
	values, err := root.GetValuesAtStem(basicDataKey[:31], nil)
	if err != nil {
		return fmt.Errorf("Error getting data at %x in delete: %w", basicDataKey, err)
	}

	// Check if this is a prefunded account
	var prefunded bool
	for i, v := range values {
		switch i {
		case 0:
			prefunded = len(v) == 32
		case 1:
			prefunded = len(v) == 32 && bytes.Equal(v, common.HexToHash("c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470").Bytes())
		default:
			prefunded = v == nil
		}
		if !prefunded {
			break
		}
	}

	if prefunded {
		// Overwrite balance with 0
		var zero [32]byte
		return tree.Insert(basicDataKey, zero[:], nil)
	}

	return nil
}

// GetEmptyCodeHash returns the Keccak256 hash of empty code
func GetEmptyCodeHash() []byte {
	return common.HexToHash("c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470").Bytes()
}

// InsertCode performs a complete code insertion into the Verkle tree
// This includes:
// 1. BasicData update (code_size field at offset 5-7)
// 2. CodeHash insertion at suffix 1
// 3. Code chunks insertion (ChunkifyCode + InsertCodeChunks)
//
// Parameters:
//   - tree: Verkle tree root node
//   - address: Contract address
//   - code: Full contract bytecode
//   - codeHash: Keccak256 hash of the code (32 bytes)
//
// Returns: Error if insertion fails
func InsertCode(tree verkle.VerkleNode, address []byte, code []byte, codeHash []byte) error {
	root, ok := tree.(*verkle.InternalNode)
	if !ok {
		return errInvalidRootType
	}

	// 1. Insert code chunks
	if len(code) > 0 {
		if err := InsertCodeChunks(tree, address, code); err != nil {
			return fmt.Errorf("InsertCode: failed to insert code chunks: %w", err)
		}
	}

	// 2. Insert code hash
	codeHashKey := CodeHashKey(address)
	if err := tree.Insert(codeHashKey, codeHash, nil); err != nil {
		return fmt.Errorf("InsertCode: failed to insert code hash: %w", err)
	}

	// 3. Update BasicData with code_size
	basicDataKey := BasicDataKey(address)

	// Read existing BasicData (to preserve version, nonce, balance)
	values, err := root.GetValuesAtStem(basicDataKey[:31], nil)
	if err != nil {
		// If stem doesn't exist, create new BasicData
		values = make([][]byte, verkle.NodeWidth)
	}

	// Get or create BasicData leaf (suffix 0)
	var basicData [32]byte
	if values[BasicDataLeafKey] != nil && len(values[BasicDataLeafKey]) >= 32 {
		copy(basicData[:], values[BasicDataLeafKey][:32])
	}

	// Update code_size field (offset 5-7, 3 bytes, big-endian uint24)
	codeSize := uint32(len(code))
	basicData[5] = byte(codeSize >> 16)
	basicData[6] = byte(codeSize >> 8)
	basicData[7] = byte(codeSize)

	// Insert updated BasicData
	if err := tree.Insert(basicDataKey, basicData[:], nil); err != nil {
		return fmt.Errorf("InsertCode: failed to update BasicData: %w", err)
	}

	return nil
}

// ReadCode reads contract code from the Verkle tree
// This reads:
// 1. BasicData to get code_size (offset 5-7, 3 bytes big-endian)
// 2. Code chunks based on code_size
// 3. Reconstructs the full code from chunks
//
// Parameters:
//   - tree: Verkle tree root node
//   - address: Contract address
//
// Returns: (code []byte, error)
func ReadCode(tree verkle.VerkleNode, address []byte) ([]byte, error) {
	// 1. Read BasicData to get code_size
	basicDataKey := BasicDataKey(address)
	basicData, err := tree.Get(basicDataKey, nil)
	if err != nil || len(basicData) < 32 {
		// Account doesn't exist or invalid BasicData
		return []byte{}, nil
	}

	// Extract code_size from BasicData (offset 5-7, 3 bytes, big-endian)
	codeSize := uint32(basicData[5])<<16 | uint32(basicData[6])<<8 | uint32(basicData[7])
	if codeSize == 0 {
		// EOA or contract with no code
		return []byte{}, nil
	}

	// 2. Calculate number of chunks needed
	// Each chunk is 31 bytes (see ChunkifyCode)
	numChunks := (codeSize + 30) / 31 // Round up

	// 3. Read code chunks
	code := make([]byte, 0, codeSize)
	for chunkIdx := uint64(0); chunkIdx < uint64(numChunks); chunkIdx++ {
		chunkKey := CodeChunkKey(address, chunkIdx)
		chunkData, err := tree.Get(chunkKey, nil)
		if err != nil {
			return nil, fmt.Errorf("ReadCode: failed to read chunk %d: %w", chunkIdx, err)
		}

		// Each chunk is 32 bytes, but only first 31 contain code (first byte is push data length)
		if len(chunkData) < 32 {
			return nil, fmt.Errorf("ReadCode: chunk %d has invalid size %d", chunkIdx, len(chunkData))
		}

		// Append chunk data (skip first byte which is PUSHDATA length marker)
		chunkSize := 31
		if uint32(len(code))+31 > codeSize {
			chunkSize = int(codeSize) - len(code)
		}
		code = append(code, chunkData[1:1+chunkSize]...)
	}

	return code, nil
}

// DeleteAccount removes all account data from the Verkle tree
// This includes:
// 1. BasicData (version, nonce, balance, code_size, code_hash)
// 2. First 64 storage slots (HEADER_STORAGE_OFFSET range)
// 3. All code chunks
//
// This is more thorough than RollBackAccount and is used for SELFDESTRUCT.
//
// Parameters:
//   - tree: Verkle tree root node
//   - address: Contract address
//   - codeSize: Size of contract code (needed to delete all code chunks)
//
// Returns: Error if deletion fails
func DeleteAccount(tree verkle.VerkleNode, address []byte, codeSize uint32) error {
	root, ok := tree.(*verkle.InternalNode)
	if !ok {
		return errInvalidRootType
	}

	basicDataKey := BasicDataKey(address)

	// Delete the account header + first 64 storage slots + first 128 code chunks
	// All are in the same stem (suffix 0-255)
	_, err := root.DeleteAtStem(basicDataKey[:31], nil)
	if err != nil {
		return fmt.Errorf("DeleteAccount: failed to delete account header stem: %w", err)
	}

	// Delete all further code chunks (beyond the first 128)
	// Code chunks start at CODE_OFFSET (128) and continue in groups of 256
	if codeSize > 31*128 {
		for i, chunknr := uint64(31*128), uint64(128); i < uint64(codeSize); i, chunknr = i+31*256, chunknr+256 {
			offset := uint256.NewInt(chunknr)
			key := CodeChunkKey(address, offset.Uint64())

			if _, err := root.DeleteAtStem(key[:31], nil); err != nil {
				return fmt.Errorf("DeleteAccount: failed to delete code chunk stem (addr=%x, chunk=%d): %w", address, chunknr, err)
			}
		}
	}

	return nil
}

// GenerateVerkleProof generates a multiproof for a set of keys
//
// This wraps the go-verkle proof generation API and returns:
// - Serialized proof bytes
// - Keys that were proven
// - Values for those keys (from the tree parameter)
//
// Parameters:
//   - tree: Verkle tree root node (for read-only proofs, pass same tree for both params)
//   - keys: List of 32-byte verkle keys to prove
//
// Returns: (proof, keys, values, error)
func GenerateVerkleProof(tree verkle.VerkleNode, keys [][]byte) ([]byte, [][]byte, [][]byte, error) {
	return GenerateVerkleProofWithTransition(tree, tree, keys)
}

// GenerateVerkleProofWithTransition generates a multiproof for a state transition
//
// This wraps the go-verkle proof generation API for state transitions and returns:
// - Serialized proof bytes
// - Keys that were proven
// - Post-state values for those keys
//
// Parameters:
//   - preTree: Verkle tree root node BEFORE state transition
//   - postTree: Verkle tree root node AFTER state transition
//   - keys: List of 32-byte verkle keys to prove
//
// Returns: (proof, keys, postValues, error)
func GenerateVerkleProofWithTransition(preTree verkle.VerkleNode, postTree verkle.VerkleNode, keys [][]byte) ([]byte, [][]byte, [][]byte, error) {
	if len(keys) == 0 {
		return nil, nil, nil, fmt.Errorf("GenerateVerkleProofWithTransition: empty key list")
	}

	if preTree == nil || postTree == nil {
		return nil, nil, nil, fmt.Errorf("GenerateVerkleProofWithTransition: nil tree")
	}

	// Collect post-values from post-tree
	values := make([][]byte, len(keys))
	for i, key := range keys {
		val, err := postTree.Get(key, nil)
		if err != nil {
			// Missing keys get nil value
			values[i] = nil
		} else {
			values[i] = val
		}
	}

	// Generate proof using go-verkle v0.2.2 API
	// MakeVerkleMultiProof(preroot, postroot, keys, resolver) returns proof, commitments, indices, fr values, error
	proof, _, _, _, err := verkle.MakeVerkleMultiProof(preTree, postTree, keys, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("GenerateVerkleProofWithTransition: MakeVerkleMultiProof failed: %w", err)
	}

	// Serialize proof to VerkleProof format using package-level function
	vp, _, err := verkle.SerializeProof(proof)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("GenerateVerkleProofWithTransition: proof serialization failed: %w", err)
	}

	// Marshal VerkleProof to JSON bytes
	proofBytes, err := vp.MarshalJSON()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("GenerateVerkleProofWithTransition: proof marshaling failed: %w", err)
	}

	return proofBytes, keys, values, nil
}

// VerifyVerkleProof verifies a Verkle multiproof
//
// This wraps the go-verkle proof verification API.
//
// Parameters:
//   - proof: Serialized proof bytes
//   - root: Expected state root (32 bytes)
//   - keys: List of keys being proven
//   - values: Expected values for those keys
//
// Returns: true if proof is valid, false otherwise
func VerifyVerkleProof(proof []byte, root []byte, keys [][]byte, values [][]byte) (bool, error) {
	if len(keys) != len(values) {
		return false, fmt.Errorf("VerifyVerkleProof: keys and values length mismatch")
	}

	if len(root) != 32 {
		return false, fmt.Errorf("VerifyVerkleProof: invalid root length %d, expected 32", len(root))
	}

	if len(proof) == 0 {
		return false, fmt.Errorf("VerifyVerkleProof: empty proof")
	}

	// Unmarshal VerkleProof from JSON bytes
	var vp verkle.VerkleProof
	if err := vp.UnmarshalJSON(proof); err != nil {
		return false, fmt.Errorf("VerifyVerkleProof: proof unmarshaling failed: %w", err)
	}

	// Build StateDiff for verification
	// For read-only witness (Guarantor mode), we need to build state diff from keys/values
	// IMPORTANT: Must build in deterministic order matching proof generation (sorted keys)
	statediff := make(verkle.StateDiff, 0)

	// Group keys by stem, preserving input order
	type stemInfo struct {
		stem        [31]byte
		suffixDiffs []verkle.SuffixStateDiff
	}
	stemMap := make(map[[31]byte]*stemInfo)
	stemOrder := make([][31]byte, 0)

	for i, key := range keys {
		if len(key) != 32 {
			return false, fmt.Errorf("VerifyVerkleProof: invalid key length %d", len(key))
		}

		var stem [31]byte
		copy(stem[:], key[:31])
		suffix := key[31]

		if stemMap[stem] == nil {
			stemMap[stem] = &stemInfo{
				stem:        stem,
				suffixDiffs: make([]verkle.SuffixStateDiff, 0),
			}
			stemOrder = append(stemOrder, stem)
		}

		// Convert value to *[32]byte
		var val32 [32]byte
		if values[i] != nil {
			copy(val32[:], values[i])
		}

		stemMap[stem].suffixDiffs = append(stemMap[stem].suffixDiffs, verkle.SuffixStateDiff{
			Suffix:       suffix,
			CurrentValue: &val32,
			NewValue:     &val32, // Read-only: pre == post
		})
	}

	// Build statediff in deterministic order (order of first occurrence in keys slice)
	for _, stem := range stemOrder {
		info := stemMap[stem]
		statediff = append(statediff, verkle.StemStateDiff{
			Stem:        info.stem,
			SuffixDiffs: info.suffixDiffs,
		})
	}

	// Verify using go-verkle v0.2.2 API
	// Verify(vp, preStateRoot, postStateRoot, statediff) returns error
	// For this simplified API, we verify against a single root (no state transition)
	// Both pre and post are set to the same root
	err := verkle.Verify(&vp, root, root, statediff)
	if err != nil {
		return false, fmt.Errorf("VerifyVerkleProof: verification failed: %w", err)
	}

	return true, nil
}

// VerifyVerkleProofWithTransition verifies a Verkle multiproof for a state transition
//
// This wraps the go-verkle proof verification API for state transitions.
//
// Parameters:
//   - proof: Serialized proof bytes
//   - preRoot: Pre-state root (32 bytes)
//   - postRoot: Post-state root (32 bytes)
//   - keys: List of keys being proven
//   - preValues: Pre-state values for those keys
//   - postValues: Post-state values for those keys
//
// Returns: true if proof is valid, false otherwise
func VerifyVerkleProofWithTransition(proof []byte, preRoot []byte, postRoot []byte, keys [][]byte, preValues [][]byte, postValues [][]byte) (bool, error) {
	if len(keys) != len(preValues) || len(keys) != len(postValues) {
		return false, fmt.Errorf("VerifyVerkleProofWithTransition: keys and values length mismatch")
	}

	if len(preRoot) != 32 || len(postRoot) != 32 {
		return false, fmt.Errorf("VerifyVerkleProofWithTransition: invalid root length")
	}

	if len(proof) == 0 {
		return false, fmt.Errorf("VerifyVerkleProofWithTransition: empty proof")
	}

	// Unmarshal VerkleProof from JSON bytes
	var vp verkle.VerkleProof
	if err := vp.UnmarshalJSON(proof); err != nil {
		return false, fmt.Errorf("VerifyVerkleProofWithTransition: proof unmarshaling failed: %w", err)
	}

	// Build StateDiff for verification
	// IMPORTANT: Must build in deterministic order matching proof generation (sorted keys)
	// Go map iteration is random, causing intermittent "proof of absence (empty) stem has a value" errors
	statediff := make(verkle.StateDiff, 0)

	// Group keys by stem, preserving input order
	type stemInfo struct {
		stem        [31]byte
		suffixDiffs []verkle.SuffixStateDiff
		firstIndex  int // Track first occurrence for stable sorting
	}
	stemMap := make(map[[31]byte]*stemInfo)
	stemOrder := make([][31]byte, 0) // Track stem order

	for i, key := range keys {
		if len(key) != 32 {
			return false, fmt.Errorf("VerifyVerkleProofWithTransition: invalid key length %d", len(key))
		}

		var stem [31]byte
		copy(stem[:], key[:31])
		suffix := key[31]

		if stemMap[stem] == nil {
			stemMap[stem] = &stemInfo{
				stem:        stem,
				suffixDiffs: make([]verkle.SuffixStateDiff, 0),
				firstIndex:  i,
			}
			stemOrder = append(stemOrder, stem)
		}

		// Convert pre and post values to *[32]byte
		var preValPtr, postValPtr *[32]byte

		if preValues[i] != nil {
			preVal32 := new([32]byte)
			copy(preVal32[:], preValues[i])
			preValPtr = preVal32
		} else {
			preValPtr = nil // Key was absent in pre-state
		}

		if postValues[i] != nil {
			postVal32 := new([32]byte)
			copy(postVal32[:], postValues[i])
			postValPtr = postVal32
		} else {
			postValPtr = nil // Key is absent in post-state
		}

		stemMap[stem].suffixDiffs = append(stemMap[stem].suffixDiffs, verkle.SuffixStateDiff{
			Suffix:       suffix,
			CurrentValue: preValPtr,
			NewValue:     postValPtr,
		})
	}

	// Build statediff in deterministic order (order of first occurrence in keys slice)
	for _, stem := range stemOrder {
		info := stemMap[stem]
		statediff = append(statediff, verkle.StemStateDiff{
			Stem:        info.stem,
			SuffixDiffs: info.suffixDiffs,
		})
	}

	// Verify using go-verkle v0.2.2 API with BOTH roots
	// Verify(vp, preStateRoot, postStateRoot, statediff) returns error
	err := verkle.Verify(&vp, preRoot, postRoot, statediff)
	if err != nil {
		return false, fmt.Errorf("VerifyVerkleProofWithTransition: verification failed: %w", err)
	}

	return true, nil
}
