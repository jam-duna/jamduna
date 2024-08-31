package codec

import (
	"bytes"
	"fmt"
)

// Encode serializes the given object using the JAM codec rules.
func Encode(obj interface{}) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	encoder := NewEncoder(buffer)

	err := encoder.Encode(obj)
	if err != nil {
		return nil, fmt.Errorf("encoding failed: %w", err)
	}

	return buffer.Bytes(), nil
}

// Decode deserializes the given byte slice into an object of the specified type using the JAM codec.
func Decode(inp []byte, typ interface{}) (interface{}, error) {
	decoder := NewDecoder(bytes.NewReader(inp))

	err := decoder.Decode(typ)
	if err != nil {
		return nil, fmt.Errorf("decoding failed: %w", err)
	}

	return typ, nil
}
