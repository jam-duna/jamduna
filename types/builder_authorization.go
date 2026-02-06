package types

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/jam-duna/jamduna/common"
)
















type BuilderConfigurationBlob struct {
	Version         uint8
	BuilderEd25519  [32]byte
	AllowedCoreMask uint16
	TargetSlot      uint32
	Nonce           uint32

	BuilderRegistryHash common.Hash
}

const (
	BuilderConfigurationBlobVersionV1 = 1
	BuilderConfigurationBlobVersionV2 = 2
	BuilderConfigurationBlobV1Size    = 1 + 32 + 2 + 4 + 4
	BuilderConfigurationBlobV2Size    = 1 + 32 + 2 + 4 + 32
)


func NewBuilderConfigurationBlob(ed25519Pubkey [32]byte, allowedCores uint16, targetSlot, nonce uint32) *BuilderConfigurationBlob {
	return &BuilderConfigurationBlob{
		Version:         BuilderConfigurationBlobVersionV1,
		BuilderEd25519:  ed25519Pubkey,
		AllowedCoreMask: allowedCores,
		TargetSlot:      targetSlot,
		Nonce:           nonce,
	}
}


func NewBuilderConfigurationBlobV2(ed25519Pubkey [32]byte, allowedCores uint16, targetSlot uint32, builderRegistryHash common.Hash) *BuilderConfigurationBlob {
	return &BuilderConfigurationBlob{
		Version:             BuilderConfigurationBlobVersionV2,
		BuilderEd25519:      ed25519Pubkey,
		AllowedCoreMask:     allowedCores,
		TargetSlot:          targetSlot,
		BuilderRegistryHash: builderRegistryHash,
	}
}


func (b *BuilderConfigurationBlob) Bytes() []byte {
	switch b.Version {
	case BuilderConfigurationBlobVersionV1:
		buf := make([]byte, BuilderConfigurationBlobV1Size)
		offset := 0

		buf[offset] = b.Version
		offset++

		copy(buf[offset:offset+32], b.BuilderEd25519[:])
		offset += 32

		binary.LittleEndian.PutUint16(buf[offset:offset+2], b.AllowedCoreMask)
		offset += 2

		binary.LittleEndian.PutUint32(buf[offset:offset+4], b.TargetSlot)
		offset += 4

		binary.LittleEndian.PutUint32(buf[offset:offset+4], b.Nonce)

		return buf
	case BuilderConfigurationBlobVersionV2:
		buf := make([]byte, BuilderConfigurationBlobV2Size)
		offset := 0

		buf[offset] = b.Version
		offset++

		copy(buf[offset:offset+32], b.BuilderEd25519[:])
		offset += 32

		binary.LittleEndian.PutUint16(buf[offset:offset+2], b.AllowedCoreMask)
		offset += 2

		binary.LittleEndian.PutUint32(buf[offset:offset+4], b.TargetSlot)
		offset += 4

		copy(buf[offset:offset+32], b.BuilderRegistryHash[:])

		return buf
	default:
		return nil
	}
}


func ParseBuilderConfigurationBlob(data []byte) (*BuilderConfigurationBlob, error) {
	if len(data) < 1+32+2+4 {
		return nil, fmt.Errorf("configuration blob too short: got %d", len(data))
	}

	b := &BuilderConfigurationBlob{}
	offset := 0

	b.Version = data[offset]
	offset++

	switch b.Version {
	case BuilderConfigurationBlobVersionV1:
		if len(data) < BuilderConfigurationBlobV1Size {
			return nil, fmt.Errorf("configuration blob v1 too short: got %d, need %d", len(data), BuilderConfigurationBlobV1Size)
		}
		copy(b.BuilderEd25519[:], data[offset:offset+32])
		offset += 32

		b.AllowedCoreMask = binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2

		b.TargetSlot = binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4

		b.Nonce = binary.LittleEndian.Uint32(data[offset : offset+4])
		return b, nil
	case BuilderConfigurationBlobVersionV2:
		if len(data) < BuilderConfigurationBlobV2Size {
			return nil, fmt.Errorf("configuration blob v2 too short: got %d, need %d", len(data), BuilderConfigurationBlobV2Size)
		}
		copy(b.BuilderEd25519[:], data[offset:offset+32])
		offset += 32

		b.AllowedCoreMask = binary.LittleEndian.Uint16(data[offset : offset+2])
		offset += 2

		b.TargetSlot = binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4

		copy(b.BuilderRegistryHash[:], data[offset:offset+32])
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported configuration blob version: %d", b.Version)
	}
}


