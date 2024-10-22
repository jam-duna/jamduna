package node

import (
	"fmt"
	"github.com/colorfulnotion/jam/types"
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
	if debugDA {
		fmt.Printf("%s Core: %d, WorkPackageHash=%v, len(coworker)=%x\n", n.String(), coreIndex, wp.Hash(), len(coworker))
	}
	for id, p := range n.peersInfo {
		for _, worker := range coworker {
			if worker.Ed25519 == p.Validator.Ed25519 {
				if id == n.id {
					continue
				}
				//fmt.Printf("broadcastWorkPackage wp=%v", wp.String())
				bundle := n.CompilePackageBundle(wp)
				// TODO: parallelize the RefineBundle with the 2 ShareWorkPackage calls -- whichever one matches our
				fellowWorkReportHash, fellowSignature, errfellow := p.ShareWorkPackage(coreIndex, bundle.Bytes(), p.Validator.Ed25519)
				if errfellow != nil {
					fmt.Printf("ShareWorkPackage ERR in broadcastWorkpackage: %v\n", err)
					//try the next one
				}
				if !doneExecute {
					guarantee, _, _, err = n.executeWorkPackage(wp)
					doneExecute = true
					if debugG {
						fmt.Printf("%s [broadcastWorkPackage] Guarantee from self\n", n.String())
					}
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
					n.broadcast(guarantee)
					return
				} else {
					fmt.Printf("%s [broadcastWorkPackage] Guarantee from fellow did not match!\n", n.String())
				}
			}
		}
	}
	return
}
