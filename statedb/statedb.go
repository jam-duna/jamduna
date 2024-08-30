package statedb

import (
	//"github.com/ethereum/go-ethereum/crypto"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"sort"
	"sync"
	"time"
)

type Message struct {
	MsgType string
	Payload interface{}
}

type StateDB struct {
	Id         uint32       `json:"id"`
	Block      *types.Block `json:"block"`
	ParentHash common.Hash  `json:"parentHash"`
	BlockHash  common.Hash  `json:"blockHash"`
	StateRoot  common.Hash  `json:"stateRoot"`
	JamState   *JamState    `json:"Jamstate"`
	sdb        *storage.StateDBStorage
	trie       *trie.MerkleTree

	VMs     map[uint32]*pvm.VM
	vmMutex sync.Mutex

	knownPreimageLookups  map[common.Hash]uint32
	queuedPreimageLookups map[common.Hash]types.PreimageLookup
	preimageLookupsMutex  sync.Mutex

	knownGuarantees  map[common.Hash]int
	queuedGuarantees map[common.Hash]types.Guarantee
	guaranteeMutex   sync.Mutex

	knownAssurances  map[common.Hash]int
	queuedAssurances map[common.Hash]types.Assurance
	assuranceMutex   sync.Mutex

	knownDisputes  map[common.Hash]int
	queuedDisputes map[common.Hash]types.Dispute
	disputeMutex   sync.Mutex

	knownTickets  map[common.Hash]int
	queuedTickets map[common.Hash]types.Ticket
	ticketMutex   sync.Mutex

	X XContext
	S uint32
}

func (s *StateDB) AddTicketToQueue(t types.Ticket) {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	s.queuedTickets[t.TicketID()] = t
}

func (s *StateDB) AddLookupToQueue(l types.PreimageLookup) {
	s.preimageLookupsMutex.Lock()
	defer s.preimageLookupsMutex.Unlock()
	s.queuedPreimageLookups[l.AccountPreimageHash()] = l
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
	s.preimageLookupsMutex.Lock()
	defer s.preimageLookupsMutex.Unlock()
	_, exists := s.knownTickets[ticketID]
	return exists
}

