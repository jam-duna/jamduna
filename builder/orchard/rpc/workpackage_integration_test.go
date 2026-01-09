package rpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/builder/orchard/ffi"
	orchardshielded "github.com/colorfulnotion/jam/builder/orchard/shielded"
	orchardstate "github.com/colorfulnotion/jam/builder/orchard/state"
	orchardtransparent "github.com/colorfulnotion/jam/builder/orchard/transparent"
	orchardwitness "github.com/colorfulnotion/jam/builder/orchard/witness"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

type fakeStateProvider struct {
	refineContext types.RefineContext
	submitted     []*types.WorkPackageBundle
}

func (f *fakeStateProvider) GetStateDB() *statedb.StateDB {
	return nil
}

func (f *fakeStateProvider) GetRefineContext() (types.RefineContext, error) {
	return f.refineContext, nil
}

func (f *fakeStateProvider) SubmitBundleSameCore(_ *types.WorkPackageBundle) error {
	return nil
}

func (f *fakeStateProvider) SubmitAndWaitForWorkPackageBundle(_ context.Context, bundle *types.WorkPackageBundle) (common.Hash, uint32, error) {
	f.submitted = append(f.submitted, bundle)
	return common.Hash{}, 0, nil
}

func (f *fakeStateProvider) GetWorkReport(_ common.Hash) (*types.WorkReport, error) {
	return nil, nil
}

type mapCommitmentDecoder struct {
	commitments map[string][][32]byte
}

type workPackageJSON struct {
	PreStateWitnessExtrinsic  string                `json:"pre_state_witness_extrinsic"`
	PostStateWitnessExtrinsic string                `json:"post_state_witness_extrinsic"`
	BundleProofExtrinsic      string                `json:"bundle_proof_extrinsic"`
	PreState                  OrchardStateRoots     `json:"pre_state"`
	PostState                 OrchardStateRoots     `json:"post_state"`
	SpentCommitmentProofs     []SpentCommitmentProof `json:"spent_commitment_proofs"`
}

func (m mapCommitmentDecoder) DecodeBundle(bundle []byte) (*orchardwitness.DecodedBundle, error) {
	return &orchardwitness.DecodedBundle{
		Commitments: m.commitments[string(bundle)],
	}, nil
}

func (m mapCommitmentDecoder) DecodeBundleV6(orchardBundle []byte, _ []byte) (*orchardwitness.DecodedBundleV6, error) {
	return &orchardwitness.DecodedBundleV6{
		Commitments: m.commitments[string(orchardBundle)],
	}, nil
}

func loadWorkPackageJSON(t *testing.T, path string) workPackageJSON {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read work package: %v", err)
	}

	var payload workPackageJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("failed to decode work package JSON: %v", err)
	}
	return payload
}

func decodeBundleProofExtrinsic(bundleProof []byte) (OrchardBundleType, []byte, []byte, error) {
	if len(bundleProof) == 0 {
		return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof empty")
	}

	switch bundleProof[0] {
	case 3:
		cursor := 1
		if len(bundleProof) < cursor+4 {
			return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (vk_id)")
		}
		cursor += 4
		if len(bundleProof) < cursor+4 {
			return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (inputs len)")
		}
		inputCount := int(binary.LittleEndian.Uint32(bundleProof[cursor : cursor+4]))
		cursor += 4
		inputsLen := inputCount * 32
		if len(bundleProof) < cursor+inputsLen {
			return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (inputs)")
		}
		cursor += inputsLen
		if len(bundleProof) < cursor+4 {
			return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (proof len)")
		}
		proofLen := int(binary.LittleEndian.Uint32(bundleProof[cursor : cursor+4]))
		cursor += 4
		if len(bundleProof) < cursor+proofLen {
			return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (proof)")
		}
		cursor += proofLen
		if len(bundleProof) < cursor+4 {
			return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (bundle len)")
		}
		bundleLen := int(binary.LittleEndian.Uint32(bundleProof[cursor : cursor+4]))
		cursor += 4
		if len(bundleProof) < cursor+bundleLen {
			return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (bundle)")
		}
		bundle := append([]byte(nil), bundleProof[cursor:cursor+bundleLen]...)
		return OrchardBundleVanilla, bundle, nil, nil
	case 5:
		if len(bundleProof) < 6 {
			return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (v6 header)")
		}
		bundleType := OrchardBundleType(bundleProof[1])
		orchardLen := int(binary.LittleEndian.Uint32(bundleProof[2:6]))
		cursor := 6
		if len(bundleProof) < cursor+orchardLen+1 {
			return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (orchard bytes)")
		}
		orchardBytes := append([]byte(nil), bundleProof[cursor:cursor+orchardLen]...)
		cursor += orchardLen
		issueFlag := bundleProof[cursor]
		cursor++
		if issueFlag == 0x01 {
			if len(bundleProof) < cursor+4 {
				return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (issue len)")
			}
			issueLen := int(binary.LittleEndian.Uint32(bundleProof[cursor : cursor+4]))
			cursor += 4
			if len(bundleProof) < cursor+issueLen {
				return OrchardBundleVanilla, nil, nil, fmt.Errorf("bundle proof truncated (issue bytes)")
			}
			issueBytes := append([]byte(nil), bundleProof[cursor:cursor+issueLen]...)
			return bundleType, orchardBytes, issueBytes, nil
		}
		return bundleType, orchardBytes, nil, nil
	default:
		return OrchardBundleVanilla, nil, nil, fmt.Errorf("unknown bundle proof tag %d", bundleProof[0])
	}
}

