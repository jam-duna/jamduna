package rpc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/colorfulnotion/jam/builder/orchard/ffi"
	orchardshielded "github.com/colorfulnotion/jam/builder/orchard/shielded"
	orchardstate "github.com/colorfulnotion/jam/builder/orchard/state"
	orchardtransparent "github.com/colorfulnotion/jam/builder/orchard/transparent"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
)

// OrchardRollup is a PoC rollup helper that mirrors the EVM Rollup shape but
// tracks simplified Orchard state in-memory. It is intentionally stateful so
// downstream work can wire it into real host calls.
// THREAD-SAFE: All state access is protected by RWMutex to prevent race conditions.
type OrchardRollup struct {
	node      statedb.StateProvider // Optional: provides access to node's StateDB for queries
	serviceID uint32

	// Protect all shared state with RWMutex for thread-safe concurrent access
	mu sync.RWMutex

	// Shielded (Orchard) state
	notes           map[common.Hash]OrchardNote
	nullifiers      map[common.Hash]bool
	commitmentTree  *ffi.SinsemillaMerkleTree
	commitmentOrder [][32]byte
	commitmentIndex map[[32]byte]uint64
	nullifierTree   *orchardstate.SparseNullifierTree
	transfers       []OrchardTransfer
	withdrawals     []OrchardWithdrawal
	events          []OrchardEvent

	// Transparent transaction state
	transparentTxStore  *TransparentTxStore
	transparentUtxoTree *TransparentUtxoTree
	transparentStore    *orchardtransparent.TransparentStore

	// Persistent shielded state
	shieldedStore *orchardshielded.ShieldedStore
}

// OrchardNote represents a simplified shielded note tracked inside the rollup.
// It is intentionally minimal and mirrors the fields described in services/orchard/docs/RAILGUN.md.
type OrchardNote struct {
	Commitment  common.Hash
	Nullifier   common.Hash
	Value       uint64
	OwnerPubKey [32]byte
	Rho         [32]byte
	Source      common.Address
}

// OrchardTransfer captures an in-rollup transfer between notes along with a builder fee.
type OrchardTransfer struct {
	From       common.Hash
	To         common.Hash
	BuilderFee uint64
	Block      uint32
}

// OrchardWithdrawal records a public exit of a note back to a recipient address.
type OrchardWithdrawal struct {
	Nullifier common.Hash
	To        common.Address
	Value     uint64
	Block     uint32
}

// OrchardEvent is a lightweight event log to keep the PoC debuggable.
type OrchardEvent struct {
	Kind       string
	Commitment common.Hash
	Nullifier  common.Hash
	Value      uint64
	Block      uint32
	Details    string
}

// OrchardBlockMetadata provides minimal block context for RPC responses.
type OrchardBlockMetadata struct {
	Height     uint32
	Hash       common.Hash
	ParentHash common.Hash
	Timestamp  uint64
	Fallback   bool // true when hash/timestamp are derived rather than read from storage
}

// TreeState captures commitment tree root and size for Orchard-style RPCs.
type TreeState struct {
	Root common.Hash
	Size uint64
}

// BlockCommitment represents the complete commitment for a rollup block,
// including both shielded (Orchard) and transparent state roots.
type BlockCommitment struct {
	// Shielded state commitments
	OrchardCommitmentRoot [32]byte // Merkle root of Orchard note commitments
	OrchardNullifierRoot  [32]byte // Merkle root of spent nullifiers

	// Transparent state commitments
	TransparentMerkleRoot [32]byte // Bitcoin/Zcash-style Merkle root of transparent txids
	TransparentUtxoRoot   [32]byte // Merkle root of UTXO set
	TransparentUtxoSize   uint64   // Number of UTXOs in set
}

