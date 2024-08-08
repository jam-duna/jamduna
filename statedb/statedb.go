package statedb

import (
	//"github.com/ethereum/go-ethereum/crypto"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/safrole"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"

	//"github.com/colorfulnotion/jam/bandersnatch"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Message struct {
	MsgType string
	Payload interface{}
}

type StateDB struct {
	id              int
	credential      safrole.ValidatorSecret
	parentHash      common.Hash
	blockHash       common.Hash
	priorStateRoot  common.Hash
	stateRoot       common.Hash
	block           *Block
	prevBlock       *Block
	sdb             *storage.StateDBStorage
	selfTickets     map[uint32][]common.Hash //keep track of its own tickets
	knownTickets    map[common.Hash]int      //keep track of known tickets
	queuedticket    map[common.Hash]*safrole.Ticket
	trie            *trie.MerkleTree
	safrole         *safrole.SafroleState
	nodeMessageChan chan Message
	ticketMutex     sync.Mutex
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

func (s *StateDB) AddTicketToQueue(t *safrole.Ticket) {
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

func (s *StateDB) ProcessIncomingBlock(b *Block) {
	currEpoch, currPhase := safrole.EpochAndPhase(b.Header.TimeSlot)
	fmt.Printf("[N%v] Receving Block %v. (Epoch,Phase) = (%v,%v) Slot=%v\n", s.id, b.Hash(), currEpoch, currPhase, b.Header.TimeSlot)
	//TODO validate block
	is_validated := true
	if is_validated {
		tickets := b.Tickets()
		s.RemoveTicketFromQueue(tickets)
		s.ApplyStateTransitionFromBlock(context.Background(), b)
	}
}

func (s *StateDB) ProcessIncomingTicket(t *safrole.Ticket) {
	//s.QueueTicketEnvelope(t)
	//statedb.tickets[common.BytesToHash(ticket_id)] = t
	sf := s.GetSafrole()
	ticketID, err := sf.ValidateProposedTicket(t)
	if err != nil {
		fmt.Printf("Invalid Ticket. Err=%v\n", err)
		return
	}
	if s.CheckTicketExists(ticketID) {
		return
	}
	fmt.Printf("[N%v] Adding ticketID=%v\n", s.id, ticketID)
	s.AddTicketToQueue(t)
	s.knownTickets[ticketID] = t.Attempt
}

// Collect all tickets in the queue
func (s *StateDB) GetTicketQueue() []safrole.Ticket {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()

	ticketQueue := make([]safrole.Ticket, 0, len(s.queuedticket))
	for _, ticket := range s.queuedticket {
		ticketQueue = append(ticketQueue, *ticket)
	}

	// Clear the queue
	s.queuedticket = make(map[common.Hash]*safrole.Ticket)
	return ticketQueue
}

// Remove tickets in the queue by ID
func (s *StateDB) RemoveTicketFromQueue(tickets []safrole.Ticket) {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()

	for _, ticket := range tickets {
		ticketID := ticket.TicketID()
		delete(s.queuedticket, ticketID)
	}
}

func newEmptyStateDB(sdb *storage.StateDBStorage) (statedb *StateDB) {
	statedb = new(StateDB)
	statedb.queuedticket = make(map[common.Hash]*safrole.Ticket)
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
func NewGenesisStateDB(sdb *storage.StateDBStorage, c *safrole.GenesisConfig) (genesisBlk *Block, statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return genesisBlk, statedb, err
	}
	//Genesis block is computed before safrole get initiated
	genesisBlk, err = statedb.MakeGenesisBlock(context.Background())

	/*
		// testing decode and encode..
		genesisBlk2, err := BlockFromBytes(genesisBlk.Bytes())
		if err == nil {
			fmt.Printf("Genesis Byte len=%v. %x\n", len(genesisBlk2.Bytes()), genesisBlk2.Bytes())
			fmt.Printf("Genesis blkHash: %v\n", genesisBlk.Hash())
			fmt.Printf("Genesis unsigned headerHash: %v\n", genesisBlk.Header.UnsignedHash())
			fmt.Printf("Genesis Hash: %v\n", genesisBlk.Header.Hash())
		}
	*/

	statedb.block = genesisBlk
	statedb.safrole = safrole.InitGenesisState(c) // setting the safrole state so that block 1 can be produced
	jsonData, err := json.Marshal(genesisBlk)
	if err != nil {
		fmt.Errorf("Error marshaling block to JSON: %v", err)
	}
	fmt.Printf("GenesisBlock JSON:\n%s\n", jsonData)
	statedb.UpdateTrieState()
	return genesisBlk, statedb, nil
}

func (s *StateDB) GetTrie() *trie.MerkleTree {
	return s.trie
}

func (s *StateDB) GetSafrole() *safrole.SafroleState {
	return s.safrole
}

func (s *StateDB) UpdateTrieState() {
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

	t.SetState(C4, safroleStateEncode)
	t.SetState(C6, entropyEncode)
	t.SetState(C7, nextEpochValidatorsEncode)
	t.SetState(C8, currEpochValidatorsEncode)
	t.SetState(C9, priorEpochValidatorEncode)
	t.SetState(C11, mostRecentBlockTimeSlotEncode)
}

func (s *StateDB) GetSafroleState() *safrole.SafroleState {
	return s.safrole
}

func (s *StateDB) String() string {
	if s != nil {
		//return fmt.Sprintf("[parent %x]", s.parentHash)
	}
	return fmt.Sprint("{}")
}

// newStateDB initiates the StateDB using the blockHash+bn; the bn input must refer to the epoch for which the blockHash belongs to
func newStateDB(sdb *storage.StateDBStorage, blockHash common.Hash) (statedb *StateDB, err error) {
	statedb = newEmptyStateDB(sdb)
	statedb.queuedticket = make(map[common.Hash]*safrole.Ticket)
	statedb.knownTickets = make(map[common.Hash]int)
	statedb.selfTickets = make(map[uint32][]common.Hash)
	statedb.trie = trie.NewMerkleTree(nil, sdb)

	block := Block{}
	b := make([]byte, 32)
	zeroHash := common.BytesToHash(b)
	if bytes.Compare(blockHash.Bytes(), zeroHash.Bytes()) == 0 {
		// genesis block situation
		statedb.safrole = safrole.NewSafroleState()
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
		statedb.block = &block
		statedb.parentHash = block.Header.ParentHash
		statedb.safrole = safrole.NewSafroleState() //TODO: DO entropy initiation
		err = statedb.SetRootFromBlock(&block)
		if err != nil {
			return statedb, err
		}
	}
	return statedb, nil
}

func (s *StateDB) SetRootFromBlock(block *Block) error {
	s.priorStateRoot = block.Header.PriorStateRoot
	return nil
}

// Copy generates a copy of the StateDB
func (s *StateDB) Copy() (n *StateDB, err error) {
	n = newEmptyStateDB(s.sdb)
	n.block = s.block.StateCopy()
	n.parentHash = s.parentHash
	return n, nil
}

func NewStateDBFromBlock(ctx context.Context, sdb *storage.StateDBStorage, block *Block) (statedb *StateDB, err error) {
	statedb = newEmptyStateDB(sdb)
	statedb.block = block
	statedb.parentHash = block.Header.ParentHash
	err = statedb.SetRootFromBlock(block)
	if err != nil {
		return statedb, err
	}
	return statedb, err
}

func (s *StateDB) ProcessState() {
	genesisReady := s.safrole.CheckGenesisReady()
	if !genesisReady {
		return
	}
	currJCE, timeSlotReady := s.safrole.CheckTimeSlotReady()
	if timeSlotReady {
		//fmt.Printf("[N%v] Ready to start!\n", s.id)
		//Time to propose block if selected
		isAuthorizedBlockBuilder := s.IsAuthorizedBlockBuilder(currJCE)
		if isAuthorizedBlockBuilder {
			fmt.Printf("[N%v] MakeBlock %v from %v\n", s.id, currJCE, s.GetBandersnatchPub())
			//TODO: advance safrole here
			s.ProcessSafroleAsAuthor(currJCE)
		} else {
			//waiting for block ... potentially submit ticket here
		}
		isAuthorizedTicketBuilder := s.IsAuthorizedTicketBuilder()
		if isAuthorizedTicketBuilder {
			s.GenerateTickets(currJCE)
		}
	}
}

func (s *StateDB) SelfTicketCount(epoch uint32) int {
	tickets, ok := s.selfTickets[epoch]
	if !ok {
		return 0
	}
	return len(tickets)
}

func (s *StateDB) AddSelfTicket(epoch uint32, t *safrole.Ticket) bool {
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
func (s *StateDB) GetSelfTickets(epoch uint32) []common.Hash {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()

	ticketIDs, ok := s.selfTickets[epoch]
	if !ok {
		return nil
	}
	return ticketIDs
}

func (s *StateDB) GenerateTickets(currJCE uint32) {
	sf := s.GetSafrole()
	currEpoch, currPhase := safrole.EpochAndPhase(currJCE)
	if currPhase >= safrole.EpochTail {
		return
	}
	if s.SelfTicketCount(currEpoch) >= safrole.NumAttempts {
		return
	}
	fmt.Printf("[N%v] (Epoch,Phase) = (%v,%v) Generating Tickets!\n", s.id, currEpoch, currPhase)
	tickets := sf.GenerateTickets(s.GetBandersnatchSecret())
	for _, ticket := range tickets {
		if s.AddSelfTicket(currEpoch, &ticket) {
			s.SendOutgoingMessage("Ticket", ticket)

		}
	}
	// send tickets to neighbors
}

func (s *StateDB) ProcessSafroleAsAuthor(targetJCE uint32) {
	//sf := s.GetSafrole()
	//s.ApplyExtrinsic(context.Background()) // what shoulApplyExtrinsicd this do??
	//sf.AdvanceSafrole(targetJCE)
	currEpoch, currPhase := safrole.EpochAndPhase(targetJCE)
	proposedBlk, err := s.MakeBlock(context.Background(), targetJCE)
	if err == nil {
		fmt.Printf("[N%v] Proposed %v (Epoch,Phase) = (%v,%v) Slot=%v Blk=%v\n", s.id, proposedBlk.Hash(), currEpoch, currPhase, targetJCE, proposedBlk)
		s.SendOutgoingMessage("Block", proposedBlk)
		s.ApplyStateTransitionFromBlock(context.Background(), proposedBlk)
	}
}

func (s *StateDB) SetCredential(credential safrole.ValidatorSecret) {
	s.credential = credential
}

func (s *StateDB) OpenMsgChannel(messageChan chan Message) {
	s.nodeMessageChan = messageChan
}

func (s *StateDB) SetID(id int) {
	s.id = id
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
	//fmt.Printf("IsAuthorized caller: %v\n", bandersnatchPub)
	targetEpoch, _ := safrole.EpochAndPhase(targetJCE)
	ticketIDs := s.GetSelfTickets(targetEpoch)
	return sf.IsAuthorizedBuilder(targetJCE, bandersnatchPub, ticketIDs)
}

// given previous safrole, applt state transition using block
// σ'≡Υ(σ,B)
func (s *StateDB) ApplyStateTransitionFromBlock(ctx context.Context, blk *Block) (err error) {
	// TODO: implement the transition code here
	ticketExts := blk.Tickets()
	sf_header := blk.ConvertToSafroleHeader()
	targetJCE := blk.TimeSlot()
	sf := s.GetSafrole()
	s2, err := sf.ApplyStateTransitionFromBlock(ticketExts, targetJCE, sf_header)
	if err != nil {
		return err
	}
	s.safrole = &s2
	//s.prevblock = s.block
	s.block = blk
	t := s.GetTrie()
	priorStateRoot := t.GetRoot()
	s.priorStateRoot = priorStateRoot

	s.UpdateTrieState()
	return nil
}

func (s *StateDB) MakeGenesisBlock(ctx context.Context) (*Block, error) {
	b := NewBlock()
	h := NewBlockHeader()

	mostRecentBlockTimeSlotEncode := common.EncodeUint64(0)

	t := s.GetTrie()

	t.SetState(C11, mostRecentBlockTimeSlotEncode)

	// these flush operations compute new root chunkhashs / merkleroots
	s.Flush(ctx, b.Header.TimeSlot)

	priorStateRoot := t.GetRoot()

	h.ParentHash = common.Hash{}
	h.PriorStateRoot = priorStateRoot
	b.Header = *h

	return b, nil
}

// make block generate block prior to state execution
func (s *StateDB) MakeBlock(ctx context.Context, targetJCE uint32) (bl *Block, err error) {
	t := s.GetTrie()
	sf := s.GetSafrole()
	bandersnatchSecret := s.GetBandersnatchSecret()
	needEpochMarker := sf.IsFirstEpochPhase(targetJCE)
	needWinningMarker := sf.IsTicketSubmissionCloses(targetJCE)
	//prevBlk := s.prevBlock.StateCopy() //TODO fix the copy func

	//_, currPhase := EpochAndPhase(targetJCE)

	prevBlk, _ := BlockFromBytes(s.prevBlock.Bytes())
	b := NewBlock()
	h := NewBlockHeader()
	extrinsicData := NewExtrinsic()
	h.ParentHash = prevBlk.Hash()
	h.PriorStateRoot = t.GetRoot()
	h.TimeSlot = targetJCE
	if needEpochMarker {
		epochMarker := sf.GenerateEpochMarker()
		//a tuple of the epoch randomness and a sequence of Bandersnatch keys defining the Bandersnatch valida- tor keys (kb) beginning in the next epoch
		fmt.Printf("targetJCE=%v EpochMarker:%v\n", targetJCE, epochMarker)
		h.EpochMark = epochMarker
	}
	if needWinningMarker {
		fmt.Printf("targetJCE=%v check WinningMarker\n", targetJCE)
		winningMarker, err := sf.GenerateWinningMarker()
		//block is the first after the end of the submission period for tickets and if the ticket accumulator is saturated
		if err == nil {
			h.WinningTicketsMark = winningMarker
		}
	}

	// If there's new ticketID, add them into extrinsic
	// Question: can we submit tickets at the exact tail end block?
	if !needWinningMarker {
		ext_t := s.GetTicketQueue()
		if len(ext_t) > 0 {
			fmt.Printf("targetJCE=%v Including tickets Len=%v\n", targetJCE, len(ext_t))
			extrinsicData.Tickets = ext_t
		}
	}

	h.ExtrinsicHash = extrinsicData.Hash()
	author_index, err := sf.GetAuthorIndex(s.GetBandersnatchPub(), "Curr")
	if err != nil {
		return bl, err
	}
	h.BlockAuthorKey = author_index
	b.Header = *h
	b.Extrinsic = extrinsicData

	unsignHeaderHash := h.UnsignedHash()

	//signing
	epochType := sf.CheckEpochType()
	if epochType == "fallback" {
		blockseal, fresh_vrfSig, err := sf.SignFallBack(bandersnatchSecret, unsignHeaderHash)
		if err != nil {
			return bl, err
		}
		h.BlockSeal = blockseal
		h.VRFSignature = fresh_vrfSig
	} else {
		attempt, err := sf.GetBindedAttempt(targetJCE)
		blockseal, fresh_vrfSig, err := sf.SignPrimary(bandersnatchSecret, unsignHeaderHash, attempt)
		if err != nil {
			return bl, err
		}
		h.BlockSeal = blockseal
		h.VRFSignature = fresh_vrfSig
	}
	return b, nil
}

// Flush calls each SMTs flush operation
func (s *StateDB) Flush(ctx context.Context, timeSlotIndex uint32) error {
	//log.Info(fmt.Sprintf("[statedb:Flush] Round %d updated epochRoot %x", statedb.proposedRound, statedb.epochStorage.GetRootChunkHash()))
	return nil
}

func ComputeCurrentJCETime(unixTimestamp int64) int64 {
	currentTime := time.Now()
	return ComputeJCETime(currentTime.Unix())
}

// The current time expressed in seconds after the start of the Jam Common Era. See section 4.4
func ComputeJCETime(unixTimestamp int64) int64 {
	// Define the start of the Jam Common Era
	jceStart := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)

	// Convert the Unix timestamp to a Time object
	currentTime := time.Unix(unixTimestamp, 0).UTC()

	// Calculate the difference in seconds
	diff := currentTime.Sub(jceStart)
	return int64(diff.Seconds())
}
