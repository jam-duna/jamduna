package rpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	orchardffi "github.com/colorfulnotion/jam/builder/orchard/ffi"
	orchardwitness "github.com/colorfulnotion/jam/builder/orchard/witness"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
)

const (
	// Transaction pool configuration
	MaxPoolSize          = 1000            // Maximum number of pending bundles
	MaxBundleAge         = 5 * time.Minute // Maximum time a bundle can stay in pool
	WorkPackageThreshold = 10              // Minimum bundles to trigger work package generation
	MaxActionsPerBundle  = 4               // Orchard limit
	MaxBundleSize        = 64 * 1024       // 64KB maximum bundle size to prevent memory abuse
)

// ParsedBundle represents a validated Orchard bundle ready for pool inclusion
type ParsedBundle struct {
    ID           string            `json:"id"`            // Unique bundle identifier
    RawData      []byte            `json:"raw_data"`      // Original transaction bytes
    BundleType   OrchardBundleType `json:"bundle_type"`   // Vanilla/ZSA/Swap
    IssueBundle  []byte            `json:"issue_bundle"`  // Optional IssueBundle bytes (NU7)
    ActionGroups []int             `json:"action_groups"` // Action counts per group (NU7)
    Actions      []TxPoolAction    `json:"actions"`       // Parsed Orchard actions
    Nullifiers   [][32]byte        `json:"nullifiers"`    // Spent nullifiers
    Commitments  [][32]byte        `json:"commitments"`   // New commitments
    PreState     *OrchardStateRoots `json:"-"`            // Snapshot prior to applying this bundle
    NullifierProofs map[[32]byte]NullifierAbsenceProof `json:"-"` // Absence proofs captured at enqueue time
    SpentCommitmentProofs []SpentCommitmentProof `json:"-"`        // Membership proofs for spent notes
    BindingProof []byte            `json:"binding_proof"` // Bundle binding signature (legacy)
    ProofBytes   []byte            `json:"proof_bytes"`   // Halo2 proof bytes
    PublicInputs [][32]byte        `json:"public_inputs"` // Orchard NU5 public inputs
    Timestamp    time.Time         `json:"timestamp"`     // When added to pool
    GasEstimate  uint64            `json:"gas_estimate"`  // Estimated gas for inclusion
}

type commitmentDecoder interface {
	DecodeBundle(bundle []byte) (*orchardwitness.DecodedBundle, error)
	DecodeBundleV6(orchardBundle []byte, issueBundle []byte) (*orchardwitness.DecodedBundleV6, error)
}

// TxPoolAction represents a single action in an Orchard bundle for transaction pool purposes
type TxPoolAction struct {
	Nullifier    [32]byte `json:"nullifier"`     // Spent note nullifier (may be zero for outputs)
	Commitment   [32]byte `json:"commitment"`    // New note commitment (may be zero for inputs)
	Value        uint64   `json:"value"`         // Action value
	EphemeralKey [32]byte `json:"ephemeral_key"` // Ephemeral key for encryption
}

// OrchardTxPool manages pending Orchard transactions and automatic work package generation
type OrchardTxPool struct {
	// Dependencies
	tree       *orchardffi.SinsemillaMerkleTree // FFI interface for Merkle tree operations
	orchardFFI *orchardwitness.OrchardFFI
	commitmentDecoder commitmentDecoder
	rollup     *OrchardRollup // Access to current state
	node       interface{}    // JAM node interface for work package submission

	// Pool state protection
	mu sync.RWMutex

	// Pool storage
	pendingBundles map[string]*ParsedBundle // Bundle ID -> parsed bundle
	nullifiers     map[[32]byte]bool        // Track spent nullifiers to prevent double spends
	bundleQueue    chan *ParsedBundle       // Queue for processing bundles

	// State tracking for rollback
	bundleNullifiers map[string][][32]byte      // Bundle ID -> nullifiers for rollback
	treeCheckpoints  map[string]*TreeCheckpoint // Bundle ID -> tree state for rollback

	// Pool management
	maxPoolSize   int             // Maximum bundles in pool
	bundlesByTime []*ParsedBundle // Bundles sorted by timestamp for cleanup

	// Work package generation
	workPackageThreshold int       // Minimum bundles to trigger work package
	lastWorkPackageTime  time.Time // Last work package generation time

	// Statistics
	stats PoolStats
}

// PoolStats tracks transaction pool statistics
type PoolStats struct {
	TotalBundles          uint64    `json:"total_bundles"`
	PendingBundles        uint64    `json:"pending_bundles"`
	WorkPackagesGenerated uint64    `json:"work_packages_generated"`
	LastActivity          time.Time `json:"last_activity"`
	TreeSize              uint64    `json:"tree_size"`
	NullifierCount        uint64    `json:"nullifier_count"`
}

// TreeCheckpoint represents a tree state snapshot for rollback
type TreeCheckpoint struct {
	Root      [32]byte   `json:"root"`
	Size      uint64     `json:"size"`
	Frontier  [][32]byte `json:"frontier"`
	Timestamp time.Time  `json:"timestamp"`
}

