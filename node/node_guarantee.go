package node

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	storage "github.com/colorfulnotion/jam/storage"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) runWPQueue() {
	pulseTicker := time.NewTicker(100 * time.Millisecond)
	defer pulseTicker.Stop()
	if !n.GetIsSync() {
		return
	}

	for range pulseTicker.C {
		n.workPackageQueue.Range(func(key, value interface{}) bool {
			wpItem := value.(*types.WPQueueItem)
			if time.Now().Unix() >= wpItem.NextAttemptAfterTS {
				wpItem.NextAttemptAfterTS = time.Now().Unix()
				if n.processWPQueueItem(wpItem) {
					n.workPackageQueue.Delete(key)
					return false
				} else {
					// allow 6 seconds between attempts, which may result in a core rotation
					wpItem.NextAttemptAfterTS = time.Now().Unix() + types.SecondsPerSlot
					wpItem.NumFailures++
					if wpItem.NumFailures > 3 {
						log.Warn(log.G, "runWPQueue", "n", n.String(), "numFailures", wpItem.NumFailures)
						n.workPackageQueue.Delete(key)
						return false
					}
				}
			}
			return true
		})
	}
}

func (n *Node) clearQueueUsingBlock(guarantees []types.Guarantee) {
	for _, g := range guarantees {
		n.workPackageQueue.Delete(g.Report.AvailabilitySpec.WorkPackageHash)
	}
}

