package jamerrors

import (
	"errors"
	"strings"
)

// Ticket (T) Errors
var (
	ErrTBadTicketAttemptNumber = errors.New("T1|BadTicketAttemptNumber: Submit an extrinsic with a bad ticket attempt number.")
	ErrTTicketAlreadyInState   = errors.New("T2|TicketAlreadyInState: Submit one ticket already recorded in the state.")
	ErrTTicketsBadOrder        = errors.New("T3|TicketsBadOrder: Submit tickets in bad order.")
	ErrTBadRingProof           = errors.New("T4|BadRingProof: Submit tickets with a bad ring proof.")
	ErrTEpochLotteryOver       = errors.New("T5|EpochLotteryOver: Submit tickets when the epoch's lottery is over.")
	ErrTTimeslotNotMonotonic   = errors.New("T6|TimeslotNotMonotonic: Progress from slot X to slot X. Timeslot must be strictly monotonic.")
)

// Assurances Errors
var (
	ErrABadSignature      = errors.New("A1|BadSignature: One assurance has a bad signature.")
	ErrABadValidatorIndex = errors.New("A2|BadValidatorIndex: One assurance has a bad validator index.")
	ErrABadCore           = errors.New("A3|BadCore: One assurance targets a core without any assigned work report.")
	ErrABadParentHash     = errors.New("A4|BadParentHash: One assurance has a bad attestation parent hash.")
	ErrAStaleReport       = errors.New("A5|StaleReport: One assurance targets a core with a stale report.")
	ErrADuplicateAssurer  = errors.New("A6|DuplicateAssurer: Duplicate assurer.")
	ErrANotSortedAssurers = errors.New("A7|NotSortedAssurers: Assurers not sorted.")
)

// Guarantee & Work Reports Errors
var (
	ErrGBadCodeHash                             = errors.New("G1|BadCodeHash: Work result code hash doesn't match the one expected for the service.")
	ErrGBadCoreIndex                            = errors.New("G2|BadCoreIndex: Core index is too big.")
	ErrGBadSignature                            = errors.New("G3|BadSignature: Invalid report guarantee signature.")
	ErrGCoreEngaged                             = errors.New("G4|CoreEngaged: A core is not available.")
	ErrGDependencyMissing                       = errors.New("G5|DependencyMissing: Prerequisite is missing.")
	ErrGDuplicatePackageTwoReports              = errors.New("G6|DuplicatePackageTwoReports: Report contains a duplicate package (two reports from the same package).")
	ErrGFutureReportSlot                        = errors.New("G7|FutureReportSlot: Report refers to a slot in the future with respect to container block slot.")
	ErrGInsufficientGuarantees                  = errors.New("G8|InsufficientGuarantees: Report without enough guarantors' signatures.")
	ErrGDuplicateGuarantors                     = errors.New("G9|DuplicateGuarantors: Guarantors' indices are not sorted or unique.")
	ErrGOutOfOrderGuarantee                     = errors.New("G10|OutOfOrderGuarantee: Reports' cores are not sorted or unique.")
	ErrGWorkReportGasTooHigh                    = errors.New("G11|WorkReportGasTooHigh: Work report per-core gas is too high.")
	ErrGServiceItemTooLow                       = errors.New("G12|ServiceItemTooLow: Accumulated gas is below the service minimum.")
	ErrGBadValidatorIndex                       = errors.New("G13|BadValidatorIndex: Validator index is too big.")
	ErrGWrongAssignment                         = errors.New("G14|WrongAssignment: Unexpected guarantor for work report core.")
	ErrGAnchorNotRecent                         = errors.New("G15|AnchorNotRecent: Context anchor is not recent enough.")
	ErrGBadBeefyMMRRoot                         = errors.New("G16|BadBeefyMMRRoot: Context Beefy MMR root doesn't match the one at anchor.")
	ErrGBadServiceID                            = errors.New("G17|BadServiceID: Work result service identifier doesn't have any associated account in state.")
	ErrGBadStateRoot                            = errors.New("G18|BadStateRoot: Context state root doesn't match the one at anchor.")
	ErrGDuplicatePackageRecentHistory           = errors.New("G19|DuplicatePackageRecentHistory: Package was already available in recent history.")
	ErrGReportEpochBeforeLast                   = errors.New("G20|ReportEpochBeforeLast: Report guarantee slot is too old with respect to block slot.")
	ErrGSegmentRootLookupInvalidNotRecentBlocks = errors.New("G21|SegmentRootLookupInvalidNotRecentBlocks: Segments tree root lookup item not found in recent blocks history.")
	ErrGSegmentRootLookupInvalidUnexpectedValue = errors.New("G22|SegmentRootLookupInvalidUnexpectedValue: Segments tree root lookup found in recent blocks history but with an unexpected value.")
	ErrGCoreWithoutAuthorizer                   = errors.New("G23|CoreWithoutAuthorizer: Target core without any authorizer.")
	ErrGCoreUnexpectedAuthorizer                = errors.New("G24|CoreUnexpectedAuthorizer: Target core with an unexpected authorizer.")
)

