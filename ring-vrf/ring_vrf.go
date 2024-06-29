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
	"crypto/rand"
	"encoding/hex"
	"strings"
	"encoding/json"
)

const (
   // length of secret key seed.
   SEED_SERIALIZED_SIZE = 32;
   // length of secret key seed.
   SECRET_SERIALIZED_SIZE = 32;
   // length of serialized public key.
   PUBLIC_SERIALIZED_SIZE = 33;
   // length of serialized signature.
   SIGNATURE_SERIALIZED_SIZE = 784
   // length of serialized pre-output.
   PREOUT_SERIALIZED_SIZE = 33
)

// Jam Paper p13 - 336 bytes total
type ValidatorKeySet struct {
	BanderSnatch [32]byte
	Ed25519 [32]byte
	BLS [144]byte
	Metadata [128]byte
}


type SecretKey = [SECRET_SERIALIZED_SIZE]byte
type PublicKey = [PUBLIC_SERIALIZED_SIZE]byte
type Seed = [SEED_SERIALIZED_SIZE]byte
type Signature = [SIGNATURE_SERIALIZED_SIZE]byte

func GenerateKey() (SecretKey, PublicKey) {
   var sk SecretKey
   var pk PublicKey
   return  sk, pk
}

func RingVRFSignSimple(secret SecretKey, sassafrasTicketSeal, message []byte, ticketVRFInput []byte) ([]byte, []byte) {
     signature := []byte{}
     output := []byte{}
     return signature, output
}

func RingVRFVerifySimple(ticketVRFInput, extra []byte, signature [SIGNATURE_SERIALIZED_SIZE]byte) error {
     return nil
}

// return 32 byte ticket
func VRFOutput(secret SecretKey, ticket_vrf_input []byte) []byte {
     output := []byte{}
	 //TODO: need to port vrf_signed_output
     return output
}

func RingVRFSignedOutput(signature [SIGNATURE_SERIALIZED_SIZE]byte) []byte {
     output := []byte{}
	 //TODO: need to port vrf_signed_output
     return output // this is the ticketID
}

func RingVRFSignPKS(secret, domain, message, transcript []byte, pks [][]byte) ([]byte, error) {
    secretPtr := (*C.uchar)(unsafe.Pointer(&secret[0]))
    domainPtr := (*C.uchar)(unsafe.Pointer(&domain[0]))
    messagePtr := (*C.uchar)(unsafe.Pointer(&message[0]))
    transcriptPtr := (*C.uchar)(unsafe.Pointer(&transcript[0]))

    // Flatten the pks slice of byte slices into a single byte slice
    var flatPKS []byte
    for _, pk := range pks {
        if len(pk) != 33 {
            return nil, fmt.Errorf("invalid public key length: expected 33 bytes, got %d", len(pk))
        }
        flatPKS = append(flatPKS, pk...)
    }
    if len(flatPKS) == 0 {
        return nil, fmt.Errorf("no public keys provided")
    }
    pksPtr := (*C.uchar)(unsafe.Pointer(&flatPKS[0]))

    // Print debugging information
    fmt.Printf("Flattened PKS length: %d\n", len(flatPKS))
    fmt.Printf("Flattened PKS: %x\n", flatPKS)

    // Allocate memory for the signature in Go
    signatureLen := 163
    signature := make([]byte, signatureLen)
    signaturePtr := (*C.uchar)(unsafe.Pointer(&signature[0]))

    result := C.ring_vrf_sign_c_pks(
        secretPtr,
        domainPtr, C.size_t(len(domain)),
        messagePtr, C.size_t(len(message)),
        transcriptPtr, C.size_t(len(transcript)),
        pksPtr, C.size_t(len(flatPKS)),
        signaturePtr, C.size_t(signatureLen),
    )
    if result != 0 {
        return nil, fmt.Errorf("failed to sign with error code: %d", result)
    }

    return signature, nil
}

