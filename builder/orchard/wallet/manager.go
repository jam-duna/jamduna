package wallet

import (
	cryptoRand "crypto/rand"
	"encoding/hex"
	"fmt"
	mathRand "math/rand"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
)

// WalletManager manages a collection of Orchard wallets
type WalletManager struct {
	mu      sync.RWMutex
	wallets map[string]*OrchardWallet // Wallet ID -> wallet

	// Global statistics
	totalWallets   int
	totalBalance   uint64
	totalNotes     uint64
	lastActivity   time.Time
}

// NewWalletManager creates a new wallet manager
func NewWalletManager() *WalletManager {
	return &WalletManager{
		wallets:      make(map[string]*OrchardWallet),
		lastActivity: time.Now(),
	}
}

// CreateWallet creates a new wallet and adds it to the manager
func (wm *WalletManager) CreateWallet(name string) (*OrchardWallet, error) {
	wallet, err := NewOrchardWallet(name)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.wallets[wallet.ID] = wallet
	wm.totalWallets++
	wm.lastActivity = time.Now()

	log.Info(log.Node, "Created wallet",
		"wallet_id", wallet.ID,
		"name", name,
		"total_wallets", wm.totalWallets)

	return wallet, nil
}

// GetWallet retrieves a wallet by ID
func (wm *WalletManager) GetWallet(walletID string) (*OrchardWallet, error) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	wallet, exists := wm.wallets[walletID]
	if !exists {
		return nil, fmt.Errorf("wallet not found: %s", walletID)
	}

	return wallet, nil
}

// GetAllWallets returns all managed wallets
func (wm *WalletManager) GetAllWallets() []*OrchardWallet {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	wallets := make([]*OrchardWallet, 0, len(wm.wallets))
	for _, wallet := range wm.wallets {
		wallets = append(wallets, wallet)
	}

	return wallets
}

// GetWalletByAddress finds a wallet that owns a specific address
func (wm *WalletManager) GetWalletByAddress(address string) (*OrchardWallet, error) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	for _, wallet := range wm.wallets {
		wallet.mu.RLock()
		_, hasAddress := wallet.addresses[address]
		wallet.mu.RUnlock()

		if hasAddress {
			return wallet, nil
		}
	}

	return nil, fmt.Errorf("no wallet found for address: %s", address)
}

// GenerateInitialNotes creates initial notes for all wallets
func (wm *WalletManager) GenerateInitialNotes(notesPerWallet int, valuePerNote uint64) error {
	wm.mu.RLock()
	wallets := make([]*OrchardWallet, 0, len(wm.wallets))
	for _, wallet := range wm.wallets {
		wallets = append(wallets, wallet)
	}
	wm.mu.RUnlock()

	for _, wallet := range wallets {
		for i := 0; i < notesPerWallet; i++ {
			note, err := wm.generateMockNote(wallet, valuePerNote)
			if err != nil {
				return fmt.Errorf("failed to generate note for wallet %s: %w", wallet.ID, err)
			}

			if err := wallet.AddNote(note); err != nil {
				return fmt.Errorf("failed to add note to wallet %s: %w", wallet.ID, err)
			}
		}
	}

	wm.updateGlobalStats()

	log.Info(log.Node, "Generated initial notes for all wallets",
		"total_wallets", len(wallets),
		"notes_per_wallet", notesPerWallet,
		"value_per_note", valuePerNote,
		"total_value", uint64(len(wallets)*notesPerWallet)*valuePerNote)

	return nil
}

// generateMockNote creates a mock Orchard note for a wallet
func (wm *WalletManager) generateMockNote(wallet *OrchardWallet, value uint64) (*OrchardNote, error) {
	// Generate random commitment
	var commitment [32]byte
	if _, err := cryptoRand.Read(commitment[:]); err != nil {
		return nil, fmt.Errorf("failed to generate commitment: %w", err)
	}

	// Generate nullifier from commitment (simplified)
	nullifierHash := common.Blake2Hash(commitment[:])
	var nullifier [32]byte
	copy(nullifier[:], nullifierHash[:32])

	// Generate random rho
	var rho [32]byte
	if _, err := cryptoRand.Read(rho[:]); err != nil {
		return nil, fmt.Errorf("failed to generate rho: %w", err)
	}

	// Use wallet's viewing key as owner PK (simplified)
	ownerPK := wallet.ViewingKey

	// Get default address
	defaultAddr := wallet.GetDefaultAddress()
	if defaultAddr == "" {
		return nil, fmt.Errorf("wallet has no addresses")
	}

	note := &OrchardNote{
		Commitment: commitment,
		Nullifier:  nullifier,
		Value:      value,
		Rho:        rho,
		OwnerPK:    ownerPK,
		Address:    defaultAddr,
		Height:     1, // Mock block height
		TxID:       generateMockTxID(),
		Timestamp:  time.Now(),
		IsSpent:    false,
	}

	return note, nil
}

