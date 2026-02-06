package statedb

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"

	"os"
	"time"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/grandpa"
	log "github.com/jam-duna/jamduna/log"
	evmtypes "github.com/jam-duna/jamduna/types"
	storage "github.com/jam-duna/jamduna/storage"
	"github.com/jam-duna/jamduna/types"
)

const isIncludeEVM = true

// initializeOrchardStorage creates genesis storage for Orchard service
func initializeOrchardStorage() map[common.Hash][]byte {
	storage := make(map[common.Hash][]byte)

	// commitment_root = empty tree root (32 bytes)
	commitmentRootKey := common.BytesToHash([]byte("commitment_root"))
	commitmentRootValue := common.HexToHash("0x0dd677e24a166116d3f6274df39eaad8ebc47ed549b464042ef16a4164b98553").Bytes()
	storage[commitmentRootKey] = commitmentRootValue

	// commitment_size = 0 (u64, 8 bytes little-endian)
	commitmentSizeKey := common.BytesToHash([]byte("commitment_size"))
	commitmentSizeValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(commitmentSizeValue, 0)
	storage[commitmentSizeKey] = commitmentSizeValue

	// min_fee = 0 (u64, 8 bytes little-endian)
	minFeeKey := common.BytesToHash([]byte("min_fee"))
	minFeeValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(minFeeValue, 0)
	storage[minFeeKey] = minFeeValue

	// gas_min = 50,000 (u64, 8 bytes little-endian)
	gasMinKey := common.BytesToHash([]byte("gas_min"))
	gasMinValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(gasMinValue, 50_000)
	storage[gasMinKey] = gasMinValue

	// gas_max = 5,000,000 (u64, 8 bytes little-endian)
	gasMaxKey := common.BytesToHash([]byte("gas_max"))
	gasMaxValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(gasMaxValue, 5_000_000)
	storage[gasMaxKey] = gasMaxValue

	// fee_tally_0 = 0 (u128, 16 bytes little-endian)
	feeTallyKey := common.BytesToHash([]byte("fee_tally_0"))
	feeTallyValue := make([]byte, 16)
	// Already all zeros
	storage[feeTallyKey] = feeTallyValue

	return storage
}

