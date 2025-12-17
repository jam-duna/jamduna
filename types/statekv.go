package types

import (
	"bytes"
	"io"
)

type StateKeyValueList struct {
	Items []StateKeyValue
}

type StateKeyValue struct {
	Key   [31]byte `json:"key"`
	Value []byte   `json:"value"`
}

// ToBytes serializes the StateKeyValueList into a byte array
// Per GP spec CE129: Key = [u8; 31], Value = len++[u8] (JAM variable-length encoding)
func (kvs *StateKeyValueList) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)
	for _, kv := range kvs.Items {
		// Serialize Key (31 bytes)
		if _, err := buf.Write(kv.Key[:]); err != nil {
			return nil, err
		}

		// Serialize Value with JAM's len++[u8] encoding (variable-length prefix)
		lenBytes := E(uint64(len(kv.Value)))
		if _, err := buf.Write(lenBytes); err != nil {
			return nil, err
		}

		// Serialize Value (dynamically sized based on length)
		if _, err := buf.Write(kv.Value); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a StateKeyValue struct
// Per GP spec CE129: Key = [u8; 31], Value = len++[u8] (JAM variable-length encoding)
func (kv *StateKeyValue) FromBytes(data []byte) error {
	if len(data) < 31 {
		return io.ErrUnexpectedEOF
	}

	// Deserialize Key (31 bytes)
	copy(kv.Key[:], data[:31])
	data = data[31:]

	// Deserialize Value using JAM's len++[u8] encoding
	valueLength, bytesRead := DecodeE(data)
	if bytesRead == 0 {
		return io.ErrUnexpectedEOF
	}
	data = data[bytesRead:]

	if uint64(len(data)) < valueLength {
		return io.ErrUnexpectedEOF
	}

	kv.Value = make([]byte, valueLength)
	copy(kv.Value, data[:valueLength])

	return nil
}

// FromBytes deserializes a byte array into a StateKeyValueList
// Per GP spec CE129: Key = [u8; 31], Value = len++[u8] (JAM variable-length encoding)
func (kvs *StateKeyValueList) FromBytes(data []byte) error {
	kvs.Items = nil

	for len(data) > 0 {
		var kv StateKeyValue

		// Need at least 31 bytes for the key
		if len(data) < 31 {
			break
		}

		// Deserialize Key (31 bytes)
		copy(kv.Key[:], data[:31])
		data = data[31:]

		// Deserialize Value using JAM's len++[u8] encoding (variable-length prefix)
		valueLength, bytesRead := DecodeE(data)
		if bytesRead == 0 {
			return io.ErrUnexpectedEOF
		}
		data = data[bytesRead:]

		if uint64(len(data)) < valueLength {
			return io.ErrUnexpectedEOF
		}

		kv.Value = make([]byte, valueLength)
		copy(kv.Value, data[:valueLength])
		data = data[valueLength:]

		kvs.Items = append(kvs.Items, kv)
	}

	return nil
}

// LastKey returns the last key in the list, or nil if empty
func (kvs *StateKeyValueList) LastKey() *[31]byte {
	if len(kvs.Items) == 0 {
		return nil
	}
	return &kvs.Items[len(kvs.Items)-1].Key
}
