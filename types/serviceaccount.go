package types

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

const (
	ServiceAccountPrefix = 255
)

const (
	EntryPointRefine        = 5
	EntryPointAccumulate    = 10
	EntryPointOnTransfer    = 15
	EntryPointAuthorization = 0
	EntryPointGeneric       = 255
)

/*

const (
	PreimageRecordType          = "Preimage"
	LookupRecordType            = "Lookup"
	PreimageANDLookupRecordType = "PL"
	StorageRecordType           = "Storage"
	UnknownRecordType           = "Unknown"
)
*/

// ServiceAccount represents a service account.
type ServiceAccount struct {
	serviceIndex    uint32
	CodeHash        common.Hash `json:"code_hash"`         //a_c - account code hash c
	Balance         uint64      `json:"balance"`           //a_b - account balance b, which must be greater than a_t (The threshold needed in terms of its storage footprint)
	GasLimitG       uint64      `json:"gas_limit_g"`       //a_g - the minimum gas required in order to execute the Accumulate entry-point of the service's code,
	GasLimitM       uint64      `json:"gas_limit_m"`       //a_m - the minimum required for the On Transfer entry-point.
	StorageSize     uint64      `json:"storage_size"`      //a_l - total number of octets used in storage (9.3)
	NumStorageItems uint32      `json:"num_storage_items"` //a_i - the number of items in storage (9.3)

	Dirty    bool
	Storage  map[common.Hash]StorageObject  `json:"s_map"` // arbitrary_k -> v. if v=[]byte. use as delete
	Lookup   map[common.Hash]LookupObject   `json:"l_map"` // (h,l) -> anchor
	Preimage map[common.Hash]PreimageObject `json:"p"`     // H(p)  -> p
}

func (s ServiceAccount) Clone() *ServiceAccount {
	// Start by cloning primitive fields directly
	clone := ServiceAccount{
		serviceIndex:    s.serviceIndex,
		CodeHash:        s.CodeHash,
		Balance:         s.Balance,
		GasLimitG:       s.GasLimitG,
		GasLimitM:       s.GasLimitM,
		StorageSize:     s.StorageSize,
		NumStorageItems: s.NumStorageItems,
		Dirty:           s.Dirty,
	}

	// Clone the Storage map
	clone.Storage = make(map[common.Hash]StorageObject, len(s.Storage))
	for k, v := range s.Storage {
		clone.Storage[k] = v.Clone() // Assuming StorageObject has a Clone method
	}

	// Clone the Lookup map
	clone.Lookup = make(map[common.Hash]LookupObject, len(s.Lookup))
	for k, v := range s.Lookup {
		clone.Lookup[k] = v.Clone() // Assuming LookupObject has a Clone method
	}

	// Clone the Preimage map
	clone.Preimage = make(map[common.Hash]PreimageObject, len(s.Preimage))
	for k, v := range s.Preimage {
		clone.Preimage[k] = v.Clone() // Assuming PreimageObject has a Clone method
	}

	return &clone
}

type StorageObject struct {
	Deleted bool
	Dirty   bool
	Value   []byte
}

func (o StorageObject) Clone() StorageObject {
	// Deep copy the Value slice
	valueCopy := make([]byte, len(o.Value))
	copy(valueCopy, o.Value)

	return StorageObject{
		Deleted: o.Deleted,
		Dirty:   o.Dirty,
		Value:   valueCopy,
	}
}

type LookupObject struct {
	Deleted bool
	Dirty   bool
	Z       uint32
	T       []uint32
}

func (o LookupObject) Clone() LookupObject {
	// Deep copy the T slice
	tCopy := make([]uint32, len(o.T))
	copy(tCopy, o.T)

	return LookupObject{
		Deleted: o.Deleted,
		Dirty:   o.Dirty,
		Z:       o.Z,
		T:       tCopy,
	}
}

type PreimageObject struct {
	Deleted  bool
	Dirty    bool
	Preimage []byte
}

func (o PreimageObject) Clone() PreimageObject {
	// Deep copy the Preimage slice
	preimageCopy := make([]byte, len(o.Preimage))
	copy(preimageCopy, o.Preimage)

	return PreimageObject{
		Deleted:  o.Deleted,
		Dirty:    o.Dirty,
		Preimage: preimageCopy,
	}
}

// Convert the ServiceAccount to a byte slice.
// ac ⌢ E8(ab,ag,am,al) ⌢ E4(ai)
// 32 + 8*4 + 4 = 68
// TODO: Need codec E here

