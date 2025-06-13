package node

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

type WPQueueItem struct {
	workPackage        types.WorkPackage
	coreIndex          uint16
	extrinsics         types.ExtrinsicsBlobs
	addTS              int64
	nextAttemptAfterTS int64
	numFailures        int
	slot               uint32 // the slot for which this work package is intended
}

func (n *Node) runWPQueue() {
	pulseTicker := time.NewTicker(100 * time.Millisecond)
	defer pulseTicker.Stop()
	if !n.GetIsSync() {
		return
	}

	for range pulseTicker.C {
		n.workPackageQueue.Range(func(key, value interface{}) bool {
			wpItem := value.(*WPQueueItem)
			if time.Now().Unix() >= wpItem.nextAttemptAfterTS {
				wpItem.nextAttemptAfterTS = time.Now().Unix()
				if n.processWPQueueItem(wpItem) {
					n.workPackageQueue.Delete(key)
					return false
				} else {
					// allow 6 seconds between attempts, which may result in a core rotation
					wpItem.nextAttemptAfterTS = time.Now().Unix() + types.SecondsPerSlot
					wpItem.numFailures++
					if wpItem.numFailures > 3 {
						log.Warn(log.G, "runWPQueue", "n", n.String(), "numFailures", wpItem.numFailures)
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
func (n *Node) buildBundle(wpQueueItem *WPQueueItem) (bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, err error) {
	workPackage := wpQueueItem.workPackage
	segmentRootLookup = make(types.SegmentRootLookup, 0)
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
		receiveSegments, specJustifications, err := n.reconstructSegments(specIndex)
		if err != nil {
			log.Warn(log.Node, "reconstructSegments", "err", err)
			return bundle, segmentRootLookup, err
		}
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
		ExtrinsicData:     wpQueueItem.extrinsics,
		ImportSegmentData: importSegments,
		Justification:     justifications, // recipients use VerifyBundle
	}
	return bundle, segmentRootLookup, nil
}

type GuaranteeDerivation struct {
	Bundle            types.WorkPackageBundle         `json:"bundle"`
	CoreIndex         uint16                          `json:"core_index"`
	SegmentRootLookup types.SegmentRootLookup         `json:"segment_root_lookup"`
	Guarantee         types.Guarantee                 `json:"guarantee"`
	SpecDerivation    AvailabilitySpecifierDerivation `json:"spec_derivation"`
}

func saveGuaranteeDerivation(gd GuaranteeDerivation) (err error) {
	// Encode to JSON
	data := types.ToJSON(gd)
	// check if there is a directory to write called refine_testvector
	if _, err := os.Stat("guarantees"); os.IsNotExist(err) {
		if err := os.Mkdir("guarantees", 0755); err != nil {
			return err
		}
	}
	// Write to file
	filename := fmt.Sprintf("guarantees/%s.json", gd.Bundle.WorkPackage.Hash())
	if err := os.WriteFile(filename, []byte(data), 0644); err != nil {
		return err
	}
	return nil
}

func (n *Node) processWPQueueItem(wpItem *WPQueueItem) bool {
	var pvmElapsed uint32 // REVIEW: we seem to execute multiple times sometimes???

	// counting the time for this function execution
	coreIndex := wpItem.coreIndex
	slot := wpItem.slot

	// here we are a first guarantor building a bundle (imported segments, justifications, extrinsics)
	bundle, segmentRootLookup, err := n.buildBundle(wpItem)
	if err != nil {
		log.Error(log.Node, "processWPQueueItem", "n", n.String(), "err", err, "nextAttemptAfterTS", wpItem.nextAttemptAfterTS, "wpItem.workPackage.Hash()", wpItem.workPackage.Hash())
		return false
	}

	curr_statedb := n.statedb.Copy()
	err = curr_statedb.VerifyPackage(bundle)
	if err != nil {
		log.Error(log.Node, "processWPQueueItem -  wp not verified", "n", n.String(), "err", err, "nextAttemptAfterTS", wpItem.nextAttemptAfterTS, "wpItem.workPackage.Hash()", wpItem.workPackage.Hash())
		return false
	}
	log.Info(log.Node, "processWPQueueItem", "n", n.id, "wpItem.workPackage.Hash()", wpItem.workPackage.Hash())
	var wg sync.WaitGroup
	mutex := &sync.Mutex{}
	fellow_responses := make(map[types.Ed25519Key]JAMSNPWorkPackageShareResponse)
	coworkers := n.GetCoreCoWorkerPeersByStateDB(coreIndex, curr_statedb)
	var guarantee types.Guarantee
	var report types.WorkReport
	var d AvailabilitySpecifierDerivation
	for _, coworker := range coworkers {
		wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error(log.G, "panic in coworker goroutine", "err", r)
					fmt.Printf("stack trace: %s\n", string(debug.Stack()))
				}
				wg.Done()
			}()
			// if it's itself, execute the workpackage
			if coworker.PeerID == n.id {
				var execErr error

				report, d, pvmElapsed, execErr = n.executeWorkPackageBundle(coreIndex, bundle, segmentRootLookup, true)
				if execErr != nil {
					log.Warn(log.Node, "processWPQueueItem", "err", execErr)
					return
				}
				guarantee.Report = report
				signerSecret := n.GetEd25519Secret()
				gc := report.Sign(signerSecret, uint16(n.GetCurrValidatorIndex()))
				guarantee.Signatures = append(guarantee.Signatures, gc)
				return
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), MiniTimeout)
				defer cancel()
				fellow_response, errfellow := coworker.ShareWorkPackage(ctx, coreIndex, bundle, segmentRootLookup, coworker.Validator.Ed25519, coworker.PeerID)
				if errfellow != nil {
					log.Warn(log.Node, "processWPQueueItem", "n", n.String(), "errfellow", errfellow)
					return
				}
				mutex.Lock()
				fellow_responses[coworker.Validator.Ed25519] = fellow_response
				mutex.Unlock()
			}
		}()
	}
	wg.Wait()
	selfWorkReportHash := guarantee.Report.Hash()

	for key, fellow_response := range fellow_responses {

		validator_idx := curr_statedb.GetSafrole().GetCurrValidatorIndex(key)
		if validator_idx == -1 {
			return false // "validator_idx not found"
		}

		fellowWorkReportHash := fellow_response.WorkReportHash
		fellowSignature := fellow_response.Signature
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
			log.Warn(log.G, "processWPQueueItem Guarantee from fellow did not match", "n", n.String(),
				"selfWorkReportHash", selfWorkReportHash, "fellowWorkReportHash", fellowWorkReportHash)
			log.Warn(log.G, "our reports", "n", n.String(), "selfWorkReport", guarantee.Report.String())
		}
	}

	if len(guarantee.Signatures) >= 2 {
		guarantee.Slot = slot // this is the slot when ORIGINALLY added to the queue
		log.Debug(log.G, "processWPQueueItem Guarantee enough signatures", "n", n.id, "guarantee.Slot", guarantee.Slot, "guarantee.Signatures", types.ToJSONHex(guarantee.Signatures), "nextAttemptAfterTS", wpItem.nextAttemptAfterTS)
		ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
		defer cancel()
		n.broadcast(ctx, guarantee) // send via CE135

		go saveGuaranteeDerivation(GuaranteeDerivation{
			Bundle:            bundle,
			CoreIndex:         coreIndex,
			SegmentRootLookup: segmentRootLookup,
			Guarantee:         guarantee,
			SpecDerivation:    d,
		})
		err := n.processGuarantee(guarantee, "processWPQueueItem") // should it be curr_statedb.GetTimeslot() here?
		if err != nil {
			log.Error(log.G, "processWPQueueItem:processGuarantee:", "n", n.String(), "err", err)
		}
	} else {
		log.Warn(log.G, "processWPQueueItem Guarantee not enough signatures", "n", n.String(),
			"guarantee.Signatures", types.ToJSONHex(guarantee.Signatures), "nextAttemptAfterTS", wpItem.nextAttemptAfterTS)
		return false
	}
	metadata := fmt.Sprintf("role=Guarantor|numSig=%d", len(guarantee.Signatures))
	n.Telemetry(log.MsgTypeWorkReport, guarantee.Report, "msg_type", getMessageType(guarantee.Report), "metadata", metadata, "elapsed", pvmElapsed, "codec_encoded", types.EncodeAsHex(guarantee.Report))
	return true
}
