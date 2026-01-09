package ubt

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVector represents a test vector from Rust
type TestVector struct {
	Name         string      `json:"name"`
	Profile      string      `json:"profile"`
	Hash         string      `json:"hash"`
	UBTVersion   string      `json:"ubt_version"`
	Operations   []VectorOp  `json:"operations"`
	ExpectedRoot string      `json:"expected_root"`
}

// VectorOp represents an insert or delete operation.
type VectorOp struct {
	Type  string `json:"type"`
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// TestBlake3EIPVectors validates Go implementation against Rust-generated Blake3 EIP vectors
func TestBlake3EIPVectors(t *testing.T) {
	// Load vectors from JSON file
	vectorsPath := "../test_vectors_blake3_eip.json"
	data, err := os.ReadFile(vectorsPath)
	require.NoError(t, err, "Failed to read test vectors file")

	var vectors []TestVector
	err = json.Unmarshal(data, &vectors)
	require.NoError(t, err, "Failed to parse test vectors JSON")

	t.Logf("Loaded %d test vectors from %s", len(vectors), vectorsPath)

	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			// Create tree with EIP profile
			tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})

			// Execute operations
			for i, op := range vector.Operations {
				keyBytes, err := hex.DecodeString(op.Key)
				require.NoError(t, err, "Failed to decode key hex for operation %d", i)
				require.Equal(t, 32, len(keyBytes), "Key must be 32 bytes for operation %d", i)

				var keyArr [32]byte
				copy(keyArr[:], keyBytes)
				key := TreeKeyFromBytes(keyArr)

				switch op.Type {
				case "insert":
					valueBytes, err := hex.DecodeString(op.Value)
					require.NoError(t, err, "Failed to decode value hex for operation %d", i)
					require.Equal(t, 32, len(valueBytes), "Value must be 32 bytes for operation %d", i)

					value := [32]byte{}
					copy(value[:], valueBytes)

					tree.Insert(key, value)

				case "delete":
					tree.Delete(&key)

				default:
					t.Fatalf("Unknown operation type: %s", op.Type)
				}
			}

			// Compute root hash
			actualRoot := tree.RootHash()
			actualRootHex := hex.EncodeToString(actualRoot[:])

			// Compare with expected root
			assert.Equal(t, vector.ExpectedRoot, actualRootHex,
				"Root hash mismatch for vector '%s'", vector.Name)

			t.Logf("[PASS] %s: root = %s", vector.Name, actualRootHex)
		})
	}
}

// TestBlake3JAMVectors validates Go implementation against Rust-generated Blake3 JAM vectors.
func TestBlake3JAMVectors(t *testing.T) {
	vectorsPath := "../test_vectors_blake3_jam.json"
	data, err := os.ReadFile(vectorsPath)
	require.NoError(t, err, "Failed to read test vectors file")

	var vectors []TestVector
	err = json.Unmarshal(data, &vectors)
	require.NoError(t, err, "Failed to parse test vectors JSON")

	t.Logf("Loaded %d test vectors from %s", len(vectors), vectorsPath)

	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			require.Equal(t, "jam", vector.Profile)
			require.Equal(t, "blake3", vector.Hash)

			tree := NewUnifiedBinaryTree(Config{Profile: JAMProfile})

			for i, op := range vector.Operations {
				keyBytes, err := hex.DecodeString(op.Key)
				require.NoError(t, err, "Failed to decode key hex for operation %d", i)
				require.Equal(t, 32, len(keyBytes), "Key must be 32 bytes for operation %d", i)

				var keyArr [32]byte
				copy(keyArr[:], keyBytes)
				key := TreeKeyFromBytes(keyArr)

				switch op.Type {
				case "insert":
					valueBytes, err := hex.DecodeString(op.Value)
					require.NoError(t, err, "Failed to decode value hex for operation %d", i)
					require.Equal(t, 32, len(valueBytes), "Value must be 32 bytes for operation %d", i)

					value := [32]byte{}
					copy(value[:], valueBytes)

					tree.Insert(key, value)
				case "delete":
					tree.Delete(&key)
				default:
					t.Fatalf("Unknown operation type: %s", op.Type)
				}
			}

			actualRoot := tree.RootHash()
			actualRootHex := hex.EncodeToString(actualRoot[:])

			assert.Equal(t, vector.ExpectedRoot, actualRootHex,
				"Root hash mismatch for vector '%s'", vector.Name)
		})
	}
}

