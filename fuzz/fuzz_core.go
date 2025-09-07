package fuzz

import (
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"sort"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
)

const (
	debugFuzz = false
)

// Error map for each mode
var ErrorMap = map[string][]error{
	"safrole": {
		jamerrors.ErrTBadTicketAttemptNumber,
		jamerrors.ErrTTicketAlreadyInState,
		jamerrors.ErrTTicketsBadOrder,
		jamerrors.ErrTBadRingProof,
		jamerrors.ErrTEpochLotteryOver,
		//jamerrors.ErrTTimeslotNotMonotonic,
	},

	"assurances": {
		jamerrors.ErrABadSignature,
		jamerrors.ErrABadValidatorIndex,
		jamerrors.ErrABadCore,
		jamerrors.ErrABadParentHash,
		//jamerrors.ErrAStaleReport,
		jamerrors.ErrADuplicateAssurer,
		jamerrors.ErrANotSortedAssurers,
	},

	"reports": {
		// jamerrors.ErrGBadCodeHash,
		// jamerrors.ErrGBadCoreIndex,
		// jamerrors.ErrGBadSignature,
		// jamerrors.ErrGCoreEngaged,
		// jamerrors.ErrGDependencyMissing,
		// jamerrors.ErrGDuplicatePackageTwoReports,
		// jamerrors.ErrGFutureReportSlot,
		// jamerrors.ErrGInsufficientGuarantees,
		// jamerrors.ErrGDuplicateGuarantors,
		// jamerrors.ErrGOutOfOrderGuarantee,
		// jamerrors.ErrGWorkReportGasTooHigh,
		// jamerrors.ErrGServiceItemTooLow,
		// jamerrors.ErrGBadValidatorIndex,
		// jamerrors.ErrGBadValidatorIndex,
		// jamerrors.ErrGWrongAssignment,
		// jamerrors.ErrGAnchorNotRecent,
		// jamerrors.ErrGBadBeefyMMRRoot,
		// jamerrors.ErrGBadServiceID,
		// jamerrors.ErrGBadStateRoot,
		// jamerrors.ErrGReportEpochBeforeLast,
		// jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks,
		// jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue,
		// jamerrors.ErrGCoreWithoutAuthorizer,
		// jamerrors.ErrGCoreUnexpectedAuthorizer,
	},

	/*
		"disputes": {
			jamerrors.ErrDNotSortedWorkReports,
			jamerrors.ErrDNotUniqueVotes,
			jamerrors.ErrDNotSortedValidVerdicts,
			jamerrors.ErrDNotHomogenousJudgements,
			jamerrors.ErrDMissingCulpritsBadVerdict,
			jamerrors.ErrDSingleCulpritBadVerdict,
			jamerrors.ErrDTwoCulpritsBadVerdictNotSorted,
			jamerrors.ErrDAlreadyRecordedVerdict,
			jamerrors.ErrDCulpritAlreadyInOffenders,
			jamerrors.ErrDOffenderNotPresentVerdict,
			jamerrors.ErrDMissingFaultsGoodVerdict,
			jamerrors.ErrDTwoFaultOffendersGoodVerdict,
			jamerrors.ErrDAlreadyRecordedVerdictWithFaults,
			jamerrors.ErrDFaultOffenderInOffendersList,
			jamerrors.ErrDAuditorMarkedOffender,
			jamerrors.ErrDBadSignatureInVerdict,
			jamerrors.ErrDBadSignatureInCulprits,
			jamerrors.ErrDAgeTooOldInVerdicts},
	*/
}

func (f *Fuzzer) Shuffle(slice interface{}) {
	if len(f.seed) == 0 {
		return
	}

	var seedInt64 int64
	if len(f.seed) >= 8 {
		seedInt64 = int64(binary.BigEndian.Uint64(f.seed[:8]))
	} else {
		paddedSeed := make([]byte, 8)
		copy(paddedSeed, f.seed)
		seedInt64 = int64(binary.BigEndian.Uint64(paddedSeed))
	}

	source := rand.NewSource(seedInt64)
	r := rand.New(source)

	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		log.Printf("Shuffle error: provided data of type %T is not a slice", slice)
		return
	}

	length := rv.Len()
	swap := reflect.Swapper(slice)

	r.Shuffle(length, func(i, j int) {
		swap(i, j)
	})
}

func NewRand(seed []byte) *rand.Rand {
	var seedInt int64
	if len(seed) >= 8 {
		// Deterministically convert the byte slice seed to an int64
		seedInt = int64(binary.BigEndian.Uint64(seed))
	} else {
		// Fallback for short seeds; less deterministic but still works
		seedInt = time.Now().UnixNano()
	}

	source := rand.NewSource(seedInt)
	return rand.New(source)
}

