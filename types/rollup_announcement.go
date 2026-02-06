package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/jam-duna/jamduna/common"
)

type UP1Announcement struct {
	ServiceID             uint32      `json:"service_id"`
	CoreIndex             uint16      `json:"core_index"`
	Checkpoint            uint8       `json:"checkpoint"`
	CheckpointNumber      uint32      `json:"checkpoint_number"`
	ParentCheckpointHash  common.Hash `json:"parent_checkpoint_hash"`
	AuthorizationCodeHash common.Hash `json:"authorization_code_hash"`
	AuthorizerHash        common.Hash `json:"authorizer_hash"`
	ConfigurationBlob     []byte      `json:"configuration_blob"`
	RollupBlock           []byte      `json:"rollup_block"`
}

func (ann *UP1Announcement) ToBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.LittleEndian, ann.ServiceID); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, ann.CoreIndex); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, ann.Checkpoint); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, ann.CheckpointNumber); err != nil {
		return nil, err
	}

	if ann.Checkpoint != 0 {
		if _, err := buf.Write(ann.ParentCheckpointHash[:]); err != nil {
			return nil, err
		}
	}

	if _, err := buf.Write(ann.AuthorizationCodeHash[:]); err != nil {
		return nil, err
	}

	if _, err := buf.Write(ann.AuthorizerHash[:]); err != nil {
		return nil, err
	}

	configLen := uint32(len(ann.ConfigurationBlob))
	if err := binary.Write(buf, binary.LittleEndian, configLen); err != nil {
		return nil, err
	}
	if _, err := buf.Write(ann.ConfigurationBlob); err != nil {
		return nil, err
	}

	blockLen := uint32(len(ann.RollupBlock))
	if err := binary.Write(buf, binary.LittleEndian, blockLen); err != nil {
		return nil, err
	}
	if _, err := buf.Write(ann.RollupBlock); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (ann *UP1Announcement) FromBytes(data []byte) error {
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &ann.ServiceID); err != nil {
		return fmt.Errorf("failed to read ServiceID: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &ann.CoreIndex); err != nil {
		return fmt.Errorf("failed to read CoreIndex: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &ann.Checkpoint); err != nil {
		return fmt.Errorf("failed to read Checkpoint: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &ann.CheckpointNumber); err != nil {
		return fmt.Errorf("failed to read CheckpointNumber: %w", err)
	}

	ann.ParentCheckpointHash = common.Hash{}
	if ann.Checkpoint != 0 {
		if _, err := io.ReadFull(buf, ann.ParentCheckpointHash[:]); err != nil {
			return fmt.Errorf("failed to read ParentCheckpointHash: %w", err)
		}
	}

	if _, err := io.ReadFull(buf, ann.AuthorizationCodeHash[:]); err != nil {
		return fmt.Errorf("failed to read AuthorizationCodeHash: %w", err)
	}

	if _, err := io.ReadFull(buf, ann.AuthorizerHash[:]); err != nil {
		return fmt.Errorf("failed to read AuthorizerHash: %w", err)
	}

	var configLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &configLen); err != nil {
		return fmt.Errorf("failed to read ConfigurationBlob length: %w", err)
	}
	ann.ConfigurationBlob = make([]byte, configLen)
	if _, err := io.ReadFull(buf, ann.ConfigurationBlob); err != nil {
		return fmt.Errorf("failed to read ConfigurationBlob: %w", err)
	}

	var blockLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &blockLen); err != nil {
		return fmt.Errorf("failed to read RollupBlock length: %w", err)
	}
	ann.RollupBlock = make([]byte, blockLen)
	if _, err := io.ReadFull(buf, ann.RollupBlock); err != nil {
		return fmt.Errorf("failed to read RollupBlock: %w", err)
	}

	return nil
}

func (ann *UP1Announcement) VerifyAuthorizerPreimage() bool {
	data := make([]byte, 0, len(ann.AuthorizationCodeHash)+len(ann.ConfigurationBlob))
	data = append(data, ann.AuthorizationCodeHash[:]...)
	data = append(data, ann.ConfigurationBlob...)
	computedHash := common.Blake2Hash(data)
	return bytes.Equal(computedHash[:], ann.AuthorizerHash[:])
}
