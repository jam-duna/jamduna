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

func (s *StateDB) WriteAccount(sa types.ServiceAccount) {
	service_idx := sa.ServiceIndex()
	//two ways of proceeding this... via journal (most reliable) or via memory (faster but order is not guaranteed)
	for key, storage := range sa.Storage {
		if storage.Dirty {
			if len(storage.Value) == 0 || storage.Deleted {
				s.DeleteServiceStorageKey(service_idx, key)
			} else {
				s.WriteServiceStorage(service_idx, key, storage.Value)
			}
		}
	}
	for blobHash, v := range sa.Lookup {
		if v.Dirty {
			if v.Deleted {
				s.DeleteServicePreimageLookupKey(service_idx, blobHash, v.Z)
			} else {
				s.WriteServicePreimageLookup(service_idx, blobHash, v.Z, v.T)
			}
		}
	}
	for blobHash, v := range sa.Preimage {
		if v.Dirty {
			if len(v.Preimage) == 0 || v.Deleted {
				s.DeleteServicePreimageKey(service_idx, blobHash)
			} else {
				s.WriteServicePreimageBlob(service_idx, v.Preimage)
			}
		}
	}
	s.WriteService(service_idx, sa)
}

func (s *StateDB) ApplyXContext(U *types.PartialState) {
	for _, sa := range U.D {
		if sa.Dirty {
			s.WriteAccount(*sa)
		}
	}
	// p - Empower => Kai_state 12.4.1 (164)
	s.JamState.PrivilegedServiceIndices = U.PrivilegedState

	// c - Designate => AuthorizationQueue
	for i := 0; i < types.TotalCores; i++ {
		copy(s.JamState.AuthorizationQueue[i], U.QueueWorkReport[i][:])
	}
	// v - Assign => DesignatedValidators
	s.JamState.SafroleState.DesignedValidators = U.UpcomingValidators
}

func (s *StateDB) GetTimeslot() uint32 {
	// Successfully casted to *statedb.StateDB
	sf := s.GetSafrole()
	return sf.GetTimeSlot()
}

func (s *StateDB) ReadServiceBytes(service uint32) []byte {
	tree := s.GetTrie()
	value, err := tree.GetService(255, service)
	if err != nil {
		return nil
	}
	return value
}

func (s *StateDB) GetService(service uint32) (*types.ServiceAccount, error) {
	serviceBytes := s.ReadServiceBytes(service)
	if serviceBytes == nil {
		return nil, fmt.Errorf("Service not found")
	}
	return types.ServiceAccountFromBytes(service, serviceBytes)
}

func (s *StateDB) WriteService(service uint32, sa types.ServiceAccount) {
	v, _ := sa.Bytes()
	s.WriteServiceBytes(service, v)
}

func (s *StateDB) WriteServiceBytes(service uint32, v []byte) {
	tree := s.GetTrie()
	tree.SetService(255, service, v)
}

func (s *StateDB) ReadServiceStorage(service uint32, k common.Hash) []byte {
	// not init case
	tree := s.GetTrie()
	storage, err := tree.GetServiceStorage(service, k)
	if err != nil {
		return nil
	} else {
		//fmt.Printf("ReadServiceStorage (S,K)=(%v,%x) RESULT: storage=%x, err=%v\n", service, k, storage, err)
		return storage
	}
}

func (s *StateDB) WriteServiceStorage(service uint32, k common.Hash, storage []byte) {
	tree := s.GetTrie()
	tree.SetServiceStorage(service, k, storage)
}

func (s *StateDB) ReadServicePreimageBlob(service uint32, blob_hash common.Hash) []byte {
	tree := s.GetTrie()
	blob, err := tree.GetPreImageBlob(service, blob_hash)
	if err != nil {
		return nil
	} else {
		if debug {
			fmt.Printf("ReadServicePreimageBlob (s,l)=(%v, %v) RESULT: blob=%x (len=%v), err=%v\n", service, blob_hash, blob, len(blob), err)
		}
		return blob
	}
}

func (s *StateDB) WriteServicePreimageBlob(service uint32, blob []byte) {
	tree := s.GetTrie()
	tree.SetPreImageBlob(service, blob)
}

func (s *StateDB) ReadServicePreimageLookup(service uint32, blob_hash common.Hash, blob_length uint32) []uint32 {
	tree := s.GetTrie()
	time_slots, err := tree.GetPreImageLookup(service, blob_hash, blob_length)
	if err != nil {
		return nil
	} else {
		fmt.Printf("ReadServicePreimageLookup (s, (h,l))=(%v, (%v,%v))  RESULT: time_slots=%v, err=%v\n", service, blob_hash, blob_length, time_slots, err)
		return time_slots
	}
}

func (s *StateDB) WriteServicePreimageLookup(service uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32) {
	//fmt.Printf("WriteServicePreimageLookup Called! service=%v, (h,l)=(%v,%v). anchors:%v\n", service, blob_hash, blob_length, time_slots)
	tree := s.GetTrie()
	tree.SetPreImageLookup(service, blob_hash, blob_length, time_slots)
	//fmt.Printf("Latest root=%v\n", s.GetTentativeStateRoot())
	//v, _ := tree.GetPreImageLookup(service, blob_hash, blob_length)
	//fmt.Printf("GetPreImageLookup0 right after %v\n", v)
}

// HistoricalLookup, GetImportItem, ExportSegment
func (s *StateDB) HistoricalLookup(service uint32, t uint32, blob_hash common.Hash) []byte {
	tree := s.GetTrie()
	rootHash := tree.GetRoot()
	fmt.Printf("Root Hash=%v\n", rootHash)
	blob, err_v := tree.GetPreImageBlob(service, blob_hash)
	if err_v != nil {
		return nil
	}

	blob_length := uint32(len(blob))

	timeslots, err_t := tree.GetPreImageLookup(service, blob_hash, blob_length)
	if err_t != nil {
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

func (s *StateDB) DeleteServiceStorageKey(service uint32, k common.Hash) error {
	tree := s.GetTrie()
	err := tree.DeleteServiceStorage(service, k)
	if err != nil {
		fmt.Printf("DeleteServiceStorageKey: Failed to delete k: %x, error: %v\n", k, err)
		return err
	}
	return nil
}

func (s *StateDB) DeleteServicePreimageKey(service uint32, blob_hash common.Hash) error {
	tree := s.GetTrie()
	err := tree.DeletePreImageBlob(service, blob_hash)
	if err != nil {
		fmt.Printf("DeleteServicePreimageKey: Failed to delete blob_hash: %x, error: %v\n", blob_hash.Bytes(), err)
		return err
	}
	return nil
}

func (s *StateDB) DeleteServicePreimageLookupKey(service uint32, blob_hash common.Hash, blob_length uint32) error {
	tree := s.GetTrie()

	err := tree.DeletePreImageLookup(service, blob_hash, blob_length)
	if err != nil {
		// =====> CHECK fmt.Printf("DeleteServicePreimageLookupKey: Failed to delete blob_hash: %v, blob_lookup_len: %d, error: %v\n", blob_hash, blob_length, err)
		return err
	}
	return nil
}
