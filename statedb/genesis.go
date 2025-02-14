package statedb

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"

	//"io/ioutil"
	"os"
	//"path/filepath"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

// 6.4.1 Startup parameters

// CreateGenesisState generates the StateDB object and genesis statedb
func CreateGenesisState(sdb *storage.StateDBStorage, chainSpec types.ChainSpec, epochFirstSlot uint64, network string) (outfn string, err error) {
	statedb, err := newStateDB(sdb, common.Hash{})
	if err != nil {
		return
	}

	validators := make(types.Validators, chainSpec.NumValidators)
	for i := uint32(0); i < uint32(chainSpec.NumValidators); i++ {
		seed := make([]byte, 32)

		bandersnatch_seed, ed25519_seed, bls_seed := seed, seed, seed
		for j := 0; j < 8; j++ {
			binary.LittleEndian.PutUint32(seed[j*4:], i)
		}

		v, err0 := InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, "")
		if err0 != nil {
			return outfn, err0
		}
		var vpub types.Validator
		copy(vpub.Bandersnatch[:], v.BandersnatchPub[:])
		copy(vpub.Ed25519[:], v.Ed25519Pub[:])
		copy(vpub.Bls[:], v.BlsPub[:])
		copy(vpub.Metadata[:], v.Metadata[:])
		validators[i] = vpub
	}

	j := NewJamState()
	j.SafroleState.EpochFirstSlot = uint32(epochFirstSlot)
	j.SafroleState.Timeslot = 0
	if types.TimeUnitMode != "TimeStamp" {
		j.SafroleState.Timeslot = j.SafroleState.Timeslot / types.SecondsPerSlot
	}

	vB := []byte{}
	for _, v := range validators {
		vB = append(vB, v.Bytes()...)
	}
	j.SafroleState.PrevValidators = validators
	j.SafroleState.CurrValidators = validators
	j.SafroleState.NextValidators = validators
	j.SafroleState.DesignedValidators = validators

	/*
		The on-chain randomness is initialized after the genesis block construction.
		The first buffer entry is set as the Blake2b hash of the genesis block,
		each of the other entries is set as the Blake2b hash of the previous entry.
	*/
	j.SafroleState.Entropy[0] = common.BytesToHash(common.ComputeHash(vB))                                //BLAKE2B hash of the genesis block#0
	j.SafroleState.Entropy[1] = common.BytesToHash(common.ComputeHash(j.SafroleState.Entropy[0].Bytes())) //BLAKE2B of Current
	j.SafroleState.Entropy[2] = common.BytesToHash(common.ComputeHash(j.SafroleState.Entropy[1].Bytes())) //BLAKE2B of EpochN1
	j.SafroleState.Entropy[3] = common.BytesToHash(common.ComputeHash(j.SafroleState.Entropy[2].Bytes())) //BLAKE2B of EpochN2
	j.SafroleState.TicketsOrKeys.Keys, _ = j.SafroleState.ChooseFallBackValidator()
	j.DisputesState = Psi_state{}

	// Setup Bootstrap Service for all 3 privileges
	j.PrivilegedServiceIndices.Kai_a = BootstrapServiceCode
	j.PrivilegedServiceIndices.Kai_v = BootstrapServiceCode
	j.PrivilegedServiceIndices.Kai_m = BootstrapServiceCode
	for i := 0; i < types.TotalCores; i++ {
		j.AuthorizationsPool[i] = make([]common.Hash, types.MaxAuthorizationPoolItems)
		var temp [types.MaxAuthorizationQueueItems]common.Hash
		j.AuthorizationQueue[i] = temp
		//j.AuthorizationQueue[i] = make([]common.Hash, types.MaxAuthorizationQueueItems)
	}
	// setup the initial state of the accumulate state
	for i := 0; i < types.EpochLength; i++ {
		j.AccumulationQueue[i] = make([]types.AccumulationQueue, 0)
		j.AccumulationHistory[i] = types.AccumulationHistory{
			WorkPackageHash: make([]common.Hash, 0),
		}
	}

	statedb.JamState = j
	statedb.Block = nil

	// Load services into genesis state
	services := []types.TestService{
		{ServiceCode: BootstrapServiceCode, FileName: BootstrapServiceFile},
	}

	for _, service := range services {
		if service.ServiceCode != 0 {
			// only Bootstrap
			continue
		}
		fn := common.GetFilePath(service.FileName)
		code, err0 := os.ReadFile(fn)
		if err0 != nil {
			return outfn, err
		}
		var balance uint64 = uint64(10000000000)
		codeHash := common.Blake2Hash(code)
		codeLen := uint32(len(code)) // z
		bootStrapAnchor := []uint32{0}

		bootstrapServiceAccount := types.ServiceAccount{
			CodeHash:        codeHash,
			Balance:         balance,
			GasLimitG:       100,
			GasLimitM:       100,
			StorageSize:     uint64(81 + codeLen), // a_l = ∑ 81+z per (h,z) + ∑ 32+s https://graypaper.fluffylabs.dev/#/5f542d7/116e01116e01
			NumStorageItems: 2*1 + 0,              //a_i = 2⋅∣al∣+∣as∣
		}

		statedb.WriteServicePreimageBlob(service.ServiceCode, code)
		statedb.writeService(service.ServiceCode, &bootstrapServiceAccount)
		statedb.WriteServicePreimageLookup(service.ServiceCode, codeHash, codeLen, bootStrapAnchor)
	}

	statedb.StateRoot = statedb.UpdateTrieState()
	outfn = common.GetFilePath(fmt.Sprintf("chainspecs/state_snapshots/genesis-%s", network))

	trace := StateSnapshotRaw{
		StateRoot: statedb.StateRoot,
		KeyVals:   statedb.GetAllKeyValues(),
	}
	types.SaveObject(outfn, statedb.JamState.Snapshot(&trace))
	outfn = common.GetFilePath(fmt.Sprintf("chainspecs/traces/genesis-%s", network))
	types.SaveObject(outfn, trace)

	/*
		tree := statedb.GetTrie()
		tree.PrintTree(tree.Root, 0)
	*/
	rawOutfn := common.GetFilePath(fmt.Sprintf("chainspecs/rawkv/genesis-%s.json", network))
	rawByte, err := trace.CustomMarshalJSON()
	err = os.WriteFile(rawOutfn, rawByte, 0644)
	if err != nil {
		return rawOutfn, fmt.Errorf("Error writing rawOut file: %v\n", err)
	}
	types.SaveObject(outfn, trace)

	return
}