// NewOrchardTxPool creates a new transaction pool with FFI integration
func NewOrchardTxPool(rollup *OrchardRollup, node interface{}) (*OrchardTxPool, error) {
	tree := orchardffi.NewSinsemillaMerkleTree()
	orchardFFI, err := orchardwitness.NewOrchardFFI()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Orchard FFI: %w", err)
	}

	pool := &OrchardTxPool{
		tree:                 tree,
		orchardFFI:           orchardFFI,
		commitmentDecoder:    orchardFFI,
		rollup:               rollup,
		node:                 node,
		pendingBundles:       make(map[string]*ParsedBundle),
		nullifiers:           make(map[[32]byte]bool),
		bundleQueue:          make(chan *ParsedBundle, MaxPoolSize),
		bundleNullifiers:     make(map[string][][32]byte),
		treeCheckpoints:      make(map[string]*TreeCheckpoint),
		maxPoolSize:          MaxPoolSize,
		bundlesByTime:        make([]*ParsedBundle, 0),
		workPackageThreshold: WorkPackageThreshold,
		stats: PoolStats{
			LastActivity: time.Now(),
		},
	}

	// Initialize tree with current state from rollup
	if err := pool.initializeFromRollupState(); err != nil {
		return nil, fmt.Errorf("failed to initialize pool from rollup state: %w", err)
	}

	// Start background workers
	go pool.bundleProcessor()
	go pool.workPackageGenerator()
	go pool.poolCleaner()

	log.Info(log.Node, "OrchardTxPool initialized",
		"tree_size", pool.tree.GetSize(),
		"max_pool_size", MaxPoolSize,
		"work_package_threshold", WorkPackageThreshold)

	return pool, nil
}

// initializeFromRollupState rebuilds the commitment tree from current rollup state
func (p *OrchardTxPool) initializeFromRollupState() error {
	commitments := p.rollup.CommitmentList()

	// Initialize tree with all commitments in order
	if len(commitments) > 0 {
		if err := p.tree.ComputeRoot(commitments); err != nil {
			return fmt.Errorf("failed to compute initial tree root: %w", err)
		}
	}

	// Initialize nullifier set from spent nullifiers
	spentNullifiers := p.rollup.SpentNullifiers()
	for _, nullifierHash := range spentNullifiers {
		var nullifier [32]byte
		copy(nullifier[:], nullifierHash[:])
		p.nullifiers[nullifier] = true
	}

	treeRoot := p.tree.GetRoot()
	log.Info(log.Node, "TxPool state initialized from rollup",
		"commitments", len(commitments),
		"nullifiers", len(p.nullifiers),
		"tree_root", hex.EncodeToString(treeRoot[:]))

	return nil
}

