package fuzz

import (
	"math/rand"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

/*
TODO - Michael:
fuzzBlockGDependencyMissing
fuzzBlockGWorkReportGasTooHigh
fuzzBlockGAnchorNotRecent
fuzzBlockGBadBeefyMMRRoot
fuzzBlockGBadStateRoot
fuzzBlockGDuplicatePackageRecentHistory
fuzzBlockGReportEpochBeforeLast
fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks
fuzzBlockGSegmentRootLookupInvalidUnexpectedValue
fuzzBlockGCoreWithoutAuthorizer
fuzzBlockGCoreUnexpectedAuthorizer
fuzzBlockGServiceItemTooLow

type Guarantee struct {
	Report     WorkReport            `json:"report"`
	Slot       uint32                `json:"slot"`
	Signatures []GuaranteeCredential `json:"signatures"`
}
type WorkReport struct {
	AvailabilitySpec  AvailabilitySpecifier `json:"package_spec"`
	RefineContext     RefineContext         `json:"context"`
	CoreIndex         uint16                `json:"core_index"`
	AuthorizerHash    common.Hash           `json:"authorizer_hash"`
	AuthOutput        []byte                `json:"auth_output"`
	SegmentRootLookup Hash2Hash             `json:"segment_root_lookup"`
	Results           []WorkResult          `json:"results"`
}

type RefineContext struct {
	Anchor           common.Hash   `json:"anchor"`
	StateRoot        common.Hash   `json:"state_root"`
	BeefyRoot        common.Hash   `json:"beefy_root"`
	LookupAnchor     common.Hash   `json:"lookup_anchor"`
	LookupAnchorSlot uint32        `json:"lookup_anchor_slot"`
	Prerequisites    []common.Hash `json:"prerequisites"`
}

type AvailabilitySpecifier struct {
	WorkPackageHash       common.Hash `json:"hash"`
	BundleLength          uint32      `json:"length"`
	ErasureRoot           common.Hash `json:"erasure_root"`
	ExportedSegmentRoot   common.Hash `json:"exports_root"`
	ExportedSegmentLength uint16      `json:"exports_length"`
}

type WorkResult struct {
	ServiceID   uint32      `json:"service_id"`
	CodeHash    common.Hash `json:"code_hash"`
	PayloadHash common.Hash `json:"payload_hash"`
	Gas         uint64      `json:"accumulate_gas"`
	Result      Result      `json:"result"`
}
*/

// reports fuzzBlocks
func randomGuarantee(block *types.Block) *types.Guarantee {
	if len(block.Extrinsic.Guarantees) == 0 {
		return nil
	}
	return &(block.Extrinsic.Guarantees[rand.Intn(len(block.Extrinsic.Guarantees))])
}

func lastGuarantee(block *types.Block) *types.Guarantee {
	if len(block.Extrinsic.Guarantees) == 0 {
		return nil
	}
	return &(block.Extrinsic.Guarantees[len(block.Extrinsic.Guarantees)-1])
}

func randomGuaranteeResult(g *types.Guarantee) *types.WorkResult {
	if len(g.Report.Results) == 0 {
		return nil
	}
	return &(g.Report.Results[rand.Intn(len(g.Report.Results))])
}

func lastGuaranteeResult(g *types.Guarantee) *types.WorkResult {
	if len(g.Report.Results) == 0 {
		return nil
	}
	return &(g.Report.Results[len(g.Report.Results)-1])
}

// Generate a random hash
func randomHash() common.Hash {
	var b [32]byte
	_, err := rand.Read(b[:]) // Fill b with random bytes
	if err != nil {
		panic("randomHash failed to generate random hash: " + err.Error())
	}
	return common.BytesToHash(b[:])
}

// Generate a random hash that is different from the input hash
func randomDifferentHash(input common.Hash) common.Hash {
	var b [32]byte
	for {
		_, err := rand.Read(b[:]) // Generate random bytes
		if err != nil {
			panic("randomDifferentHash failed to generate random hash: " + err.Error())
		}

		newHash := common.BytesToHash(b[:])
		if newHash != input { // Check if the new hash is different from the input
			return newHash
		}
	}
}

// randomDifferentHash generates a random hash that is different from all hashes in the input slice
func randomDifferentHashes(input []common.Hash) common.Hash {
	var b [32]byte
	for {
		_, err := rand.Read(b[:]) // Generate random bytes
		if err != nil {
			panic("randomDifferentHashes failed to generate random hash: " + err.Error())
		}
		newHash := common.BytesToHash(b[:])

		// Check if newHash is different from all hashes in input
		isUnique := true
		for _, h := range input {
			if newHash == h {
				isUnique = false
				break
			}
		}

		if isUnique {
			return newHash
		}
	}
}

// Work result code hash doesn't match the one expected for the service.
func fuzzBlockGBadCodeHash(block *types.Block) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	if len(g.Report.Results) == 0 {
		return nil
	}
	i := rand.Intn(len(g.Report.Results))
	g.Report.Results[i].CodeHash = randomHash()
	return jamerrors.ErrGBadCoreIndex
}

