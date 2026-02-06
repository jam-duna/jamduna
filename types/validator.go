package types

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/jam-duna/jamduna/common"
)

type Validator struct {
	Bandersnatch BandersnatchKey           `json:"bandersnatch"`
	Ed25519      Ed25519Key                `json:"ed25519"`
	Bls          [BlsPubInBytes]byte       `json:"bls"`
	Metadata     [MetadataSizeInBytes]byte `json:"metadata"`
}
type Validators []Validator

func GetValidatorsLength(v Validators) int {
	var tmp []Validator
	tmp = v
	return len(tmp)
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
	bytes, err := Encode(v)
	if err != nil {
		return nil
	}
	return bytes
}

func ValidatorFromBytes(data []byte) (Validator, error) {
	var v Validator
	decoded, _, err := Decode(data, reflect.TypeOf(v))
	if err != nil {
		return v, err
	}
	v = decoded.(Validator)
	return v, nil
}

// MarshalJSON custom marshaler to convert byte arrays to hex strings
func (v Validator) MarshalJSON() ([]byte, error) {

	return json.Marshal(&struct {
		Bandersnatch BandersnatchKey `json:"bandersnatch"`
		Ed25519      Ed25519Key      `json:"ed25519"`
		Bls          string          `json:"bls"`
		Metadata     string          `json:"metadata"`
	}{
		Bandersnatch: v.Bandersnatch,
		Ed25519:      v.Ed25519,
		Bls:          common.Bytes2Hex(v.Bls[:]),
		Metadata:     common.Bytes2Hex(v.Metadata[:]),
	})
}

func (v *Validator) String() string {
	// print JSON
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return string(jsonBytes)
}

func (v *Validator) Print() {
	fmt.Println(v.String())
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

	bls_pub := common.Hex2Bytes(aux.Bls)
	meta := common.Hex2Bytes(aux.Metadata)

	if len(bls_pub) != BlsPubInBytes {
		return fmt.Errorf("invalid length for bls field")
	}

	if len(meta) != MetadataSizeInBytes {
		return fmt.Errorf("invalid length for metadata field")
	}

	copy(v.Bls[:], bls_pub)
	copy(v.Metadata[:], meta)

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
		Bls:                common.Bytes2Hex(v.BlsPub[:]),
		Metadata:           common.Bytes2Hex(v.Metadata[:]),
		BandersnatchSecret: common.Bytes2Hex(v.BandersnatchSecret),
		Ed25519Secret:      common.Bytes2Hex(v.Ed25519Secret[:]),
		BlsSecret:          common.Bytes2Hex(v.BlsSecret[:]),
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

	bls_pub := common.Hex2Bytes(aux.Bls)
	metadata := common.Hex2Bytes(aux.Metadata)
	bandersnatch_secret := common.Hex2Bytes(aux.BandersnatchSecret)
	ed25519_secret := common.Hex2Bytes(aux.Ed25519Secret)
	bls_secret := common.Hex2Bytes(aux.BlsSecret)

	if len(bls_pub) != BlsPubInBytes {
		return fmt.Errorf("invalid BlsPub length: expected %d bytes, got %d", BlsPubInBytes, len(bls_pub))
	}

	if len(metadata) != MetadataSizeInBytes {
		return fmt.Errorf("invalid Metadata length: expected %d bytes, got %d", metadata, len(metadata))
	}

	copy(v.BlsPub[:], bls_pub)
	copy(v.Metadata[:], metadata)
	copy(v.BandersnatchSecret, bandersnatch_secret)
	copy(v.Ed25519Secret[:], ed25519_secret)
	copy(v.BlsSecret[:], bls_secret)
	return nil
}

// C2 AuthQueue
func (T AuthorizationQueue) Decode(data []byte) (interface{}, uint32) {
	authorizations_queue := [TotalCores][MaxAuthorizationQueueItems]common.Hash{}
	decoded, length, err := Decode(data, reflect.TypeOf(authorizations_queue))
	if err != nil {
		return AuthorizationQueue{}, 0
	}
	authorizations_queue = decoded.([TotalCores][MaxAuthorizationQueueItems]common.Hash)
	for i := 0; i < TotalCores; i++ {
		if len(T[i]) == 0 {
			var temp [MaxAuthorizationQueueItems]common.Hash
			T[i] = temp //	T[i] = make([]common.Hash, MaxAuthorizationQueueItems)
		}

		for j := 0; j < MaxAuthorizationQueueItems; j++ {
			T[i][j] = authorizations_queue[i][j]
		}
	}
	return T, length
}

func (v Validators) String() string {
	// print JSON
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return string(jsonBytes)
}

// validators
func (T Validators) Decode(data []byte) (interface{}, uint32) {
	if len(data) == 0 {
		return Validators{}, 0
	}
	validators, length, err := Decode(data, reflect.TypeOf([TotalValidators]Validator{}))
	if err != nil {
		return Validators{}, 0
	}
	for i := 0; i < TotalValidators; i++ {
		T = append(T, validators.([TotalValidators]Validator)[i])
	}
	return T, length
}

// validators
func (T Validators) Encode() []byte {
	var validators [TotalValidators]Validator
	if len(T) == 0 || len(T) > TotalValidators {
		return []byte{}
	}
	copy(validators[:], T)
	encoded, err := Encode(validators)
	if err != nil {
		return []byte{}
	}
	return encoded
}

// C2
func (T AuthorizationQueue) Encode() []byte {
	authorizations_queue := [TotalCores][MaxAuthorizationQueueItems]common.Hash{}
	if len(T) == 0 || len(T) > TotalCores {
		return []byte{}
	}
	for i := 0; i < len(T); i++ {
		copy(authorizations_queue[i][:], T[i][:])
		// copy(authorizations_queue[i][:], T[i])
	}
	encoded, err := Encode(authorizations_queue)
	if err != nil {
		return []byte{}
	}
	return encoded
}