// AddBundle validates and adds a new bundle to the transaction pool
func (p *OrchardTxPool) AddBundle(rawData []byte, issueBundle []byte, bundleID string) error {
	// Parse and validate bundle
	bundle, err := p.parseBundle(rawData, issueBundle, bundleID)
	if err != nil {
		return fmt.Errorf("bundle parsing failed: %w", err)
	}

	// Validate against current pool state
	if err := p.validateBundle(bundle); err != nil {
		return fmt.Errorf("bundle validation failed: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check pool capacity
	if len(p.pendingBundles) >= p.maxPoolSize {
		return fmt.Errorf("transaction pool full (max %d bundles)", p.maxPoolSize)
	}

	// Create checkpoint before making changes
	checkpoint := &TreeCheckpoint{
		Root:      p.tree.GetRoot(),
		Size:      p.tree.GetSize(),
		Frontier:  p.tree.GetFrontier(),
		Timestamp: time.Now(),
	}
	p.treeCheckpoints[bundleID] = checkpoint

	// Store nullifiers for potential rollback
	p.bundleNullifiers[bundleID] = bundle.Nullifiers

	// Add to pool
	p.pendingBundles[bundleID] = bundle
	p.bundlesByTime = append(p.bundlesByTime, bundle)

	// Update nullifier tracking
	for _, nullifier := range bundle.Nullifiers {
		p.nullifiers[nullifier] = true
	}

	// Update statistics
	p.stats.TotalBundles++
	p.stats.PendingBundles = uint64(len(p.pendingBundles))
	p.stats.LastActivity = time.Now()
	p.stats.NullifierCount = uint64(len(p.nullifiers))

	log.Info(log.Node, "Bundle added to pool",
		"bundle_id", bundleID,
		"actions", len(bundle.Actions),
		"nullifiers", len(bundle.Nullifiers),
		"commitments", len(bundle.Commitments),
		"pool_size", len(p.pendingBundles))

	// Queue for processing
	select {
	case p.bundleQueue <- bundle:
		// Successfully queued
	default:
		log.Warn(log.Node, "Bundle queue full, dropping bundle", "bundle_id", bundleID)
	}

	return nil
}

// parseBundle parses raw transaction data into a structured bundle
func (p *OrchardTxPool) parseBundle(rawData []byte, issueBundle []byte, bundleID string) (*ParsedBundle, error) {
	// Validate bundle size to prevent memory abuse
	if len(rawData) == 0 && len(issueBundle) == 0 {
		return nil, fmt.Errorf("empty bundle data")
	}
	if len(rawData) > 0 && len(rawData) > MaxBundleSize {
		return nil, fmt.Errorf("bundle too large: %d bytes > %d max", len(rawData), MaxBundleSize)
	}
	if len(rawData) == 0 && len(issueBundle) > MaxBundleSize {
		return nil, fmt.Errorf("issue bundle too large: %d bytes > %d max", len(issueBundle), MaxBundleSize)
	}

	if p.orchardFFI == nil {
		return nil, fmt.Errorf("orchard FFI not initialized")
	}

	if len(rawData) > 0 {
		if decoded, err := p.orchardFFI.DecodeBundle(rawData); err == nil {
			if len(decoded.Nullifiers) == 0 {
				return nil, fmt.Errorf("bundle has no actions")
			}
			if len(decoded.Nullifiers) > MaxActionsPerBundle {
				return nil, fmt.Errorf("too many actions: %d > %d", len(decoded.Nullifiers), MaxActionsPerBundle)
			}
			if len(decoded.Commitments) != len(decoded.Nullifiers) {
				return nil, fmt.Errorf("bundle commitments mismatch: %d commitments for %d actions",
					len(decoded.Commitments), len(decoded.Nullifiers))
			}

			bundle := &ParsedBundle{
				ID:           bundleID,
				RawData:      rawData,
				BundleType:   OrchardBundleVanilla,
				Timestamp:    time.Now(),
				GasEstimate:  1000000, // 1M gas estimate
				ProofBytes:   decoded.Proof,
				PublicInputs: decoded.PublicInputs,
			}

			for i := range decoded.Nullifiers {
				action := TxPoolAction{
					Nullifier:  decoded.Nullifiers[i],
					Commitment: decoded.Commitments[i],
					Value:      0,
				}
				bundle.Actions = append(bundle.Actions, action)
				bundle.Nullifiers = append(bundle.Nullifiers, action.Nullifier)
				bundle.Commitments = append(bundle.Commitments, action.Commitment)
			}

			return bundle, nil
		}
	}

	decodedV6, err := p.orchardFFI.DecodeBundleV6(rawData, issueBundle)
	if err != nil {
		return nil, fmt.Errorf("bundle decode failed: %w", err)
	}
	bundleType, err := mapV6BundleType(decodedV6.BundleType)
	if err != nil {
		return nil, err
	}
	if len(decodedV6.Nullifiers) == 0 && len(decodedV6.IssueBundle) == 0 {
		return nil, fmt.Errorf("bundle has no actions")
	}
	if len(decodedV6.Nullifiers) > 0 {
		if len(decodedV6.Nullifiers) > MaxActionsPerBundle {
			return nil, fmt.Errorf("too many actions: %d > %d", len(decodedV6.Nullifiers), MaxActionsPerBundle)
		}
		if len(decodedV6.Commitments) != len(decodedV6.Nullifiers) {
			return nil, fmt.Errorf("bundle commitments mismatch: %d commitments for %d actions",
				len(decodedV6.Commitments), len(decodedV6.Nullifiers))
		}
	}
	if len(decodedV6.ActionGroupSizes) > 0 {
		expectedActions := 0
		for _, size := range decodedV6.ActionGroupSizes {
			expectedActions += size
		}
		if expectedActions != len(decodedV6.Nullifiers) {
			return nil, fmt.Errorf("action group size mismatch: expected %d actions, got %d",
				expectedActions, len(decodedV6.Nullifiers))
		}
		if expectedActions != len(decodedV6.Commitments) {
			return nil, fmt.Errorf("commitment count mismatch: expected %d commitments, got %d",
				expectedActions, len(decodedV6.Commitments))
		}
	}

	bundle := &ParsedBundle{
		ID:           bundleID,
		RawData:      rawData,
		BundleType:   bundleType,
		IssueBundle:  decodedV6.IssueBundle,
		ActionGroups: decodedV6.ActionGroupSizes,
		Timestamp:    time.Now(),
		GasEstimate:  1000000, // 1M gas estimate
	}

	for i := range decodedV6.Nullifiers {
		action := TxPoolAction{
			Nullifier:  decodedV6.Nullifiers[i],
			Commitment: decodedV6.Commitments[i],
			Value:      0,
		}
		bundle.Actions = append(bundle.Actions, action)
		bundle.Nullifiers = append(bundle.Nullifiers, action.Nullifier)
		bundle.Commitments = append(bundle.Commitments, action.Commitment)
	}

	return bundle, nil
}

func (p *OrchardTxPool) rederiveBundleCommitments(bundle *ParsedBundle) ([][32]byte, error) {
	if len(bundle.RawData) == 0 {
		return nil, nil
	}

	decoder := p.commitmentDecoder
	if decoder == nil {
		if p.orchardFFI == nil {
			return nil, fmt.Errorf("orchard FFI not initialized")
		}
		decoder = p.orchardFFI
	}

	switch bundle.BundleType {
	case OrchardBundleVanilla:
		decoded, err := decoder.DecodeBundle(bundle.RawData)
		if err != nil {
			return nil, fmt.Errorf("bundle decode failed: %w", err)
		}
		return decoded.Commitments, nil
	case OrchardBundleZSA, OrchardBundleSwap:
		decoded, err := decoder.DecodeBundleV6(bundle.RawData, bundle.IssueBundle)
		if err != nil {
			return nil, fmt.Errorf("bundle v6 decode failed: %w", err)
		}
		return decoded.Commitments, nil
	default:
		return nil, fmt.Errorf("unknown bundle type: %v", bundle.BundleType)
	}
}

func mapV6BundleType(value uint8) (OrchardBundleType, error) {
	switch OrchardBundleType(value) {
	case OrchardBundleVanilla, OrchardBundleZSA, OrchardBundleSwap:
		return OrchardBundleType(value), nil
	default:
		return OrchardBundleVanilla, fmt.Errorf("unknown bundle type: %d", value)
	}
}

// validateBundle checks if a bundle is valid for inclusion in the pool
func (p *OrchardTxPool) validateBundle(bundle *ParsedBundle) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Basic structural validation
	if len(bundle.RawData) == 0 && len(bundle.IssueBundle) == 0 {
		return fmt.Errorf("empty bundle data")
	}

	// Validate action count
	if len(bundle.Actions) == 0 {
		if len(bundle.IssueBundle) == 0 {
			return fmt.Errorf("bundle has no actions")
		}
	}
	if len(bundle.Actions) > MaxActionsPerBundle {
		return fmt.Errorf("bundle has too many actions: %d > %d", len(bundle.Actions), MaxActionsPerBundle)
	}

	// Validate nullifier/commitment structure consistency
	if len(bundle.Actions) > 0 && len(bundle.Nullifiers) != len(bundle.Actions) {
		return fmt.Errorf("nullifiers count (%d) doesn't match actions count (%d)", len(bundle.Nullifiers), len(bundle.Actions))
	}

	// Check for double-spend (nullifier reuse)
	for i, nullifier := range bundle.Nullifiers {
		if nullifier == [32]byte{} {
			continue // Skip zero nullifiers (outputs)
		}
		if p.nullifiers[nullifier] {
			return fmt.Errorf("double-spend detected: nullifier %x already spent", nullifier[:8])
		}
		// Check against rollup state as well
		if p.rollup.IsNullifierSpent(common.Hash(nullifier)) {
			return fmt.Errorf("nullifier %x already spent in rollup", nullifier[:8])
		}
		// Check for duplicate nullifiers within the same bundle
		for j := i + 1; j < len(bundle.Nullifiers); j++ {
			if nullifier == bundle.Nullifiers[j] {
				return fmt.Errorf("duplicate nullifier in bundle: %x", nullifier[:8])
			}
		}
	}

	// Validate gas estimate is reasonable
	if bundle.GasEstimate == 0 {
		return fmt.Errorf("bundle has zero gas estimate")
	}
	if bundle.GasEstimate > 10000000 { // 10M gas cap
		return fmt.Errorf("bundle gas estimate too high: %d", bundle.GasEstimate)
	}

	if bundle.BundleType == OrchardBundleVanilla {
		if len(bundle.ProofBytes) == 0 || len(bundle.PublicInputs) == 0 {
			return fmt.Errorf("bundle missing proof or public inputs")
		}
	}

	// TODO: Add cryptographic validation with FFI
	// - Verify bundle proof using FFI
	// - Validate note commitments structure
	// - Check action proofs against Orchard rules
	// - Verify binding signature
	log.Debug(log.Node, "Bundle validation passed", "bundle_id", bundle.ID)

	return nil
}

// GetPoolStats returns current pool statistics
func (p *OrchardTxPool) GetPoolStats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := p.stats
	stats.PendingBundles = uint64(len(p.pendingBundles))
	stats.TreeSize = p.tree.GetSize()
	stats.NullifierCount = uint64(len(p.nullifiers))

	return stats
}

