package statedb

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"

	"os"
	"time"

	bandersnatch "github.com/colorfulnotion/jam/bandersnatch"
	bls "github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	storage "github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

func MakeGenesisStateTransition(sdb *storage.StateDBStorage, epochFirstSlot uint64, network string, addresses []string) (trace *StateTransition, err error) {
	statedb, err := newStateDB(sdb, common.Hash{})
	if err != nil {
		return
	}
	var validatorshashes [types.TotalValidators]types.ValidatorKeyTuple
	validators := make(types.Validators, types.TotalValidators)
	for i := uint32(0); i < uint32(types.TotalValidators); i++ {
		seed := make([]byte, 32)

		bandersnatch_seed, ed25519_seed, bls_seed := seed, seed, seed
		for j := 0; j < 8; j++ {
			binary.LittleEndian.PutUint32(seed[j*4:], i)
		}
		metadata := make([]byte, 0)
		if len(addresses) == types.TotalValidators {
			metadata, err = common.AddressToMetadata(addresses[i])
			if err != nil {
				return nil, err
			}
		}
		v, err0 := InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err0 != nil {
			return nil, err0
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
	j.SafroleState.DesignatedValidators = validators

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
	j.DisputesState = DisputeState{}

	// Setup Bootstrap Service for all 3 privileges
	authQueueServiceID := [types.TotalCores]uint32{}
	for i := 0; i < types.TotalCores; i++ {
		authQueueServiceID[i] = AuthCopyServiceCode
	}
	j.PrivilegedServiceIndices.AuthQueueServiceID = authQueueServiceID
	j.PrivilegedServiceIndices.UpcomingValidatorsServiceID = BootstrapServiceCode
	j.PrivilegedServiceIndices.ManagerServiceID = BootstrapServiceCode
	// 0.7.1 introduces RegistrarServiceID
	j.PrivilegedServiceIndices.RegistrarServiceID = BootstrapServiceCode
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
	storage := make(map[common.Hash][]byte)

	// Set up EVM block 0 in genesis, Initialize BLOCK_NUMBER_KEY with 0
	//   BLOCK_NUMBER_KEY =  block_number (LE) || parent_hash.
	// Refine only allows authorized builders
	// Accumulate only allows increments on BLOCK_NUMBER_KEY when there is a timestamp greater than this
	// SubmitGenesisWorkPackage uses ReadStateWitness must submit extrinsics that match these storage entries
	storage[evmtypes.GetBlockNumberKey()] = evmtypes.SerializeBlockNumber(0, common.Hash{})
	// this is the genesis block
	storage[evmtypes.BlockNumberToObjectID(0)] = evmtypes.SerializeEvmBlockPayload(&evmtypes.EvmBlockPayload{
		Number:        0,
		ParentHash:    common.Hash{},
		LogsBloom:     [256]byte{},
		GasLimit:      30000000,
		Timestamp:     0,
		TxHashes:      []common.Hash{},
		ReceiptHashes: []common.Hash{},
	})

	// Load services into genesis state
	services := []types.TestService{
		{
			ServiceCode: BootstrapServiceCode,
			FileName:    BootstrapServiceFile,
			ServiceName: "bootstrap",
		},
		{
			ServiceCode: AlgoServiceCode,
			FileName:    AlgoServiceFile,
			ServiceName: "algo",
			Storage:     storage,
		},
		{
			ServiceCode: AuthCopyServiceCode,
			FileName:    AuthCopyServiceFile,
			ServiceName: "auth_copy",
		},
	}

	isIncludeEVM := false
	if isIncludeEVM {
		services = append(services, types.TestService{
			ServiceCode: EVMServiceCode,
			FileName:    EVMServiceFile,
			ServiceName: "evm",
		})
	} else {
		fmt.Printf("Genspec Without EVM Service\n")
	}

	auth_pvm := common.GetFilePath(BootStrapNullAuthFile)
	auth_code_bytes, err0 := os.ReadFile(auth_pvm)
	if err0 != nil {
		return nil, err0
	}
	auth_code := AuthorizeCode{
		PackageMetaData:   []byte("bootstrap"),
		AuthorizationCode: auth_code_bytes,
	}
	auth_code_encoded_bytes, err := auth_code.Encode()
	if err != nil {
		return nil, err
	}
	auth_code_hash := common.Blake2Hash(auth_code_encoded_bytes) //pu
	auth_code_hash_hash := common.Blake2Hash(auth_code_hash[:])  //pa
	auth_code_len := uint32(len(auth_code_encoded_bytes))
	for _, service := range services {

		code, err0 := types.ReadCodeWithMetadata(service.FileName, service.ServiceName)
		if err0 != nil {
			return nil, err0
		}

		var balance uint64 = uint64(10000000000)
		codeHash := common.Blake2Hash(code)
		codeLen := uint32(len(code)) // z
		bootStrapAnchor := []uint32{0}
		if service.ServiceCode == BootstrapServiceCode {
			bootstrapServiceAccount := types.ServiceAccount{
				CodeHash:        codeHash,
				Balance:         balance,
				GasLimitG:       100,
				GasLimitM:       100,
				StorageSize:     uint64(81*2 + codeLen + auth_code_len), // a_l = ∑ 81+z per (h,z) + ∑ 32+s https://graypaper.fluffylabs.dev/#/5f542d7/116e01116e01
				NumStorageItems: 2*2 + 0,                                //a_i = 2⋅∣al∣+∣as∣
			}

			statedb.WriteServicePreimageBlob(service.ServiceCode, code)
			statedb.WriteServicePreimageLookup(service.ServiceCode, codeHash, codeLen, bootStrapAnchor)

			statedb.WriteServicePreimageBlob(service.ServiceCode, auth_code_encoded_bytes)
			statedb.WriteServicePreimageLookup(service.ServiceCode, auth_code_hash, auth_code_len, bootStrapAnchor)

			statedb.writeService(service.ServiceCode, &bootstrapServiceAccount)
			fmt.Printf("Bootstrap Service %s (fn:%s), codeHash %s, codeLen=%d, anchor %v\n", service.ServiceName, service.FileName, codeHash.String(), codeLen, bootStrapAnchor)
		} else {
			sa := types.ServiceAccount{
				CodeHash:        codeHash,
				Balance:         balance,
				GasLimitG:       100,
				GasLimitM:       100,
				StorageSize:     uint64(81*1 + codeLen), // a_l = ∑ 81+z per (h,z) + ∑ 32+s https://graypaper.fluffylabs.dev/#/5f542d7/116e01116e01
				NumStorageItems: 2*1 + 0,                //a_i = 2⋅∣al∣+∣as∣
			}

			statedb.WriteServicePreimageBlob(service.ServiceCode, code)
			statedb.WriteServicePreimageLookup(service.ServiceCode, codeHash, codeLen, bootStrapAnchor)

			for k, v := range service.Storage {
				statedb.WriteServiceStorage(service.ServiceCode, k.Bytes(), v)
			}
			statedb.writeService(service.ServiceCode, &sa)

			fmt.Printf("Service %d %s (fn:%s), codeHash %s, codeLen=%d, anchor %v\n", service.ServiceCode, service.ServiceName, service.FileName, codeHash.String(), codeLen, bootStrapAnchor)
		}
	}

	// Flush service blobs to levelDB before calling UpdateTrieState
	log.Info(log.SDB, "Genesis: Flushing service blobs to levelDB")
	if _, err := statedb.trie.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush service blobs: %w", err)
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

	trace = &StateTransition{
		PreState: StateSnapshotRaw{
			StateRoot: common.Hash{},
			KeyVals:   make([]KeyVal, 0),
		},
		Block: types.Block{
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
		},
		PostState: StateSnapshotRaw{
			StateRoot: statedb.StateRoot,
			KeyVals:   statedb.GetAllKeyValues(),
		},
	}

	b := trace.Block
	statedb.Block = &b

	fmt.Printf("Genesis Header Hash: %s\n", b.Header.Hash().String())
	return trace, nil
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

func NewStateDBFromStateTransitionFile(sdb *storage.StateDBStorage, network string) (statedb *StateDB, err error) {
	//fn := common.GetFilePath(statefilename) // TODO: SHWAN THIS IS NOT REAL
	fn := common.GetFilePathForNetwork(network)
	snapshotRawBytes, err := os.ReadFile(fn)
	if err != nil {
		fmt.Printf("Error reading file %s: %v\n", fn, err)
		return statedb, err
	}
	var statetransition StateTransition
	err = json.Unmarshal(snapshotRawBytes, &statetransition)
	if err != nil {
		log.Crit(log.SDB, "Error unmarshalling state snapshot raw file", "error", err)
		return statedb, err
	}
	fmt.Printf("StateTransition: %s\n", statetransition.String())
	return NewStateDBFromStateTransition(sdb, &statetransition)
}

func IsGenesisSTF(statetransition *StateTransition) bool {
	if (statetransition != nil && statetransition.PreState.StateRoot == common.Hash{} && statetransition.Block.Header.Slot == 0 && statetransition.Block.Header.ParentStateRoot == common.Hash{}) {
		return true
	}
	return false
}

func NewStateDBFromStateTransitionPost(sdb *storage.StateDBStorage, statetransition *StateTransition) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}
	statedb.Block = &(statetransition.Block)
	statedb.StateRoot = statedb.UpdateAllTrieStateRaw(statetransition.PostState) // NOTE: MK -- USE POSTSTATE
	statedb.JamState = NewJamState()
	if err := statedb.InitTrieAndLoadJamState(statedb.StateRoot); err != nil {
		return nil, err
	}
	return statedb, nil
}

func NewStateDBFromStateKeyVals(sdb *storage.StateDBStorage, stateKeyVals *StateKeyVals) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}
	statedb.StateRoot = statedb.UpdateAllTrieKeyVals(*stateKeyVals)
	statedb.JamState = NewJamState()
	if err := statedb.InitTrieAndLoadJamState(statedb.StateRoot); err != nil {
		return nil, err
	}
	return statedb, nil
}

