package statedb

import (
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

const OK uint32 = 0

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
	tree := s.GetTrie()
	tree.SetPreImageLookup(service, blob_hash, blob_length, time_slots)
}

/* Does this make sense?
func (nh *HostEnv) WriteServicePreimageBlob(s uint32, blob []byte) bool {
	t := nh.GetTrie()
	t.SetPreImageBlob(s, blob)
	return true
}
*/

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
func (s *StateDB) empower(e types.Empower) uint32 {
	return OK
}

func (s *StateDB) designate(e types.Designate) uint32 {
	return OK
}

func (s *StateDB) assign(e types.Assign) uint32 {
	return OK
}

func (s *StateDB) newService(e types.NewService) uint32 {
	return OK
}

func (s *StateDB) upgradeService(e types.UpgradeService) uint32 {
	return OK
}

func (s *StateDB) addTransfer(e types.AddTransfer) uint32 {
	return OK
}
func (s *StateDB) SetX(obj interface{}) uint32 {
	switch v := obj.(type) {
	case types.Empower:
		return s.empower(v)
	case types.Designate:
		return s.designate(v)
	case types.Assign:
		return s.assign(v)
	case types.NewService:
		return s.newService(v)
	case types.UpgradeService:
		return s.upgradeService(v)
	case types.AddTransfer:
		return s.addTransfer(v)
	default:
		panic(0)
	}
	return 0
}

func (s *StateDB) GetX(ctx string) interface{} {
	// TODO: for each possible value of ctx return some internal state variable of X
	switch ctx {
	case "p":
		break
	case "s":
		break
	case "c":
		break
	case "v":
		break
	case "i":
		break
	default:
		panic(0)
	}
	return 0
}

func uint32ToBytes(s uint32) []byte {
	sbytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(sbytes, s)
	return sbytes
}
