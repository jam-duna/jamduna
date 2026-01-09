package ubt

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/colorfulnotion/jam/storage"
	"github.com/stretchr/testify/require"
)

type proofVector struct {
	Name         string             `json:"name"`
	Profile      string             `json:"profile"`
	Hash         string             `json:"hash"`
	UBTVersion   string             `json:"ubt_version"`
	Key          proofVectorKey     `json:"key"`
	Value        *string            `json:"value"`
	ExpectedRoot string             `json:"expected_root"`
	Path         []proofVectorNode  `json:"path"`
}

type proofVectorKey struct {
	Stem     string `json:"stem"`
	Subindex uint8  `json:"subindex"`
}

type proofVectorNode struct {
	Type            string   `json:"type"`
	Stem            string   `json:"stem,omitempty"`
	StemHash        string   `json:"stem_hash,omitempty"`
	DivergenceDepth uint16   `json:"divergence_depth,omitempty"`
	SubtreeSiblings []string `json:"subtree_siblings,omitempty"`
	Direction       string   `json:"direction,omitempty"`
	Sibling         string   `json:"sibling,omitempty"`
}

func TestBlake3ProofVectors(t *testing.T) {
	vectorsPath := "../test_vectors_proof_blake3.json"
	data, err := os.ReadFile(vectorsPath)
	require.NoError(t, err, "Failed to read proof vectors file")

	var vectors []proofVector
	require.NoError(t, json.Unmarshal(data, &vectors), "Failed to parse proof vectors JSON")
	require.NotEmpty(t, vectors, "Proof vectors file should not be empty")

	hasher := NewBlake3Hasher(EIPProfile)

	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			require.Equal(t, "eip", vector.Profile)
			require.Equal(t, "blake3", vector.Hash)
			require.NotEmpty(t, vector.UBTVersion)

			key := decodeTreeKey(t, vector.Key)
			expectedRoot := decodeBytes32(t, vector.ExpectedRoot)

			path := make([]storage.ProofNode, 0, len(vector.Path))
			for i, node := range vector.Path {
				switch node.Type {
				case "stem":
					stem := decodeStem(t, node.Stem)
					siblings := make([][32]byte, len(node.SubtreeSiblings))
					for j, sibHex := range node.SubtreeSiblings {
						siblings[j] = decodeBytes32(t, sibHex)
					}
					path = append(path, &storage.StemProofNode{
						Stem:            stem,
						SubtreeSiblings: siblings,
					})
				case "extension":
					stem := decodeStem(t, node.Stem)
					stemHash := decodeBytes32(t, node.StemHash)
					path = append(path, &storage.ExtensionProofNode{
						Stem:            stem,
						StemHash:        stemHash,
						DivergenceDepth: node.DivergenceDepth,
					})
				case "internal":
					sibling := decodeBytes32(t, node.Sibling)
					var direction storage.Direction
					switch node.Direction {
					case "left":
						direction = storage.Left
					case "right":
						direction = storage.Right
					default:
						t.Fatalf("Invalid direction %q at node %d", node.Direction, i)
					}
					path = append(path, &storage.InternalProofNode{
						Sibling:   sibling,
						Direction: direction,
					})
				default:
					t.Fatalf("Unknown node type %q at index %d", node.Type, i)
				}
			}

			var valuePtr *[32]byte
			if vector.Value != nil && *vector.Value != "" {
				val := decodeBytes32(t, *vector.Value)
				valuePtr = &val
			}

			proof := storage.Proof{
				Key:   key,
				Value: valuePtr,
				Path:  path,
			}

			require.NoError(t, proof.Verify(hasher, expectedRoot))
		})
	}
}

func decodeTreeKey(t *testing.T, key proofVectorKey) storage.TreeKey {
	stem := decodeStem(t, key.Stem)
	return storage.TreeKey{
		Stem:     stem,
		Subindex: key.Subindex,
	}
}

func decodeStem(t *testing.T, hexStem string) storage.Stem {
	stemBytes, err := hex.DecodeString(hexStem)
	require.NoError(t, err, "Failed to decode stem hex")
	require.Len(t, stemBytes, 31, "Stem must be 31 bytes")
	var stem storage.Stem
	copy(stem[:], stemBytes)
	return stem
}

func decodeBytes32(t *testing.T, hexValue string) [32]byte {
	valueBytes, err := hex.DecodeString(hexValue)
	require.NoError(t, err, "Failed to decode bytes32 hex")
	require.Len(t, valueBytes, 32, "Expected 32-byte value")
	var out [32]byte
	copy(out[:], valueBytes)
	return out
}
