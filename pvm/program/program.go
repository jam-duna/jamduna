package program

import (
	"fmt"

	"github.com/jam-duna/jamduna/types"
)

type Program struct {
	JSize uint64
	Z     uint8
	CSize uint64
	J     []uint32
	Code  []byte
	K     []byte
}

func extractBytes(input []byte) ([]byte, []byte) {
	/*
		In GP_0.36 (272):
		If the input value of (272) is large, "l" will also increase and vice versa.
		"l" is than be used to encode first byte and the reaming "l" bytes.
		If the first byte is large, that means the number of the entire encoded bytes is large and vice versa.
		So the first byte can be used to determine the number of bytes to extract and the rule is as follows:
	*/

	if len(input) == 0 {
		return nil, input
	}

	firstByte := input[0]
	var numBytes int

	// Determine the number of bytes to extract based on the value of the 0th byte.
	switch {
	case firstByte < 128:
		numBytes = 1
	case firstByte >= 128 && firstByte < 192:
		numBytes = 2
	case firstByte >= 192 && firstByte < 224:
		numBytes = 3
	case firstByte >= 224 && firstByte < 240:
		numBytes = 4
	case firstByte >= 240 && firstByte < 248:
		numBytes = 5
	case firstByte >= 248 && firstByte < 252:
		numBytes = 6
	case firstByte >= 252 && firstByte < 254:
		numBytes = 7
	case firstByte >= 254:
		numBytes = 8
	default:
		numBytes = 1
	}

	// If the input length is insufficient to extract the specified number of bytes, return the original input.
	if len(input) < numBytes {
		return input, nil
	}

	// Extract the specified number of bytes and return the remaining bytes.
	extracted := input[:numBytes]
	remaining := input[numBytes:]

	return extracted, remaining
}
func DecodeCorePart(p []byte) (*Program, error) {
	if len(p) == 0 {
		return nil, fmt.Errorf("empty program blob")
	}

	j_size_b, p_remaining := extractBytes(p)
	z_b, p_remaining := extractBytes(p_remaining)
	c_size_b, p_remaining := extractBytes(p_remaining)

	j_size, _ := types.DecodeE(j_size_b)
	z, _ := types.DecodeE(z_b)
	c_size, _ := types.DecodeE(c_size_b)

	j_len := j_size * z
	c_len := c_size

	// Validate that the blob contains enough data for jump table and code
	requiredLen := int(j_len + c_len)
	if len(p_remaining) < requiredLen {
		return nil, fmt.Errorf("code size %d exceeds blob length %d (j_len=%d, c_len=%d, available=%d)",
			c_size, len(p), j_len, c_len, len(p_remaining))
	}

	j_byte := p_remaining[:int(j_len)]
	c_byte := p_remaining[int(j_len):int(j_len+c_len)]
	k_bytes := p_remaining[int(j_len+c_len):]

	var j_array []uint32
	if z > 0 {
		for i := 0; i < len(j_byte); i += int(z) {
			end := min(i+int(z), len(j_byte))
			j_array = append(j_array, uint32(types.DecodeE_l(j_byte[i:end])))
		}
	}

	// build bitmask - now use actual code length
	kCombined := expandBits(k_bytes, uint32(len(c_byte)))
	startBasicBlock := true
	for i, v := range kCombined {
		if v > 0 {
			if startBasicBlock {
				kCombined[i] |= 2
				startBasicBlock = false
			}
			if i < len(c_byte) && IsBasicBlockInstruction(c_byte[i]) {
				startBasicBlock = true
			}
		}
	}
	return &Program{
		JSize: j_size,
		Z:     uint8(z),
		CSize: uint64(len(c_byte)), // Use actual code length, not claimed
		J:     j_array,
		Code:  c_byte,
		K:     kCombined,
	}, nil
}

func IsBasicBlockInstruction(opcode byte) bool {
	switch opcode {
	case 0, 1, 40, 50, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 170, 171, 172, 173, 174, 175, 180:
		return true
	default:
		return false
	}
}

func expandBits(k_bytes []byte, c_size uint32) []byte {
	totalBits := len(k_bytes) * 8
	if totalBits > int(c_size) {
		totalBits = int(c_size)
	}
	kCombined := make([]byte, totalBits)
	bitIndex := 0

	for _, b := range k_bytes {
		if bitIndex+8 <= totalBits {
			kCombined[bitIndex+0] = b & 1
			kCombined[bitIndex+1] = (b >> 1) & 1
			kCombined[bitIndex+2] = (b >> 2) & 1
			kCombined[bitIndex+3] = (b >> 3) & 1
			kCombined[bitIndex+4] = (b >> 4) & 1
			kCombined[bitIndex+5] = (b >> 5) & 1
			kCombined[bitIndex+6] = (b >> 6) & 1
			kCombined[bitIndex+7] = (b >> 7) & 1
			bitIndex += 8
		} else {
			// Handle final partial byte...
			for i := 0; bitIndex < totalBits; i++ {
				kCombined[bitIndex] = (b >> i) & 1
				bitIndex++
			}
			break
		}
	}
	return kCombined
}

func DecodeProgram(p []byte) (*Program, uint32, uint32, uint32, uint32, []byte, []byte, error) {
	pure := p
	if len(pure) < 11 {
		return nil, 0, 0, 0, 0, nil, nil, fmt.Errorf("program blob too short: %d bytes", len(pure))
	}
	// see A.37
	o_size := types.DecodeE_l(pure[:3])
	w_size := types.DecodeE_l(pure[3:6])
	z_val := types.DecodeE_l(pure[6:8])
	s_val := types.DecodeE_l(pure[8:11])
	//fmt.Printf("DecodeProgram: o_size=%d, w_size=%d, z_val=%d, s_val=%d, total_header_size=11\n",o_size, w_size, z_val, s_val)
	var o_byte, w_byte []byte
	offset := uint64(11)
	if offset+o_size <= uint64(len(pure)) {
		o_byte = pure[offset : offset+o_size]
	} else {
		o_byte = make([]byte, o_size)
	}
	offset += o_size

	if offset+w_size <= uint64(len(pure)) {
		w_byte = pure[offset : offset+w_size]
	} else {
		w_byte = make([]byte, w_size)
	}
	offset += w_size

	if offset+4 > uint64(len(pure)) {
		return nil, 0, 0, 0, 0, nil, nil, fmt.Errorf("program blob too short for c_size field")
	}
	c_size := types.DecodeE_l(pure[offset : offset+4])
	offset += 4
	if len(pure[offset:]) != int(c_size) {
		// fmt.Printf("DecodeProgram o_size: %d, w_size: %d, z_val: %d, s_val: %d len(w_byte)=%d\n", o_size, w_size, z_val, s_val, len(w_byte))
		return nil, 0, 0, 0, 0, nil, nil, fmt.Errorf("c_size mismatch: expected %d, got %d", c_size, len(pure[offset:]))
	}
	prog, err := DecodeCorePart(pure[offset:])
	if err != nil {
		return nil, 0, 0, 0, 0, nil, nil, err
	}
	return prog, uint32(o_size), uint32(w_size), uint32(z_val), uint32(s_val), o_byte, w_byte, nil
}
