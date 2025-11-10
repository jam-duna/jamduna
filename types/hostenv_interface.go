package types

import (
	"github.com/colorfulnotion/jam/common"
)

type HostEnv interface {
	GetID() uint16
	GetService(service uint32) (*ServiceAccount, bool, error)
	ReadServiceStorage(s uint32, k []byte) ([]byte, bool, error)
	GetParentStateRoot() common.Hash
	ReadServicePreimageBlob(s uint32, blob_hash common.Hash) ([]byte, bool, error)
	ReadServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32) ([]uint32, bool, error)
	HistoricalLookup(a *ServiceAccount, t uint32, blob_hash common.Hash) []byte
	GetTimeslot() uint32
	GetHeader() *BlockHeader

	// ReadStateWitnessRef fetches StateWitness for given objectID; If fetchPayloadFromDA=true, fetches payload via FetchJAMDASegments and populates witness.Payload
	ReadStateWitnessRef(serviceID uint32, objectID common.Hash, fetchPayloadFromDA bool) (StateWitness, bool, error)

	WriteServicePreimageBlob(s uint32, blob []byte)
	WriteServicePreimageLookup(s uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32)
}
