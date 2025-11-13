package statedb

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	trie "github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

const OK uint64 = 0

func (s *StateDB) writeAccount(sa *types.ServiceAccount) (serviceUpdate *types.ServiceUpdate, err error) {
	if !sa.Mutable {
		return nil, fmt.Errorf("WriteAccount")
	}
	if !sa.Dirty {
		return nil, nil
	}

	service_idx := sa.GetServiceIndex()
	tree := s.GetTrie()

	serviceUpdate = types.NewServiceUpdate(service_idx)
	for _, storage := range sa.Storage {
		log.Trace(log.SDB, "writeAccount Storage", "service_idx", service_idx, "key", fmt.Sprintf("%x", storage.Key), "rawkey", storage.InternalKey, "value", fmt.Sprintf("%x", storage.Value), "storage.Accessed", storage.Accessed, "storage.Deleted", storage.Deleted, "storage.Dirty", storage.Dirty, "storage.source", storage.Source)
		as_internal_key := storage.InternalKey
		if storage.Dirty {
			if storage.Deleted {
				log.Debug(s.Authoring, "writeAccount DELETE", "service_idx", service_idx, "key", fmt.Sprintf("%x", storage.Key), "rawkey", as_internal_key, "storage.Accessed", storage.Accessed, "storage.Deleted", storage.Deleted, "storage.source", storage.Source)
				err = tree.DeleteServiceStorage(service_idx, storage.Key)
				if err != nil {
					continue
				}
			} else {
				// Here we are returning ALL storage values written
				serviceUpdate.ServiceValue[as_internal_key] = &types.SubServiceValueResult{
					ServiceID:  service_idx,
					HeaderHash: s.HeaderHash,
					Slot:       s.GetTimeslot(),
					//Hash:       storage.InternalKey, //TODO: SOURABH CHECK
					Key:   common.Bytes2Hex(storage.Key),
					Value: common.Bytes2Hex(storage.Value),
				}
				err = tree.SetServiceStorage(service_idx, storage.Key, storage.Value)
				if err != nil {
					log.Warn(log.SDB, "SetServiceStorage Failure", "n", s.Id, "service_idx", service_idx, "k", fmt.Sprintf("%x", storage.Key), "rawkey", as_internal_key, "err", err)
					return
				}
			}
		}
	}

	for blob_hash_str, v := range sa.Lookup {
		log.Trace(log.SDB, "writeAccount Lookup", "service_idx", service_idx, "blob_hash_str", blob_hash_str, "v.Dirty", v.Dirty, "v.Deleted", v.Deleted, "v.Z", v.Z, "v.Timeslots", v.Timeslots, "source", v.Source)
		blob_hash := common.HexToHash(blob_hash_str)
		if v.Dirty {
			if v.Deleted {
				err = tree.DeletePreImageLookup(service_idx, blob_hash, v.Z)
				if err != nil {
					continue
				}
				//log.Debug("authoring", "tree.DeletePreImageLookup [FORGET OK]", "blob_hash", blob_hash, "v.Z", v.Z)
				serviceUpdate.ServiceRequest[blob_hash_str] = &types.SubServiceRequestResult{
					HeaderHash: s.HeaderHash,
					Slot:       s.GetTimeslot(),
					Hash:       blob_hash, //TODO: SOURABH CHECK
					ServiceID:  service_idx,
					Timeslots:  nil,
				}
			} else {
				//log.Debug(log.SDB, "writeAccount SET Lookup", "service_idx", service_idx, "blob_hash", blob_hash_str, "v.Z", v.Z, "v.Timeslots", v.Timeslots)
				err = tree.SetPreImageLookup(service_idx, blob_hash, v.Z, v.Timeslots)
				if err != nil {
					log.Warn(log.SDB, "tree.SetPreimageLookup", "blob_hash", blob_hash, "v.Z", v.Z, "v.Timeslots", v.Timeslots, "err", err)
					return
				}
				serviceUpdate.ServiceRequest[blob_hash_str] = &types.SubServiceRequestResult{
					HeaderHash: s.HeaderHash,
					Slot:       s.GetTimeslot(),
					Hash:       blob_hash, //TODO: SOURABH CHECK
					ServiceID:  service_idx,
					Timeslots:  v.Timeslots,
				}
			}
		}
	}

	for blobHash_str, v := range sa.Preimage {
		//log.Debug(log.SDB, "writeAccount Preimage", "service_idx", service_idx, "blobHash_str", blobHash_str, "v.Dirty", v.Dirty, "v.Deleted", v.Deleted, "len(v.Preimage)", len(v.Preimage), "source", v.Source)
		blobHash := common.HexToHash(blobHash_str)
		if v.Dirty {
			if v.Deleted {
				if v.Source == "trie" {
					err = tree.DeletePreImageBlob(service_idx, blobHash)
					if err != nil {
						continue
					}
					log.Trace("authoring", "DeletePreImageBlob [FORGET OK] A", "blobHash", blobHash, "v.Deleted", v.Deleted, "len(v.Preimage)", len(v.Preimage), "source", v.Source)
				}
			} else {
				err = tree.SetPreImageBlob(service_idx, v.Preimage)
				if err != nil {
					log.Warn(log.SDB, "SetPreImageBlob", "err", err)
					return
				}
				serviceUpdate.ServicePreimage[blobHash_str] = &types.SubServicePreimageResult{
					HeaderHash: s.HeaderHash,
					Slot:       s.GetTimeslot(),
					Hash:       blobHash, // TODO: SOURABH CHECK
					ServiceID:  service_idx,
					Preimage:   common.Bytes2Hex(v.Preimage),
				}
			}
		}
	}
	if sa.DeletedAccount {
		tree.DeleteService(service_idx)
		//log.Info(log.SDB, "tree.DeleteService SUCCESS", "service_idx", service_idx)
	} else {
		err = s.writeService(service_idx, sa)
	}
	serviceUpdate.ServiceInfo = &types.SubServiceInfoResult{
		HeaderHash: s.HeaderHash,
		Slot:       s.GetTimeslot(),
		ServiceID:  service_idx,
		Info:       *sa,
	}
	return serviceUpdate, err
}

