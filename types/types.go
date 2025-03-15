package types

import (
	"encoding/json"
	//	"math"

	"github.com/colorfulnotion/jam/common"
)

type BMTProof []common.Hash

const (
	// tiny
	validatorCount = 12
	// full
	// validatorCount = 1023
	coreCount      = 341
	epochLength    = 600
	rotationPeriod = 10
)

const (
	InfinityError = "∞"
	ZeroError     = "∅"
	BadError      = "BAD"
	BigError      = "BIG"
)

// ----------------------------------------------

type BlockQuery struct {
	BlockHash common.Hash `json:"block_hash"`
}

type ConformantECChunk struct {
	Data       []byte `json:"data"`
	ShardIndex uint32 `json:"shardIdx"`
}

// Marshal marshals ConformantECChunk into JSON
func (c *ConformantECChunk) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

// Unmarshal unmarshals JSON data into DistributeECChunk
func (c *ConformantECChunk) Unmarshal(data []byte) error {
	return json.Unmarshal(data, c)
}

func (c *ConformantECChunk) Bytes() []byte {
	jsonData, _ := c.Marshal()
	return jsonData
}

func (c *ConformantECChunk) String() string {
	return string(c.Bytes())
}

type DistributeECChunk struct {
	Data []byte `json:"data"`
}

type DistributeECChunks []DistributeECChunk

type ECChunkResponse struct {
	SegmentRoot []byte `json:"segment_root"`
	Data        []byte `json:"data"`
}

type ECChunkQuery struct {
	SegmentRoot common.Hash `json:"segment_root"`
}

// AccumulationState represents the state required for accumulation.
type AccumulationState struct {
	ServiceIndices    []int              `json:"service_indices"`
	WorkReports       []WorkReport       `json:"work_reports"`
	DeferredTransfers []DeferredTransfer `json:"deferred_transfers"`
}

// GasAttributable represents the gas attributable for each service.
type GasAttributable struct {
	ServiceIndex int     `json:"service_index"`
	Gas          float64 `json:"gas"`
}

// For BPT
type BPTNode struct {
	Value []byte `json:"value"`
	Key   []byte `json:"key"`
}

// ----------map[common.Hash]common.Hash----------

type Hash2Hash map[common.Hash]common.Hash

/*
// ---use dictionary---
// MarshalJSON serializes Hash2Hash as a map[string]string
func (h Hash2Hash) MarshalJSON() ([]byte, error) {
	stringMap := make(map[string]string)
	for k, v := range h {
		stringMap[k.Hex()] = v.Hex() // Convert keys and values to hex strings
	}
	return json.Marshal(stringMap)
}

UnmarshalJSON deserializes Hash2Hash from a map[string]string
func (h *Hash2Hash) UnmarshalJSON(data []byte) error {
	stringMap := make(map[string]string)
	if err := json.Unmarshal(data, &stringMap); err != nil {
		return err
	}
	*h = make(Hash2Hash)
	for k, v := range stringMap {
		keyHash := common.HexToHash(k) // Convert hex strings back to common.Hash
		valueHash := common.HexToHash(v)
		(*h)[keyHash] = valueHash
	}
	return nil
}
*/

// ---use pair---
// MarshalJSON serializes Hash2Hash as a slice of pairs
func (h Hash2Hash) MarshalJSON() ([]byte, error) {
	pairs := make([][2]common.Hash, 0, len(h))
	for k, v := range h {
		pairs = append(pairs, [2]common.Hash{k, v})
	}
	return json.Marshal(pairs)
}

// UnmarshalJSON deserializes Hash2Hash from a slice of pairs
func (h *Hash2Hash) UnmarshalJSON(data []byte) error {
	var pairs [][2]common.Hash
	if err := json.Unmarshal(data, &pairs); err != nil {
		return err
	}
	*h = make(Hash2Hash)
	for _, pair := range pairs {
		(*h)[pair[0]] = pair[1]
	}
	return nil
}
