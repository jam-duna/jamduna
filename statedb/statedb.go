package statedb

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"math"
	"os"
	"sort"

	bandersnatch "github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	storage "github.com/colorfulnotion/jam/storage"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	trie "github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

const (
	debugB = "beefy_mod"

	blockAuthoringChaos = false // turn off for production (or publication of traces)
)

type StateDB struct {
	Finalized               bool
	Id                      uint16       `json:"id"`
	Block                   *types.Block `json:"block"`
	ParentHeaderHash        common.Hash  `json:"parentHeaderHash"`
	HeaderHash              common.Hash  `json:"headerHash"`
	StateRoot               common.Hash  `json:"stateRoot"`
	JamState                *JamState    `json:"Jamstate"`
	sdb                     *storage.StateDBStorage
	trie                    *trie.MerkleTree
	posteriorSafroleEntropy *SafroleState // used to manage entropy, validator, and winning ticket

	// used in ApplyStateRecentHistory between statedbs
	Authoring string
	X         *types.XContext

	GuarantorAssignments         []types.GuarantorAssignment
	PreviousGuarantorAssignments []types.GuarantorAssignment
	AvailableWorkReport          []types.WorkReport // every block has its own available work report

	stateUpdate *types.StateUpdate
	logChan     chan storage.LogMessage

	ElapsedMicrosecondsValidation uint32

	AncestorSet       map[common.Hash]uint32               `json:"ancestorSet"` // AncestorSet is a set of block headers which include the recent 24 hrs of blocks
	BlockServicesCost map[uint32]*telemetry.AccumulateCost `json:"blockServicesCost"`
}

func (s *StateDB) MarshalJSON() ([]byte, error) {
	ancestorSet := make(map[string]uint32)
	for k, v := range s.AncestorSet {
		ancestorSet[k.Hex()] = v
	}

	type Alias StateDB
	return json.Marshal(&struct {
		*Alias
		AncestorSet map[string]uint32 `json:"ancestorSet"`
	}{
		Alias:       (*Alias)(s),
		AncestorSet: ancestorSet,
	})
}