func (s *StateDB) ApplyXContext(U *types.PartialState) (stateUpdate *types.StateUpdate) {
	stateUpdate = types.NewStateUpdate()
	for _, sa := range U.ServiceAccounts {
		// U.D should only have service accounts with Mutable = true
		if !sa.Mutable {
			log.Crit(log.SDB, "Immutable Service account in X.U.D", "s", sa.ServiceIndex)
		} else if sa.Dirty {
			serviceUpdate, err := s.writeAccount(sa)
			if err != nil {
				log.Crit(log.SDB, "ApplyXContext", "err", err)
			}
			if false {
				stateUpdate.AddServiceUpdate(sa.ServiceIndex, serviceUpdate)
			}
		}
	}
	// p - Bless => PrivilegedServiceState 12.4.1 (164)
	// Direct assignment to reduce field access overhead
	privilegedIndices := &s.JamState.PrivilegedServiceIndices
	privilegedState := &U.PrivilegedState

	privilegedIndices.AuthQueueServiceID = privilegedState.AuthQueueServiceID
	privilegedIndices.ManagerServiceID = privilegedState.ManagerServiceID
	privilegedIndices.UpcomingValidatorsServiceID = privilegedState.UpcomingValidatorsServiceID
	// 0.7.1 introduces RegistrarServiceID
	privilegedIndices.RegistrarServiceID = privilegedState.RegistrarServiceID

	// Optimize map copying - check if we need to allocate new map
	if privilegedIndices.AlwaysAccServiceID == nil || len(privilegedState.AlwaysAccServiceID) > len(privilegedIndices.AlwaysAccServiceID) {
		privilegedIndices.AlwaysAccServiceID = make(map[uint32]uint64, len(privilegedState.AlwaysAccServiceID))
	} else {
		// Clear existing map
		for k := range privilegedIndices.AlwaysAccServiceID {
			delete(privilegedIndices.AlwaysAccServiceID, k)
		}
	}

	// Copy map entries
	for k, v := range privilegedState.AlwaysAccServiceID {
		privilegedIndices.AlwaysAccServiceID[k] = v
	}

	// c - Designate => AuthorizationQueue
	// Use range loop to avoid bounds checking overhead
	authQueue := s.JamState.AuthorizationQueue
	queueWorkReport := U.QueueWorkReport
	for i := range authQueue {
		copy(authQueue[i][:], queueWorkReport[i][:])
	}

	// v - Assign => DesignatedValidators
	// Direct assignment - no optimization needed here
	s.JamState.SafroleState.DesignatedValidators = U.UpcomingValidators

	return
}

