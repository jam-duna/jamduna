package types

import (
	//"fmt"
	// "github.com/colorfulnotion/jam/scale"
	//"encoding/json"

	"github.com/colorfulnotion/jam/common"
	//"github.com/colorfulnotion/jam/trie"
	"math"
)

type BMTProof []common.Hash

const (
	validatorCount = 1023
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

// -----Custom types for tiny QUIC experiment-----

type DistributeECChunk struct {
	SegmentRoot []byte `json:"segment_root"`
	Data        []byte `json:"data"`
}

type ECChunkResponse struct {
	SegmentRoot []byte `json:"segment_root"`
	Data        []byte `json:"data"`
}

type ECChunkQuery struct {
	SegmentRoot common.Hash `json:"segment_root"`
}

// `ImportDAQuery` + `ImportDAResponse` WIP:
type ImportDAQuery struct {
	SegmentRoot    common.Hash `json:"segment_root"`
	SegmentIndex   uint32      `json:"segment_index"`
	ProofRequested bool        `json:"proof_requested"`
}

type ImportDAResponse struct {
	Data  [][]byte `json:"data"`
	Proof BMTProof `json:"proof"`
}

type AuditDAQuery struct {
	SegmentRoot    common.Hash `json:"segment_root"`
	Index          int         `json:"segment_index"`
	ProofRequested bool        `json:"proof_requested"`
}

type AuditDAResponse struct {
	Data  []byte   `json:"data"`
	Proof BMTProof `json:"proof"`
}

type ImportDAReconstructQuery struct {
	SegmentRoot common.Hash `json:"segment_root"`
}

type ImportDAReconstructResponse struct {
	Data []byte `json:"data"`
}

// DeferredTransfer represents a deferred transfer.
type DeferredTransfer struct {
	SenderIndex   int      `json:"sender_index"`
	ReceiverIndex int      `json:"receiver_index"`
	Amount        int      `json:"amount"`
	Memo          [64]byte `json:"memo"`
	GasLimit      int      `json:"gas_limit"`
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

// ServiceAccumulation represents a service accumulation result.
type ServiceAccumulation struct {
	ServiceIndex int         `json:"service_index"`
	Result       common.Hash `json:"result"`
}

// GasAttributable represents the gas attributable for each service.
type GasAttributable struct {
	ServiceIndex int     `json:"service_index"`
	Gas          float64 `json:"gas"`
}

// the b part of EQ(186)
type AuditFriendlyWorkPackage struct {
	Package              []byte // "p":comprising the workpackage itself
	ExtrinsicData        []byte // "x":the extrinsic data
	ImportSegment        []byte // "i":the concatenated import segments
	MerkleJustifications []byte // "j":their proofs of correctness
}

func ComputeC_Base(blob_length int) int {
	c := int(math.Ceil(float64(blob_length) / float64(W_C)))
	return c
}
