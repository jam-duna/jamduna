package types

import (
	"encoding/binary"
	"fmt"
)

const RollupValidatorsSnapshotPrefixV1 = "ROLLUP_VALIDATORS_V1:"
const RollupValidatorsSetHashPrefixV1 = "RVH0"
const RollupValidatorsActivationPrefixV1 = "RVA0"
const RollupValidatorsActiveSetIDKeyV1 = "RVSI"
const RollupValidatorsActiveSetHashKeyV1 = "RVSH"

func RollupValidatorsSnapshotKeyV1(setID uint32) []byte {
	return []byte(fmt.Sprintf("%s%d", RollupValidatorsSnapshotPrefixV1, setID))
}

func RollupValidatorsSetHashKeyV1(setID uint32) []byte {
	key := make([]byte, 8)
	copy(key[:4], []byte(RollupValidatorsSetHashPrefixV1))
	binary.LittleEndian.PutUint32(key[4:], setID)
	return key
}

func RollupValidatorsActivationKeyV1(setID uint32) []byte {
	key := make([]byte, 8)
	copy(key[:4], []byte(RollupValidatorsActivationPrefixV1))
	binary.LittleEndian.PutUint32(key[4:], setID)
	return key
}

const rollupValidatorSnapshotEntryLenV1 = 32 + BlsPubInBytes + 8

func EncodeRollupValidatorsSnapshotV1(validators []Validator, weights []uint64) ([]byte, error) {
	if len(validators) == 0 {
		return nil, fmt.Errorf("rollup validators snapshot requires at least one validator")
	}
	if len(weights) != len(validators) {
		return nil, fmt.Errorf("rollup validators snapshot weights length mismatch")
	}

	out := make([]byte, 0, len(validators)*rollupValidatorSnapshotEntryLenV1)
	var weightBuf [8]byte
	for i, v := range validators {
		out = append(out, v.Ed25519[:]...)
		out = append(out, v.Bls[:]...)
		binary.LittleEndian.PutUint64(weightBuf[:], weights[i])
		out = append(out, weightBuf[:]...)
	}
	return out, nil
}

func DecodeRollupValidatorsSnapshotV1(data []byte) ([]Validator, []uint64, error) {
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("rollup validators snapshot empty")
	}
	if len(data)%rollupValidatorSnapshotEntryLenV1 != 0 {
		return nil, nil, fmt.Errorf("rollup validators snapshot length invalid")
	}
	count := len(data) / rollupValidatorSnapshotEntryLenV1
	validators := make([]Validator, count)
	weights := make([]uint64, count)

	offset := 0
	for i := 0; i < count; i++ {
		copy(validators[i].Ed25519[:], data[offset:offset+32])
		offset += 32
		copy(validators[i].Bls[:], data[offset:offset+BlsPubInBytes])
		offset += BlsPubInBytes
		weights[i] = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
	}
	return validators, weights, nil
}
