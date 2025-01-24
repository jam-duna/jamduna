package fuzz

import (
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
)

// Not sorted work reports within a verdict
func fuzzBlockDNotSortedWorkReports(block *types.Block) error {
	// TODO: Sourabh
	return jamerrors.ErrDNotSortedWorkReports
}

// Not unique votes within a verdict
func fuzzBlockDNotUniqueVotes(block *types.Block) error {
	// TODO: Michael
	return nil
}

// Not sorted, valid verdicts
func fuzzBlockDNotSortedValidVerdicts(block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Not homogeneous judgements, but positive votes count is not correct
func fuzzBlockDNotHomogenousJudgements(block *types.Block) error {
	// TODO: Michael
	return nil
}

// Missing culprits for bad verdict
func fuzzBlockDMissingCulpritsBadVerdict(block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Single culprit for bad verdict
func fuzzBlockDSingleCulpritBadVerdict(block *types.Block) error {
	// TODO: Michael
	return nil
}

// Two culprits for bad verdict, not sorted
func fuzzBlockDTwoCulpritsBadVerdictNotSorted(block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Report an already recorded verdict, with culprits
func fuzzBlockDAlreadyRecordedVerdict(block *types.Block) error {
	// TODO: Michael
	return nil
}

// Culprit offender already in the offenders list
func fuzzBlockDCulpritAlreadyInOffenders(block *types.Block) error {
	// TODO: Michael
	return nil
}

// Offender relative to a not present verdict
func fuzzBlockDOffenderNotPresentVerdict(block *types.Block) error {
	// TODO: Michael
	return nil
}

// Missing faults for good verdict
func fuzzBlockDMissingFaultsGoodVerdict(block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Two fault offenders for a good verdict, not sorted
func fuzzBlockDTwoFaultOffendersGoodVerdict(block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Report an already recorded verdict, with faults
func fuzzBlockDAlreadyRecordedVerdictWithFaults(block *types.Block) error {
	// TODO: Michael
	return nil
}

// Fault offender already in the offenders list
func fuzzBlockDFaultOffenderInOffendersList(block *types.Block) error {
	// TODO: Michael
	return nil
}

// Auditor marked as offender, but vote matches the verdict.
func fuzzBlockDAuditorMarkedOffender(block *types.Block) error {
	// TODO: Michael
	return nil
}

// Bad signature within the verdict judgements
func fuzzBlockDBadSignatureInVerdict(block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Bad signature within the culprits sequence
func fuzzBlockDBadSignatureInCulprits(block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Age too old for verdicts judgements
func fuzzBlockDAgeTooOldInVerdicts(block *types.Block) error {
	// TODO: Michael
	return nil
}
