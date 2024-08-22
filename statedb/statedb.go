package statedb

import (
	//"github.com/ethereum/go-ethereum/crypto"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/corejam"
	"github.com/colorfulnotion/jam/safrole"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

type Message struct {
	MsgType string
	Payload interface{}
}

type StateDB struct {
	Id              uint32                 `json:"id"`
	Block           *types.Block           `json:"block"`
	ParentHash      common.Hash            `json:"parentHash"`
	BlockHash       common.Hash            `json:"blockHash"`
	StateRoot       common.Hash            `json:"stateRoot"`
	Safrole         *safrole.SafroleState  `json:"safrole"`
	Corejam         *corejam.CorejamState  `json:"corejam"`
	Dispute         *safrole.DisputeState `json:"dispute"`
	sdb             *storage.StateDBStorage
	trie            *trie.MerkleTree

	knownPreimageLookups map[common.Hash]int
	queuedPreimageLookups map[common.Hash]types.PreimageLookup
	preimageLookupsMutex  sync.Mutex
	
	knownGuarantees map[common.Hash]int
	queuedGuarantees map[common.Hash]types.Guarantee
	guaranteeMutex  sync.Mutex

	knownAssurances map[common.Hash]int
	queuedAssurances map[common.Hash]types.Assurance
	assuranceMutex  sync.Mutex

	knownDisputes map[common.Hash]int
	queuedDisputes map[common.Hash]types.Dispute
	disputeMutex  sync.Mutex

	knownTickets map[common.Hash]int
	queuedTickets map[common.Hash]types.Ticket
	ticketMutex  sync.Mutex
}

func (s *StateDB) AddTicketToQueue(t types.Ticket) {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	s.queuedTickets[t.TicketID()] = t
}

func (s *StateDB) AddDisputeToQueue(d types.Dispute) {
	s.disputeMutex.Lock()
	defer s.disputeMutex.Unlock()
	s.queuedDisputes[d.Hash()] = d
}

func (s *StateDB) AddGuaranteeToQueue(g types.Guarantee) {
	s.guaranteeMutex.Lock()
	defer s.guaranteeMutex.Unlock()
	s.queuedGuarantees[g.Hash()] = g
}

func (s *StateDB) AddAssuranceToQueue(a types.Assurance) {
	s.assuranceMutex.Lock()
	defer s.assuranceMutex.Unlock()
	s.queuedAssurances[a.Hash()] = a
}

func (s *StateDB) CheckTicketExists(ticketID common.Hash) bool {
	if (ticketID == common.Hash{}) {
		return true
	}
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	_, exists := s.knownTickets[ticketID]
	return exists
}

func (s *StateDB) ProcessIncomingBlock(b *types.Block) error {
	currEpoch, currPhase := s.Safrole.EpochAndPhase(b.Header.TimeSlot)
	// TODO: validate block
	is_validated := true
	if is_validated {
		// taking the parent, applying it
		tickets := b.Tickets()
		s.Block = b
		s.ApplyStateTransitionFromBlock(context.Background(), b)
		// shawn apply the disputes state
		for _, ticket := range tickets {
			s.RemoveTicket(&ticket)
		}
		if s.Id == 0 {
			fmt.Printf("[N%v] ProcessIncomingBlock Block %v. (Epoch,Phase) = (%v,%v) Slot=%v\n", s.Id, b.Hash(), currEpoch, currPhase, b.Header.TimeSlot)
		}
		return nil
	}

	return fmt.Errorf("ERROR in validating block")
}

func (s *StateDB) ProcessIncomingTicket(t types.Ticket) {
	//s.QueueTicketEnvelope(t)
	//statedb.tickets[common.BytesToHash(ticket_id)] = t
	sf := s.GetSafrole()
	ticketID, err := sf.ValidateProposedTicket(&t)
	if err != nil {
		fmt.Printf("Invalid Ticket. Err=%v\n", err)
		return
	}
	if s.CheckTicketExists(ticketID) {
		return
	}
	// fmt.Printf("[N%v] ProcessIncomingTicket -- Adding ticketID=%v\n", s.Id, ticketID)
	s.AddTicketToQueue(t)
	s.knownTickets[ticketID] = t.Attempt
}

func (s *StateDB) ProcessIncomingDispute(d types.Dispute) {
	// get the disputes state
	disp := s.GetDisputes()
	err := disp.ValidateProposedDispute(&d);
	if err != nil {
		fmt.Printf("Invalid dispute. Err=%v\n", err)
		return
	}
	s.AddDisputeToQueue(d)
}

func (s *StateDB) ProcessIncomingGuarantee(g types.Guarantee) {
	// get the guarantee state
	cj := s.GetCorejam()
	err := cj.ValidateProposedGuarantee(&g);
	if err != nil {
		fmt.Printf("Invalid guarantee. Err=%v\n", err)
		return
	}
	s.AddGuaranteeToQueue(g)
}

func (s *StateDB) ProcessIncomingAssurance(a types.Assurance) {
	// get the assurances state
	cj := s.GetCorejam()
	err := cj.ValidateProposedAssurance(&a);
	if err != nil {
		fmt.Printf("Invalid guarantee. Err=%v\n", err)
		return
	}
	s.AddAssuranceToQueue(a)
}

func (s *StateDB) RemoveTicket(t *types.Ticket) {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	delete(s.queuedTickets, t.TicketID())
}

func (s *StateDB) RemoveDispute(d *types.Dispute) {
	s.disputeMutex.Lock()
	defer s.disputeMutex.Unlock()
	// delete(s.queuedDispute, d.Hash())
}

func newEmptyStateDB(sdb *storage.StateDBStorage) (statedb *StateDB) {
	statedb = new(StateDB)
	statedb.queuedTickets = make(map[common.Hash]types.Ticket)
	statedb.knownTickets = make(map[common.Hash]int)
	statedb.queuedDisputes = make(map[common.Hash]types.Dispute)
	statedb.knownDisputes = make(map[common.Hash]int)
	statedb.queuedGuarantees = make(map[common.Hash]types.Guarantee)
	statedb.knownGuarantees = make(map[common.Hash]int)
	statedb.queuedAssurances = make(map[common.Hash]types.Assurance)
	statedb.knownAssurances = make(map[common.Hash]int)
	statedb.queuedPreimageLookups = make(map[common.Hash]types.PreimageLookup)
	statedb.knownPreimageLookups = make(map[common.Hash]int)
	statedb.trie = trie.NewMerkleTree(nil, sdb)
	return statedb
}

// state-key constructor functions C(X)
const (
	C1  = "CoreAuthPool"
	C2  = "AuthQueue"
	C3  = "RecentBlocks"
	C4  = "safroleState"
	C5  = "PastJudgements"
	C6  = "Entropy"
	C7  = "NextEpochValidatorKeys"
	C8  = "CurrentValidatorKeys"
	C9  = "PriorEpochValidatorKeys"
	C10 = "PendingReports"
	C11 = "MostRecentBlockTimeslot"
	C12 = "PrivilegedServiceIndices"
	C13 = "ActiveValidator"
)

// NewGenesisStateDB generates the first StateDB object and genesis block
func NewGenesisStateDB(sdb *storage.StateDBStorage, c *safrole.GenesisConfig) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}

	statedb.Block = nil
	s, d := safrole.InitGenesisState(c)
	statedb.Safrole = s // setting the safrole state so that block 1 can be produced
	statedb.Dispute = d // setting the dispute state so that block 1 can be produced
	statedb.Corejam = &corejam.CorejamState{} // .InitGenesisState(c) // setting the corejam state so that block 1 can be produced
	statedb.UpdateTrieState()
	return statedb, nil
}