// GetPendingBundles returns all bundles currently in the pool
func (p *OrchardTxPool) GetPendingBundles() []*ParsedBundle {
	p.mu.RLock()
	defer p.mu.RUnlock()

	bundles := make([]*ParsedBundle, 0, len(p.pendingBundles))
	for _, bundle := range p.pendingBundles {
		bundles = append(bundles, bundle)
	}

	return bundles
}

// GetTree returns access to the underlying Sinsemilla tree for frontier operations
func (p *OrchardTxPool) GetTree() *orchardffi.SinsemillaMerkleTree {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tree
}

// CurrentAnchor returns the latest commitment root for bundle anchors.
func (p *OrchardTxPool) CurrentAnchor() [32]byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.tree == nil {
		return [32]byte{}
	}
	return p.tree.GetRoot()
}

// GenerateBundle builds a deterministic Orchard bundle anchored to the current root.
func (p *OrchardTxPool) GenerateBundle(seed []byte, anchor [32]byte) ([]byte, error) {
	if p == nil || p.orchardFFI == nil {
		return nil, fmt.Errorf("orchard FFI unavailable")
	}
	return p.orchardFFI.GenerateBundle(seed, anchor)
}

// RemoveBundles removes processed bundles from the transaction pool
func (p *OrchardTxPool) RemoveBundles(bundleIDs []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	removed := 0
	for _, bundleID := range bundleIDs {
		if bundle, exists := p.pendingBundles[bundleID]; exists {
			delete(p.pendingBundles, bundleID)

			// Remove from time-ordered list
			newBundlesByTime := make([]*ParsedBundle, 0, len(p.bundlesByTime))
			for _, b := range p.bundlesByTime {
				if b.ID != bundleID {
					newBundlesByTime = append(newBundlesByTime, b)
				}
			}
			p.bundlesByTime = newBundlesByTime

			// Remove nullifiers (they're now spent)
			for _, nullifier := range bundle.Nullifiers {
				delete(p.nullifiers, nullifier)
			}

			// Clean up tracking data
			delete(p.bundleNullifiers, bundleID)
			delete(p.treeCheckpoints, bundleID)

			removed++
		}
	}

	// Update statistics
	p.stats.PendingBundles = uint64(len(p.pendingBundles))
	p.stats.NullifierCount = uint64(len(p.nullifiers))

	if removed > 0 {
		log.Info(log.Node, "Removed processed bundles from pool",
			"removed", removed,
			"remaining", len(p.pendingBundles))
	}
}

