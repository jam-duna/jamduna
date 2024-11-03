package types

import (
	"encoding/json"
	"math"

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
	SegmentRoot []byte      `json:"segment_root"`
	Data        []byte      `json:"data"`
	RootHash    common.Hash `json:"root_hash"`
	BlobMeta    []byte      `json:"blob_meta"`
}

// Marshal marshals DistributeECChunk into JSON
func (d *DistributeECChunk) Marshal() ([]byte, error) {
	return json.Marshal(d)
}

// Unmarshal unmarshals JSON data into DistributeECChunk
func (d *DistributeECChunk) Unmarshal(data []byte) error {
	return json.Unmarshal(data, d)
}

func (d *DistributeECChunk) Bytes() []byte {
	jsonData, _ := d.Marshal()
	return jsonData
}

func (d *DistributeECChunk) String() string {
	return string(d.Bytes())
}

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

// ValidatorStatistics represents the statistics tracked for each validator.
type ValidatorStatistics struct {
	BlocksProduced         int `json:"blocks_produced"`
	TicketsIntroduced      int `json:"tickets_introduced"`
	PreimagesIntroduced    int `json:"preimages_introduced"`
	OctetsIntroduced       int `json:"octets_introduced"`
	ReportsGuaranteed      int `json:"reports_guaranteed"`
	AvailabilityAssurances int `json:"availability_assurances"`
}

// StatisticalReporter represents the statistics for the current and previous epochs.
type StatisticalReporter struct {
	CurrentEpochStats  []ValidatorStatistics `json:"current_epoch_stats"`
	PreviousEpochStats []ValidatorStatistics `json:"previous_epoch_stats"`
}

// GasAttributable represents the gas attributable for each service.
type GasAttributable struct {
	ServiceIndex int     `json:"service_index"`
	Gas          float64 `json:"gas"`
}

func ComputeC_Base(blob_length int) int {
	c := int(math.Ceil(float64(blob_length) / float64(W_E)))
	return c
}

// For BPT
type BPTNode struct {
	Value []byte `json:"value"`
	Key   []byte `json:"key"`
}