func (s *StateDB) GetTrie() *trie.MerkleTree {
	return s.trie
}

func (s *StateDB) GetCorejam() *corejam.CorejamState {
	return s.Corejam
}

func (s *StateDB) GetSafrole() *safrole.SafroleState {
	return s.Safrole
}

func (s *StateDB) GetDisputes() *safrole.DisputeState {
	return s.Dispute
}

// todo: implement this, not sure if this correct
func (s *StateDB) GetDisputesState() {
	//not sure if this is correct
	t := s.GetTrie()
	//TODO: deserialize the disputes state
	disputeState := safrole.DisputeState{}
	PsiBytes, err := t.GetState(C5)
	disputeState.SetPsi(PsiBytes)
	RhoBytes, err := t.GetState(C10)
	disputeState.SetRho(RhoBytes)
	tauByte, err := t.GetState(C11)
	tau := binary.BigEndian.Uint32(tauByte) // not sure if this is big endian
	disputeState.SetTau(tau)
	disputeState.SetKappa(s.Safrole.CurrValidators)
	disputeState.SetLambda(s.Safrole.PrevValidators)
	if err != nil {
		fmt.Println("Error getting disputes state", err)
	}
	s.Dispute = &disputeState
}

func (s *StateDB) UpdateTrieState() common.Hash {
	//γ ≡⎩γk, γz, γs, γa⎭
	//γk :one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	//γz :epoch’s root, a Bandersnatch ring root composed with the one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	//γa :the ticket accumulator, a series of highest-scoring ticket identifiers to be used for the next epoch (epoch N+1)
	//γs :current epoch’s slot-sealer series, which is either a full complement of E tickets or, in the case of a fallback mode, a series of E Bandersnatch keys (epoch N)
	sf := s.GetSafrole()
	sb := sf.GetSafroleBasicState()
	safroleStateEncode, _ := sb.Bytes()
	entropyEncode := sf.EntropyToBytes()
	nextEpochValidatorsEncode := sf.GetValidatorData("Next")
	currEpochValidatorsEncode := sf.GetValidatorData("Curr")
	priorEpochValidatorEncode := sf.GetValidatorData("Pre")
	mostRecentBlockTimeSlotEncode := common.EncodeUint64(uint64(sf.GetTimeSlot()))

	t := s.GetTrie()

	// Corejam: PreimageLookups, Guarantees, Assurances
	// cj := s.GetCorejam()
	// TODO
	
	// Disputes: C5, C10
	d := s.GetDisputes()
	disputeState, err := d.GetPsiBytes()
	if err != nil {
		fmt.Println("Error getting disputes state", err)
	}
	t.SetState(C5, disputeState)
	// TODO: set C10
	// IMPORTANT:dispute only modifies the C10 dagger

	// Safrole
	t.SetState(C4, safroleStateEncode)
	t.SetState(C6, entropyEncode)
	t.SetState(C7, nextEpochValidatorsEncode)
	t.SetState(C8, currEpochValidatorsEncode)
	t.SetState(C9, priorEpochValidatorEncode)
	t.SetState(C11, mostRecentBlockTimeSlotEncode)
	// TODO: C1 = "CoreAuthPool"
	// TODO: C2 = "AuthQueue"
	// TODO: C3 = "RecentBlocks"
	// TODO: C12 = "PrivilegedServiceIndices"
	// TODO: C13 = "ActiveValidator"
	return common.BytesToHash(t.GetRootHash())
}

