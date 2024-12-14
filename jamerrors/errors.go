package jamerrors

import (
	"errors"
	"strings"
)

// T-safrole errors
var (
	ErrTBadTicketAttemptNumber = errors.New("BadTicketAttemptNumber: Submit an extrinsic with a bad ticket attempt number.")
	ErrTTicketAlreadyInState   = errors.New("TicketAlreadyInState: Submit one ticket already recorded in the state.")
	ErrTTicketsBadOrder        = errors.New("TicketsBadOrder: Submit tickets in bad order.")
	ErrTBadRingProof           = errors.New("BadRingProof: Submit tickets with bad ring proof.")
	ErrTEpochLotteryOver       = errors.New("EpochLotteryOver: Submit tickets when epoch's lottery is over.")
	ErrTTimeslotNotMonotonic   = errors.New("TimeslotNotMonotonic: Progress from slot X to slot X. Timeslot must be strictly monotonic.")
)

// G-reports errors
var (
	ErrGBadCodeHash                             = errors.New("BadCodeHash: Work result code hash doesn't match the one expected for the service.")
	ErrGBadCoreIndex                            = errors.New("BadCoreIndex: Core index is too big.")
	ErrGBadSignature                            = errors.New("BadSignature: Invalid report guarantee signature.")
	ErrGCoreEngaged                             = errors.New("CoreEngaged: A core is not available.")
	ErrGDependencyMissing                       = errors.New("DependencyMissing: Prerequisite is missing.")
	ErrGDuplicatePackageTwoReports              = errors.New("DuplicatePackageTwoReports: Report contains a duplicate package (send two reports from same package).")
	ErrGFutureReportSlot                        = errors.New("FutureReportSlot: Report refers to a slot in the future with respect to container block slot.")
	ErrGInsufficientGuarantees                  = errors.New("InsufficientGuarantees: Report with no enough guarantors signatures.")
	ErrGDuplicateGuarantors                     = errors.New("DuplicateGuarantors: Guarantors indices are not sorted or unique.")
	ErrGOutOfOrderGuarantee                     = errors.New("OutOfOrderGuarantee: Reports cores are not sorted or unique.")
	ErrGWorkReportGasTooHigh                    = errors.New("WorkReportGasTooHigh: Work report per core gas is too much high.")
	ErrGServiceItemTooLow                       = errors.New("ServiceItemTooLow: Accumulate gas is below the service minimum.")
	ErrGBadValidatorIndex                       = errors.New("BadValidatorIndex: Validator index is too big.")
	ErrGWrongAssignment                         = errors.New("WrongAssignment: Unexpected guarantor for work report core.")
	ErrGAnchorNotRecent                         = errors.New("AnchorNotRecent: Context anchor is not recent enough.")
	ErrGBadBeefyMMRRoot                         = errors.New("BadBeefyMMRRoot: Context Beefy MMR root doesn't match the one at anchor.")
	ErrGBadServiceID                            = errors.New("BadServiceID: Work result service identifier doesn't have any associated account in state.")
	ErrGBadStateRoot                            = errors.New("BadStateRoot: Context state root doesn't match the one at anchor.")
	ErrGDuplicatePackageRecentHistory           = errors.New("DuplicatePackageRecentHistory: Package was already available in recent history.")
	ErrGReportEpochBeforeLast                   = errors.New("ReportEpochBeforeLast: Report guarantee slot is too old with respect to block slot.")
	ErrGSegmentRootLookupInvalidNotRecentBlocks = errors.New("SegmentRootLookupInvalidNotRecentBlocks: Segments tree root lookup item not found in recent blocks history.")
	ErrGSegmentRootLookupInvalidUnexpectedValue = errors.New("SegmentRootLookupInvalidUnexpectedValue: Segments tree root lookup item found in recent blocks history but with an unexpected value.")
	ErrGCoreWithoutAuthorizer                   = errors.New("CoreWithoutAuthorizer: Target core without any authorizer.")
	ErrGCoreUnexpectedAuthorizer                = errors.New("CoreUnexpectedAuthorizer: Target core with unexpected authorizer.")
)

// A-assurances
var (
	ErrABadSignature      = errors.New("BadSignature: One assurance has a bad signature.")
	ErrABadValidatorIndex = errors.New("BadValidatorIndex: One assurance has a bad validator index.")
	ErrABadCore           = errors.New("BadCore: One assurance targets a core without any assigned work report.")
	ErrABadParentHash     = errors.New("BadParentHash: One assurance has a bad attestation parent hash.")
	ErrAStaleReport       = errors.New("StaleReport: One assurance targets a core with a stale report.")
)

// disputes errors
var (
	ErrDNotSortedWorkReports             = errors.New("NotSortedWorkReports: Not sorted work reports within a verdict.")
	ErrDNotUniqueVotes                   = errors.New("NotUniqueVotes: Not unique votes within a verdict.")
	ErrDNotSortedValidVerdicts           = errors.New("NotSortedValidVerdicts: Not sorted, valid verdicts.")
	ErrDNotHomogenousJudgements          = errors.New("NotHomogenousJudgements: Not homogeneous judgements, but positive votes count is not correct.")
	ErrDMissingCulpritsBadVerdict        = errors.New("MissingCulpritsBadVerdict: Missing culprits for bad verdict.")
	ErrDSingleCulpritBadVerdict          = errors.New("SingleCulpritBadVerdict: Single culprit for bad verdict.")
	ErrDTwoCulpritsBadVerdictNotSorted   = errors.New("TwoCulpritsBadVerdictNotSorted: Two culprits for bad verdict, not sorted.")
	ErrDAlreadyRecordedVerdict           = errors.New("AlreadyRecordedVerdict: Report an already recorded verdict, with culprits.")
	ErrDCulpritAlreadyInOffenders        = errors.New("CulpritAlreadyInOffenders: Culprit offender already in the offenders list.")
	ErrDOffenderNotPresentVerdict        = errors.New("OffenderNotPresentVerdict: Offender relative to a not present verdict.")
	ErrDMissingFaultsGoodVerdict         = errors.New("MissingFaultsGoodVerdict: Missing faults for good verdict.")
	ErrDTwoFaultOffendersGoodVerdict     = errors.New("TwoFaultOffendersGoodVerdict: Two fault offenders for a good verdict, not sorted.")
	ErrDAlreadyRecordedVerdictWithFaults = errors.New("AlreadyRecordedVerdictWithFaults: Report an already recorded verdict, with faults.")
	ErrDFaultOffenderInOffendersList     = errors.New("FaultOffenderInOffendersList: Fault offender already in the offenders list.")
	ErrDAuditorMarkedOffender            = errors.New("AuditorMarkedOffender: Auditor marked as offender, but vote matches the verdict.")
	ErrDBadSignatureInVerdict            = errors.New("BadSignatureInVerdict: Bad signature within the verdict judgements.")
	ErrDBadSignatureInCulprits           = errors.New("BadSignatureInCulprits: Bad signature within the culprits sequence.")
	ErrDAgeTooOldInVerdicts              = errors.New("AgeTooOldInVerdicts: Age too old for verdicts judgements.")
)

func GetErrorStr(err error) string {
	errStr := err.Error()
	if !strings.Contains(errStr, ":") {
		return errStr
	}
	errStr = strings.Split(errStr, ":")[0]
	return errStr
}