func TestWorkPackageGeneration_SingleSwapBundle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "orchard_swap_wp")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "shielded_store")
	shieldedStore, err := orchardshielded.NewShieldedStore(storePath)
	if err != nil {
		t.Fatalf("failed to create ShieldedStore: %v", err)
	}
	defer shieldedStore.Close()

	var commitmentA [32]byte
	commitmentA[0] = 0xAA
	if err := shieldedStore.AddCommitment(0, commitmentA); err != nil {
		t.Fatalf("failed to add commitment: %v", err)
	}

	rollup := &OrchardRollup{
		commitmentTree: ffi.NewSinsemillaMerkleTree(),
		nullifierTree:  orchardstate.NewSparseNullifierTree(32),
		shieldedStore:  shieldedStore,
	}
	if err := rollup.loadShieldedStateFromStore(); err != nil {
		t.Fatalf("loadShieldedStateFromStore failed: %v", err)
	}

	commitmentRoot, commitmentSize, frontier, err := rollup.CommitmentSnapshot()
	if err != nil {
		t.Fatalf("CommitmentSnapshot failed: %v", err)
	}
	nullifierRoot, nullifierSize, err := rollup.NullifierSnapshot()
	if err != nil {
		t.Fatalf("NullifierSnapshot failed: %v", err)
	}

	preState := &OrchardStateRoots{
		CommitmentRoot:     commitmentRoot,
		CommitmentSize:     commitmentSize,
		CommitmentFrontier: frontier,
		NullifierRoot:      nullifierRoot,
		NullifierSize:      nullifierSize,
	}

	// Generate nullifier and absence proof for the spend
	var spentNullifier [32]byte
	spentNullifier[0] = 0xAA
	spentNullifier[1] = 0x11 // Nullifier for commitment 0xAA

	nullifierProof, err := rollup.NullifierAbsenceProof(spentNullifier)
	if err != nil {
		t.Fatalf("NullifierAbsenceProof failed: %v", err)
	}

	nullifierProofs := map[[32]byte]NullifierAbsenceProof{
		spentNullifier: NullifierAbsenceProof{
			Leaf:     nullifierProof.Leaf,
			Siblings: nullifierProof.Siblings,
			Root:     nullifierProof.Root,
			Position: nullifierProof.Position,
		},
	}
	nullifiers := [][32]byte{spentNullifier}

	position, branch, err := rollup.CommitmentProof(commitmentA)
	if err != nil {
		t.Fatalf("CommitmentProof failed: %v", err)
	}
	spentCommitmentProofs := []SpentCommitmentProof{
		{
			Nullifier:      spentNullifier,
			Commitment:     commitmentA,
			TreePosition:   position,
			BranchSiblings: branch,
		},
	}

	preWitness, err := SerializePreStateWitness(preState, nullifiers, nullifierProofs, [32]byte{}, nil, nil)
	if err != nil {
		t.Fatalf("SerializePreStateWitness failed: %v", err)
	}

	newCommitment := [32]byte{0xDD}
	postState, err := computePostState(preState, [][32]byte{newCommitment}, len(nullifiers))
	if err != nil {
		t.Fatalf("computePostState failed: %v", err)
	}
	postWitness, err := SerializePostStateWitness(postState)
	if err != nil {
		t.Fatalf("SerializePostStateWitness failed: %v", err)
	}

	orchardBundleBytes := []byte{0x02, 0x01, 0xFF}
	bundleProof := SerializeBundleProofV6(OrchardBundleSwap, orchardBundleBytes, nil)

	wpFile := &WorkPackageFile{
		PreStateWitnessExtrinsic:  preWitness,
		PostStateWitnessExtrinsic: postWitness,
		BundleProofExtrinsic:      bundleProof,
		PreState:                  preState,
		PostState:                 postState,
		SpentCommitmentProofs:     spentCommitmentProofs,
		BundleType:                OrchardBundleSwap,
		OrchardBundleBytes:        orchardBundleBytes,
	}

	workPackage, err := wpFile.ConvertToJAMWorkPackageBundle(1, types.RefineContext{}, [32]byte{})
	if err != nil {
		t.Fatalf("ConvertToJAMWorkPackageBundle failed: %v", err)
	}
	if workPackage == nil {
		t.Fatal("expected work package bundle")
	}

	if len(workPackage.ExtrinsicData) != 1 {
		t.Fatalf("expected 1 extrinsic blob set, got %d", len(workPackage.ExtrinsicData))
	}
	if len(workPackage.ExtrinsicData[0]) != 3 {
		t.Fatalf("expected 3 extrinsics, got %d", len(workPackage.ExtrinsicData[0]))
	}
	if len(workPackage.WorkPackage.WorkItems) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(workPackage.WorkPackage.WorkItems))
	}

	payload := workPackage.WorkPackage.WorkItems[0].Payload
	expectedPayload := buildOrchardPreStatePayload(*preState, frontier, spentCommitmentProofs)
	if !bytes.Equal(payload, expectedPayload) {
		t.Fatalf("payload mismatch: expected %d bytes, got %d", len(expectedPayload), len(payload))
	}

	extrinsics := workPackage.ExtrinsicData[0]
	if !bytes.Equal(extrinsics[0], preWitness) {
		t.Fatal("pre-state witness extrinsic mismatch")
	}
	if !bytes.Equal(extrinsics[1], postWitness) {
		t.Fatal("post-state witness extrinsic mismatch")
	}
	if !bytes.Equal(extrinsics[2], bundleProof) {
		t.Fatal("bundle proof extrinsic mismatch")
	}
}

func TestWorkPackageGeneration_MultiBundle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "orchard_multi_wp")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "shielded_store")
	shieldedStore, err := orchardshielded.NewShieldedStore(storePath)
	if err != nil {
		t.Fatalf("failed to create ShieldedStore: %v", err)
	}
	defer shieldedStore.Close()

	var seedCommitment [32]byte
	seedCommitment[0] = 0xAA
	if err := shieldedStore.AddCommitment(0, seedCommitment); err != nil {
		t.Fatalf("failed to add commitment: %v", err)
	}

	refineRoot := [32]byte{0xEF}
	fakeNode := &fakeStateProvider{
		refineContext: types.RefineContext{
			StateRoot: common.BytesToHash(refineRoot[:]),
		},
	}

	rollup := &OrchardRollup{
		node:           fakeNode,
		serviceID:      1,
		commitmentTree: ffi.NewSinsemillaMerkleTree(),
		nullifierTree:  orchardstate.NewSparseNullifierTree(32),
		shieldedStore:  shieldedStore,
	}
	if err := rollup.loadShieldedStateFromStore(); err != nil {
		t.Fatalf("loadShieldedStateFromStore failed: %v", err)
	}

	commitmentRoot, commitmentSize, frontier, err := rollup.CommitmentSnapshot()
	if err != nil {
		t.Fatalf("CommitmentSnapshot failed: %v", err)
	}
	nullifierRoot, nullifierSize, err := rollup.NullifierSnapshot()
	if err != nil {
		t.Fatalf("NullifierSnapshot failed: %v", err)
	}

	preState := &OrchardStateRoots{
		CommitmentRoot:     commitmentRoot,
		CommitmentSize:     commitmentSize,
		CommitmentFrontier: frontier,
		NullifierRoot:      nullifierRoot,
		NullifierSize:      nullifierSize,
	}

	commitmentA := [32]byte{0xA1}
	commitmentB := [32]byte{0xB2}
	commitmentC := [32]byte{0xC3}
	baseTime := time.Now()
	bundleA := &ParsedBundle{
		ID:              "bundle-a",
		RawData:         []byte{0xA0},
		BundleType:      OrchardBundleSwap,
		Commitments:     [][32]byte{commitmentA},
		PreState:        preState,
		NullifierProofs: map[[32]byte]NullifierAbsenceProof{},
		Timestamp:       baseTime,
	}
	bundleB := &ParsedBundle{
		ID:              "bundle-b",
		RawData:         []byte{0xB0},
		BundleType:      OrchardBundleSwap,
		Commitments:     [][32]byte{commitmentB},
		PreState:        preState,
		NullifierProofs: map[[32]byte]NullifierAbsenceProof{},
		Timestamp:       baseTime.Add(1 * time.Second),
	}
	bundleC := &ParsedBundle{
		ID:              "bundle-c",
		RawData:         []byte{0xC0},
		BundleType:      OrchardBundleSwap,
		Commitments:     [][32]byte{commitmentC},
		PreState:        preState,
		NullifierProofs: map[[32]byte]NullifierAbsenceProof{},
		Timestamp:       baseTime.Add(2 * time.Second),
	}

	decoder := mapCommitmentDecoder{
		commitments: map[string][][32]byte{
			string(bundleA.RawData): {commitmentA},
			string(bundleB.RawData): {commitmentB},
			string(bundleC.RawData): {commitmentC},
		},
	}

	pool := &OrchardTxPool{
		tree:                 rollup.commitmentTree,
		rollup:               rollup,
		commitmentDecoder:    decoder,
		pendingBundles:       map[string]*ParsedBundle{bundleA.ID: bundleA, bundleB.ID: bundleB, bundleC.ID: bundleC},
		bundlesByTime:        []*ParsedBundle{bundleA, bundleB, bundleC},
		workPackageThreshold: 3,
		lastWorkPackageTime:  time.Now().Add(-2 * time.Minute),
		stats: PoolStats{
			LastActivity: time.Now(),
		},
	}

	if err := pool.generateWorkPackage(); err != nil {
		t.Fatalf("generateWorkPackage failed: %v", err)
	}
	if len(fakeNode.submitted) != 3 {
		t.Fatalf("expected 3 submitted work packages, got %d", len(fakeNode.submitted))
	}

	expectedBundles := []*ParsedBundle{bundleA, bundleB, bundleC}
	for i, bundle := range expectedBundles {
		submitted := fakeNode.submitted[i]
		if submitted == nil || len(submitted.ExtrinsicData) != 1 {
			t.Fatalf("bundle %d missing extrinsics", i)
		}
		if len(submitted.ExtrinsicData[0]) != 3 {
			t.Fatalf("bundle %d expected 3 extrinsics, got %d", i, len(submitted.ExtrinsicData[0]))
		}

		rootBytes := [32]byte{}
		copy(rootBytes[:], fakeNode.refineContext.StateRoot.Bytes())
		preWitness, err := SerializePreStateWitness(
			bundle.PreState,
			bundle.Nullifiers,
			bundle.NullifierProofs,
			rootBytes,
			nil,
			nil,
		)
		if err != nil {
			t.Fatalf("pre-state witness failed: %v", err)
		}
		postState, err := computePostState(bundle.PreState, bundle.Commitments, len(bundle.Nullifiers))
		if err != nil {
			t.Fatalf("post-state compute failed: %v", err)
		}
		postWitness, err := SerializePostStateWitness(postState)
		if err != nil {
			t.Fatalf("post-state witness failed: %v", err)
		}
		bundleProof := SerializeBundleProofV6(bundle.BundleType, bundle.RawData, bundle.IssueBundle)

		extrinsics := submitted.ExtrinsicData[0]
		if !bytes.Equal(extrinsics[0], preWitness) {
			t.Fatalf("bundle %d pre-state witness mismatch", i)
		}
		if !bytes.Equal(extrinsics[1], postWitness) {
			t.Fatalf("bundle %d post-state witness mismatch", i)
		}
		if !bytes.Equal(extrinsics[2], bundleProof) {
			t.Fatalf("bundle %d bundle proof mismatch", i)
		}

		expectedPayload := buildOrchardPreStatePayload(*bundle.PreState, bundle.PreState.CommitmentFrontier, bundle.SpentCommitmentProofs)
		payload := submitted.WorkPackage.WorkItems[0].Payload
		if !bytes.Equal(payload, expectedPayload) {
			t.Fatalf("bundle %d payload mismatch: expected %d bytes, got %d", i, len(expectedPayload), len(payload))
		}
	}
}