func (s StateSnapshotRaw) CustomMarshalJSON() ([]byte, error) {
	type SnapshotOutput struct {
		Input  map[string]string `json:"input"`
		Output string            `json:"output"`
	}

	// Build the map of key->value from s.KeyVals.
	inputMap := make(map[string]string)
	for _, kv := range s.KeyVals {
		keyHex := hex.EncodeToString(kv.Key)
		valHex := hex.EncodeToString(kv.Value)
		inputMap[keyHex] = valHex
	}

	stateRootHex := hex.EncodeToString(s.StateRoot[:]) // strip 0x

	output := SnapshotOutput{
		Input:  inputMap,
		Output: stateRootHex,
	}

	outputs := make([]SnapshotOutput, 1)
	outputs[0] = output
	return json.MarshalIndent(outputs, "", "  ")
}

func NewStateDBFromSnapshotRawFile(sdb *storage.StateDBStorage, filename string) (statedb *StateDB, err error) {
	fn := common.GetFilePath(filename)
	snapshotRawBytes, err := os.ReadFile(fn)
	if err != nil {
		return statedb, err
	}
	var stateSnapshotRaw StateSnapshotRaw
	err = json.Unmarshal(snapshotRawBytes, &stateSnapshotRaw)
	if err != nil {
		return statedb, err
	}
	return NewStateDBFromSnapshotRaw(sdb, &stateSnapshotRaw)
}

