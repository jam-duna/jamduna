package storage

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type JAMDA interface {
	FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (payload []byte, err error)
}

type MockJAMDA struct{}

func (m *MockJAMDA) FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) ([]byte, error) {
	fmt.Printf("MockJAMDA.FetchJAMDASegments called: workPackageHash=%s, indexStart=%d, indexEnd=%d, payloadLength=%d\n", workPackageHash.Hex(), indexStart, indexEnd, payloadLength)
	return []byte{}, nil
}

func (m *MockJAMDA) BuildBundleWithStateWitnesses(workPackage types.WorkPackage, extrinsicsBlobs []types.ExtrinsicsBlobs, coreIndex uint16, rawObjectIDs []common.Hash, pvmBackend string) (b *types.WorkPackageBundle, wr *types.WorkReport, err error) {
	return nil, nil, nil
}

// NewMockJAMDA creates a new MockJAMDA instance
func NewMockJAMDA() *MockJAMDA {
	return &MockJAMDA{}
}
