package types

import (
	"github.com/colorfulnotion/jam/common"
)

type HostEnv interface {
	GetService(service uint32) (*ServiceAccount, bool, error)
	ReadServiceStorage(s uint32, k []byte) ([]byte, bool, error)
	ReadServicePreimageBlob(s uint32, blob_hash common.Hash) ([]byte, bool, error)
	ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) ([]uint32, bool, error)
	HistoricalLookup(s uint32, t uint32, blob_hash common.Hash) []byte
	GetTimeslot() uint32

	// TODO LATER: remove this by using NewXContext + ApplyXContext in genesis.go
	WriteServicePreimageBlob(s uint32, blob []byte)
	WriteServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32)
}
