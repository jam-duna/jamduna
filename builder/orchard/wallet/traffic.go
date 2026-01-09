package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"

	log "github.com/colorfulnotion/jam/log"
)

// Seed random number generator once at package initialization
func init() {
	rand.Seed(time.Now().UnixNano())
}

// TrafficGenerator generates realistic transaction patterns for testing
type TrafficGenerator struct {
	// Configuration
	walletManager *WalletManager
	rpcClient     RPCClient
	config        TrafficConfig

	// State
	mu              sync.RWMutex
	isRunning       bool
	stopChan        chan struct{}
	stats           TrafficStats
	lastTransaction time.Time
}

// TrafficConfig defines traffic generation parameters
type TrafficConfig struct {
	// Rate control
	TransactionsPerMinute int           `json:"transactions_per_minute"` // Target transaction rate
	BurstSize            int           `json:"burst_size"`              // Max transactions in burst
	BurstInterval        time.Duration `json:"burst_interval"`          // Time between bursts

	// Transaction patterns
	MinAmount        uint64  `json:"min_amount"`         // Minimum transfer amount
	MaxAmount        uint64  `json:"max_amount"`         // Maximum transfer amount
	ChangeThreshold  float64 `json:"change_threshold"`   // Probability of generating change

	// Wallet behavior
	ActiveWalletRatio float64 `json:"active_wallet_ratio"` // Fraction of wallets that are active
	NewAddressRate    float64 `json:"new_address_rate"`    // Probability of using new address

	// Advanced patterns
	EnableBursts      bool    `json:"enable_bursts"`       // Enable bursty traffic
	EnableLargeTransfers bool `json:"enable_large_transfers"` // Enable occasional large transfers
	LargeTransferProb float64 `json:"large_transfer_prob"` // Probability of large transfer
}

// TrafficStats tracks traffic generation statistics
type TrafficStats struct {
	// Transaction counts
	TotalTransactions    uint64 `json:"total_transactions"`
	SuccessfulTxs       uint64 `json:"successful_txs"`
	FailedTxs           uint64 `json:"failed_txs"`
	RPCCalls            uint64 `json:"rpc_calls"`
	RPCErrors           uint64 `json:"rpc_errors"`

	// Timing
	StartTime           time.Time `json:"start_time"`
	LastTransactionTime time.Time `json:"last_transaction_time"`
	AverageTPM          float64   `json:"average_tpm"` // Transactions per minute

	// Transaction patterns
	TotalVolume         uint64 `json:"total_volume"`     // Total value transferred
	AverageAmount       uint64 `json:"average_amount"`   // Average transfer amount
	LargeTransactions   uint64 `json:"large_transactions"` // Count of large transfers
	ChangeTransactions  uint64 `json:"change_transactions"` // Transactions with change

	// RPC patterns
	SendManyCallsCount  uint64 `json:"send_many_calls"`  // z_sendmany calls
	BundleSubmissions   uint64 `json:"bundle_submissions"` // Raw bundle submissions
	AddressGenerations  uint64 `json:"address_generations"` // New address generations
}

// RPCClient interface for interacting with Orchard RPC
type RPCClient interface {
	SendMany(from string, amounts []SendAmount) (string, error)
	SendRawBundle(hexBundle string) (string, error)
	GetNewAddress() (string, error)
	GetMempoolInfo() (map[string]interface{}, error)
}

// SendAmount represents a recipient and amount for z_sendmany
type SendAmount struct {
	Address string  `json:"address"`
	Amount  float64 `json:"amount"`
}

// NewTrafficGenerator creates a new traffic generator
func NewTrafficGenerator(walletManager *WalletManager, rpcClient RPCClient) *TrafficGenerator {
	return &TrafficGenerator{
		walletManager: walletManager,
		rpcClient:     rpcClient,
		config:        DefaultTrafficConfig(),
		stopChan:      make(chan struct{}),
		stats: TrafficStats{
			StartTime: time.Now(),
		},
	}
}