func TestWorkPackageGeneration_SwapPlusIssuance(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "orchard_swap_issue_wp")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "shielded_store")
	shieldedStore, err := orchardshielded.NewShieldedStore(storePath)
	if err != nil {
		t.Fatalf("failed to create ShieldedStore: %v", err)
	}
	defer shieldedStore.Close()

	// Add two commitments (AAA and BBB) representing pre-swap state
	var commitmentAAA [32]byte
	commitmentAAA[0] = 0xAA
	commitmentAAA[1] = 0xA1
	if err := shieldedStore.AddCommitment(0, commitmentAAA); err != nil {
		t.Fatalf("failed to add commitment AAA: %v", err)
	}

	var commitmentBBB [32]byte
	commitmentBBB[0] = 0xBB
	commitmentBBB[1] = 0xB1
	if err := shieldedStore.AddCommitment(1, commitmentBBB); err != nil {
		t.Fatalf("failed to add commitment BBB: %v", err)
	}

	rollup := &OrchardRollup{
		commitmentTree: ffi.NewSinsemillaMerkleTree(),
		nullifierTree:  orchardstate.NewSparseNullifierTree(32),
		shieldedStore:  shieldedStore,
	}
	if err := rollup.loadShieldedStateFromStore(); err != nil {
		t.Fatalf("loadShieldedStateFromStore failed: %v", err)
	}

	commitmentRoot, commitmentSize, frontier, err := rollup.CommitmentSnapshot()
	if err != nil {
		t.Fatalf("CommitmentSnapshot failed: %v", err)
	}
	nullifierRoot, nullifierSize, err := rollup.NullifierSnapshot()
	if err != nil {
		t.Fatalf("NullifierSnapshot failed: %v", err)
	}

	preState := &OrchardStateRoots{
		CommitmentRoot:     commitmentRoot,
		CommitmentSize:     commitmentSize,
		CommitmentFrontier: frontier,
		NullifierRoot:      nullifierRoot,
		NullifierSize:      nullifierSize,
	}

	// Generate nullifiers and absence proofs for the 2 swaps (Alice spends AAA, Bob spends BBB)
	var nullifierAliceAAA [32]byte
	nullifierAliceAAA[0] = 0xAA
	nullifierAliceAAA[1] = 0xA1
	nullifierAliceAAA[2] = 0x01 // Alice's nullifier for AAA

	var nullifierBobBBB [32]byte
	nullifierBobBBB[0] = 0xBB
	nullifierBobBBB[1] = 0xB1
	nullifierBobBBB[2] = 0x02 // Bob's nullifier for BBB

	nullifierProofAlice, err := rollup.NullifierAbsenceProof(nullifierAliceAAA)
	if err != nil {
		t.Fatalf("NullifierAbsenceProof for Alice failed: %v", err)
	}

	nullifierProofBob, err := rollup.NullifierAbsenceProof(nullifierBobBBB)
	if err != nil {
		t.Fatalf("NullifierAbsenceProof for Bob failed: %v", err)
	}

	nullifiers := [][32]byte{nullifierAliceAAA, nullifierBobBBB}
	nullifierProofs := map[[32]byte]NullifierAbsenceProof{
		nullifierAliceAAA: NullifierAbsenceProof{
			Leaf:     nullifierProofAlice.Leaf,
			Siblings: nullifierProofAlice.Siblings,
			Root:     nullifierProofAlice.Root,
			Position: nullifierProofAlice.Position,
		},
		nullifierBobBBB: NullifierAbsenceProof{
			Leaf:     nullifierProofBob.Leaf,
			Siblings: nullifierProofBob.Siblings,
			Root:     nullifierProofBob.Root,
			Position: nullifierProofBob.Position,
		},
	}

	positionAAA, branchAAA, err := rollup.CommitmentProof(commitmentAAA)
	if err != nil {
		t.Fatalf("CommitmentProof AAA failed: %v", err)
	}
	positionBBB, branchBBB, err := rollup.CommitmentProof(commitmentBBB)
	if err != nil {
		t.Fatalf("CommitmentProof BBB failed: %v", err)
	}
	spentCommitmentProofs := []SpentCommitmentProof{
		{
			Nullifier:      nullifierAliceAAA,
			Commitment:     commitmentAAA,
			TreePosition:   positionAAA,
			BranchSiblings: branchAAA,
		},
		{
			Nullifier:      nullifierBobBBB,
			Commitment:     commitmentBBB,
			TreePosition:   positionBBB,
			BranchSiblings: branchBBB,
		},
	}

	preWitness, err := SerializePreStateWitness(preState, nullifiers, nullifierProofs, [32]byte{}, nil, nil)
	if err != nil {
		t.Fatalf("SerializePreStateWitness failed: %v", err)
	}

	// Post-state: 2 swap outputs (Alice→Bob AAA, Bob→Alice BBB) + 1 issuance (Carol DDD)
	var swapOutputAliceToBob [32]byte
	swapOutputAliceToBob[0] = 0xAA
	swapOutputAliceToBob[2] = 0x02 // Output to Bob

	var swapOutputBobToAlice [32]byte
	swapOutputBobToAlice[0] = 0xBB
	swapOutputBobToAlice[2] = 0x03 // Output to Alice

	var issuanceOutputCarolDDD [32]byte
	issuanceOutputCarolDDD[0] = 0xDD
	issuanceOutputCarolDDD[1] = 0xD1

	newCommitments := [][32]byte{
		swapOutputAliceToBob,
		swapOutputBobToAlice,
		issuanceOutputCarolDDD,
	}

	// 2 nullifiers from swap (Alice spends AAA, Bob spends BBB)
	postState, err := computePostState(preState, newCommitments, len(nullifiers))
	if err != nil {
		t.Fatalf("computePostState failed: %v", err)
	}
	postWitness, err := SerializePostStateWitness(postState)
	if err != nil {
		t.Fatalf("SerializePostStateWitness failed: %v", err)
	}

	// SwapBundle: 2 action groups (Alice→Bob AAA, Bob→Alice BBB)
	swapBundleBytes := []byte{0x02, 0x02, 0xAA, 0xBB} // tag=2 (swap), 2 action groups

	// IssueBundle: 1 action (Carol receives 500 DDD)
	issueBundleBytes := []byte{0x01, 0xDD, 0x01, 0xF4} // 1 action, asset DDD, 500 units

	// Bundle proof V6 with both SwapBundle + IssueBundle
	bundleProof := SerializeBundleProofV6(OrchardBundleSwap, swapBundleBytes, issueBundleBytes)

	wpFile := &WorkPackageFile{
		PreStateWitnessExtrinsic:  preWitness,
		PostStateWitnessExtrinsic: postWitness,
		BundleProofExtrinsic:      bundleProof,
		PreState:                  preState,
		PostState:                 postState,
		SpentCommitmentProofs:     spentCommitmentProofs,
		BundleType:                OrchardBundleSwap,
		OrchardBundleBytes:        swapBundleBytes,
		IssueBundleBytes:          issueBundleBytes,
	}

	workPackage, err := wpFile.ConvertToJAMWorkPackageBundle(1, types.RefineContext{}, [32]byte{})
	if err != nil {
		t.Fatalf("ConvertToJAMWorkPackageBundle failed: %v", err)
	}
	if workPackage == nil {
		t.Fatal("expected work package bundle")
	}

	if len(workPackage.ExtrinsicData) != 1 {
		t.Fatalf("expected 1 extrinsic blob set, got %d", len(workPackage.ExtrinsicData))
	}
	if len(workPackage.ExtrinsicData[0]) != 3 {
		t.Fatalf("expected 3 extrinsics, got %d", len(workPackage.ExtrinsicData[0]))
	}
	if len(workPackage.WorkPackage.WorkItems) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(workPackage.WorkPackage.WorkItems))
	}

	payload := workPackage.WorkPackage.WorkItems[0].Payload
	expectedPayload := buildOrchardPreStatePayload(*preState, frontier, spentCommitmentProofs)
	if !bytes.Equal(payload, expectedPayload) {
		t.Fatalf("payload mismatch: expected %d bytes, got %d", len(expectedPayload), len(payload))
	}

	extrinsics := workPackage.ExtrinsicData[0]
	if !bytes.Equal(extrinsics[0], preWitness) {
		t.Fatal("pre-state witness extrinsic mismatch")
	}
	if !bytes.Equal(extrinsics[1], postWitness) {
		t.Fatal("post-state witness extrinsic mismatch")
	}
	if !bytes.Equal(extrinsics[2], bundleProof) {
		t.Fatal("bundle proof extrinsic mismatch")
	}

	// Verify bundle proof contains both SwapBundle and IssueBundle
	// Format: [tag=5][bundle_type][orchard_len][orchard_bytes][issue_flag][issue_len][issue_bytes]
	if bundleProof[0] != 5 {
		t.Fatalf("expected bundle proof tag 5, got %d", bundleProof[0])
	}
	if bundleProof[1] != byte(OrchardBundleSwap) {
		t.Fatalf("expected bundle type Swap (2), got %d", bundleProof[1])
	}

	// Check that IssueBundle is present (flag byte should be 0x01)
	orchardLen := binary.LittleEndian.Uint32(bundleProof[2:6])
	issueFlagOffset := 6 + int(orchardLen)
	if issueFlagOffset >= len(bundleProof) {
		t.Fatal("bundle proof too short to contain issue flag")
	}
	if bundleProof[issueFlagOffset] != 0x01 {
		t.Fatalf("expected issue bundle present flag 0x01, got 0x%02x", bundleProof[issueFlagOffset])
	}

	// Validate post-state has 3 new commitments
	if postState.CommitmentSize != commitmentSize+3 {
		t.Fatalf("expected commitment size %d, got %d", commitmentSize+3, postState.CommitmentSize)
	}

	// Validate 2 nullifiers from swap
	if postState.NullifierSize != nullifierSize+2 {
		t.Fatalf("expected nullifier size %d, got %d", nullifierSize+2, postState.NullifierSize)
	}
}

