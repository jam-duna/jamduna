package fuzz

import (
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
)

// Not sorted work reports within a verdict
func fuzzBlockDNotSortedWorkReports(seed []byte, block *types.Block) error {
	// TODO: Sourabh
	return jamerrors.ErrDNotSortedWorkReports
}

// Not unique votes within a verdict
func fuzzBlockDNotUniqueVotes(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}

// Not sorted, valid verdicts
func fuzzBlockDNotSortedValidVerdicts(seed []byte, block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Not homogeneous judgements, but positive votes count is not correct
func fuzzBlockDNotHomogenousJudgements(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}

// Missing culprits for bad verdict
func fuzzBlockDMissingCulpritsBadVerdict(seed []byte, block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Single culprit for bad verdict
func fuzzBlockDSingleCulpritBadVerdict(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}

// Two culprits for bad verdict, not sorted
func fuzzBlockDTwoCulpritsBadVerdictNotSorted(seed []byte, block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Report an already recorded verdict, with culprits
func fuzzBlockDAlreadyRecordedVerdict(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}

// Culprit offender already in the offenders list
func fuzzBlockDCulpritAlreadyInOffenders(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}

// Offender relative to a not present verdict
func fuzzBlockDOffenderNotPresentVerdict(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}

// Missing faults for good verdict
func fuzzBlockDMissingFaultsGoodVerdict(seed []byte, block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Two fault offenders for a good verdict, not sorted
func fuzzBlockDTwoFaultOffendersGoodVerdict(seed []byte, block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Report an already recorded verdict, with faults
func fuzzBlockDAlreadyRecordedVerdictWithFaults(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}

// Fault offender already in the offenders list
func fuzzBlockDFaultOffenderInOffendersList(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}

// Auditor marked as offender, but vote matches the verdict.
func fuzzBlockDAuditorMarkedOffender(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}

// Bad signature within the verdict judgements
func fuzzBlockDBadSignatureInVerdict(seed []byte, block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Bad signature within the culprits sequence
func fuzzBlockDBadSignatureInCulprits(seed []byte, block *types.Block) error {
	// TODO: Sourabh
	return nil
}

// Age too old for verdicts judgements
func fuzzBlockDAgeTooOldInVerdicts(seed []byte, block *types.Block) error {
	// TODO: Michael
	return nil
}
