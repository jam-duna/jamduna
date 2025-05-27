package statedb

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

const OK uint32 = 0

const (
	x_s = "S"
	x_c = "C"
	x_v = "V"
	x_i = "I"
	x_t = "T"
	x_n = "N"
	x_p = "P"

	debugStorageCalc = false
)

func (s *StateDB) writeAccount(sa *types.ServiceAccount) (serviceUpdate *types.ServiceUpdate, err error) {
	if sa.Mutable == false {
		return nil, fmt.Errorf("WriteAccount")
	}
	if sa.Dirty == false {
		return nil, nil
	}
	service_idx := sa.GetServiceIndex()
	tree := s.GetTrie()
	start_StorageSize := sa.StorageSize
	start_NumStorageItems := sa.NumStorageItems
	serviceUpdate = types.NewServiceUpdate(service_idx)
	for _, storage := range sa.Storage {
		if storage.Dirty {
			if len(storage.Value) == 0 || storage.Deleted {
				log.Trace(s.Authoring, "writeAccount DELETE", "service_idx", service_idx, "rawkey", storage.RawKey, "storage.Accessed", storage.Accessed, "storage.Deleted", storage.Deleted)
				err = tree.DeleteServiceStorage(service_idx, storage.RawKey)
				if err != nil {
					// DeleteServiceStorageKey: Failed to delete k: 0xffffffffdecedb51effc9737c5fea18873dbf428c55f0d5d3b522672f234a9b1, error: key not found
					log.Warn(module, "DeleteServiceStorage Failure", "n", s.Id, "service_idx", service_idx, "rawkey", storage.RawKey, "err", err)
					return
				}
			} else {
				log.Trace(module, "writeAccount SET", "service_idx", fmt.Sprintf("%d", service_idx), "k", fmt.Sprintf("%x", storage.Key), "rawkey", storage.RawKey, "value", storage.Value)
				// Here we are returning ALL storage values written
				serviceUpdate.ServiceValue[storage.RawKey] = &types.SubServiceValueResult{
					ServiceID:  service_idx,
					HeaderHash: s.HeaderHash,
					Slot:       s.GetTimeslot(),
					Hash:       storage.RawKey,
					Key:        common.Bytes2Hex(storage.Key),
					Value:      common.Bytes2Hex(storage.Value),
				}
				err = tree.SetServiceStorage(service_idx, storage.RawKey, storage.Value)
				if err != nil {
					log.Warn(module, "SetServiceStorage Failure", "n", s.Id, "service_idx", service_idx, "rawkey", storage.RawKey, "err", err)
					return
				}
			}
		}
	}

	for blob_hash, v := range sa.Lookup {
		if v.Dirty {
			if v.Deleted {
				err = tree.DeletePreImageLookup(service_idx, blob_hash, v.Z)
				if err != nil {
					log.Warn(module, "tree.DeletePreImageLookup", "blob_hash", blob_hash, "v.Z", v.Z, "err", err)
					return
				}
				log.Info("authoring", "tree.DeletePreImageLookup [FORGET OK]", "blob_hash", blob_hash, "v.Z", v.Z)
				serviceUpdate.ServiceRequest[blob_hash] = &types.SubServiceRequestResult{
					HeaderHash: s.HeaderHash,
					Slot:       s.GetTimeslot(),
					Hash:       blob_hash,
					ServiceID:  service_idx,
					Timeslots:  nil,
				}
			} else {
				err = tree.SetPreImageLookup(service_idx, blob_hash, v.Z, v.T)
				if err != nil {
					log.Warn(module, "tree.SetPreimageLookup", "blob_hash", blob_hash, "v.Z", v.Z, "v.T", v.T, "err", err)
					return
				}
				serviceUpdate.ServiceRequest[blob_hash] = &types.SubServiceRequestResult{
					HeaderHash: s.HeaderHash,
					Slot:       s.GetTimeslot(),
					Hash:       blob_hash,
					ServiceID:  service_idx,
					Timeslots:  v.T,
				}
			}
		}
	}
	for blobHash, v := range sa.Preimage {
		if v.Dirty {
			if len(v.Preimage) == 0 || v.Deleted {
				err = tree.DeletePreImageBlob(service_idx, blobHash)
				if err != nil {
					log.Warn(module, "DeletePreImageBlob", "blobHash", blobHash, "err", err)
					return
				}
				log.Info("authoring", "DeletePreImageBlob [FORGET OK]", "blobHash", blobHash)
			} else {
				err = tree.SetPreImageBlob(service_idx, v.Preimage)
				if err != nil {
					log.Warn(module, "SetPreImageBlob", "err", err)
					return
				}
				serviceUpdate.ServicePreimage[blobHash] = &types.SubServicePreimageResult{
					HeaderHash: s.HeaderHash,
					Slot:       s.GetTimeslot(),
					Hash:       blobHash,
					ServiceID:  service_idx,
					Preimage:   common.Bytes2Hex(v.Preimage),
				}
			}
		}
	}
	err = s.writeService(service_idx, sa)
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
	return
}