func (s *StateDB) GetSafroleState() *safrole.SafroleState {
	return s.Safrole
}

func (s *StateDB) String() string {
	enc, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}

// newStateDB initiates the StateDB using the blockHash+bn; the bn input must refer to the epoch for which the blockHash belongs to
func newStateDB(sdb *storage.StateDBStorage, blockHash common.Hash) (statedb *StateDB, err error) {
	statedb = newEmptyStateDB(sdb)
	statedb.queuedTickets = make(map[common.Hash]types.Ticket)
	statedb.knownTickets = make(map[common.Hash]int)
	statedb.queuedDisputes = make(map[common.Hash]types.Dispute)
	statedb.knownDisputes = make(map[common.Hash]int)

	statedb.trie = trie.NewMerkleTree(nil, sdb)

	block := types.Block{}
	b := make([]byte, 32)
	zeroHash := common.BytesToHash(b)
	if bytes.Compare(blockHash.Bytes(), zeroHash.Bytes()) == 0 {
		// genesis block situation
		statedb.Safrole = safrole.NewSafroleState()
	} else {
		encodedBlock, err := sdb.ReadKV(blockHash)
		if err != nil {
			return statedb, err
		}

		h := common.Blake2AsHex(encodedBlock)
		if bytes.Compare(h.Bytes(), blockHash.Bytes()) != 0 {
			return statedb, fmt.Errorf("[statedb:newStateDB] hash of data incorrect [%d bytes]", len(encodedBlock))
		}
		if err := json.Unmarshal(encodedBlock, &block); err != nil {
			return statedb, fmt.Errorf("[statedb:newStateDB] JSON decode error: %v", err)
		}
		statedb.Block = &block
		statedb.ParentHash = block.Header.ParentHash
		statedb.Safrole = safrole.NewSafroleState() //TODO: DO entropy initiation
	}

	return statedb, nil
}

