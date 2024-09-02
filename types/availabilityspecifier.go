package types

import (
	"github.com/colorfulnotion/jam/common"
)

// EQ(186):Availability Specifier
type AvailabilitySpecifier struct {
	WorkPackageHash common.Hash `json:"work_package_hash"` // the hash of the workpackage
	BundleLength    uint32      `json:"bundle_length"`     // the length of the AuditFriendlyWorkPackage
	// root of a binary Merkle tree which functions as a commitment to all data required for the auditing of the report and for use by later workpackages should they need to retrieve any data yielded
	ErasureRoot common.Hash `json:"erasure_root"` // The root of a transport (AuditFriendlyWorkPackage Hashed and segment) encoding which is built by CDT
	// root of a constant-depth, left-biased and zero-hash-padded binary Merkle tree committing to the hashes of each of the exported segments of each work-item
	SegmentRoot      common.Hash   `json:"segment_root"`
	ExportedSegments []common.Hash // the exported segments root which is built by WBT
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
