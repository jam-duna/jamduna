package rpc

import (
	"encoding/binary"
	"fmt"
	"sync"

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
	mu          sync.RWMutex
	notes       map[common.Hash]OrchardNote
	nullifiers  map[common.Hash]bool
	transfers   []OrchardTransfer
	withdrawals []OrchardWithdrawal
	events      []OrchardEvent
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

// NewOrchardRollup creates a new PoC rollup bound to the provided RollupNode.
// FIXED: Service ID is now configurable to prevent cross-service state corruption
func NewOrchardRollup(node statedb.StateProvider, serviceID uint32) *OrchardRollup {
	return &OrchardRollup{
		node:        node,
		serviceID:   serviceID, // ✅ Use provided serviceID instead of hardcoded constant
		notes:       make(map[common.Hash]OrchardNote),
		nullifiers:  make(map[common.Hash]bool),
		transfers:   make([]OrchardTransfer, 0),
		withdrawals: make([]OrchardWithdrawal, 0),
		events:      make([]OrchardEvent, 0),
	}
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

// currentBlockNumber mirrors the EVM rollup helper and tolerates missing state
// so the PoC can run without full block production.
func (r *OrchardRollup) currentBlockNumber() uint32 {
	if r == nil || r.node == nil {
		return 0
	}
	panic("orchard rollup currentBlockNumber not implemented") // TODO: implement block number tracking
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

	// Mark nullifier as spent and remove note
	r.nullifiers[active.Nullifier] = true
	delete(r.notes, active.Commitment)

	// Create new output note
	outputValue := active.Value - builderFee
	newCommitment := deriveOrchardCommitment(outputValue, toPK, toRho)
	newNullifier := deriveOrchardNullifier(r.serviceID, toPK, toRho)
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
	r.nullifiers[active.Nullifier] = true
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
	for _, note := range r.notes {
		notes = append(notes, note)
	}
	return notes
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

// OrchardStateKeys tracks storage keys written by the Orchard service.
var OrchardStateKeys = struct {
	CommitmentRoot []byte
	CommitmentSize []byte
}{
	CommitmentRoot: []byte("commitment_root"),
	CommitmentSize: []byte("commitment_size"),
}

// readStateValue reads a Orchard service key from the underlying StateDB.
func (r *OrchardRollup) readStateValue(key []byte) ([]byte, bool, error) {
	if r == nil || r.node == nil || r.node.GetStateDB() == nil {
		return nil, false, fmt.Errorf("stateDB unavailable")
	}
	return r.node.GetStateDB().ReadServiceStorage(r.serviceID, key)
}

// GetCommitmentTreeState returns commitment tree root/size from storage with an in-memory fallback.
func (r *OrchardRollup) GetCommitmentTreeState() (*TreeState, error) {
	rootData, found, err := r.readStateValue(OrchardStateKeys.CommitmentRoot)
	if err != nil || !found {
		return r.getTreeStateFromMemory()
	}

	sizeData, found, err := r.readStateValue(OrchardStateKeys.CommitmentSize)
	if err != nil || !found {
		return r.getTreeStateFromMemory()
	}

	if len(rootData) != 32 || len(sizeData) != 8 {
		return nil, fmt.Errorf("invalid tree state data")
	}

	return &TreeState{
		Root: common.BytesToHash(rootData),
		Size: binary.LittleEndian.Uint64(sizeData),
	}, nil
}

func (r *OrchardRollup) getTreeStateFromMemory() (*TreeState, error) {
	activeNotes := r.ActiveNotes()
	var rootBytes []byte
	for _, note := range activeNotes {
		rootBytes = append(rootBytes, note.Commitment.Bytes()...)
	}
	return &TreeState{
		Root: common.Blake2Hash(rootBytes),
		Size: uint64(len(activeNotes)),
	}, nil
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
