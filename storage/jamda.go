package storage

import (
	"fmt"

	"github.com/colorfulnotion/jam/common"
)

type JAMDA interface {
	FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) (payload []byte, err error)
}

type MockJAMDA struct{}

func (m *MockJAMDA) FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) ([]byte, error) {
	fmt.Printf("MockJAMDA.FetchJAMDASegments called: workPackageHash=%s, indexStart=%d, indexEnd=%d, payloadLength=%d\n", workPackageHash.Hex(), indexStart, indexEnd, payloadLength)
	return []byte{}, nil
}

// NewMockJAMDA creates a new MockJAMDA instance
func NewMockJAMDA() *MockJAMDA {
	return &MockJAMDA{}
}
