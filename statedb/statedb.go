package statedb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

type StateDB struct {
	Finalized  bool
	Id         uint16       `json:"id"`
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
	queuedPreimageLookups map[common.Hash]types.Preimages
	preimageLookupsMutex  sync.Mutex

	knownGuarantees  map[common.Hash]int
	queuedGuarantees map[common.Hash]types.Guarantee
	guaranteeMutex   sync.Mutex

	knownAssurances  map[common.Hash]int
	queuedAssurances map[common.Hash]types.Assurance
	assuranceMutex   sync.Mutex

	knownJudgements map[common.Hash]int
	queueJudgements map[common.Hash]types.Judgement
	judgementMutex  sync.Mutex

	knownTickets  map[common.Hash]uint8
	queuedTickets map[common.Hash][]types.Ticket
	ticketMutex   sync.Mutex

	// used in ApplyStateRecentHistory between statedbs
	accumulationRoot common.Hash

	X *types.XContext

	GuarantorAssignments         []types.GuarantorAssignment
	PreviousGuarantorAssignments []types.GuarantorAssignment
	AvailableWorkReport          []types.WorkReport // every block has its own available work report

	logChan chan storage.LogMessage
}

func (s *StateDB) AddTicketToQueue(t types.Ticket, used_entropy common.Hash) {
	s.ticketMutex.Lock()
	defer s.ticketMutex.Unlock()
	s.queuedTickets[used_entropy] = append(s.queuedTickets[used_entropy], t)
}

func (s *StateDB) AddLookupToQueue(l types.Preimages) {
	s.preimageLookupsMutex.Lock()
	defer s.preimageLookupsMutex.Unlock()
	s.queuedPreimageLookups[l.AccountPreimageHash()] = l
}

func (s *StateDB) AddJudgementToQueue(j types.Judgement) {
	s.judgementMutex.Lock()
	defer s.judgementMutex.Unlock()
	s.queueJudgements[j.WorkReportHash] = j
}

func (s *StateDB) AddGuaranteeToQueue(g types.Guarantee) {
	s.guaranteeMutex.Lock()
	defer s.guaranteeMutex.Unlock()
	s.queuedGuarantees[g.Hash()] = g
	if debug {
		fmt.Printf("[N%v] [statedb:AddGuaranteeToQueue] -- Adding guarantee workPackageHash: %v\n", s.Id, g.Report.GetWorkPackageHash())
	}
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

func (s *StateDB) writeLog(obj interface{}, timeslot uint32) {
	s.sdb.WriteLog(obj, timeslot)
}

func (s *StateDB) ProcessIncomingTicket(t types.Ticket) {
	//s.QueueTicketEnvelope(t)
	//statedb.tickets[common.BytesToHash(ticket_id)] = t
	sf := s.GetSafrole()
	start := time.Now()
	ticketID, entropy_idx, err := sf.ValidateIncomingTicket(&t)
	if err != nil {
		if debug {
			fmt.Printf("ProcessIncomingTicket Error Invalid Ticket. Err=%v\n", err)
		}
		return
	}
	elapsed := time.Since(start).Microseconds()
	if trace && elapsed > 500000 {
		fmt.Printf("[N%v] ProcessIncomingTicket -- Adding ticketID=%v [%d ms]\n", s.Id, ticketID, time.Since(start).Microseconds()/1000)
	}

	if s.CheckTicketExists(ticketID) {
		return
	}

	used_entropy := s.GetSafrole().Entropy[entropy_idx]
	s.AddTicketToQueue(t, used_entropy)
	//TODO: log ticket here
	currJCE, _ := s.JamState.SafroleState.CheckTimeSlotReady()
	s.writeLog(&t, currJCE)
	s.knownTickets[ticketID] = t.Attempt
}

func (s *StateDB) ProcessIncomingLookup(l types.Preimages) {
	//TODO: check existence of lookup and stick into map
	//cj := s.GetPvmState()
	//fmt.Printf("[N%v] ProcessIncomingLookup -- Adding lookup: %v\n", s.Id, l.String())
	account_preimage_hash := l.AccountPreimageHash()
	// Willaim: dont do validation here. Instead, do have a procedure to remove stale EP
	/*
		account_preimage_hash, err := s.ValidateLookup(&l)
		if err != nil {
			fmt.Printf("Invalid lookup. Err=%v\n", err)
			return
		}
	*/
	if s.CheckLookupExists(account_preimage_hash) {
		return
	}
	s.AddLookupToQueue(l)
	sf := s.GetSafrole()
	s.knownPreimageLookups[account_preimage_hash] = sf.GetTimeSlot() // hostForget has certain logic that will probably reference this field???
}

func (s *StateDB) ProcessIncomingJudgement(j types.Judgement) {
	// get the disputes state

}

func (s *StateDB) ProcessIncomingGuarantee(g types.Guarantee) {
	// get the guarantee state
	CurrV := s.GetSafrole().CurrValidators
	err := g.Verify(CurrV)
	if err != nil {
		fmt.Printf("ProcessIncomingGuarantee: Invalid guarantee %s. Err=%v\n", g.String(), err)
		return
	}
	s.AddGuaranteeToQueue(g)
}

func (s *StateDB) getValidatorCredential() []byte {
	// TODO
	return nil
}

func (s *StateDB) ProcessIncomingAssurance(a types.Assurance) {
	if len(a.Signature) == 0 {
		fmt.Printf("Invalid Assurance. Err=%v\n", errors.New("Empty signature"))
		return
	}
	cred := s.GetSafrole().GetCurrValidator(int(a.ValidatorIndex))
	err := a.Verify(s.BlockHash, cred)
	if err != nil {
		fmt.Printf("Invalid Assurance. Err=%v\n", err)
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
	using_entropy := s.GetSafrole().Entropy[2]
	for i, ticket := range s.queuedTickets[using_entropy] {
		if ticket.Hash() == t.Hash() {
			s.queuedTickets[using_entropy] = append(s.queuedTickets[using_entropy][:i], s.queuedTickets[using_entropy][i+1:]...)
			break
		}
	}

}

func (s *StateDB) RemoveLookup(p *types.Preimages) {
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

func (s *StateDB) RemoveJudgement(j *types.Judgement) {
	s.judgementMutex.Lock()
	defer s.judgementMutex.Unlock()
	delete(s.queueJudgements, j.WorkReportHash)
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
	debug                     = false
	debugA                    = false
	debugG                    = false
	debugAudit                = false
	trace                     = false
	errServiceIndices         = "ServiceIndices duplicated or not ordered"
	errPreimageLookupNotSet   = "Preimagelookup (h,l) not set"
	errPreimageLookupNotEmpty = "Preimagelookup not empty"
	errPreimageBlobSet        = "PreimageBlob already set"
)

func (s *StateDB) ValidateLookup(l *types.Preimages) (common.Hash, error) {
	// check 157 - (1) a_p not equal to P (2) a_l is empty
	t := s.GetTrie()
	a_p := l.AccountPreimageHash()
	//a_l := l.AccountLookupHash()
	preimage_blob, err := t.GetPreImageBlob(l.Service_Index(), l.BlobHash())
	//TODO: stanley to make sure we can check whether a key exist or not. err here is ambiguous here
	if err == nil { // key found
		if l.BlobHash() == common.Blake2Hash(preimage_blob) {
			//H(p) = p
			fmt.Printf("Fail at 157 - (1) a_p not equal to P\n")
			return common.Hash{}, fmt.Errorf(errPreimageBlobSet)
		}
	}

	//fmt.Printf("Validating E_p %v\n",l.String())
	anchors, err := t.GetPreImageLookup(l.Service_Index(), l.BlobHash(), l.BlobLength())
	if err != nil {
		fmt.Printf("Fail at no lookup\n")
		return common.Hash{}, fmt.Errorf(errPreimageLookupNotSet) //TODO: differentiate key not found vs leveldb error
	}
	if len(anchors) == 0 {
		// non-empty anchor
		fmt.Printf("Fail at anchor wrong\n")
		return common.Hash{}, fmt.Errorf(errPreimageLookupNotEmpty)
	}
	return a_p, nil
}
func newEmptyStateDB(sdb *storage.StateDBStorage) (statedb *StateDB) {
	statedb = new(StateDB)
	statedb.queuedTickets = make(map[common.Hash][]types.Ticket)
	statedb.knownTickets = make(map[common.Hash]uint8)
	statedb.queueJudgements = make(map[common.Hash]types.Judgement)
	statedb.knownJudgements = make(map[common.Hash]int)
	statedb.queuedGuarantees = make(map[common.Hash]types.Guarantee)
	statedb.knownGuarantees = make(map[common.Hash]int)
	statedb.queuedAssurances = make(map[common.Hash]types.Assurance)
	statedb.knownAssurances = make(map[common.Hash]int)
	statedb.queuedPreimageLookups = make(map[common.Hash]types.Preimages)
	statedb.knownPreimageLookups = make(map[common.Hash]uint32)
	statedb.SetStorage(sdb)
	statedb.trie = trie.NewMerkleTree(nil, sdb)
	statedb.logChan = make(chan storage.LogMessage, 100)
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
	C14 = "AccumulationQueue"
	C15 = "AccumulationHistory"
)

// Initial services
const (
	BootstrapServiceCode = 0
	BootstrapServiceFile = "../services/bootstrap.pvm"
)

// NewGenesisStateDB generates the first StateDB object and genesis statedb
func NewGenesisStateDB(sdb *storage.StateDBStorage, c *GenesisConfig) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return
	}

	statedb.Block = nil
	statedb.JamState = InitGenesisState(c)

	// Load services into genesis state
	services := []types.TestService{
		{ServiceCode: BootstrapServiceCode, FileName: BootstrapServiceFile},
		// Add more services here as needed IF they are needed for Genesis
	}

	for _, service := range services {
		code, err := os.ReadFile(service.FileName)
		if err != nil {
			return statedb, err
		}
		codeHash := common.Blake2Hash(code)
		bootstrapServiceAccount := types.ServiceAccount{
			CodeHash:        codeHash,
			Balance:         10000,
			GasLimitG:       100,
			GasLimitM:       100,
			StorageSize:     uint64(len(code)),
			NumStorageItems: 1,
		}
		statedb.WriteServicePreimageBlob(service.ServiceCode, code)
		statedb.WriteService(service.ServiceCode, bootstrapServiceAccount)
	}

	statedb.StateRoot = statedb.UpdateTrieState()
	return statedb, nil
}

func InitStateDBFromSnapshot(sdb *storage.StateDBStorage, snapshot *StateSnapshot) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}

	statedb.Block = nil
	statedb.JamState = InitStateFromSnapshot(snapshot)
	// setting the safrole state so that block 1 can be produced
	statedb.StateRoot = statedb.UpdateTrieState()

	return statedb, nil
}