func (s *StateDB) GetTimeslot() uint32 {
	// Successfully casted to *statedb.StateDB
	sf := s.GetSafrole()
	return sf.GetTimeSlot()
}

// GetService returns a **Immutable** service object.
func (s *StateDB) GetService(service uint32) (sa *types.ServiceAccount, ok bool, err error) {
	tree := s.GetTrie()
	var serviceBytes []byte
	serviceBytes, ok, err = tree.GetService(service)
	if err != nil {
		return
	}
	if !ok {
		return
	}
	sa, err = types.ServiceAccountFromBytes(service, serviceBytes)
	if err != nil {
		return
	}
	return
}

func (s *StateDB) writeService(service uint32, sa *types.ServiceAccount) (err error) {
	tree := s.GetTrie()
	v, _ := sa.Bytes()
	//log.Trace(log.SDB, "writeService", "service_idx", service, "sa.NumStorageItems", sa.NumStorageItems, "sa.StorageSize", sa.StorageSize, "sa.Balance", sa.Balance, "sa.GasLimitG", sa.GasLimitG, "sa.GasLimitM", sa.GasLimitM, "sa.CreateTime", sa.CreateTime, "sa.RecentAccumulation", sa.RecentAccumulation, "sa.ParentService", sa.ParentService, "bytes", fmt.Sprintf("%x", (v)))
	return tree.SetService(service, v)
}

func (s *StateDB) ReadServiceStorage(service uint32, k []byte) (storage []byte, ok bool, err error) {
	// not init case
	tree := s.GetTrie()
	storage, ok, err = tree.GetServiceStorage(service, k)
	if err != nil || !ok {
		log.Trace(log.SDB, "ReadServiceStorage: Not found", "service", service, "key", fmt.Sprintf("%x", k), "err", err)
		return storage, ok, err
	}
	log.Trace(log.SDB, "ReadServiceStorage", "service", service, "key", fmt.Sprintf("%x", k), "value", fmt.Sprintf("%x", storage))
	return storage, ok, err
}

func (s *StateDB) ReadServicePreimageBlob(service uint32, blob_hash common.Hash) (blob []byte, ok bool, err error) {
	tree := s.GetTrie()
	blob, ok, err = tree.GetPreImageBlob(service, blob_hash)
	if err != nil || !ok {
		return
	} else {
		log.Trace(log.P, "ReadServicePreimageBlob", "service", service, "blob_hash", blob_hash, "len(blob)", len(blob))
		return
	}
}

func (s *StateDB) ReadServicePreimageLookup(service uint32, blob_hash common.Hash, blob_length uint32) (time_slots []uint32, ok bool, err error) {
	tree := s.GetTrie()
	time_slots, ok, err = tree.GetPreImageLookup(service, blob_hash, blob_length)
	if err != nil || !ok {
		return
	}
	log.Debug(log.P, "ReadServicePreimageLookup", "service", service, "blob_hash", blob_hash, "blob_length", blob_length, time_slots)
	return

}

// HistoricalLookup, GetImportItem, ExportSegment
func (s *StateDB) HistoricalLookup(a *types.ServiceAccount, t uint32, blob_hash common.Hash) []byte {
	ok, blob, preimage_source := a.ReadPreimage(blob_hash, s)
	if !ok {
		log.Error(log.SDB, "HistoricalLookup ERR", "ok", ok, "s", a.ServiceIndex, "blob_hash", blob_hash, "preimage_source", preimage_source)
		return nil
	}

	z := uint32(len(blob))

	ok, timeslots, lookup_source := a.ReadLookup(blob_hash, z, s)
	if !ok {
		log.Error(log.SDB, "HistoricalLookup ReadLookup ERR", "ok", ok, "s", a.ServiceIndex, "blob_hash", blob_hash, "z", z, "lookup_source", lookup_source)
		return nil
	}

	if len(timeslots) == 0 {
		return nil
	} else if len(timeslots) == 1 {
		if timeslots[0] <= t {
			return blob
		} else {
			return nil
		}
	} else if len(timeslots) == 2 {
		if timeslots[0] <= t && t < timeslots[1] {
			return blob
		} else {
			return nil
		}
	} else {
		if timeslots[0] <= t && t < timeslots[1] || timeslots[2] <= t {
			return blob
		} else {
			return nil
		}
	}
}

