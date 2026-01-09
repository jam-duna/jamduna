package storage

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/colorfulnotion/jam/common"
	"github.com/zeebo/blake3"
)

const (
	BASIC_DATA_LEAF_KEY = uint8(0)
	CODE_HASH_LEAF_KEY  = uint8(1)

	HEADER_STORAGE_OFFSET = uint8(64)
	CODE_OFFSET           = uint8(128)

	STEM_SUBTREE_WIDTH = uint64(256)

	BASIC_DATA_CODE_SIZE_OFFSET = 5
	BASIC_DATA_NONCE_OFFSET     = 8
	BASIC_DATA_BALANCE_OFFSET   = 16
)

var zeroPrefix = [12]byte{}

// DeriveBinaryTreeKeyEIP derives a tree key using SHA-256.
// key = sha256(zeroPrefix || address || inputKey[:31]); subindex preserved.
func DeriveBinaryTreeKeyEIP(address common.Address, inputKey [32]byte) TreeKey {
	var payload [12 + 20 + 31]byte
	copy(payload[:12], zeroPrefix[:])
	addr := [20]byte(address)
	copy(payload[12:32], addr[:])
	copy(payload[32:], inputKey[:31])

	hash := sha256.Sum256(payload[:])
	var stem Stem
	copy(stem[:], hash[:31])

	return TreeKey{
		Stem:     stem,
		Subindex: inputKey[31],
	}
}

// DeriveBinaryTreeKeyJAM derives a tree key using Blake3.
// key = blake3(zeroPrefix || address || inputKey[:31]); subindex preserved.
func DeriveBinaryTreeKeyJAM(address common.Address, inputKey [32]byte) TreeKey {
	var payload [12 + 20 + 31]byte
	copy(payload[:12], zeroPrefix[:])
	addr := [20]byte(address)
	copy(payload[12:32], addr[:])
	copy(payload[32:], inputKey[:31])

	hash := blake3.Sum256(payload[:])
	var stem Stem
	copy(stem[:], hash[:31])

	return TreeKey{
		Stem:     stem,
		Subindex: inputKey[31],
	}
}

// GetBinaryTreeKey derives a tree key for the provided address and input key.
func GetBinaryTreeKey(profile Profile, address common.Address, inputKey [32]byte) TreeKey {
	if profile == EIPProfile {
		return DeriveBinaryTreeKeyEIP(address, inputKey)
	}
	return DeriveBinaryTreeKeyJAM(address, inputKey)
}

// GetBasicDataKey returns the key for basic account data.
func GetBasicDataKey(profile Profile, address common.Address) TreeKey {
	var k [32]byte
	k[31] = BASIC_DATA_LEAF_KEY
	return GetBinaryTreeKey(profile, address, k)
}

// GetCodeHashKey returns the key for the code hash leaf.
func GetCodeHashKey(profile Profile, address common.Address) TreeKey {
	var k [32]byte
	k[31] = CODE_HASH_LEAF_KEY
	return GetBinaryTreeKey(profile, address, k)
}

// GetStorageSlotKey returns the tree key for an EVM storage slot.
func GetStorageSlotKey(profile Profile, address common.Address, slot [32]byte) TreeKey {
	var k [32]byte
	isHeaderSlot := true
	for i := 0; i < 31; i++ {
		if slot[i] != 0 {
			isHeaderSlot = false
			break
		}
	}
	if isHeaderSlot && slot[31] < HEADER_STORAGE_OFFSET {
		k[31] = HEADER_STORAGE_OFFSET + slot[31]
		return GetBinaryTreeKey(profile, address, k)
	}

	for i := 1; i < 31; i++ {
		k[i] = slot[i]
	}
	k[0] = slot[0] + 1
	k[31] = slot[31]
	return GetBinaryTreeKey(profile, address, k)
}

// GetCodeChunkKey returns the tree key for a code chunk.
func GetCodeChunkKey(profile Profile, address common.Address, chunkNumber uint64) TreeKey {
	pos := uint64(CODE_OFFSET) + chunkNumber
	if pos < STEM_SUBTREE_WIDTH {
		var k [32]byte
		k[31] = uint8(pos)
		return GetBinaryTreeKey(profile, address, k)
	}

	stemIndex := pos / STEM_SUBTREE_WIDTH
	subindex := uint8(pos % STEM_SUBTREE_WIDTH)

	var k [32]byte
	var stemBytes [8]byte
	binary.BigEndian.PutUint64(stemBytes[:], stemIndex)
	copy(k[23:31], stemBytes[:])
	k[31] = subindex
	return GetBinaryTreeKey(profile, address, k)
}

// BasicDataLeaf packs basic account data into a 32-byte value.
type BasicDataLeaf struct {
	Version  uint8
	CodeSize uint32
	Nonce    uint64
	Balance  [16]byte
}

func NewBasicDataLeaf(nonce uint64, balance [16]byte, codeSize uint32) BasicDataLeaf {
	return BasicDataLeaf{
		Version:  0,
		CodeSize: codeSize,
		Nonce:    nonce,
		Balance:  balance,
	}
}

func (b BasicDataLeaf) Encode() [32]byte {
	var out [32]byte
	out[0] = b.Version
	var codeSizeBytes [4]byte
	binary.BigEndian.PutUint32(codeSizeBytes[:], b.CodeSize)
	copy(out[5:8], codeSizeBytes[1:4])
	binary.BigEndian.PutUint64(out[8:16], b.Nonce)
	copy(out[16:32], b.Balance[:])
	return out
}

func DecodeBasicDataLeaf(value [32]byte) BasicDataLeaf {
	var codeSizeBytes [4]byte
	copy(codeSizeBytes[1:4], value[5:8])

	var balance [16]byte
	copy(balance[:], value[16:32])

	return BasicDataLeaf{
		Version:  value[0],
		CodeSize: binary.BigEndian.Uint32(codeSizeBytes[:]),
		Nonce:    binary.BigEndian.Uint64(value[8:16]),
		Balance:  balance,
	}
}