// buildBundle is called ONLY by The first guarantor -- it maps a workpackage into a workpackagebundle
//
//	(1) uses WorkReportSearch to ensure that the WorkReports have been seen for all work items and thus imported segments are fetchable
//	(2) uses CE139 in reconstructSegments to get all the imported segments and their justifications
func (n *NodeContent) BuildBundleFromWPQueueItem(wpQueueItem *types.WPQueueItem) (bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, err error) {
	workPackage := wpQueueItem.WorkPackage
	eventID := wpQueueItem.EventID
	segmentRootLookup = make(types.SegmentRootLookup, 0)
	workReportSearchMap := make(map[common.Hash]*storage.SpecIndex)
	segmentRootLookupMap := make(map[common.Hash]common.Hash)
	// because CE139 requires erasureroot
	erasureRootIndex := make(map[common.Hash]*storage.SpecIndex)
	workItemErasureRootsMapping := make([][]*storage.SpecIndex, len(workPackage.WorkItems))
	for workItemIdx, workItem := range workPackage.WorkItems {
		workItemErasureRootsMapping[workItemIdx] = make([]*storage.SpecIndex, len(workItem.ImportedSegments))
		for idx, importedSegment := range workItem.ImportedSegments {
			// these are actually work package hashes or segment roots
			si, ok := workReportSearchMap[importedSegment.RequestedHash]
			if !ok {
				// check for availability reports
				si = n.WorkReportSearch(importedSegment.RequestedHash)
				if si == nil {
					err = fmt.Errorf("WorkReportSearch(%s) not found", importedSegment.RequestedHash)
					log.Warn(log.Node, "buildBundle", "err", err)
					return bundle, segmentRootLookup, err
				}
				workReportSearchMap[importedSegment.RequestedHash] = si
				segmentRootLookupMap[si.WorkReport.AvailabilitySpec.WorkPackageHash] = si.WorkReport.AvailabilitySpec.ExportedSegmentRoot
			}
			erasureRoot := si.WorkReport.AvailabilitySpec.ErasureRoot
			oldwpi, exists := erasureRootIndex[erasureRoot]
			if exists {
				oldwpi.AddIndex(uint16(importedSegment.Index))
				workItemErasureRootsMapping[workItemIdx][idx] = oldwpi
			} else {
				erasureRootIndex[erasureRoot] = si
				workItemErasureRootsMapping[workItemIdx][idx] = si
				si.AddIndex(uint16(importedSegment.Index))
			}
		}
	}

	// Now, if we got to this point, we have confirmed that there availability reports, so we probably could reconstruct them
	// Use reconstructSegments (CE139) to fetch the segments by erasureRoots and indices
	receiveSegmentMapping := make(map[common.Hash][][]byte)
	justificationsMapping := make(map[common.Hash][][]common.Hash)
	for erasureRoot, specIndex := range erasureRootIndex {
		// Telemetry: ReconstructingSegments (event 168) - Segment reconstruction begins
		var receiveSegments [][]byte
		var specJustifications [][]common.Hash
		var err error

		isTrivial := false // CE139 reconstruction is not trivial (uses erasure coding)
		n.telemetryClient.ReconstructingSegments(eventID, specIndex.Indices, isTrivial)

		receiveSegments, specJustifications, err = n.reconstructSegments(specIndex, eventID)
		if err != nil {
			// Telemetry: SegmentReconstructionFailed (event 169)
			n.telemetryClient.SegmentReconstructionFailed(eventID, err.Error())
			log.Warn(log.Node, "reconstructSegments", "err", err)
			return bundle, segmentRootLookup, err
		}

		// Telemetry: SegmentsReconstructed (event 170) - Segment reconstruction succeeded
		n.telemetryClient.SegmentsReconstructed(eventID)

		if len(receiveSegments) != len(specIndex.Indices) {
			return bundle, segmentRootLookup, fmt.Errorf("receiveSegments and specIndex.Indices length mismatch")
		}
		//fmt.Printf("len %d == %d\n", len(receiveSegments), len(specIndex.Indices))
		// ***** TODO: cache receiveSegments so that we don't have to call reconstructSegments again on another attempt
		receiveSegmentMapping[erasureRoot] = receiveSegments
		justificationsMapping[erasureRoot] = specJustifications
	}

	// Remap the segments to [workItenIndex][importedSegmentIndex][bytes]
	importSegments := make([][][]byte, len(workPackage.WorkItems))
	justifications := make([][][]common.Hash, len(workPackage.WorkItems))
	for workItemIndex, workItem := range workPackage.WorkItems {
		importSegments[workItemIndex] = make([][]byte, len(workItem.ImportedSegments))
		justifications[workItemIndex] = make([][]common.Hash, len(workItem.ImportedSegments))
		for idx, impseg := range workItem.ImportedSegments {
			wpi := workItemErasureRootsMapping[workItemIndex][idx]
			if wpi == nil {
				err = fmt.Errorf("missing segments for workItemIndex %d idx %d", workItemIndex, idx)
				log.Warn(log.Node, "buildBundle", "err", err)
				return bundle, segmentRootLookup, err
			}
			erasureRoot := wpi.WorkReport.AvailabilitySpec.ErasureRoot
			receivedSegments, exists := receiveSegmentMapping[erasureRoot]
			receivedJustifications, existJ := justificationsMapping[erasureRoot]
			if !exists || !existJ {
				err = fmt.Errorf("missing segments for erasureRoot %v", erasureRoot)
				log.Error(log.Node, "buildBundle", "err", err)
				return bundle, segmentRootLookup, err
			}
			for y, z := range wpi.Indices { // these are the indices of the imported segments with the same erasure root
				if z == impseg.Index { // we found the segment with the same index
					importSegments[workItemIndex][idx] = receivedSegments[y]
					justifications[workItemIndex][idx] = receivedJustifications[y]
					break
				}
			}
		}
	}
	// build the segmentRootLookup so we may pass to other guarantors
	for wph, segmentRootHash := range segmentRootLookupMap {
		segmentRootLookup = append(segmentRootLookup, types.SegmentRootLookupItem{
			WorkPackageHash: wph,
			SegmentRoot:     segmentRootHash,
		})
	}
	// Sort the slice by WorkPackageHash in ascending order.
	sort.Slice(segmentRootLookup, func(i, j int) bool {
		return bytes.Compare(segmentRootLookup[i].WorkPackageHash.Bytes(), segmentRootLookup[j].WorkPackageHash.Bytes()) < 0
	})

	bundle = types.WorkPackageBundle{
		WorkPackage:       workPackage,
		ExtrinsicData:     []types.ExtrinsicsBlobs{wpQueueItem.Extrinsics},
		ImportSegmentData: importSegments,
		Justification:     justifications, // recipients use VerifyBundle
	}
	return bundle, segmentRootLookup, nil
}

