package types

import (
	"github.com/colorfulnotion/jam/common"
)

// AvailabilitySpecification represents the set of availability specifications.
type AvailabilitySpecification struct {
	WorkPackageHash common.Hash `json:"work_package_hash"`
	BundleLength    int         `json:"bundle_length"`
	// root of a binary Merkle tree which functions as a commitment to all data required for the auditing of the report and for use by later workpackages should they need to retrieve any data yielded
	ErasureRoot common.Hash `json:"erasure_root"`
	// root of a constant-depth, left-biased and zero-hash-padded binary Merkle tree committing to the hashes of each of the exported segments of each work-item
	SegmentRoot common.Hash `json:"segment_root"`
}
