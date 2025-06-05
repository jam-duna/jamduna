//go:build cgo
// +build cgo

package bls

/*
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../ffi -lbls.linux_amd64 -ldl
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/../ffi -lbls.linux_arm64 -ldl
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../ffi -lbls.mac_amd64 -ldl
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../ffi -lbls.mac_arm64 -ldl
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/../ffi -lbls.windows_amd64 -lws2_32

#include <stdint.h>
#include <stdlib.h>
#include <bls.h>
*/
import "C"

import (
	"errors"
	"unsafe"

	"github.com/colorfulnotion/jam/log"
)

type DoublePublicKey [DoubleKeyLen]byte

func (dpk *DoublePublicKey) Bytes() []byte {
	return dpk[:]
}

type G1PublicKey [G1Len]byte
type G2PublicKey [G2Len]byte

func (g2 *G2PublicKey) Bytes() []byte {
	return g2[:]
}

type SecretKey [SecretKeyLen]byte

func (sk *SecretKey) Bytes() []byte {
	return sk[:]
}

type Signature [SigLen]byte

func (s *Signature) Bytes() []byte {
	return s[:]
}

type Seed []byte

func GetSecretKey(seed []byte) (SecretKey, error) {
	// Retrieve the secret key
	secret := SecretKey{}
	if len(seed) == 0 {
		return secret, errors.New("BLS seed empty")
	}
	C.get_secret_key(
		(*C.uchar)(unsafe.Pointer(&seed[0])),
		(C.size_t)(len(seed)),
		(*C.uchar)(unsafe.Pointer(&secret[0])),
		(C.size_t)(SecretKeyLen),
	)
	if len(secret.Bytes()) == 0 {
		return secret, errors.New("failed to retrieve secret key")
	}
	return secret, nil
}

func GetDoublePublicKey(seed []byte) (DoublePublicKey, error) {
	// Retrieve the double public key
	double := DoublePublicKey{}
	C.get_double_pubkey(
		(*C.uchar)(unsafe.Pointer(&seed[0])),
		(C.size_t)(len(seed)),
		(*C.uchar)(unsafe.Pointer(&double[0])),
		(C.size_t)(DoubleKeyLen),
	)
	if len(double.Bytes()) == 0 {
		return double, errors.New("failed to retrieve double public key")
	}
	return double, nil
}

func GetPublicKey_G2(seed []byte) (G2PublicKey, error) {
	// Retrieve the G2 public key
	g2 := G2PublicKey{}
	C.get_pubkey_g2(
		(*C.uchar)(unsafe.Pointer(&seed[0])),
		(C.size_t)(len(seed)),
		(*C.uchar)(unsafe.Pointer(&g2[0])),
		(C.size_t)(G2Len),
	)
	if len(g2.Bytes()) == 0 {
		return g2, errors.New("failed to retrieve G2 public key")
	}
	return g2, nil
}

func (secret *SecretKey) Sign(msg []byte) (Signature, error) {
	// Sign the message
	sig := Signature{}
	C.sign(
		(*C.uchar)(unsafe.Pointer(secret)),
		(C.size_t)(SecretKeyLen),
		(*C.uchar)(unsafe.Pointer(&msg[0])),
		(C.size_t)(len(msg)),
		(*C.uchar)(unsafe.Pointer(&sig[0])),
		(C.size_t)(SigLen),
	)
	if len(sig.Bytes()) == 0 {
		return sig, errors.New("failed to sign message")
	}
	return sig, nil
}

func (pub *G2PublicKey) Verify(msg []byte, sig Signature) bool {
	// Verify the message
	return C.verify(
		(*C.uchar)(unsafe.Pointer(pub)),
		(C.size_t)(G2Len),
		(*C.uchar)(unsafe.Pointer(&msg[0])),
		(C.size_t)(len(msg)),
		(*C.uchar)(unsafe.Pointer(&sig[0])),
		(C.size_t)(SigLen),
	) == 1
}

