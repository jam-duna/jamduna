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

func (s *StateDB) writeAccount(sa *types.ServiceAccount) (err error) {
	if sa.Mutable == false {
		panic("WriteAccount")
	}
	service_idx := sa.GetServiceIndex()
	tree := s.GetTrie()
	start_StorageSize := sa.StorageSize
	start_NumStorageItems := sa.NumStorageItems
	for k, storage := range sa.Storage {
		if storage.Dirty {
			oldValue, exists, err := tree.GetServiceStorage(service_idx, storage.RawKey)
			if err != nil {
				fmt.Printf("GetServiceStorage err %v\n", err)
				panic(err)
				return err
			}
			log.Trace(module, "writeAccount", "STORAGE key", storage.RawKey, "exists", exists, "oldValue", oldValue, "newValue", storage.Value)

			if len(storage.Value) == 0 || storage.Deleted {
				err = tree.DeleteServiceStorage(service_idx, storage.RawKey)
				if err != nil {
					fmt.Printf("DeleteServiceStorageKey: Failed to delete k: %v, error: %v\n", k, err)
					return err
				}
			} else {
				err = tree.SetServiceStorage(service_idx, storage.RawKey, storage.Value)
				if err != nil {
					fmt.Printf("SetServiceStorage err %v\n", err)
					return err
				}
			}
		}
	}
	for blob_hash, v := range sa.Lookup {
		if v.Dirty {
			if v.Deleted {
				panic("check this case as [] is natural -- does it exist")
			} else {
				err = tree.SetPreImageLookup(service_idx, blob_hash, v.Z, v.T)
				if err != nil {
					return err
				}
			}
		}
	}
	for blobHash, v := range sa.Preimage {
		if v.Dirty {
			if len(v.Preimage) == 0 || v.Deleted {
				err = tree.DeletePreImageBlob(service_idx, blobHash)
				if err != nil {
					return err
				}
			} else {
				err = tree.SetPreImageBlob(service_idx, v.Preimage)
				if err != nil {
					return err
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
	return err
}

func (s *StateDB) ApplyXContext(U *types.PartialState) {

	for _, sa := range U.D {
		// U.D should only have service accounts with Mutable = true
		if sa.Mutable == false {
			fmt.Printf("ApplyXContext -- Immutable %d in U.X\n", sa.ServiceIndex)
			panic("Immutable Service account in X.U.D")
		} else if sa.Dirty {
			err := s.writeAccount(sa)
			if err != nil {
				panic(err)
			}
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

	ap_internal_key := common.Compute_preimageBlob_internal(common.Blake2Hash(blob_hash.Bytes()))
	account_preimagehash := common.ComputeC_sh(a.ServiceIndex, ap_internal_key)

	blob := a.Preimage[account_preimagehash].Preimage
	al_internal_key := common.Compute_preimageLookup_internal(blob_hash, uint32(len(blob)))
	account_lookuphash := common.ComputeC_sh(a.ServiceIndex, al_internal_key)

	timeslots := a.Lookup[account_lookuphash].T
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