// DefaultTrafficConfig returns sensible default configuration
func DefaultTrafficConfig() TrafficConfig {
	return TrafficConfig{
		TransactionsPerMinute: 10,
		BurstSize:            5,
		BurstInterval:        30 * time.Second,
		MinAmount:            1000,      // 0.001 units
		MaxAmount:            1000000,   // 1 unit
		ChangeThreshold:      0.7,       // 70% chance of change
		ActiveWalletRatio:    0.8,       // 80% of wallets active
		NewAddressRate:       0.1,       // 10% use new addresses
		EnableBursts:         true,
		EnableLargeTransfers: true,
		LargeTransferProb:    0.05,      // 5% large transfers
	}
}

// SetConfig updates the traffic generation configuration
func (tg *TrafficGenerator) SetConfig(config TrafficConfig) {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.config = config
}

// validateConfig ensures configuration values are safe
func (tg *TrafficGenerator) validateConfig() error {
	if tg.config.TransactionsPerMinute <= 0 {
		return fmt.Errorf("TransactionsPerMinute must be > 0, got %d", tg.config.TransactionsPerMinute)
	}
	if tg.config.ActiveWalletRatio < 0 || tg.config.ActiveWalletRatio > 1 {
		return fmt.Errorf("ActiveWalletRatio must be between 0 and 1, got %f", tg.config.ActiveWalletRatio)
	}
	if tg.config.MinAmount >= tg.config.MaxAmount {
		return fmt.Errorf("MinAmount (%d) must be < MaxAmount (%d)", tg.config.MinAmount, tg.config.MaxAmount)
	}
	if tg.config.MaxAmount == 0 {
		return fmt.Errorf("MaxAmount must be > 0")
	}
	if tg.config.BurstSize <= 0 {
		return fmt.Errorf("BurstSize must be > 0, got %d", tg.config.BurstSize)
	}
	return nil
}

// Start begins traffic generation
func (tg *TrafficGenerator) Start(ctx context.Context) error {
	tg.mu.Lock()
	if tg.isRunning {
		tg.mu.Unlock()
		return fmt.Errorf("traffic generator already running")
	}

	// Validate configuration to prevent panics
	if err := tg.validateConfig(); err != nil {
		tg.mu.Unlock()
		return fmt.Errorf("invalid configuration: %w", err)
	}

	tg.isRunning = true
	tg.stats.StartTime = time.Now()
	tg.mu.Unlock()

	log.Info(log.Node, "Starting traffic generator",
		"tpm", tg.config.TransactionsPerMinute,
		"burst_size", tg.config.BurstSize,
		"burst_interval", tg.config.BurstInterval)

	go tg.trafficLoop(ctx)
	return nil
}

// Stop halts traffic generation
func (tg *TrafficGenerator) Stop() {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	if !tg.isRunning {
		return
	}

	tg.isRunning = false
	close(tg.stopChan)
	// Recreate the channel for potential restart
	tg.stopChan = make(chan struct{})

	log.Info(log.Node, "Stopped traffic generator",
		"total_transactions", tg.stats.TotalTransactions,
		"runtime", time.Since(tg.stats.StartTime))
}

// trafficLoop is the main traffic generation loop
func (tg *TrafficGenerator) trafficLoop(ctx context.Context) {
	defer func() {
		tg.mu.Lock()
		tg.isRunning = false
		tg.mu.Unlock()
	}()

	// Calculate base transaction interval
	baseInterval := time.Duration(60.0/float64(tg.config.TransactionsPerMinute)) * time.Second

	burstTicker := time.NewTicker(tg.config.BurstInterval)
	defer burstTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info(log.Node, "Traffic generator stopped by context")
			return
		case <-tg.stopChan:
			log.Info(log.Node, "Traffic generator stopped by stop channel")
			return
		case <-burstTicker.C:
			if tg.config.EnableBursts {
				tg.generateBurst(ctx)
			}
		default:
			// Generate regular transaction
			if err := tg.generateTransaction(); err != nil {
				log.Debug(log.Node, "Failed to generate transaction", "error", err)
				tg.incrementFailedTxs()
			}

			// Wait for next transaction
			time.Sleep(baseInterval + tg.calculateJitter())
		}
	}
}