func (s *StateDB) CheckLookupExists(a_p common.Hash) bool {
	if (a_p == common.Hash{}) {
		return true
	}
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	_, exists := s.knownPreimageLookups[a_p]
	return exists
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

func (s *StateDB) ProcessIncomingLookup(l types.PreimageLookup) {
	//TODO: check existence of lookup and stick into map
	//cj := s.GetPvmState()
	fmt.Printf("[N%v] ProcessIncomingLookup -- Adding lookup: %v\n", s.Id, l.String())
	account_preimage_hash, err := s.ValidateLookup(&l)
	if err != nil {
		fmt.Printf("Invalid lookup. Err=%v\n", err)
		return
	}
	if s.CheckLookupExists(account_preimage_hash) {
		return
	}
	s.AddLookupToQueue(l)
	sf := s.GetSafrole()
	s.knownPreimageLookups[account_preimage_hash] = sf.GetTimeSlot() // hostForget has certain logic that will probably reference this field???
}

func (s *StateDB) ProcessIncomingDispute(d types.Dispute) {
	// get the disputes state
	disp := s.GetJamState()
	err := disp.ValidateProposedDispute(&d)
	if err != nil {
		fmt.Printf("Invalid dispute. Err=%v\n", err)
		return
	}
	s.AddDisputeToQueue(d)
}

func (s *StateDB) ProcessIncomingGuarantee(g types.Guarantee) {
	// get the guarantee state
	err := g.ValidateSignatures()
	if err != nil {
		fmt.Printf("Invalid guarantee. Err=%v\n", err)
		return
	}
	s.AddGuaranteeToQueue(g)
}

func (s *StateDB) getValidatorCredential() []byte {
	// TODO
	return nil
}

func (s *StateDB) ProcessIncomingAssurance(a types.Assurance) {
	cred := s.getValidatorCredential()
	err := a.ValidateSignature(cred)
	if err != nil {
		fmt.Printf("Invalid guarantee. Err=%v\n", err)
		return
	}
	s.AddAssuranceToQueue(a)
}

func (s *StateDB) GetVMMutex() sync.Mutex {
	return s.vmMutex
}

func (s *StateDB) RemoveTicket(t *types.Ticket) {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	delete(s.queuedTickets, t.TicketID())
}

func (s *StateDB) RemoveLookup(p *types.PreimageLookup) {
	s.preimageLookupsMutex.Lock()
	defer s.preimageLookupsMutex.Unlock()
	delete(s.queuedPreimageLookups, p.AccountPreimageHash())
}

func (s *StateDB) RemoveGuarantee(g *types.Guarantee) {
	s.guaranteeMutex.Lock()
	defer s.guaranteeMutex.Unlock()
	delete(s.queuedGuarantees, g.Hash())
}

func (s *StateDB) RemoveAssurance(a *types.Assurance) {
	s.assuranceMutex.Lock()
	defer s.assuranceMutex.Unlock()
	delete(s.queuedAssurances, a.Hash())
}

func (s *StateDB) RemoveDispute(d *types.Dispute) {
	s.disputeMutex.Lock()
	defer s.disputeMutex.Unlock()
	delete(s.queuedDisputes, d.Hash())
}

// IsAuthorizedPVM performs the is-authorized PVM function.
func IsAuthorizedPVM(workPackage types.WorkPackage) (bool, error) {
	// Ensure the work-package warrants the needed core-time
	// Ensure all segment-tree roots which form imported segment commitments are known and valid
	// Ensure that all preimage data referenced as commitments of extrinsic segments can be fetched

	// For demonstration, let's assume these checks are passed
	//for _, workItem := range workPackage.WorkItems {

	//}

	return true, nil
}

// EP Errors
const (
	errServiceIndices         = "ServiceIndices duplicated or not ordered"
	errPreimageLookupNotSet   = "Preimagelookup (h,l) not set"
	errPreimageLookupNotEmpty = "Preimagelookup not empty"
	errPreimageBlobSet        = "PreimageBlob already set"
)

func (s *StateDB) ValidateLookup(l *types.PreimageLookup) (common.Hash, error) {
	// check 157 - (1) a_p not equal to P (2) a_l is empty
	t := s.GetTrie()
	a_p := l.AccountPreimageHash()
	//a_l := l.AccountLookupHash()

	preimage_blob, err := t.GetPreImageBlob(l.Service_Index(), l.BlobHash().Bytes())
	//TODO: stanley to make sure we can check whether a key exist or not. err here is ambiguous here
	if err == nil { // key found
		if l.BlobHash() == common.Blake2Hash(preimage_blob) {
			//H(p) = p
			return common.Hash{}, fmt.Errorf(errPreimageBlobSet)
		}
	}

	//fmt.Printf("Validating E_p %v\n",l.String())
	anchors, err := t.GetPreImageLookup(l.Service_Index(), l.BlobHash(), l.BlobLength())
	if err != nil {
		return common.Hash{}, fmt.Errorf(errPreimageLookupNotSet) //TODO: differentiate key not found vs leveldb error
	}
	if len(anchors) != 0 {
		// non-empty anchor
		return common.Hash{}, fmt.Errorf(errPreimageLookupNotEmpty)
	}
	return a_p, nil
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
	statedb.knownPreimageLookups = make(map[common.Hash]uint32)
	statedb.SetStorage(sdb)
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
func NewGenesisStateDB(sdb *storage.StateDBStorage, c *GenesisConfig) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}

	statedb.Block = nil
	j := InitGenesisState(c)
	statedb.JamState = j // setting the dispute state so that block 1 can be produced
	// setting the safrole state so that block 1 can be produced

	statedb.StateRoot = statedb.UpdateTrieState()
	return statedb, nil
}

func (s *StateDB) GetStateRoot() common.Hash {
	return s.StateRoot
}

func (s *StateDB) GetTentativeStateRoot() common.Hash {
	// return the trie root at the moment
	t := s.GetTrie()
	return t.GetRoot()
}

func (s *StateDB) GetTrie() *trie.MerkleTree {
	return s.trie
}

func (s *StateDB) GetStorage() *storage.StateDBStorage {
	return s.sdb
}

func (s *StateDB) SetStorage(sdb *storage.StateDBStorage) {
	s.sdb = sdb
}

func (s *StateDB) GetSafrole() *SafroleState {
	return s.JamState.SafroleState
}

func (s *StateDB) GetJamState() *JamState {
	return s.JamState
}