// PerformRandomTransfer executes a random transfer between wallets
func (wm *WalletManager) PerformRandomTransfer(minAmount, maxAmount uint64) (*TransferResult, error) {
	wallets := wm.GetAllWallets()
	if len(wallets) < 2 {
		return nil, fmt.Errorf("need at least 2 wallets for transfer")
	}

	// Select random sender and receiver
	senderIdx := mathRand.Intn(len(wallets))
	receiverIdx := mathRand.Intn(len(wallets))
	for receiverIdx == senderIdx {
		receiverIdx = mathRand.Intn(len(wallets))
	}

	sender := wallets[senderIdx]
	receiver := wallets[receiverIdx]

	// Check sender balance
	senderBalance := sender.GetBalance()
	if senderBalance < minAmount {
		return nil, fmt.Errorf("sender %s has insufficient balance: %d < %d", sender.ID, senderBalance, minAmount)
	}

	// Calculate transfer amount
	maxTransfer := min(maxAmount, senderBalance)
	amount := minAmount + uint64(mathRand.Intn(int(maxTransfer-minAmount+1)))

	// Select notes to spend
	notesToSpend, totalValue, err := sender.SelectNotesForAmount(amount)
	if err != nil {
		return nil, fmt.Errorf("failed to select notes: %w", err)
	}

	// Calculate change amount
	changeAmount := totalValue - amount

	// Perform the transfer
	result, err := wm.executeTransfer(sender, receiver, notesToSpend, amount, changeAmount)
	if err != nil {
		return nil, fmt.Errorf("transfer failed: %w", err)
	}

	wm.updateGlobalStats()

	return result, nil
}

// TransferResult represents the result of a transfer operation
type TransferResult struct {
	Sender        string    `json:"sender"`
	Receiver      string    `json:"receiver"`
	Amount        uint64    `json:"amount"`
	Change        uint64    `json:"change"`
	SpentNotes    int       `json:"spent_notes"`
	CreatedNotes  int       `json:"created_notes"`
	TxID          string    `json:"tx_id"`
	Timestamp     time.Time `json:"timestamp"`
}

// executeTransfer performs the actual transfer between wallets
func (wm *WalletManager) executeTransfer(sender, receiver *OrchardWallet, notesToSpend []*OrchardNote, amount, changeAmount uint64) (*TransferResult, error) {
	txID := generateMockTxID()

	// Extract commitments from notes to spend
	var commitments [][32]byte
	for _, note := range notesToSpend {
		commitments = append(commitments, note.Commitment)
	}

	// Spend notes from sender
	_, err := sender.SpendNotes(commitments)
	if err != nil {
		return nil, fmt.Errorf("failed to spend notes: %w", err)
	}

	createdNotes := 0

	// Create output note for receiver
	if amount > 0 {
		receiverNote, err := wm.generateMockNote(receiver, amount)
		if err != nil {
			return nil, fmt.Errorf("failed to generate receiver note: %w", err)
		}
		receiverNote.TxID = txID

		if err := receiver.AddNote(receiverNote); err != nil {
			return nil, fmt.Errorf("failed to add note to receiver: %w", err)
		}
		createdNotes++
	}

	// Create change note for sender if needed
	if changeAmount > 0 {
		changeNote, err := wm.generateMockNote(sender, changeAmount)
		if err != nil {
			return nil, fmt.Errorf("failed to generate change note: %w", err)
		}
		changeNote.TxID = txID

		if err := sender.AddNote(changeNote); err != nil {
			return nil, fmt.Errorf("failed to add change note to sender: %w", err)
		}
		createdNotes++
	}

	result := &TransferResult{
		Sender:       sender.ID,
		Receiver:     receiver.ID,
		Amount:       amount,
		Change:       changeAmount,
		SpentNotes:   len(notesToSpend),
		CreatedNotes: createdNotes,
		TxID:         txID,
		Timestamp:    time.Now(),
	}

	log.Info(log.Node, "Executed transfer",
		"tx_id", txID,
		"sender", sender.ID,
		"receiver", receiver.ID,
		"amount", amount,
		"change", changeAmount,
		"notes_spent", len(notesToSpend),
		"notes_created", createdNotes)

	return result, nil
}

// GetGlobalStats returns aggregated statistics across all wallets
func (wm *WalletManager) GetGlobalStats() map[string]interface{} {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	wm.updateGlobalStatsUnsafe()

	return map[string]interface{}{
		"total_wallets":   wm.totalWallets,
		"total_balance":   wm.totalBalance,
		"total_notes":     wm.totalNotes,
		"last_activity":   wm.lastActivity,
		"average_balance": wm.calculateAverageBalance(),
	}
}

// updateGlobalStats recalculates global statistics
func (wm *WalletManager) updateGlobalStats() {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.updateGlobalStatsUnsafe()
}

func (wm *WalletManager) updateGlobalStatsUnsafe() {
	wm.totalBalance = 0
	wm.totalNotes = 0

	for _, wallet := range wm.wallets {
		wm.totalBalance += wallet.GetBalance()
		stats := wallet.GetStats()
		if noteCount, ok := stats["total_notes"].(uint64); ok {
			wm.totalNotes += noteCount
		}
	}

	wm.lastActivity = time.Now()
}

func (wm *WalletManager) calculateAverageBalance() uint64 {
	if wm.totalWallets == 0 {
		return 0
	}
	return wm.totalBalance / uint64(wm.totalWallets)
}

// generateMockTxID creates a mock transaction ID
func generateMockTxID() string {
	var randomBytes [16]byte
	if _, err := cryptoRand.Read(randomBytes[:]); err != nil {
		// Fallback to timestamp-based ID on entropy failure
		return fmt.Sprintf("tx_fallback_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(randomBytes[:])
}

// Helper function
func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}