// rollbackBundle reverts changes made by a bundle that failed processing
func (p *OrchardTxPool) rollbackBundle(bundleID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Remove bundle from pool
	if _, exists := p.pendingBundles[bundleID]; exists {
		if checkpoint, ok := p.treeCheckpoints[bundleID]; ok && p.tree != nil {
			p.tree.RestoreState(checkpoint.Root, checkpoint.Size, checkpoint.Frontier)
			p.stats.TreeSize = p.tree.GetSize()
			log.Warn(log.Node, "Restored commitment tree checkpoint", "bundle_id", bundleID, "tree_size", p.stats.TreeSize)
		}
		delete(p.pendingBundles, bundleID)

		// Remove from time-ordered list
		newBundlesByTime := make([]*ParsedBundle, 0, len(p.bundlesByTime))
		for _, b := range p.bundlesByTime {
			if b.ID != bundleID {
				newBundlesByTime = append(newBundlesByTime, b)
			}
		}
		p.bundlesByTime = newBundlesByTime

		// Rollback nullifiers if we have tracking data
		if trackedNullifiers, exists := p.bundleNullifiers[bundleID]; exists {
			for _, nullifier := range trackedNullifiers {
				delete(p.nullifiers, nullifier)
			}
		}

		// Clean up tracking data
		delete(p.bundleNullifiers, bundleID)
		delete(p.treeCheckpoints, bundleID)

		// Update statistics
		p.stats.PendingBundles = uint64(len(p.pendingBundles))
		p.stats.NullifierCount = uint64(len(p.nullifiers))

		log.Info(log.Node, "Rolled back failed bundle",
			"bundle_id", bundleID,
			"remaining", len(p.pendingBundles))
	}
}

// bundleProcessor processes bundles from the queue
func (p *OrchardTxPool) bundleProcessor() {
	for bundle := range p.bundleQueue {
		if err := p.processBundleAsync(bundle); err != nil {
			log.Error(log.Node, "Failed to process bundle, rolling back", "bundle_id", bundle.ID, "error", err)
			p.rollbackBundle(bundle.ID)
		}
	}
}

