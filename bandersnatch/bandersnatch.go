//go:build cgo
// +build cgo

package bandersnatch

/*
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/../ffi -lcrypto.linux_amd64 -ldl
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/../ffi -lcrypto.linux_arm64 -ldl
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/../ffi -lcrypto.mac_amd64 -ldl
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/../ffi -lcrypto.mac_arm64 -ldl
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/../ffi -lcrypto.windows_amd64 -lws2_32

#include <stdint.h>
#include <stdlib.h>
#include <bandersnatch.h>
*/
import "C"

import (
	"bytes"
	"sync"

	//"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"unsafe"

	"github.com/colorfulnotion/jam/common"
	//"github.com/colorfulnotion/jam/types"
)

/*
Appendix G: Bandersnatch IETF Ring VRF (As defined in 3.8.1)

Functionality:
- GP uses three different expressions to specify the same idea: {auxData, vrfInputData}, {m, c}, {m, x}.
- These are functionally equivalent:
  F_[auxData]H⟨vrfInputData⟩ ≡ F_[m]H⟨c⟩ ≡ F_[m]H⟨x⟩
- VRFSignedOutput(vrfSignature) outputs a 32-byte verifiably random value.
- auxData:
  - Does not affect vrfOutput.
  - Mutates the signature.

Behavior of auxData:
- Let auxData1 != auxData2:
  - VRFSig1 = F_[auxData1]H⟨vrfInputData⟩
  - VRFSig2 = F_[auxData2]H⟨vrfInputData⟩
- Then:
  VRFSig1 != VRFSig2
  VRFSignedOutput(VRFSig1) = VRFSignedOutput(VRFSig2)

G.2: IETF for Block Sealing
- Formula:
  F_[m]H_B⟨c⟩ ⊂ Y96
- Components:
  - H_B: 32-byte bandersnatch_priv from Validator.
  - C (Context): Derived from VrfInputData.
  - M (Message): Derived from AuxData.
  - O (Output): Y96 Bytes (IETFSignature).

G.3: Ring for Ticket Generation
- Formula:
  F_[m]H_R⟨c⟩ ⊂ Y784
- Components:
  - H_R: Ringset root specific to the set of validators, derived from KZG_commitment.
  - C (Context): Derived from VrfInputData.
  - M (Message): Derived from AuxData.
  - O (Output): 784 Bytes (RingSignature).
*/

const (
	X_BANDERSNATCH_SEED = "jam_val_key_bandersnatch"
	MAX_CACHE_SIZE      = 64
)

type lruCache[T any] struct {
	mu      sync.RWMutex
	cache   map[string]T
	keys    []string
	maxSize int
	cleanup func(T)
}

func newLRUCache[T any](maxSize int, cleanup func(T)) *lruCache[T] {
	return &lruCache[T]{
		cache:   make(map[string]T),
		keys:    make([]string, 0),
		maxSize: maxSize,
		cleanup: cleanup,
	}
}

func (c *lruCache[T]) get(key string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.cache[key]
	return val, ok
}

func (c *lruCache[T]) put(key string, value T) {
	isDebug := false
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.cache[key]; !exists {
		if isDebug {
			fmt.Printf("DEBUG: Cache size before put: %d (max: %d)\n", len(c.cache), c.maxSize)
		}
		if len(c.cache) >= c.maxSize {
			if isDebug {
				fmt.Printf("DEBUG: Triggering eviction...\n")
			}
			c.evictOldest()
		}
		c.cache[key] = value
		c.keys = append(c.keys, key)
		if isDebug {
			fmt.Printf("DEBUG: Added to cache, size now: %d\n", len(c.cache))
		}
	}
}

func (c *lruCache[T]) evictOldest() {
	if len(c.keys) == 0 {
		return
	}
	oldest := c.keys[0]
	if value, ok := c.cache[oldest]; ok {
		if c.cleanup != nil {
			c.cleanup(value)
		}
		delete(c.cache, oldest)
	}
	c.keys = c.keys[1:]
}