func (s *StateDB) UpdateTrieState() common.Hash {
	//γ ≡⎩γk, γz, γs, γa⎭
	//γk :one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	//γz :epoch’s root, a Bandersnatch ring root composed with the one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	//γa :the ticket accumulator, a series of highest-scoring ticket identifiers to be used for the next epoch (epoch N+1)
	//γs :current epoch’s slot-sealer series, which is either a full complement of E tickets or, in the case of a fallback mode, a series of E Bandersnatch keys (epoch N)
	sf := s.GetSafrole()
	if sf == nil {
		fmt.Printf("NO SAFROLE %v", s)
		panic(222)
	}
	sb := sf.GetSafroleBasicState()
	safroleStateEncode, _ := sb.Bytes()
	entropyEncode := sf.EntropyToBytes()
	nextEpochValidatorsEncode := sf.GetValidatorData("Next")
	currEpochValidatorsEncode := sf.GetValidatorData("Curr")
	priorEpochValidatorEncode := sf.GetValidatorData("Pre")
	mostRecentBlockTimeSlotEncode := common.EncodeUint64(uint64(sf.GetTimeSlot()))

	d := s.GetJamState()
	disputeState, err := d.GetPsiBytes()
	if err != nil {
		fmt.Println("Error serializing psi", err)
	}
	rhoEncode, err := d.GetRhoBytes()
	if err != nil {
		fmt.Println("Error serializing rho", err)
	}
	piEncode, err := d.GetPiBytes()
	if err != nil {
		fmt.Println("Error serializing pi", err)
	}

	coreAuthPoolEncode, err := d.GetAuthQueueBytes()
	if err != nil {
		fmt.Println("Error serializing CoreAuthPool", err)
	}

	authQueueEncode, err := d.GetAuthQueueBytes()
	if err != nil {
		fmt.Println("Error serializing AuthQueue", err)
	}

	privilegedServiceIndicesEncode, err := d.GetPrivilegedServicesIndicesBytes()
	if err != nil {
		fmt.Println("Error serializing PrivilegedServicesIndices", err)
	}

	recentBlocksEncode, err := d.GetRecentBlocksBytes()
	if err != nil {
		fmt.Println("Error serializing Recent Blocks", err)
	}

	t := s.GetTrie()
	fmt.Printf("UpdateTrieState - before root:%v\n", t.GetRoot())
	t.SetState(C1, coreAuthPoolEncode)
	t.SetState(C2, authQueueEncode)
	t.SetState(C3, recentBlocksEncode)
	t.SetState(C4, safroleStateEncode)
	t.SetState(C5, disputeState)
	t.SetState(C6, entropyEncode)
	t.SetState(C7, nextEpochValidatorsEncode)
	t.SetState(C8, currEpochValidatorsEncode)
	t.SetState(C9, priorEpochValidatorEncode)
	t.SetState(C10, rhoEncode)
	t.SetState(C11, mostRecentBlockTimeSlotEncode)
	t.SetState(C12, privilegedServiceIndicesEncode)
	t.SetState(C13, piEncode)
	updated_root := t.GetRoot()
	fmt.Printf("UpdateTrieState - after root:%v\n", updated_root)
	return updated_root
}

func (s *StateDB) GetSafroleState() *SafroleState {
	return s.JamState.SafroleState
}

func (s *StateDB) String() string {
	enc, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}

func NewStateDB(sdb *storage.StateDBStorage, blockHash common.Hash) (statedb *StateDB, err error) {
	return newStateDB(sdb, blockHash)
}

// newStateDB initiates the StateDB using the blockHash+bn; the bn input must refer to the epoch for which the blockHash belongs to
func newStateDB(sdb *storage.StateDBStorage, blockHash common.Hash) (statedb *StateDB, err error) {
	statedb = newEmptyStateDB(sdb)
	statedb.queuedTickets = make(map[common.Hash]types.Ticket)
	statedb.knownTickets = make(map[common.Hash]int)
	statedb.queuedDisputes = make(map[common.Hash]types.Dispute)
	statedb.knownDisputes = make(map[common.Hash]int)

	statedb.trie = trie.NewMerkleTree(nil, sdb)
	statedb.JamState = NewJamState()

	block := types.Block{}
	b := make([]byte, 32)
	zeroHash := common.BytesToHash(b)
	if bytes.Compare(blockHash.Bytes(), zeroHash.Bytes()) == 0 {
		// genesis block situation

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
	}

	return statedb, nil
}

func (s *StateDB) CopyTrieState(stateRoot common.Hash) *trie.MerkleTree {
	t, _ := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), s.sdb)
	return t
}

// Copy generates a copy of the StateDB
func (s *StateDB) Copy() (newStateDB *StateDB) {
	// Create a new instance of StateDB
	// T.P.G.A.
	newStateDB = &StateDB{
		Id:                    s.Id,
		Block:                 s.Block.Copy(), // You might need to deep copy the Block if it's mutable
		ParentHash:            s.ParentHash,
		BlockHash:             s.BlockHash,
		StateRoot:             s.StateRoot,
		JamState:              s.JamState.Copy(), // DisputesState has a Copy method
		sdb:                   s.sdb,
		trie:                  s.CopyTrieState(s.StateRoot),
		knownTickets:          make(map[common.Hash]int),
		knownPreimageLookups:  make(map[common.Hash]uint32),
		knownGuarantees:       make(map[common.Hash]int),
		knownAssurances:       make(map[common.Hash]int),
		knownDisputes:         make(map[common.Hash]int),
		queuedTickets:         make(map[common.Hash]types.Ticket),
		queuedPreimageLookups: make(map[common.Hash]types.PreimageLookup),
		queuedGuarantees:      make(map[common.Hash]types.Guarantee),
		queuedAssurances:      make(map[common.Hash]types.Assurance),
		queuedDisputes:        make(map[common.Hash]types.Dispute),

		/*
			Following flds are not copied over..?

			VMs       map[uint32]*pvm.VM
			vmMutex   sync.Mutex
			X 		  XContext
			S 		  uint32

		*/
	}
	s.CloneExtrinsicMap(newStateDB)
	return newStateDB
}

