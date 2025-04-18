package types

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
)

type ExtrinsicPool struct {
	// assurances queue storage
	queuedAssurances map[common.Hash]map[uint16]*Assurance
	assuranceMutex   sync.Mutex
	// guarantees queue storage
	queuedGuarantees map[uint32]map[uint16]*Guarantee // use timeslot to store guarantees, and core index to distinguish
	knownGuarantees  map[common.Hash]*uint32          // use package hash to store guarantees
	guaranteeMutex   sync.Mutex
	// tickets queue storage
	queuedTickets map[common.Hash]map[common.Hash]*Ticket // use entropy hash to store tickets, and ticket id to distinguish
	knownTickets  map[common.Hash]struct{}                // use first 32 bytes of ticket signature to distinguish
	ticketMutex   sync.Mutex
	// preimage queue storage
	queuedPreimages map[common.Hash]*Preimages // use AccountPreimageHash hash to store preimages
	knownPreimages  map[common.Hash]uint32     // use AccountPreimageHash hash to store preimages
	preimageMutex   sync.Mutex
}

func NewExtrinsicPool() *ExtrinsicPool {
	return &ExtrinsicPool{
		queuedAssurances: make(map[common.Hash]map[uint16]*Assurance),
		queuedGuarantees: make(map[uint32]map[uint16]*Guarantee),
		knownGuarantees:  make(map[common.Hash]*uint32),
		queuedTickets:    make(map[common.Hash]map[common.Hash]*Ticket),
		knownTickets:     make(map[common.Hash]struct{}),
		queuedPreimages:  make(map[common.Hash]*Preimages),
		knownPreimages:   make(map[common.Hash]uint32),
	}
}