var commitmentCache = newLRUCache[[]byte](MAX_CACHE_SIZE, nil)
var verifierCache_global = newLRUCache[uintptr](MAX_CACHE_SIZE, func(ptr uintptr) {
	fmt.Printf("DEBUG: free_verifier called for ptr=%x\n", ptr)
	C.free_verifier(unsafe.Pointer(ptr))
})

type BanderSnatchSecret [SecretLen]byte
type BanderSnatchKey [PubkeyLen]byte
type BandersnatchVrfSignature [IETFSignatureLen]byte
type BandersnatchRingSignature [RingSignatureLen]byte

type Seed []byte

func (bk BanderSnatchKey) Bytes() []byte {
	return bk[:]
}

func (bs BanderSnatchSecret) Bytes() []byte {
	return bs[:]
}

func (bk BanderSnatchKey) String() string {
	return hex.EncodeToString(bk.Bytes())
}

func (bs BanderSnatchSecret) String() string {
	return hex.EncodeToString(bs.Bytes())
}

func BytesToBanderSnatchSecret(b []byte) (bs BanderSnatchSecret, err error) {
	if len(b) != SecretLen {
		return BanderSnatchSecret{}, fmt.Errorf("invalid byte slice length: expected %d bytes, got %d", SecretLen, len(b))
	}
	copy(bs[:], b)
	return bs, nil
}

func BytesToBanderSnatchKey(b []byte) (bs BanderSnatchKey, err error) {
	if len(b) != SecretLen {
		return BanderSnatchKey{}, fmt.Errorf("invalid byte slice length: expected %d bytes, got %d", SecretLen, len(b))
	}
	copy(bs[:], b)
	return bs, nil
}

// InitBanderSnatchKey initializes the BanderSnatch keys using the provided seed.
func InitBanderSnatchKey(seed []byte) (key BanderSnatchKey, secret BanderSnatchSecret, err error) {
	// Check if the seed length is 32 bytes
	if len(seed) != SeedLen {
		return key, secret, fmt.Errorf("seed length must be %v bytes", SeedLen)
	}

	//bandersnatch_seed := seed
	bandersnatch_seed := common.ComputeHash(append([]byte(X_BANDERSNATCH_SEED), seed...))

	// Retrieve the public key
	banderSnatch_pub, err := getBanderSnatchPublicKey(bandersnatch_seed)
	if err != nil {
		return key, secret, fmt.Errorf("failed to get public key: %v", err)
	}

	// Retrieve the private key
	banderSnatch_priv, err := getBanderSnatchPrivateKey(bandersnatch_seed)
	if err != nil {
		return key, secret, fmt.Errorf("failed to get private key: %v", err)
	}
	// fmt.Printf("seed:%x\n", seed)
	// fmt.Printf("bandersnatch_seed: %x\n", bandersnatch_seed)
	// fmt.Printf("banderSnatch_pub: %x\n", banderSnatch_pub)
	// fmt.Printf("banderSnatch_priv: %x\n", banderSnatch_priv)

	return banderSnatch_pub, banderSnatch_priv, nil
}

// goal : speed up the processes
func getBanderSnatchPublicKey(seed []byte) (BanderSnatchKey, error) {
	//pubKey := make([]byte, 32) // Adjust size as necessary
	pubKey := BanderSnatchKey{}
	C.get_public_key(
		(*C.uchar)(unsafe.Pointer(&seed[0])),
		C.size_t(len(seed)),
		(*C.uchar)(unsafe.Pointer(&pubKey[0])),
		C.size_t(len(pubKey)),
	)
	return pubKey, nil
}

func getBanderSnatchPrivateKey(seed []byte) (BanderSnatchSecret, error) {
	//secret := make([]byte, 32) // Adjust size as necessary
	secret := BanderSnatchSecret{}
	C.get_private_key(
		(*C.uchar)(unsafe.Pointer(&seed[0])),
		C.size_t(len(seed)),
		(*C.uchar)(unsafe.Pointer(&secret[0])),
		C.size_t(len(secret)),
	)
	return secret, nil
}