func TestWorkPackageRefine_EndToEnd(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test path")
	}
	wpPath := filepath.Join(filepath.Dir(file), "..", "work_packages", "package_000.json")
	payload := loadWorkPackageJSON(t, wpPath)

	preWitness, err := base64.StdEncoding.DecodeString(payload.PreStateWitnessExtrinsic)
	if err != nil {
		t.Fatalf("failed to decode pre-state witness: %v", err)
	}
	postWitness, err := base64.StdEncoding.DecodeString(payload.PostStateWitnessExtrinsic)
	if err != nil {
		t.Fatalf("failed to decode post-state witness: %v", err)
	}
	bundleProof, err := base64.StdEncoding.DecodeString(payload.BundleProofExtrinsic)
	if err != nil {
		t.Fatalf("failed to decode bundle proof: %v", err)
	}

	preStatePayload := buildOrchardPreStatePayload(
		payload.PreState,
		payload.PreState.CommitmentFrontier,
		payload.SpentCommitmentProofs,
	)

	ffi, err := orchardwitness.NewOrchardFFI()
	if err != nil {
		t.Fatalf("failed to init Orchard FFI: %v", err)
	}
	defer ffi.Cleanup()

	bundleType, orchardBytes, issueBytes, err := decodeBundleProofExtrinsic(bundleProof)
	if err != nil {
		t.Fatalf("failed to parse bundle proof extrinsic: %v", err)
	}

	var commitments [][32]byte
	var nullifiers [][32]byte
	switch bundleType {
	case OrchardBundleVanilla:
		decoded, err := ffi.DecodeBundle(orchardBytes)
		if err != nil {
			t.Fatalf("DecodeBundle failed: %v", err)
		}
		commitments = decoded.Commitments
		nullifiers = decoded.Nullifiers
	case OrchardBundleZSA, OrchardBundleSwap:
		decoded, err := ffi.DecodeBundleV6(orchardBytes, issueBytes)
		if err != nil {
			t.Fatalf("DecodeBundleV6 failed: %v", err)
		}
		commitments = decoded.Commitments
		nullifiers = decoded.Nullifiers
		if len(decoded.IssueBundle) > 0 {
			issueCommitments, err := ffi.IssueBundleCommitments(decoded.IssueBundle)
			if err != nil {
				t.Fatalf("IssueBundleCommitments failed: %v", err)
			}
			commitments = append(commitments, issueCommitments...)
		}
	default:
		t.Fatalf("unsupported bundle type %d", bundleType)
	}

	expectedPost, err := computePostState(&payload.PreState, commitments, len(nullifiers))
	if err != nil {
		t.Fatalf("computePostState failed: %v", err)
	}
	if expectedPost.CommitmentRoot != payload.PostState.CommitmentRoot {
		t.Fatalf("post-state commitment root mismatch: expected %x got %x",
			payload.PostState.CommitmentRoot[:4], expectedPost.CommitmentRoot[:4])
	}
	if expectedPost.CommitmentSize != payload.PostState.CommitmentSize {
		t.Fatalf("post-state commitment size mismatch: expected %d got %d",
			payload.PostState.CommitmentSize, expectedPost.CommitmentSize)
	}
	if expectedPost.NullifierSize != payload.PostState.NullifierSize {
		t.Fatalf("post-state nullifier size mismatch: expected %d got %d",
			payload.PostState.NullifierSize, expectedPost.NullifierSize)
	}

	serviceID := ffi.GetServiceID()
	if err := ffi.RefineWorkPackage(serviceID, preStatePayload, preWitness, postWitness, bundleProof); err != nil {
		t.Fatalf("refine failed: %v", err)
	}

	if len(bundleProof) > 0 {
		badBundle := append([]byte(nil), bundleProof...)
		badBundle[len(badBundle)-1] ^= 0x01
		if err := ffi.RefineWorkPackage(serviceID, preStatePayload, preWitness, postWitness, badBundle); err == nil {
			t.Fatal("expected refine to reject corrupted bundle proof")
		}
	}
}