// NewOrchardRollup creates a new PoC rollup bound to the provided RollupNode.
// FIXED: Service ID is now configurable to prevent cross-service state corruption
func NewOrchardRollup(node statedb.StateProvider, serviceID uint32) *OrchardRollup {
	rollup := &OrchardRollup{
		node:                node,
		serviceID:           serviceID, // ✅ Use provided serviceID instead of hardcoded constant
		notes:               make(map[common.Hash]OrchardNote),
		nullifiers:          make(map[common.Hash]bool),
		commitmentTree:      ffi.NewSinsemillaMerkleTree(),
		commitmentOrder:     make([][32]byte, 0),
		commitmentIndex:     make(map[[32]byte]uint64),
		nullifierTree:       orchardstate.NewSparseNullifierTree(32),
		transfers:           make([]OrchardTransfer, 0),
		withdrawals:         make([]OrchardWithdrawal, 0),
		events:              make([]OrchardEvent, 0),
		transparentTxStore:  createDefaultTransparentTxStore(),
		transparentUtxoTree: NewTransparentUtxoTree(),
		transparentStore:    createDefaultTransparentStore(),
		shieldedStore:       createDefaultShieldedStore(),
	}

	if err := rollup.loadShieldedStateFromStore(); err != nil {
		log.Warn(log.Node, "Failed to load shielded state from store", "err", err)
	}
	if err := rollup.loadTransparentStateFromStore(); err != nil {
		log.Warn(log.Node, "Failed to load transparent state from store", "err", err)
	}
	rollup.attachTransparentUtxoState()

	return rollup
}

// Close releases resources owned by the rollup.
func (r *OrchardRollup) Close() error {
	r.mu.RLock()
	store := r.transparentTxStore
	transparentStore := r.transparentStore
	shieldedStore := r.shieldedStore
	r.mu.RUnlock()

	if store != nil {
		_ = store.Close()
	}
	if transparentStore != nil {
		_ = transparentStore.Close()
	}
	if shieldedStore != nil {
		_ = shieldedStore.Close()
	}
	return nil
}

// createDefaultTransparentTxStore creates a transparent tx store with default data directory
func createDefaultTransparentTxStore() *TransparentTxStore {
	dataDir := "./data"
	store, err := NewTransparentTxStore(dataDir)
	if err != nil {
		log.Error(log.Node, "Failed to create transparent tx store", "dataDir", dataDir, "error", err)
		// Return empty store for backwards compatibility
		return &TransparentTxStore{
			transactions: make(map[string]*TransparentTransaction),
			mempool:      make(map[string]*TransparentTransaction),
			utxos:        make(map[string][]UTXO),
			blockTxs:     make(map[uint32][]string),
			txIndex:      make(map[string]uint32),
		}
	}
	return store
}

func createDefaultTransparentStore() *orchardtransparent.TransparentStore {
	dataDir := filepath.Join("./data", "transparent_store")
	store, err := orchardtransparent.NewTransparentStore(dataDir)
	if err != nil {
		log.Error(log.Node, "Failed to create transparent UTXO store", "path", dataDir, "error", err)
		return nil
	}
	return store
}

func createDefaultShieldedStore() *orchardshielded.ShieldedStore {
	dataDir := filepath.Join("./data", "shielded_store")
	store, err := orchardshielded.NewShieldedStore(dataDir)
	if err != nil {
		log.Error(log.Node, "Failed to create shielded store", "path", dataDir, "error", err)
		return nil
	}
	return store
}

func (r *OrchardRollup) loadShieldedStateFromStore() error {
	if r.shieldedStore == nil {
		return nil
	}

	entries, err := r.shieldedStore.ListCommitments()
	if err != nil {
		return err
	}
	commitments := make([][32]byte, len(entries))
	commitmentIndex := make(map[[32]byte]uint64, len(entries))
	for i, entry := range entries {
		commitments[i] = entry.Commitment
		commitmentIndex[entry.Commitment] = uint64(i)
	}

	tree := ffi.NewSinsemillaMerkleTree()
	if err := tree.ComputeRoot(commitments); err != nil {
		return fmt.Errorf("compute commitment root: %w", err)
	}

	nullifiers, err := r.shieldedStore.ListNullifiers()
	if err != nil {
		return err
	}
	nullifierTree := orchardstate.NewSparseNullifierTree(32)
	nullifierMap := make(map[common.Hash]bool, len(nullifiers))
	for _, nullifier := range nullifiers {
		nullifierTree.Insert(nullifier)
		nullifierMap[common.BytesToHash(nullifier[:])] = true
	}

	r.mu.Lock()
	r.commitmentTree = tree
	r.commitmentOrder = commitments
	r.commitmentIndex = commitmentIndex
	r.nullifierTree = nullifierTree
	r.nullifiers = nullifierMap
	r.mu.Unlock()

	return nil
}

