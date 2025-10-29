package types

import (
	"github.com/colorfulnotion/jam/common"
)

type WorkPackageBundleSnapshot struct {
	PackageHash       common.Hash       `json:"package_hash"`
	CoreIndex         uint16            `json:"core_index"`
	Bundle            WorkPackageBundle `json:"bundle"`
	SegmentRootLookup SegmentRootLookup `json:"segment_root_lookup"`
	Slot              uint32            `json:"slot"`
	Report            WorkReport        `json:"report"`
}
