package types

import (
	"github.com/jam-duna/jamduna/common"
)

type PageProof struct {
	JustificationX []common.Hash `json:"j_x"`
	LeafHashes     []common.Hash `json:"l_x"`
}

type PolkaJamPageProof struct {
	JustificationX []common.Hash `json:"j_x"`
	LeafHashes     []common.Hash `json:"l_x"`
}

/*
func (p PageProof) Encode() []byte {
	return p.Bytes()
}

func (p PageProof) Decode(data []byte) (interface{}, uint32) {
	pageproof, leng, _ := DecodeProof(data)
	return *pageproof, leng
}

func DecodeProof(remaining []byte) (*PageProof, uint32, error) {
	pp := PageProof{}
	// Decode the JustificationX
	justificationX, length, err := Decode(remaining, reflect.TypeOf([]common.Hash{}))
	if err != nil {
		return nil, length, err
	}
	pp.JustificationX = justificationX.([]common.Hash)
	remaining = remaining[length:]
	if !GPCompliant {
		leafHashesLen := len(remaining) / 32
		lengthdiscriminator := E(uint64(leafHashesLen))
		remaining = append(lengthdiscriminator, remaining...)
	}
	leafHashes, length, err := Decode(remaining, reflect.TypeOf([]common.Hash{}))
	if err != nil {
		return nil, length, err
	}
	pp.LeafHashes = leafHashes.([]common.Hash)
	return &pp, length, nil
}

func (p *PageProof) Bytes() []byte {
	globalJustificationEncode, _ := Encode(p.JustificationX)
	localHashedLeavesEncode, _ := Encode(p.LeafHashes)
	if GPCompliant {
		return append(globalJustificationEncode, localHashedLeavesEncode[:]...)
	} else {
		return append(globalJustificationEncode, localHashedLeavesEncode[1:]...)
	}
}
*/