// Copy generates a copy of the StateDB
func (s *StateDB) Copy() *StateDB {
	// Create a new instance of StateDB
	n := &StateDB{
		Id:              s.Id,
		Block:           s.Block.Copy(), // You might need to deep copy the Block if it's mutable
		ParentHash:      s.ParentHash,
		BlockHash:       s.BlockHash,
		StateRoot:       s.StateRoot,
		Safrole:         s.Safrole.Copy(), // SafroleState has a Copy method
		Dispute:        s.Dispute.Copy(), // DisputesState has a Copy method
		Corejam:         s.Corejam.Copy(), // CorejamState has a Copy method
		sdb:             s.sdb,            
		trie:            s.trie,            // Deep copy if the MerkleTree is mutable
		knownTickets:    make(map[common.Hash]int),
		queuedTickets:   make(map[common.Hash]types.Ticket),
		knownDisputes:    make(map[common.Hash]int),
		queuedDisputes:   make(map[common.Hash]types.Dispute),
	}

	// Copy maps
	for k, v := range s.knownTickets {
		n.knownTickets[k] = v
	}

	for k, v := range s.queuedTickets {
		t, _ := v.DeepCopy()
		n.queuedTickets[k] = t
	}

	/*
	   for k, v := range s.queuedDisputes {
		t, _ := v.DeepCopy()
		n.queuedDisputes[k] = t
	for k, v := range s.knownDisputes {
		t, _ := v.DeepCopy()
		n.knownDisputes[k] = t
	}
	}
	*/

	return n
}

func (s *StateDB) ProcessState(credential types.ValidatorSecret) (*types.Block, *StateDB) {
	genesisReady := s.Safrole.CheckGenesisReady()
	if !genesisReady {
		return nil, nil
	}
	currJCE, timeSlotReady := s.Safrole.CheckTimeSlotReady()
	if timeSlotReady {
		// Time to propose block if authorized
		isAuthorizedBlockBuilder := false
		//bandersnatchPub := credential.BandersnatchPub
		_, phase := s.Safrole.EpochAndPhase(currJCE)
		// round robin TEMPORARY
		if phase%safrole.NumValidators == s.Id {
			//fmt.Printf("IsAuthorized caller: phase(%d) == s.Id(%d)\n", phase, s.Id)
			isAuthorizedBlockBuilder = true
		}
		
		//sf := s.GetSafrole()
		//ticketIDs := s.GetSelfTickets(targetEpoch)
		//isAuthorizedBlockBuilder = sf.IsAuthorizedBuilder(currJCE, credential, ticketIDs)
		if isAuthorizedBlockBuilder {
			currEpoch, currPhase := s.Safrole.EpochAndPhase(currJCE)
			// take the current stateDB + generate a new proposed Block and a new stateDB
			proposedBlk, newStateDB, err := s.MakeBlock(credential, currJCE)
			if err == nil {
				fmt.Printf("[N%v] Proposed %v<-%v (Epoch,Phase) = (%v,%v) Slot=%v\n", s.Id, proposedBlk.ParentHash(), proposedBlk.Hash(), currEpoch, currPhase, currJCE)
				return proposedBlk, newStateDB
			}
		} else {
			//waiting for block ... potentially submit ticket here
		}
	}
	return nil, nil
}


