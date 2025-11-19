package fuzz

import (
	"math/rand"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

// reports fuzzBlocks
func randomGuarantee(r *rand.Rand, block *types.Block) *types.Guarantee {
	if len(block.Extrinsic.Guarantees) == 0 {
		return nil
	}
	return &(block.Extrinsic.Guarantees[r.Intn(len(block.Extrinsic.Guarantees))])
}

func lastGuarantee(block *types.Block) *types.Guarantee {
	if len(block.Extrinsic.Guarantees) == 0 {
		return nil
	}
	return &(block.Extrinsic.Guarantees[len(block.Extrinsic.Guarantees)-1])
}

func randomGuaranteeResult(r *rand.Rand, g *types.Guarantee) *types.WorkDigest {
	if len(g.Report.Results) == 0 {
		return nil
	}
	return &(g.Report.Results[r.Intn(len(g.Report.Results))])
}

func lastGuaranteeResult(g *types.Guarantee) *types.WorkDigest {
	if len(g.Report.Results) == 0 {
		return nil
	}
	return &(g.Report.Results[len(g.Report.Results)-1])
}

// Generate a random hash
func randomHash(r *rand.Rand) common.Hash {
	var b [32]byte
	_, err := r.Read(b[:]) // Fill b with random bytes
	if err != nil {
		panic("randomHash failed to generate random hash: " + err.Error())
	}
	return common.BytesToHash(b[:])
}

// Generate a random hash that is different from the input hash
func randomDifferentHash(r *rand.Rand, input common.Hash) common.Hash {
	for {
		newHash := randomHash(r)
		if newHash != input { // Check if the new hash is different from the input
			return newHash
		}
	}
}

// Work result code hash doesn't match the one expected for the service.
func fuzzBlockGBadCodeHash(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	if len(g.Report.Results) == 0 {
		return nil
	}
	i := r.Intn(len(g.Report.Results))
	g.Report.Results[i].CodeHash = randomHash(r)
	return jamerrors.ErrGBadCodeHash
}

// Core index is too big.
func fuzzBlockGBadCoreIndex(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	g.Report.CoreIndex = uint(types.TotalCores + r.Intn(10))
	return jamerrors.ErrGBadCoreIndex
}

// Invalid report guarantee signature.
func fuzzBlockGBadSignature(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	if len(g.Signatures) == 0 {
		return nil
	}
	s := &(g.Signatures[r.Intn(len(g.Signatures))])
	s.Signature = types.GenerateRandomEd25519Signature()
	return jamerrors.ErrGBadSignature
}

// A core is not available.
func fuzzBlockGCoreEngaged(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	j := s.JamState
	if j.AvailabilityAssignments[int(g.Report.CoreIndex)] == nil {
		return nil
	}
	j.AvailabilityAssignments[int(g.Report.CoreIndex)] = &statedb.CoreState{
		WorkReport: g.Report,
		Timeslot:   block.Header.Slot,
	}
	return jamerrors.ErrGCoreEngaged
}

// Prerequisite is missing.
func fuzzBlockGDependencyMissing(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	g.Report.RefineContext.Prerequisites = append(g.Report.RefineContext.Prerequisites, randomHash(r))
	return jamerrors.ErrGDependencyMissing
}

// Report contains a duplicate package (send two reports from same package)
func fuzzBlockGDuplicatePackageTwoReports(seed []byte, block *types.Block, s *statedb.StateDB) error {
	if len(block.Extrinsic.Guarantees) < 2 {
		return nil
	}
	r := NewSeededRand(seed)
	i := r.Intn(len(block.Extrinsic.Guarantees) - 1)
	g0 := &block.Extrinsic.Guarantees[i]
	g1 := &block.Extrinsic.Guarantees[i+1]

	if r.Intn(10) < 5 {
		g0.Report.AvailabilitySpec.WorkPackageHash = g1.Report.AvailabilitySpec.WorkPackageHash
	} else {
		g1.Report.AvailabilitySpec.WorkPackageHash = g0.Report.AvailabilitySpec.WorkPackageHash
	}
	return nil
}

// Report refers to a slot in the future with respect to container block slot.
func fuzzBlockGFutureReportSlot(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	g.Slot = block.Header.Slot + 1 + uint32(r.Intn(100))
	return jamerrors.ErrGFutureReportSlot
}

// Report with no enough guarantors signatures.
func fuzzBlockGInsufficientGuarantees(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	g.Signatures = g.Signatures[0:1] // only 1
	return jamerrors.ErrGInsufficientGuarantees
}

// Guarantors indices are not sorted or unique.
func fuzzBlockGDuplicateGuarantors(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil || len(g.Signatures) < 2 {
		return nil
	}
	i := r.Intn(len(g.Signatures))
	j := (i + 1) % len(g.Signatures)
	g.Signatures[i], g.Signatures[j] = g.Signatures[j], g.Signatures[i]
	return nil
}

// Reports cores are not sorted or unique.
func fuzzBlockGOutOfOrderGuarantee(seed []byte, block *types.Block) error {
	if len(block.Extrinsic.Guarantees) < 2 {
		return nil
	}
	r := NewSeededRand(seed)
	i := r.Intn(len(block.Extrinsic.Guarantees) - 1)
	g0 := &block.Extrinsic.Guarantees[i]
	g1 := &block.Extrinsic.Guarantees[i+1]
	g0.Report.CoreIndex, g1.Report.CoreIndex = g1.Report.CoreIndex, g0.Report.CoreIndex
	return jamerrors.ErrGOutOfOrderGuarantee
}

// Work report per core gas is too much high.
func fuzzBlockGWorkReportGasTooHigh(seed []byte, block *types.Block) error {
	g := lastGuarantee(block)
	if g == nil {
		return nil
	}
	work_result := lastGuaranteeResult(g)
	if work_result == nil {
		return nil
	}
	work_result.Gas = types.AccumulationGasAllocation + 123
	return jamerrors.ErrGWorkReportGasTooHigh
}

// Accumulate gas is below the service minimum.
func fuzzBlockGServiceItemTooLow(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}

	work_result := randomGuaranteeResult(r, g)
	if work_result == nil {
		return nil
	}
	service_id := work_result.ServiceID

	service_acct, ok, err := s.GetService(service_id)
	if err != nil || !ok {
		return nil
	}
	min_item_gas := service_acct.GasLimitG
	if work_result.Gas >= min_item_gas {
		if 1 > min_item_gas {
			return nil
		}
		work_result.Gas = min_item_gas - 1
	}
	return jamerrors.ErrGServiceItemTooLow
}

