package statedb

import (
	//"github.com/ethereum/go-ethereum/crypto"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/safrole"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"sync"
	"time"
)

type Message struct {
	MsgType string
	Payload interface{}
}

type StateDB struct {
	Id              uint32                `json:"id"`
	Block           *types.Block          `json:"block"`
	ParentHash      common.Hash           `json:"parentHash"`
	BlockHash       common.Hash           `json:"blockHash"`
	StateRoot       common.Hash           `json:"stateRoot"`
	Safrole         *safrole.SafroleState `json:"safrole"`
	sdb             *storage.StateDBStorage
	credential      types.ValidatorSecret
	trie            *trie.MerkleTree
	nodeMessageChan chan Message

	selfTickets  map[uint32][]common.Hash //keep track of its own tickets
	knownTickets map[common.Hash]int      //keep track of known tickets
	queuedticket map[common.Hash]types.Ticket
	ticketMutex  sync.Mutex
}

// Function to send a message from StateDB to Node
func (sdb *StateDB) SendOutgoingMessage(msgType string, payload interface{}) {
	msg := Message{
		MsgType: msgType,
		Payload: payload,
	}
	//fmt.Printf("[N%v] SendOutgoingMessage msgType=%v\n", sdb.id, msgType)
	sdb.nodeMessageChan <- msg
}

