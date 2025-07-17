package types

import (
	"bytes"
	"encoding/binary"
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

// WorkItem represents a work item.
type WorkItem struct {
	Service            uint32              `json:"service"`              // s: the identifier of the service to which it relates
	CodeHash           common.Hash         `json:"code_hash"`            // c: the code hash of the service at the time of reporting
	Payload            []byte              `json:"payload"`              // y: a payload blob
	RefineGasLimit     uint64              `json:"refine_gas_limit"`     // g: a refine gas limit
	AccumulateGasLimit uint64              `json:"accumulate_gas_limit"` // a: an accumulate gas limit
	ImportedSegments   []ImportSegment     `json:"import_segments"`      // i: a sequence of imported data segments
	Extrinsics         []WorkItemExtrinsic `json:"extrinsic"`            // x: extrinsic
	ExportCount        uint16              `json:"export_count"`         // e: the number of data segments exported by this work item
}

// 0.6.2 14.5
func (i *WorkItem) GetTotalDataLength() int {
	total := 0
	total += len(i.Payload)
	import_count := len(i.ImportedSegments)
	data_len_import := import_count * SegmentSize
	total += data_len_import
	for _, extrinsic := range i.Extrinsics {
		total += int(extrinsic.Len)
	}
	return total
}

// From Sec 14: Once done, then imported segments must be reconstructed. This process may in fact be lazy as the Refine function makes no usage of the data until the ${\tt import}$ hostcall is made. Fetching generally implies that, for each imported segment, erasure-coded chunks are retrieved from enough unique validators (342, including the guarantor).  Chunks must be fetched for both the data itself and for justification metadata which allows us to ensure that the data is correct.
type ImportSegment struct {
	RequestedHash common.Hash `json:"tree_root"`
	Index         uint16      `json:"index"`
}
type WorkItemExtrinsic struct {
	Hash common.Hash `json:"hash"`
	Len  uint32      `json:"len"`
}

// Segment represents a segment of data
type Segment struct {
	Data []byte
}

func (w *WorkItem) EncodeS() ([]byte, error) {

	var buf bytes.Buffer

	writeUint64 := func(value uint64) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}
	writeUint32 := func(value uint32) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}
	writeUint16 := func(value uint16) error {
		return binary.Write(&buf, binary.LittleEndian, value)
	}
	writeHash := func(value common.Hash) error {
		return binary.Write(&buf, binary.LittleEndian, value.Bytes())
	}
	// E_4: w_s
	if err := writeUint32(w.Service); err != nil {
		return nil, err
	}
	// E_2: w_h
	if err := writeHash(w.CodeHash); err != nil {
		return nil, err
	}
	// E_8: w_g, w_a
	if err := writeUint64(w.RefineGasLimit); err != nil {
		return nil, err
	}
	if err := writeUint64(w.AccumulateGasLimit); err != nil {
		return nil, err
	}
	// E_2: w_e, |w_i|, |w_x|
	if err := writeUint16(w.ExportCount); err != nil {
		return nil, err
	}
	if err := writeUint16(uint16(len(w.ImportedSegments))); err != nil {
		return nil, err
	}
	if err := writeUint16(uint16(len(w.Extrinsics))); err != nil {
		return nil, err
	}
	// E_4: |w_y|
	if err := writeUint32(uint32(len(w.Payload))); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (a *WorkItem) UnmarshalJSON(data []byte) error {
	var s struct {
		Service            uint32              `json:"service"`
		CodeHash           common.Hash         `json:"code_hash"`
		Payload            string              `json:"payload"`
		RefineGasLimit     uint64              `json:"refine_gas_limit"`
		AccumulateGasLimit uint64              `json:"accumulate_gas_limit"`
		ImportedSegments   []ImportSegment     `json:"import_segments"`
		Extrinsics         []WorkItemExtrinsic `json:"extrinsic"`
		ExportCount        uint16              `json:"export_count"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	a.Service = s.Service
	a.CodeHash = s.CodeHash
	a.Payload = common.FromHex(s.Payload)
	a.RefineGasLimit = s.RefineGasLimit
	a.AccumulateGasLimit = s.AccumulateGasLimit
	a.ImportedSegments = s.ImportedSegments
	a.Extrinsics = s.Extrinsics
	a.ExportCount = s.ExportCount
	return nil
}

func (a WorkItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Service            uint32              `json:"service"`
		CodeHash           common.Hash         `json:"code_hash"`
		Payload            string              `json:"payload"`
		RefineGasLimit     uint64              `json:"refine_gas_limit"`
		AccumulateGasLimit uint64              `json:"accumulate_gas_limit"`
		ImportedSegments   []ImportSegment     `json:"import_segments"`
		Extrinsics         []WorkItemExtrinsic `json:"extrinsic"`
		ExportCount        uint16              `json:"export_count"`
	}{
		Service:            a.Service,
		CodeHash:           a.CodeHash,
		Payload:            common.HexString(a.Payload),
		RefineGasLimit:     a.RefineGasLimit,
		AccumulateGasLimit: a.AccumulateGasLimit,
		ImportedSegments:   a.ImportedSegments,
		Extrinsics:         a.Extrinsics,
		ExportCount:        a.ExportCount,
	})
}
