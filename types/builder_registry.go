package types

import (
	"encoding/binary"
	"fmt"

	"github.com/jam-duna/jamduna/common"
)

const (
	BuilderRegistryMagicV1        = "BRG1"
	BuilderRegistryScheduleRR     = 0
	builderRegistryHeaderSizeV1   = 4 + 4 + 4 + 4 + 1 + 3 + 4
	BuilderRegistryEntrySizeV1    = 32 + 2 + 6
	BuilderRegistryReservedV1Size = 3
)


type BuilderRegistryEntryV1 struct {
	BuilderEd25519  [32]byte
	AllowedCoreMask uint16
}


func EncodeBuilderRegistryV1(serviceID, registryID, activateAtSlot uint32, scheduleID uint8, builders []BuilderRegistryEntryV1) ([]byte, error) {
	if len(builders) == 0 {
		return nil, fmt.Errorf("builder registry requires at least one entry")
	}

	totalLen := builderRegistryHeaderSizeV1 + len(builders)*BuilderRegistryEntrySizeV1
	buf := make([]byte, 0, totalLen)

	buf = append(buf, []byte(BuilderRegistryMagicV1)...)
	tmp := make([]byte, 4)

	binary.LittleEndian.PutUint32(tmp, serviceID)
	buf = append(buf, tmp...)

	binary.LittleEndian.PutUint32(tmp, registryID)
	buf = append(buf, tmp...)

	binary.LittleEndian.PutUint32(tmp, activateAtSlot)
	buf = append(buf, tmp...)

	buf = append(buf, scheduleID)
	buf = append(buf, make([]byte, BuilderRegistryReservedV1Size)...)

	binary.LittleEndian.PutUint32(tmp, uint32(len(builders)))
	buf = append(buf, tmp...)

	for _, entry := range builders {
		buf = append(buf, entry.BuilderEd25519[:]...)
		maskBytes := make([]byte, 2)
		binary.LittleEndian.PutUint16(maskBytes, entry.AllowedCoreMask)
		buf = append(buf, maskBytes...)
		buf = append(buf, make([]byte, 6)...)
	}

	return buf, nil
}


func BuilderRegistryHashV1(serviceID, registryID, activateAtSlot uint32, scheduleID uint8, builders []BuilderRegistryEntryV1) (common.Hash, []byte, error) {
	encoded, err := EncodeBuilderRegistryV1(serviceID, registryID, activateAtSlot, scheduleID, builders)
	if err != nil {
		return common.Hash{}, nil, err
	}
	return common.Blake2Hash(encoded), encoded, nil
}
