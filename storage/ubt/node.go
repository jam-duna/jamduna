package ubt

import "github.com/jam-duna/jamduna/storage"

type NodeType uint8

const (
	NodeTypeEmpty NodeType = iota
	NodeTypeInternal
	NodeTypeStem
	NodeTypeLeaf
)

// Node is a simple proof/test helper with recursive hashing.
type Node struct {
	Type   NodeType
	Left   *Node
	Right  *Node
	Stem   *Stem
	Values map[uint8][32]byte
	Value  *[32]byte
}

func (n Node) Hash(hasher Hasher) [32]byte {
	switch n.Type {
	case NodeTypeEmpty:
		return [32]byte{}
	case NodeTypeLeaf:
		if n.Value == nil {
			return [32]byte{}
		}
		return hasher.Hash32(n.Value)
	case NodeTypeInternal:
		var leftHash [32]byte
		var rightHash [32]byte
		if n.Left != nil {
			leftHash = n.Left.Hash(hasher)
		}
		if n.Right != nil {
			rightHash = n.Right.Hash(hasher)
		}
		return hasher.Hash64(&leftHash, &rightHash)
	case NodeTypeStem:
		if n.Stem == nil {
			return [32]byte{}
		}
		stemNode := storage.NewStemNode(*n.Stem)
		for idx, val := range n.Values {
			stemNode.SetValue(idx, val)
		}
		return stemNode.Hash(hasher)
	default:
		return [32]byte{}
	}
}