// convertProofToHashes converts [][]byte proof to []common.Hash
// Panics if proof hash length is not 32 bytes (indicates trie corruption)
func convertProofToHashes(proofBytes [][]byte) []common.Hash {
	path := make([]common.Hash, len(proofBytes))
	for i, p := range proofBytes {
		if len(p) != 32 {
			panic(fmt.Sprintf("invalid proof hash length: expected 32, got %d at index %d", len(p), i))
		}
		copy(path[i][:], p)
	}
	return path
}

func (s *StateDB) ReadStateWitnessRaw(serviceID uint32, objectID common.Hash) (types.StateWitnessRaw, bool, common.Hash, error) {
	tree := s.GetTrie()
	// Use the low-level trie method to get value, proof, and state root
	valueBytes, proofBytes, stateRoot, ok, err := tree.GetServiceStorageWithProof(serviceID, objectID[:])
	if err != nil {
		return types.StateWitnessRaw{}, false, common.Hash{}, err
	}
	if !ok {
		return types.StateWitnessRaw{}, false, common.Hash{}, nil
	}
	path := convertProofToHashes(proofBytes)
	// ObjectKindRaw cases
	if objectID == evmtypes.GetBlockNumberKey() {
		// BLOCK_NUMBER_KEY: RAW storage (no ObjectRef, just raw value)
		// Value format: block_number (4 bytes LE) + parent_hash (32 bytes) = 36 bytes
		witness := types.StateWitnessRaw{
			ObjectID:      objectID,
			ServiceID:     serviceID,
			Path:          path,
			PayloadLength: uint32(len(valueBytes)),
			Payload:       valueBytes, // RAW value from JAM State
		}
		return witness, true, stateRoot, nil
	}

	isBlockObject, blockNumber := evmtypes.IsBlockObjectID(objectID)
	if isBlockObject {
		// Block objects: JAM State contains only EvmBlockPayload (no ObjectRef)
		// We need to reconstruct ObjectRef from the block data and objectID
		if len(valueBytes) < 4 {
			return types.StateWitnessRaw{}, true, common.Hash{}, fmt.Errorf("invalid Block object value length: expected at least 4, got %d", len(valueBytes))
		}

		// Deserialize EvmBlockPayload to extract block details
		evmBlock, err := evmtypes.DeserializeEvmBlockPayload(valueBytes)
		if err != nil {
			log.Error(log.SDB, "ReadStateWitness:DeserializeEvmBlockPayload", "objectID", objectID, "expected", blockNumber)
			return types.StateWitnessRaw{}, true, common.Hash{}, fmt.Errorf("failed to deserialize EvmBlockPayload: %w", err)
		}

		// Verify block number matches
		if evmBlock.Number != uint64(blockNumber) {
			log.Error(log.SDB, "ReadStateWitness:Block number mismatch", "objectID", objectID, "expected", blockNumber, "got", evmBlock.Number)
			return types.StateWitnessRaw{}, true, common.Hash{}, fmt.Errorf("block number mismatch: objectID has %d, payload has %d", blockNumber, evmBlock.Number)
		}

		// For Block objects, JAM State contains the complete EvmBlockPayload
		witness := types.StateWitnessRaw{
			ObjectID:      objectID,
			ServiceID:     serviceID,
			Path:          path,
			PayloadLength: uint32(len(valueBytes)),
			Payload:       valueBytes, // Complete EvmBlockPayload from JAM State
		}

		return witness, true, stateRoot, nil
	}
	return types.StateWitnessRaw{}, false, common.Hash{}, fmt.Errorf("ReadStateWitnessRaw: unsupported ObjectID: %v", objectID)
}