func (s *StateDB) CloneExtrinsicMap(n *StateDB) {
	// Tickets
	for k, v := range s.knownTickets {
		n.knownTickets[k] = v
	}
	for k, v := range s.queuedTickets {
		t, _ := v.DeepCopy()
		n.queuedTickets[k] = t
	}

	// Preimages
	for k, v := range s.knownPreimageLookups {
		n.knownPreimageLookups[k] = v
	}
	for k, v := range s.queuedPreimageLookups {
		p, _ := v.DeepCopy()
		n.queuedPreimageLookups[k] = p
	}

	// Guarantees
	for k, v := range s.knownGuarantees {
		n.knownGuarantees[k] = v
	}

	for k, v := range s.queuedGuarantees {
		t, _ := v.DeepCopy()
		n.queuedGuarantees[k] = t
	}

	// Assurance
	for k, v := range s.knownAssurances {
		n.knownAssurances[k] = v
	}
	for k, v := range s.queuedAssurances {
		t, _ := v.DeepCopy()
		n.queuedAssurances[k] = t
	}

	// Dispute
	for k, v := range s.knownDisputes {
		n.knownDisputes[k] = v
	}
	for k, v := range s.queuedDisputes {
		d, _ := v.DeepCopy()
		n.queuedDisputes[k] = d
	}
}

func (s *StateDB) RemoveExtrinsics(tickets []types.Ticket, lookups []types.PreimageLookup, guarantees []types.Guarantee, assurances []types.Assurance, disputes []types.Dispute) {
	// T.P.G.A.D
	for _, t := range tickets {
		s.RemoveTicket(&t)
	}
	for _, p := range lookups {
		s.RemoveLookup(&p)
	}
	for _, g := range guarantees {
		s.RemoveGuarantee(&g)
	}
	for _, a := range assurances {
		s.RemoveAssurance(&a)
	}
	for _, d := range disputes {
		s.RemoveDispute(&d)
	}
}

func (s *StateDB) ProcessState(credential types.ValidatorSecret) (*types.Block, *StateDB, error) {
	genesisReady := s.JamState.SafroleState.CheckGenesisReady()
	if !genesisReady {
		return nil, nil, nil
	}
	currJCE, timeSlotReady := s.JamState.SafroleState.CheckTimeSlotReady()
	if timeSlotReady {
		// Time to propose block if authorized
		isAuthorizedBlockBuilder := false
		//bandersnatchPub := credential.BandersnatchPub
		_, phase := s.JamState.SafroleState.EpochAndPhase(currJCE)
		// round robin TEMPORARY
		if phase%types.TotalValidators == s.Id {
			//fmt.Printf("IsAuthorized caller: phase(%d) == s.Id(%d)\n", phase, s.Id)
			isAuthorizedBlockBuilder = true
		}

		//sf := s.GetSafrole()
		//ticketIDs := s.GetSelfTickets(targetEpoch)
		//isAuthorizedBlockBuilder = sf.IsAuthorizedBuilder(currJCE, credential, ticketIDs)
		if isAuthorizedBlockBuilder {
			currEpoch, currPhase := s.JamState.SafroleState.EpochAndPhase(currJCE)
			// propose block without state transition
			proposedBlk, err := s.MakeBlock(credential, currJCE)
			if err != nil {
				return nil, nil, err
			}
			fmt.Printf("[N%v] Proposed %v<-%v (Epoch,Phase) = (%v,%v) Slot=%v\n", s.Id, proposedBlk.ParentHash(), proposedBlk.Hash(), currEpoch, currPhase, currJCE)
			newStateDB, err := ApplyStateTransitionFromBlock(s, context.Background(), proposedBlk)
			if err != nil {
				// HOW could this happen, we made the block ourselves!
				return nil, nil, err
			}
			fmt.Printf("[N%d] MakeBlock StateDB (after application of Block %v) %v\n", s.Id, proposedBlk.Hash(), newStateDB.String())
			return proposedBlk, newStateDB, nil
		} else {
			//waiting for block ... potentially submit ticket here
		}
	}
	return nil, nil, nil
}

func (s *StateDB) SetID(id uint32) {
	s.Id = id
	s.JamState.SafroleState.Id = id
}