func (n *Node) processWPQueueItem(wpItem *types.WPQueueItem) bool {
	// counting the time for this function execution
	coreIndex := wpItem.CoreIndex
	slot := wpItem.Slot

	// here we are a first guarantor building a bundle (imported segments, justifications, extrinsics)
	bundle, segmentRootLookup, err := n.BuildBundleFromWPQueueItem(wpItem)
	if err != nil {
		log.Error(log.Node, "processWPQueueItem", "n", n.String(), "err", err,
			"nextAttemptAfterTS", wpItem.NextAttemptAfterTS, "wpItem.workPackage.Hash()", wpItem.WorkPackage.Hash())
		return false
	}
	n.telemetryClient.ImportsReceived(wpItem.EventID)
	//n.getStateDBByHeaderHash()

	curr_statedb := n.statedb.Copy()
	err = curr_statedb.VerifyPackage(bundle)
	if err != nil {
		log.Error(log.Node, "processWPQueueItem -  wp not verified", "n", n.String(), "err", err, "nextAttemptAfterTS", wpItem.NextAttemptAfterTS, "wpItem.workPackage.Hash()", wpItem.WorkPackage.Hash())
		return false
	}
	log.Info(log.Node, "processWPQueueItem", "n", n.id, "wpItem.workPackage.Hash()", wpItem.WorkPackage.Hash())
	//var wg sync.WaitGroup
	mutex := &sync.Mutex{}
	fellow_responses := make(map[types.Ed25519Key]JAMSNPWorkPackageShareResponse)
	coworkers := n.GetCoreCoWorkerPeersByStateDB(coreIndex, curr_statedb)
	var guarantee types.Guarantee
	var report types.WorkReport

	// Do our own execution first
	var execErr error
	report, execErr = n.executeWorkPackageBundle(coreIndex, bundle, segmentRootLookup, slot, true, wpItem.EventID)
	if execErr != nil {
		log.Warn(log.Node, "processWPQueueItem", "err", execErr)
		panic(111)
	}
	guarantee.Report = report
	signerSecret := n.GetEd25519Secret()
	gc := report.Sign(signerSecret, uint16(n.GetCurrValidatorIndex()))
	guarantee.Signatures = append(guarantee.Signatures, gc)

	for _, coworker := range coworkers {
		if coworker.PeerID == n.id {
			continue
		}
		fellow_response, errfellow := coworker.ShareWorkPackage(context.Background(), coreIndex, bundle, segmentRootLookup, coworker.Validator.Ed25519, coworker.PeerID, wpItem.EventID)
		if errfellow != nil {
			log.Warn(log.Node, "processWPQueueItem", "n", n.String(), "errfellow", errfellow)
			n.telemetryClient.WorkPackageSharingFailed(wpItem.EventID, n.PeerID32(coworker.PeerID), errfellow.Error())

		}
		mutex.Lock()
		fellow_responses[coworker.Validator.Ed25519] = fellow_response
		mutex.Unlock()
		// do just ONE of our coworkers
		//break
	}

	selfWorkReportHash := guarantee.Report.Hash()
	for key, fellow_response := range fellow_responses {

		validator_idx := curr_statedb.GetSafrole().GetCurrValidatorIndex(key)
		if validator_idx == -1 {
			return false // "validator_idx not found"
		}

		fellowWorkReportHash := fellow_response.WorkReportHash
		fellowSignature := fellow_response.Signature

		// Log comparison details
		log.Debug(log.G, "processWPQueueItem comparing work report hashes",
			"n", n.String(),
			"fellowValidator", key.String()[:16]+"...",
			"selfHash", selfWorkReportHash.String(),
			"fellowHash", fellowWorkReportHash.String(),
			"match", selfWorkReportHash == fellowWorkReportHash)

		if selfWorkReportHash == fellowWorkReportHash {
			guarantee.Signatures = append(guarantee.Signatures, types.GuaranteeCredential{
				ValidatorIndex: uint16(validator_idx),
				Signature:      fellowSignature,
			})
			sort.Slice(guarantee.Signatures, func(i, j int) bool {
				status := guarantee.Signatures[i].ValidatorIndex < guarantee.Signatures[j].ValidatorIndex
				return status
			})
		} else {
			if (guarantee.Report.GetWorkPackageHash() != common.Hash{}) {
				log.Warn(log.G, "processWPQueueItem Guarantee from fellow did not match", "n", n.String(), "selfWorkReportHash", selfWorkReportHash, "fellowWorkReportHash", fellowWorkReportHash)
				//return false
			}
		}
	}

	if len(guarantee.Signatures) >= 2 {
		guarantee.Slot = slot // this is the slot when ORIGINALLY added to the queue
		// log.Info(log.G, "processWPQueueItem Guarantee enough signatures", "n", n.id, "guarantee.Slot", guarantee.Slot, "numSig", len(guarantee.Signatures), "guarantee.Signatures", types.ToJSONHex(guarantee.Signatures), "nextAttemptAfterTS", wpItem.nextAttemptAfterTS)
		ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
		defer cancel()
		guarantorIndices := make([]uint16, len(guarantee.Signatures))
		for i, sig := range guarantee.Signatures {
			guarantorIndices[i] = sig.ValidatorIndex
		}
		guaranteeOutline := telemetry.GuaranteeOutline{
			WorkReportHash: guarantee.Report.Hash(),
			Slot:           guarantee.Slot,
			Guarantors:     guarantorIndices,
		}
		n.telemetryClient.GuaranteeBuilt(wpItem.EventID, guaranteeOutline)
		n.broadcast(ctx, guarantee, wpItem.EventID) // send via CE135
		n.telemetryClient.GuaranteesDistributed(wpItem.EventID)

		err := n.processGuarantee(guarantee, "processWPQueueItem") // should it be curr_statedb.GetTimeslot() here?
		if err != nil {
			log.Error(log.G, "processWPQueueItem:processGuarantee:", "n", n.String(), "err", err)
		}
		log.Trace(log.G, "processWPQueueItem Guarantee processed", "n", n.String(), "guarantee.Slot", guarantee.Slot, "numSig", len(guarantee.Signatures), "guarantee.Signatures", types.ToJSONHex(guarantee.Signatures), "nextAttemptAfterTS", wpItem.NextAttemptAfterTS)
	} else {
		log.Debug(log.G, "processWPQueueItem Guarantee not enough signatures", "n", n.String(), "guarantee.Signatures", types.ToJSONHex(guarantee.Signatures), "nextAttemptAfterTS", wpItem.NextAttemptAfterTS)
		return false
	}

	return true
}

// executeWorkPackageBundle can be called by a guarantor OR an auditor -- the caller MUST do  VerifyBundle call prior to execution (verifying the imported segments)
// If eventID is non-zero, telemetry events for Authorized and Refined will be emitted
func (n *NodeContent) executeWorkPackageBundle(workPackageCoreIndex uint16, package_bundle types.WorkPackageBundle,
	segmentRootLookup types.SegmentRootLookup, slot uint32, firstGuarantorOrAuditor bool, eventID uint64) (work_report types.WorkReport, err error) {
	targetStateDB, err := n.getStateDBByStateRoot(package_bundle.WorkPackage.RefineContext.StateRoot)
	if err != nil {
		return work_report, fmt.Errorf("executeWorkPackageBundle:getStateDBByStateRoot: %v", err)
	}
	workReport, err := targetStateDB.ExecuteWorkPackageBundle(workPackageCoreIndex, package_bundle, segmentRootLookup, slot, firstGuarantorOrAuditor, eventID, n.pvmBackend)
	return workReport, err
}
