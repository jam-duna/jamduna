package main

import (
	//"errors"

	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
)

// Error map for each mode
var ErrorMap = map[string][]error{
	"safrole": {
		jamerrors.ErrTBadTicketAttemptNumber,
		jamerrors.ErrTTicketAlreadyInState,
		jamerrors.ErrTTicketsBadOrder,
		jamerrors.ErrTBadRingProof,
		jamerrors.ErrTEpochLotteryOver,
		jamerrors.ErrTTimeslotNotMonotonic,
	},
	"reports": {
		jamerrors.ErrGBadCodeHash,
		jamerrors.ErrGBadCoreIndex,
		jamerrors.ErrGBadSignature,
		jamerrors.ErrGCoreEngaged,
		jamerrors.ErrGDependencyMissing, // Michael
		jamerrors.ErrGDuplicatePackageTwoReports,
		jamerrors.ErrGFutureReportSlot,
		jamerrors.ErrGInsufficientGuarantees,
		jamerrors.ErrGDuplicateGuarantors,
		jamerrors.ErrGOutOfOrderGuarantee,
		jamerrors.ErrGWorkReportGasTooHigh, // Michael
		jamerrors.ErrGServiceItemTooLow,    // Michael
		jamerrors.ErrGBadValidatorIndex,
		jamerrors.ErrGBadValidatorIndex,
		jamerrors.ErrGWrongAssignment,
		jamerrors.ErrGAnchorNotRecent, // Michael
		jamerrors.ErrGBadBeefyMMRRoot, // Michael
		jamerrors.ErrGBadServiceID,
		jamerrors.ErrGBadStateRoot,                            // Michael
		jamerrors.ErrGReportEpochBeforeLast,                   // Michael
		jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks, // Michael
		jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue, // Michael
		jamerrors.ErrGCoreWithoutAuthorizer,                   // Michael
		jamerrors.ErrGCoreUnexpectedAuthorizer,                // Michael
	},
	"assurances": {jamerrors.ErrABadSignature,
		jamerrors.ErrABadValidatorIndex,
		jamerrors.ErrABadCore,
		jamerrors.ErrABadParentHash,
		jamerrors.ErrAStaleReport, // Michael
	},
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
}

func InitFuzzStorage(testDir string) (*storage.StateDBStorage, error) {
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err = os.MkdirAll(testDir, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("Failed to create directory /tmp/fuzz: %v", err)
		}
	}

	sdb_storage, err := storage.NewStateDBStorage(testDir)
	if err != nil {
		return nil, fmt.Errorf("Error with storage: %v", err)
	}
	return sdb_storage, nil

}

