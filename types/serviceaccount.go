package types

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"

	"slices"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
)

const (
	ServiceAccountPrefix = 255
	module               = "types"
)

const (
	EntryPointRefine        = 0
	EntryPointAccumulate    = 5
	EntryPointOnTransfer    = 10
	EntryPointAuthorization = 0
	EntryPointGeneric       = 255
)

// ServiceAccount represents a service account.
type ServiceAccount struct {
	ServiceIndex    uint32      `json:"service_index"`
	CodeHash        common.Hash `json:"code_hash"`    //a_c - account code hash c
	Balance         uint64      `json:"balance"`      //a_b - account balance b, which must be greater than a_t (The threshold needed in terms of its storage footprint)
	GasLimitG       uint64      `json:"min_item_gas"` //a_g - the minimum gas required in order to execute the Accumulate entry-point of the service's code,
	GasLimitM       uint64      `json:"min_memo_gas"` //a_m - the minimum required for the On Transfer entry-point.
	StorageSize     uint64      `json:"code_size"`    //a_l - total number of octets used in storage (9.3)
	NumStorageItems uint32      `json:"items"`        //a_i - the number of items in storage (9.3)

	// X.D will have Mutable=false, but X.D.U will have Mutable=True
	Mutable      bool `json:"-"`
	NewAccount   bool `json:"-"`
	Checkpointed bool `json:"-"`
	Dirty        bool `json:"-"`

	Storage  map[common.Hash]*StorageObject  `json:"-"` // arbitrary_k -> v. if v=[]byte. use as delete
	Lookup   map[common.Hash]*LookupObject   `json:"-"` // (h,l) -> anchor
	Preimage map[common.Hash]*PreimageObject `json:"-"` // H(p)  -> p
}

func (s *ServiceAccount) Clone() *ServiceAccount {
	// Start by cloning primitive fields directly
	clone := ServiceAccount{
		ServiceIndex:    s.ServiceIndex,
		CodeHash:        s.CodeHash,
		Balance:         s.Balance,
		GasLimitG:       s.GasLimitG,
		GasLimitM:       s.GasLimitM,
		StorageSize:     s.StorageSize,
		NumStorageItems: s.NumStorageItems,
		Dirty:           s.Dirty,
		NewAccount:      s.NewAccount,
		Checkpointed:    s.Checkpointed,
		Mutable:         s.Mutable, // should ALLOW_MUTABLE explicitly... check??
	}

	clone.Storage = make(map[common.Hash]*StorageObject, len(s.Storage))
	for k, v := range s.Storage {
		clone.Storage[k] = v.Clone()
	}
	clone.Lookup = make(map[common.Hash]*LookupObject, len(s.Lookup))
	for k, v := range s.Lookup {
		clone.Lookup[k] = v.Clone()
	}
	clone.Preimage = make(map[common.Hash]*PreimageObject, len(s.Preimage))
	for k, v := range s.Preimage {
		clone.Preimage[k] = v.Clone()
	}

	return &clone
}

type StorageObject struct {
	Accessed bool
	Deleted  bool
	Dirty    bool
	Key      []byte      `json:"key"`    // k
	Value    []byte      `json:"value"`  // v
	RawKey   common.Hash `json:"rawkey"` // rawKey
}

func (o *StorageObject) String() string {
	return fmt.Sprintf("Value: [%x] Deleted: %v", o.Value, o.Deleted)
}
func (o *StorageObject) Clone() *StorageObject {
	// Deep copy the Key+Value slice
	keyCopy := make([]byte, len(o.Key))
	copy(keyCopy, o.Key)
	valueCopy := make([]byte, len(o.Value))
	copy(valueCopy, o.Value)

	return &StorageObject{
		Accessed: o.Accessed,
		Deleted:  o.Deleted,
		Dirty:    o.Dirty,
		Key:      keyCopy,
		Value:    valueCopy,
		RawKey:   o.RawKey,
	}
}

type LookupObject struct {
	Accessed bool
	Deleted  bool
	Dirty    bool
	Z        uint32   `json:"z"` // z
	T        []uint32 `json:"t"` // t
}

func (o *LookupObject) Clone() *LookupObject {
	// Deep copy the T slice
	tCopy := make([]uint32, len(o.T))
	copy(tCopy, o.T)

	return &LookupObject{
		Deleted: o.Deleted,
		Dirty:   o.Dirty,
		Z:       o.Z,
		T:       tCopy,
	}
}

