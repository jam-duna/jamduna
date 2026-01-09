package ubt

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// IntegrationTestVector represents a comprehensive test case with operations and queries
type IntegrationTestVector struct {
	Name         string              `json:"name"`
	Operations   []IntegrationOp     `json:"operations"`
	ExpectedRoot string              `json:"expected_root"`
	Queries      []IntegrationQuery  `json:"queries,omitempty"`
}

// IntegrationOp represents an insert or delete operation.
type IntegrationOp struct {
	Type     string `json:"type"`     // "insert" or "delete"
	Stem     string `json:"stem"`     // hex string (variable length)
	Subindex uint8  `json:"subindex"` // 0..255
	Value    string `json:"value"`    // hex string (only for insert)
}

// IntegrationQuery represents a get operation with expected result.
type IntegrationQuery struct {
	Stem     string  `json:"stem"`     // hex string (variable length)
	Subindex uint8   `json:"subindex"` // 0..255
	Expected *string `json:"expected"` // hex string or null
}

func TestBlake3IntegrationVectors(t *testing.T) {
	vectorsPath := "../test_vectors_integration_blake3.json"
	data, err := os.ReadFile(vectorsPath)
	require.NoError(t, err, "Failed to read integration test vectors file")

	var vectors []IntegrationTestVector
	err = json.Unmarshal(data, &vectors)
	require.NoError(t, err, "Failed to parse integration test vectors JSON")

	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			// Create tree with EIP profile
			tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

			// Execute operations
			for _, op := range vector.Operations {
				stem, err := parseStemHex(op.Stem)
				require.NoError(t, err, "Failed to decode stem: %s", op.Stem)
				treeKey := TreeKey{Stem: stem, Subindex: op.Subindex}

				switch op.Type {
				case "insert":
					valueBytes, err := hex.DecodeString(op.Value)
					require.NoError(t, err, "Failed to decode value: %s", op.Value)
					require.Len(t, valueBytes, 32, "Value must be 32 bytes")

					var value32 [32]byte
					copy(value32[:], valueBytes)
					tree.Insert(treeKey, value32)

				case "delete":
					tree.Delete(&treeKey)

				default:
					t.Fatalf("Unknown operation: %s", op.Type)
				}
			}

			// Execute queries
			for _, query := range vector.Queries {
				stem, err := parseStemHex(query.Stem)
				require.NoError(t, err, "Failed to decode query stem: %s", query.Stem)
				treeKey := TreeKey{Stem: stem, Subindex: query.Subindex}

				value, found, err := tree.Get(&treeKey)
				require.NoError(t, err, "Get failed")

				if query.Expected == nil {
					assert.False(t, found, "Expected key not found, but it was found")
				} else {
					assert.True(t, found, "Expected key to be found, but it wasn't")
					expectedBytes, err := hex.DecodeString(*query.Expected)
					require.NoError(t, err, "Failed to decode expected value: %s", *query.Expected)
					require.Len(t, expectedBytes, 32, "Expected value must be 32 bytes")

					var expected32 [32]byte
					copy(expected32[:], expectedBytes)
					assert.Equal(t, expected32, value, "Query result mismatch")
				}
			}

			// Verify root hash
			actualRoot := tree.RootHash()
			actualRootHex := hex.EncodeToString(actualRoot[:])
			assert.Equal(t, vector.ExpectedRoot, actualRootHex, "Root hash mismatch for %s", vector.Name)
		})
	}

	t.Logf("Successfully validated %d integration test vectors", len(vectors))
}

func parseStemHex(value string) (Stem, error) {
	var stem Stem
	if value == "" {
		return stem, nil
	}
	bytes, err := hex.DecodeString(value)
	if err != nil {
		return stem, err
	}
	if len(bytes) > len(stem) {
		return stem, fmt.Errorf("stem length %d exceeds %d bytes", len(bytes), len(stem))
	}
	copy(stem[:], bytes)
	return stem, nil
}
