package storage

import (
	"fmt"
	"sync"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/types"
)

type MockJAMDA struct {
	mu       sync.RWMutex
	segments map[common.Hash]map[uint16][]byte // workPackageHash -> segmentIndex -> segmentData
}

func (m *MockJAMDA) StoreSegment(workPackageHash common.Hash, segmentIndex uint16, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.segments[workPackageHash] == nil {
		m.segments[workPackageHash] = make(map[uint16][]byte)
	}
	m.segments[workPackageHash][segmentIndex] = data
}

func (m *MockJAMDA) FetchJAMDASegments(workPackageHash common.Hash, indexStart uint16, indexEnd uint16, payloadLength uint32) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	wpSegments, ok := m.segments[workPackageHash]
	if !ok {
		return []byte{}, nil
	}

	var payload []byte
	for i := indexStart; i < indexEnd; i++ {
		segment, exists := wpSegments[i]
		if !exists {
			return []byte{}, fmt.Errorf("MockJAMDA segment %d not found", i)
		}
		payload = append(payload, segment...)
	}

	// Truncate to expected payload length
	if uint32(len(payload)) > payloadLength {
		payload = payload[:payloadLength]
	}

	return payload, nil
}

func (m *MockJAMDA) fetchSegment(workPackageHash common.Hash, idx uint16) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	wpSegments, ok := m.segments[workPackageHash]
	if !ok {
		fmt.Printf("MockJAMDA.FetchJAMDASegments: no segments found for workPackageHash=%s (available: ", workPackageHash.Hex())
		for wph := range m.segments {
			fmt.Printf("%s ", wph.Hex())
		}
		fmt.Printf(")\n")
		return []byte{}, nil
	}

	segment, exists := wpSegments[idx]
	if !exists {
		fmt.Printf("fetchSegment: segment not found for workPackageHash=%s idx=%d\n",
			workPackageHash.Hex(), idx)
		return []byte{}, fmt.Errorf("segment %d not found", idx)
	}
	return segment, nil
}

func (m *MockJAMDA) StoreBundleSpecSegments(as *types.AvailabilitySpecifier, d types.AvailabilitySpecifierDerivation, b types.WorkPackageBundle, segments [][]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workPackageHash := as.WorkPackageHash
	// fmt.Printf("MockJAMDA.StoreBundleSpecSegments: workPackageHash=%s, %d segments\n",
	// 	workPackageHash.Hex(), len(segments))

	// Store each exported segment by index
	for segmentIdx, segmentData := range segments {
		if m.segments[workPackageHash] == nil {
			m.segments[workPackageHash] = make(map[uint16][]byte)
		}
		m.segments[workPackageHash][uint16(segmentIdx)] = segmentData
		// fmt.Printf("====> StoreBundleSpecSegments. Stored segment[%s][%d]: %d bytes\n", workPackageHash.Hex(), segmentIdx, len(segmentData))
	}
}

func (m *MockJAMDA) BuildBundleFromWPQueueItem(wpQueueItem *types.WPQueueItem) (bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, err error) {
	workPackage := wpQueueItem.WorkPackage
	segmentRootLookup = make(types.SegmentRootLookup, 0)
	// Remap the segments to [workItemIndex][importedSegmentIndex][bytes]
	importSegments := make([][][]byte, len(workPackage.WorkItems))
	justifications := make([][][]common.Hash, len(workPackage.WorkItems))

	for workItemIndex, workItem := range workPackage.WorkItems {
		importSegments[workItemIndex] = make([][]byte, len(workItem.ImportedSegments))
		justifications[workItemIndex] = make([][]common.Hash, len(workItem.ImportedSegments))

		for idx, impseg := range workItem.ImportedSegments {
			segment, err := m.fetchSegment(impseg.RequestedHash, uint16(impseg.Index))
			if err != nil {
				return bundle, segmentRootLookup, fmt.Errorf("failed to fetch segment for workItem %d, importSeg %d: %v", workItemIndex, idx, err)
			}
			importSegments[workItemIndex][idx] = segment
			justifications[workItemIndex][idx] = []common.Hash{} // Empty justifications for mock
		}
	}

	bundle = types.WorkPackageBundle{
		WorkPackage:       workPackage,
		ExtrinsicData:     []types.ExtrinsicsBlobs{wpQueueItem.Extrinsics},
		ImportSegmentData: importSegments,
		Justification:     justifications,
	}

	return bundle, segmentRootLookup, nil
}

func (m *MockJAMDA) BuildWPQueueItem(workPackage types.WorkPackage, extrinsicsBlobs []types.ExtrinsicsBlobs, coreIndex uint16, rawObjectIDs []common.Hash, pvmBackend string) (b *types.WorkPackageBundle, wr *types.WorkReport, err error) {
	return nil, nil, nil
}

// NewMockJAMDA creates a new MockJAMDA instance
func NewMockJAMDA() *MockJAMDA {
	return &MockJAMDA{
		segments: make(map[common.Hash]map[uint16][]byte),
	}
}
