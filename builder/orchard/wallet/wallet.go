package wallet

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
)

// OrchardWallet represents a simplified Orchard wallet with UTXO management
type OrchardWallet struct {
	// Wallet identity
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Created  time.Time `json:"created"`

	// Cryptographic keys
	SpendingKey   [32]byte `json:"spending_key"`   // Private spending key
	ViewingKey    [32]byte `json:"viewing_key"`    // Derived viewing key
	DiversifierKey [32]byte `json:"diversifier_key"` // For address generation

	// UTXO management
	mu           sync.RWMutex
	unspentNotes map[string]*OrchardNote `json:"unspent_notes"` // Commitment hash -> note
	spentNotes   map[string]*OrchardNote `json:"spent_notes"`   // Track spent notes
	addresses    map[string]*Address     `json:"addresses"`     // Generated addresses

	// Wallet state
	balance      uint64    `json:"balance"`       // Total balance in unspent notes
	lastActivity time.Time `json:"last_activity"` // Last transaction time
	noteCount    uint64    `json:"note_count"`    // Total notes received
}

// OrchardNote represents a shielded note in the wallet's UTXO set
type OrchardNote struct {
	// Note identification
	Commitment [32]byte `json:"commitment"`    // Note commitment hash
	Nullifier  [32]byte `json:"nullifier"`     // Nullifier for spending

	// Note content
	Value      uint64   `json:"value"`         // Note value
	Rho        [32]byte `json:"rho"`           // Note randomness
	OwnerPK    [32]byte `json:"owner_pk"`      // Owner public key

	// Metadata
	Address    string    `json:"address"`      // Receiving address
	Height     uint32    `json:"height"`       // Block height received
	TxID       string    `json:"tx_id"`        // Transaction ID
	Timestamp  time.Time `json:"timestamp"`    // When received
	IsSpent    bool      `json:"is_spent"`     // Spending status
	SpentAt    *time.Time `json:"spent_at,omitempty"` // When spent
}

// Address represents a diversified Orchard address
type Address struct {
	Address      string    `json:"address"`       // The actual address string
	Diversifier  [11]byte  `json:"diversifier"`   // Diversifier for this address
	PublicKey    [32]byte  `json:"public_key"`    // Transmission key
	Created      time.Time `json:"created"`       // When generated
	Label        string    `json:"label"`         // Optional label
	UsageCount   int       `json:"usage_count"`   // How many times used
}

// NewOrchardWallet creates a new wallet with random keys
func NewOrchardWallet(name string) (*OrchardWallet, error) {
	wallet := &OrchardWallet{
		ID:           generateWalletID(),
		Name:         name,
		Created:      time.Now(),
		unspentNotes: make(map[string]*OrchardNote),
		spentNotes:   make(map[string]*OrchardNote),
		addresses:    make(map[string]*Address),
		lastActivity: time.Now(),
	}

	// Generate random cryptographic keys
	if _, err := rand.Read(wallet.SpendingKey[:]); err != nil {
		return nil, fmt.Errorf("failed to generate spending key: %w", err)
	}

	// Derive viewing key from spending key (simplified)
	viewingKeyHash := common.Blake2Hash(wallet.SpendingKey[:])
	copy(wallet.ViewingKey[:], viewingKeyHash[:32])

	// Generate diversifier key
	if _, err := rand.Read(wallet.DiversifierKey[:]); err != nil {
		return nil, fmt.Errorf("failed to generate diversifier key: %w", err)
	}

	// Generate default address
	defaultAddr, err := wallet.GenerateAddress("default")
	if err != nil {
		return nil, fmt.Errorf("failed to generate default address: %w", err)
	}

	log.Info(log.Node, "Created new Orchard wallet",
		"wallet_id", wallet.ID,
		"name", name,
		"default_address", defaultAddr)

	return wallet, nil
}

// GenerateAddress creates a new diversified address for the wallet
func (w *OrchardWallet) GenerateAddress(label string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Generate random diversifier
	var diversifier [11]byte
	if _, err := rand.Read(diversifier[:]); err != nil {
		return "", fmt.Errorf("failed to generate diversifier: %w", err)
	}

	// Derive transmission key (simplified)
	keyData := append(w.ViewingKey[:], diversifier[:]...)
	keyHash := common.Blake2Hash(keyData)
	var transmissionKey [32]byte
	copy(transmissionKey[:], keyHash[:32])

	// Create address string (simplified encoding)
	addressData := append(diversifier[:], transmissionKey[:]...)
	addressHash := common.Blake2Hash(addressData)
	address := fmt.Sprintf("orchard1%s", hex.EncodeToString(addressHash[:20]))

	// Store address
	w.addresses[address] = &Address{
		Address:     address,
		Diversifier: diversifier,
		PublicKey:   transmissionKey,
		Created:     time.Now(),
		Label:       label,
		UsageCount:  0,
	}

	log.Debug(log.Node, "Generated new address",
		"wallet_id", w.ID,
		"address", address,
		"label", label)

	return address, nil
}