func (s *StateDB) ApplyStateTransitionPreimages(preimages []types.PreimageLookup, targetJCE uint32, id uint32) error {
	num_preimages := uint32(0)
	num_octets := uint32(0)

	//TODO: (eq 156) need to make sure E_P is sorted. by what??
	// validate (eq 156)
	for i := 1; i < len(preimages); i++ {
		if preimages[i].ServiceIndex <= preimages[i-1].ServiceIndex {
			return fmt.Errorf(errServiceIndices)
		}
	}

	for _, l := range preimages {
		// validate eq 157
		_, err := s.ValidateLookup(&l)
		if err != nil {
			return err
		}
	}

	// ready for state transisiton
	t := s.GetTrie()
	for _, l := range preimages {
		// (eq 158)
		// δ†[s]p[H(p)] = p
		// δ†[s]l[H(p),∣p∣] = [τ′]
		t.SetPreImageBlob(l.Service_Index(), l.Blob())
		t.SetPreImageLookup(l.Service_Index(), l.BlobHash(), l.BlobLength(), []uint32{targetJCE})
		num_preimages++
		num_octets += l.BlobLength()
	}

	s.JamState.tallyStatistics(s.Id, "preimages", num_preimages)
	s.JamState.tallyStatistics(s.Id, "octets", num_octets)

	return nil
}

func (s *StateDB) getRhoWorkReportByWorkPackage(workPackageHash common.Hash) (types.WorkReport, uint32, bool) {
	// TODO
	return types.WorkReport{}, 0, false
}

// for any hits in m, remove them from pool
func (s *StateDB) remove_guarantees_authhash(pool []common.Hash, m map[common.Hash]bool) []common.Hash {
	p := make([]common.Hash, 0)
	for _, h := range p {
		_, ok := m[h]
		if ok {

		} else {
			p = append(p, h)
		}
	}
	return p
}

func (s *StateDB) getServiceAccount(c uint32) (*types.ServiceAccount, bool, error) {
	t := s.GetTrie()
	v, err := t.GetService(types.ServiceAccountPrefix, c)
	if err != nil {
		return &types.ServiceAccount{}, false, nil
	}
	// v looks like: ac ⌢ E8(ab,ag,am,al) ⌢ E4(ai)
	// TODO: William to figure out the transformation
	a, err := types.ServiceAccountFromBytes(v)
	if err != nil {
		return &types.ServiceAccount{}, false, nil
	}
	fmt.Printf("getServiceAccount s=%v, v=%v\n", c, a.String())
	return &types.ServiceAccount{}, false, nil
}

func (s *StateDB) getPreimageBlob(c uint32, codeHash common.Hash) ([]byte, error) {
	t := s.GetTrie()
	preimage_blob, err := t.GetPreImageBlob(c, codeHash.Bytes())
	if err != nil {
		return []byte{}, err
	}
	return preimage_blob, nil
}

func (s *StateDB) getServiceCoreCode(c uint32) (code []byte, err error) {
	serviceAccount, ok, err := s.getServiceAccount(c)
	if err != nil {
		return []byte{}, err
	}
	if !ok {
		return []byte{}, errors.New("Service Account/Core not found")
	}
	codeHash := serviceAccount.CodeHash
	code, err = s.getPreimageBlob(c, codeHash)
	if err != nil {
		return []byte{}, errors.New("Code not found")
	}
	return code, nil
}

func (s *StateDB) getWrangledWorkResultsBytes(results []types.WrangledWorkResult) []byte {
	output := make([]byte, 0)
	for _, r := range results {
		output = append(output, r.Output...) // TODO: r.Error
		output = append(output, r.PayloadHash.Bytes()...)
		output = append(output, r.AuthorizationOutput...)
		output = append(output, r.WorkPackageHash.Bytes()...)
	}
	return output
}

func (s *StateDB) Accumulate(cores map[uint32]*Rho_state) error {
	for c, rho_state := range cores {
		if rho_state != nil {
			wrangledWorkResults := make([]types.WrangledWorkResult, 0)
			code, err := s.getServiceCoreCode(c)
			if err == nil {
				// Wrangle results from work report
				workReport := rho_state.WorkReport
				for _, workResult := range workReport.Results {
					wrangledWorkResult := workResult.Wrangle(workReport.AuthorizationOutput, workReport.AvailabilitySpec.WorkPackageHash)
					wrangledWorkResults = append(wrangledWorkResults, wrangledWorkResult)
				}
			}
			wrangledWorkResultsBytes := s.getWrangledWorkResultsBytes(wrangledWorkResults)
			vm := pvm.NewVMFromCode(c, code, 0, s)
			vm.SetArgumentInputs(wrangledWorkResultsBytes)
			vm.Execute(types.EntryPointAccumulate)
		}
	}
	return nil
}

