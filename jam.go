package main

import (
	"encoding/json"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"

	"fmt"

	"github.com/colorfulnotion/jam/chainspecs"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/pvm/interpreter"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/telemetry"
	"github.com/colorfulnotion/jam/types"
	"github.com/spf13/cobra"

	// orchardrpc "github.com/colorfulnotion/jam/builder/orchard/rpc" // Disabled: requires librailgun_service FFI

	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

func main() {

	// cmd
	var rootCmd = &cobra.Command{
		Use:   "./jamduna",
		Short: "JAM DUNA node",
	}
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	var (
		help       bool
		configPath string
		temp       bool
		version    bool

		//run flags
		dataPath       string
		chainSpec      string
		Port           int
		RPCPort        int
		validatorIndex int
		debug          string
		start_time     string
		role           string // "validator" or "builder"

		logLevel string

		// run flags that is not supported yet
		pvmBackend        string
		peerID            int
		externalIP        string
		listenIP          string
		rpcListenIP       string
		bootnode          string
		telemetryEndpoint string

		// serviceIDs should trigger role=builder
		serviceIDs    []uint32
		serviceIDsStr string

		// run variables
		validatorIndexFlagSet bool
		start_timeFlagSet     bool
	)

	var (
		helpFlag     = "help"
		logLevelFlag = "log-level"
		tempFlag     = "temp"
		versionFlag  = "version"

		// run flags
		dataPathFlag       = "data-path"
		PortFlag           = "port"
		RPCPortFlag        = "rpc-port"
		startTimeFlag      = "start-time"
		validatorIndexFlag = "dev-validator"
		debugFlag          = "debug"
		chainSpecFlag      = "chain"
		serviceIDsFlag     = "services"

		roleFlag = "role"

		//run flags that is not supported yet
		pvmBackendFlag  = "pvm-backend"
		peerIDFlag      = "peer-id"
		externalIPFlag  = "external-ip"
		listenIPFlag    = "listen-ip"
		rpcListenIPFlag = "rpc-listen-ip"
		bootnodeFlag    = "bootnode"
		telemetryFlag   = "telemetry"
	)
	rootCmd.PersistentFlags().BoolVarP(&help, helpFlag, "h", false, "Displays help information about the commands and flags.")
	rootCmd.PersistentFlags().StringVarP(&logLevel, logLevelFlag, "l", "debug", "Log level (trace, debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVarP(&temp, tempFlag, "t", false, "Use a temporary data directory, removed on exit. Conflicts with data-path")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Path to the config file")
	rootCmd.PersistentFlags().BoolVarP(&version, versionFlag, "v", false, "Prints the version of the program.")

	//gen-keys
	var genKeysCmd = &cobra.Command{
		Use:   "gen-keys",
		Short: "Generate keys for validators, pls generate keys for all validators before running the node",
		Run: func(cmd *cobra.Command, args []string) {
			// Generate keys for validators (TotalValidators + 1 for the builder/full node)
			_, _, err := grandpa.GenerateValidatorSecretSetToPath(types.TotalValidators+1, true, dataPath)
			if err != nil {
				fmt.Printf("Error generating keys: %s\n", err)
				os.Exit(1)
			}
			keysPath := filepath.Join(dataPath, "keys")
			fmt.Printf("Successfully generated %d validator keys in %s\n", types.TotalValidators+1, keysPath)
		},
	}

	// list-keys
	var listKeysCmd = &cobra.Command{
		Use:   "list-keys",
		Short: "List keys for validators",
		Run: func(cmd *cobra.Command, args []string) {
			keysPath := filepath.Join(dataPath, "keys")
			files, err := os.ReadDir(keysPath)
			if err != nil {
				fmt.Printf("Error reading keys directory: %s\n", err)
				os.Exit(1)
			}

			for _, file := range files {
				seedFile := filepath.Join(keysPath, file.Name())
				seed, err := os.ReadFile(seedFile)
				if err != nil {
					fmt.Printf("Error reading seed file: %s\n", err)
					os.Exit(1)
				}
				seed = seed[:32]

				validator, err := grandpa.GenerateValidatorPubKeyFromSeed(seed)
				if err != nil {
					fmt.Printf("Error generating validator from seed: %s\n", err)
					os.Exit(1)
				}
				fmt.Println("--------------------------------------------------")
				fmt.Printf("%-14s %s\n", "file:", file.Name())
				fmt.Printf("%-14s %x\n", "seed:", seed)
				fmt.Printf("%-14s %v\n", "ed25519:", validator.Ed25519)
				fmt.Printf("%-14s %s\n", "bandersnatch:", common.BytesToHexStr(validator.Bandersnatch[:]))
				fmt.Printf("%-14s %s\n", "bls:", common.BytesToHexStr(validator.Bls[:]))
				fmt.Printf("%-14s %s\n", "metadata:", common.BytesToHexStr(validator.Metadata[:]))
				fmt.Printf("%-14s %s\n\n", "dns_alt_name:", common.ToSAN(validator.Ed25519[:]))
			}
		},
	}

	var runCmdSTF = &cobra.Command{
		Use:   "test-stf <input.json>",
		Args:  cobra.ExactArgs(1),
		Short: "Run the STF Validation",
		Run: func(cmd *cobra.Command, args []string) {
			stfFile := args[0]
			outputFile := "poststate.json"
			interpreter.PvmLogging = true
			log.InitLogger("debug")
			log.EnableModule(log.PvmAuthoring)
			log.EnableModule(log.PvmValidating)
			log.EnableModule(log.GeneralAuthoring)
			log.EnableModule(log.GeneralValidating)
			_, err := statedb.ValidateStateTransitionFile(stfFile, dataPath, outputFile)
			if err != nil {
				fmt.Printf("Error running STF Validation: %s\n", err)
				os.Exit(1)
			}
		},
	}

	// test-stf flag used
	runCmdSTF.Flags().StringVarP(&dataPath, dataPathFlag, "d", filepath.Join(os.Getenv("HOME"), ".jamduna"), "Specifies the directory for the blockchain, keystore, and other data.")

	var testRefineCmd = &cobra.Command{
		Use:   "test-refine",
		Short: "Run the refine test",
		Run: func(cmd *cobra.Command, args []string) {
			// Run the refine test
			fmt.Printf("not implemented yet")
			os.Exit(1)
		},
	}
	// test-refine flag used
	// test case : ./jam gen-spec chainspecs.json jamduna-spec.json
	var genSpecCmd = &cobra.Command{
		Use:   "gen-spec <input.json> <output.json>",
		Short: "Generate new chain spec from the spec config",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			inputFile := args[0]
			outputFile := args[1]

			data, err := os.ReadFile(inputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to read input file: %v\n", err)
				os.Exit(1)
			}

			var devCfg chainspecs.DevConfig
			if err := json.Unmarshal(data, &devCfg); err != nil {
				fmt.Fprintf(os.Stderr, "failed to unmarshal input JSON: %v\n", err)
				os.Exit(1)
			}

			chainSpec, err := chainspecs.GenSpec(devCfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to generate spec: %v\n", err)
				os.Exit(1)
			}

			outBytes, err := json.MarshalIndent(chainSpec, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to marshal output JSON: %v\n", err)
				os.Exit(1)
			}

			if err := os.WriteFile(outputFile, outBytes, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "failed to write output file: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Chain spec written to %s\n", outputFile)
		},
	}
	// gen-spec flag used

	// test case : ./jam print-spec chainspecs.json
	var printSpecCmd = &cobra.Command{
		Use:   "print-spec <input.json>",
		Short: "Generate new chain spec from the spec config",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			inputFile := args[0]
			chainSpecData, err := chainspecs.ReadSpec(inputFile)
			if err != nil {
				fmt.Printf("ReadSpec ERR %s", err)
				os.Exit(1)
			}
			node.PrintSpec(chainSpecData)
		},
	}
	// gen-spec flag used

	// run node command
	var runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run the JAM DUNA node",
		Run: func(cmd *cobra.Command, args []string) {

			if cmd.Flags().Changed(validatorIndexFlag) {
				validatorIndexFlagSet = true
			}
			if cmd.Flags().Changed(startTimeFlag) {
				start_timeFlagSet = true
			}
			if cmd.Flags().Changed(logLevelFlag) {
				logLevel, _ = cmd.Flags().GetString("log_level")
				if logLevel == "debug" {
					monitor = true
				}
			}
			if cmd.Flags().Changed(dataPathFlag) {
				dataPath, _ = cmd.Flags().GetString(dataPathFlag)
			}
			if cmd.Flags().Changed(RPCPortFlag) {
				node.WSPort = RPCPort
			}
			if cmd.Flags().Changed(serviceIDsFlag) {
				parts := strings.Split(serviceIDsStr, ",")
				serviceIDs = make([]uint32, 0, len(parts))
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p == "" {
						continue
					}
					val, err := strconv.ParseUint(p, 10, 32)
					if err != nil {
						fmt.Printf("Invalid service ID '%s': %v\n", p, err)
						os.Exit(1)
					}
					serviceIDs = append(serviceIDs, uint32(val))
				}
			}

			fmt.Printf("Running JAM DUNA node with the following flags:\n")
			fmt.Printf("\033[33m  PVM Backend: %s\n", pvmBackend)
			fmt.Printf("  Data Path: %s\n", dataPath)
			fmt.Printf("  Port: %d, RPC Port: %d\n", Port, RPCPort)
			fmt.Printf("  Validator Index: %d\n", validatorIndex)
			fmt.Printf("  Chain Spec: %s\n", chainSpec)
			fmt.Printf("  Log Level: %s, Debug: %s\n", logLevel, debug)
			fmt.Printf("  Start Time: %s\033[0m\n", start_time)

			var err error
			var validators []types.Validator

			var selfSecret types.ValidatorSecret
			// Run the JAM DUNA node
			now := time.Now()
			loc := now.Location()
			interpreter.PvmLogging = false
			log.InitLogger(logLevel)

			log.EnableModule(log.PvmAuthoring)
			log.EnableModule(log.PvmValidating)
			log.EnableModule(log.FirstGuarantor)
			log.EnableModule(log.OtherGuarantor)
			log.EnableModule(log.GeneralAuthoring)
			log.EnableModule(log.GeneralValidating)
			log.EnableModules(debug)
			var peers []string
			var peerList map[uint16]*node.Peer

			fmt.Printf("\033[34mRead spec %s...\033[0m\n", chainSpec)
			chainSpecData, err := chainspecs.ReadSpec(chainSpec)
			if err != nil {
				fmt.Printf("ReadSpec ERR %s", err)
				os.Exit(1)
			}
			peerList = make(map[uint16]*node.Peer)

			validators, err = getValidatorFromChainSpec(*chainSpecData)
			if err != nil {
				fmt.Printf("getValidatorFromChainSpec ERR %s", err)
				os.Exit(1)
			}
			for i, v := range validators {
				ip_addr, port, err := common.MetadataToAddress(v.Metadata[:])
				peerAddr := fmt.Sprintf("127.0.0.1:%d", 40000+i)
				if err == nil {
					peerAddr = fmt.Sprintf("%s:%d", ip_addr, port)
				} else {
					fmt.Printf("Error extracting metadata: %s, using default setting...\n", err)
				}
				peerList[uint16(i)] = &node.Peer{
					PeerID:    uint16(i),
					PeerAddr:  peerAddr,
					Validator: v,
				}
			}
			peers = make([]string, 0)
			selfSecret = CheckValidatorInfo(validatorIndex, peerList, dataPath)
			for _, peer := range peerList {
				peers = append(peers, fmt.Sprintf("%v", peer.Validator.Ed25519))
			}
			if validatorIndexFlagSet {
				Port = 40000 + validatorIndex // fix later with a good metadata abstraction that works
			}
			dataPath = filepath.Join(dataPath, "jam-"+strconv.Itoa(validatorIndex))

			// to make sure our genesis timestamp is not too far from polkajam+javajam setup
			if start_timeFlagSet {
				for len(start_time) > 0 && (start_time[0] < '0' || start_time[0] > '9') {
					start_time = start_time[1:]
				}
				if len(start_time) > 0 && start_time[len(start_time)-1] == ' ' {
					start_time = start_time[:len(start_time)-1]
				}

				startTime, err := time.ParseInLocation("2006-01-02 15:04:05", start_time, loc)
				if err != nil {
					fmt.Printf("start_time: %s\n", start_time)
					fmt.Println("Invalid time format. Use YYYY-MM-DD HH:MM:SS")
					return
				}

				duration := time.Until(startTime)
				if duration <= 0 {
					fmt.Println("Start time already passed. Running now...")
				} else {
					fmt.Printf("Waiting until start time: %s (%v seconds remaining)\n",
						startTime.Format("2006-01-02 15:04:05"), duration.Seconds())
					const logInterval = 20 * time.Second

					for time.Until(startTime) > logInterval {
						fmt.Printf("Time remaining: %v\n", time.Until(startTime).Truncate(time.Second))
						time.Sleep(logInterval)
					}

					finalSleep := time.Until(startTime)
					if finalSleep > 0 {
						time.Sleep(finalSleep)
					}
					fmt.Println("Start time reached. Running now...")
				}
			}

			// TODO: take in serviceids for multi-rollup support
			// Normalize role string
			nodeRole := types.RoleValidator
			if role == "builder" {
				nodeRole = types.RoleBuilder
			}

			n, err := node.NewNode(uint16(validatorIndex), selfSecret, chainSpecData, pvmBackend, peers, peerList, dataPath, Port, nodeRole)
			if err != nil {
				fmt.Printf("New Node Err:%s", err.Error())
				os.Exit(1)
			}
			n.SetServiceDir("/services")

			// Initialize telemetry if endpoint is specified
			if telemetryEndpoint != "" {
				if err := n.InitTelemetry(telemetryEndpoint); err != nil {
					fmt.Printf("Warning: Failed to initialize telemetry: %v\n", err)
				} else {
					fmt.Printf("Telemetry enabled: %s\n", telemetryEndpoint)
				}
			}
			n.WriteDebugFlag = true
			storage, err := n.GetStorage()
			if err != nil {
				fmt.Printf("GetStorage Err:%s", err.Error())
				os.Exit(1)
			}
			defer storage.Close()

			fmt.Printf("New Node %d started, edkey %v, port%d, time:%s. buildVersion=%v pvm_backend=%v\n", validatorIndex, selfSecret.Ed25519Pub, Port, time.Now().String(), n.GetBuild(), pvmBackend)
			StartRuntimeMonitor(30 * time.Second)
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				// just do nothing ...
			}
		},
	}

	// data path
	runCmd.Flags().StringVarP(&dataPath, dataPathFlag, "d", filepath.Join(os.Getenv("HOME"), ".jamduna"), "Specifies the directory for the blockchain, keystore, and other data.")
	runCmd.Flags().IntVar(&Port, PortFlag, node.GetJAMNetworkPort(), "Specifies the network listening port.")
	runCmd.Flags().IntVar(&RPCPort, RPCPortFlag, node.GetJAMNetworkWSPort(), "Specifies the RPC listening port.")
	runCmd.Flags().StringVar(&start_time, startTimeFlag, "", "Start time in format: YYYY-MM-DD HH:MM:SS")
	runCmd.Flags().IntVar(&validatorIndex, validatorIndexFlag, 0, "Validator Index (only for development)")
	runCmd.Flags().StringVar(&debug, debugFlag, "r,g", "Specifies debug flags for enhanced logging (block,guarantees,rotation,assurances,audit,da,node,quic,beefy,audit,grandpa,web,state)")
	runCmd.Flags().StringVar(&chainSpec, chainSpecFlag, "chainspec.json", `Chain to run. "polkadot", "dev", or the path of a chain spec file`)

	desc := flagDescription("The PVM backend to use", map[string]string{
		pvm.BackendInterpreter: "Use a PVM interpreter. Slow, but works everywhere",
		pvm.BackendCompiler:    "Use a PVM compiler. Fast, but is Linux-only",
	})
	runCmd.Flags().StringVar(&pvmBackend, pvmBackendFlag, pvm.BackendInterpreter, desc)
	runCmd.Flags().IntVar(&peerID, peerIDFlag, 0, "Peer ID of this node. If not specified, a new peer ID will be generated. The corresponding secret key will not be persisted.")
	runCmd.Flags().StringVar(&externalIP, externalIPFlag, "", "External IP of this node, as used by other nodes to connect. If not specified, this will be guessed.")
	runCmd.Flags().StringVar(&listenIP, listenIPFlag, "", "IP address to listen on. `::` (the default) means all addresses. [default: ::]")
	runCmd.Flags().StringVar(&rpcListenIP, rpcListenIPFlag, "", "IP address for RPC server to listen on. `::` (the default) means all addresses. [default: ::]")
	runCmd.Flags().StringVar(&bootnode, bootnodeFlag, "", "Specify a bootnode")
	runCmd.Flags().StringVar(&telemetryEndpoint, telemetryFlag, "", " Send data to TART server (JIP-3)")
	runCmd.Flags().StringVar(&role, roleFlag, "validator", "Node role: 'validator' (default) or 'builder' (full node that doesn't participate in consensus)")
	runCmd.Flags().StringVar(&serviceIDsStr, serviceIDsFlag, "", "Comma-separated list of service IDs to enable builder role")

	// telemetry command (server mode)
	var telemetryAddr string
	var telemetryLogFile string
	var telemetryServerCmd = &cobra.Command{
		Use:   "telemetry",
		Short: "Run a telemetry server to receive events from polkajam and jamduna nodes",
		Long: `Run a telemetry server that listens for telemetry connections from JAM nodes.

Example usage:
  # Start telemetry server on default port 9999
  ./jamduna telemetry

  # Start with custom address and log file
  ./jamduna telemetry --addr 0.0.0.0:9999 --log /tmp/telemetry.log

  # Connect polkajam to this server
  ./polkajam run --telemetry localhost:9999 ...

  # Connect jamduna to this server (future support)
  ./jamduna run --telemetry localhost:9999 ...`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Starting telemetry server on %s, logging to %s\n", telemetryAddr, telemetryLogFile)

			server, err := telemetry.NewTelemetryServer(telemetryAddr, telemetryLogFile)
			if err != nil {
				fmt.Printf("Failed to create telemetry server: %v\n", err)
				os.Exit(1)
			}

			// Handle graceful shutdown
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				<-sigChan
				fmt.Println("\nShutting down telemetry server...")
				server.Stop()
				os.Exit(0)
			}()

			if err := server.Start(); err != nil {
				fmt.Printf("Telemetry server error: %v\n", err)
				os.Exit(1)
			}
		},
	}
	telemetryServerCmd.Flags().StringVar(&telemetryAddr, "addr", "0.0.0.0:9999", "Address to listen on for telemetry connections")
	telemetryServerCmd.Flags().StringVar(&telemetryLogFile, "log", filepath.Join(os.TempDir(), "jamduna-telemetry.log"), "Path to telemetry log file")

	// add commands to root
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(genKeysCmd)
	rootCmd.AddCommand(listKeysCmd)
	rootCmd.AddCommand(runCmdSTF)
	rootCmd.AddCommand(testRefineCmd)
	rootCmd.AddCommand(genSpecCmd)
	rootCmd.AddCommand(printSpecCmd)
	rootCmd.AddCommand(telemetryServerCmd)

	// parse the persistent flags (Global flags)
	rootCmd.PersistentFlags().Parse(os.Args[1:])
	if version {
		fmt.Printf("Version: %s, Commit: %s, BuildTime: %s\n", Version, Commit, BuildTime)
		os.Exit(0)
	}
	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error executing command:", err)
		os.Exit(1)
	}
	// fmt.Println("JAM DUNA node started")

}