func (s *StateDB) SetID(id uint32) {
	s.Id = id
	s.Safrole.Id = id
}

func (s *StateDB) ApplyStateTransitionPreimages(preimages []types.PreimageLookup, targetJCE uint32, id uint32) error {
	// bring together the queuedPreimages from solicit (from DA) and update stateDB (but not BPT) with the blobs
	// solicit execution should have updated service state to "requested"
	// some NODE process should then run erasure decoding to make it "available"
	return nil
}

func (s *StateDB) ApplyStateTransitionGuarantees(guarantees []types.Guarantee, targetJCE uint32, id uint32) error {
	// TODO: Guarantee
	return nil
}

func (s *StateDB) ApplyStateTransitionAssurances(assurances []types.Assurance, targetJCE uint32, id uint32) error {
	// TODO: In the same way that extrinsic Tickets are processed, Assurances are
	return nil
}

func (s *StateDB) ApplyStateTransitionDisputes(Disputes []types.Dispute, targetJCE uint32, id uint32) (types.VerdictMarker, types.OffenderMarker, error) {
	/*
	s.GetDisputesState()
	var VMark types.VerdictMarker
	var OMark types.OffenderMarker
	// apply the disputes
	VMark, OMark, err := s.Dispute.Disputes(d)
	if err != nil {
		return types.VerdictMarker{}, types.OffenderMarker{}, err
	}
	(types.VerdictMarker, types.OffenderMarker, error)
	// VMark, OMark, nil
	*/
	// TODO
	var VMark types.VerdictMarker
	var OMark types.OffenderMarker
	s.GetDisputesState()
	for _, dispute := range Disputes {
		//get the dispute state
		//apply the dispute
		var err error
		VMark, OMark, err = s.Dispute.Disputes(dispute)
		if err != nil {
			return types.VerdictMarker{}, types.OffenderMarker{}, err
		}
		//update the state
	}
	return VMark, OMark, nil

}

// given previous safrole, applt state transition using block
// σ'≡Υ(σ,B)
func (s *StateDB) ApplyStateTransitionFromBlock(ctx context.Context, blk *types.Block) (err error) {
	targetJCE := blk.TimeSlot()

	// Preimages
	preimages := blk.PreimageLookups()
	err = s.ApplyStateTransitionPreimages(preimages, targetJCE, s.Id)
	if err != nil {
		return err
	}

	// Guarantees
	guarantees := blk.Guarantees()
	err = s.ApplyStateTransitionGuarantees(guarantees, targetJCE, s.Id)
	if err != nil {
		return err
	}

	// Assurances
	assurances := blk.Assurances()
	err = s.ApplyStateTransitionAssurances(assurances, targetJCE, s.Id)
	if err != nil {
		return err
	}

	// Disputes
	disputes := blk.Disputes()
	_, _, err = s.ApplyStateTransitionDisputes(disputes, targetJCE, s.Id)
	if err != nil {
		return err
	}
	// Safrole last
	ticketExts := blk.Tickets()
	//sf_header := blk.ConvertToSafroleHeader()
	sf_header := blk.GetHeader()
	epochMark := blk.EpochMark()
	if epochMark != nil {
		s.knownTickets = make(map[common.Hash]int) //keep track of known tickets
		s.queuedTickets = make(map[common.Hash]types.Ticket)
	}
	sf := s.GetSafrole()
	s2, err := sf.ApplyStateTransitionTickets(ticketExts, targetJCE, sf_header, s.Id)
	if err != nil {
		fmt.Printf("sf.ApplyStateTransitionFromBlock %v\n", err)
		panic(1)
		return err
	}
	s.Safrole = &s2
	s.Block = blk
	s.ParentHash = s.BlockHash
	s.BlockHash = blk.Hash()
	s.StateRoot = s.UpdateTrieState()
	fmt.Printf("ApplyStateTransitionFromBlock blk.Hash()=%v s.StateRoot=%v\n", blk.Hash(), s.StateRoot)
	return nil
}

