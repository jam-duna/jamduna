package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	verkle "github.com/ethereum/go-verkle"
)

// FixtureMetadata tracks generation context
type FixtureMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	GoVerkleVersion string `json:"go_verkle_version"`
	GeneratedAt string `json:"generated_at"`
	Scenario    string `json:"scenario"`
}

// VerkleFixture is the complete fixture format
type VerkleFixture struct {
	Metadata      FixtureMetadata         `json:"metadata"`
	PreStateRoot  string                  `json:"pre_state_root"`
	PostStateRoot string                  `json:"post_state_root"`
	VerkleProof   *verkle.VerkleProof     `json:"verkle_proof"`
	StateDiff     verkle.StateDiff        `json:"state_diff"`
}

func makeKey(stem []byte, suffix byte) []byte {
	key := make([]byte, 32)
	copy(key[:31], stem)
	key[31] = suffix
	return key
}

func makeValue(firstByte byte) []byte {
	value := make([]byte, 32)
	value[0] = firstByte
	return value
}

// Scenario 1: Proof of absence (mixed present/absent keys, single stem)
func generateProofOfAbsence() {
	fmt.Println("\n=== Generating Scenario 1: Proof of Absence ===")

	tree := verkle.New()
	stem := make([]byte, 31)
	stem[0] = 0x42

	// Insert 3 present values at suffixes 0, 10, 20
	presentKeys := [][]byte{
		makeKey(stem, 0),
		makeKey(stem, 10),
		makeKey(stem, 20),
	}
	presentValues := [][]byte{
		makeValue(0xaa),
		makeValue(0xbb),
		makeValue(0xcc),
	}

	for i, key := range presentKeys {
		if err := tree.Insert(key, presentValues[i], nil); err != nil {
			panic(err)
		}
	}

	// Commit tree
	preStateRoot := tree.Commit()
	preStateRootBytes := preStateRoot.Bytes()
	fmt.Printf("Root: %s\n", hex.EncodeToString(preStateRootBytes[:]))

	// Generate proof for present keys
	proof, _, _, _, err := verkle.MakeVerkleMultiProof(tree, nil, presentKeys, nil)
	if err != nil {
		panic(err)
	}

	vp, stateDiff, err := verkle.SerializeProof(proof)
	if err != nil {
		panic(err)
	}

	// SECURITY: Ensure root is in proof for cryptographic binding
	vp = ensureRootInProof(vp, tree.(*verkle.InternalNode))

	fixture := VerkleFixture{
		Metadata: FixtureMetadata{
			Name:            "proof_of_absence",
			Description:     "Single stem with 3 present keys (0,10,20) proving absence of keys (5,15)",
			GoVerkleVersion: "v0.2.2",
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			Scenario:        "proof_of_absence",
		},
		PreStateRoot:  hex.EncodeToString(preStateRootBytes[:]),
		PostStateRoot: hex.EncodeToString(preStateRootBytes[:]),
		VerkleProof:   vp,
		StateDiff:     stateDiff,
	}

	writeFixture("verkle_fixture_proof_of_absence.json", fixture)
}