func (ep *ExtrinsicPool) RemoveUsedExtrinsicFromPool(block *Block, used_entropy common.Hash, IsTicketSubmissionClosed bool) {
	parent_hash := block.Header.ParentHeaderHash
	// Remove assurances
	ep.RemoveAssurancesFromPool(parent_hash)
	// Remove guarantees
	for _, guarantee := range block.Extrinsic.Guarantees {
		ep.RemoveOldGuarantees(guarantee)
	}
	// Remove tickets
	if IsTicketSubmissionClosed { // if ticket submission is closed, remove all tickets from useless entropy
		ep.RemoveTicketsFromPool(used_entropy)
	} else {
		ep.RemoveOldTickets(block.Extrinsic.Tickets, used_entropy)
	}
	// Remove preimages
	ep.RemoveOldPreimages(block.Extrinsic.Preimages, block.Header.Slot)
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
	if _, exists := ep.knownGuarantees[guarantee.Report.AvailabilitySpec.WorkPackageHash]; exists {
		return fmt.Errorf("guarantee %s already exists", guarantee.Report.AvailabilitySpec.WorkPackageHash.String_short())
	}
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

func (ep *ExtrinsicPool) GetSpecGuaranteeFromPool(accepted_slot uint32, core_index uint16) *Guarantee {
	ep.guaranteeMutex.Lock()
	defer ep.guaranteeMutex.Unlock()
	if _, exists := ep.queuedGuarantees[accepted_slot]; exists {
		if guarantee, exists := ep.queuedGuarantees[accepted_slot][core_index]; exists {
			return guarantee
		}
	}
	return nil
}

// remove the guarantees that are already used by the block
func (ep *ExtrinsicPool) RemoveOldGuarantees(guarantee Guarantee) error {
	ep.guaranteeMutex.Lock()
	defer ep.guaranteeMutex.Unlock()
	if slotMap, exists := ep.queuedGuarantees[guarantee.Slot]; exists {
		delete(slotMap, guarantee.Report.CoreIndex)
		if len(slotMap) == 0 {
			delete(ep.queuedGuarantees, guarantee.Slot)
		}
	}
	known_guarantee_hash := guarantee.Report.AvailabilitySpec.WorkPackageHash
	ep.knownGuarantees[known_guarantee_hash] = &guarantee.Slot
	return nil // Success
}

// we need to remove the tickets that are already useless (outdated)
func (ep *ExtrinsicPool) RemoveGuaranteesFromPool(accepted_slot uint32) error {
	ep.guaranteeMutex.Lock()
	defer ep.guaranteeMutex.Unlock()
	if _, exists := ep.queuedGuarantees[accepted_slot]; exists {
		delete(ep.queuedGuarantees, accepted_slot)
	}
	for guarantee_hash, slot := range ep.knownGuarantees {
		if *slot < accepted_slot-2*EpochLength {
			delete(ep.knownGuarantees, guarantee_hash)
		}
	}
	return nil // Success
}

func (ep *ExtrinsicPool) IsSeenTicket(ticket Ticket) bool {
	ep.ticketMutex.Lock()
	defer ep.ticketMutex.Unlock()
	// Check if the ticket is already known
	ticket_short := ticket.Signature[:32]
	hash := common.Hash(ticket_short)
	if _, exists := ep.knownTickets[hash]; exists {
		return true
	}
	return false
}

func (ep *ExtrinsicPool) AddTicketToPool(ticket Ticket, id common.Hash, used_entropy common.Hash) error {
	ep.ticketMutex.Lock()
	defer ep.ticketMutex.Unlock()

	ticket_short := ticket.Signature[:32]
	hash := common.Hash(ticket_short)
	if _, exists := ep.knownTickets[hash]; exists {
		return fmt.Errorf("ticket %s already exists", ticket.String())
	} else {
		ep.knownTickets[hash] = struct{}{}
	}
	// Store the ticket in the tip's queued ticket
	// Ensure the map for this anchor exists

	if _, exists := ep.queuedTickets[used_entropy]; !exists {
		ep.queuedTickets[used_entropy] = make(map[common.Hash]*Ticket)
	}
	// Store the ticket in the appropriate map
	// TODO: id to blake2b hash
	ep.queuedTickets[used_entropy][id] = &ticket

	return nil // Success
}

// get the tickets for the given used_entropy
func (ep *ExtrinsicPool) GetTicketsFromPool(used_entropy common.Hash) []Ticket {
	tickets := make([]Ticket, 0)
	ep.ticketMutex.Lock()
	defer ep.ticketMutex.Unlock()
	if _, exists := ep.queuedTickets[used_entropy]; exists {
		for _, ticket := range ep.queuedTickets[used_entropy] {
			tickets = append(tickets, *ticket)
		}
	}
	return tickets
}

func (ep *ExtrinsicPool) GetTicketIDPairFromPool(used_entropy common.Hash) map[common.Hash]common.Hash {
	tickets := make(map[common.Hash]common.Hash)
	ep.ticketMutex.Lock()
	defer ep.ticketMutex.Unlock()
	if _, exists := ep.queuedTickets[used_entropy]; exists {
		for id, ticket := range ep.queuedTickets[used_entropy] {
			tickets[ticket.Hash()] = id
		}
	}
	return tickets
}

// remove the tickets that are already used by the block
func (ep *ExtrinsicPool) RemoveOldTickets(tickets []Ticket, entropy common.Hash) error {
	ep.ticketMutex.Lock()
	defer ep.ticketMutex.Unlock()
	if _, exists := ep.queuedTickets[entropy]; exists {
		for _, ticket := range tickets {
			id, err := ticket.TicketID()
			if err != nil {
				return err
			}
			delete(ep.queuedTickets[entropy], id)
		}
		if len(ep.queuedTickets[entropy]) == 0 {
			delete(ep.queuedTickets, entropy)
		}
	}
	return nil // Success
}

// get the tickets from the pool that are the same as the given ticket
func (ep *ExtrinsicPool) GetSameTicketsFromPool(ticket []Ticket, used_entropy common.Hash) []Ticket {
	tickets := make([]Ticket, 0)
	ep.ticketMutex.Lock()
	defer ep.ticketMutex.Unlock()
	if _, exists := ep.queuedTickets[used_entropy]; exists {
		for _, ticket_ := range ep.queuedTickets[used_entropy] {
			for _, ticket_2 := range ticket {
				if reflect.DeepEqual(*ticket_, ticket_2) {
					tickets = append(tickets, *ticket_)
				}
			}
		}
	}
	return tickets
}

// remove specific ticket from the pool
func (ep *ExtrinsicPool) RemoveTicketFromPool(ticket_id common.Hash, used_entropy common.Hash) error {
	ep.ticketMutex.Lock()
	defer ep.ticketMutex.Unlock()
	if _, exists := ep.queuedTickets[used_entropy]; exists {
		delete(ep.queuedTickets[used_entropy], ticket_id)
		if len(ep.queuedTickets[used_entropy]) == 0 {
			delete(ep.queuedTickets, used_entropy)
		}
	}
	return nil // Success
}

// this function is used to remove all tickets generated by used_entropy from the pool
func (ep *ExtrinsicPool) RemoveTicketsFromPool(used_entropy common.Hash) error {
	ep.ticketMutex.Lock()
	defer ep.ticketMutex.Unlock()
	if _, exists := ep.queuedTickets[used_entropy]; exists {
		delete(ep.queuedTickets, used_entropy)
	}
	return nil // Success
}

func (ep *ExtrinsicPool) ForgetPreimages(preimages []*SubServiceRequestResult) error {
	ep.preimageMutex.Lock()
	defer ep.preimageMutex.Unlock()
	for _, preimage := range preimages {
		// use AccountHash - combines serviceID + preimageHash
		ah := ComputeAccountHash(preimage.ServiceID, preimage.Hash)
		if _, exists := ep.knownPreimages[ah]; exists {
			delete(ep.knownPreimages, ah)
		}
	}
	return nil // Success
}

func (ep *ExtrinsicPool) AddPreimageToPool(preimage Preimages) error {
	ep.preimageMutex.Lock()
	defer ep.preimageMutex.Unlock()
	// Store the preimage in the tip's queued preimage -- TODO: use AccountHash instead
	ah := ComputeAccountHash(preimage.Requester, preimage.Hash())
	if _, exists := ep.knownPreimages[ah]; exists {
		log.Warn("authoring", "AddPreimageToPool: EXISTS -- DID we have a forget or is this actual spam")
		return nil
	}
	ep.queuedPreimages[ah] = &preimage

	return nil // Success
}

func (ep *ExtrinsicPool) GetPreimageFromPool() []*Preimages {
	ep.preimageMutex.Lock()
	defer ep.preimageMutex.Unlock()
	preimages := make([]*Preimages, 0)
	for ah, preimage := range ep.queuedPreimages {
		if _, exists := ep.knownPreimages[ah]; exists {
			continue
		}
		preimages = append(preimages, preimage)
	}
	return preimages
}

func (ep *ExtrinsicPool) GetPreimageByHash(preimageHash common.Hash) (*Preimages, bool) {
	ep.preimageMutex.Lock()
	defer ep.preimageMutex.Unlock()
	for _, x := range ep.queuedPreimages {
		if x.Hash() == preimageHash {
			return x, true
		}
	}
	return nil, false
}

// remove preimages from the pool that are already known or used by the block
func (ep *ExtrinsicPool) RemoveOldPreimages(block_EPs []Preimages, timeslot uint32) error {
	ep.preimageMutex.Lock()
	defer ep.preimageMutex.Unlock()
	for _, block_EP := range block_EPs {
		ah := block_EP.AccountHash()
		if _, exists := ep.queuedPreimages[ah]; exists {
			delete(ep.queuedPreimages, ah)
		}
		if _, exists := ep.knownPreimages[ah]; exists {
			delete(ep.knownPreimages, ah)
		}
		timeslot_tmp := timeslot
		ep.knownPreimages[ah] = timeslot_tmp
	}
	// remove the known preimages by time slot
	for account_preimage_hash, ts := range ep.knownPreimages {
		if ts < timeslot-2*EpochLength {
			delete(ep.knownPreimages, account_preimage_hash)
		}
	}
	return nil // Success
}