// Disputes Errors
var (
	ErrDNotSortedWorkReports             = errors.New("D1|NotSortedWorkReports: Not sorted work reports within a verdict.")
	ErrDNotUniqueVotes                   = errors.New("D2|NotUniqueVotes: Not unique votes within a verdict.")
	ErrDNotSortedValidVerdicts           = errors.New("D3|NotSortedValidVerdicts: Not sorted, valid verdicts.")
	ErrDNotHomogenousJudgements          = errors.New("D4|NotHomogenousJudgements: Not homogeneous judgements; positive votes count is incorrect.")
	ErrDMissingCulpritsBadVerdict        = errors.New("D5|MissingCulpritsBadVerdict: Missing culprits for bad verdict.")
	ErrDSingleCulpritBadVerdict          = errors.New("D6|SingleCulpritBadVerdict: Single culprit for bad verdict.")
	ErrDTwoCulpritsBadVerdictNotSorted   = errors.New("D7|TwoCulpritsBadVerdictNotSorted: Two culprits for bad verdict, not sorted.")
	ErrDAlreadyRecordedVerdict           = errors.New("D8|AlreadyRecordedVerdict: Report an already recorded verdict with culprits.")
	ErrDCulpritAlreadyInOffenders        = errors.New("D9|CulpritAlreadyInOffenders: Culprit offender already in the offenders list.")
	ErrDOffenderNotPresentVerdict        = errors.New("D10|OffenderNotPresentVerdict: Offender relative to a not-present verdict.")
	ErrDMissingFaultsGoodVerdict         = errors.New("D11|MissingFaultsGoodVerdict: Missing faults for good verdict.")
	ErrDTwoFaultOffendersGoodVerdict     = errors.New("D12|TwoFaultOffendersGoodVerdict: Two fault offenders for a good verdict, not sorted.")
	ErrDAlreadyRecordedVerdictWithFaults = errors.New("D13|AlreadyRecordedVerdictWithFaults: Report an already recorded verdict with faults.")
	ErrDFaultOffenderInOffendersList     = errors.New("D14|FaultOffenderInOffendersList: Fault offender already in the offenders list.")
	ErrDAuditorMarkedOffender            = errors.New("D15|AuditorMarkedOffender: Auditor marked as offender, but vote matches the verdict.")
	ErrDBadSignatureInVerdict            = errors.New("D16|BadSignatureInVerdict: Bad signature within the verdict judgements.")
	ErrDBadSignatureInCulprits           = errors.New("D17|BadSignatureInCulprits: Bad signature within the culprits sequence.")
	ErrDAgeTooOldInVerdicts              = errors.New("D18|AgeTooOldInVerdicts: Age too old for verdicts judgements.")
)

// GetErrorName extracts the error name from the error message.
func GetErrorName(err error) string {
	if err == nil {
		return "No Error"
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "|") || !strings.Contains(errStr, ":") {
		return errStr
	}
	parts := strings.SplitN(errStr, "|", 2)
	if len(parts) < 2 {
		return errStr
	}
	nameDesc := parts[1]
	// Split on ':' to separate the error name from its description.
	nameParts := strings.SplitN(nameDesc, ":", 2)
	if len(nameParts) < 1 {
		return errStr
	}
	return strings.TrimSpace(nameParts[0])
}

func GetErrorNames(errs []error) []string {
	errStrs := make([]string, len(errs))
	for i, err := range errs {
		errStrs[i] = GetErrorName(err)
	}
	return errStrs
}

// GetErrorCode extracts the error code from the error message.
func GetErrorCode(err error) string {
	if err == nil {
		return ""
	}
	errStr := err.Error()
	// Check if the error string contains '|'.
	if !strings.Contains(errStr, "|") {
		return ""
	}
	parts := strings.SplitN(errStr, "|", 2)
	return strings.TrimSpace(parts[0])
}

// GetErrorCodeWithName returns the error code and name in the format "Code_ErrorName".
func GetErrorCodeWithName(err error) string {
	code := GetErrorCode(err)
	name := GetErrorName(err)
	if code == "" || name == "" {
		return ""
	}
	return code + "_" + name
}

// GetErrorDesc extracts the error description from the error message.
func GetErrorDesc(err error) string {
	if err == nil {
		return ""
	}
	errStr := err.Error()
	parts := strings.SplitN(errStr, ":", 2)
	if len(parts) < 2 {
		return "DESC NOT SET"
	}
	return strings.TrimSpace(parts[1])
}