func StartFuzzingProducerWithOptions(fuzzer *Fuzzer, output chan<- StateTransitionQA, baseSTFs []*statedb.StateTransition, invalidRate float64, numBlocks int, disableShuffling bool) {
	defer close(output)

	// Sort baseSTFs by block slot if shuffling is disabled
	if disableShuffling {
		sort.Slice(baseSTFs, func(i, j int) bool {
			return baseSTFs[i].Block.Header.Slot < baseSTFs[j].Block.Header.Slot
		})
		log.Printf("FUZZER: Shuffling disabled - sorted %d STFs by block slot", len(baseSTFs))
	}

	modes := []string{"safrole", "assurances"}
	finalSTFs, err := fuzzer.FuzzWithTargetedInvalidRateWithOptions(modes, baseSTFs, invalidRate, numBlocks, disableShuffling)
	if err != nil {
		log.Printf("FUZZER: Failed to fuzz blocks: %v", err)
		return
	}

	for _, stfQA := range finalSTFs {
		output <- stfQA
	}
}

func StartFuzzingProducer(fuzzer *Fuzzer, output chan<- StateTransitionQA, baseSTFs []*statedb.StateTransition, invalidRate float64, numBlocks int) {
	defer close(output)
	rng := NewRand(fuzzer.GetSeed())

	store, err := statedb.InitStorage("/tmp/test_locala_producer")
	if err != nil {
		log.Printf("FUZZER: Failed to initialize storage, exiting: %v", err)
		return
	}

	numInvalidBlocks := int(float64(numBlocks) * invalidRate)
	numValidBlocks := numBlocks - numInvalidBlocks
	log.Printf("FUZZER: Starting. Target: %d invalid blocks, %d valid blocks.", numInvalidBlocks, numValidBlocks)

	for i := 0; i < numBlocks; i++ {
		if len(baseSTFs) == 0 {
			log.Println("FUZZER: No more base state transitions available.")
			return
		}

		remainingBlocks := numInvalidBlocks + numValidBlocks
		probInvalid := float64(numInvalidBlocks) / float64(remainingBlocks)

		if rng.Float64() < probInvalid {
			var fuzzedQA *StateTransitionQA
			for {
				stfIndex := rng.Intn(len(baseSTFs))
				baseSTF := baseSTFs[stfIndex]

				//modeList := []string{"safrole", "reports", "assurances"}
				modeList := []string{"safrole", "assurances"}
				mutatedResult, err := fuzzer.FuzzSingleStf(store, baseSTF, modeList)
				if err == nil {
					fuzzedQA = mutatedResult
					break
				}
			}
			output <- *fuzzedQA
			numInvalidBlocks--
		} else {
			stfIndex := rng.Intn(len(baseSTFs))
			baseSTF := baseSTFs[stfIndex]
			validQA := StateTransitionQA{
				Mutated: false,
				Error:   nil,
				STF:     baseSTF,
			}
			output <- validQA
			numValidBlocks--
		}
	}
	log.Println("FUZZER: Finished generating all blocks.")
}

func (f *Fuzzer) FuzzSingleStf(store *storage.StateDBStorage, stf_org *statedb.StateTransition, modes []string) (*StateTransitionQA, error) {
	stf := stf_org.DeepCopy()
	if stf == nil {
		return nil, fmt.Errorf("failed to copy STF")
	}
	possibleMutations := selectImportBlocksErrorArr(f.GetSeed(), store, modes, stf, true)
	if len(possibleMutations) == 0 {
		return nil, fmt.Errorf("the provided STF is not fuzzable with the given modes")
	}

	// Pick one of the possible mutations at random.
	rng := NewRand(f.GetSeed())
	chosenMutation := possibleMutations[rng.Intn(len(possibleMutations))]

	resultQA := &StateTransitionQA{
		Mutated: true,
		Error:   chosenMutation.Error,
		STF:     chosenMutation.StateTransition,
	}

	return resultQA, nil
}