// processBundleAsync handles background processing of a bundle
func (p *OrchardTxPool) processBundleAsync(bundle *ParsedBundle) error {
	if p.rollup == nil {
		return fmt.Errorf("orchard rollup not initialized")
	}

	expectedCommitments, err := p.rederiveBundleCommitments(bundle)
	if err != nil {
		return fmt.Errorf("commitment re-derivation failed for bundle %s: %w", bundle.ID, err)
	}
	if len(expectedCommitments) != len(bundle.Commitments) {
		return fmt.Errorf("commitment count mismatch for bundle %s: expected %d, got %d",
			bundle.ID, len(expectedCommitments), len(bundle.Commitments))
	}
	for i := range expectedCommitments {
		if expectedCommitments[i] != bundle.Commitments[i] {
			return fmt.Errorf("commitment mismatch for bundle %s at index %d", bundle.ID, i)
		}
	}

	preCommitmentRoot, preCommitmentSize, preFrontier, err := p.rollup.CommitmentSnapshot()
	if err != nil {
		return fmt.Errorf("commitment snapshot failed: %w", err)
	}
	preNullifierRoot, preNullifierSize, err := p.rollup.NullifierSnapshot()
	if err != nil {
		return fmt.Errorf("nullifier snapshot failed: %w", err)
	}

	nullifierProofs := make(map[[32]byte]NullifierAbsenceProof, len(bundle.Nullifiers))
	spentCommitmentProofs := make([]SpentCommitmentProof, 0, len(bundle.Nullifiers))
	for _, nullifier := range bundle.Nullifiers {
		absenceProof, err := p.rollup.NullifierAbsenceProof(nullifier)
		if err != nil {
			return fmt.Errorf("nullifier absence proof failed: %w", err)
		}
		nullifierProofs[nullifier] = NullifierAbsenceProof{
			Leaf:     absenceProof.Leaf,
			Siblings: absenceProof.Siblings,
			Root:     absenceProof.Root,
			Position: absenceProof.Position,
		}

		commitment, ok := p.rollup.CommitmentForNullifier(nullifier)
		if !ok {
			return fmt.Errorf("missing commitment for nullifier %x", nullifier[:4])
		}
		position, proof, err := p.rollup.CommitmentProof(commitment)
		if err != nil {
			return fmt.Errorf("commitment proof failed: %w", err)
		}
		spentCommitmentProofs = append(spentCommitmentProofs, SpentCommitmentProof{
			Nullifier:      nullifier,
			Commitment:     commitment,
			TreePosition:   position,
			BranchSiblings: proof,
		})
	}

	p.mu.Lock()
	bundle.PreState = &OrchardStateRoots{
		CommitmentRoot:     preCommitmentRoot,
		CommitmentSize:     preCommitmentSize,
		CommitmentFrontier: preFrontier,
		NullifierRoot:      preNullifierRoot,
		NullifierSize:      preNullifierSize,
	}
	bundle.NullifierProofs = nullifierProofs
	bundle.SpentCommitmentProofs = spentCommitmentProofs
	p.mu.Unlock()

	commitments := make([][32]byte, 0, len(expectedCommitments))
	commitments = append(commitments, expectedCommitments...)
	if len(bundle.IssueBundle) > 0 {
		issueCommitments, err := p.orchardFFI.IssueBundleCommitments(bundle.IssueBundle)
		if err != nil {
			return fmt.Errorf("failed to decode issue bundle commitments: %w", err)
		}
		commitments = append(commitments, issueCommitments...)
	}

	// Update commitment tree with new commitments
	if len(commitments) > 0 {
		if err := p.tree.AppendWithFrontier(commitments); err != nil {
			return fmt.Errorf("failed to update commitment tree: %w", err)
		}

		treeRoot := p.tree.GetRoot()
		log.Debug(log.Node, "Updated commitment tree",
			"bundle_id", bundle.ID,
			"new_commitments", len(commitments),
			"tree_size", p.tree.GetSize(),
			"tree_root", hex.EncodeToString(treeRoot[:]))
	}

	if p.rollup != nil {
		if err := p.rollup.ApplyBundleState(commitments, bundle.Nullifiers); err != nil {
			return fmt.Errorf("failed to persist bundle state: %w", err)
		}
	}

	return nil
}

// workPackageGenerator monitors the pool and generates work packages when threshold is reached
func (p *OrchardTxPool) workPackageGenerator() {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()

	for range ticker.C {
		p.maybeGenerateWorkPackage()
	}
}

// maybeGenerateWorkPackage checks if conditions are met to generate a work package
func (p *OrchardTxPool) maybeGenerateWorkPackage() {
	p.mu.RLock()
	bundleCount := len(p.pendingBundles)
	p.mu.RUnlock()

	// Check if we have enough bundles to justify a work package
	if bundleCount >= p.workPackageThreshold {
		// Check if enough time has passed since last work package
		if time.Since(p.lastWorkPackageTime) >= time.Minute {
			if err := p.generateWorkPackage(); err != nil {
				log.Error(log.Node, "Failed to generate work package", "error", err)
			}
		}
	}
}