type PreimageObject struct {
	Accessed bool
	Deleted  bool
	Dirty    bool
	Preimage []byte `json:"preimage"` // p
}

func (o *PreimageObject) Clone() *PreimageObject {
	// Deep copy the Preimage slice
	preimageCopy := make([]byte, len(o.Preimage))
	copy(preimageCopy, o.Preimage)

	return &PreimageObject{
		Accessed: o.Accessed,
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
	acct.ServiceIndex = service_index
	return &acct, nil
}

func (s *ServiceAccount) GetServiceIndex() uint32 {
	return s.ServiceIndex
}

func (s *ServiceAccount) ALLOW_MUTABLE() {
	//fmt.Printf("ALLOW_MUTABLE: %v!!!\n", s.ServiceIndex)
	s.Mutable = true
}

func ServiceAccountFromBytes(service_index uint32, state_data []byte) (*ServiceAccount, error) {
	// Convert the internal state from bytes
	acctState, err := AccountStateFromBytes(service_index, state_data)
	if err != nil {
		return nil, err
	}
	// Initialize a new ServiceAccount and copy the relevant fields
	serviceAccount := &ServiceAccount{
		ServiceIndex:    service_index,
		CodeHash:        acctState.CodeHash,
		Balance:         acctState.Balance,
		GasLimitG:       acctState.GasLimitG,
		GasLimitM:       acctState.GasLimitM,
		StorageSize:     acctState.StorageSize,
		NumStorageItems: acctState.NumStorageItems,
		Mutable:         false, // THIS THE DEFAULT
		// Initialize maps to avoid nil reference errors
		Storage:  make(map[common.Hash]*StorageObject),
		Lookup:   make(map[common.Hash]*LookupObject),
		Preimage: make(map[common.Hash]*PreimageObject),
	}
	return serviceAccount, nil
}

// Convert the ServiceAccount to a human-readable string.
func (s *ServiceAccount) String() string {
	// Initial account information
	str := fmt.Sprintf("ServiceAccount %d CodeHash: %v B=%d (%x), G=%v M=%v L=%v, I=%v [Mutable: %v]\n",
		s.ServiceIndex, s.CodeHash.Hex(), s.Balance, s.Balance, s.GasLimitG, s.GasLimitM, s.StorageSize, s.NumStorageItems, s.Mutable)

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
		str4 += fmt.Sprintf("  Storage: %v => %v\n", h, lo.String())
	}
	return str + str2 + str3 + str4
}

func (s *ServiceAccount) JsonString() string {
	return ToJSON(s)
}
func (s *ServiceAccount) ReadStorage(mu_k []byte, rawK common.Hash, sdb HostEnv) (ok bool, v []byte) {
	serviceIndex := s.ServiceIndex
	hk := common.Compute_storageKey_internal(rawK)
	storageObj, ok := s.Storage[hk]
	if ok {
		fmt.Printf("ReadStorage hk=%s FOUND\n", hk)
		if storageObj.Deleted {
			fmt.Printf("ReadStorage hk=%s DEL\n", hk)
			return false, nil
		}
		return true, storageObj.Value
	}
	var err error
	v, ok, err = sdb.ReadServiceStorage(serviceIndex, rawK)
	if err != nil || !ok {
		return false, nil
	}
	fmt.Printf("ReadStorage hk=%s NEW STORAGEOBJ v=%x\n", hk, v)
	return true, v
}

func (s *ServiceAccount) ReadPreimage(blobHash common.Hash, sdb HostEnv) (ok bool, preimage []byte) {
	if s.Preimage == nil {
		s.Preimage = make(map[common.Hash]*PreimageObject)
	}

	preimageObj, ok := s.Preimage[blobHash]
	if !ok {
		var err error
		preimage, ok, err = sdb.ReadServicePreimageBlob(s.GetServiceIndex(), blobHash)
		if err != nil || !ok {
			return false, preimage
		}
		if s.Preimage == nil {
			s.Preimage = make(map[common.Hash]*PreimageObject)
		}
		s.Preimage[blobHash] = &PreimageObject{
			Accessed: true,
			Dirty:    false,
			Preimage: preimage,
		}
		return true, preimage
	}
	if preimageObj.Deleted {
		return false, nil
	}
	return true, preimageObj.Preimage
}