func (s *StateDB) UnmarshalJSON(data []byte) error {
	type Alias StateDB
	aux := &struct {
		AncestorSet map[string]uint32 `json:"ancestorSet"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	s.AncestorSet = make(map[common.Hash]uint32)
	for k, v := range aux.AncestorSet {
		s.AncestorSet[common.HexToHash(k)] = v
	}

	return nil
}

func (s *StateDB) ProcessIncomingJudgement(j types.Judgement) {
	// get the disputes state

}

func (s *StateDB) CheckIncomingAssurance(a *types.Assurance) (err error) {
	cred := s.GetSafrole().GetCurrValidator(int(a.ValidatorIndex))
	err = a.VerifySignature(cred)
	if err != nil {
		log.Error(log.SDB, "CheckIncomingAssurance: Invalid Assurance", "err", err)
		return
	}
	return nil
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
	errServiceIndices         = "serviceIndices duplicated or not ordered"
	errPreimageLookupNotSet   = "preimagelookup (h,l) not set"
	errPreimageLookupNotEmpty = "preimagelookup not empty"
	errPreimageBlobSet        = "preimageBlob already set"
)

// ValidateAddPreimage checks that the
func (s *StateDB) ValidateAddPreimage(serviceID uint32, blob []byte) (common.Hash, error) {
	l := &types.Preimages{
		Requester: serviceID,
		Blob:      blob,
	}
	// check 157 - (1) a_p not equal to P (2) a_l is empty
	preimageHash := common.Blake2Hash(blob)
	t := s.GetTrie()
	anchors, ok, err := t.GetPreImageLookup(l.Service_Index(), l.Hash(), l.BlobLength())
	if err != nil {
		log.Warn(log.SDB, "[ValidateAddPreimage:GetPreImageLookup] anchor not set", "err", err, "s", l.Service_Index(), "blob hash", l.Hash(), "blob length", l.BlobLength())
		return common.Hash{}, fmt.Errorf("%s", errPreimageLookupNotSet) //TODO: differentiate key not found vs leveldb error
	} else if !ok {
		log.Warn(log.SDB, "[ValidateAddPreimage:GetPreImageLookup] Can't find the anchor", "s", l.Service_Index(), "blob hash", l.Hash(), "blob length", l.BlobLength())
		return common.Hash{}, fmt.Errorf("%s", errPreimageLookupNotSet) //TODO: differentiate key not found vs leveldb error
	}
	if len(anchors) == 1 { // we have to forget it -- check!
		return common.Hash{}, errors.New(errPreimageLookupNotEmpty)
	}
	return preimageHash, nil
}
func newEmptyStateDB(sdb *storage.StateDBStorage) (statedb *StateDB) {
	statedb = new(StateDB)
	statedb.SetStorage(sdb)
	statedb.trie = trie.NewMerkleTree(nil, sdb)
	statedb.logChan = make(chan storage.LogMessage, 100)
	return statedb
}

func NewCleanStateDB(sdb *storage.StateDBStorage, id uint16) *StateDB {
	statedb := new(StateDB)
	statedb.SetStorage(sdb)
	statedb.JamState = NewJamState() // Need to create JamState before SetID
	statedb.SetID(id)
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
	C16 = "AccumulationOutputs"
)

var StateKeyMap = map[byte]string{
	0x01: "c1",
	0x02: "c2",
	0x03: "c3",
	0x04: "c4",
	0x05: "c5",
	0x06: "c6",
	0x07: "c7",
	0x08: "c8",
	0x09: "c9",
	0x0A: "c10",
	0x0B: "c11",
	0x0C: "c12",
	0x0D: "c13",
	0x0E: "c14",
	0x0F: "c15",
	0x10: "c16",
}

// Initial services
const (
	BootstrapServiceCode  = 0
	BootstrapServiceFile  = "/services/bootstrap/bootstrap.pvm"
	BootStrapNullAuthFile = "/services/null_authorizer/null_authorizer.pvm"

	AlgoServiceCode = 10
	AlgoServiceFile = "/services/algo/algo.pvm"

	AuthCopyServiceCode = 20
	AuthCopyServiceFile = "/services/auth_copy/auth_copy.pvm"

	EVMServiceCode = 35
	EVMServiceFile = "/services/evm/evm.pvm"
)

func RequiresBackendGo(s uint32) bool {
	return s == EVMServiceCode
}

func (s *StateDB) GetHeaderHash() common.Hash {
	return s.Block.Header.Hash()
}

func (s *StateDB) GetStateRoot() common.Hash {
	return s.StateRoot
}

func (s *StateDB) GetParentStateRoot() common.Hash {
	// this is "root" before trie gets flushed
	return s.StateRoot
}

func (s *StateDB) GetTentativeStateRoot() common.Hash {
	// return the trie root at the moment
	t := s.GetTrie()
	return t.GetRoot()
}

func (s *StateDB) SetTrie(t *trie.MerkleTree) {
	s.trie = t
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

func (s *StateDB) GetStateUpdates() *types.StateUpdate {
	return s.stateUpdate
}

func (s *StateDB) SetJamState(jamState *JamState) {
	s.JamState = jamState
}
func (s *StateDB) RecoverJamState(stateRoot common.Hash) error {
	// Now read C1.....C15 from the trie and put it back into JamState
	t, err := s.CopyTrieState(stateRoot)
	if err != nil {
		return err
	}

	coreAuthPoolEncode, err := t.GetState(C1)
	if err != nil {
		log.Crit(log.SDB, "Error reading C1 CoreAuthPool from trie", err)
	}
	authQueueEncode, err := t.GetState(C2)
	if err != nil {
		log.Crit(log.SDB, "Error reading C2 AuthQueue from trie: %v\n", err)
	}
	recentBlocksEncode, err := t.GetState(C3)
	if err != nil {
		log.Crit(log.SDB, "Error reading C3 RecentBlocks from trie: %v\n", err)
	}
	safroleStateEncode, err := t.GetState(C4)
	if err != nil {
		log.Crit(log.SDB, "Error reading C4 SafroleState from trie: %v\n", err)
	}
	disputeStateEncode, err := t.GetState(C5)
	if err != nil {
		log.Crit(log.SDB, "Error reading C5 DisputeState from trie: %v\n", err)
	}
	entropyEncode, err := t.GetState(C6)
	if err != nil {
		log.Crit(log.SDB, "Error reading C6 Entropy from trie: %v\n", err)
	}
	DesignedEpochValidatorsEncode, err := t.GetState(C7)
	if err != nil {
		log.Crit(log.SDB, "Error reading C7 NextEpochValidators from trie: %v\n", err)
	}
	currEpochValidatorsEncode, err := t.GetState(C8)
	if err != nil {
		log.Crit(log.SDB, "Error reading C8 CurrentEpochValidators from trie: %v\n", err)
	}
	priorEpochValidatorEncode, err := t.GetState(C9)
	if err != nil {
		log.Crit(log.SDB, "Error reading C9 PriorEpochValidators from trie: %v\n", err)
	}
	availability_assignmentEncode, err := t.GetState(C10)
	if err != nil {
		log.Crit(log.SDB, "Error reading C10 AvailabilityAssignments from trie: %v\n", err)
	}
	mostRecentBlockTimeSlotEncode, err := t.GetState(C11)
	if err != nil {
		log.Crit(log.SDB, "Error reading C11 MostRecentBlockTimeSlot from trie: %v\n", err)
	}
	privilegedServiceIndicesEncode, err := t.GetState(C12)
	if err != nil {
		log.Crit(log.SDB, "Error reading C12 PrivilegedServiceIndices from trie: %v\n", err)
	}
	piEncode, err := t.GetState(C13)
	if err != nil {
		log.Crit(log.SDB, "Error reading C13 ActiveValidator from trie: %v\n", err)
	}
	accumulateQueueEncode, err := t.GetState(C14)
	if err != nil {
		log.Crit(log.SDB, "Error reading C14 accunulateQueue from trie: %v\n", err)
	}
	accumulateHistoryEncode, err := t.GetState(C15)
	if err != nil {
		log.Crit(log.SDB, "Error reading C15 accunulateHistory from trie: %v\n", err)
	}
	accumulateOutputsEncode, err := t.GetState(C16)
	if err != nil {
		log.Crit(log.SDB, "Error reading C16 accunulateOutputs from trie: %v\n", err)
	}

	//Decode(authQueueEncode) -> AuthorizationQueue
	//set AuthorizationQueue back to JamState

	d := s.GetJamState()
	d.SetAuthPool(coreAuthPoolEncode)
	d.SetAuthQueue(authQueueEncode)
	d.SetRecentBlocks(recentBlocksEncode)
	d.SetSafroleState(safroleStateEncode)
	d.SetDisputesState(disputeStateEncode)
	d.SetEntropy(entropyEncode)
	d.SetDesignatedValidators(DesignedEpochValidatorsEncode)
	d.SetCurrEpochValidators(currEpochValidatorsEncode)
	d.SetPriorEpochValidators(priorEpochValidatorEncode)
	d.SetMostRecentBlockTimeSlot(mostRecentBlockTimeSlotEncode)
	d.SetAvailabilityAssignments(availability_assignmentEncode)

	d.SetPrivilegedServicesIndices(privilegedServiceIndicesEncode)
	d.SetPi(piEncode)
	d.SetAccumulateQueue(accumulateQueueEncode)
	d.SetAccumulateHistory(accumulateHistoryEncode)
	d.SetAccumulateOutputs(accumulateOutputsEncode)
	s.SetJamState(d)

	// Because we have safrolestate as internal state, JamState is NOT enough.
	d.SafroleState.NextEpochTicketsAccumulator = d.SafroleBasicState.TicketAccumulator   // γa: Ticket accumulator for the next epoch (epoch N+1) DONE
	d.SafroleState.TicketsVerifierKey = d.SafroleBasicState.RingCommitment               // γz: Epoch's root, a Bandersnatch ring root composed with one Bandersnatch key of each of the next epoch's validators (epoch N+1)
	d.SafroleState.TicketsOrKeys = d.SafroleBasicState.SlotSealerSeries                  // γs: Current epoch's slot-sealer series (epoch N)
	d.SafroleState.NextValidators = types.Validators(d.SafroleBasicState.NextValidators) // γk: Next epoch's validators (epoch N+1)

	// Update the trie and state root to point to the recovered state
	s.SetTrie(t)
	s.StateRoot = t.GetRoot()
	if s.StateRoot.Hex() != stateRoot.Hex() {
		return fmt.Errorf("StateRoot mismatch after recovery: actual=%s, expected=%s", s.StateRoot.Hex(), stateRoot.Hex())
	}
	return nil
}

func (s *StateDB) UpdateTrieState() common.Hash {
	//γ ≡⎩γk, γz, γs, γa⎭
	//γk :one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	//γz :epoch’s root, a Bandersnatch ring root composed with the one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	//γa :the ticket accumulator, a series of highest-scoring ticket identifiers to be used for the next epoch (epoch N+1)
	//γs :current epoch’s slot-sealer series, which is either a full complement of E tickets or, in the case of a fallback mode, a series of E Bandersnatch keys (epoch N)

	sf := s.GetSafrole()
	if sf == nil {
		log.Crit(log.SDB, "UpdateTrieState: NO SAFROLE")
	}
	//benchRec.Add("- UpdateTrieState:GetSafrole", time.Since(t0))

	t0 := time.Now()
	sb := sf.GetSafroleBasicState()
	benchRec.Add("- UpdateTrieState:GetSafroleBasicState", time.Since(t0))

	t0 = time.Now()
	safroleStateEncode := sb.GetSafroleStateBytes()
	entropyEncode := sf.GetEntropyBytes()
	nextnextEpochValidatorsEncode := sf.GetNextNextEpochValidatorsBytes()
	currEpochValidatorsEncode := sf.GetCurrEpochValidatorsBytes()
	priorEpochValidatorEncode := sf.GetPriorEpochValidatorsBytes()
	mostRecentBlockTimeSlotEncode := sf.GetMostRecentBlockTimeSlotBytes()
	d := s.GetJamState()
	disputeState := d.GetDisputesStateBytes()
	availability_assignmentEncode := d.GetAvailabilityAssignmentsBytes()
	piEncode := d.GetPiBytes()
	coreAuthPoolEncode := d.GetAuthPoolBytes()
	authQueueEncode := d.GetAuthQueueBytes()
	privilegedServiceIndicesEncode := d.GetPrivilegedServicesIndicesBytes()
	recentBlocksEncode := d.GetRecentBlocksBytes()
	accumulateQueueEncode := d.GetAccumulationQueueBytes()
	accumulateHistoryEncode := d.GetAccumulationHistoryBytes()
	accumulateOutputsEncode := d.GetAccumulationOutputsBytes()
	benchRec.Add("- UpdateTrieState:Codec", time.Since(t0))

	t := s.GetTrie()

	t0 = time.Now()
	t.SetState(C1, coreAuthPoolEncode)
	t.SetState(C2, authQueueEncode)
	t.SetState(C3, recentBlocksEncode)
	t.SetState(C4, safroleStateEncode)
	t.SetState(C5, disputeState)
	t.SetState(C6, entropyEncode)
	t.SetState(C7, nextnextEpochValidatorsEncode)
	t.SetState(C8, currEpochValidatorsEncode)
	t.SetState(C9, priorEpochValidatorEncode)
	t.SetState(C10, availability_assignmentEncode)
	t.SetState(C11, mostRecentBlockTimeSlotEncode)
	t.SetState(C12, privilegedServiceIndicesEncode)
	t.SetState(C13, piEncode)
	t.SetState(C14, accumulateQueueEncode)
	t.SetState(C15, accumulateHistoryEncode)
	t.SetState(C16, accumulateOutputsEncode)
	benchRec.Add("- UpdateTrieState:SetState", time.Since(t0))

	updated_root := t.GetRoot()

	t0 = time.Now()
	verify := false
	if verify {
		t2, _ := trie.InitMerkleTreeFromHash(updated_root.Bytes(), s.sdb)
		checkingResult, err := CheckingAllState(t, t2)
		if !checkingResult || err != nil {
			log.Crit(log.SDB, "CheckingAllState", "err", err)
		}
		benchRec.Add("- UpdateTrieState:Verify", time.Since(t0))
	}
	return updated_root
}

// THIS DOES A FULL SCAN OF THE TRIE AND IS SLOW
func (s *StateDB) GetAllKeyValues() []KeyVal {
	startKey := common.Hex2Bytes("0x0000000000000000000000000000000000000000000000000000000000000000")
	endKey := common.Hex2Bytes("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	maxSize := uint32(math.MaxUint32)
	t, err := s.CopyTrieState(s.StateRoot)
	if err != nil {
		log.Crit(log.SDB, "GetAllKeyValues: failed to copy trie state", "error", err)
		return []KeyVal{}
	}
	foundKeyVal, _, _ := t.GetStateByRange(startKey, endKey, maxSize)

	tmpKeyVals := make([]KeyVal, 0)
	for _, keyValue := range foundKeyVal {
		fetchRealKey := t.GetRealKey(keyValue.Key, keyValue.Value)
		realValue := make([]byte, len(keyValue.Value))
		var realKey [31]byte
		copy(realKey[:], fetchRealKey)
		copy(realValue, keyValue.Value)
		keyVal := KeyVal{
			Key:   realKey,
			Value: realValue,
		}
		//fmt.Printf("~~~key: %x, v: %x,\n", keyVal.Key, keyVal.Value)
		tmpKeyVals = append(tmpKeyVals, keyVal)
	}

	sortedKeyVals := sortKeyValsByKey(tmpKeyVals)
	return sortedKeyVals
}

func sortKeyValsByKey(tmpKeyVals []KeyVal) []KeyVal {
	sort.Slice(tmpKeyVals, func(i, j int) bool {
		return bytes.Compare(tmpKeyVals[i].Key[:], tmpKeyVals[j].Key[:]) < 0
	})
	return tmpKeyVals
}

func (s *StateDB) CompareStateRoot(genesis []KeyVal, parentStateRoot common.Hash) (bool, error) {
	parent_root := s.StateRoot
	newTrie := trie.NewMerkleTree(nil, s.sdb)
	for _, kv := range genesis {
		newTrie.SetRawKeyVal(kv.Key, kv.Value)
	}
	new_root := newTrie.GetRoot()
	if !common.CompareBytes(parent_root[:], new_root[:]) {
		return false, fmt.Errorf("roots are not the same")
	}

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
		log.Crit(log.SDB, "UpdateAllTrieState:ReadFile", "genesis", genesis, "err", err)
		return common.Hash{}
	}
	snapshotRaw := StateSnapshotRaw{}
	json.Unmarshal(snapshotBytesRaw, &snapshotRaw)

	t := s.GetTrie()
	verify := true

	for _, kv := range snapshotRaw.KeyVals {
		t.SetRawKeyVal(kv.Key, kv.Value)
	}
	updated_root := t.GetRoot()

	sf := s.GetSafrole()
	if sf == nil {
		log.Crit(log.SDB, "UpdateAllTrieState:GetSafrole")
	}

	if verify {
		t2, _ := trie.InitMerkleTreeFromHash(updated_root.Bytes(), s.sdb)
		checkingResult, err := CheckingAllState(t, t2)
		if !checkingResult || err != nil {
			log.Crit(log.SDB, "UpdateAllTrieState:CheckingAllState", "err", err)
		}
	}
	return updated_root
}

func (s *StateDB) UpdateAllTrieKeyVals(skv StateKeyVals) common.Hash {
	for _, kv := range skv.KeyVals {
		s.trie.SetRawKeyVal(kv.Key, kv.Value)
	}
	return s.trie.GetRoot()
}

func (s *StateDB) UpdateAllTrieStateRaw(snapshotRaw StateSnapshotRaw) common.Hash {
	for _, kv := range snapshotRaw.KeyVals {
		s.trie.SetRawKeyVal(kv.Key, kv.Value)
	}
	return s.trie.GetRoot()
}

func (s *StateDB) GetSafroleState() *SafroleState {
	return s.JamState.SafroleState
}

func CheckingAllState(t *trie.MerkleTree, t2 *trie.MerkleTree) (bool, error) {
	c1a, _ := t.GetState(C1)
	c1b, _ := t2.GetState(C1)
	if !common.CompareBytes(c1a, c1b) {
		log.Error(log.SDB, "CheckingAllState: C1 is not the same")
		return false, fmt.Errorf("C1 is not the same")
	}
	c2a, _ := t.GetState(C2)
	c2b, _ := t2.GetState(C2)
	if !common.CompareBytes(c2a, c2b) {
		log.Error(log.SDB, "CheckingAllState: C2 is not the same")
		return false, fmt.Errorf("C2 is not the same")
	}
	c3a, _ := t.GetState(C3)
	c3b, _ := t2.GetState(C3)
	if !common.CompareBytes(c3a, c3b) {
		log.Error(log.SDB, "CheckingAllState: C3 is not the same")
		return false, fmt.Errorf("C3 is not the same")
	}
	c4a, _ := t.GetState(C4)
	c4b, _ := t2.GetState(C4)
	if !common.CompareBytes(c4a, c4b) {
		log.Error(log.SDB, "CheckingAllState: C4 is not the same")
		return false, fmt.Errorf("C4 is not the same")
	}
	c5a, _ := t.GetState(C5)
	c5b, _ := t2.GetState(C5)
	if !common.CompareBytes(c5a, c5b) {
		log.Error(log.SDB, "CheckingAllState: C5 is not the same")
		return false, fmt.Errorf("C5 is not the same")
	}
	c6a, _ := t.GetState(C6)
	c6b, _ := t2.GetState(C6)
	if !common.CompareBytes(c6a, c6b) {
		log.Error(log.SDB, "CheckingAllState: C6 is not the same")
		return false, fmt.Errorf("C6 is not the same")
	}
	c7a, _ := t.GetState(C7)
	c7b, _ := t2.GetState(C7)
	if !common.CompareBytes(c7a, c7b) {
		log.Error(log.SDB, "CheckingAllState: C7 is not the same")
		return false, fmt.Errorf("C7 is not the same")
	}
	c8a, _ := t.GetState(C8)
	c8b, _ := t2.GetState(C8)
	if !common.CompareBytes(c8a, c8b) {
		log.Error(log.SDB, "CheckingAllState: C8 is not the same")
		return false, fmt.Errorf("C8 is not the same")
	}
	c9a, _ := t.GetState(C9)
	c9b, _ := t2.GetState(C9)
	if !common.CompareBytes(c9a, c9b) {
		log.Error(log.SDB, "CheckingAllState: C9 is not the same")
		return false, fmt.Errorf("C9 is not the same")
	}
	c10a, _ := t.GetState(C10)
	c10b, _ := t2.GetState(C10)
	if !common.CompareBytes(c10a, c10b) {
		log.Error(log.SDB, "CheckingAllState: C10 is not the same")
		return false, fmt.Errorf("C10 is not the same")
	}
	c11a, _ := t.GetState(C11)
	c11b, _ := t2.GetState(C11)
	if !common.CompareBytes(c11a, c11b) {
		log.Error(log.SDB, "CheckingAllState: C11 is not the same")
		return false, fmt.Errorf("C11 is not the same")
	}
	c12a, _ := t.GetState(C12)
	c12b, _ := t2.GetState(C12)
	if !common.CompareBytes(c12a, c12b) {
		log.Error(log.SDB, "CheckingAllState: C12 is not the same")
		return false, fmt.Errorf("C12 is not the same")
	}
	c13a, _ := t.GetState(C13)
	c13b, _ := t2.GetState(C13)
	if !common.CompareBytes(c13a, c13b) {
		log.Error(log.SDB, "CheckingAllState: C13 is not the same")
		return false, fmt.Errorf("C13 is not the same")
	}
	c14a, _ := t.GetState(C14)
	c14b, _ := t2.GetState(C14)
	if !common.CompareBytes(c14a, c14b) {
		log.Error(log.SDB, "CheckingAllState: C14 is not the same")
		return false, fmt.Errorf("C14 is not the same")
	}
	c15a, _ := t.GetState(C15)
	c15b, _ := t2.GetState(C15)
	if !common.CompareBytes(c15a, c15b) {
		log.Error(log.SDB, "CheckingAllState: C15 is not the same")
		return false, fmt.Errorf("C15 is not the same")
	}
	return true, nil
}

func (s *StateDB) String() string {
	return types.ToJSON(s)
}

func DumpStateDBKeyValues(db *StateDB, description string, nodeID uint16, showDump bool) {
	if !showDump {
		return
	}

	kvList := db.GetAllKeyValues()
	stateRoot := db.GetStateRoot()

	var kvDump strings.Builder

	var timeslot uint64 = 0
	for _, kv := range kvList {
		if len(kv.Key) >= 2 && kv.Key[0] == 0x0B && kv.Key[1] == 0x00 {
			timeslot = types.DecodeE_l(kv.Value)
			fmt.Printf("decoded timeslot: %v\n", timeslot)
			break
		}
	}

	kvDump.WriteString(fmt.Sprintf("\n[N%d][Slot=%d] ===== %s %d key-values (Root:%v)=====\n", nodeID, timeslot, description, len(kvList), stateRoot))

	var c13Value []byte
	for i, kv := range kvList {
		valHash := common.Blake2Hash(kv.Value)
		kvDump.WriteString(fmt.Sprintf("[N%d][Slot=%d][Key %d][ValHash] 0x%x -> %s Len=%d\n",
			nodeID, timeslot, i, kv.Key, valHash.String_shortLen(4), len(kv.Value)))

		// Capture C13 (ValidatorStatistics) value if found (key 0x0d00)
		if len(kv.Key) >= 2 && kv.Key[0] == 0x0d && kv.Key[1] == 0x00 {
			c13Value = kv.Value
		}
	}

	kvDump.WriteString(fmt.Sprintf("[N%d][Slot=%d] ===== End of %s key-values =====\n", nodeID, timeslot, description))

	// Decode and print C13 (ValidatorStatistics) if found
	c13Debug := false
	if len(c13Value) > 0 && c13Debug {
		var validatorStats types.ValidatorStatistics
		decoded, _, err := types.Decode(c13Value, reflect.TypeOf(validatorStats))
		if err == nil && decoded != nil {
			validatorStats = decoded.(types.ValidatorStatistics)
			c13JSON, jsonErr := json.MarshalIndent(validatorStats, "", "  ")
			if jsonErr == nil {
				kvDump.WriteString(fmt.Sprintf("\n[N%d][Slot=%d] ===== C13 ValidatorStatistics JSON =====\n", nodeID, timeslot))
				kvDump.WriteString(string(c13JSON))
				kvDump.WriteString(fmt.Sprintf("\n[N%d][Slot=%d] ===== End C13 ValidatorStatistics JSON =====\n", nodeID, timeslot))
			}
		}
	}
	fmt.Print(kvDump.String())
}

func NewStateDBFromBlock(sdb *storage.StateDBStorage, block *types.Block) (statedb *StateDB, err error) {
	statedb = newEmptyStateDB(sdb)
	statedb.Finalized = false
	statedb.trie = trie.NewMerkleTree(nil, sdb)
	statedb.JamState = NewJamState()
	statedb.Block = block
	statedb.ParentHeaderHash = block.Header.ParentHeaderHash
	statedb.StateRoot = block.Header.ParentStateRoot
	if err := statedb.RecoverJamState(statedb.StateRoot); err != nil {
		return nil, fmt.Errorf("failed to recover state for block %s: %w", block.Header.Hash().Hex(), err)
	}
	// Because we have safrolestate as internal state, JamState is NOT enough.
	s := statedb.JamState
	s.SafroleState.NextEpochTicketsAccumulator = s.SafroleBasicState.TicketAccumulator   // γa: Ticket accumulator for the next epoch (epoch N+1) DONE
	s.SafroleState.TicketsVerifierKey = s.SafroleBasicState.RingCommitment               // γz: Epoch’s root, a Bandersnatch ring root composed with one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	s.SafroleState.TicketsOrKeys = s.SafroleBasicState.SlotSealerSeries                  // γs: Current epoch’s slot-sealer series (epoch N)
	s.SafroleState.NextValidators = types.Validators(s.SafroleBasicState.NextValidators) // γk: Next epoch’s validators (epoch N+1)
	return statedb, nil
}

func NewStateDB(sdb *storage.StateDBStorage, blockHash common.Hash) (statedb *StateDB, err error) {
	return newStateDB(sdb, blockHash)
}

func NewStateDBFromStateRoot(stateRoot common.Hash, sdb *storage.StateDBStorage) (recoveredStateDB *StateDB, err error) {
	recoveredStateDB = newEmptyStateDB(sdb)
	recoveredStateDB.Finalized = true // Historical state is always finalized
	//recoveredStateDB.StateRoot = stateRoot
	recoveredStateDB.JamState = NewJamState()

	err = recoveredStateDB.RecoverJamState(stateRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to recover state from root %s: %w", stateRoot.Hex(), err)
	}

	//DumpStateDBKeyValues(recoveredStateDB, "Recovered", 0, 0)

	// Verify that recovery succeeded by checking if trie was initialized
	if recoveredStateDB.trie == nil {
		return nil, fmt.Errorf("failed to initialize merkle tree from state root %s", stateRoot.Hex())
	}

	return recoveredStateDB, nil
}

// newStateDB initiates the StateDB using the blockHash+bn; the bn input must refer to the epoch for which the blockHash belongs to
func newStateDB(sdb *storage.StateDBStorage, blockHash common.Hash) (statedb *StateDB, err error) {
	statedb = newEmptyStateDB(sdb)
	statedb.Finalized = false
	statedb.trie = trie.NewMerkleTree(nil, sdb)

	// TODO: MK this potentially need a JCE to be passed in
	statedb.JamState = NewJamState()

	block := types.Block{}
	b := make([]byte, 32)
	zeroHash := common.BytesToHash(b)
	if bytes.Equal(blockHash.Bytes(), zeroHash.Bytes()) {
		// genesis block situation

	} else {
		encodedBlock, err := sdb.ReadKV(blockHash)
		if err != nil {
			return statedb, err
		}

		h := common.Blake2Hash(encodedBlock)
		if !bytes.Equal(h.Bytes(), blockHash.Bytes()) {
			return statedb, fmt.Errorf("[statedb:newStateDB] hash of data incorrect [%d bytes]", len(encodedBlock))
		}
		if err := json.Unmarshal(encodedBlock, &block); err != nil {
			return statedb, fmt.Errorf("[statedb:newStateDB] JSON decode error: %v", err)
		}
		statedb.Block = &block
		statedb.ParentHeaderHash = block.Header.ParentHeaderHash
	}

	return statedb, nil
}

func (s *StateDB) CopyTrieState(stateRoot common.Hash) (*trie.MerkleTree, error) {
	t, err := trie.InitMerkleTreeFromHash(stateRoot.Bytes(), s.sdb)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize trie from state root %s: %w", stateRoot.Hex(), err)
	}
	return t, nil
}

// Copy generates a copy of the StateDB
func (s *StateDB) Copy() (newStateDB *StateDB) {
	// Create a new instance of StateDB
	// T.P.G.A.
	tmpAvailableWorkReport := make([]types.WorkReport, len(s.AvailableWorkReport))
	copy(tmpAvailableWorkReport, s.AvailableWorkReport)

	copiedTrie, err := s.CopyTrieState(s.StateRoot)
	if err != nil {
		log.Crit(log.SDB, "Copy: failed to copy trie state", "error", err)
		return nil
	}

	newStateDB = &StateDB{
		Id:                  s.Id,
		Block:               s.Block.Copy(), // You might need to deep copy the Block if it's mutable
		ParentHeaderHash:    s.ParentHeaderHash,
		HeaderHash:          s.HeaderHash,
		StateRoot:           s.StateRoot,
		JamState:            s.JamState.Copy(), // DisputesState has a Copy method
		sdb:                 s.sdb,
		trie:                copiedTrie,
		logChan:             make(chan storage.LogMessage, 100),
		AvailableWorkReport: tmpAvailableWorkReport,
		AncestorSet:         s.AncestorSet, // TODO: CHECK why we have this in CheckStateTransition
		Authoring:           s.Authoring,
		/*
			Following flds are not copied over..?

			VMs       map[uint32]*VM
			vmMutex   sync.Mutex
			X 		  XContext
			S 		  uint32

		*/
	}
	// copy instead of recalculate
	newStateDB.RotateGuarantors()
	return newStateDB
}

func GenerateEpochPhaseTraceID(epoch uint32, phase uint32) string {
	var traceIDBytes [16]byte
	binary.LittleEndian.PutUint32(traceIDBytes[0:4], epoch)
	binary.LittleEndian.PutUint32(traceIDBytes[4:8], phase)
	return hex.EncodeToString(traceIDBytes[:])
}

func (s *StateDB) ProcessState(ctx context.Context, currJCE uint32, credential types.ValidatorSecret, ticketIDs []common.Hash, extrinsic_pool *types.ExtrinsicPool, pvmBackend string) (isAuthorizedBlockRefiner bool, blk *types.Block, sdb *StateDB, err error) {
	genesisReady := s.JamState.SafroleState.CheckFirstPhaseReady(currJCE)
	if !genesisReady {
		//log.Warn(log.SDB, "ProcessState:GenesisNotReady", "currJCE", currJCE)
		return false, nil, nil, nil
	}
	targetJCE, timeSlotReady := s.JamState.SafroleState.CheckTimeSlotReady(currJCE)
	if timeSlotReady {
		// Time to propose block if authorized
		sf0, err := s.GetPosteriorSafroleEntropy(targetJCE) // always be hit
		if err != nil {
			return false, nil, nil, err
		}
		isAuthorizedBlockRefiner, ticketID, _, _ := sf0.IsAuthorizedBuilder(targetJCE, common.Hash(credential.BandersnatchPub), ticketIDs)
		currEpoch, currPhase := s.JamState.SafroleState.EpochAndPhase(targetJCE)
		if isAuthorizedBlockRefiner {
			telemetryClient := s.sdb.GetTelemetryClient()
			// Telemetry: Authoring (event 40) - Block authoring begins
			authoringEventID := telemetryClient.GetEventID(s.HeaderHash)
			telemetryClient.Authoring(targetJCE, s.HeaderHash)

			proposedBlk, err := s.MakeBlock(ctx, credential, targetJCE, ticketID, extrinsic_pool)
			if err != nil {
				// Telemetry: AuthoringFailed (event 41) - Block authoring failed
				telemetryClient.AuthoringFailed(authoringEventID, err.Error())
				log.Error(log.SDB, "ProcessState:MakeBlock", "author", s.Id, "currJCE", currJCE, "e'", currEpoch, "m'", currPhase, "err", err)
				return true, nil, nil, err
			}
			log.Info(log.SDB, "Proposed Block", "authoring", s.Authoring)

			// Telemetry: Authored (event 42) - Block has been authored
			// Create BlockOutline from the proposed block
			blockBytes := proposedBlk.Bytes()
			preimages := proposedBlk.PreimageLookups()

			// Calculate total preimage size
			var preimagesSizeInBytes uint32
			for _, preimage := range preimages {
				preimagesSizeInBytes += uint32(len(preimage.Blob))
			}

			// Count dispute verdicts (Disputes() returns a single Dispute, not a slice)
			dispute := proposedBlk.Disputes()
			numDisputeVerdicts := uint32(len(dispute.Verdict))

			blockOutline := telemetry.BlockOutline{
				SizeInBytes:          uint32(len(blockBytes)),
				HeaderHash:           proposedBlk.Header.Hash(),
				NumTickets:           uint32(len(proposedBlk.Tickets())),
				NumPreimages:         uint32(len(preimages)),
				PreimagesSizeInBytes: preimagesSizeInBytes,
				NumGuarantees:        uint32(len(proposedBlk.Guarantees())),
				NumAssurances:        uint32(len(proposedBlk.Assurances())),
				NumDisputeVerdicts:   numDisputeVerdicts,
			}
			telemetryClient.Authored(authoringEventID, blockOutline)

			if blockAuthoringChaos {
				if noAuthoring := SimulateBlockAuthoringInterruption(proposedBlk); noAuthoring {
					return true, nil, nil, fmt.Errorf("simulated Interruption: Block @ %v not proposed", currJCE)
				}
			}

			var used_entropy common.Hash // to avoid jump epoch
			if proposedBlk.EpochMark() != nil {
				used_entropy = proposedBlk.EpochMark().TicketsEntropy
			} else {
				used_entropy = s.GetSafrole().Entropy[2]
			}
			valid_tickets := extrinsic_pool.GetTicketIDPairFromPool(used_entropy)
			newStateDB, err := ApplyStateTransitionFromBlock(authoringEventID, s, ctx, proposedBlk, valid_tickets, pvmBackend) // shawn to check.. valid_tickets was nil here before
			if err != nil {
				// Telemetry: BlockExecutionFailed (event 46) - Block execution failed after authoring
				telemetryClient.BlockExecutionFailed(authoringEventID, err.Error())
				log.Error(log.SDB, "ProcessState:ApplyStateTransitionFromBlock", "s.ID", s.Id, "currJCE", currJCE, "e'", currEpoch, "m'", currPhase, "err", err)
				return true, nil, nil, err
			}
			mode := "safrole"
			if sf0.GetEpochTWithPhase(targetJCE) == 0 {
				mode = "fallback"
			}
			log.Info(log.SDB, "Authored Block",
				"mode", mode,
				"AUTHOR", s.Id,
				"p", common.Str(proposedBlk.GetParentHeaderHash()),
				//"s", common.Str(proposedBlk.Header.ParentStateRoot),
				"s+", newStateDB.StateRoot.String_short(),
				"h", common.Str(proposedBlk.Header.Hash()),
				"e'", currEpoch,
				"m'", currPhase,
				"len(γ_a')", len(newStateDB.JamState.SafroleState.NextEpochTicketsAccumulator),
				"blk", proposedBlk.Str())
			return true, proposedBlk, newStateDB, nil
		}
		log.Debug(log.B, "ProcessState:NotAuthorizedBlockRefiner timeSlotReady", "currJCE", currJCE, "targetJCE", targetJCE, "credential", credential.BandersnatchPub.Hash(), "ticket", len(ticketIDs), "isAuthorizedBlockRefiner", isAuthorizedBlockRefiner)
		return false, nil, nil, nil
	}
	//waiting for block ... potentially submit ticket here
	log.Debug(log.B, "ProcessState:NotAuthorizedBlockRefiner", "currJCE", currJCE, "targetJCE", targetJCE, "credential", credential.BandersnatchPub.Hash(), "ticketLen", len(ticketIDs))
	return false, nil, nil, nil
}

func (s *StateDB) GetID() uint16 {
	return s.Id
}

func (s *StateDB) SetID(id uint16) {
	s.Id = id
	s.JamState.SafroleState.Id = id
}

func (s *StateDB) WriteServiceStorage(service uint32, k []byte, v []byte) {
	tree := s.GetTrie()
	tree.SetServiceStorage(service, k, v)
}

func (s *StateDB) WriteServicePreimageBlob(service uint32, blob []byte) {
	tree := s.GetTrie()
	tree.SetPreImageBlob(service, blob)
}
func (s *StateDB) WriteServicePreimageLookup(service uint32, blob_hash common.Hash, blob_length uint32, time_slots []uint32) {
	tree := s.GetTrie()
	tree.SetPreImageLookup(service, blob_hash, blob_length, time_slots)
}
func (s *StateDB) DeleteServicePreimageKey(service uint32, blob_hash common.Hash) error {
	tree := s.GetTrie()
	err := tree.DeletePreImageBlob(service, blob_hash)
	if err != nil {
		log.Error(log.SDB, "DeleteServicePreimageKey:DeletePreImageBlob", "blob_hash", blob_hash, "err", err)
		return err
	}
	return nil
}

// 1 bring back AccountPreimageHash for use in extrinsic pool maps
// 2 ONLY do ValidateAddPreimage at the VERY END (MakeBlock and ApplyStateTransitionPreimages)

func (s *StateDB) ApplyStateTransitionPreimages(preimages []types.Preimages, targetJCE uint32) (uint32, uint32, error) {
	num_preimages := uint32(0)
	num_octets := uint32(0)

	//(12.39) EP sort by serviceID & blob byte sequence
	for i := 1; i < len(preimages); i++ {
		curr := preimages[i]
		prev := preimages[i-1]
		if curr.Requester < prev.Requester {
			return 0, 0, fmt.Errorf(errServiceIndices)
		} else if curr.Requester == prev.Requester {
			// If Requester is the same, compare Blob by byte sequence
			if bytes.Compare(curr.Blob, prev.Blob) <= 0 {
				return 0, 0, fmt.Errorf(errServiceIndices)
			}
		}
	}

	// (12.42)
	for _, l := range preimages {
		_, err := s.ValidateAddPreimage(l.Requester, l.Blob)
		if err != nil {
			log.Error(log.SDB, "ApplyStateTransitionPreimages:ValidateAddPreimage", "n", s.Id, "err", err)
			return 0, 0, err
		}
	}

	// (12.43) ready for state transition
	for _, l := range preimages {
		// δ†[s]p[H(p)] = p
		// δ†[s]l[H(p),∣p∣] = [τ′]
		log.Trace(log.P, "WriteServicePreimageBlob", "Service_Index", l.Service_Index(), "Blob", l.Blob)
		s.WriteServicePreimageBlob(l.Service_Index(), l.Blob)
		s.WriteServicePreimageLookup(l.Service_Index(), l.Hash(), l.BlobLength(), []uint32{targetJCE})
		num_preimages++
		num_octets += l.BlobLength()
	}

	return num_preimages, num_octets, nil
}

func (s *StateDB) GetBlock() *types.Block {
	return s.Block
}

// SealBlockMaterial holds all intermediate values for debug, auditing, or external verification.
type SealBlockMaterial struct {
	BlockAuthorPub  string `json:"bandersnatch_pub"`
	BlockAuthorPriv string `json:"bandersnatch_priv"` // never store real priv keys in production!
	TicketID        string `json:"ticket_id"`
	Attempt         uint8  `json:"attempt"`

	// We store intermediate VRF inputs: cForHs is c used for H_s; mForHs is the message used for H_s
	// cForHv is c used for H_v; mForHv is the message used for H_v (often empty).
	CForHs string `json:"c_for_H_s"`
	MForHs string `json:"m_for_H_s"`
	Hs     string `json:"H_s"`

	CForHv string `json:"c_for_H_v"`
	MForHv string `json:"m_for_H_v"`
	Hv     string `json:"H_v"`

	// We also save some block info.
	Entropy3    string `json:"eta3"`
	T           uint8  `json:"T"`
	HeaderBytes string `json:"header_bytes"`
}

func (s *StateDB) VerifyBlockHeader(bl *types.Block, sf0 *SafroleState) (isValid bool, validatorIdx uint16, ietf_pub bandersnatch.BanderSnatchKey, verificationErr error) {
	targetJCE := bl.TimeSlot()
	h := bl.GetHeader()
	validatorIdx = h.AuthorIndex
	var err error

	if sf0 == nil {
		// ValidateTicketTransition
		sf0, err = ApplyStateTransitionTickets(s, context.TODO(), bl, nil)
		if err != nil {
			log.Error(log.SDB, "ApplyStateTransitionTickets", "err", err)
			return false, validatorIdx, bandersnatch.BanderSnatchKey{}, fmt.Errorf("VerifyBlockHeader Failed: ApplyStateTransitionTickets")
		}
	}

	// author_idx is the K' so we use the sf_tmp
	signing_validator := sf0.GetCurrValidator(int(validatorIdx))
	block_author_ietf_pub := bandersnatch.BanderSnatchKey(signing_validator.GetBandersnatchKey())

	// compute c within (6.15) & (6.16)
	blockSealEntropy := sf0.Entropy[3] // Use entropy[3] for VRF input

	var c []byte

	if sf0.GetEpochTWithPhase(targetJCE) == 1 {
		// Safrole
		_, currPhase := sf0.EpochAndPhase(targetJCE)
		winning_ticket := (sf0.TicketsOrKeys.Tickets)[currPhase]
		c = ticketSealVRFInput(blockSealEntropy, uint8(winning_ticket.Attempt))
	} else {
		// Fallback
		_, currPhase := sf0.EpochAndPhase(targetJCE)
		currentValidatorKey := (sf0.TicketsOrKeys.Keys)[currPhase]
		if !bytes.Equal(currentValidatorKey.Bytes(), block_author_ietf_pub.Bytes()) {
			fmt.Printf("=== VALIDATOR KEY MISMATCH ===\n")
			fmt.Printf("block_author_ietf_pub (received): %v\n", block_author_ietf_pub)
			fmt.Printf("currentValidatorKey (expected):   %v\n", currentValidatorKey)
			fmt.Printf("currPhase: %d\n", currPhase)
			fmt.Printf("=== Validator List ===\n")
			for i, v := range sf0.TicketsOrKeys.Keys {
				marker := "   "
				if i == int(currPhase) {
					marker = ">>>"
				}
				fmt.Printf("%s validator %d : %v\n", marker, i, v)
			}
			return false, validatorIdx, block_author_ietf_pub, fmt.Errorf("VerifyBlockHeader Failed: FallbackMode ValidatorKeyMismatch")
		}
		c = append([]byte(types.X_F), blockSealEntropy.Bytes()...)
	}

	// H_s Verification (6.15/6.16)
	H_s := h.Seal[:]
	m := h.BytesWithoutSig()
	vrfOutput, err := bandersnatch.IetfVrfVerify(block_author_ietf_pub, H_s, c, m)
	if err != nil {
		log.Error(log.SDB, "IetfVrfVerify", "err", err)
		log.Error(log.SDB, "IetfVrfVerify",
			"H_s", common.BytesToHexStr(H_s),
			"c", common.BytesToHexStr(c),
			"m", common.BytesToHexStr(m),
			"block_author_ietf_pub", common.BytesToHexStr(block_author_ietf_pub[:]))
		return false, validatorIdx, block_author_ietf_pub, fmt.Errorf("VerifyBlockHeader Failed: H_s Verification")
	}

	// H_v Verification (6.17)
	H_v := h.EntropySource[:]
	c = append([]byte(types.X_E), vrfOutput...)
	_, err = bandersnatch.IetfVrfVerify(block_author_ietf_pub, H_v, c, []byte{})
	if err != nil {
		log.Error(log.SDB, "IetfVrfVerify", "err", err)
		return false, validatorIdx, block_author_ietf_pub, fmt.Errorf("VerifyBlockHeader Failed: H_v Verification")
	}

	extrinsicHash := bl.Header.ExtrinsicHash
	if !reflect.DeepEqual(extrinsicHash, bl.Extrinsic.Hash()) {
		log.Error(log.SDB, "VerifyBlockHeader:ExtrinsicHashMismatch",
			"extrinsicHash", common.BytesToHexStr(extrinsicHash[:]),
			"bl.Extrinsic.Hash()", common.BytesToHexStr(bl.Extrinsic.Hash()),
			"block_author_ietf_pub", common.BytesToHexStr(block_author_ietf_pub[:]),
			"validatorIdx", validatorIdx)
		log.Error(log.SDB, "VerifyBlockHeader:ExtrinsicHashMismatch",
			"guarantees[0]", bl.Extrinsic.Guarantees[0].String())
		return false, validatorIdx, block_author_ietf_pub, fmt.Errorf("VerifyBlockHeader Failed: ExtrinsicHash Mismatch")
	}

	return true, validatorIdx, block_author_ietf_pub, nil
}

func (s *StateDB) SealBlockWithEntropy(blockAuthorPub bandersnatch.BanderSnatchKey, blockAuthorPriv bandersnatch.BanderSnatchSecret, validatorIdx uint16, targetJCE uint32, originalBlock *types.Block) (*types.Block, error) {
	newBlock := originalBlock.Copy()
	header := newBlock.GetHeader()
	header.ExtrinsicHash = newBlock.Extrinsic.Hash()
	//fmt.Printf("!!SealBlockWithEntropy: targetJCE %d, authorIdx %d, authorPub %x | extrinsicHash %x\n", targetJCE, validatorIdx, blockAuthorPub[:], header.ExtrinsicHash[:])

	// Validate ticket transition
	sf0, err := s.GetPosteriorSafroleEntropy(targetJCE)
	if err != nil {
		log.Error(log.SDB, "GetPosteriorSafroleEntropy", "err", err)
		return nil, fmt.Errorf("SealBlockWithEntropy Failed: GetPosteriorSafroleEntropy")
	}
	// Prepare a container to store all intermediate values for debugging / auditing

	blockSealEntropy := sf0.Entropy[3]
	if sf0.GetEpochTWithPhase(targetJCE) == 1 {
		_, currPhase := sf0.EpochAndPhase(targetJCE)
		winningTicket := sf0.TicketsOrKeys.Tickets[currPhase]
		ticketID := winningTicket.Id

		// H_v generation (primary) 6.17
		c := append([]byte(types.X_E), ticketID.Bytes()...)
		H_v, _, err := bandersnatch.IetfVrfSign(blockAuthorPriv, c, []byte{})
		if err != nil {
			return nil, fmt.Errorf("error generating H_v for primary epoch: %w", err)
		}
		copy(header.EntropySource[:], H_v[:])
		log.Trace(log.SDB, "IETF SIGN 1 H_v", "k", blockAuthorPriv[:], "c", c, "header.EntropySource", header.EntropySource[:])

		// H_s generation (primary) 6.15
		c = append(append([]byte(types.X_T), blockSealEntropy.Bytes()...), byte(uint8(winningTicket.Attempt)&0xF))
		m := header.BytesWithoutSig()
		H_s, _, err := bandersnatch.IetfVrfSign(blockAuthorPriv, c, m)
		if err != nil {
			return nil, fmt.Errorf("error generating H_s for primary epoch: %w", err)
		}
		copy(header.Seal[:], H_s[:])
		log.Trace(log.SDB, "IETF SIGN H_s", "k", blockAuthorPriv[:], "c", c, header.BytesWithoutSig(), "header.Seal", header.Seal[:])

	} else {
		// Y(H_s) generation with an *INCOMPLETE* header because it is missing H_v
		c := append([]byte(types.X_F), blockSealEntropy.Bytes()...)
		_, vrfOutput, err := bandersnatch.IetfVrfSign(blockAuthorPriv, c, header.BytesWithoutSig())
		if err != nil {
			return nil, fmt.Errorf("error generating H_s for fallback epoch: %w", err)
		}

		// H_v generation (fallback) 6.17 -- note that vrfOutput above is used
		cHv := append([]byte(types.X_E), vrfOutput...)
		H_v, _, err := bandersnatch.IetfVrfSign(blockAuthorPriv, cHv, []byte{})
		if err != nil {
			return nil, fmt.Errorf("error generating H_v for fallback epoch: %w", err)
		}
		copy(header.EntropySource[:], H_v[:])

		// H_s generation (fallback) 6.16
		m := header.BytesWithoutSig()
		H_s, _, err := bandersnatch.IetfVrfSign(blockAuthorPriv, c, m)
		if err != nil {
			return nil, fmt.Errorf("error generating H_s for fallback epoch: %w", err)
		}
		copy(header.Seal[:], H_s[:])

	}

	newBlock.Header = header
	return newBlock, nil
}

// Make sure ticketID "x_t ++ n3' ++ attempt" match the one in TicketsOrKeys. Fallback has no ticketID to check.
func (s *StateDB) ValidateVRFSealInput(ticketID common.Hash, targetJCE uint32) (bool, error) {

	// ValidateTicketTransition
	sf0, err := s.GetPosteriorSafroleEntropy(targetJCE)
	if err != nil {
		log.Error(log.SDB, "GetPosteriorSafroleEntropy", "err", err)
		return false, fmt.Errorf("ValidateVRFSealInput Failed: GetPosteriorSafroleEntropy")
	}
	if sf0.GetEpochTWithPhase(targetJCE) == 0 {
		return true, nil
	}

	_, targetPhase := sf0.EpochAndPhase(targetJCE)
	winning_ticket := (sf0.TicketsOrKeys.Tickets)[targetPhase]
	expectedTicketID := winning_ticket.Id

	if expectedTicketID != ticketID {
		return false, fmt.Errorf("[%v] Ticket Mismatch! Expected=%v | Actual=%v", targetJCE, expectedTicketID, ticketID)
	}
	return true, nil
}

func (s *StateDB) GetAncestorTimeSlot() []uint32 {
	timeslots := make([]uint32, 0)
	for _, t := range s.AncestorSet {
		timeslots = append(timeslots, t)
	}
	sort.Slice(timeslots, func(i, j int) bool {
		return timeslots[i] < timeslots[j]
	})
	return timeslots
}

func (s *StateDB) SetAncestor(blockHeader types.BlockHeader, oldState *StateDB) {
	ancestorSet := oldState.AncestorSet
	if ancestorSet == nil {
		ancestorSet = make(map[common.Hash]uint32)
	}
	ancestorSet[blockHeader.Hash()] = blockHeader.Slot
	s.AncestorSet = ancestorSet
}

func HeaderContains(headers []types.BlockHeader, checkHeader types.BlockHeader) bool {
	for _, h := range headers {
		if h.Hash() == checkHeader.Hash() {
			return true
		}
	}
	return false
}
