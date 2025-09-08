package program

import "github.com/colorfulnotion/jam/types"

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
func DecodeCorePart(p []byte) *Program {

	j_size_b, p_remaining := extractBytes(p)
	z_b, p_remaining := extractBytes(p_remaining)
	c_size_b, p_remaining := extractBytes(p_remaining)

	j_size, _ := types.DecodeE(j_size_b)
	z, _ := types.DecodeE(z_b)
	c_size, _ := types.DecodeE(c_size_b)

	j_len := j_size * z
	c_len := c_size

	j_byte := p_remaining[:min(len(p_remaining), int(j_len))]
	c_byte := p_remaining[min(len(p_remaining), int(j_len)):min(len(p_remaining), int(j_len+c_len))]
	k_bytes := p_remaining[min(len(p_remaining), int(j_len+c_len)):]

	var j_array []uint32
	for i := 0; i < len(j_byte); i += int(z) {
		end := min(i+int(z), len(j_byte))
		j_array = append(j_array, uint32(types.DecodeE_l(j_byte[i:end])))
	}

	// build bitmask
	kCombined := expandBits(k_bytes, uint32(c_size))
	return &Program{
		JSize: j_size,
		Z:     uint8(z),
		CSize: c_size,
		J:     j_array,
		Code:  c_byte,
		K:     kCombined,
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
