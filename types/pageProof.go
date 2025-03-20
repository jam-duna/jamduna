package types

import "github.com/colorfulnotion/jam/common"

type PageProof struct {
	JustificationX []common.Hash `json:"j_x"`
	LeafHashes     []common.Hash `json:"l_x"`
}