func (f *Fuzzer) FuzzWithTargetedInvalidRateWithOptions(modes []string, stfs []*statedb.StateTransition, invalidRate float64, numBlocks int, disableShuffling bool) (finalSTFs []StateTransitionQA, err error) {
	allowFuzzing := true
	seed := f.GetSeed()
	store, err := statedb.InitStorage("/tmp/test_locala")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %v", err)
	}

	numInvalidBlocks := int(float64(numBlocks) * invalidRate)
	numValidBlocks := numBlocks - numInvalidBlocks
	if numInvalidBlocks == 0 {
		allowFuzzing = false
	}
	fmt.Printf("Seed=%x InvalidRate=%.2f -> invalidBlocks=%d | validBlocks=%d | total=%d\n",
		seed, invalidRate, numInvalidBlocks, numValidBlocks, numBlocks,
	)

	if !disableShuffling {
		f.Shuffle(stfs)
	}

	var allPossibleMutations []STFError
	var validBlockPool []*statedb.StateTransition
	validateInput := false
	for _, stf := range stfs {
		if validateInput {
			diffs, stfErr := statedb.CheckStateTransitionWithOutput(store, stf, nil, f.pvmBackend, false)
			if stfErr != nil {
				statedb.HandleDiffs(diffs)
				return nil, fmt.Errorf("invalid base STF provided: %v | %v", stfErr, stf.ToJSON())
			}
		}

		validBlockPool = append(validBlockPool, stf)
		mutations := selectImportBlocksErrorArr(seed, store, modes, stf, allowFuzzing)
		if len(mutations) > 0 {
			allPossibleMutations = append(allPossibleMutations, mutations...)
		}
	}

	if numInvalidBlocks > 0 && len(allPossibleMutations) == 0 {
		log.Println("No STFs are fuzzable to generate the required invalid blocks.")
		return nil, fmt.Errorf("no STFs are fuzzable")
	}

	var fuzzedSTFs []StateTransitionQA
	if numInvalidBlocks > 0 {
		if !disableShuffling {
			f.Shuffle(allPossibleMutations)
		}
		for i := 0; i < numInvalidBlocks; i++ {
			mutation := allPossibleMutations[i%len(allPossibleMutations)]
			fuzzedSTFs = append(fuzzedSTFs, StateTransitionQA{
				Mutated: true,
				Error:   mutation.Error, // The specific error for this mutation
				STF:     mutation.StateTransition,
			})
		}
	}

	var finalValidSTFs []StateTransitionQA
	if numValidBlocks > 0 {
		if len(validBlockPool) == 0 {
			return nil, fmt.Errorf("cannot generate valid blocks: no source STFs available")
		}
		if !disableShuffling {
			f.Shuffle(validBlockPool)
		}
		for i := 0; i < numValidBlocks; i++ {
			stf := validBlockPool[i%len(validBlockPool)]
			finalValidSTFs = append(finalValidSTFs, StateTransitionQA{
				Mutated: false,
				Error:   nil,
				STF:     stf,
			})
		}
	}

	finalSTFs = append(fuzzedSTFs, finalValidSTFs...)
	if !disableShuffling {
		f.Shuffle(finalSTFs)
	}

	log.Printf("Fuzz completed: %d invalid blocks, %d valid blocks", len(fuzzedSTFs), len(finalValidSTFs))
	return finalSTFs, nil
}

func (f *Fuzzer) FuzzWithTargetedInvalidRate(modes []string, stfs []*statedb.StateTransition, invalidRate float64, numBlocks int) (finalSTFs []StateTransitionQA, err error) {
	allowFuzzing := true
	seed := f.GetSeed()
	store, err := statedb.InitStorage("/tmp/test_locala")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %v", err)
	}

	numInvalidBlocks := int(float64(numBlocks) * invalidRate)
	numValidBlocks := numBlocks - numInvalidBlocks
	if numInvalidBlocks == 0 {
		allowFuzzing = false
	}
	fmt.Printf("Seed=%x InvalidRate=%.2f -> invalidBlocks=%d | validBlocks=%d | total=%d\n",
		seed, invalidRate, numInvalidBlocks, numValidBlocks, numBlocks,
	)

	f.Shuffle(stfs)

	var allPossibleMutations []STFError
	var validBlockPool []*statedb.StateTransition
	validateInput := false
	for _, stf := range stfs {
		if validateInput {
			diffs, stfErr := statedb.CheckStateTransitionWithOutput(store, stf, nil, f.pvmBackend, false)
			if stfErr != nil {
				statedb.HandleDiffs(diffs)
				return nil, fmt.Errorf("invalid base STF provided: %v | %v", stfErr, stf.ToJSON())
			}
		}

		validBlockPool = append(validBlockPool, stf)
		mutations := selectImportBlocksErrorArr(seed, store, modes, stf, allowFuzzing)
		if len(mutations) > 0 {
			allPossibleMutations = append(allPossibleMutations, mutations...)
		}
	}

	if numInvalidBlocks > 0 && len(allPossibleMutations) == 0 {
		log.Println("No STFs are fuzzable to generate the required invalid blocks.")
		return nil, fmt.Errorf("no STFs are fuzzable")
	}

	var fuzzedSTFs []StateTransitionQA
	if numInvalidBlocks > 0 {
		f.Shuffle(allPossibleMutations)
		for i := 0; i < numInvalidBlocks; i++ {
			mutation := allPossibleMutations[i%len(allPossibleMutations)]
			fuzzedSTFs = append(fuzzedSTFs, StateTransitionQA{
				Mutated: true,
				Error:   mutation.Error, // The specific error for this mutation
				STF:     mutation.StateTransition,
			})
		}
	}

	var finalValidSTFs []StateTransitionQA
	if numValidBlocks > 0 {
		if len(validBlockPool) == 0 {
			return nil, fmt.Errorf("cannot generate valid blocks: no source STFs available")
		}
		f.Shuffle(validBlockPool)
		for i := 0; i < numValidBlocks; i++ {
			stf := validBlockPool[i%len(validBlockPool)]
			finalValidSTFs = append(finalValidSTFs, StateTransitionQA{
				Mutated: false,
				Error:   nil,
				STF:     stf,
			})
		}
	}

	finalSTFs = append(fuzzedSTFs, finalValidSTFs...)
	f.Shuffle(finalSTFs)

	log.Printf("Fuzz completed: %d invalid blocks, %d valid blocks", len(fuzzedSTFs), len(finalValidSTFs))
	return finalSTFs, nil
}