func possibleError(selectedError error, block *types.Block, s *statedb.StateDB, validatorSecrets []types.ValidatorSecret) error {
	// Dispatch to the appropriate fuzzBlock function based on the selected error
	switch selectedError {

	// safrole errors
	case jamerrors.ErrTBadTicketAttemptNumber:
		return fuzzBlockTBadTicketAttemptNumber(block)
	case jamerrors.ErrTTicketAlreadyInState:
		return fuzzBlockTTicketAlreadyInState(block, s)
	case jamerrors.ErrTTicketsBadOrder:
		return fuzzBlockTTicketsBadOrder(block)
	case jamerrors.ErrTBadRingProof:
		return fuzzBlockTBadRingProof(block)
	case jamerrors.ErrTEpochLotteryOver:
		return fuzzBlockTEpochLotteryOver(block, s)
	case jamerrors.ErrTTimeslotNotMonotonic:
		return fuzzBlockTTimeslotNotMonotonic(block)

	// reports errors
	case jamerrors.ErrGBadCodeHash:
		return fuzzBlockGBadCodeHash(block)
	case jamerrors.ErrGBadCoreIndex:
		return fuzzBlockGBadCoreIndex(block)
	case jamerrors.ErrGBadSignature:
		return fuzzBlockGBadSignature(block)
	case jamerrors.ErrGCoreEngaged:
		return fuzzBlockGCoreEngaged(block, s)
	case jamerrors.ErrGDependencyMissing:
		return fuzzBlockGDependencyMissing(block, s)
	case jamerrors.ErrGDuplicatePackageTwoReports:
		return fuzzBlockGDuplicatePackageTwoReports(block, s)
	case jamerrors.ErrGFutureReportSlot:
		return fuzzBlockGFutureReportSlot(block)
	case jamerrors.ErrGInsufficientGuarantees:
		return fuzzBlockGInsufficientGuarantees(block)
	case jamerrors.ErrGDuplicateGuarantors:
		return fuzzBlockGDuplicateGuarantors(block)
	case jamerrors.ErrGOutOfOrderGuarantee:
		return fuzzBlockGOutOfOrderGuarantee(block)
	case jamerrors.ErrGWorkReportGasTooHigh:
		return fuzzBlockGWorkReportGasTooHigh(block)
	case jamerrors.ErrGServiceItemTooLow:
		return fuzzBlockGServiceItemTooLow(block, s)
	case jamerrors.ErrGBadValidatorIndex:
		return fuzzBlockGBadValidatorIndex(block)
	case jamerrors.ErrGWrongAssignment:
		return fuzzBlockGWrongAssignment(block, s)
	case jamerrors.ErrGAnchorNotRecent:
		return fuzzBlockGAnchorNotRecent(block, s)
	case jamerrors.ErrGBadBeefyMMRRoot:
		return fuzzBlockGBadBeefyMMRRoot(block, s)
	case jamerrors.ErrGBadServiceID:
		return fuzzBlockGBadServiceID(block, s)
	case jamerrors.ErrGBadStateRoot:
		return fuzzBlockGBadStateRoot(block, s)
	case jamerrors.ErrGReportEpochBeforeLast:
		return fuzzBlockGReportEpochBeforeLast(block, s)
	case jamerrors.ErrGDuplicatePackageRecentHistory:
		return fuzzBlockGDuplicatePackageRecentHistory(block, s)
	case jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks:
		return fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks(block)
	case jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue:
		return fuzzBlockGSegmentRootLookupInvalidUnexpectedValue(block)
	case jamerrors.ErrGCoreWithoutAuthorizer:
		return fuzzBlockGCoreWithoutAuthorizer(block, s)
	case jamerrors.ErrGCoreUnexpectedAuthorizer:
		return fuzzBlockGCoreUnexpectedAuthorizer(block, s)

	// assurances errors
	case jamerrors.ErrABadSignature:
		return fuzzBlockABadSignature(block)
	case jamerrors.ErrABadValidatorIndex:
		return fuzzBlockABadValidatorIndex(block)
	case jamerrors.ErrABadCore:
		return fuzzBlockABadCore(block, s, validatorSecrets)
	case jamerrors.ErrABadParentHash:
		return fuzzBlockABadParentHash(block, validatorSecrets)
	case jamerrors.ErrAStaleReport:
		return fuzzBlockAStaleReport(block, s)

	// disputes errors
	case jamerrors.ErrDNotSortedWorkReports:
		return fuzzBlockDNotSortedWorkReports(block)
	case jamerrors.ErrDNotUniqueVotes:
		return fuzzBlockDNotUniqueVotes(block)
	case jamerrors.ErrDNotSortedValidVerdicts:
		return fuzzBlockDNotSortedValidVerdicts(block)
	case jamerrors.ErrDNotHomogenousJudgements:
		return fuzzBlockDNotHomogenousJudgements(block)
	case jamerrors.ErrDMissingCulpritsBadVerdict:
		return fuzzBlockDMissingCulpritsBadVerdict(block)
	case jamerrors.ErrDSingleCulpritBadVerdict:
		return fuzzBlockDSingleCulpritBadVerdict(block)
	case jamerrors.ErrDTwoCulpritsBadVerdictNotSorted:
		return fuzzBlockDTwoCulpritsBadVerdictNotSorted(block)
	case jamerrors.ErrDAlreadyRecordedVerdict:
		return fuzzBlockDAlreadyRecordedVerdict(block)
	case jamerrors.ErrDCulpritAlreadyInOffenders:
		return fuzzBlockDCulpritAlreadyInOffenders(block)
	case jamerrors.ErrDOffenderNotPresentVerdict:
		return fuzzBlockDOffenderNotPresentVerdict(block)
	case jamerrors.ErrDMissingFaultsGoodVerdict:
		return fuzzBlockDMissingFaultsGoodVerdict(block)
	case jamerrors.ErrDTwoFaultOffendersGoodVerdict:
		return fuzzBlockDTwoFaultOffendersGoodVerdict(block)
	case jamerrors.ErrDAlreadyRecordedVerdictWithFaults:
		return fuzzBlockDAlreadyRecordedVerdictWithFaults(block)
	case jamerrors.ErrDFaultOffenderInOffendersList:
		return fuzzBlockDFaultOffenderInOffendersList(block)
	case jamerrors.ErrDAuditorMarkedOffender:
		return fuzzBlockDAuditorMarkedOffender(block)
	case jamerrors.ErrDBadSignatureInVerdict:
		return fuzzBlockDBadSignatureInVerdict(block)
	case jamerrors.ErrDBadSignatureInCulprits:
		return fuzzBlockDBadSignatureInCulprits(block)
	case jamerrors.ErrDAgeTooOldInVerdicts:
		return fuzzBlockDAgeTooOldInVerdicts(block)

	default:
		return nil
	}
}

