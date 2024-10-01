package node

import (
	//"context"
	//"crypto/rand"
	"fmt"
	"reflect"

	//"sync"
	//"time"

	//"encoding/json"
	"testing"

	//"io/ioutil"
	//"os"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/trie"

	//github.com/colorfulnotion/jam/pvm"
	//"github.com/colorfulnotion/jam/statedb"
	//"github.com/colorfulnotion/jam/trie"

	"github.com/colorfulnotion/jam/types"
)

// computeFib calculates Fib[n] and Fib[n-1]
func computeFib(n int) (uint32, uint32) {
	if n == 1 {
		return 1, 0
	}
	a, b := uint32(1), uint32(0)
	for i := 2; i <= n; i++ {
		a, b = a+b, a
	}
	return a, b
}

// computeHexValue constructs the 12-byte hex value
func fibNSegment(n int) ([]byte, error) {
	if n < 1 || n > 46 {
		return nil, fmt.Errorf("n must be between 1 and 46 to avoid uint32 overflow")
	}

	fibN, fibNMinus1 := computeFib(n)

	segment := make([]byte, 12)

	// Write n as uint32 in big endian
	byte_n := common.Uint32ToBytes(uint32(n))

	// Write Fib[n] as uint32 in big endian
	byte_fibN := common.Uint32ToBytes(uint32(fibN))

	// Write Fib[n-1] as uint32 in big endian
	byte_fibN_1 := common.Uint32ToBytes(uint32(fibNMinus1))

	copy(segment[0:4], byte_n)
	copy(segment[4:8], byte_fibN)
	copy(segment[8:12], byte_fibN_1)

	return segment, nil
}

func RecoverFibNSegment(segment []byte) (int, uint32, uint32, error) {
	if len(segment) != 12 {
		return 0, 0, 0, fmt.Errorf("segment must be 12 bytes long")
	}

	// Extract bytes for n, Fib[n], and Fib[n-1]
	nBytes := segment[0:4]
	fibNBytes := segment[4:8]
	fibNMinus1Bytes := segment[8:12]

	// Convert bytes to uint32 values (big endian)
	nUint32 := common.BytesToUint32(nBytes)
	fibN := common.BytesToUint32(fibNBytes)
	fibNMinus1 := common.BytesToUint32(fibNMinus1Bytes)

	n := int(nUint32)

	return n, fibN, fibNMinus1, nil
}

func testWorkpackage(fibN int) (workPackage types.WorkPackage, segments [][]byte) {
	segment, _ := fibNSegment(fibN)
	segments = append(segments, segment)

	fmt.Printf("\n\n\n********************** FIB N=%v Starts **********************\n", fibN)
	importedSegments := make([]types.ImportSegment, 0)
	refine_context := types.RefineContext{}

	authToken := []byte("0x")

	// fib code
	code, err := loadByteCode("../jamtestvectors/workpackages/fib-refine-fixed.pvm")
	if err != nil {
		panic(0)
	}

	// TODO: need to use TestNodePOAAccumulatePVM logic to put the code into system
	codeHash := common.Blake2Hash(code)

	workPackage = types.WorkPackage{
		Authorization: authToken,
		AuthCodeHost:  47,
		Authorizer:    types.Authorizer{},
		RefineContext: refine_context,
		WorkItems: []types.WorkItem{
			{
				Service:          47,
				CodeHash:         codeHash,
				Payload:          []byte("0x00000010"),
				GasLimit:         10000000,
				ImportedSegments: importedSegments,
				ExportCount:      1,
			},
		},
	}
	return workPackage, segments
}

var targetFIB = 10

