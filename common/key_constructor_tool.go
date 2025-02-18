package common

import (
	"encoding/binary"
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

	as_internal_key := append(prefixBytes, key_hash[0:28]...)
	return BytesToHash(as_internal_key)
}

func Compute_storageKey_internal_byte(s uint32, k []byte) []byte {
	sBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sBytes, s)     // E4(s)
	raw_key := ComputeHash(append(sBytes, k...)) // H(E4(s) ⌢ vk ⋅⋅⋅+k )
	return raw_key
}

// EQ 290 - state-key constructor functions C
func ComputeC_i(i uint8) Hash {
	//i ∈ N_8 ↦ [i,0,0,...]
	stateKey := make([]byte, 32)
	stateKey[0] = byte(i)
	return BytesToHash(stateKey)
}

func ComputeC_is(i uint8, s uint32) Hash {
	//(i,s ∈ N_S) ↦ [i,n0,n1,n2,n3,0,0,...] where n = E4(s)
	stateKey := make([]byte, 32)
	stateKey[0] = i
	byteSlice := make([]byte, 4)
	binary.LittleEndian.PutUint32(byteSlice, s)
	stateKey[1] = byteSlice[0]
	stateKey[3] = byteSlice[1]
	stateKey[5] = byteSlice[2]
	stateKey[7] = byteSlice[3]
	//fmt.Printf("C(255, s=%d (hex=%x))=%s\n", s, s, BytesToHash(stateKey))

	return BytesToHash(stateKey)
}

func ComputeC_sh(s uint32, h0 Hash) Hash {
	//s: service_index
	//h: hash_component (assumed to be exact 32bytes)
	//(s,h) ↦ [n0,h0,n1,h1,n2,h2,n3,h3,h4,h5,...,h27] where n = E4(s)
	h := h0.Bytes()
	stateKey := make([]byte, 32)
	nBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(nBytes, s) // n = E4(s)

	for i := 0; i < 4; i++ {
		stateKey[2*i] = nBytes[i]
		if i < 32 {
			stateKey[2*i+1] = h[i]
		}
	}
	for i := 4; i < 28; i++ {
		if i < len(h) {
			stateKey[i+4] = h[i]
		}
	}
	return BytesToHash(stateKey)
}

func ComputeC_sh_Byte(s uint32, h0 []byte) Hash {
	//s: service_index
	//h: hash_component (assumed to be exact 32bytes)
	//(s,h) ↦ [n0,h0,n1,h1,n2,h2,n3,h3,h4,h5,...,h27] where n = E4(s)
	h := make([]byte, 32)
	copy(h, h0)
	stateKey := make([]byte, 32)
	nBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(nBytes, s) // n = E4(s)

	for i := 0; i < 4; i++ {
		stateKey[2*i] = nBytes[i]
		if i < 32 {
			stateKey[2*i+1] = h[i]
		}
	}
	for i := 4; i < 28; i++ {
		if i < len(h) {
			stateKey[i+4] = h[i]
		}
	}
	return BytesToHash(stateKey)
}
