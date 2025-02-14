package node

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (n *Node) broadcastWorkpackage(wp types.WorkPackage, wpCoreIndex uint16, curr_statedb *statedb.StateDB, extrinsics types.ExtrinsicsBlobs) (guarantee types.Guarantee, err error) {
	if n.store.SendTrace {
		tracer := n.store.Tp.Tracer("NodeTracer")
		// n.InitWPContext(wp)
		tags := trace.WithAttributes(attribute.String("WorkpackageHash", common.Str(wp.Hash())))
		ctx, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] broadcastWorkpackage", n.store.NodeID), tags)
		n.store.UpdateWorkPackageContext(ctx)
		defer span.End()
	}
	// counting the time for this function execution
	timer := time.Now()
	currTimeslot := curr_statedb.GetTimeslot()
	coreIndex := wpCoreIndex
	if err != nil {
		Logger.RecordLogs(storage.EG_error, fmt.Sprintf("%s [broadcastWorkPackage] GetCoreCoWorkerPeersByStateDB Error: %v\n", n.String(), err), true)
	}
	coworkers := n.GetCoreCoWorkerPeersByStateDB(wpCoreIndex, curr_statedb)
	if debugDA {
		fmt.Printf("%s n.Core: %d, | wpCoreIndex=%v, WorkPackageHash=%v, len(coworkers)=%x\n", n.String(), coreIndex, wpCoreIndex, wp.Hash(), len(coworkers))
	}
	if debugSegments {
		fmt.Printf("[N%d] broadcastWorkpackage executeWorkPackage\n", n.id)
	}
	importedSegments, err := n.FetchWorkpackageImportSegments(wp)
	if err != nil {
		Logger.RecordLogs(storage.EG_error, fmt.Sprintf("%s [broadcastWorkPackage] FetchWorkpackageImportSegments Error: %v\n", n.String(), err), true)
		return types.Guarantee{}, fmt.Errorf("%s [broadcastWorkPackage] FetchWorkpackageImportSegments Error: %v\n", n.String(), err)
	}
	segmentRootLookup, err := n.GetSegmentRootLookup(wp)
	if err != nil {
		Logger.RecordLogs(storage.EG_error, fmt.Sprintf("%s [broadcastWorkPackage] GetSegmentRootLookup Error: %v\n", n.String(), err), true)
	}
	if debugG {
		fmt.Printf("%s [broadcastWorkPackage] Guarantee from self\n", n.String())
	}
	bundle := n.CompilePackageBundle(wp, importedSegments, extrinsics)
	var wg sync.WaitGroup
	mutex := &sync.Mutex{}
	fellow_responses := make(map[types.Ed25519Key]JAMSNPWorkPackageShareResponse)
	for _, coworker := range coworkers {
		wg.Add(1)
		go func(coworker Peer) {
			defer wg.Done()
			// if it's itself, execute the workpackage
			if coworker.PeerID == n.id {
				var execErr error
				report, execErr := n.executeWorkPackageBundle(wpCoreIndex, bundle, segmentRootLookup)
				if execErr != nil {
					Logger.RecordLogs(storage.EG_error, fmt.Sprintf("%s [broadcastWorkPackage] executeWorkPackage Error: %v\n", n.String(), execErr), true)
					return
				}
				guarantee.Report = report
				signerSecret := n.GetEd25519Secret()
				gc := report.Sign(signerSecret, uint16(n.GetCurrValidatorIndex()))
				guarantee.Signatures = append(guarantee.Signatures, gc)
				return
			} else {
				fellow_response, errfellow := coworker.ShareWorkPackage(wpCoreIndex, bundle, segmentRootLookup, coworker.Validator.Ed25519)
				if errfellow != nil {
					Logger.RecordLogs(storage.EG_error, fmt.Sprintf("%s [broadcastWorkPackage] ShareWorkPackage Error: %v\n", n.String(), errfellow), true)
					return
				}
				mutex.Lock()
				fellow_responses[coworker.Validator.Ed25519] = fellow_response
				mutex.Unlock()
			}

		}(coworker)
	}
	wg.Wait()
	selfReport := guarantee.Report
	selfWorkReportHash := guarantee.Report.Hash()
	// go Logger.RecordLogs(storage.EG_status, fmt.Sprintf("%s [broadcastWorkPackage] outgoing workReport: %v\n", n.String(), selfReport.String()), true)
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
			fmt.Printf("%s [broadcastWorkPackage] outgoing workReport: %v\n", n.String(), selfReport.String())
			fmt.Printf("%s [broadcastWorkPackage] outgoing guarantee: %v\n", n.String(), guarantee.String())
			error_string := fmt.Sprintf("%s [broadcastWorkPackage] Guarantee from fellow [N%d] did not match! \neg_wr: %v, fellow_wr: %v\n", n.String(), validator_idx, selfWorkReportHash, fellowWorkReportHash)
			//panic("Guarantee from fellow did not match!")
			Logger.RecordLogs(storage.EG_error, error_string, true)
			return
		}
	}

	if len(guarantee.Signatures) >= 2 {
		if len(guarantee.Signatures) == 2 {
			fmt.Printf("WARNING: ABNORMAL- Only 2 signatures, expected 3! This is not normal!")
		}
		guarantee.Slot = currTimeslot
		go n.broadcast(guarantee)
		eclapsed := time.Since(timer)
		Logger.RecordLogs(storage.EG_status, fmt.Sprintf("%s [broadcastWorkPackage] outgoing guarantee(%v) for core%d, took %s\n",
			n.String(), guarantee.Report.GetWorkPackageHash().String_short(), guarantee.Report.CoreIndex, eclapsed.String()), true)
		n.processGuarantee(guarantee)
		log := fmt.Sprintf("%s (core %d) [broadcast guarantee in slot %d, actually slot %d]\n", n.String(), coreIndex, guarantee.Slot, n.statedb.GetTimeslot())
		Logger.RecordLogs(storage.Grandpa_status, log, true)
	}
	return
}
