package node

import (
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
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

	// concatSegmentShards refers to the EC shards of the EXPORTED segments AND PROOF PAGES shards from the guaranteed work package
	guarantor := g.Signatures[0].ValidatorIndex
	bundleShard, exported_segments_and_proofpageShards, encodedPath, err := n.peersInfo[guarantor].SendFullShardRequest(spec.ErasureRoot, n.id)
	if err != nil {
		log.Error(debugDA, "assureData:SendFullShardRequest", "n", n.String(), "err", err)
		return
	}
	// CRITICAL: PRIOR to StoreFullShard_Assurer:  exported_segments_and_proofpageShards with the justification (provided by one of the 2-3 guarantors) to ensure it matches the erasureRoot
	verified, err := VerifyFullShard(spec.ErasureRoot, n.id, bundleShard, exported_segments_and_proofpageShards, encodedPath)
	if err != nil || !verified {
		log.Error(debugDA, "assureData:VerifyFullShard", "n", n.String(), "err", err)
		return
	}

	err = n.StoreFullShard_Assurer(spec.ErasureRoot, n.id, bundleShard, exported_segments_and_proofpageShards, encodedPath)
	if err != nil {
		return
	}

	err = n.StoreWorkReport(g.Report)
	if err != nil {
		log.Error(debugDA, "assureData:StoreWorkReport", "n", n.String(), "err", err)
		return
	}

	n.markAssuring(spec.WorkPackageHash)

	return nil
}