// Validator index is too big.
func fuzzBlockGBadValidatorIndex(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil || len(g.Signatures) == 0 {
		return nil
	}
	i := r.Intn(len(g.Signatures))
	g.Signatures[i].ValidatorIndex = uint16(types.TotalValidators + r.Intn(10))
	return jamerrors.ErrGBadValidatorIndex
}

// Unexpected guarantor for work report core.
func fuzzBlockGWrongAssignment(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	// TODO: Sourabh - This logic needs to be implemented deterministically
	for _, _ = range s.GuarantorAssignments {
		if false { // uint16(i) == assignment.ValidatorIndex && assignment.CoreIndex == g.Report.CoreIndex {
			g.Report.CoreIndex = (g.Report.CoreIndex + 1) % types.TotalCores
			return jamerrors.ErrGWrongAssignment
		}
	}
	return nil
}

// Context anchor is not recent enough.
func fuzzBlockGAnchorNotRecent(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	anchor := g.Report.RefineContext.Anchor
	g.Report.RefineContext.Anchor = randomDifferentHash(r, anchor)
	return jamerrors.ErrGAnchorNotRecent
}

// Context Beefy MMR root doesn't match the one at anchor.
func fuzzBlockGBadBeefyMMRRoot(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	beefyRoot := g.Report.RefineContext.BeefyRoot
	g.Report.RefineContext.BeefyRoot = randomDifferentHash(r, beefyRoot)
	return jamerrors.ErrGBadBeefyMMRRoot
}

// Work result service identifier doesn't have any associated account in state.
func fuzzBlockGBadServiceID(seed []byte, block *types.Block, s *statedb.StateDB) error {
	t := s.GetStorage()
	if t == nil {
		return nil
	}
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	for i := range g.Report.Results {
		result := &g.Report.Results[i]
		_, ok, _ := t.GetService(result.ServiceID + 1)
		if !ok {
			result.ServiceID++
			return jamerrors.ErrGBadServiceID
		}
	}
	return nil
}

