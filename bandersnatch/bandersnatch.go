package bandersnatch

/*
#cgo LDFLAGS: -L target/release -lbandersnatch
#include "bandersnatch.h"
*/
import "C"
import (
	//"fmt"
	"unsafe"
	"errors"
)

type SecretKey []byte
type PublicKey []byte

const (
	RingSignatureLen = 784
	IETFSignatureLen = 123 // TODO: what's the size of this
)

func GenerateKeyFromSeed(seed []byte) (PublicKey, SecretKey){
	//call bandersnatch::Secret::from_seed() to generate key
	pub := PublicKey{}  // Initialize an empty PublicKey
	sk := SecretKey{}   // Initialize an empty SecretKey
	return pub, sk
}

func InitRingSet(ringset []PublicKey) (ringsetBytes []byte){
	// Flatten pubkeys into a single byte slice
    for _, pubkey := range ringset {
        ringsetBytes = append(ringsetBytes, pubkey...)
    }
	return ringsetBytes
}

//Return vrfOutput given SecretKey & vrfInputData
func VRFOutput(authority_secret_key SecretKey, vrfInputData []byte) ([]byte, error){
	return []byte{}, nil
}

//Return vrfOutput given valid
func VRFSignedOutput(VRFSignature []byte) ([]byte, error) {
	vrfOutput := make([]byte, 32)
	if (len(VRFSignature) == RingSignatureLen){
		//let signature = match RingVrfSignature::deserialize_compressed(signature)
		//vrfOutput = signature.output.hash()[..32].try_into().unwrap();
	}else if (len(VRFSignature) == IETFSignatureLen){
		//let signature = match IetfVrfSignature::deserialize_compressed(signature)
		//vrfOutput = signature.output.hash()[..32].try_into().unwrap();
	}else{
		return nil, errors.New("invalid signature length")
	}
	return vrfOutput, nil
}


// Anonymous VRF (aka ring vrf)

/**
Source: ring_vrf_verify
Used for tickets verification.
RingVRFVerify calls the external Rust function to verify the VRF signature and returns vrfOutput on success
*/
func RingVRFVerify(ringsetBytes []byte , vrfInputData, auxData, signature []byte) ([]byte, int) {

    // Allocate memory for the VRF output hash
    vrfOutput := make([]byte, 32)

    auxDataL := C.uint64_t(len(auxData))
    auxDataF := auxData
    if len(auxData) == 0 {
        auxDataF = []byte{1}
        auxDataL = C.uint64_t(0)
    }

    // Call the Rust function
    res := C.ring_vrf_verify_external(
        (*C.uint8_t)(unsafe.Pointer(&ringsetBytes[0])),
        C.uint64_t(len(ringsetBytes)),
        (*C.uint8_t)(unsafe.Pointer(&signature[0])),
        C.uint64_t(len(signature)),
        (*C.uint8_t)(unsafe.Pointer(&vrfInputData[0])),
        C.uint64_t(len(vrfInputData)),
        (*C.uint8_t)(unsafe.Pointer(&auxDataF[0])),
        C.uint64_t(auxDataL),
        (*C.uint8_t)(unsafe.Pointer(&vrfOutput[0])),
    )

    return vrfOutput, int(res)
}


/**
Source: ring_vrf_sign
Used for tickets submission.
RingVRFSign calls the external Rust function to sign ticket anonymously. Output and Ring Proof bundled together (as per section 2.2)
*/
func RingVRFSign(ringsetBytes, authority_secret_key SecretKey, vrfInputData, auxData []byte) ([]byte, error) {
	//ring_signature := C.ring_vrf_sign_external(ringsetBytes, ringsetBytesL, authority_secret_key, authority_secret_key_L, vrfInputData, vrfInputDataL, auxData, auxDataL)
	return []byte{}, nil
}


// Non Anonymous VRF (aka IETF VRF)

/**
Source: ietf_vrf_verify
Used for ticket claim verification during block import.
returns vrfOutput on success
*/
func IETFVRFVerify(authority_public_key PublicKey, vrfInputData, auxData, signature []byte) ([]byte, int){
	// Allocate memory for the VRF output hash
	vrfOutput := make([]byte, 32)
	res := 0
	//NOTE. this external func should use PublicKey directly instead of index
	//res := C.ietf_vrf_verify_external(authority_public_key, authority_public_key_L, vrfInputData, vrfInputDataL, auxData, auxDataL, signature, signatureL)
	return vrfOutput, int(res)
}

/**
Source: ietf_vrf_sign
Used for ticket claiming during block production.
*/
func IETFVRFSign(authority_secret_key SecretKey, vrfInputData, auxData []byte) ([]byte, error){
	//ietf_signature := C.ietf_vrf_sign_external(authority_secret_key,authority_secret_key_L, vrfInputData, vrfInputDataL, auxData, auxDataL)
	return []byte{}, nil
}