func TestTransparentStore_WorkPackageIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "orchard_transparent_wp")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	txStore, err := NewTransparentTxStore(tempDir)
	if err != nil {
		t.Fatalf("failed to create TransparentTxStore: %v", err)
	}
	defer txStore.Close()

	storePath := filepath.Join(tempDir, "transparent_store")
	utxoStore, err := orchardtransparent.NewTransparentStore(storePath)
	if err != nil {
		t.Fatalf("failed to create TransparentStore: %v", err)
	}
	defer utxoStore.Close()

	var txid [32]byte
	txid[0] = 0xAB
	outpoint := orchardtransparent.OutPoint{TxID: txid, Index: 0}
	if err := utxoStore.AddUTXO(outpoint, orchardtransparent.UTXO{
		Value:        42,
		ScriptPubKey: []byte{0x51},
		Height:       1,
		IsCoinbase:   false,
	}); err != nil {
		t.Fatalf("failed to add UTXO: %v", err)
	}

	rollup := &OrchardRollup{
		transparentTxStore:  txStore,
		transparentUtxoTree: NewTransparentUtxoTree(),
		transparentStore:    utxoStore,
	}

	entries, extrinsic, merkleRoot, utxoRoot, utxoSize, postRoot, postSize, err :=
		rollup.buildTransparentTxDataExtrinsic(100)
	if err != nil {
		t.Fatalf("buildTransparentTxDataExtrinsic failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no mempool entries, got %d", len(entries))
	}
	if len(extrinsic) == 0 {
		t.Fatal("expected transparent tx extrinsic to be populated")
	}
	if utxoSize != 1 {
		t.Fatalf("expected utxo size 1, got %d", utxoSize)
	}
	if utxoRoot == ([32]byte{}) {
		t.Fatal("expected non-zero utxo root")
	}
	if merkleRoot != ([32]byte{}) {
		t.Fatalf("expected empty mempool merkle root, got %x", merkleRoot[:4])
	}
	if postSize != utxoSize {
		t.Fatalf("expected post size %d, got %d", utxoSize, postSize)
	}
	if postRoot != utxoRoot {
		t.Fatalf("expected post root to match pre root")
	}
}

