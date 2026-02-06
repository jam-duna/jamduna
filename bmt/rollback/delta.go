package rollback

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/jam-duna/jamduna/bmt/beatree"
)

// Delta represents a reverse delta that can undo a commit.
// It contains the prior values for all keys that were modified or deleted.
type Delta struct {
	// Priors maps each modified key to its prior value.
	// nil value indicates the key did not exist before the commit (should be erased on rollback).
	Priors map[beatree.Key][]byte
}

// NewDelta creates an empty delta.
func NewDelta() *Delta {
	return &Delta{
		Priors: make(map[beatree.Key][]byte),
	}
}

// AddPrior adds a prior value for a key.
// Pass nil value if the key did not exist before.
func (d *Delta) AddPrior(key beatree.Key, value []byte) {
	d.Priors[key] = value
}

// Encode serializes the delta to bytes.
//
// Format:
//   [4 bytes: erase_count (little-endian u32)]
//   [32 bytes each: keys to erase]
//   [4 bytes: reinstate_count (little-endian u32)]
//   For each key to reinstate:
//     [32 bytes: key]
//     [4 bytes: value_len (little-endian u32)]
//     [value_len bytes: value]
func (d *Delta) Encode() []byte {
	// Separate keys into two groups: erase (nil value) and reinstate (non-nil value)
	var toErase []beatree.Key
	var toReinstate []struct {
		key   beatree.Key
		value []byte
	}

	for key, value := range d.Priors {
		if value == nil {
			toErase = append(toErase, key)
		} else {
			toReinstate = append(toReinstate, struct {
				key   beatree.Key
				value []byte
			}{key, value})
		}
	}

	// Estimate buffer size
	bufSize := 4 + len(toErase)*32 + 4
	for _, item := range toReinstate {
		bufSize += 32 + 4 + len(item.value)
	}

	buf := make([]byte, 0, bufSize)
	writer := bytes.NewBuffer(buf)

	// Write erase count
	binary.Write(writer, binary.LittleEndian, uint32(len(toErase)))

	// Write keys to erase
	for _, key := range toErase {
		writer.Write(key[:])
	}

	// Write reinstate count
	binary.Write(writer, binary.LittleEndian, uint32(len(toReinstate)))

	// Write keys to reinstate with their values
	for _, item := range toReinstate {
		writer.Write(item.key[:])
		binary.Write(writer, binary.LittleEndian, uint32(len(item.value)))
		writer.Write(item.value)
	}

	return writer.Bytes()
}

// Decode deserializes a delta from bytes.
func Decode(reader io.Reader) (*Delta, error) {
	delta := NewDelta()

	// Read erase count
	var eraseCount uint32
	if err := binary.Read(reader, binary.LittleEndian, &eraseCount); err != nil {
		return nil, fmt.Errorf("failed to read erase count: %w", err)
	}

	// Read keys to erase
	for i := uint32(0); i < eraseCount; i++ {
		var keyBytes [32]byte
		if _, err := io.ReadFull(reader, keyBytes[:]); err != nil {
			return nil, fmt.Errorf("failed to read erase key %d: %w", i, err)
		}
		key := beatree.KeyFromBytes(keyBytes[:])
		if _, exists := delta.Priors[key]; exists {
			return nil, fmt.Errorf("duplicate key in delta (erase): %x", key)
		}
		delta.Priors[key] = nil
	}

	// Read reinstate count
	var reinstateCount uint32
	if err := binary.Read(reader, binary.LittleEndian, &reinstateCount); err != nil {
		return nil, fmt.Errorf("failed to read reinstate count: %w", err)
	}

	// Read keys to reinstate with values
	for i := uint32(0); i < reinstateCount; i++ {
		var keyBytes [32]byte
		if _, err := io.ReadFull(reader, keyBytes[:]); err != nil {
			return nil, fmt.Errorf("failed to read reinstate key %d: %w", i, err)
		}
		key := beatree.KeyFromBytes(keyBytes[:])

		var valueLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &valueLen); err != nil {
			return nil, fmt.Errorf("failed to read value length for key %d: %w", i, err)
		}

		value := make([]byte, valueLen)
		if _, err := io.ReadFull(reader, value); err != nil {
			return nil, fmt.Errorf("failed to read value for key %d: %w", i, err)
		}

		if _, exists := delta.Priors[key]; exists {
			return nil, fmt.Errorf("duplicate key in delta (reinstate): %x", key)
		}
		delta.Priors[key] = value
	}

	return delta, nil
}
