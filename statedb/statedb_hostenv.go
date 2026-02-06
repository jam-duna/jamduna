package statedb

import (
	"encoding/binary"
	"fmt"

	"github.com/jam-duna/jamduna/common"
	log "github.com/jam-duna/jamduna/log"
	trie "github.com/jam-duna/jamduna/trie"
	"github.com/jam-duna/jamduna/types"
)

func (s *StateDB) writeAccount(sa *types.ServiceAccount) (serviceUpdate *types.ServiceUpdate, err error) {
	if !sa.Mutable {
		return nil, fmt.Errorf("WriteAccount")
	}
	if !sa.Dirty {
		return nil, nil
	}

	service_idx := sa.GetServiceIndex()

	serviceUpdate = types.NewServiceUpdate(service_idx)
	for _, storage := range sa.Storage {
		log.Trace(log.SDB, "writeAccount Storage", "service_idx", service_idx, "key", fmt.Sprintf("%x", storage.Key), "rawkey", storage.InternalKey, "value", fmt.Sprintf("%x", storage.Value), "storage.Accessed", storage.Accessed, "storage.Deleted", storage.Deleted, "storage.Dirty", storage.Dirty, "storage.source", storage.Source)
		as_internal_key := storage.InternalKey
		if storage.Dirty {
			if storage.Deleted {
				log.Debug(s.Authoring, "writeAccount DELETE", "service_idx", service_idx, "key", fmt.Sprintf("%x", storage.Key), "rawkey", as_internal_key, "storage.Accessed", storage.Accessed, "storage.Deleted", storage.Deleted, "storage.source", storage.Source)
				err = s.sdb.DeleteServiceStorage(service_idx, storage.Key)
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
				err = s.sdb.SetServiceStorage(service_idx, storage.Key, storage.Value)
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
				err = s.sdb.DeletePreImageLookup(service_idx, blob_hash, v.Z)
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
				err = s.sdb.SetPreImageLookup(service_idx, blob_hash, v.Z, v.Timeslots)
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
					err = s.sdb.DeletePreImageBlob(service_idx, blobHash)
					if err != nil {
						continue
					}
					log.Trace("authoring", "DeletePreImageBlob [FORGET OK] A", "blobHash", blobHash, "v.Deleted", v.Deleted, "len(v.Preimage)", len(v.Preimage), "source", v.Source)
				}
			} else {
				err = s.sdb.SetPreImageBlob(service_idx, v.Preimage)
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
		s.sdb.DeleteService(service_idx)
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
	var serviceBytes []byte
	serviceBytes, ok, err = s.sdb.GetService(service)
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
	v, _ := sa.Bytes()
	//log.Trace(log.SDB, "writeService", "service_idx", service, "sa.NumStorageItems", sa.NumStorageItems, "sa.StorageSize", sa.StorageSize, "sa.Balance", sa.Balance, "sa.GasLimitG", sa.GasLimitG, "sa.GasLimitM", sa.GasLimitM, "sa.CreateTime", sa.CreateTime, "sa.RecentAccumulation", sa.RecentAccumulation, "sa.ParentService", sa.ParentService, "bytes", fmt.Sprintf("%x", (v)))
	return s.sdb.SetService(service, v)
}

func (s *StateDB) ReadServiceStorage(service uint32, k []byte) (storage []byte, ok bool, err error) {
	// not init case
	storage, ok, err = s.sdb.GetServiceStorage(service, k)
	if err != nil || !ok {
		log.Trace(log.SDB, "ReadServiceStorage: Not found", "service", service, "key", fmt.Sprintf("%x", k), "err", err)
		return storage, ok, err
	}
	log.Trace(log.SDB, "ReadServiceStorage", "service", service, "key", fmt.Sprintf("%x", k), "value", fmt.Sprintf("%x", storage))
	return storage, ok, err
}

func (s *StateDB) ReadServicePreimageBlob(service uint32, blob_hash common.Hash) (blob []byte, ok bool, err error) {
	blob, ok, err = s.sdb.GetPreImageBlob(service, blob_hash)
	if err != nil || !ok {
		return
	} else {
		log.Trace(log.P, "ReadServicePreimageBlob", "service", service, "blob_hash", blob_hash, "len(blob)", len(blob))
		return
	}
}

func (s *StateDB) ReadServicePreimageLookup(service uint32, blob_hash common.Hash, blob_length uint32) (time_slots []uint32, ok bool, err error) {
	time_slots, ok, err = s.sdb.GetPreImageLookup(service, blob_hash, blob_length)
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

// ReadObject fetches ObjectRef and state proof for given objectID using meta-shard routing
func (s *StateDB) ReadObject(serviceID uint32, objectID common.Hash) (*types.StateWitness, bool, error) {
	// Special case: Meta-shards are stored as ObjectRefs in JAM State that point to DA segments
	// We must read the ObjectRef with proof, then fetch the payload from DA segments
	if shardID, ok := ParseMetaShardObjectID(objectID); ok {
		log.Trace(log.EVM, "ReadObject: Detected meta-shard object_id, reading ObjectRef", "objectID", objectID, "serviceID", serviceID, "shardID", shardID)
		objRefBytes, proofBytes, _, found, err := s.sdb.GetServiceStorageWithProof(serviceID, objectID[:])
		if err != nil {
			return nil, false, fmt.Errorf("failed to read meta-shard ObjectRef: %v", err)
		}
		if !found {
			log.Trace(log.EVM, "ReadObject: Meta-shard ObjectRef not found in state", "objectID", objectID, "shardID", shardID)
			return nil, false, nil // Not an error - meta-shard just doesn't exist yet
		}

		// Convert proof bytes to common.Hash slice
		proof := make([]common.Hash, len(proofBytes))
		for i, p := range proofBytes {
			proof[i] = common.BytesToHash(p)
		}

		// Deserialize the ObjectRef
		offset := 0
		objRef, err := types.DeserializeObjectRef(objRefBytes, &offset)
		if err != nil {
			return nil, false, fmt.Errorf("failed to deserialize meta-shard ObjectRef: %v", err)
		}

		// Fetch the payload from DA segments using the ObjectRef
		numSegments, _ := types.CalculateSegmentsAndLastBytes(objRef.PayloadLength)
		payload, err := s.sdb.FetchJAMDASegments(
			objRef.WorkPackageHash,
			objRef.IndexStart,
			objRef.IndexStart+numSegments,
			objRef.PayloadLength,
		)
		if err != nil {
			return nil, false, fmt.Errorf("failed to fetch meta-shard payload from DA: %v", err)
		}

		// Return the witness with ObjectRef and payload
		witness := &types.StateWitness{
			ServiceID: serviceID,
			ObjectID:  objectID,
			Value:     objRefBytes, // The ObjectRef bytes (what's stored in JAM State)
			Ref:       objRef,
			Payload:   payload,
			Path:      proof,
		}

		return witness, true, nil
	}

	// Step 1: Read global_depth hint from special SSR key in JAM State
	ssrKey := buildSSRKey()
	ldBytes, found, err := s.ReadServiceStorage(serviceID, ssrKey[:])
	if err != nil {
		return nil, false, fmt.Errorf("failed to read global_depth hint: %v", err)
	}
	if !found {
		return nil, false, fmt.Errorf("global_depth hint not found for service %d", serviceID)
	}
	if len(ldBytes) < 1 {
		return nil, false, fmt.Errorf("global_depth hint invalid length: %d", len(ldBytes))
	}

	globalDepth := ldBytes[0]
	// DIAGNOSTIC: Log global_depth for receipt lookup debugging
	log.Warn(log.EVM, "🔎 LOOKUP_START",
		"serviceID", serviceID,
		"objectID", objectID.Hex(),
		"globalDepth", globalDepth,
	)
	// Step 2: Probe backwards from global_depth to find the actual meta-shard
	var metaShardObjectID common.Hash
	var shardID ShardId
	found = false

	keyPrefix := TakePrefix56(objectID)

	for ld := globalDepth; ; ld-- {
		// Compute meta-shard key at this depth
		maskedPrefix := MaskPrefix56(keyPrefix, ld)
		candidateShardID := ShardId{
			Ld:       ld,
			Prefix56: maskedPrefix,
		}
		candidateObjectID := ComputeMetaShardObjectID(serviceID, candidateShardID)

		// Check if this meta-shard exists in JAM State
		_, exists, checkErr := s.ReadServiceStorage(serviceID, candidateObjectID[:])
		if checkErr != nil {
			return nil, false, fmt.Errorf("failed to probe meta-shard at ld=%d: %v", ld, checkErr)
		}

		if exists {
			// Found the meta-shard!
			metaShardObjectID = candidateObjectID
			shardID = candidateShardID
			found = true
			// DIAGNOSTIC: Log chosen meta-shard for receipt lookup debugging
			log.Warn(log.EVM, "🔎 LOOKUP_FOUND_SHARD",
				"ld", ld,
				"metaShardObjectID", metaShardObjectID.Hex(),
				"shardPrefix", fmt.Sprintf("%02x%02x%02x", shardID.Prefix56[0], shardID.Prefix56[1], shardID.Prefix56[2]),
			)
			break
		}

		// Stop at ld=0 to avoid underflow
		if ld == 0 {
			log.Trace(log.EVM, "ReadObject: stopped at ld=0")
			break
		}
	}

	if !found {
		return nil, false, fmt.Errorf("meta-shard not found for object_id %s (probed ld 0-%d)", objectID.Hex(), globalDepth)
	}

	// Step 3: Check if we have a cached witness for this meta-shard
	log.Trace(log.EVM, "ReadObject: meta-shard found", "objectID", objectID, "metaShardObjectID", metaShardObjectID, "shardID", shardID)

	// Check cache first
	if witness, cached := s.metashardWitnesses[metaShardObjectID]; cached {
		// Cache hit! Check if we have the ObjectRef for this objectID
		if cachedRef, hasRef := witness.ObjectRefs[objectID]; hasRef {
			// Ensure we have (or can build) the per-object BMT proof
			proof, ok := witness.ObjectProofs[objectID]
			if !ok && len(witness.MetaShardPayload) > 0 {
				entries, err := parseMetaShardEntries(witness.MetaShardPayload)
				if err == nil {
					proof, err = generateMetaShardInclusionProof(witness.MetaShardMerkleRoot, objectID, entries)
					if err == nil {
						witness.ObjectProofs[objectID] = proof
					}
				}
			}

			// Check if we also have the payload cached
			if cachedPayload, hasPayload := witness.Payloads[objectID]; hasPayload {
				// Full cache hit - return complete witness
				resultWitness := &types.StateWitness{
					ServiceID:           serviceID,
					ObjectID:            witness.ObjectID,
					Ref:                 cachedRef,
					Path:                witness.Path,
					Value:               witness.Value,
					Payload:             cachedPayload,
					MetaShardMerkleRoot: witness.MetaShardMerkleRoot,
					MetaShardPayload:    witness.MetaShardPayload,
					BlockNumber:         witness.BlockNumber,
					Timeslot:            witness.Timeslot,
					ObjectRefs:          witness.ObjectRefs,
					Payloads:            witness.Payloads,
					ObjectProofs:        witness.ObjectProofs,
				}
				return resultWitness, true, nil
			}
			// Have ObjectRef but not payload - fetch payload only
			numSegments, _ := types.CalculateSegmentsAndLastBytes(cachedRef.PayloadLength)
			objectPayload, err := s.sdb.FetchJAMDASegments(
				cachedRef.WorkPackageHash,
				cachedRef.IndexStart,
				cachedRef.IndexStart+numSegments,
				cachedRef.PayloadLength,
			)
			if err != nil {
				return nil, false, fmt.Errorf("failed to fetch object payload from DA: %v", err)
			}
			// Cache the payload for next time
			witness.Payloads[objectID] = objectPayload

			// Return complete witness with newly fetched payload
			resultWitness := &types.StateWitness{
				ServiceID:           serviceID,
				ObjectID:            witness.ObjectID,
				Ref:                 cachedRef,
				Path:                witness.Path,
				Value:               witness.Value,
				Payload:             objectPayload,
				MetaShardMerkleRoot: witness.MetaShardMerkleRoot,
				MetaShardPayload:    witness.MetaShardPayload,
				BlockNumber:         witness.BlockNumber,
				Timeslot:            witness.Timeslot,
				ObjectRefs:          witness.ObjectRefs,
				Payloads:            witness.Payloads,
				ObjectProofs:        witness.ObjectProofs,
			}
			return resultWitness, true, nil
		}
		// Have meta-shard cached but not this specific objectID - this shouldn't happen
		// (ObjectRefs map should contain all entries from meta-shard)
		// Fall through to normal lookup
	}

	// Cache miss - perform full lookup with state proof
	metaShardRefBytes, proofBytes, _, found, err := s.sdb.GetServiceStorageWithProof(serviceID, metaShardObjectID[:])
	if err != nil {
		return nil, false, fmt.Errorf("failed to read meta-shard ObjectRef: %v", err)
	}
	if !found {
		return nil, false, fmt.Errorf("meta-shard ObjectRef not found for shard %+v", shardID)
	}

	// Deserialize meta-shard ObjectRef (37 bytes) + extract block metadata (8 bytes)
	offset := 0
	metaShardRef, err := types.DeserializeObjectRef(metaShardRefBytes, &offset)
	if err != nil {
		return nil, false, fmt.Errorf("failed to deserialize meta-shard ObjectRef: %v", err)
	}

	// Extract block metadata from the additional 8 bytes (timeslot + blocknumber)
	var timeslot, blockNumber uint32
	if len(metaShardRefBytes) >= 45 { // 37 + 8 = 45 bytes total
		timeslot = binary.LittleEndian.Uint32(metaShardRefBytes[37:41])
		blockNumber = binary.LittleEndian.Uint32(metaShardRefBytes[41:45])
	}

	// Step 4: Fetch meta-shard payload from JAM DA
	numSegments, _ := types.CalculateSegmentsAndLastBytes(metaShardRef.PayloadLength)
	metaShardPayload, err := s.sdb.FetchJAMDASegments(
		metaShardRef.WorkPackageHash,
		metaShardRef.IndexStart,
		metaShardRef.IndexStart+numSegments,
		metaShardRef.PayloadLength,
	)
	if err != nil {
		log.Error(log.SDB, "ReadObject: FetchJAMDASegments failed",
			"metaShardObjectID", metaShardObjectID,
			"workPackageHash", metaShardRef.WorkPackageHash,
			"indexStart", metaShardRef.IndexStart,
			"numSegments", numSegments,
			"payloadLength", metaShardRef.PayloadLength,
			"err", err,
		)
		return nil, false, fmt.Errorf("failed to fetch meta-shard payload from DA: %v", err)
	}

	// Step 5: Deserialize meta-shard and search for object_id
	_, metaShard, err := DeserializeMetaShard(metaShardPayload, metaShardObjectID, serviceID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to deserialize meta-shard: %v", err)
	}
	log.Trace(log.SDB, "ReadObject: meta-shard deserialized", "numEntries", len(metaShard.Entries))

	// Create a witness entry for this meta-shard and populate ObjectRefs cache
	witness := &types.StateWitness{
		ServiceID:           serviceID,
		Ref:                 metaShardRef,
		ObjectID:            metaShardObjectID,
		MetaShardMerkleRoot: metaShard.MerkleRoot,
		MetaShardPayload:    metaShardPayload,
		Path:                convertProofToHashes(proofBytes),
		Value:               metaShardRefBytes,
		BlockNumber:         blockNumber,
		Timeslot:            timeslot,
		ObjectRefs:          make(map[common.Hash]types.ObjectRef),
		Payloads:            make(map[common.Hash][]byte),
		ObjectProofs:        make(map[common.Hash][]common.Hash),
	}

	// Populate ObjectRefs map with all entries from the meta-shard
	for i := range metaShard.Entries {
		witness.ObjectRefs[metaShard.Entries[i].ObjectID] = metaShard.Entries[i].ObjectRef
		log.Trace(log.SDB, "ReadObject: meta-shard entry", "index", i, "objectID", metaShard.Entries[i].ObjectID, "objectRef", metaShard.Entries[i].ObjectRef.WorkPackageHash)
	}

	// Cache this witness for future lookups
	s.metashardWitnesses[metaShardObjectID] = witness

	// Look up the requested objectID in the cached map
	foundRef, hasRef := witness.ObjectRefs[objectID]
	if !hasRef {
		// DIAGNOSTIC: Dump meta-shard entries on miss
		log.Warn(log.SDB, "🔍 METASHARD_MISS: object_id not found",
			"lookupObjectID", objectID.Hex(),
			"metaShardID", metaShardObjectID.Hex(),
			"numEntries", len(metaShard.Entries),
		)
		for i, entry := range metaShard.Entries {
			log.Warn(log.SDB, "🔍 METASHARD_ENTRY",
				"index", i,
				"entryObjectID", entry.ObjectID.Hex(),
				"matchesLookup", entry.ObjectID == objectID,
			)
		}
		return nil, false, fmt.Errorf("object_id %s not found in meta-shard", objectID.Hex())
	}

	// Step 6: Fetch actual object payload from JAM DA
	numSegments, _ = types.CalculateSegmentsAndLastBytes(foundRef.PayloadLength)
	objectPayload, err := s.sdb.FetchJAMDASegments(
		foundRef.WorkPackageHash,
		foundRef.IndexStart,
		foundRef.IndexStart+numSegments,
		foundRef.PayloadLength,
	)
	if err != nil {
		return nil, false, fmt.Errorf("failed to fetch object payload from DA: %v", err)
	}

	// Cache the payload for future lookups
	witness.Payloads[objectID] = objectPayload

	// Generate BMT inclusion proof for this object within the meta-shard
	proof, proofErr := generateMetaShardInclusionProof(witness.MetaShardMerkleRoot, objectID, metaShard.Entries)
	if proofErr != nil {
		return nil, false, fmt.Errorf("failed to generate meta-shard proof for object %s: %v", objectID.Hex(), proofErr)
	}
	witness.ObjectProofs[objectID] = proof

	// Verify witness against stateRoot
	if !trie.Verify(serviceID, witness.ObjectID.Bytes(), witness.Value, s.StateRoot.Bytes(), witness.Path) {
		log.Error(log.SDB, "BMT Proof verification failed", "object_id", objectID.Hex())
		return nil, false, fmt.Errorf("BMT Proof verification failed for object %s", objectID.Hex())
	}

	// Return complete StateWitness with all verification data populated
	resultWitness := &types.StateWitness{
		ServiceID:           serviceID,
		ObjectID:            witness.ObjectID,
		Ref:                 metaShardRef,
		Path:                witness.Path,
		Value:               witness.Value,
		Payload:             objectPayload,
		MetaShardMerkleRoot: witness.MetaShardMerkleRoot,
		MetaShardPayload:    witness.MetaShardPayload,
		BlockNumber:         witness.BlockNumber,
		Timeslot:            witness.Timeslot,
		ObjectRefs:          witness.ObjectRefs,
		Payloads:            witness.Payloads,
		ObjectProofs:        witness.ObjectProofs,
	}
	return resultWitness, true, nil
}

// GetWitnesses returns the meta-shard witnesses cache containing all cached StateWitness data
// This includes both the ObjectRefs and Payloads maps built up during ReadObject calls
func (s *StateDB) GetWitnesses() map[common.Hash]*types.StateWitness {
	return s.metashardWitnesses
}

// FetchBlock is the special path for blocks
func (s *StateDB) FetchBlock(serviceID uint32, objectID common.Hash) (types.StateWitness, bool, error) {
	// Use the low-level trie method to get value, proof, and state root
	valueBytes, proofBytes, _, ok, err := s.sdb.GetServiceStorageWithProof(serviceID, objectID[:])
	if err != nil {
		return types.StateWitness{}, false, err
	}
	if !ok {
		return types.StateWitness{}, false, nil
	}
	path := convertProofToHashes(proofBytes)

	// Receipt and other objects: JAM State starts with ObjectRef + timeslot + blocknumber
	if len(valueBytes) < 45 {
		return types.StateWitness{}, true, fmt.Errorf("invalid value length: expected at least 45 (ObjectRef + timeslot + blocknumber), got %d", len(valueBytes))
	}
	// Deserialize ObjectRef from storage
	offset := 0
	objRef, deserErr := types.DeserializeObjectRef(valueBytes, &offset)
	if deserErr != nil {
		return types.StateWitness{}, true, fmt.Errorf("failed to deserialize ObjectRef: %w", deserErr)
	}

	// Extract timeslot and blocknumber from valueBytes[37:45]
	// Format: ObjectRef (37 bytes) + timeslot (4 bytes LE) + blocknumber (4 bytes LE) = 45 bytes total
	var extractedTimeslot, extractedBlockNumber uint32
	if len(valueBytes) >= 45 {
		extractedTimeslot = binary.LittleEndian.Uint32(valueBytes[37:41])
		extractedBlockNumber = binary.LittleEndian.Uint32(valueBytes[41:45])
	}

	witness := types.StateWitness{
		ServiceID:   serviceID,
		ObjectID:    objectID,
		Ref:         objRef,
		BlockNumber: extractedBlockNumber,
		Timeslot:    extractedTimeslot,
		Path:        path,
		Value:       valueBytes,
	}

	// fetch block with FetchJAMDASegments
	// Calculate number of segments needed
	numSegments, _ := types.CalculateSegmentsAndLastBytes(objRef.PayloadLength)
	indexEnd := objRef.IndexStart + numSegments
	payload, err := s.sdb.FetchJAMDASegments(objRef.WorkPackageHash, objRef.IndexStart, indexEnd, objRef.PayloadLength)
	if err != nil {
		// Don't fail if CE139 read fails, just leave payload empty
		witness.Payload = []byte{}
	} else {
		witness.Payload = payload
	}

	return witness, true, nil
}

// VerifyStateWitnessRef verifies a StateWitness proof against a state root
//
//	every imported object must undergo same verification against refine context state root
//	Note: service_id has been removed from ObjectRef, using 0 for verification
func VerifyStateWitnessRef(witness types.StateWitness, stateRoot common.Hash) bool {
	value := witness.Value
	if len(value) == 0 {
		serializedRef := witness.Ref.Serialize()
		value = serializedRef
		// Preserve accumulated metadata when available
		if witness.Timeslot != 0 || witness.BlockNumber != 0 {
			buf := make([]byte, 0, len(serializedRef)+8)
			buf = append(buf, serializedRef...)
			buf = append(buf, common.Uint32ToBytes(witness.Timeslot)...)
			buf = append(buf, common.Uint32ToBytes(witness.BlockNumber)...)
			value = buf
		}
	}

	// For meta-sharded objects, proof is for MetaShard Key
	proofKey := witness.ObjectID[:]

	return trie.Verify(witness.ServiceID, proofKey, value, stateRoot[:], witness.Path)
}

func (s *StateDB) GetForgets() []*types.SubServiceRequestResult {
	return s.stateUpdate.GetForgets()
}

// EVMStorage exposes EVM storage operations for PVM host functions.
func (s *StateDB) EVMStorage() (types.EVMJAMStorage, bool) {
	evmStorage, ok := s.sdb.(types.EVMJAMStorage)
	return evmStorage, ok
}

// FetchJAMDASegments proxies DA segment retrieval to the underlying storage.
func (s *StateDB) FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (payload []byte, err error) {
	return s.sdb.FetchJAMDASegments(workPackageHash, indexStart, indexEnd, payloadLength)
}
