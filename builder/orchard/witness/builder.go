// Package witness provides Orchard rollup builder functionality for JAM services
package witness

import (
	"errors"
	"fmt"

	orchardmerkle "github.com/colorfulnotion/jam/builder/orchard/merkle"
	jamcommon "github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/ethereum/go-ethereum/common"
)

// OrchardBuilder provides builder-specific Orchard operations for rollup processing
type OrchardBuilder struct {
	serviceID          uint32
	storage            orchardmerkle.MerkleTreeStorage
	jamClient          types.NodeClient // CE146/147/148 interface to JAM network
	wallet             OrchardWallet
	commitmentProvider CommitmentProvider
	notes              map[common.Hash]OrchardNote
	nullifiers         map[common.Hash]bool
	transfers          []OrchardTransfer
	withdrawals        []OrchardWithdrawal
	events             []OrchardEvent
}

// OrchardNote represents a simplified shielded note tracked inside the rollup
type OrchardNote struct {
	Commitment   common.Hash
	Nullifier    common.Hash
	Value        uint64
	AssetID      uint32
	Amount       Uint128
	OwnerPubKey  [32]byte
	Rho          [32]byte
	NoteRseed    [32]byte
	UnlockHeight uint64
	MemoHash     [32]byte
	Source       common.Address
}

// OrchardNoteInput defines the full input set used by the circuits.
type OrchardNoteInput struct {
	AssetID      uint32
	Amount       Uint128
	OwnerPubKey  [32]byte
	Rho          [32]byte
	NoteRseed    [32]byte
	UnlockHeight uint64
	MemoHash     [32]byte
}

// OrchardTransfer captures an in-rollup transfer between notes along with a builder fee
type OrchardTransfer struct {
	From       common.Hash
	To         common.Hash
	BuilderFee uint64
	Block      uint32
}

// OrchardWithdrawal records a public exit of a note back to a recipient address
type OrchardWithdrawal struct {
	Nullifier common.Hash
	To        common.Address
	Value     uint64
	Block     uint32
}

// OrchardEvent is a lightweight event log to keep the PoC debuggable
type OrchardEvent struct {
	Kind       string
	Commitment common.Hash
	Nullifier  common.Hash
	Value      uint64
	Block      uint32
	Details    string
}

// OrchardTransaction represents a Orchard transaction for bundle building
type OrchardTransaction struct {
	Type   string      // "deposit", "transfer", "withdraw"
	Data   interface{} // Transaction-specific data
	Sender common.Address
}

// NewOrchardBuilder creates a new Orchard builder instance
func NewOrchardBuilder(serviceID uint32, storage orchardmerkle.MerkleTreeStorage, jamClient types.NodeClient) *OrchardBuilder {
	return NewOrchardBuilderWithWallet(serviceID, storage, jamClient, nil, nil)
}

// NewOrchardBuilderWithWallet creates a new Orchard builder instance with a wallet.
func NewOrchardBuilderWithWallet(
	serviceID uint32,
	storage orchardmerkle.MerkleTreeStorage,
	jamClient types.NodeClient,
	wallet OrchardWallet,
	commitmentProvider CommitmentProvider,
) *OrchardBuilder {
	return &OrchardBuilder{
		serviceID:          serviceID,
		storage:            storage,
		jamClient:          jamClient,
		wallet:             wallet,
		commitmentProvider: commitmentProvider,
		notes:              make(map[common.Hash]OrchardNote),
		nullifiers:         make(map[common.Hash]bool),
		transfers:          make([]OrchardTransfer, 0),
		withdrawals:        make([]OrchardWithdrawal, 0),
		events:             make([]OrchardEvent, 0),
	}
}

// GetServiceID returns the service ID for this builder
func (b *OrchardBuilder) GetServiceID() uint32 {
	return b.serviceID
}

// SetWallet updates the wallet used for nullifier derivation and proofs.
func (b *OrchardBuilder) SetWallet(wallet OrchardWallet) {
	b.wallet = wallet
}

// Wallet returns the configured wallet, if any.
func (b *OrchardBuilder) Wallet() OrchardWallet {
	return b.wallet
}

// SetCommitmentProvider updates the provider used for commitment derivation.
func (b *OrchardBuilder) SetCommitmentProvider(provider CommitmentProvider) {
	b.commitmentProvider = provider
}

// CommitmentProvider returns the configured commitment provider, if any.
func (b *OrchardBuilder) CommitmentProvider() CommitmentProvider {
	return b.commitmentProvider
}

