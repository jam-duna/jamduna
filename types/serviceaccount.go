package types

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"

	"slices"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
)

const (
	ServiceAccountPrefix = 255
	module               = "types"
)

const (
	EntryPointRefine        = 0
	EntryPointAccumulate    = 5
	EntryPointAuthorization = 0
	EntryPointGeneric       = 255
)

// ServiceAccount represents a service account.
type ServiceAccount struct {
	ServiceIndex       uint32      `json:"service_index"`
	CodeHash           common.Hash `json:"code_hash"`           //a_c - account code hash c
	Balance            uint64      `json:"balance"`             //a_b - account balance b, which must be greater than a_t (The threshold needed in terms of its storage footprint)
	GasLimitG          uint64      `json:"min_item_gas"`        //a_g - the minimum gas required in order to execute the Accumulate entry-point of the service's code,
	GasLimitM          uint64      `json:"min_memo_gas"`        //a_m - the minimum gas required per deferred transfer.
	StorageSize        uint64      `json:"bytes"`               //a_o - total number of octets used in storage (9.3) -- renamed from code_size
	GratisOffset       uint64      `json:"gratis_offset"`       //a_f - the gratis storage offset
	NumStorageItems    uint32      `json:"items"`               //a_i - the number of items in storage (9.3)
	CreateTime         uint32      `json:"create_time"`         //a_r - the timeslot at creation. used for checkpointing
	RecentAccumulation uint32      `json:"recent_accumulation"` //a_a - the timeslot at the most recent accumulation, used for checkpointing
	ParentService      uint32      `json:"parent_service"`      //a_p - the parent service index

	// Mutable is used to determine if the account can be modified.
	// X.D will have Mutable=false, but X.D.U will have Mutable=True
	Mutable    bool `json:"-"`
	NewAccount bool `json:"-"`
	Dirty      bool `json:"-"`

	NumStorageItemsDirty bool `json:"-"`
	StorageSizeDirty     bool `json:"-"`

	DeletedAccount bool `json:"-"` // if true, this account is deleted

	Storage  map[string]*StorageObject  `json:"-"` // arbitrary_k -> v. if v=[]byte. use as delete
	Lookup   map[string]*LookupObject   `json:"-"` // (h,l) -> anchor
	Preimage map[string]*PreimageObject `json:"-"` // H(p)  -> p
}

func NewEmptyServiceAccount(serviceIndex uint32, c []byte, g uint64, m uint64, storageSize uint64, gratisOffset uint64, createdSlot uint32, parentService uint32) *ServiceAccount {
	a := &ServiceAccount{
		ServiceIndex:       serviceIndex,
		Mutable:            false,
		Dirty:              false,
		CodeHash:           common.BytesToHash(c),
		Balance:            0,
		GasLimitG:          g,
		GasLimitM:          m,
		StorageSize:        storageSize,   //a_l =  ∑ 81+z per (h,z) + ∑ 34 + |y| + |x|; Initialized for first a_l
		GratisOffset:       gratisOffset,  //a_f = 0
		NumStorageItems:    2,             //a_s = 2⋅∣al∣+∣as∣; Initialized for first a_l
		CreateTime:         createdSlot,   //a_r = t // check pre vs post?
		RecentAccumulation: 0,             //a_a = 0
		ParentService:      parentService, //a_p = x_s
		Storage:            make(map[string]*StorageObject),
		Lookup:             make(map[string]*LookupObject),
		Preimage:           make(map[string]*PreimageObject),
		NewAccount:         true, // with this flag, if an account is Dirty OR NewAccount then it is written

		StorageSizeDirty:     true,
		NumStorageItemsDirty: true,
	}
	return a
}

