package merkle

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestMerkleTreeBasics(t *testing.T) {
	tree := NewMerkleTree()

	// Test initial state
	if tree.GetSize() != 0 {
		t.Fatalf("expected size 0, got %d", tree.GetSize())
	}

	// Test append
	commitment1 := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
	err := tree.Append(commitment1)
	if err != nil {
		t.Fatalf("failed to append: %v", err)
	}

	if tree.GetSize() != 1 {
		t.Fatalf("expected size 1, got %d", tree.GetSize())
	}

	// Test leaf retrieval
	leaf, err := tree.GetLeaf(0)
	if err != nil {
		t.Fatalf("failed to get leaf: %v", err)
	}
	if leaf != commitment1 {
		t.Fatalf("leaf mismatch: expected %v, got %v", commitment1, leaf)
	}

	// Test root changes
	root1 := tree.GetRoot()
	commitment2 := common.HexToHash("0xfedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321")
	err = tree.Append(commitment2)
	if err != nil {
		t.Fatalf("failed to append second commitment: %v", err)
	}

	root2 := tree.GetRoot()
	if root1 == root2 {
		t.Fatal("root should change after append")
	}
}

func TestMerkleWitness(t *testing.T) {
	tree := NewMerkleTree()

	// Add some commitments
	commitments := []common.Hash{
		common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
		common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222"),
		common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333"),
		common.HexToHash("0x4444444444444444444444444444444444444444444444444444444444444444"),
	}

	err := tree.AppendBatch(commitments)
	if err != nil {
		t.Fatalf("failed to append batch: %v", err)
	}

	// Generate witness for first commitment
	witness, err := tree.GenerateWitness(0)
	if err != nil {
		t.Fatalf("failed to generate witness: %v", err)
	}

	if witness.Position != 0 {
		t.Fatalf("expected position 0, got %d", witness.Position)
	}

	if len(witness.Path) != MerkleTreeDepth {
		t.Fatalf("expected path length %d, got %d", MerkleTreeDepth, len(witness.Path))
	}

	// Verify witness
	root := tree.GetRoot()
	if !tree.VerifyWitness(witness, commitments[0], root) {
		t.Fatal("witness verification failed")
	}

	// Test witness for different positions
	for i := 0; i < len(commitments); i++ {
		w, err := tree.GenerateWitness(uint64(i))
		if err != nil {
			t.Fatalf("failed to generate witness for position %d: %v", i, err)
		}

		if !tree.VerifyWitness(w, commitments[i], root) {
			t.Fatalf("witness verification failed for position %d", i)
		}
	}
}

func TestMerkleBatchAppend(t *testing.T) {
	tree := NewMerkleTree()

	// Create a batch of commitments
	batch := make([]common.Hash, 100)
	for i := range batch {
		hash := common.Hash{}
		hash[31] = byte(i)
		batch[i] = hash
	}

	err := tree.AppendBatch(batch)
	if err != nil {
		t.Fatalf("failed to append batch: %v", err)
	}

	if tree.GetSize() != 100 {
		t.Fatalf("expected size 100, got %d", tree.GetSize())
	}

	// Verify all leaves are correct
	for i := 0; i < 100; i++ {
		leaf, err := tree.GetLeaf(uint64(i))
		if err != nil {
			t.Fatalf("failed to get leaf %d: %v", i, err)
		}
		if leaf != batch[i] {
			t.Fatalf("leaf %d mismatch", i)
		}
	}
}

func TestMerkleSnapshot(t *testing.T) {
	tree := NewMerkleTree()

	// Add initial commitments
	commitments := []common.Hash{
		common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
		common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222"),
	}
	tree.AppendBatch(commitments)

	// Create snapshot
	snapshot, err := tree.CreateSnapshot()
	if err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	if snapshot.Size != 2 {
		t.Fatalf("expected snapshot size 2, got %d", snapshot.Size)
	}

	root1 := snapshot.Root

	// Add more commitments
	tree.Append(common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333"))

	// Root should have changed
	if tree.GetRoot() == root1 {
		t.Fatal("root should change after append")
	}
}

func TestWitnessSerializationDeserialization(t *testing.T) {
	witness := MerkleWitness{
		Position: 42,
		Path: []common.Hash{
			common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
			common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222"),
		},
	}

	// Serialize
	data := SerializeWitness(witness)

	// Deserialize
	decoded, err := DeserializeWitness(data)
	if err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if decoded.Position != witness.Position {
		t.Fatalf("position mismatch: expected %d, got %d", witness.Position, decoded.Position)
	}

	if len(decoded.Path) != len(witness.Path) {
		t.Fatalf("path length mismatch: expected %d, got %d", len(witness.Path), len(decoded.Path))
	}

	for i := range witness.Path {
		if decoded.Path[i] != witness.Path[i] {
			t.Fatalf("path[%d] mismatch", i)
		}
	}
}

func TestStorageIsolation(t *testing.T) {
	// Test that two separate trees maintain independent state
	tree1 := NewMerkleTree()
	tree2 := NewMerkleTree()

	commitment1 := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	commitment2 := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")

	// Add different commitments to each tree
	tree1.Append(commitment1)
	tree2.Append(commitment2)

	// Verify roots are different
	if tree1.GetRoot() == tree2.GetRoot() {
		t.Fatal("trees should have different roots")
	}

	// Verify sizes
	if tree1.GetSize() != 1 || tree2.GetSize() != 1 {
		t.Fatal("both trees should have size 1")
	}

	// Verify leaves
	leaf1, _ := tree1.GetLeaf(0)
	leaf2, _ := tree2.GetLeaf(0)

	if leaf1 != commitment1 {
		t.Fatal("tree1 leaf mismatch")
	}
	if leaf2 != commitment2 {
		t.Fatal("tree2 leaf mismatch")
	}

	// Modify tree1 should not affect tree2
	tree1.Append(common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333"))

	if tree2.GetSize() != 1 {
		t.Fatal("tree2 size should still be 1")
	}
}
