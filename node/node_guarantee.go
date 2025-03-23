package node

import (
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
	wp                 types.WorkPackage
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

func (n *Node) processWPQueueItem(wpItem *WPQueueItem) bool {
	if n.store.SendTrace {
		tracer := n.store.Tp.Tracer("NodeTracer")
		// n.InitWPContext(wp)
		tags := trace.WithAttributes(attribute.String("WorkpackageHash", common.Str(wpItem.wp.Hash())))
		ctx, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] broadcastWorkpackage", n.store.NodeID), tags)
		n.store.UpdateWorkPackageContext(ctx)
		defer span.End()
	}
	// counting the time for this function execution

	timer := time.Now()
	coreIndex := wpItem.coreIndex
	wp := wpItem.wp
	log.Debug(debugDA, "processWPQueueItem", "n", n.String(), "coreIndex", coreIndex, "workPackageHash", wp.Hash())
	segmentRootLookup, err := n.GetSegmentRootLookup(wpItem.wp)
	if err != nil {
		log.Warn(debugG, "broadcastWorkPackage:GetSegmentRootLookup", "n", n.String(), "err", err)
		wpItem.nextAttemptAfterTS = time.Now().Unix() + 6
		return false
	}
	// here we are a first guarantor making justifications and reconstructSegments is used inside FetchWorkpackageImportSegments
	importedSegments, justifications, err := n.FetchWorkpackageImportSegments(wpItem.wp, segmentRootLookup)
	if err != nil {
		log.Warn(debugG, "broadcastWorkPackage:FetchWorkpackageImportSegments", "n", n.String(), "err", err)
		wpItem.nextAttemptAfterTS = time.Now().Unix() + 6
		return false
	}

	curr_statedb := n.statedb.Copy()
	// reject if the work package is not for this core
	if wpItem.coreIndex != curr_statedb.GetSelfCoreIndex() {
		wpItem.nextAttemptAfterTS = time.Now().Unix() + 6
		return false
	}
	bundle := types.WorkPackageBundle{
		WorkPackage:       wp,
		ExtrinsicData:     wpItem.extrinsics,
		ImportSegmentData: importedSegments,
		Justification:     justifications, // recipients use VerifyBundle
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
					log.Error(debugG, "broadcastWorkPackage:ShareWorkPackage", "n", n.String(), "err", errfellow)
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
			log.Crit(debugG, "broadcastWorkpackage Guarantee from fellow did not match", "n", n.String(),
				"selfWorkReportHash", selfWorkReportHash, "fellowWorkReportHash", fellowWorkReportHash)

			panic(9234)
			return false
		}
	}

	if len(guarantee.Signatures) >= 2 {
		if len(guarantee.Signatures) == 2 {
			log.Warn(debugG, "broadcastWorkpackage:Only 2 signatures, expected 3")
		}
		guarantee.Slot = curr_statedb.GetTimeslot()
		go n.broadcast(guarantee)
		eclapsed := time.Since(timer)
		log.Debug(debugG, "broadcastWorkPackage: outgoing guarantee for core",
			"n", n.String(), "wph", guarantee.Report.GetWorkPackageHash().String_short(), "core", guarantee.Report.CoreIndex,
			"elapsed", eclapsed.String())
		err := n.processGuarantee(guarantee)
		if err != nil {
			log.Error(debugG, "processGuarantee", "n", n.String(), "err", err)
		}
		log.Trace(debugG, "broadcast guarantee in slot", "n", n.String(), "coreIndex", coreIndex, "slot", guarantee.Slot, "actual", n.statedb.GetTimeslot())
	}
	return true
}
