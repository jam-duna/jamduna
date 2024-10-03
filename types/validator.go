package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/colorfulnotion/jam/common"
)

type Validator struct {
	Bandersnatch BandersnatchKey           `json:"bandersnatch"`
	Ed25519      Ed25519Key                `json:"ed25519"`
	Bls          [BlsPubInBytes]byte       `json:"bls"`
	Metadata     [MetadataSizeInBytes]byte `json:"metadata"`
}

type ValidatorSecret struct {
	BandersnatchPub    BandersnatchKey           `json:"bandersnatch"`
	Ed25519Pub         Ed25519Key                `json:"ed25519"`
	BlsPub             [BlsPubInBytes]byte       `json:"bls"`
	Metadata           [MetadataSizeInBytes]byte `json:"metadata"`
	BandersnatchSecret []byte                    `json:"bandersnatch_priv"`
	Ed25519Secret      [Ed25519PrivInBytes]byte  `json:"ed25519_priv"`
	BlsSecret          [BlsPrivInBytes]byte      `json:"bls_priv"`
}

func (v *Validator) GetEd25519Key() Ed25519Key {
	return v.Ed25519
}

func (v Validator) GetBandersnatchKey() BandersnatchKey {
	return v.Bandersnatch
}

func (v Validator) Bytes() []byte {
	bytes := Encode(v)
	return bytes
}

func ValidatorFromBytes(data []byte) (Validator, error) {
	var v Validator
	decoded, _ := Decode(data, reflect.TypeOf(v))
	v = decoded.(Validator)
	return v, nil
}

// MarshalJSON custom marshaler to convert byte arrays to hex strings
func (v Validator) MarshalJSON() ([]byte, error) {
	type Alias struct {
		Ed25519      Ed25519Key      `json:"ed25519"`
		Bandersnatch BandersnatchKey `json:"bandersnatch"`
	}

	return json.Marshal(&struct {
		Alias
		Bls      string `json:"bls"`
		Metadata string `json:"metadata"`
	}{
		Alias:    Alias{Ed25519: v.Ed25519, Bandersnatch: v.Bandersnatch},
		Bls:      hex.EncodeToString(v.Bls[:]),
		Metadata: hex.EncodeToString(v.Metadata[:]),
	})
}

