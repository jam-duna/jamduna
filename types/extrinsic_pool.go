package types

import (
	"sync"

	"github.com/colorfulnotion/jam/common"
)

type ExtrinsicPool struct {
	// entire pool mutex
	poolMutex sync.Mutex
	// assurances queue storage
	queuedAssurances map[common.Hash]map[uint16]*Assurance
	assuranceMutex   sync.Mutex
	// guarantees queue storage
	queuedGuarantees map[uint32]map[uint16]*Guarantee // use timeslot to store guarantees, and core index to distinguish
	guaranteeMutex   sync.Mutex
}

func NewExtrinsicPool() *ExtrinsicPool {
	return &ExtrinsicPool{
		queuedAssurances: make(map[common.Hash]map[uint16]*Assurance),
		queuedGuarantees: make(map[uint32]map[uint16]*Guarantee),
	}
}

func (ep *ExtrinsicPool) RemoveUsedExtrinsicFromPool(block *Block) {
	parent_hash := block.Header.ParentHeaderHash
	// Remove assurances
	ep.RemoveAssurancesFromPool(parent_hash)
	// Remove guarantees
	for _, guarantee := range block.Extrinsic.Guarantees {
		ep.RemoveOldGuarantees(guarantee)
	}

}

func (ep *ExtrinsicPool) AddAssuranceToPool(assurance Assurance) error {
	ep.assuranceMutex.Lock()
	defer ep.assuranceMutex.Unlock()
	// Store the assurance in the tip's queued assurance
	// Ensure the map for this anchor exists
	if _, exists := ep.queuedAssurances[assurance.Anchor]; !exists {
		ep.queuedAssurances[assurance.Anchor] = make(map[uint16]*Assurance)
	}
	// Store the assurance in the appropriate map
	ep.queuedAssurances[assurance.Anchor][assurance.ValidatorIndex] = &assurance
	return nil // Success
}

func (ep *ExtrinsicPool) GetAssurancesFromPool(parentHash common.Hash) []Assurance {
	assurances := make([]Assurance, 0)
	ep.assuranceMutex.Lock()
	defer ep.assuranceMutex.Unlock()
	if _, exists := ep.queuedAssurances[parentHash]; exists {
		for _, assurance := range ep.queuedAssurances[parentHash] {
			assurances = append(assurances, *assurance)
		}
	}
	return assurances
}

func (ep *ExtrinsicPool) RemoveOldAssurances(assurance Assurance) error {
	ep.assuranceMutex.Lock()
	defer ep.assuranceMutex.Unlock()
	if _, exists := ep.queuedAssurances[assurance.Anchor]; exists {
		if _, exists := ep.queuedAssurances[assurance.Anchor][assurance.ValidatorIndex]; exists {
			delete(ep.queuedAssurances[assurance.Anchor], assurance.ValidatorIndex)
		}
	}
	return nil // Success
}

func (ep *ExtrinsicPool) RemoveAssurancesFromPool(parentHash common.Hash) error {
	ep.assuranceMutex.Lock()
	defer ep.assuranceMutex.Unlock()
	if _, exists := ep.queuedAssurances[parentHash]; exists {
		delete(ep.queuedAssurances, parentHash)
	}
	return nil // Success
}

func (ep *ExtrinsicPool) AddGuaranteeToPool(guarantee Guarantee) error {
	ep.guaranteeMutex.Lock()
	defer ep.guaranteeMutex.Unlock()
	// Store the guarantee in the tip's queued guarantee
	//fmt.Printf("%s [node:processGuarantee]\n", n.String()) // , guarantee.String()
	if _, exists := ep.queuedGuarantees[guarantee.Slot]; !exists {
		ep.queuedGuarantees[guarantee.Slot] = make(map[uint16]*Guarantee)
	}
	ep.queuedGuarantees[guarantee.Slot][guarantee.Report.CoreIndex] = &guarantee
	return nil // Success
}

/*
Get the guarantees for the given accepted slot
any guarantee that is younger than the accepted slot will be removed
*/
func (ep *ExtrinsicPool) GetGuaranteesFromPool(accepted_slot uint32) []Guarantee {
	guarantees := make([]Guarantee, 0)
	ep.guaranteeMutex.Lock()
	defer ep.guaranteeMutex.Unlock()
	for slot, guaranteesMap := range ep.queuedGuarantees {
		if slot > accepted_slot {
			for _, guarantee := range guaranteesMap {
				guarantees = append(guarantees, *guarantee)
			}
		} else {
			delete(ep.queuedGuarantees, slot)
		}
	}
	return guarantees
}

func (ep *ExtrinsicPool) RemoveOldGuarantees(guarantee Guarantee) error {
	ep.guaranteeMutex.Lock()
	defer ep.guaranteeMutex.Unlock()
	if _, exists := ep.queuedGuarantees[guarantee.Slot]; exists {
		if _, exists := ep.queuedGuarantees[guarantee.Slot][guarantee.Report.CoreIndex]; exists {
			delete(ep.queuedGuarantees[guarantee.Slot], guarantee.Report.CoreIndex)
		}
	}
	return nil // Success
}

func (ep *ExtrinsicPool) RemoveGuaranteesFromPool(accepted_slot uint32) error {
	ep.guaranteeMutex.Lock()
	defer ep.guaranteeMutex.Unlock()
	if _, exists := ep.queuedGuarantees[accepted_slot]; exists {
		delete(ep.queuedGuarantees, accepted_slot)
	}
	return nil // Success
}