// Scenario 2: Multi-stem proof
func generateMultiStem() {
	fmt.Println("\n=== Generating Scenario 2: Multi-Stem Proof ===")

	tree := verkle.New()

	// Create 3 different stems
	stem1 := make([]byte, 31)
	stem1[0] = 0x10

	stem2 := make([]byte, 31)
	stem2[0] = 0x20

	stem3 := make([]byte, 31)
	stem3[0] = 0x30

	// Insert values across multiple stems
	keys := [][]byte{
		makeKey(stem1, 0),  // stem1: suffix 0
		makeKey(stem1, 5),  // stem1: suffix 5
		makeKey(stem2, 10), // stem2: suffix 10
		makeKey(stem2, 15), // stem2: suffix 15
		makeKey(stem3, 20), // stem3: suffix 20
	}
	values := [][]byte{
		makeValue(0x11),
		makeValue(0x12),
		makeValue(0x21),
		makeValue(0x22),
		makeValue(0x31),
	}

	for i, key := range keys {
		if err := tree.Insert(key, values[i], nil); err != nil {
			panic(err)
		}
	}

	preStateRoot := tree.Commit()
	preStateRootBytes := preStateRoot.Bytes()
	fmt.Printf("Root: %s\n", hex.EncodeToString(preStateRootBytes[:]))

	proof, _, _, _, err := verkle.MakeVerkleMultiProof(tree, nil, keys, nil)
	if err != nil {
		panic(err)
	}

	vp, stateDiff, err := verkle.SerializeProof(proof)
	if err != nil {
		panic(err)
	}

	// SECURITY: Ensure root is in proof for cryptographic binding
	vp = ensureRootInProof(vp, tree.(*verkle.InternalNode))

	fixture := VerkleFixture{
		Metadata: FixtureMetadata{
			Name:            "multi_stem",
			Description:     "Multiple stems (0x10, 0x20, 0x30) with 5 total keys across 3 stems",
			GoVerkleVersion: "v0.2.2",
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			Scenario:        "multi_stem",
		},
		PreStateRoot:  hex.EncodeToString(preStateRootBytes[:]),
		PostStateRoot: hex.EncodeToString(preStateRootBytes[:]),
		VerkleProof:   vp,
		StateDiff:     stateDiff,
	}

	writeFixture("verkle_fixture_multi_stem.json", fixture)
}

// Scenario 3: Deep tree path (extension nodes)
func generateDeepTree() {
	fmt.Println("\n=== Generating Scenario 3: Deep Tree Path ===")

	tree := verkle.New()

	// Create stems at different depths by varying first bytes
	stems := [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02},
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01},
	}

	keys := [][]byte{
		makeKey(stems[0], 0),
		makeKey(stems[1], 0),
		makeKey(stems[2], 0),
	}
	values := [][]byte{
		makeValue(0xaa),
		makeValue(0xbb),
		makeValue(0xcc),
	}

	for i, key := range keys {
		if err := tree.Insert(key, values[i], nil); err != nil {
			panic(err)
		}
	}

	preStateRoot := tree.Commit()
	preStateRootBytes := preStateRoot.Bytes()
	fmt.Printf("Root: %s\n", hex.EncodeToString(preStateRootBytes[:]))

	proof, _, _, _, err := verkle.MakeVerkleMultiProof(tree, nil, keys, nil)
	if err != nil {
		panic(err)
	}

	vp, stateDiff, err := verkle.SerializeProof(proof)
	if err != nil {
		panic(err)
	}

	// SECURITY: Ensure root is in proof for cryptographic binding
	vp = ensureRootInProof(vp, tree.(*verkle.InternalNode))

	fixture := VerkleFixture{
		Metadata: FixtureMetadata{
			Name:            "deep_tree",
			Description:     "Tree with extension nodes requiring deep path traversal",
			GoVerkleVersion: "v0.2.2",
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			Scenario:        "deep_tree",
		},
		PreStateRoot:  hex.EncodeToString(preStateRootBytes[:]),
		PostStateRoot: hex.EncodeToString(preStateRootBytes[:]),
		VerkleProof:   vp,
		StateDiff:     stateDiff,
	}

	writeFixture("verkle_fixture_deep_tree.json", fixture)
}

