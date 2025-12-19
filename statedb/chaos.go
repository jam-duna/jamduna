package statedb

import (
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func SimulateBlockAuthoringInterruption(blk *types.Block) (authoring_supressed bool) {
	if blk == nil {
		return false
	}
	slot := blk.Header.Slot
	epoch, phase := ComputeEpochAndPhase(blk.Header.Slot)
	r := blk.Header.Seal[0] //

	var pNot float64 // prob of NOT Brodcasting
	caseStr := "default"
	switch {
	case phase == 0:
		caseStr = "first_phase"
		pNot = 0.2
	case phase > 0 && phase < types.TicketSubmissionEndSlot:
		caseStr = "ticket_submission_phase"
		pNot = 0.2
	case phase > types.TicketSubmissionEndSlot && phase < types.EpochLength-1:
		caseStr = "submission_closed_phase"
		pNot = 0.3
	default:
		pNot = 0
	}
	threshold := uint8(pNot * 256)
	if r < threshold {
		log.Warn(log.SDB, "Simulated Interruption: Not Broadcasting/Proposing", "n", blk.Header.AuthorIndex, "slot", slot, "e'", epoch, "p'", phase, "case", caseStr, "r", float64(r)/256, "threshold", pNot)
		return true
	}
	return false
}
