package types

import (
	//"encoding/hex"
	//"encoding/json"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

const (
	ServiceAccountPrefix = 255
)

const (
	EntryPointGeneric    = ""
	EntryPointRefine     = "refine"
	EntryPointAccumulate = "accumulate"
	EntryPointOnTransfer = "on_transfer"
	EntryPointAuthorize  = "authorization"
)

const (
	PreimageRecordType          = "Preimage"
	LookupRecordType            = "Lookup"
	PreimageANDLookupRecordType = "PL"
	StorageRecordType           = "Storage"
	UnknownRecordType           = "Unknown"
	JournalOPWrite              = "WRITE"
	JournalOPDelete             = "DELETE"
	JournalOPRead               = "READ"
	JournalOPNOTInitiated       = "NONE"
)

// ServiceAccount represents a service account.
type AccountState struct {
	serviceIndex    uint32      //a_idx - account idx helpful for identifying xs
	CodeHash        common.Hash `json:"code_hash"`         //a_c - account code hash c
	Balance         uint64      `json:"balance"`           //a_b - account balance b, which must be greater than a_t (The threshold needed in terms of its storage footprint)
	GasLimitG       uint64      `json:"gas_limit_g"`       //a_g - the minimum gas required in order to execute the Accumulate entry-point of the service's code,
	GasLimitM       uint64      `json:"gas_limit_m"`       //a_m - the minimum required for the On Transfer entry-point.
	StorageSize     uint64      `json:"storage_size"`      //a_l - total number of octets used in storage (9.3)
	NumStorageItems uint32      `json:"num_storage_items"` //a_i - the number of items in storage (9.3)
}

type ServiceAccount struct {
	//account state portion
	serviceIndex    uint32
	CodeHash        common.Hash `json:"code_hash"`
	Balance         uint64      `json:"balance"`
	GasLimitG       uint64      `json:"gas_limit_g"`
	GasLimitM       uint64      `json:"gas_limit_m"`
	StorageSize     uint64      `json:"storage_size"`      //a_l - total number of octets used in storage (9.3)
	NumStorageItems uint32      `json:"num_storage_items"` //a_i - the number of items in storage (9.3)

	// journal portion
	Journals []Journal
	Storage  map[string][]byte   `json:"s_map"`  // arbitrary_k -> v. if v=[]byte. use as delete
	Lookup   map[string][]uint32 `json:"l_map"`  // (h,l) -> anchor
	Preimage map[string][]byte   `json:"p"`      // H(p)  -> p
	Delete   map[string]string   `json:"delete"` // (key) -> type, if (h,l) If present, delete a_l & a_p
	Exist    map[string]string   `json:"exist"`  // key is effectively touched. this is essential to switch between bpt vs lookup here
}

type Journal struct {
	OP      string
	KeyType string
	Key     string
	Obj     interface{}
}

type JournalStorageKV struct {
	K []byte
	V []byte
}

type JournalLookupKV struct {
	H common.Hash
	Z uint32
	T []uint32
}

type JournalPreimageKV struct {
	H common.Hash
	P []byte
}

func (j *Journal) GetJournalRecordType() (string, string, interface{}) {
	op := j.OP
	obj := j.Obj
	switch obj.(type) { // Type assertion to check the underlying type of Obj
	case JournalStorageKV:
		return op, StorageRecordType, obj
	case JournalLookupKV:
		return op, LookupRecordType, obj
	case JournalPreimageKV:
		return op, PreimageRecordType, obj
	default:
		return op, UnknownRecordType, obj
	}
}

// Convert the ServiceAccount to a byte slice.
// ac ⌢ E8(ab,ag,am,al) ⌢ E4(ai)
// 32 + 8*4 + 4 = 68
// TODO: Need codec E here

// Bytes encodes the AccountState as a byte slice
func (s *AccountState) Bytes() ([]byte, error) {
	var buf bytes.Buffer

	if _, err := buf.Write(s.CodeHash.Bytes()); err != nil {
		return nil, err
	}
	writeUint64 := func(value uint64) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}

	writeUint32 := func(value uint32) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}

	if err := writeUint64(s.Balance); err != nil {
		return nil, err
	}
	if err := writeUint64(s.GasLimitG); err != nil {
		return nil, err
	}
	if err := writeUint64(s.GasLimitM); err != nil {
		return nil, err
	}
	if err := writeUint64(s.StorageSize); err != nil {
		return nil, err
	}
	if err := writeUint32(s.NumStorageItems); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Recover reconstructs an AccountState from a byte slice
