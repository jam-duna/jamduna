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

func (n *Node) generateAssurance(headerHash common.Hash) (a types.Assurance, numCores uint16, err error) {
	reports, err := n.statedb.GetJamState().GetWorkReportFromRho()
	if err != nil {
		return
	}
	numCores = 0
	for _, r := range reports {
		// let fib send that bit later by setting delaysend
		no_delay := true
		// if r.AvailabilitySpec.WorkPackageHash is in node.delaysend
		// then no_delay = false
		if _, ok := n.delaysend[r.AvailabilitySpec.WorkPackageHash]; ok {
			// see the value of the delay
			// if delay is 0, then no_delay = true
			if n.delaysend[r.AvailabilitySpec.WorkPackageHash] == 0 {
				no_delay = true
				// delete the key from node.delaysend
				delete(n.delaysend, r.AvailabilitySpec.WorkPackageHash)
			} else {
				no_delay = false
				n.delaysend[r.AvailabilitySpec.WorkPackageHash]--
			}
		}
		if n.isAssuring(r.AvailabilitySpec.WorkPackageHash) && no_delay {
			a.SetBitFieldBit(r.CoreIndex, true)
			numCores++
		}
	}
	if numCores == 0 {
		return a, numCores, nil
	}
	a.Anchor = headerHash
	a.ValidatorIndex = n.id
	a.Sign(n.GetEd25519Secret())

	return
}

// assureData, given a Guarantee with a AvailabiltySpec within a WorkReport, fetches the bundleShard and segmentShards and stores in ImportDA + AuditDA
func (n *Node) assureData(g types.Guarantee) (err error) {
	spec := g.Report.AvailabilitySpec
	erasureRoot := spec.ErasureRoot

	guarantor := g.Signatures[0].ValidatorIndex // TODO: try any of them, not the 0th one
	bundleShard, concatSegmentShards, justification, err := n.peersInfo[guarantor].SendFullShardRequest(erasureRoot, n.id)
	fullshard_identifier := fmt.Sprintf("%v_%d", erasureRoot, n.id)
	if err != nil {
		fmt.Printf("%s [assureData: SendShardRequest] ERR %v\n", n.String(), err)
		return
	}
	segmentShards, err := SplitToSegmentShards(concatSegmentShards)
	if err != nil {
		fmt.Printf("%s [assureData: SplitAsSegmentShards] ERR %v\n", n.String(), err)
		return
	}
	verified, err := VerifyFullShard(erasureRoot, n.id, bundleShard, segmentShards, justification)
	if err != nil || !verified {
		fmt.Printf("%s [assureData:VerifyFullShard] ERR %v verified %v\n", n.String(), err, verified)
		return
	}
	if debugDA {
		fmt.Printf("%s [assureData:VerifyFullShard] %v verified %v\n", n.String(), verified, fullshard_identifier)
	}

	if debugKV {
		fmt.Printf("N%d StoreFullShard_AssurererasureRoot %v\n", n.id, erasureRoot)
	}
	err = n.StoreFullShard_Assurer(erasureRoot, n.id, bundleShard, segmentShards, justification)
	if err != nil {
		return
	}

	err = n.StoreImportDAWorkReportMap(spec)
	if err != nil {
		fmt.Printf("%s [assureData:StoreImportDAWorkReportMap] ERR %v\n", n.String(), err)
		return
	}

	n.markAssuring(spec.WorkPackageHash)

	return nil
}
