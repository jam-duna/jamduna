package node

import (
	"context"
	"fmt"

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
		if n.isAssuring(r.AvailabilitySpec.WorkPackageHash) {
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

// assureData, given a Guarantee with an AvailabilitySpec within a WorkReport,
// fetches the bundleShard and segmentShards and stores in ImportDA + AuditDA.
func (n *Node) assureData(ctx context.Context, g types.Guarantee) error {
	spec := g.Report.AvailabilitySpec
	guarantor := g.Signatures[0].ValidatorIndex

	bundleShard, exportedShards, encodedPath, err := n.peersInfo[guarantor].SendFullShardRequest(ctx, spec.ErasureRoot, n.id)
	if err != nil {
		log.Error(debugDA, "assureData: SendFullShardRequest failed", "n", n.String(), "erasureRoot", spec.ErasureRoot, "guarantor", guarantor, "err", err)
		return fmt.Errorf("SendFullShardRequest: %w", err)
	}

	// CRITICAL: verify justification matches the erasure root before storage
	verified, err := VerifyFullShard(spec.ErasureRoot, n.id, bundleShard, exportedShards, encodedPath)
	if err != nil {
		log.Error(debugDA, "assureData: VerifyFullShard error", "n", n.String(), "err", err)
		return fmt.Errorf("VerifyFullShard: %w", err)
	}
	if !verified {
		log.Error(debugDA, "assureData: VerifyFullShard failed", "n", n.String(), "verified", false)
		return fmt.Errorf("VerifyFullShard: failed verification")
	}

	if err := n.StoreFullShard_Assurer(spec.ErasureRoot, n.id, bundleShard, exportedShards, encodedPath); err != nil {
		return fmt.Errorf("StoreFullShard_Assurer: %w", err)
	}

	if err := n.StoreWorkReport(g.Report); err != nil {
		log.Error(debugDA, "assureData: StoreWorkReport failed", "n", n.String(), "err", err)
		return fmt.Errorf("StoreWorkReport: %w", err)
	}

	n.markAssuring(spec.WorkPackageHash)

	return nil
}
