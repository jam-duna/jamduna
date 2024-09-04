package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

type Validator struct {
	Ed25519      Ed25519Key                `json:"ed25519"`
	Bandersnatch common.Hash               `json:"bandersnatch"`
	Bls          [BlsSizeInBytes]byte      `json:"bls"`
	Metadata     [MetadataSizeInBytes]byte `json:"metadata"`
}

// used for calling FFI
type ValidatorSecret struct {
	Ed25519Pub         common.Hash `json:"ed25519_pub"`
	BandersnatchPub    common.Hash `json:"bandersnatch_pub"`
	Ed25519Secret      []byte      `json:"ed25519_priv"`
	BandersnatchSecret []byte      `json:"bandersnatch_priv"`
}

func (v ValidatorSecret) MarshalJSON() ([]byte, error) {
	type Alias ValidatorSecret
	return json.Marshal(&struct {
		Alias
		Ed25519Secret      string `json:"ed25519_priv"`
		BandersnatchSecret string `json:"bandersnatch_priv"`
	}{
		Alias:              (Alias)(v),
		Ed25519Secret:      hex.EncodeToString(v.Ed25519Secret),
		BandersnatchSecret: hex.EncodeToString(v.BandersnatchSecret),
	})
}

func (v Validator) GetBandersnatchKey() common.Hash {
	return v.Bandersnatch
}

func (v Validator) Bytes() []byte {
	// Initialize a byte slice with the required size
	bytes := make([]byte, 0, ValidatorInByte)
	bytes = append(bytes, v.Ed25519[:]...)
	bytes = append(bytes, v.Bandersnatch[:]...)
	bytes = append(bytes, v.Bls[:]...)
	bytes = append(bytes, v.Metadata[:]...)
	return bytes
}

func ValidatorFromBytes(data []byte) (Validator, error) {
	if len(data) != ValidatorInByte {
		return Validator{}, fmt.Errorf("invalid data length: expected %d, got %d", 32+32+144+128, len(data))
	}
	var v Validator
	offset := 0
	copy(v.Ed25519[:], data[offset:offset+32])
	offset += 32
	copy(v.Bandersnatch[:], data[offset:offset+32])
	offset += 32
	copy(v.Bls[:], data[offset:offset+144])
	offset += 144
	copy(v.Metadata[:], data[offset:offset+128])
	return v, nil
}

// MarshalJSON custom marshaler to convert byte arrays to hex strings
func (v Validator) MarshalJSON() ([]byte, error) {
	type Alias Validator
	return json.Marshal(&struct {
		Alias
		Bls      string `json:"bls"`
		Metadata string `json:"metadata"`
	}{
		Alias:    (Alias)(v),
		Bls:      hex.EncodeToString(v.Bls[:]),
		Metadata: hex.EncodeToString(v.Metadata[:]),
	})
}

// UnmarshalJSON custom unmarshal method to handle hex strings for byte arrays
func (v *Validator) UnmarshalJSON(data []byte) error {
	type Alias Validator
	aux := &struct {
		Bls      string `json:"bls"`
		Metadata string `json:"metadata"`
		*Alias
	}{
		Alias: (*Alias)(v),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Decode the hex strings into byte arrays
	blsBytes, err := hex.DecodeString(aux.Bls)
	if err != nil {
		return err
	}
	if len(blsBytes) != BlsSizeInBytes {
		return fmt.Errorf("invalid length for bls field")
	}
	copy(v.Bls[:], blsBytes)

	metadataBytes, err := hex.DecodeString(aux.Metadata)
	if err != nil {
		return err
	}
	if len(metadataBytes) != MetadataSizeInBytes {
		return fmt.Errorf("invalid length for metadata field")
	}
	copy(v.Metadata[:], metadataBytes)

	return nil
}

func HexToBLS(hexStr string) [BlsSizeInBytes]byte {
	b := common.Hex2Bytes(hexStr)
	var bls [BlsSizeInBytes]byte
	copy(bls[:], b)
	return bls
}

func HexToMetadata(hexStr string) [MetadataSizeInBytes]byte {
	b := common.Hex2Bytes(hexStr)
	var meta [MetadataSizeInBytes]byte
	copy(meta[:], b)
	return meta
}