func TestAvailabilityReconstruction(t *testing.T) {
	// Set up the network
	genesisConfig, peers, peerList, validatorSecrets, err := SetupQuicNetwork()
	if err != nil {
		t.Fatalf("Error setting up nodes: %v\n", err)
	}
	nodes := make([]*Node, numNodes)
	for i := 0; i < numNodes; i++ {
		node, err := newNode(uint32(i), validatorSecrets[i], &genesisConfig, peers, peerList, DAFlag)
		if err != nil {
			t.Fatalf("Failed to create node %d: %v\n", i, err)
		}
		nodes[i] = node
	}

	// Simulate a work package and segments
	var originalAS *types.AvailabilitySpecifier

	senderIndex := 0
	senderNode := nodes[senderIndex]

	for fibN := 1; fibN <= targetFIB; fibN++ {
		workPackage, segments := testWorkpackage(fibN)
		if fibN > 1 {
			importedSegments := make([]types.ImportSegment, 0)
			importedSegments = append(importedSegments, types.ImportSegment{
				TreeRoot: originalAS.ExportedSegmentRoot,
				Index:    uint16(0),
			})
			workPackage.WorkItems[0].ImportedSegments = importedSegments
		}

		// Generate the AvailabilitySpecifier
		packageHash := workPackage.Hash()
		originalAS = senderNode.NewAvailabilitySpecifier(packageHash, workPackage, segments)

		encodeCheck := senderNode.VerifyWorkPackage(workPackage)
		if !encodeCheck {
			t.Fatalf("VerifyWorkPackage FAILED! \n")
		}

		// for _, n := range nodes {
		for idx, n := range nodes {
			if idx != senderIndex {
				continue
			}
			workPackageReconstruction, bClubHash, err := n.FetchWorkPackage(originalAS.ErasureRoot, int(originalAS.BundleLength))
			if err != nil {
				t.Errorf("[N%v] WP Reconstruction failure err:%v\n", n.coreIndex, err)
			}
			fmt.Printf("%v\n", bClubHash)
			// if workPackageReconstruction.Hash() != workPackage.Hash() {
			if !common.CompareBytes(workPackageReconstruction.Bytes(), workPackage.Bytes()) {
				fmt.Printf("workPackageReconstruction.Bytes=%v\n", workPackageReconstruction.Bytes())
				fmt.Printf("workPackage.Bytes=%v\n", workPackage.Bytes())
				t.Fatalf("[N%v] WP Reconstruction Mismatch!\n", n.coreIndex)
			}

			// exportedSegments, pageProofs, treeRoots, sClubHash, err := n.FetchExportedSegments(originalAS.ErasureRoot)
			exportedSegments, pageProofs, treeRoots, _, err := n.FetchExportedSegments(originalAS.ErasureRoot)
			fmt.Printf("len(exportedSegments)=%v, len(treeRoots)=%v\n", len(exportedSegments), len(treeRoots))
			if err != nil {
				t.Fatalf("[N%v] Exported Segment Reconstruction failure err:%v\n", n.coreIndex, err)
			}

			for i, proof := range pageProofs {
				decodedData, _ := types.Decode(proof, reflect.TypeOf([][]uint8{}))
				decodedSegments := decodedData.([][]byte)
				t.Logf("Page %d PageProof: %x\n", i, decodedSegments)
				// Compare decoded segments with original
				start := i * 64
				end := start + 64
				if end > len(segments) {
					end = len(segments)
				}
				var position int
				for _, segment := range segments {
					position = trie.FindPositions(decodedSegments, segment)
					if position != -1 {
						break
					}
				}
				if position == -1 {
					t.Fatalf("Segment not found in the decoded segments")
				}

				expectedSegments := segments[start:end]
				expecteddecodedSegments := decodedSegments[position:]
				for j := range expecteddecodedSegments {
					if !common.CompareBytes(expecteddecodedSegments[j], expectedSegments[j]) {
						t.Errorf("Segment mismatch: expected %x, got %x", expectedSegments[j], decodedSegments[j])
					}
				}
				result, err := trie.VerifyPageProof(decodedSegments, i)
				if err != nil {
					t.Errorf("Page %d PageProof Verification failed: %v\n", i, err)
				} else if result {
					t.Logf("Page %d PageProof Verified\n", i)
				} else {
					t.Errorf("Page %d PageProof Verification failed: %v\n", i, err)
				}
			}

			//TODO: in it consider success when we get here??
			fmt.Printf("✅✅ [N%v] reconstructing FIB=%v\n", n.id, fibN)

			// isValid, err := senderNode.IsValidAvailabilitySpecifier(bClubHash, int(originalAS.BundleLength), sClubHash, originalAS)
			// if err != nil {
			// 	t.Fatalf("[N%v] Error validating AvailabilitySpecifier: %v", n.coreIndex, err)
			// }
			// if isValid == false {
			// 	t.Fatalf("[N%v] AvailabilitySpecifier is not valid: %v", n.coreIndex, err)
			// }
		}

		// TODO: similar operation between segments and segmentReconstruction
	}
}