func TestShieldedStore_WorkPackageIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "orchard_shielded_wp")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "shielded_store")
	shieldedStore, err := orchardshielded.NewShieldedStore(storePath)
	if err != nil {
		t.Fatalf("failed to create ShieldedStore: %v", err)
	}
	defer shieldedStore.Close()

	var commitmentA [32]byte
	var commitmentB [32]byte
	commitmentA[0] = 0xAA
	commitmentB[0] = 0xBB
	if err := shieldedStore.AddCommitment(0, commitmentA); err != nil {
		t.Fatalf("failed to add commitment A: %v", err)
	}
	if err := shieldedStore.AddCommitment(1, commitmentB); err != nil {
		t.Fatalf("failed to add commitment B: %v", err)
	}

	var nullifier [32]byte
	nullifier[0] = 0xCC
	if err := shieldedStore.AddNullifier(nullifier, 1); err != nil {
		t.Fatalf("failed to add nullifier: %v", err)
	}

	rollup := &OrchardRollup{
		commitmentTree: ffi.NewSinsemillaMerkleTree(),
		nullifierTree:  orchardstate.NewSparseNullifierTree(32),
		shieldedStore:  shieldedStore,
	}
	if err := rollup.loadShieldedStateFromStore(); err != nil {
		t.Fatalf("loadShieldedStateFromStore failed: %v", err)
	}

	commitmentRoot, commitmentSize, frontier, err := rollup.CommitmentSnapshot()
	if err != nil {
		t.Fatalf("CommitmentSnapshot failed: %v", err)
	}
	if commitmentSize != 2 {
		t.Fatalf("expected commitment size 2, got %d", commitmentSize)
	}
	if commitmentRoot == ([32]byte{}) {
		t.Fatal("expected non-zero commitment root")
	}

	expectedTree := ffi.NewSinsemillaMerkleTree()
	if err := expectedTree.ComputeRoot([][32]byte{commitmentA, commitmentB}); err != nil {
		t.Fatalf("failed to compute expected root: %v", err)
	}
	if expectedRoot := expectedTree.GetRoot(); commitmentRoot != expectedRoot {
		t.Fatalf("commitment root mismatch: expected %x, got %x",
			expectedRoot[:4], commitmentRoot[:4])
	}
	if len(frontier) == 0 {
		t.Fatal("expected non-empty commitment frontier")
	}

	nullifierRoot, nullifierSize, err := rollup.NullifierSnapshot()
	if err != nil {
		t.Fatalf("NullifierSnapshot failed: %v", err)
	}
	if nullifierSize != 1 {
		t.Fatalf("expected nullifier size 1, got %d", nullifierSize)
	}
	if nullifierRoot == ([32]byte{}) {
		t.Fatal("expected non-zero nullifier root")
	}

	expectedNullifierTree := orchardstate.NewSparseNullifierTree(32)
	expectedNullifierTree.Insert(nullifier)
	if expectedNullifierRoot := expectedNullifierTree.Root(); nullifierRoot != expectedNullifierRoot {
		t.Fatalf("nullifier root mismatch: expected %x, got %x",
			expectedNullifierRoot[:4], nullifierRoot[:4])
	}

	preState := &OrchardStateRoots{
		CommitmentRoot:     commitmentRoot,
		CommitmentSize:     commitmentSize,
		CommitmentFrontier: frontier,
		NullifierRoot:      nullifierRoot,
		NullifierSize:      nullifierSize,
	}
	payload := buildOrchardPreStatePayload(*preState, frontier, nil)
	if len(payload) < orchardPreStatePayloadBaseLen {
		t.Fatalf("expected payload >= %d bytes, got %d", orchardPreStatePayloadBaseLen, len(payload))
	}
	if !bytes.Equal(payload[:32], commitmentRoot[:]) {
		t.Fatal("commitment root not embedded in payload")
	}
	commitmentSizeEncoded := binary.LittleEndian.Uint64(payload[32:40])
	if commitmentSizeEncoded != commitmentSize {
		t.Fatalf("commitment size mismatch: expected %d, got %d", commitmentSize, commitmentSizeEncoded)
	}
	frontierCount := binary.LittleEndian.Uint32(payload[40:44])
	if int(frontierCount) != len(frontier) {
		t.Fatalf("frontier count mismatch: expected %d, got %d", len(frontier), frontierCount)
	}
	frontierOffset := 44 + (len(frontier) * 32)
	if frontierOffset+32+8 > len(payload) {
		t.Fatalf("payload too short for nullifier root, len=%d", len(payload))
	}
	if !bytes.Equal(payload[frontierOffset:frontierOffset+32], nullifierRoot[:]) {
		t.Fatal("nullifier root not embedded in payload")
	}
	nullifierSizeEncoded := binary.LittleEndian.Uint64(payload[frontierOffset+32 : frontierOffset+40])
	if nullifierSizeEncoded != nullifierSize {
		t.Fatalf("nullifier size mismatch: expected %d, got %d", nullifierSize, nullifierSizeEncoded)
	}

	preWitness, err := SerializePreStateWitness(preState, nil, nil, [32]byte{}, nil, nil)
	if err != nil {
		t.Fatalf("SerializePreStateWitness failed: %v", err)
	}
	if len(preWitness) < 5 {
		t.Fatalf("expected pre-state witness length >= 5, got %d", len(preWitness))
	}
	if preWitness[0] != 1 {
		t.Fatalf("expected pre-state witness tag 1, got %d", preWitness[0])
	}
	payloadLen := binary.LittleEndian.Uint32(preWitness[1:5])
	if int(payloadLen) != len(preWitness)-5 {
		t.Fatalf("payload length mismatch: header=%d actual=%d", payloadLen, len(preWitness)-5)
	}
	payloadStart := preWitness[5:]
	if len(payloadStart) < 64 {
		t.Fatalf("expected witness payload >= 64 bytes, got %d", len(payloadStart))
	}
	if !bytes.Equal(payloadStart[:32], nullifierRoot[:]) || !bytes.Equal(payloadStart[32:64], nullifierRoot[:]) {
		t.Fatal("expected nullifier root to be embedded in witness payload")
	}
}

func TestTxpoolBundleSelection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "orchard_txpool_selection")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "shielded_store")
	shieldedStore, err := orchardshielded.NewShieldedStore(storePath)
	if err != nil {
		t.Fatalf("failed to create ShieldedStore: %v", err)
	}
	defer shieldedStore.Close()

	var seedCommitment [32]byte
	seedCommitment[0] = 0xAA
	if err := shieldedStore.AddCommitment(0, seedCommitment); err != nil {
		t.Fatalf("failed to add commitment: %v", err)
	}

	rollup := &OrchardRollup{
		commitmentTree: ffi.NewSinsemillaMerkleTree(),
		nullifierTree:  orchardstate.NewSparseNullifierTree(32),
		shieldedStore:  shieldedStore,
	}
	if err := rollup.loadShieldedStateFromStore(); err != nil {
		t.Fatalf("loadShieldedStateFromStore failed: %v", err)
	}

	newPool := func(t *testing.T) *OrchardTxPool {
		orchardFFI, err := orchardwitness.NewOrchardFFI()
		if err != nil {
			t.Skipf("orchard FFI unavailable: %v", err)
		}
		pool := &OrchardTxPool{
			tree:                 ffi.NewSinsemillaMerkleTree(),
			orchardFFI:           orchardFFI,
			commitmentDecoder:    orchardFFI,
			rollup:               rollup,
			pendingBundles:       make(map[string]*ParsedBundle),
			nullifiers:           make(map[[32]byte]bool),
			bundleQueue:          make(chan *ParsedBundle, MaxPoolSize),
			bundleNullifiers:     make(map[string][][32]byte),
			treeCheckpoints:      make(map[string]*TreeCheckpoint),
			maxPoolSize:          MaxPoolSize,
			bundlesByTime:        make([]*ParsedBundle, 0),
			workPackageThreshold: WorkPackageThreshold,
			lastWorkPackageTime:  time.Now().Add(-10 * time.Minute),
		}
		if err := pool.initializeFromRollupState(); err != nil {
			t.Fatalf("initializeFromRollupState failed: %v", err)
		}
		return pool
	}

	issueBundleBytes := func() []byte {
		bundle := make([]byte, 0, 1+1+1+32+1+43+8+32+32+3)
		bundle = append(bundle, 1)           // issuer_len
		bundle = append(bundle, 0x01)        // issuer_key
		bundle = append(bundle, 1)           // action_count
		bundle = append(bundle, make([]byte, 32)...)
		bundle = append(bundle, 1) // note_count
		bundle = append(bundle, make([]byte, 43)...)
		bundle = append(bundle, 1, 0, 0, 0, 0, 0, 0, 0) // value
		bundle = append(bundle, make([]byte, 32)...)
		bundle = append(bundle, make([]byte, 32)...)
		bundle = append(bundle, 0, 0, 0) // finalize, sighash_info_len, signature_len
		return bundle
	}

	addBundle := func(t *testing.T, pool *OrchardTxPool, id string) {
		if err := pool.AddBundle(nil, issueBundleBytes(), id); err != nil {
			t.Fatalf("AddBundle failed: %v", err)
		}
	}

	t.Run("fifo ordering", func(t *testing.T) {
		pool := newPool(t)
		pool.workPackageThreshold = 5

		for i := 0; i < 10; i++ {
			addBundle(t, pool, fmt.Sprintf("bundle-%d", i))
		}

		for i := 0; i < 10; i++ {
			expectedID := fmt.Sprintf("bundle-%d", i)
			if pool.bundlesByTime[i].ID != expectedID {
				t.Fatalf("expected bundle[%d] to have ID %s, got %s", i, expectedID, pool.bundlesByTime[i].ID)
			}
		}

		if len(pool.bundlesByTime) < int(pool.workPackageThreshold) {
			t.Fatalf("expected at least %d bundles, got %d", pool.workPackageThreshold, len(pool.bundlesByTime))
		}

		for i, bundle := range pool.bundlesByTime[:pool.workPackageThreshold] {
			expectedID := fmt.Sprintf("bundle-%d", i)
			if bundle.ID != expectedID {
				t.Fatalf("expected bundle[%d] to have ID %s, got %s", i, expectedID, bundle.ID)
			}
		}
	})

	t.Run("pool capacity", func(t *testing.T) {
		pool := newPool(t)
		pool.maxPoolSize = 3

		for i := 0; i < pool.maxPoolSize; i++ {
			addBundle(t, pool, fmt.Sprintf("bundle-%d", i))
		}

		if err := pool.AddBundle(nil, issueBundleBytes(), "bundle-overflow"); err == nil {
			t.Fatal("expected pool capacity error, got nil")
		}
	})

	t.Run("remove bundles", func(t *testing.T) {
		pool := newPool(t)

		for i := 0; i < 5; i++ {
			addBundle(t, pool, fmt.Sprintf("bundle-%d", i))
		}

		pool.RemoveBundles([]string{"bundle-1", "bundle-3"})

		if len(pool.pendingBundles) != 3 {
			t.Fatalf("expected 3 pending bundles after removal, got %d", len(pool.pendingBundles))
		}
		if _, exists := pool.pendingBundles["bundle-1"]; exists {
			t.Fatal("bundle-1 should be removed from pendingBundles")
		}
		if _, exists := pool.pendingBundles["bundle-3"]; exists {
			t.Fatal("bundle-3 should be removed from pendingBundles")
		}
		for _, bundle := range pool.bundlesByTime {
			if bundle.ID == "bundle-1" || bundle.ID == "bundle-3" {
				t.Fatalf("removed bundle %s still present in bundlesByTime", bundle.ID)
			}
		}
	})

	t.Run("bundle size limit", func(t *testing.T) {
		pool := newPool(t)
		oversized := make([]byte, MaxBundleSize+1)
		if err := pool.AddBundle(oversized, nil, "bundle-oversized"); err == nil {
			t.Fatal("expected oversized bundle error, got nil")
		}
	})
}

