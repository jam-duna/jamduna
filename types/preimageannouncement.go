package types

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

type PreimageAnnouncement struct {
	ValidatorIndex uint16      `json:"validator_index"`
	ServiceIndex   uint32      `json:"service"`
	PreimageHash   common.Hash `json:"h"`
	PreimageLen    uint32      `json:"z"`
}

func (req *PreimageAnnouncement) String() string {
	return fmt.Sprintf(" Preimage: (s=%d, h=%v, l=%d)", req.ServiceIndex, req.PreimageHash, req.PreimageLen)
}

func (req *PreimageAnnouncement) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write MaximumBlocks (4 bytes)
	if err := binary.Write(buf, binary.LittleEndian, req.ServiceIndex); err != nil {
		return nil, err
	}

	// Write HeaderHash (32 bytes for common.Hash)
	if _, err := buf.Write(req.PreimageHash[:]); err != nil {
		return nil, err
	}

	// Write MaximumBlocks (4 bytes)
	if err := binary.Write(buf, binary.LittleEndian, req.PreimageLen); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Deserialize function to convert a byte array back into a JAMSNPBlockRequest struct
func (req *PreimageAnnouncement) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	// Read ServiceID (4 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &req.ServiceIndex); err != nil {
		return err
	}

	// Read PreimageHash (32 bytes)
	if _, err := buf.Read(req.PreimageHash[:]); err != nil {
		return err
	}

	// Read PreimageLen (4 bytes)
	if err := binary.Read(buf, binary.LittleEndian, &req.PreimageLen); err != nil {
		return err
	}

	return nil
}
