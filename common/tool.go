package common

import (
	"encoding/binary"
	"fmt"
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
