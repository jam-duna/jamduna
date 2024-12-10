package main

import (
	"math/rand"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

/*
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
*/

// reports fuzzBlocks
func randomGuarantee(block *types.Block) *types.Guarantee {
	return &(block.Extrinsic.Guarantees[rand.Intn(len(block.Extrinsic.Guarantees))])
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

// Sean
func fuzzBlockGBadCodeHash(block *types.Block) error {
	// TODO: Implement fuzzing logic for GBadCodeHash
	return nil
}

// Sourabh
func fuzzBlockGBadCoreIndex(block *types.Block) error {
	g := randomGuarantee(block)
	if g != nil {
		g.Report.CoreIndex = uint16(types.TotalCores + rand.Intn(10))
		return jamerrors.ErrGBadCoreIndex
	}
	return nil
}

// Sourabh
func fuzzBlockGBadSignature(block *types.Block) error {
	g := randomGuarantee(block)
	if g != nil {
		// pick a random signature and update it
		s := &(g.Signatures[rand.Intn(len(g.Signatures))])
		s.Signature = types.GenerateRandomEd25519Signature()
		return jamerrors.ErrGBadSignature
	}
	return nil
}

// Sean
func fuzzBlockGCoreEngaged(block *types.Block) error {
	// TODO: Implement fuzzing logic for GCoreEngaged
	return nil
}

// Sean
func fuzzBlockGDependencyMissing(block *types.Block) error {
	// TODO: Implement fuzzing logic for GDependencyMissing
	return nil
}

// Sean
func fuzzBlockGDuplicatePackageTwoReports(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for GDuplicatePackage
	return nil
}

// Sean
func fuzzBlockGFutureReportSlot(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for GFutureReportSlot
	return nil
}

// Sourabh
func fuzzBlockGInsufficientGuarantees(block *types.Block) error {
	g := randomGuarantee(block)
	if g != nil {
		g.Signatures = g.Signatures[0:1] // only 1
		return jamerrors.ErrGInsufficientGuarantees
	}
	return nil
}

// Sean
func fuzzBlockGDuplicateGuarantors(block *types.Block) error {
	// TODO: Implement fuzzing logic for GDuplicateGuarantors
	return nil
}

// Sourabh
func fuzzBlockGOutOfOrderGuarantee(block *types.Block) error {
	if len(block.Extrinsic.Guarantees) >= 2 {
		// TODO: reverse order
	}
	return nil
}

// Michael
func fuzzBlockGWorkReportGasTooHigh(block *types.Block) error {
	// TODO: Implement fuzzing logic for GWorkReportGasTooHigh
	return nil
}

// Sourabh
func fuzzBlockGBadValidatorIndex(block *types.Block) error {
	g := randomGuarantee(block)
	if g != nil {
		i := rand.Intn(len(g.Signatures))
		g.Signatures[i].ValidatorIndex = uint16(types.TotalValidators + rand.Intn(10))
		return jamerrors.ErrGBadValidatorIndex
	}
	return nil
}

// Sean
func fuzzBlockGWrongAssignment(block *types.Block) error {
	// TODO: Implement fuzzing logic for GWrongAssignment
	return nil
}

// Michael
func fuzzBlockGAnchorNotRecent(block *types.Block) error {
	// TODO: Implement fuzzing logic for GAnchorNotRecent
	return nil
}

// Stanley
func fuzzBlockGBadBeefyMMRRoot(block *types.Block) error {
	// TODO: Implement fuzzing logic for GBadBeefyMMRRoot
	g := randomGuarantee(block)
	if g != nil {
		beefyRoot := g.Report.RefineContext.BeefyRoot
		g.Report.RefineContext.BeefyRoot = randomDifferentHash(beefyRoot)
		return jamerrors.ErrGBadBeefyMMRRoot
	}
	return nil
}

// Sourabh
func fuzzBlockGBadServiceID(block *types.Block, statedb *statedb.StateDB) error {
	t := statedb.GetTrie()
	if t == nil {
		return nil
	}
	g := randomGuarantee(block)
	if g != nil {
		// get service from trie
		for _, result := range g.Report.Results {
			_, ok, _ := t.GetService(255, result.ServiceID+1)
			if !ok {
				result.ServiceID++
				return jamerrors.ErrGBadServiceID
			}
		}
	}
	return nil
}

// Michael
func fuzzBlockGBadStateRoot(block *types.Block, statedb *statedb.StateDB) error {

	if false {
		return jamerrors.ErrGBadStateRoot
	}
	return nil
}

// Stanley
func fuzzBlockGDuplicatePackageRecentHistory(statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for GReportEpochBeforeLast
	RecentBlocks := statedb.JamState.RecentBlocks
	if len(RecentBlocks) > 0 {
		// pick a random block from recent blocks
		if len(RecentBlocks) == 1 {
			RecentBlocks = append(RecentBlocks, RecentBlocks[0])
			statedb.JamState.RecentBlocks = RecentBlocks
			return jamerrors.ErrGDuplicatePackageRecentHistory
		} else {
			RecentBlocks[len(RecentBlocks)-1] = RecentBlocks[len(RecentBlocks)-2]
			statedb.JamState.RecentBlocks = RecentBlocks
			return jamerrors.ErrGDuplicatePackageRecentHistory
		}
	}
	return nil
}

// Sean
func fuzzBlockGReportEpochBeforeLast(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for GReportEpochBeforeLast
	return nil
}

// Stanley
func fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks(block *types.Block) error {
	// TODO: Implement fuzzing logic for fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks
	g := randomGuarantee(block)
	if g != nil {
		// Clear the current SegmentRootLookup and add random non-matching entries
		g.Report.SegmentRootLookup = nil
		for _, segmentRootLookup := range g.Report.SegmentRootLookup {
			// Add random work package hashes and segment roots
			g.Report.SegmentRootLookup = append(g.Report.SegmentRootLookup, types.SegmentRootLookupItem{
				WorkPackageHash: randomDifferentHash(segmentRootLookup.WorkPackageHash),
				SegmentRoot:     segmentRootLookup.SegmentRoot,
			})
		}
		return jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks
	}
	return nil
}

// Stanley
func fuzzBlockGSegmentRootLookupInvalidUnexpectedValue(block *types.Block) error {
	// TODO: Implement fuzzing logic for fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks
	g := randomGuarantee(block)
	if g != nil {
		// Clear the current SegmentRootLookup and add random non-matching entries
		g.Report.SegmentRootLookup = nil
		for _, segmentRootLookup := range g.Report.SegmentRootLookup {
			// Add random work package hashes and segment roots
			g.Report.SegmentRootLookup = append(g.Report.SegmentRootLookup, types.SegmentRootLookupItem{
				WorkPackageHash: segmentRootLookup.WorkPackageHash,
				SegmentRoot:     randomDifferentHash(segmentRootLookup.SegmentRoot),
			})
		}
		return jamerrors.ErrGSegmentRootLookupInvalidNotRecentBlocks
	}
	return nil
}

// Shawn
func fuzzBlockGCoreWithoutAuthorizer(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for GCoreWithoutAuthorizer
	return nil
}

// Shawn
func fuzzBlockGCoreUnexpectedAuthorizer(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for GCoreUnexpectedAuthorizer
	return nil
}
