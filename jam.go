package main

import (
	"encoding/binary"
	"encoding/json"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"

	"fmt"

	"github.com/colorfulnotion/jam/chainspecs"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"github.com/spf13/cobra"

	"os"
	"path/filepath"
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

		logLevel string

		// run flags that is not supported yet
		pvmBackend  string
		peerID      int
		externalIP  string
		listenIP    string
		rpcListenIP string
		bootnode    string
		telemetry   string

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
			// Generate keys for validators
			_, _, err := GenerateValidatorSecretSet(types.TotalValidators, true, dataPath)
			if err != nil {
				fmt.Printf("Error generating keys: %s", err)
				os.Exit(1)
			}
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

				validator, err := generateSelfValidatorPubKey(seed)
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
			pvm.PvmLogging = true
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
			pvm.Set_PVM_Backend(pvmBackend)

			fmt.Printf("Running JAM DUNA node with the following flags:\n")
			fmt.Printf("\033[33mdataPath: %s, Port: %d, RPCPort: %d, validatorIndex: %d, debug: %s, chainSpec: %s, logLevel: %s, start_time: %s. pvm_backend: %s\033[0m\n", dataPath, Port, RPCPort, validatorIndex, debug, chainSpec, logLevel, start_time, pvm.VM_MODE)

			var err error
			var validators []types.Validator

			var selfSecret types.ValidatorSecret
			// Run the JAM DUNA node
			now := time.Now()
			loc := now.Location()
			pvm.PvmLogging = false
			log.InitLogger(logLevel)

			log.EnableModule(log.PvmAuthoring)
			log.EnableModule(log.PvmValidating)
			log.EnableModule(log.FirstGuarantorOrAuditor)
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
				fmt.Printf("Port from chainSpec: %d\n", Port)
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
			epoch0Timestamp := statedb.NewEpoch0Timestamp("jam", start_time)

			n, err := node.NewNode(uint16(validatorIndex), selfSecret, chainSpecData, epoch0Timestamp, peers, peerList, dataPath, Port)
			if err != nil {
				fmt.Printf("New Node Err:%s", err.Error())
				os.Exit(1)
			}
			n.SetServiceDir("/services")
			n.WriteDebugFlag = true
			storage, err := n.GetStorage()
			if err != nil {
				fmt.Printf("GetStorage Err:%s", err.Error())
				os.Exit(1)
			}
			defer storage.Close()
			fmt.Printf("New Node %d started, edkey %v, port%d, time:%s. buildVersion=%v pvm_backend=%v\n", validatorIndex, selfSecret.Ed25519Pub, Port, time.Now().String(), n.GetBuild(), pvm.VM_MODE)
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
		"interpreter": "Use a PVM interpreter. Slow, but works everywhere",
		"compiler":    "Use a PVM recompiler. Fast, but is Linux-only",
		"sandbox":     "Use a PVM recompiler sandbox for debugging",
	})
	runCmd.Flags().StringVar(&pvmBackend, pvmBackendFlag, "interpreter", desc)
	runCmd.Flags().IntVar(&peerID, peerIDFlag, 0, "Peer ID of this node. If not specified, a new peer ID will be generated. The corresponding secret key will not be persisted.")
	runCmd.Flags().StringVar(&externalIP, externalIPFlag, "", "External IP of this node, as used by other nodes to connect. If not specified, this will be guessed.")
	runCmd.Flags().StringVar(&listenIP, listenIPFlag, "", "IP address to listen on. `::` (the default) means all addresses. [default: ::]")
	runCmd.Flags().StringVar(&rpcListenIP, rpcListenIPFlag, "", "IP address for RPC server to listen on. `::` (the default) means all addresses. [default: ::]")
	runCmd.Flags().StringVar(&bootnode, bootnodeFlag, "", "Specify a bootnode")
	runCmd.Flags().StringVar(&telemetry, telemetryFlag, "", " Send data to TART server (JIP-3)")

	// add commands to root
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(genKeysCmd)
	rootCmd.AddCommand(listKeysCmd)
	rootCmd.AddCommand(runCmdSTF)
	rootCmd.AddCommand(testRefineCmd)
	rootCmd.AddCommand(genSpecCmd)
	rootCmd.AddCommand(printSpecCmd)

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
	selfSecrets, err := statedb.InitValidatorSecret(seed, seed, seed, []byte{})
	if err != nil {
		fmt.Printf("CheckValidatorInfo err %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("validatorIndex %d out of %d peers\n", validatorIndex, len(peerList))

	if selfSecrets.BandersnatchPub != peerList[uint16(validatorIndex)].Validator.Bandersnatch {
		fmt.Printf("Error: seed file does not match the metadata. self.BandersnatchPub=0x%x | peerList[%v].BandersnatchPub=0x%x", selfSecrets.BandersnatchPub, validatorIndex, peerList[uint16(validatorIndex)].Validator.Bandersnatch)
		os.Exit(1)
	}
	return selfSecrets
}

func GenerateValidatorSecretSet(numNodes int, save bool, dataDir ...string) ([]types.Validator, []types.ValidatorSecret, error) {

	seeds, _ := generateSeedSet(numNodes)
	validators := make([]types.Validator, numNodes)
	validatorSecrets := make([]types.ValidatorSecret, numNodes)

	for i := 0; i < int(numNodes); i++ {

		seed_i := seeds[i]
		if len(dataDir) != 0 {
			keyDir := dataDir[0]
			keyDir = filepath.Join(keyDir, "keys") // store the keys in a subdir
			// if there is no seed file, create it
			if err := os.MkdirAll(keyDir, 0700); err != nil {
				return validators, validatorSecrets, fmt.Errorf("failed to create keys directory %s: %v", dataDir, err)
			}
			if save {
				seedFile := filepath.Join(keyDir, fmt.Sprintf("seed_%d", i))

				if _, err := os.Stat(seedFile); os.IsNotExist(err) {
					// create the file
					f, err := os.Create(seedFile)
					if err != nil {
						return validators, validatorSecrets, fmt.Errorf("failed to create seed file %s", seedFile)
					}
					// write the seed to the file
					_, err = f.Write(seed_i)
					if err != nil {
						return validators, validatorSecrets, fmt.Errorf("failed to write seed to file %s", seedFile)
					}
					fmt.Printf("Seed file %s created\n", seedFile)
					f.Close()
				}

			}
		}

		bandersnatch_seed := seed_i
		ed25519_seed := seed_i
		bls_seed := seed_i
		metadata := []byte{}
		//metadata, _ := generateMetadata(i) // this is NOT used by other teams. somehow we agreed on empty metadata for now

		validator, err := statedb.InitValidator(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("failed to init validator %v", i)
		}
		validators[i] = validator

		//bandersnatch_seed, ed25519_seed, bls_seed
		validatorSecret, err := statedb.InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("failed to init validator secret=%v", i)
		}
		validatorSecrets[i] = validatorSecret
	}

	return validators, validatorSecrets, nil
}
func generateSelfValidatorPubKey(seed []byte) (types.Validator, error) {
	// Generate the validator public key from the seed
	validator, err := statedb.InitValidator(seed, seed, seed, []byte{})
	if err != nil {
		return types.Validator{}, fmt.Errorf("failed to init validator %v", err)
	}
	return validator, nil
}

func generateSeedSet(ringSize int) ([][]byte, error) {
	ringSet := make([][]byte, ringSize)
	for i := uint32(0); i < uint32(ringSize); i++ {
		seed := make([]byte, 32)
		for j := 0; j < 8; j++ {
			binary.LittleEndian.PutUint32(seed[j*4:], i)
		}
		ringSet[i] = seed
	}
	return ringSet, nil
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
				buf := make([]byte, 1<<20)
				stackSize := runtime.Stack(buf, true)
				fmt.Printf("Stack trace:\n%s\n", buf[:stackSize])
				// exit with error code
				fmt.Println("OOM: Out of memory")
				// os.Exit(1)
				dumpHeapProfile(fmt.Sprintf("/tmp/heap_dump_%d.pprof", time.Now().Unix()))
				os.Exit(1)
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