// TestBlake3VectorStructure validates the structure of the test vectors file
func TestBlake3VectorStructure(t *testing.T) {
	vectorsPath := "../test_vectors_blake3_eip.json"
	data, err := os.ReadFile(vectorsPath)
	require.NoError(t, err)

	var vectors []TestVector
	err = json.Unmarshal(data, &vectors)
	require.NoError(t, err)

	require.NotEmpty(t, vectors, "Test vectors file should not be empty")

	for i, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			assert.NotEmpty(t, vector.Name, "Vector %d should have a name", i)
			assert.Equal(t, "eip", vector.Profile, "Vector %d should use EIP profile", i)
			assert.Equal(t, "blake3", vector.Hash, "Vector %d should use Blake3 hash", i)
			assert.NotEmpty(t, vector.UBTVersion, "Vector %d should have UBT version", i)
			assert.NotEmpty(t, vector.ExpectedRoot, "Vector %d should have expected root", i)

			// Validate expected root is valid hex
			_, err := hex.DecodeString(vector.ExpectedRoot)
			assert.NoError(t, err, "Vector %d expected_root should be valid hex", i)

			// Validate all operations have valid keys/values
			for j, op := range vector.Operations {
				assert.NotEmpty(t, op.Type, "Vector %d operation %d should have type", i, j)
				assert.NotEmpty(t, op.Key, "Vector %d operation %d should have key", i, j)

				keyBytes, err := hex.DecodeString(op.Key)
				assert.NoError(t, err, "Vector %d operation %d key should be valid hex", i, j)
				assert.Equal(t, 32, len(keyBytes), "Vector %d operation %d key should be 32 bytes", i, j)

				if op.Type == "insert" {
					assert.NotEmpty(t, op.Value, "Vector %d operation %d insert should have value", i, j)
					valueBytes, err := hex.DecodeString(op.Value)
					assert.NoError(t, err, "Vector %d operation %d value should be valid hex", i, j)
					assert.Equal(t, 32, len(valueBytes), "Vector %d operation %d value should be 32 bytes", i, j)
				}
			}
		})
	}
}

// TestEmptyTreeVector specifically tests the empty tree vector
func TestEmptyTreeVector(t *testing.T) {
	tree := NewUnifiedBinaryTree(Config{Profile: EIPProfile})
	root := tree.RootHash()
	rootHex := hex.EncodeToString(root[:])

	// Empty tree must have ZERO32 root
	expectedEmpty := "0000000000000000000000000000000000000000000000000000000000000000"
	assert.Equal(t, expectedEmpty, rootHex, "Empty tree must have ZERO32 root hash")
}

// TestVectorCoverage ensures we have good coverage of test scenarios
func TestVectorCoverage(t *testing.T) {
	vectorsPath := "../test_vectors_blake3_eip.json"
	data, err := os.ReadFile(vectorsPath)
	require.NoError(t, err)

	var vectors []TestVector
	err = json.Unmarshal(data, &vectors)
	require.NoError(t, err)

	// Check for expected vector types
	foundEmpty := false
	foundSingle := false
	foundMultiple := false
	foundColocated := false
	foundOrderIndep := false

	for _, vector := range vectors {
		switch {
		case len(vector.Operations) == 0:
			foundEmpty = true
		case len(vector.Operations) == 1:
			foundSingle = true
		case len(vector.Operations) >= 2:
			foundMultiple = true
		}

		if contains(vector.Name, "colocated") {
			foundColocated = true
		}
		if contains(vector.Name, "order") || contains(vector.Name, "independence") {
			foundOrderIndep = true
		}
	}

	assert.True(t, foundEmpty, "Should have empty tree test vector")
	assert.True(t, foundSingle, "Should have single insert test vector")
	assert.True(t, foundMultiple, "Should have multiple insert test vectors")
	assert.True(t, foundColocated, "Should have colocated values test vector")
	assert.True(t, foundOrderIndep, "Should have order independence test vector")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