func CheckValidatorInfo(validatorIndex int, peerList map[uint16]*node.Peer, dataPath string) types.ValidatorSecret {
	// get the seed from the data dir
	dataPath = filepath.Join(dataPath, "keys") // store the keys in a subdir
	seedFile := filepath.Join(dataPath, fmt.Sprintf("seed_%d", validatorIndex))
	seed, err := os.ReadFile(seedFile)
	if err != nil {
		fmt.Printf("Error reading seed file: %s %v", seedFile, err)
		os.Exit(1)
	}
	seed = seed[:32]
	// generate the validator from the seed
	selfSecrets, err := grandpa.InitValidatorSecret(seed, seed, seed, []byte{})
	if err != nil {
		fmt.Printf("CheckValidatorInfo err %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("validatorIndex %d out of %d peers\n", validatorIndex, len(peerList))

	// For builder/full nodes (validatorIndex >= TotalValidators), skip the chainspec validation
	// since they are not part of the active validator set
	if validatorIndex < types.TotalValidators {
		if selfSecrets.BandersnatchPub != peerList[uint16(validatorIndex)].Validator.Bandersnatch {
			fmt.Printf("Error: seed file does not match the metadata. self.BandersnatchPub=0x%x | peerList[%v].BandersnatchPub=0x%x", selfSecrets.BandersnatchPub, validatorIndex, peerList[uint16(validatorIndex)].Validator.Bandersnatch)
			os.Exit(1)
		}
	} else {
		//fmt.Printf("Builder/full node mode: validator index %d is outside active validator set (0-%d), skipping chainspec validation\n", validatorIndex, types.TotalValidators-1)
	}
	return selfSecrets
}

func StartRuntimeMonitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		var mem runtime.MemStats
		var highest, count int

		// Immediate first report
		const asMB = 1024 * 1024
		runtime.ReadMemStats(&mem)
		count = runtime.NumGoroutine()
		highest = count
		allocMB := mem.Alloc / asMB
		// if mem > 4GB panic

		totalAllocMB := mem.TotalAlloc / asMB
		sysMB := mem.Sys / asMB
		if monitor {
			fmt.Printf("%-22s 🧠 Memory:%4dMB | 💾 TotalAlloc:%4dMB | 📦 Sys:%4dMB | ♻️ GC:%4d | 🧵 Goroutines:%4d\n",
				"[MONITOR New Record]", allocMB, totalAllocMB, sysMB, mem.NumGC, count)
		}

		// Then on every tick
		for range ticker.C {
			runtime.ReadMemStats(&mem)
			count = runtime.NumGoroutine()
			allocMB = mem.Alloc / asMB
			totalAllocMB = mem.TotalAlloc / asMB
			sysMB = mem.Sys / asMB
			if allocMB > 4096 {
				fmt.Printf("❗ Memory usage is too high: %dMB\n", allocMB)
				// print stack trace
				// buf := make([]byte, 1<<20)
				// stackSize := runtime.Stack(buf, true)
				// fmt.Printf("Stack trace:\n%s\n", buf[:stackSize])
				// exit with error code
				// os.Exit(1)
				dumpHeapProfile(fmt.Sprintf("/tmp/heap_dump_%d.pprof", time.Now().Unix()))
			}
			label := "[MONITOR]"
			if count > highest {
				highest = count
				label = "[MONITOR New Record]"
			}
			if monitor {
				fmt.Printf("%-22s 🧠 Memory:%4dMB | 💾 TotalAlloc:%4dMB | 📦 Sys:%4dMB | ♻️ GC:%4d | 🧵 Goroutines:%4d\n",
					label, allocMB, totalAllocMB, sysMB, mem.NumGC, count)
			}
		}
	}()
}

var monitor = false

func dumpHeapProfile(filename string) {
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	pprof.Lookup("heap").WriteTo(f, 0)
}

func getValidatorFromChainSpec(network_file chainspecs.ChainSpec) ([]types.Validator, error) {
	// Get the metadata from the validator
	keyvals := network_file.GenesisState
	currValidatorRawBytes := []byte{}
	for _, keyval := range keyvals {
		if keyval.Key[0] == 0x08 {
			currValidatorRawBytes = keyval.Value
			break
		}
	}
	currValidatorRaw, _, err := types.Decode(currValidatorRawBytes, reflect.TypeOf(types.Validators{}))
	if err != nil {
		return nil, err
	}
	currValidators := currValidatorRaw.(types.Validators)
	return currValidators, nil
}

func flagDescription(main string, options map[string]string) string {
	var b strings.Builder
	b.WriteString(main + "\nPossible values:\n")
	for key, val := range options {
		b.WriteString(fmt.Sprintf("  - %-10s %s\n", key+":", val))
	}
	return b.String()
}