func (s *StateDB) OnTransfer(cores map[uint32]*Rho_state) error {
	for c, _ := range cores {
		code, err := s.getServiceCoreCode(c)
		if err == nil {
			fmt.Printf("OnTransfers %d\n", c)
			vm := pvm.NewVMFromCode(c, code, 0, s)
			vm.Execute(types.EntryPointOnTransfer)
		}
	}
	return nil
}

// Section 8.2 - Eq 85+86
func (s *StateDB) ApplyStateTransitionAuthorizations(guarantees []types.Guarantee) error {
	for c := uint32(0); c < types.TotalCores; c++ {
		if len(guarantees) > 0 {
			// build m, holding the set of authorization hashes matching the core c in guarantees
			m := make(map[common.Hash]bool)
			hits := 0
			for _, g := range guarantees {
				if g.WorkReport.Core == c {
					m[g.WorkReport.AuthorizerHash] = true
					hits++
				}
			}
			// only use m to update the pool with remove_guarantees_authhash if we have hits > 0
			if hits > 0 {
				s.JamState.AuthorizationsPool[c] = s.remove_guarantees_authhash(s.JamState.AuthorizationsPool[c], m)
			}
		}
		// Eq 85 -- add
		for _, q := range s.JamState.AuthorizationQueue[c] {
			s.JamState.AuthorizationsPool[c] = append(s.JamState.AuthorizationsPool[c], q)
		}
		// cap AuthorizationsPool length at O=types.MaxAuthorizationPoolItems
		if len(s.JamState.AuthorizationsPool[c]) > types.MaxAuthorizationPoolItems {
			s.JamState.AuthorizationsPool[c] = s.JamState.AuthorizationsPool[c][:types.MaxAuthorizationPoolItems]
		}
	}
	return nil
}

// Process Rho - Eq 25/26/27 using disputes, assurances, guarantees in that order
func (s *StateDB) ApplyStateTransitionRho(Disputes []types.Dispute, assurances []types.Assurance, guarantees []types.Guarantee, targetJCE uint32, id uint32) (types.VerdictMarker, types.OffenderMarker, map[uint32]*Rho_state, error) {
	var VMark types.VerdictMarker
	var OMark types.OffenderMarker
	cores := make(map[uint32]*Rho_state)

	// (25) / (111) We clear any work-reports which we judged as uncertain or invalid from their core
	d := s.GetJamState()
	for _, dispute := range Disputes {
		for _, v := range dispute.Verdict {
			_, core, ok := s.getRhoWorkReportByWorkPackage(v.WorkReportHash)
			if ok {
				if len(v.Votes) > 2*types.TotalValidators/3 {
					d.clearRhoByCore(core)
				}
			}
		}
		//apply the dispute
		var err error
		VMark, OMark, err = s.JamState.Disputes(dispute)
		if err != nil {
			return types.VerdictMarker{}, types.OffenderMarker{}, cores, err
		}
	}

	// Assurances: get the bitstring from the availability
	tally := make([]uint32, types.TotalCores)
	for _, a := range assurances {
		for c, bs := range a.Bitstring {
			tally[c] += uint32(bs)
		}
	}
	// core's data is now available
	num_assurances := uint32(0)
	for c, available := range tally {
		if available > 2*types.TotalValidators/3 {
			cores[uint32(c)] = d.clearRhoByCore(uint32(c))
		}
		num_assurances++
	}
	s.JamState.tallyStatistics(s.Id, "assurances", num_assurances)

	// Guarantees
	num_reports := uint32(0)
	for _, g := range guarantees {
		d.setRhoByWorkReport(g.WorkReport.Core, g.WorkReport, targetJCE)
		num_reports++
	}
	s.JamState.tallyStatistics(s.Id, "reports", num_reports)

	return VMark, OMark, cores, nil
}

