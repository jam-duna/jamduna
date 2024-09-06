package types

import (
	//	"bytes"
	"crypto/ed25519"
	//	"errors"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"reflect"
)

const (
	Ed25519PubkeySize     = 32
	Ed25519PrivateKeySize = 64 // 32 byte seeds + 32 pub concatenated
	Ed25519SignatureSize  = 64 // 32 byte R + 32 byte S
)

type Ed25519Signature [Ed25519SignatureSize]byte
type Ed25519Key common.Hash

//type Ed25519Key [Ed25519PubkeySize]byte

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

func HexToEd25519Key(hexStr string) Ed25519Key {
	b := common.Hex2Bytes(hexStr)
	var pubkey Ed25519Key
	copy(pubkey[:], b)
	return pubkey
}

func InitEd25519Key(seed []byte) ([]byte, []byte, error) {
	// Check if the seed length is 32 bytes
	if len(seed) != ed25519.SeedSize {
		return nil, nil, fmt.Errorf("seed length must be %d bytes", ed25519.SeedSize)
	}

	// Generate the private key from the seed
	ed25519_priv := ed25519.NewKeyFromSeed(seed)

	// The public key is the second half of the private key
	ed25519_pub := ed25519_priv.Public().(ed25519.PublicKey)

	return ed25519_pub, ed25519_priv, nil
}