// BuildBundle executes a batch of Orchard transactions and generates a work package
func (b *OrchardBuilder) BuildBundle(txs []OrchardTransaction) (*types.WorkPackage, error) {
	// Get current Orchard service state
	orchardService, ok, err := b.jamClient.GetService(b.serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get Orchard service: %v", err)
	}
	if !ok {
		return nil, fmt.Errorf("Orchard service not found")
	}

	// Convert transactions to extrinsics
	var extrinsics []types.WorkItemExtrinsic
	var extrinsicsBlobs []types.ExtrinsicsBlobs
	var extrinsicDataArray [][]byte

	for _, tx := range txs {
		// Convert Orchard transaction to JAM extrinsic format
		extrinsicData, err := b.convertOrchardTxToExtrinsic(tx)
		if err != nil {
			return nil, fmt.Errorf("failed to convert transaction to extrinsic: %v", err)
		}

		txHash := jamcommon.Blake2Hash(extrinsicData)

		extrinsics = append(extrinsics, types.WorkItemExtrinsic{
			Hash: txHash,
			Len:  uint32(len(extrinsicData)),
		})

		extrinsicDataArray = append(extrinsicDataArray, extrinsicData)
	}

	// Get actual global depth from StateDB instead of hardcoding to 0
	globalDepth, err := b.jamClient.ReadGlobalDepth(orchardService.ServiceIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to read global depth: %v", err)
	}

	// Create work package template with initial payload
	workPackage := types.WorkPackage{
		AuthCodeHost:          0,
		AuthorizationCodeHash: jamcommon.Hash{}, // Will be set by caller
		AuthorizationToken:    nil,
		ConfigurationBlob:     nil,
		RefineContext:         types.RefineContext{},
		WorkItems: []types.WorkItem{
			{
				Service:            b.serviceID,
				CodeHash:           orchardService.CodeHash,
				RefineGasLimit:     types.RefineGasAllocation,
				AccumulateGasLimit: types.AccumulationGasAllocation,
				ImportedSegments:   []types.ImportSegment{},
				ExportCount:        0,
				Extrinsics:         extrinsics,
				// Use initial payload with 0 witnesses - will be updated after BuildBundle execution
				Payload: b.buildPayload(len(txs), globalDepth, 0, common.Hash{}),
			},
		},
	}

	// Package extrinsics data
	extrinsicsBlobs = append(extrinsicsBlobs, extrinsicDataArray)

	// Execute bundle to generate witnesses for the pending transactions
	_, workReport, err := b.jamClient.BuildBundle(workPackage, extrinsicsBlobs, 0, nil, "", false)
	if err != nil {
		return nil, fmt.Errorf("failed to execute bundle: %v", err)
	}

	if workReport == nil {
		return nil, fmt.Errorf("build bundle returned nil work report")
	}

	// Get actual witness count from StateDB after BuildBundle execution
	// The BuildBundle call populated the StateDB witness cache with witnesses for pending transactions
	numWitnesses := b.jamClient.GetWitnessCount()

	// Update payload with real witness count from pending transactions
	workPackage.WorkItems[0].Payload = b.buildPayload(len(txs), globalDepth, numWitnesses, common.Hash{})

	return &workPackage, nil
}

// Deposit creates a new shielded note from public funds
func (b *OrchardBuilder) Deposit(sender common.Address, input OrchardNoteInput) (OrchardNote, error) {
	value, err := noteValue(input.Amount)
	if err != nil {
		return OrchardNote{}, err
	}

	commitment, err := b.deriveOrchardCommitment(input)
	if err != nil {
		return OrchardNote{}, err
	}
	if _, exists := b.notes[commitment]; exists {
		return OrchardNote{}, fmt.Errorf("commitment already exists: %s", commitment.String())
	}

	nullifier, err := b.deriveOrchardNullifier(input.OwnerPubKey, input.Rho, commitment)
	if err != nil {
		return OrchardNote{}, err
	}
	if b.nullifiers[nullifier] {
		return OrchardNote{}, fmt.Errorf("nullifier already spent: %s", nullifier.String())
	}

	note := OrchardNote{
		Commitment:   commitment,
		Nullifier:    nullifier,
		Value:        value,
		AssetID:      input.AssetID,
		Amount:       input.Amount,
		OwnerPubKey:  input.OwnerPubKey,
		Rho:          input.Rho,
		NoteRseed:    input.NoteRseed,
		UnlockHeight: input.UnlockHeight,
		MemoHash:     input.MemoHash,
		Source:       sender,
	}

	b.notes[commitment] = note
	b.recordEvent("deposit", commitment, nullifier, value, fmt.Sprintf("from %s", sender.Hex()))

	// Add commitment to Merkle tree
	err = b.storage.Append(commitment)
	if err != nil {
		return OrchardNote{}, fmt.Errorf("failed to add commitment to tree: %v", err)
	}

	return note, nil
}