func (s *ServiceAccount) Clone() *ServiceAccount {
	// Start by cloning primitive fields directly
	clone := ServiceAccount{
		ServiceIndex:         s.ServiceIndex,
		CodeHash:             s.CodeHash,
		Balance:              s.Balance,
		GasLimitG:            s.GasLimitG,
		GasLimitM:            s.GasLimitM,
		StorageSize:          s.StorageSize,
		StorageSizeDirty:     s.StorageSizeDirty,
		GratisOffset:         s.GratisOffset,
		NumStorageItems:      s.NumStorageItems,
		NumStorageItemsDirty: s.NumStorageItemsDirty,
		CreateTime:           s.CreateTime,
		RecentAccumulation:   s.RecentAccumulation,
		ParentService:        s.ParentService,
		Dirty:                s.Dirty,
		NewAccount:           s.NewAccount,
		DeletedAccount:       s.DeletedAccount,
		Mutable:              s.Mutable, // should ALLOW_MUTABLE explicitly... check??
	}

	clone.Storage = make(map[string]*StorageObject, len(s.Storage))
	for k, v := range s.Storage {
		clone.Storage[k] = v.Clone()
	}
	clone.Lookup = make(map[string]*LookupObject, len(s.Lookup))
	for k, v := range s.Lookup {
		clone.Lookup[k] = v.Clone()
	}
	clone.Preimage = make(map[string]*PreimageObject, len(s.Preimage))
	for k, v := range s.Preimage {
		clone.Preimage[k] = v.Clone()
	}

	return &clone
}

type StorageObject struct {
	Accessed    bool
	Deleted     bool
	Dirty       bool
	Key         []byte `json:"key"`          // k
	Value       []byte `json:"value"`        // v
	InternalKey string `json:"internal_key"` // as_internal_key
	StorageKey  string `json:"storage_key"`  // c(s,h)
	Source      string `json:"-"`            // "trie" or "memory"
}

func (o *StorageObject) String() string {
	return fmt.Sprintf("Value: [%x] Deleted: %v Dirty: %v StorageKey: %v Key: %x Value: %x", o.Value, o.Deleted, o.Dirty, o.StorageKey, o.Key, o.Value)
}
func (o *StorageObject) Clone() *StorageObject {
	// Deep copy the Key+Value slice
	keyCopy := make([]byte, len(o.Key))
	copy(keyCopy, o.Key)
	valueCopy := make([]byte, len(o.Value))
	copy(valueCopy, o.Value)

	return &StorageObject{
		Accessed:    o.Accessed,
		Deleted:     o.Deleted,
		Dirty:       o.Dirty,
		Key:         keyCopy,
		Value:       valueCopy,
		InternalKey: o.InternalKey,
		StorageKey:  o.StorageKey,
		Source:      o.Source,
	}
}

type LookupObject struct {
	Accessed  bool
	Deleted   bool
	Dirty     bool
	Z         uint32   `json:"z"` // z
	Timeslots []uint32 `json:"t"` // t
	Source    string   `json:"-"` // "trie" or "memory"
}

func (o *LookupObject) String() string {
	return fmt.Sprintf("Z: [%v] Timeslots: %v Deleted: %v Dirty: %v", o.Z, o.Timeslots, o.Deleted, o.Dirty)
}

func (o *LookupObject) Clone() *LookupObject {
	// Deep copy the Timeslots slice
	tCopy := make([]uint32, len(o.Timeslots))
	copy(tCopy, o.Timeslots)

	return &LookupObject{
		Deleted:   o.Deleted,
		Dirty:     o.Dirty,
		Z:         o.Z,
		Timeslots: tCopy,
		Source:    o.Source,
	}
}

type PreimageObject struct {
	Accessed bool
	Deleted  bool
	Dirty    bool
	Preimage []byte `json:"preimage"` // p
	Source   string `json:"-"`        // "trie" or "memory"
}

func (o *PreimageObject) String() string {
	return fmt.Sprintf("P: %v  Deleted: %v Dirty: %v Source: %s", common.Blake2Hash(o.Preimage), o.Deleted, o.Dirty, o.Source)
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
		Source:   o.Source,
	}
}

// Convert the ServiceAccount to a byte slice.
// ac ++ E8(b,g,m,o,f) ++E4(i,r,a,p)
// 1 + 32 + 8*5 + 4*4 = 89
const ACCOUNT_VERSION = 0