func (r *OrchardRollup) loadTransparentStateFromStore() error {
	if r.transparentStore == nil {
		return nil
	}
	utxos, err := r.transparentStore.GetAllUTXOs()
	if err != nil {
		return err
	}

	tree := NewTransparentUtxoTree()
	outpoints := make([]orchardtransparent.OutPoint, 0, len(utxos))
	for outpoint := range utxos {
		outpoints = append(outpoints, outpoint)
	}
	sort.Slice(outpoints, func(i, j int) bool {
		if cmp := bytes.Compare(outpoints[i].TxID[:], outpoints[j].TxID[:]); cmp != 0 {
			return cmp < 0
		}
		return outpoints[i].Index < outpoints[j].Index
	})
	for _, outpoint := range outpoints {
		utxo := utxos[outpoint]
		tree.Insert(
			Outpoint{Txid: outpoint.TxID, Vout: outpoint.Index},
			UtxoData{
				Value:        utxo.Value,
				ScriptPubKey: utxo.ScriptPubKey,
				Height:       utxo.Height,
				IsCoinbase:   utxo.IsCoinbase,
			},
		)
	}

	r.mu.Lock()
	r.transparentUtxoTree = tree
	r.mu.Unlock()
	r.attachTransparentUtxoState()
	return nil
}

// syncTransparentUtxoTreeFromStore rebuilds the in-memory tree from the persistent store.
func (r *OrchardRollup) syncTransparentUtxoTreeFromStore() error {
	return r.loadTransparentStateFromStore()
}

func (r *OrchardRollup) attachTransparentUtxoState() {
	if r == nil || r.transparentTxStore == nil {
		return
	}
	r.transparentTxStore.AttachUtxoStore(r.transparentStore, r.transparentUtxoTree)
}

// deriveOrchardCommitment produces a deterministic commitment for this PoC.
func deriveOrchardCommitment(value uint64, pk [32]byte, rho [32]byte) common.Hash {
	bytes := make([]byte, 0, 8+32+32)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], value)
	bytes = append(bytes, buf[:]...)
	bytes = append(bytes, pk[:]...)
	bytes = append(bytes, rho[:]...)
	return common.Blake2Hash(bytes)
}

// deriveOrchardNullifier keeps the service ID in scope to avoid collisions.
func deriveOrchardNullifier(serviceID uint32, pk [32]byte, rho [32]byte) common.Hash {
	bytes := make([]byte, 0, 4+32+32)
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], serviceID)
	bytes = append(bytes, buf[:]...)
	bytes = append(bytes, pk[:]...)
	bytes = append(bytes, rho[:]...)
	return common.Blake2Hash(bytes)
}

func (r *OrchardRollup) appendCommitmentsLocked(commitments [][32]byte) error {
	if len(commitments) == 0 {
		return nil
	}
	if r.commitmentTree == nil {
		return fmt.Errorf("commitment tree unavailable")
	}

	snapshotRoot := r.commitmentTree.GetRoot()
	snapshotSize := r.commitmentTree.GetSize()
	frontier := r.commitmentTree.GetFrontier()
	snapshotFrontier := make([][32]byte, len(frontier))
	copy(snapshotFrontier, frontier)

	for _, commitment := range commitments {
		if _, exists := r.commitmentIndex[commitment]; exists {
			return fmt.Errorf("commitment already exists: %x", commitment)
		}
	}

	if err := r.commitmentTree.AppendWithFrontier(commitments); err != nil {
		return fmt.Errorf("append commitments failed: %w", err)
	}

	start := len(r.commitmentOrder)
	for _, commitment := range commitments {
		r.commitmentIndex[commitment] = uint64(len(r.commitmentOrder))
		r.commitmentOrder = append(r.commitmentOrder, commitment)
	}

	if r.shieldedStore != nil {
		for i, commitment := range commitments {
			position := uint64(start + i)
			if err := r.shieldedStore.AddCommitment(position, commitment); err != nil {
				for j := 0; j < i; j++ {
					if err := r.shieldedStore.DeleteCommitment(uint64(start + j)); err != nil {
						log.Warn(log.Node, "Rollback commitment persist failed", "position", start+j, "err", err)
					}
				}
				for _, inserted := range commitments {
					delete(r.commitmentIndex, inserted)
				}
				if start < len(r.commitmentOrder) {
					r.commitmentOrder = r.commitmentOrder[:start]
				}
				r.commitmentTree.RestoreState(snapshotRoot, snapshotSize, snapshotFrontier)
				return fmt.Errorf("persist commitment: %w", err)
			}
		}
	}

	return nil
}

