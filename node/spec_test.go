package node

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type SpecTestCase struct {
	B                string                      `json:"b"`
	BClubs           []common.Hash               `json:"bClubs,omitempty"`
	BShards          []string                    `json:"bShards,omitempty"`
	SClubs           []common.Hash               `json:"sClubs,omitempty"`
	SShards          []string                    `json:"sShards,omitempty"`
	AvailabilitySpec types.AvailabilitySpecifier `json:"package_spec,omitempty"`
}

func testAvailabilitySpec(t *testing.T, exportCount uint16) {
	b := []byte{0x14, 0x21, 0xdd, 0xac, 0x87, 0x3a, 0x19, 0x9a, 0x7c, 0x87}
	bLength := uint32(len(b))

	packageHash := common.ComputeHash(b)
	// Compute a hash chain of 32-byte hashes using common.ComputeHash
	h := common.ComputeHash([]byte{})

	// Fill 65 segments of 4104 bytes each with hashes
	if debugSpec {
		fmt.Printf("H('') = %x  ", h)
	}
	segments := make([][]byte, exportCount)
	for i := range segments {
		segments[i] = make([]byte, 4104)
		for j := 0; j < 4104; j += 32 {
			copy(segments[i][j:], h[:])
			h = common.ComputeHash(h)
			if debugSpec {
				if i == 0 {
					if j < 128 {
						fmt.Printf("<= %x ", h)
					} else if j == 128 {
						fmt.Printf("\n")
					}
				}
			}
		}
		if debugSpec {
			fmt.Printf("segment %d: %x... (%d bytes)\n", i, segments[i][0:36], len(segments[i]))
		}
	}
	n := &Node{}
	// Build b♣ and s♣
	bClubs, bShards := n.buildBClub(b)
	sClubs, sShards := n.buildSClub(segments)
	// u = (bClub, sClub)
	erasure_root_u := generateErasureRoot(bClubs, sClubs)
	exported_segment_root_e := generateExportedSegmentsRoot(segments)

	// Return the Availability Specifier
	availabilitySpecifier := types.AvailabilitySpecifier{
		WorkPackageHash:       common.BytesToHash(packageHash),
		BundleLength:          bLength,
		ErasureRoot:           erasure_root_u,
		ExportedSegmentRoot:   exported_segment_root_e,
		ExportedSegmentLength: uint16(exportCount),
	}

	bchunks := make([]string, len(bShards))
	for i, bS := range bShards {
		bchunks[i] = fmt.Sprintf("0x%x", bS.Data)
	}
	schunks := make([]string, len(sShards))
	for i, sS := range sShards {
		schunks[i] = fmt.Sprintf("0x%x", sS.Data)
	}

	tc := SpecTestCase{
		B:                fmt.Sprintf("0x%x", b),
		BClubs:           bClubs,
		BShards:          bchunks,
		SClubs:           sClubs,
		SShards:          schunks,
		AvailabilitySpec: availabilitySpecifier,
	}
	fn := fmt.Sprintf("spec-exportcount-%d.json", exportCount)
	// Save tc in a JSON file
	jsonData, err := json.MarshalIndent(tc, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal SpecTestCase to JSON: %v", err)
	}
	err = os.WriteFile(fn, jsonData, 0644)
	if err != nil {
		t.Fatalf("Failed to write JSON file: %v", err)
	}
	fmt.Printf("AvailabilitySpecifier (exportCount=%d): %s\n", exportCount, availabilitySpecifier.String())
}

func TestAvailabilitySpec(t *testing.T) {
	exportCounts := []uint16{1, 2, 3, 64, 65, 128, 129}
	for _, exportCount := range exportCounts {
		testAvailabilitySpec(t, exportCount)
	}
}
