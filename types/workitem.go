package types

import (
	"encoding/json"

	"github.com/colorfulnotion/jam/common"
)

/*
A work item includes: (See Equation 175)
* $s$, the identifier of the service to which it relates
* $c$, the code hash of the service at the time of reporting  (whose preimage must be available from the perspective of the lookup anchor block)
* ${\bf y}$, a payload blob
* $g$, a gas limit and
* the three elements of its manifest:
  - ${\bf i}$, a sequence of imported data segments identified by the root of the segments tree and an index into it;
  - ${\bf x}$, a sequence of hashes of data segments to be introduced in this block (and which we assume the validator knows);
  - $e$, the number of data segments exported by this work item
*/

type ExtrinsicsBlobs [][]byte

// WorkItem represents a work item.
type WorkItem struct {
	// s: the identifier of the service to which it relates
	Service uint32 `json:"service"`
	// c: the code hash of the service at the time of reporting
	CodeHash common.Hash `json:"code_hash"`
	// y: a payload blob
	Payload []byte `json:"payload"`
	// g: a gas limit
	GasLimit         uint64          `json:"gas_limit"`
	ImportedSegments []ImportSegment `json:"import_segments"`
	// x: extrinsic
	Extrinsics      []WorkItemExtrinsic `json:"extrinsic"`
	ExtrinsicsBlobs ExtrinsicsBlobs     `json:"extrinsics"`
	// x: extrinsic
	ExportCount uint16 `json:"export_count"`
}

// From Sec 14: Once done, then imported segments must be reconstructed. This process may in fact be lazy as the Refine function makes no usage of the data until the ${\tt import}$ hostcall is made. Fetching generally implies that, for each imported segment, erasure-coded chunks are retrieved from enough unique validators (342, including the guarantor).  Chunks must be fetched for both the data itself and for justification metadata which allows us to ensure that the data is correct.
type ImportSegment struct {
	TreeRoot common.Hash `json:"tree_root"`
	Index    uint16      `json:"index"`
}
type WorkItemExtrinsic struct {
	Hash common.Hash `json:"hash"`
	Len  uint32      `json:"len"`
}

// Segment represents a segment of data
type Segment struct {
	Data []byte
}

// The workitem is an ordered collection of segments
type ASWorkItem struct {
	Segments   []Segment
	Extrinsics []WorkItemExtrinsic `json:"extrinsic"`
}

func (E ExtrinsicsBlobs) Encode() []byte {
	return []byte{}
}

func (E ExtrinsicsBlobs) Decode(data []byte) (interface{}, uint32) {
	return nil, 0
}

func (a *WorkItem) UnmarshalJSON(data []byte) error {
	var s struct {
		Service          uint32              `json:"service"`
		CodeHash         common.Hash         `json:"code_hash"`
		Payload          string              `json:"payload"`
		GasLimit         uint64              `json:"gas_limit"`
		ImportedSegments []ImportSegment     `json:"import_segments"`
		Extrinsics       []WorkItemExtrinsic `json:"extrinsic"`
		ExportCount      uint16              `json:"export_count"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	a.Service = s.Service
	a.CodeHash = s.CodeHash
	a.Payload = common.FromHex(s.Payload)
	a.GasLimit = s.GasLimit
	a.ImportedSegments = s.ImportedSegments
	a.Extrinsics = s.Extrinsics
	a.ExportCount = s.ExportCount
	return nil
}

func (a WorkItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Service          uint32              `json:"service"`
		CodeHash         common.Hash         `json:"code_hash"`
		Payload          string              `json:"payload"`
		GasLimit         uint64              `json:"gas_limit"`
		ImportedSegments []ImportSegment     `json:"import_segments"`
		Extrinsics       []WorkItemExtrinsic `json:"extrinsic"`
		ExportCount      uint16              `json:"export_count"`
	}{
		Service:          a.Service,
		CodeHash:         a.CodeHash,
		Payload:          common.HexString(a.Payload),
		GasLimit:         a.GasLimit,
		ImportedSegments: a.ImportedSegments,
		Extrinsics:       a.Extrinsics,
		ExportCount:      a.ExportCount,
	})
}