func (s *StateDB) GetBlock() *types.Block {
	return s.Block
}

// make block generate block prior to state execution
func (s *StateDB) MakeBlock(credential types.ValidatorSecret, targetJCE uint32) (bl *types.Block, newStateDB *StateDB, err error) {
	sf := s.GetSafrole()
	isNewEpoch := sf.IsNewEpoch(targetJCE)
	needWinningMarker := sf.IsTicketSubmissionCloses(targetJCE)

	b := types.NewBlock()
	h := types.NewBlockHeader()
	extrinsicData := types.NewExtrinsic()
	h.ParentHash = s.BlockHash
	h.PriorStateRoot = s.StateRoot
	h.TimeSlot = targetJCE

	// Extrinsic Data has 5 different Extrinsics
	// cj := s.GetCorejam()

	// E_P - Preimages:  aggregate queuedPreimageLookups into extrinsicData.Preimages
	extrinsicData.PreimageLookups = make([]types.PreimageLookup, 0)
	// TODO
	for _, preimageLookup := range s.queuedPreimageLookups {
		pl, err := preimageLookup.DeepCopy()
		if err != nil {
			continue
		}
		extrinsicData.PreimageLookups = append(extrinsicData.PreimageLookups, pl)
	}
	s.queuedAssurances = make(map[common.Hash]types.Assurance)

	// E_G - Guarantees: aggregate queuedGuarantees into extrinsicData.Guarantees
	extrinsicData.Guarantees = make([]types.Guarantee, 0)
	// TODO
	for _, guarantee := range s.queuedGuarantees {
		g, err := guarantee.DeepCopy()
		if err != nil {
			continue
		}
		extrinsicData.Guarantees = append(extrinsicData.Guarantees, g)
	}
	s.queuedGuarantees = make(map[common.Hash]types.Guarantee)

	// E_A - Assurances  aggregate queuedAssurances into extrinsicData.Assurances
	extrinsicData.Assurances = make([]types.Assurance, 0)
	// TODO
	for _, assurance := range s.queuedAssurances {
		a, err := assurance.DeepCopy()
		if err != nil {
			continue
		}
		extrinsicData.Assurances = append(extrinsicData.Assurances, a)
	}
	s.queuedAssurances = make(map[common.Hash]types.Assurance)


	
	// E_D - Disputes: aggregate queuedDisputes into extrinsicData.Disputes
	d := s.GetDisputes()
	needMarkerVerdicts := d.NeedsVerdictsMarker(targetJCE)
	needMarkerOffenders := d.NeedsOffendersMarker(targetJCE)
	if needMarkerVerdicts {
		// TODO
	}
	if needMarkerOffenders {
		// TODO
	}
	extrinsicData.Disputes = make([]types.Dispute, 0)
	for _, dispute := range s.queuedDisputes {
		d, err := dispute.DeepCopy()
		if err != nil {
			continue
		}
		extrinsicData.Disputes = append(extrinsicData.Disputes, d)
	}
	s.queuedDisputes = make(map[common.Hash]types.Dispute)




	needEpochMarker := isNewEpoch && false
	if needEpochMarker {
		epochMarker := sf.GenerateEpochMarker()
		//a tuple of the epoch randomness and a sequence of Bandersnatch keys defining the Bandersnatch valida- tor keys (kb) beginning in the next epoch
		fmt.Printf("targetJCE=%v EpochMarker:%v\n", targetJCE, epochMarker)
		h.EpochMark = epochMarker
	}
	if isNewEpoch {
		h.WinningTicketsMark = make([]*types.TicketBody, 0) // clear out the tickets if its a new epoch
		s.queuedTickets = make(map[common.Hash]types.Ticket)
	}
	if needWinningMarker {
		fmt.Printf("targetJCE=%v check WinningMarker\n", targetJCE)
		winningMarker, err := sf.GenerateWinningMarker()
		//block is the first after the end of the submission period for tickets and if the ticket accumulator is saturated
		if err == nil {
			h.WinningTicketsMark = winningMarker
		}
	} else {
		// If there's new ticketID, add them into extrinsic
		// Question: can we submit tickets at the exact tail end block?
		extrinsicData.Tickets = make([]types.Ticket, 0)
		for ticketID, ticket := range s.queuedTickets {
			t, err := ticket.DeepCopy()
			if err != nil {
				continue
			}
			if s.Safrole.InTicketAccumulator(ticketID) {
			} else {
				// fmt.Printf("[N%d] GetTicketQueue %v => %v\n", s.Id, ticketID, t)
				extrinsicData.Tickets = append(extrinsicData.Tickets, t)
			}
		}
		s.queuedTickets = make(map[common.Hash]types.Ticket)
	}

	h.ExtrinsicHash = extrinsicData.Hash()
	author_index, err := sf.GetAuthorIndex(credential.BandersnatchPub, "Curr")
	if err != nil {
		return bl, newStateDB, err
	}
	h.BlockAuthorKey = author_index
	b.Extrinsic = extrinsicData

	unsignHeaderHash := h.UnsignedHash()

	//signing
	epochType := sf.CheckEpochType()
	if epochType == "fallback" {
		blockseal, fresh_vrfSig, err := sf.SignFallBack(credential.BandersnatchSecret, unsignHeaderHash)
		if err != nil {
			return bl, newStateDB, err
		}
		h.BlockSeal = blockseal
		h.VRFSignature = fresh_vrfSig
	} else {
		attempt, err := sf.GetBindedAttempt(targetJCE)
		blockseal, fresh_vrfSig, err := sf.SignPrimary(credential.BandersnatchSecret, unsignHeaderHash, attempt)
		if err != nil {
			return bl, newStateDB, err
		}
		h.BlockSeal = blockseal
		h.VRFSignature = fresh_vrfSig
	}

	b.Header = *h
	fmt.Printf("[N%d] MakeBlock StateDB (after application of Block %v) %v\n", s.Id, b.Hash(), newStateDB.String())
	newStateDB = s.Copy() // newEmptyStateDB(s.sdb)
	newSafrole := s.Safrole.Copy()
	newStateDB.Safrole = newSafrole
	newStateDB.Block = b

	newStateDB.ParentHash = b.Header.ParentHash
	newStateDB.ApplyStateTransitionFromBlock(context.Background(), b)
	return b, newStateDB, nil
}

// Flush calls each SMTs flush operation
func (s *StateDB) Flush(ctx context.Context, timeSlotIndex uint32) error {
	//log.Info(fmt.Sprintf("[statedb:Flush] Round %d updated epochRoot %x", statedb.proposedRound, statedb.epochStorage.GetRootChunkHash()))
	return nil
}

func ComputeCurrentJCETime(unixTimestamp int64) int64 {
	currentTime := time.Now()
	// this is unnecessary for PoC
	return ComputeJCETime(currentTime.Unix())
}

// The current time expressed in seconds after the start of the Jam Common Era. See section 4.4
func ComputeJCETime(unixTimestamp int64) int64 {
	production := false
	if production {
		// Define the start of the Jam Common Era
		jceStart := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)

		// Convert the Unix timestamp to a Time object
		currentTime := time.Unix(unixTimestamp, 0).UTC()

		// Calculate the difference in seconds
		diff := currentTime.Sub(jceStart)
		return int64(diff.Seconds())
	} else {
		return unixTimestamp
	}
}
