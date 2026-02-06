package trie

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jam-duna/jamduna/common"
)

// Full137Resp defines the structure for test cases.
type Full137Resp struct {
	TreeLen        int
	ShardIdx       int
	ErasureRoot    common.Hash
	LeafHash       []byte
	Path           [][]byte
	EncodedPath    []byte
	BundleShard    []byte
	ExportedShards []byte
}

// Bundle138Resp defines the structure for bundle test cases.
type Bundle138Resp struct {
	TreeLen     int
	ShardIdx    int
	ErasureRoot common.Hash
	LeafHash    []byte
	Path        [][]byte
	EncodedPath []byte
	BundleShard []byte
	Sclub       []byte
}

type Segment139Resp struct {
	ShardIdx       uint16
	SegmentShards  []byte
	Justifications [][]byte
}

// MarshalJSON custom marshaler for Full137Resp.
func (f *Full137Resp) MarshalJSON() ([]byte, error) {
	type Aux struct {
		TreeLen        int         `json:"treeLen"`
		ShardIdx       int         `json:"shardIdx"`
		ErasureRoot    common.Hash `json:"erasureRoot"` // common.Hash marshals to hex automatically
		LeafHash       string      `json:"leafHash,omitempty"`
		Path           []string    `json:"path,omitempty"`
		EncodedPath    string      `json:"encodedPath,omitempty"`
		BundleShard    string      `json:"bundleShard,omitempty"`
		ExportedShards string      `json:"exportedShards,omitempty"`
	}

	aux := Aux{
		TreeLen:     f.TreeLen,
		ShardIdx:    f.ShardIdx,
		ErasureRoot: f.ErasureRoot,
	}

	if f.LeafHash == nil {
		aux.LeafHash = "" // Ensure omitempty works for nil slices
	} else {
		aux.LeafHash = common.Bytes2String(f.LeafHash) // common.Bytes2String handles empty slices (e.g. to "0x")
	}
	if f.EncodedPath == nil {
		aux.EncodedPath = ""
	} else {
		aux.EncodedPath = common.Bytes2String(f.EncodedPath)
	}
	if f.BundleShard == nil {
		aux.BundleShard = ""
	} else {
		aux.BundleShard = common.Bytes2String(f.BundleShard)
	}
	if f.ExportedShards == nil {
		aux.ExportedShards = ""
	} else {
		aux.ExportedShards = common.Bytes2String(f.ExportedShards)
	}

	if f.Path != nil {
		aux.Path = make([]string, len(f.Path))
		for i, p := range f.Path {
			if p == nil {
				aux.Path[i] = ""
			} else {
				aux.Path[i] = common.Bytes2String(p)
			}
		}
	}

	return json.Marshal(aux)
}