func TestNullifierProofGeneration(t *testing.T) {
	rollup := &OrchardRollup{
		nullifierTree: orchardstate.NewSparseNullifierTree(32),
	}

	var spent [32]byte
	spent[0] = 0x11
	rollup.nullifierTree.Insert(spent)

	var unspent [32]byte
	unspent[0] = 0x22
	proof, err := rollup.NullifierAbsenceProof(unspent)
	if err != nil {
		t.Fatalf("NullifierAbsenceProof failed: %v", err)
	}
	if len(proof.Siblings) != 32 {
		t.Fatalf("expected 32 sibling hashes, got %d", len(proof.Siblings))
	}
	if !proof.Verify() {
		t.Fatal("absence proof failed to verify")
	}
	if proof.Root != rollup.nullifierTree.Root() {
		t.Fatal("absence proof root mismatch")
	}

	if _, err := rollup.NullifierAbsenceProof(spent); err == nil {
		t.Fatal("expected error for spent nullifier")
	}
}

func TestCommitmentTreeSync(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "orchard_commitment_sync")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "shielded_store")
	shieldedStore, err := orchardshielded.NewShieldedStore(storePath)
	if err != nil {
		t.Fatalf("failed to create ShieldedStore: %v", err)
	}
	defer shieldedStore.Close()

	var commitmentA [32]byte
	commitmentA[0] = 0xA1
	if err := shieldedStore.AddCommitment(0, commitmentA); err != nil {
		t.Fatalf("failed to add commitment A: %v", err)
	}

	rollup := &OrchardRollup{
		commitmentTree: ffi.NewSinsemillaMerkleTree(),
		nullifierTree:  orchardstate.NewSparseNullifierTree(32),
		shieldedStore:  shieldedStore,
	}
	if err := rollup.loadShieldedStateFromStore(); err != nil {
		t.Fatalf("loadShieldedStateFromStore failed: %v", err)
	}

	commitmentRoot, commitmentSize, frontier, err := rollup.CommitmentSnapshot()
	if err != nil {
		t.Fatalf("CommitmentSnapshot failed: %v", err)
	}
	preState := &OrchardStateRoots{
		CommitmentRoot:     commitmentRoot,
		CommitmentSize:     commitmentSize,
		CommitmentFrontier: frontier,
		NullifierRoot:      [32]byte{},
		NullifierSize:      0,
	}

	allCommitments := [][32]byte{commitmentA}
	newCommitments := [][32]byte{
		{0xB2},
		{0xC3},
		{0xD4},
	}

	for idx, commitment := range newCommitments {
		postState, err := computePostState(preState, [][32]byte{commitment}, 0)
		if err != nil {
			t.Fatalf("computePostState step %d failed: %v", idx, err)
		}

		if err := rollup.ApplyBundleState([][32]byte{commitment}, nil); err != nil {
			t.Fatalf("ApplyBundleState step %d failed: %v", idx, err)
		}
		root, size, frontier, err := rollup.CommitmentSnapshot()
		if err != nil {
			t.Fatalf("CommitmentSnapshot step %d failed: %v", idx, err)
		}
		if root != postState.CommitmentRoot || size != postState.CommitmentSize {
			t.Fatalf("snapshot mismatch at step %d", idx)
		}

		allCommitments = append(allCommitments, commitment)
		expectedTree := ffi.NewSinsemillaMerkleTree()
		if err := expectedTree.ComputeRoot(allCommitments); err != nil {
			t.Fatalf("expected root compute failed at step %d: %v", idx, err)
		}
		if expectedRoot := expectedTree.GetRoot(); expectedRoot != postState.CommitmentRoot {
			t.Fatalf("root mismatch at step %d", idx)
		}
		expectedFrontier := expectedTree.GetFrontier()
		if len(frontier) != len(expectedFrontier) {
			t.Fatalf("frontier length mismatch at step %d", idx)
		}
		for i := range frontier {
			if frontier[i] != expectedFrontier[i] {
				t.Fatalf("frontier mismatch at step %d index %d", idx, i)
			}
		}

		preState = postState
		preState.CommitmentFrontier = frontier
	}
}