func selectImportBlocksError(store *storage.StateDBStorage, modes []string, stf *statedb.StateTransition) (*statedb.StateTransition, error, []error) {
	var aggregatedErrors []error
	var mutatedSTFs []statedb.StateTransition
	block := stf.Block
	sdb, err := statedb.NewStateDBFromSnapshotRaw(store, &stf.PreState)
	if err != nil {
		return nil, err, nil
	}

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
		return nil, nil, nil
	}

	//fmt.Printf("V(TotalValidators) | Expected=%v Found=%v\n", expectedNumNodes, len(validatorSecrets))

	// Create STF copy for original block
	oStatedbCopy := sdb.Copy()
	oBlockCopy := block.Copy()
	oSlot := oBlockCopy.TimeSlot()
	oEpoch, oPhase := oStatedbCopy.GetSafrole().EpochAndPhase(oSlot)

	// Make sure original block passes seal test: which requires author guessing, entropy, attempt for passing
	oValid, oValidatorIdx, oValidatorPub, err := oStatedbCopy.VerifyBlockHeader(oBlockCopy)
	if !oValid || err != nil || oBlockCopy.Header.AuthorIndex != oValidatorIdx {
		panic(fmt.Sprintf("Original block failed seal test: %v | %v | %v\n", oValid, err, oBlockCopy.Header.AuthorIndex))
		return nil, nil, nil
	}

	if len(aggregatedErrors) == 0 {
		fmt.Printf("[#%v e=%v,m=%03d] \033[31mNotFuzzable\033[0m  Author: %v (Idx:%v)\n", oSlot, oEpoch, oPhase, oValidatorPub, oValidatorIdx)
		//fmt.Printf("ExtrinsicHash=%v\nSeal=%x\nEntropySource=%x\n", oBlockCopy.Header.ExtrinsicHash, oBlockCopy.Header.Seal, oBlockCopy.GetHeader().EntropySource)
		return nil, nil, nil
	}

	fmt.Printf("[#%v e=%v,m=%03d] \033[0mFuzzable!!!\033[0m  Author: %v (Idx:%v)\n", oSlot, oEpoch, oPhase, oValidatorPub, oValidatorIdx)
	//fmt.Printf("ExtrinsicHash=%v\nSeal=%x\nEntropySource=%x\n", oBlockCopy.Header.ExtrinsicHash, oBlockCopy.Header.Seal, oBlockCopy.GetHeader().EntropySource)

	for _, selectedError := range aggregatedErrors {
		blockCopy := block.Copy()
		statedbCopy := sdb.Copy()
		stfErrExpected := possibleError(selectedError, blockCopy, statedbCopy, validatorSecrets)
		if stfErrExpected == nil {
			continue
		} else {

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
			//fmt.Printf("Resealing Slot %v with: %v (Idx:%v) | priv: %v\n", blockCopy.TimeSlot(), block_author_ietf_pub, blockCopy.Header.AuthorIndex, block_author_ietf_priv)
			mSealedBlk, sealErr := statedbCopy.SealBlockWithEntropy(block_author_ietf_pub, block_author_ietf_priv, blockCopy.Header.AuthorIndex, blockCopy.TimeSlot(), blockCopy)
			if sealErr != nil {
				fmt.Printf("Fuzzing failed to seal block!!!\n")
				continue
			}

			// Step 2: make sure it passes re-seal test again..
			mValid, mValidatorIdx, mValidatorPub, err := statedbCopy.VerifyBlockHeader(mSealedBlk)
			if !mValid || err != nil {
				panic(fmt.Sprintf("mutated block failed seal entropy test failed: %v | %v\n", mValid, err))
				continue
			} else {
				//fmt.Printf("Mutated block passed seal test. Author: %v (Idx:%v) ExtrinsicHash=%v, Seal=%x, EntropySource=%x\n", mValidatorPub, mValidatorIdx, mSealedBlk.GetHeader().ExtrinsicHash, mSealedBlk.GetHeader().Seal, mSealedBlk.GetHeader().EntropySource)
				//fmt.Printf("MutatedBlock=%v\n", mSealedBlk.String())

			}

			if mValidatorIdx != oValidatorIdx && mSealedBlk.TimeSlot() == oBlockCopy.TimeSlot() {
				fmt.Printf("Validator changed unexpectedly!!! Original=%x (Idx:%v) | Mutated=%x (Idx:%v)\n", oValidatorPub, oValidatorIdx, mValidatorPub, mValidatorIdx)
				//continue
			}

			// Step 3: Constructe mutated state transition
			stfMutated := statedb.StateTransition{
				PreState:  stf.PreState,
				Block:     *mSealedBlk,
				PostState: stf.PreState,
			}

			// TODO: need ancestorSet, accumulationRoot
			stfErrActual := statedb.CheckStateTransition(store, &stfMutated, nil)
			if stfErrActual == stfErrExpected {
				//fmt.Printf("[#%v e=%v,m=%03d] Fuzzed Correctly: err %v\n", oSlot, oEpoch, oPhase, jamerrors.GetErrorStr(stfErrExpected))
				errorList = append(errorList, stfErrExpected)
				mutatedSTFs = append(mutatedSTFs, stfMutated)
			} else {
				fmt.Printf("[#%v e=%v,m=%03d] Fuzzed Failed!!  Actual: \033[32m%v\033[0m  | Expected:%v\n", oSlot, oEpoch, oPhase, jamerrors.GetErrorStr(stfErrActual), jamerrors.GetErrorStr(stfErrExpected))
				if jamerrors.GetErrorStr(stfErrActual) == "BadSignature" {
					//fmt.Printf("PreState: %v\n", stf.Block.Extrinsic.Guarantees[0].String())
				}
				if jamerrors.GetErrorStr(stfErrActual) == "BadValidatorIndex" {
					//	fmt.Printf("PreState: %v\n", stf.Block.Extrinsic.Guarantees[0].String())
				}
			}
		}
	}
	// pick a random error based on our success
	if len(errorList) > 0 {
		rand.Seed(time.Now().UnixNano())
		errSelectionIdx := rand.Intn(len(errorList))
		mutatedSTF := &mutatedSTFs[errSelectionIdx]
		expectedErr := errorList[errSelectionIdx]
		possibleErrs := errorList
		fmt.Printf("[#%v e=%v,m=%03d] Fuzzed with \033[32m%v\033[0m ouf of %v possible errors = %v\n", oSlot, oEpoch, oPhase, jamerrors.GetErrorStr(expectedErr), len(possibleErrs), jamerrors.GetErrorStrs(possibleErrs))

		return mutatedSTF, expectedErr, possibleErrs
	}
	return nil, nil, nil
}