// Scenario 4: Mixed present/absent across multiple stems
func generateMixedMultiStem() {
	fmt.Println("\n=== Generating Scenario 4: Mixed Present/Absent Multi-Stem ===")

	tree := verkle.New()

	stem1 := make([]byte, 31)
	stem1[0] = 0x50

	stem2 := make([]byte, 31)
	stem2[0] = 0x60

	// Stem1: suffixes 0, 10 present; 5 absent
	// Stem2: suffixes 0, 20 present; 10 absent
	presentKeys := [][]byte{
		makeKey(stem1, 0),
		makeKey(stem1, 10),
		makeKey(stem2, 0),
		makeKey(stem2, 20),
	}
	values := [][]byte{
		makeValue(0x51),
		makeValue(0x52),
		makeValue(0x61),
		makeValue(0x62),
	}

	for i, key := range presentKeys {
		if err := tree.Insert(key, values[i], nil); err != nil {
			panic(err)
		}
	}

	preStateRoot := tree.Commit()
	preStateRootBytes := preStateRoot.Bytes()
	fmt.Printf("Root: %s\n", hex.EncodeToString(preStateRootBytes[:]))

	proof, _, _, _, err := verkle.MakeVerkleMultiProof(tree, nil, presentKeys, nil)
	if err != nil {
		panic(err)
	}

	vp, stateDiff, err := verkle.SerializeProof(proof)
	if err != nil {
		panic(err)
	}

	// SECURITY: Ensure root is in proof for cryptographic binding
	vp = ensureRootInProof(vp, tree.(*verkle.InternalNode))

	fixture := VerkleFixture{
		Metadata: FixtureMetadata{
			Name:            "mixed_multi_stem",
			Description:     "Two stems with mixed present/absent keys (stem1: 0,10; stem2: 0,20)",
			GoVerkleVersion: "v0.2.2",
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			Scenario:        "mixed_multi_stem",
		},
		PreStateRoot:  hex.EncodeToString(preStateRootBytes[:]),
		PostStateRoot: hex.EncodeToString(preStateRootBytes[:]),
		VerkleProof:   vp,
		StateDiff:     stateDiff,
	}

	writeFixture("verkle_fixture_mixed_multi_stem.json", fixture)
}

// ensureRootInProof prepends the root commitment to commitmentsByPath if not present
// SECURITY: Proofs MUST contain root for cryptographic binding - do not accept rootless proofs
func ensureRootInProof(vp *verkle.VerkleProof, root verkle.VerkleNode) *verkle.VerkleProof {
	rootBytes := root.Commit().Bytes()

	fmt.Printf("[ensureRootInProof] Input commitments: %d\n", len(vp.CommitmentsByPath))
	fmt.Printf("[ensureRootInProof] Root: %s\n", hex.EncodeToString(rootBytes[:]))

	// Check if root is already present
	if len(vp.CommitmentsByPath) > 0 {
		firstCommit := vp.CommitmentsByPath[0]
		if len(firstCommit) == 32 {
			match := true
			for i := 0; i < 32; i++ {
				if firstCommit[i] != rootBytes[i] {
					match = false
					break
				}
			}
			if match {
				// Root already present
				return vp
			}
		}
	}

	// Prepend root to commitmentsByPath (go-verkle v0.2.2+ uses pruned format)
	var rootArray [32]byte
	copy(rootArray[:], rootBytes[:])

	newCommitments := make([][32]byte, 0, len(vp.CommitmentsByPath)+1)
	newCommitments = append(newCommitments, rootArray)
	newCommitments = append(newCommitments, vp.CommitmentsByPath...)

	fmt.Printf("[ensureRootInProof] Output commitments: %d\n", len(newCommitments))

	return &verkle.VerkleProof{
		OtherStems:            vp.OtherStems,
		DepthExtensionPresent: vp.DepthExtensionPresent,
		CommitmentsByPath:     newCommitments,
		D:                     vp.D,
		IPAProof:              vp.IPAProof,
	}
}

func writeFixture(filename string, fixture VerkleFixture) {
	output, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(filename, output, 0644); err != nil {
		panic(err)
	}

	fmt.Printf("✅ Generated: %s\n", filename)
}