func NewStateDBFromStateTransition(sdb *storage.StateDBStorage, statetransition *StateTransition) (statedb *StateDB, err error) {
	t0 := time.Now()

	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}
	benchRec.Add("NewStateDBFromStateTransition:newStateDB", time.Since(t0))
	t0 = time.Now()
	statedb.Block = &(statetransition.Block)
	isGenesis := IsGenesisSTF(statetransition)
	if isGenesis && false {
		statetransition.PreState = statetransition.PostState // Allow genesis stf to use poststate as prestate for first non-genesis block
	}
	statedb.StateRoot = statedb.UpdateAllTrieStateRaw(statetransition.PreState) // NOTE: MK -- USE PRESTATE
	benchRec.Add("NewStateDBFromStateTransition:UpdateAllTrieStateRaw", time.Since(t0))
	t0 = time.Now()
	//fmt.Printf("NewStateDBFromStateTransition StateRoot: %s | isGenesis:%v\n", statedb.StateRoot.String(), isGenesis)
	if (statedb.StateRoot != statetransition.Block.Header.ParentStateRoot && statetransition.Block.Header.ParentStateRoot != common.Hash{}) {
		return nil, fmt.Errorf("StateRoot %s != ParentStateRoot %s", statedb.StateRoot.String(), statetransition.Block.Header.ParentStateRoot.String())
	}
	statedb.JamState = NewJamState()
	if err := statedb.InitTrieAndLoadJamState(statedb.StateRoot); err != nil {
		return nil, err
	}
	benchRec.Add("NewStateDBFromStateTransition:InitTrieAndLoadJamState", time.Since(t0))
	return statedb, nil
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
		log.Trace(log.SDB, "NewEpoch0Timestamp", "Raw now", uint64(now), "Raw waitTime", waitTime, "Raw epoch0P", epoch0Phase, "Raw epoch0Timestamp", epoch0Timestamp)

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
		log.Trace(log.SDB, "NewEpoch0Timestamp", "NewGenesisConfig epoch0Timestamp", epoch0Timestamp, "Wait", uint64(waitTime))
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
		log.Trace(log.SDB, "NewEpoch0Timestamp", "Raw now", uint64(now), "Raw waitTime", waitTime, "Raw epoch0P", epoch0Phase, "Raw epoch0Timestamp", epoch0Timestamp)
		return uint64(epoch0Timestamp)
	}
}

