package bandersnatch

/*
#cgo LDFLAGS: -L target/release -lbandersnatch
#include "bandersnatch.h"
*/
import "C"
import (
	//"fmt"
	"unsafe"
)

type SecretKey []byte
type PublicKey []byte

func RingVRFSign(authority_secret_key SecretKey, domain []byte, message []byte, input []byte) ([]byte, []byte) {
	return []byte{}, []byte{}
}

func VRFOutput(authority_secret_key SecretKey, ticket_vrf_input []byte) []byte {
	return []byte{}
}

// RingVRFVerify calls the external Rust function to verify the VRF signature
func RingVRFVerify(pubkeys [][]byte, vrfInputData, auxData, signature []byte) ([]byte, int) {
	// Flatten pubkeys into a single byte slice
	var pubkeysBytes []byte
	for _, pubkey := range pubkeys {
		pubkeysBytes = append(pubkeysBytes, pubkey...)
	}

	// Allocate memory for the VRF output hash
	vrfOutput := make([]byte, 32)
	//fmt.Printf("pubkeys:%d signature:%d vrfInput:%d auxData:%d\n", len(pubkeysBytes), len(signature), len(vrfInputData), len(auxData))

	auxDataL := C.uint64_t(len(auxData))
	auxDataF := auxData
	if len(auxData) == 0 {
		auxDataF = []byte{1}
		auxDataL = C.uint64_t(0)
	}
	// Call the Rust function
	res := C.ring_vrf_verify_external(
		(*C.uint8_t)(unsafe.Pointer(&pubkeysBytes[0])),
		C.uint64_t(len(pubkeysBytes)),
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
