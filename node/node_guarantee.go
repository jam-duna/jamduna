package node

import (
	"fmt"
	"sort"
	"sync"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func (n *Node) broadcastWorkpackage(wp types.WorkPackage, wpCoreIndex uint16, curr_statedb *statedb.StateDB) (guarantee types.Guarantee, err error) {
	currTimeslot := curr_statedb.GetTimeslot()
	coreIndex := wpCoreIndex
	if err != nil {
		Logger.RecordLogs(EG_error, fmt.Sprintf("%s [broadcastWorkPackage] GetCoreCoWorkerPeersByStateDB Error: %v\n", n.String(), err), true)
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
		Logger.RecordLogs(EG_error, fmt.Sprintf("%s [broadcastWorkPackage] FetchWorkpackageImportSegments Error: %v\n", n.String(), err), true)
	}
	segmentRootLookup, err := n.GetSegmentRootLookup(wp)
	if err != nil {
		Logger.RecordLogs(EG_error, fmt.Sprintf("%s [broadcastWorkPackage] GetSegmentRootLookup Error: %v\n", n.String(), err), true)
	}
	if debugG {
		fmt.Printf("%s [broadcastWorkPackage] Guarantee from self\n", n.String())
	}
	bundle := n.CompilePackageBundle(wp, importedSegments)
	var wg sync.WaitGroup
	mutex := &sync.Mutex{}
	fellow_responses := make(map[types.Ed25519Key]JAMSNPWorkPackageShareResponse)
	for _, coworker := range coworkers {
		wg.Add(1)
		go func(coworker Peer) {
			defer wg.Done()

			if coworker.PeerID == n.id {
				var execErr error
				guarantee, _, _, execErr = n.executeWorkPackage(wpCoreIndex, wp, importedSegments, segmentRootLookup)
				if execErr != nil {
					Logger.RecordLogs(EG_error, fmt.Sprintf("%s [broadcastWorkPackage] executeWorkPackage Error: %v\n", n.String(), execErr), true)
					return
				}
				return
			}

			fellow_response, errfellow := coworker.ShareWorkPackage(wpCoreIndex, bundle, segmentRootLookup, coworker.Validator.Ed25519)
			if errfellow != nil {
				Logger.RecordLogs(EG_error, fmt.Sprintf("%s [broadcastWorkPackage] ShareWorkPackage Error: %v\n", n.String(), errfellow), true)
				return
			}

			mutex.Lock()
			fellow_responses[coworker.Validator.Ed25519] = fellow_response
			mutex.Unlock()
		}(coworker)
	}
	wg.Wait()
	selfReport := guarantee.Report
	selfWorkReportHash := guarantee.Report.Hash()
	// go Logger.RecordLogs(EG_status, fmt.Sprintf("%s [broadcastWorkPackage] outgoing workReport: %v\n", n.String(), selfReport.String()), true)
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
			Logger.RecordLogs(EG_error, error_string, true)
			return
		}
	}

	if len(guarantee.Signatures) < 3 {
		//TODO: Shawn - if more than 2s has passed after receiving 2nd sig, you can potentiall move on.
		panic(222)
	} else {
		guarantee.Slot = currTimeslot
		go n.broadcast(guarantee)

		Logger.RecordLogs(EG_status, fmt.Sprintf("%s [broadcastWorkPackage] outgoing guarantee(%v) for core%d\n",
			n.String(), guarantee.Report.GetWorkPackageHash().String_short(), guarantee.Report.CoreIndex), true)

		n.processGuarantee(guarantee)
		log := fmt.Sprintf("%s (core %d) [broadcast guarantee in slot %d]\n", n.String(), coreIndex, guarantee.Slot)
		Logger.RecordLogs(grandpa_status, log, true)
	}
	return
}