// generateTransaction creates and submits a single transaction
func (tg *TrafficGenerator) generateTransaction() error {
	// Get active wallets
	wallets := tg.getActiveWallets()
	if len(wallets) < 2 {
		return fmt.Errorf("need at least 2 active wallets")
	}

	// Select sender and receiver
	sender := wallets[rand.Intn(len(wallets))]
	receiver := wallets[rand.Intn(len(wallets))]
	for receiver.ID == sender.ID {
		receiver = wallets[rand.Intn(len(wallets))]
	}

	// Determine transaction amount
	amount := tg.calculateTransactionAmount()
	if sender.GetBalance() < amount {
		return fmt.Errorf("sender %s insufficient balance: %d < %d", sender.ID, sender.GetBalance(), amount)
	}

	// Get or generate receiver address
	receiverAddr := tg.getReceiverAddress(receiver)

	// Generate sender address (simplified - use default)
	senderAddr := sender.GetDefaultAddress()

	// Create send amounts
	sendAmounts := []SendAmount{
		{
			Address: receiverAddr,
			Amount:  float64(amount) / 1000000, // Convert to standard units
		},
	}

	// Execute z_sendmany RPC call
	operationID, err := tg.rpcClient.SendMany(senderAddr, sendAmounts)
	if err != nil {
		tg.incrementRPCErrors()
		return fmt.Errorf("z_sendmany failed: %w", err)
	}

	// Update local wallet state (simulate the transaction)
	if err := tg.simulateTransaction(sender, receiver, amount); err != nil {
		log.Warn(log.Node, "Failed to simulate transaction locally", "error", err)
	}

	// Update statistics
	tg.updateTransactionStats(amount, operationID)

	log.Debug(log.Node, "Generated transaction",
		"operation_id", operationID,
		"sender", sender.ID,
		"receiver", receiver.ID,
		"amount", amount,
		"receiver_addr", receiverAddr)

	return nil
}

