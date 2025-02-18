package statedb

import (
	"bytes"
	"context"

	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	VMs                     map[uint32]*pvm.VM
	vmMutex                 sync.Mutex

	// used in ApplyStateRecentHistory between statedbs
	AccumulationRoot common.Hash

	X *types.XContext

	GuarantorAssignments         []types.GuarantorAssignment
	PreviousGuarantorAssignments []types.GuarantorAssignment
	AvailableWorkReport          []types.WorkReport // every block has its own available work report

	logChan chan storage.LogMessage

	AncestorSet map[common.Hash]uint32 `json:"ancestorSet"` // AncestorSet is a set of block headers which include the recent 24 hrs of blocks
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

func (s *StateDB) writeLog(obj interface{}, timeslot uint32) {
	s.sdb.WriteLog(obj, timeslot)
}

func (s *StateDB) ProcessIncomingJudgement(j types.Judgement) {
	// get the disputes state

}

func (s *StateDB) getValidatorCredential() []byte {
	// TODO
	return nil
}

func (s *StateDB) CheckIncomingAssurance(a *types.Assurance) (err error) {
	cred := s.GetSafrole().GetCurrValidator(int(a.ValidatorIndex))
	err = a.VerifySignature(cred)
	if err != nil {
		fmt.Printf("Invalid Assurance. Err=%v\n", err)
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
	debug                     = false
	debugA                    = false
	debugG                    = false
	debugP                    = false
	debugAudit                = false
	debugSeal                 = false
	saveSealBlockMaterial     = false
	debugtrace                = false
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
	preimage_blob, ok, err := t.GetPreImageBlob(l.Service_Index(), l.BlobHash())
	if ok { // key found
		if l.BlobHash() == common.Blake2Hash(preimage_blob) {
			//H(p) = p
			return common.Hash{}, fmt.Errorf(errPreimageBlobSet)
		}
	}

	//fmt.Printf("Validating E_p %v\n",l.String())
	anchors, ok, err := t.GetPreImageLookup(l.Service_Index(), l.BlobHash(), l.BlobLength())
	if err != nil {
		fmt.Printf("Fail at anchor not set, service idx %v, blob hash %v, blob length %v\n", l.Service_Index(), l.BlobHash(), l.BlobLength())
		// va := s.GetAllKeyValues() // ISSUE: this does NOT show 00 but PrintTree does!
		t.PrintAllKeyValues()
		t.PrintTree(t.Root, 0)
		return common.Hash{}, fmt.Errorf(errPreimageLookupNotSet) //TODO: differentiate key not found vs leveldb error
	} else if !ok {
		fmt.Printf("Can't find the anchor, service idx %v, blob hash %v, blob length %v\n", l.Service_Index(), l.BlobHash(), l.BlobLength())
		// va := s.GetAllKeyValues() // ISSUE: this does NOT show 00 but PrintTree does!
		t.PrintAllKeyValues()
		t.PrintTree(t.Root, 0)
		return common.Hash{}, fmt.Errorf(errPreimageLookupNotSet) //TODO: differentiate key not found vs leveldb error
	}
	if len(anchors) == 1 { // we have to forget it -- check!
		return common.Hash{}, fmt.Errorf(errPreimageLookupNotEmpty)
	}
	return a_p, nil
}
func newEmptyStateDB(sdb *storage.StateDBStorage) (statedb *StateDB) {
	statedb = new(StateDB)
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
	BootstrapServiceFile = "/services/bootstrap.pvm"
)

func (s *StateDB) GetHeaderHash() common.Hash {
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

func (s *StateDB) RecoverJamState(stateRoot common.Hash) {
	// Now read C1.....C15 from the trie and put it back into JamState
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
	DesignedEpochValidatorsEncode, err := t.GetState(C7)
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
	d.SetDesignedValidators(DesignedEpochValidatorsEncode)
	d.SetCurrEpochValidators(currEpochValidatorsEncode)
	d.SetPriorEpochValidators(priorEpochValidatorEncode)
	d.SetMostRecentBlockTimeSlot(mostRecentBlockTimeSlotEncode)
	d.SetRho(rhoEncode)

	d.SetPrivilegedServicesIndices(privilegedServiceIndicesEncode)
	d.SetPi(piEncode)
	d.SetAccumulateQueue(accunulateQueueEncode)
	d.SetAccumulateHistory(accunulateHistoryEncode)
	s.SetJamState(d)
	//fmt.Printf("[N%v] RecoverJamState jam state: %s -- safrolestate: %s\n", s.Id, d.String(), d.SafroleState.String())
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
	t := s.CopyTrieState(s.StateRoot)
	foundKeyVal, _, _ := t.GetStateByRange(startKey, endKey, maxSize)

	tmpKeyVals := make([]KeyVal, 0)
	for _, keyValue := range foundKeyVal {
		fetchRealKey := t.GetRealKey(keyValue.Key, keyValue.Value)
		realValue := make([]byte, len(keyValue.Value))
		realKey := make([]byte, 32)
		copy(realKey, fetchRealKey)
		copy(realValue, keyValue.Value)

		metaKey := fmt.Sprintf("meta_%x", realKey)
		metaKeyBytes, err := types.Encode(metaKey)
		if err != nil {
			fmt.Printf("PrintAllKeyValues Encode Error: %v\n", err)
		}
		metaValue := ""
		metaValues := make([]string, 2)
		switch {
		case common.CompareBytes(realKey, common.Hex2Bytes("0x0100000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c1"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0200000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c2"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0300000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c3"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0400000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c4"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0500000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c5"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0600000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c6"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0700000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c7"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0800000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c8"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0900000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c9"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0A00000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c10"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0B00000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c11"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0C00000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c12"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0D00000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c13"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0E00000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c14"
			metaValues[1] = ""

		case common.CompareBytes(realKey, common.Hex2Bytes("0x0F00000000000000000000000000000000000000000000000000000000000000")):
			metaValues[0] = "c15"
			metaValues[1] = ""

		default:
			metaValueBytes, ok, err := t.LevelDBGet(metaKeyBytes)
			if err != nil || !ok {
				fmt.Printf("PrintAllKeyValues levelDBGet Error: %v\n", err)
			}
			if metaValueBytes != nil {
				metaValueDecode, _, err := types.Decode(metaValueBytes, reflect.TypeOf(""))
				if err != nil {
					fmt.Printf("PrintAllKeyValues Decode Error: %v\n", err)
				}
				metaValue = metaValueDecode.(string)
				metaValues = strings.SplitN(metaValue, "|", 2)
				//metaValues[1] = metaValue
				//metaValues = strings.SplitN(metaValue, "|", 2)
			} else {
				metaValues = append(metaValues, "")
				metaValues = append(metaValues, "")
			}
		}

		keyVal := KeyVal{
			Key:        realKey,
			Value:      realValue,
			StructType: metaValues[0],
			Metadata:   metaValues[1],
		}
		tmpKeyVals = append(tmpKeyVals, keyVal)
	}
	sortedKeyVals := sortKeyValsByKey(tmpKeyVals)
	return sortedKeyVals
}

func sortKeyValsByKey(tmpKeyVals []KeyVal) []KeyVal {
	sort.Slice(tmpKeyVals, func(i, j int) bool {
		return bytes.Compare(tmpKeyVals[i].Key, tmpKeyVals[j].Key) < 0
	})
	return tmpKeyVals
}

func (s *StateDB) CompareStateRoot(genesis KeyVals, parentStateRoot common.Hash) (bool, error) {
	parent_root := s.StateRoot
	newTrie := trie.NewMerkleTree(nil, s.sdb)
	for _, kv := range genesis {
		newTrie.SetRawKeyVal(common.Hash(kv.Key), kv.Value)
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
		t.SetRawKeyVal(common.Hash(kv.Key), kv.Value)
		//fmt.Printf("SetRawKeyVal %v %x\n", common.Hash(kv[0]), kv[1])
	}
	updated_root := t.GetRoot()

	sf := s.GetSafrole()
	if sf == nil {
		fmt.Printf("NO SAFROLE %v", s)
		panic(223)
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

func (s *StateDB) UpdateAllTrieStateRaw(snapshotRaw StateSnapshotRaw) common.Hash {
	for _, kv := range snapshotRaw.KeyVals {
		s.trie.SetRawKeyVal(common.Hash(kv.Key), kv.Value)
		if kv.Metadata != "" {
			metaKey := fmt.Sprintf("meta_%x", kv.Key)
			metaKeyBytes, err := types.Encode(metaKey)
			if err != nil {
				fmt.Printf("UpdateAllTrieStateRaw Encode Error: %v\n", err)
			}
			metaData := fmt.Sprintf("%s|%s", kv.StructType, kv.Metadata)
			metaValueBytes, err := types.Encode(metaData)
			if err != nil {
				fmt.Printf("UpdateAllTrieStateRaw Encode Error: %v\n", err)
			}
			s.sdb.WriteRawKV(metaKeyBytes, metaValueBytes)
		}
	}
	// bootStrapCode := common.FromHex("0x000000000000001000000084000000000072051100000005100000000518000000055f04071300040a0400fffe040b24040713000211f8031004031504050000fffe01582004070000fffe04090020040a0010040b0030040c00404e090d0503570404090400fffe04070000fffe040804040a044e03011004011502110813000407130021842a4825050922222a4190945201")
	// bootStrapCodeHash := common.Blake2Hash(bootStrapCode)
	// fmt.Printf("**** Adding s=0, bootStrapCodeHash=%v, len(%v), | bootStrapCode=%x\n", bootStrapCodeHash, len(bootStrapCode), bootStrapCode)

	return s.trie.GetRoot()
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
		statedb.ParentHeaderHash = block.Header.ParentHeaderHash
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
		Id:                  s.Id,
		Block:               s.Block.Copy(), // You might need to deep copy the Block if it's mutable
		ParentHeaderHash:    s.ParentHeaderHash,
		HeaderHash:          s.HeaderHash,
		StateRoot:           s.StateRoot,
		JamState:            s.JamState.Copy(), // DisputesState has a Copy method
		sdb:                 s.sdb,
		trie:                s.CopyTrieState(s.StateRoot),
		logChan:             make(chan storage.LogMessage, 100),
		AccumulationRoot:    s.AccumulationRoot, // compressed C
		AvailableWorkReport: tmpAvailableWorkReport,
		AncestorSet:         s.AncestorSet,
		/*
			Following flds are not copied over..?

			VMs       map[uint32]*pvm.VM
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

func (s *StateDB) ProcessState(credential types.ValidatorSecret, ticketIDs []common.Hash, extrinsic_pool *types.ExtrinsicPool) (*types.Block, *StateDB, error) {
	genesisReady := s.JamState.SafroleState.CheckFirstPhaseReady()
	if !genesisReady {
		return nil, nil, nil
	}
	targetJCE, timeSlotReady := s.JamState.SafroleState.CheckTimeSlotReady()
	if timeSlotReady {
		// Time to propose block if authorized
		sf0 := s.GetPosteriorSafroleEntropy(targetJCE)
		isAuthorizedBlockBuilder, ticketID, _ := sf0.IsAuthorizedBuilder(targetJCE, common.Hash(credential.BandersnatchPub), ticketIDs)
		if isAuthorizedBlockBuilder {
			// Add MakeBlock span
			if s.sdb.SendTrace {
				tracer := s.sdb.Tp.Tracer("NodeTracer")
				// s.InitEpochPhaseContext()
				ctx, span := tracer.Start(context.Background(), fmt.Sprintf("[N%d] ProcessState -> MakeBlock", s.sdb.NodeID))
				s.sdb.UpdateBlockContext(ctx)
				defer span.End()
			}

			start := time.Now()
			proposedBlk, err := s.MakeBlock(credential, targetJCE, ticketID, extrinsic_pool)
			if err != nil {
				fmt.Printf("Error making block: %v\n", err)
				return nil, nil, err
			}
			// Add ApplyStateTransitionFromBlock span
			if s.sdb.Tp != nil && s.sdb.BlockContext != nil && s.sdb.SendTrace {
				tracer := s.sdb.Tp.Tracer("NodeTracer")

				var block_hash common.Hash
				if proposedBlk == nil {
					block_hash = common.Hash{}
				} else {
					block_hash = proposedBlk.Header.Hash()
				}
				tags := trace.WithAttributes(attribute.String("BlockHash", common.Str(block_hash)))
				_, span := tracer.Start(s.sdb.BlockContext, fmt.Sprintf("[N%d] ProcessState -> ApplyStateTransitionFromBlock", s.sdb.NodeID), tags)
				// oldState.sdbs.UpdateBlockContext(ctx)
				defer span.End()
			}

			newStateDB, err := ApplyStateTransitionFromBlock(s, context.Background(), proposedBlk)
			if err != nil {
				// HOW could this happen, we made the block ourselves!
				fmt.Printf("Error applying state transition: %v\n", err)
				return nil, nil, err
			}
			currEpoch, currPhase := s.JamState.SafroleState.EpochAndPhase(targetJCE)
			if sf0.GetEpochT() == 1 {
				fmt.Printf("[N%v] \033[33m Blk %s<-%s \033[0m e'=%d,m'=%02d, len(γ_a')=%d   \t%s %s\n", s.Id, common.Str(proposedBlk.GetParentHeaderHash()), common.Str(proposedBlk.Header.Hash()),
					currEpoch, currPhase, len(newStateDB.JamState.SafroleState.NextEpochTicketsAccumulator), proposedBlk.Str(), newStateDB.JamState.GetValidatorStats())
			} else {
				// change a color
				fmt.Printf("[N%v] \033[32m Blk %s<-%s \033[0m e'=%d,m'=%02d, len(γ_a')=%d   \t%s %s\n", s.Id, common.Str(proposedBlk.GetParentHeaderHash()), common.Str(proposedBlk.Header.Hash()),
					currEpoch, currPhase, len(newStateDB.JamState.SafroleState.NextEpochTicketsAccumulator), proposedBlk.Str(), newStateDB.JamState.GetValidatorStats())
			}

			elapsed := time.Since(start)
			if debugtrace && elapsed > 2000000 {
				fmt.Printf("\033[31m MakeBlock / ApplyStateTransitionFromBlock\033[0m %d ms\n", elapsed/1000)
			}
			return proposedBlk, newStateDB, nil
		} else {
			//waiting for block ... potentially submit ticket here
		}
	}
	return nil, nil, nil
}

// see GP 11.1.2 Refinement Context where there TWO historical blocks A+B but only A has to be in RecentBlocks
func (s *StateDB) GetRefineContext() types.RefineContext {
	// A) ANCHOR -- checkRecentBlock checks if Anchor is in RecentBlocks
	anchor := common.Hash{}
	stateRoot := common.Hash{}
	beefyRoot := common.Hash{}
	if len(s.JamState.RecentBlocks) > 1 {
		anchorBlock := s.JamState.RecentBlocks[len(s.JamState.RecentBlocks)-2]
		//fmt.Printf("GetRefineContext (# RecentBlocks=%d) AnchorBlock: %s\n", len(s.JamState.RecentBlocks), anchorBlock.String())
		anchor = anchorBlock.HeaderHash          // header hash a must be in s.JamState.RecentBlocks
		stateRoot = anchorBlock.StateRoot        // state root s must be in s.JamState.RecentBlocks
		beefyRoot = *(anchorBlock.B.SuperPeak()) // beefy root b must be in s.JamState.RecentBlocks
		//fmt.Printf("  ... Anchor: %s StateRoot: %s BeefyRoot: %s\n", anchor, stateRoot, beefyRoot)
	}

	// B) LOOKUP ANCHOR -- there are NO restrictions here but we choose these to have something
	lookupAnchorBlock := s.Block
	lookupAnchor := lookupAnchorBlock.Header.Hash() // header hash l does NOT have to be in s.JamState.RecentBlocks
	ts := lookupAnchorBlock.GetHeader().Slot
	return types.RefineContext{
		// A) ANCHOR
		Anchor:    anchor,
		StateRoot: stateRoot,
		BeefyRoot: beefyRoot,
		// B) LOOKUP ANCHOR
		LookupAnchor:     lookupAnchor,
		LookupAnchorSlot: ts,
		Prerequisites:    []common.Hash{},
	}
}

func (s *StateDB) SetID(id uint16) {
	s.Id = id
	s.JamState.SafroleState.Id = id
}

// TODO: REMOVE THESE and use service account object methods INSTEAD!
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
		fmt.Printf("DeleteServicePreimageKey: Failed to delete blob_hash: %x, error: %v\n", blob_hash.Bytes(), err)
		return err
	}
	return nil
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
			fmt.Printf("[N%d] ApplyStateTransitionPreimages ValidateLookup Error: %v\n", s.Id, err)
			return 0, 0, err
		}
	}

	// ready for state transisiton
	for _, l := range preimages {
		// (eq 158)
		// δ†[s]p[H(p)] = p
		// δ†[s]l[H(p),∣p∣] = [τ′]
		if debugP {
			fmt.Printf(("WriteServicePreimageBlob, Service_Index: %d, Blob: %x\n"), l.Service_Index(), l.Blob)
		}
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
	v, ok, err := t.GetService(types.ServiceAccountPrefix, c)
	if err != nil || !ok {
		if !ok {
			// fmt.Printf("getServiceAccount: ServiceAccount not found for core %d\n", c)
		}
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
	preimage_blob, ok, err := t.GetPreImageBlob(c, codeHash)
	if err != nil || !ok {
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

// Process Rho - Eq 25/26/27 using disputes, assurances, guarantees in that order
func (s *StateDB) ApplyStateTransitionRho(disputes types.Dispute, assurances []types.Assurance, guarantees []types.Guarantee, targetJCE uint32) (num_reports map[uint16]uint16, num_assurances map[uint16]uint16, err error) {

	// (25) / (111) We clear any work-reports which we judged as uncertain or invalid from their core
	d := s.GetJamState()
	//apply the dispute
	result, err := d.IsValidateDispute(&disputes)
	if err != nil {
		return
	}
	//state changing here
	//cores reading the old jam state
	//ρ†
	d.ProcessDispute(result, disputes.Culprit, disputes.Fault)
	if err != nil {
		return
	}

	// original validate assurances logic (prior to guarantees) -- we cannot do signature checking here ... otherwise it would trigger bad sig
	// for fuzzing to work, we cannot check signature until everything has been properly considered
	// assuranceErr := s.ValidateAssurancesWithSig(assurances)
	// if assuranceErr != nil {
	// 	return 0, 0, err
	// }

	err = s.ValidateAssurancesTransition(assurances)
	if err != nil {
		return
	}

	// Assurances: get the bitstring from the availability
	// core's data is now available
	//ρ††
	num_assurances, availableWorkReport := d.ProcessAssurances(assurances, targetJCE)
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

	// Sort the assurances by validator index
	// sortingErr := CheckSortingEAs(assurances)
	// if sortingErr != nil {
	// 	return 0, 0, sortingErr
	// }

	// Verify each assurance's signature
	// sigErr := s.ValidateAssurancesSig(assurances)
	// if sigErr != nil {
	// 	return 0, 0, sigErr
	// }

	// Guarantees
	err = s.Verify_Guarantees()
	if err != nil {
		return
	}

	num_reports = d.ProcessGuarantees(guarantees)
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
	old_timeslot := s.GetSafrole().Timeslot
	s.JamState = oldState.JamState.Copy()
	s.Block = blk
	s.ParentHeaderHash = blk.Header.ParentHeaderHash
	s.HeaderHash = blk.Header.Hash()
	isValid, _, _, headerErr := s.VerifyBlockHeader(blk)
	if !isValid || headerErr != nil {
		// panic("MK validation check!! Block header is not valid")
		return s, fmt.Errorf("Block header is not valid")
	}

	if debug {
		fmt.Printf("[N%d] ApplyStateTransitionFromBlock (%v <== %v) s.StateRoot=%v\n", s.Id, s.ParentHeaderHash, s.HeaderHash, s.StateRoot)
	}
	targetJCE := blk.TimeSlot()
	// 17+18 -- takes the PREVIOUS accumulationRoot which summarizes C a set of (service, result) pairs and
	// 19-22 - Safrole last
	ticketExts := blk.Tickets()
	sf_header := blk.GetHeader()
	epochMark := blk.EpochMark()

	if epochMark != nil {
		// s.queuedTickets = make(map[common.Hash]types.Ticket)
		s.GetJamState().ResetTallyStatistics()
	}
	sf := s.GetSafrole()
	var vs []types.Validator
	vs = sf.PrevValidators
	if len(vs) == 0 {
		panic("No validators")
	}

	s2, err := sf.ApplyStateTransitionTickets(ticketExts, targetJCE, sf_header) // Entropy computed!
	if err != nil {
		//fmt.Printf("sf.ApplyStateTransitionTickets %v\n", jamerrors.GetErrorName(err))
		return s, err
	}
	err = VerifySafroleSTF(sf, &s2, blk)
	if err != nil {
		panic(fmt.Sprintf("VerifySafroleSTF %v\n", err))
	}
	vs = s2.PrevValidators
	if len(vs) == 0 {
		panic("No validators")
	}
	s.JamState.SafroleState = &s2
	s.RotateGuarantors()
	//fmt.Printf("ApplyStateTransitionFromBlock - SafroleState \n")
	s.JamState.tallyStatistics(uint32(blk.Header.AuthorIndex), "tickets", uint32(len(ticketExts)))
	elapsed := time.Since(start).Microseconds()
	if debugtrace && elapsed > 1000000 { // OPTIMIZED ApplyStateTransitionTickets/ValidateProposedTicket
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
	for validatorIndex, nassurances := range num_assurances {
		s.JamState.tallyStatistics(uint32(validatorIndex), "assurances", uint32(nassurances))
	}
	for validatorIndex, nreports := range num_reports {
		s.JamState.tallyStatistics(uint32(validatorIndex), "reports", uint32(nreports))
	}

	// 28 -- ACCUMULATE
	var g uint64 = 10000
	o := s.JamState.newPartialState()
	if debug {
		fmt.Printf("[N%d] s.StateRoot=%v newPartialState len=%v\n", s.Id, s.StateRoot, len(o.D))
	}
	var f map[uint32]uint32
	var b []BeefyCommitment
	accumulate_input_wr := s.AvailableWorkReport
	accumulate_input_wr = s.AccumulatableSequence(accumulate_input_wr)
	n, t, b := s.OuterAccumulate(g, accumulate_input_wr, o, f)
	if debug {
		fmt.Printf("ApplyStateTransitionFromBlock - Accumulate\n")
	}

	// Not sure whether transfer happens here
	tau := s.GetTimeslot() // Not sure whether τ ′ is set up like this
	if len(t) > 0 {
		s.ProcessDeferredTransfers(o.D, tau, t)
	}
	// make sure all service accounts can be written
	for _, sa := range o.D {
		sa.Mutable = true
		sa.Dirty = true
	}

	s.ApplyXContext(o)
	s.ApplyStateTransitionAccumulation(accumulate_input_wr, n, old_timeslot)
	s.ApplyStateTransitionAuthorizations()
	// n.r = M_B( [ s \ E_4(s) ++ E(h) | (s,h) in C] , H_K)
	var leaves [][]byte
	for _, sa := range b {
		// put (s,h) of C  into leaves
		leaf := append(common.Uint32ToBytes(sa.Service), sa.Commitment.Bytes()...)
		leaves = append(leaves, leaf)
	}
	tree := trie.NewWellBalancedTree(leaves, types.Keccak)
	s.AccumulationRoot = common.Hash(tree.Root())

	// appends "n" to MMR "Beta" s.JamState.RecentBlocks
	s.ApplyStateRecentHistory(blk, &(s.AccumulationRoot))

	// 29 -  Update Authorization Pool alpha'
	err = s.ApplyStateTransitionAuthorizations()
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

	// err = s.OnTransfer()
	// if err != nil {
	// 	return s, err
	// }
	// if debug {
	// 	fmt.Printf("ApplyStateTransitionFromBlock - OnTransfer\n")
	// }
	s.StateRoot = s.UpdateTrieState()
	return s, nil
}

func (s *StateDB) GetBlock() *types.Block {
	return s.Block
}

func (s *StateDB) isCorrectCodeHash(workReport types.WorkReport) bool {
	// TODO: logic to validate the code hash prediction.
	return true
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

func (s *StateDB) VerifyBlockHeader(bl *types.Block) (isValid bool, validatorIdx uint16, ietf_pub bandersnatch.BanderSnatchKey, verificationErr error) {
	targetJCE := bl.TimeSlot()
	h := bl.GetHeader()
	validatorIdx = h.AuthorIndex

	// ValidateTicketTransition
	sf0 := s.GetPosteriorSafroleEntropy(targetJCE)

	// author_idx is the K' so we use the sf_tmp
	signing_validator := sf0.GetCurrValidator(int(validatorIdx))
	block_author_ietf_pub := bandersnatch.BanderSnatchKey(signing_validator.GetBandersnatchKey())

	// compute c within (6.15) & (6.16)
	blockSealEntropy := sf0.Entropy[3]
	var c []byte
	if sf0.GetEpochT() == 1 {
		_, currPhase := sf0.EpochAndPhase(targetJCE)
		winning_ticket := (sf0.TicketsOrKeys.Tickets)[currPhase]
		c = ticketSealVRFInput(blockSealEntropy, uint8(winning_ticket.Attempt))
	} else {
		c = append([]byte(types.X_F), blockSealEntropy.Bytes()...)
	}

	// H_s Verification (6.15/6.16)
	H_s := h.Seal[:]
	m := h.BytesWithoutSig()
	vrfOutput, err := bandersnatch.IetfVrfVerify(block_author_ietf_pub, H_s, c, m)
	if err != nil {
		if debugSeal {
			fmt.Printf("**** IETF Verify FAIL H_s(pub=%x, c=%x, m=%x)=%x\n", block_author_ietf_pub[:], c, m, H_s[:])
		}
		return false, validatorIdx, block_author_ietf_pub, fmt.Errorf("VerifyBlockHeader Failed: H_s Verification")
		//return true, validatorIdx, block_author_ietf_pub, nil
	}

	// H_v Verification (6.17)
	H_v := h.EntropySource[:]
	c = append([]byte(types.X_E), vrfOutput...)
	_, err = bandersnatch.IetfVrfVerify(block_author_ietf_pub, H_v, c, []byte{})
	if err != nil {
		if debugSeal {
			fmt.Printf("**** IETF Verify FAIL H_v(pub=%x, c=%x, m=[])=%x\n", block_author_ietf_pub[:], c, H_v[:])
		}
		return false, validatorIdx, block_author_ietf_pub, fmt.Errorf("VerifyBlockHeader Failed: H_v Verification")
		//return true, validatorIdx, block_author_ietf_pub, nil
	}
	return true, validatorIdx, block_author_ietf_pub, nil
}

func (s *StateDB) SealBlockWithEntropy(blockAuthorPub bandersnatch.BanderSnatchKey, blockAuthorPriv bandersnatch.BanderSnatchSecret, validatorIdx uint16, targetJCE uint32, originalBlock *types.Block) (*types.Block, error) {
	newBlock := originalBlock.Copy()
	header := newBlock.GetHeader()
	header.ExtrinsicHash = newBlock.Extrinsic.Hash()

	// Validate ticket transition
	sf0 := s.GetPosteriorSafroleEntropy(targetJCE)

	// Prepare a container to store all intermediate values for debugging / auditing
	material := &SealBlockMaterial{
		BlockAuthorPub:  fmt.Sprintf("%x", blockAuthorPub[:]),
		BlockAuthorPriv: fmt.Sprintf("%x", blockAuthorPriv[:]), // do NOT store real priv keys in production
	}

	blockSealEntropy := sf0.Entropy[3]
	if sf0.GetEpochT() == 1 {
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
		if debugSeal {
			fmt.Printf("**** IETF SIGN 1 H_v(k=%x, c=%x, m=[])=%x\n", blockAuthorPriv[:], c, header.EntropySource[:])
		}
		if saveSealBlockMaterial {
			// Save for the material
			material.TicketID = fmt.Sprintf("%s", ticketID)
			material.Attempt = winningTicket.Attempt
			material.CForHv = fmt.Sprintf("%x", c[:])
			material.MForHv = ""
			material.Hv = fmt.Sprintf("%x", H_v[:])
		}

		// H_s generation (primary) 6.15
		c = append(append([]byte(types.X_T), blockSealEntropy.Bytes()...), byte(uint8(winningTicket.Attempt)&0xF))
		m := header.BytesWithoutSig()
		H_s, _, err := bandersnatch.IetfVrfSign(blockAuthorPriv, c, m)
		if err != nil {
			return nil, fmt.Errorf("error generating H_s for primary epoch: %w", err)
		}
		copy(header.Seal[:], H_s[:])
		if debugSeal {
			fmt.Printf("**** IETF SIGN 2 H_s(k=%x, c=%x, m=%x)=%x\n", blockAuthorPriv[:], c, header.BytesWithoutSig(), header.Seal[:])
		}

		// Save for the material
		if saveSealBlockMaterial {
			material.T = 1
			material.Entropy3 = fmt.Sprintf("%x", blockSealEntropy[:])
			material.CForHs = fmt.Sprintf("%x", c[:])
			material.MForHs = fmt.Sprintf("%x", m[:])
			material.Hs = fmt.Sprintf("%x", H_s[:])
		}
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

		// Save for the material
		if saveSealBlockMaterial {
			material.T = 0
			material.Entropy3 = fmt.Sprintf("%x", blockSealEntropy[:])
			material.CForHv = fmt.Sprintf("%x", cHv[:])
			material.MForHv = "" // empty
			material.Hv = fmt.Sprintf("%x", H_v[:])

			material.CForHs = fmt.Sprintf("%x", c[:])
			material.MForHs = fmt.Sprintf("%x", m[:])
			material.Hs = fmt.Sprintf("%x", H_s[:])
		}
	}

	newBlock.Header = header
	headerbytes, _ := header.Bytes()

	if saveSealBlockMaterial {
		material.HeaderBytes = fmt.Sprintf("%x", headerbytes)
		// Write material as JSON into a file: seals/validatorIdx-targetJCE.json
		if err := os.MkdirAll("../jamtestvectors/seals", 0o755); err != nil {
			return nil, fmt.Errorf("failed to mkdir seals: %w", err)
		}
		jsonData, err := json.MarshalIndent(material, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal SealBlockMaterial: %w", err)
		}
		fileName := fmt.Sprintf("../jamtestvectors/seals/%d-%d.json", material.T, validatorIdx)
		if err := ioutil.WriteFile(fileName, jsonData, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write SealBlockMaterial to file: %w", err)
		}
	}

	return newBlock, nil
}

// make block generate block prior to state execution
func (s *StateDB) MakeBlock(credential types.ValidatorSecret, targetJCE uint32, ticketID common.Hash, extrinsic_pool *types.ExtrinsicPool) (bl *types.Block, err error) {
	sf := s.GetSafrole()
	isNewEpoch := sf.IsNewEpoch(targetJCE)
	needWinningMarker := sf.IseWinningMarkerNeeded(targetJCE)
	stateRoot := s.GetStateRoot()
	s.JamState.CheckInvalidCoreIndex()
	//fmt.Printf("\n\n----- MAKEBLOCK\n[N%v] MakeBlock using stateRoot %v JamState %s\n", s.Id, stateRoot, s.String())
	s.RecoverJamState(stateRoot)
	//fmt.Printf("[N%v] Recovered JamState %s\n", s.Id, s.String())
	s.JamState.CheckInvalidCoreIndex()
	//fmt.Printf("------ MAKEBLOCK\n\n")

	b := types.NewBlock()
	h := types.NewBlockHeader()
	extrinsicData := types.NewExtrinsic()
	h.ParentHeaderHash = s.HeaderHash
	h.ParentStateRoot = stateRoot
	h.Slot = targetJCE
	b.Header = *h
	// eq 71
	if isNewEpoch {
		epochMarker := sf.GenerateEpochMarker()
		//a tuple of the epoch randomness and a sequence of Bandersnatch keys defining the Bandersnatch valida- tor keys (kb) beginning in the next epoch
		if debug {
			fmt.Printf("[N%d] *** \033[32mEpochMarker\033[0m %v\n", s.Id, epochMarker)
		}
		h.EpochMark = epochMarker
	}
	// Extrinsic Data has 5 different Extrinsics
	// E_P - Preimages:  aggregate queuedPreimageLookups into extrinsicData.Preimages
	extrinsicData.Preimages = make([]types.Preimages, 0)

	// Make sure this Preimages is ready to be included..
	queued_preimage := extrinsic_pool.GetPreimageFromPool()
	for _, preimageLookup := range queued_preimage {
		_, err := s.ValidateLookup(preimageLookup)
		if err == nil {
			pl, err := preimageLookup.DeepCopy()
			if err != nil {
				continue
			}
			extrinsicData.Preimages = append(extrinsicData.Preimages, pl)
			extrinsic_pool.RemoveOldPreimages([]types.Preimages{*preimageLookup}, targetJCE)
		} else {
			storage.Logger.RecordLogs(storage.Preimage_error, fmt.Sprintf("Error in ValidateLookup: %v", err), true)
			extrinsic_pool.RemoveOldPreimages([]types.Preimages{*preimageLookup}, targetJCE)
			continue
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

	// E_A - Assurances
	// 126 - The assurances must ordered by validator index
	extrinsicData.Assurances = extrinsic_pool.GetAssurancesFromPool(h.ParentHeaderHash)
	SortAssurances(extrinsicData.Assurances)

	tmpState := s.JamState.Copy()
	_, _ = tmpState.ProcessAssurances(extrinsicData.Assurances, targetJCE)
	// E_G - Guarantees: aggregate queuedGuarantees into extrinsicData.Guarantees
	extrinsicData.Guarantees = make([]types.Guarantee, 0)
	queuedGuarantees := make([]types.Guarantee, 0)
	currRotationIdx := s.GetTimeslot() / types.ValidatorCoreRotationPeriod
	previousIdx := currRotationIdx - 1
	acceptedTimeslot := previousIdx * types.ValidatorCoreRotationPeriod
	queuedGuarantees = extrinsic_pool.GetGuaranteesFromPool(acceptedTimeslot)
	storage.Logger.RecordLogs(storage.EG_status, fmt.Sprintf("MakeBlock: Queued Guarantees: %v for slot %d, acceptedTimeslot %d", len(queuedGuarantees), targetJCE, acceptedTimeslot), true)
	for _, guarantee := range queuedGuarantees {
		g, err := guarantee.DeepCopy()
		if err != nil {
			continue
		}
		s.JamState.CheckInvalidCoreIndex()
		for _, rho := range s.JamState.AvailabilityAssignments {
			if debug {
				fmt.Printf("Rho %v\n", rho)
			}
		}
		err = s.Verify_Guarantee_MakeBlock(g, b, tmpState)
		if err != nil {
			storage.Logger.RecordLogs(storage.EG_error, fmt.Sprintf("Error in Verify_Guarantee_MakeBlock: %v", err), true)
			continue
		}
		extrinsicData.Guarantees = append(extrinsicData.Guarantees, g)
		if debugG {
			storage.Logger.RecordLogs(storage.EG_status, fmt.Sprintf("MakeBlock: Added Guarantee: %v", g), true)
		}
		// check guarantee one per core
		// check guarantee is not a duplicate
	}
	for i := 0; i < len(extrinsicData.Guarantees); i++ {
		log := fmt.Sprintf("ExtrinsicData.Guarantees[%d] = %v, core%d", i, extrinsicData.Guarantees[i].Report.GetWorkPackageHash(), extrinsicData.Guarantees[i].Report.CoreIndex)
		storage.Logger.RecordLogs(storage.EG_status, log, true)
	}
	extrinsicData.Guarantees, err, _ = s.VerifyGuaranteesMakeBlock(extrinsicData.Guarantees, b)
	if err != nil {
		storage.Logger.RecordLogs(storage.EG_error, fmt.Sprintf("Error in Verify_Guarantees_MakeBlock: %v", err), true)
	}
	// E_D - Disputes: aggregate queuedDisputes into extrinsicData.Disputes
	// d := s.GetJamState()

	// extrinsicData.Disputes = make([]types.Dispute, 0)
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
			tmp_accumulator := make([]types.TicketBody, len(s.JamState.SafroleState.NextEpochTicketsAccumulator))
			copy(tmp_accumulator, s.JamState.SafroleState.NextEpochTicketsAccumulator)
			// remove the tickets that already in state from the pool
			for _, ticket := range tmp_accumulator {
				extrinsic_pool.RemoveTicketFromPool(ticket.Id, next_n2)
			}
			// get the clean tickets out from the pool
			tickets := extrinsic_pool.GetTicketsFromPool(next_n2)
			SortTicketsById(tickets) // first include the better id
			if len(tickets) > types.MaxTicketsPerExtrinsic {
				tickets = tickets[:types.MaxTicketsPerExtrinsic]
			}
			for _, ticket := range tickets {
				ticket_body := ticket.ToBody()
				tmp_accumulator = append(tmp_accumulator, ticket_body)
			}
			SortTicketBodies(tmp_accumulator)
			tmp_accumulator = TrimTicketBodies(tmp_accumulator)
			// only include the tickets that will be included in the accumulator
			for _, ticket := range tickets {
				t, err := ticket.DeepCopy()
				if err != nil {
					continue
				}
				ticketID, _ := t.TicketID()
				if s.JamState.SafroleState.InTicketAccumulator(ticketID) {
					continue
				}
				if TicketInTmpAccumulator(ticketID, tmp_accumulator) {
					extrinsicData.Tickets = append(extrinsicData.Tickets, t)
				} else {
					extrinsic_pool.RemoveTicketFromPool(ticketID, next_n2)
				}
			}

		}
	}

	h.ExtrinsicHash = extrinsicData.Hash()
	author_index, err := sf.GetAuthorIndex(credential.BandersnatchPub.Hash(), "Curr")
	if err != nil {
		return bl, err
	}
	h.AuthorIndex = author_index
	b.Extrinsic = extrinsicData

	block_author_ietf_priv, err := ConvertBanderSnatchSecret(credential.BandersnatchSecret)
	if err != nil {
		return bl, err
	}
	block_author_ietf_pub, err := ConvertBanderSnatchPub(credential.BandersnatchPub[:])
	if err != nil {
		return bl, err
	}

	// For Primary, Verify ticketID actually matched the expected winning ticket
	_, ticketIDErr := s.ValidateVRFSealInput(ticketID, targetJCE)
	if ticketIDErr != nil {
		return bl, err
	}

	b.Header = *h
	sealedBlock, sealErr := s.SealBlockWithEntropy(block_author_ietf_pub, block_author_ietf_priv, author_index, targetJCE, b)
	if sealErr != nil {
		return bl, sealErr
	}
	return sealedBlock, nil
}

// Make sure ticketID "x_t ++ n3' ++ attempt" match the one in TicketsOrKeys. Fallback has no ticketID to check.
func (s *StateDB) ValidateVRFSealInput(ticketID common.Hash, targetJCE uint32) (bool, error) {

	// ValidateTicketTransition
	sf0 := s.GetPosteriorSafroleEntropy(targetJCE)

	if sf0.GetEpochT() == 0 {
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