func InitRingSet(ringset []BanderSnatchKey) (ringsetBytes []byte) {
	// Flatten pubkeys into a single byte slice
	for _, pubkey := range ringset {
		ringsetBytes = append(ringsetBytes, pubkey[:]...)
	}
	return ringsetBytes
}

// Anonymous Ring VRF
// RingVRFSign is Used for tickets submission to sign ticket anonymously. Output and Ring Proof bundled together (as per section 2.2)
func RingVrfSign(ringSize int, privateKey BanderSnatchSecret, ringsetBytes, vrfInputData, auxData []byte) ([]byte, []byte, error) {
	sig := make([]byte, RingSignatureLen) // 784 bytes
	vrfOutput := make([]byte, 32)
	auxDataL := C.size_t(len(auxData))
	auxDataF := auxData
	if len(auxData) == 0 {
		auxDataF = []byte{1}
		auxDataL = C.size_t(0)
	}
	if len(ringsetBytes) == 0 {
		return []byte{}, []byte{}, fmt.Errorf("Not able to sign without ringset bytes")
	}
	result := C.ring_vrf_sign(
		(*C.uchar)(unsafe.Pointer(&privateKey[0])),
		C.size_t(len(privateKey)),
		(*C.uchar)(unsafe.Pointer(&ringsetBytes[0])),
		C.size_t(len(ringsetBytes)),
		C.size_t(ringSize),
		(*C.uchar)(unsafe.Pointer(&vrfInputData[0])),
		C.size_t(len(vrfInputData)),
		(*C.uchar)(unsafe.Pointer(&auxDataF[0])),
		auxDataL,
		(*C.uchar)(unsafe.Pointer(&sig[0])),
		C.size_t(len(sig)),
		(*C.uchar)(unsafe.Pointer(&vrfOutput[0])),
		C.size_t(len(vrfOutput)),
	)
	if result != 1 {
		return nil, nil, fmt.Errorf("failed to RingVrfSign")
	}
	return sig, vrfOutput, nil
}

func RingVrfVerifyWithCache(ringSize int, ringsetBytes []byte, signature, vrfInputData, auxData []byte) ([]byte, error) {
	if len(ringsetBytes) == 0 {
		return []byte{}, fmt.Errorf("No ringsetBytes")
	}

	hash := common.Blake2Hash(ringsetBytes)
	key := hex.EncodeToString(hash.Bytes())

	ptr, ok := verifierCache_global.get(key)

	if !ok {
		v := C.create_verifier(
			(*C.uchar)(unsafe.Pointer(&ringsetBytes[0])),
			C.size_t(len(ringsetBytes)),
			C.size_t(ringSize),
		)
		if v == nil {
			return nil, fmt.Errorf("create verifier failed")
		}
		ptr = uintptr(v)
		verifierCache_global.put(key, ptr)
	}

	out := make([]byte, VRFOutputLen)
	auxL := C.size_t(len(auxData))
	aux := auxData
	if len(auxData) == 0 {
		aux = []byte{1}
		auxL = C.size_t(0)
	}

	res := C.ring_vrf_verify_with_verifier(
		unsafe.Pointer(ptr),
		(*C.uchar)(unsafe.Pointer(&signature[0])),
		C.size_t(len(signature)),
		(*C.uchar)(unsafe.Pointer(&vrfInputData[0])),
		C.size_t(len(vrfInputData)),
		(*C.uchar)(unsafe.Pointer(&aux[0])),
		auxL,
		(*C.uchar)(unsafe.Pointer(&out[0])),
		C.size_t(len(out)),
	)

	if res != 1 {
		return nil, fmt.Errorf("verification failed")
	}
	return out, nil
}