func (r *OrchardRollup) insertNullifiersLocked(nullifiers [][32]byte) {
	if len(nullifiers) == 0 || r.nullifierTree == nil {
		return
	}

	for _, nullifier := range nullifiers {
		if nullifier == ([32]byte{}) {
			continue
		}
		r.nullifierTree.Insert(nullifier)
		r.nullifiers[common.BytesToHash(nullifier[:])] = true
		if r.shieldedStore != nil {
			if err := r.shieldedStore.AddNullifier(nullifier, 0); err != nil {
				log.Warn(log.Node, "Persist nullifier failed", "err", err)
			}
		}
	}
}

// ApplyBundleState updates shielded state for a single bundle (commitments + nullifiers).
func (r *OrchardRollup) ApplyBundleState(commitments [][32]byte, nullifiers [][32]byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.appendCommitmentsLocked(commitments); err != nil {
		return err
	}
	r.insertNullifiersLocked(nullifiers)
	return nil
}

// currentBlockNumber mirrors the EVM rollup helper and tolerates missing state
// so the PoC can run without full block production.
func (r *OrchardRollup) currentBlockNumber() uint32 {
	if r == nil || r.node == nil {
		return 0
	}
	ctx, err := r.node.GetRefineContext()
	if err != nil {
		log.Warn(log.Node, "Orchard rollup: failed to fetch refine context", "err", err)
		return 0
	}
	return ctx.LookupAnchorSlot
}

// recordEvent appends an event with the latest block context.
// THREAD-SAFE: Uses write lock to prevent concurrent access.
func (r *OrchardRollup) recordEvent(kind string, commitment common.Hash, nullifier common.Hash, value uint64, details string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recordEventUnsafe(kind, commitment, nullifier, value, details)
}

// recordEventUnsafe appends an event without acquiring locks (for internal use when lock is already held).
func (r *OrchardRollup) recordEventUnsafe(kind string, commitment common.Hash, nullifier common.Hash, value uint64, details string) {
	r.events = append(r.events, OrchardEvent{
		Kind:       kind,
		Commitment: commitment,
		Nullifier:  nullifier,
		Value:      value,
		Block:      r.currentBlockNumber(),
		Details:    details,
	})
}

// Deposit binds a value and recipient public key into a new Orchard note.
// Uses PVM integration when available, falls back to in-memory PoC.
// THREAD-SAFE: Uses write lock to prevent concurrent access.
func (r *OrchardRollup) Deposit(sender common.Address, value uint64, pk [32]byte, rho [32]byte) (OrchardNote, error) {
	commitment := deriveOrchardCommitment(value, pk, rho)
	nullifier := deriveOrchardNullifier(r.serviceID, pk, rho)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.notes[commitment]; exists {
		return OrchardNote{}, fmt.Errorf("commitment already exists: %s", commitment.String())
	}
	if r.nullifiers[nullifier] {
		return OrchardNote{}, fmt.Errorf("nullifier already spent: %s", nullifier.String())
	}

	var commitmentBytes [32]byte
	copy(commitmentBytes[:], commitment[:])
	if err := r.appendCommitmentsLocked([][32]byte{commitmentBytes}); err != nil {
		return OrchardNote{}, err
	}

	note := OrchardNote{
		Commitment:  commitment,
		Nullifier:   nullifier,
		Value:       value,
		OwnerPubKey: pk,
		Rho:         rho,
		Source:      sender,
	}

	r.notes[commitment] = note
	r.recordEventUnsafe("deposit", commitment, nullifier, value, fmt.Sprintf("from %s", sender.Hex()))
	return note, nil
}

// DepositViaPVM executes a deposit through the Orchard PVM service (refine + accumulate)
// This is the production path that replaces in-memory note tracking
// TODO: Reimplement without circular dependency between rpc and witness packages
/*
func (r *OrchardRollup) DepositViaPVM(sender common.Address, value uint64, pk [32]byte, rho [32]byte) (OrchardNote, error) {
	commitment := deriveOrchardCommitment(value, pk, rho)

	// Build DepositPublic extrinsic matching services/orchard/docs/RAILGUN.md spec
	ext := &witness.DepositPublicExtrinsic{
		Commitment:  commitment,
		Value:       value,
		SenderIndex: 0, // Cross-service transfer from EVM (statedb.EVMServiceCode)
	}

	// Execute refine
	if err := witness.SubmitOrchardBlock(r, []witness.OrchardExtrinsic{ext}); err != nil {
		return OrchardNote{}, fmt.Errorf("refine failed: %w", err)
	}

	// Return note for test validation (production code would query state)
	nullifier := deriveOrchardNullifier(r.serviceID, pk, rho)
	note := OrchardNote{
		Commitment:  commitment,
		Nullifier:   nullifier,
		Value:       value,
		OwnerPubKey: pk,
		Rho:         rho,
		Source:      sender,
	}

	r.recordEvent("deposit_pvm", commitment, nullifier, value, fmt.Sprintf("from %s", sender.Hex()))
	return note, nil
}
*/