func InitValidator(bandersnatch_seed, ed25519_seed, bls_seed []byte, metadata []byte) (types.Validator, error) {
	validator := types.Validator{}
	banderSnatch_pub, _, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validator, fmt.Errorf("failed to init BanderSnatch Key")
	}
	ed25519_pub, _, err := types.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validator, fmt.Errorf("failed to init Ed25519 Key")
	}
	bls_pub, _, err := bls.InitBLSKey(bls_seed)
	if err != nil {
		return validator, fmt.Errorf("failed to init BanderSnatch Key")
	}

	validator.Ed25519 = ed25519_pub
	copy(validator.Bandersnatch[:], banderSnatch_pub.Bytes())
	copy(validator.Metadata[:], metadata)
	copy(validator.Bls[:], bls_pub.Bytes())
	return validator, nil
}

func InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed []byte, metadata []byte) (types.ValidatorSecret, error) {
	validatorSecret := types.ValidatorSecret{}
	banderSnatch_pub, banderSnatch_priv, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("failed to init BanderSnatch Key")
	}
	ed25519_pub, ed25519_priv, err := types.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("failed to init Ed25519 Key")
	}
	bls_pub, bls_priv, err := bls.InitBLSKey(bls_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("failed to init BLS Key")
	}

	copy(validatorSecret.Ed25519Secret[:], ed25519_priv[:])
	validatorSecret.Ed25519Pub = ed25519_pub

	validatorSecret.BandersnatchSecret = banderSnatch_priv.Bytes()
	validatorSecret.BandersnatchPub = types.BandersnatchKey(banderSnatch_pub)

	copy(validatorSecret.BlsSecret[:], bls_priv.Bytes())
	copy(validatorSecret.BlsPub[:], bls_pub.Bytes())

	copy(validatorSecret.Metadata[:], metadata)
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
		metadata := []byte{}
		//metadata, _ := generateMetadata(i) // this is NOT used by other teams. somehow we agreed on empty metadata for now

		validator, err := InitValidator(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("failed to init validator %v", i)
		}
		validators[i] = validator

		//bandersnatch_seed, ed25519_seed, bls_seed
		validatorSecret, err := InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("failed to init validator secret=%v", i)
		}
		validatorSecrets[i] = validatorSecret
	}

	return validators, validatorSecrets, nil
}
