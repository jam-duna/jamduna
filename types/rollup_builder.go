package types

import (
	"time"

	"github.com/jam-duna/jamduna/common"
)

type RollupInput struct {
	Payload any
}

type RollupBlock struct {
	Number       uint32
	Bytes        []byte
	Announcement UP1Announcement
	CreatedAt    time.Time
	Meta         any
}

type RollupRun struct {
	Blocks []RollupBlock
}

type WorkPackageOut struct{
	Bundle                     *WorkPackageBundle
	OriginalExtrinsics         []ExtrinsicsBlobs
	OriginalWorkItemExtrinsics [][]WorkItemExtrinsic
	OriginalPayloads           [][]byte
	TxHashes                   []common.Hash
	PreRoot                    common.Hash
	PostRoot                   common.Hash
	BlockCommitment            common.Hash
	BlockNumberEnd             uint64
}