// NewSeededRand creates a new deterministic random number generator based on a byte slice seed.
func NewSeededRand(seed []byte) *rand.Rand {
	var seedInt64 int64
	if len(seed) >= 8 {
		seedInt64 = int64(binary.BigEndian.Uint64(seed[:8]))
	} else {
		paddedSeed := make([]byte, 8)
		copy(paddedSeed, seed)
		seedInt64 = int64(binary.BigEndian.Uint64(paddedSeed))
	}
	source := rand.NewSource(seedInt64)
	return rand.New(source)
}

func possibleError(seed []byte, selectedError error, block *types.Block, s *statedb.StateDB, validatorSecrets []types.ValidatorSecret) error {
	// Dispatch to the appropriate fuzzBlock function based on the selected error
	switch selectedError {

	// safrole errors
	case jamerrors.ErrTBadTicketAttemptNumber:
		return fuzzBlockTBadTicketAttemptNumber(seed, block)
	case jamerrors.ErrTTicketAlreadyInState:
		return fuzzBlockTTicketAlreadyInState(seed, block, s, validatorSecrets)
	case jamerrors.ErrTTicketsBadOrder:
		return fuzzBlockTTicketsBadOrder(seed, block)
	case jamerrors.ErrTBadRingProof:
		return fuzzBlockTBadRingProof(seed, block)
	case jamerrors.ErrTEpochLotteryOver:
		return fuzzBlockTEpochLotteryOver(seed, block, s)
	case jamerrors.ErrTTimeslotNotMonotonic:
		return fuzzBlockTTimeslotNotMonotonic(seed, block, s)

	// reports errors
	case jamerrors.ErrGBadCodeHash:
		return fuzzBlockGBadCodeHash(seed, block)
	case jamerrors.ErrGBadCoreIndex:
		return fuzzBlockGBadCoreIndex(seed, block)
	case jamerrors.ErrGBadSignature:
		return fuzzBlockGBadSignature(seed, block)
	case jamerrors.ErrGCoreEngaged:
		return fuzzBlockGCoreEngaged(seed, block, s)
	case jamerrors.ErrGDependencyMissing:
		return fuzzBlockGDependencyMissing(seed, block, s)
	case jamerrors.ErrGDuplicatePackageTwoReports:
		return fuzzBlockGDuplicatePackageTwoReports(seed, block, s)
	case jamerrors.ErrGFutureReportSlot:
		return fuzzBlockGFutureReportSlot(seed, block)
	case jamerrors.ErrGInsufficientGuarantees:
		return fuzzBlockGInsufficientGuarantees(seed, block)
	case jamerrors.ErrGDuplicateGuarantors:
		return fuzzBlockGDuplicateGuarantors(seed, block)
	case jamerrors.ErrGOutOfOrderGuarantee:
		return fuzzBlockGOutOfOrderGuarantee(seed, block)
	case jamerrors.ErrGWorkReportGasTooHigh:
		return fuzzBlockGWorkReportGasTooHigh(seed, block)
	case jamerrors.ErrGServiceItemTooLow:
		return fuzzBlockGServiceItemTooLow(seed, block, s)
	case jamerrors.ErrGBadValidatorIndex:
		return fuzzBlockGBadValidatorIndex(seed, block)
	case jamerrors.ErrGWrongAssignment:
		return fuzzBlockGWrongAssignment(seed, block, s)
	case jamerrors.ErrGAnchorNotRecent:
		return fuzzBlockGAnchorNotRecent(seed, block, s)
	case jamerrors.ErrGBadBeefyMMRRoot:
		return fuzzBlockGBadBeefyMMRRoot(seed, block, s)
	case jamerrors.ErrGBadServiceID:
		return fuzzBlockGBadServiceID(seed, block, s)
	case jamerrors.ErrGBadStateRoot:
		return fuzzBlockGBadStateRoot(seed, block, s)
	case jamerrors.ErrGReportEpochBeforeLast:
		return fuzzBlockGReportEpochBeforeLast(seed, block, s)
	case jamerrors.ErrGDuplicatePackageRecentHistory:
		return fuzzBlockGDuplicatePackageRecentHistory(seed, block, s)
	case jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks:
		return fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks(seed, block)
	case jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue:
		return fuzzBlockGSegmentRootLookupInvalidUnexpectedValue(seed, block)
	case jamerrors.ErrGCoreWithoutAuthorizer:
		return fuzzBlockGCoreWithoutAuthorizer(seed, block, s)
	case jamerrors.ErrGCoreUnexpectedAuthorizer:
		return fuzzBlockGCoreUnexpectedAuthorizer(seed, block, s)

	// assurances errors
	case jamerrors.ErrABadSignature:
		return fuzzBlockABadSignature(seed, block)
	case jamerrors.ErrABadValidatorIndex:
		return fuzzBlockABadValidatorIndex(seed, block)
	case jamerrors.ErrABadCore:
		return fuzzBlockABadCore(seed, block, s, validatorSecrets)
	case jamerrors.ErrABadParentHash:
		return fuzzBlockABadParentHash(seed, block, validatorSecrets)
	case jamerrors.ErrAStaleReport:
		return fuzzBlockAStaleReport(seed, block, s)
	case jamerrors.ErrADuplicateAssurer:
		return fuzzBlockADuplicateAssurer(seed, block, s, validatorSecrets)
	case jamerrors.ErrANotSortedAssurers:
		return fuzzBlockANotSortedAssurers(seed, block, s, validatorSecrets)

	// disputes errors
	case jamerrors.ErrDNotSortedWorkReports:
		return fuzzBlockDNotSortedWorkReports(seed, block)
	case jamerrors.ErrDNotUniqueVotes:
		return fuzzBlockDNotUniqueVotes(seed, block)
	case jamerrors.ErrDNotSortedValidVerdicts:
		return fuzzBlockDNotSortedValidVerdicts(seed, block)
	case jamerrors.ErrDNotHomogenousJudgements:
		return fuzzBlockDNotHomogenousJudgements(seed, block)
	case jamerrors.ErrDMissingCulpritsBadVerdict:
		return fuzzBlockDMissingCulpritsBadVerdict(seed, block)
	case jamerrors.ErrDSingleCulpritBadVerdict:
		return fuzzBlockDSingleCulpritBadVerdict(seed, block)
	case jamerrors.ErrDTwoCulpritsBadVerdictNotSorted:
		return fuzzBlockDTwoCulpritsBadVerdictNotSorted(seed, block)
	case jamerrors.ErrDAlreadyRecordedVerdict:
		return fuzzBlockDAlreadyRecordedVerdict(seed, block)
	case jamerrors.ErrDCulpritAlreadyInOffenders:
		return fuzzBlockDCulpritAlreadyInOffenders(seed, block)
	case jamerrors.ErrDOffenderNotPresentVerdict:
		return fuzzBlockDOffenderNotPresentVerdict(seed, block)
	case jamerrors.ErrDMissingFaultsGoodVerdict:
		return fuzzBlockDMissingFaultsGoodVerdict(seed, block)
	case jamerrors.ErrDTwoFaultOffendersGoodVerdict:
		return fuzzBlockDTwoFaultOffendersGoodVerdict(seed, block)
	case jamerrors.ErrDAlreadyRecordedVerdictWithFaults:
		return fuzzBlockDAlreadyRecordedVerdictWithFaults(seed, block)
	case jamerrors.ErrDFaultOffenderInOffendersList:
		return fuzzBlockDFaultOffenderInOffendersList(seed, block)
	case jamerrors.ErrDAuditorMarkedOffender:
		return fuzzBlockDAuditorMarkedOffender(seed, block)
	case jamerrors.ErrDBadSignatureInVerdict:
		return fuzzBlockDBadSignatureInVerdict(seed, block)
	case jamerrors.ErrDBadSignatureInCulprits:
		return fuzzBlockDBadSignatureInCulprits(seed, block)
	case jamerrors.ErrDAgeTooOldInVerdicts:
		return fuzzBlockDAgeTooOldInVerdicts(seed, block)

	default:
		return nil
	}
}

