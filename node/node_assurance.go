package node

import (
	"context"
	"fmt"
	rand0 "math/rand"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	storage "github.com/colorfulnotion/jam/storage"
	trie "github.com/colorfulnotion/jam/trie"
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

// For generating assurance extrinsic. sdb must be the statedb for the block being assured.
func (n *Node) generateAssurance(headerHash common.Hash, timeslot uint32, sdb *statedb.StateDB) (a types.Assurance, numCores uint16) {
	wph := sdb.GetJamState().GetRecentWorkPackagesFromAvailabilityAssignments(timeslot)
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

	// Node has ONE identity (pubkey). The validator INDEX can change after epoch rotation,
	// but the signing key is always the same for now
	selfPubKey := n.GetEd25519Key()
	safrole := sdb.GetSafrole()

	// CRITICAL: Verify the passed statedb matches the anchor slot
	if safrole.Timeslot != timeslot {
		log.Error(log.DA, "generateAssurance: STATEDB SLOT MISMATCH!",
			"n", n.String(),
			"anchorSlot", timeslot,
			"statedbSlot", safrole.Timeslot,
			"selfPubKey", selfPubKey.String()[:16])
	}

	// Find our validator index in the validator set active at the anchor slot
	selfValidatorIdx := safrole.GetValidatorIndexAtSlot(selfPubKey, timeslot)
	if selfValidatorIdx < 0 {
		log.Warn(log.DA, "generateAssurance: self not in validator set at anchor slot",
			"anchorSlot", timeslot, "selfPubKey", selfPubKey.String()[:16])
		return a, 0
	}

	// DEBUG: Print signing key details for baseline verification
	ed25519Secret := n.GetEd25519Secret()
	epoch, phase := safrole.EpochAndPhase(timeslot)
	statedbEpoch, _ := safrole.EpochAndPhase(safrole.Timeslot)
	validatorSetUsed := "CurrValidators"
	if epoch == statedbEpoch+1 {
		validatorSetUsed = "NextValidators"
	} else if epoch == statedbEpoch-1 {
		validatorSetUsed = "PrevValidators"
	}
	log.Trace(log.DA, "generateAssurance SIGNING", "n", n.String(),
		"selfPubKey", selfPubKey.String()[:16],
		"validatorIdx", selfValidatorIdx,
		"anchorSlot", timeslot,
		"anchorEpoch", epoch,
		"phase", phase,
		"statedbSlot", safrole.Timeslot,
		"statedbEpoch", statedbEpoch,
		"validatorSetUsed", validatorSetUsed)

	a.Anchor = headerHash
	a.ValidatorIndex = uint16(selfValidatorIdx)
	a.Sign(ed25519Secret)

	n.telemetryClient.AssuranceProvided(a)
	return
}

func (n *NodeContent) ReadGlobalDepth(serviceID uint32) (uint8, error) {
	return n.statedb.ReadGlobalDepth(serviceID)
}

func (n *NodeContent) GetRefineContext() (types.RefineContext, error) {
	// Get base refine context from statedb (uses best block for Anchor)
	refineCtx := n.statedb.GetRefineContext()

	// Override LookupAnchor and LookupAnchorSlot with finalized block
	// External implementations (e.g., PolkaJAM) require LookupAnchor to be finalized
	finalizedBlock, err := n.GetFinalizedBlock()
	if err != nil {
		log.Warn(log.Node, "GetRefineContext: failed to get finalized block, using best block for LookupAnchor", "err", err)
		return refineCtx, nil
	}
	if finalizedBlock != nil {
		refineCtx.LookupAnchor = finalizedBlock.Header.Hash()
		refineCtx.LookupAnchorSlot = finalizedBlock.Header.Slot
	}

	return refineCtx, nil
}
func (n *NodeContent) BuildBundle(workPackage types.WorkPackage, extrinsicsBlobs []types.ExtrinsicsBlobs, coreIndex uint16, rawObjectIDs []common.Hash) (b *types.WorkPackageBundle, wr *types.WorkReport, err error) {
	return n.statedb.BuildBundle(workPackage, extrinsicsBlobs, coreIndex, rawObjectIDs, n.pvmBackend)
}

// upon audit, this does CE138 AND CE139 calls to ALL Assurers
// targetDB is the anchor-specific statedb for this audit operation (not n.statedb which may be at a different epoch)
func (n *NodeContent) FetchAllBundleAndSegmentShards(coreIdx uint16, erasureRoot common.Hash, exportedSegmentLength uint16, verify bool, eventID uint64, targetDB *statedb.StateDB) {
	safrole := targetDB.GetSafrole()
	currSlot := safrole.Timeslot
	// Auditor -> Assurer CE138
	shardIdxMap := make(map[uint16]uint16) // shardIdx -> validatorIdx
	for vIdx := range types.TotalValidators {
		shardIdx := storage.ComputeShardIndex(coreIdx, uint16(vIdx))
		shardIdxMap[shardIdx] = uint16(vIdx)
	}

	resps := make([]trie.Bundle138Resp, types.TotalValidators)
	for i := range types.TotalValidators {
		shardIdx := uint16(i)
		vIdx := shardIdxMap[shardIdx]
		pubKey, ok := safrole.GetValidatorPubKeyAtTimeSlot(vIdx, currSlot)
		if !ok {
			continue
		}
		peer, ok := n.GetPeerByPubKey(pubKey)
		if !ok {
			continue
		}
		bundleShard, sClub, encodedPath, err := peer.SendBundleShardRequest(context.TODO(), erasureRoot, shardIdx, eventID)
		if err == nil {
			resps[shardIdx] = trie.Bundle138Resp{
				TreeLen:     types.TotalValidators,
				ShardIdx:    int(shardIdx),
				ErasureRoot: erasureRoot,
				LeafHash:    nil,
				Path:        nil,
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
		segmentsPerPageProof := uint16(64)
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
			pubKey, ok := safrole.GetValidatorPubKeyAtTimeSlot(vIdx, currSlot)
			if !ok {
				continue
			}
			peer, ok := n.GetPeerByPubKey(pubKey)
			if !ok {
				continue
			}
			segmentShards, justifications, err := peer.SendSegmentShardRequest(context.TODO(), erasureRoot, shardIdx, allsegmentindices, verify, eventID)
			if err == nil {
				resps[i] = trie.Segment139Resp{
					ShardIdx:       shardIdx,
					SegmentShards:  segmentShards,
					Justifications: justifications,
				}
			} else {
				log.Warn(log.DA, "FetchAllBundleAndSegmentShards: SendSegmentShardRequest failed",
					"validatorIdx", vIdx, "shardIdx", shardIdx, "err", err)
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
		log.Error(log.DA, "SelectGuarantor: No valid guarantors found in guarantee signatures", "guarantee", "reportHash", g.Report.Hash(), "error", err)
		return 0, err
	}
	return guarantorList[rand0.Intn(len(guarantorList))], nil
}

// Assurer -> Guarantor
func (n *Node) FetchAllFullShards(g types.Guarantee, verify bool) {
	spec := g.Report.AvailabilitySpec
	coreIdx := g.Report.CoreIndex
	blacklistID := []uint16{5}
	guarantorIdx, err := SelectGuarantor(g, blacklistID)
	if err != nil {
		return
	}

	guarantorPubKey, ok := n.statedb.GetSafrole().GetValidatorPubKeyAtTimeSlot(guarantorIdx, g.Slot)
	if !ok {
		log.Warn(log.DA, "FetchAllFullShards: guarantor pubkey not found", "guarantorIdx", guarantorIdx)
		return
	}
	guarantorPeer, ok := n.GetPeerByPubKey(guarantorPubKey)
	if !ok {
		log.Warn(log.DA, "FetchAllFullShards: guarantor peer not found", "guarantorPubKey", guarantorPubKey)
		return
	}

	resps := make([]trie.Full137Resp, types.TotalValidators)
	fmt.Printf("FetchAllFullShards: coreIdx=%d, guarantorIdx=%d, erasureRoot=%s\n", coreIdx, guarantorIdx, spec.ErasureRoot)
	for i := 0; i < types.TotalValidators; i++ {
		shardIdx := uint16(i)
		bundleShard, exportedShards, encodedPath, err := guarantorPeer.SendFullShardRequest(context.TODO(), spec.ErasureRoot, shardIdx)
		if err == nil {
			resps[int(shardIdx)] = trie.Full137Resp{
				TreeLen:        types.TotalValidators,
				ShardIdx:       int(shardIdx),
				ErasureRoot:    spec.ErasureRoot,
				BundleShard:    bundleShard,
				ExportedShards: exportedShards,
				EncodedPath:    encodedPath,
			}
			log.Trace(log.DA, "FetchAllFullShards: SendFullShardRequest success CE137",
				"coreIdx", coreIdx, "shardIdx", shardIdx)
			if verify {
				VerifyFullShard(spec.ErasureRoot, shardIdx, bundleShard, exportedShards, encodedPath)
			}
		}
	}
	// Get the directory of this source file and save to trie/test/
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Printf("FetchAllFullShards: Failed to get current file path\n")
		return
	}
	fnDir := filepath.Join(filepath.Dir(filename), "..", "trie", "test")
	fnName := fmt.Sprintf("full137resp_%d.json", spec.ExportedSegmentLength)
	fn := filepath.Join(fnDir, fnName)
	fmt.Printf("FetchAllFullShards: Saving responses to %s\n", fn)
	os.WriteFile(fn, []byte(types.ToJSON(resps)), 0644)
}

func (n *Node) FetchAllInternalFullShardsForGuarantee(g types.Guarantee, verify bool) {

}

const attemptReconstruction = false

// assureData, given a Guarantee with an AvailabilitySpec within a WorkReport,
// fetches the bundleShard and segmentShards and stores in ImportDA + AuditDA.
func (n *Node) assureData(ctx context.Context, g types.Guarantee, sdb *statedb.StateDB) error {
	spec := g.Report.AvailabilitySpec
	coredIdx := g.Report.CoreIndex
	workReportHash := g.Report.Hash()

	myPubKey := n.GetEd25519Key()
	selfValidatorIdx := n.statedb.GetSafrole().GetCurrValidatorIndex(myPubKey)
	if selfValidatorIdx < 0 {
		return fmt.Errorf("assureData: self not in current validator set")
	}
	shardIdx := storage.ComputeShardIndex(uint16(coredIdx), uint16(selfValidatorIdx))

	const maxRetries = 3
	var bundleShard []byte
	var exportedShards []byte
	var encodedPath []byte
	var err error
	var guarantorIdx uint16

	if attemptReconstruction {
		n.FetchAllFullShards(g, true)
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		randomguarantor := rand0.Intn(len(g.Signatures))
		guarantorIdx = g.Signatures[randomguarantor].ValidatorIndex

		guarantorPubKey, ok := n.statedb.GetSafrole().GetValidatorPubKeyAtTimeSlot(guarantorIdx, g.Slot)
		if !ok {
			log.Warn(log.DA, "assureData: guarantor pubkey not found", "guarantorIdx", guarantorIdx, "slot", g.Slot)
			continue
		}
		guarantorPeer, ok := n.GetPeerByPubKey(guarantorPubKey)
		if !ok {
			log.Warn(log.DA, "assureData: guarantor peer not found", "guarantorPubKey", guarantorPubKey)
			continue
		}

		bundleShard, exportedShards, encodedPath, err = guarantorPeer.SendFullShardRequest(ctx, spec.ErasureRoot, shardIdx)
		if err == nil {
			log.Trace(log.DA, "assureData: SendFullShardRequest success",
				"coreIdx", coredIdx, "selfValidatorIdx", selfValidatorIdx, "shardIdx", shardIdx,
				"erasureRoot", spec.ErasureRoot, "guarantorIdx", guarantorIdx)
			break
		}
		log.Warn(log.DA, "assureData: SendFullShardRequest attempt failed",
			"coredIdx", coredIdx, "selfValidatorIdx", selfValidatorIdx, "shardIdx", shardIdx,
			"attempt", attempt, "n", n.String(), "erasureRoot", spec.ErasureRoot,
			"guarantorIdx", guarantorIdx, "err", err)
	}

	if err != nil {
		log.Error(log.DA, "assureData: SendFullShardRequest failed after retries",
			"coredIdx", coredIdx, "selfValidatorIdx", selfValidatorIdx, "shardIdx", shardIdx,
			"n", n.String(), "erasureRoot", spec.ErasureRoot, "err", err)
		return fmt.Errorf("SendFullShardRequest (after retries): %w", err)
	}

	// CRITICAL: verify justification matches the erasure root before storage
	verified, err := VerifyFullShard(spec.ErasureRoot, shardIdx, bundleShard, exportedShards, encodedPath)
	if err != nil {
		log.Error(log.DA, "assureData: VerifyFullShard error", "coredIdx", coredIdx, "selfValidatorIdx", selfValidatorIdx, "shardIdx", shardIdx, "n", n.String(), "err", err)
		return fmt.Errorf("VerifyFullShard: %w", err)
	}
	if !verified {
		log.Error(log.DA, "assureData: VerifyFullShard failed", "coredIdx", coredIdx, "selfValidatorIdx", selfValidatorIdx, "shardIdx", shardIdx, "n", n.String())
		return fmt.Errorf("VerifyFullShard: failed verification")
	}

	if err := n.StoreFullShard_Assurer(spec.ErasureRoot, shardIdx, bundleShard, exportedShards, encodedPath); err != nil {
		return fmt.Errorf("StoreFullShard_Assurer: %w", err)
	}

	if err := n.StoreWorkReport(g.Report); err != nil {
		log.Error(log.DA, "assureData: StoreWorkReport failed", "coredIdx", coredIdx, "selfValidatorIdx", selfValidatorIdx, "shardIdx", shardIdx, "n", n.String(), "err", err)
		return fmt.Errorf("StoreWorkReport: %w", err)
	}

	n.telemetryClient.ContextAvailable(workReportHash, uint16(coredIdx), g.Slot, spec)

	n.markAssuring(spec.WorkPackageHash)

	return nil
}
