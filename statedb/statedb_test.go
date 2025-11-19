package statedb

import (
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

const (
	numBlocksPartialState = 100 // Number of blocks for TestStateDBPartialState
	changePercent         = 20  // 20% of objects changed per block
	newObjectsPerBlock    = 50  // New objects added per block
	serviceID             = uint32(35)
)

// GenerateObjectID creates a hash-based object ID from pair index and payload
func GenerateObjectID(pairIndex int, payload []byte) common.Hash {
	data := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(data[0:4], uint32(pairIndex))
	copy(data[4:], payload)
	return common.Blake2Hash(data)
}

func genesisBlock(t *testing.T) (storage *storage.StateDBStorage, stateRootsAtBlock []common.Hash, ObjectRefsAtBlock []map[common.Hash]types.ObjectRef) {
	t.Helper()

	testDir := t.TempDir()
	storage, err := initStorage(testDir)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	statedb0, err := NewStateDB(storage, common.Hash{})
	if err != nil {
		t.Fatalf("Failed to create initial StateDB: %v", err)
	}

	stateRootsAtBlock = make([]common.Hash, numBlocksPartialState+1)
	ObjectRefsAtBlock = make([]map[common.Hash]types.ObjectRef, numBlocksPartialState+1)
	stateRootsAtBlock[0] = statedb0.sdb.GetRoot()
	ObjectRefsAtBlock[0] = make(map[common.Hash]types.ObjectRef)
	t.Logf("Genesis state root: %s, total objects: %d", stateRootsAtBlock[0], len(ObjectRefsAtBlock[0]))
	return storage, stateRootsAtBlock, ObjectRefsAtBlock
}

func advanceBlockPartialState(t *testing.T, block int, prevStateRoot common.Hash, prevObjectRefs map[common.Hash]types.ObjectRef,
	storage *storage.StateDBStorage, nextObjectIdx *int) (common.Hash, map[common.Hash]types.ObjectRef) {
	t.Helper()

	statedb, err := NewStateDBFromStateRoot(prevStateRoot, storage)
	if err != nil {
		panic(err)
	}

	objectRefs := make(map[common.Hash]types.ObjectRef)
	for k, v := range prevObjectRefs {
		objectRefs[k] = v
	}

	partialState := statedb.JamState.newPartialState()
	sa, ok, err := statedb.GetService(serviceID)
	if err != nil {
		panic(err)
	}
	if !ok {
		codeHash := common.Blake2Hash([]byte("test_service"))
		sa = types.NewEmptyServiceAccount(serviceID, codeHash.Bytes(), 1000, 1000, 0, 0, 0, 0)
		sa.Balance = 1000000
	}
	sa.ALLOW_MUTABLE()
	sa.Dirty = true

	writeObjectRef := func(objectID common.Hash, ref types.ObjectRef) {
		refPayload := ref.Serialize()
		sa.WriteStorage(serviceID, objectID[:], refPayload, false, "test")
		objectRefs[objectID] = ref
	}

	// Update some existing objects in objectRefs with WriteStorage
	numObjectsToChange := len(prevObjectRefs) * changePercent / 100
	i := 0
	for objectID, prevRef := range prevObjectRefs {
		if i >= numObjectsToChange {
			break
		}
		updatedRef := prevRef
		updatedRef.Version = uint32(block + 1)
		updatedRef.GasUsed = uint32(100 + block*10 + i)
		updatedRef.Timeslot = uint32(block)
		updatedRef.TxSlot = uint16(i)
		writeObjectRef(objectID, updatedRef)
		i++
	}

	// Add some new objects to objectRefs with WriteStorage
	for i := 0; i < newObjectsPerBlock; i++ {
		payloadSize := 8 + ((block*newObjectsPerBlock + i) % 32)
		payload := make([]byte, payloadSize)
		if _, err := rand.Read(payload); err != nil {
			panic(err)
		}
		objectID := GenerateObjectID(*nextObjectIdx, payload)
		*nextObjectIdx++

		newRef := types.ObjectRef{
			ServiceID:       serviceID,
			WorkPackageHash: common.Hash{},
			IndexStart:      uint16(*nextObjectIdx),
			IndexEnd:        uint16(*nextObjectIdx + 1),
			Version:         1,
			PayloadLength:   uint32(payloadSize),
			Timeslot:        uint32(block),
			GasUsed:         uint32(100 + i),
			EvmBlock:        uint32(block),
			ObjKind:         0,
			LogIndex:        0,
			TxSlot:          uint16(i),
		}
		writeObjectRef(objectID, newRef)
	}

	sa.NumStorageItems = uint32(len(sa.Storage))
	sa.StorageSize = uint64(len(sa.Storage) * (32 + 20))
	partialState.ServiceAccounts[serviceID] = sa
	statedb.ApplyXContext(partialState)
	stateRoot := common.Hash{}
	numWrites := numObjectsToChange + newObjectsPerBlock
	t.Logf("Block %d state root: %s, total objects: %d, batch writes: %d", block, stateRoot.Hex(), len(objectRefs), numWrites)
	return stateRoot, objectRefs
}

// verifyBlock verifies all objects for a given stateroot using both ReadServiceStorage and GetServiceStorage
func verifyBlock(t *testing.T, block int, stateRoot common.Hash, storage *storage.StateDBStorage,
	expectedRefs map[common.Hash]types.ObjectRef) uint64 {
	t.Helper()

	// Create StateDB from this block's state root using NewStateDBFromStateRoot
	verifyDB, err := NewStateDBFromStateRoot(stateRoot, storage)
	if err != nil {
		t.Fatalf("Verification block %d: Failed to create StateDB: %v", block, err)
	}
	verifications := uint64(0)
	mismatchCount := 0
	for objectID, expectedRef := range expectedRefs {
		expectedValue := expectedRef.Serialize()

		// Verify using ReadServiceStorage (high-level API)
		actualValue, ok, err := verifyDB.ReadServiceStorage(serviceID, objectID[:])
		if err != nil {
			mismatchCount++
			panic("mismatch")
		} else if !ok {
			mismatchCount++
			panic("mismatch")
		} else if string(actualValue) != string(expectedValue) {
			mismatchCount++
			panic("mismatch")
		}

		// Verify using GetServiceStorage (trie-level API)
		actualValue, ok, err = verifyDB.sdb.GetServiceStorage(serviceID, objectID[:])
		if err != nil {
			mismatchCount++
			panic("mismatch")
		} else if !ok {
			mismatchCount++
			panic("mismatch")
		} else if string(actualValue) != string(expectedValue) {
			mismatchCount++
			panic("mismatch")
		}

		// Deserialize and verify ObjectRef metadata
		offset := 0
		actualRef, err := types.DeserializeObjectRef(actualValue, &offset)
		if err != nil {
			t.Fatalf("Block %d: Failed to deserialize ObjectRef: %v", block, err)
		}
		if actualRef.ServiceID != expectedRef.ServiceID ||
			actualRef.Version != expectedRef.Version ||
			actualRef.GasUsed != expectedRef.GasUsed ||
			actualRef.Timeslot != expectedRef.Timeslot {
			t.Fatalf("Block %d: ObjectRef metadata mismatch for object %x", block, objectID)
		}

		verifications++
	}
	return verifications
}

// TestStateDBPartialState checks advanceBlock with PartialState + WriteStorage abstractions
func TestStateDBPartialState(t *testing.T) {
	storage, stateRootsAtBlock, ObjectRefsAtBlock := genesisBlock(t)
	defer storage.Close()

	verifications := uint64(0)
	nextObjectIdx := 0
	for block := 1; block <= numBlocksPartialState; block++ {
		stateRoot, objectRefs := advanceBlockPartialState(t, block, stateRootsAtBlock[block-1], ObjectRefsAtBlock[block-1], storage, &nextObjectIdx)
		stateRootsAtBlock[block] = stateRoot
		ObjectRefsAtBlock[block] = objectRefs
		for bn := 0; bn <= block; bn++ {
			verifications += verifyBlock(t, bn, stateRootsAtBlock[bn], storage, ObjectRefsAtBlock[bn])
		}
		t.Logf("Block %d verified", block)
	}

	t.Logf("Total blocks: %d, Verifications %d", numBlocksPartialState, verifications)
}
