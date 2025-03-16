package types

import (
	"encoding/json"
	//	"errors"
	//	"fmt"
	//	"reflect"

	"github.com/colorfulnotion/jam/common"
)

// WBT justification helper
type Justification struct {
	Root     common.Hash `json:"root"`
	ShardIdx int         `json:"shard_index"`
	TreeLen  int         `json:"len"`
	//Leaf     []byte        `json:"leaf"`
	LeafHash []byte   `json:"leaf_hash"`
	Path     [][]byte `json:"path"`
}

func (j *Justification) EncodeJustification() ([]byte, error) {
	return common.EncodeJustification(j.Path)
}

func (j *Justification) Marshal() ([]byte, error) {
	return json.Marshal(j)
}

func (j *Justification) Unmarshal(data []byte) error {
	return json.Unmarshal(data, j)
}

func (j *Justification) Bytes() []byte {
	jsonData, _ := j.Marshal()
	return jsonData
}

func (j *Justification) String() string {
	return string(j.Bytes())
}