// Transfer spends an input note and creates a new note for the recipient
func (b *OrchardBuilder) Transfer(input OrchardNote, output OrchardNoteInput, builderFee uint64) (OrchardNote, error) {
	active, ok := b.notes[input.Commitment]
	if !ok {
		return OrchardNote{}, fmt.Errorf("unknown commitment: %s", input.Commitment.String())
	}

	if b.nullifiers[active.Nullifier] {
		return OrchardNote{}, fmt.Errorf("nullifier already spent: %s", active.Nullifier.String())
	}

	if builderFee > active.Value {
		return OrchardNote{}, fmt.Errorf("builder fee %d exceeds note value %d", builderFee, active.Value)
	}

	outputValue := active.Value - builderFee
	outputAmount, err := noteValue(output.Amount)
	if err != nil {
		return OrchardNote{}, err
	}
	if outputAmount != outputValue {
		return OrchardNote{}, fmt.Errorf("output amount %d must equal input value %d minus builder fee %d", outputAmount, active.Value, builderFee)
	}
	if output.AssetID != active.AssetID {
		return OrchardNote{}, fmt.Errorf("output asset %d does not match input asset %d", output.AssetID, active.AssetID)
	}

	// Spend the input note
	b.nullifiers[active.Nullifier] = true
	delete(b.notes, active.Commitment)

	// Create output note
	newCommitment, err := b.deriveOrchardCommitment(output)
	if err != nil {
		return OrchardNote{}, err
	}
	newNullifier, err := b.deriveOrchardNullifier(output.OwnerPubKey, output.Rho, newCommitment)
	if err != nil {
		return OrchardNote{}, err
	}

	newNote := OrchardNote{
		Commitment:   newCommitment,
		Nullifier:    newNullifier,
		Value:        outputValue,
		AssetID:      output.AssetID,
		Amount:       output.Amount,
		OwnerPubKey:  output.OwnerPubKey,
		Rho:          output.Rho,
		NoteRseed:    output.NoteRseed,
		UnlockHeight: output.UnlockHeight,
		MemoHash:     output.MemoHash,
		Source:       active.Source,
	}

	b.notes[newCommitment] = newNote

	transfer := OrchardTransfer{
		From:       active.Commitment,
		To:         newCommitment,
		BuilderFee: builderFee,
		Block:      b.currentBlockNumber(),
	}
	b.transfers = append(b.transfers, transfer)
	b.recordEvent("transfer", newCommitment, active.Nullifier, outputValue, fmt.Sprintf("builder_fee=%d", builderFee))

	// Add new commitment to Merkle tree
	err = b.storage.Append(newCommitment)
	if err != nil {
		return OrchardNote{}, fmt.Errorf("failed to add commitment to tree: %v", err)
	}

	return newNote, nil
}

// Withdraw consumes a note and creates a public withdrawal
func (b *OrchardBuilder) Withdraw(note OrchardNote, recipient common.Address) (OrchardWithdrawal, error) {
	active, ok := b.notes[note.Commitment]
	if !ok {
		return OrchardWithdrawal{}, fmt.Errorf("commitment not active: %s", note.Commitment.String())
	}

	if b.nullifiers[active.Nullifier] {
		return OrchardWithdrawal{}, fmt.Errorf("nullifier already spent: %s", active.Nullifier.String())
	}

	b.nullifiers[active.Nullifier] = true
	delete(b.notes, active.Commitment)

	withdrawal := OrchardWithdrawal{
		Nullifier: active.Nullifier,
		To:        recipient,
		Value:     active.Value,
		Block:     b.currentBlockNumber(),
	}
	b.withdrawals = append(b.withdrawals, withdrawal)
	b.recordEvent("withdraw", active.Commitment, active.Nullifier, active.Value, recipient.Hex())

	return withdrawal, nil
}

// GenerateWitness creates a Merkle witness for a commitment
func (b *OrchardBuilder) GenerateWitness(commitment common.Hash) (orchardmerkle.MerkleWitness, error) {
	// Find position of commitment in tree
	treeSize := b.storage.GetSize()
	for i := uint64(0); i < treeSize; i++ {
		leaf, err := b.storage.GetLeaf(i)
		if err != nil {
			continue
		}
		if leaf == commitment {
			return b.storage.GenerateWitness(i)
		}
	}
	return orchardmerkle.MerkleWitness{}, fmt.Errorf("commitment not found in tree")
}

// GetMerkleRoot returns the current Merkle tree root
func (b *OrchardBuilder) GetMerkleRoot() common.Hash {
	return b.storage.GetRoot()
}

// GetTreeSize returns the current Merkle tree size
func (b *OrchardBuilder) GetTreeSize() uint64 {
	return b.storage.GetSize()
}