// Core index is too big.
func fuzzBlockGBadCoreIndex(block *types.Block) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	g.Report.CoreIndex = uint16(types.TotalCores + rand.Intn(10))
	return jamerrors.ErrGBadCoreIndex
}

// Invalid report guarantee signature.
func fuzzBlockGBadSignature(block *types.Block) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	// pick a random signature and update it
	s := &(g.Signatures[rand.Intn(len(g.Signatures))])
	s.Signature = types.GenerateRandomEd25519Signature()
	return jamerrors.ErrGBadSignature
}

// A core is not available.
// TODO: we need to mutate the pre-state, not the Block
func fuzzBlockGCoreEngaged(block *types.Block, s *statedb.StateDB) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	j := s.JamState
	if j.AvailabilityAssignments[int(g.Report.CoreIndex)] == nil {
		return nil
	}
	// for the g
	j.AvailabilityAssignments[int(g.Report.CoreIndex)] = &statedb.Rho_state{
		WorkReport: g.Report,
		Timeslot:   block.Header.Slot,
	}
	return jamerrors.ErrGCoreEngaged
}

// Prerequisite is missing.
func fuzzBlockGDependencyMissing(block *types.Block, s *statedb.StateDB) error {
	// TODO: Michael
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}

	// (1) EG has unknown packageHash in prerequisites that cannot be found in the other E_G's
	g.Report.RefineContext.Prerequisites = append(g.Report.RefineContext.Prerequisites, randomHash())

	// (2) nor from the recent_history state

	return jamerrors.ErrGDependencyMissing
}

// Report contains a duplicate package (send two reports from same package)
func fuzzBlockGDuplicatePackageTwoReports(block *types.Block, s *statedb.StateDB) error {
	if len(block.Extrinsic.Guarantees) < 2 {
		return nil
	}
	i := rand.Intn(len(block.Extrinsic.Guarantees) - 1)
	g0 := &block.Extrinsic.Guarantees[i]
	g1 := &block.Extrinsic.Guarantees[i+1]

	if rand.Intn(10) < 5 {
		g0.Report.AvailabilitySpec.WorkPackageHash = g1.Report.AvailabilitySpec.WorkPackageHash
	} else {
		g1.Report.AvailabilitySpec.WorkPackageHash = g0.Report.AvailabilitySpec.WorkPackageHash
	}
	return nil
}

// Report refers to a slot in the future with respect to container block slot.
func fuzzBlockGFutureReportSlot(block *types.Block) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	g.Slot = block.Header.Slot + 1 + uint32(rand.Intn(100))
	return jamerrors.ErrGFutureReportSlot
}

// Report with no enough guarantors signatures.
func fuzzBlockGInsufficientGuarantees(block *types.Block) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	g.Signatures = g.Signatures[0:1] // only 1
	return jamerrors.ErrGInsufficientGuarantees
}

// Guarantors indices are not sorted or unique.
func fuzzBlockGDuplicateGuarantors(block *types.Block) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	if len(g.Signatures) < 2 {
		return nil
	}
	// swap  2 signatures
	i := rand.Intn(len(g.Signatures))
	j := (i + 1) % len(g.Signatures)
	g.Signatures[i], g.Signatures[j] = g.Signatures[j], g.Signatures[i]
	return nil
}