func selectAllImportBlocksErrors(seed []byte, store *storage.StateDBStorage, modes []string, stf *statedb.StateTransition, allowFuzzing bool) (oSlot uint32, oEpoch int32, oPhase uint32, mutated_STFs []statedb.StateTransition, fuzzable_errors []error) {
	var aggregatedErrors []error
	var mutatedSTFs []statedb.StateTransition
	block := stf.Block
	sdb, err := statedb.NewStateDBFromStateTransition(store, stf)
	if err != nil {
		return 0, 0, 0, nil, nil
	}
	//jsonOutput, _ := json.MarshalIndent(stf, "", "  ")
	//fmt.Printf("stf!! %v\n", stf.ToJSON())

	for _, mode := range modes {
		if mode == "safrole" && len(block.Extrinsic.Tickets) == 0 {
			continue
		}
		if mode == "reports" && len(block.Extrinsic.Guarantees) == 0 {
			continue
		}
		if mode == "assurances" && len(block.Extrinsic.Assurances) == 0 {
			continue
		}
		if mode == "preimages" && len(block.Extrinsic.Preimages) == 0 {
			continue
		}

		// TODO: add disputes filter
		if errorsForMode, exists := ErrorMap[mode]; exists {
			aggregatedErrors = append(aggregatedErrors, errorsForMode...)
		}
	}

	errorList := make([]error, 0)
	expectedNumNodes := 6
	validators, validatorSecrets, err := statedb.GenerateValidatorSecretSet(expectedNumNodes)
	//TODO: extract out validatorSet from stf.PreState and make sure sure validatorSecrets and validators are equal in size, opposed to hardcode numNodes
	if len(validatorSecrets) != expectedNumNodes || len(validators) != expectedNumNodes || err != nil {
		fmt.Printf("Invalid V(TotalValidators) | Expected=%v Found=%v\n", expectedNumNodes, len(validatorSecrets))
		return 0, 0, 0, nil, nil
	}

	// Create STF copy for original block
	oStatedbCopy := sdb.Copy()
	oBlockCopy := block.Copy()
	oSlot = oBlockCopy.TimeSlot()
	//oHeader := oBlockCopy.GetHeader()
	//oTicketExts := oBlockCopy.Tickets()
	oEpoch, oPhase = oStatedbCopy.GetSafrole().EpochAndPhase(oSlot)

	// Make sure original block passes seal test: which requires author guessing, entropy, attempt for passing
	oValid, oValidatorIdx, oValidatorPub, err := oStatedbCopy.VerifyBlockHeader(oBlockCopy, nil)
	if !oValid || err != nil || oBlockCopy.Header.AuthorIndex != oValidatorIdx {
		if allowFuzzing {
			//panic(fmt.Sprintf("Original block failed seal test: %v | %v | %v\n", oValid, err, oBlockCopy.Header.AuthorIndex))
			fmt.Printf("Original block failed seal test: %v | %v | %v\n", oValid, err, oBlockCopy.Header.AuthorIndex)
			return oSlot, oEpoch, oPhase, nil, nil
		}
	}

	if !allowFuzzing {
		if debugFuzz {
			fmt.Printf("[#%v e=%v,m=%03d] %sSkip Fuzzing%s  Author: %v (Idx:%v)\n", oSlot, oEpoch, oPhase, common.ColorGray, common.ColorReset, oValidatorPub, oValidatorIdx)
		}
		return oSlot, oEpoch, oPhase, nil, nil
	}

	if len(aggregatedErrors) == 0 {
		if debugFuzz {
			fmt.Printf("[#%v e=%v,m=%03d] %sNotFuzzable%s  Author: %v (Idx:%v)\n", oSlot, oEpoch, oPhase, common.ColorGray, common.ColorReset, oValidatorPub, oValidatorIdx)
		}
		return oSlot, oEpoch, oPhase, nil, nil
	}

	if debugFuzz {
		fmt.Printf("[#%v e=%v,m=%03d] %sFuzzable   %s  Author: %v (Idx:%v)\n", oSlot, oEpoch, oPhase, common.ColorMagenta, common.ColorReset, oValidatorPub, oValidatorIdx)
	}

	for _, selectedError := range aggregatedErrors {
		blockCopy := block.Copy()
		statedbCopy := sdb.Copy()
		// header := blockCopy.GetHeader()
		// slot := blockCopy.TimeSlot()
		// ticketExts := blockCopy.Tickets()
		stfErrExpected := possibleError(seed, selectedError, blockCopy, statedbCopy, validatorSecrets)

		var sealerUnknown bool
		switch selectedError {
		case jamerrors.ErrTEpochLotteryOver, jamerrors.ErrTTimeslotNotMonotonic:
			sealerUnknown = true
		default:
			sealerUnknown = false
		}
		if stfErrExpected == nil {
			continue
		} else {
			mSealedBlkFinal := types.Block{}
			if !sealerUnknown {
				// TODO: Step 0: Sealing each individual object????

				// Step 1: re-seal here. Need to retrieve ValidatorSecret from validatorIdx
				credential := validatorSecrets[blockCopy.Header.AuthorIndex]
				block_author_ietf_priv, err := statedb.ConvertBanderSnatchSecret(credential.BandersnatchSecret)
				if err != nil {
					continue
				}
				block_author_ietf_pub, err := statedb.ConvertBanderSnatchPub(credential.BandersnatchPub[:])
				if err != nil {
					continue
				}
				mSealedBlk, sealErr := statedbCopy.SealBlockWithEntropy(block_author_ietf_pub, block_author_ietf_priv, blockCopy.Header.AuthorIndex, blockCopy.TimeSlot(), blockCopy)
				if sealErr != nil {
					fmt.Printf("Fuzzing failed to seal block!!!\n")
					continue
				}

				// Step 2: make sure it passes re-seal test again..
				//msf := statedbCopy.GetSafrole()
				//ms2, err := msf.ApplyStateTransitionTickets(context.TODO(), ticketExts, slot, header, nil) // Entropy computed!
				mValid, mValidatorIdx, mValidatorPub, err := statedbCopy.VerifyBlockHeader(mSealedBlk, nil)
				if !mValid || err != nil {
					if debugFuzz {
						fmt.Printf("Mutated block failed seal test!!! %v | %v | %v\n", mValid, err, mSealedBlk.Header.AuthorIndex)
						//panic(fmt.Sprintf("mutated block failed seal entropy test failed: %v |  mValidatorIdx=%v | mValidatorPub=%v | err: %v\n", mValid, mValidatorIdx, mValidatorPub, err))
					}
					continue
				}

				if mValidatorIdx != oValidatorIdx && mSealedBlk.TimeSlot() == oBlockCopy.TimeSlot() {
					fmt.Printf("Validator changed unexpectedly!!! Original=%x (Idx:%v) | Mutated=%x (Idx:%v)\n", oValidatorPub, oValidatorIdx, mValidatorPub, mValidatorIdx)
					//continue
				}
				mSealedBlkFinal = *mSealedBlk
			}
			if sealerUnknown {
				// have to brute force the author index ...
				for authorIndex := 0; authorIndex < len(validatorSecrets); authorIndex++ {
					// TODO: Step 0: Sealing each individual object????

					// Step 1: re-seal here. Need to retrieve ValidatorSecret from validatorIdx
					blockCopy.Header.AuthorIndex = uint16(authorIndex)
					credential := validatorSecrets[blockCopy.Header.AuthorIndex]
					block_author_ietf_priv, err := statedb.ConvertBanderSnatchSecret(credential.BandersnatchSecret)
					if err != nil {
						continue
					}
					block_author_ietf_pub, err := statedb.ConvertBanderSnatchPub(credential.BandersnatchPub[:])
					if err != nil {
						continue
					}
					mSealedBlk, sealErr := statedbCopy.SealBlockWithEntropy(block_author_ietf_pub, block_author_ietf_priv, blockCopy.Header.AuthorIndex, blockCopy.TimeSlot(), blockCopy)
					if sealErr != nil {
						continue
					}

					// Step 2: make sure it passes re-seal test again..
					//msf := statedbCopy.GetSafrole()
					//ms2, err := msf.ApplyStateTransitionTickets(context.TODO(), ticketExts, slot, header, nil) // Entropy computed!
					mValid, _, _, err := statedbCopy.VerifyBlockHeader(mSealedBlk, nil)
					if !mValid || err != nil {
						continue
					} else {
						mSealedBlkFinal = *mSealedBlk
						break
					}
				}

			}

			// Step 3: Constructe mutated state transition
			stfMutated := statedb.StateTransition{
				PreState:  stf.PreState,
				Block:     mSealedBlkFinal,
				PostState: stf.PreState,
			}

			// TODO: need ancestorSet
			stfErrActual := statedb.CheckStateTransition(store, &stfMutated, nil, statedb.BackendInterpreter)
			if stfErrActual == stfErrExpected || true {
				errorList = append(errorList, stfErrExpected)
				mutatedSTFs = append(mutatedSTFs, stfMutated)
			} else {
				fmt.Printf("[#%v e=%v,m=%03d] Fuzzed Failed!!  Actual: \033[32m%v\033[0m  | Expected:%v\n", oSlot, oEpoch, oPhase, jamerrors.GetErrorName(stfErrActual), jamerrors.GetErrorName(stfErrExpected))
				if jamerrors.GetErrorName(stfErrActual) == "BadSignature" {
				}
				if jamerrors.GetErrorName(stfErrActual) == "BadValidatorIndex" {
				}
			}
		}
	}
	// pick a random error based on our success
	if len(errorList) > 0 {
		possibleErrs := errorList
		if debugFuzz || true {
			fmt.Printf("[#%v e=%v,m=%03d] %v possible mutations = %s%v%s\n", oSlot, oEpoch, oPhase, len(possibleErrs), common.ColorMagenta, jamerrors.GetErrorNames(possibleErrs), common.ColorReset)
		}
		return oSlot, oEpoch, oPhase, mutatedSTFs, possibleErrs
	}
	return oSlot, oEpoch, oPhase, nil, nil

}