// Context state root doesn't match the one at anchor.
func fuzzBlockGBadStateRoot(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	stateRoot := g.Report.RefineContext.StateRoot
	g.Report.RefineContext.StateRoot = randomDifferentHash(r, stateRoot)
	return jamerrors.ErrGBadStateRoot
}

// Package was already available in recent history.
func fuzzBlockGDuplicatePackageRecentHistory(seed []byte, block *types.Block, s *statedb.StateDB) error {
	RecentBlocks := s.JamState.RecentBlocks.B_H
	if len(RecentBlocks) < 2 {
		return nil
	}

	anyReported := false
	var lastNonEmptyRecentBlock *statedb.HistoryState
	for i := len(RecentBlocks) - 1; i >= 0; i-- {
		recentBlock := RecentBlocks[i]
		if len(recentBlock.Reported) > 0 {
			anyReported = true
			lastNonEmptyRecentBlock = &recentBlock
			break
		}
	}

	if !anyReported {
		return nil
	}

	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	g.Report.AvailabilitySpec.WorkPackageHash = lastNonEmptyRecentBlock.Reported[0].WorkPackageHash
	g.Report.AvailabilitySpec.ExportedSegmentRoot = lastNonEmptyRecentBlock.Reported[0].SegmentRoot
	return jamerrors.ErrGDuplicatePackageRecentHistory
}

// Report guarantee slot is too old with respect to block slot.
func fuzzBlockGReportEpochBeforeLast(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}

	g_slot := g.Slot
	block_slot := block.Header.Slot
	if block_slot < types.ValidatorCoreRotationPeriod || g_slot < types.ValidatorCoreRotationPeriod {
		return nil
	}
	g_remainder := g_slot % types.ValidatorCoreRotationPeriod
	g.Slot -= (g_remainder + 1)
	return jamerrors.ErrGReportEpochBeforeLast
}

// Segments tree root lookup item not found in recent blocks history.
func fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	if len(g.Report.SegmentRootLookup) == 0 {
		return nil
	}
	idx := r.Intn(len(g.Report.SegmentRootLookup))
	g.Report.SegmentRootLookup[idx].WorkPackageHash = randomDifferentHash(r, g.Report.SegmentRootLookup[idx].WorkPackageHash)
	return jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks
}

// Segments tree root lookup item found in recent blocks history but with an unexpected value.
func fuzzBlockGSegmentRootLookupInvalidUnexpectedValue(seed []byte, block *types.Block) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	if len(g.Report.SegmentRootLookup) == 0 {
		return nil
	}
	idx := r.Intn(len(g.Report.SegmentRootLookup))
	g.Report.SegmentRootLookup[idx].SegmentRoot = randomDifferentHash(r, g.Report.SegmentRootLookup[idx].SegmentRoot)
	return jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue
}

// Target core without any authorizer.
func fuzzBlockGCoreWithoutAuthorizer(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}

	var emptyCores []int
	for coreIdx, authorizers := range s.JamState.AuthorizationsPool {
		if len(authorizers) == 0 {
			emptyCores = append(emptyCores, coreIdx)
		}
	}
	if len(emptyCores) == 0 {
		return nil
	}
	g.Report.CoreIndex = uint(emptyCores[r.Intn(len(emptyCores))])
	return jamerrors.ErrGCoreWithoutAuthorizer
}

// Target core with unexpected authorizer.
func fuzzBlockGCoreUnexpectedAuthorizer(seed []byte, block *types.Block, s *statedb.StateDB) error {
	r := NewSeededRand(seed)
	g := randomGuarantee(r, block)
	if g == nil {
		return nil
	}
	jamState := s.JamState
	authorizers := jamState.AuthorizationsPool[int(g.Report.CoreIndex)]
	if len(authorizers) == 0 {
		return nil
	}
	g.Report.AuthorizerHash = randomDifferentHash(r, g.Report.AuthorizerHash)
	return jamerrors.ErrGCoreUnexpectedAuthorizer
}
