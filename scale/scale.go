package scale

import (
	"bytes"
	//"errors"
)

// EncodeEmpty encodes an empty sequence
func EncodeEmpty() []byte {
	return []byte{}
}

// EncodeOctetSequence encodes an octet sequence as itself
func EncodeOctetSequence(x []byte) []byte {
	return x
}

// EncodeTuple encodes anonymous tuples by concatenating their encoded elements
func EncodeTuple(elements ...[]byte) []byte {
	return bytes.Join(elements, []byte{})
}

// EncodeInteger encodes a natural number
func EncodeInteger(x uint64) []byte {
	if x == 0 {
		return []byte{0}
	}
	result := []byte{}
	for x > 0 {
		result = append(result, byte(x%256))
		x /= 256
	}
	return result
}

// EncodeGeneralInteger encodes natural numbers of up to 2^64 - 1
func EncodeGeneralInteger(x uint64) ([]byte, error) {
	if x < 1<<7 {
		return EncodeInteger(x), nil
	} else if x < 1<<14 {
		return []byte{byte(0x80 | (x >> 8)), byte(x & 0xFF)}, nil
	} else if x < 1<<29 {
		return []byte{
			byte(0xC0 | (x >> 24)),
			byte((x >> 16) & 0xFF),
			byte((x >> 8) & 0xFF),
			byte(x & 0xFF),
		}, nil
	} else {
		return []byte{
			byte(0xE0 | (x >> 56)),
			byte((x >> 48) & 0xFF),
			byte((x >> 40) & 0xFF),
			byte((x >> 32) & 0xFF),
			byte((x >> 24) & 0xFF),
			byte((x >> 16) & 0xFF),
			byte((x >> 8) & 0xFF),
			byte(x & 0xFF),
		}, nil
	}
}

// EncodeSequence encodes a sequence
func EncodeSequence(seq [][]byte) []byte {
	result := []byte{}
	for _, elem := range seq {
		result = append(result, EncodeOctetSequence(elem)...)
	}
	return result
}

// EncodeDiscriminated encodes a value with a discriminator
func EncodeDiscriminated(value []byte) []byte {
	length := EncodeInteger(uint64(len(value)))
	return append(length, value...)
}

// EncodeBitSequence encodes a bit sequence
func EncodeBitSequence(bits []bool) []byte {
	length := EncodeInteger(uint64(len(bits)))
	byteCount := (len(bits) + 7) / 8
	result := make([]byte, byteCount+1)
	result[0] = byte(len(bits) % 8)
	for i, bit := range bits {
		if bit {
			result[1+(i/8)] |= 1 << (7 - (i % 8))
		}
	}
	return append(length, result...)
}

// EncodeDictionary encodes a dictionary
func EncodeDictionary(dict map[uint64][]byte) []byte {
	result := []byte{}
	for key, value := range dict {
		keyEncoded := EncodeInteger(key)
		valueEncoded := EncodeOctetSequence(value)
		result = append(result, EncodeTuple(keyEncoded, valueEncoded)...)
	}
	return result
}

// EncodeBlock serializes a block as defined in the spec
func EncodeBlock(h, etr, er []byte, t, lv []uint64, tu, cu [][]byte, c map[uint64][]byte) []byte {
	eh := EncodeTuple(h)
	etrEncoded := EncodeTuple(etr)
	erEncoded := EncodeTuple(er)
	ev := EncodeTuple(EncodeSequence(tu))
	ep := EncodeDictionary(c)

	// Building the block B as a tuple
	b := EncodeTuple(eh, etrEncoded, erEncoded, ev, ep)
	return b
}
