package types

import (
	"encoding/binary"
	"io"

	"bytes"
)

type StateKeyValue struct {
	Key   [31]byte `json:"key"`
	Len   uint8    `json:"len"`
	Value []byte   `json:"value"`
}

// ToBytes serializes the JAMSNPStateKeyValue struct into a byte array
func (kv *StateKeyValue) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize Key (31 bytes)
	if _, err := buf.Write(kv.Key[:]); err != nil {
		return nil, err
	}

	// Serialize Len (1 byte, uint8)
	if err := buf.WriteByte(kv.Len); err != nil {
		return nil, err
	}

	// Serialize Value length (4 bytes for uint32 length)
	valueLength := uint32(len(kv.Value))
	if err := binary.Write(buf, binary.BigEndian, valueLength); err != nil {
		return nil, err
	}

	// Serialize Value (dynamically sized based on length)
	if _, err := buf.Write(kv.Value); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes deserializes a byte array into a JAMSNPStateKeyValue struct
func (kv *StateKeyValue) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize Key (31 bytes)
	if _, err := io.ReadFull(buf, kv.Key[:]); err != nil {
		return err
	}

	// Deserialize Len (1 byte)
	lenByte, err := buf.ReadByte()
	if err != nil {
		return err
	}
	kv.Len = lenByte

	// Deserialize Value length (4 bytes)
	var valueLength uint32
	if err := binary.Read(buf, binary.BigEndian, &valueLength); err != nil {
		return err
	}

	// Deserialize Value (dynamically sized)
	kv.Value = make([]byte, valueLength)
	if _, err := io.ReadFull(buf, kv.Value); err != nil {
		return err
	}

	return nil
}
