package statedb

import (
	"fmt"
	"github.com/colorfulnotion/jam/pvm"
)

func (s *StateDB) OnTransfer() error {
	for _, core := range s.JamState.AvailabilityAssignments {
		if core == nil {
			continue
		}
		code, err := s.getServiceCoreCode(uint32(core.WorkReport.CoreIndex))
		if err == nil {
			fmt.Printf("OnTransfers %d\n", core.WorkReport.CoreIndex)
			vm := pvm.NewVMFromCode(uint32(core.WorkReport.CoreIndex), code, 0, s)
			vm.ExecuteTransfer(s.X.T)
		}
	}
	return nil
}