func (s *AccountState) Recover(data []byte) error {
	// Ensure the length of the data is correct
	expectedLen := 32 + 8*4 + 4 // 32 bytes for CodeHash, 4 * 8 bytes for uint64, 4 bytes for uint32
	if len(data) != expectedLen {
		return fmt.Errorf("invalid data length: expected %d, got %d", expectedLen, len(data))
	}

	// Create a reader from the data
	buf := bytes.NewReader(data)

	// Read CodeHash (32 bytes)
	codeHashBytes := make([]byte, 32)
	if _, err := buf.Read(codeHashBytes); err != nil {
		return err
	}
	s.CodeHash = common.BytesToHash(codeHashBytes)

	readUint64 := func() (uint64, error) {
		var value uint64
		err := binary.Read(buf, binary.LittleEndian, &value)
		return value, err
	}
	readUint32 := func() (uint32, error) {
		var value uint32
		err := binary.Read(buf, binary.LittleEndian, &value)
		return value, err
	}
	var err error
	if s.Balance, err = readUint64(); err != nil {
		return err
	}
	if s.GasLimitG, err = readUint64(); err != nil {
		return err
	}
	if s.GasLimitM, err = readUint64(); err != nil {
		return err
	}
	if s.StorageSize, err = readUint64(); err != nil {
		return err
	}
	if s.NumStorageItems, err = readUint32(); err != nil {
		return err
	}

	return nil
}

func AccountStateFromBytes(service_index uint32, data []byte) (*AccountState, error) {
	acct := AccountState{}
	if err := acct.Recover(data); err != nil {
		fmt.Println("Error recovering:", err)
		return nil, err
	}
	acct.SetServiceIndex(service_index)
	return &acct, nil
}

func (s *AccountState) ServiceIndex() uint32 {
	return s.serviceIndex
}

func (s *AccountState) SetServiceIndex(service_index uint32) {
	s.serviceIndex = service_index
}

func ServiceAccountFromBytes(service_index uint32, state_data []byte) (*ServiceAccount, error) {
	// Convert the internal state from bytes
	acctState, err := AccountStateFromBytes(service_index, state_data)
	if err != nil {
		return nil, err
	}
	// Initialize a new ServiceAccount and copy the relevant fields
	serviceAccount := &ServiceAccount{
		serviceIndex:    service_index,
		CodeHash:        acctState.CodeHash,
		Balance:         acctState.Balance,
		GasLimitG:       acctState.GasLimitG,
		GasLimitM:       acctState.GasLimitM,
		StorageSize:     acctState.StorageSize,
		NumStorageItems: acctState.NumStorageItems,

		// Initialize maps to avoid nil reference errors
		Journals: make([]Journal, 0),
		Storage:  make(map[string][]byte),
		Lookup:   make(map[string][]uint32),
		Preimage: make(map[string][]byte),
		Delete:   make(map[string]string),
		Exist:    make(map[string]string),
	}
	return serviceAccount, nil
}

func (s *ServiceAccount) AccountState() *AccountState {
	// Create a new AccountState and copy relevant fields
	return &AccountState{
		serviceIndex:    s.serviceIndex,
		CodeHash:        s.CodeHash,
		Balance:         s.Balance,
		GasLimitG:       s.GasLimitG,
		GasLimitM:       s.GasLimitM,
		StorageSize:     s.StorageSize,
		NumStorageItems: s.NumStorageItems,
	}
}

func (s *ServiceAccount) ServiceIndex() uint32 {
	return s.serviceIndex
}

func (s *ServiceAccount) SetServiceIndex(service_index uint32) {
	s.serviceIndex = service_index
}

func (s *ServiceAccount) Bytes() ([]byte, error) {
	// get the accountState portion only
	return s.AccountState().Bytes()
}

// Convert the ServiceAccount to a human-readable string.
func (s *ServiceAccount) String() string {
	str := fmt.Sprintf("ServiceAccount[i=%v] {CodeHash: %v, Balance: %d, GasLimitG: %d, GasLimitM: %d, StorageSize: %d, NumStorageItems: %d}, Map{}",
		s.serviceIndex, s.CodeHash.Hex(), s.Balance, s.GasLimitG, s.GasLimitM, s.StorageSize, s.NumStorageItems)
	return str
}

func (s *ServiceAccount) AddJournal(op string, key_type string, key string, obj interface{}) {
	j := Journal{
		OP:      op,
		KeyType: key_type,
		Key:     key,
		Obj:     obj,
	}
	s.MarkDirty(key, key_type) // IMPORTANT: must mark dirty here..
	s.Journals = append(s.Journals, j)
}

func (s *ServiceAccount) IsMarkedAsDelete(k string) bool {
	// Check if the key exists in the Delete map
	if _, exists := s.Delete[k]; exists {
		return true
	}
	return false
}

func (s *ServiceAccount) OverwriteDelete(k string) {
	if s.IsMarkedAsDelete(k) {
		delete(s.Delete, k)
	}
}

func (s *ServiceAccount) OverwriteJournalMemory(k string, key_type string) {
	isDirty, _ := s.IsDirty(k)
	if isDirty {
		switch key_type {
		case StorageRecordType:
			delete(s.Storage, k)
		case LookupRecordType:
			delete(s.Lookup, k)
		case PreimageRecordType:
			delete(s.Preimage, k)
		}
	}
}

func (s *ServiceAccount) IsDirty(k string) (bool, string) {
	// Check if the key exists in the `Exist` map and is marked as `true`
	if op_type, exists := s.Exist[k]; exists {
		return true, op_type
	}
	return false, JournalOPNOTInitiated
}

func (s *ServiceAccount) MarkDirty(k string, op_type string) {
	s.Exist[k] = op_type
}

