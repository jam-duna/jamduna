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

/*
wr, _, pvmElapsed, err := n.executeWorkPackageBundle(uint16(workReport.CoreIndex), workPackageBundle, workReport.SegmentRootLookup, n.statedb.GetTimeslot(), false)
	if err != nil {
		return
	}

func (n *NodeContent) executeWorkPackageBundle(
	workPackageCoreIndex uint16,
	package_bundle types.WorkPackageBundle,
	segmentRootLookup types.SegmentRootLookup,
	slot uint32, firstGuarantorOrAuditor bool,
) (
	work_report types.WorkReport,
	d AvailabilitySpecifierDerivation,
	elapsed uint32,
	err error) {
}

*/