func TestStateRecoveryAfterRestart(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "orchard_recovery")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storePath := filepath.Join(tempDir, "shielded_store")

	// Phase 1: Initial state creation
	shieldedStore, err := orchardshielded.NewShieldedStore(storePath)
	if err != nil {
		t.Fatalf("failed to create ShieldedStore: %v", err)
	}

	var commitment1 [32]byte
	commitment1[0] = 0xAA
	commitment1[1] = 0x01
	if err := shieldedStore.AddCommitment(0, commitment1); err != nil {
		t.Fatalf("failed to add commitment 1: %v", err)
	}

	var commitment2 [32]byte
	commitment2[0] = 0xBB
	commitment2[1] = 0x02
	if err := shieldedStore.AddCommitment(1, commitment2); err != nil {
		t.Fatalf("failed to add commitment 2: %v", err)
	}

	var commitment3 [32]byte
	commitment3[0] = 0xCC
	commitment3[1] = 0x03
	if err := shieldedStore.AddCommitment(2, commitment3); err != nil {
		t.Fatalf("failed to add commitment 3: %v", err)
	}

	// Load initial state into rollup
	rollup := &OrchardRollup{
		commitmentTree: ffi.NewSinsemillaMerkleTree(),
		nullifierTree:  orchardstate.NewSparseNullifierTree(32),
		shieldedStore:  shieldedStore,
	}
	if err := rollup.loadShieldedStateFromStore(); err != nil {
		t.Fatalf("loadShieldedStateFromStore failed: %v", err)
	}

	// Capture state before "crash"
	commitmentRootBefore, commitmentSizeBefore, frontierBefore, err := rollup.CommitmentSnapshot()
	if err != nil {
		t.Fatalf("CommitmentSnapshot failed: %v", err)
	}

	var nullifier1 [32]byte
	nullifier1[0] = 0xAA
	nullifier1[1] = 0xA1
	rollup.nullifierTree.Insert(nullifier1)
	if err := shieldedStore.AddNullifier(nullifier1, 1); err != nil {
		t.Fatalf("failed to persist nullifier 1: %v", err)
	}

	var nullifier2 [32]byte
	nullifier2[0] = 0xBB
	nullifier2[1] = 0xB1
	rollup.nullifierTree.Insert(nullifier2)
	if err := shieldedStore.AddNullifier(nullifier2, 1); err != nil {
		t.Fatalf("failed to persist nullifier 2: %v", err)
	}

	nullifierRootBefore, nullifierSizeBefore, err := rollup.NullifierSnapshot()
	if err != nil {
		t.Fatalf("NullifierSnapshot failed: %v", err)
	}

	// Close store (simulate crash)
	if err := shieldedStore.Close(); err != nil {
		t.Fatalf("failed to close ShieldedStore: %v", err)
	}

	// Phase 2: Recovery after restart
	shieldedStoreRecovered, err := orchardshielded.NewShieldedStore(storePath)
	if err != nil {
		t.Fatalf("failed to reopen ShieldedStore: %v", err)
	}
	defer shieldedStoreRecovered.Close()

	rollupRecovered := &OrchardRollup{
		commitmentTree: ffi.NewSinsemillaMerkleTree(),
		nullifierTree:  orchardstate.NewSparseNullifierTree(32),
		shieldedStore:  shieldedStoreRecovered,
	}
	if err := rollupRecovered.loadShieldedStateFromStore(); err != nil {
		t.Fatalf("loadShieldedStateFromStore after recovery failed: %v", err)
	}

	// Validate commitment state recovered
	commitmentRootAfter, commitmentSizeAfter, frontierAfter, err := rollupRecovered.CommitmentSnapshot()
	if err != nil {
		t.Fatalf("CommitmentSnapshot after recovery failed: %v", err)
	}

	if commitmentRootAfter != commitmentRootBefore {
		t.Fatalf("commitment root mismatch after recovery: expected %x, got %x",
			commitmentRootBefore[:8], commitmentRootAfter[:8])
	}

	if commitmentSizeAfter != commitmentSizeBefore {
		t.Fatalf("commitment size mismatch after recovery: expected %d, got %d",
			commitmentSizeBefore, commitmentSizeAfter)
	}

	if len(frontierAfter) != len(frontierBefore) {
		t.Fatalf("frontier length mismatch after recovery: expected %d, got %d",
			len(frontierBefore), len(frontierAfter))
	}

	for i := range frontierBefore {
		if frontierAfter[i] != frontierBefore[i] {
			t.Fatalf("frontier[%d] mismatch after recovery", i)
		}
	}

	// Validate all 3 commitments present in recovered store
	commitmentEntries, err := shieldedStoreRecovered.ListCommitments()
	if err != nil {
		t.Fatalf("ListCommitments after recovery failed: %v", err)
	}

	if len(commitmentEntries) != 3 {
		t.Fatalf("expected 3 commitments after recovery, got %d", len(commitmentEntries))
	}

	if commitmentEntries[0].Commitment != commitment1 {
		t.Fatal("commitment 1 not recovered correctly")
	}
	if commitmentEntries[1].Commitment != commitment2 {
		t.Fatal("commitment 2 not recovered correctly")
	}
	if commitmentEntries[2].Commitment != commitment3 {
		t.Fatal("commitment 3 not recovered correctly")
	}

	// Nullifier tree is rebuilt from persisted nullifiers on restart.
	nullifierRootAfter, nullifierSizeAfter, err := rollupRecovered.NullifierSnapshot()
	if err != nil {
		t.Fatalf("NullifierSnapshot after recovery failed: %v", err)
	}

	if nullifierSizeAfter != nullifierSizeBefore {
		t.Fatalf("nullifier size mismatch after recovery: expected %d, got %d",
			nullifierSizeBefore, nullifierSizeAfter)
	}
	if nullifierRootAfter != nullifierRootBefore {
		t.Fatalf("nullifier root mismatch after recovery: expected %x, got %x",
			nullifierRootBefore[:8], nullifierRootAfter[:8])
	}

	// Validate we can add new commitments after recovery
	var commitment4 [32]byte
	commitment4[0] = 0xDD
	commitment4[1] = 0x04
	if err := shieldedStoreRecovered.AddCommitment(3, commitment4); err != nil {
		t.Fatalf("failed to add commitment after recovery: %v", err)
	}

	// Reload commitment tree with new commitment
	rollupRecovered.commitmentTree = ffi.NewSinsemillaMerkleTree()
	if err := rollupRecovered.loadShieldedStateFromStore(); err != nil {
		t.Fatalf("loadShieldedStateFromStore after adding commitment failed: %v", err)
	}

	commitmentRootFinal, commitmentSizeFinal, _, err := rollupRecovered.CommitmentSnapshot()
	if err != nil {
		t.Fatalf("CommitmentSnapshot after adding commitment failed: %v", err)
	}

	if commitmentSizeFinal != 4 {
		t.Fatalf("expected commitment size 4 after adding new commitment, got %d", commitmentSizeFinal)
	}

	if commitmentRootFinal == commitmentRootAfter {
		t.Fatal("commitment root should change after adding new commitment")
	}

	// Nullifier tree is rebuilt from persisted nullifiers (expected behavior).
	_ = nullifierRootBefore
	_ = nullifierSizeBefore
	_ = nullifierRootAfter
}

func TestRollbackAndReorg(t *testing.T) {
	t.Skip("integration test scaffold")
}
