package types

import (
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
// WorkItem represents a work item.
type WorkItem struct {
	// s: the identifier of the service to which it relates
	ServiceIdentifier int `json:"service_identifier"`
	// c: the code hash of the service at the time of reporting
	CodeHash common.Hash `json:"code_hash"`
	// y: a payload blob
	PayloadBlob []byte `json:"payload_blob"`
	// x: extrinsic
	Extrinsics [][]byte `json:"extrinsic"`
	// g: a gas limit
	GasLimit            int               `json:"gas_limit"`
	ImportedSegments    []ImportSegment   `json:"imported_segments"`
	NewData             []NewDataSegments `json:"new_data"` // shawn: not sure if this is correct
	NumSegmentsExported uint32            `json:"num_segments_exported"`
}

//From Sec 14: Once done, then imported segments must be reconstructed. This process may in fact be lazy as the Refine function makes no usage of the data until the ${\tt import}$ hostcall is made. Fetching generally implies that, for each imported segment, erasure-coded chunks are retrieved from enough unique validators (342, including the guarantor).  Chunks must be fetched for both the data itself and for justification metadata which allows us to ensure that the data is correct.

type ImportSegment struct {
	SegmentRoot  common.Hash `json:"segment_root"`
	SegmentIndex uint32      `json:"segment_index"`
}
type NewDataSegments struct {
	BlobHash      []common.Hash `json:"blob_hash"`
	BlobHashIndex []uint32      `json:"blob_hash_index"`
}
