package bandersnatch

/*
#cgo LDFLAGS: -L target/release -lbandersnatch
#include <stdint.h>
#include <stdlib.h>
#include <bandersnatch.h>
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

type PrivateKey []byte
type PublicKey []byte
type Seed []byte

const (
	RingSignatureLen = 784
	IETFSignatureLen = 96
	VRFOutputLen     = 32
)

func GetPublicKey(seed Seed) (PublicKey, error) {
	pubKey := make([]byte, 32) // Adjust size as necessary
	C.get_public_key(
		(*C.uchar)(unsafe.Pointer(&seed[0])),
		C.size_t(len(seed)),
		(*C.uchar)(unsafe.Pointer(&pubKey[0])),
		C.size_t(len(pubKey)),
	)
	return pubKey, nil
}

func GetPrivateKey(seed Seed) (PrivateKey, error) {
	secret := make([]byte, 32) // Adjust size as necessary
	C.get_private_key(
		(*C.uchar)(unsafe.Pointer(&seed[0])),
		C.size_t(len(seed)),
		(*C.uchar)(unsafe.Pointer(&secret[0])),
		C.size_t(len(secret)),
	)
	return secret, nil
}

func InitRingSet(ringset []PublicKey) (ringsetBytes []byte) {
	// Flatten pubkeys into a single byte slice
	for _, pubkey := range ringset {
		ringsetBytes = append(ringsetBytes, pubkey...)
	}
	return ringsetBytes
}

// Anonymous Ring VRF
// RingVRFSign is Used for tickets submission to sign ticket anonymously. Output and Ring Proof bundled together (as per section 2.2)
func RingVrfSign(privateKey PrivateKey, ringsetBytes, vrfInputData, auxData []byte /*, proverIdx int*/) ([]byte, []byte, error) {
	sig := make([]byte, RingSignatureLen) // 784 bytes
	vrfOutput := make([]byte, 32)
	C.ring_vrf_sign(
		(*C.uchar)(unsafe.Pointer(&privateKey[0])),
		C.size_t(len(privateKey)),
		(*C.uchar)(unsafe.Pointer(&ringsetBytes[0])),
		C.size_t(len(ringsetBytes)),
		//C.size_t(proverIdx),
		(*C.uchar)(unsafe.Pointer(&vrfInputData[0])),
		C.size_t(len(vrfInputData)),
		(*C.uchar)(unsafe.Pointer(&auxData[0])),
		C.size_t(len(auxData)),
		(*C.uchar)(unsafe.Pointer(&sig[0])),
		C.size_t(len(sig)),
		(*C.uchar)(unsafe.Pointer(&vrfOutput[0])),
		C.size_t(len(vrfOutput)),
		//C.size_t(proverIdx),
	)
	return sig, vrfOutput, nil
}

// RingVRFVerify is Used for tickets verification, and returns vrfOutput on success
func RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte) ([]byte, error) {
	vrfOutput := make([]byte, VRFOutputLen)
	auxDataL := C.size_t(len(auxData))
	auxDataF := auxData
	if len(auxData) == 0 {
		auxDataF = []byte{1}
		auxDataL = C.size_t(0)
	}

	result := C.ring_vrf_verify(
		(*C.uchar)(unsafe.Pointer(&ringsetBytes[0])),
		C.size_t(len(ringsetBytes)),
		(*C.uchar)(unsafe.Pointer(&signature[0])),
		C.size_t(len(signature)),
		(*C.uchar)(unsafe.Pointer(&vrfInputData[0])),
		C.size_t(len(vrfInputData)),
		(*C.uchar)(unsafe.Pointer(&auxDataF[0])),
		auxDataL,
		(*C.uchar)(unsafe.Pointer(&vrfOutput[0])),
		C.size_t(len(vrfOutput)),
	)
	if result != 1 {
		return nil, fmt.Errorf("verification failed")
	}
	return vrfOutput, nil
}

