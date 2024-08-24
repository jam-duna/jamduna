package node

import "github.com/colorfulnotion/jam/common"

type ImportDAQuery struct {
	SegmentRoot  common.Hash `json:"segment_root"`  // The root of the segments tree, which responsible for identifying the exported-segments data
	SegmentIndex uint32      `json:"segment_index"` // The index of the segment in the segments tree
	// ProofRequested bool        `json:"proof_requested"` // Whether the proof chunk need be supplied
}

type ImportDAResponse struct {
	Data      []byte      `json:"data"`       // The reconstructed SegmentIndex-th segment belonging to the segment root specify in the query
	ChunkRoot common.Hash `json:"chunk_root"` // root of the erasure coded chunks of specific segment. This is used to verify the data
}

type ImportDAReconstructQuery struct {
	SegmentRoot common.Hash `json:"segment_root"` // The root of the segments tree, which responsible for identifying the exported-segments data
}

type ImportDAReconstructResponse struct {
	Data []byte `json:"data"` // The reconstructed, concatenated exported-segments data
}

type AuditDAQuery struct { // See 17.2.
	SegmentRoot  common.Hash `json:"segment_root"`  // The root of the segments tree, which responsible for identifying the auditable work-package.
	SegmentIndex uint32      `json:"segment_index"` // The index of the segment in the segments tree
	// ProofRequested bool        `json:"proof_requested"`
}

type AuditDAResponse struct { // See 17.2.
	// Note that for one auditable work-package will contain three different type of chunks: workpackage super-chunks, the self-justifying imports superchunks and the extrinsic segments super-chunks.
	Chunk     []byte      `json:"data"`
	ChunkRoot common.Hash `json:"chunk_root"` // root of the erasure coded chunks of specific segment. This is used to verify the data
}

type DistributeECChunk struct {
	ChunkRoot []byte `json:"chunk_root"` // root of the erasure coded chunks of specific segment
	Data      []byte `json:"data"`       // The [node_id]-th chunk of the segment
}

type ECChunkResponse struct {
	ChunkRoot []byte `json:"chunk_root"` // root of the erasure coded chunks of specific segment
	Data      []byte `json:"data"`       // The [node_id]-th chunk of the segment
}

type ECChunkQuery struct {
	ChunkRoot []byte `json:"chunk_root"` // root of the erasure coded chunks of specific segment
}

type Segment struct {
	Data []byte
}

type PagedProof struct {
	Hashes     [64][32]byte
	MerkleRoot [32]byte
}
