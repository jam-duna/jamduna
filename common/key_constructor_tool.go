package common

import (
	"encoding/binary"
	//"fmt"
)

// (h,l) -> E4(l)++H(h) for a_l
func Compute_preimageLookup_internal(blob_hash Hash, blob_len uint32) Hash {
	lBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lBytes, blob_len)        // E4(l)
	h_blobHash := ComputeHash(blob_hash.Bytes())           // H(h) -- hash of blobHash
	al_internal_key := append(lBytes, h_blobHash[2:30]...) // (E4(l) ⌢ H(h) -- this is 36 bytes. but only the first 32 bytes matters
	return BytesToHash(al_internal_key)
}

// h -> E4(2^32-2)++h1...h29 for a_p
func Compute_preimageBlob_internal(blob_hash Hash) Hash {
	//2^32 - 2 or fffffffe (BE)
	prefixBytes := make([]byte, 4)
	prefix := uint32((1 << 32) - 2)
	binary.LittleEndian.PutUint32(prefixBytes, prefix)

	ap_internal_key := append(prefixBytes, blob_hash[1:29]...)
	return BytesToHash(ap_internal_key)
}

// k -> E4(2^32-1)++k0...k28 for a_s

func Compute_storageKey_internal(key_hash Hash) Hash {
	prefixBytes := make([]byte, 4)
	prefix := uint32((1 << 32) - 1)
	binary.LittleEndian.PutUint32(prefixBytes, prefix)
	return BytesToHash(append(prefixBytes, key_hash[0:28]...))
}

// EQ 290 - state-key constructor functions C
func ComputeC_i(i uint8) Hash {
	//i ∈ N_8 ↦ [i,0,0,...]
	stateKey := make([]byte, 32)
	stateKey[0] = byte(i)
	return BytesToHash(stateKey)
}

// used in GetService + SetService (1/4) https://graypaper.fluffylabs.dev/#/5f542d7/382b03382b03
func ComputeC_is(s uint32) Hash {
	// (i,s ∈ N_S) ↦ [i, n0, n1, n2, n3, 0, 0, ...] where n = E4(s)
	stateKey := make([]byte, 32)
	stateKey[0] = 255
	stateKey[1] = byte(s)
	stateKey[3] = byte(s >> 8)
	stateKey[5] = byte(s >> 16)
	stateKey[7] = byte(s >> 24)
	return BytesToHash(stateKey)
}

func ComputeC_sh(s uint32, h0 Hash) Hash {
	// (s,h) ↦ [n0, h0, n1, h1, n2, h2, n3, h3, h4, h5, ... , h27]
	h := h0.Bytes()

	// n0, h0, n1, h1, n2, h2, n3, h3
	var stateKey [32]byte
	for i := 0; i < 4; i++ {
		stateKey[2*i] = byte(s >> (8 * i)) // compute the little-endian byte for s
		stateKey[2*i+1] = h[i]
	}

	// h4..h7
	copy(stateKey[8:32], h[4:28])

	// fmt.Printf("ComputeC_sh(s=%d, h=%s)=%x\n", s, h0, stateKey)
	return BytesToHash(stateKey[:])
}

func ComputeC_sh_Byte(s uint32, k []byte) Hash {
	var stateKey [32]byte
	copy(stateKey[:], k)
	return ComputeC_sh(s, BytesToHash(stateKey[:]))
}

func ServiceStorageKey(s uint32, k []byte) Hash {
	sb := make([]byte, 4)
	binary.LittleEndian.PutUint32(sb, uint32(s))
	return Blake2Hash(append(sb, k...))
}