// Reports cores are not sorted or unique.
func fuzzBlockGOutOfOrderGuarantee(block *types.Block) error {
	if len(block.Extrinsic.Guarantees) < 2 {
		return nil
	}
	i := rand.Intn(len(block.Extrinsic.Guarantees) - 1)
	g0 := &block.Extrinsic.Guarantees[i]
	g1 := &block.Extrinsic.Guarantees[i+1]
	// swap g0.Report.CoreIndex and g1.Report.CoreIndex
	tmp := g0.Report.CoreIndex
	g0.Report.CoreIndex = g1.Report.CoreIndex
	g1.Report.CoreIndex = tmp
	return jamerrors.ErrGOutOfOrderGuarantee
}

// Work report per core gas is too much high.
func fuzzBlockGWorkReportGasTooHigh(block *types.Block) error {
	// exceeding G_A or types.AccumulationGasAllocation
	g := lastGuarantee(block)
	if g == nil {
		return nil
	}
	work_result := lastGuaranteeResult(g)
	work_result.Gas = types.AccumulationGasAllocation + 123
	return jamerrors.ErrGWorkReportGasTooHigh
}

// Accumulate gas is below the service minimum.
func fuzzBlockGServiceItemTooLow(block *types.Block, s *statedb.StateDB) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}

	work_result := randomGuaranteeResult(g)
	service_id := work_result.ServiceID

	v, ok, err := s.GetTrie().GetService(service_id)
	if err != nil || !ok {
		return nil
	}
	service_acct, err := types.ServiceAccountFromBytes(service_id, v)
	if err != nil {
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
func fuzzBlockGBadValidatorIndex(block *types.Block) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	i := rand.Intn(len(g.Signatures))
	g.Signatures[i].ValidatorIndex = uint16(types.TotalValidators + rand.Intn(10))
	return jamerrors.ErrGBadValidatorIndex
}

// Unexpected guarantor for work report core.
func fuzzBlockGWrongAssignment(block *types.Block, s *statedb.StateDB) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	// TODO: Sourabh
	for _, _ = range s.GuarantorAssignments {
		if false { // uint16(i) == assignment.ValidatorIndex && assignment.CoreIndex == g.Report.CoreIndex {
			g.Report.CoreIndex = (g.Report.CoreIndex + 1) % types.TotalCores
			return jamerrors.ErrGWrongAssignment
		}
	}
	return nil
}

// Context anchor is not recent enough.
func fuzzBlockGAnchorNotRecent(block *types.Block, s *statedb.StateDB) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	anchor := g.Report.RefineContext.Anchor
	g.Report.RefineContext.Anchor = randomDifferentHash(anchor)
	return jamerrors.ErrGAnchorNotRecent
}

// Context Beefy MMR root doesn't match the one at anchor.
func fuzzBlockGBadBeefyMMRRoot(block *types.Block, s *statedb.StateDB) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	beefyRoot := g.Report.RefineContext.BeefyRoot
	g.Report.RefineContext.BeefyRoot = randomDifferentHash(beefyRoot)
	// go one step further and use recently forgotten "recentblocks"
	return jamerrors.ErrGBadBeefyMMRRoot
}

// Work result service identifier doesn't have any associated account in state.
func fuzzBlockGBadServiceID(block *types.Block, s *statedb.StateDB) error {
	t := s.GetTrie()
	if t == nil {
		return nil
	}
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	// get service from trie
	for _, result := range g.Report.Results {
		_, ok, _ := t.GetService(result.ServiceID + 1)
		if !ok {
			result.ServiceID++
			// TODO: make sure there is no serviceID in state
			return jamerrors.ErrGBadServiceID
		}
	}
	return nil
}

// Context state root doesn't match the one at anchor.
func fuzzBlockGBadStateRoot(block *types.Block, s *statedb.StateDB) error {
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	stateRoot := g.Report.RefineContext.StateRoot
	g.Report.RefineContext.StateRoot = randomDifferentHash(stateRoot)
	return jamerrors.ErrGBadStateRoot
}