// generateWorkPackage creates and submits a work package from pending bundles
func (p *OrchardTxPool) generateWorkPackage() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.pendingBundles) < p.workPackageThreshold {
		return fmt.Errorf("insufficient bundles for work package: %d < %d", len(p.pendingBundles), p.workPackageThreshold)
	}
	if p.rollup == nil || p.rollup.node == nil {
		return fmt.Errorf("orchard rollup not initialized")
	}

	// Select bundles for inclusion (simple FIFO for now)
	selectedBundles := make([]*ParsedBundle, 0, p.workPackageThreshold)
	for _, bundle := range p.bundlesByTime {
		if len(selectedBundles) >= p.workPackageThreshold {
			break
		}
		selectedBundles = append(selectedBundles, bundle)
	}

	refineContext, err := p.rollup.node.GetRefineContext()
	if err != nil {
		return fmt.Errorf("failed to fetch refine context: %w", err)
	}

	var preStateRoot [32]byte
	copy(preStateRoot[:], refineContext.StateRoot[:])

	var orchardStateValue []byte
	var orchardStateProof [][32]byte
	if stateDB := p.rollup.node.GetStateDB(); stateDB != nil {
		stateValue, stateProof, err := fetchOrchardStateWitness(stateDB, p.rollup.serviceID)
		if err != nil {
			log.Warn(log.Node, "Failed to fetch orchard_state witness", "err", err)
		} else {
			orchardStateValue = stateValue
			orchardStateProof = stateProof
		}
	}

	preTransparentMerkleRoot := [32]byte{}
	if p.rollup.transparentTxStore != nil {
		root, err := p.rollup.transparentTxStore.GetMerkleRoot(refineContext.LookupAnchorSlot)
		if err != nil {
			log.Warn(log.Node, "Failed to read transparent pre-state merkle root",
				"height", refineContext.LookupAnchorSlot,
				"err", err)
		} else {
			preTransparentMerkleRoot = root
		}
	}

	_, transparentExtrinsic, transparentMerkleRoot, transparentUtxoRoot, transparentUtxoSize, transparentPostRoot, transparentPostSize, err := p.rollup.buildTransparentTxDataExtrinsic(refineContext.LookupAnchorSlot)
	if err != nil {
		return fmt.Errorf("failed to build transparent tx extrinsic: %w", err)
	}

	codeHash := common.Hash{}
	if stateDB := p.rollup.node.GetStateDB(); stateDB != nil {
		service, ok, err := stateDB.GetService(p.rollup.serviceID)
		if err != nil || !ok {
			log.Warn(log.Node, "Could not get service info, using empty code hash", "service_id", p.rollup.serviceID)
		} else if service != nil {
			codeHash = service.CodeHash
		}
	} else {
		log.Warn(log.Node, "StateDB unavailable; using empty code hash", "service_id", p.rollup.serviceID)
	}

	submitted := make([]string, 0, len(selectedBundles))
	for _, bundle := range selectedBundles {
		if bundle.PreState == nil {
			return fmt.Errorf("missing pre-state snapshot for bundle %s", bundle.ID)
		}
		if len(bundle.Nullifiers) > 0 && len(bundle.NullifierProofs) == 0 {
			return fmt.Errorf("missing nullifier proofs for bundle %s", bundle.ID)
		}
		expectedCommitments, err := p.rederiveBundleCommitments(bundle)
		if err != nil {
			return fmt.Errorf("commitment re-derivation failed for bundle %s: %w", bundle.ID, err)
		}
		if len(expectedCommitments) != len(bundle.Commitments) {
			return fmt.Errorf("commitment count mismatch for bundle %s: expected %d, got %d",
				bundle.ID, len(expectedCommitments), len(bundle.Commitments))
		}
		for i := range expectedCommitments {
			if expectedCommitments[i] != bundle.Commitments[i] {
				return fmt.Errorf("commitment mismatch for bundle %s at index %d", bundle.ID, i)
			}
		}

		preWitness, err := SerializePreStateWitness(
			bundle.PreState,
			bundle.Nullifiers,
			bundle.NullifierProofs,
			preStateRoot,
			orchardStateValue,
			orchardStateProof,
		)
		if err != nil {
			return fmt.Errorf("pre-state witness failed: %w", err)
		}

		commitments := make([][32]byte, 0, len(expectedCommitments))
		commitments = append(commitments, expectedCommitments...)
		if len(bundle.IssueBundle) > 0 {
			issueCommitments, err := p.orchardFFI.IssueBundleCommitments(bundle.IssueBundle)
			if err != nil {
				return fmt.Errorf("issue bundle commitments failed: %w", err)
			}
			commitments = append(commitments, issueCommitments...)
		}

		postState, err := computePostState(bundle.PreState, commitments, len(bundle.Nullifiers))
		if err != nil {
			return fmt.Errorf("post-state compute failed: %w", err)
		}
		postWitness, err := SerializePostStateWitness(postState)
		if err != nil {
			return fmt.Errorf("post-state witness failed: %w", err)
		}

		var bundleProofExtrinsic []byte
		if bundle.BundleType == OrchardBundleVanilla {
			if len(bundle.ProofBytes) == 0 || len(bundle.PublicInputs) == 0 {
				return fmt.Errorf("missing proof data for vanilla bundle %s", bundle.ID)
			}
			bundleProofExtrinsic = SerializeBundleProof(1, bundle.PublicInputs, bundle.ProofBytes, bundle.RawData)
		} else {
			bundleProofExtrinsic = SerializeBundleProofV6(bundle.BundleType, bundle.RawData, bundle.IssueBundle)
		}

		wpFile := &WorkPackageFile{
			PreStateWitnessExtrinsic:  preWitness,
			PostStateWitnessExtrinsic: postWitness,
			BundleProofExtrinsic:      bundleProofExtrinsic,
			PreState:                  bundle.PreState,
			PostState:                 postState,
			SpentCommitmentProofs:     bundle.SpentCommitmentProofs,
			BundleType:                bundle.BundleType,
			OrchardBundleBytes:        bundle.RawData,
			IssueBundleBytes:          bundle.IssueBundle,
		}

		if len(transparentExtrinsic) > 0 {
			wpFile.TransparentTxDataExtrinsic = transparentExtrinsic
			if wpFile.PreState != nil {
				wpFile.PreState.TransparentMerkleRoot = preTransparentMerkleRoot
				wpFile.PreState.TransparentUtxoRoot = transparentUtxoRoot
				wpFile.PreState.TransparentUtxoSize = transparentUtxoSize
			}
			if wpFile.PostState != nil {
				wpFile.PostState.TransparentMerkleRoot = transparentMerkleRoot
				wpFile.PostState.TransparentUtxoRoot = transparentPostRoot
				wpFile.PostState.TransparentUtxoSize = transparentPostSize
			}
			postWitness, err := SerializePostStateWitness(wpFile.PostState)
			if err != nil {
				return fmt.Errorf("post-state witness update failed: %w", err)
			}
			wpFile.PostStateWitnessExtrinsic = postWitness
		}

		workPackageBundle, err := wpFile.ConvertToJAMWorkPackageBundle(p.rollup.serviceID, refineContext, codeHash)
		if err != nil {
			return fmt.Errorf("work package conversion failed: %w", err)
		}

		if _, _, err := p.rollup.node.SubmitAndWaitForWorkPackageBundle(context.Background(), workPackageBundle); err != nil {
			return fmt.Errorf("work package submit failed: %w", err)
		}

		submitted = append(submitted, bundle.ID)
	}

	for _, bundleID := range submitted {
		delete(p.pendingBundles, bundleID)
		delete(p.bundleNullifiers, bundleID)
		delete(p.treeCheckpoints, bundleID)
	}

	if len(submitted) > 0 && len(p.bundlesByTime) >= len(submitted) {
		p.bundlesByTime = p.bundlesByTime[len(submitted):]
	}

	treeRoot := p.tree.GetRoot()
	log.Info(log.Node, "Generated work package",
		"bundles_included", len(submitted),
		"tree_size", p.tree.GetSize(),
		"tree_root", hex.EncodeToString(treeRoot[:]))

	p.stats.WorkPackagesGenerated++
	p.stats.PendingBundles = uint64(len(p.pendingBundles))
	p.lastWorkPackageTime = time.Now()

	return nil
}