func (s *ServiceAccount) JournalGetStorage(key []byte) (bool, error, []byte) {
	k := common.Bytes2Hex(key)
	isDirty, op_type := s.IsDirty(k) // get last rec
	if isDirty {
		//should be return via journal mem
		if op_type == JournalOPDelete {
			// has been marked as delete - should have save not found behavior as if we are calling bpt
			return true, nil, nil
		} else if op_type == JournalOPWrite {
			// return value from jornal map
			v, found := s.Storage[k]
			if !found {
				// shouldn't be here
				panic(0)
			}
			return true, nil, v
		}
	}
	// NOT found in memory, require bpt lookup
	return false, nil, nil
}

func (s *ServiceAccount) JournalGetPreimage(blobHash common.Hash) (bool, error, []byte) {
	k := blobHash.Hex()
	isDirty, op_type := s.IsDirty(k) // get last rec
	if isDirty {
		//should be return via journal mem
		if op_type == JournalOPDelete {
			// has been marked as delete - should have save not found behavior as if we are calling bpt
			return true, nil, nil
		} else if op_type == JournalOPWrite {
			// return value from jornal map
			blob, found := s.Preimage[k]
			if !found {
				// shouldn't be here
				panic(0)
			}
			return true, nil, blob
		}
	}
	// NOT found in memory, require bpt lookup
	return false, nil, nil
}

func (s *ServiceAccount) JournalGetLookup(blobHash common.Hash, z uint32) (bool, error, []uint32) {
	k := fmt.Sprintf("%v_%v", blobHash, z)
	isDirty, op_type := s.IsDirty(k) // get last rec
	if isDirty {
		//should be return via journal mem
		if op_type == JournalOPDelete {
			// has been marked as delete - should have save not found behavior as if we are calling bpt
			return true, nil, nil
		} else if op_type == JournalOPWrite {
			// return value from jornal map
			anchor_timeslot, found := s.Lookup[k]
			if !found {
				// shouldn't be here
				panic(0)
			}
			return true, nil, anchor_timeslot
		}
	}
	// NOT found in memory, require bpt lookup
	return false, nil, []uint32{}
}

func (s *ServiceAccount) JournalInsertStorage(key []byte, val []byte) {
	k := common.Bytes2Hex(key)
	s.AddJournal(JournalOPWrite, StorageRecordType, k, JournalStorageKV{K: key, V: val})
	s.Storage[k] = val
	s.OverwriteDelete(k)
}

func (s *ServiceAccount) JournalDeleteStorage(key []byte) {
	k := common.Bytes2Hex(key)
	s.OverwriteJournalMemory(k, StorageRecordType)
	s.AddJournal(JournalOPDelete, StorageRecordType, k, JournalStorageKV{K: key})
	s.Delete[k] = StorageRecordType
}

func (s *ServiceAccount) JournalInsertPreimage(blobHash common.Hash, blob []byte) {
	k := blobHash.Hex()
	s.AddJournal(JournalOPWrite, PreimageRecordType, k, JournalPreimageKV{H: blobHash, P: blob})
	s.Preimage[blobHash.Hex()] = blob
	s.OverwriteDelete(k)
}

func (s *ServiceAccount) JournalDeletePreimage(blobHash common.Hash) {
	k := blobHash.Hex()
	s.OverwriteJournalMemory(k, PreimageRecordType)
	s.AddJournal(JournalOPDelete, PreimageRecordType, k, JournalPreimageKV{H: blobHash})
	s.Delete[k] = PreimageRecordType
}

func (s *ServiceAccount) JournalInsertLookup(blobHash common.Hash, z uint32, time_slots []uint32) {
	// key is 32 ++ 4 byte length
	k := fmt.Sprintf("%v_%v", blobHash, z)
	s.AddJournal(JournalOPWrite, LookupRecordType, k, JournalLookupKV{H: blobHash, Z: z, T: time_slots})
	s.Lookup[k] = time_slots
	s.OverwriteDelete(k)
}

func (s *ServiceAccount) JournalDeleteLookup(blobHash common.Hash, z uint32) {
	// key is 32 ++ 4 byte length
	k := fmt.Sprintf("%v_%v", blobHash, z)
	s.OverwriteJournalMemory(k, LookupRecordType)
	s.AddJournal(JournalOPDelete, LookupRecordType, k, JournalLookupKV{H: blobHash})
	s.Delete[k] = LookupRecordType
}

func (s *ServiceAccount) JournalDeletePreimageAndLookup(blobHash common.Hash, z uint32) {
	s.JournalDeleteLookup(blobHash, z)
	s.JournalDeletePreimage(blobHash)
}

func (s *ServiceAccount) GetJournals() []Journal {
	return s.Journals
}

// eq 95
func (s *ServiceAccount) ComputeThreshold() uint64 {
	//BS +BI ⋅ai +BL ⋅al
	account_threshold := BaseServiceBalance + MinElectiveServiceItemBalance*uint64(s.NumStorageItems) + MinElectiveServiceOctetBalance*s.StorageSize
	return account_threshold
}