// Bytes encodes the AccountState as a byte slice
func (s *ServiceAccount) Bytes() ([]byte, error) {
	var buf bytes.Buffer

	writeUint64 := func(value uint64) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}

	writeUint32 := func(value uint32) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}
	writeUint8 := func(value uint8) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}

	// 0.7.1 Includes version byte prefix for accounts version 0 byte https://graypaper.fluffylabs.dev/#/1c979cb/3b81033b8103?v=0.7.1
	if err := writeUint8(ACCOUNT_VERSION); err != nil {
		return nil, err
	}
	if _, err := buf.Write(s.CodeHash.Bytes()); err != nil {
		return nil, err
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
	if err := writeUint64(s.GratisOffset); err != nil {
		return nil, err
	}

	//[Gratis] E4(i,r,a,p) NumStorageItems, CreateTime, RecentAccumulation, ParentService
	if err := writeUint32(uint32(s.NumStorageItems)); err != nil {
		return nil, err
	}
	if err := writeUint32(uint32(s.CreateTime)); err != nil {
		return nil, err
	}
	if err := writeUint32(uint32(s.RecentAccumulation)); err != nil {
		return nil, err
	}
	if err := writeUint32(uint32(s.ParentService)); err != nil {
		return nil, err
	}
	debugService := false
	if debugService {
		service_account := common.ComputeC_is(s.ServiceIndex)
		stateKey := service_account.Bytes()
		storageSizeLE := make([]byte, 8)
		binary.LittleEndian.PutUint64(storageSizeLE, s.StorageSize)
		log.Info(log.SDB, "ServiceAccount Write", "Service", fmt.Sprintf("%d", s.ServiceIndex),
			"service_state_key", fmt.Sprintf("0x%x", stateKey[:31]),
			"StorageSize (dec)", fmt.Sprintf("%d", s.StorageSize),
			"StorageSize (LE)", fmt.Sprintf("0x%x", storageSizeLE),
			"NumStorageItems", fmt.Sprintf("%d", s.NumStorageItems),
			"Bytes", fmt.Sprintf("%x", buf.Bytes()))
	}
	return buf.Bytes(), nil
}

// Recover reconstructs an AccountState from a byte slice
func (s *ServiceAccount) Recover(data []byte) error {
	// Ensure the length of the data is correct (88 bytes)
	expectedLen := 32 + 8*5 + 4*4 + 1 // 32 bytes for CodeHash, 5 * 8 bytes for uint64, 4 * 4 bytes for uint32
	if len(data) != expectedLen {
		return fmt.Errorf("invalid data length: expected %d, got %d", expectedLen, len(data))
	}

	// Create a reader from the data
	buf := bytes.NewReader(data)

	readUint8 := func() (uint8, error) {
		var value uint8
		err := binary.Read(buf, binary.LittleEndian, &value)
		return value, err
	}

	// 0.7.1 Includes version byte prefix for accounts version 0 byte https://graypaper.fluffylabs.dev/#/1c979cb/3b81033b8103?v=0.7.1
	var err1 error
	versionByte, err1 := readUint8()
	if err1 != nil {
		return err1
	}
	if versionByte != 0 {
		return fmt.Errorf("unsupported account version: expected 0, got %d", versionByte)
	}
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

	//[Gratis] E8(b,g,m,o,f) Balance, GasLimitG, GasLimitM, StorageSize, GratisOffset
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
	if s.GratisOffset, err = readUint64(); err != nil {
		return err
	}

	//[Gratis] E4(i,r,a,p) NumStorageItems, CreateTime, RecentAccumulation, ParentService
	if s.NumStorageItems, err = readUint32(); err != nil {
		return err
	}
	if s.CreateTime, err = readUint32(); err != nil {
		return err
	}
	if s.RecentAccumulation, err = readUint32(); err != nil {
		return err
	}
	if s.ParentService, err = readUint32(); err != nil {
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
		ServiceIndex:       service_index,
		CodeHash:           acctState.CodeHash,           // a_c
		Balance:            acctState.Balance,            // a_b
		GasLimitG:          acctState.GasLimitG,          // a_g
		GasLimitM:          acctState.GasLimitM,          // a_m
		StorageSize:        acctState.StorageSize,        // a_o
		GratisOffset:       acctState.GratisOffset,       // a_f
		NumStorageItems:    acctState.NumStorageItems,    // a_i
		CreateTime:         acctState.CreateTime,         // a_r
		RecentAccumulation: acctState.RecentAccumulation, // a_a
		ParentService:      acctState.ParentService,      // a_p
		Mutable:            false,                        // THIS THE DEFAULT
		// Initialize maps to avoid nil reference errors
		Storage:  make(map[string]*StorageObject),
		Lookup:   make(map[string]*LookupObject),
		Preimage: make(map[string]*PreimageObject),
	}
	return serviceAccount, nil
}