func InitStateDBFromGenesis(sdb *storage.StateDBStorage, snapshot *StateSnapshot, genesis string) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}

	statedb.Block = nil
	statedb.JamState = InitStateFromSnapshot(snapshot)
	// setting the safrole state so that block 1 can be produced
	// statedb.StateRoot = statedb.UpdateTrieState()
	statedb.StateRoot = statedb.UpdateAllTrieState(genesis)

	return statedb, nil
}

func (s *StateDB) HeaderHash() common.Hash {
	return s.Block.Header.Hash()
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

func (s *StateDB) SetJamState(jamState *JamState) {
	s.JamState = jamState
}

func (s *StateDB) GetJamSnapshot() *StateSnapshot {
	return s.JamState.Snapshot()
}

func (s *StateDB) RecoverJamState(stateRoot common.Hash) {
	// Now read C1.....C13 from the trie and put it back into JamState
	//t := s.GetTrie()

	t := s.CopyTrieState(stateRoot)

	coreAuthPoolEncode, err := t.GetState(C1)
	if err != nil {
		fmt.Printf("Error reading C1 CoreAuthPool from trie: %v\n", err)
	}
	authQueueEncode, err := t.GetState(C2)
	if err != nil {
		fmt.Printf("Error reading C2 AuthQueue from trie: %v\n", err)
	}
	recentBlocksEncode, err := t.GetState(C3)
	if err != nil {
		fmt.Printf("Error reading C3 RecentBlocks from trie: %v\n", err)
	}
	safroleStateEncode, err := t.GetState(C4)
	if err != nil {
		fmt.Printf("Error reading C4 SafroleState from trie: %v\n", err)
	}
	disputeStateEncode, err := t.GetState(C5)
	if err != nil {
		fmt.Printf("Error reading C5 DisputeState from trie: %v\n", err)
	}
	entropyEncode, err := t.GetState(C6)
	if err != nil {
		fmt.Printf("Error reading C6 Entropy from trie: %v\n", err)
	}
	nextEpochValidatorsEncode, err := t.GetState(C7)
	if err != nil {
		fmt.Printf("Error reading C7 NextEpochValidators from trie: %v\n", err)
	}
	currEpochValidatorsEncode, err := t.GetState(C8)
	if err != nil {
		fmt.Printf("Error reading C8 CurrentEpochValidators from trie: %v\n", err)
	}
	priorEpochValidatorEncode, err := t.GetState(C9)
	if err != nil {
		fmt.Printf("Error reading C9 PriorEpochValidators from trie: %v\n", err)
	}
	rhoEncode, err := t.GetState(C10)
	if err != nil {
		fmt.Printf("Error reading C10 Rho from trie: %v\n", err)
	}
	mostRecentBlockTimeSlotEncode, err := t.GetState(C11)
	if err != nil {
		fmt.Printf("Error reading C11 MostRecentBlockTimeSlot from trie: %v\n", err)
	}
	privilegedServiceIndicesEncode, err := t.GetState(C12)
	if err != nil {
		fmt.Printf("Error reading C12 PrivilegedServiceIndices from trie: %v\n", err)
	}
	piEncode, err := t.GetState(C13)
	if err != nil {
		fmt.Printf("Error reading C13 ActiveValidator from trie: %v\n", err)
	}
	accunulateQueueEncode, err := t.GetState(C14)
	if err != nil {
		fmt.Printf("Error reading C14 accunulateQueue from trie: %v\n", err)
	}
	accunulateHistoryEncode, err := t.GetState(C15)
	if err != nil {
		fmt.Printf("Error reading C15 accunulateHistory from trie: %v\n", err)
	}
	//Decode(authQueueEncode) -> AuthorizationQueue
	//set AuthorizationQueue back to JamState

	// fmt.Printf("retrieved C7 NextEpochValidators %v\n", nextEpochValidatorsEncode)
	// fmt.Printf("retrieved C8 CurrentEpochValidators%v\n", currEpochValidatorsEncode)
	// fmt.Printf("retrieved C9 PriorEpochValidators%v\n", priorEpochValidatorEncode)
	// fmt.Printf("retrieved C7 NextEpochValidators %v\n", nextEpochValidatorsEncode)
	// fmt.Printf("retrieved C8 CurrentEpochValidators%v\n", currEpochValidatorsEncode)
	// fmt.Printf("retrieved C9 PriorEpochValidators%v\n", priorEpochValidatorEncode)

	d := s.GetJamState()
	d.SetAuthPool(coreAuthPoolEncode)
	d.SetAuthQueue(authQueueEncode)
	d.SetRecentBlocks(recentBlocksEncode)
	d.SetSafroleState(safroleStateEncode)
	d.SetPsi(disputeStateEncode)
	d.SetEntropy(entropyEncode)
	d.SetNextEpochValidators(nextEpochValidatorsEncode)
	d.SetCurrEpochValidators(currEpochValidatorsEncode)
	d.SetPriorEpochValidators(priorEpochValidatorEncode)
	d.SetMostRecentBlockTimeSlot(mostRecentBlockTimeSlotEncode)
	d.SetRho(rhoEncode)
	d.SetPrivilegedServicesIndices(privilegedServiceIndicesEncode)
	d.SetPi(piEncode)
	d.SetAccumulateQueue(accunulateQueueEncode)
	d.SetAccumulateHistory(accunulateHistoryEncode)
	s.SetJamState(d)
	//fmt.Printf("[N%v] RecoverJamState %v\n", s.Id, s.GetJamSnapshot())
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
	safroleStateEncode := sb.GetSafroleStateBytes()
	entropyEncode := sf.GetEntropyBytes()
	nextEpochValidatorsEncode := sf.GetNextEpochValidatorsBytes()
	currEpochValidatorsEncode := sf.GetCurrEpochValidatorsBytes()
	priorEpochValidatorEncode := sf.GetPriorEpochValidatorsBytes()
	mostRecentBlockTimeSlotEncode := sf.GetMostRecentBlockTimeSlotBytes()

	d := s.GetJamState()
	disputeState := d.GetPsiBytes()
	rhoEncode := d.GetRhoBytes()
	piEncode := d.GetPiBytes()
	coreAuthPoolEncode := d.GetAuthPoolBytes()
	authQueueEncode := d.GetAuthQueueBytes()
	privilegedServiceIndicesEncode := d.GetPrivilegedServicesIndicesBytes()
	recentBlocksEncode := d.GetRecentBlocksBytes()

	accunulateQueueEncode := d.GetAccumulationQueueBytes()
	accunulateHistoryEncode := d.GetAccumulationHistoryBytes()

	t := s.GetTrie()
	prev_root := t.GetRoot()
	debug := false
	verify := true
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
	t.SetState(C14, accunulateQueueEncode)
	t.SetState(C15, accunulateHistoryEncode)
	updated_root := t.GetRoot()

	if debug {
		fmt.Printf("[N%v] UpdateTrieState - before root:%v\n", s.Id, prev_root)
		fmt.Printf("[N%v] UpdateTrieState - after root:%v\n", s.Id, updated_root)
		// fmt.Printf("C1 coreAuthPoolEncode %x \n", coreAuthPoolEncode)
		// fmt.Printf("C2 authQueueEncode %x \n", authQueueEncode)
		// fmt.Printf("C3 recentBlocksEncode %x \n", recentBlocksEncode)
		// fmt.Printf("C4 safroleStateEncode %x \n", safroleStateEncode)
		// fmt.Printf("C5 disputeState %x \n", disputeState)
		// fmt.Printf("C6 entropyEncode %x \n", entropyEncode)
		// fmt.Printf("C7 nextEpochValidatorsEncode %x \n", nextEpochValidatorsEncode)
		// fmt.Printf("C8 currEpochValidatorsEncode %x \n", currEpochValidatorsEncode)
		// fmt.Printf("C9 priorEpochValidatorEncode %x \n", priorEpochValidatorEncode)
		// fmt.Printf("C10 rhoEncode %x \n", rhoEncode)
		// fmt.Printf("C11 mostRecentBlockTimeSlotEncode %x \n", mostRecentBlockTimeSlotEncode)
		// fmt.Printf("C12 privilegedServiceIndicesEncode %x \n", privilegedServiceIndicesEncode)
		// fmt.Printf("C13 piEncode %x \n", piEncode)
		// fmt.Printf("C14 accunulateQueueEncode %x \n", accunulateQueueEncode)
		// fmt.Printf("C15 accunulateHistoryEncode %x \n", accunulateHistoryEncode)
	}

	if debug || verify {
		t2, _ := trie.InitMerkleTreeFromHash(updated_root.Bytes(), s.sdb)
		checkingResult, err := CheckingAllState(t, t2)
		if !checkingResult || err != nil {
			panic(fmt.Sprintf("CheckingAllState ERROR: %v\n", err))
		}
	}

	/*
		// use C1
		startKey := common.Hex2Bytes("0x0100000000000000000000000000000000000000000000000000000000000000")

		// use C14
		endKey := common.Hex2Bytes("0x0d00000000000000000000000000000000000000000000000000000000000000")

		// maxSize
		maxSize := uint32(10000)
		foundKeyVal, boundaryNode, err := t.GetStateByRange(startKey, endKey, maxSize)
		if err != nil {
			fmt.Printf("Error getting state by range: %v\n", err)
		}


		fmt.Printf("foundKeyVal: ")
		for i, kv := range foundKeyVal {
			fmt.Printf("[%d] %x\n", i, kv)
		}
		fmt.Printf("\n")
		fmt.Printf("boundaryNode: ")
		for i, nodeHash := range boundaryNode {
			fmt.Printf("[%d] %x\n", i, nodeHash)
		}
		fmt.Printf("\n")
	*/
	return updated_root
}

func (s *StateDB) GetAllKeyValues() []KeyVal {
	startKey := common.Hex2Bytes("0x0000000000000000000000000000000000000000000000000000000000000000")
	endKey := common.Hex2Bytes("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	maxSize := uint32(math.MaxUint32)
	trie := s.CopyTrieState(s.StateRoot)
	foundKeyVal, _, _ := trie.GetStateByRange(startKey, endKey, maxSize)

	KeyVals := make([]KeyVal, 0)
	for _, keyValue := range foundKeyVal {
		var keyVal [2][]byte
		realKey := trie.GetRealKey(keyValue.Key, keyValue.Value)
		keyVal[0] = make([]byte, len(realKey))
		keyVal[1] = make([]byte, len(keyValue.Value))
		copy(keyVal[0], realKey)
		copy(keyVal[1], keyValue.Value)
		KeyVals = append(KeyVals, keyVal)
	}
	return KeyVals
}

func (s *StateDB) CompareStateRoot(genesis []KeyVal, parentStateRoot common.Hash) (bool, error) {
	parent_root := s.StateRoot
	newTrie := trie.NewMerkleTree(nil, s.sdb)
	for _, kv := range genesis {
		newTrie.SetRawKeyVal(common.Hash(kv[0]), kv[1])
	}
	new_root := newTrie.GetRoot()
	//timeslot := s.GetSafrole().Timeslot
	if !common.CompareBytes(parent_root[:], new_root[:]) {
		return false, fmt.Errorf("Roots are not the same")
	}

	//fmt.Printf("[%d] ", timeslot)
	//fmt.Printf("current_root, new_root %v %v parentStateRoot: %v\n", parent_root, new_root, parentStateRoot)

	return true, nil
}

func (s *StateDB) UpdateAllTrieState(genesis string) common.Hash {
	//γ ≡⎩γk, γz, γs, γa⎭
	//γk :one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	//γz :epoch’s root, a Bandersnatch ring root composed with the one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	//γa :the ticket accumulator, a series of highest-scoring ticket identifiers to be used for the next epoch (epoch N+1)
	//γs :current epoch’s slot-sealer series, which is either a full complement of E tickets or, in the case of a fallback mode, a series of E Bandersnatch keys (epoch N)
	snapshotBytesRaw, err := os.ReadFile(genesis)
	if err != nil {
		log.Fatalf("[readSnapshot:ReadFile] %s ERR %v\n", genesis, err)
		return common.Hash{}
	}
	snapshotRaw := StateSnapshotRaw{}
	json.Unmarshal(snapshotBytesRaw, &snapshotRaw)

	t := s.GetTrie()
	prev_root := t.GetRoot()
	debug := false
	verify := true

	for _, kv := range snapshotRaw.KeyVals {
		t.SetRawKeyVal(common.Hash(kv[0]), kv[1])
		fmt.Printf("SetRawKeyVal %x %x\n", common.Hash(kv[0]), kv[1])
	}
	updated_root := t.GetRoot()

	sf := s.GetSafrole()
	if sf == nil {
		fmt.Printf("NO SAFROLE %v", s)
		panic(222)
	}

	if debug {
		fmt.Printf("[N%v] UpdateTrieState - before root:%v\n", s.Id, prev_root)
		fmt.Printf("[N%v] UpdateTrieState - after root:%v\n", s.Id, updated_root)
	}

	if debug || verify {
		t2, _ := trie.InitMerkleTreeFromHash(updated_root.Bytes(), s.sdb)
		checkingResult, err := CheckingAllState(t, t2)
		if !checkingResult || err != nil {
			panic(fmt.Sprintf("CheckingAllState ERROR: %v\n", err))
		}
	}
	return updated_root
}

func (s *StateDB) GetSafroleState() *SafroleState {
	return s.JamState.SafroleState
}

func CheckingAllState(t *trie.MerkleTree, t2 *trie.MerkleTree) (bool, error) {
	c1a, _ := t.GetState(C1)
	c1b, _ := t2.GetState(C1)
	if !common.CompareBytes(c1a, c1b) {
		fmt.Printf("C1 is not the same\n")
		return false, fmt.Errorf("C1 is not the same")
	}
	c2a, _ := t.GetState(C2)
	c2b, _ := t2.GetState(C2)
	if !common.CompareBytes(c2a, c2b) {
		fmt.Printf("C2 is not the same\n")
		return false, fmt.Errorf("C2 is not the same")
	}
	c3a, _ := t.GetState(C3)
	c3b, _ := t2.GetState(C3)
	if !common.CompareBytes(c3a, c3b) {
		fmt.Printf("C3 is not the same\n")
		return false, fmt.Errorf("C3 is not the same")
	}
	c4a, _ := t.GetState(C4)
	c4b, _ := t2.GetState(C4)
	if !common.CompareBytes(c4a, c4b) {
		fmt.Printf("C4 is not the same\n")
		return false, fmt.Errorf("C4 is not the same")
	}
	c5a, _ := t.GetState(C5)
	c5b, _ := t2.GetState(C5)
	if !common.CompareBytes(c5a, c5b) {
		fmt.Printf("C5 is not the same\n")
		return false, fmt.Errorf("C5 is not the same")
	}
	c6a, _ := t.GetState(C6)
	c6b, _ := t2.GetState(C6)
	if !common.CompareBytes(c6a, c6b) {
		fmt.Printf("C6 is not the same\n")
		return false, fmt.Errorf("C6 is not the same")
	}
	c7a, _ := t.GetState(C7)
	c7b, _ := t2.GetState(C7)
	if !common.CompareBytes(c7a, c7b) {
		fmt.Printf("C7 is not the same\n")
		return false, fmt.Errorf("C7 is not the same")
	}
	c8a, _ := t.GetState(C8)
	c8b, _ := t2.GetState(C8)
	if !common.CompareBytes(c8a, c8b) {
		fmt.Printf("C8 is not the same\n")
		return false, fmt.Errorf("C8 is not the same")
	}
	c9a, _ := t.GetState(C9)
	c9b, _ := t2.GetState(C9)
	if !common.CompareBytes(c9a, c9b) {
		fmt.Printf("C9 is not the same\n")
		return false, fmt.Errorf("C9 is not the same")
	}
	c10a, _ := t.GetState(C10)
	c10b, _ := t2.GetState(C10)
	if !common.CompareBytes(c10a, c10b) {
		fmt.Printf("C10 is not the same\n")
		return false, fmt.Errorf("C10 is not the same")
	}
	c11a, _ := t.GetState(C11)
	c11b, _ := t2.GetState(C11)
	if !common.CompareBytes(c11a, c11b) {
		fmt.Printf("C11 is not the same\n")
		return false, fmt.Errorf("C11 is not the same")
	}
	c12a, _ := t.GetState(C12)
	c12b, _ := t2.GetState(C12)
	if !common.CompareBytes(c12a, c12b) {
		fmt.Printf("C12 is not the same\n")
		return false, fmt.Errorf("C12 is not the same")
	}
	c13a, _ := t.GetState(C13)
	c13b, _ := t2.GetState(C13)
	if !common.CompareBytes(c13a, c13b) {
		fmt.Printf("C13 is not the same\n")
		return false, fmt.Errorf("C13 is not the same")
	}
	c14a, _ := t.GetState(C14)
	c14b, _ := t2.GetState(C14)
	if !common.CompareBytes(c14a, c14b) {
		fmt.Printf("C14 is not the same\n")
		return false, fmt.Errorf("C14 is not the same")
	}
	c15a, _ := t.GetState(C15)
	c15b, _ := t2.GetState(C15)
	if !common.CompareBytes(c15a, c15b) {
		fmt.Printf("C15 is not the same\n")
		return false, fmt.Errorf("C15 is not the same")
	}
	return true, nil
}

// func (s *StateDB) writeLog(obj interface{}, timeslot uint32) {
// 	WriteLog
//
//     msg := storage.LogMessage{
//         Payload:  obj,
//         Timeslot: timeslot,
//     }
// 	fmt.Printf("sending logMsg: %v\n", msg)
//     s.logChan <- msg
// }

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
	statedb.Finalized = false
	statedb.queuedTickets = make(map[common.Hash][]types.Ticket)
	statedb.knownTickets = make(map[common.Hash]uint8)
	statedb.queueJudgements = make(map[common.Hash]types.Judgement)
	statedb.knownJudgements = make(map[common.Hash]int)

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

		h := common.Blake2Hash(encodedBlock)
		if bytes.Compare(h.Bytes(), blockHash.Bytes()) != 0 {
			return statedb, fmt.Errorf("[statedb:newStateDB] hash of data incorrect [%d bytes]", len(encodedBlock))
		}
		if err := json.Unmarshal(encodedBlock, &block); err != nil {
			return statedb, fmt.Errorf("[statedb:newStateDB] JSON decode error: %v", err)
		}
		statedb.Block = &block
		statedb.ParentHash = block.Header.Parent
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
	tmpAvailableWorkReport := make([]types.WorkReport, len(s.AvailableWorkReport))
	copy(tmpAvailableWorkReport, s.AvailableWorkReport)
	newStateDB = &StateDB{
		Id:                    s.Id,
		Block:                 s.Block.Copy(), // You might need to deep copy the Block if it's mutable
		ParentHash:            s.ParentHash,
		BlockHash:             s.BlockHash,
		StateRoot:             s.StateRoot,
		JamState:              s.JamState.Copy(), // DisputesState has a Copy method
		sdb:                   s.sdb,
		accumulationRoot:      s.accumulationRoot, // compressed C
		trie:                  s.CopyTrieState(s.StateRoot),
		knownTickets:          make(map[common.Hash]uint8),
		knownPreimageLookups:  make(map[common.Hash]uint32),
		knownGuarantees:       make(map[common.Hash]int),
		knownAssurances:       make(map[common.Hash]int),
		knownJudgements:       make(map[common.Hash]int),
		queuedTickets:         make(map[common.Hash][]types.Ticket),
		queuedPreimageLookups: make(map[common.Hash]types.Preimages),
		queuedGuarantees:      make(map[common.Hash]types.Guarantee),
		queuedAssurances:      make(map[common.Hash]types.Assurance),
		queueJudgements:       make(map[common.Hash]types.Judgement),
		logChan:               make(chan storage.LogMessage, 100),
		AvailableWorkReport:   tmpAvailableWorkReport,
		/*
			Following flds are not copied over..?

			VMs       map[uint32]*pvm.VM
			vmMutex   sync.Mutex
			X 		  XContext
			S 		  uint32

		*/
	}
	s.CloneExtrinsicMap(newStateDB)
	newStateDB.AssignGuarantors(true)
	newStateDB.PreviousGuarantors(true)
	return newStateDB
}

func (s *StateDB) CloneExtrinsicMap(n *StateDB) {
	// Tickets
	for k, v := range s.knownTickets {
		n.knownTickets[k] = v
	}
	// use copy
	for k, v := range s.queuedTickets {
		t := make([]types.Ticket, len(v))
		copy(t, v)
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

	// Vote
	for k, v := range s.knownJudgements {
		n.knownJudgements[k] = v
	}
	for k, v := range s.queueJudgements {
		d, _ := v.DeepCopy()
		n.queueJudgements[k] = d
	}
}

func (s *StateDB) RemoveExtrinsics(tickets []types.Ticket, lookups []types.Preimages, guarantees []types.Guarantee, assurances []types.Assurance, dispute types.Dispute) {
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
	// TODO: manage dispute
}

func (s *StateDB) ProcessState(credential types.ValidatorSecret, ticketIDs []common.Hash) (*types.Block, *StateDB, error) {
	genesisReady := s.JamState.SafroleState.CheckFirstPhaseReady()
	if !genesisReady {
		return nil, nil, nil
	}
	currJCE, timeSlotReady := s.JamState.SafroleState.CheckTimeSlotReady()
	if timeSlotReady {
		// Time to propose block if authorized
		isAuthorizedBlockBuilder := false
		sf := s.GetSafrole()
		isAuthorizedBlockBuilder = sf.IsAuthorizedBuilder(currJCE, common.Hash(credential.BandersnatchPub), ticketIDs)
		if isAuthorizedBlockBuilder {
			// propose block without state transition
			start := time.Now()
			proposedBlk, err := s.MakeBlock(credential, currJCE)
			if err != nil {
				return nil, nil, err
			}
			newStateDB, err := ApplyStateTransitionFromBlock(s, context.Background(), proposedBlk)
			if err != nil {
				// HOW could this happen, we made the block ourselves!
				return nil, nil, err
			}

			currEpoch, currPhase := s.JamState.SafroleState.EpochAndPhase(currJCE)
			AddDrawBlock(common.Str(proposedBlk.Hash()), common.Str(proposedBlk.ParentHash()), int(proposedBlk.Header.AuthorIndex), fmt.Sprintf("%d", proposedBlk.Header.Slot))
			fmt.Printf("[N%v] \033[33m Blk %s<-%s \033[0m e'=%d,m'=%02d, len(γ_a')=%d   \t%s %s\n", s.Id, common.Str(proposedBlk.ParentHash()), common.Str(proposedBlk.Hash()), currEpoch, currPhase, len(newStateDB.JamState.SafroleState.NextEpochTicketsAccumulator), proposedBlk.Str(), newStateDB.JamState.GetValidatorStats())
			elapsed := time.Since(start)
			if trace && elapsed > 2000000 {
				fmt.Printf("\033[31m MakeBlock / ApplyStateTransitionFromBlock\033[0m %d ms\n", elapsed/1000)
			}
			return proposedBlk, newStateDB, nil
		} else {
			//waiting for block ... potentially submit ticket here
		}
	}
	return nil, nil, nil
}

func (s *StateDB) SetID(id uint16) {
	s.Id = id
	s.JamState.SafroleState.Id = id
}

func (s *StateDB) ApplyStateTransitionPreimages(preimages []types.Preimages, targetJCE uint32) (uint32, uint32, error) {
	num_preimages := uint32(0)
	num_octets := uint32(0)

	//TODO: (eq 156) need to make sure E_P is sorted. by what??
	//validate (eq 156)
	for i := 1; i < len(preimages); i++ {
		if preimages[i].Requester <= preimages[i-1].Requester {
			return 0, 0, fmt.Errorf(errServiceIndices)
		}
	}

	for _, l := range preimages {
		// validate eq 157
		_, err := s.ValidateLookup(&l)
		if err != nil {
			return 0, 0, err
		}
	}

	// ready for state transisiton
	// t := s.GetTrie()
	for _, l := range preimages {
		// (eq 158)
		// δ†[s]p[H(p)] = p
		// δ†[s]l[H(p),∣p∣] = [τ′]
		// t.SetPreImageBlob(l.Service_Index(), l.Blob)
		// t.SetPreImageLookup(l.Service_Index(), l.BlobHash(), l.BlobLength(), []uint32{targetJCE})
		fmt.Printf(("WriteServicePreimageBlob, Service_Index: %d, Blob: %x\n"), l.Service_Index(), l.Blob)
		s.WriteServicePreimageBlob(l.Service_Index(), l.Blob)
		s.WriteServicePreimageLookup(l.Service_Index(), l.BlobHash(), l.BlobLength(), []uint32{targetJCE})
		num_preimages++
		num_octets += l.BlobLength()
	}

	return num_preimages, num_octets, nil
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
	a, err := types.ServiceAccountFromBytes(c, v)
	if err != nil {
		return &types.ServiceAccount{}, false, nil
	}
	//William check here!
	//fmt.Printf("getServiceAccount s=%v, v=%v\n", c, a.String())
	return a, false, nil
}

func (s *StateDB) getPreimageBlob(c uint32, codeHash common.Hash) ([]byte, error) {
	t := s.GetTrie()
	preimage_blob, err := t.GetPreImageBlob(c, codeHash)
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
	output, err := types.Encode(results)
	if err != nil {
		return []byte{}
	}
	return output
}

// Section 8.2 - Eq 85+86
func (s *StateDB) ApplyStateTransitionAuthorizations(guarantees []types.Guarantee) error {
	for c := uint16(0); c < types.TotalCores; c++ {
		if len(guarantees) > 0 {
			// build m, holding the set of authorization hashes matching the core c in guarantees
			m := make(map[common.Hash]bool)
			hits := 0
			for _, g := range guarantees {
				if g.Report.CoreIndex == c {
					m[g.Report.AuthorizerHash] = true
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
func (s *StateDB) ApplyStateTransitionRho(disputes types.Dispute, assurances []types.Assurance, guarantees []types.Guarantee, targetJCE uint32) (uint32, uint32, error) {

	// (25) / (111) We clear any work-reports which we judged as uncertain or invalid from their core
	d := s.GetJamState()
	//apply the dispute
	var err error
	result, err := d.IsValidateDispute(&disputes)
	if err != nil {
		return 0, 0, err
	}
	//state changing here
	//cores reading the old jam state
	//ρ†
	d.ProcessDispute(result, disputes.Culprit, disputes.Fault)
	if err != nil {
		return 0, 0, err
	}

	err = s.ValidateAssurances(assurances)
	if err != nil {
		return 0, 0, err
	}

	// Assurances: get the bitstring from the availability
	// core's data is now available
	//ρ††
	num_assurances, availableWorkReport := d.ProcessAssurances(assurances)
	_ = availableWorkReport                     // availableWorkReport is the work report that is available for the core, will be used in the audit section
	s.AvailableWorkReport = availableWorkReport // every block has new available work report

	if debugA {
		fmt.Printf("Rho State Update - Assurances\n")
		for i, rho := range s.JamState.AvailabilityAssignments {
			if rho == nil {
				fmt.Printf("Rho core[%d] WorkPackage Hash: nil\n", i)
			} else {
				fmt.Printf("Rho core[%d] WorkPackage Hash: %v\n", i, rho.WorkReport.GetWorkPackageHash())
			}
		}
	}

	// Guarantees
	err = s.ValidateGuarantees(guarantees)
	if err != nil {
		return 0, 0, err
	}
	num_reports := uint32(len(guarantees))

	d.ProcessGuarantees(guarantees)
	if debug {
		fmt.Printf("Rho State Update - Guarantees\n")
		for i, rho := range s.JamState.AvailabilityAssignments {
			if rho == nil {
				fmt.Printf("Rho core[%d] WorkPackage Hash: nil\n", i)
			} else {
				fmt.Printf("Rho core[%d] WorkPackage Hash: %v\n", i, rho.WorkReport.GetWorkPackageHash())
			}
		}
	}

	return num_reports, num_assurances, nil
}

// given previous safrole, applt state transition using block
// σ'≡Υ(σ,B)
func ApplyStateTransitionFromBlock(oldState *StateDB, ctx context.Context, blk *types.Block) (s *StateDB, err error) {
	start := time.Now()
	s = oldState.Copy()
	s.JamState = oldState.JamState.Copy()
	s.Block = blk
	s.ParentHash = blk.Header.Parent
	s.RemoveUnusedTickets()
	targetJCE := blk.TimeSlot()
	if debug {
		fmt.Printf("ApplyStateTransitionFromBlock blk.Hash()=%v s.StateRoot=%v\n", blk.Hash(), s.StateRoot)
	}
	// 17+18 -- takes the PREVIOUS accumulationRoot which summarizes C a set of (service, result) pairs and
	// appends "n" to MMR "Beta" s.JamState.RecentBlocks
	s.ApplyStateRecentHistory(blk, &(s.accumulationRoot))

	// 19-22 - Safrole last
	ticketExts := blk.Tickets()
	sf_header := blk.GetHeader()
	epochMark := blk.EpochMark()

	if epochMark != nil {
		s.knownTickets = make(map[common.Hash]uint8) //keep track of known tickets
		// s.queuedTickets = make(map[common.Hash]types.Ticket)
		s.GetJamState().ResetTallyStatistics()
	}
	sf := s.GetSafrole()
	s2, err := sf.ApplyStateTransitionTickets(ticketExts, targetJCE, sf_header)
	if err != nil {
		fmt.Printf("sf.ApplyStateTransitionFromBlock %v\n", err)
		panic(1)
		return s, err
	}
	s.JamState.SafroleState = &s2
	//fmt.Printf("ApplyStateTransitionFromBlock - SafroleState \n")
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "tickets", uint32(len(ticketExts)))
	elapsed := time.Since(start).Microseconds()
	if trace && elapsed > 1000000 { // OPTIMIZED ApplyStateTransitionTickets/ValidateProposedTicket
		fmt.Printf("\033[31mApplyStateTransitionFromBlock:Tickets\033[0m %d ms\n", elapsed/1000)
	}

	// 24 - Preimages
	preimages := blk.PreimageLookups()
	num_preimage, num_octets, err := s.ApplyStateTransitionPreimages(preimages, targetJCE)
	if err != nil {
		return s, err
	}
	//fmt.Printf("ApplyStateTransitionFromBlock - Preimages\n")
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "preimages", num_preimage)
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "octets", num_octets)
	// 23,25-27 Disputes, Assurances. Guarantees
	disputes := blk.Disputes()
	assurances := blk.Assurances()
	guarantees := blk.Guarantees()

	if debug {
		for _, g := range guarantees {
			fmt.Printf("[Core: %d]ApplyStateTransitionFromBlock Guarantee W_Hash%v\n", g.Report.CoreIndex, g.Report.GetWorkPackageHash())
		}
	}

	num_reports, num_assurances, err := s.ApplyStateTransitionRho(disputes, assurances, guarantees, targetJCE)
	if err != nil {
		return s, err
	}
	if debug {
		fmt.Printf("ApplyStateTransitionFromBlock - Disputes, Assurances, Guarantees\n")
		for _, rho := range s.JamState.AvailabilityAssignments {
			if rho != nil {
				fmt.Printf("ApplyStateTransitionFromBlock - Rho core[%d] WorkPackage Hash: %v\n", rho.WorkReport.CoreIndex, rho.WorkReport.GetWorkPackageHash())
			}
		}
	}
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "assurances", num_assurances)
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "reports", num_reports)

	// 28 -- ACCUMULATE
	var g uint64
	o := s.JamState.newPartialState()
	var f map[uint32]uint32
	var b []BeefyCommitment
	accumulate_input_wr := s.AvailableWorkReport
	accumulate_input_wr = s.AccumulatableSequence(accumulate_input_wr)
	g, _, b = s.OuterAccumulate(g, accumulate_input_wr, &o, f)
	if debug {
		fmt.Printf("ApplyStateTransitionFromBlock - Accumulate\n")
	}
	s.ApplyXContext(&o)
	// n.r = M_B( [ s \ E_4(s) ++ E(h) | (s,h) in C] , H_K)
	var leaves [][]byte
	for _, sa := range b {
		// put (s,h) of C  into leaves
		leaf := append(common.Uint32ToBytes(sa.Service), sa.Commitment.Bytes()...)
		leaves = append(leaves, leaf)
	}
	tree := trie.NewWellBalancedTree(leaves, types.Keccak)
	s.accumulationRoot = common.Hash(tree.Root())

	// 29 -  Update Authorization Pool alpha'
	err = s.ApplyStateTransitionAuthorizations(blk.Guarantees())
	if err != nil {
		return s, err
	}
	if debug {
		fmt.Printf("ApplyStateTransitionFromBlock - Authorizations\n")
	}
	// 30 - compute pi
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "blocks", 1)
	if debug {
		fmt.Printf("ApplyStateTransitionFromBlock - Blocks\n")
	}

	err = s.OnTransfer()
	if err != nil {
		return s, err
	}
	if debug {
		fmt.Printf("ApplyStateTransitionFromBlock - OnTransfer\n")
	}
	s.Block = blk
	s.ParentHash = s.BlockHash
	s.BlockHash = blk.Hash()
	s.StateRoot = s.UpdateTrieState()

	//State transisiton is successful.  Remove E(T,P,A,G,D) from statedb queue
	s.RemoveExtrinsics(ticketExts, preimages, guarantees, assurances, disputes)
	if debug {
		fmt.Printf("Queue Tickets Length: %v\n", len(s.queuedTickets))
	}

	elapsed = time.Since(start).Microseconds()
	if trace && elapsed > 3000000 {
		fmt.Printf("\033[31m ApplyStateTransitionFromBlock:TOTAL \033[0m %d ms\n", elapsed/1000)
	}
	return s, nil
}

func (s *StateDB) GetBlock() *types.Block {
	return s.Block
}

func (s *StateDB) isCorrectCodeHash(workReport types.WorkReport) bool {
	// TODO: logic to validate the code hash prediction.
	return true
}

// make block generate block prior to state execution
func (s *StateDB) MakeBlock(credential types.ValidatorSecret, targetJCE uint32) (bl *types.Block, err error) {

	sf := s.GetSafrole()
	isNewEpoch := sf.IsNewEpoch(targetJCE)
	needWinningMarker := sf.IseWinningMarkerNeeded(targetJCE)
	stateRoot := s.GetStateRoot()

	//fmt.Printf("[N%v] Original JamState %v\n", s.Id, s.GetJamSnapshot())
	//fmt.Printf("[N%v] MakeBlock using stateRoot %v\n", s.Id, stateRoot)
	s.RecoverJamState(stateRoot)
	//fmt.Printf("[N%v] Recovered JamState %v\n", s.Id, s.GetJamSnapshot())

	b := types.NewBlock()
	h := types.NewBlockHeader()
	extrinsicData := types.NewExtrinsic()
	h.Parent = s.BlockHash
	h.ParentStateRoot = stateRoot
	h.Slot = targetJCE
	// Extrinsic Data has 5 different Extrinsics
	s.RemoveUnusedTickets()

	// E_P - Preimages:  aggregate queuedPreimageLookups into extrinsicData.Preimages
	extrinsicData.Preimages = make([]types.Preimages, 0)

	// Make sure this Preimages is ready to be included..

	for _, preimageLookup := range s.queuedPreimageLookups {
		_, err := s.ValidateLookup(&preimageLookup)
		if err == nil {
			//fmt.Printf("Preimage ready: %v\n", preimageLookup.String())
			pl, err := preimageLookup.DeepCopy()
			if err != nil {
				continue
			}
			extrinsicData.Preimages = append(extrinsicData.Preimages, pl)
		}
	}

	// 156: These pairs must be ordered and without duplicates
	for i := 0; i < len(extrinsicData.Preimages); i++ {
		for j := 0; j < len(extrinsicData.Preimages)-1; j++ {
			if extrinsicData.Preimages[j].Requester > extrinsicData.Preimages[j+1].Requester {
				extrinsicData.Preimages[j], extrinsicData.Preimages[j+1] = extrinsicData.Preimages[j+1], extrinsicData.Preimages[j]
			}
		}
	}
	s.queuedPreimageLookups = make(map[common.Hash]types.Preimages)

	// E_A - Assurances  aggregate queuedAssurances into extrinsicData.Assurances
	extrinsicData.Assurances = make([]types.Assurance, 0)
	for i, assurance := range s.queuedAssurances {
		a, err := assurance.DeepCopy()
		if err != nil {
			continue
		}
		// 125 - The assurances must all be anchored on the parent
		err = s.VerifyAssurance(a)
		if err != nil {
			fmt.Println("Error verifying assurance: ", err)
			// remove the assurance from the queue
			delete(s.queuedAssurances, i)
			continue
		}
		err = CheckDuplicate(extrinsicData.Assurances, a)
		if err != nil {
			fmt.Println("Error checking duplicate assurance: ", err)
			continue
		}
		extrinsicData.Assurances = append(extrinsicData.Assurances, a)
	}
	// 126 - The assurances must ordered by validator index
	SortAssurances(extrinsicData.Assurances)
	s.queuedAssurances = make(map[common.Hash]types.Assurance)
	tmpState := s.JamState.Copy()
	_, _ = tmpState.ProcessAssurances(extrinsicData.Assurances)
	// E_G - Guarantees: aggregate queuedGuarantees into extrinsicData.Guarantees
	extrinsicData.Guarantees = make([]types.Guarantee, 0)
	for _, guarantee := range s.queuedGuarantees {
		g, err := guarantee.DeepCopy()
		if err != nil {
			continue
		}

		err = s.Verify_Guarantee(g)
		if err != nil {
			fmt.Println("Error verifying guarantee: ", err)
			continue
		}
		//142 check pending report
		err = tmpState.CheckReportTimeOut(guarantee, s.GetTimeslot())
		if err != nil {
			fmt.Println("Error checking report timeout: ", err)
			continue
		}
		err = tmpState.CheckReportPendingOnCore(g)
		if err != nil {
			fmt.Println("Error checking report pending on core: ", err)
			continue
		}
		err = CheckCoreIndex(extrinsicData.Guarantees, g)
		if err != nil {
			fmt.Println("Error checking core index: ", err)
			continue
		}
		extrinsicData.Guarantees = append(extrinsicData.Guarantees, g)
		if debugG {
			fmt.Printf("[N%d] Include Guarantee (Package Hash : %v)\n", s.Id, g.Report.GetWorkPackageHash())
		}
		// check guarantee one per core
		// check guarantee is not a duplicate

	}
	SortByCoreIndex(extrinsicData.Guarantees)
	// return duplicate guarantee err
	err = s.CheckGuaranteesWorkReport(extrinsicData.Guarantees)
	if err != nil {
		return nil, err
	}

	s.queuedGuarantees = make(map[common.Hash]types.Guarantee)

	// E_D - Disputes: aggregate queuedDisputes into extrinsicData.Disputes
	// d := s.GetJamState()

	// extrinsicData.Disputes = make([]types.Dispute, 0)
	for _, v := range s.queueJudgements {
		// TODO: use votes to make dispute d
		if v.Judge {

		}
		// extrinsicData.Disputes = append(extrinsicData.Disputes, d)
	}
	s.queueJudgements = make(map[common.Hash]types.Judgement)
	// dispute := FormDispute(s.queuedVotes)
	// if d.NeedsOffendersMarker(&dispute) {
	// 	// Handle the case where the dispute does not need an offenders marker.
	// 	OffendMark, err := d.GetOffenderMark(dispute)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	h.OffendersMark = OffendMark.OffenderKey
	// }

	// TODO: 103 Verdicts v must be ordered by report hash.
	// TODO: 104 Offender signatures c and f must each be ordered by the validator’s Ed25519 key.
	// TODO: 105 There may be no duplicate report hashes within the extrinsic, nor amongst any past reported hashes.
	// target_Epoch, target_Phase := sf.EpochAndPhase(targetJCE)
	needEpochMarker := isNewEpoch
	// eq 71
	if needEpochMarker {
		epochMarker := sf.GenerateEpochMarker()
		//a tuple of the epoch randomness and a sequence of Bandersnatch keys defining the Bandersnatch valida- tor keys (kb) beginning in the next epoch
		if debug {
			fmt.Printf("[N%d] *** \033[32mEpochMarker\033[0m %v\n", s.Id, epochMarker)
		}
		h.EpochMark = epochMarker
	}
	// eq 72
	if needWinningMarker {
		winningMarker, err := sf.GenerateWinningMarker()
		//block is the first after the end of the submission period for tickets and if the ticket accumulator is saturated
		if err == nil {
			if debug {
				fmt.Printf("[N%d] *** \033[32mWinningTicketMarker\033[0m #Tickets=%d targetJCE=%v\n", s.Id, len(winningMarker), targetJCE)
			}
			h.TicketsMark = winningMarker
		}
	} else {
		// If there's new ticketID, add them into extrinsic
		// Question: can we submit tickets at the exact tail end block?
		extrinsicData.Tickets = make([]types.Ticket, 0)
		// add the limitation for receiving tickets
		if s.JamState.SafroleState.IsTicketSubmissionClosed(targetJCE) && !isNewEpoch {
			// s.queuedTickets = make(map[common.Hash]types.Ticket)

		} else {
			next_n2 := s.JamState.SafroleState.GetNextN2()
			for _, ticket := range s.queuedTickets[next_n2] {
				t, err := ticket.DeepCopy()
				if err != nil {
					continue
				}
				ticketID, _ := t.TicketID()
				if s.JamState.SafroleState.InTicketAccumulator(ticketID) {
					continue
				} else if len(extrinsicData.Tickets) >= types.MaxTicketsPerExtrinsic {
					// we only allow a maxium number of tickets per block
					continue
				} else {
					// fmt.Printf("[N%d] GetTicketQueue %v => %v\n", s.Id, ticketID, t)
					extrinsicData.Tickets = append(extrinsicData.Tickets, t)
				}
			}

			// s.queuedTickets = make(map[common.Hash]types.Ticket)
		}
	}

	h.ExtrinsicHash = extrinsicData.Hash()
	author_index, err := sf.GetAuthorIndex(credential.BandersnatchPub.Hash(), "Curr")
	if err != nil {
		return bl, err
	}
	h.AuthorIndex = author_index
	b.Extrinsic = extrinsicData

	unsignHeaderHash := h.UnsignedHash()

	//signing

	auth_secret_key, err := sf.ConvertBanderSnatchSecret(credential.BandersnatchSecret)
	if err != nil {
		return bl, err
	}

	epochType := sf.CheckEpochType()
	if epochType == "fallback" {
		blockseal, fresh_vrfSig, err := sf.SignFallBack(auth_secret_key, unsignHeaderHash)
		if err != nil {
			return bl, err
		}
		copy(h.Seal[:], blockseal[:])
		copy(h.EntropySource[:], fresh_vrfSig[:])
	} else {
		attempt, err := sf.GetBindedAttempt(targetJCE)
		blockseal, fresh_vrfSig, err := sf.SignPrimary(auth_secret_key, unsignHeaderHash, attempt)
		if err != nil {
			return bl, err
		}
		copy(h.Seal[:], blockseal[:])
		copy(h.EntropySource[:], fresh_vrfSig[:])
	}
	b.Header = *h
	return b, nil
}
