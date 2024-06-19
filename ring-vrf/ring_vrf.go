package ring_vrf

/*
#cgo LDFLAGS: -L./target/release -l bandersnatch_vrfs
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include "ring_vrf.h"
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func RingVRFSign(secret, domain, message, transcript []byte) ([]byte, error) {
	secretPtr := (*C.char)(unsafe.Pointer(&secret[0]))
	domainPtr := (*C.char)(unsafe.Pointer(&domain[0]))
	messagePtr := (*C.char)(unsafe.Pointer(&message[0]))
	transcriptPtr := (*C.char)(unsafe.Pointer(&transcript[0]))

	// Allocate memory for the signature in Go
	signatureLen := 163
	signature := make([]byte, signatureLen)
	signaturePtr := (*C.char)(unsafe.Pointer(&signature[0])) // Use C.char for C-style string

	result := C.ring_vrf_sign_c(
		secretPtr,
		domainPtr, C.size_t(len(domain)),
		messagePtr, C.size_t(len(message)),
		transcriptPtr, C.size_t(len(transcript)),
		signaturePtr, C.size_t(signatureLen),
	)
	if result != 0 {
		return nil, fmt.Errorf("failed to sign with error code: %d", result)
	}

	return signature, nil
}

func RingVRFVerify(pks, pubkey, domain, message, transcript, signature []byte) error {
	pksPtr := (*C.char)(unsafe.Pointer(&pks[0]))
	pubkeyPtr := (*C.char)(unsafe.Pointer(&pubkey[0]))
	domainPtr := (*C.char)(unsafe.Pointer(&domain[0]))
	messagePtr := (*C.char)(unsafe.Pointer(&message[0]))
	transcriptPtr := (*C.char)(unsafe.Pointer(&transcript[0]))
	signaturePtr := (*C.char)(unsafe.Pointer(&signature[0])) // Use C.char for C-style string

	result := C.ring_vrf_verify_c(
		pksPtr,
		pubkeyPtr, C.size_t(len(pubkey)),
		domainPtr, C.size_t(len(domain)),
		messagePtr, C.size_t(len(message)),
		transcriptPtr, C.size_t(len(transcript)),
		signaturePtr, C.size_t(len(signature)),
	)
	if result != 0 {
		return fmt.Errorf("failed to verify with error code: %d", result)
	}

	return nil
}