// given previous safrole, applt state transition using block
// σ'≡Υ(σ,B)
func ApplyStateTransitionFromBlock(oldState *StateDB, ctx context.Context, blk *types.Block) (s *StateDB, err error) {
	s = oldState.Copy()
	s.JamState = oldState.JamState.Copy()
	s.Block = blk
	s.ParentHash = blk.Header.ParentHash

	targetJCE := blk.TimeSlot()

	// 19-22 - Safrole last
	ticketExts := blk.Tickets()
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
		return s, err
	}
	s.JamState.SafroleState = &s2
	s.JamState.tallyStatistics(s.Id, "tickets", uint32(len(ticketExts)))

	// 24 - Preimages
	preimages := blk.PreimageLookups()
	err = s.ApplyStateTransitionPreimages(preimages, targetJCE, s.Id)
	if err != nil {
		return s, err
	}

	// 23,25-27 Disputes, Assurances. Guarantees
	disputes := blk.Disputes()
	assurances := blk.Assurances()
	guarantees := blk.Guarantees()

	_, _, cores, err := s.ApplyStateTransitionRho(disputes, assurances, guarantees, targetJCE, s.Id)
	if err != nil {
		return s, err
	}

	// 28 -- ACCUMULATE OPERATIONS BASED ON cores
	s.Accumulate(cores)

	// 29 -  Update Authorization Pool alpha'
	s.ApplyStateTransitionAuthorizations(blk.Guarantees())

	// 30 - compute pi
	s.JamState.tallyStatistics(s.Id, "blocks", 1)

	s.ApplyXContext()

	s.OnTransfer(cores)

	s.Block = blk
	s.ParentHash = s.BlockHash
	s.BlockHash = blk.Hash()
	s.StateRoot = s.UpdateTrieState()
	fmt.Printf("ApplyStateTransitionFromBlock blk.Hash()=%v s.StateRoot=%v\n", blk.Hash(), s.StateRoot)

	//State transisiton is successful.  Remove E(T,P,A,G,D) from statedb queue
	s.RemoveExtrinsics(ticketExts, preimages, guarantees, assurances, disputes)
	return s, nil
}

func (s *StateDB) GetBlock() *types.Block {
	return s.Block
}

func (s *StateDB) areValidatorsAssignedToCore(coreIndex uint32, credentials []types.GuaranteeCredential) bool {
	// TODO: logic to verify if validators are assigned to the core.
	return true
}

func (s *StateDB) isReportPendingOnCore(coreIndex uint32) bool {
	// TODO: logic to check if a report is pending on the core.
	return false
}

func (s *StateDB) hasReportTimedOut(workReport types.WorkReport) bool {
	// TODO: logic to check if a work report has timed out.
	return true
}

func (s *StateDB) isCorrectCodeHash(workReport types.WorkReport) bool {
	// TODO: logic to validate the code hash prediction.
	return true
}

