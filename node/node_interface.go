package node

import (
	"context"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type JNode interface {
	GetFinalizedBlock() (blk *types.Block, err error)
	SetJCEManager(jceManager *ManualJCEManager) (err error)
	GetJCEManager() (jceManager *ManualJCEManager, err error)
	SubmitAndWaitForWorkPackage(ctx context.Context, wpr *WorkPackageRequest) (common.Hash, error)
	SubmitAndWaitForWorkPackages(ctx context.Context, wpr []*WorkPackageRequest) ([]common.Hash, error)
	SubmitAndWaitForPreimage(ctx context.Context, serviceID uint32, preimage []byte) error
	GetWorkReport(requestedHash common.Hash) (*types.WorkReport, error)
	GetService(service uint32) (sa *types.ServiceAccount, ok bool, err error)
	GetServiceStorage(serviceID uint32, storageKey []byte) ([]byte, bool, error)
	GetSegments(importedSegments []types.ImportSegment) (raw_segments [][]byte, err error)
	GetSegmentsByRequestedHash(requestedHashes common.Hash) (raw_segments [][]byte, ExportedSegmentLength uint16, err error)
}
