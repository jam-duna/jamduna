//go:build network_test
// +build network_test

package node

import (
	"fmt"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

func safrole(jceManager *ManualJCEManager) {
	if jceManager != nil {
		// replenish the JCE
		go jceManager.Replenish()
	}
}

// Monitor the Timeslot & Epoch Progression & Kill as Necessary
func (n *Node) TerminateAt(offsetTimeSlot uint32, maxTimeAllowed uint32) (bool, error) {
	initialTimeSlot := uint32(0)
	startTime := time.Now()

	// Terminate it at epoch N
	statusTicker := time.NewTicker(3 * time.Second)
	defer statusTicker.Stop()

	for done := false; !done; {
		<-statusTicker.C
		currTimeSlot := n.statedb.GetSafrole().Timeslot
		if initialTimeSlot == 0 && currTimeSlot > 0 {
			currEpoch, _ := n.statedb.GetSafrole().EpochAndPhase(currTimeSlot)
			initialTimeSlot = uint32(currEpoch) * types.EpochLength
		}
		currEpoch, currPhase := n.statedb.GetSafrole().EpochAndPhase(currTimeSlot)
		if currTimeSlot-initialTimeSlot >= offsetTimeSlot {
			done = true
			continue
		}
		if time.Since(startTime).Seconds() >= float64(maxTimeAllowed) {
			s := fmt.Sprintf("[TIMEOUT] H_t=%v e'=%v,m'=%v | missing %v Slot!", currTimeSlot, currEpoch, currPhase, currTimeSlot-initialTimeSlot)
			return false, fmt.Errorf(s)
		}
	}
	return true, nil
}

func waitForTermination(watchNode *Node, caseType string, targetedEpochLen int, bufferTime int, t *testing.T) {
	targetTimeslotLength := uint32(targetedEpochLen * types.EpochLength)
	maxTimeAllowed := (targetTimeslotLength+1)*types.SecondsPerSlot + uint32(bufferTime)

	done := make(chan bool, 1)
	errChan := make(chan error, 1)

	go func() {
		ok, err := watchNode.TerminateAt(targetTimeslotLength, maxTimeAllowed)
		if err != nil {
			errChan <- err
		} else if ok {
			done <- true
		}
	}()

	select {
	case <-done:
		log.Info(log.Node, "Completed")
	case err := <-errChan:
		t.Fatalf("[%v] Failed: %v", caseType, err)
	}
}
