package main

import (
	//"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/node"
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

func possibleError(selectedError error, block *types.Block, s *statedb.StateDB) error {
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
		return fuzzBlockABadCore(block)
	case jamerrors.ErrABadParentHash:
		return fuzzBlockABadParentHash(block)
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

func selectImportBlocksError(modes []string, stf *statedb.StateTransition) error {
	var aggregatedErrors []error
	block := stf.Block
	stdb, err := storage.NewStateDBStorage("/tmp/testImportBlocksError")
	if err != nil {
		return err
	}
	sdb, err := statedb.NewStateDBFromSnapshotRaw(stdb, &stf.PreState)
	if err != nil {
		return err
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

	if len(aggregatedErrors) == 0 {
		return nil
	}
	errorList := make([]error, 0)
	for _, selectedError := range aggregatedErrors {
		blockCopy := block.Copy()
		statedbCopy := sdb.Copy()
		err := possibleError(selectedError, blockCopy, statedbCopy)
		if err == nil {

		} else {
			stfMutated := statedb.StateTransition{
				PreState:  stf.PreState,
				Block:     *blockCopy,
				PostState: stf.PreState,
			}
			// need ancestorSet, accumulationRoot
			errActual := statedb.CheckStateTransition(stdb, &stfMutated, nil, common.Hash{})
			if errActual == err {
				fmt.Printf("Error %v is expected", err)
				errorList = append(errorList, err)
			} else {
				fmt.Printf("Error %v is not expected", jamerrors.GetErrorStr(err))
			}
		}
	}
	// pick a random error based on our success
	if len(errorList) > 0 {
		rand.Seed(time.Now().UnixNano())
		return errorList[rand.Intn(len(errorList))]
	}
	return nil
}

func validateConfig(config types.ConfigJamBlocks) {
	if config.HTTP == "" && config.QUIC == "" {
		log.Fatalf("You must specify either an HTTP URL or a QUIC address")
	}
	if config.QUIC != "" {
		log.Fatalf("QUIC functionality is not implemented yet. Endpoint: %s", config.Endpoint)
	}
	if config.Network != "tiny" {
		log.Fatalf("Tiny network only")
	}
	if config.Mode != "fallback" && config.Mode != "safrole" && config.Mode != "assurances" && config.Mode != "orderedaccumulation" {
		log.Fatalf("Invalid mode: %s. Must be one of fallback, safrole, assurances, orderedaccumulation", config.Mode)
	}
}

func main() {
	fmt.Printf("importblocks - JAM Import Blocks generator\n")

	mode := flag.String("m", "safrole", "Block generation mode: fallback, safrole, assurances, orderedaccumulation (under development: authorization, recenthistory, blessed, basichostfunctions, disputes, gas, finalization)")
	flag.StringVar(mode, "mode", *mode, "Block generation mode: fallback, safrole, assurances, orderedaccumulation")

	httpEndpoint := flag.String("h", "", "HTTP endpoint to send blocks")
	flag.StringVar(httpEndpoint, "http", *httpEndpoint, "HTTP endpoint to send blocks")

	quicEndpoint := flag.String("q", "", "QUIC endpoint to send blocks")
	flag.StringVar(quicEndpoint, "quic", *quicEndpoint, "QUIC endpoint to send blocks")

	verbose := flag.Bool("v", false, "Enable detailed logging")
	flag.BoolVar(verbose, "verbose", *verbose, "Enable detailed logging")

	network := flag.String("n", "tiny", "JAM network size: tiny, full")
	flag.StringVar(network, "network", *network, "JAM network size: tiny, full")

	numBlocks := flag.Int("numblocks", 50, "Number of valid blocks to generate (max 600)")
	invalidRate := flag.Int("invalidrate", 0, "Percentage of blocks that are invalid (under development)")
	statistics := flag.Int("statistics", 10, "Number of valid blocks between statistics dumps")

	flag.Parse()
	config := types.ConfigJamBlocks{
		Mode:        *mode,
		HTTP:        *httpEndpoint,
		QUIC:        *quicEndpoint,
		Verbose:     *verbose,
		NumBlocks:   *numBlocks,
		InvalidRate: *invalidRate,
		Statistics:  *statistics,
		Network:     *network,
	}
	validateConfig(config)
	// set up network with config
	node.ImportBlocks(&config)

	// TODO: adjust importblocks to send stateTransition JSON via HTTP and receive statetransition; adjust validatetraces to validatestatetransition
	for {

	}
}