type STFError struct {
	StateTransition *statedb.StateTransition
	Error           error
	Errors          []error
}

func selectImportBlocksErrorArr(seed []byte, store *storage.StateDBStorage, modes []string, stf *statedb.StateTransition, allowFuzzing bool) (stfErrors []STFError) {
	oSlot, oEpoch, oPhase, mutatedSTFs, errorList := selectAllImportBlocksErrors(seed, store, modes, stf, allowFuzzing)
	if len(mutatedSTFs) == 0 {
		return nil
	}
	stfErrors = make([]STFError, 0, len(mutatedSTFs))
	for i := 0; i < len(mutatedSTFs); i++ {
		stfErrors = append(stfErrors, STFError{
			StateTransition: &mutatedSTFs[i],
			Error:           errorList[i],
			Errors:          errorList,
		})
	}
	debug := false
	if debug {
		fmt.Printf("[#%v e=%v,m=%03d] Found %v possible mutations = %v\n", oSlot, oEpoch, oPhase, len(errorList), jamerrors.GetErrorNames(errorList))
	}
	return stfErrors
}
func (f *Fuzzer) ValidateStateTransitionChallengeResponse(stfQA *StateTransitionQA, stfResp *StateTransitionResponse) (isMatch bool, validationErr error) {
	return validateStateTransitionChallengeResponse(f.store, stfQA, stfResp)
}

