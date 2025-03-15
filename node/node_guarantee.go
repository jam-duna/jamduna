package node

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
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
		log.Error(debugG, "broadcastWorkPackage:GetCoreCoWorkerPeersByStateDB", "n", n.String(), "err", err)
	}
	coworkers := n.GetCoreCoWorkerPeersByStateDB(wpCoreIndex, curr_statedb)
	log.Debug(debugDA, "broadcastWorkpackage", "n", n.String(), "n.Core", coreIndex, "wpCoreIndex", wpCoreIndex, "WorkPackageHash", wp.Hash(), "len(coworkers)", len(coworkers))
	// here we are a first guarantor making justifications and reconstructSegments is used inside FetchWorkpackageImportSegments
	importedSegments, justifications, err := n.FetchWorkpackageImportSegments(wp)
	if err != nil {
		log.Error(debugG, "broadcastWorkPackage:FetchWorkpackageImportSegments", "n", n.String(), "err", err)
		return types.Guarantee{}, fmt.Errorf("%s [broadcastWorkPackage] FetchWorkpackageImportSegments Error: %v\n", n.String(), err)
	}
	segmentRootLookup, err := n.GetSegmentRootLookup(wp)
	if err != nil {
		log.Error(debugG, "broadcastWorkPackage:GetSegmentRootLookup", "n", n.String(), "err", err)
	}
	log.Debug(debugG, "broadcastWorkPackage:Guarantee from self", "id", n.String())

	// s - [ImportSegmentData] should be size of G = W_E * W_S
	// TODO: this should be codec encoded
	bundle := types.WorkPackageBundle{
		WorkPackage:       wp,
		ExtrinsicData:     extrinsics,
		ImportSegmentData: importedSegments,
		Justification:     justifications, // this is something the recipients can check the ImportedSegmentData against the WorkItems in the WorkPackage
	}

	err = curr_statedb.VerifyPackage(bundle)
	if err != nil {
		log.Error(debugG, "broadcastWorkPackage:CompilePackageBundle", "n", n.String(), "err", err)
		return types.Guarantee{}, fmt.Errorf("%s [broadcastWorkPackage] CompilePackageBundle Error: %v\n", n.String(), err)
	}
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
					log.Error(debugG, "broadcastWorkPackage:executeWorkPackage", "n", n.String(), "err", execErr)
					panic(fmt.Sprintf("executeWorkPackage Error: %v", execErr))
				}
				guarantee.Report = report
				signerSecret := n.GetEd25519Secret()
				gc := report.Sign(signerSecret, uint16(n.GetCurrValidatorIndex()))
				guarantee.Signatures = append(guarantee.Signatures, gc)
				return
			} else {
				fellow_response, errfellow := coworker.ShareWorkPackage(wpCoreIndex, bundle, segmentRootLookup, coworker.Validator.Ed25519)
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
			fmt.Printf("")
			log.Crit(debugG, "broadcastWorkpackage Guarantee from fellow did not match", "n", n.String(),
				"selfWorkReportHash", selfWorkReportHash, "fellowWorkReportHash", fellowWorkReportHash)

			panic(9234)
			return
		}
	}

	if len(guarantee.Signatures) >= 2 {
		if len(guarantee.Signatures) == 2 {
			log.Warn(debugG, "broadcastWorkpackage:Only 2 signatures, expected 3")
		}
		guarantee.Slot = currTimeslot
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
	return
}
