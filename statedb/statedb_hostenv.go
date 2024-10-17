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

func (s *StateDB) WriteAccount(sa *types.ServiceAccount) {
	service_idx := sa.ServiceIndex()
	//two ways of proceeding this... via journal (most reliable) or via memory (faster but order is not guaranteed)
	jounrnals := sa.GetJournals()
	for _, j := range jounrnals {
		op_type, _, obj := j.GetJournalRecordType()
		switch v := obj.(type) {
		case types.JournalStorageKV:
			if op_type == types.JournalOPWrite {
				s.WriteServiceStorage(service_idx, v.K, v.V)
			} else if op_type == types.JournalOPDelete {
				s.DeleteServiceStorageKey(service_idx, v.K)
			}
		case types.JournalLookupKV:
			if op_type == types.JournalOPWrite {
				s.WriteServicePreimageLookup(service_idx, v.H, v.Z, v.T)
			} else if op_type == types.JournalOPDelete {
				s.DeleteServicePreimageLookupKey(service_idx, v.H, v.Z)
			}
		case types.JournalPreimageKV:
			if op_type == types.JournalOPWrite {
				s.WriteServicePreimageBlob(service_idx, v.P)
			} else if op_type == types.JournalOPDelete {
				s.DeleteServicePreimageKey(service_idx, v.H)
			}
		default:
			panic(0)
		}
	}
	s.WriteService(service_idx, sa)
}

func (s *StateDB) GetXContext() *types.XContext {
	return s.X
}

func (s *StateDB) SetXContext(X *types.XContext) {
	s.X = X
}

func (s *StateDB) UpdateXContext(X *types.XContext) {
	s.X = X
}

func (s *StateDB) ApplyXContext() {

	x := s.X
	if x.S != nil {
		// TODO: write s
		s.WriteAccount(x.S)
	}
	// n - NewService 12.4.2 (165)
	for _, sa := range x.N {
		s.WriteAccount(sa)
	}
	return
	// TODO: Sourabh to connect this portion next
	// p - Empower => Kai_state 12.4.1 (164)
	// s.JamState.PrivilegedServiceIndices.Kai_m = x.P.M
	// s.JamState.PrivilegedServiceIndices.Kai_a = x.P.A
	// s.JamState.PrivilegedServiceIndices.Kai_v = x.P.V

	// c - Designate => AuthorizationQueue
	for i := 0; i < types.TotalCores; i++ {
		copy(s.JamState.AuthorizationQueue[i], x.C[i][:])
	}
	// v - Assign => DesignatedValidators

	fmt.Printf("AAA=%v\n", s.JamState)
	fmt.Printf("BBB=%v\n", s.JamState.SafroleState)
	fmt.Printf("CCC=%v\n", s.JamState.SafroleState.DesignedValidators)
	//fmt.Printf("DDD=%v\n", s.JamState)

	//s.JamState.SafroleState.DesignedValidators = x.V // []types.Validator `json:"designed_validators"`
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
		return nil, nil
	}
	return types.ServiceAccountFromBytes(service, serviceBytes)
}

func (s *StateDB) WriteService(service uint32, sa *types.ServiceAccount) {
	v, _ := sa.Bytes()
	s.WriteServiceBytes(service, v)
}

func (s *StateDB) WriteServiceBytes(service uint32, v []byte) {
	tree := s.GetTrie()
	tree.SetService(255, service, v)
}

func (s *StateDB) ReadServiceStorage(service uint32, k []byte) []byte {
	// first check journal
	XContext := s.GetXContext()
	xs := XContext.GetX_s()
	foundInJournal := false
	if service != xs.ServiceIndex() {
		//weird.. how?
	}
	foundInJournal, error, val := xs.JournalGetStorage(k)
	if foundInJournal {
		if error != nil {
			// there shouldn't be error..
		}
		return val
	}

	// not init case
	tree := s.GetTrie()
	storage, err := tree.GetServiceStorage(service, k)
	if err != nil {
		return nil
	} else {
		fmt.Printf("ReadServiceStorage (S,K)=(%v,%x) RESULT: storage=%x, err=%v\n", service, k, storage, err)
		return storage
	}
}

func (s *StateDB) WriteServiceStorage(service uint32, k []byte, storage []byte) {
	tree := s.GetTrie()
	tree.SetServiceStorage(service, k, storage)
}