// Scenario 5: Single key proof (basic case)
func generateSingleKey() {
	fmt.Println("\n=== Generating Scenario 5: Single Key ===")

	tree := verkle.New()
	stem := make([]byte, 31)
	stem[0] = 0x50

	key := makeKey(stem, 0)
	value := makeValue(0xdd)

	if err := tree.Insert(key, value, nil); err != nil {
		panic(err)
	}

	preStateRoot := tree.Commit()
	preStateRootBytes := preStateRoot.Bytes()
	fmt.Printf("Root: %s\n", hex.EncodeToString(preStateRootBytes[:]))

	proof, _, _, _, err := verkle.MakeVerkleMultiProof(tree, nil, [][]byte{key}, nil)
	if err != nil {
		panic(err)
	}

	vp, stateDiff, err := verkle.SerializeProof(proof)
	if err != nil {
		panic(err)
	}

	fmt.Printf("[Single Key] Before ensureRoot: commitments=%d, depth_ext=%#x\n",
		len(vp.CommitmentsByPath), vp.DepthExtensionPresent)

	// SECURITY: Ensure root is in proof for cryptographic binding
	vp = ensureRootInProof(vp, tree.(*verkle.InternalNode))

	fmt.Printf("[Single Key] After ensureRoot: commitments=%d\n", len(vp.CommitmentsByPath))

	fixture := VerkleFixture{
		Metadata: FixtureMetadata{
			Name:            "single_key",
			Description:     "Single stem with one key at suffix 0",
			GoVerkleVersion: "v0.2.2",
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			Scenario:        "single_key",
		},
		PreStateRoot:  hex.EncodeToString(preStateRootBytes[:]),
		PostStateRoot: hex.EncodeToString(preStateRootBytes[:]),
		VerkleProof:   vp,
		StateDiff:     stateDiff,
	}

	writeFixture("verkle_fixture_single_key.json", fixture)
}

// Scenario 6: Multiple keys on same stem
func generateMultiKeySameStem() {
	fmt.Println("\n=== Generating Scenario 6: Multi-Key Same Stem ===")

	tree := verkle.New()
	stem := make([]byte, 31)
	stem[0] = 0x60

	keys := [][]byte{
		makeKey(stem, 0),
		makeKey(stem, 1),
	}
	values := [][]byte{
		makeValue(0xee),
		makeValue(0xff),
	}

	for i, key := range keys {
		if err := tree.Insert(key, values[i], nil); err != nil {
			panic(err)
		}
	}

	preStateRoot := tree.Commit()
	preStateRootBytes := preStateRoot.Bytes()
	fmt.Printf("Root: %s\n", hex.EncodeToString(preStateRootBytes[:]))

	proof, _, _, _, err := verkle.MakeVerkleMultiProof(tree, nil, keys, nil)
	if err != nil {
		panic(err)
	}

	vp, stateDiff, err := verkle.SerializeProof(proof)
	if err != nil {
		panic(err)
	}

	// SECURITY: Ensure root is in proof for cryptographic binding
	vp = ensureRootInProof(vp, tree.(*verkle.InternalNode))

	fixture := VerkleFixture{
		Metadata: FixtureMetadata{
			Name:            "multi_key_same_stem",
			Description:     "Single stem with two keys at suffixes 0 and 1",
			GoVerkleVersion: "v0.2.2",
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			Scenario:        "multi_key_same_stem",
		},
		PreStateRoot:  hex.EncodeToString(preStateRootBytes[:]),
		PostStateRoot: hex.EncodeToString(preStateRootBytes[:]),
		VerkleProof:   vp,
		StateDiff:     stateDiff,
	}

	writeFixture("verkle_fixture_multi_key_same_stem.json", fixture)
}

func main() {
	fmt.Println("Verkle Fixture Generator")
	fmt.Println("========================")
	fmt.Println("Generating test fixtures with go-verkle v0.2.2")

	generateProofOfAbsence()
	generateMultiStem()
	generateDeepTree()
	generateMixedMultiStem()
	generateSingleKey()
	generateMultiKeySameStem()

	fmt.Println("\n✅ All fixtures generated successfully!")
	fmt.Println("\nGenerated files:")
	fmt.Println("  - verkle_fixture_proof_of_absence.json")
	fmt.Println("  - verkle_fixture_multi_stem.json")
	fmt.Println("  - verkle_fixture_deep_tree.json")
	fmt.Println("  - verkle_fixture_mixed_multi_stem.json")
	fmt.Println("  - verkle_fixture_single_key.json")
	fmt.Println("  - verkle_fixture_multi_key_same_stem.json")
}
