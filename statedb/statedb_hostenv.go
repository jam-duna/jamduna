package statedb

import (
	"fmt"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
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
	log.Trace(log.SDB, "!writeAccount", "service_idx", sa.ServiceIndex, "dirty", sa.Dirty, "s", sa.JsonString())

	service_idx := sa.GetServiceIndex()
	tree := s.GetTrie()
	start_StorageSize := sa.StorageSize
	start_NumStorageItems := sa.NumStorageItems
	serviceUpdate = types.NewServiceUpdate(service_idx)
	t0 := time.Now()
	for _, storage := range sa.Storage {
		log.Trace(log.SDB, "writeAccount Storage", "service_idx", service_idx, "key", fmt.Sprintf("%x", storage.Key), "rawkey", storage.InternalKey, "storage.Accessed", storage.Accessed, "storage.Deleted", storage.Deleted, "storage.source", storage.Source)
		as_internal_key := storage.InternalKey
		if storage.Dirty {
			if storage.Deleted {
				log.Trace(s.Authoring, "writeAccount DELETE", "service_idx", service_idx, "key", fmt.Sprintf("%x", storage.Key), "rawkey", as_internal_key, "storage.Accessed", storage.Accessed, "storage.Deleted", storage.Deleted, "storage.source", storage.Source)
				if storage.Source == "trie" {
					err = tree.DeleteServiceStorage(service_idx, storage.Key)
					if err != nil {
						// DeleteServiceStorageKey: Failed to delete k: 0xffffffffdecedb51effc9737c5fea18873dbf428c55f0d5d3b522672f234a9b1, error: key not found
						//log.Debug(log.SDB, "DeleteServiceStorage Failure", "n", s.Id, "service_idx", service_idx, "key", fmt.Sprintf("%x", storage.Key), "rawkey", as_internal_key, "err", err)
						continue
					}
				} else {
					//log.Trace(log.SDB, "DeleteServiceStorage from memory", "n", s.Id, "service_idx", service_idx, "key", fmt.Sprintf("%x", storage.Key), "rawkey", as_internal_key, "err", err)
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
	benchRec.Add("*** ApplyXContext: writeAccount:Storage", time.Since(t0))

	t0 = time.Now()
	for blob_hash_str, v := range sa.Lookup {
		log.Trace(log.SDB, "writeAccount Lookup", "service_idx", service_idx, "blob_hash_str", blob_hash_str, "v.Dirty", v.Dirty, "v.Deleted", v.Deleted, "v.Z", v.Z, "v.Timeslots", v.Timeslots, "source", v.Source)
		blob_hash := common.HexToHash(blob_hash_str)
		if v.Dirty {
			if v.Deleted {
				if v.Source == "trie" {
					err = tree.DeletePreImageLookup(service_idx, blob_hash, v.Z)
					if err != nil {
						log.Warn(log.SDB, "tree.DeletePreImageLookup", "blob_hash", blob_hash, "v.Z", v.Z, "err", err)
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
	benchRec.Add("*** ApplyXContext: writeAccount:Lookup", time.Since(t0))

	t0 = time.Now()
	for blobHash_str, v := range sa.Preimage {
		//log.Debug(log.SDB, "writeAccount Preimage", "service_idx", service_idx, "blobHash_str", blobHash_str, "v.Dirty", v.Dirty, "v.Deleted", v.Deleted, "len(v.Preimage)", len(v.Preimage), "source", v.Source)
		blobHash := common.HexToHash(blobHash_str)
		if v.Dirty {
			if v.Deleted {
				if v.Source == "trie" {
					err = tree.DeletePreImageBlob(service_idx, blobHash)
					if err != nil {
						log.Warn(log.SDB, "DeletePreImageBlob Forget A", "blobHash", blobHash, "err", err, "v.Deleted", v.Deleted, "len(v.Preimage)", len(v.Preimage), "source", v.Source)
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
	benchRec.Add("*** ApplyXContext: writeAccount:Preimage", time.Since(t0))

	if sa.DeletedAccount {
		err = tree.DeleteService(service_idx)
		if err != nil {
			log.Warn(log.SDB, "tree.DeleteService", "service_idx", service_idx, "err", err)
			return
		}
		//log.Info(log.SDB, "tree.DeleteService SUCCESS", "service_idx", service_idx)
	} else {
		t0 = time.Now()
		err = s.writeService(service_idx, sa)
		benchRec.Add("*** ApplyXContext: writeAccount:writeService", time.Since(t0))

	}
	if sa.NumStorageItems != start_NumStorageItems {
		fmt.Printf(" a_i [s=%d] changed from %d to %d\n", sa.ServiceIndex, start_NumStorageItems, sa.NumStorageItems)
	}
	if sa.StorageSize != start_StorageSize {
		fmt.Printf(" a_o [s=%d] changed from %d to %d\n", sa.ServiceIndex, start_StorageSize, sa.StorageSize)
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
			t0 := time.Now()
			serviceUpdate, err := s.writeAccount(sa)
			if err != nil {
				log.Crit(log.SDB, "ApplyXContext", "err", err)
			}
			if false {
				stateUpdate.AddServiceUpdate(sa.ServiceIndex, serviceUpdate)
			}
			benchRec.Add("-- ApplyXContext: writeAccount", time.Since(t0))
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
	log.Trace(log.P, "ReadServicePreimageLookup", "service", service, "blob_hash", blob_hash, "blob_length", blob_length, time_slots)
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

func (s *StateDB) GetForgets() []*types.SubServiceRequestResult {
	return s.stateUpdate.GetForgets()
}
