package node

import (
	"fmt"
	"sort"

	"github.com/colorfulnotion/jam/types"
)

func (n *Node) broadcastWorkpackage(wp types.WorkPackage) (guarantee types.Guarantee, err error) {
	coreIndex, err := n.GetSelfCoreIndex()
	if err != nil {
		fmt.Printf("coreBroadcast Error: %v\n", err)
		return
	}
	doneExecute := false
	coworker := n.GetCoreCoWorkers(coreIndex)
	if debugDA {
		fmt.Printf("%s Core: %d, WorkPackageHash=%v, len(coworker)=%x\n", n.String(), coreIndex, wp.Hash(), len(coworker))
	}
	guarantee, _, _, err = n.executeWorkPackage(wp)
	doneExecute = true
	if debugG {
		fmt.Printf("%s [broadcastWorkPackage] Guarantee from self\n", n.String())
	}
	for id, p := range n.peersInfo {
		for _, worker := range coworker {
			if worker.Ed25519 == p.Validator.Ed25519 && worker.Ed25519 != n.credential.Ed25519Pub {
				//fmt.Printf("broadcastWorkPackage wp=%v", wp.String())
				bundle := n.CompilePackageBundle(wp)
				// TODO: parallelize the RefineBundle with the 2 ShareWorkPackage calls -- whichever one matches our
				fellowWorkReportHash, fellowSignature, errfellow := p.ShareWorkPackage(coreIndex, bundle.Bytes(), p.Validator.Ed25519)
				if errfellow != nil {
					fmt.Printf("ShareWorkPackage ERR in broadcastWorkpackage: %v\n", err)
					//try the next one
				}
				if !doneExecute {
					panic("executeWorkPackage not done")
				}
				if guarantee.Report.Hash() == fellowWorkReportHash {
					guarantee.Signatures = append(guarantee.Signatures, types.GuaranteeCredential{
						ValidatorIndex: id,
						Signature:      fellowSignature,
					})
					sort.Slice(guarantee.Signatures, func(i, j int) bool {
						return guarantee.Signatures[i].ValidatorIndex < guarantee.Signatures[j].ValidatorIndex
					})
					if debugG {
						fmt.Printf("%s [broadcastWorkPackage] Guarantee from fellow.. broadcast\n", n.String())
					}

				} else {
					fmt.Printf("%s [broadcastWorkPackage] Guarantee from fellow %s did not match! \neg_wr: %v, fellow_wr: %v\n", n.String(), p.node.String(), guarantee.Report.Hash(), fellowWorkReportHash)
					panic("Guarantee from fellow did not match!")
				}
			}
		}
	}
	if len(guarantee.Signatures) < 3 {
		//TODO: Shawn - if more than 2s has passed after receiving 2nd sig, you can potentiall move on.
		panic(222)
	} else {
		n.broadcast(guarantee)
	}
	return
}
