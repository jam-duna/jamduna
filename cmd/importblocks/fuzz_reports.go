package main

import (
	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"math/rand"
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
func fuzzBlockGDuplicatePackageRecentHistory(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for GReportEpochBeforeLast
	return nil
}

// Sean
func fuzzBlockGReportEpochBeforeLast(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for GReportEpochBeforeLast
	return nil
}

// Stanley
func fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks
	return nil
}

// Stanley
func fuzzBlockGSegmentRootLookupInvalidUnexpectedValue(block *types.Block, statedb *statedb.StateDB) error {
	// TODO: Implement fuzzing logic for fuzzBlockGSegmentRootLookupInvalidNotRecentBlocks
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
