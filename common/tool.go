package common

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// This is ONLY used to setup levelDB paths
func ComputeCurrentTS() uint32 {
	currentTime := time.Now().Unix()
	return uint32(currentTime)
}

func ComputeTimeSlot(TimeUnitMode string) uint32 {
	secondsPerSlot := uint32(6)
	currentTime := time.Now().Unix()
	JCE := uint32(ComputeJCETime(currentTime, true))
	timeslot := JCE / secondsPerSlot
	return timeslot
}

func ComputeTimeUnit(TimeUnitMode string) uint32 {
	unit := ComputeTimeSlot(TimeUnitMode)
	if TimeUnitMode == "JAM" {
		unit = ComputeTimeSlot(TimeUnitMode)
	} else if TimeUnitMode == "Raw" {
		panic("Raw time unit mode is not supported")
	}
	return unit
}

var JceStart = time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)

func AddJamStart(time time.Duration) {
	JceStart = JceStart.Add(time)
}

// Jam Common Era: 173568960 or Jan 01 2025 12:00:00 GMT+0000; See section 4.4
func ComputeJCETime(unixTimestamp int64, production bool) int64 {
	if production {
		// Define the start of the Jam Common Era -- JceStart

		// Convert the Unix timestamp to a Time object
		currentTime := time.Unix(unixTimestamp, 0).UTC()

		// Calculate the difference in seconds
		diff := currentTime.Sub(JceStart)
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

func CompareKeys(b1, b2 []byte) int {
	// Find the minimum length of the two slices
	minLen := len(b1)
	if len(b2) < minLen {
		minLen = len(b2)
	}

	// Compare byte by byte
	for i := 0; i < minLen; i++ {
		if b1[i] < b2[i] {
			return -1 // b1 is smaller than b2
		} else if b1[i] > b2[i] {
			return 1 // b1 is greater than b2
		}
	}

	// If all compared bytes are equal, compare lengths
	if len(b1) < len(b2) {
		return -1 // b1 is smaller than b2
	} else if len(b1) > len(b2) {
		return 1 // b1 is greater than b2
	}

	// If lengths are also equal, the slices are identical
	return 0
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

// Justification = [0 ++ Hash OR 1 ++ Hash ++ Hash OR 2 ++ Segment Shard] (Each discriminator is a single byte)
func EncodeJustification(path [][]byte, numECPiecesPerSegment int) ([]byte, error) {
	if len(path) == 0 {
		return []byte{}, nil
	}
	var combined []byte
	for i, h := range path {
		if len(h) != 32 && len(h) != 64 && len(h) != 2*numECPiecesPerSegment {
			return nil, fmt.Errorf("invalid length %d at index %d; expected 32 or 64 or %d", len(h), i, 2*numECPiecesPerSegment)
		}
		if len(h) == 32 {
			combined = append(combined, 0x00)
			combined = append(combined, h...)
		} else if len(h) == 64 {
			combined = append(combined, 0x01)
			combined = append(combined, h...)
		} else if len(h) == 2*numECPiecesPerSegment {
			// For marker 0x02, the segment shard must be exactly shardSize bytes.
			combined = append(combined, 0x02)
			combined = append(combined, h...)
		}
	}
	return combined, nil
}

func DecodeJustification(compact []byte, numECPiecesPerSegment int) ([][]byte, error) {
	if len(compact) == 0 {
		return [][]byte{}, nil
	}
	var path [][]byte
	i := 0
	for i < len(compact) {
		if i+1 > len(compact) {
			return nil, errors.New("unexpected end of data: missing marker")
		}
		marker := compact[i]
		switch marker {
		case 0x00:
			// Marker 0x00 indicates a 32-byte hash.
			if i+1+32 > len(compact) {
				return nil, fmt.Errorf("unexpected end of data for 32-byte hash at position %d", i)
			}
			h := make([]byte, 32)
			copy(h, compact[i+1:i+1+32])
			path = append(path, h)
			i += 1 + 32
		case 0x01:
			// Marker 0x01 indicates a 64-byte hash.
			if i+1+64 > len(compact) {
				return nil, fmt.Errorf("unexpected end of data for 64-byte hash at position %d", i)
			}
			h := make([]byte, 64)
			copy(h, compact[i+1:i+1+64])
			path = append(path, h)
			i += 1 + 64
		case 0x02:
			// Marker 0x02 indicates a segment shard.
			if i+1+2*numECPiecesPerSegment > len(compact) {
				return nil, fmt.Errorf("unexpected end of data for segment shard at position %d", i)
			}
			shard := make([]byte, 2*numECPiecesPerSegment)
			copy(shard, compact[i+1:i+1+2*numECPiecesPerSegment])
			path = append(path, shard)
			i += 1 + 2*numECPiecesPerSegment
		default:
			return nil, fmt.Errorf("invalid marker 0x%x at position %d", marker, i)
		}
	}
	if i != len(compact) {
		return nil, errors.New("extra data found after decoding justification")
	}
	return path, nil
}

func GetFilePath(fn string) string {
	// Use environment variable JAM_PATH, but if its not set, use
	basePath := os.Getenv("JAM_PATH")
	if basePath == "" {
		basePath = "/root/go/src/github.com/colorfulnotion/jam/"
	}

	// Construct the full file path using filepath package
	return filepath.Join(basePath, fn)
}

func Uint16Contains(slice []uint16, value uint16) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func StrContains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func HashContains(slice []Hash, value Hash) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

func FormatPaddedBytes(data []byte, minLen int) string {
	if len(data) < minLen {
		return "0x" + fmt.Sprintf("%x", data)
	}

	hexStr := fmt.Sprintf("%x", data[:minLen])
	if len(data) > minLen {
		hexStr += "..."
	}
	return "0x" + hexStr
}

func FormatPaddedBytesArray(data [][]byte, minLen int) []string {
	formattedList := make([]string, len(data))
	for i, d := range data {
		formattedList[i] = FormatPaddedBytes(d, minLen)
	}
	return formattedList
}

func FormatPaddedBytes3D(data [][][]byte, minLen int) [][]string {
	formatted := make([][]string, len(data))
	for i, innerSlice := range data {
		formatted[i] = FormatPaddedBytesArray(innerSlice, minLen)
	}
	return formatted
}

func BytesToHexStr(v interface{}) interface{} {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr:
		if rv.IsNil() {
			return nil
		}
		return BytesToHexStr(rv.Elem().Interface())
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			if b, ok := v.([]byte); ok {
				return "0x" + hex.EncodeToString(b)
			}
		}
		result := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = BytesToHexStr(rv.Index(i).Interface())
		}
		return result
	case reflect.Map:
		result := make(map[string]interface{})
		for _, key := range rv.MapKeys() {
			keyStr := fmt.Sprintf("%v", key.Interface())
			result[keyStr] = BytesToHexStr(rv.MapIndex(key).Interface())
		}
		return result
	case reflect.Struct:
		result := make(map[string]interface{})
		rt := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			field := rt.Field(i)
			if field.PkgPath != "" {
				continue
			}
			result[field.Name] = BytesToHexStr(rv.Field(i).Interface())
		}
		return result
	default:
		return v
	}
}
