package common

import (
	"encoding/binary"
	//"fmt"
)

// k -> E4(2^32-1)++k for a_s
func Compute_storageKey_internal(rawKey []byte) []byte {
	prefixBytes := make([]byte, 4)
	prefix := uint32((1 << 32) - 1)
	binary.LittleEndian.PutUint32(prefixBytes, prefix)
	return append(prefixBytes, rawKey[:]...)
}

// h -> E4(2^32-2)++h for a_p
func Compute_preimageBlob_internal(blob_hash Hash) []byte {
	//2^32 - 2 or fffffffe (BE)
	prefixBytes := make([]byte, 4)
	prefix := uint32((1 << 32) - 2)
	binary.LittleEndian.PutUint32(prefixBytes, prefix)
	ap_internal_key := append(prefixBytes, blob_hash.Bytes()...)
	// fmt.Printf("Compute_preimageBlob_internal E_4(2^32-2)=%x, ++ h[1:28]=%x => %x\n", prefixBytes, blob_hash[1:28], ap_internal_key)
	return ap_internal_key
}

// (h,l) -> E4(l)++h for a_l
func Compute_preimageLookup_internal(blob_hash Hash, blob_len uint32) []byte {
	lBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lBytes, blob_len)         // E4(l)
	al_internal_key := append(lBytes, blob_hash.Bytes()...) // (E4(l) ⌢ H(h) -- this is 31 bytes. but only the first 32 bytes matters
	// fmt.Printf(" E_4(l=%d)=%x ++ h=%s => %x\n", blob_len, lBytes, blob_hash, al_internal_key)
	return al_internal_key
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
	sBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sBytes, s)
	stateKey := make([]byte, 32)
	stateKey[0] = 255
	stateKey[1] = sBytes[0]
	stateKey[3] = sBytes[1]
	stateKey[5] = sBytes[2]
	stateKey[7] = sBytes[3]
	//fmt.Printf("C(s=%d [LE=%x])=%x\n", s, sBytes, stateKey)
	return BytesToHash(stateKey)
}

func ComputeC_sh(s uint32, h []byte) Hash {
	// (s,h) ↦ [n0, a0, n1, a1, n2, a2, n3, a3, a4, a5, ... , a26,  0 ] -- 0.6.7, last byte left as zero
	var stateKey [32]byte

	// n: E4(s)
	n := []byte{
		byte(s >> 0),
		byte(s >> 8),
		byte(s >> 16),
		byte(s >> 24),
	}
	a := Blake2Hash(h)

	// Interleave stateKey[0:8] as [n0, a0, n1, a1, n2, a2, n3, a3]
	for i := 0; i < 4; i++ {
		stateKey[2*i] = n[i]
		stateKey[2*i+1] = a[i]
	}
	// stateKey[8;31] as [a4..a26]
	copy(stateKey[8:31], a[4:])
	// fmt.Printf("C(s=%d [LE=%x], h=%x)=%x\n", s, n, h, stateKey)
	return BytesToHash(stateKey[:])
}

func ServiceStorageKeyOLD(s uint32, k []byte) []byte {
	// sb := make([]byte, 4)
	// binary.LittleEndian.PutUint32(sb, uint32(s))
	// return Blake2Hash(append(sb, k...))
	return k
}

func ServiceStorageKey(s uint32, k []byte) []byte {
	// TODO: delete all references to this function
	return k
}
