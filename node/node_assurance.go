package node

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) markAssuring(workPackageHash common.Hash) {
	n.assuranceMutex.Lock()
	defer n.assuranceMutex.Unlock()
	n.assurancesBucket[workPackageHash] = true
}
func (n *Node) isAssuring(workPackageHash common.Hash) bool {
	n.assuranceMutex.Lock()
	defer n.assuranceMutex.Unlock()
	_, ok := n.assurancesBucket[workPackageHash]
	return ok
}

func (n *Node) GenerateAssurance() (a types.Assurance, err error) {
	reports, err := n.statedb.GetJamState().GetWorkReportFromRho()
	if err != nil {
		return
	}
	for _, r := range reports {
		wph := r.AvailabilitySpec.WorkPackageHash
		isA := n.isAssuring(wph)
		if isA {
			a.SetBitFieldBit(r.CoreIndex, isA)
		}
	}
	a.Anchor = n.statedb.GetBlock().ParentHash()
	a.ValidatorIndex = n.statedb.Id
	a.Sign(n.GetEd25519Secret())
	return
}

// assureData, given a Guarantee with a AvailabiltySpec within a WorkReport, fetches the bundleShard and segmentShards and stores in ImportDA + AuditDA
func (n *Node) assureData(g types.Guarantee) (err error) {
	spec := g.Report.AvailabilitySpec
	erasureRoot := spec.ErasureRoot
	guarantor := g.Signatures[0].ValidatorIndex // TODO: try any of them, not the 0th one
	bundleShard, concatSegmentShards, justification, err := n.peersInfo[guarantor].SendFullShardRequest(erasureRoot, n.id)
	if err != nil {
		fmt.Printf("%s assureData: SendShardRequest %v\n", n.String(), err)
		return
	}
	segmentShards, err := SplitToSegmentShards(concatSegmentShards)
	if err != nil {
		fmt.Printf("%s assureData: SplitAsSegmentShards %v\n", n.String(), err)
		return
	}

	n.StoreFullShard_Assurer(erasureRoot, n.id, bundleShard, segmentShards, justification)
	// TODO: is this verified? check err

	n.markAssuring(spec.WorkPackageHash)

	a, err := n.GenerateAssurance()
	if err != nil {
		return
	}
	if debugA {
		fmt.Printf("%s [assureData] Broadcasting assurance CORE %d: bitfield=%x\n", n.String(), g.Report.CoreIndex, a.Bitfield)
	}
	n.broadcast(a)
	return nil
}