// Non Anonymous IETF VRF
// IetfVrfSign is Used for ticket claiming during block production.
func IetfVrfSign(privateKey PrivateKey, vrfInputData, auxData []byte) ([]byte, []byte, error) {
	sig := make([]byte, IETFSignatureLen) // 96 bytes
	vrfOutput := make([]byte, 32)
	C.ietf_vrf_sign(
		(*C.uchar)(unsafe.Pointer(&privateKey[0])),
		C.size_t(len(privateKey)),
		(*C.uchar)(unsafe.Pointer(&vrfInputData[0])),
		C.size_t(len(vrfInputData)),
		(*C.uchar)(unsafe.Pointer(&auxData[0])),
		C.size_t(len(auxData)),
		(*C.uchar)(unsafe.Pointer(&sig[0])),
		C.size_t(len(sig)),
		(*C.uchar)(unsafe.Pointer(&vrfOutput[0])),
		C.size_t(len(vrfOutput)),
	)
	return sig, vrfOutput, nil
}

// IetfVrfVerifyAndGenerateVrfOutput is Used for ticket claim verification during block import
// returns vrfOutput on success
// NOTE: this external func should use PublicKey directly instead of index
func IetfVrfVerify(pubKey PublicKey, signature, vrfInputData, auxData []byte) ([]byte, error) {
	vrfOutput := make([]byte, VRFOutputLen)
	result := C.ietf_vrf_verify(
		(*C.uchar)(unsafe.Pointer(&pubKey[0])),
		C.size_t(len(pubKey)),
		(*C.uchar)(unsafe.Pointer(&signature[0])),
		C.size_t(len(signature)),
		(*C.uchar)(unsafe.Pointer(&vrfInputData[0])),
		C.size_t(len(vrfInputData)),
		(*C.uchar)(unsafe.Pointer(&auxData[0])),
		C.size_t(len(auxData)),
		(*C.uchar)(unsafe.Pointer(&vrfOutput[0])),
		C.size_t(len(vrfOutput)),
	)
	if result != 1 {
		return nil, fmt.Errorf("verification failed")
	}
	return vrfOutput, nil
}

/*
since VRFSignedOutput(ringSig) and  VRFSignedOutput(ietfSig) yield same output_hash,
it should be possible to compute ticketID without goign through RingVRFSign
*/
// Return vrfOutput given PrivateKey, vrfInputData
func VRFOutput(privateKey PrivateKey, vrfInputData, auxData []byte) ([]byte, error){
	vrfOutput := make([]byte, 32)
	_, vrfOutput, err := IetfVrfSign(privateKey, vrfInputData, auxData)
	if err != nil {
		return nil, fmt.Errorf("Error Getting vrfOutput")
	}
	return vrfOutput, nil
}

// Return vrfOutput given valid signature -- inputs are different though so probably not necessary?
func VRFSignedOutput(signature []byte) ([]byte, error) {
	vrfOutput := make([]byte, 32)
	if len(signature) == RingSignatureLen {
		result := C.get_ring_vrf_output(
			(*C.uchar)(unsafe.Pointer(&signature[0])),
			C.size_t(len(signature)),
			(*C.uchar)(unsafe.Pointer(&vrfOutput[0])),
			C.size_t(len(vrfOutput)),
		)
		if result != 1 {
			return nil, fmt.Errorf("failed to get Ring VRF output")
		}
		return vrfOutput, nil
	} else if len(signature) == IETFSignatureLen {
		result := C.get_ietf_vrf_output(
			(*C.uchar)(unsafe.Pointer(&signature[0])),
			C.size_t(len(signature)),
			(*C.uchar)(unsafe.Pointer(&vrfOutput[0])),
			C.size_t(len(vrfOutput)),
		)
		if result != 1 {
			return nil, fmt.Errorf("failed to get IETF VRF output")
		}
	} else {
		return nil, errors.New("invalid signature length")
	}
	return vrfOutput, nil
}
