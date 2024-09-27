package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

type Validator struct {
	Bandersnatch BandersnatchKey           `json:"bandersnatch"`
	Ed25519      Ed25519Key                `json:"ed25519"`
	Bls          [BlsSizeInBytes]byte      `json:"bls"`
	Metadata     [MetadataSizeInBytes]byte `json:"metadata"`
}

type ValidatorSecret struct {
	BandersnatchPub    BandersnatchKey `json:"bandersnatch"`
	Ed25519Pub         Ed25519Key `json:"ed25519"`
	BandersnatchSecret []byte      `json:"bandersnatch_priv"`
	Ed25519Secret      []byte      `json:"ed25519_priv"`
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

func (b *BandersnatchVrfSignature) Bytes() []byte {
	return b[:]
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

func (v ValidatorSecret) MarshalJSON() ([]byte, error) {
    // Define an alias without the secret fields
    type Alias struct {
        BandersnatchPub BandersnatchKey `json:"bandersnatch"`
        Ed25519Pub      Ed25519Key      `json:"ed25519"`
    }

    return json.Marshal(&struct {
        Alias
		BandersnatchSecret string `json:"bandersnatch_priv"`
        Ed25519Secret      string `json:"ed25519_priv"`

    }{
        Alias:              Alias{BandersnatchPub: v.BandersnatchPub, Ed25519Pub: v.Ed25519Pub},
		BandersnatchSecret: hex.EncodeToString(v.BandersnatchSecret),
        Ed25519Secret:      hex.EncodeToString(v.Ed25519Secret),
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
		BandersnatchSecret string `json:"bandersnatch_priv"`
        Ed25519Secret      string `json:"ed25519_priv"`
    }{}

    if err := json.Unmarshal(data, aux); err != nil {
        return err
    }

    // Assign the public keys from Alias
    v.BandersnatchPub = aux.BandersnatchPub
    v.Ed25519Pub = aux.Ed25519Pub

    // Decode the hex strings into byte slices for the secret keys
    ed25519SecretBytes, err := hex.DecodeString(aux.Ed25519Secret)
    if err != nil {
        return err
    }
    v.Ed25519Secret = ed25519SecretBytes

    bandersnatchSecretBytes, err := hex.DecodeString(aux.BandersnatchSecret)
    if err != nil {
        return err
    }
    v.BandersnatchSecret = bandersnatchSecretBytes

    return nil
}
