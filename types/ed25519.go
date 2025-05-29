package types

import (
	//	"bytes"
	"crypto/ed25519"
	"encoding/json"

	//	"errors"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
)

const (
	Ed25519PubkeySize     = 32
	Ed25519PrivateKeySize = 64 // 32 byte seeds + 32 pub concatenated
	Ed25519SignatureSize  = 64 // 32 byte R + 32 byte S
	X_Ed25519_SECRET      = "jam_val_key_ed25519"
	//X_Ed25519_SECRET = ""
)

type Ed25519Key common.Hash
type Ed25519Priv ed25519.PrivateKey
type Ed25519Signature [Ed25519SignatureSize]byte

func (k Ed25519Signature) MarshalJSON() ([]byte, error) {
	return json.Marshal(common.Bytes2Hex(k[:]))
}

func (k Ed25519Key) MarshalJSON() ([]byte, error) {
	return json.Marshal(common.Hash(k).Hex())
}

func GenerateRandomEd25519Signature() Ed25519Signature {
	var s Ed25519Signature
	// TODO
	return s
}

func (k *Ed25519Key) UnmarshalJSON(data []byte) error {
	var hexStr string
	if err := json.Unmarshal(data, &hexStr); err != nil {
		return err
	}
	*k = Ed25519Key(common.HexToHash(hexStr))
	return nil
}

func (priv Ed25519Priv) PublicKey() Ed25519Key {
	return Ed25519Key(ed25519.PrivateKey(priv).Public().(ed25519.PublicKey))
}

func (k Ed25519Key) String() string {
	return common.Hash(k).Hex()
}

func (pk Ed25519Key) Bytes() []byte {
	return pk[:]
}

func (pk Ed25519Key) PublicKey() ed25519.PublicKey {
	return ed25519.PublicKey(pk[:])
}

func BytesToFixedLength(sig []byte) (interface{}, error) {
	length := len(sig)

	// Dynamically create a fixed-size array type of the appropriate length
	fixedLengthArrayType := reflect.ArrayOf(length, reflect.TypeOf(byte(0)))
	fixedLengthArray := reflect.New(fixedLengthArrayType).Elem()

	if len(sig) != length {
		return nil, fmt.Errorf("invalid byte slice length: expected %d bytes, got %d", length, len(sig))
	}

	// Copy bytes to the fixed-length array
	reflect.Copy(fixedLengthArray, reflect.ValueOf(sig))

	// Return as an interface{} since the actual type is not known until runtime
	return fixedLengthArray.Interface(), nil
}

func HexToEd25519Sig(hexStr string) Ed25519Signature {
	b := common.Hex2Bytes(hexStr)
	var signature Ed25519Signature
	copy(signature[:], b)
	return signature
}

// Ed25519Signature to byte
func (e *Ed25519Signature) Bytes() []byte {
	return e[:]
}

func HexToEd25519Key(hexStr string) Ed25519Key {
	b := common.Hex2Bytes(hexStr)
	var pubkey Ed25519Key
	copy(pubkey[:], b)
	return pubkey
}

func InitEd25519Key(seed []byte) (Ed25519Key, []byte, error) {
	// Check if the seed length is 32 bytes
	if len(seed) != ed25519.SeedSize {
		return Ed25519Key{}, nil, fmt.Errorf("seed length must be %d bytes", ed25519.SeedSize)
	}

	ed25519_secret := common.ComputeHash(append([]byte(X_Ed25519_SECRET), seed...))
	ed25519_priv := ed25519.NewKeyFromSeed(ed25519_secret)
	ed25519_pub := ed25519_priv.Public().(ed25519.PublicKey)
	// fmt.Printf("seed: %x\n", seed)
	// fmt.Printf("ed25519 prefix_raw: %s -> %x\n", X_Ed25519_SECRET, []byte(X_Ed25519_SECRET))
	// fmt.Printf("blake2b(prefix, seed): %x\n", ed25519_secret)
	// fmt.Printf("ed25519_priv: %x\n", ed25519_priv)
	// fmt.Printf("ed25519_pub: %x\n", ed25519_pub)
	return Ed25519Key(ed25519_pub), ed25519_priv, nil
}

func BytesToEd25519Priv(b []byte) (Ed25519Priv, error) {
	if len(b) != Ed25519PrivateKeySize {
		return nil, fmt.Errorf("invalid byte slice length: expected %d bytes, got %d", Ed25519PrivateKeySize, len(b))
	}
	var priv Ed25519Priv
	copy(priv[:], b)
	return priv, nil
}

func BytesToEd25519Key(b []byte) (Ed25519Key, error) {
	if len(b) != Ed25519PubkeySize {
		return Ed25519Key{}, fmt.Errorf("invalid byte slice length: expected %d bytes, got %d", Ed25519PubkeySize, len(b))
	}
	var key Ed25519Key
	copy(key[:], b)
	return key, nil
}

func BytesToEd25519Signature(b []byte) (Ed25519Signature, error) {
	if len(b) != 64 {
		return Ed25519Signature{}, fmt.Errorf("invalid signature size: expected %d bytes, got %d", 64, len(b))
	}
	var sig Ed25519Signature
	copy(sig[:], b)
	return sig, nil
}

func Ed25519Sign(priv Ed25519Priv, msg []byte) Ed25519Signature {
	signature := ed25519.Sign(ed25519.PrivateKey(priv), msg)
	var ed25519Signature Ed25519Signature
	copy(ed25519Signature[:], signature)
	return ed25519Signature
}

func Ed25519SignByBytes(priv []byte, msg []byte) Ed25519Signature {
	signature := ed25519.Sign(ed25519.PrivateKey(priv), msg)
	var ed25519Signature Ed25519Signature
	copy(ed25519Signature[:], signature)
	return ed25519Signature
}

func Ed25519Verify(pub Ed25519Key, msg []byte, sig Ed25519Signature) bool {
	return ed25519.Verify(pub.PublicKey(), msg, sig[:])
}