// Convert the ServiceAccount to a human-readable string.
func (s *ServiceAccount) String() string {
	// Initial account information
	str := fmt.Sprintf("ServiceAccount %d CodeHash: %v B=%d (%x) G=%v M=%v O=%v F=%v | I=%v R=%v A=%v P=%v [Mutable: %v Dirty: %v]\n",
		s.ServiceIndex, s.CodeHash.Hex(), s.Balance, s.Balance, s.GasLimitG, s.GasLimitM, s.StorageSize, s.GratisOffset, //E8(ab,ag,am,ao,af)
		s.NumStorageItems, s.CreateTime, s.RecentAccumulation, s.ParentService, // E4(ai,ar,aa,ap)
		s.Mutable, s.Dirty)

	// Lookup entries
	str2 := ""
	for h, lo := range s.Lookup {
		str2 += fmt.Sprintf("  Lookup: %v => %s\n", h, lo.String())
	}

	// Preimage entries
	str3 := ""
	for h, lo := range s.Preimage {
		str3 += fmt.Sprintf("  Preimage: %v => %s\n", h, lo.String())
	}

	// Storage entries
	str4 := ""
	for h, lo := range s.Storage {
		str4 += fmt.Sprintf("  Storage: %v => %v\n", h, lo.String())
	}
	return str + str2 + str3 + str4
}

func (s *ServiceAccount) JsonString() string {
	return ToJSONHexIndent(s)
}
func (s *ServiceAccount) ReadStorage(mu_k []byte, sdb HostEnv) (ok bool, v []byte, source string) {
	serviceIndex := s.ServiceIndex
	as_internal_key := common.Compute_storageKey_internal(mu_k)
	as_internal_key_str := common.Bytes2Hex(as_internal_key[:])
	storageObj, ok := s.Storage[as_internal_key_str]
	if ok {
		if storageObj.Deleted {
			return false, nil, storageObj.Source
		}
		return true, storageObj.Value, storageObj.Source
	}
	var err error
	v, ok, err = sdb.ReadServiceStorage(serviceIndex, mu_k)
	if err != nil || !ok {
		return false, nil, "trie"
	}
	return true, v, "trie"
}

func (s *ServiceAccount) ReadPreimage(blobHash common.Hash, sdb HostEnv) (ok bool, preimage []byte, source string) {
	if s.Preimage == nil {
		s.Preimage = make(map[string]*PreimageObject)
	}
	blobHashStr := blobHash.Hex()

	preimageObj, ok := s.Preimage[blobHashStr]
	if ok {
		if preimageObj.Deleted {
			return false, nil, preimageObj.Source
		}
		return true, preimageObj.Preimage, preimageObj.Source
	}

	var err error
	preimage, ok, err = sdb.ReadServicePreimageBlob(s.GetServiceIndex(), blobHash)
	if err != nil || !ok {
		return false, preimage, "trie"
	}
	/* TODO: */
	if s.Preimage == nil {
		s.Preimage = make(map[string]*PreimageObject)
	}
	s.Preimage[blobHashStr] = &PreimageObject{
		Accessed: true,
		Dirty:    false,
		Preimage: preimage,
		Source:   "trie",
	}

	return true, preimage, "trie"
}

