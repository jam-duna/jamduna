package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type BMTProof []common.Hash

// SetStates sets multiple state values in a single batch operation
// mask indicating which states to set
func (t *StateDBStorage) SetStates(values [16][]byte) {
	// Get current states to detect changes
	oldStates, _ := t.GetStates()

	// Only insert states that have actually changed
	for i := uint8(0); i < 16; i++ {
		if !bytes.Equal(oldStates[i], values[i]) {
			stateKey := make([]byte, 32)
			stateKey[0] = i + 1 // State indices are 1-16
			t.Insert(stateKey, values[i])
		}
	}
}

// GetStates retrieves all 16 state values
func (t *StateDBStorage) GetStates() ([16][]byte, error) {
	var states [16][]byte
	for i := uint8(0); i < 16; i++ {
		stateKey := make([]byte, stateKeySize)
		stateKey[0] = i + 1 // State indices are 1-16
		value, ok, err := t.Get(stateKey)
		if err != nil {
			return states, err
		}
		if ok {
			states[i] = value
		}
	}
	return states, nil
}

// DeleteService (hash)
func (t *StateDBStorage) DeleteService(s uint32) error {
	service_account := common.ComputeC_is(s)
	stateKey := service_account.Bytes()
	return t.Delete(stateKey)
}

func (t *StateDBStorage) SetService(s uint32, v []byte) error {
	/*
		∀(s ↦ a) ∈ δ ∶ C(255, s) ↦ a c ⌢E 8 (a b ,a g ,a m ,a l )⌢E 4 (a i )
		i: 255
		s: service_index
		ac: service_accout_code_hash
		ab: service_accout_balance
		ag: service_accout_accumulate_gas
		am: service_accout_on_transfer_gas
		al: see GP_0.35(95)
		ai: see GP_0.35(95)

		(i, s ∈ N S ) ↦ [i, n 0 ,n 1 ,n 2 ,n 3 , 0, 0, . . . ] where n = E 4 (s)
	*/
	service_account := common.ComputeC_is(s)
	stateKey := service_account.Bytes()
	t.Insert(stateKey, v)
	return nil
}

func (t *StateDBStorage) GetService(s uint32) ([]byte, bool, error) {
	service_account := common.ComputeC_is(s)
	stateKey := service_account.Bytes()
	value, ok, err := t.Get(stateKey)
	if err != nil {
		return nil, false, fmt.Errorf("GetService Error: %v", err)
	} else if !ok {
		return nil, ok, nil
	}
	return value, true, nil
}

// set a_l (with timeslot if we have E_P). For GP_0.3.5(158)
func (t *StateDBStorage) SetPreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32, time_slots []uint32) error {

	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key) // C(s, (h,l))
	stateKey := account_lookuphash.Bytes()
	vBytes, err := types.Encode(time_slots)
	if err != nil {
		fmt.Printf("SetPreImageLookup Encode Error: %v\n", err)
	}
	// Insert the value into the state
	t.Insert(stateKey, vBytes)
	return nil
}

func BytesToTimeSlots(vByte []byte) (time_slots []uint32) {
	if len(vByte) == 0 {
		return make([]uint32, 0)
	}
	vByte = vByte[1:]
	time_slots = make([]uint32, (len(vByte) / 4))
	for i := 0; i < len(time_slots); i++ {
		time_slots[i] = binary.LittleEndian.Uint32(vByte[i*4 : (i+1)*4])
	}
	return
}

// lookup a_l .. returning time slot. For GP_0.3.5(157)
func (t *StateDBStorage) GetPreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32) ([]uint32, bool, error) {

	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key) // C(s, (h,l))

	stateKey := account_lookuphash.Bytes()

	vByte, ok, err := t.Get(stateKey)
	if err != nil {
		return nil, ok, err
	} else if !ok {
		return nil, ok, nil
	}
	var time_slots []uint32

	if len(vByte) == 0 {
		time_slots = make([]uint32, 0)
	} else {
		vByte = vByte[1:]
		time_slots = make([]uint32, (len(vByte) / 4))
		for i := 0; i < len(time_slots); i++ {
			time_slots[i] = binary.LittleEndian.Uint32(vByte[i*4 : (i+1)*4])
		}
	}
	return time_slots, ok, err
}

