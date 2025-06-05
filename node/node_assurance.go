package node

import (
	"context"
	"fmt"
	rand0 "math/rand"
	"os"
	"slices"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/trie"
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

// For generating assurance extrinsic
func (n *Node) generateAssurance(headerHash common.Hash, timeslot uint32) (a types.Assurance, numCores uint16) {
	// this will generate an assurance based on RECENT work packages (based on some block timeslot)
	wph := n.statedb.GetJamState().GetRecentWorkPackagesFromRho(timeslot)
	numCores = 0
	for core, wph := range wph {
		if n.isAssuring(wph) {
			a.SetBitFieldBit(core, true)
			numCores++
		}
	}
	if numCores == 0 {
		return a, numCores
	}
	a.Anchor = headerHash
	a.ValidatorIndex = n.id
	a.Sign(n.GetEd25519Secret())
	return
}

// upon audit, this does CE138 AND CE139 calls to ALL Assurers
func (n *NodeContent) FetchAllBundleAndSegmentShards(coreIdx uint16, erasureRoot common.Hash, exportedSegmentLength uint16, verify bool) {
	// Auditor -> Assurer CE138
	shardIdxMap := make(map[uint16]uint16) // shardIdx -> validatorIdx
	for vIdx := range types.TotalValidators {
		shardIdx := ComputeShardIndex(coreIdx, uint16(vIdx))
		shardIdxMap[shardIdx] = uint16(vIdx)
	}
	resps := make([]trie.Bundle138Resp, types.TotalValidators)
	for i := range types.TotalValidators {
		shardIdx := uint16(i)
		vIdx := shardIdxMap[shardIdx]
		//bundleShard []byte, sClub common.Hash, encodedPath []byte, err error
		bundleShard, sClub, encodedPath, err := n.peersInfo[vIdx].SendBundleShardRequest(context.TODO(), erasureRoot, shardIdx)
		if err == nil {
			resps[shardIdx] = trie.Bundle138Resp{
				TreeLen:     types.TotalValidators,
				ShardIdx:    int(shardIdx),
				ErasureRoot: erasureRoot,
				LeafHash:    nil, // TODO: compute leaf hash
				Path:        nil, // TODO
				EncodedPath: encodedPath,
				BundleShard: bundleShard,
				Sclub:       sClub.Bytes(),
			}
			if verify {
				VerifyBundleShard(erasureRoot, shardIdx, bundleShard, sClub, encodedPath)
			}
		}
	}
	fn := fmt.Sprintf("bundle138resp-%d.json", exportedSegmentLength)
	os.WriteFile(fn, []byte(types.ToJSON(resps)), 0644)
	// Guarantor -> Assurer CE139
	if exportedSegmentLength > 0 {
		segmentsPerPageProof := uint16(64) // TODO: make this a constant
		allsegmentindices := make([]uint16, exportedSegmentLength)
		proofpages := make([]uint16, 0)
		for i := range exportedSegmentLength {
			allsegmentindices[i] = i
			p := i / segmentsPerPageProof
			if !slices.Contains(proofpages, p) {
				proofpages = append(proofpages, p)
			}
		}
		for _, p := range proofpages {
			allsegmentindices = append(allsegmentindices, exportedSegmentLength+p)
		}
		resps := make([]trie.Segment139Resp, types.TotalValidators)
		for i := range types.TotalValidators {
			shardIdx := uint16(i)
			vIdx := shardIdxMap[shardIdx]
			// segmentShards []byte, justifications [][]byte, err error
			segmentShards, justifications, err := n.peersInfo[vIdx].SendSegmentShardRequest(context.TODO(), erasureRoot, shardIdx, allsegmentindices, verify)
			if err == nil {
				resps[i] = trie.Segment139Resp{
					ShardIdx:       shardIdx,
					SegmentShards:  segmentShards,
					Justifications: justifications,
				}
			} else {
				log.Warn(debugDA, "assureData: SendSegmentShardRequest failed",
					"validatorIdx", vIdx,
					"shardIdx", shardIdx,
					"err", err)
			}
		}
		fn := fmt.Sprintf("segment139resp-%d.json", exportedSegmentLength)
		os.WriteFile(fn, []byte(types.ToJSON(resps)), 0644)
	}
}

func SelectGuarantor(g types.Guarantee, blacklistIdx []uint16) (uint16, error) {
	// Select a random guarantor from the signatures
	guarantorList := make([]uint16, 0)
	for _, sig := range g.Signatures {
		garantorIdx := sig.ValidatorIndex
		for _, blacklisted := range blacklistIdx {
			if garantorIdx == blacklisted {
				continue
			}
		}
		guarantorList = append(guarantorList, garantorIdx)
	}
	if len(guarantorList) == 0 {
		err := fmt.Errorf("SelectGuarantor: No valid guarantors found in guarantee signatures for guarantee")
		log.Error(debugDA, "SelectGuarantor: No valid guarantors found in guarantee signatures", "guarantee", "reportHash", g.Report.Hash(), "error", err)
		return 0, err
	}
	return guarantorList[rand0.Intn(len(guarantorList))], nil
}

// Assurer -> Guarantor
func (n *Node) FetchAllFullShards(g types.Guarantee, verify bool) {
	spec := g.Report.AvailabilitySpec
	coreIdx := g.Report.CoreIndex
	vIdx := n.id
	blacklistID := []uint16{5}
	guarantor, err := SelectGuarantor(g, blacklistID)
	if err != nil {
		return
	}
	resps := make([]trie.Full137Resp, types.TotalValidators)
	fmt.Printf("FetchAllFullShards: coreIdx=%d, vIdx=%d, guarantor=%d, erasureRoot=%s\n", coreIdx, vIdx, guarantor, spec.ErasureRoot)
	for i := 0; i < types.TotalValidators; i++ {
		shardIdx := uint16(i)
		bundleShard, exportedShards, encodedPath, err := n.peersInfo[guarantor].SendFullShardRequest(context.TODO(), spec.ErasureRoot, shardIdx)
		if err == nil {
			resps[int(shardIdx)] = trie.Full137Resp{
				TreeLen:        types.TotalValidators,
				ShardIdx:       int(shardIdx),
				ErasureRoot:    spec.ErasureRoot,
				BundleShard:    bundleShard,
				ExportedShards: exportedShards,
				EncodedPath:    encodedPath,
			}
			log.Trace(debugDA, "FetchAllFullShards: SendFullShardRequest success CE137",
				"coreIdx", coreIdx,
				"validatorIdx", vIdx,
				"shardIdx", shardIdx,
			)
			if verify {
				VerifyFullShard(spec.ErasureRoot, shardIdx, bundleShard, exportedShards, encodedPath)
			}
		}
	}
	// save resps to fn in JSON
	fn := fmt.Sprintf("full137resp-%d.json", g.Report.AvailabilitySpec.ExportedSegmentLength)
	os.WriteFile(fn, []byte(types.ToJSON(resps)), 0644)
}

const attemptReconstruction = true

// assureData, given a Guarantee with an AvailabilitySpec within a WorkReport,
// fetches the bundleShard and segmentShards and stores in ImportDA + AuditDA.
func (n *Node) assureData(ctx context.Context, g types.Guarantee) error {
	spec := g.Report.AvailabilitySpec
	coredIdx := g.Report.CoreIndex
	vIdx := n.id
	shardIdx := ComputeShardIndex(uint16(coredIdx), vIdx) // shardIdx != validatorIdx

	const maxRetries = 3
	var bundleShard []byte
	var exportedShards []byte
	var encodedPath []byte
	var err error
	var guarantor uint16

	// Get All Shards
	if attemptReconstruction {
		n.FetchAllFullShards(g, true)
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		randomguarantor := rand0.Intn(len(g.Signatures))
		guarantor = g.Signatures[randomguarantor].ValidatorIndex

		bundleShard, exportedShards, encodedPath, err = n.peersInfo[guarantor].SendFullShardRequest(ctx, spec.ErasureRoot, shardIdx)
		if err == nil {
			log.Trace(debugDA, "assureData: SendFullShardRequest success",
				"coreIdx", coredIdx, "validatorIdx", vIdx, "shardIdx", shardIdx,
				"erasureRoot", spec.ErasureRoot,
				"guarantor", guarantor,
				"bundleShard", fmt.Sprintf("%x", bundleShard))
			break
		}
		log.Warn(debugDA, "assureData: SendFullShardRequest attempt failed",
			"coredIdx", coredIdx, "validatorIdx", vIdx, "shardIdx", shardIdx,
			"attempt", attempt, "n", n.String(), "erasureRoot", spec.ErasureRoot,
			"guarantor", guarantor, "err", err)
	}

	if err != nil {
		log.Error(debugDA, "assureData: SendFullShardRequest failed after retries",
			"coredIdx", coredIdx, "shardIdx", "validatorIdx", vIdx, "shardIdx", shardIdx,
			"n", n.String(), "erasureRoot", spec.ErasureRoot,
			"guarantor", guarantor, "err", err)
		return fmt.Errorf("SendFullShardRequest (after retries): %w", err)
	}

	// CRITICAL: verify justification matches the erasure root before storage
	verified, err := VerifyFullShard(spec.ErasureRoot, shardIdx, bundleShard, exportedShards, encodedPath)
	if err != nil {
		log.Error(debugDA, "assureData: VerifyFullShard error", "coredIdx", coredIdx, "validatorIdx", vIdx, "shardIdx", shardIdx, "n", n.String(), "err", err)
		return fmt.Errorf("VerifyFullShard: %w", err)
	}
	if !verified {
		log.Error(debugDA, "assureData: VerifyFullShard failed", "coredIdx", coredIdx, "validatorIdx", vIdx, "shardIdx", shardIdx, "n", n.String(), "verified", false)
		return fmt.Errorf("VerifyFullShard: failed verification")
	}

	if err := n.StoreFullShard_Assurer(spec.ErasureRoot, shardIdx, bundleShard, exportedShards, encodedPath); err != nil {
		return fmt.Errorf("StoreFullShard_Assurer: %w", err)
	}

	if err := n.StoreWorkReport(g.Report); err != nil {
		log.Error(debugDA, "assureData: StoreWorkReport failed", "coredIdx", coredIdx, "shardIdx", "validatorIdx", vIdx, "shardIdx", shardIdx, "n", n.String(), "err", err)
		return fmt.Errorf("StoreWorkReport: %w", err)
	}

	n.markAssuring(spec.WorkPackageHash)

	return nil
}
