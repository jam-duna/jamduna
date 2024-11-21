package types

import (
	"github.com/colorfulnotion/jam/common"
)

/* Availability Specifier
WorkPackageHash(h)	: the hash of the workpackage
BundleLength(l)		: the len of the packagebundle
ErasureRoot(u)		: MB([x∣x∈T[b♣,s♣]]) - root of a binary Merkle tree which (WBT) functions as a commitment to all data required for the auditing of the report and for use by later workpackages should they need to retrieve any data yielded.
					  The root of a transport (packagebundle Hashed and segment) encoding which is built by CDT
SegmentRoot(e)		: M(s) - root of a constant-depth, left-biased and zero-hash-padded binary Merkle tree (CDT) committing to the hashes of each of the exported segments of each work-item.
*/

// EQ(186):Availability Specifier
type AvailabilitySpecifier struct {
	WorkPackageHash     common.Hash `json:"hash"`
	BundleLength        uint32      `json:"length"`
	ErasureRoot         common.Hash `json:"erasure_root"`
	ExportedSegmentRoot common.Hash `json:"exports_root"`
	ExportsCount        uint16      `json:"exports_count"`
}

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
