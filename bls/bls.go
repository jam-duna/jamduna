package bls

/*
#cgo LDFLAGS: -L target/release -lbls
#include <stdint.h>
#include <stdlib.h>
#include <bls.h>
*/
import "C"
import (
	"errors"
	"unsafe"
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
	if result == C.int(1) {
		return true
	}
	return false
}

// package main

// /*
// #cgo LDFLAGS: -L /mnt/d/EE303_code/jam/bls/target/release -lbls
// #include <bls.h>
// */
// import "C"
// import (
// 	"fmt"
// 	"unsafe"
// )

// func main() {
// 	seed := []byte{0x00, 0x01, 0x02, 0x03}
// 	var secret [32]byte
// 	C.get_secret_key(
// 		(*C.uchar)(unsafe.Pointer(&seed[0])),
// 		(C.size_t)(len(seed)),
// 		(*C.uchar)(unsafe.Pointer(&secret[0])),
// 		(C.size_t)(32),
// 	)
// 	fmt.Printf("Secret Key: %x\n", secret)
// }