// UnmarshalJSON custom unmarshal method to handle hex strings for byte arrays
func (v *Validator) UnmarshalJSON(data []byte) error {
	type Alias struct {
		Ed25519      Ed25519Key      `json:"ed25519"`
		Bandersnatch BandersnatchKey `json:"bandersnatch"`
	}

	aux := &struct {
		Alias
		Bls      string `json:"bls"`
		Metadata string `json:"metadata"`
	}{}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	v.Ed25519 = aux.Ed25519
	v.Bandersnatch = aux.Bandersnatch

	blsBytes, err := hex.DecodeString(aux.Bls)
	if err != nil {
		return err
	}
	if len(blsBytes) != BlsPubInBytes {
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

func (b *BandersnatchVrfSignature) Bytes() []byte {
	return b[:]
}

func HexToBLS(hexStr string) [BlsPubInBytes]byte {
	b := common.Hex2Bytes(hexStr)
	var bls [BlsPubInBytes]byte
	copy(bls[:], b)
	return bls
}

func HexToMetadata(hexStr string) [MetadataSizeInBytes]byte {
	b := common.Hex2Bytes(hexStr)
	var meta [MetadataSizeInBytes]byte
	copy(meta[:], b)
	return meta
}

func (v ValidatorSecret) MarshalJSON() ([]byte, error) {
	// Define an alias without the secret fields to prevent recursion
	type Alias struct {
		BandersnatchPub BandersnatchKey `json:"bandersnatch"`
		Ed25519Pub      Ed25519Key      `json:"ed25519"`
	}

	// Helper function to encode []byte to hex string, handling empty slices
	encodeBytesToHex := func(data []byte) string {
		if len(data) == 0 {
			return "0x"
		}
		return "0x" + hex.EncodeToString(data)
	}

	// Similarly for fixed-size arrays
	encodeArrayToHex := func(data []byte) string {
		if len(data) == 0 {
			return "0x"
		}
		return "0x" + hex.EncodeToString(data)
	}

	return json.Marshal(&struct {
		Alias
		Bls                string `json:"bls"`
		Metadata           string `json:"metadata"`
		BandersnatchSecret string `json:"bandersnatch_priv"`
		Ed25519Secret      string `json:"ed25519_priv"`
		BlsSecret          string `json:"bls_priv"`
	}{
		Alias: Alias{
			BandersnatchPub: v.BandersnatchPub,
			Ed25519Pub:      v.Ed25519Pub,
		},
		Bls:                encodeArrayToHex(v.BlsPub[:]),
		Metadata:           encodeArrayToHex(v.Metadata[:]),
		BandersnatchSecret: encodeBytesToHex(v.BandersnatchSecret),
		Ed25519Secret:      encodeArrayToHex(v.Ed25519Secret[:]),
		BlsSecret:          encodeArrayToHex(v.BlsSecret[:]),
	})
}

func (v *ValidatorSecret) UnmarshalJSON(data []byte) error {
	// Define an alias without the secret fields
	type Alias struct {
		BandersnatchPub BandersnatchKey `json:"bandersnatch"`
		Ed25519Pub      Ed25519Key      `json:"ed25519"`
	}

	aux := &struct {
		Alias
		Bls                string `json:"bls"`
		Metadata           string `json:"metadata"`
		BandersnatchSecret string `json:"bandersnatch_priv"`
		Ed25519Secret      string `json:"ed25519_priv"`
		BlsSecret          string `json:"bls_priv"`
	}{}

	// Unmarshal the data into the auxiliary struct
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Assign the public keys from Alias
	v.BandersnatchPub = aux.BandersnatchPub
	v.Ed25519Pub = aux.Ed25519Pub

	// Helper function to decode hex strings to []byte, handling "0x"
	decodeHexToBytes := func(s string) ([]byte, error) {
		if s == "0x" || s == "" {
			return []byte{}, nil
		}
		if strings.HasPrefix(s, "0x") {
			s = s[2:]
		}
		return hex.DecodeString(s)
	}

	var err error

	// Decode BlsPub
	blsBytes, err := decodeHexToBytes(aux.Bls)
	if err != nil {
		return fmt.Errorf("failed to decode BlsPub: %v", err)
	}
	if len(blsBytes) != BlsPubInBytes {
		return fmt.Errorf("invalid BlsPub length: expected %d bytes, got %d", BlsPubInBytes, len(blsBytes))
	}
	copy(v.BlsPub[:], blsBytes)

	// Decode Metadata
	metadataBytes, err := decodeHexToBytes(aux.Metadata)
	if err != nil {
		return fmt.Errorf("failed to decode Metadata: %v", err)
	}
	if len(metadataBytes) != MetadataSizeInBytes {
		return fmt.Errorf("invalid Metadata length: expected %d bytes, got %d", MetadataSizeInBytes, len(metadataBytes))
	}
	copy(v.Metadata[:], metadataBytes)

	// Decode Secret Fields
	v.BandersnatchSecret, err = decodeHexToBytes(aux.BandersnatchSecret)
	if err != nil {
		return fmt.Errorf("failed to decode BandersnatchSecret: %v", err)
	}
	ed25519_secret, err := decodeHexToBytes(aux.Ed25519Secret[:])
	if err != nil {
		return fmt.Errorf("failed to decode Ed25519Secret: %v", err)
	}
	copy(v.Ed25519Secret[:], ed25519_secret[:])
	bls_secret, err := decodeHexToBytes(aux.BlsSecret[:])
	if err != nil {
		return fmt.Errorf("failed to decode BlsSecret: %v", err)
	}
	copy(v.BlsSecret[:], bls_secret)
	return nil
}