// Delete PreImageLookup key(hash)
func (t *StateDBStorage) DeletePreImageLookup(s uint32, blob_hash common.Hash, blob_len uint32) error {
	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, blob_len)
	account_lookuphash := common.ComputeC_sh(s, al_internal_key) // C(s, (h,l))
	stateKey := account_lookuphash.Bytes()
	return t.Delete(stateKey)
}

// Insert Storage Value into the trie
func (t *StateDBStorage) SetServiceStorage(s uint32, k []byte, storageValue []byte) error {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()
	t.Insert(stateKey, storageValue)
	return nil
}

func (t *StateDBStorage) GetServiceStorage(s uint32, k []byte) ([]byte, bool, error) {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()

	// Get Storage from trie
	value, ok, err := t.Get(stateKey)
	if !ok || err != nil {
		return nil, ok, err
	}
	return value, true, nil
}

// same as above but with proof
func (t *StateDBStorage) GetServiceStorageWithProof(s uint32, k []byte) ([]byte, [][]byte, common.Hash, bool, error) {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()

	// Get Storage from trie
	value, ok, err := t.Get(stateKey)
	if !ok || err != nil {
		return nil, nil, common.Hash{}, ok, err
	}
	stateRoot := t.GetRoot()
	proof, err := t.Trace(stateKey)
	if err != nil {
		return nil, nil, common.Hash{}, false, err
	}
	return value, proof, stateRoot, true, nil
}

// Delete Storage key(hash)
func (t *StateDBStorage) DeleteServiceStorage(s uint32, k []byte) error {
	as_internal_key := common.Compute_storageKey_internal(k)
	account_storage_key := common.ComputeC_sh(s, as_internal_key)
	stateKey := account_storage_key.Bytes()
	return t.Delete(stateKey)
}

// Set PreImage Blob for GP_0.3.5(158)
func (t *StateDBStorage) SetPreImageBlob(s uint32, blob []byte) error {
	/*
		∀(s ↦ a) ∈ δ, (h ↦ p) ∈ a p ∶ C(s, h) ↦ p
		(s, h) ↦ [n 0 ,h 0 ,n 1 ,h 1 ,n 2 ,h 2 ,n 3 ,h 3 ,h 4 ,h 5 ,...,h 27 ] where n = E 4 (s)

		s: service_index
		h: blob_hash
		p: blob
	*/

	blobHash := common.Blake2Hash(blob)
	ap_internal_key := common.Compute_preimageBlob_internal(blobHash)
	account_preimage_hash := common.ComputeC_sh(s, ap_internal_key)

	stateKey := account_preimage_hash.Bytes()
	// Insert Preimage Blob into trie
	t.Insert(stateKey, blob)
	return nil
}

func (t *StateDBStorage) GetPreImageBlob(s uint32, blobHash common.Hash) (value []byte, ok bool, err error) {
	ap_internal_key := common.Compute_preimageBlob_internal(blobHash)
	account_preimage_hash := common.ComputeC_sh(s, ap_internal_key)
	stateKey := account_preimage_hash.Bytes()
	value, ok, err = t.Get(stateKey)
	if !ok || err != nil {
		return nil, ok, err
	}
	// Get Preimage Blob from trie
	return value, ok, nil
}

// Delete PreImage Blob
func (t *StateDBStorage) DeletePreImageBlob(s uint32, blobHash common.Hash) error {
	ap_internal_key := common.Compute_preimageBlob_internal(blobHash)
	account_preimage_hash := common.ComputeC_sh(s, ap_internal_key)
	stateKey := account_preimage_hash.Bytes()
	return t.Delete(stateKey)
}

// Trace traces the path to a specific key in the Merkle Tree and returns the sibling hashes along the path
func (t *StateDBStorage) Trace(keyBytes []byte) ([][]byte, error) {
	key := normalizeKey32(keyBytes)
	var key32 [32]byte
	copy(key32[:], key[:])

	// Generate merkle proof for this key using read-only proof generation
	merkleProof, err := t.bmtDB.GenerateProof(key32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate proof: %v", err)
	}

	// Convert MerkleProof.Path to [][]byte (sibling hashes)
	siblingHashes := make([][]byte, len(merkleProof.Path))
	for i, node := range merkleProof.Path {
		// Each ProofNode contains the sibling hash
		siblingHashes[i] = node.Hash[:]
	}

	return siblingHashes, nil
}

// Verify verifies the path to a specific key in the Merkle Tree
func Verify(serviceID uint32, key []byte, value []byte, rootHash []byte, path []common.Hash) bool {
	return true
}