func (s *ServiceAccount) ReadLookup(blobHash common.Hash, z uint32, sdb HostEnv) (ok bool, anchor_timeslot []uint32, source string) {
	blobHashStr := blobHash.Hex()
	lookupObj, ok := s.Lookup[blobHashStr]
	if ok {
		if lookupObj.Deleted {
			return false, []uint32{}, lookupObj.Source
		}
		return true, lookupObj.Timeslots, lookupObj.Source
	}

	var err error
	anchor_timeslot, found, err := sdb.ReadServicePreimageLookup(s.GetServiceIndex(), blobHash, z)
	if err != nil {
		log.Info(module, "ReadLookup: Error reading", "serviceID", s.GetServiceIndex(), "blobHash", blobHashStr, "z", z, "err", err)
		return false, []uint32{}, "trie"
	}
	if !found {
		log.Trace(module, "ReadLookup: Not found", "serviceID", s.GetServiceIndex(), "blobHash", blobHashStr, "z", z)
		return false, []uint32{}, "trie"
	}
	/* TODO: */
	if s.Lookup == nil {
		s.Lookup = make(map[string]*LookupObject)
	}
	lookupObj = &LookupObject{
		Accessed:  true,
		Dirty:     false,
		Z:         z,
		Timeslots: anchor_timeslot,
		Source:    "trie",
	}
	s.Lookup[blobHashStr] = lookupObj

	log.Trace(module, "ReadLookup From Cache found", "serviceID", s.GetServiceIndex(), "blobHash", blobHashStr, "z", z, "anchor_timeslot", anchor_timeslot)
	return true, anchor_timeslot, "trie"
}

// a_c - account code hash c
func (s *ServiceAccount) SetCodeHash(codeHash common.Hash) {
	if !s.Mutable {
		log.Crit(module, "SetCodeHash")
	}
	s.Dirty = true
	s.CodeHash = codeHash
}

// a_b - account balance b, which must be greater than a_t (The threshold needed in terms of its storage footprint)
// Note this could be IncBalance DecBalance instead?
func (s *ServiceAccount) DecBalance(balance uint64) {
	if !s.Mutable {
		log.Crit(module, "DecBalance SetBalance Non-Mutable Err")
		return
	}
	s.Dirty = true
	s.Balance -= balance
}
func (s *ServiceAccount) IncBalance(balance uint64) {
	if !s.Mutable {
		log.Crit(module, "IncBalance SetBalance Non-Mutable Err")
		return
	}
	s.Dirty = true
	s.Balance += balance
}

// AdjustNumStorageItems adjusts the number of storage items by delta and marks it dirty
func (s *ServiceAccount) AdjustNumStorageItems(delta int32) {
	if !s.Mutable {
		log.Crit(module, "Called AdjustNumStorageItems on immutable ServiceAccount", "service", s.ServiceIndex)
	}
	// Handle underflow - don't let it go negative
	if delta < 0 && uint32(-delta) > s.NumStorageItems {
		s.NumStorageItems = 0
	} else {
		s.NumStorageItems = uint32(int32(s.NumStorageItems) + delta)
	}
	s.NumStorageItemsDirty = true
}

// AdjustStorageSize adjusts the storage size by delta and marks it dirty if changed
func (s *ServiceAccount) AdjustStorageSize(delta int64) {
	if !s.Mutable {
		log.Crit(module, "Called AdjustStorageSize on immutable ServiceAccount", "service", s.ServiceIndex)
	}
	if delta == 0 {
		return
	}
	// Handle underflow - don't let it go negative
	if delta < 0 && uint64(-delta) > s.StorageSize {
		s.StorageSize = 0
	} else {
		s.StorageSize = uint64(int64(s.StorageSize) + delta)
	}
	// Only mark dirty if there was an actual change
	if delta != 0 {
		s.StorageSizeDirty = true
	}
}

// ResetDirtyFlags clears the dirty markers on the account -- This works with MergeUpdates so that
// we don't keep re-merging other accounts which may be stale overwriting newer mutations (e.g. storage counters).
func (s *ServiceAccount) ResetDirtyFlags() {
	s.Dirty = false
	s.NumStorageItemsDirty = false
	s.StorageSizeDirty = false
	for _, storage := range s.Storage {
		storage.Dirty = false
	}
	for _, lookup := range s.Lookup {
		lookup.Dirty = false
	}
	for _, preimage := range s.Preimage {
		preimage.Dirty = false
	}
}

