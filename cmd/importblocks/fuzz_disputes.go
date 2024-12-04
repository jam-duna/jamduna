package main

import (
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
)

// disputes fuzzBlocks

// Sourabh
func fuzzBlockDNotSortedWorkReports(block *types.Block) error {
	// TODO: Implement fuzzing logic for DNotSortedWorkReports

	return jamerrors.ErrDNotSortedWorkReports
}

// Shawn
func fuzzBlockDNotUniqueVotes(block *types.Block) error {
	// TODO: Implement fuzzing logic for DNotUniqueVotes
	return nil
}

// Sourabh
func fuzzBlockDNotSortedValidVerdicts(block *types.Block) error {
	// TODO: Implement fuzzing logic for DNotSortedValidVerdicts
	return nil
}

// Shawn
func fuzzBlockDNotHomogenousJudgements(block *types.Block) error {
	// TODO: Implement fuzzing logic for DNotHomogenousJudgements
	return nil
}

// Sourabh
func fuzzBlockDMissingCulpritsBadVerdict(block *types.Block) error {
	// TODO: Implement fuzzing logic for DMissingCulpritsBadVerdict
	return nil
}

// Shawn
func fuzzBlockDSingleCulpritBadVerdict(block *types.Block) error {
	// TODO: Implement fuzzing logic for DSingleCulpritBadVerdict
	return nil
}

// Sourabh
func fuzzBlockDTwoCulpritsBadVerdictNotSorted(block *types.Block) error {
	// TODO: Implement fuzzing logic for DTwoCulpritsBadVerdictNotSorted
	return nil
}

// Shawn
func fuzzBlockDAlreadyRecordedVerdict(block *types.Block) error {
	// TODO: Implement fuzzing logic for DAlreadyRecordedVerdict
	return nil
}

// Shawn
func fuzzBlockDCulpritAlreadyInOffenders(block *types.Block) error {
	// TODO: Implement fuzzing logic for DCulpritAlreadyInOffenders
	return nil
}

// Shawn
func fuzzBlockDOffenderNotPresentVerdict(block *types.Block) error {
	// TODO: Implement fuzzing logic for DOffenderNotPresentVerdict
	return nil
}

// Sourabh
func fuzzBlockDMissingFaultsGoodVerdict(block *types.Block) error {
	// TODO: Implement fuzzing logic for DMissingFaultsGoodVerdict
	return nil
}

// Michael
func fuzzBlockDTwoFaultOffendersGoodVerdict(block *types.Block) error {
	// TODO: Implement fuzzing logic for DTwoFaultOffendersGoodVerdict
	return nil
}

// Shawn
func fuzzBlockDAlreadyRecordedVerdictWithFaults(block *types.Block) error {
	// TODO: Implement fuzzing logic for DAlreadyRecordedVerdictWithFaults
	return nil
}

// Michael
func fuzzBlockDFaultOffenderInOffendersList(block *types.Block) error {
	// TODO: Implement fuzzing logic for DFaultOffenderInOffendersList
	return nil
}

// Shawn
func fuzzBlockDAuditorMarkedOffender(block *types.Block) error {
	// TODO: Implement fuzzing logic for DAuditorMarkedOffender
	return nil
}

// Sourabh
func fuzzBlockDBadSignatureInVerdict(block *types.Block) error {
	// TODO: Implement fuzzing logic for DBadSignatureInVerdict
	return nil
}

// Sourabh
func fuzzBlockDBadSignatureInCulprits(block *types.Block) error {
	// TODO: Implement fuzzing logic for DBadSignatureInCulprits
	return nil
}

// Shawn
func fuzzBlockDAgeTooOldInVerdicts(block *types.Block) error {
	// TODO: Implement fuzzing logic for DAgeTooOldInVerdicts
	return nil
}
