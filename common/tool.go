package common

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"time"
)

// encodeUint64 encodes a uint64 value into a byte slice in LittleEndian order
func EncodeUint64(num uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, num)
	return buf
}

// decodeUint64 decodes a byte slice into an int value in LittleEndian order
func DecodeUint64(data []byte) int {
	if len(data) != 8 {
		fmt.Println("Invalid byte slice length")
		return 0
	}
	return int(binary.LittleEndian.Uint64(data))
}

// This is FAKE
func ComputeCurrentJCETime() uint32 {
	currentTime := time.Now().Unix()
	return uint32(currentTime) // computeJCETime(currentTime)
}

func ComputeRealCurrentJCETime(TimeUnitMode string) uint32 {
	currentTime := time.Now().Unix()
	if TimeUnitMode == "TimeStamp" {
		return uint32(ComputeJCETime(currentTime, false))
	} else {
		return uint32(ComputeJCETime(currentTime, true))
	}
}

// here if I use types.SecondPerSlot, it will be a circular import
func ComputeTimeSlot(TimeUnitMode string) uint32 {
	currentTime := time.Now().Unix()
	JCE := uint32(ComputeJCETime(currentTime, true))
	timeslot := JCE / 6
	return timeslot
}

func ComputeTimeUnit(TimeUnitMode string) uint32 {
	unit := ComputeTimeSlot(TimeUnitMode)
	if TimeUnitMode == "TimeStamp" {
		unit = ComputeRealCurrentJCETime(TimeUnitMode)
	}
	return unit
}

// Jam Common Era: 1704110400 or Jan 01 2024 12:00:00 GMT+0000; See section 4.4
func ComputeJCETime(unixTimestamp int64, production bool) int64 {
	if production {
		// Define the start of the Jam Common Era
		jceStart := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)

		// Convert the Unix timestamp to a Time object
		currentTime := time.Unix(unixTimestamp, 0).UTC()

		// Calculate the difference in seconds
		diff := currentTime.Sub(jceStart)
		return int64(diff.Seconds())
	} else {
		return unixTimestamp
	}
}

func ComputeCurrenTS() uint32 {
	currentTime := time.Now().Unix()
	return uint32(currentTime)
}

func CompareBytes(b1 []byte, b2 []byte) bool {
	return bytes.Equal(b1, b2)
}

func FalseBytes(data []byte) []byte {
	result := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		result[i] = 0xFF - data[i]
		// result[i] = ^data[i]
	}
	return result
}

func ConvertToSlice(arr interface{}) []byte {
	// Use reflection to handle different lengths of fixed-length arrays
	v := reflect.ValueOf(arr)

	if v.Kind() != reflect.Array {
		panic("input is not an array")
	}

	// Convert to a byte slice
	byteSlice := make([]byte, v.Len())
	for i := 0; i < v.Len(); i++ {
		byteSlice[i] = byte(v.Index(i).Uint())
	}

	return byteSlice
}

// ConcatenateByteSlices concatenates a slice of byte slices into a single byte slice
func ConcatenateByteSlices(slices [][]byte) []byte {
	// Calculate the total length of the concatenated byte slice
	totalLen := 0
	for _, b := range slices {
		totalLen += len(b)
	}

	// Create a single byte slice with the total length
	result := make([]byte, 0, totalLen)

	// Append each byte slice to the result
	for _, b := range slices {
		result = append(result, b...)
	}

	return result
}

func CompactPath(path []Hash) []byte {
    combined := make([]byte, 0)
    for _, h := range path {
        combined = append(combined, h[:]...)
    }
    return combined
}

func ExpandPath(compact []byte) ([]Hash, error) {
    if len(compact)%32 != 0 {
        return nil, fmt.Errorf("invalid compact path length, must be a multiple of 32")
    }
    hashCount := len(compact) / 32
    hashes := make([]Hash, hashCount)
    for i := 0; i < hashCount; i++ {
        var hash Hash
        copy(hash[:], compact[i*32:(i+1)*32]) // Copy 32 bytes into the hash
        hashes[i] = hash
    }
    return hashes, nil
}
