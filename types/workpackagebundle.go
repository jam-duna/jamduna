package types

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
)

// WorkPackageBundle represents a work package.
type WorkPackageBundle struct {
	WorkPackage       WorkPackage       `json:"work_package"`    // P: workPackage
	ExtrinsicData     ExtrinsicsBlobs   `json:"extrinsics"`      // X: extrinsic data for some workitem argument w
	ImportSegmentData [][][]byte        `json:"import_segments"` // M: import segment data, previouslly called m (each of segment is size of W_G)
	Justification     [][][]common.Hash `json:"justifications"`  // J: justifications of segment data build using CDT
}

func (b *WorkPackageBundle) Validate() error {
	//0.6.2 14.2
	work_package := b.WorkPackage
	if len(work_package.WorkItems) < 1 {
		return fmt.Errorf("WorkPackageBundle must have at least one WorkItem")
	}
	if len(work_package.WorkItems) > MaxWorkItemsPerPackage {
		return fmt.Errorf("WorkPackageBundle has too many WorkItems")
	}
	// 0.6.3 14.4
	total_exports := 0
	total_imports := 0
	total_extrinsics := 0
	for _, work_item := range work_package.WorkItems {
		total_exports += int(work_item.ExportCount)
		total_imports += len(work_item.ImportedSegments)
		total_extrinsics += len(work_item.Extrinsics)
	}
	if total_exports > MaxExports {
		return fmt.Errorf("WorkPackageBundle has too many exports")
	}
	if total_imports > MaxImports {
		return fmt.Errorf("WorkPackageBundle has too many imports")
	}
	//0.6.3 added maximum extrinsics
	if total_extrinsics > ExtrinsicMaximumPerPackage {
		return fmt.Errorf("WorkPackageBundle has too many extrinsics")
	}
	// 0.6.2 14.5
	data_lens := 0
	data_lens += len(work_package.Authorization)
	data_lens += len(work_package.ParameterizationBlob)
	for _, work_item := range work_package.WorkItems {
		data_lens += work_item.GetTotalDataLength()
	}
	if data_lens > MaxEncodedWorkPackageSize {
		return fmt.Errorf("WorkPackageBundle has too much data")
	}
	// 0.6.2 14.6
	Gas_a := uint64(0)
	Gas_r := uint64(0)
	for _, work_item := range work_package.WorkItems {
		Gas_a += work_item.AccumulateGasLimit
		Gas_r += work_item.RefineGasLimit
	}
	/*
		// TODO: FIX This
		if Gas_a > AccumulationGasAllocation {
			return fmt.Errorf("WorkPackageBundle has too much accumulate gas (%d must be less than %d)", Gas_a, AccumulationGasAllocation)
		}
		if Gas_r > RefineGasAllocation {
			return fmt.Errorf("WorkPackageBundle has too much refine gas (%d must be less than %d)", Gas_r, RefineGasAllocation)
		}
	*/
	return nil
}

func (b *WorkPackageBundle) StringL() string {
	jsonByte, _ := json.MarshalIndent(b, "", "  ")
	return string(jsonByte)
}

func (b *WorkPackageBundle) String() string {
	jsonByte, _ := json.Marshal(b)
	return string(jsonByte)
}

func (b *WorkPackageBundle) PackageHash() common.Hash {
	return b.WorkPackage.Hash()
}

func (b *WorkPackageBundle) Package() WorkPackage {
	return b.WorkPackage
}

func (b WorkPackageBundle) Encode() []byte {
	return b.Bytes()
}

func (b WorkPackageBundle) Decode(data []byte) (interface{}, uint32) {
	bp, leng, _ := DecodeBundle(data)
	return *bp, leng
}