func (s *StateDB) ReadServicePreimageBlob(service uint32, blob_hash common.Hash) []byte {
	//TODO: willaim DO NOT disable this

	// XContext := s.GetXContext()
	// xs := XContext.GetX_s()
	// sa := &types.ServiceAccount{}
	// if service == xs.ServiceIndex() {
	// 	sa = xs
	// } else {
	// 	// weird .. how
	// }
	// found, sa := XContext.GetX_n(service)
	// if found {
	// 	foundInJournal, error, blob := sa.JournalGetPreimage(blob_hash)
	// 	if foundInJournal {
	// 		if error != nil {
	// 			// there shouldn't be error..
	// 		}
	// 		return blob
	// 	}
	// }

	// not init case
	tree := s.GetTrie()
	blob, err := tree.GetPreImageBlob(service, blob_hash.Bytes())
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
	// first check journal
	XContext := s.GetXContext()
	xs := XContext.GetX_s()
	if service != xs.ServiceIndex() {
		//weird.. how?
	}
	foundInJournal, error, anchors := xs.JournalGetLookup(blob_hash, blob_length)
	if foundInJournal {
		if error != nil {
			// there shouldn't be error..
		}
		return anchors
	}
	// not init case
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
	fmt.Printf("WriteServicePreimageLookup Called! service=%v, (h,l)=(%v,%v). anchors:%v\n", service, blob_hash, blob_length, time_slots)
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
	blob, err_v := tree.GetPreImageBlob(service, blob_hash.Bytes())
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

func (s *StateDB) DeleteServiceStorageKey(service uint32, k []byte) error {
	tree := s.GetTrie()
	err := tree.DeleteServiceStorage(service, k)
	if err != nil {
		fmt.Printf("Failed to delete k: %x, error: %v", k, err)
		return err
	}
	return nil
}

func (s *StateDB) DeleteServicePreimageKey(service uint32, blob_hash common.Hash) error {
	tree := s.GetTrie()
	err := tree.DeletePreImageBlob(service, blob_hash.Bytes())
	if err != nil {
		fmt.Printf("Failed to delete blob_hash: %x, error: %v", blob_hash.Bytes(), err)
		return err
	}
	return nil
}

func (s *StateDB) DeleteServicePreimageLookupKey(service uint32, blob_hash common.Hash, blob_length uint32) error {
	tree := s.GetTrie()

	err := tree.DeletePreImageLookup(service, blob_hash, blob_length)
	if err != nil {
		fmt.Printf("Failed to delete blob_hash: %v, blob_lookup_len: %d, error: %v", blob_hash, blob_length, err)
		return err
	}
	return nil
}

func (s *StateDB) empower(p *types.Empower) uint32 {
	s.X.P = p
	return OK
}

func (s *StateDB) designate(designate *types.Designate) uint32 {
	for i := 0; i < types.TotalValidators; i++ {
		s.X.V[i], _ = types.ValidatorFromBytes(designate.V[i*176 : (i+1)*176])
	}
	return OK
}

func (s *StateDB) assign(assign *types.Assign) uint32 {
	for i := 0; i < types.MaxAuthorizationQueueItems; i++ {
		s.X.C[assign.Core][i] = common.BytesToHash(assign.C[i*32 : (i+1)*32])
	}
	return OK
}

func (s *StateDB) addTransfer(t *types.AddTransfer) uint32 {
	s.X.T = append(s.X.T, t)
	return OK
}

func (s *StateDB) SetX(obj interface{}) uint32 {
	switch v := obj.(type) {
	case types.Empower:
		return s.empower(&v)
	case types.Designate:
		return s.designate(&v)
	case types.Assign:
		return s.assign(&v)
	case types.AddTransfer:
		return s.addTransfer(&v)
	default:
		panic(0)
	}
	return 0
}

func (s *StateDB) GetX(ctx string) interface{} {
	// for each possible value of ctx return some internal state variable of X
	x := s.X
	switch ctx {
	case x_s:
		return x.S
	case x_c:
		return x.C
	case x_v:
		return x.V
	case x_i:
		return x.I
	case x_t:
		return x.T
	case x_n:
		return x.N
	case x_p:
		return x.P
	default:
		panic(0)
	}
	return 0
}