// AddNote adds a received note to the wallet's UTXO set
func (w *OrchardWallet) AddNote(note *OrchardNote) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	commitmentHex := hex.EncodeToString(note.Commitment[:])

	// Check if we already have this note
	if _, exists := w.unspentNotes[commitmentHex]; exists {
		return fmt.Errorf("note already exists: %s", commitmentHex[:16])
	}

	// Add to unspent notes
	w.unspentNotes[commitmentHex] = note
	w.balance += note.Value
	w.noteCount++
	w.lastActivity = time.Now()

	// Update address usage
	if addr, exists := w.addresses[note.Address]; exists {
		addr.UsageCount++
	}

	log.Info(log.Node, "Added note to wallet",
		"wallet_id", w.ID,
		"commitment", commitmentHex[:16],
		"value", note.Value,
		"balance", w.balance,
		"note_count", w.noteCount)

	return nil
}

// SpendNotes marks notes as spent and removes them from UTXO set
func (w *OrchardWallet) SpendNotes(commitments [][32]byte) ([]*OrchardNote, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var spentNotes []*OrchardNote
	var totalValue uint64

	// Find and validate all notes can be spent
	for _, commitment := range commitments {
		commitmentHex := hex.EncodeToString(commitment[:])
		note, exists := w.unspentNotes[commitmentHex]
		if !exists {
			return nil, fmt.Errorf("note not found or already spent: %s", commitmentHex[:16])
		}
		spentNotes = append(spentNotes, note)
		totalValue += note.Value
	}

	// Move notes from unspent to spent
	now := time.Now()
	for _, note := range spentNotes {
		commitmentHex := hex.EncodeToString(note.Commitment[:])

		// Mark as spent
		note.IsSpent = true
		note.SpentAt = &now

		// Move to spent notes tracking
		delete(w.unspentNotes, commitmentHex)
		w.spentNotes[commitmentHex] = note

		w.balance -= note.Value
	}

	w.lastActivity = time.Now()

	log.Info(log.Node, "Spent notes from wallet",
		"wallet_id", w.ID,
		"notes_spent", len(spentNotes),
		"total_value", totalValue,
		"remaining_balance", w.balance)

	return spentNotes, nil
}

// GetUnspentNotes returns all unspent notes in the wallet
func (w *OrchardWallet) GetUnspentNotes() []*OrchardNote {
	w.mu.RLock()
	defer w.mu.RUnlock()

	notes := make([]*OrchardNote, 0, len(w.unspentNotes))
	for _, note := range w.unspentNotes {
		notes = append(notes, note)
	}

	return notes
}

// GetBalance returns the current wallet balance
func (w *OrchardWallet) GetBalance() uint64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.balance
}

// GetAddresses returns all generated addresses
func (w *OrchardWallet) GetAddresses() map[string]*Address {
	w.mu.RLock()
	defer w.mu.RUnlock()

	addresses := make(map[string]*Address)
	for addr, info := range w.addresses {
		addresses[addr] = info
	}
	return addresses
}

// GetDefaultAddress returns the default address for the wallet
func (w *OrchardWallet) GetDefaultAddress() string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for addr, info := range w.addresses {
		if info.Label == "default" {
			return addr
		}
	}

	// Return first address if no default found
	for addr := range w.addresses {
		return addr
	}

	return ""
}

// SelectNotesForAmount selects unspent notes to cover a specific amount
func (w *OrchardWallet) SelectNotesForAmount(amount uint64) ([]*OrchardNote, uint64, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.balance < amount {
		return nil, 0, fmt.Errorf("insufficient balance: need %d, have %d", amount, w.balance)
	}

	var selectedNotes []*OrchardNote
	var selectedValue uint64

	// Simple selection algorithm - use largest notes first
	// In production, this would be more sophisticated
	for _, note := range w.unspentNotes {
		if selectedValue >= amount {
			break
		}
		selectedNotes = append(selectedNotes, note)
		selectedValue += note.Value
	}

	if selectedValue < amount {
		return nil, 0, fmt.Errorf("could not select sufficient notes: selected %d, need %d", selectedValue, amount)
	}

	return selectedNotes, selectedValue, nil
}

// GetStats returns wallet statistics
func (w *OrchardWallet) GetStats() map[string]interface{} {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return map[string]interface{}{
		"wallet_id":       w.ID,
		"name":           w.Name,
		"balance":        w.balance,
		"unspent_notes":  len(w.unspentNotes),
		"spent_notes":    len(w.spentNotes),
		"total_notes":    w.noteCount,
		"addresses":      len(w.addresses),
		"last_activity":  w.lastActivity,
	}
}

// generateWalletID creates a unique wallet identifier
func generateWalletID() string {
	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		// Fallback to timestamp-based ID on entropy failure
		return fmt.Sprintf("wallet_fallback_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("wallet_%s", hex.EncodeToString(randomBytes[:8]))
}