package node

import (
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) broadcastWorkpackage(wp types.WorkPackage, wpCoreIndex uint16, curr_statedb *statedb.StateDB) (guarantee types.Guarantee, err error) {
	currTimeslot := curr_statedb.GetTimeslot()
	coreIndex := wpCoreIndex
	if err != nil {
		fmt.Printf("coreBroadcast Error: %v\n", err)
		return
	}
	doneExecute := false
	coworker := curr_statedb.GetCoreCoWorkers(coreIndex)
	if debugDA {
		fmt.Printf("%s n.Core: %d, | wpCoreIndex=%v, WorkPackageHash=%v, len(coworker)=%x\n", n.String(), coreIndex, wpCoreIndex, wp.Hash(), len(coworker))
	}
	if debugSegments {
		fmt.Printf("[N%d] broadcastWorkpackage executeWorkPackage\n", n.id)
	}
	importedSegments, err := n.FetchWorkpackageImportSegments(wp)
	if err != nil {
		// fmt.Printf("FetchWorkpackageImportSegments Error: %v\n", err)
	}
	segmentRootLookup, err := n.GetSegmentRootLookup(wp)
	if err != nil {
		fmt.Printf("FetchWorkpackageImportSegments GetSegmentRootLookup Error: %v\n", err)
	}
	guarantee, _, _, err = n.executeWorkPackage(wpCoreIndex, wp, importedSegments, segmentRootLookup)
	doneExecute = true
	if debugG {
		fmt.Printf("%s [broadcastWorkPackage] Guarantee from self\n", n.String())
	}
	bundle := n.CompilePackageBundle(wp, importedSegments)
	for _, p := range n.peersInfo {
		for _, worker := range coworker {
			if worker.Ed25519 == p.Validator.Ed25519 && worker.Ed25519 != n.credential.Ed25519Pub {
				//fmt.Printf("broadcastWorkPackage wp=%v", wp.String())
				// TODO: parallelize the RefineBundle with the 2 ShareWorkPackage calls -- whichever one matches our
				fellowWorkReportHash, fellowSignature, errfellow := p.ShareWorkPackage(wpCoreIndex, bundle, segmentRootLookup, p.Validator.Ed25519)
				if errfellow != nil {
					fmt.Printf("ShareWorkPackage ERR in broadcastWorkpackage: %v\n", err)
					//try the next one
				}
				if !doneExecute {
					panic("executeWorkPackage not done")
				}
				selfReport := guarantee.Report
				selfWorkReportHash := guarantee.Report.Hash()
				validator_idx := curr_statedb.GetSafrole().GetCurrValidatorIndex(p.Validator.Ed25519)
				if validator_idx == -1 {
					panic("validator_idx not found")
				}
				if selfWorkReportHash == fellowWorkReportHash {
					guarantee.Signatures = append(guarantee.Signatures, types.GuaranteeCredential{
						ValidatorIndex: uint16(validator_idx),
						Signature:      fellowSignature,
					})
					sort.Slice(guarantee.Signatures, func(i, j int) bool {
						return guarantee.Signatures[i].ValidatorIndex < guarantee.Signatures[j].ValidatorIndex
					})
					if debugG {
						fmt.Printf("%s [broadcastWorkPackage] Guarantee from fellow.. broadcast\n", n.String())
					}

				} else {
					fmt.Printf("%s [broadcastWorkPackage] outgoing workReport: %v\n", n.String(), selfReport.String())
					fmt.Printf("%s [broadcastWorkPackage] outgoing guarantee: %v\n", n.String(), guarantee.String())
					fmt.Printf("%s [broadcastWorkPackage] Guarantee from fellow %s did not match! \neg_wr: %v, fellow_wr: %v\n", n.String(), p.node.String(), selfWorkReportHash, fellowWorkReportHash)
					//panic("Guarantee from fellow did not match!")
					return
				}
			}
		}
	}
	if len(guarantee.Signatures) < 3 {
		//TODO: Shawn - if more than 2s has passed after receiving 2nd sig, you can potentiall move on.
		panic(222)
	} else {
		guarantee.Slot = currTimeslot
		n.broadcast(guarantee)
		n.processGuarantee(guarantee)
		fmt.Printf("%s (core %d) [broadcast guarantee in slot %d]\n", n.String(), coreIndex, guarantee.Slot)
	}
	return
}
