package node

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type WPQueueItem struct {
	workPackage        types.WorkPackage
	coreIndex          uint16
	extrinsics         types.ExtrinsicsBlobs
	addTS              int64
	nextAttemptAfterTS int64
}

func (n *Node) runWPQueue() {
	pulseTicker := time.NewTicker(100 * time.Millisecond)
	defer pulseTicker.Stop()
	for {
		select {
		case <-pulseTicker.C:
			n.workPackageQueue.Range(func(key, value interface{}) bool {
				wpItem := value.(*WPQueueItem)
				if time.Now().Unix() >= wpItem.nextAttemptAfterTS {
					wpItem.nextAttemptAfterTS = time.Now().Unix()
					if n.processWPQueueItem(wpItem) {
						n.workPackageQueue.Delete(key)
						return false
					}
				}
				return true
			})
		}
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
func (n *Node) buildBundle(wpQueueItem *WPQueueItem) (bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, err error) {
	workPackage := wpQueueItem.workPackage

	workReportSearchMap := make(map[common.Hash]*SpecIndex)
	segmentRootLookupMap := make(map[common.Hash]common.Hash)
	// because CE139 requires erasureroot
	erasureRootIndex := make(map[common.Hash]*SpecIndex)
	workItemErasureRootsMapping := make([][]*SpecIndex, len(workPackage.WorkItems))
	for workItemIdx, workItem := range workPackage.WorkItems {
		workItemErasureRootsMapping[workItemIdx] = make([]*SpecIndex, len(workItem.ImportedSegments))
		for idx, importedSegment := range workItem.ImportedSegments {
			// these are actually work package hashes or segment roots
			si, ok := workReportSearchMap[importedSegment.RequestedHash]
			if !ok {
				// check for availability reports
				si = n.WorkReportSearch(importedSegment.RequestedHash)
				if si == nil {
					err = fmt.Errorf("WorkReportSearch(%s) not found", importedSegment.RequestedHash)
					log.Warn(module, "buildBundle", "err", err)
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
		receiveSegments, specJustifications, err := n.reconstructSegments(specIndex)
		if err != nil {
			log.Warn(module, "reconstructSegments", "err", err)
			return bundle, segmentRootLookup, err
		}
		if len(receiveSegments) != len(specIndex.Indices) {
			panic("receiveSegments and specIndex.Indices length mismatch")
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
				err = fmt.Errorf("Missing segments for workItemIndex %d idx %d", workItemIndex, idx)
				log.Warn(module, "buildBundle", "err", err)
				return bundle, segmentRootLookup, err
			}
			erasureRoot := wpi.WorkReport.AvailabilitySpec.ErasureRoot
			receivedSegments, exists := receiveSegmentMapping[erasureRoot]
			receivedJustifications, existJ := justificationsMapping[erasureRoot]
			if !exists || !existJ {
				err = fmt.Errorf("Missing segments for erasureRoot %v", erasureRoot)
				log.Error(module, "buildBundle", "err", err)
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
		ExtrinsicData:     wpQueueItem.extrinsics,
		ImportSegmentData: importSegments,
		Justification:     justifications, // recipients use VerifyBundle
	}
	return bundle, segmentRootLookup, nil
}

func (n *Node) processWPQueueItem(wpItem *WPQueueItem) bool {
	if n.store.SendTrace {
		tracer := n.store.Tp.Tracer("NodeTracer")
		// n.InitWPContext(wp)
		tags := trace.WithAttributes(attribute.String("WorkpackageHash", common.Str(wpItem.workPackage.Hash())))
		ctx, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] processWPQueueItem", n.store.NodeID), tags)
		n.store.UpdateWorkPackageContext(ctx)
		defer span.End()
	}
	// counting the time for this function execution

	timer := time.Now()
	coreIndex := wpItem.coreIndex

	// here we are a first guarantor building a bundle (imported segments, justifications, extrinsics)
	bundle, segmentRootLookup, err := n.buildBundle(wpItem)
	if err != nil {
		log.Warn(debugG, "processWPQueueItem", "n", n.String(), "err", err)
		wpItem.nextAttemptAfterTS = time.Now().Unix() + 6
		return false
	}

	curr_statedb := n.statedb.Copy()
	// reject if the work package is not for this core
	if wpItem.coreIndex != curr_statedb.GetSelfCoreIndex() {
		wpItem.nextAttemptAfterTS = time.Now().Unix() + 6
		return false
	}
	err = curr_statedb.VerifyPackage(bundle)
	if err != nil {
		return false
	}
	var wg sync.WaitGroup
	mutex := &sync.Mutex{}
	fellow_responses := make(map[types.Ed25519Key]JAMSNPWorkPackageShareResponse)
	coworkers := n.GetCoreCoWorkerPeersByStateDB(coreIndex, curr_statedb)
	var guarantee types.Guarantee
	for _, coworker := range coworkers {
		wg.Add(1)
		go func(coworker Peer) {
			defer wg.Done()
			// if it's itself, execute the workpackage
			if coworker.PeerID == n.id {
				report, execErr := n.executeWorkPackageBundle(coreIndex, bundle, segmentRootLookup, true)
				if execErr != nil {
					return
				}
				guarantee.Report = report
				signerSecret := n.GetEd25519Secret()
				gc := report.Sign(signerSecret, uint16(n.GetCurrValidatorIndex()))
				guarantee.Signatures = append(guarantee.Signatures, gc)
				return
			} else {
				fellow_response, errfellow := coworker.ShareWorkPackage(coreIndex, bundle, segmentRootLookup, coworker.Validator.Ed25519)
				if errfellow != nil {
					log.Error(debugG, "processWPQueueItem", "n", n.String(), "err", errfellow)
					return
				}
				mutex.Lock()
				fellow_responses[coworker.Validator.Ed25519] = fellow_response
				mutex.Unlock()
			}

		}(coworker)
	}
	wg.Wait()
	selfWorkReportHash := guarantee.Report.Hash()

	for key, fellow_response := range fellow_responses {

		validator_idx := curr_statedb.GetSafrole().GetCurrValidatorIndex(key)
		if validator_idx == -1 {
			panic("validator_idx not found")
		}

		fellowWorkReportHash := fellow_response.WorkReportHash
		fellowSignature := fellow_response.Signature
		if selfWorkReportHash == fellowWorkReportHash {
			guarantee.Signatures = append(guarantee.Signatures, types.GuaranteeCredential{
				ValidatorIndex: uint16(validator_idx),
				Signature:      fellowSignature,
			})
			sort.Slice(guarantee.Signatures, func(i, j int) bool {
				return guarantee.Signatures[i].ValidatorIndex < guarantee.Signatures[j].ValidatorIndex
			})
		} else {
			log.Warn(debugG, "processWPQueueItem Guarantee from fellow did not match", "n", n.String(),
				"selfWorkReportHash", selfWorkReportHash, "fellowWorkReportHash", fellowWorkReportHash)
		}
	}

	if len(guarantee.Signatures) >= 2 {
		if len(guarantee.Signatures) == 2 {
			log.Warn(debugG, "processWPQueueItem:Only 2 signatures, expected 3")
		}
		guarantee.Slot = curr_statedb.GetTimeslot()
		go n.broadcast(guarantee)
		eclapsed := time.Since(timer)
		log.Debug(debugG, "processWPQueueItem: outgoing guarantee for core",
			"n", n.String(), "wph", guarantee.Report.GetWorkPackageHash().String_short(), "core", guarantee.Report.CoreIndex,
			"elapsed", eclapsed.String())
		err := n.processGuarantee(guarantee)
		if err != nil {
			log.Error(debugG, "processWPQueueItem", "n", n.String(), "err", err)
		}
	}
	return true
}