// make block generate block prior to state execution
func (s *StateDB) MakeBlock(credential types.ValidatorSecret, targetJCE uint32) (bl *types.Block, err error) {
	sf := s.GetSafrole()
	isNewEpoch := sf.IsNewEpoch(targetJCE)
	needWinningMarker := sf.IsTicketSubmissionCloses(targetJCE)
	stateRoot := s.GetStateRoot()
	fmt.Printf("MakeBlock start - stateRoot:%v\n", stateRoot)

	b := types.NewBlock()
	h := types.NewBlockHeader()
	extrinsicData := types.NewExtrinsic()
	h.ParentHash = s.BlockHash
	h.PriorStateRoot = stateRoot
	h.TimeSlot = targetJCE

	// Extrinsic Data has 5 different Extrinsics

	// E_P - Preimages:  aggregate queuedPreimageLookups into extrinsicData.Preimages
	extrinsicData.PreimageLookups = make([]types.PreimageLookup, 0)
	for _, preimageLookup := range s.queuedPreimageLookups {
		pl, err := preimageLookup.DeepCopy()
		if err != nil {
			continue
		}
		extrinsicData.PreimageLookups = append(extrinsicData.PreimageLookups, pl)
	}

	// 156: These pairs must be ordered and without duplicates
	for i := 0; i < len(extrinsicData.PreimageLookups); i++ {
		for j := 0; j < len(extrinsicData.PreimageLookups)-1; j++ {
			if extrinsicData.PreimageLookups[j].ServiceIndex > extrinsicData.PreimageLookups[j+1].ServiceIndex {
				extrinsicData.PreimageLookups[j], extrinsicData.PreimageLookups[j+1] = extrinsicData.PreimageLookups[j+1], extrinsicData.PreimageLookups[j]
			}
		}
	}
	s.queuedPreimageLookups = make(map[common.Hash]types.PreimageLookup)

	// E_G - Guarantees: aggregate queuedGuarantees into extrinsicData.Guarantees
	extrinsicData.Guarantees = make([]types.Guarantee, 0)
	for _, guarantee := range s.queuedGuarantees {
		g, err := guarantee.DeepCopy()
		if err != nil {
			continue
		}
		// 138 - Ensure we have 2 or 3 credentials per work report minimum.
		if len(g.Credentials) < 2 {
			// Skip guarantees that do not meet the minimum number of credentials.
			continue
		}
		extrinsicData.Guarantees = append(extrinsicData.Guarantees, g)
	}
	// 139 - Order guarantees by core (assuming WorkReport contains core index).
	sort.Slice(extrinsicData.Guarantees, func(i, j int) bool {
		return extrinsicData.Guarantees[i].WorkReport.Core < extrinsicData.Guarantees[j].WorkReport.Core
	})

	// 140 - credentials ordered by validator index
	for i := range extrinsicData.Guarantees {
		sort.Slice(extrinsicData.Guarantees[i].Credentials, func(a, b int) bool {
			return extrinsicData.Guarantees[i].Credentials[a].ValidatorIndex < extrinsicData.Guarantees[i].Credentials[b].ValidatorIndex
		})
	}
	// 141 - The signing validators must be assigned to the core in G or G*
	for _, guarantee := range extrinsicData.Guarantees {
		if !s.areValidatorsAssignedToCore(guarantee.WorkReport.Core, guarantee.Credentials) {
			// Handle the case where validators are not correctly assigned.
			return nil, errors.New("validators not correctly assigned to core")
		}
	}
	// 144 - No reports may be placed on cores with a report pending availability on it unless it has timed out.
	for _, guarantee := range extrinsicData.Guarantees {
		if s.isReportPendingOnCore(guarantee.WorkReport.Core) && !s.hasReportTimedOut(guarantee.WorkReport) {
			// Skip this guarantee if the core has a pending report that hasn't timed out.
			continue
		}
	}
	// 147 - There must be no duplicate work-package hashes (i.e. two work-reports of the same package).
	workPackageHashes := make(map[common.Hash]bool)
	for _, guarantee := range extrinsicData.Guarantees {
		hash := guarantee.WorkReport.AvailabilitySpec.WorkPackageHash
		if workPackageHashes[hash] {
			// Handle duplicate work-package hash.
			return nil, errors.New("duplicate work-package hash detected")
		}
		workPackageHashes[hash] = true
	}

	// TODO: (skip) Recent Blocks

	// 153 - We require that all work results within the extrinsic predicted the correct code hash for their corresponding service:
	for _, guarantee := range extrinsicData.Guarantees {
		if !s.isCorrectCodeHash(guarantee.WorkReport) {
			// Handle incorrect code hash prediction.
			return nil, errors.New("incorrect code hash prediction for service")
		}
	}
	s.queuedGuarantees = make(map[common.Hash]types.Guarantee)

	// E_A - Assurances  aggregate queuedAssurances into extrinsicData.Assurances
	extrinsicData.Assurances = make([]types.Assurance, 0)
	for _, assurance := range s.queuedAssurances {
		a, err := assurance.DeepCopy()
		if err != nil {
			continue
		}
		// 125 - The assurances must all be anchored on the parent
		if a.ParentHash != s.ParentHash {
			continue
		}
		extrinsicData.Assurances = append(extrinsicData.Assurances, a)
	}
	// 126 - The assurances must ordered by validator index:
	sort.Slice(extrinsicData.Assurances, func(i, j int) bool {
		return extrinsicData.Assurances[i].ValidatorIndex < extrinsicData.Assurances[j].ValidatorIndex
	})
	s.queuedAssurances = make(map[common.Hash]types.Assurance)

	// E_D - Disputes: aggregate queuedDisputes into extrinsicData.Disputes
	d := s.GetJamState()
	needMarkerVerdicts := d.NeedsVerdictsMarker(targetJCE)
	needMarkerOffenders := d.NeedsOffendersMarker(targetJCE)
	if needMarkerVerdicts {
		// TODO: 116 - b.Header.VerdictsMarkers
	}
	if needMarkerOffenders {
		// TODO: 117 - b.Header.OffenderMarkers
	}
	extrinsicData.Disputes = make([]types.Dispute, 0)
	for _, dispute := range s.queuedDisputes {
		d, err := dispute.DeepCopy()
		if err != nil {
			continue
		}
		extrinsicData.Disputes = append(extrinsicData.Disputes, d)
	}
	// TODO: 103 Verdicts v must be ordered by report hash.
	// TODO: 104 Offender signatures c and f must each be ordered by the validator’s Ed25519 key.
	// TODO: 105 There may be no duplicate report hashes within the extrinsic, nor amongst any past reported hashes.
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
			if s.JamState.SafroleState.InTicketAccumulator(ticketID) {
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
		return bl, err
	}
	h.BlockAuthorKey = author_index
	b.Extrinsic = extrinsicData

	unsignHeaderHash := h.UnsignedHash()

	//signing
	epochType := sf.CheckEpochType()
	if epochType == "fallback" {
		blockseal, fresh_vrfSig, err := sf.SignFallBack(credential.BandersnatchSecret, unsignHeaderHash)
		if err != nil {
			return bl, err
		}
		h.BlockSeal = blockseal
		h.VRFSignature = fresh_vrfSig
	} else {
		attempt, err := sf.GetBindedAttempt(targetJCE)
		blockseal, fresh_vrfSig, err := sf.SignPrimary(credential.BandersnatchSecret, unsignHeaderHash, attempt)
		if err != nil {
			return bl, err
		}
		h.BlockSeal = blockseal
		h.VRFSignature = fresh_vrfSig
	}
	b.Header = *h
	return b, nil
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