func MakeGenesisStateTransition(sdb types.JAMStorage, epochFirstSlot uint64, network string, addresses []string) (trace *StateTransition, err error) {
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
		v, err0 := grandpa.InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
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

	// introduces some variation in the validator sets
	variantValidators := make(types.Validators, types.TotalValidators)
	// 0 -> 1, 1->2, ..., N-1 -> 0
	for i := 0; i < types.TotalValidators; i++ {
		variantValidators[i] = validators[(i+1)%types.TotalValidators]
	}
	var variantBool bool = false
	if variantBool {
		j.SafroleState.NextValidators = variantValidators
	} else {
		j.SafroleState.NextValidators = validators
	}
	// do it again for designated validators
	variantValidators2 := make(types.Validators, types.TotalValidators)
	// 0 -> 2, 1->3, ..., N-2 -> 0, N-1 -> 1
	for i := 0; i < types.TotalValidators; i++ {
		variantValidators2[i] = validators[(i+2)%types.TotalValidators]
	}
	if variantBool {
		j.SafroleState.DesignatedValidators = variantValidators2
	} else {
		j.SafroleState.DesignatedValidators = validators
	}

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

	// Setup EVM Service for all 3 privileges
	authQueueServiceID := [types.TotalCores]uint32{}
	for i := 0; i < types.TotalCores; i++ {
		authQueueServiceID[i] = AuthCopyServiceCode
	}
	j.PrivilegedServiceIndices.AuthQueueServiceID = authQueueServiceID
	j.PrivilegedServiceIndices.UpcomingValidatorsServiceID = EVMServiceCode // 0 - EVM service can call designate() -- TODO: change it to algo next
	j.PrivilegedServiceIndices.ManagerServiceID = EVMServiceCode
	// 0.7.1 introduces RegistrarServiceID
	j.PrivilegedServiceIndices.RegistrarServiceID = EVMServiceCode
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

	// Refine only allows authorized builders
	// Accumulate only allows increments on BLOCK_NUMBER_KEY when there is a timestamp greater than this
	// SubmitGenesisWorkPackage uses ReadStateWitness must submit extrinsics that match these storage entries
	storage[evmtypes.GetBlockNumberKey()] = evmtypes.SerializeBlockNumber(1)

	// Load services into genesis state
	services := []types.TestService{
		{
			ServiceCode: AuthCopyServiceCode,
			FileName:    AuthCopyServiceFile,
			ServiceName: "auth_copy",
		},
		{
			ServiceCode: AlgoServiceCode,
			FileName:    AlgoServiceFile,
			ServiceName: "algo",
		},
		{
			ServiceCode: FibServiceCode,
			FileName:    FibServiceFile,
			ServiceName: "fib",
		},
		{
			ServiceCode: OrchardServiceCode,
			FileName:    OrchardServiceFile,
			ServiceName: "orchard",
			// Note: Orchard uses string keys directly (not hashed)
			// Storage map uses Hash as key type, but we store the raw string bytes
			Storage: initializeOrchardStorage(),
		},
	}

	boostrapServiceIdx := FibServiceCode
	if isIncludeEVM {
		boostrapServiceIdx = EVMServiceCode
		services = append(services, types.TestService{
			ServiceCode: EVMServiceCode,
			FileName:    EVMServiceFile,
			ServiceName: "evm",
			Storage:     storage,
		})
	}

	for i, service := range services {
		code, err0 := types.ReadCodeWithMetadata(service.FileName, service.ServiceName)
		if err0 != nil {
			return nil, err0
		}
		services[i].CodeLen = uint32(len(code))
		services[i].CodeHash = common.Blake2Hash(code)
		service_account := common.ComputeC_is(service.ServiceCode)
		services[i].AccountKey = service_account.Bytes()[:31]
		ap_internal_key := common.Compute_preimageBlob_internal(common.Blake2Hash(code))
		account_preimage_hash := common.ComputeC_sh(service.ServiceCode, ap_internal_key)
		services[i].PreimageKey = account_preimage_hash.Bytes()[:31]
	}

	//fmt.Printf("Genesis Services:\n%+v\n", types.ToJSONHexIndent(services))

	auth_pvm, err0 := common.GetFilePath(BootStrapNullAuthFile)
	if err0 != nil {
		return nil, err0
	}
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
		fmt.Printf("Loading Service %d: %s (fn:%s), codeHash %s, codeLen=%d\n", service.ServiceCode, service.ServiceName, service.FileName, common.Blake2Hash(code).String(), len(code))

		var balance uint64 = uint64(10000000000)
		codeHash := common.Blake2Hash(code)
		codeLen := uint32(len(code)) // z
		bootStrapAnchor := []uint32{0}
		if service.ServiceCode == uint32(boostrapServiceIdx) {
			// Write preimage blobs and lookups first
			statedb.WriteServicePreimageBlob(service.ServiceCode, code)
			statedb.WriteServicePreimageLookup(service.ServiceCode, codeHash, codeLen, bootStrapAnchor)

			statedb.WriteServicePreimageBlob(service.ServiceCode, auth_code_encoded_bytes)
			statedb.WriteServicePreimageLookup(service.ServiceCode, auth_code_hash, auth_code_len, bootStrapAnchor)

			// Write staking balances to Bootstrap service storage
			for k, v := range service.Storage {
				statedb.WriteServiceStorage(service.ServiceCode, k.Bytes(), v)
			}

			numPreimageBlobs := uint32(2) // code + auth_code
			numStorageEntries := uint32(len(service.Storage))
			if service.ServiceCode == EVMServiceCode {
				// Initialize SSR key with global_depth=0 for ReadObject to work before first accumulate
				// Key: "SSR" (3 bytes raw), Value: single byte 0 (global_depth hint)
				// This allows GetTransactionReceipt and other ReadObject calls to work immediately
				ssrKey := []byte("SSR")
				statedb.WriteServiceStorage(service.ServiceCode, ssrKey, []byte{0})
				numStorageEntries++ // SSR key adds 1 storage entry
			}

			// Calculate storage size including staking balances
			// Each staking entry: 32 bytes (key) + 8 bytes (value) = 40 bytes
			stakingStorageSize := uint64(len(service.Storage) * (32 + 8))

			bootstrapServiceAccount := types.ServiceAccount{
				CodeHash:        codeHash,
				Balance:         balance,
				GasLimitG:       100,
				GasLimitM:       100,
				StorageSize:     uint64(81*numPreimageBlobs) + uint64(codeLen) + uint64(auth_code_len) + stakingStorageSize, // a_l = ∑ 81+z per (h,z) + ∑ 32+s https://graypaper.fluffylabs.dev/#/5f542d7/116e01116e01
				NumStorageItems: 2*numPreimageBlobs + numStorageEntries,                                                     //a_i = 2⋅∣al∣+∣as∣
			}

			statedb.writeService(service.ServiceCode, &bootstrapServiceAccount)
			//fmt.Printf("Service %s(%d) %x\n%v\n\n", service.ServiceName, service.ServiceCode, service.AccountKey, bootstrapServiceAccount.JsonString())
			//fmt.Printf("Service %s (fn:%s), codeHash %s, codeLen=%d, anchor %v, staking entries=%d\n", service.ServiceName, service.FileName, codeHash.String(), codeLen, bootStrapAnchor, len(service.Storage))
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
		}
	}
	// Flush service blobs

	// fmt.Printf("Bootstrap AuthorizationHash: %v\n", auth_code_hash_hash) //p_a
	// fmt.Printf("Bootstrap AuthorizationCodeHash: %v\n", auth_code_hash)  //p_u
	for idx := range j.AuthorizationQueue {
		for i := range j.AuthorizationQueue[idx] {
			j.AuthorizationQueue[idx][i] = auth_code_hash_hash
		}
	}
	// Initialize AuthorizationsPool with bootstrap authorizer for all cores
	for i := 0; i < types.TotalCores; i++ {
		j.AuthorizationsPool[i][0] = auth_code_hash_hash
	}
	statedb.JamState = j

	root := statedb.Flush()

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
			StateRoot: root,
			KeyVals:   statedb.sdb.GetAllKeyValues(),
		},
	}

	b := trace.Block
	statedb.Block = &b

	//fmt.Printf("Genesis Header Hash: %s\n", b.Header.Hash().String())
	return trace, nil
}
func NewBlockFromFile(blockfilename string) *types.Block {
	fn, err := common.GetFilePath(blockfilename)
	if err != nil {
		panic(fmt.Sprintf("failed to get file path for %s: %v", blockfilename, err))
	}
	expectedCodec, err := os.ReadFile(fn)
	if err != nil {
		panic(fmt.Sprintf("failed to read codec file %s: %v", fn, err))
	}
	r, _, err := types.Decode(expectedCodec, reflect.TypeOf(types.Block{}))
	if err != nil {
		fmt.Printf("%s - %v", fn, err)
		panic("failed to decode codec data")
	}
	b := r.(types.Block)
	return &b
}