func computePostState(preState *OrchardStateRoots, commitments [][32]byte, nullifierCount int) (*OrchardStateRoots, error) {
	if preState == nil {
		return nil, fmt.Errorf("pre-state unavailable")
	}

	post := *preState
	post.NullifierSize = preState.NullifierSize + uint64(nullifierCount)

	if len(commitments) == 0 {
		return &post, nil
	}

	tree := orchardffi.NewSinsemillaMerkleTree()
	tree.RestoreState(preState.CommitmentRoot, preState.CommitmentSize, preState.CommitmentFrontier)
	if err := tree.AppendWithFrontier(commitments); err != nil {
		return nil, err
	}
	post.CommitmentRoot = tree.GetRoot()
	post.CommitmentSize = tree.GetSize()
	post.CommitmentFrontier = tree.GetFrontier()
	return &post, nil
}

func fetchOrchardStateWitness(stateDB *statedb.StateDB, serviceID uint32) ([]byte, [][32]byte, error) {
	if stateDB == nil {
		return nil, nil, fmt.Errorf("state DB unavailable")
	}
	key := common.Compute_storage_opaqueKey(serviceID, []byte("orchard_state"))
	if len(key) != 32 {
		return nil, nil, fmt.Errorf("orchard_state key length mismatch")
	}

	objectID := common.BytesToHash(key)
	witness, found, err := stateDB.ReadObject(serviceID, objectID)
	if err != nil {
		return nil, nil, err
	}
	if !found || witness == nil {
		return nil, nil, fmt.Errorf("orchard_state not found")
	}
	if len(witness.Value) == 0 {
		return nil, nil, fmt.Errorf("orchard_state witness missing value")
	}

	proof := make([][32]byte, len(witness.Path))
	for i, node := range witness.Path {
		copy(proof[i][:], node[:])
	}
	return append([]byte(nil), witness.Value...), proof, nil
}

// poolCleaner removes old bundles from the pool
func (p *OrchardTxPool) poolCleaner() {
	ticker := time.NewTicker(1 * time.Minute) // Clean every minute
	defer ticker.Stop()

	for range ticker.C {
		p.cleanExpiredBundles()
	}
}

// cleanExpiredBundles removes bundles that have been in the pool too long
func (p *OrchardTxPool) cleanExpiredBundles() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	removed := 0

	// Remove expired bundles with proper cleanup
	newBundlesByTime := make([]*ParsedBundle, 0, len(p.bundlesByTime))
	for _, bundle := range p.bundlesByTime {
		if now.Sub(bundle.Timestamp) > MaxBundleAge {
			// Clean up bundle with tracking data
			delete(p.pendingBundles, bundle.ID)

			// Rollback nullifiers if we have tracking data
			if trackedNullifiers, exists := p.bundleNullifiers[bundle.ID]; exists {
				for _, nullifier := range trackedNullifiers {
					delete(p.nullifiers, nullifier)
				}
			}

			// Clean up tracking data
			delete(p.bundleNullifiers, bundle.ID)
			delete(p.treeCheckpoints, bundle.ID)

			removed++
		} else {
			newBundlesByTime = append(newBundlesByTime, bundle)
		}
	}

	p.bundlesByTime = newBundlesByTime
	p.stats.PendingBundles = uint64(len(p.pendingBundles))

	if removed > 0 {
		log.Info(log.Node, "Cleaned expired bundles from pool", "removed", removed, "remaining", len(p.pendingBundles))
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
