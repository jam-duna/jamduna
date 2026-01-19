package types

import (
	"context"

	"github.com/colorfulnotion/jam/common"
)

// JNode interface defines the methods that a JAM node must implement
// This interface is used by other packages to interact with the node without circular dependencies
type JNode interface {
	GetFinalizedBlock() (blk *Block, err error)
	SubmitAndWaitForWorkPackageBundle(ctx context.Context, b *WorkPackageBundle) (stateRoot common.Hash, ts uint32, err error)
	SubmitAndWaitForWorkPackageBundles(ctx context.Context, b []*WorkPackageBundle) (stateRoots []common.Hash, timeslots []uint32, err error)
	SubmitAndWaitForPreimage(ctx context.Context, serviceID uint32, preimage []byte) error
	GetWorkReport(requestedHash common.Hash) (*WorkReport, error)
	GetService(service uint32) (sa *ServiceAccount, ok bool, err error)
	GetServiceStorage(serviceID uint32, storageKey []byte) ([]byte, bool, error)
	GetRefineContext() (RefineContext, error)
	GetRefineContextWithBuffer(buffer int) (RefineContext, error)
	// BuildBundle executes a work package and generates witnesses.
	// skipApplyWrites: If true, skip storing/applying contract writes to state.
	// Use skipApplyWrites=true when Phase 1 has already applied state changes.
	BuildBundle(workPackage WorkPackage, extrinsics []ExtrinsicsBlobs, coreIndex uint16, rawObjectIDs []common.Hash, skipApplyWrites bool) (*WorkPackageBundle, *WorkReport, error)
	GetSegmentWithProof(segmentsRoot common.Hash, segmentIndex uint16) (segment []byte, importProof []common.Hash, found bool)
}