func AggregateSignatures(sigs []Signature, message []byte) (Signature, error) {
	// Aggregate the signatures
	aggSig := Signature{}
	signaturesbytes := make([]byte, 0)
	for _, sig := range sigs {
		signaturesbytes = append(signaturesbytes, sig.Bytes()...)
	}
	if len(signaturesbytes) == 0 {
		return aggSig, errors.New("no signatures to aggregate")
	}
	if len(signaturesbytes)%SigLen != 0 {
		return aggSig, errors.New("invalid signature length")
	}
	C.aggregate_sign(
		(*C.uchar)(unsafe.Pointer(&signaturesbytes[0])),
		(C.size_t)(len(signaturesbytes)),
		(*C.uchar)(unsafe.Pointer(&message[0])),
		(C.size_t)(len(message)),
		(*C.uchar)(unsafe.Pointer(&aggSig[0])),
		(C.size_t)(SigLen),
	)
	if len(aggSig.Bytes()) == 0 {
		return aggSig, errors.New("failed to aggregate signatures")
	}
	return aggSig, nil
}

func AggregateVerify(pubkeys []DoublePublicKey, agg_sig Signature, message []byte) bool {
	// Aggregate verify the message
	pubkeysbytes := make([]byte, 0)
	for _, pubkey := range pubkeys {
		pubkeysbytes = append(pubkeysbytes, pubkey.Bytes()...)
	}
	result := C.aggregate_verify_by_signature(
		(*C.uchar)(unsafe.Pointer(&pubkeysbytes[0])),
		(C.size_t)(len(pubkeysbytes)),
		(*C.uchar)(unsafe.Pointer(&message[0])),
		(C.size_t)(len(message)),
		(*C.uchar)(unsafe.Pointer(&agg_sig[0])),
		(C.size_t)(SigLen),
	)
	return result == C.int(1)
}

func InitBLSKey(seed []byte) (bls_pub DoublePublicKey, bls_priv SecretKey, err error) {
	bls_priv, err = GetSecretKey(seed)
	if err != nil {
		return bls_pub, bls_priv, err
	}
	bls_pub, err = GetDoublePublicKey(seed)
	if err != nil {
		return bls_pub, bls_priv, err
	}
	return bls_pub, bls_priv, nil
}

// Encode converts input data into V Reed-Solomon encoded shards
func Encode(data []byte, V int) ([][]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("input data is empty")
	}
	Cores := V / 3
	shardSize := (len(data) / Cores)
	// TODO: address the padding to multiple of W_E

	// Allocate a single Go-managed buffer for all shards
	output := make([]byte, shardSize*V)
	dataPtr := (*C.uchar)(unsafe.Pointer(&data[0]))
	outputPtr := (*C.uchar)(unsafe.Pointer(&output[0]))

	C.encode(dataPtr, C.size_t(len(data)), C.size_t(V), outputPtr, C.size_t(shardSize))

	// Convert the flat buffer into [][]byte
	shards := make([][]byte, V)
	for i := 0; i < V; i++ {
		start := i * shardSize
		shards[i] = output[start : start+shardSize]
	}

	return shards, nil
}

// Decode reconstructs the original data from encoded shards.
func Decode(shards [][]byte, V int, indexes []uint32, outputSize int) ([]byte, error) {
	Cores := V / 3
	if len(shards) != Cores || len(shards) != len(indexes) {
		log.Crit("bls", "Decode FAIL", "len(shards)", len(shards), "len(indexes)", len(indexes))
		return nil, errors.New("shards and indexes length mismatch")
	}
	
	// check if any indexes are repeated
	seen := make(map[uint32]bool, len(indexes))
	for _, index := range indexes {
		if seen[index] {
			return nil, errors.New("duplicate shard index found")
		}
		seen[index] = true
	}

	shardSize := len(shards[0])
	output := make([]byte, outputSize)

	// Flatten shard data into a contiguous byte slice
	flatShards := make([]byte, len(shards)*shardSize)
	for i := 0; i < len(shards); i++ {
		copy(flatShards[i*shardSize:], shards[i])
	}

	// Convert indexes to a contiguous C-compatible array
	indexesPtr := make([]C.uint, len(indexes))
	for i, val := range indexes {
		indexesPtr[i] = C.uint(val)
	}

	// Convert pointers to C types
	shardsPtr := (*C.uchar)(unsafe.Pointer(&flatShards[0]))
	outputPtr := (*C.uchar)(unsafe.Pointer(&output[0]))
	indexesCPtr := (*C.uint)(unsafe.Pointer(&indexesPtr[0]))

	// Call Rust FFI function
	C.decode(shardsPtr, indexesCPtr, C.size_t(V), C.size_t(shardSize), outputPtr, C.size_t(outputSize))

	return output, nil
}