func (s *StateDB) ApplyXContext(U *types.PartialState) (stateUpdate *types.StateUpdate) {
	stateUpdate = types.NewStateUpdate()
	for _, sa := range U.D {
		// U.D should only have service accounts with Mutable = true
		if sa.Mutable == false {
			log.Crit(module, "Immutable Service account in X.U.D", "s", sa.ServiceIndex)
		} else if sa.Dirty {
			serviceUpdate, err := s.writeAccount(sa)
			if err != nil {
				log.Crit(module, "ApplyXContext", "err", err)
			}
			stateUpdate.AddServiceUpdate(sa.ServiceIndex, serviceUpdate)
		}
	}
	// p - Bless => Kai_state 12.4.1 (164)
	s.JamState.PrivilegedServiceIndices = U.PrivilegedState

	// c - Designate => AuthorizationQueue
	for i := 0; i < types.TotalCores; i++ {
		copy(s.JamState.AuthorizationQueue[i][:], U.QueueWorkReport[i][:])
	}

	// v - Assign => DesignatedValidators
	s.JamState.SafroleState.DesignedValidators = U.UpcomingValidators
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
	v, _ := sa.Bytes()
	tree := s.GetTrie()
	return tree.SetService(service, v)
}

func (s *StateDB) ReadServiceStorage(service uint32, k common.Hash) (storage []byte, ok bool, err error) {
	// not init case
	tree := s.GetTrie()
	storage, ok, err = tree.GetServiceStorage(service, k)
	if err != nil || !ok {
		return
	}
	return
}

func (s *StateDB) ReadServicePreimageBlob(service uint32, blob_hash common.Hash) (blob []byte, ok bool, err error) {
	tree := s.GetTrie()
	blob, ok, err = tree.GetPreImageBlob(service, blob_hash)
	if err != nil || !ok {
		return
	} else {
		log.Trace(debugP, "ReadServicePreimageBlob", "service", service, "blob_hash", blob_hash, "len(blob)", len(blob))
		return
	}
}

func (s *StateDB) ReadServicePreimageLookup(service uint32, blob_hash common.Hash, blob_length uint32) (time_slots []uint32, ok bool, err error) {
	tree := s.GetTrie()
	time_slots, ok, err = tree.GetPreImageLookup(service, blob_hash, blob_length)
	if err != nil || !ok {
		return
	}
	log.Trace(debugP, "ReadServicePreimageLookup", "service", service, "blob_hash", blob_hash, "blob_length", blob_length, time_slots)
	return

}

// HistoricalLookup, GetImportItem, ExportSegment
func (s *StateDB) HistoricalLookup(a *types.ServiceAccount, t uint32, blob_hash common.Hash) []byte {
	ok, blob := a.ReadPreimage(blob_hash, s)
	if !ok {
		return nil
	}

	z := uint32(len(blob))

	ok, timeslots := a.ReadLookup(blob_hash, z, s)
	if !ok {
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