// Transfer spends an input note, accounts for a builder fee, and emits a new note.
// THREAD-SAFE: Uses write lock to prevent concurrent access.
func (r *OrchardRollup) Transfer(input OrchardNote, toPK [32]byte, toRho [32]byte, builderFee uint64) (OrchardNote, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	active, ok := r.notes[input.Commitment]
	if !ok {
		return OrchardNote{}, fmt.Errorf("unknown commitment: %s", input.Commitment.String())
	}

	if r.nullifiers[active.Nullifier] {
		return OrchardNote{}, fmt.Errorf("nullifier already spent: %s", active.Nullifier.String())
	}

	if builderFee > active.Value {
		return OrchardNote{}, fmt.Errorf("builder fee %d exceeds note value %d", builderFee, active.Value)
	}

	// Create new output note
	outputValue := active.Value - builderFee
	newCommitment := deriveOrchardCommitment(outputValue, toPK, toRho)
	newNullifier := deriveOrchardNullifier(r.serviceID, toPK, toRho)

	var newCommitmentBytes [32]byte
	copy(newCommitmentBytes[:], newCommitment[:])
	if err := r.appendCommitmentsLocked([][32]byte{newCommitmentBytes}); err != nil {
		return OrchardNote{}, err
	}

	var spentNullifierBytes [32]byte
	copy(spentNullifierBytes[:], active.Nullifier[:])
	r.insertNullifiersLocked([][32]byte{spentNullifierBytes})

	// Mark nullifier as spent and remove note
	delete(r.notes, active.Commitment)

	newNote := OrchardNote{
		Commitment:  newCommitment,
		Nullifier:   newNullifier,
		Value:       outputValue,
		OwnerPubKey: toPK,
		Rho:         toRho,
		Source:      active.Source,
	}

	r.notes[newCommitment] = newNote
	transfer := OrchardTransfer{
		From:       active.Commitment,
		To:         newCommitment,
		BuilderFee: builderFee,
		Block:      r.currentBlockNumber(),
	}
	r.transfers = append(r.transfers, transfer)
	r.recordEventUnsafe("transfer", newCommitment, active.Nullifier, outputValue, fmt.Sprintf("builder_fee=%d", builderFee))
	return newNote, nil
}

// Withdraw consumes a note and records a public exit for the current block.
// THREAD-SAFE: Uses write lock to prevent concurrent access.
func (r *OrchardRollup) Withdraw(note OrchardNote, recipient common.Address) (OrchardWithdrawal, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	active, ok := r.notes[note.Commitment]
	if !ok {
		return OrchardWithdrawal{}, fmt.Errorf("commitment not active: %s", note.Commitment.String())
	}

	if r.nullifiers[active.Nullifier] {
		return OrchardWithdrawal{}, fmt.Errorf("nullifier already spent: %s", active.Nullifier.String())
	}

	// Mark nullifier as spent and remove note
	var spentNullifierBytes [32]byte
	copy(spentNullifierBytes[:], active.Nullifier[:])
	r.insertNullifiersLocked([][32]byte{spentNullifierBytes})
	delete(r.notes, active.Commitment)

	// Record withdrawal
	withdrawal := OrchardWithdrawal{
		Nullifier: active.Nullifier,
		To:        recipient,
		Value:     active.Value,
		Block:     r.currentBlockNumber(),
	}
	r.withdrawals = append(r.withdrawals, withdrawal)
	r.recordEventUnsafe("withdraw", active.Commitment, active.Nullifier, active.Value, recipient.Hex())
	return withdrawal, nil
}

