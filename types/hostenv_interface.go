package types

import (
	"github.com/colorfulnotion/jam/common"
)

type HostEnv interface {
	// ∀(s ↦ a) ∈ δ
	ReadServiceBytes(s uint32) []byte
	WriteServiceBytes(s uint32, v []byte)

	// ∀(s↦a)∈δ,(h↦v)∈as
	ReadServiceStorage(s uint32, k common.Hash) []byte
	WriteServiceStorage(s uint32, k common.Hash, storage []byte)
	DeleteServiceStorageKey(s uint32, k common.Hash) error

	// ∀(s ↦ a) ∈ δ, (h ↦ p) ∈ a p ∶
	ReadServicePreimageBlob(s uint32, blob_hash common.Hash) []byte
	WriteServicePreimageBlob(s uint32, blob []byte)
	DeleteServicePreimageKey(s uint32, blob_hash common.Hash) error

	// ∀(s ↦ a) ∈ δ, ( ⎧⎩ h, l ⎫⎭ ↦ t)∈ a l ∶ C(s, E 4 (l)⌢(¬h 4∶ )) ↦ E(↕[E 4 (x) ∣ x <− t])
	ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) []uint32
	WriteServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32)
	DeleteServicePreimageLookupKey(s uint32, blob_hash common.Hash, blob_length uint32) error

	// Λ∶(A, N T , H)→ Y? , GP_0.3.5(94)
	HistoricalLookup(s uint32, t uint32, blob_hash common.Hash) []byte

	GetService(service uint32) (*ServiceAccount, error)
	Check(service uint32) uint32
	GetTimeslot() uint32
}
