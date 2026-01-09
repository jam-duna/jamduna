// Wallet Demo - Dummy wallet generator and traffic simulator for Orchard
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/colorfulnotion/jam/builder/orchard/wallet"
	log "github.com/colorfulnotion/jam/log"
	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "wallet-demo",
		Short: "Orchard Wallet Demo and Traffic Generator",
		Long: `A demonstration tool that creates dummy Orchard wallets and generates
realistic transaction traffic to test the Orchard RPC endpoints.`,
	}
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Global flags
	var (
		rpcURL        string
		debugModules  string
		walletCount   int
		initialValue  uint64
		notesPerWallet int
		tpm           int
		duration      time.Duration
	)

	// Setup command - creates wallets and initial notes
	var setupCmd = &cobra.Command{
		Use:   "setup",
		Short: "Create dummy wallets with initial notes",
		Run: func(cmd *cobra.Command, args []string) {
			log.InitLogger("debug")
			log.EnableModules(debugModules)

			fmt.Printf("Setting up Orchard wallet demo environment\n")
			fmt.Printf("  Wallets to create: %d\n", walletCount)
			fmt.Printf("  Notes per wallet: %d\n", notesPerWallet)
			fmt.Printf("  Initial value per note: %d\n", initialValue)

			if err := setupWallets(walletCount, notesPerWallet, initialValue); err != nil {
				fmt.Printf("Setup failed: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✅ Setup completed successfully!\n")
		},
	}

	setupCmd.Flags().IntVar(&walletCount, "wallets", 10, "Number of wallets to create")
	setupCmd.Flags().IntVar(&notesPerWallet, "notes", 5, "Initial notes per wallet")
	setupCmd.Flags().Uint64Var(&initialValue, "value", 1000000, "Initial value per note")

	// Traffic command - runs the traffic generator
	var trafficCmd = &cobra.Command{
		Use:   "traffic",
		Short: "Generate transaction traffic",
		Run: func(cmd *cobra.Command, args []string) {
			log.InitLogger("debug")
			log.EnableModules(debugModules)

			fmt.Printf("Starting Orchard traffic generator\n")
			fmt.Printf("  RPC URL: %s\n", rpcURL)
			fmt.Printf("  Wallet count: %d\n", walletCount)
			fmt.Printf("  TPM: %d\n", tpm)
			if duration > 0 {
				fmt.Printf("  Duration: %v\n", duration)
			} else {
				fmt.Printf("  Duration: infinite\n")
			}

			if err := runTrafficGenerator(rpcURL, walletCount, notesPerWallet, initialValue, tpm, duration); err != nil {
				fmt.Printf("Traffic generator failed: %v\n", err)
				os.Exit(1)
			}
		},
	}

	trafficCmd.Flags().StringVar(&rpcURL, "rpc-url", "http://localhost:8232", "Orchard RPC server URL")
	trafficCmd.Flags().IntVar(&walletCount, "wallets", 10, "Number of wallets to create/use")
	trafficCmd.Flags().IntVar(&notesPerWallet, "notes", 5, "Initial notes per wallet")
	trafficCmd.Flags().Uint64Var(&initialValue, "value", 1000000, "Initial value per note")
	trafficCmd.Flags().IntVar(&tpm, "tpm", 15, "Transactions per minute")
	trafficCmd.Flags().DurationVar(&duration, "duration", 0, "Duration to run traffic generator (0 = infinite)")

	// Test command - runs basic functionality tests
	var testCmd = &cobra.Command{
		Use:   "test",
		Short: "Run basic functionality tests",
		Run: func(cmd *cobra.Command, args []string) {
			log.InitLogger("debug")
			log.EnableModules(debugModules)

			fmt.Printf("Running Orchard wallet tests\n")

			if err := runTests(rpcURL); err != nil {
				fmt.Printf("Tests failed: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✅ All tests passed!\n")
		},
	}

	testCmd.Flags().StringVar(&rpcURL, "rpc-url", "http://localhost:8232", "Orchard RPC server URL")

	// Stats command - displays wallet and transaction statistics
	var statsCmd = &cobra.Command{
		Use:   "stats",
		Short: "Display wallet and transaction statistics",
		Run: func(cmd *cobra.Command, args []string) {
			log.InitLogger("info")
			log.EnableModules(debugModules)

			if err := displayStats(walletCount, notesPerWallet, initialValue); err != nil {
				fmt.Printf("Failed to display stats: %v\n", err)
				os.Exit(1)
			}
		},
	}

	statsCmd.Flags().IntVar(&walletCount, "wallets", 10, "Number of wallets")
	statsCmd.Flags().IntVar(&notesPerWallet, "notes", 5, "Notes per wallet")
	statsCmd.Flags().Uint64Var(&initialValue, "value", 1000000, "Value per note")

	// Global flags
	rootCmd.PersistentFlags().StringVar(&debugModules, "debug", "wallet,traffic", "Debug modules to enable")

	// Add commands
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(trafficCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(statsCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// setupWallets creates the wallet infrastructure
func setupWallets(walletCount, notesPerWallet int, initialValue uint64) error {
	fmt.Printf("\n[1/3] Creating wallet manager...\n")
	manager := wallet.NewWalletManager()

	fmt.Printf("\n[2/3] Creating %d wallets...\n", walletCount)
	for i := 0; i < walletCount; i++ {
		walletName := fmt.Sprintf("demo_wallet_%d", i+1)
		w, err := manager.CreateWallet(walletName)
		if err != nil {
			return fmt.Errorf("failed to create wallet %d: %w", i+1, err)
		}

		// Generate additional addresses for each wallet
		for j := 0; j < 3; j++ {
			addrLabel := fmt.Sprintf("addr_%d", j+1)
			_, err := w.GenerateAddress(addrLabel)
			if err != nil {
				return fmt.Errorf("failed to generate address for wallet %s: %w", w.ID, err)
			}
		}

		fmt.Printf("  ✓ Created wallet: %s (ID: %s)\n", walletName, w.ID)
	}

	fmt.Printf("\n[3/3] Generating initial notes...\n")
	if err := manager.GenerateInitialNotes(notesPerWallet, initialValue); err != nil {
		return fmt.Errorf("failed to generate initial notes: %w", err)
	}

	// Display summary
	stats := manager.GetGlobalStats()
	fmt.Printf("\n📊 Setup Summary:\n")
	fmt.Printf("  Total Wallets: %v\n", stats["total_wallets"])
	fmt.Printf("  Total Balance: %v\n", stats["total_balance"])
	fmt.Printf("  Total Notes: %v\n", stats["total_notes"])
	fmt.Printf("  Average Balance: %v\n", stats["average_balance"])

	return nil
}

// runTrafficGenerator starts the traffic generation process
func runTrafficGenerator(rpcURL string, walletCount, notesPerWallet int, initialValue uint64, tpm int, duration time.Duration) error {
	// Setup wallet infrastructure
	fmt.Printf("\n[1/4] Setting up wallets...\n")
	manager := wallet.NewWalletManager()

	for i := 0; i < walletCount; i++ {
		walletName := fmt.Sprintf("traffic_wallet_%d", i+1)
		w, err := manager.CreateWallet(walletName)
		if err != nil {
			return fmt.Errorf("failed to create wallet %d: %w", i+1, err)
		}

		// Generate additional addresses
		for j := 0; j < 2; j++ {
			addrLabel := fmt.Sprintf("traffic_addr_%d", j+1)
			w.GenerateAddress(addrLabel)
		}
	}

	if err := manager.GenerateInitialNotes(notesPerWallet, initialValue); err != nil {
		return fmt.Errorf("failed to generate initial notes: %w", err)
	}

	// Setup RPC client
	fmt.Printf("\n[2/4] Connecting to RPC server...\n")
	var rpcClient wallet.RPCClient

	if rpcURL == "mock" || rpcURL == "" {
		fmt.Printf("  Using mock RPC client for testing\n")
		rpcClient = wallet.NewMockRPCClient()
	} else {
		fmt.Printf("  Connecting to: %s\n", rpcURL)
		realClient := wallet.NewOrchardRPCClient(rpcURL)

		// Test connection
		if err := realClient.Ping(); err != nil {
			fmt.Printf("  Warning: RPC connection failed: %v\n", err)
			fmt.Printf("  Falling back to mock client\n")
			rpcClient = wallet.NewMockRPCClient()
		} else {
			fmt.Printf("  ✓ RPC connection successful\n")
			rpcClient = realClient
		}
	}

	// Setup traffic generator
	fmt.Printf("\n[3/4] Configuring traffic generator...\n")
	trafficGen := wallet.NewTrafficGenerator(manager, rpcClient)

	// Configure traffic patterns
	config := wallet.TrafficConfig{
		TransactionsPerMinute: tpm,
		BurstSize:            3,
		BurstInterval:        45 * time.Second,
		MinAmount:            1000,
		MaxAmount:            500000,
		ChangeThreshold:      0.8,
		ActiveWalletRatio:    0.9,
		NewAddressRate:       0.15,
		EnableBursts:         true,
		EnableLargeTransfers: true,
		LargeTransferProb:    0.1,
	}
	trafficGen.SetConfig(config)

	fmt.Printf("  Transaction Rate: %d TPM\n", config.TransactionsPerMinute)
	fmt.Printf("  Burst Configuration: %d transactions every %v\n", config.BurstSize, config.BurstInterval)
	fmt.Printf("  Amount Range: %d - %d\n", config.MinAmount, config.MaxAmount)

	// Start traffic generation
	fmt.Printf("\n[4/4] Starting traffic generation...\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := trafficGen.Start(ctx); err != nil {
		return fmt.Errorf("failed to start traffic generator: %w", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start statistics reporting
	statsTicker := time.NewTicker(30 * time.Second)
	defer statsTicker.Stop()

	// Setup duration timer if specified
	var durationTimer *time.Timer
	if duration > 0 {
		durationTimer = time.NewTimer(duration)
		defer durationTimer.Stop()
		fmt.Printf("✅ Traffic generator started! Will run for %v. Press Ctrl+C to stop early.\n", duration)
	} else {
		fmt.Printf("✅ Traffic generator started! Press Ctrl+C to stop.\n")
	}
	fmt.Printf("📊 Statistics will be displayed every 30 seconds.\n\n")

	for {
		select {
		case <-sigChan:
			fmt.Printf("\n🛑 Shutdown signal received...\n")
			cancel()
			trafficGen.Stop()

			// Generate final report
			report, err := trafficGen.SaveReport()
			if err != nil {
				fmt.Printf("Failed to generate final report: %v\n", err)
			} else {
				fmt.Printf("📋 Final Report:\n%s\n", report)
			}

			return nil

		case <-func() <-chan time.Time {
			if durationTimer != nil {
				return durationTimer.C
			}
			return make(chan time.Time) // Never fires
		}():
			fmt.Printf("\n⏰ Duration expired (%v), shutting down...\n", duration)
			cancel()
			trafficGen.Stop()

			// Generate final report
			report, err := trafficGen.SaveReport()
			if err != nil {
				fmt.Printf("Failed to generate final report: %v\n", err)
			} else {
				fmt.Printf("📋 Final Report:\n%s\n", report)
			}

			return nil

		case <-statsTicker.C:
			displayTrafficStats(trafficGen)
		}
	}
}

// runTests executes basic functionality tests
func runTests(rpcURL string) error {
	fmt.Printf("\n[1/5] Testing wallet creation...\n")
	manager := wallet.NewWalletManager()

	// Test wallet creation
	testWallet, err := manager.CreateWallet("test_wallet")
	if err != nil {
		return fmt.Errorf("wallet creation failed: %w", err)
	}
	fmt.Printf("  ✓ Wallet created: %s\n", testWallet.ID)

	// Test address generation
	fmt.Printf("\n[2/5] Testing address generation...\n")
	for i := 0; i < 3; i++ {
		addr, err := testWallet.GenerateAddress(fmt.Sprintf("test_addr_%d", i))
		if err != nil {
			return fmt.Errorf("address generation failed: %w", err)
		}
		fmt.Printf("  ✓ Generated address: %s\n", addr)
	}

	// Test note management
	fmt.Printf("\n[3/5] Testing note management...\n")
	if err := manager.GenerateInitialNotes(3, 100000); err != nil {
		return fmt.Errorf("initial note generation failed: %w", err)
	}

	balance := testWallet.GetBalance()
	fmt.Printf("  ✓ Initial balance: %d\n", balance)

	unspentNotes := testWallet.GetUnspentNotes()
	fmt.Printf("  ✓ Unspent notes: %d\n", len(unspentNotes))

	// Test RPC client
	fmt.Printf("\n[4/5] Testing RPC client...\n")
	mockClient := wallet.NewMockRPCClient()

	sendAmounts := []wallet.SendAmount{
		{Address: "test_address_123", Amount: 0.1},
	}

	opID, err := mockClient.SendMany("from_address", sendAmounts)
	if err != nil {
		return fmt.Errorf("RPC SendMany test failed: %w", err)
	}
	fmt.Printf("  ✓ z_sendmany successful: %s\n", opID)

	mempoolInfo, err := mockClient.GetMempoolInfo()
	if err != nil {
		return fmt.Errorf("RPC GetMempoolInfo test failed: %w", err)
	}
	fmt.Printf("  ✓ z_getmempoolinfo successful: %d transactions\n", mempoolInfo["size"])

	// Test traffic generator
	fmt.Printf("\n[5/5] Testing traffic generator...\n")
	trafficGen := wallet.NewTrafficGenerator(manager, mockClient)

	config := wallet.DefaultTrafficConfig()
	config.TransactionsPerMinute = 60 // High rate for quick test
	trafficGen.SetConfig(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := trafficGen.Start(ctx); err != nil {
		return fmt.Errorf("traffic generator start failed: %w", err)
	}

	// Wait for some transactions
	time.Sleep(3 * time.Second)
	trafficGen.Stop()

	stats := trafficGen.GetStats()
	fmt.Printf("  ✓ Generated %d transactions in test run\n", stats.TotalTransactions)

	return nil
}

// displayStats shows current wallet and transaction statistics
func displayStats(walletCount, notesPerWallet int, initialValue uint64) error {
	// Create a sample environment for stats
	manager := wallet.NewWalletManager()

	for i := 0; i < walletCount; i++ {
		walletName := fmt.Sprintf("stats_wallet_%d", i+1)
		w, err := manager.CreateWallet(walletName)
		if err != nil {
			return fmt.Errorf("failed to create wallet for stats: %w", err)
		}

		// Generate additional addresses
		w.GenerateAddress("primary")
		w.GenerateAddress("secondary")
	}

	if err := manager.GenerateInitialNotes(notesPerWallet, initialValue); err != nil {
		return fmt.Errorf("failed to generate notes for stats: %w", err)
	}

	// Perform some sample transactions
	fmt.Printf("Generating sample transactions for realistic stats...\n")
	for i := 0; i < 5; i++ {
		_, err := manager.PerformRandomTransfer(10000, 100000)
		if err != nil {
			fmt.Printf("  Warning: Sample transfer %d failed: %v\n", i+1, err)
		}
	}

	// Display comprehensive statistics
	fmt.Printf("\n📊 Orchard Wallet Demo Statistics\n")
	fmt.Printf("==================================\n\n")

	// Global stats
	globalStats := manager.GetGlobalStats()
	fmt.Printf("🏦 Global Wallet Statistics:\n")
	fmt.Printf("  Total Wallets: %v\n", globalStats["total_wallets"])
	fmt.Printf("  Total Balance: %v\n", globalStats["total_balance"])
	fmt.Printf("  Total Notes: %v\n", globalStats["total_notes"])
	fmt.Printf("  Average Balance per Wallet: %v\n", globalStats["average_balance"])
	fmt.Printf("  Last Activity: %v\n", globalStats["last_activity"])

	// Individual wallet details
	fmt.Printf("\n👛 Individual Wallet Details:\n")
	wallets := manager.GetAllWallets()
	for i, w := range wallets {
		if i >= 3 { // Limit display to first 3 wallets
			fmt.Printf("  ... and %d more wallets\n", len(wallets)-3)
			break
		}

		stats := w.GetStats()
		addresses := w.GetAddresses()
		fmt.Printf("  Wallet %s:\n", stats["name"])
		fmt.Printf("    ID: %s\n", stats["wallet_id"])
		fmt.Printf("    Balance: %v\n", stats["balance"])
		fmt.Printf("    Unspent Notes: %v\n", stats["unspent_notes"])
		fmt.Printf("    Total Notes Received: %v\n", stats["total_notes"])
		fmt.Printf("    Addresses: %d\n", len(addresses))
		fmt.Printf("    Last Activity: %v\n", stats["last_activity"])
		fmt.Printf("\n")
	}

	// Configuration info
	fmt.Printf("⚙️ Configuration Used:\n")
	fmt.Printf("  Wallet Count: %d\n", walletCount)
	fmt.Printf("  Notes per Wallet: %d\n", notesPerWallet)
	fmt.Printf("  Initial Value per Note: %d\n", initialValue)
	fmt.Printf("  Total Initial Value: %d\n", uint64(walletCount*notesPerWallet)*initialValue)

	return nil
}

// displayTrafficStats shows real-time traffic generation statistics
func displayTrafficStats(trafficGen *wallet.TrafficGenerator) {
	stats := trafficGen.GetDetailedStats()

	trafficStats := stats["traffic_stats"].(wallet.TrafficStats)
	walletStats := stats["wallet_stats"].(map[string]interface{})

	fmt.Printf("\n📈 Live Traffic Statistics (updated every 30s)\n")
	fmt.Printf("================================================\n")
	fmt.Printf("🚀 Transaction Generation:\n")
	fmt.Printf("  Total Transactions: %d\n", trafficStats.TotalTransactions)
	fmt.Printf("  Successful: %d\n", trafficStats.SuccessfulTxs)
	fmt.Printf("  Failed: %d\n", trafficStats.FailedTxs)
	fmt.Printf("  Success Rate: %.1f%%\n",
		float64(trafficStats.SuccessfulTxs)/float64(trafficStats.TotalTransactions)*100)
	fmt.Printf("  Average TPM: %.2f\n", trafficStats.AverageTPM)

	fmt.Printf("\n💰 Transaction Volume:\n")
	fmt.Printf("  Total Volume: %d\n", trafficStats.TotalVolume)
	fmt.Printf("  Average Amount: %d\n", trafficStats.AverageAmount)
	fmt.Printf("  Large Transfers: %d\n", trafficStats.LargeTransactions)
	fmt.Printf("  Transactions with Change: %d\n", trafficStats.ChangeTransactions)

	fmt.Printf("\n🌐 RPC Activity:\n")
	fmt.Printf("  Total RPC Calls: %d\n", trafficStats.RPCCalls)
	fmt.Printf("  RPC Errors: %d\n", trafficStats.RPCErrors)
	fmt.Printf("  z_sendmany Calls: %d\n", trafficStats.SendManyCallsCount)
	fmt.Printf("  Bundle Submissions: %d\n", trafficStats.BundleSubmissions)
	fmt.Printf("  New Addresses Generated: %d\n", trafficStats.AddressGenerations)

	fmt.Printf("\n🏦 Wallet State:\n")
	fmt.Printf("  Active Wallets: %v\n", walletStats["total_wallets"])
	fmt.Printf("  Total Balance: %v\n", walletStats["total_balance"])
	fmt.Printf("  Total Notes: %v\n", walletStats["total_notes"])

	runtime := time.Since(trafficStats.StartTime)
	fmt.Printf("\n⏱️ Runtime: %v\n", runtime.Round(time.Second))
	fmt.Printf("────────────────────────────────────────────────\n")
}