func NewStateDBFromSnapshotRaw(sdb *storage.StateDBStorage, stateSnapshotRaw *StateSnapshotRaw) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}
	statedb.Block = nil
	statedb.StateRoot = statedb.UpdateAllTrieStateRaw(*stateSnapshotRaw)
	statedb.JamState = NewJamState()
	statedb.RecoverJamState(statedb.StateRoot)

	// Because we have safrolestate as internal state, JamState is NOT enough.
	s := statedb.JamState
	s.SafroleState.NextEpochTicketsAccumulator = s.SafroleStateGamma.GammaA      // γa: Ticket accumulator for the next epoch (epoch N+1) DONE
	s.SafroleState.TicketsVerifierKey = s.SafroleStateGamma.GammaZ               // γz: Epoch’s root, a Bandersnatch ring root composed with one Bandersnatch key of each of the next epoch’s validators (epoch N+1)
	s.SafroleState.TicketsOrKeys = s.SafroleStateGamma.GammaS                    // γs: Current epoch’s slot-sealer series (epoch N)
	s.SafroleState.NextValidators = types.Validators(s.SafroleStateGamma.GammaK) // γk: Next epoch’s validators (epoch N+1)
	// fmt.Printf("GammaK: %x\n", types.Validators(s.SafroleStateGamma.GammaK))
	// what about GammaK?
	//fmt.Printf("JS: %s\n", statedb.JamState.String())
	//fmt.Printf("Safrole State: %s\n", statedb.JamState.SafroleState.String())

	return statedb, nil
}

// The current time expressed in seconds after the start of the Jam Common Era. See section 4.4
func computeJCETime(unixTimestamp int64) uint32 {
	// Define the start of the Jam Common Era
	jceStart := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)

	// Convert the Unix timestamp to a Time object
	currentTime := time.Unix(unixTimestamp, 0).UTC()

	// Calculate the difference in seconds
	diff := currentTime.Sub(jceStart)
	return uint32(diff.Seconds())
}

// Function to convert JCETime back to the original Unix timestamp
func JCETimeToUnixTimestamp(jceTime uint32) int64 {
	// Define the start of the Jam Common Era
	jceStart := time.Date(2025, time.January, 1, 12, 0, 0, 0, time.UTC)

	// Add the JCE time (in seconds) to the start time
	originalTime := jceStart.Add(time.Duration(jceTime) * time.Second)
	return originalTime.Unix()
}

func NewEpoch0Timestamp() uint32 {
	now := time.Now().Unix()
	second_per_epoch := types.SecondsPerEpoch // types.EpochLength
	if types.TimeUnitMode != "TimeStamp" {
		now = common.ComputeJCETime(now, true)
	}
	waitTime := int64(second_per_epoch) - now%int64(second_per_epoch)
	epoch0Timestamp := uint64(now) + uint64(waitTime)
	epoch0Phase := uint64(now) / uint64(second_per_epoch)
	fmt.Printf("Raw now: %v\n", uint64(now))
	fmt.Printf("Raw waitTime: %v\n", waitTime)
	fmt.Printf("Raw epoch0P: %v\n", epoch0Phase)
	fmt.Printf("Raw epoch0Timestamp: %v\n", epoch0Timestamp)

	if types.TimeSavingMode {
		fmt.Printf("===Time Saving Mode===\n")
		deDuctedTime := (time.Duration(0)) * time.Second
		if !(waitTime < 5) {
			deDuctedTime = (time.Duration(-waitTime + 5)) * time.Second
		}
		driftTime := (time.Duration(int64(epoch0Phase) * int64(second_per_epoch))) * time.Second // adjust it to e'=1,m'=00
		adjustedTime := deDuctedTime
		if types.TimeSavingMode {
			adjustedTime += driftTime
		}
		fmt.Printf("AdjustTime: %v\n", driftTime)
		common.AddJamStart(adjustedTime)
		fmt.Printf("JCE Start Time: %v\n", common.JceStart)
		currTS := time.Now().Unix()
		if types.TimeUnitMode != "TimeStamp" {
			now = common.ComputeJCETime(currTS, true)
		}
		fmt.Printf("Epoch0P Drift: %v\n", now)
		waitTime = int64(second_per_epoch) - now%int64(second_per_epoch)
		fmt.Printf("TimeSavingMode Wait: %v\n", uint64(waitTime))
		epoch0Timestamp = uint64(now) + uint64(waitTime)
	}

	fmt.Printf("!!!NewGenesisConfig epoch0Timestamp: %v. Wait:%v Sec \n", epoch0Timestamp, uint64(waitTime))
	return uint32(epoch0Timestamp)
}

