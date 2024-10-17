package node

import (
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"reflect"
	"sort"
)

func (n *Node) broadcastWorkpackage(wp types.WorkPackage) (guarantee types.Guarantee, err error) {

	coreIndex, err := n.GetSelfCoreIndex()
	if err != nil {
		fmt.Printf("coreBroadcast Error: %v\n", err)
		return
	}
	doneExecute := false
	coworker := n.GetCoreCoWorkers(coreIndex)
	for id, p := range n.peersInfo {
		for _, worker := range coworker {
			if worker.Ed25519 == p.Validator.Ed25519 {
				if id == n.id {
					continue
				}
				if immediateAvailability {
					// TODO: parallelize the RefineBundle with the 2 ShareWorkPackage calls -- whichever one matches our
					fellowWorkReportHash, fellowSignature, errfellow := p.ShareWorkPackage(coreIndex, wp.Bytes(), p.Validator.Ed25519)
					if errfellow != nil {
						fmt.Printf("ShareWorkPackage ERR in broadcastWorkpackage: %v\n", err)
						//try the next one
					}
					if !doneExecute {
						guarantee, _, _, err = n.executeWorkPackage(wp)
						doneExecute = true
					}
					if guarantee.Report.Hash() == fellowWorkReportHash {
						guarantee.Signatures = append(guarantee.Signatures, types.GuaranteeCredential{
							ValidatorIndex: id,
							Signature:      fellowSignature,
						})
						sort.Slice(guarantee.Signatures, func(i, j int) bool {
							return guarantee.Signatures[i].ValidatorIndex < guarantee.Signatures[j].ValidatorIndex
						})
						n.broadcast(guarantee)
						return
					}
				} else {
					// TODO: short-term: bundle := CompilePackageBundle(wp).Bytes()
				}
			}
		}
	}
	return
}

func (n *Node) RefineBundle(coreIndex uint16, workpackagehashes, segmentroots []common.Hash, bundle []byte) (guarantee types.Guarantee, err error) {
	if len(bundle) == 0 {
		panic(123)
	}
	var workPackage types.WorkPackage
	if immediateAvailability {
		decoded, _, err := types.Decode(bundle, reflect.TypeOf(types.WorkPackage{}))
		if err != nil {
			panic(125)
		}
		workPackage = decoded.(types.WorkPackage)
	} else {
		bp, err := types.WorkPackageBundleFromBytes(bundle)
		if err != nil {
			panic(123)
		}
		workPackage = bp.WorkPackage
	}
	guarantee, _, _, err = n.executeWorkPackage(workPackage)
	if err != nil {
		return
	}
	//fmt.Printf("%s [RefineBundle:executeWorkPackage] workReportHash %v (spec.workPackageHash: %v)\n", n.String(), workReport.Hash(), workReport.AvailabilitySpec.WorkPackageHash)
	return guarantee, nil
}