func (s *ServiceAccount) ReadLookup(blobHash common.Hash, z uint32, sdb HostEnv) (ok bool, anchor_timeslot []uint32) {
	lookupObj, ok := s.Lookup[blobHash]
	if lookupObj.Deleted {
		return false, []uint32{}
	}
	if !ok {
		var err error
		anchor_timeslot, ok, err = sdb.ReadServicePreimageLookup(s.GetServiceIndex(), blobHash, z)
		if err != nil || !ok {
			return ok, anchor_timeslot
		}
		s.Lookup[blobHash] = &LookupObject{
			Accessed: true,
			Dirty:    false,
			Z:        z,
			T:        anchor_timeslot,
		}
		return true, anchor_timeslot
	}
	return true, lookupObj.T
}

// a_c - account code hash c
func (s *ServiceAccount) SetCodeHash(codeHash common.Hash) {
	if s.Mutable == false {
		log.Crit(module, "SetCodeHash")
	}
	s.Dirty = true
	s.CodeHash = codeHash
}

// a_b - account balance b, which must be greater than a_t (The threshold needed in terms of its storage footprint)
// Note this could be IncBalance DecBalance instead?
func (s *ServiceAccount) DecBalance(balance uint64) {
	if s.Mutable == false {
		log.Crit(module, "SetBalance")
	}
	s.Dirty = true
	s.Balance -= balance
}
func (s *ServiceAccount) IncBalance(balance uint64) {
	if s.Mutable == false {
		log.Crit(module, "SetBalance")
	}
	s.Dirty = true
	s.Balance += balance
}

// a_g - the minimum gas required in order to execute the Accumulate entry-point of the service's code,
func (s *ServiceAccount) SetGasLimitG(g uint64) {
	if s.Mutable == false {
		log.Crit(module, "SetGasLimitG")
	}
	s.Dirty = true
	s.GasLimitG = g
}

// a_m - the minimum required for the On Transfer entry-point.
func (s *ServiceAccount) SetGasLimitM(g uint64) {
	if s.Mutable == false {
		log.Crit(module, "SetGasLimitM")
	}
	s.Dirty = true
	s.GasLimitM = g
}

// a_l - total number of octets used in storage (9.3)
func (s *ServiceAccount) SetStorageSize(storageSize uint64) {
	if s.Mutable == false {
		log.Crit(module, "SetStorageSize")
	}
	s.Dirty = true
	s.StorageSize = storageSize
}

// a_i - the number of items in storage (9.3)
func (s *ServiceAccount) SetNumStorageItems(numStorageItems uint32) {
	if s.Mutable == false {
		log.Crit(module, "SetNumStorageItems")
	}
	s.Dirty = true
	s.NumStorageItems = numStorageItems
}

func (s *ServiceAccount) WriteStorage(serviceIndex uint32, mu_k []byte, rawK common.Hash, val []byte) {
	log.Info("serviceAccount", "WriteStorage", "serviceIndex", serviceIndex, "mu_k", fmt.Sprintf("%x", mu_k), "rawK", rawK.Hex(), "val", fmt.Sprintf("%x", val))
	if s.Mutable == false {
		log.Crit(log.PvmAuthoring, "WriteStorage Mutable Err: Called WriteStorage on immutable ServiceAccount", "serviceIndex", serviceIndex, "mu_k", fmt.Sprintf("%x", mu_k), "rawK", rawK.Hex(), "val", fmt.Sprintf("%x", val))
	}

	hk := common.Compute_storageKey_internal(rawK)
	s.Dirty = true

	storeObj, exists := s.Storage[hk]
	if exists {
		storeObj.Accessed = true
		storeObj.Dirty = true
		storeObj.Deleted = (len(val) == 0)
		storeObj.Key = slices.Clone(mu_k)  // copy
		storeObj.Value = slices.Clone(val) // copy
		storeObj.RawKey = rawK
		fmt.Printf("HOSTWRITE: EXISTS %s\n", s.String())
	} else {
		s.Storage[hk] = &StorageObject{
			Accessed: false,
			Dirty:    true,
			Deleted:  len(val) == 0,
			Key:      slices.Clone(mu_k), // copy of mu_k
			Value:    slices.Clone(val),  // copy of val
			RawKey:   rawK,
		}
		fmt.Printf("HOSTWRITE: NEWLY %s %v\n", s.String(), s.Storage)
	}
}