func NewStateDBFromStateTransitionFile(sdb *storage.StorageHub, network string) (statedb *StateDB, err error) {
	fn, err := common.GetFilePathForNetwork(network)
	if err != nil {
		return statedb, fmt.Errorf("failed to get file path for network %s: %w", network, err)
	}
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

func NewStateDBFromStateTransitionPost(sdb *storage.StorageHub, statetransition *StateTransition) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}
	statedb.Block = &(statetransition.Block)
	statedb.StateRoot, err = statedb.UpdateAllTrieStateRaw(statetransition.PostState) // NOTE: MK -- USE POSTSTATE
	if err != nil {
		return nil, fmt.Errorf("UpdateAllTrieStateRaw failed: %w", err)
	}
	statedb.JamState = NewJamState()
	if err := statedb.InitTrieAndLoadJamState(statedb.StateRoot); err != nil {
		return nil, err
	}
	return statedb, nil
}

func NewStateDBFromStateKeyVals(sdb *storage.StorageHub, stateKeyVals *StateKeyVals) (statedb *StateDB, err error) {
	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}
	statedb.StateRoot, err = statedb.UpdateAllTrieKeyVals(*stateKeyVals)
	if err != nil {
		return nil, fmt.Errorf("UpdateAllTrieKeyVals failed: %w", err)
	}
	statedb.JamState = NewJamState()
	if err := statedb.InitTrieAndLoadJamState(statedb.StateRoot); err != nil {
		return nil, err
	}
	return statedb, nil
}

func NewStateDBFromStateTransition(sdb types.JAMStorage, statetransition *StateTransition) (statedb *StateDB, err error) {
	t0 := time.Now()

	statedb, err = newStateDB(sdb, common.Hash{})
	if err != nil {
		return statedb, err
	}
	benchRec.Add("NewStateDBFromStateTransition:newStateDB", time.Since(t0))
	t0 = time.Now()
	statedb.Block = &(statetransition.Block)
	isGenesis := IsGenesisSTF(statetransition)
	if isGenesis {
		statedb.StateRoot, err = statedb.UpdateAllTrieStateRaw(statetransition.PostState) // Use PostState for genesis to get the flushed state root
	} else {
		statedb.StateRoot, err = statedb.UpdateAllTrieStateRaw(statetransition.PreState) // NOTE: MK -- USE PRESTATE
	}
	if err != nil {
		return nil, fmt.Errorf("UpdateAllTrieStateRaw failed: %w", err)
	}
	benchRec.Add("NewStateDBFromStateTransition:UpdateAllTrieStateRaw", time.Since(t0))
	t0 = time.Now()
	//fmt.Printf("NewStateDBFromStateTransition StateRoot: %s | isGenesis:%v\n", statedb.StateRoot.String(), isGenesis)
	// if (statedb.StateRoot != statetransition.Block.Header.ParentStateRoot && statetransition.Block.Header.ParentStateRoot != common.Hash{}) {
	// 	return nil, fmt.Errorf("StateRoot %s != ParentStateRoot %s", statedb.StateRoot.String(), statetransition.Block.Header.ParentStateRoot.String())
	// }
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