func (s *ServiceAccount) WriteStorage(serviceIndex uint32, mu_k []byte, val []byte, Isdeleted bool, source string) {
	if !s.Mutable {
		log.Crit(log.PvmAuthoring, "WriteStorage Mutable Err: Called WriteStorage on immutable ServiceAccount", "serviceIndex", serviceIndex, "mu_k", fmt.Sprintf("%x", mu_k), "val", fmt.Sprintf("%x", val), "source", source)
	}

	as_internal_key := common.Compute_storageKey_internal(mu_k)
	as_internal_key_str := common.Bytes2Hex(as_internal_key[:])
	account_storage_key := fmt.Sprintf("0x%x", common.ComputeC_sh(serviceIndex, as_internal_key).Bytes()[:31])
	s.Dirty = true

	storeObj, exists := s.Storage[as_internal_key_str]
	prevLen := 0
	if exists {
		prevLen = len(storeObj.Value)
		storeObj.Accessed = true
		storeObj.Dirty = true
		storeObj.Deleted = Isdeleted
		storeObj.Key = slices.Clone(mu_k)  // copy
		storeObj.Value = slices.Clone(val) // copy
		storeObj.StorageKey = account_storage_key
		storeObj.InternalKey = as_internal_key_str
	} else {
		s.Storage[as_internal_key_str] = &StorageObject{
			Accessed:    false,
			Dirty:       true,
			Deleted:     Isdeleted,
			Key:         slices.Clone(mu_k), // copy of mu_k
			Value:       slices.Clone(val),  // copy of val
			StorageKey:  account_storage_key,
			InternalKey: as_internal_key_str,
			Source:      source,
		}
	}

	// REVIEW: why do we need this?
	if Isdeleted {
		s.NumStorageItemsDirty = true
		s.StorageSizeDirty = true
	} else if !exists {
		if len(val) > 0 {
			s.NumStorageItemsDirty = true
			s.StorageSizeDirty = true
		}
	} else if exists && len(val) != prevLen {
		s.StorageSizeDirty = true
	}
}

func (s *ServiceAccount) WritePreimage(blobHash common.Hash, preimage []byte, source string) {
	if s.Preimage == nil {
		s.Preimage = make(map[string]*PreimageObject)
	}
	if s.Mutable == false {
		log.Crit(module, "Called WriteStorage on immutable ServiceAccount", "source", source)
	}
	blobHashStr := blobHash.Hex()

	o, exists := s.Preimage[blobHashStr]
	if exists {
		o.Accessed = true
		o.Dirty = true
		o.Deleted = len(preimage) == 0
		o.Preimage = preimage
		o.Source = source
	} else {
		s.Preimage[blobHashStr] = &PreimageObject{
			Accessed: false,
			Dirty:    true,
			Deleted:  len(preimage) == 0,
			Preimage: preimage,
			Source:   source,
		}
	}
	s.Dirty = true
}

func (s *ServiceAccount) WriteLookup(blobHash common.Hash, z uint32, time_slots []uint32, source string) {
	if s.Lookup == nil {
		s.Lookup = make(map[string]*LookupObject)
	}
	if s.Mutable == false {
		log.Crit(module, "Called WriteStorage on immutable ServiceAccount", "source", source)
	}
	blobHashStr := blobHash.Hex()
	s.Dirty = true
	o, exists := s.Lookup[blobHashStr]
	if exists {
		o.Accessed = true
		o.Dirty = true
		o.Deleted = time_slots == nil
		o.Z = z
		o.Timeslots = time_slots
		o.Source = source
		return
	}
	s.NumStorageItemsDirty = true
	s.StorageSizeDirty = true
	s.Lookup[blobHashStr] = &LookupObject{
		Accessed:  false,
		Dirty:     true,
		Deleted:   time_slots == nil,
		Z:         z,
		Timeslots: time_slots,
		Source:    source,
	}
}