func (b *BuilderConfigurationBlob) IsCoreAllowed(coreIndex uint16) bool {
	if coreIndex >= 16 {
		return false
	}
	return (b.AllowedCoreMask & (1 << coreIndex)) != 0
}



func (b *BuilderConfigurationBlob) ComputeAuthorizerHash(authCodeHash common.Hash) common.Hash {
	data := append(authCodeHash.Bytes(), b.Bytes()...)
	return common.Blake2Hash(data)
}














type BuilderAuthorizationToken struct {
	CoreIndex       uint16
	Slot            uint32
	WorkPackageHash common.Hash
	Signature       [64]byte
}

const (
	BuilderAuthorizationTokenSize    = 2 + 4 + 32 + 64
	BuilderAuthDomainSeparator       = "JAM_BUILDER_AUTH_V1"
	BuilderAuthDomainSeparatorBytes  = 19
	BuilderAuthDomainSeparatorV2     = "JAM_BUILDER_AUTH_V2"
	BuilderAuthDomainSeparatorV2Size = 19
)



func NewBuilderAuthorizationToken(coreIndex uint16, slot uint32, wpHash common.Hash) *BuilderAuthorizationToken {
	return &BuilderAuthorizationToken{
		CoreIndex:       coreIndex,
		Slot:            slot,
		WorkPackageHash: wpHash,
	}
}


func (t *BuilderAuthorizationToken) SigningMessage() []byte {
	msg := make([]byte, BuilderAuthDomainSeparatorBytes+2+4+32)
	offset := 0


	copy(msg[offset:], BuilderAuthDomainSeparator)
	offset += BuilderAuthDomainSeparatorBytes


	binary.LittleEndian.PutUint16(msg[offset:offset+2], t.CoreIndex)
	offset += 2


	binary.LittleEndian.PutUint32(msg[offset:offset+4], t.Slot)
	offset += 4


	copy(msg[offset:offset+32], t.WorkPackageHash[:])

	return common.Blake2Hash(msg).Bytes()
}


func (t *BuilderAuthorizationToken) SigningMessageV2(builderRegistryHash common.Hash) []byte {
	msg := make([]byte, BuilderAuthDomainSeparatorV2Size+2+4+32+32)
	offset := 0

	copy(msg[offset:], BuilderAuthDomainSeparatorV2)
	offset += BuilderAuthDomainSeparatorV2Size

	binary.LittleEndian.PutUint16(msg[offset:offset+2], t.CoreIndex)
	offset += 2

	binary.LittleEndian.PutUint32(msg[offset:offset+4], t.Slot)
	offset += 4

	copy(msg[offset:offset+32], builderRegistryHash[:])
	offset += 32

	copy(msg[offset:offset+32], t.WorkPackageHash[:])

	return common.Blake2Hash(msg).Bytes()
}


func (t *BuilderAuthorizationToken) Bytes() []byte {
	buf := make([]byte, BuilderAuthorizationTokenSize)
	offset := 0


	binary.LittleEndian.PutUint16(buf[offset:offset+2], t.CoreIndex)
	offset += 2


	binary.LittleEndian.PutUint32(buf[offset:offset+4], t.Slot)
	offset += 4


	copy(buf[offset:offset+32], t.WorkPackageHash[:])
	offset += 32


	copy(buf[offset:offset+64], t.Signature[:])

	return buf
}


func ParseBuilderAuthorizationToken(data []byte) (*BuilderAuthorizationToken, error) {
	if len(data) < BuilderAuthorizationTokenSize {
		return nil, fmt.Errorf("authorization token too short: got %d, need %d", len(data), BuilderAuthorizationTokenSize)
	}

	t := &BuilderAuthorizationToken{}
	offset := 0


	t.CoreIndex = binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2


	t.Slot = binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4


	copy(t.WorkPackageHash[:], data[offset:offset+32])
	offset += 32


	copy(t.Signature[:], data[offset:offset+64])

	return t, nil
}


func (t *BuilderAuthorizationToken) SetSignature(sig [64]byte) {
	t.Signature = sig
}


var (
	ErrInvalidConfigurationBlob    = errors.New("invalid configuration blob")
	ErrInvalidAuthorizationToken   = errors.New("invalid authorization token")
	ErrCoreNotAllowed              = errors.New("core not allowed for this builder")
	ErrSlotOutOfRange              = errors.New("slot out of valid range")
	ErrSignatureVerificationFailed = errors.New("signature verification failed")
	ErrWorkPackageHashMismatch     = errors.New("work package hash mismatch")
	ErrCoreIndexMismatch           = errors.New("core index mismatch")
)