// Package was already available in recent history.
func fuzzBlockGDuplicatePackageRecentHistory(block *types.Block, s *statedb.StateDB) error {
	RecentBlocks := s.JamState.RecentBlocks
	if len(RecentBlocks) < 2 {
		return nil
	}

	// loop through recent blocks and replace EG one already found in the core starting from last record in recent blocks
	anyReported := false
	lastNonEmptyRecentBlock := &statedb.Beta_state{}
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

	// TODO: use package hash to properly fetch a valid EG.

	//In the absence of it, we will mutate wr into something totally invalid...
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	g.Report.AvailabilitySpec.WorkPackageHash = lastNonEmptyRecentBlock.Reported[0].WorkPackageHash
	g.Report.AvailabilitySpec.ExportedSegmentRoot = lastNonEmptyRecentBlock.Reported[0].SegmentRoot
	return jamerrors.ErrGDuplicatePackageRecentHistory
}

// Report guarantee slot is too old with respect to block slot.
func fuzzBlockGReportEpochBeforeLast(block *types.Block, s *statedb.StateDB) error {
	// Concerning types.ValidatorCoreRotationPeriod (R)
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}

	g_slot := g.Slot
	block_slot := block.Header.Slot
	if block_slot < types.ValidatorCoreRotationPeriod || g_slot < types.ValidatorCoreRotationPeriod {
		return nil
	}
	// TODO: not sure if it's ValidatorCoreRotationPeriod or 2*ValidatorCoreRotationPeriod
	// 22 -> 22 mod 4 = 5 ... 2; want 19
	g_remainder := g_slot % types.ValidatorCoreRotationPeriod
	g.Slot -= (g_remainder + 1)
	return jamerrors.ErrGReportEpochBeforeLast
}

// Segments tree root lookup item not found in recent blocks history.
func fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks(block *types.Block) error {
	// Invalid packageHash in SegmentRootLookupItem
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	// Mutate SegmentRootLookup's PackageHash
	g.Report.SegmentRootLookup = nil
	for idx, segmentRootLookup := range g.Report.SegmentRootLookup {
		g.Report.SegmentRootLookup[idx].WorkPackageHash = randomDifferentHash(segmentRootLookup.WorkPackageHash)
	}
	return jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks
}

// Segments tree root lookup item found in recent blocks history but with an unexpected value.
func fuzzBlockGSegmentRootLookupInvalidUnexpectedValue(block *types.Block) error {
	// Invalid exportedRoot in SegmentRootLookupItem
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	// Mutate SegmentRootLookup's PackageHash
	for idx, segmentRootLookup := range g.Report.SegmentRootLookup {
		g.Report.SegmentRootLookup[idx].SegmentRoot = randomDifferentHash(segmentRootLookup.WorkPackageHash)
	}
	return jamerrors.ErrGSegmentRootLookupInvalidUnexpectedValue
}

// Target core without any authorizer.
func fuzzBlockGCoreWithoutAuthorizer(block *types.Block, s *statedb.StateDB) error {
	// [Harder] auth_pool has empty authorizer at the core, harder if want to nullify the auth_pool state
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}

	anyEmptyCore := false
	emptyCores := make([]int, 0)

	// find an empty core to fuzz
	for coreIdx, authorizers := range s.JamState.AuthorizationsPool {
		if len(authorizers) == 0 {
			anyEmptyCore = true
			emptyCores = append(emptyCores, coreIdx)
		}
	}
	if !anyEmptyCore {
		return nil
	}
	g.Report.CoreIndex = uint16(emptyCores[rand.Intn(len(emptyCores))])

	return jamerrors.ErrGCoreWithoutAuthorizer
}

// Target core with unexpected authorizer.
func fuzzBlockGCoreUnexpectedAuthorizer(block *types.Block, s *statedb.StateDB) error {
	// auth_pool has non-empty authorizer at the core
	g := randomGuarantee(block)
	if g == nil {
		return nil
	}
	// auth_pool must be non-empty to such fuzzing
	jamState := s.JamState
	authorizers := jamState.AuthorizationsPool[int(g.Report.CoreIndex)]
	if len(authorizers) == 0 {
		return nil
	}
	g.Report.AuthorizerHash = randomDifferentHash(g.Report.AuthorizerHash)
	return jamerrors.ErrGCoreUnexpectedAuthorizer
}