func (s *ServiceAccount) UpdateRecentAccumulation(timeslot uint32) {
	s.Dirty = true
	s.RecentAccumulation = timeslot
}

func (s *ServiceAccount) MergeUpdates(updates *ServiceAccount) {
	// Only merge if dirty in updates OR if entry doesn't exist yet
	// If entry exists and update is not dirty, keep existing (preserves Dirty=true)
	for k, v := range updates.Storage {
		if _, exists := s.Storage[k]; !exists || v.Dirty {
			s.Storage[k] = v
		}
	}
	for k, v := range updates.Lookup {
		if _, exists := s.Lookup[k]; !exists || v.Dirty {
			s.Lookup[k] = v
		}
	}
	for k, v := range updates.Preimage {
		if _, exists := s.Preimage[k]; !exists || v.Dirty {
			s.Preimage[k] = v
		}
	}
	if updates.NumStorageItemsDirty {
		s.NumStorageItems = updates.NumStorageItems
		s.NumStorageItemsDirty = true
	}
	if updates.StorageSizeDirty {
		s.StorageSize = updates.StorageSize
		s.StorageSizeDirty = true
	}
	if updates.RecentAccumulation > s.RecentAccumulation {
		s.RecentAccumulation = updates.RecentAccumulation
	}
}

// (9.8) [Gratis] max(0, BS + BI ⋅ ai + BL ⋅ ao − af )
func (s *ServiceAccount) ComputeThreshold() uint64 {
	footprint := BaseServiceBalance +
		MinElectiveServiceItemBalance*uint64(int32(s.NumStorageItems)) +
		MinElectiveServiceOctetBalance*(uint64(int64(s.StorageSize)))
		// af ≥ (BS+BI⋅ai+BL⋅ao)
	if s.GratisOffset >= footprint {
		return 0
	}
	// BS+BI⋅ai+BL⋅ao - af
	return footprint - s.GratisOffset
}

func (s *ServiceAccount) ComputeThresholdDelta(deltaNumStorageItems int32, deltaStorageSize int64) uint64 {
	// Check for overflow in addition first
	if uint64(s.StorageSize) > ^uint64(0)-uint64(deltaStorageSize) {
		// Addition would overflow, return max uint64
		return ^uint64(0)
	}

	storageOctets := uint64(s.StorageSize) + uint64(deltaStorageSize)
	octetCost := uint64(MinElectiveServiceOctetBalance)

	// Check for overflow in multiplication
	if octetCost > 0 && storageOctets > ^uint64(0)/octetCost {
		// Multiplication would overflow, return max uint64
		return ^uint64(0)
	}

	footprint := BaseServiceBalance +
		MinElectiveServiceItemBalance*uint64(int32(s.NumStorageItems)+deltaNumStorageItems) +
		octetCost*storageOctets
		// af ≥ (BS+BI⋅ai+BL⋅ao)
	if s.GratisOffset >= footprint {
		return 0
	}
	// BS+BI⋅ai+BL⋅ao - af
	return footprint - s.GratisOffset
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

func convertHashMapToStringMap[T any](input map[string]*T) map[string]*T {
	output := make(map[string]*T, len(input))
	for k, v := range input {
		output[k] = v
	}
	return output
}

func convertStringMapToHashMap[T any](input map[string]*T) map[string]*T {
	output := make(map[string]*T, len(input))
	for k, v := range input {
		output[k] = v
	}
	return output
}

func CombineMetadataAndCode(service_name string, code []byte) []byte {
	service_name_bytes := []byte(service_name)
	service_name_length_bytes := E(uint64(len(service_name_bytes)))
	return append(service_name_length_bytes, append(service_name_bytes, code...)...)
}

// (9.2)E(↕m,c) ?
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
	filePath, err := common.GetFilePath(fp)
	if err != nil {
		return nil, fmt.Errorf("failed to get file path: %w", err)
	}
	raw_code, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return CombineMetadataAndCode(metadata, raw_code), nil
}
