package types

import (
	"github.com/colorfulnotion/jam/common"
	// "encoding/json"
)

// sharing (justified) DA chunks:  Vec<Hash> ++ Blob ++ Vec<Hash> ++ Vec<SegmentChunk> ++ Vec<Hash>.
// The Vec<Hash> will just be complementary Merkle-node-hashes from leaf to root.
// The first will contain hashes for the blob-subtree, the second for the segments subtree and the third for the super-tree.

type AvailabilityJustification struct {
	BlobSubtreeJustification     []common.Hash `json:"blob_subtree_proof"`
	Blob                         []byte        `json:"blob"`
	SegmentsSubtreeJustification []common.Hash `json:"segments_subtree_justification"`
	SegmentChunks                [][]byte      `json:"segment_chunks"`
	SuperTreeJustification       []common.Hash `json:"supertree_justification"`
}