func (s *StateDB) AddTicketToQueue(t types.Ticket) {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	s.queuedticket[t.TicketID()] = t
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
		for _, ticket := range tickets {
			s.RemoveTicket(&ticket)
		}
		if s.Id == 0 {
			fmt.Printf("[N%v] ProcessIncomingBlock Block %v. (Epoch,Phase) = (%v,%v) Slot=%v STATEDB %s\n", s.Id, b.Hash(), currEpoch, currPhase, b.Header.TimeSlot, s.String())
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

func (s *StateDB) RemoveTicket(t *types.Ticket) {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	delete(s.queuedticket, t.TicketID())

	// Iterate through selfTickets epoch by epoch, to remove the ticket from the appropriate list -- TODO: remove old epochs
	for epoch, ticketIDs := range s.selfTickets {
		var newtickets []common.Hash
		for _, h := range ticketIDs {
			if h == t.TicketID() {
			} else {
				newtickets = append(newtickets, t.TicketID())
				break
			}
		}
		s.selfTickets[epoch] = newtickets
	}
}

func newEmptyStateDB(sdb *storage.StateDBStorage) (statedb *StateDB) {
	statedb = new(StateDB)
	statedb.queuedticket = make(map[common.Hash]types.Ticket)
	statedb.knownTickets = make(map[common.Hash]int)
	statedb.selfTickets = make(map[uint32][]common.Hash)
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
	statedb.Safrole = safrole.InitGenesisState(c) // setting the safrole state so that block 1 can be produced
	statedb.UpdateTrieState()
	return statedb, nil
}

func (s *StateDB) GetTrie() *trie.MerkleTree {
	return s.trie
}

func (s *StateDB) GetSafrole() *safrole.SafroleState {
	return s.Safrole
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

	// Disputes: C5, C10
	/*
	   d := s.GetDisputes()
	   disputeState := d.GetDisputesBasicState()
	   disputeStateEncode, _ := d.Bytes()
	   t.SetState(C5, disputeStateEncode)
	   // TODO: set C10
	*/
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
	statedb.queuedticket = make(map[common.Hash]types.Ticket)
	statedb.knownTickets = make(map[common.Hash]int)
	statedb.selfTickets = make(map[uint32][]common.Hash)
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
		Safrole:         s.Safrole.Copy(), // Assuming SafroleState has a Copy method
		sdb:             s.sdb,            // Again, consider deep copying if needed
		credential:      s.credential,
		trie:            s.trie,            // Deep copy if the MerkleTree is mutable
		nodeMessageChan: s.nodeMessageChan, // Channels are reference types, consider whether to create a new channel
		selfTickets:     make(map[uint32][]common.Hash),
		knownTickets:    make(map[common.Hash]int),
		queuedticket:    make(map[common.Hash]types.Ticket),
	}

	// Copy maps
	for k, v := range s.selfTickets {
		n.selfTickets[k] = append([]common.Hash(nil), v...)
	}

	for k, v := range s.knownTickets {
		n.knownTickets[k] = v
	}

	for k, v := range s.queuedticket {
		t, _ := v.DeepCopy()
		n.queuedticket[k] = t
	}

	return n
}

func (s *StateDB) ProcessState() (*types.Block, *StateDB) {
	genesisReady := s.Safrole.CheckGenesisReady()
	if !genesisReady {
		return nil, nil
	}
	currJCE, timeSlotReady := s.Safrole.CheckTimeSlotReady()
	if timeSlotReady {
		//fmt.Printf("[N%v] Ready to start!\n", s.id)
		isAuthorizedTicketBuilder := s.IsAuthorizedTicketBuilder()
		if isAuthorizedTicketBuilder {
			s.GenerateTickets(currJCE)
		}
		// Time to propose block if selected
		isAuthorizedBlockBuilder := s.IsAuthorizedBlockBuilder(currJCE)
		if isAuthorizedBlockBuilder {
			currEpoch, currPhase := s.Safrole.EpochAndPhase(currJCE)
			// take the current stateDB + generate a new proposed Block and a new stateDB
			proposedBlk, newStateDB, err := s.MakeBlock(context.Background(), currJCE)
			if err == nil {
				fmt.Printf("[N%v] Proposed %v (Epoch,Phase) = (%v,%v) Slot=%v Blk=%v\n", s.Id, proposedBlk.Hash(), currEpoch, currPhase, currJCE, proposedBlk)
				return proposedBlk, newStateDB
			}
		} else {
			//waiting for block ... potentially submit ticket here
		}
	}
	return nil, nil
}

func (s *StateDB) SelfTicketCount(epoch uint32) int {
	tickets, ok := s.selfTickets[epoch]
	if !ok {
		return 0
	}
	return len(tickets)
}

func (s *StateDB) AddSelfTicket(epoch uint32, t types.Ticket) bool {
	if s.selfTickets == nil {
		s.selfTickets = make(map[uint32][]common.Hash)
	}
	ticketCnt := s.SelfTicketCount(epoch)
	if ticketCnt < safrole.NumAttempts {
		ticketID := t.TicketID()
		s.selfTickets[epoch] = append(s.selfTickets[epoch], ticketID)
		s.AddTicketToQueue(t)
		return true
	}
	return false
}

// GetSelfTickets returns the self-tickets for a given epoch.
func (s *StateDB) GetSelfTickets(epoch int32) []common.Hash {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()

	ticketIDs, ok := s.selfTickets[uint32(epoch)]
	if !ok {
		return nil
	}
	return ticketIDs
}

func (s *StateDB) GenerateTickets(currJCE uint32) {
	sf := s.GetSafrole()
	currEpoch, currPhase := s.Safrole.EpochAndPhase(currJCE)
	if currPhase >= safrole.EpochTail {
		return
	}
	if s.SelfTicketCount(uint32(currEpoch)) >= safrole.NumAttempts {
		return
	}
	fmt.Printf("[N%v] Generating Tickets for (Epoch,Phase) = (%v,%v) s.Entropy[2]=%v\n", s.Id, currEpoch, currPhase, s.Safrole.Entropy[2])
	tickets := sf.GenerateTickets(s.GetBandersnatchSecret())
	for _, ticket := range tickets {
		if s.AddSelfTicket(uint32(currEpoch), ticket) {
			s.SendOutgoingMessage("Ticket", ticket)
		}
	}
	// send tickets to neighbors
}

func (s *StateDB) SetCredential(credential types.ValidatorSecret) {
	s.credential = credential
}

func (s *StateDB) OpenMsgChannel(messageChan chan Message) {
	s.nodeMessageChan = messageChan
}

func (s *StateDB) SetID(id uint32) {
	s.Id = id
	s.Safrole.Id = id
}

func (s *StateDB) GetEd25519Pub() common.Hash {
	return s.credential.Ed25519Pub
}

func (s *StateDB) GetBandersnatchPub() common.Hash {
	return s.credential.BandersnatchPub
}

func (s *StateDB) GetBandersnatchSecret() []byte {
	return s.credential.BandersnatchSecret
}

func (s *StateDB) IsAuthorizedTicketBuilder() bool {
	sf := s.GetSafrole()
	bandersnatchPub := s.GetBandersnatchPub()
	return sf.IsAuthorizedTicketBuilder(bandersnatchPub)
}

func (s *StateDB) IsAuthorizedBlockBuilder(targetJCE uint32) bool {
	sf := s.GetSafrole()
	bandersnatchPub := s.GetBandersnatchPub()
	targetEpoch, phase := s.Safrole.EpochAndPhase(targetJCE)
	// round robin
	if phase%safrole.NumValidators == s.Id {
		fmt.Printf("IsAuthorized caller: phase(%d) == s.Id(%d)\n", phase, s.Id)
		return true
	} else {
		return false
	}

	ticketIDs := s.GetSelfTickets(targetEpoch)
	return sf.IsAuthorizedBuilder(targetJCE, bandersnatchPub, ticketIDs)
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

func (s *StateDB) ApplyStateTransitionDisputes(disputes []types.Dispute, targetJCE uint32, id uint32) error {
	// TODO
	return nil
}

// given previous safrole, applt state transition using block
// σ'≡Υ(σ,B)
func (s *StateDB) ApplyStateTransitionFromBlock(ctx context.Context, blk *types.Block) (err error) {
	targetJCE := blk.TimeSlot()

	// Preimages
	preimages := blk.PreImages()
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
	err = s.ApplyStateTransitionDisputes(disputes, targetJCE, s.Id)
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
		s.queuedticket = make(map[common.Hash]types.Ticket)
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
func (s *StateDB) MakeBlock(ctx context.Context, targetJCE uint32) (bl *types.Block, newStateDB *StateDB, err error) {
	sf := s.GetSafrole()
	bandersnatchSecret := s.GetBandersnatchSecret()
	isNewEpoch := sf.IsNewEpoch(targetJCE)
	needWinningMarker := sf.IsTicketSubmissionCloses(targetJCE)

	b := types.NewBlock()
	h := types.NewBlockHeader()
	extrinsicData := types.NewExtrinsic()
	h.ParentHash = s.BlockHash
	h.PriorStateRoot = s.StateRoot
	h.TimeSlot = targetJCE

	// Extrinsic Data has 5 different Extrinsics
	// E_P - Preimages
	/*
	   TODO: aggregate queuedPreimageLookups into extrinsicData.Preimages
	*/
	// E_G - Guarantees
	/*
	   TODO: aggregate queuedGuarantees into extrinsicData.Guarantees
	*/
	// E_A - Assurances
	/*
	   TODO: aggregate queuedAssurances into extrinsicData.Assurances
	*/
	// E_D - Disputes
	/*
			   TODO: aggregate queuedVerdicts/Culprits/Faults into extrinsicData.Disputes
		   	d := s.GetDisputes()
			needMarkerVerdicts := d.NeedsVerdictsMarker(targetJCE)
			needMarkerOffenders := d.NeedsOffendersMarker(targetJCE)
	*/

	needEpochMarker := isNewEpoch && false
	if needEpochMarker {
		epochMarker := sf.GenerateEpochMarker()
		//a tuple of the epoch randomness and a sequence of Bandersnatch keys defining the Bandersnatch valida- tor keys (kb) beginning in the next epoch
		fmt.Printf("targetJCE=%v EpochMarker:%v\n", targetJCE, epochMarker)
		h.EpochMark = epochMarker
	}
	if isNewEpoch {
		h.WinningTicketsMark = make([]*types.TicketBody, 0) // clear out the tickets if its a new epoch
		s.queuedticket = make(map[common.Hash]types.Ticket)
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
		for ticketID, ticket := range s.queuedticket {
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
		s.queuedticket = make(map[common.Hash]types.Ticket)
	}

	h.ExtrinsicHash = extrinsicData.Hash()
	author_index, err := sf.GetAuthorIndex(s.GetBandersnatchPub(), "Curr")
	if err != nil {
		return bl, newStateDB, err
	}
	h.BlockAuthorKey = author_index
	b.Extrinsic = extrinsicData

	unsignHeaderHash := h.UnsignedHash()

	//signing
	epochType := sf.CheckEpochType()
	if epochType == "fallback" {
		blockseal, fresh_vrfSig, err := sf.SignFallBack(bandersnatchSecret, unsignHeaderHash)
		if err != nil {
			return bl, newStateDB, err
		}
		h.BlockSeal = blockseal
		h.VRFSignature = fresh_vrfSig
	} else {
		attempt, err := sf.GetBindedAttempt(targetJCE)
		blockseal, fresh_vrfSig, err := sf.SignPrimary(bandersnatchSecret, unsignHeaderHash, attempt)
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