func InitValidator(bandersnatch_seed, ed25519_seed, bls_seed []byte, metadata string) (types.Validator, error) {
	validator := types.Validator{}
	banderSnatch_pub, _, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init BanderSnatch Key")
	}
	ed25519_pub, _, err := types.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init Ed25519 Key")
	}
	bls_pub, _, err := bls.InitBLSKey(bls_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init BanderSnatch Key")
	}

	validator.Ed25519 = ed25519_pub
	copy(validator.Bandersnatch[:], banderSnatch_pub.Bytes())
	copy(validator.Metadata[:], []byte(metadata))
	copy(validator.Bls[:], bls_pub.Bytes())
	return validator, nil
}

func InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed []byte, metadata string) (types.ValidatorSecret, error) {
	validatorSecret := types.ValidatorSecret{}
	banderSnatch_pub, banderSnatch_priv, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init BanderSnatch Key")
	}
	ed25519_pub, ed25519_priv, err := types.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init Ed25519 Key")
	}
	bls_pub, bls_priv, err := bls.InitBLSKey(bls_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init BLS Key")
	}

	copy(validatorSecret.Ed25519Secret[:], ed25519_priv[:])
	validatorSecret.Ed25519Pub = ed25519_pub

	validatorSecret.BandersnatchSecret = banderSnatch_priv.Bytes()
	validatorSecret.BandersnatchPub = types.BandersnatchKey(banderSnatch_pub)

	copy(validatorSecret.BlsSecret[:], bls_priv.Bytes())
	copy(validatorSecret.BlsPub[:], bls_pub.Bytes())

	copy(validatorSecret.Metadata[:], []byte(metadata))
	return validatorSecret, nil
}

func generateSeedSet(ringSize int) ([][]byte, error) {
	ringSet := make([][]byte, ringSize)
	for i := uint32(0); i < uint32(ringSize); i++ {
		seed := make([]byte, 32)
		for j := 0; j < 8; j++ {
			binary.LittleEndian.PutUint32(seed[j*4:], i)
		}
		ringSet[i] = seed
	}
	return ringSet, nil
}

func GenerateValidatorSecretSet(numNodes int) ([]types.Validator, []types.ValidatorSecret, error) {

	seeds, _ := generateSeedSet(numNodes)
	validators := make([]types.Validator, numNodes)
	validatorSecrets := make([]types.ValidatorSecret, numNodes)

	for i := 0; i < int(numNodes); i++ {

		seed_i := seeds[i]
		bandersnatch_seed := seed_i
		ed25519_seed := seed_i
		bls_seed := seed_i
		metadata := ""
		//metadata, _ := generateMetadata(i) // this is NOT used by other teams. somehow we agreed on empty metadata for now

		validator, err := InitValidator(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("Failed to init validator %v", i)
		}
		validators[i] = validator

		//bandersnatch_seed, ed25519_seed, bls_seed
		validatorSecret, err := InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("Failed to init validator secret=%v", i)
		}
		validatorSecrets[i] = validatorSecret
	}

	return validators, validatorSecrets, nil
}
