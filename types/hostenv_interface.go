package types

import (
	"github.com/colorfulnotion/jam/common"
)

type HostEnv interface {

	// ∀(s ↦ a) ∈ δ
	ReadServiceBytes(s uint32) []byte
	WriteServiceBytes(s uint32, v []byte)

	// ∀(s↦a)∈δ,(h↦v)∈as
	ReadServiceStorage(s uint32, k []byte) []byte
	WriteServiceStorage(s uint32, k []byte, storage []byte)

	// ∀(s ↦ a) ∈ δ, (h ↦ p) ∈ a p ∶
	ReadServicePreimageBlob(s uint32, blob_hash common.Hash) []byte
	WriteServicePreimageBlob(s uint32, blob []byte)

	// ∀(s ↦ a) ∈ δ, ( ⎧⎩ h, l ⎫⎭ ↦ t)∈ a l ∶ C(s, E 4 (l)⌢(¬h 4∶ )) ↦ E(↕[E 4 (x) ∣ x <− t])
	ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) []uint32
	WriteServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32)

	// Delete key(hash)
	DeleteServiceStorageKey(s uint32, k []byte) error
	DeleteServicePreimageKey(s uint32, blob_hash common.Hash) error
	DeleteServicePreimageLookupKey(s uint32, blob_hash common.Hash, blob_length uint32) error

	// Λ∶(A, N T , H)→ Y? , GP_0.3.5(94)
	HistoricalLookup(s uint32, t uint32, blob_hash common.Hash) []byte

	// Below are not used:

	// Service Management
	NewService(c []byte, l, b uint32, g, m uint64) uint32
	UpgradeService(c []byte, g, m uint64) uint32
	AddTransfer(m []byte, a, g uint64, d uint32) uint32

	// Preimage/DA
	GetImportItem(i uint32) ([]byte, uint32)
	ExportSegment(x []byte) uint32

	// StateDB
	DeleteKey(k common.Hash) error
	ContainsKey(h []byte, z []byte) bool
	SetKey(k common.Hash, v []byte, b0 uint32, numBytes int) error
	AddKey(k common.Hash, v []byte) error

	// VM Management
	CreateVM(code []byte, i uint32) uint32
	//GetVM(n uint32) (*VM, bool)
	ExpungeVM(n uint32) bool

	// Privileged Services
	Designate(v []byte) uint32
	Empower(m uint32, a uint32, v uint32) uint32
	Assign(c []byte) uint32

	GetEnvType() string

	//
	FlexBackend() interface{}
}

type CustomeBackend interface {
	GetTimeSlot() uint32
}