// ReadObject fetches ObjectRef and state proof for given objectID
func (s *StateDB) ReadStateWitnessRef(serviceID uint32, objectID common.Hash, fetchPayloadFromDA bool) (types.StateWitness, bool, error) {
	tree := s.GetTrie()

	// Use the low-level trie method to get value, proof, and state root
	valueBytes, proofBytes, _, ok, err := tree.GetServiceStorageWithProof(serviceID, objectID[:])
	if err != nil {
		return types.StateWitness{}, false, err
	}
	if !ok {
		return types.StateWitness{}, false, nil
	}
	path := convertProofToHashes(proofBytes)

	// Receipt and other objects: JAM State starts with ObjectRef
	if len(valueBytes) < 64 {
		return types.StateWitness{}, true, fmt.Errorf("invalid value length: expected at least 64, got %d", len(valueBytes))
	}
	// Deserialize ObjectRef from storage
	offset := 0
	objRef, deserErr := types.DeserializeObjectRef(valueBytes[0:64], &offset)
	if deserErr != nil {
		return types.StateWitness{}, true, fmt.Errorf("failed to deserialize ObjectRef: %w", deserErr)
	}

	witness := types.StateWitness{
		ObjectID: objectID,
		Ref:      objRef,
		Path:     path,
	}

	// Handle payload based on object type
	if objRef.ObjKind == uint8(common.ObjectKindReceipt) {
		// Receipt objects: JAM State = ObjectRef + receipt_payload
		// Extract the receipt payload from the remaining bytes after ObjectRef
		if len(valueBytes) > 64 {
			witness.Payload = valueBytes[64:] // Receipt payload starts after ObjectRef
		}
	} else if fetchPayloadFromDA {
		// For other objects, optionally fetch payload via FetchJAMDASegments
		payload, err := s.sdb.FetchJAMDASegments(objRef.WorkPackageHash, objRef.IndexStart, objRef.IndexEnd, objRef.PayloadLength)
		if err != nil {
			// Don't fail if CE139 read fails, just leave payload empty
			witness.Payload = []byte{}
		} else {
			witness.Payload = payload
		}
	}

	return witness, true, nil
}

// VerifyStateWitnessRef verifies a StateWitness proof against a state root
//
//	every imported object must undergo same verification against refine context state root
func VerifyStateWitnessRef(witness types.StateWitness, stateRoot common.Hash) bool {
	return trie.Verify(witness.Ref.ServiceID, witness.ObjectID[:], witness.Ref.Serialize(), stateRoot[:], witness.Path)
}

// VerifyStateWitnessRaw verifies a StateWitnessRaw proof against a state root
func VerifyStateWitnessRaw(witness types.StateWitnessRaw, stateRoot common.Hash) bool {
	return trie.Verify(witness.ServiceID, witness.ObjectID[:], witness.Payload, stateRoot[:], witness.Path)
}

// GetStateWitnesses extracts state witnesses for all objects referenced in the work report (for all work items)
func (s *StateDB) GetStateWitnesses(workReports []*types.WorkReport) (witnesses []types.StateWitness, stateRoot common.Hash, err error) {
	stateRoot = s.GetStateRoot()
	witnesses = make([]types.StateWitness, 0)

	// for all work items in the work report, extract witnesses for every objectID in the write intent
	for _, workReport := range workReports {
		for _, r := range workReport.Results {
			if len(r.Result.Ok) > 0 {
				effects, err := types.DeserializeExecutionEffects(r.Result.Ok)
				if err != nil {
					return nil, common.Hash{}, fmt.Errorf("DeserializeExecutionEffects failed: %v", err)
				}
				// Use ReadStateWitnessRef to get proof with built-in verification
				for _, intent := range effects.WriteIntents {
					objectID := intent.Effect.ObjectID

					// Skip Block objects - they don't have ObjectRef entries in JAM state
					if intent.Effect.RefInfo.ObjKind == uint8(common.ObjectKindBlock) || intent.Effect.RefInfo.ObjKind == uint8(common.ObjectKindReceipt) {
						continue
					}

					w, ok, err := s.ReadStateWitnessRef(r.ServiceID, objectID, false)
					if err != nil {
						return nil, common.Hash{}, fmt.Errorf("ReadStateWitnessRef failed: %v", err)
					} else if !ok {
						// It is important to recognize that when conflicts arise, a write intent may
						// reference an object that was not actually written.
						return nil, common.Hash{}, fmt.Errorf("ReadStateWitnessRef NOT FOUND: %v", objectID)
					}
					witnesses = append(witnesses, w)
				}
			}
		}
	}
	// Verify the witness (for paranoia)
	for _, w := range witnesses {
		if !VerifyStateWitnessRef(w, stateRoot) {
			return nil, common.Hash{}, fmt.Errorf("ReadStateWitnessRef NOT VERIFIED: %v", w.ObjectID)
		}
	}
	return witnesses, stateRoot, nil
}

func (s *StateDB) GetForgets() []*types.SubServiceRequestResult {
	return s.stateUpdate.GetForgets()
}
