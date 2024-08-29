package statedb

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

const OK uint32 = 0

const (
	x_s = "s"
	x_c = "c"
	x_v = "v"
	x_i = "i"
	x_t = "t"
	x_n = "n"
	x_p = "p"
)

type XContext struct {
	s *types.ServiceAccount
	c [types.TotalCores][types.MaxAuthorizationQueueItems]common.Hash
	v []types.Validator
	i uint32
	t []*types.AddTransfer
	n map[uint32]*types.ServiceAccount
	p *types.Empower
}

func (s *StateDB) ApplyXContext() {
	return
	x := s.X
	if x.s != nil {
		// TODO: write s
		s.WriteService(s.S, x.s)
	}
	// n - NewService 12.4.2 (165)
	for service, sa := range x.n {
		// TODO: write service => sa
		s.WriteService(service, sa)
	}
	// p - Empower => Kai_state 12.4.1 (164)
	s.JamState.PrivilegedServiceIndices.Kai_m = x.p.M
	s.JamState.PrivilegedServiceIndices.Kai_a = x.p.A
	s.JamState.PrivilegedServiceIndices.Kai_v = x.p.V


	// c - Designate => AuthorizationQueue
	for i := 0; i < types.TotalCores; i++ {
		copy(s.JamState.AuthorizationQueue[i], x.c[i][:])
	}
	// v - Assign => DesignatedValidators

	fmt.Printf("AAA=%v\n", s.JamState)
	fmt.Printf("BBB=%v\n", s.JamState.SafroleState)
	fmt.Printf("CCC=%v\n", s.JamState.SafroleState.DesignedValidators)
	//fmt.Printf("DDD=%v\n", s.JamState)

	s.JamState.SafroleState.DesignedValidators = x.v // []types.Validator `json:"designed_validators"`
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
	return types.ServiceAccountFromBytes(serviceBytes)
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
	tree := s.GetTrie()
	storage, err := tree.GetServiceStorage(service, k)
	if err != nil {
		return nil
	} else {
		fmt.Printf("get value=%x, err=%v\n", storage, err)
		return storage
	}
}

func (s *StateDB) WriteServiceStorage(service uint32, k []byte, storage []byte) {
	tree := s.GetTrie()
	tree.SetServiceStorage(service, k, storage)
}

func (s *StateDB) ReadServicePreimageBlob(service uint32, blob_hash common.Hash) []byte {
	tree := s.GetTrie()
	blob, err := tree.GetPreImageBlob(service, blob_hash.Bytes())
	if err != nil {
		return nil
	} else {
		fmt.Printf("get value=%x, err=%v\n", blob, err)
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
		fmt.Printf("get value=%x, err=%v\n", time_slots, err)
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
	s.X.p = p
	return OK
}

func (s *StateDB) designate(designate *types.Designate) uint32 {
	for i := 0; i < types.TotalValidators; i++ {
		s.X.v[i], _ = types.ValidatorFromBytes(designate.V[i*176 : (i+1)*176])
	}
	return OK
}

func (s *StateDB) assign(assign *types.Assign) uint32 {
	for i := 0; i < types.MaxAuthorizationQueueItems; i++ {
		s.X.c[assign.Core][i] = common.BytesToHash(assign.C[i*32 : (i+1)*32])
	}
	return OK
}

func (s *StateDB) newService(newService *types.NewService) uint32 {
	// ServiceAccount represents a service account.
	a := &types.ServiceAccount{
		CodeHash:  common.BytesToHash(newService.C),
		Balance:   newService.B,
		GasLimitG: newService.G,
		GasLimitM: newService.M,
	}
	i := newService.I
	s.X.n[i] = a
	return OK
}

func (s *StateDB) upgradeService(upgrade *types.UpgradeService) uint32 {
	a, ok := s.X.n[s.S]
	if ok {
		a.CodeHash = common.BytesToHash(upgrade.C)
		a.GasLimitG = upgrade.G
		a.GasLimitM = upgrade.M
	}
	return OK
}

func (s *StateDB) addTransfer(t *types.AddTransfer) uint32 {
	s.X.t = append(s.X.t, t)
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
	case types.NewService:
		return s.newService(&v)
	case types.UpgradeService:
		return s.upgradeService(&v)
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
		return x.s
	case x_c:
		return x.c
	case x_v:
		return x.v
	case x_i:
		return x.i
	case x_t:
		return x.t
	case x_n:
		return x.n
	case x_p:
		return x.p
	default:
		panic(0)
	}
	return 0
}