// UnmarshalJSON custom unmarshaler for Full137Resp.
func (f *Full137Resp) UnmarshalJSON(data []byte) error {
	aux := &struct {
		TreeLen        int         `json:"treeLen"`
		ShardIdx       int         `json:"shardIdx"`
		ErasureRoot    common.Hash `json:"erasureRoot"`
		LeafHash       string      `json:"leafHash,omitempty"`
		Path           []string    `json:"path,omitempty"`
		EncodedPath    string      `json:"encodedPath,omitempty"`
		BundleShard    string      `json:"bundleShard,omitempty"`
		ExportedShards string      `json:"exportedShards,omitempty"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal Full137Resp JSON: %w", err)
	}

	f.TreeLen = aux.TreeLen
	f.ShardIdx = aux.ShardIdx
	f.ErasureRoot = aux.ErasureRoot

	if aux.LeafHash != "" {
		f.LeafHash = common.FromHex(aux.LeafHash)
	} else {
		f.LeafHash = nil // Explicitly set to nil if string is empty
	}
	if aux.EncodedPath != "" {
		f.EncodedPath = common.FromHex(aux.EncodedPath)
	} else {
		f.EncodedPath = nil
	}
	if aux.BundleShard != "" {
		f.BundleShard = common.FromHex(aux.BundleShard)
	} else {
		f.BundleShard = nil
	}
	if aux.ExportedShards != "" {
		f.ExportedShards = common.FromHex(aux.ExportedShards)
	} else {
		f.ExportedShards = nil
	}
	if aux.Path != nil {
		f.Path = make([][]byte, len(aux.Path))
		for i, hexP := range aux.Path {
			if hexP != "" {
				f.Path[i] = common.FromHex(hexP)
			} else {
				f.Path[i] = nil
			}
		}
	} else {
		f.Path = nil
	}
	return nil
}

// MarshalJSON custom marshaler for Bundle138Resp.
func (b *Bundle138Resp) MarshalJSON() ([]byte, error) {
	aux := &struct {
		TreeLen     int         `json:"treeLen"`
		ShardIdx    int         `json:"shardIdx"`
		ErasureRoot common.Hash `json:"erasureRoot"`
		LeafHash    string      `json:"leafHash,omitempty"`
		Path        []string    `json:"path,omitempty"`
		EncodedPath string      `json:"encodedPath,omitempty"`
		BundleShard string      `json:"bundleShard,omitempty"`
		Sclub       string      `json:"sclub,omitempty"`
	}{
		TreeLen:     b.TreeLen,
		ShardIdx:    b.ShardIdx,
		ErasureRoot: b.ErasureRoot,
	}

	if b.LeafHash == nil {
		aux.LeafHash = ""
	} else {
		aux.LeafHash = common.Bytes2String(b.LeafHash)
	}
	if b.EncodedPath == nil {
		aux.EncodedPath = ""
	} else {
		aux.EncodedPath = common.Bytes2String(b.EncodedPath)
	}
	if b.BundleShard == nil {
		aux.BundleShard = ""
	} else {
		aux.BundleShard = common.Bytes2String(b.BundleShard)
	}
	if b.Sclub == nil {
		aux.Sclub = ""
	} else {
		aux.Sclub = common.Bytes2String(b.Sclub)
	}

	if b.Path != nil {
		aux.Path = make([]string, len(b.Path))
		for i, p := range b.Path {
			if p == nil {
				aux.Path[i] = ""
			} else {
				aux.Path[i] = common.Bytes2String(p)
			}
		}
	}

	return json.Marshal(aux)
}

// UnmarshalJSON custom unmarshaler for Bundle138Resp.
func (b *Bundle138Resp) UnmarshalJSON(data []byte) error {
	aux := &struct {
		TreeLen     int         `json:"treeLen"`
		ShardIdx    int         `json:"shardIdx"`
		ErasureRoot common.Hash `json:"erasureRoot"`
		LeafHash    string      `json:"leafHash,omitempty"`
		Path        []string    `json:"path,omitempty"`
		EncodedPath string      `json:"encodedPath,omitempty"`
		BundleShard string      `json:"bundleShard,omitempty"`
		Sclub       string      `json:"sclub,omitempty"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal Bundle138Resp JSON: %w", err)
	}

	b.TreeLen = aux.TreeLen
	b.ShardIdx = aux.ShardIdx
	b.ErasureRoot = aux.ErasureRoot

	if aux.LeafHash != "" {
		b.LeafHash = common.FromHex(aux.LeafHash)
	} else {
		b.LeafHash = nil
	}
	if aux.EncodedPath != "" {
		b.EncodedPath = common.FromHex(aux.EncodedPath)
	} else {
		b.EncodedPath = nil
	}
	if aux.BundleShard != "" {
		b.BundleShard = common.FromHex(aux.BundleShard)
	} else {
		b.BundleShard = nil
	}
	if aux.Sclub != "" {
		b.Sclub = common.FromHex(aux.Sclub)
	} else {
		b.Sclub = nil
	}

	if aux.Path != nil {
		b.Path = make([][]byte, len(aux.Path))
		for i, hexP := range aux.Path {
			if hexP != "" {
				b.Path[i] = common.FromHex(hexP)
			} else {
				b.Path[i] = nil
			}
		}
	} else {
		b.Path = nil
	}
	return nil
}

func (f *Full137Resp) String() string {
	b, _ := f.MarshalJSON()
	return string(b)
}

func (f *Bundle138Resp) String() string {
	b, _ := f.MarshalJSON()
	return string(b)
}

func ParseExportedRoots(filePath string) ([]common.Hash, error) {
	// Read the entire content of the file.
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	// Declare a slice of common.Hash to hold the unmarshaled data.
	var roots []common.Hash

	err = json.Unmarshal(data, &roots)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON data from '%s': %w", filePath, err)
	}
	return roots, nil
}