// ActiveNotes returns a copy of active notes to keep callers from mutating internal state.
// THREAD-SAFE: Uses read lock to prevent concurrent access.
func (r *OrchardRollup) ActiveNotes() []OrchardNote {
	r.mu.RLock()
	defer r.mu.RUnlock()

	notes := make([]OrchardNote, 0, len(r.notes))
	for _, commitment := range r.commitmentOrder {
		note, ok := r.notes[common.BytesToHash(commitment[:])]
		if ok {
			notes = append(notes, note)
		}
	}
	return notes
}

// CommitmentSnapshot returns the current commitment tree state.
func (r *OrchardRollup) CommitmentSnapshot() ([32]byte, uint64, [][32]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.commitmentTree == nil {
		return [32]byte{}, 0, nil, fmt.Errorf("commitment tree unavailable")
	}

	frontier := r.commitmentTree.GetFrontier()
	frontierCopy := make([][32]byte, len(frontier))
	copy(frontierCopy, frontier)

	return r.commitmentTree.GetRoot(), r.commitmentTree.GetSize(), frontierCopy, nil
}

// NullifierSnapshot returns the current nullifier tree state.
func (r *OrchardRollup) NullifierSnapshot() ([32]byte, uint64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.nullifierTree == nil {
		return [32]byte{}, 0, fmt.Errorf("nullifier tree unavailable")
	}

	return r.nullifierTree.Root(), uint64(r.nullifierTree.Len()), nil
}

// CommitmentList returns the ordered commitments in the builder state.
func (r *OrchardRollup) CommitmentList() [][32]byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([][32]byte, len(r.commitmentOrder))
	copy(out, r.commitmentOrder)
	return out
}

// CommitmentForNullifier returns the commitment associated with a nullifier if known.
func (r *OrchardRollup) CommitmentForNullifier(nullifier [32]byte) ([32]byte, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	target := common.BytesToHash(nullifier[:])
	for commitmentHash, note := range r.notes {
		if note.Nullifier == target {
			var commitment [32]byte
			copy(commitment[:], commitmentHash[:])
			return commitment, true
		}
	}

	return [32]byte{}, false
}

// CommitmentProof returns a membership proof for the given commitment.
func (r *OrchardRollup) CommitmentProof(commitment [32]byte) (uint64, [][32]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.commitmentTree == nil {
		return 0, nil, fmt.Errorf("commitment tree unavailable")
	}
	position, ok := r.commitmentIndex[commitment]
	if !ok {
		return 0, nil, fmt.Errorf("commitment not found")
	}

	proof, err := r.commitmentTree.GenerateProof(commitment, position, r.commitmentOrder)
	if err != nil {
		return 0, nil, err
	}

	return position, proof, nil
}

// NullifierAbsenceProof returns a sparse Merkle absence proof for a nullifier.
func (r *OrchardRollup) NullifierAbsenceProof(nullifier [32]byte) (orchardstate.SparseMerkleProof, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.nullifierTree == nil {
		return orchardstate.SparseMerkleProof{}, fmt.Errorf("nullifier tree unavailable")
	}
	if r.nullifierTree.Contains(nullifier) {
		return orchardstate.SparseMerkleProof{}, fmt.Errorf("nullifier already spent")
	}

	return r.nullifierTree.ProveAbsence(nullifier), nil
}

// SpentNullifiers returns the set of seen nullifiers.
// THREAD-SAFE: Uses read lock to prevent concurrent access.
func (r *OrchardRollup) SpentNullifiers() []common.Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nullifiers := make([]common.Hash, 0, len(r.nullifiers))
	for nullifier := range r.nullifiers {
		nullifiers = append(nullifiers, nullifier)
	}
	return nullifiers
}

// ApplyParsedBundleState updates commitment and nullifier trees from a bundle batch.
func (r *OrchardRollup) ApplyParsedBundleState(bundles []*ParsedBundle) error {
	if len(bundles) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var commitments [][32]byte
	var nullifiers [][32]byte
	for _, bundle := range bundles {
		for _, commitment := range bundle.Commitments {
			if commitment == ([32]byte{}) {
				continue
			}
			commitments = append(commitments, commitment)
		}
		for _, nullifier := range bundle.Nullifiers {
			if nullifier == ([32]byte{}) {
				continue
			}
			nullifiers = append(nullifiers, nullifier)
		}
	}

	if err := r.appendCommitmentsLocked(commitments); err != nil {
		return err
	}
	r.insertNullifiersLocked(nullifiers)

	return nil
}

// IsNullifierSpent checks if a nullifier has been spent.
// THREAD-SAFE: Uses read lock to prevent concurrent access.
func (r *OrchardRollup) IsNullifierSpent(nullifier common.Hash) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nullifiers[nullifier]
}