func validateStateTransitionChallengeResponse(db *storage.StateDBStorage, stfQA *StateTransitionQA, stfResp *StateTransitionResponse) (isMatch bool, validationErr error) {
	challengerFuzzed := stfQA.Mutated
	var challengerErrorMsg *string
	if stfQA.Error != nil {
		errStr := stfQA.Error.Error()
		challengerErrorMsg = &errStr
	}
	solverFuzzed := stfResp.Mutated
	var solverErrorMsg *string
	if stfResp.JamError != nil {
		errStr := stfResp.JamError.Error
		solverErrorMsg = &errStr
	}

	if challengerFuzzed && !solverFuzzed {
		// Fuzzed blocks that were missed
		return false, fmt.Errorf("FuzzFalseNegatives")
	}
	if !challengerFuzzed && solverFuzzed {
		// Original blocks wrongly flagged
		return false, fmt.Errorf("OrigFalsePositives")
	}
	if challengerFuzzed && solverFuzzed {
		if challengerErrorMsg != solverErrorMsg {
			// TODO: kinda hard to get this level of errorMatch without jamErrorCode - omit for now
		}
		return true, nil
	}
	if !challengerFuzzed && !solverFuzzed {
		// Check STF Root & KeyVal Match
		challengerPostState := &stfQA.STF.PostState
		solverPostState := stfResp.PostState

		challenerTree, challenerTreeErr := lowlevelTrieInit(db, challengerPostState)
		if challenerTreeErr != nil {
			return false, fmt.Errorf("challergerTrieErr")
		}
		solverTree, solverTreeErr := lowlevelTrieInit(db, solverPostState)
		if solverTreeErr != nil {
			return false, fmt.Errorf("solverTrieErr")
		}
		if challenerTree.GetRoot() != solverTree.GetRoot() {
			return false, fmt.Errorf("ChallengerSolver Trie Mismatch. C=%v S=%v", challenerTree.GetRoot(), solverTree.GetRoot())
		}
		return true, nil
	}
	return false, nil
}

func lowlevelTrieInit(db *storage.StateDBStorage, snapshotRaw *statedb.StateSnapshotRaw) (*trie.MerkleTree, error) {
	expectedRoot := snapshotRaw.StateRoot
	tree := trie.NewMerkleTree(nil, db)
	for _, kv := range snapshotRaw.KeyVals {
		tree.SetRawKeyVal(kv.Key, kv.Value)
	}
	actualRoot := tree.GetRoot()
	if (expectedRoot != common.Hash{}) && expectedRoot != actualRoot {
		return nil, fmt.Errorf("root mismatch: expected=%v actual=%v", expectedRoot, actualRoot)
	}
	return tree, nil
}