// Bytes encodes the AccountState as a byte slice
func (s *ServiceAccount) Bytes() ([]byte, error) {
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
func (s *ServiceAccount) Recover(data []byte) error {
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

func AccountStateFromBytes(service_index uint32, data []byte) (*ServiceAccount, error) {
	acct := ServiceAccount{}
	if err := acct.Recover(data); err != nil {
		fmt.Println("Error recovering:", err)
		return nil, err
	}
	acct.SetServiceIndex(service_index)
	return &acct, nil
}

func (s *ServiceAccount) ServiceIndex() uint32 {
	return s.serviceIndex
}

func (s *ServiceAccount) SetServiceIndex(service_index uint32) {
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
		Storage:  make(map[common.Hash]StorageObject),
		Lookup:   make(map[common.Hash]LookupObject),
		Preimage: make(map[common.Hash]PreimageObject),
	}
	return serviceAccount, nil
}

// Convert the ServiceAccount to a human-readable string.
func (s *ServiceAccount) String() string {
	// Initial account information
	str := fmt.Sprintf("ServiceAccount %d CodeHash: %v\n",
		s.serviceIndex, s.CodeHash.Hex()) // s.Balance, s.GasLimitG, s.GasLimitM, s.StorageSize, s.NumStorageItems

	// Lookup entries
	str2 := ""
	for h, lo := range s.Lookup {
		str2 += fmt.Sprintf("  Lookup: %v => %v\n", h, lo)
	}

	// Preimage entries
	str3 := ""
	for h, lo := range s.Preimage {
		str3 += fmt.Sprintf("  Preimage: %v => %v\n", h, lo)
	}

	// Storage entries
	str4 := ""
	for h, lo := range s.Storage {
		str4 += fmt.Sprintf("  Storage: %v => %v\n", h, lo)
	}

	return str + str2 + str3 + str4
}
func (s *ServiceAccount) ReadStorage(key common.Hash, sdb HostEnv) (ok bool, v []byte) {
	storageObj, ok := s.Storage[key]
	if storageObj.Deleted {
		return false, nil
	}
	if !ok {
		var err error
		v = sdb.ReadServiceStorage(s.serviceIndex, key)
		if err != nil {
			return false, nil
		}
	}
	return true, storageObj.Value
}

func (s *ServiceAccount) ReadPreimage(blobHash common.Hash, sdb HostEnv) (ok bool, preimage []byte) {
	preimageObj, ok := s.Preimage[blobHash]
	if preimageObj.Deleted {
		return false, nil
	}
	if !ok {
		preimage = sdb.ReadServicePreimageBlob(s.ServiceIndex(), blobHash)
		s.Preimage[blobHash] = PreimageObject{
			Dirty:    false,
			Preimage: preimage,
		}
		return true, preimage
	}
	return true, preimageObj.Preimage
}

func (s *ServiceAccount) ReadLookup(blobHash common.Hash, z uint32, sdb HostEnv) (ok bool, anchor_timeslot []uint32) {
	lookupObj, ok := s.Lookup[blobHash]
	if lookupObj.Deleted {
		return false, []uint32{}
	}
	if !ok {
		anchor_timeslot = sdb.ReadServicePreimageLookup(s.ServiceIndex(), blobHash, z)
		s.Lookup[blobHash] = LookupObject{
			Dirty: false,
			Z:     z,
			T:     anchor_timeslot,
		}
		return true, anchor_timeslot
	}
	return true, lookupObj.T
}

func (s *ServiceAccount) WriteStorage(key common.Hash, val []byte) {
	s.Dirty = true
	s.Storage[key] = StorageObject{
		Dirty:   true,
		Deleted: len(val) == 0,
		Value:   val,
	}
}

func (s *ServiceAccount) WritePreimage(blobHash common.Hash, preimage []byte) {
	s.Dirty = true
	s.Preimage[blobHash] = PreimageObject{
		Dirty:    true,
		Deleted:  len(preimage) == 0,
		Preimage: preimage,
	}
}

func (s *ServiceAccount) WriteLookup(blobHash common.Hash, z uint32, time_slots []uint32) {
	s.Dirty = true
	s.Lookup[blobHash] = LookupObject{
		Dirty:   true,
		Deleted: false, // WAS len(time_slots) == 0,
		Z:       z,
		T:       time_slots,
	}
}

// eq 95
func (s *ServiceAccount) ComputeThreshold() uint64 {
	//BS +BI ⋅ai +BL ⋅al
	account_threshold := BaseServiceBalance + MinElectiveServiceItemBalance*uint64(s.NumStorageItems) + MinElectiveServiceOctetBalance*s.StorageSize
	return account_threshold
}