// History exposes the recorded events for debugging and documentation.
// THREAD-SAFE: Uses read lock to prevent concurrent access.
func (r *OrchardRollup) History() []OrchardEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	events := make([]OrchardEvent, len(r.events))
	copy(events, r.events)
	return events
}

// Withdrawals exposes completed withdrawals to downstream consumers.
// THREAD-SAFE: Uses read lock to prevent concurrent access.
func (r *OrchardRollup) Withdrawals() []OrchardWithdrawal {
	r.mu.RLock()
	defer r.mu.RUnlock()

	w := make([]OrchardWithdrawal, len(r.withdrawals))
	copy(w, r.withdrawals)
	return w
}

// Transfers exposes completed transfers to downstream consumers.
// THREAD-SAFE: Uses read lock to prevent concurrent access.
func (r *OrchardRollup) Transfers() []OrchardTransfer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t := make([]OrchardTransfer, len(r.transfers))
	copy(t, r.transfers)
	return t
}

// GetCommitmentTreeState returns commitment tree root/size from builder state.
func (r *OrchardRollup) GetCommitmentTreeState() (*TreeState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.commitmentTree == nil {
		return nil, fmt.Errorf("commitment tree unavailable")
	}

	root := r.commitmentTree.GetRoot()
	return &TreeState{
		Root: common.BytesToHash(root[:]),
		Size: r.commitmentTree.GetSize(),
	}, nil
}

// GetBlockCommitment returns the complete commitment for a rollup block,
// combining both shielded (Orchard) and transparent state roots.
// The height parameter specifies which block height to compute the commitment for.
func (r *OrchardRollup) GetBlockCommitment(height uint32) (BlockCommitment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	commitment := BlockCommitment{}

	// Get Orchard shielded state roots
	if r.commitmentTree != nil {
		orchardRoot := r.commitmentTree.GetRoot()
		copy(commitment.OrchardCommitmentRoot[:], orchardRoot[:])
	}

	if r.nullifierTree != nil {
		nullifierRoot := r.nullifierTree.Root()
		copy(commitment.OrchardNullifierRoot[:], nullifierRoot[:])
	}

	// Get transparent Merkle root for this block height
	if r.transparentTxStore != nil {
		transparentRoot, err := r.transparentTxStore.GetMerkleRoot(height)
		if err != nil {
			// Non-fatal: block may have no transparent transactions
			log.Info(log.Node, "No transparent transactions at height", "height", height, "err", err)
		} else {
			commitment.TransparentMerkleRoot = transparentRoot
		}
	}

	// Get transparent UTXO root
	if r.transparentUtxoTree != nil {
		utxoRoot, err := r.transparentUtxoTree.GetRoot()
		if err != nil {
			log.Warn(log.Node, "Failed to get UTXO root", "err", err)
		} else {
			commitment.TransparentUtxoRoot = utxoRoot
			commitment.TransparentUtxoSize = uint64(r.transparentUtxoTree.Size())
		}
	}

	return commitment, nil
}

// LatestBlockNumber returns the latest Orchard block number using the shared RollupNode helper.
func (r *OrchardRollup) LatestBlockNumber() (uint32, error) {
	if r == nil || r.node == nil {
		return 0, fmt.Errorf("orchard rollup not initialized")
	}
	return 0, fmt.Errorf("orchard rollup not initialized")
}

// GetOrchardBlockMetadata returns minimal metadata for a given Orchard block height.
// TODO: replace fallback hash/timestamp with real values read from Orchard service storage.
func (r *OrchardRollup) GetOrchardBlockMetadata(height uint32) (*OrchardBlockMetadata, error) {
	latest, err := r.LatestBlockNumber()
	if err != nil {
		return nil, fmt.Errorf("failed to read latest block: %w", err)
	}
	if height > latest {
		return nil, fmt.Errorf("block %d not available (latest %d)", height, latest)
	}

	// Fallback hash derived from height until Orchard block hashes are persisted.
	var hash common.Hash
	binary.LittleEndian.PutUint32(hash[:4], height)
	log.Warn(log.Node, "Orchard block hash not yet stored; using fallback", "height", height)

	return &OrchardBlockMetadata{
		Height:    height,
		Hash:      hash,
		Timestamp: 0,
		Fallback:  true,
	}, nil
}