// generateBurst creates a burst of transactions
func (tg *TrafficGenerator) generateBurst(ctx context.Context) {
	burstSize := tg.config.BurstSize
	log.Debug(log.Node, "Generating transaction burst", "size", burstSize)

	for i := 0; i < burstSize; i++ {
		select {
		case <-ctx.Done():
			return
		case <-tg.stopChan:
			return
		default:
			if err := tg.generateTransaction(); err != nil {
				log.Debug(log.Node, "Burst transaction failed", "index", i, "error", err)
				tg.incrementFailedTxs()
			}

			// Small delay between burst transactions
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// getActiveWallets returns a subset of wallets for transaction generation
func (tg *TrafficGenerator) getActiveWallets() []*OrchardWallet {
	allWallets := tg.walletManager.GetAllWallets()
	if len(allWallets) == 0 {
		return allWallets
	}

	activeCount := int(float64(len(allWallets)) * tg.config.ActiveWalletRatio)

	// Ensure we have at least 2 wallets for transactions, but don't exceed total
	if activeCount < 2 {
		if len(allWallets) >= 2 {
			activeCount = 2
		} else {
			return allWallets
		}
	}
	if activeCount > len(allWallets) {
		activeCount = len(allWallets)
	}

	// Randomly select active wallets
	rand.Shuffle(len(allWallets), func(i, j int) {
		allWallets[i], allWallets[j] = allWallets[j], allWallets[i]
	})

	return allWallets[:activeCount]
}

// calculateTransactionAmount determines the amount for a transaction
func (tg *TrafficGenerator) calculateTransactionAmount() uint64 {
	if tg.config.EnableLargeTransfers && rand.Float64() < tg.config.LargeTransferProb {
		// Generate large transfer
		tg.incrementLargeTransactions()
		half := tg.config.MaxAmount / 2
		if half == 0 {
			return tg.config.MaxAmount
		}
		return half + uint64(rand.Intn(int(half)))
	}

	// Normal transfer
	diff := tg.config.MaxAmount - tg.config.MinAmount
	if diff == 0 {
		return tg.config.MinAmount
	}
	return tg.config.MinAmount + uint64(rand.Intn(int(diff)))
}

// getReceiverAddress gets or generates an address for the receiver
func (tg *TrafficGenerator) getReceiverAddress(receiver *OrchardWallet) string {
	if rand.Float64() < tg.config.NewAddressRate {
		// Generate new address
		newAddr, err := receiver.GenerateAddress(fmt.Sprintf("traffic_%d", time.Now().Unix()))
		if err != nil {
			log.Warn(log.Node, "Failed to generate new address", "error", err)
			return receiver.GetDefaultAddress()
		}
		tg.incrementAddressGenerations()
		return newAddr
	}

	// Use existing address
	return receiver.GetDefaultAddress()
}

// simulateTransaction updates local wallet state to match the RPC call
func (tg *TrafficGenerator) simulateTransaction(sender, receiver *OrchardWallet, amount uint64) error {
	// This is a simplified simulation
	// In practice, we'd wait for the transaction to be confirmed

	// Select notes to spend
	notesToSpend, totalValue, err := sender.SelectNotesForAmount(amount)
	if err != nil {
		return err
	}

	// Calculate change
	changeAmount := totalValue - amount

	// Execute the transfer via wallet manager
	_, err = tg.walletManager.executeTransfer(sender, receiver, notesToSpend, amount, changeAmount)
	if err != nil {
		return err
	}

	if changeAmount > 0 {
		tg.incrementChangeTransactions()
	}

	return nil
}

// calculateJitter adds randomness to transaction timing
func (tg *TrafficGenerator) calculateJitter() time.Duration {
	// Add ±20% jitter to prevent synchronized transactions
	jitterRange := 0.2
	jitter := (rand.Float64() - 0.5) * jitterRange * 2
	baseInterval := time.Duration(60.0/float64(tg.config.TransactionsPerMinute)) * time.Second
	return time.Duration(float64(baseInterval) * jitter)
}

// Statistics update methods
func (tg *TrafficGenerator) updateTransactionStats(amount uint64, operationID string) {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	tg.stats.TotalTransactions++
	tg.stats.SuccessfulTxs++
	tg.stats.RPCCalls++
	tg.stats.SendManyCallsCount++
	tg.stats.TotalVolume += amount
	tg.stats.LastTransactionTime = time.Now()

	// Update average amount
	if tg.stats.SuccessfulTxs > 0 {
		tg.stats.AverageAmount = tg.stats.TotalVolume / tg.stats.SuccessfulTxs
	}

	// Update transactions per minute
	elapsed := time.Since(tg.stats.StartTime)
	if elapsed.Minutes() > 0 {
		tg.stats.AverageTPM = float64(tg.stats.TotalTransactions) / elapsed.Minutes()
	}

	tg.lastTransaction = time.Now()
}

func (tg *TrafficGenerator) incrementFailedTxs() {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.stats.TotalTransactions++
	tg.stats.FailedTxs++
}

func (tg *TrafficGenerator) incrementRPCErrors() {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.stats.RPCCalls++
	tg.stats.RPCErrors++
}

func (tg *TrafficGenerator) incrementLargeTransactions() {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.stats.LargeTransactions++
}

func (tg *TrafficGenerator) incrementChangeTransactions() {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.stats.ChangeTransactions++
}

func (tg *TrafficGenerator) incrementAddressGenerations() {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.stats.AddressGenerations++
}

// GetStats returns current traffic generation statistics
func (tg *TrafficGenerator) GetStats() TrafficStats {
	tg.mu.RLock()
	defer tg.mu.RUnlock()
	return tg.stats
}

// GetDetailedStats returns comprehensive statistics including wallet state
func (tg *TrafficGenerator) GetDetailedStats() map[string]interface{} {
	tg.mu.RLock()
	trafficStats := tg.stats
	tg.mu.RUnlock()

	walletStats := tg.walletManager.GetGlobalStats()

	return map[string]interface{}{
		"traffic_stats": trafficStats,
		"wallet_stats":  walletStats,
		"config":        tg.config,
		"is_running":    tg.isRunning,
	}
}

// SaveReport generates a comprehensive traffic report
func (tg *TrafficGenerator) SaveReport() (string, error) {
	stats := tg.GetDetailedStats()

	report, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal report: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("traffic_report_%s.json", timestamp)

	log.Info(log.Node, "Generated traffic report",
		"filename", filename,
		"total_transactions", tg.stats.TotalTransactions,
		"runtime", time.Since(tg.stats.StartTime))

	return string(report), nil
}