func RingVrfVerify(ringSize int, ringsetBytes []byte, signature, vrfInputData, auxData []byte) ([]byte, error) {
	vrfOutput := make([]byte, VRFOutputLen)
	auxDataL := C.size_t(len(auxData))
	auxDataF := auxData
	if len(auxData) == 0 {
		auxDataF = []byte{1}
		auxDataL = C.size_t(0)
	}
	if len(ringsetBytes) == 0 {
		return []byte{}, fmt.Errorf("No ringsetBytes")
	}
	result := C.ring_vrf_verify(
		(*C.uchar)(unsafe.Pointer(&ringsetBytes[0])),
		C.size_t(len(ringsetBytes)),
		C.size_t(ringSize),
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
func IetfVrfSign(privateKey BanderSnatchSecret, vrfInputData, auxData []byte) ([]byte, []byte, error) {
	sig := make([]byte, IETFSignatureLen) // 96 bytes
	vrfOutput := make([]byte, 32)
	auxDataL := C.size_t(len(auxData))
	auxDataF := auxData
	if len(auxData) == 0 {
		auxDataF = []byte{1}
		auxDataL = C.size_t(0)
	}
	C.ietf_vrf_sign(
		(*C.uchar)(unsafe.Pointer(&privateKey[0])),
		C.size_t(len(privateKey)),
		(*C.uchar)(unsafe.Pointer(&vrfInputData[0])),
		C.size_t(len(vrfInputData)),
		(*C.uchar)(unsafe.Pointer(&auxDataF[0])),
		auxDataL,
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
func IetfVrfVerify(pubKey BanderSnatchKey, signature, vrfInputData, auxData []byte) ([]byte, error) {
	vrfOutput := make([]byte, VRFOutputLen)
	auxDataL := C.size_t(len(auxData))
	auxDataF := auxData
	if len(auxData) == 0 {
		auxDataF = []byte{1}
		auxDataL = C.size_t(0)
	}
	result := C.ietf_vrf_verify(
		(*C.uchar)(unsafe.Pointer(&pubKey[0])),
		C.size_t(len(pubKey)),
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

/*
since VRFSignedOutput(ringSig) and  VRFSignedOutput(ietfSig) yield same output_hash,
it should be possible to compute ticketID without goign through RingVRFSign
*/
// Return vrfOutput given PrivateKey, vrfInputData
func VRFOutput(privateKey BanderSnatchSecret, vrfInputData, auxData []byte) ([]byte, error) {
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

// GetRingCommitment computes ring commitment with caching based on ringsetBytes hash
func GetRingCommitment(ringSize int, ringsetBytes []byte) ([]byte, error) {
	if len(ringsetBytes) == 0 {
		return []byte{}, fmt.Errorf("no ringset")
	}

	// Compute hash of ringsetBytes for cache key
	ringsetHash := common.Blake2Hash(ringsetBytes)
	cacheKey := hex.EncodeToString(ringsetHash.Bytes())

	// Check cache first
	if cachedCommitment, exists := commitmentCache.get(cacheKey); exists {
		// Return a copy to prevent external modification of cached data
		result := make([]byte, len(cachedCommitment))
		copy(result, cachedCommitment)
		return result, nil
	}

	// Not in cache, compute the commitment
	//fmt.Printf("GetRingCommitment (computing): len(ringsetBytes)=%d, ringSize=%d c=%s\n", len(ringsetBytes), ringSize, common.Blake2Hash(ringsetBytes))

	emptyBytes := make([]byte, BlsLen)
	commitmentBytes := make([]byte, BlsLen)
	C.get_ring_commitment(
		(*C.uchar)(unsafe.Pointer(&ringsetBytes[0])),
		C.size_t(len(ringsetBytes)),
		C.size_t(ringSize),
		(*C.uchar)(unsafe.Pointer(&commitmentBytes[0])),
		C.size_t(len(commitmentBytes)),
	)

	if bytes.Equal(emptyBytes, commitmentBytes) {
		return nil, fmt.Errorf("failed to compute ring commitment")
	}

	// Store in cache
	// Make a copy before storing to prevent external modification
	cachedValue := make([]byte, len(commitmentBytes))
	copy(cachedValue, commitmentBytes)
	commitmentCache.put(cacheKey, cachedValue)

	return commitmentBytes, nil
}
