package types

import (
	"context"

	"github.com/jam-duna/jamduna/common"
)

type Rollup interface {
	GetServiceID() uint32
	GetLatestBlockNumber() (uint32, error)
	ValidateRollupAnnouncement(ann UP1Announcement) error
	ImportRollupBlock(ann UP1Announcement) error
	ApplyBlock(rollupBlock []byte) error
	RollbackToBlock(blockNumber uint32) error
	OnCheckpointFinalized(height uint64, round uint64, checkpointHash common.Hash)
	BroadcastRollupAnnouncement(ann UP1Announcement) error
	EnqueueRollupAnnouncement(ann UP1Announcement)
	DumpCheckpoint(path string) error
	RecoverCheckpoint(path string) (uint32, error)
	WriteRollupBlock(blockNumber uint32, rollupBlock []byte) error
	ReadRollupBlock(blockNumber uint32) ([]byte, bool, error)
	ResetFromRollupState() error
	BuildRollupBlock(ctx context.Context, blockNumber uint32, input RollupInput) (RollupBlock, error)
	BuildWorkPackage(
		ctx context.Context,
		run RollupRun,
		refineCtx RefineContext,
		originalExtrinsics []ExtrinsicsBlobs,
		originalWorkItemExtrinsics [][]WorkItemExtrinsic,
		originalPayloads [][]byte,
		preRoot common.Hash,
		postRoot common.Hash,
	) (WorkPackageOut, RollupRun, RollupRun, error)
	CommitRun(ctx context.Context, run RollupRun) error
	OnRunFailed(ctx context.Context, run RollupRun) error
}

type BundleWitnessRebuilder interface {
	RebuildBundleWitnessOnly(
		ctx context.Context,
		bundle *WorkPackageBundle,
		refineCtx RefineContext,
	) (*WorkPackageBundle, error)
}