func DecodeBundle(remaining []byte) (*WorkPackageBundle, uint32, error) {
	if len(remaining) < 1 {
		return nil, uint32(len(remaining)), fmt.Errorf("encoded bundle is empty")
	}
	// Decode the work package
	workPackage, length, err := Decode(remaining, reflect.TypeOf(WorkPackage{}))
	if err != nil {
		return nil, length, err
	}
	remaining = remaining[length:]
	//fmt.Printf("remaining length after work package decode: %d\n", len(remaining))
	wp := workPackage.(WorkPackage)
	// Create a new WorkPackageBundle
	bundle := &WorkPackageBundle{
		WorkPackage:       wp,
		ExtrinsicData:     make(ExtrinsicsBlobs, 0),
		ImportSegmentData: make([][][]byte, len(wp.WorkItems)),
		Justification:     make([][][]common.Hash, len(wp.WorkItems)),
	}
	// Decode the extrinsic data using *work items*
	for _, wpItem := range wp.WorkItems {
		extrinsics := wpItem.Extrinsics
		if len(extrinsics) == 0 {
			continue
		}
		for _, x := range extrinsics {
			h := x.Hash
			l := x.Len
			ext := remaining[:l]
			if h != common.Blake2Hash(ext) {
				log.Warn("codec", "extrinsic hash mismatch", "expected", h, "got", common.Blake2Hash(ext))
			}
			if len(remaining) < int(l) {
				return nil, length, fmt.Errorf("not enough data for extrinsic %s", h)
			}
			bundle.ExtrinsicData = append(bundle.ExtrinsicData, remaining[:l])
			length += uint32(l)
			remaining = remaining[l:]
		}
	}

	// Decode the imported segment data using *work items*
	for j, wpItem := range wp.WorkItems {
		importedSegments := wpItem.ImportedSegments
		if len(importedSegments) == 0 {
			continue
		}
		bundle.ImportSegmentData[j] = make([][]byte, len(importedSegments))
		for imp_segment_idx, _ := range importedSegments {
			s := remaining[:SegmentSize]
			bundle.ImportSegmentData[j][imp_segment_idx] = s
			length += SegmentSize
			remaining = remaining[SegmentSize:]
		}
	}

	// Decode the justification data using *work items*
	for j, wpItem := range wp.WorkItems {
		importedSegments := wpItem.ImportedSegments
		if len(importedSegments) == 0 {
			continue
		}
		bundle.Justification[j] = make([][]common.Hash, len(importedSegments))

		for imp_segment_idx, _ := range importedSegments {
			justification, l0, err := Decode(remaining, reflect.TypeOf([]common.Hash{}))
			if err != nil {
				log.Warn("codec", "justification decode error", "workItemIdx", j, "error", err)
				return nil, length, err
			}
			bundle.Justification[j][imp_segment_idx] = justification.([]common.Hash)
			length += l0
			remaining = remaining[l0:]
		}
	}
	return bundle, length, nil
}

func (b *WorkPackageBundle) Bytes() []byte {
	encode, err := Encode(b.WorkPackage)
	if err != nil {
		return nil
	}
	// Append the extrinsic data
	for _, extrinsic := range b.ExtrinsicData {
		if len(extrinsic) == 0 {
			continue
		}
		encode = append(encode, extrinsic...)
	}
	// Append the import segment data
	for _, wpItemSegments := range b.ImportSegmentData {
		if len(wpItemSegments) == 0 {
			continue
		}
		for _, segment := range wpItemSegments {
			if len(segment) == 0 {
				//continue
			}
			encode = append(encode, segment...)
		}
	}
	// Append the justification data
	for _, wpItemJustifications := range b.Justification {
		if len(wpItemJustifications) == 0 {
			continue
		}
		for _, seg_justification := range wpItemJustifications {
			if len(seg_justification) == 0 {
				//continue
			}
			// Encode each justification
			sj, err := Encode(seg_justification)
			if err != nil {
				return nil
			}
			encode = append(encode, sj...)
		}
	}
	return encode
}

func (b WorkPackageBundle) MarshalJSON() ([]byte, error) {
	type Alias WorkPackageBundle
	aux := struct {
		Alias
		ImportSegmentData [][]string `json:"import_segments"`
	}{Alias: (Alias)(b)}

	aux.ImportSegmentData = make([][]string, len(b.ImportSegmentData))
	for i, outer := range b.ImportSegmentData {
		aux.ImportSegmentData[i] = make([]string, len(outer))
		for j, segment := range outer {
			aux.ImportSegmentData[i][j] = fmt.Sprintf("0x%x", segment)
		}
	}
	return json.Marshal(aux)
}

func (b *WorkPackageBundle) UnmarshalJSON(data []byte) error {
	type Alias WorkPackageBundle
	aux := struct {
		Alias
		ImportSegmentData [][]string `json:"import_segments"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*b = WorkPackageBundle(aux.Alias)

	b.ImportSegmentData = make([][][]byte, len(aux.ImportSegmentData))
	for i, outer := range aux.ImportSegmentData {
		b.ImportSegmentData[i] = make([][]byte, len(outer))
		for j, hexStr := range outer {
			b.ImportSegmentData[i][j] = common.FromHex(hexStr)
		}
	}
	return nil
}
