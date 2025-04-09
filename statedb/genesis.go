package statedb

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	//"io/ioutil"
	"os"
	//"path/filepath"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/google/go-cmp/cmp"
)

// 6.4.1 Startup parameters

// CreateGenesisState generates the StateDB object and genesis statedb
func CreateGenesisState(sdb *storage.StateDBStorage, chainSpec types.ChainSpec, epochFirstSlot uint64, network string) (outfn string, err error) {
	statedb, err := newStateDB(sdb, common.Hash{})
	if err != nil {
		return
	}
	var validatorshashes [types.TotalValidators]types.ValidatorKeyTuple
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
		copy(validatorshashes[i].BandersnatchKey[:], v.BandersnatchPub[:])
		copy(validatorshashes[i].Ed25519Key[:], v.Ed25519Pub[:])
		validators[i] = vpub
	}

	j := NewJamState()
	j.SafroleState.EpochFirstSlot = uint32(epochFirstSlot)
	j.SafroleState.Timeslot = 0
	j.SafroleState.Timeslot = j.SafroleState.Timeslot / types.SecondsPerSlot

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
	// Load services into genesis state
	services := []types.TestService{
		{
			ServiceCode: BootstrapServiceCode,
			FileName:    BootstrapServiceFile,
			ServiceName: "bootstrap",
		},
	}
	auth_pvm := common.GetFilePath(BootStrapNullAuthFile)
	auth_code_bytes, err0 := os.ReadFile(auth_pvm)
	if err0 != nil {
		return outfn, err
	}
	auth_code := AuthorizeCode{
		PackageMetaData:   []byte("bootstrap"),
		AuthorizationCode: auth_code_bytes,
	}
	auth_code_encoded_bytes, err := auth_code.Encode()
	if err != nil {
		return outfn, err
	}
	auth_code_hash := common.Blake2Hash(auth_code_encoded_bytes) //pu
	auth_code_hash_hash := common.Blake2Hash(auth_code_hash[:])  //pa
	auth_code_len := uint32(len(auth_code_encoded_bytes))
	for _, service := range services {
		if service.ServiceCode != 0 {
			// only Bootstrap
			continue
		}

		code, err0 := types.ReadCodeWithMetadata(service.FileName, service.ServiceName)
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
			StorageSize:     uint64(81*2 + codeLen + auth_code_len), // a_l = ∑ 81+z per (h,z) + ∑ 32+s https://graypaper.fluffylabs.dev/#/5f542d7/116e01116e01
			NumStorageItems: 2*2 + 0,                                //a_i = 2⋅∣al∣+∣as∣
		}

		statedb.WriteServicePreimageBlob(service.ServiceCode, code)
		statedb.WriteServicePreimageBlob(service.ServiceCode, auth_code_encoded_bytes)
		statedb.writeService(service.ServiceCode, &bootstrapServiceAccount)
		statedb.WriteServicePreimageLookup(service.ServiceCode, codeHash, codeLen, bootStrapAnchor)
		statedb.WriteServicePreimageLookup(service.ServiceCode, auth_code_hash, auth_code_len, bootStrapAnchor)
	}
	fmt.Printf("Bootstrap AuthorizationHash: %v\n", auth_code_hash_hash) //p_a
	fmt.Printf("Bootstrap AuthorizationCodeHash: %v\n", auth_code_hash)  //p_u
	for idx := range j.AuthorizationQueue {
		for i := range j.AuthorizationQueue[idx] {
			j.AuthorizationQueue[idx][i] = auth_code_hash_hash
		}
	}
	statedb.JamState = j
	statedb.StateRoot = statedb.UpdateTrieState()
	statefn := common.GetFilePath(fmt.Sprintf("chainspecs/state_snapshots/genesis-%s", network))
	blockfn := common.GetFilePath(fmt.Sprintf("chainspecs/blocks/genesis-%s", network))

	trace := StateSnapshotRaw{
		StateRoot: statedb.StateRoot,
		KeyVals:   statedb.GetAllKeyValues(),
	}
	types.SaveObject(statefn, statedb.JamState.Snapshot(&trace))
	b := types.Block{
		Header: types.BlockHeader{
			ParentHeaderHash: common.Hash{},
			ParentStateRoot:  common.Hash{},
			Slot:             0,
			ExtrinsicHash:    common.Hash{},
			EpochMark: &types.EpochMark{
				Entropy:        common.Hash{},
				TicketsEntropy: common.Hash{},
				Validators:     validatorshashes,
			},
			//TicketsMark: types.TicketsMark{},
			//OffendersMark: [],
			AuthorIndex: 65535,
		},
		Extrinsic: types.ExtrinsicData{
			Tickets:    make([]types.Ticket, 0),
			Preimages:  make([]types.Preimages, 0),
			Guarantees: make([]types.Guarantee, 0),
			Assurances: make([]types.Assurance, 0),
			Disputes: types.Dispute{
				Verdict: make([]types.Verdict, 0),
				Culprit: make([]types.Culprit, 0),
				Fault:   make([]types.Fault, 0),
			},
		},
	}
	statedb.Block = &b

	blockfn = common.GetFilePath(fmt.Sprintf("chainspecs/blocks/genesis-%s", network))
	fmt.Printf("Genesis %s: %s\n", blockfn, b.String())
	fmt.Printf("Genesis Header Hash: %s\n", b.Header.Hash().String())
	types.SaveObject(blockfn, b)

	outfn = common.GetFilePath(fmt.Sprintf("chainspecs/traces/genesis-%s", network))
	types.SaveObject(outfn, trace)

	rawOutfn := common.GetFilePath(fmt.Sprintf("chainspecs/rawkv/genesis-%s.json", network))
	rawBinOutfn := common.GetFilePath(fmt.Sprintf("chainspecs/rawkv/genesis-%s.bin", network))
	rawByte, err := trace.CustomMarshalJSON()
	rawCodec, err := trace.CustomCodecEncode()
	err = os.WriteFile(rawOutfn, rawByte, 0644)
	if err != nil {
		return rawOutfn, fmt.Errorf("Error writing rawOut file: %v\n", err)
	}
	err = os.WriteFile(rawBinOutfn, rawCodec, 0644)
	if err != nil {
		return rawBinOutfn, fmt.Errorf("Error writing rawBinOut file: %v\n", err)
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

type KeyVal_custom struct {
	Key common.Hash
	Val []byte
}
type SnapshotOutput_custum struct {
	Input  []KeyVal_custom `json:"input"`
	Output common.Hash     `json:"output"`
}

func (s *StateSnapshotRaw) CustomCodecEncode() ([]byte, error) {

	input := make([]KeyVal_custom, 0)
	for _, kv := range s.KeyVals {
		if len(kv.Key) != 32 {
			return nil, fmt.Errorf("Key %x length is not 32 bytes", kv.Key)
		}
		key := common.BytesToHash(kv.Key)
		input = append(input, KeyVal_custom{Key: key, Val: kv.Value})
	}
	// sort it by key
	sort.Slice(input, func(i, j int) bool {
		return bytes.Compare(input[i].Key.Bytes(), input[j].Key.Bytes()) < 0
	})
	output := SnapshotOutput_custum{
		Input:  input,
		Output: s.StateRoot,
	}
	bytes, err := types.Encode(output)
	decoded, _, err := types.Decode(bytes, reflect.TypeOf(SnapshotOutput_custum{}))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	// transform it back to the original struct
	// see if it is the same
	outputdecoded := decoded.(SnapshotOutput_custum)
	if !reflect.DeepEqual(output, outputdecoded) {
		diff := cmp.Diff(output, outputdecoded)
		fmt.Printf("Diff: %s\n", diff)
	}
	return bytes, err
}

func NewBlockFromFile(blockfilename string) *types.Block {
	fn := common.GetFilePath(blockfilename)
	expectedCodec, err := os.ReadFile(fn)
	if err != nil {
		panic(fmt.Sprintf("failed to read codec file %s", fn))
	}
	r, _, err := types.Decode(expectedCodec, reflect.TypeOf(types.Block{}))
	if err != nil {
		fmt.Printf("%s - %v", fn, err)
		panic("failed to decode codec data")
	}
	b := r.(types.Block)
	return &b
}

func NewStateDBFromSnapshotRawFile(sdb *storage.StateDBStorage, statefilename string) (statedb *StateDB, err error) {
	fn := common.GetFilePath(statefilename)
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

func NewEpoch0Timestamp(test_name ...string) uint64 {
	if len(test_name) == 0 {
		now := time.Now().Unix()
		second_per_epoch := types.SecondsPerEpoch // types.EpochLength
		now = common.ComputeJCETime(now, true)
		waitTime := int64(second_per_epoch) - now%int64(second_per_epoch) // how far is next epoch (sec)
		epoch0Timestamp := uint64(now) + uint64(waitTime)
		epoch0Phase := uint64(now) / uint64(second_per_epoch)
		log.Trace(module, "NewEpoch0Timestamp", "Raw now", uint64(now), "Raw waitTime", waitTime, "Raw epoch0P", epoch0Phase, "Raw epoch0Timestamp", epoch0Timestamp)

		if types.TimeSavingMode { //always be five second
			deDuctedTime := (time.Duration(-waitTime + 5)) * time.Second                             // how much time we have to deduct
			driftTime := (time.Duration(int64(epoch0Phase) * int64(second_per_epoch))) * time.Second // adjust it to e'=1,m'=00
			adjustedTime := deDuctedTime
			adjustedTime += driftTime
			common.AddJamStart(adjustedTime)
			currTS := time.Now().Unix()
			now = common.ComputeJCETime(currTS, true)
			waitTime = int64(second_per_epoch) - now%int64(second_per_epoch)
			epoch0Timestamp = uint64(now) + uint64(waitTime)
		}
		log.Trace(module, "NewEpoch0Timestamp", "NewGenesisConfig epoch0Timestamp", epoch0Timestamp, "Wait", uint64(waitTime))
		return epoch0Timestamp
	} else if test_name[0] == "jamtestnet" { // make sure the timestamp is 72
		if len(test_name) != 2 {
			panic("jamtestnet should have two parameters")
		}
		now := time.Now()
		loc := now.Location()
		start_time := test_name[1]
		startTime, err := time.ParseInLocation("2006-01-02 15:04:05", start_time, loc)
		if err != nil {
			fmt.Printf("start_time: %s\n", start_time)
			fmt.Println("Invalid time format. Use YYYY-MM-DD HH:MM:SS")
			return 0
		}
		starting := startTime.Unix()
		starting = common.ComputeJCETime(starting, true)
		second_per_epoch := types.SecondsPerEpoch //72
		diff_seconds := starting - int64(second_per_epoch)
		fmt.Printf("Current time: %v\n", now)
		fmt.Printf("Seconds per epoch: %v\n", second_per_epoch)
		fmt.Printf("Difference in seconds: %v\n", diff_seconds)
		adjustedTime := time.Duration(diff_seconds) * time.Second
		fmt.Printf("Adjusted time: %v\n", adjustedTime)
		common.AddJamStart(adjustedTime)
		fmt.Printf("JAM start now: %v\n", common.JceStart)
		starting = time.Now().Unix()
		starting = common.ComputeJCETime(starting, true)
		return uint64(starting)
	} else if test_name[0] == "jam" {

		epoch0Timestampint64 := common.JceStart.Unix()
		epoch0Timestamp := uint64(epoch0Timestampint64)
		fmt.Printf("JAM start now: %v\n", common.JceStart)
		fmt.Printf("JAM start Timestamp: %v\n", epoch0Timestamp)
		return 0
	} else {
		now := time.Now().Unix()
		second_per_epoch := types.SecondsPerEpoch // types.EpochLength
		now = common.ComputeJCETime(now, true)
		waitTime := int64(second_per_epoch) - now%int64(second_per_epoch)
		if waitTime < 24 {
			panic("Wait time is less than 24 seconds")
		}
		epoch0Timestamp := uint64(now) + uint64(waitTime)
		epoch0Phase := uint64(now) / uint64(second_per_epoch)
		log.Trace(module, "NewEpoch0Timestamp", "Raw now", uint64(now), "Raw waitTime", waitTime, "Raw epoch0P", epoch0Phase, "Raw epoch0Timestamp", epoch0Timestamp)
		return uint64(epoch0Timestamp)
	}
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
