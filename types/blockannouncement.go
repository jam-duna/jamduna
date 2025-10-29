package types

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/colorfulnotion/jam/common"
)

type BlockAnnouncement struct {
	HeaderHash     common.Hash `json:"headerHash"`
	Timeslot       uint32      `json:"slot"`
	Header         BlockHeader `json:"header"`
	ValidatorIndex uint16      `json:"validator_index"`
}

// ToBytes for JAMSNPBlockAnnouncement
func (ann *BlockAnnouncement) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Serialize HeaderHash (32 bytes for common.Hash)
	if _, err := buf.Write(ann.HeaderHash[:]); err != nil {
		return nil, err
	}

	// Serialize Timeslot (4 bytes)
	if err := binary.Write(buf, binary.LittleEndian, ann.Timeslot); err != nil {
		return nil, err
	}

	// Serialize Header
	headerBytes, err := ann.Header.Bytes()
	if err != nil {
		return nil, err
	}
	if _, err := buf.Write(headerBytes); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// FromBytes for JAMSNPBlockAnnouncement
func (ann *BlockAnnouncement) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Deserialize HeaderHash (32 bytes)
	if _, err := io.ReadFull(buf, ann.HeaderHash[:]); err != nil {
		return err
	}

	// Deserialize Timeslot (4 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &ann.Timeslot); err != nil {
		return err
	}

	// Deserialize Header
	headerBytes, err := ann.Header.Bytes()
	if err != nil {
		return err
	}
	if _, err := io.ReadFull(buf, headerBytes); err != nil {
		return err
	}

	return nil
}