func (s *ServiceAccount) WritePreimage(blobHash common.Hash, preimage []byte) {
	if s.Mutable == false {
		log.Crit(module, "Called WriteStorage on immutable ServiceAccount")
	}

	o, exists := s.Preimage[blobHash]
	if exists {
		s.Preimage[blobHash] = &PreimageObject{
			Accessed: o.Accessed,
			Dirty:    true,
			Deleted:  len(preimage) == 0,
			Preimage: preimage,
		}
	} else {
		s.Preimage[blobHash] = &PreimageObject{
			Accessed: false,
			Dirty:    true,
			Deleted:  len(preimage) == 0,
			Preimage: preimage,
		}
	}
	s.Dirty = true
}

func (s *ServiceAccount) WriteLookup(blobHash common.Hash, z uint32, time_slots []uint32) {
	if s.Mutable == false {
		log.Crit(module, "Called WriteStorage on immutable ServiceAccount")

	}
	s.Dirty = true
	o, exists := s.Lookup[blobHash]
	if exists {
		s.Lookup[blobHash] = &LookupObject{
			Accessed: o.Accessed,
			Dirty:    true,
			Deleted:  time_slots == nil,
			Z:        z,
			T:        time_slots,
		}
		return
	}
	s.Lookup[blobHash] = &LookupObject{
		Accessed: false,
		Dirty:    true,
		Deleted:  time_slots == nil,
		Z:        z,
		T:        time_slots,
	}
}

func (s *ServiceAccount) ComputeThreshold() uint64 {
	return BaseServiceBalance + MinElectiveServiceItemBalance*uint64(s.NumStorageItems) + MinElectiveServiceOctetBalance*s.StorageSize
}
func (s *ServiceAccount) MarshalJSON() ([]byte, error) {
	type Alias ServiceAccount
	return json.Marshal(&struct {
		*Alias
		CodeHash string                     `json:"code_hash"`
		Storage  map[string]*StorageObject  `json:"storage"`
		Lookup   map[string]*LookupObject   `json:"lookup"`
		Preimage map[string]*PreimageObject `json:"preimage"`
	}{
		Alias:    (*Alias)(s),
		CodeHash: s.CodeHash.Hex(),
		Storage:  convertHashMapToStringMap(s.Storage),
		Lookup:   convertHashMapToStringMap(s.Lookup),
		Preimage: convertHashMapToStringMap(s.Preimage),
	})
}

func (s *ServiceAccount) UnmarshalJSON(data []byte) error {
	type Alias ServiceAccount
	aux := &struct {
		*Alias
		CodeHash string                     `json:"code_hash"`
		Storage  map[string]*StorageObject  `json:"storage"`
		Lookup   map[string]*LookupObject   `json:"lookup"`
		Preimage map[string]*PreimageObject `json:"preimage"`
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	s.CodeHash = common.HexToHash(aux.CodeHash)
	s.Storage = convertStringMapToHashMap(aux.Storage)
	s.Lookup = convertStringMapToHashMap(aux.Lookup)
	s.Preimage = convertStringMapToHashMap(aux.Preimage)
	return nil
}

func convertHashMapToStringMap[T any](input map[common.Hash]*T) map[string]*T {
	output := make(map[string]*T, len(input))
	for k, v := range input {
		output[k.Hex()] = v
	}
	return output
}

func convertStringMapToHashMap[T any](input map[string]*T) map[common.Hash]*T {
	output := make(map[common.Hash]*T, len(input))
	for k, v := range input {
		output[common.HexToHash(k)] = v
	}
	return output
}

func CombineMetadataAndCode(service_name string, code []byte) []byte {
	service_name_bytes := []byte(service_name)
	service_name_length_bytes := E(uint64(len(service_name_bytes)))
	return append(service_name_length_bytes, append(service_name_bytes, code...)...)
}

func SplitMetadataAndCode(data []byte) (service_name string, code []byte) {
	metadata_length_byte, remaining := extractBytes(data)
	metadata_length, _ := DecodeE(metadata_length_byte)

	if uint64(len(remaining)) > metadata_length { // metadata_length < 32 &&
		code = remaining[metadata_length:]
		service_name = string(remaining[:metadata_length])
	} else {
		service_name = "unknown"
		code = data
	}
	return
}

func ReadCodeWithMetadata(fp string, metadata string) ([]byte, error) {
	fp = common.GetFilePath(fp)
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		return nil, err
	}
	return CombineMetadataAndCode(metadata, raw_code), nil
}