// ActiveNotes returns a copy of active notes
func (b *OrchardBuilder) ActiveNotes() []OrchardNote {
	notes := make([]OrchardNote, 0, len(b.notes))
	for _, note := range b.notes {
		notes = append(notes, note)
	}
	return notes
}

// IsNullifierSpent checks if a nullifier has been spent
func (b *OrchardBuilder) IsNullifierSpent(nullifier common.Hash) bool {
	return b.nullifiers[nullifier]
}

// deriveOrchardNullifier derives nullifier from wallet-provided spending keys.
func (b *OrchardBuilder) deriveOrchardNullifier(pk [32]byte, rho [32]byte, commitment common.Hash) (common.Hash, error) {
	if b.wallet == nil {
		return common.Hash{}, errors.New("orchard wallet not configured")
	}

	nullifier, err := b.wallet.NullifierFor(pk, rho, commitment)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to derive nullifier: %v", err)
	}

	return common.BytesToHash(nullifier[:]), nil
}

// deriveOrchardCommitment produces a circuit-aligned commitment via the configured provider.
func (b *OrchardBuilder) deriveOrchardCommitment(input OrchardNoteInput) (common.Hash, error) {
	if b.commitmentProvider == nil {
		return common.Hash{}, errors.New("orchard commitment provider not configured")
	}

	commitment, err := b.commitmentProvider.CommitmentFor(
		input.AssetID,
		input.Amount,
		input.OwnerPubKey,
		input.Rho,
		input.NoteRseed,
		input.UnlockHeight,
		input.MemoHash,
	)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to derive commitment: %v", err)
	}

	return common.BytesToHash(commitment[:]), nil
}

func noteValue(amount Uint128) (uint64, error) {
	if amount.Hi != 0 {
		return 0, fmt.Errorf("amount exceeds uint64 range: hi limb %d", amount.Hi)
	}
	return amount.Lo, nil
}

// currentBlockNumber gets the current block number
func (b *OrchardBuilder) currentBlockNumber() uint32 {
	// This would typically query the JAM client for the current block
	return 0 // Placeholder
}

// recordEvent appends an event with the latest block context
func (b *OrchardBuilder) recordEvent(kind string, commitment common.Hash, nullifier common.Hash, value uint64, details string) {
	b.events = append(b.events, OrchardEvent{
		Kind:       kind,
		Commitment: commitment,
		Nullifier:  nullifier,
		Value:      value,
		Block:      b.currentBlockNumber(),
		Details:    details,
	})
}

// convertOrchardTxToExtrinsic converts a Orchard transaction to JAM extrinsic format
func (b *OrchardBuilder) convertOrchardTxToExtrinsic(tx OrchardTransaction) ([]byte, error) {
	// Simplified extrinsic format for Orchard transactions
	// This would need to match the actual Orchard service specification

	switch tx.Type {
	case "deposit":
		// Deposit extrinsic format: type(1) + commitment(32) + value(8) + sender(20)
		extrinsic := make([]byte, 61)
		extrinsic[0] = 0x01 // Deposit type
		// Additional fields would be filled based on tx.Data
		return extrinsic, nil
	case "transfer":
		// Transfer extrinsic format: type(1) + input_nullifier(32) + output_commitment(32) + fee(8)
		extrinsic := make([]byte, 73)
		extrinsic[0] = 0x02 // Transfer type
		// Additional fields would be filled based on tx.Data
		return extrinsic, nil
	case "withdraw":
		// Withdraw extrinsic format: type(1) + nullifier(32) + recipient(20) + value(8)
		extrinsic := make([]byte, 61)
		extrinsic[0] = 0x03 // Withdraw type
		// Additional fields would be filled based on tx.Data
		return extrinsic, nil
	default:
		return nil, fmt.Errorf("unknown transaction type: %s", tx.Type)
	}
}

// buildPayload constructs a payload byte array for Orchard transaction processing
func (b *OrchardBuilder) buildPayload(count int, globalDepth uint8, numWitnesses int, blockAccessListHash common.Hash) []byte {
	payload := make([]byte, 40) // 1 + 4 + 1 + 2 + 32 = 40 bytes
	payload[0] = 0x01           // PayloadTypeTransactions

	// count (4 bytes, little-endian)
	for i := 0; i < 4; i++ {
		payload[1+i] = byte(count >> (i * 8))
	}

	payload[5] = globalDepth

	// numWitnesses (2 bytes, little-endian)
	payload[6] = byte(numWitnesses)
	payload[7] = byte(numWitnesses >> 8)

	copy(payload[8:40], blockAccessListHash[:])
	return payload
}
