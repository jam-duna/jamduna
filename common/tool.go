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

func ComputeRealCurrentJCETime() uint32 {
	currentTime := time.Now().Unix()
	return uint32(ComputeJCETime(currentTime))
}

// The current time expressed in seconds after the start of the Jam Common Era. See section 4.4
func ComputeJCETime(unixTimestamp int64) int64 {
	production := false
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
