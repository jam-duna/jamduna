package types

import (
	"github.com/colorfulnotion/jam/common"
)

type HostEnv interface {
	GetService(service uint32) (*ServiceAccount, bool, error)
	ReadServiceStorage(s uint32, k []byte) ([]byte, bool, error)
	GetStateRoot() common.Hash
	ReadServicePreimageBlob(s uint32, blob_hash common.Hash) ([]byte, bool, error)
	ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) ([]uint32, bool, error)
	HistoricalLookup(a *ServiceAccount, t uint32, blob_hash common.Hash) []byte
	GetTimeslot() uint32
	GetHeader() *BlockHeader

	ReadObject(serviceID uint32, objectID common.Hash) (*StateWitness, bool, error)
	GetWitnesses() map[common.Hash]*StateWitness

	WriteServicePreimageBlob(s uint32, blob []byte)
	WriteServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32)
}
