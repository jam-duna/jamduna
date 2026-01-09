package verkle

import (
	"testing"

	verkle "github.com/ethereum/go-verkle"
)

// TestVerkleProofVerifyWithStateDiff tests proof generation and verification
// using the combined proof+statediff format (the correct way).
func TestVerkleProofVerifyWithStateDiff(t *testing.T) {
	// Create a simple tree with some values
	tree := verkle.New()

	// Insert some test data
	key1 := make([]byte, 32)
	key1[0] = 1 // Different stem than key2
	val1 := make([]byte, 32)
	val1[0] = 0xAA

	key2 := make([]byte, 32)
	key2[0] = 2 // Different stem than key1
	val2 := make([]byte, 32)
	val2[0] = 0xBB

	if err := tree.Insert(key1, val1, nil); err != nil {
		t.Fatalf("Insert key1 failed: %v", err)
	}
	if err := tree.Insert(key2, val2, nil); err != nil {
		t.Fatalf("Insert key2 failed: %v", err)
	}

	// Commit the tree
	tree.Commit()

	// Get the root as bytes
	rootBytes := tree.Commit().Bytes()
	t.Logf("Tree root: %x", rootBytes[:])

	// Generate proof for key1 using go-verkle directly
	keys := [][]byte{key1}
	proof, _, _, _, err := verkle.MakeVerkleMultiProof(tree, tree, keys, nil)
	if err != nil {
		t.Fatalf("MakeVerkleMultiProof failed: %v", err)
	}

	// Serialize to VerkleProof + StateDiff
	vp, statediff, err := verkle.SerializeProof(proof)
	if err != nil {
		t.Fatalf("SerializeProof failed: %v", err)
	}

	// Marshal to combined compact format
	proofBytes, err := marshalCompactProofWithStateDiff(vp, statediff)
	if err != nil {
		t.Fatalf("marshalCompactProofWithStateDiff failed: %v", err)
	}

	t.Logf("Proof bytes length: %d", len(proofBytes))

	// Now verify the proof using our VerifyVerkleProofWithStateDiff function
	ok, err := VerifyVerkleProofWithStateDiff(proofBytes, rootBytes[:], rootBytes[:])
	if err != nil {
		t.Fatalf("VerifyVerkleProofWithStateDiff failed: %v", err)
	}
	if !ok {
		t.Fatalf("VerifyVerkleProofWithStateDiff returned false")
	}

	t.Log("Proof verified successfully!")
}

// TestVerkleProofVerifyTransitionWithStateDiff tests a state transition proof
// using the combined proof+statediff format.
func TestVerkleProofVerifyTransitionWithStateDiff(t *testing.T) {
	// Create a pre-state tree
	preTree := verkle.New()

	// Insert initial value
	key1 := make([]byte, 32)
	key1[0] = 1
	preVal := make([]byte, 32)
	preVal[0] = 0xAA

	if err := preTree.Insert(key1, preVal, nil); err != nil {
		t.Fatalf("Insert key1 failed: %v", err)
	}
	preTree.Commit()
	preRootBytes := preTree.Commit().Bytes()

	// Create post-state tree by copying and modifying
	postTree := preTree.Copy()
	postVal := make([]byte, 32)
	postVal[0] = 0xBB

	if err := postTree.Insert(key1, postVal, nil); err != nil {
		t.Fatalf("Insert modified key1 failed: %v", err)
	}
	postTree.Commit()
	postRootBytes := postTree.Commit().Bytes()

	t.Logf("Pre-state root:  %x", preRootBytes[:])
	t.Logf("Post-state root: %x", postRootBytes[:])

	// Generate transition proof using go-verkle directly
	keys := [][]byte{key1}
	proof, _, _, _, err := verkle.MakeVerkleMultiProof(preTree, postTree, keys, nil)
	if err != nil {
		t.Fatalf("MakeVerkleMultiProof failed: %v", err)
	}

	// Serialize to VerkleProof + StateDiff
	vp, statediff, err := verkle.SerializeProof(proof)
	if err != nil {
		t.Fatalf("SerializeProof failed: %v", err)
	}

	// Marshal to combined compact format
	proofBytes, err := marshalCompactProofWithStateDiff(vp, statediff)
	if err != nil {
		t.Fatalf("marshalCompactProofWithStateDiff failed: %v", err)
	}

	t.Logf("Proof bytes length: %d", len(proofBytes))

	// Now verify the transition proof
	ok, err := VerifyVerkleProofWithStateDiff(proofBytes, preRootBytes[:], postRootBytes[:])
	if err != nil {
		t.Fatalf("VerifyVerkleProofWithStateDiff failed: %v", err)
	}
	if !ok {
		t.Fatalf("VerifyVerkleProofWithStateDiff returned false")
	}

	t.Log("Transition proof verified successfully!")
}

// TestStateDiffRoundTrip tests marshalStateDiff/unmarshalStateDiff
func TestStateDiffRoundTrip(t *testing.T) {
	// Create a simple statediff
	currentVal := [32]byte{0xAA}
	newVal := [32]byte{0xBB}

	original := verkle.StateDiff{
		{
			Stem: [31]byte{1, 2, 3},
			SuffixDiffs: []verkle.SuffixStateDiff{
				{
					Suffix:       0,
					CurrentValue: &currentVal,
					NewValue:     &newVal,
				},
				{
					Suffix:       1,
					CurrentValue: nil,
					NewValue:     &newVal,
				},
			},
		},
		{
			Stem: [31]byte{4, 5, 6},
			SuffixDiffs: []verkle.SuffixStateDiff{
				{
					Suffix:       255,
					CurrentValue: &currentVal,
					NewValue:     nil,
				},
			},
		},
	}

	// Marshal
	serialized, err := marshalStateDiff(original)
	if err != nil {
		t.Fatalf("marshalStateDiff failed: %v", err)
	}

	t.Logf("Serialized statediff length: %d bytes", len(serialized))

	// Unmarshal
	roundTrip, err := unmarshalStateDiff(serialized)
	if err != nil {
		t.Fatalf("unmarshalStateDiff failed: %v", err)
	}

	// Compare
	if len(original) != len(roundTrip) {
		t.Fatalf("stem count mismatch: %d vs %d", len(original), len(roundTrip))
	}

	for i := range original {
		if original[i].Stem != roundTrip[i].Stem {
			t.Fatalf("stem %d mismatch", i)
		}
		if len(original[i].SuffixDiffs) != len(roundTrip[i].SuffixDiffs) {
			t.Fatalf("suffix count mismatch for stem %d", i)
		}
		for j := range original[i].SuffixDiffs {
			origSd := original[i].SuffixDiffs[j]
			rtSd := roundTrip[i].SuffixDiffs[j]
			if origSd.Suffix != rtSd.Suffix {
				t.Fatalf("suffix mismatch at %d:%d", i, j)
			}
			if (origSd.CurrentValue == nil) != (rtSd.CurrentValue == nil) {
				t.Fatalf("current value nil mismatch at %d:%d", i, j)
			}
			if origSd.CurrentValue != nil && *origSd.CurrentValue != *rtSd.CurrentValue {
				t.Fatalf("current value mismatch at %d:%d", i, j)
			}
			if (origSd.NewValue == nil) != (rtSd.NewValue == nil) {
				t.Fatalf("new value nil mismatch at %d:%d", i, j)
			}
			if origSd.NewValue != nil && *origSd.NewValue != *rtSd.NewValue {
				t.Fatalf("new value mismatch at %d:%d", i, j)
			}
		}
	}

	t.Log("StateDiff round-trip successful!")
}
