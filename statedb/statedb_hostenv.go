package statedb

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
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
)

func (s *StateDB) writeAccount(sa *types.ServiceAccount) (err error) {
	if sa.Mutable == false {
		panic("WriteAccount")
	}
	service_idx := sa.GetServiceIndex()
	tree := s.GetTrie()
	//fmt.Printf("[N%d] WriteAccount %v\n", s.Id, sa.String())
	for k, storage := range sa.Storage {
		if storage.Dirty {
			if len(storage.Value) == 0 || storage.Deleted {
				err = tree.DeleteServiceStorage(service_idx, storage.RawKey)
				if err != nil {
					fmt.Printf("DeleteServiceStorageKey: Failed to delete k: %x, error: %v\n", k, err)
					return err
				}
			} else {
				tree.SetServiceStorage(service_idx, storage.RawKey, storage.Value)

			}
		}
	}
	for blob_hash, v := range sa.Lookup {
		if v.Dirty {
			if v.Deleted {
				panic("check this case as [] is natural -- does it exist")
			} else {
				tree.SetPreImageLookup(service_idx, blob_hash, v.Z, v.T)
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
				tree.SetPreImageBlob(service_idx, v.Preimage)
			}
		}
	}
	s.writeService(service_idx, sa)
	return nil
}

func (s *StateDB) ApplyXContext(U *types.PartialState) {

	for _, sa := range U.D {
		// U.D should only have service accounts with Mutable = true
		if sa.Mutable == false {
			fmt.Printf("ApplyXContext -- Immutable %d in U.X\n", sa.ServiceIndex)
			panic("Immutable Service account in X.U.D")
		} else if sa.Dirty {
			s.writeAccount(sa)
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
	serviceBytes, ok, err = tree.GetService(255, service)
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

func (s *StateDB) writeService(service uint32, sa *types.ServiceAccount) {
	v, _ := sa.Bytes()
	tree := s.GetTrie()
	tree.SetService(255, service, v)
}

func (s *StateDB) ReadServiceStorage(service uint32, k []byte) (storage []byte, ok bool, err error) {
	// not init case
	tree := s.GetTrie()
	storage, ok, err = tree.GetServiceStorage(service, k)
	if err != nil || !ok {
		return
	} else {
		//fmt.Printf("ReadServiceStorage (S,K)=(%v,%x) RESULT: storage=%x, err=%v\n", service, k, storage, err)
		return
	}
}

func (s *StateDB) ReadServicePreimageBlob(service uint32, blob_hash common.Hash) (blob []byte, ok bool, err error) {
	tree := s.GetTrie()
	blob, ok, err = tree.GetPreImageBlob(service, blob_hash)
	if err != nil || !ok {
		return
	} else {
		if debug {
			fmt.Printf("ReadServicePreimageBlob (s,l)=(%v, %v) RESULT: blob=%x (len=%v), err=%v\n", service, blob_hash, blob, len(blob), err)
		}
		return
	}
}

func (s *StateDB) ReadServicePreimageLookup(service uint32, blob_hash common.Hash, blob_length uint32) (time_slots []uint32, ok bool, err error) {
	tree := s.GetTrie()
	time_slots, ok, err = tree.GetPreImageLookup(service, blob_hash, blob_length)
	if err != nil || !ok {
		return
	} else {
		fmt.Printf("ReadServicePreimageLookup (s, (h,l))=(%v, (%v,%v))  RESULT: time_slots=%v, err=%v\n", service, blob_hash, blob_length, time_slots, err)
		return
	}
}

// HistoricalLookup, GetImportItem, ExportSegment
func (s *StateDB) HistoricalLookup(service uint32, t uint32, blob_hash common.Hash) []byte {
	tree := s.GetTrie()
	rootHash := tree.GetRoot()
	fmt.Printf("Root Hash=%v\n", rootHash)
	blob, ok, err_v := tree.GetPreImageBlob(service, blob_hash)
	if err_v != nil || !ok {
		return nil
	}

	blob_length := uint32(len(blob))

	timeslots, ok, err_t := tree.GetPreImageLookup(service, blob_hash, blob_length)
	if err_t != nil || !ok {
		return nil
	}

	if timeslots[0] == 0 {
		return nil
	} else if len(timeslots) == (12 + 1) {
		x := timeslots[0]
		y := timeslots[1]
		z := timeslots[2]
		if (x <= t && t < y) || (z <= t) {
			return blob
		} else {
			return nil
		}
	} else if len(timeslots) == (8 + 1) {
		x := timeslots[0]
		y := timeslots[1]
		if x <= t && t < y {
			return blob
		} else {
			return nil
		}
	} else {
		x := timeslots[0]
		if x <= t {
			return blob
		} else {
			return nil
		}
	}
}