func RingVRFVerifyPKS(domain, message, transcript, signature []byte, pks [][]byte) (int, error) {
    domainPtr := (*C.uchar)(unsafe.Pointer(&domain[0]))
    messagePtr := (*C.uchar)(unsafe.Pointer(&message[0]))
    transcriptPtr := (*C.uchar)(unsafe.Pointer(&transcript[0]))
    signaturePtr := (*C.uchar)(unsafe.Pointer(&signature[0]))

    // Flatten the pks slice of byte slices into a single byte slice
    var flatPKS []byte
    for _, pk := range pks {
        if len(pk) != 33 {
            return 0, fmt.Errorf("invalid public key length: expected 33 bytes, got %d", len(pk))
        }
        flatPKS = append(flatPKS, pk...)
    }
    if len(flatPKS) == 0 {
        return 0, fmt.Errorf("no public keys provided")
    }
    pksPtr := (*C.uchar)(unsafe.Pointer(&flatPKS[0]))

    // Print debugging information
    fmt.Printf("Flattened PKS length: %d\n", len(flatPKS))
    fmt.Printf("Flattened PKS: %x\n", flatPKS)

    result := C.ring_vrf_verify_c_pks(
        domainPtr, C.size_t(len(domain)),
        messagePtr, C.size_t(len(message)),
        transcriptPtr, C.size_t(len(transcript)),
        pksPtr, C.size_t(len(flatPKS)),
        signaturePtr, C.size_t(len(signature)),
    )
    if result != 0 {
        return 0, fmt.Errorf("verification failed with error code: %d", result)
    }

    return 1, nil
}

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
	fmt.Printf("RingVRFSign signature=%x\n, result=%x", signature, result)
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

func InitByteSliceFromHex(hexString string) ([]byte, error) {
	if strings.HasPrefix(hexString, "0x") {
		hexString = hexString[2:]
	}
	bytes, err := hex.DecodeString(hexString)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex string: %v", err)
	}
	return bytes, nil
}

func RingVRFPublicKey(secret []byte) ([]byte, error) {
	if len(secret) != 32 {
		return nil, fmt.Errorf("invalid secret key length: expected 32 bytes")
	}

	pubkey := make([]byte, 33) // Public key length is 33 bytes
	result := C.ring_vrf_public_key(
		(*C.uchar)(unsafe.Pointer(&secret[0])),
		(*C.uchar)(unsafe.Pointer(&pubkey[0])),
		C.size_t(len(pubkey)),
	)

	if result != 0 {
		return nil, fmt.Errorf("failed to generate public key, error code: %d", result)
	}

	return pubkey, nil
}

func GenerateRandomSecret() ([]byte, error) {
	secret := make([]byte, 32)
	_, err := rand.Read(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random secret: %v", err)
	}
	return secret, nil
}

// GeneratePKS generates the specified number of secrets and their corresponding public keys.
func GeneratePKS(ringSize int) ([][]byte, [][]byte, error) {
	secrets := make([][]byte, ringSize)
	pubkeys := make([][]byte, ringSize)

	for i := 0; i < ringSize; i++ {
		secret, err := GenerateRandomSecret()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate secret: %v", err)
		}
		pubkey, err := RingVRFPublicKey(secret)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate public key: %v", err)
		}
		//fmt.Printf("Key#%v - (Priv, Pub)=(%x,%x)\n", i, secret, pubkey)
		secrets[i] = secret
		pubkeys[i] = pubkey
	}
	return secrets, pubkeys, nil
}

func MarshalPKS(pk_secrets [][]byte, pks [][]byte) (string, error) {
	hexSecrets := make([]string, len(pk_secrets))
	for i, secret := range pk_secrets {
		hexSecrets[i] = hex.EncodeToString(secret)
	}

	hexPKs := make([]string, len(pks))
	for i, pk := range pks {
		hexPKs[i] = hex.EncodeToString(pk)
	}

	data := map[string][]string{
		"pk_secrets": hexSecrets,
		"pks":        hexPKs,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal PKS: %v", err)
	}

	return string(jsonData), nil
}

func UnmarshalPKS(jsonData string) ([][]byte, [][]byte, error) {
	data := make(map[string][]string)

	err := json.Unmarshal([]byte(jsonData), &data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal PKS: %v", err)
	}

	pk_secrets := make([][]byte, len(data["pk_secrets"]))
	for i, hexSecret := range data["pk_secrets"] {
		secret, err := hex.DecodeString(hexSecret)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode hex secret: %v", err)
		}
		pk_secrets[i] = secret
	}

	pks := make([][]byte, len(data["pks"]))
	for i, hexPK := range data["pks"] {
		pk, err := hex.DecodeString(hexPK)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode hex public key: %v", err)
		}
		pks[i] = pk
	}

	return pk_secrets, pks, nil
}
