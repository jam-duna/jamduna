package jamerrors

import (
	"errors"
)

// T-safrole errors
var (
	ErrTBadTicketAttemptNumber = errors.New("Submit an extrinsic with a bad ticket attempt number.")
	ErrTTicketAlreadyInState   = errors.New("Submit one ticket already recorded in the state.")
	ErrTTicketsBadOrder        = errors.New("Submit tickets in bad order.")
	ErrTBadRingProof           = errors.New("Submit tickets with bad ring proof.")
	ErrTEpochLotteryOver       = errors.New("Submit tickets when epoch's lottery is over.")
	ErrTTimeslotNotMonotonic   = errors.New("Progress from slot X to slot X. Timeslot must be strictly monotonic.")
)

// G-reports errors
var (
	ErrGBadCodeHash                   = errors.New("Work result code hash doesn't match the one expected for the service.")
	ErrGBadCoreIndex                  = errors.New("Core index is too big.")
	ErrGBadSignature                  = errors.New("Invalid report guarantee signature.")
	ErrGCoreEngaged                   = errors.New("A core is not available.")
	ErrGDependencyMissing             = errors.New("Prerequisite is missing.")
	ErrGDuplicatePackageTwoReports    = errors.New("Report contains a duplicate package (send two reports from same package).")
	ErrGFutureReportSlot              = errors.New("Report refers to a slot in the future with respect to container block slot.")
	ErrGInsufficientGuarantees        = errors.New("Report with no enough guarantors signatures.")
	ErrGDuplicateGuarantors           = errors.New("Guarantors indices are not sorted or unique.")
	ErrGOutOfOrderGuarantee           = errors.New("Reports cores are not sorted or unique.")
	ErrGWorkReportGasTooHigh          = errors.New("Work report per core gas is too much high.")
	ErrGBadValidatorIndex             = errors.New("Validator index is too big.")
	ErrGWrongAssignment               = errors.New("Unexpected guarantor for work report core.")
	ErrGAnchorNotRecent               = errors.New("Context anchor is not recent enough.")
	ErrGBadBeefyMMRRoot               = errors.New("Context Beefy MMR root doesn't match the one at anchor.")
	ErrGBadServiceID                  = errors.New("Work result service identifier doesn't have any associated account in state.")
	ErrGBadStateRoot                  = errors.New("Context state root doesn't match the one at anchor.")
	ErrGDuplicatePackageRecentHistory = errors.New("Package was already available in recent history.")
	ErrGReportEpochBeforeLast         = errors.New("Report guarantee slot is too old with respect to block slot.")

	ErrGSegmentRootLookupInvalidNotRecentBlocks = errors.New("Segments tree root lookup item not found in recent blocks history.")
	ErrGSegmentRootLookupInvalidUnexpectedValue = errors.New("Segments tree root lookup item found in recent blocks history but with an unexpected value.")
	ErrGCoreWithoutAuthorizer                   = errors.New("Target core without any authorizer.")
	ErrGCoreUnexpectedAuthorizer                = errors.New("Target core with unexpected authorizer.")
)

// A-assurances
var (
	ErrABadSignature      = errors.New("One assurance has a bad signature.")
	ErrABadValidatorIndex = errors.New("One assurance has a bad validator index.")
	ErrABadCore           = errors.New("One assurance targets a core without any assigned work report.")
	ErrABadParentHash     = errors.New("One assurance has a bad attestation parent hash.")
	ErrAStaleReport       = errors.New("One assurance targets a core with a stale report.")
)

// disputes errors
var (
	ErrDNotSortedWorkReports             = errors.New("Not sorted work reports within a verdict.")
	ErrDNotUniqueVotes                   = errors.New("Not unique votes within a verdict.")
	ErrDNotSortedValidVerdicts           = errors.New("Not sorted, valid verdicts.")
	ErrDNotHomogenousJudgements          = errors.New("Not homogeneous judgements, but positive votes count is not correct.")
	ErrDMissingCulpritsBadVerdict        = errors.New("Missing culprits for bad verdict.")
	ErrDSingleCulpritBadVerdict          = errors.New("Single culprit for bad verdict.")
	ErrDTwoCulpritsBadVerdictNotSorted   = errors.New("Two culprits for bad verdict, not sorted.")
	ErrDAlreadyRecordedVerdict           = errors.New("Report an already recorded verdict, with culprits.")
	ErrDCulpritAlreadyInOffenders        = errors.New("Culprit offender already in the offenders list.")
	ErrDOffenderNotPresentVerdict        = errors.New("Offender relative to a not present verdict.")
	ErrDMissingFaultsGoodVerdict         = errors.New("Missing faults for good verdict.")
	ErrDTwoFaultOffendersGoodVerdict     = errors.New("Two fault offenders for a good verdict, not sorted.")
	ErrDAlreadyRecordedVerdictWithFaults = errors.New("Report an already recorded verdict, with faults.")
	ErrDFaultOffenderInOffendersList     = errors.New("Fault offender already in the offenders list.")
	ErrDAuditorMarkedOffender            = errors.New("Auditor marked as offender, but vote matches the verdict.")
	ErrDBadSignatureInVerdict            = errors.New("Bad signature within the verdict judgements.")
	ErrDBadSignatureInCulprits           = errors.New("Bad signature within the culprits sequence.")
	ErrDAgeTooOldInVerdicts              = errors.New("Age too old for verdicts judgements.")
)
