package types

import (
	//	"fmt"
	"github.com/colorfulnotion/jam/common"
	//"encoding/hex"
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
	Payload []byte `json:"payload"`
	// x: extrinsic
	Extrinsics      []WorkItemExtrinsic `json:"extrinsic"`
	ExtrinsicsBlobs [][]byte            `json:"extrinsics"`
	// g: a gas limit
	GasLimit         int             `json:"gas_limit"`
	ImportedSegments []ImportSegment `json:"imported_segments"`
	ExportCount      uint32          `json:"export_count"`
}

type ImportSegment struct {
	TreeRoot common.Hash `json:"tree_root"`
	Index    uint32      `json:"index"`
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

func (item ASWorkItem) ToByteSlices() [][]byte {
	var slices [][]byte
	for _, segment := range item.Segments {
		slices = append(slices, segment.Data)
	}
	return slices
}

type SWorkItem struct {
	// s: the identifier of the service to which it relates
	ServiceIdentifier int `json:"service_identifier"`
	// c: the code hash of the service at the time of reporting
	CodeHash common.Hash `json:"code_hash"`
	// y: a payload blob
	Payload string `json:"payload"`
	// x: extrinsic
	Extrinsics []WorkItemExtrinsic `json:"extrinsic"`
	// g: a gas limit
	GasLimit         int             `json:"gas_limit"`
	ImportedSegments []ImportSegment `json:"imported_segments"`
	ExportCount      uint32          `json:"export_count"`
}

//From Sec 14: Once done, then imported segments must be reconstructed. This process may in fact be lazy as the Refine function makes no usage of the data until the ${\tt import}$ hostcall is made. Fetching generally implies that, for each imported segment, erasure-coded chunks are retrieved from enough unique validators (342, including the guarantor).  Chunks must be fetched for both the data itself and for justification metadata which allows us to ensure that the data is correct.

/*
	{
	 "service": 16909060,
	 "code_hash": "0x70a50829851e8f6a8c80f92806ae0e95eb7c06ad064e311cc39107b3219e532e",
	 "payload": "0x0102030405",
	 "gas_limit": 42,
	 "import_segments": [
	     {
	         "tree_root": "0x461236a7eb29dcffc1dd282ce1de0e0ed691fc80e91e02276fe8f778f088a1b8",
	         "index": 0
	     },
	     {
	         "tree_root": "0xe7cb536522c1c1b41fff8021055b774e929530941ea12c10f1213c56455f29ad",
	         "index": 1
	     },
	     {
	         "tree_root": "0xb0a487a4adf6a0eda5d69ddd2f8b241cf44204f0ff793e993e5e553b7862a1dc",
	         "index": 2
	     }
	 ],
	 "extrinsic": [
	     {
	         "hash": "0x381a0e351c5593018bbc87dd6694695caa1c0c1ddb24e70995da878d89495bf1",
	         "len": 16
	     },
	     {
	         "hash": "0x6c437d85cd8327f42a35d427ede1b5871347d3aae7442f2df1ff80f834acf17a",
	         "len": 17
	     }
	 ],
	 "export_count": 4
	 }
*/
func (s *SWorkItem) Deserialize() (WorkItem, error) {

	payloadBytes := common.FromHex(s.Payload)
	return WorkItem{
		ServiceIdentifier: s.ServiceIdentifier,
		CodeHash:          s.CodeHash,
		Payload:           payloadBytes,
		Extrinsics:        s.Extrinsics, // Assuming no conversion needed
		GasLimit:          s.GasLimit,
		ImportedSegments:  s.ImportedSegments, // Assuming no conversion needed
		ExportCount:       s.ExportCount,
	}, nil
}
